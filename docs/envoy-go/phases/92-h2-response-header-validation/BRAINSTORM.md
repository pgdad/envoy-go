# Phase 92 — `h2-response-header-validation` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Base master `47e44004`.** Branch `phase-92-brainstorm`.

**SELF-PICKED** per the 2026-07-12 standing directive, with **no banked mid-lifecycle work — PROVEN, not assumed**:
all 123 ROADMAP rows read `done`, sentinel check (1) is SILENT at `want=123`, and NC-A reads the one-line
`NOT DONE: row 62` (§8).

> ⚠️ **THE INHERITED "STRONG SMALL CANDIDATE" WAS REFUTED BEFORE IT WAS PICKED, AND SO WAS THE INSTRUCTION
> TELLING THIS STAGE WHERE TO REGISTER ITS ROW.** Both are in §9. Neither is a matter of taste: the first
> would have chartered a row against a defect that does not exist, and the second would have left
> `ROADMAP.md` and `STATE.md` desynchronised for a whole stage.

---

## 1. The pick, and why it is defensible as "smallest first"

**CHARTER: on the HTTP/2 downstream leg, REJECT a response whose upstream header block carries a
connection-specific or otherwise malformed field, instead of laundering it onto the downstream stream.**

This is the **encode-direction mirror of a guard this project already landed on the decode direction**.
Row 89 shipped `h2.IsIllegalH2RequestHeader` (`internal/filter/hcm/h2/stream.go`, cite BY SYMBOL) and wired
it into `reconcileH2DecodeDelta` so a filter-written request header that RFC 9113 §8.2.2 forbids is dropped
before the upstream HEADERS block is built. **The response direction has no equivalent, and nothing in the
tree supplies one.** The shape is exactly row 89's own — *"two of three symmetric paths are already
correct"* — restated one direction over: the request side has the guard, the response side does not.

Five properties make it the smallest defensible candidate available at this tip:

1. **Both sides are MEASURED, not predicted.** Nineteen arms against `envoyproxy/envoy:contrib-v1.37.2`
   and against the subject, three byte-identical subject reproductions, positive controls first and last,
   and a direct-to-backend **discriminating negative control** proving the instrument can see what the
   subject erases (§2).
2. **The cost floor is measured by a prototype that was BUILT, RUN, and REVERTED** — `git diff --numstat`
   **`44  0  internal/filter/hcm/h2/client.go`**: one file, one symbol, `sha256sum -c` => `OK` on the
   restore (§6.1).
3. **It reuses predicates that already exist and are already documented as canonical.**
   `isConnectionSpecificField` (`h2/stream.go`) carries an in-tree comment naming it *"the ONE source of
   truth"*, and `hasUppercaseHeaderChar` (`client.go`) already backs the trailer-side check. The row adds
   a call site and a branch; it does not add a policy.
4. **The axis is UNPINNED IN BOTH DIRECTIONS, proven by execution rather than by grep.** The prototype
   leaves `./internal/filter/hcm/... ./internal/filter/http/router/...` at **655 `=== RUN` / 0 FAIL**, and
   the second prototype leaves the whole `./internal/...` tree at **70/70 packages ok**. That green is
   **not** vacuous: a reachability control (`panic()` injected at the site) reddens with
   `--- FAIL: TestChainIntegration_H2_DirectResponseHappy`, so the site IS executed by the suite — the
   suite simply never asserts what it writes (§2.5).
5. **The blast radius is a conformance violation reachable by an ordinary client.** envoy-go currently
   emits a downstream response that RFC 9113 §8.2.2 requires every conformant peer to treat as malformed.
   The reference answers **502** on every one of the same seven shapes.

**Rejected alternatives, each priced at THIS tip, are in §4.** The three strongest — an upstream-connection
lifetime defect, the banked HTTP/1.1 no-`Host` divergence, and the uncounted `ssl.connection_error` bucket
— are all real and all are recorded with their measured costs so a later row can take them without
re-deriving anything.

---

## 2. The defect, REPRODUCED BY EXECUTION at `47e44004`

### 2.1 The instrument, and why each part of it is load-bearing

- **Downstream:** a raw `http2.Framer` client holding **ONE `hpack.Decoder` per connection** (the dynamic
  table is connection-scoped; a per-request decoder reads as *"headers were lost"*), with
  **`ReadMetaHeaders` deliberately NOT set** so `x/net` validates nothing away before the probe can observe
  it. Pattern reused from the in-tree raw-framer client at `test/fixtures/0119-grpc-unary-trailers/driver/`.
  ⚠️ **`helpers.H2RoundTrip` was NOT used** — it is structurally incapable of expressing these shapes, the
  same class of blocker caught at the phase-89 PLAN and again at the phase-90 BRAINSTORM.
- **Upstream:** a purpose-built raw h2c server with byte-level control of the response block
  (`hpack.Encoder`, never-indexed literals, names written verbatim). An off-the-shelf server cannot emit
  the illegal shapes at all.
- **Subject:** `go build -o <scratch>/envoy-go ./cmd/envoy-go/` — built with `-o` into scratch, because a
  bare `go build ./cmd/...` drops an untracked binary in the worktree root. h2c listener
  (`codec_type: HTTP2` + `--allow-h2c`), static cluster with `http2_protocol_options`.
