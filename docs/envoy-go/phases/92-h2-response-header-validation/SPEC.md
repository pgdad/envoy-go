# Phase 92 — `h2-response-header-validation` — SPEC

**Lifecycle-state 1 -> 2.** Base master `221eedf4`. Row 92 stays `in-progress` at `ROADMAP.md:154`;
this stage leaves `ROADMAP.md` **BYTE-UNTOUCHED** and sentinel `want` STAYS **124**.

Every figure below was RE-DERIVED at `221eedf4` by execution. Where a measurement contradicts the
BRAINSTORM, the BRAINSTORM is named and corrected rather than quietly replaced. Where a claim remains
INFERRED it is labelled INFERRED.

---

## 1. Charter, unchanged in substance and bounded more tightly than the BRAINSTORM stated

**On the HTTP/2 downstream leg, REJECT a response whose UPSTREAM leading header block carries a
connection-specific or otherwise malformed field, instead of laundering it onto the downstream stream.**
This is the ENCODE-direction mirror of `h2.IsIllegalH2RequestHeader`, which row 89 landed on the DECODE
direction.

The BRAINSTORM's bounding claim — an H/2 downstream always drives an H/2 upstream, so the charter is
closed under one codec pair — **HOLDS**, re-derived: `AcquireH2Stream` has exactly two non-test
occurrences repo-wide, its definition (`internal/cluster/h2pool.go`) and its single call site in
`doH2ClusterAction`. Two nuances are recorded rather than smoothed:

- ⚠️ **"Unconditionally" is true of the CODEC PAIR and FALSE of the CALL COUNT.** `H2ClusterAction`
  dispatches through hedge / retry / direct arms, all three converging on `doH2ClusterAction`, so the
  validator is **re-entered once per attempt**. This is load-bearing for D-92-POSTURE below.
- ⚠️ **An H3-in/H1-out path DOES exist** (`h3dispatch.go` calls the H1 `asRouterAction()` builder, with
  an in-tree comment saying so). H/3 is outside this charter, but no prose in this row may claim the tree
  is free of downstream/upstream codec mismatch in general.

---

## 2. What this SPEC refuted by execution

**One of the refutations is of this SPEC's own controller-side hypothesis, and it is stated first.**

1. ⚠️ **THE EVICTION HYPOTHESIS IS REFUTED. THE CONNECTION TEARDOWN IS PARITY, NOT A NEW DIVERGENCE.**
   Reading `doH2ClusterAction` showed that the 502 arm calls `EvictH2ConnOnError` -> `evictH2ConnLocked`
   -> `_ = pc.cc.Close()`, a hard close of a SHARED MULTIPLEXED conn, while the trailer arm deliberately
   sits above it. The hypothesis drawn from that — *"matching the status code while diverging on
   connection lifetime is a NEW divergence introduced by the fix"* — **is FALSE at the default posture.**
   Measured with both sides on ONE downstream connection and `BACKEND_ACCEPTED_TCP_CONNS=1`:

   | | offending stream | legal in-flight sibling | NC (`/slow` + `/ok`) |
   |---|---|---|---|
   | subject, patched | 502 | **502 — killed** | both 200 |
   | reference, DEFAULT posture | 502 | **502 — killed** | both 200 |
   | reference, cluster posture `true` | 502 | **200 — survives** | both 200 |

   Reference default-posture deltas: `upstream_cx_destroy_local` 0->1, `upstream_cx_protocol_error` 0->1,
   `upstream_rq_tx_reset` 0->**2** (BOTH streams reset), with `BACKEND_READ_ERR EOF` logged before `/slow`
   could answer. **The reference treats a malformed response as a CONNECTION-level protocol error and
   inflicts exactly the same collateral harm.** ⇒ the banked claim that hard-closing a shared conn is
   *"true for the accounting and FALSE for every sibling stream"* is **accurate as a description of the
   harm and WRONG as a divergence claim**. The real gap is narrower: **the reference offers an OPT-OUT
   and envoy-go has none.**

2. ⚠️ **THE POSTURE FLAG IS THE WRONG KNOB IN THE BRAINSTORM'S FRAMING.** §6.1 gap 3 asks whether the
   guard belongs behind a `stream_error_on_invalid_http_messaging`-style posture, citing ADR-0312
   §Context ¶4's measurement that the reference's **request-side** reject is posture-dependent. Measured
   on the RESPONSE side, the **HCM (downstream) flag has NO EFFECT**; the controlling field is the
   **CLUSTER's** `http2_protocol_options`. Both spellings (`stream_error_on_invalid_http_messaging` and
   `override_stream_error_on_invalid_http_message`) behave identically. **The downstream STATUS is
   posture-INVARIANT at 502; only CONNECTION LIFETIME moves.**

3. ⚠️ **THE COST FLOOR IS SHORT FOR A SIXTH CONSECUTIVE ROW, AND THE CAUSE IS NOW NAMED PRECISELY.**
   Two independent prototypes measured **`+74 / -0`** and **`+77 / -0`**, both ONE file
   (`internal/filter/hcm/h2/client.go`), both `gofmt` clean and `golangci-lint` rc=0 with zero output.
   The `+74` splits **code=44, comment=27, blank=3** — the banked `+44` matches the CODE-ONLY count
   EXACTLY. ⇒ (INFERRED) the BRAINSTORM's prototype was comment-free, and `+44` is a **comment-free lower
   bound**, not an under-enumeration of behaviour. At repo commenting density the single-file floor is
   **74-77**.

4. ⚠️ **`0120`'s PORT IS NOT FREE, AND TWO INHERITED DOCUMENTS SAY IT IS.**
   `test/fixtures/0028-http-lua-multi-script-and-per-route/inputs/driver.go:65` holds
   `refLATestPort = 10120` (with `:66-70` holding 10121-10125). This refutes
   `phases/88-h2-continuation-frames/PLAN.md:303` (*"a `0120` would take **10120**"*) and
   `phases/89-h2-decode-filter-mutations/SPEC.md:222` (`| new port | none | 10120 | none | none |`).
   Census of `101xx` in `test/`: **10100-10125 and 10130-10140 are taken; 10126-10129 are free.**
   Verified independently at this tip: `10120` is referenced by one file, `10126` and `10127` by none.

5. ⚠️ **THE `\t`-IN-ERE TRAP IS TOOL-DEPENDENT, AND TWO AGENTS CONTRADICTED EACH OTHER ON IT.**
   One measured `grep -cE '^\t_ "'` => **0**; another measured **123** on the same file at the same tip.
   Both are correct. Resolved by the controller: `grep` in this harness is a **shell function** wrapping
   **ugrep 7.8.4**, whose ERE DOES honour `\t`; `/usr/bin/grep` is **GNU grep 3.11**, whose ERE matches a
   literal `t` and returns 0. Confirmed here:
   `GNU -cE : 0` · `GNU -cP : 123` · `GNU -cE` with `$(printf '\t')` : **123**.
   ⇒ **the standing record's blanket "ERE reads 0" is true only of GNU grep. Any gate restating it MUST
   NAME THE TOOL.** The safe forms — `-P`, or `$(printf '\t')` — are correct under both.

6. ⚠️ **THE INHERITED ADR HEADING COUNTS ARE STALE BY ONE.** `^## ADR-` in `docs/envoy-go/DECISIONS.md`
   is **312**, not 311; bare `^## ` is **320**, not 319. The `+8 ## Amendment` relationship is INTACT
   (all eight enumerated). Cause named: 311/319 was measured at the phase-90 IMPL close when the tail was
   ADR-0312; **the phase-91 SPEC drafted ADR-0313 §Context, adding one heading.**
   ⇒ ⚠️ **the headings+1 collision is now WORSE than recorded**: headings+1 = **313**, which is the TAIL
   ITSELF — a TAKEN id. A session using that form would try to write ADR-0313 a SECOND time. Gap audit
   over `0001..0313`: exactly ONE missing id (`0209`), ZERO duplicates. **Derive next-free from the TAIL.**

7. ⚠️ **THE RECORDED REMEDY FOR THE ` + ` SPLIT TRAP IS ITSELF INSUFFICIENT.** The trap reproduces (the
   `awk` regex split reads **1 on all six** windows). But the prescribed correct form
   `sed 's/ + /\n/g' | wc -l`, run as written on the whole ROADMAP cell, reads
   `:202` 7 · `:208` 10 · `:214` 12 · `:224` 92 · `:230` 8 · `:238` 19 = **148**, because it splits every
   ` + ` in the entire cell including prose outside the list. List-scoped and semantically read the total
   is **40** — the inherited total REPRODUCES — but **no single mechanical command produces it**:
   `:214` carries ` + ` INSIDE one candidate (`upstream SDS (server-cert + validation_context)`); `:224`'s
   list ENDS mid-cell with unrelated bold prose after it; `:238` is **comma-delimited**, so a ` + `
   splitter reads 1. ⇒ **no gate in this row may rest on a ` + ` split in any form.**

8. **`IsIllegalH2RequestHeader` HAS NO FUZZ TARGET** — the opposite of the hypothesis put to the probe.
   Six repo-wide hits: its definition, two doc lines, its production call site, a fixture comment. Zero
   in any `*fuzz*_test.go`. Same for `isConnectionSpecificField` and `hasUppercaseHeaderChar`. See D-92-FUZZ.

9. **`codec_type: HTTP2` appears ZERO times across the fixture corpus.** Measured values: `HTTP1` × 267,
   `AUTO` × 5, `HTTP3` × 3, `HTTP2` × **0**. All four H2-capable fixtures are `AUTO` + ALPN-driven,
   confirming the phase-91 finding that the real discriminator is the downstream ALPN `h2` offer.