- **Reference:** `envoyproxy/envoy:contrib-v1.37.2`, container `probe-c-ref`, `-p 127.0.0.1:22004:10992`,
  `--add-host=host.docker.internal:host-gateway`, same route shape. Torn down BY NAME; the four foreign
  containers were left untouched.

### 2.2 The controls, stated before the result

- **POSITIVE CONTROL, run FIRST and LAST** (`ctl-ok`, `ctl-ok2`): an ordinary 200 with `x-probe=ok` and a
  5-byte body round-trips on all three targets. A null result therefore cannot be a dead probe.
- **DISCRIMINATING NEGATIVE CONTROL:** the same client driven **direct to the backend**, bypassing the
  subject entirely, observed every illegal shape verbatim — `connection="keep-alive"`, `X-Upper-Case="yes"`,
  two `content-length` fields. ⚠️ **Without this arm the finding is unfalsifiable**, because "the subject
  stripped it" and "the instrument cannot see it" are otherwise indistinguishable.
- **REPRODUCIBILITY:** three independent subject runs are **byte-identical** (`R1 == R2`, `R1 == original`).

### 2.3 The result — seven shapes, one direction of divergence

| # | upstream response carries | **envoy-go** | **reference** |
|---|---|---|---|
| 2 | `connection: keep-alive` | **forwarded verbatim, 200** | **502**, `reset reason: protocol error` |
| 3 | `transfer-encoding: chunked` | **forwarded, 200** | **502** |
| 4 | `keep-alive: timeout=5` | **forwarded, 200** | **502** |
| 5 | `upgrade: websocket` | **forwarded, 200** | **502** |
| 6 | `proxy-connection: keep-alive` | **forwarded, 200** | **502** |
| 7 | uppercase name `X-Upper-Case` | **lowercased to `x-upper-case`, 200** | **502** |
| 10 | duplicate `content-length` | **both forwarded, 200** | **502** |

RFC 9113 §8.2.2: *"An endpoint MUST NOT generate an HTTP/2 message containing connection-specific header
fields"*, and a receiver *"MUST treat a message containing connection-specific header fields as
malformed."* ⇒ **envoy-go generates a message every conformant client is required to reject.** The
reference detects the same bytes at its upstream codec and answers 502 before anything reaches the
downstream stream.

⚠️ **Arm 7 is the one that would be missed by a code-reading.** The subject does not forward
`X-Upper-Case` verbatim — `writeH2Reply` lowercases every name, so the wire is *syntactically* legal H/2.
The divergence survives anyway, because the reference treats the **upstream** message as malformed and
never produces a downstream response at all. A fix designed only to make the downstream bytes legal would
leave this arm divergent.

### 2.4 Arms that are PARITY, recorded so they are never re-probed

| # | shape | both sides |
|---|---|---|
| 8 | header value with leading/trailing OWS | forwarded |
| 9 | empty header value | forwarded |
| 17 | legal trailers (`x-trailer`, `grpc-status`) | forwarded verbatim, wire order preserved |

### 2.5 ⚠️ NOTHING PINS THIS — and the green was proven non-vacuous

- baseline `go test -v -count=1 ./internal/filter/hcm/... ./internal/filter/http/router/...`
  => **RUN=655, anchored FAIL=0**, 3 packages ok
- **prototype A applied** (the fix) => **RUN=655, anchored FAIL=0** — nothing pins the missing validation
- **prototype B applied** => `./internal/...` **70/70 packages ok**
- ⚠️ **REACHABILITY CONTROL, because two greens prove nothing on their own:** replacing the rewrite site
  with `panic("PROBE-C-REACHABILITY-CL-REWRITE")` gives **rc=1, one panic hit,
  `--- FAIL: TestChainIntegration_H2_DirectResponseHappy`**. The site **is** executed by the suite; the
  suite simply never asserts the value it writes. ⇒ the greens above are **assertion blindness, not dead
  code** — the identical distinction ADR-0312 §Consequences (ii) had to make for row 90.

⇒ **the row ships with ZERO inherited coverage and must bring its own guard.**

---

## 3. The mechanism, stated precisely

Three symbols, and the asymmetry is structural:

1. **`h2.(*ClientConn).onResponseHeaderBlock`** (`internal/filter/hcm/h2/client.go`) branches on
   `cs.respHeadersSeen`. The **TRAILING** block is validated — `validateResponseTrailers` enforces
   `hasUppercaseHeaderChar`, `isConnectionSpecificField` and the pseudo-header ban. The **LEADING** block
   is stored verbatim (`cs.respHeaders = decoded`) and **validated by nothing at all**.
2. **`router.doH2ClusterAction`** forwards the stored set, stripping only `:`-prefixed names.
3. **`hcm.writeH2Reply`** (`internal/filter/hcm/h2dispatch.go`) synthesizes `:status`, lowercases every
   name, and emits the rest unchanged. **It applies no RFC 9113 §8.2.2 filter.**

⇒ there is **no validation anywhere on the leading-block encode path.**