**Claims that REPRODUCED unchanged** (recorded so they are not re-probed): the seven-shape divergence
table and the three parity arms; the `ErrMalformedTrailers` design constraint, now confirmed AT THE LIVE
BRANCH and by its own in-tree comment (*"THE DISCRIMINATOR IS THE SENTINEL, NOT THE CODE"*); the
655 `=== RUN` / 0 FAIL baseline; the H2-capable fixture set `{0004, 0079, 0080, 0119}`; fixtures **121**
with `0120` unconsumed as a DIRECTORY NAME; fuzzers **55 / 48**; BackendKind tail **38
`H2GoawayResponder`**; stat surface **406**; `go.mod` **67**; `^---$` **216**;
`BEHAVIOR_CONTRACT.md` **5962**; row 92 **NF=8 under both forms**; malformed-row baseline **2**
escape-aware / **17** naive; all six documented gate traps.

---

## 3. The mechanism, re-derived at this tip

Cite BY SYMBOL. ⚠️ A symbol assertion whose receiver is written `(cc *ClientConn)` MUST use `grep -F` —
ERE reads the parentheses as a group and returns a FAIL-UNSAFE ZERO, reproduced here (`-F` reads 1,
`-E` reads 0).

| symbol | file | line |
|---|---|---|
| `(cc *ClientConn).onResponseHeaderBlock` | `internal/filter/hcm/h2/client.go` | 609 |
| `validateResponseTrailers` | `internal/filter/hcm/h2/client.go` | 788 |
| `hasUppercaseHeaderChar` | `internal/filter/hcm/h2/client.go` | 779 |
| `isConnectionSpecificField` | `internal/filter/hcm/h2/stream.go` | 392 |
| `IsIllegalH2RequestHeader` | `internal/filter/hcm/h2/stream.go` | 414 |
| `ErrMalformedTrailers` | `internal/filter/hcm/h2/client.go` | 716 |
| `doH2ClusterAction` | `internal/filter/http/router/router_h2.go` | 73 |
| `writeH2Reply` | `internal/filter/hcm/h2dispatch.go` | 1004 |
| `reconcileH2DecodeDelta` | `internal/filter/hcm/h2dispatch.go` | 866 |

⚠️ `isConnectionSpecificField` lives in `h2/stream.go`, **co-located with `IsIllegalH2RequestHeader`** —
the decode-direction mirror. The encode-direction validator has a natural home beside them, but see
D-92-VALIDATOR for why it does not go there.

**The asymmetry.** `onResponseHeaderBlock` branches on `cs.respHeadersSeen`: the TRAILING block is
validated by `validateResponseTrailers`; the LEADING block is stored verbatim as `cs.respHeaders = decoded`
and validated by nothing. `doH2ClusterAction` forwards the stored set stripping only `:`-prefixed names.
`writeH2Reply` applies no §8.2.2 filter. ⇒ **no validation anywhere on the leading-block encode path.**

### 3.1 ⚠️ THE CRUX RESOLVES IN THE FIX SITE'S FAVOUR — MEASURED, NOT REASONED

The BRAINSTORM warned that arm 7 (an uppercase name) is the one a code-reading misses, because
`writeH2Reply` lowercases every name so the downstream wire is *syntactically legal* H/2. That raised a
question the BRAINSTORM did not answer: **if something normalizes the name before the fix site, a guard
there cannot fire on arm 7 and the whole fix-site choice is wrong.**

Instrumenting the `if !cs.respHeadersSeen` branch directly and driving a real response through
`RoundTrip`:

```
PROBE-SITE-LEAD stream=1 nfields=5 endStream=false
PROBE-SITE-LEAD[0] name=":status" value="200"
PROBE-SITE-LEAD[1] name="X-Upper-Case" value="yes"      <-- UPPERCASE INTACT
PROBE-SITE-LEAD[2] name="connection" value="keep-alive"
```

**The uppercase name survives intact to the fix site.** `writeH2Reply`'s lowercasing
(`h2dispatch.go:1012`, `:1023`) is DOWNSTREAM of it. `client.go:434` hands raw `HeaderBlockFragment()`
bytes to `onResponseHeaderBlock`, which decodes with the package's own `hpackState.decodeBlock`; `x/net`'s
decoder does not case-fold and `ReadMetaHeaders` is not in play on this path. **Nothing normalizes
upstream of the guard.** ⇒ the fix site is CORRECT and sufficient for all seven arms.

### 3.2 Duplicate `content-length` is FULLY VISIBLE at the fix site

Sending `content-length: 5` then `content-length: 99`:

```
PROBE-SITE-LEAD[3] name="content-length" value="5"
PROBE-SITE-LEAD[4] name="content-length" value="99"
PROBE-RT ntype=[]hpack.HeaderField content-length-count=2
```

A slice preserving both in wire order; nothing collapses at the site or below it (`doH2ClusterAction`
projects onto `OrderedHeaders`, also a slice). **Arm 10 needs no second site.**
⚠️ Note that `writeH2Reply` additionally rewrites EVERY `content-length` to `len(body)`
(`h2dispatch.go:1014-1016`), so at the unbroken tip arm 10 leaves the subject wire carrying **two
identical CLs** — a second reason a downstream-bytes-only fix would misread this arm.

---

## 4. D-92-POSTURE — the reject disposition. **502, evicting, NOT retriable.**