**The fix site is `onResponseHeaderBlock`'s `if !cs.respHeadersSeen` branch**, gated by a new
`validateResponseHeaders` reusing the two existing predicates. ⚠️ **One design constraint the prototype
discovered and the SPEC must not re-derive:** the returned error **must not** carry the
`h2.ErrMalformedTrailers` sentinel, or `router.doH2ClusterAction` takes its **stream-reset** arm instead of
its **502** arm — and it is the 502 arm that produces reference parity.

⚠️ **A structural fact that bounds the charter, measured not assumed:** an H/2 **downstream** always drives
an H/2 **upstream** (`clusterRouteAction.asRouterActionH2` -> `router.H2ClusterAction` ->
`Cluster.AcquireH2Stream`, unconditionally). **There is no H2-downstream-to-H1-upstream path**, so the
charter is closed under one codec pair and the SPEC owes no H1-upstream arm.

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

Each of these is REAL and MEASURED. They are recorded with their costs so a later row can take one without
re-deriving anything. **None is dismissed; each is deferred for a stated reason.**

### 4.1 The inherited pick — the `NewClientConn` `net.Conn` leak — **REFUTED, THERE IS NO DEFECT**

See §9.1. ADR-0313 §Consequences (viii) records a leak at *"the three `NewClientConn` error paths"*. The
grep it rests on is true; **the conclusion is false**, and the count is wrong. Refuted by execution with a
load-bearing leak arm. **A row chartered on it would have had nothing to fix.**

### 4.2 The `*ClientConn` lifetime-ctx defect — **REAL, BANKED, and BIGGER than this row**

Surfaced while refuting 4.1. `NewClientConn` derives the pooled connection's **lifetime** ctx from the
**dialing caller's** ctx, and `(*ClientConn).Closed()` is literally `cc.ctx.Err() != nil`. Cancelling one
request's ctx therefore tears down a connection other requests are multiplexed on, and the pool never
reaps it. Measured: `cc2.Closed() == true` while request 2 still holds a stream (control: `false`); orphan
FD delta **+100 over N=50** against **+2** in the uncancelled control. The production cancel sites were
controller-verified: `hedge.go` (`hedgeCtx` + `defer cancelAll()` -> `doH2ClusterAction`) and `retry.go`
(`attemptCtx` from `per_try_timeout` -> `doH2ClusterAction`). `context.WithoutCancel` is available
(`go.mod` `go 1.23.0`, toolchain go1.26.7) and has **zero** uses in the tree today.

**DEFERRED, not dismissed**, on three stated grounds: the fix probably spans **two packages**
(`h2/client.go` plus a pool reaper at `h2pool.go`) where this row spans one; its reference-comparability is
undetermined; and its cost is not yet bounded by a prototype. **It is the strongest banked candidate in
this document and the next self-pick should look at it first.** §9.2 carries its full state.

### 4.3 HTTP/1.1 with no `Host` (the phase-91 banked candidate) — **BANKED; its rejection is PARTLY REFUTED**

Re-derived on both sides at this tip. Subject: **200, forwarding a literal empty `Host: ` upstream**.
Reference: **400 `missing_host_header`**, `connection: close`, never routed, and `downstream_cx_protocol_error`
does **not** move. A2 (empty `Host:`) and A3 (whitespace-only) are **200 on BOTH sides** — so the
divergence set is exactly **one arm**.

⚠️ **Phase 91 rejected the FIX as *"new machinery in a request-smuggling-sensitive path"* without pricing
it. Priced here: `+90 / −0`, ONE file, ZERO new imports** (`bytes` is already imported), zero signature
changes, package green at RUN=322. **And the hazard it named was DISCHARGED by measurement rather than
argued away:** prototype v1 (peeking only what is already buffered) is **EVASIBLE** — split the head with
a 250 ms gap and the guard silently does not fire, forwarding the empty `Host:`, which is *worse* than no
guard; prototype v2 (growing the peek to the blank line, accepting CRLF **and** bare-LF) held across six
adversarial arms including 1-byte-at-a-time fragmentation, bare-LF framing and a decoy `X-Not-Host:` header,
with an over-fire control on A0.

**DEFERRED anyway, on one stated ground:** at `+90` it is **twice** this row's `+44`, and it is a
**strictness change** (200 -> 400) in the CWE-444 surface, which owes an ADR and a `BEHAVIOR_CONTRACT.md`
edit that this row does not. Its pin half is `+7` lines and is a **PROVEN GUARD** (§9.3).

⚠️ **Two corrections a taker must not re-inherit.** Phase 91 corrected ADR-0312's `dispatchRequest` fix
site to `serveOneRequest` — and **`serveOneRequest` cannot host the guard either.** Controller-verified:
`runConnection` holds `br := bufio.NewReader(downstream)` and calls `http.ReadRequest(br)`, while
`serveOneRequest(ctx, downstream net.Conn, req *http.Request, bw *bufio.Writer)` receives the **parsed**
request and cannot reach the raw head without a signature change. The guard belongs in `runConnection`.

### 4.4 The `content-length` rewrite — **REAL, BANKED, and THREE-LEGGED**

`writeH2Reply` rewrites `content-length` to `len(body)` **unconditionally** — **method-blind and
status-blind**. Measured: a `304` carrying `content-length: 42` ships `0` where the reference preserves
`42`; a bodyless `200` with `content-length: 5` — the ordinary shape of a `HEAD` response — ships `0` where
the reference preserves `5`; and a mismatched `content-length: 999` over a 5-byte body is silently
laundered into a well-formed 200 where the reference forwards `999` and then RST_STREAMs on the under-run.

**DEFERRED on a measured reason, not a guess.** The deletion prototype is `0  3` and it does **not** reach
parity: the reference additionally *rejects* a `204` carrying `content-length` and *resets* on a body/CL
under-run, so the row owes new rules, not a deletion. ⚠️ **AND IT IS THREE-LEGGED — controller-found, not
in the probe's report:** the identical rewrite exists on **all three codecs**, at `internal/filter/hcm/codec.go`
(H1), `h2dispatch.go` (H2) and `h3dispatch.go` (H3). A row taking it must decide its leg scope first.
Mixing it into this row would import a three-codec behaviour question into a one-symbol charter.

### 4.5 1xx interim responses — **REAL, BANKED, and its stated parity target is WRONG**

Measured: a leading `103 Early Hints` or `100 Continue` block makes the subject **RST_STREAM(INTERNAL_ERROR)**
— `respHeadersSeen` is set on the first block unconditionally, so the real final block lands in the trailer
branch and is rejected for carrying a pseudo-header. The reference **delivers the 200**.

⚠️ **DEFERRED, and the deferral is itself a finding:** `client.go` states in a live comment, as a named
non-goal, *"the reference **FORWARDS** 1xx."* **Measured on `contrib-v1.37.2` at default HCM config, it
does NOT — it SWALLOWED both blocks and delivered only the final 200**, observed by a client that prints
every HEADERS block on the stream unfiltered. The divergence *direction* is unchanged, but the stated
parity target is wrong and **would misdirect the fix**. A row taking this must design against
*drop-the-1xx-and-deliver*, not against *forward*. This row does **not** touch that comment — it is in the
file being edited, and correcting it is a one-line documentary fix the SPEC should decide on explicitly
rather than absorb (§7).

### 4.6 `ssl.connection_error` — the uncounted handshake-failure bucket — **BANKED; the cheapest in-window candidate**

The only strong candidate that sits **inside a live sentinel window** (`:223`, item 2 of 3), with the
reference side already measured. Controller-verified: `connection_error` occurs in `*.go` **only inside
comments**, and the `switch classifyHandshakeErr(err)` in `serveConnection` has exactly **two** cases and
**no `case outcomeOther:`** — so five classified failure shapes increment nothing. Under 20 production
lines, in a file whose exact shape was landed twice before (phases 74 and 75), and it would **narrow a
sentinel window at row-done**, which no maintenance row can do.

**DEFERRED on one ground, stated plainly:** it is a **missing counter**, not a defect that breaks anything
on the wire. This row's charter is a conformance violation an ordinary client must reject. Between a
smaller cosmetic gap and a slightly larger correctness gap at comparable cost, the project's rows 85-91
have consistently taken the correctness gap, and this document follows that precedent rather than the bare
line count. **It remains the best candidate for a row that wants to move a sentinel window.**

### 4.7 The six family windows as a pool — **REJECTED as a pool, and the inventory CORRECTED to 40**

Re-derived mechanically (§9.4): `:201` 7 · `:207` 10 · `:213` **9** · `:223` 3 · `:229` 8 · `:237` **3**
= **40**, not the inherited **38**. Every one is a family-scale charter, and **BLOCKED IS NOT DEFERRED** —
three `:207` gRPC candidates carry measured ceiling blockers, so picking one means picking its blocker
first. None is a smallest-first candidate.

---

## 5. Family attribution

**Core-HCM / HTTP-2-codec MAINTENANCE row claiming NO family ordinal** — the row-85/86/87/88/89/90/91
precedent: a maintenance row repairs a landed deliverable and does not extend a charter.

**PROVENANCE IS OUTSIDE EVERY SENTINEL WINDOW — and the check that establishes it was itself
negative-controlled, which is how its first form was caught being over-broad.** Across all six windows
(post-insertion anchors `:202 :208 :214 :224 :230 :238`), `response header`, `8.2.2` and
`connection-specific` return **0**.

⚠️ **The bare token `encode` does NOT return 0 — it reads 1, at `:208`, and the first draft of this
section asserted 0.** The match is **`RunEncodeTrailers`**, the gRPC window's tenth candidate
(*"wiring the dead `RunEncodeTrailers` hook (coupled to the WASM trailer-map seams)"*). That is a
**filter-hook wiring** candidate — running the encode chain's trailer hook, which ADR-0273's boot-reject
keeps dead — and not response-header validation; the two share a substring and nothing else. **The claim
is therefore stated with its match disclosed rather than as a bare zero**, because a selector broad enough
to match a different candidate is exactly the kind that later gets quoted as proof of absence.

The provenance is ADR-0312 §Consequences **(xix)**, which names *"the encode/response direction"* as
**NOT MEASURED BY THIS ROW, stated so it is never inferred** — this stage is the first probe of it.

**No window narrows at row-done.** The only sentinel-affecting edit this row makes is the **+1 data row**,
which shifts the six window ANCHORS +1 without changing their COUNT or CONTENT. ⚠️ **That must be MEASURED
on both sides at row registration, never forecast** — it is measured in §8.