The BRAINSTORM framed this as a two-way choice, "502 (measured parity) versus stream reset", and told the
SPEC to pin it to the `ErrMalformedTrailers` sentinel mechanism. The mechanism is real and confirmed. But
it carries **two consequences the BRAINSTORM never named**, so the option set is three-way, and the third
option does not exist in the tree today.

Measured properties of the two EXISTING arms:

| | downstream status | pooled conn | retriable |
|---|---|---|---|
| sentinel arm (`ErrMalformedTrailers`) | `Status: 0` — stream reset, NOT 502 | kept | no |
| non-sentinel fall-through | **502** | **evicted (hard `Close()`)** | **YES — `localOrigin: true`** |
| **reference, default posture** | **502** | **destroyed** | **NO** |

- The stream-reset arm fails parity on the STATUS, which is posture-INVARIANT at 502 on the reference.
- The non-sentinel fall-through matches the reference on status AND on connection lifetime (§2 item 1),
  but **introduces a retry divergence the reference does not have**, measured:

  | | backend-observed attempts | `upstream_rq_retry` | `upstream_cx_total` |
  |---|---|---|---|
  | subject, patched, `retry_on: connect-failure`, `num_retries: 2` | **3**, on **3 separate TCP conns** | **2** | 1 -> **3** |
  | reference, same config | **1** | **0** | no extra |

  Subject-side also: `retry_limit_exceeded` 0->1, `retry_backoff_exponential` 0->2, `http2_tx_reset` 0->3.
  Cause: `localOrigin: true` makes `RetryPolicy.matches` (`retry.go:128`) classify a **perfectly
  reachable** but malformed upstream as a CONNECT FAILURE. Compounded by §1's nuance — the validator is
  re-entered once per attempt — and by the eviction, which forces a fresh dial each time.

**DECISION.** The new validator returns an `*h2.Error` carrying a NEW package-level sentinel
`ErrMalformedResponseHeaders` — explicitly **NOT** `ErrMalformedTrailers`, whose arm would give a
non-parity `Status: 0`. `doH2ClusterAction` gains a THIRD arm, placed **AFTER** `EvictH2ConnOnError` so
the eviction still happens (that is the parity behaviour), returning **502 WITHOUT `localOrigin`**:

```go
// after EvictH2ConnOnError, before/beside the ctx-cancel discrimination
if errors.Is(err, h2.ErrMalformedResponseHeaders) {
    a.cluster.IncStatusClass(502)
    a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: 502, LocalOriginErr: true})
    return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders()}, picked, nil
}
```

This yields 502 + eviction + no retry = the reference's default posture on all three axes.

⚠️ **This arm is a SECOND FILE IN A SECOND PACKAGE, and neither the `+74` nor the `+77` prototype
includes it.** It is the principal reason the single-file floor understates the row.

⚠️ **`RecordUpstreamResult`'s `LocalOriginErr` field is deliberately left `true` while `ActionResponse`'s
`localOrigin` is left unset.** They are different consumers — the former feeds outlier detection, the
latter feeds retry classification — and the PLAN must NOT "tidy" them into agreement without measuring
the outlier-detection consequence separately. This is flagged because it looks like an inconsistency and
is not.

### 4.1 The posture FLAG is OUT OF CHARTER, and the reason is measured

envoy-go parses **neither** spelling of the cluster posture field — zero `.go` hits repo-wide for either
— and booting with the field set runs normally, i.e. it is **SILENTLY IGNORED** (the documented phase-05.2
disposition). Adding config parsing plus a permissive code path is a separate row with its own config
surface. **This row hard-codes the DEFAULT posture**, which is the one that matches an unconfigured
reference, and records the opt-out gap as a banked candidate.

---

## 5. D-92-VALIDATOR — two independent functions sharing the three carrying predicates

`validateResponseTrailers` has exactly **six** legs. Only **three** carry over to a leading block; three
**invert**:

| leg | helper | carries? |
|---|---|---|
| `!endStream` -> reject | framing, pre-loop | **NO** — a leading block has no END_STREAM requirement |
| any `:`-prefixed name -> reject | inline | **NO** — `:status` is REQUIRED in a leading block |
| uppercase in name | `hasUppercaseHeaderChar` | **YES** |
| `isConnectionSpecificField(name)` | `stream.go:392` | **YES** |
| `name=="te" && value != teTrailersValue` | `teTrailersValue` | **YES** |
| `name=="content-length"` -> reject | inline | **NO** — ONE is legal; only a DUPLICATE is not |

**DECISION: two independent functions sharing the existing package-level predicates. Not a mode flag,
not thin wrappers.** Justified from the enumerated difference plus three hard constraints:

1. A boolean that changes the majority of a function's legs is a second function wearing one name.
2. ⚠️ **The duplicate-`content-length` leg is not a predicate at all** — it is a CROSS-FIELD COUNT across
   the loop, which cannot be expressed in the trailer validator's `switch` over one `(name, value)`. That
   alone forecloses "one body, one flag".
3. The error CONSTRUCTOR must differ, because sentinel selection at `router_h2.go:188` is what picks the
   arm. A shared body would have to thread the constructor AND every message string
   ("…in a trailer section") through the flag as well.

⚠️ **DO NOT collapse the connection-specific and `te` legs into the existing `IsIllegalH2RequestHeader`.**
That is the tempting "share more" move and it BREAKS trailers: `TestValidateResponseTrailers_Table/te_gzip`
asserts `wantMsgSubstr: "not 'trailers'"` while each connection-specific member asserts its own quoted
name; merging collapses both messages and **reddens the existing table**.

**Trailer behaviour must not change, and the prototype proves it can be held invariant** — RUN sets
byte-identical by `diff` across `./internal/filter/hcm/... ./internal/filter/http/router/...` (655 RUN /
0 FAIL before and after), across `-run 'Trailer'` (63 RUN / 19 top-level), and across the five named h2
trailer tests (34 RUN). The IMPL must re-run all three denominators and show the SETS, not just the counts.

### 5.1 An OPEN ARM the PLAN must close

The prototype rejects **any** second `content-length`, **including two with identical values**. Whether
`contrib-v1.37.2` answers 502 for identical duplicates is **UNMEASURED** — the measured arm used differing
values. **The PLAN owes this measurement.** If the reference accepts identical duplicates, the rule
narrows to "duplicate with differing values"; if it rejects, the rule stands as prototyped. **Do not ship
the broad rule on the assumption that it is parity.**

---

## 6. The six named gaps, priced

| # | gap | verdict | cost |
|---|---|---|---|
| 1 | stat surface | **IN CHARTER — one new counter** | see 6.1 |
| 2 | error text / `%RESPONSE_CODE_DETAILS%` | **OUT — no analogue exists** | see 6.2 |
| 3 | posture flag | **OUT — field is unparsed** | §4.1 |
| 4 | arm 7's asymmetry | **CLOSED — guard catches it** | §3.1, +0 |
| 5 | trailing-block relationship | **CLOSED — two functions** | §5, inside the +74/77 |
| 6 | differential surface | **IN CHARTER — extend 0004** | §7, +193/-0 |

### 6.1 Stat surface — the subject cannot distinguish this rejection from any other 502

Measured per rejected response, side by side:

| counter | subject (patched) | reference |
|---|---|---|
| `cluster_http2_rx_messaging_error` | **DOES NOT EXIST** | **+1** — the dedicated signal |
| `cluster_upstream_cx_protocol_error` | **DOES NOT EXIST** | **+1** |
| `cluster_upstream_cx_destroy` / `_destroy_local` | **DO NOT EXIST** (no `cx_destroy*` at all) | +1 / +1 |
| `cluster_upstream_rq_tx_reset` | absent — spelled `cluster_http2_tx_reset` | +1 |
| `cluster_http2_tx_reset` | **+1** | exists, stays 0 |
| `cluster_upstream_rq{code="502"}` | **DOES NOT EXIST** (no per-code cluster stat) | +1 |
| `cluster_external_upstream_rq*`, `_rq_completed`, `_rq_pending_total` | **DO NOT EXIST** | +1 each |
| `upstream_cx_total` / `_cx_http2_total` | +1 / +1 (RE-DIAL) | +1 / +1 |
| `upstream_rq_total`, `upstream_rq_xx{5}`, `http_downstream_rq_total` | +1 each | +1 each |
| `downstream_rq_5xx` | **DOES NOT EXIST** — it is `downstream_rq_xx{class="5"}`, +1 | `downstream_rq_xx{5}` +1 |

Scrape control PASSED (an ordinary `/ok` moved `downstream_rq_total` 9->10), so a null delta is not a dead
scrape.

**DECISION: add exactly ONE counter, the `http2.rx_messaging_error` analogue at cluster scope**, because
the only counter that moves today (`http2_tx_reset`) is **already shared with the trailer-reject path** and
therefore cannot discriminate. Stat surface **406 -> 407**. The name must pass `stats.IsValidName`.
⚠️ Assert it **subject-side only** — cross-side stat scope is a known divergence axis, and the reference's
spelling differs (`upstream_rq_tx_reset` vs `http2_tx_reset`) even where the event matches.
⚠️ A COUNTER CANNOT GATE A VALUE: the counter pins that a rejection happened, never which field caused it.
The field-level assertion belongs to the differential arm and the unit table.

### 6.2 `%RESPONSE_CODE_DETAILS%` — no analogue exists anywhere in envoy-go's HTTP stack

The reference emits, identically for all seven arms **and for the collateral-killed sibling**:

```
ACCESSLOG path=/connection code=502 flags=UPE details=upstream_reset_before_response_started{protocol_error}
```

The subject: exactly ONE repo-wide `.go` hit and **it is a comment** (`extproc/check.go:540`); the
access-log `log_format` is **BOOT-REJECTED** (`rc=1`, *"unsupported config: access_log[].log_format"*), so
the operator cannot even be REQUESTED; the default format hardcodes `" - -"` where `%RESPONSE_FLAGS%`
would go; the OTLP operator table has no RCD entry and strict-rejects unknown operators; wasm's
`ResponseCodeDetails()` returns a hardcoded `("", false)`; `network/chain.go`'s `SetResponseCodeDetails`
is network-filter-scope only and reaches no access-log operator.