---

## 6. Anticipated ADR, counts, and the cost FLOOR

### 6.1 The MEASURED cost floor — a built, run, and reverted prototype

`git diff --numstat` => **`44  0  internal/filter/hcm/h2/client.go`** — 44 added, 0 deleted, ONE file.
The patched binary was **booted and re-driven by the same probe**: arms 2-7 and 10 all became **502**,
matching the reference on every one, and both positive controls stayed green. Reverted with
`git checkout -- <path>`; `sha256sum -c` against a capture taken BEFORE patching => **`OK`**.

⚠️ **THIS IS A LOWER BOUND, AND THE CAUSE IS ALWAYS UNDER-ENUMERATION.**
`reference_measured_prototype_is_a_lower_bound` has fired **five consecutive rows** (87 through 91). The
SPEC **must** enumerate by compiling prototype and must price at least these named gaps, none of which the
floor covers:

- the **stat surface**: does a rejected response book `upstream_rq_5xx` / a protocol-error counter, and
  does the reference's 502 book anything the subject does not? **Unmeasured.**
- the **error text and `%RESPONSE_CODE_DETAILS%` analogue** — the reference logs `reset reason: protocol error`.
- whether the guard belongs behind a **`stream_error_on_invalid_http_messaging`-style posture**, as the
  reference's H/2 request-side reject is (ADR-0312 §Context ¶4 measured that posture-dependence).
- **arm 7's asymmetry** (§2.3): a downstream-bytes-only fix leaves it divergent.
- the **trailing-block path**: `validateResponseTrailers` already exists; the SPEC must say whether the new
  leading-block validator shares its body or stands beside it, and must not silently change trailer behaviour.
- the **differential surface**: whether any of the four H2-capable fixtures (`0004`, `0079`, `0080`, `0119`)
  can express an illegal upstream response, or whether the arm is unit-only.

### 6.2 Anticipated counts — every axis re-derived at this tip (§8)

Anticipated **+0 on every axis**: stat surface **406** · fuzzers **55 / 48 files** · BackendKind tail **38**
(`H2GoawayResponder`) · fixtures **121**, `0120` **UNCONSUMED** · blank imports **121 / 122 / 123**
(narrowed-in-`runner_test.go` / narrowed-repo-wide / all-in-`runner_test.go` — **state the scope whenever
the number is restated**) · `go.mod` / `go.sum` **BYTE-UNTOUCHED** · config fields **+0** · `^---$` **216**.

**`want` MOVES 123 -> 124** — this stage registers row 92, and `ROADMAP.md` goes 241 -> 242 lines /
123 -> 124 data rows as a **PURE INSERTION** (`git diff --numstat` `1 0`). See §9.5 for why that is this
stage's job and not the SPEC's.

Anticipated **ADR-0314** — **TAIL-derived** (`grep -oE '^## ADR-[0-9]+' | tail -1` => `## ADR-0313`;
`grep -c '^## ADR-0314'` => **0**). ⚠️ **NEVER derive next-free from the heading count — the id space is
SPARSE and headings+1 COLLIDES at the ADR-0209 gap.** The SPEC drafts §Context and **RE-ARMS the strict
`^> **STATUS: PROPOSED` guard 0 -> 1**, verified **BY LINE AND ADR**; the historical
`^**Status:** PROPOSED` at **ADR-0231** also reads 1, is a **DECOY**, and must be LEFT ARGUED AND ARMED.

**Cost estimate for the whole row: `~+44-120` net production `.go` in ONE package, `~+150-400` net test.**
⚠️ **AN ESTIMATE, NOT A MEASUREMENT.**

---

## 7. What the SPEC owes

1. **Price the six named gaps in §6.1 by compiling prototype**, and say explicitly which are in charter.
2. **Decide the reject POSTURE** — 502 (measured parity) versus stream reset — and pin the choice to the
   `ErrMalformedTrailers` sentinel constraint in §3, which is a *mechanism*, not a preference.
3. **Decide the leading/trailing validator relationship** and prove no trailer behaviour changes.
4. **Decide the differential question explicitly**: extend `0004` versus mint `0120` versus unit-only. ⚠️
   Minting has **three registration gates and the second fails as a SILENT PASS** — the runner `t.Skipf`s
   an unregistered fixture and **no fixture-count gate exists anywhere in the tree**. Whatever is chosen,
   **ASSERT THE FIXTURE SET**, never merely that the suite was green.
5. **Say yes or no, explicitly, to a fuzz target** for the new validator (`55 / 48` today).
6. **Decide in writing** whether to correct the wrong in-code 1xx claim (§4.5) in passing — it is in the
   file being edited — or to leave it and record it. **Do not absorb it silently either way.**
7. **Name the guard's negative controls before writing them.** ⚠️ At the phase-91 IMPL **three of the
   row's own new pins were non-discriminating and ALL THREE PASSED WHEN WRITTEN** — one structurally unable
   to fire, one vacuous, one a coin flip. **Budget for NC-ing every new pin, not just the risky-looking
   ones.**
8. **Re-derive every count and every cite at the SPEC's own tip.** Line anchors drift; symbol anchors do
   not; **the receiver name is part of the anchor**, and a receiver written `(cc *ClientConn)` needs
   `grep -F`, because ERE reads the parentheses as a group and returns a **FAIL-UNSAFE ZERO**.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT RECORDED