⇒ **the row CANNOT pin the reference's details string cross-side, and this is a DEPARTURE stated in
writing, not an omission.** Banked as a candidate: `%RESPONSE_CODE_DETAILS%` is a multi-row surface
(access-log format parsing + a plumbed field + operator tables), far larger than this charter.

---

## 7. D-92-DIFF — extend fixture `0004`. Three shapes on the wire, four at the unit layer.

**The arm is FEASIBLE and was executed end to end, both sides, before being chosen** — with a
direct-to-backend discriminating negative control, positive controls first AND last, and two
reproducibility runs per side:

```
DIRECT /p92-keepalive status=200 illegal=[keep-alive]     <-- instrument CAN see it
REF    /p92-keepalive status=502 illegal=[]
SUBJ   /p92-keepalive status=200 illegal=[keep-alive]     <-- LAUNDERED
```

**Priced by a COMPILING PROTOTYPE**, written, vetted, measured, reverted:

```
31   0  test/fixtures/0004-h2-routing/backends/main.go
162  0  test/fixtures/0004-h2-routing/driver/driver.go
```

**+193 / -0, two files.** `gofmt -l` empty · `go vet ./test/fixtures/0004-h2-routing/...` rc=0 ·
`go vet ./test/differential/...` rc=0 · `go test -c ./test/differential/` rc=0. Plus **0 YAML lines**
(the new paths fall through the existing `- match: { prefix: "/api" }`), **0 new BackendKind**,
**0 registration gates**, **0 port allocations**, and `BackendCount()` unchanged at 3 so
`AssertDistribution`'s `[3,3,3]` is untouched.

**Rejected alternatives, priced:**
- **Mint `0120` + a raw-framer BackendKind 39**: ≈ **1440 lines across ~8 files**, three registration
  gates, one port allocation. ~7.4× option (a) to buy four extra shapes that a unit table covers for a
  fraction of that. ⚠️ And `0120`'s expected port **10120 is TAKEN** (§2 item 4) — it would need **10126**.
- **Unit-only**: forfeits a divergence now PROVEN reachable at 193 lines. An infeasibility claim is not
  available, because the arm was executed.

### 7.1 ⚠️ THE DEPARTURE, STATED PRECISELY — four of seven shapes are STRUCTURALLY unreachable

Measured against the pinned `x/net` from an ordinary `net/http` backend (the `HTTPSH2` kind-2 shape that
`0004` already spawns):

- **Expressible (3):** `keep-alive`, `upgrade`, `proxy-connection` — they leak because
  `x/net/http2/server.go:2757` carries a live `TODO: remove more Connection-specific header fields here`.