**Measured at `47e44004` BEFORE the row-92 insertion, and again AFTER it. Recorded, not forecast.**

| check | BEFORE (`want=123`) | AFTER (`want=124`) |
|---|---|---|
| (1) field-parsed awk | **SILENT** | **`NOT DONE: row 92`** — the row this stage just opened |
| (2) deferred-candidate sentences | **SIX** at `:201 :207 :213 :223 :229 :237` | **SIX** at `:202 :208 :214 :224 :230 :238` |
| (3) family-opened loop | **SILENT** | **SILENT** |

⇒ **ONE check blocked the sentinel before this stage and TWO block it after. `stop` WAS EVALUATED AND
DELIBERATELY NOT CREATED** — verified absent at the git root and in the stage worktree.

### The four mandated NCs — ALL FOUR FIRED

- **NC-A** (doctor row 62 to `in-progress`): `NC LANDED? [ in-progress ]` **inspected BEFORE trusting the
  result**, then check (1) on the doctored file printed the **ONE-line** `NOT DONE: row 62`. ⚠️ The one-line
  form is the correct shape on an all-`done` board; the phase-91 two-line expectation does **not** carry.
- **NC-B** (`want=122` on the real file): `GATE FAIL: examined 123 data rows, expected 122` **and nothing
  else** — no `NOT DONE:` line, as an all-`done` board requires.
- **NC-C** (strip `gRPC-family row`): residual **2 -> 0**, `NEVER OPENED: gRPC   <- NC FIRED`, with WASM
  correctly silent.
- **NC-D** (the two check-(2) patterns separately): long form **5**, short form **1**, union **6**.

### The insertion itself, measured on both sides

- `git diff --numstat` on `ROADMAP.md` => **`1  0`** — a **PURE INSERTION**, matching the phase-91
  BRAINSTORM's own numstat exactly.
- `ROADMAP.md` **241 -> 242 lines**, **123 -> 124 data rows**; sentinel `want` **123 -> 124**.
- **Row 92 field-counts NF=8 under BOTH the escape-aware and the naive form**, checked before committing.
- The **malformed-row baseline is INVARIANT**: escape-aware **2** (ids 57 `NF=9`, 69 `NF=10`) and naive
  **17**, both unchanged. ⚠️ A gate asserting `== 0` fails on pre-existing content; assert against the
  known-2 baseline.
- ⚠️ **The six windows' CONTENT is invariant, MEASURED not assumed** — each shifted line's md5 is
  byte-identical to its pre-insertion self, and **the comparator was negative-controlled**: comparing
  `:201` against a non-corresponding `:203` produces different digests, so it discriminates rather than
  reporting a vacuous match.

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 ⚠️ THE INHERITED "STRONG SMALL CANDIDATE" IS REFUTED — there was no defect to charter

`next-prompt.txt` names it *"a strong small candidate"*; ADR-0313 §Consequences **(viii)** records it as
*"A pre-existing `net.Conn` leak at those same three paths ... All three `cancel()` without closing
`upstream`, and there is no `upstream.Close()` anywhere in `client.go`."*

**The grep is true. The conclusion is FALSE.** The sole PRODUCTION caller closes it. `h2ConnFromDialed`
(`internal/cluster/dial_h2.go`, cite BY SYMBOL) does `_ = wrapped.Close()` on **any** `NewClientConn`
error, and that function's own header comment states the ownership contract in terms: *"Each error branch
closes the underlying conn explicitly because the function returns the conn-owning `*h2.ClientConn` on
success; on error there is no caller-owned wrapper to defer-close, so the underlying conn would otherwise
leak file descriptors."*

**Measured, with a load-bearing leak arm** (without which the null result is unfalsifiable):

| arm | N | FD delta | peer observed close |
|---|---|---|---|
| production path (`h2ConnFromDialed`) | 200 | **0** | **205 / 205** |
| deliberate leak (`NewClientConn` direct, error dropped) | 200 | **+200** | **0 / 205** |

**And the count is wrong: there are FOUR error paths, not three.** The fourth is the `<-ctx.Done()` arm of
the SETTINGS_ACK wait; it runs **after** `startReader()` and `go readLoop()`, which is exactly why it
carries the extra `closeReader()`. It was measured clean on both axes — goroutine delta **0 over 100**
(instrument shown to read **+100** on a deliberate goroutine leak) and fd close **105 / 105**.

⇒ **class: `reference_code_comment_not_evidence` — a file-scoped grep read as a program property. Walk the
call graph one layer UP.** A row chartered on (viii) would have had nothing to fix, and would have
discovered that only after opening.

### 9.2 The defect the refutation surfaced instead — BANKED with everything a taker needs