- **NOT expressible (4):** `connection` (deleted at `server.go:2759`,
  `delete(rws.snapHeader, "Connection")`); `transfer-encoding` (does not survive);
  an uppercase wire name (`http.Header` canonicalization + the encoder's `lowerHeader`);
  a duplicate `content-length` (collapsed — `snapHeader.Del` then a single re-add).

These four are unreachable for a **structural** reason in the library, not a budget reason, so the
departure is defensible and precisely statable. They are pinned **at the unit layer only**, and reaching
them on the wire would need BackendKind 39 (≈ +303 runner lines) — deliberately not taken.

⚠️ **In-tree precedent for exactly this trade-off**, one row old, `0004/driver/driver.go:445`:
*"Recovering it would need a raw-framer backend (a new BackendKind for one assertion). Wire ORDER is
pinned at the UNIT layer instead."*

### 7.2 Gate discipline for the differential arm

⚠️ **ASSERT THE FIXTURE SET, never "the suite was green."** Verified at this tip: the runner `t.Skipf`s an
unregistered fixture at **`test/differential/runner_test.go:200`** (exact line), and **there is no
fixture-count gate anywhere in the tree** — `DriverRegistry` is read at exactly one site (`:194`), and
CI's differential job is a bare `go test ./test/differential/... -timeout 20m -v` with no count assertion.
That absence was established with a POSITIVE CONTROL so the zero is not a broken selector: the same search
form DOES find real count gates elsewhere (`0070/driver_test.go:58` `want 3`,
`0004/driver.go:1061` `want 3`, `internal/runtime/snapshot_test.go:85` `expected 14 rows`).

⇒ the IMPL must show the subtest RAN:
`go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v`, confirming the
`=== RUN   TestDifferential/0004-h2-routing` line appears. ⚠️ A `-run` matching nothing prints
`[no tests to run]` and **EXITS 0**.

⚠️ **Two arm-shape hazards, both already handled in the prototype:**
- The transcript line must record the **SET** of illegal names present, not one name — a shared code path
  defeats per-arm counts.
- **Each shape needs its OWN backend path.** A single path emitting all three is blind to a fix that
  catches one and launders another.

⚠️ **If any future arm on the connection-lifetime axis is built** (it is NOT in this row), both streams
must ride **ONE downstream connection**: the reference's upstream pools are **per-worker / thread-local**,
and two downstream conns landed on two upstream conns, invalidating a first attempt. A backend must also
serve **one goroutine per stream** — a synchronous backend blocks the read loop and the streams are never
concurrent, which produced a dead probe that was caught and discarded.

---

## 8. D-92-FUZZ — **YES.** One target, ~60 LoC.

Counts re-derived: **55 targets / 48 files**; CI's `fuzz-smoke` matrix drives **10** of the 55 for 30 s
each, of which the h2 package contributes `FuzzHPACKDecode` and `FuzzFrameStream`.

The precedent argument is the OPPOSITE of the one hypothesised, and it argues FOR the target:
**`IsIllegalH2RequestHeader` has NO fuzz target**, nor does `isConnectionSpecificField` or
`hasUppercaseHeaderChar`, nor does `validateResponseTrailers`. ⚠️ **Reachability is not coverage:**
`FuzzFrameStream` transitively reaches `isConnectionSpecificField` via `buildRequest` (`stream.go:494`),
but its only assertion is *"no panic + every error begins with `h2:`"* — it can never observe a wrong
classification. `FuzzHPACKDecode` reaches no predicate at all. The encode/response direction has **no
fuzz reach whatsoever**; there is no `ClientConn` fuzz target in the tree.

**Ship `FuzzValidateResponseHeaderBlock`** over arbitrary `[]hpack.HeaderField`, asserting (i) no panic,
(ii) every rejection message carries the `h2:` prefix and names the offending field **quoted in trailing
position** — the discipline `client.go:770-774` documents as load-bearing for falsifiability — and
(iii) the accept/reject verdict agrees with an independently-written oracle over the closed name set.
Fuzzers **55 -> 56 targets / 48 -> 49 files**. Precedent: `FuzzDrainTransitions` (ADR-0018).

---

## 9. D-92-1XX — **correct it, in the accurate form.**

The wrong claim is the tail of `internal/filter/hcm/h2/client.go:673`, inside the NAMED NON-GOAL comment
in the very symbol this row edits: *"the reference FORWARDS 1xx"*. Measured on `contrib-v1.37.2` at
default HCM config, it does NOT — it SWALLOWED both a `103` and a `100` and delivered only the final 200.

Priced from real edits, then reverted:

| correction | `--numstat` | gates |
|---|---|---|
| minimal (`FORWARDS` -> `SWALLOWS`) | `1 1` | gofmt clean |
| **accurate restatement** carrying the measurement + the "drop-and-deliver, not forward" design note | **`10 3`** | gofmt clean, lint rc=0 zero findings |

**DECISION: take the 10/3 accurate restatement.** The minimal edit fixes the word and leaves the next
reader to re-derive what the reference actually does; the accurate form records the measurement so a
future 1xx row designs against *drop-and-deliver* rather than *forward*.

**Scope is settled — exactly ONE wrong instance tree-wide.** ⚠️ `ROADMAP.md:154` and `next-prompt.txt:119`
carry the same string only in order to REFUTE it; they are already correct and **must not be "fixed"**.
No ADR mentions 1xx at all; every other hit is unrelated H1 `Expect: 100-continue` material.

---

## 10. Negative controls, NAMED BEFORE THEY ARE WRITTEN

⚠️ At the phase-91 IMPL **three of that row's own new pins were non-discriminating and ALL THREE PASSED
WHEN WRITTEN** — one structurally unable to fire, one vacuous, one a coin flip. **A test that passes is
not thereby a guard.** Every pin below gets an NC, and the NC result is recorded, not assumed.

| pin | the NC that must redden it | why it could be vacuous |
|---|---|---|
| each of the 7 unit reject arms | delete that leg from `validateResponseHeaders` | a leg may be shadowed by an earlier leg — vary the ORDER, not just the presence |
| the 3 parity arms (OWS, empty value, legal trailers) | make the validator reject everything | a parity arm that never reaches the validator is vacuous |
| the sentinel-discrimination pin | swap the new sentinel for `ErrMalformedTrailers` | must redden on the ARM TAKEN, not merely on the error value |
| **the no-retry pin** | restore `localOrigin: true` on the new arm | ⚠️ needs `retry_on` CONFIGURED; without it the pin cannot fire under any input |
| the eviction pin | remove `EvictH2ConnOnError` from the path | ⚠️ a stacked control is needed — a passing arm cannot catch an OVER-firing evict |
| the new counter | delete the `Inc` | ⚠️ assert the DELTA from a baseline, never the absolute |
| the differential arm | revert the production guard only | must redden on **each** of the 3 shapes SEPARATELY |
| trailer invariance | — | ⚠️ this is a NON-regression, so its "NC" is the RUN-SET diff, not a red run |

⚠️ **A `panic()` REACHABILITY CONTROL is mandatory before concluding any site is unpinned** — the
BRAINSTORM's green was proven non-vacuous only by one (`--- FAIL: TestChainIntegration_H2_DirectResponseHappy`),
and the distinction between assertion blindness and dead code cannot be made by grep.

⚠️ **HARNESS CONSTRAINT THE IMPL MUST INHERIT:** the leading-block reject writes RST_STREAM **from the
readLoop goroutine**, so a wire test MUST use `dialClientConnTCP`, **NOT** `dialClientConn`. A first probe
over `net.Pipe` **DEADLOCKED and was killed at 120 s**. `trailers_validate_test.go:281-290` already
documents this for the trailer path; it applies identically here.

---

## 11. Anticipated counts and cost

| axis | at this tip | anticipated at row-done |
|---|---|---|
| stat surface | **406** | **407** (+1, §6.1) |
| fuzzers | **55 / 48** | **56 / 49** (+1/+1, §8) |
| BackendKind tail | **38** `H2GoawayResponder` | **38** (+0 — no new kind, §7) |
| fixtures | **121**, `0120` unconsumed | **121** (+0 — extend, do not mint) |
| blank imports | **121 / 122 / 123** (scopes below) | +0 |
| `go.mod` / `go.sum` | 67 modules | BYTE-UNTOUCHED |
| config fields | — | **+0** (the posture flag is OUT, §4.1) |
| `^---$` in `DECISIONS.md` | **216** | **216** |
| `^## ADR-` / bare `^## ` | **312 / 320** | **313 / 321** |
| ADR tail / next-free | `ADR-0313` / `ADR-0314` | `ADR-0314` / `ADR-0315` |
| `BEHAVIOR_CONTRACT.md` | **5962** | rider expected; state the delta at the IMPL |

⚠️ **Blank-import scopes, because three different numbers are all correct** — **121** fixture-driver
imports in `runner_test.go` · **122** fixture-driver imports repo-wide (the extra is
`test/fixtures/0018-http-rbac/inputs/driver.go`) · **123** ALL blank imports in `runner_test.go` (the two
non-fixture ones are lua `:157` and ratelimit `:168`). A fourth number exists — repo-wide ALL = **145** —
and is NOT one of the three. **State the scope whenever the number is restated.**

**COST ESTIMATE.** Production: **~+74-77** in `internal/filter/hcm/h2/client.go` (two independent
prototypes) **plus the third router arm** in `internal/filter/http/router/router_h2.go` **plus one
counter** — call it **~+95-130 net production `.go` across two packages**. Test: unit table + NC budget
**~+250-450**, differential **+193** measured, fuzz **~+60**. ⚠️ **AN ESTIMATE, NOT A MEASUREMENT — and
the single-file figures are LOWER BOUNDS that have now been short six rows running.**

---

## 12. Sentinel — RUN MECHANICALLY AT `221eedf4`, ACTUAL OUTPUT

- **(1)** `want=124` -> **`NOT DONE: row 92`**, ONE line.
- **(2)** **SIX**, at `:202 :208 :214 :224 :230 :238`.
- **(3)** **SILENT.**

⇒ **TWO checks block the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED**, verified absent
at the git root.

**All four NCs fired:**
- **NC-A** (row 62 doctored): `NC LANDED? [ in-progress ]` **inspected FIRST**, then **TWO** lines —
  `NOT DONE: row 62` AND `NOT DONE: row 92`. ⚠️ the two-line shape is the OPEN-ROW board's, and does not
  carry to an all-`done` board.
- **NC-B** (`want=123` on the real file): `NOT DONE: row 92` AND
  `GATE FAIL: examined 124 data rows, expected 123` — **two lines, not one.**
- **NC-C**: residual `gRPC-family row` **2 -> 0** => `NEVER OPENED: gRPC` fired; the WASM control
  correctly stayed silent.
- **NC-D**: alternation split — long **5** / short **1** / union **6**.

Row 92 field-counts **NF=8 under BOTH** the naive and escape-aware forms. Malformed-row baseline is **2**
escape-aware (ids **57** NF=9, **69** NF=10) and **17** naive. ⚠️ The two forms DISAGREE on row 57
(naive 13 vs escape-aware 9), so **any gate must state which form it uses**, and a gate asserting `== 0`
FAILS on pre-existing content.

---

## 13. What the PLAN owes

1. **Close the OPEN ARM in §5.1** — measure whether the reference rejects **identical** duplicate
   `content-length` values. Do not ship the broad rule on an assumption of parity.
2. **Price the third router arm by a compiling prototype**, in `internal/filter/http/router/`. Neither
   single-file cost figure includes it, and it is a second package.
3. **Name the new counter and prove it passes `stats.IsValidName`**; pin its DELTA, never its absolute.
4. **Write the NC table in §10 as real tests and RECORD each NC's result.** Budget for NC-ing every pin,
   not just the risky-looking ones.
5. **Re-derive every count and cite at the PLAN's own tip.** Two numbers in this row's own inherited
   record were stale (`311/319`, `0120`'s port), and one trap turned out to be tool-dependent.
6. **Do not gate anything on a ` + ` split** (§2 item 7), and **name the grep tool** in any `\t` gate
   (§2 item 5).