See §4.2. `NewClientConn` derives the pooled connection's lifetime ctx from the dialing caller's ctx.
Measured: `cc2.Closed() == true` while request 2 still holds a stream on the same pooled conn (control:
`false`); orphan FD **+100 over N=50** versus **+2** uncancelled. Mechanism: `makeRelease` evicts and
Closes a `Closed()` conn only on the **last** in-flight release, so a cancel landing after `inFlight`
reached 0 is never followed by any release, and `findStreamHitLocked` merely **skips** the dead conn —
nothing ever calls `cc.Close()`. Production cancel sites controller-verified in `hedge.go` and `retry.go`.
⚠️ **Its end-to-end reachability through the filter chain was NOT executed at this stage** — that is the
first thing a taker owes, and a measured *"the blast radius is fd/permit exhaustion only, not request
failure"* would legitimately shrink the charter.

### 9.3 On the phase-91 banked H1 candidate — its rejection is PARTLY REFUTED

See §4.3. Fix priced at `+90 / −0`, one file, zero new imports; the smuggling hazard **discharged by
measurement** (v1 evasible by a 250 ms head split, v2 not, across six adversarial arms). The **pin** half
is `+7` lines — not *"roughly a dozen"* — and is a **PROVEN GUARD**, not a merely-passing test: with the
fix applied the new row is the **only** failing subtest and the three pre-existing rows stay `PASS`.

⚠️ **THREE further corrections a taker must not re-inherit.** (1) The fix site `serveOneRequest` is
**unreachable as written** — the raw head exists only in `runConnection` (§4.3). (2) *"converts ONE
divergence into THREE"* is **TWO** (A2 and A3); the phase-91 BRAINSTORM's own text says TWO and is correct
— **the inflation was minted in this stage's own agent brief and is recorded rather than buried.**
(3) `missing_host_header` is **invisible at `--log-level info`**; it needs an access log carrying
`%RESPONSE_CODE_DETAILS%`, and anyone re-deriving it from the container log alone will wrongly conclude the
cite is unbacked.

### 9.4 On the self-pick inventory — the inherited total is REFUTED and the extraction form is a trap

Measured: `:201` **7** · `:207` **10** · `:213` **9** · `:223` **3** · `:229` **8** · `:237` **3** = **40**.
The inherited **38** totals `:237` as **1** while its own enumeration names **THREE** items — it is
arithmetically inconsistent with itself. `:213`'s 9-not-10 is the paren artifact
(`upstream SDS (server-cert + validation_context)` is ONE candidate split by a `+` inside parentheses).
The six windows are family-charter **paragraphs** below the data table, which ends at the last data row.

### 9.5 ⚠️ THE ROUTING INSTRUCTION THIS STAGE INHERITED IS REFUTED BY THE PRECEDING ROW'S OWN COMMITS

`next-prompt.txt` "What the next session owes" directs: *"open `ROADMAP.md` row 92 (`want` 123 -> 124)
**at the SPEC, not the BRAINSTORM**."*

**MEASURED.** The phase-91 **BRAINSTORM** commit carries `docs/envoy-go/ROADMAP.md` at
`git show --numstat` = **`1  0`** — a pure insertion registering
`| 91 | h2-framer-partial-frame-desync | 90 | in-progress |` — and the phase-91 **SPEC** commit touches
`ROADMAP.md` **not at all** (empty numstat). It also contradicts `BOOTSTRAP_PROMPT.md` §5 state 0,
*"→ superpowers:brainstorming (adds/refines row in ROADMAP)"*, and `STATE.md`'s own §Recent entry for that
stage, which records the registration as happening **in the same commit** as the BRAINSTORM.

⇒ **THIS STAGE REGISTERS ROW 92.** Following the inherited instruction would have left `STATE.md` claiming
a phase that `ROADMAP.md` did not carry, for a whole stage.

### 9.6 Method findings — four fail-unsafe gate forms, all found by execution

1. ⚠️ **`awk split(s, a, " + ")` treats the separator as a REGEX.** ` + ` means *space, one-or-more
   spaces, space* — i.e. two-or-more spaces — which occurs nowhere in these lines, so it reads **1 on ALL
   SIX windows**. A gate reading 1 on every arm looks like a uniform structure and is a fail-unsafe zero.
   Use `sed 's/ + /\n/g' | wc -l`.
2. ⚠️ **`grep -cE '^\t_ "'` reads 0 where `grep -cP '^\t_ "'` reads 123.** GNU grep's ERE has **no `\t`
   escape** — it matches a literal `t`. The blank-import gate form as WRITTEN in the standing record is a
   fail-unsafe zero under `-E`. Use `$(printf '\t')` or `-P`.
3. ⚠️ **`ls test/fixtures/ | grep -cE '^[0-9]{4}-'` reads 119, not 121** — `0007a-cors` and
   `0007b-iteration-probe` carry a LETTER between the digits and the hyphen. Same family as the digit-blind
   `[A-Za-z]+ +BackendKind` class. Use `ls -d test/fixtures/*/ | wc -l`.
4. The inherited **88** for the naive whole-line ` + ` split of the Observability window **does not
   reproduce — it reads 92**. Neither figure means anything; the drift is the point. The number was carried
   across stages without re-derivation.
5. ⚠️ **THIS STAGE'S OWN §5 CLAIM WAS FALSE AS FIRST WRITTEN, AND ITS OWN NEGATIVE CONTROL CAUGHT IT
   BEFORE THE COMMIT.** §5 asserted that a per-line grep of all six windows for
   `response header|8.2.2|connection-specific|encode` returns **0**. Run: it returns **1**, at `:208` —
   the bare token `encode` matching **`RunEncodeTrailers`**, an unrelated filter-hook candidate. Three of
   the four terms do return 0 and the conclusion survives, but **the claim had to be restated with its
   match disclosed.** ⇒ **a provenance-absence claim is only as good as the breadth of the selector that
   establishes it**, and a selector broad enough to match a *different* candidate is the kind that later
   gets quoted as proof of absence. The general rule this stage keeps re-learning in new costumes: **run
   the control that shows what was MATCHED, not only the count.**

### 9.7 ⚠️ A SUBAGENT'S INCIDENTAL FIGURE WAS WRONG, AND SO WAS ONE OF THE CONTROLLER'S OWN

`feedback_brief_citations_not_evidence` fired twice this stage, both times on **incidental** numbers rather
than headline claims:

- An inventory agent reported the `GetStatPrefix()` firing control as **16** tree-wide. Re-derived by the
  controller: **26 matching lines across 17 files**. The *claim* it supported (**0** consumers under
  `internal/listener/` + `internal/bootstrap/`) is correct and was independently confirmed.
- The controller's own agent brief paraphrased phase 91 as *"converts ONE divergence into THREE"* when the
  correct figure is **TWO** (§9.3). **The agent caught the controller.**

⇒ **re-derive EVERY number you restate, including the ones that are not the point of the sentence.**

### 9.8 Record defects found in passing — RECORDED, deliberately NOT fixed by this stage

1. ⚠️ **A recorded documentary defect that DOES NOT EXIST.** `phases/87-h2-double-slash-path-routing/PROGRESS.md`
   records that `BEHAVIOR_CONTRACT.md`'s two riders cite ``ADR-0052 `:1821` `` and that *"line 1821 is
   retry-policy / hedging prose"*, declaring it a drifted anchor and a named documentary defect. Measured:
   **`DECISIONS.md:1821`** reads *"Future phases that extend the H2 equivalence surface ... add
   sub-sections here ... via a new ADR, not by editing 05.1's `### Not asserted` block silently"* —
   **exactly what the riders cite it for**. What *is* retry prose is **`BEHAVIOR_CONTRACT.md:1821`**. The
   phase-87 finding resolved an ADR line anchor **against the wrong file**. ⇒ **a stale candidate that
   would waste a taker's time, and itself an instance of the class it claimed to find.**
2. `internal/wasm/dynamic_stats.go` says *"The Task 17 wiring will ALSO increment two counters ... (counter
   wiring deferred)"* **fifteen lines above the code that already does it**
   (`rv.stats.DynamicStatsCapExceededInc()` / `rv.stats.EnvoyGoFailuresInc()`). Another
   `reference_code_comment_not_evidence`.
3. `Listener.stat_prefix` is parsed and silently discarded — **0** `GetStatPrefix()` consumers under
   `internal/listener/` and `internal/bootstrap/`, against a firing control of **26** tree-wide. The
   reference honours it. Cheap in production lines, but it renames every listener-scope stat and moves
   fixture goldens — a SMALL fix with a MEDIUM blast radius.
4. ⚠️ **A probe artifact worth carrying: a ONE-SHOT (non-keep-alive) backend makes envoy-go reuse a dead
   pooled upstream conn and return 502** instead of re-dialing, producing an alternating 200/502 pattern
   that **reads exactly like a Host divergence**. Any future H1 probe must use a keep-alive backend.
   Whether that stale-pool 502 is itself a robustness gap is **UNPROBED** — and it rhymes with the H2 pool
   mechanism in §9.2.

---

## 10. Probe hygiene

- **Five worktrees**, one per stream, all detached at `47e44004` except the stage branch. No probe agent
  saw another's edits.
- **Every prototype was BUILT, RUN, and REVERTED**, with `sha256sum` captured BEFORE patching and
  `sha256sum -c` => `OK` verified after `git checkout --`. Four tracked files were patched across the
  stage; all four restored.
- **Every subject binary was built with `-o` into scratch**, because a bare `go build ./cmd/...` drops an
  untracked binary in the worktree root.
- **Containers torn down BY NAME.** `docker ps -a` shows the four foreign containers — `infallible_booth`,
  `crazy_kare`, `golink-ai`, `quizzical_goldstine` — **untouched**, exactly as at stage open.
- **No `pkill -f` / `pgrep -f` anywhere** — it matches the harness's own shell and kills the tool call with
  exit 144. Processes were killed only by PIDs captured from `$!` or from a pid file.
- **Probe ports banded 21000-24999**, below the 32768 ephemeral floor, checked with `ss -tan` (ALL states)
  rather than the fail-unsafe listener-only `ss -ltn`.
- ⚠️ **Two transient files were dropped into the MAIN repo root** by a mis-parsed `cd … && … &` in one
  probe. They were removed and the main repo verified back to its pre-existing `?? .claude/` alone. Recorded
  because a silent cleanup is indistinguishable from never having noticed.
- ⚠️ **The Bash-tool cwd reset fired again** (`Shell cwd was reset to /home/esa/git/envoy-go` after a
  `cd` + `go test`), confirming `reference_bash_cwd_reset_commits_to_main` for the fifth consecutive phase.
  Every git command in this stage used `git -C <abs-worktree-path>`.
