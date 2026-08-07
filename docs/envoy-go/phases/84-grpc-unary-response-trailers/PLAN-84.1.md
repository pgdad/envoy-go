# PLAN 84.1 — `grpc-unary-response-trailers`, **the seam leg**: the §6.1 trigger **CROSSES — but at the CENTRAL estimate for the LEG, not at the floor the SPEC computed for the ROW**, and three of the SPEC's own budget figures are refuted (production floor **46 → 211**, unit-test ceiling **1400 → 2926**, `ADR-0306` **~66 → +36…+70 on a block that is already 29 lines landed**); the **RED anchor is CONFIRMED 3/3 with a SECOND, silently-degrading arm the SPEC never named**; ⚠️ **the row's highest-leverage decision — D-84-ENDSTREAM — is INVISIBLE TO THE ENTIRE EXISTING TEST SURFACE, measured independently by two agents**; ⚠️ **the reference REJECTS malformed trailers in its upstream codec and FORWARDS `host` and `trailer` verbatim, so the SPEC's forbidden-field list would have made 84.2 RED on a CORRECT tree**; and ⚠️ **a number in the LANDED `ADR-0306` is wrong — nine slash-form selectors, not ten**

**Stage:** PLAN (lifecycle-state 2 -> 3). **Date:** 2026-08-07.
**Base master:** `c470cf03661c37decb2718a6dd3d96c18f9adbe5` (from `git rev-parse master`, **not from a SHA quoted in any document**), branch `phase-84-plan`.
**The FIRST of the confirmed 84.1 / 84.2 split** (SPEC §12.3). `PLAN-84.2.md` — the differential fixture `0119` — is the session after, and **its IMPL is what flips ROADMAP row 84 `done`** (ADR-0106-as-used, `reference_roadmap_split_phase_row_done`).

⚠️ **ROW 84 STAYS `in-progress`. `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` ARE BYTE-UNTOUCHED AT THIS STAGE. The sentinel `want` STAYS 116. The strict `PROPOSED` guard on `ADR-0306` STAYS ARMED AT 1 — a PLAN that "fixes" it to 0 destroys a live pointer.**

---

## What was EXECUTED at this stage

**Five investigation agents on disjoint remits**, each in its own **DETACHED** worktree off `c470cf03`, with private scratch and a private port band inside `45300-45799`. Unlike a review stage, **four of the five wrote real production code and ran it** — two independent seam patches, a validation prototype with 22 unit cases, a live grpc-go RED-anchor probe, a live `envoyproxy/envoy:contrib-v1.37.2` reference probe over a raw-framer upstream, a live h2spec run, an executed stat-guard blindness probe, and **seven injected break arms**. Every load-bearing claim was controller-re-derived; the re-derivations are recorded inline below with their commands.

Docs-only landing: **ZERO production `.go`, ZERO test `.go`.** All five agents reverted their probes and confirmed `git status --porcelain` = **0 lines**; all docker containers were torn down **BY NAME** (`a3-84plan-{up,ref}`, `a3-84plan-net`, `a4-84plan-*`) — never `prune`, never an `ancestor=`/image filter.

**THIRTY-ONE refutations, THIRTEEN load-bearing.** Two new broken-gate shapes (**34** and **35**), taking the running count to **THIRTY-FIVE**.

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

**Every stage's job is to refute its predecessor by execution, not by review.** Lineage: phase-81 B14/S22/P14/I17 · phase-82 B26/S23/P14/I17 · phase-83 B21/S23/P19/**I37** · phase-84 B22/**S26**/**P31**.

### 1.1 ⚠️ HEADLINE — **§6.1 CROSSES, BUT NOT WHERE THE SPEC SAYS IT DOES.** THE ROW CROSSES AT ITS FLOOR; THE **LEG** CROSSES AT ITS **CENTRAL ESTIMATE**, AND THREE OF THE SPEC'S BUDGET FIGURES ARE REFUTED

SPEC §17 instructs: *"It must open with the §6.1 arithmetic already crossed — §12's floor is ≈1738 … a PLAN that re-derives a figure under 1500 is repeating [phases 80/81's] error."* **That instruction is honoured, and its arithmetic is corrected.**

The ≈1738 floor is a **whole-row** figure: it includes fixture `0119` (764 floor), PKI plumbing (31) and harness registration (5) — **all three of which are 84.2**. Subtracting them leaves the seam leg, and the seam leg must be priced from **landed** buckets, not from a prototype.

| bucket | **floor** | **budget (central)** | **ceiling** | what it rests on |
|---|---|---|---|---|
| production `.go` | **211** | **320** | **450** | two independent methods that agree: (i) comment-normalised prototype — 46 code+blank lines × the house production ratio {2.50, 3.72, 6.03}; (ii) per-file density — **5** production files × {42.7, 61.1, 74.6} landed lines/file. Includes `actions.go`, `write500H2`, the **3-call-site** `writeH2Reply` signature change, and the 5-file falsified-comment sweep |
| — of which **D-84-VALIDATE** | *(inside)* | | | **MEASURED at +94/−19 across 2 files** (§1.5), against the SPEC's ~27 |
| unit-test `.go` | **892** | **1960** | **2926** | floor = the population **minimum** (phase 80, verified); budget = 20 enumerated new test functions × the 98-line mean density; ceiling = 22 × 133. **Population median is 1910** |
| **NET `.go`, 84.1 ONLY** | **≈ 1103** | **≈ 2280** | **≈ 3376** | |
| *(non-`.go`, recorded not gated)* | `ADR-0306` **+36…+70** · `BEHAVIOR_CONTRACT.md` **+55…+61** · `STATE.md` ±9 · `STATE_HISTORY.md` +2 · `PROGRESS.md` · `next-prompt.txt` | | | |

> **⇒ THE ~1500 NET-`.go` TRIGGER CROSSES. Budget ≈2280, 1.5× the trigger; ceiling ≈3376, 2.25×.**
> **⇒ THE ~25-TASK TRIGGER DOES NOT FIRE — 84.1 is enumerated at TWENTY.** The margin is ~20%, thinner than phase 83's 28%.
> **⇒ THE MID-EXECUTION TRIGGER IS AT RISK** on Task 3 (D-84-VALIDATE) and Task 7 (the frame matrix), both of which grew under measurement.

**§6.1 gates *"`PLAN.md` estimates"* — and an estimate is a BUDGET, not a FLOOR.** Every one of the last four rows recorded a budget and landed 1.81–3.07× above it. **A PLAN-84.1 that wrote down ≈1100 would be writing a floor into the budget field, which is precisely the phases-80/81 error the SPEC names.**

⚠️ **AND THE INHERITED SENTENCE "CROSSES AT THE FLOOR" IS TRUE OF THE ROW AND FALSE OF THE LEG.** Carrying it unchanged would be `reference_stale_cite_recurs_fix_by_pattern` — an inherited-but-locally-false sentence acquiring authority by repetition (`reference_brainstorm_adjective_acquires_adr_authority`). **Stated correctly: the row crosses at its floor; 84.1 crosses at its budget.**

**Three SPEC figures corrected, each by measurement:**

1. ⚠️ **PRODUCTION FLOOR 46 → 211.** No row in the four-row population landed under **128** production lines, and that row (phase 80) had **three** production files against 84.1's **five**. A floor of 46 would make 84.1 the smallest production bucket in the population by 2.8× while touching more files. **And it is already falsified by a measured, INCOMPLETE build**: agent A1's variant A measures **+85/−19 = net +66** while *excluding* D-84-VALIDATE, the `router_h2.go` and `chain.go` comment rewrites, the H1/H3 non-change sentences, and any encode-chain fix. `reference_measured_prototype_is_a_lower_bound`, firing on the very number that was meant to bound it.
2. ⚠️ **UNIT-TEST CEILING 1400 → 2926.** Three of the four reference rows (1805, 2014, 2921) **exceeded 1400**, and the population **median is 1910**. A ceiling that 75% of its own reference distribution crossed is not a ceiling. And because 84.2 is the fixture leg, **84.1 carries essentially the whole row's unit-test bucket.**
3. ⚠️ **`ADR-0306` ~66 → +36…+70.** The block is **already 29 lines landed** (`DECISIONS.md:17928-17956`), so ~66 double-counts. Completed ADR spans measured: 62 / 66 / 72 / 62 / 68 (ADR-0301..0305).

⚠️ **AND THE COMMENT-FRACTION CATEGORY ERROR — THE SPEC COMMITTED THE SPECIES IT DIAGNOSED ONE SECTION EARLIER.** SPEC §1.3 item 3 measures **34.8 / 38.0 / 40.4 / 46.6%** and §12.1 applies that fraction to a **production-only** bucket. Controller-re-derived over the four IMPL diffs, `+` lines only, whitespace-stripped `//` rule stated:

| phase | **whole-`.go` diff** | **PRODUCTION-only** |
|---|---|---|
| 80 | 34.8% (592/1701) | **60.0%** (81/135) |
| 81 | 38.0% | **78.3%** (332/424) |
| 82 | 40.3% | **64.8%** (432/667) |
| 83 | 46.6% (1762/3785) | **83.4%** (654/784) |

**The 39.2% is a whole-diff fraction dominated by the test bucket (26–37%), which is 5–8× the production bucket's size.** The production-only fraction is **60.0–83.4%, median 71.6%.** SPEC §1.3 item 1 flags exactly this — *"the ratio's denominator counts fixture drivers as production… fix the category before the next estimate inherits it"* — **and §12.1 re-inherited it one section later.** `reference_change_set_measure_not_build_measure` has now fired at three consecutive documents. **Do not price production with 39.2%.**

⚠️ **AND THE SPEC'S "31.3%, already at the house median" IS REFUTED IN BOTH DIRECTIONS.** A landed-quality build measures **45.9%** (39 comment / 85 added), and the house production median is **71.6%**, not 31.3%. **The "add comments later" headroom §3.4 implies does not exist, and the gap is larger than the SPEC believed.**

### 1.2 ⚠️ SECOND HEADLINE — **THE RED ANCHOR IS CONFIRMED 3/3, AND THERE IS A SECOND ARM THE SPEC NEVER NAMED WHOSE FAILURE MODE IS WORSE**

SPEC §12.3 rests the entire split on 84.1 having a failing-first anchor **outside** the differential harness. **Built and run at this tip**, with three controls.

Shape: grpc-go health server (verbatim `serveGRPCHealth`, `runner_test.go:3105`) on `127.0.0.1:45310` h2c; envoy-go subject with a TLS + `alpn_protocols: ["h2","http/1.1"]` + `codec_type: AUTO` listener on `45301` (0079's shape); PEMs from `test/fixtures/0079-h2-multiplex-pool/pki/`; upstream cluster `explicit_http_config.http2_protocol_options{}`; grpc-go client with `credentials.NewTLS(RootCAs=ca.pem, ServerName="localhost")`.

| arm | BASE `c470cf03` | VARIANT A |
|---|---|---|
| **success unary** | **`rpc error: code = Internal desc = server closed the stream without sending trailers`** — 3/3, and 3/3 again on a re-run | **`SERVING`** — 3/3 |
| ⚠️ **error unary** (`/a1.NoSuchService/NoSuchMethod`) | **`Unknown desc = `** — an **EMPTY message**; grpc-go infers `codes.Unknown` from HTTP 200 with no `grpc-status` | **`Unimplemented desc = unknown service a1.NoSuchService`** |
| plain-H2 GET (**invariance control**) | `status=200 body="backend-0:/plain"` | **byte-identical** |

⚠️ **THE ERROR ARM DEGRADES SILENTLY RATHER THAN ERRORING, WHICH IS A WORSE FAILURE MODE THAN THE CHARTERED ONE**, and no prior document names it. It independently corroborates SPEC §2.1's refutation of *"gRPC error RPCs already pass unpatched"* — by a **second, different** mechanism. **The PLAN carries both arms.** The plain-H2 GET arm is the ready-made invariance control for the break roster.

⇒ **84.1's split is SOUND on this axis. The seam leg has a real RED anchor with no fixture.**

### 1.3 ⚠️ THIRD HEADLINE — **THE ROW'S HIGHEST-LEVERAGE DECISION IS INVISIBLE TO THE ENTIRE EXISTING TEST SURFACE.** TWO AGENTS, DISJOINT REMITS, THE SAME RESULT

D-84-ENDSTREAM (SPEC §8.1) is disposed CONDITIONAL. **The disposition is upheld. Its gate is not.**

- **A1 built variant B** (unconditional END_STREAM hold + always-emit a trailing block) and ran it: `go test ./internal/filter/hcm/... ./internal/filter/http/router/... -count=1` → **3 ok, EXIT=0**; the grpc RED probe → **3/3 SERVING**, error arm correct, plain-H2 GET 200. **Variant B passes every existing unit test and every runtime probe.**
- **A4 independently injected the unconditional variant** (`endStream := false`) against a minimal working patch: it reddened **exactly two assertions, both newly written**. **Zero pre-existing tests in the three packages moved.**

⇒ **The conditional/unconditional choice cannot be discharged by anything that exists today.** This is `reference_probe_must_discriminate` sitting directly on the row's most consequential decision — and it means **the SPEC's blast-radius argument, whatever its number, is not defended by the current suite at all.**

⚠️ **AND THE BLAST-RADIUS NUMBER ITSELF IS REFUTED ~13×.** Controller-re-derived:

```
fixture dirs:                                     120
dirs mentioning http2_protocol_options:            35   (SPEC: "40 of 120")
dirs declaring alpn_protocols:                      4   → 0004, 0079, 0080, 0104
  of which downstream ALPN-h2:                      3   (0104 is HTTP/3, alpn h3)
```
`writeH2Reply` is the **downstream** emit. The other 32 are **upstream cluster** config (gRPC access-log / tracing / stats-sink / ext_authz / ext_proc / SDS clusters) where envoy-go is the H2 *client* and `writeH2Reply` never fires. **The unconditional variant's differential blast radius is 3 fixtures, not 40.**

**DECISION: CONDITIONAL STANDS — but on the corrected ground and with a new gate.** 3 ≠ 0, and byte-identity on every existing path is free. **Task 7 builds the 4-cell frame-sequence matrix that makes the decision measurable**: `{trailers, no trailers} × {body, no body}`, asserting the ordered tuple `(frame, end_stream)`. **Without it the decision is UNMEASURED, not merely ungated. The PLAN must not claim the existing suite defends it.**

### 1.4 ⚠️ FOURTH — **THE REFERENCE REJECTS AT CAPTURE TIME, AND THE SPEC'S FORBIDDEN-FIELD LIST WOULD HAVE MADE 84.2 RED ON A CORRECT TREE**

Agent A3 stood up `envoyproxy/envoy:contrib-v1.37.2` (the ADR-0008 pin) as an H2→H2 proxy on a bridge network in front of a raw-framer upstream emitting hand-built trailer blocks. **16 paths, one request each, every field verified in the upstream's own log line.**

| trailer field | RFC 9110 §6.5.1 / 9113 §8.2.2 | **reference MEASURED** |
|---|---|---|
| `content-length`, pseudo-header, `transfer-encoding`, `connection`, `keep-alive`, `proxy-connection`, `upgrade`, `te: gzip` | barred | **REJECT** → RST_STREAM(INTERNAL_ERROR) |
| trailing block **without END_STREAM**; a **second** trailing block | PROTOCOL_ERROR | **REJECT** |
| `te: trailers`, `set-cookie`, legal `grpc-status: 0` | legal | forwarded verbatim |
| ⚠️ **`host`** | **barred by RFC 9110 §6.5.1** | ⚠️ **FORWARDED VERBATIM** |
| ⚠️ **`trailer`** | **barred by RFC 9110 §6.5.1** | ⚠️ **FORWARDED VERBATIM** |
| **empty** trailer block | — | ⚠️ **DROPPED**, replaced by `DATA len=0 END_STREAM` |

**Which layer rejects — measured, not inferred.** After 27 requests: `cluster.up.http2.rx_messaging_error: 17` · `cluster.up.upstream_rq_tx_reset: 17` · `http.ingress.downstream_rq_tx_reset: 17`, and **17 = 7 + 10, exactly the reject rows across the two runs.** The rejection is booked in the **upstream H2 codec** — the precise structural analogue of `h2/client.go` — and propagates downstream as a stream reset.

⚠️ **SPEC §8.3's field list is REFUTED on `host` and `trailer`.** Filtering them would make 84.2's differential **RED on a correct implementation**. **The binding set is RFC 9113 §8.2.2 + `content-length` + pseudo-headers + END_STREAM — NOT RFC 9110 §6.5.1.**

⚠️ **A PROBE-INPUT BUG THE AGENT CAUGHT AND FIXED, RECORDED RATHER THAN HIDDEN** (`reference_probe_input_is_a_claim`): the first upstream dispatched on `strings.HasPrefix(path, "/te")`, so `/tegzip` matched the transfer-encoding arm and **the `te: gzip` case never ran** — while printing a plausible RST_STREAM. Rewritten to an exact `switch path` and re-run. **The table above is the corrected run.**

### 1.5 ⚠️ FIFTH — **D-84-VALIDATE IS A 3.5× / 2.1× UNDER-ESTIMATE, AND "`h2/stream.go` STAYS BYTE-UNTOUCHED" IS FALSIFIED BY REUSE**

SPEC §8.3 prices it at ~27 production + ~110 test. **Written, built, run, broken, reverted:**

| bucket | SPEC | **MEASURED (LOWER BOUND)** |
|---|---|---|
| production | ~27 | **+94 / −19 (net +75)** across **2 files**; comment fraction **37.6%** |
| unit tests | ~110 | **~230** — 195 (17 direct cases + 5 wire cases + the second-block test) + a **28-line** frame-scripting peer helper the wire tests cannot do without |

⚠️ **AND THE REUSE QUESTION THE SPEC RULES AS SETTLED IS NOT.** The connection-specific set exists **exactly once** in production, and it is **inline inside `buildRequest`**, not a helper — controller-verified at `internal/filter/hcm/h2/stream.go:417-425`:

```go
// RFC 9113 §8.2.2: connection-specific header fields are PROTOCOL_ERROR.
switch name {
case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
    return nil, &Error{Code: ErrProtocolError, Msg: "connection-specific header field: " + name}
case "te":
    if h.Value != "trailers" { … }
}
```

**Reuse and "`h2/stream.go` stays byte-untouched" are mutually exclusive.** Measured extraction (`isConnectionSpecificField` + `const teTrailersValue`): **`stream.go` +22/−6**.

> **DECISION: EXTRACT. Reuse beats duplication, and SPEC §9's byte-untouched claim does not survive this leg.** The roster is **five** production files, not four. Duplicating the set into `client.go` would create a second source of truth for an RFC list — the failure mode `reference_grep_for_sibling_derived_constant` exists to prevent.

### 1.6 ⚠️ SIXTH — **A NUMBER IN THE LANDED `ADR-0306` IS WRONG, AND IT PROPAGATED INTO FOUR DOCUMENTS AND A COMMIT SUBJECT**

Controller-re-derived from `test/conformance/h2spec/h2spec.go`:

```
command grep -cE '"http2/6/[0-9]+"' test/conformance/h2spec/h2spec.go   → 9
command grep -oE '"http2/[0-9./]+"' … | wc -l                          → 14   (denominator)
```

`http2/6/6` is a **`//` comment line**, not a declared string: `// http2/6/6 = PUSH_PROMISE — excluded per ADR-0051 (ENABLE_PUSH=0)`.

⚠️ **THERE ARE NINE SLASH-FORM SECTION-6 SELECTORS, NOT TEN** — and *"ten"* appears in **`DECISIONS.md:17940` (`ADR-0306` §Context ¶4, LANDED)**, in SPEC §1.1 and §13 item 1, in `STATE.md` §Current, in `next-prompt.txt`, and in the phase-84 SPEC commit's own subject line.

**A1's measurement strengthens the finding rather than weakening it:** decomposing the live `http2/6` run by subsection heading, **section 6.6 contributes ZERO cases — no `6.6` heading appears at all** — so all **42** cases map **1:1 onto the nine declared selectors**. The 42 is not an over-count, and **`ADR-0051`'s PUSH_PROMISE-exclusion rationale is moot** for a second reason.

**DISPOSITION: RECORDED HERE; CORRECTED IN PLACE AT THE 84.1 IMPL when `ADR-0306`'s §Decision/§Consequences land.** `DECISIONS.md` is append-only per ADR-0288 §Decision 4, **but an ADR's own §Context has been corrected in place before** — ADR-0297 ¶7/¶9 at the phase-75 IMPL, and **four** of ADR-0298's §Context paragraphs at the phase-76 IMPL, each **leading with what survives**. ⚠️ **The PLAN itself leaves `DECISIONS.md` BYTE-UNTOUCHED.**

### 1.7 ⚠️ SEVENTH — **`actions.go` IS DEAD ON THE PRODUCTION PATH, AND THE SPEC'S ARM-CENSUS NC DID NOT CONTROL ITS CENSUS**

The census reproduces exactly — **5 lines / 3 emit sites / 2 files**:

| site | disposition for 84.1 |
|---|---|
| `h2dispatch.go:699 + :703` — `writeH2Reply` | **the chartered arm.** ⚠️ **All THREE of its callers must be updated** (`:311` no-route 404, `:527` local-reply, `:602` the router-action path — only `:602` passes non-nil). Two of them **broke the build** until updated. `writeH2Reply` has **0 test callers** |
| `h2dispatch.go:723` — `write500H2` | **no change.** Defensive non-router terminal; no upstream ⇒ no trailers. One sentence |
| `actions.go:135 + :138` — `directResponseAction.writeH2` | ⚠️ **no change, and the reason is STRONGER than the SPEC's** |

⚠️ **`git grep -n '\.writeH2(' -- '*.go'` returns EXACTLY ONE LINE tree-wide: `internal/filter/hcm/actions_test.go:86`.** Production `direct_response` over H2 goes `asRouterActionH2` → `ActionResponse` → **`writeH2Reply`**. The SPEC's *"A SECOND FILE, NOT IN THE 4-FILE ROSTER"* is a real grep hit but **not a live arm**. Say **"test-only-reachable"**, not "a second file we chose not to touch". *(Bonus: `h2dispatch.go:117`'s comment — *"we surface the 404 via a directResponseAction-equivalent closure (**writeH2 invocation**)"* — is **false**; it uses `asRouterActionH2`.)*

⚠️ **BROKEN-GATE SHAPE 34 — A NEGATIVE CONTROL RUN WITH A BROADER PATTERN THAN THE GATE IT CONTROLS.** SPEC §9 states *"NC: the same pattern in `_test.go` returns 9 files / 41 lines."* Controller-re-derived:

| pattern | `_test.go` result |
|---|---|
| `sw\.WriteHeaders\(\|sw\.WriteData\(` — **the census pattern** | **9 lines / 3 files** |
| `WriteHeaders\(\|WriteData\(` — unanchored | **41 lines / 9 files** ← what the SPEC reported |

**The NC was run with a broader pattern than the census it was attached to.** It fires either way, so it *reads* as proof the census discriminates — while never exercising the census's actual selector. **The census conclusion ("bounded at 3, not 11") survives; the NC as written does not.** Same family as `reference_gate_command_negative_control` and `reference_leading_hyphen_pattern_reads_as_flag`.

### 1.8 ⚠️ EIGHTH — **THE SECOND-TRAILING-BLOCK RULE IS DEAD CODE ON THE WIRE. DROP IT.** *(BROKEN-GATE SHAPE 35)*

A3 injected a break removing the `alreadySeen` leg. **Only the direct table's `/second_block` case reddened; no wire test moved.** A purpose-built wire probe then proved there is **no frame sequence that reaches it**:

- **first block without END_STREAM** → the END_STREAM leg fires first. *(This laundered a green in the agent's own first probe: `two_blocks_differing` reported the correct rejection via the **wrong assertion** — `"trailing HEADERS without END_STREAM"`, not `"second trailing HEADERS block"`. `reference_deliberate_break_wrong_assertion`, live.)*
- **both blocks with END_STREAM** → measured `err=<nil>, trailers=[grpc-status="0"]` (the **first** block) **and `cc.Closed()==true`**: the first block's `cs.finish(nil)` returns `RoundTrip`, whose `defer cc.streams.Delete(id)` runs; the second block then misses `lookupStream` and is handled at **conn level** as `connError(ErrProtocolError, …)` + GOAWAY — **after `RoundTrip` already returned success.**

⚠️ **SHAPE 35 — AN ORDERED VALIDATION LEG THAT NO WIRE SEQUENCE CAN REACH.** An earlier leg intercepts every malformed path; the legal path returns before the second block arrives. **The table entry reads as coverage.** Distinct from `reference_vacuous_break_modes`' "ordered legs trip an EARLIER leg" because here the leg is unreachable *in production*, not merely shadowed in the test.

**DECISION: DROP the `alreadySeen` leg and the `respTrailersSeen` field (≈6 lines), and RECORD WHY** so no later stage re-adds it. The END_STREAM leg subsumes it and the reference reaches the same reject by the same mechanism.

### 1.9 ⚠️ NINTH — **TWO LIVE TIP DEFECTS THE VALIDATION INCIDENTALLY CLOSES, NAMED BY NO DOCUMENT**

1. **A trailing HEADERS block WITHOUT END_STREAM hangs `RoundTrip` to the request timeout — today, before any capture.** Measured: `elapsed=1.5s err=context deadline exceeded status=0`. **The reference RSTs immediately.** This is a hang-to-timeout on a malformed upstream frame that exists at `c470cf03`.
2. **A second END_STREAM block GOAWAYs the pooled upstream connection AFTER `RoundTrip` has already returned 200** (§1.8's measurement).

⚠️ **AND SPEC §8.3's SMUGGLING SENTENCE IS TRUE OF THE ROW'S OUTPUT, NOT OF `master`.** At the tip **nothing is smuggled because nothing is retained** — the trailing block is decoded (for HPACK dynamic-table correctness) and dropped. With the minimal capture patch applied, smuggling is demonstrated **end-to-end**:

```
ARM2[content_length_smuggle]: status=200 TRAILERS=[grpc-status="0" content-length="999"]
ARM2[pseudo_header_status]  : status=200 TRAILERS=[:status="500" grpc-status="0"]
ARM2-EMIT[content_length]: order=[headers data headers] endStream=[false false true]
ARM2-EMIT[pseudo]        : TRAILER BLOCK ON WIRE = [:status="500" grpc-status="0"]
```

⚠️ **AND IT IS WORSE THAN §8.3 STATES: a PSEUDO-HEADER reaches the downstream trailing block, which is itself a PROTOCOL_ERROR.** envoy-go would be **generating malformed HTTP/2**, not merely relaying a bad field. **State the argument as "we are about to create a conduit", not "we have a bug".**

### 1.10 ⚠️ TENTH — **THE ENCODE CHAIN IS TOLD A LIE, AND IT IS MEASURED, NOT INFERRED**

A1 instrumented both sites under variant A and drove one real gRPC unary RPC:

```
A1PROBE: chain told RunEncodeHeaders(endStream=false) len(body)=7 len(trailers)=1
A1PROBE: chain told RunEncodeData(endStream=true)     len(body)=7 len(trailers)=1
```

**Wire:** `HEADERS(false) → DATA(false) → HEADERS(trailers, true)`. **Chain:** `EncodeHeaders(false) → EncodeData(true)`. **The chain is told END_STREAM is on the DATA frame while a trailing block still follows.** The bodyless-with-trailers case tells `RunEncodeHeaders(endStream=true)` with a block still to come.

⚠️ **Cite correction: the hardcoded `true` to `RunEncodeData` is at `h2dispatch.go:582`, not `:583`** (`:583` is a comment inside the error branch). `:575` is exact.

**Variant A does not fix this and the SPEC's roster does not budget for it.** **Task 8 decides and lands one of the two dispositions — it must not be left implicit.** This is the one place 84.1 can silently ship a filter-visible contract break.

### 1.11 ⚠️ ELEVENTH — **THE EXISTING-TEST CHURN IS NOT CHURN, AND THE GREP FIGURE IS WRONG IN BOTH DIRECTIONS**

SPEC §9: *"69 lines mentioning `endStream`/`END_STREAM` across 7 test files."* Controller-re-derived, **stating the pattern** because the figure is pattern-dependent:

| pattern / scope | lines | files |
|---|---|---|
| `endStream\|END_STREAM`, the **two** packages | **48** | **7** |
| `endStream\|END_STREAM`, all **three** packages | **55** | **9** |
| case-insensitive `end_?stream`, all three packages | **82** | **9** |

**The file count 7 is exact for two packages; no pattern yields 69.** And SPEC §12.1 lists `internal/filter/http/router` in the same row while the 69/7 omits it.

⚠️ **THE REAL FINDING: THE CHURN IS ZERO.** Three agents independently ran the touched packages under a working patch — `go test ./internal/filter/hcm/... ./internal/filter/http/router/... -count=1` → **all packages `ok`, EXIT=0**, under variant A **and** under variant B **and** under the full validation patch. **No pre-existing test breaks.**

⇒ **The unit-test bucket cannot be justified by churn.** Those 48–55 lines assert *upstream/downstream codec* END_STREAM handling, not `writeH2Reply`'s placement. **Justify the bucket by net-new discriminating assertions, and state that the existing suite is provably blind to the entire change.**

### 1.12 ⚠️ TWELFTH — **THE `BEHAVIOR_CONTRACT.md` EDIT SURFACE IS SMALLER AND SHARPER THAN THE SPEC SAYS, AND ONE OF ITS PHRASE ANCHORS FINDS NOTHING**

All five line cites resolve **exactly** at `:16 :682 :2043 :2068 :4291` in a **5900**-line file. But:

⚠️ **`:4291-4311` IS NOT ONE BLOCK.** `:4293` is the falsified prose; **`:4295` opens a DIFFERENT subsection** (`#### PARSE-REJECT roster (per ADR-0080) — PARITY vs DEPARTURE`), and `:4311`'s `| 2 trailer match arms | REJECT | boots and honors | DEPARTURE |` row **stays TRUE** — ADR-0273's boot-reject is undisturbed, because `RunEncodeTrailers` stays dead (SPEC §14.1). **A PLAN treating the range as one editable block would rewrite the tap parse-reject roster by accident.**

⚠️ **AND THE SPEC'S PHRASE ANCHOR FOR THAT HEADING FINDS NOTHING** — it renders `:scheme` without backticks:

```
command grep -cF -- '#### Trailers and :scheme coverage boundaries'    → 0    ← the SPEC's rendering
command grep -cF -- '#### Trailers and `:scheme` coverage boundaries'  → 1    ← :4291
```

**Cite-shift arithmetic, re-derived:** **197** `BEHAVIOR_CONTRACT.md:NNNN` cites across 71 files + **76** `BC:NNNN` cites = **273**. A **tail append shifts 0 of 273**. An insert at `:2043`/`:2068` shifts **23 of 197 plus 6 of 76 = 29 of 273** (the SPEC's "23 of 197" omits the `BC:` half). ⚠️ **An insert at `:16` shifts 196 of 197 — categorically forbidden.**

⚠️ **AND THE SPEC'S OWN COMMIT FALSIFIED ITS `next-prompt.txt` BLINDNESS EXAMPLE.** `BEHAVIOR_CONTRACT.md:1967` was present in `next-prompt.txt` at `e1c07208` and is **gone at this tip** — the phase-84 SPEC commit rewrote the router and dropped it. `command grep -r` and `git grep` therefore **agree at 197 right now**. ⚠️ **The METHOD still binds** (the file is tracked-but-gitignored and the next roll will very likely re-introduce a cite) — **but any sentence asserting a LIVE discrepancy is false at this tip.** `reference_recursive_grep_blind_to_gitignored_tracked_file` is about the *method*, not about a standing number.

### 1.13 Also refuted — nine more, each with its cite

- ⚠️ **`go test ./...` "126 ok / 0 FAIL" DOES NOT REPRODUCE.** Measured: **`INNER_EXIT=1`, 125 ok / 109 no-test-files / 1 FAIL of 235 packages**, aborting on `0084-otlp-access-log` — `bind: address already in use` at `driver.go:169 ensureServer`, the known driver-receiver port race. **Every non-differential package is green.** No sibling `go test` was running (`ps` + `git worktree list` checked). ⚠️ **`./...` runs the differential CONCURRENTLY with 234 other packages — a worse environment for that race than the standalone gate.** The PLAN cites it honestly, both ways.
- ⚠️ **`assertThreshold`'s `if s.Tests == 0 { continue }` is at `h2spec_test.go:310`**, not the SPEC's `:309-311`. The `continue` is real; `TestH2Spec` skips under `-short` at `:32`.
- ⚠️ **`REVIEW.md`: 37 of 125 phase dirs, not 37 of 124** — the phase-84 directory now exists. **The phase falsified its own count with its own commit, for the second consecutive stage** (`reference_branchpoint_roster_stale_midrow`). Newest carrier: `phases/25.3-http-filter-wasm-perroute-and-conformance/`.
- ⚠️ **Test-function denominators are matcher-dependent: 211/77/68 under `^func (Test|Fuzz|Benchmark)`, 210/75/68 under `^func Test`.** The SPEC states 211/77/68 without the matcher. **State the matcher.**
- ⚠️ **`serverStream.recvTrailingHeaders` is `:138-166`, not `:138-165`** — off by one at the tail. **Use the SYMBOL anchor.**
- ⚠️ **"last-one-wins on a second block" is a property of the PATCH, not of the tip.** At `c470cf03` it is *first-one-wins for `respHeaders` and total discard of the trailing block* — there is no "wins" because nothing is retained.
- ⚠️ **The frame-sequence double ALREADY EXISTS and must not be reinvented.** `captureH2Writer` (`internal/filter/hcm/h2dispatch_test.go:39`) is mutex-guarded, deep-copies, and carries an **`order []string`** field that makes the frame sequence directly assertable. ⚠️ **`captureSW` (`actions_test.go:66-80`) is the WRONG one** — it appends to **one shared `endStream []bool`** from both `WriteHeaders` and `WriteData`, conflating exactly the axis D-84-ENDSTREAM tests (`reference_shared_codepath_defeats_per_arm_counts`). Use `captureH2Writer`; do not widen `captureSW`.
- ⚠️ **The upstream-side peer fake exists but cannot emit trailers.** `fakeH2ServerPeer` (`h2/client_test.go:216`) sets END_STREAM on the last of HEADERS/DATA. Gap cost measured: **one ~18-line `writeResponseWithTrailers` method**. Router side needs a **~50-line `runH2TrailerBackend`** written as a **separate** function — adding an arm to `runH2Backend` would touch a helper four other test files depend on.
- ⚠️ **A FIFTH production file carries a falsified ADR-0058 statement** the SPEC's roster omits: `internal/filter/http/chain.go:483` — *"trailers are observed-and-discarded in the codec layer per ADR-0058"* (decode-side context; must be narrowed to **request** trailers). Token census: **5 files** (`h2/client.go:440`, `h2dispatch.go:502`, `chain.go:483`, `router_h2.go:253`, `jwt/doc.go:97`).

### 1.14 CONFIRMED, so the IMPL can rely on it

- **The full 120-fixture differential is GREEN at this tip**: `INNER_EXIT=0`, **120/120 PASS**, 0 FAIL/SKIP, 0 `no driver registered`, 0 panic/DATA RACE/SIGSEGV, **388.78 s** (inside the SPEC's doc-sourced 384.311 s envelope). `=== RUN` denominator 120. Registration gates all 120/120/120.
- **The h2spec selector defect, LIVE**: `http2/6/10` → **`No matched tests found.`, exit 0** — the silent no-op, reproduced · `http2/6.10` → **6/6** · `http2/6` → **42 tests, 37 passed, 1 skipped, 4 failed, exit 1** · **NC `http2/5` → 22/22**, matching `CONFORMANCE_PINS.md`'s 5.x sum, so the harness is not globally broken. The four failures are **exactly** SPEC §1.1's.
- **The conformance gate is not run in CI**: `command grep -rn -E 'conformance|h2spec|proxy-wasm' .github/` → **rc=1, 0 lines**; NC `go test|golangci` → **8 lines**.
- **The `TestNoNewStat*` blindness, PROVEN BY EXECUTION** with both arms compiling: a **compiling** `reg.NewCounter("a4_probe_trailer_total")` inside `writeH2Reply` → **5 PASS (BLIND)**; the same registration inside `NewFlusher` → **5 FAIL**. All five guards live in one file, `internal/statssink/registration_test.go:26/53/81/109/137`.
- **Stat census exact**: **208 code sites / 36 production files** (raw 210; Counter 175 + Gauge 35 = 210; incl. tests 508 hits / 84 files). ⚠️ **Cite 208/36, never 208/84.** NC `\.NewHistogram\(` → 0, so the two arms are disjoint and neither may be dropped.
- **Fuzzers 55 in 48 files, all under `internal/`, ZERO under `test/`**; unrestricted 161, delta 106 across 40 files that are **100% Markdown**.
- **`h2/stream.go` needs no `WriteHeaders` interface change** — proven by execution: a second `sw.WriteHeaders(tf, true)` on an already-headers-written stream reached a real grpc-go client intact. *(This survives; what does **not** survive is the byte-untouched claim, §1.5.)*
- **Fan-out is additive with exactly ONE populate site.** `ActionResponse` 19 files / 22 construction sites (18 non-test); `H2Response` 2 files / 15 lines. **`router_h2.go:202` (the `doH2ClusterAction` success return) is the only site that populates `Trailers`.** `retryExecutorH2`/`hedgeExecutorH2`/`router_weighted.go` propagate the **whole struct by value**, so trailers ride through with no edits; `retry.go:316`'s synthesized 504 replaces the value and correctly drops them. `go build ./...` and `go vet ./...` both rc=0.
- **`gofmt -l` empty** on all three packages (62 `.go` files); **`golangci-lint run` rc=0** at **v1.64.8**, `disable-all: true` + 9 linters, `misspell` locale **US**. ⚠️ **The NC FIRED on British prose** (`behaviour`) — and it fired on an agent's own comment, live. `normalised` in the same comment was **not** flagged.
- **PKI reuse**: `0004`/`0079`/`0080` ship sha256-identical `ca.pem`/`listener.pem`/`listener.key.pem`.
- **`stream.go:308-315` already carries an `*h2.Error`'s code into `writeRSTStream`** — the RST_STREAM disposition reuses an existing pattern rather than inventing plumbing.

### 1.15 ⚠️ An agent claim that did NOT survive, and a controller position that changed

- **A2 proposed widening `captureSW`.** Controller-refuted by A4's independent finding: `captureH2Writer` already carries `order` and is the correct double. **A2's diagnosis of `captureSW` is right; its remedy is not.** *(This is why five disjoint remits are run: the same defect surfaced from two sides and only the union produced the right action.)*
- **The controller's own framing changed at §1.1.** I began by intending to restate the SPEC's *"crosses at the floor"*. A2's leg-scoped enumeration showed that sentence is **true of the row and false of the leg**, and restating it would have been an inherited-but-locally-false claim. **A controller brief is a claim too** — this is the third consecutive stage to record that.

---

## 2. THE ITEMS THE SPEC SAYS THIS PLAN OWES — ALL SEVEN DISPOSED

| SPEC §17 owed item | disposition |
|---|---|
| a TDD spine over the four production files | **§4 + Tasks 2, 4, 5, 6** — and the roster is **five**, not four (§1.5), with **three** `writeH2Reply` call sites (§1.7) |
| §8.3's validation | **Task 3.** Capture-time in `h2/client.go`, **fail the stream** via `h2.NewStreamError`; enforced set corrected (§1.4); second-block rule dropped (§1.8); priced +94/−19 prod + ~230 test (§1.5) |
| the D-84-ENDSTREAM conditional implementation | **Task 6, gated by Task 7.** Disposition stands; **its ground is corrected (3 fixtures, not 40) and its gate is NEW** because the existing suite cannot see the difference (§1.3) |
| the H1/H3 non-change regression tests | **Tasks 10 and 11.** ⚠️ **Behavioural, not signature-shaped** — a signature assertion passes on a compiler check. Each asserts **full serialized bytes** are identical with `Trailers` populated vs nil, and **both are paired with the H2 positive arm in the same file** or the pair is one-sided again |
| `ADR-0306`'s §Decision/§Consequences **shape** | **Task 15.** Target 5–8 `**§Decision N**` + 7–8 `**§Consequences N**` paragraphs, block 29 → **62–72** lines, footer RETAINED, **no renumber, NO `---` separator**, STATUS flips `PROPOSED` → `COMPLETE` (disarming the strict guard 1 → 0, which is correct). **Plus the ¶4 "ten" → "nine" in-place correction** (§1.6) |
| the `BEHAVIOR_CONTRACT` reconciliation for its five falsified statements | **Task 16.** Five **in-place net-zero** rewrites + **one tail append**; the range `:4291-4311` corrected to the single line `:4293` with `:4311` left alone (§1.12) |
| **open with §6.1 already crossed** | **§1.1 and §7.** Crossed at the **budget** (≈2280 vs ~1500), with the floor/budget/ceiling distinction stated and three SPEC figures corrected |

⚠️ **Three items the SPEC leaves UNDECIDED and this PLAN rules:**

1. **Does D-84-VALIDATE duplicate the connection-specific list or extract a helper?** → **EXTRACT** (§1.5). `h2/stream.go` is **in** the roster.
2. **What is the encode chain told at `:575`/`:582`?** → **Task 8**, decided there, not left implicit (§1.10).
3. **Does `ADR-0306` complete at the 84.1 IMPL or the 84.2 IMPL?** → **at the 84.1 IMPL** (Task 15). It covers both legs (SPEC §12.3), but leaving it `PROPOSED` across an entire extra phase leaves the strict guard armed with nothing pointing at it. 84.2's IMPL amends in place if it learns something new — the ADR-0296 indented-blockquote precedent.

---

## 3. Global constraints

1. **TDD, strictly.** Every task lands its failing test **first**, and the RED is **observed and recorded** (assertion text, not "it failed"). `reference_liveness_break_needs_failing_baseline`: green can also mean "did not run".
2. **`t.Errorf` per property, never `t.Fatalf`** except on harness setup — `reference_fatalf_makes_assertions_unreachable`. A3's break B fired **two assertions in one subtest**; a `Fatalf` would have hidden the second.
3. **`-count=1` on every gate and every break arm.** Caching serves a stale PASS.
4. **`INNER_EXIT` is mandatory** on every differential launch **and** on `go test ./...` — the phase-83 IMPL's first launch aborted while the surrounding tooling reported success, and it happened again at this stage (§1.13). **Budget 2–3 launches.**
5. **Per-task `gofmt` + `golangci-lint run`** on touched packages. ⚠️ **A `typecheck` failure short-circuits `misspell`** — a *compiling* defect is required to exercise the style linters, and only British spellings in **prose** fire (not CamelCase identifiers).
6. **Restoration after every break arm verified by sha256**, not by eye. Assert the injection site's occurrence count is exactly 1 **before** writing.
7. **Assert the SYMBOL landed, not that the build passed.** A3's first capture edit had a one-tab indentation mismatch, `go build` passed, and the probe printed an empty trailer set — **indistinguishable from "the capture doesn't work"**. A build is not evidence the edit landed.
8. **Re-derive the break roster at the IMPL tip**, never carry this PLAN's — `reference_break_roster_goes_stale_within_its_own_row`.
9. **Subagent-driven**, subagents commit **locally only**, controller squash-pushes at close. One worktree per agent off a common base; private scratch; private port bands **outside** `20000-31007`, `11000-14999` and the static fixture range `10000-19172`.
10. ⚠️ **Do NOT wire `RunEncodeTrailers`** — SPEC §14.1, three independent measurements. ADR-0273's boot-reject stays undisturbed, and `BEHAVIOR_CONTRACT.md:4311` therefore stays true.

---

## 4. File structure — the IMPL's edit surface, re-derived at `c470cf03`

**FIVE production files, not four.** All line cites below were re-verified this stage; **prefer the symbol anchor** — `reference_stale_cite_recurs_fix_by_pattern`.

| file | symbol | change |
|---|---|---|
| `internal/filter/hcm/h2/client.go` | the `if !cs.respHeadersSeen` block ending `:439`; the discard comment `:440`; HPACK decode `:425` | capture the trailing block into `cs.respTrailers`; **validate before retaining**; widen `H2Response` with `Trailers []hpack.HeaderField`; populate on the `RoundTrip` return |
| ⚠️ `internal/filter/hcm/h2/stream.go` | `buildRequest`'s inline set `:417-425` | **extract** `isConnectionSpecificField` + `const teTrailersValue` (**+22/−6**). ⚠️ **NOT byte-untouched — SPEC §9 refuted** (§1.5) |
| `internal/filter/http/router/router.go` | `ActionResponse` `:79`; the `Close` codec-scoped-comment precedent `:83-88` | add `Trailers []hpack.HeaderField` with an **"(H2 only; H1/H3 ignore)"** comment |
| `internal/filter/http/router/router_h2.go` | `doH2ClusterAction` `:73`; success `ActionResponse{` `:202`; the falsified ADR-0058 block `:253-257` | populate at the **one** success site; **6 local-5xx sites stay nil**; rewrite the three falsified clauses |
| `internal/filter/hcm/h2dispatch.go` | `writeH2Reply` def `:671`, doc comment `:669-670`, `endStream :=` `:698`, emits `:699`/`:703`; callers `:311`/`:527`/`:602`; `RunEncodeHeaders` `:575`, `RunEncodeData` `:582`; `write500H2` `:715`/`:723`; the false comment `:117` | conditional END_STREAM + trailing emit; **update all three callers**; Task 8's chain decision; rewrite the doc comment that states the broken rule verbatim |
| *(comment-only)* `internal/filter/http/chain.go:483` | — | narrow to **request** trailers (§1.13) |

**Provably outside**, each owed one explicit non-change sentence **and** a behavioural regression test: `writeH1Reply` (`codec.go:74`, 4 args) · `writeH3Reply` (`h3dispatch.go:33`, 4 args) · `directResponseAction.writeH2` (`actions.go:135/:138`, **test-only-reachable**) · `write500H2` (`h2dispatch.go:723`) · `actions.go` and `internal/statssink` (byte-untouched).

**Test files:** `internal/filter/hcm/h2/client_test.go` (+ the ~18-line `writeResponseWithTrailers` peer method) · a new `internal/filter/hcm/h2/trailers_validate_test.go` · `internal/filter/hcm/h2dispatch_test.go` (**reuse `captureH2Writer`**) · `internal/filter/http/router/` (+ the ~50-line **separate** `runH2TrailerBackend`) · `internal/filter/hcm/codec_test.go` and `h3dispatch_test.go` for the non-change pair.

---

## Task 1 — PROGRESS scaffold, baselines, and the RED anchor **outside the harness**, RED FIRST

Record the four baselines of §1.14 by **running them**, not by copying this PLAN. Then stand up the grpc-go probe of §1.2 as a committed, runnable harness under the phase directory's scratch discipline (it is a *probe*, not a fixture — `0119` is 84.2).

**RED, observed:** `rpc error: code = Internal desc = server closed the stream without sending trailers`, 3/3. **Second arm:** `Unknown desc = ` with an empty message, 3/3. **Invariance control:** plain-H2 GET byte-stable.
**Gate:** all three arms recorded with their exact strings before any production byte changes.

## Task 2 — capture the trailing block into `H2Response.Trailers`, with a **stacked** control

**RED first:** `TestClientConn_RoundTrip_CapturesTrailingHEADERS` — `len(resp.Trailers)==1` ∧ `{grpc-status,0}` ∧ ⚠️ **`grpc-status` did NOT leak into `resp.Headers`**.
⚠️ **AND ITS STACKED CONTROL IN THE SAME TASK:** `TestClientConn_RoundTrip_NoTrailers_TrailersEmpty` — no trailing block ⇒ `len(Trailers)==0`.

⚠️ **THE CONTROL IS NOT OPTIONAL.** Break B6 (capture over-fires: unconditional assignment before the `if`) left **both positive arms GREEN** — the trailing block overwrites the first, so the positive assertion still sees `{grpc-status 0}`. **Only the stacked control caught it** (`reference_positive_arm_cannot_catch_overfiring`, live on this seam).

Needs the ~18-line `writeResponseWithTrailers` peer method (§1.13).

## Task 3 — **D-84-VALIDATE**: extract the shared set, validate at capture time, fail the stream

**Enforced set (§1.4): RFC 9113 §8.2.2 connection-specific fields + `content-length` + pseudo-headers + END_STREAM.** ⚠️ **`host` and `trailer` PASS THROUGH — they are reference-parity controls, not omissions.** ⚠️ **The second-block rule is DROPPED (§1.8).**

**Placement: capture-time in `client.go`'s `else` branch, failing the stream** — the reference books its reject in the **upstream codec** (`rx_messaging_error: 17` / `upstream_rq_tx_reset: 17`) and the downstream consequence is a reset, never a cleaned 200. Emit-time stripping is a **guaranteed 84.2 divergence** and would run on every local-reply/direct-response path for nothing.

**Surface it as `h2.NewStreamError(h2.ErrInternalError, …)`** so `serverStream.dispatch` (`stream.go:308-315`) emits `RST_STREAM(INTERNAL_ERROR)` downstream — **~6 lines reusing the ctx-cancel pattern** — **not** the default 502, and ⚠️ **do NOT `EvictH2ConnOnError`**: the reference resets the *stream*, not the conn.

**Two tables, `t.Errorf` per property.** Table A (17 direct cases): the legal-`grpc-status` **positive control**; empty block; `host_passes`/`trailer_passes`/`te_trailers_passes`; `no_end_stream`; `pseudo_status`/`pseudo_path`; `content_length`; **one case per connection-specific member**, not one for the set; `te_gzip`; and `content_length_not_first` (asserts the loop scans the whole set, not just `[0]`). Table B (5 wire cases) — ⚠️ **THE LIVENESS GATE**: break A (call site neutered, validator intact) reddens **Table B only** while Table A stays fully green. ⚠️ **Give the `no_end_stream` wire case a ~500 ms ctx** — when broken it fails by *hanging*.

⚠️ **Break C exposed a pre-existing gap: removing `"upgrade"` from the shared set reddened ONLY the new trailers case — zero of the 77 `hcm/h2` test functions cover it on the request side.** Task 13 closes it.

**Priced: +94/−19 production, ~230 test — LOWER BOUND.**

## Task 4 — `ActionResponse.Trailers` carrier

Additive; no compile break (`go build ./...` + `go vet ./...` rc=0). Carries the codec-scoped comment on the `Close` precedent (`router.go:83-88`). **RED first** via the router-layer assertion; **`hcm/h2` must stay GREEN** — break B2 proves layer isolation.

## Task 5 — populate in `doH2ClusterAction`, and prove the other 17 sites do not

**One populate site** (`router_h2.go:202`). Assert the six local-5xx returns in the same file stay nil, and that `retryExecutorH2`/`hedgeExecutorH2`/`router_weighted.go` propagate by value with no edit. **Stacked no-trailers control**, per Task 2's reasoning.
Needs the ~50-line **separate** `runH2TrailerBackend` (§1.13) — do not add an arm to `runH2Backend`.

## Task 6 — `writeH2Reply`: conditional END_STREAM + the trailing emit + **all three callers**

Signature gains the trailer block. **`:311` and `:527` pass nil; only `:602` passes `resp.Trailers`.** Two of the three **broke the build** until updated — budget the call-site fan-out (`reference_change_set_measure_not_build_measure`: a file-scoped measure misses them).
Rewrite the doc comment at `:669-670`, which states the rule the row breaks **verbatim**.

## Task 7 — ⚠️ **THE 4-CELL FRAME-SEQUENCE MATRIX — the gate that makes D-84-ENDSTREAM MEASURABLE**

**Reuse `captureH2Writer`** (`h2dispatch_test.go:39`); do **not** widen `captureSW`. Assert the ordered tuple `(frame, end_stream)` over `{trailers, no trailers} × {body, no body}`:

| cell | expected `order` / `endStream` |
|---|---|
| trailers + body | `[headers data headers]` / `[false false true]` |
| ⚠️ **trailers + no body** | `[headers headers]` / `[false true]` |
| no trailers + body | `[headers data]` / `[false true]` |
| no trailers + no body | `[headers]` / `[true]` |

⚠️ **THE BODYLESS-WITH-TRAILERS CELL IS LOAD-BEARING AND MUST NOT BE DROPPED.** Break B4 — `endStream = len(body)==0` restored **as a separate statement in the same function**, isolated from the emit arm — **PASSES against a with-body-only test**, because `len(body)==0` is `false` either way when a body exists. **Only the bodyless cell discriminates.** A PLAN whose emit test covers only the with-body shape has an **un-reddenable B4** (`reference_vacuous_break_modes`).

This task is also the **only** unit-layer guard on the conditional-vs-unconditional choice (§1.3).

## Task 8 — ⚠️ **What the encode chain is TOLD** (`h2dispatch.go:575` / `:582`) — decide and land

Measured under variant A: the chain is told `EncodeHeaders(false) → EncodeData(true)` while a trailing block still follows (§1.10). **Choose one, explicitly:**

- **(i) accept and document** — write the divergence into `ADR-0306` as a **named non-goal**, with the measured probe line as evidence. Cost ~0 `.go`, plus ADR prose.
- **(ii) fix** — thread the trailer presence into both call sites. Cost ~5–20 production lines + 1–2 test functions.

**Recommendation: (ii).** A bodyless-response-with-trailers (reachable when a gRPC error is raised *after* headers) tells an encode filter `endStream=true` with a block still to come — a **filter-visible** contract break, and 84.1 is the row that creates the reachable path. **Whichever is chosen, the PLAN forbids leaving it implicit.**

## Task 9 — `write500H2` and `directResponseAction.writeH2`: one sentence each

`write500H2` — defensive non-router terminal, unreachable under `ValidateChainShape`; **no upstream ⇒ no trailers**. `directResponseAction.writeH2` — ⚠️ **"test-only-reachable"**: `\.writeH2(` has exactly one caller tree-wide, `actions_test.go:86` (§1.7). While here, fix `h2dispatch.go:117`'s false comment.

## Task 10 — H1 **behavioural** non-change regression

With an `ActionResponse` carrying non-empty `Trailers`, `writeH1Reply` must produce bytes **identical** to the same response with `Trailers` nil. ⚠️ **Assert the FULL serialized bytes, not a header subset** — a chunked trailer section appears at the *tail*, which a header-map assertion never reaches. Signature stays 4-arg (`codec.go:74`).

## Task 11 — H3 **behavioural** non-change regression

Same over `writeH3Reply`'s `http.ResponseWriter`: no `Trailer`/`Trailer:`-prefixed key gained, body byte-equal. Signature stays 4-arg (`h3dispatch.go:33`); `SetH2Action` has **one** non-test call site (`h2dispatch.go:402`).

⚠️ **TASKS 10 AND 11 MUST SHIP IN THE SAME FILE AS THE H2 POSITIVE ARM, OR THE PAIR IS ONE-SIDED AGAIN.** An H1 "no trailers emitted" test is green both when the field is correctly ignored **and** when `Trailers` is never populated at all. The H2 arm supplies the failing baseline (`reference_one_sided_gate_for_a_two_sided_fix`, `reference_liveness_break_needs_failing_baseline`).

## Task 12 — the falsified-comment reconciliation, five production files

`h2/client.go:440` · `h2dispatch.go:502` · `chain.go:483` · `router_h2.go:253-257` (**three** falsified clauses, including *"never via a trailing HEADERS frame"*) · `h2dispatch.go:669-670`. ⚠️ **`jwt/doc.go:97` is an unrelated citation — do NOT touch it.** ⚠️ **Do NOT "fix" `router/router.go:284-285`** — it papers over the fact that `Run*Trailers` is dead production code (SPEC-inherited standing instruction).

## Task 13 — close the `buildRequest` per-member gap break C exposed

Per-member request-path cases for `connection`/`keep-alive`/`proxy-connection`/`transfer-encoding`/`upgrade`/`te`. **Without them the extraction refactor of Task 3 is ungated on the request side** — currently **zero** of `hcm/h2`'s 77 test functions cover `upgrade` there. Alternatively state the gap explicitly; **do not leave it unnamed.**

## Task 14 — the break roster, **re-derived at the IMPL tip**

Seven arms are already **proven reddenable** at `c470cf03` (§5). Re-derive, do not carry. **`-count=1`; confirm WHICH assertion fired; sha256-restore; expect 1–2 arms to be un-reddenable and NAME them rather than report them green.**

## Task 15 — `ADR-0306` §Decision + §Consequences, **and the ¶4 correction**

Append **IN PLACE** after the retained italic footer at `:17956`. **No renumber. NO `---` separator** (`^---$` stays 216, last at `:17020`). Footer **RETAINED** (the ADR-0302..0305 pattern; it sits at the END of §Context, before `### Decision`). STATUS flips `PROPOSED` → `COMPLETE`, **disarming the strict guard 1 → 0, which is correct**. Target block 29 → **62–72** lines ⇒ `DECISIONS.md` **+33…+43, zero deletions**.

**§Decision must dispose of all eleven §Context paragraphs.** **§Consequences must carry** the §6.1 crossing, the five reconciled contract statements, the §13 deferrals, and that **84.2's IMPL flips row 84 `done`**.
⚠️ **Plus the ¶4 in-place correction: "ten" → "nine" slash-form selectors** (§1.6), **leading with what survives** (the defect, the 42 unrun cases and the arithmetic are all unaffected) — the ADR-0297/0298 §Context-correction precedent.
**Invariants after:** headings stay **305** · ids 0001-0306, **one gap at 0209**, zero duplicates · **next-free from the TAIL = ADR-0307** (⚠️ headings+1 = 0306 **COLLIDES — do not "fix"**) · STATUS census 19 · retained footers 12 (⚠️ **non-contiguous — phase 79 has none**). ⚠️ **Carry no whole-file count of the loose PROPOSED matcher** — it is all false positives and completing this block adds one more rather than restoring any prior figure.

## Task 16 — `BEHAVIOR_CONTRACT.md`: five **in-place net-zero** rewrites + **one tail append**

⚠️ **ADR-0052 `:1821` makes `ADR-0306` the MANDATED vehicle** — verbatim: *"Future phases that extend the H2 equivalence surface (e.g., trailers in phase 07, **gRPC in a gRPC-family phase**) add sub-sections here … **via a new ADR, not by editing 05.1's `### Not asserted` block silently**."* The block it protects is `:2037`, which is exactly where statement 3 lives.

| # | line | **stable phrase anchor** | edit |
|---|---|---|---|
| 1 | `:16` | `\| Response trailers \|` | in-place net-zero — an asserted equivalence that is today **unreachable**; 84.1 makes it reachable on the H2 downstream path only. ⚠️ **An insert here shifts 196 of 197 cites** |
| 2 | `:682` | `- Trailers in access logs` | in-place net-zero — re-point a deferral whose target family has now **opened** |
| 3 | `:2043` | `- Trailers — observed but not forwarded` | in-place net-zero — the **upstream-side** discard rule is falsified; the server-side clause survives |
| 4 | `:2068` | `- Trailer forwarding (deferred` | in-place net-zero — **narrow** to request trailers + H1/H3; do not delete |
| 5 | ⚠️ **`:4293` only** | `Trailers are structurally invisible` | in-place net-zero — narrow *"structurally"* to *"to the HTTP **filter chain**"*. ⚠️ **`:4295` opens a DIFFERENT subsection and `:4311` stays TRUE — DO NOT EDIT IT** |

**Plus one tail append** for the new `## HTTP/2` trailer-forwarding subsection. **A tail append shifts 0 of 273 cites; an insert at `:2043`/`:2068` shifts 29 of 273.** Expected delta **+55…+61** — ⚠️ **not the +21 routine median**: the two rows that performed actual *reconciliation* (79 at +61/−1, 80 at +55/−9) are both at the top of the range.
⚠️ **Use the BACKTICKED phrase anchor for `:4291`** — the SPEC's rendering finds nothing (§1.12).

## Task 17 — stat surface **+0**, by call-site enumeration

⚠️ **DO NOT DISCHARGE VIA `TestNoNewStat*`** — proven blind by execution (§1.14). Prescribed gate, both arms, **with the input measure asserted** (ARM 1 on a clean tree is vacuous):

```sh
# ARM 1 — added PRODUCTION registration sites in the row's diff. MUST print NOTHING.
git diff --unified=0 "$BASE"..HEAD -- '*.go' ':!*_test.go' \
  | grep -E '^\+[^+]' | grep -E '\.New(Counter|Gauge)(IfAbsent)?\('
git diff --unified=0 "$BASE"..HEAD -- '*.go' ':!*_test.go' | wc -l   # INPUT MEASURE, MUST be > 0
# ARM 2 — production census invariant. MUST print 208 then 36.
git grep -nE '\.New(Counter|Gauge)(IfAbsent)?\(' -- '*.go' ':!*_test.go' \
  | grep -vE ':[0-9]+:[[:space:]]*//' | wc -l
git grep -lE '\.New(Counter|Gauge)(IfAbsent)?\(' -- '*.go' ':!*_test.go' | wc -l
```

⚠️ **The +0 is a FILE-level claim** — the five touched files register **0** each, while the three touched *packages* register 5 in total (all in other `internal/filter/hcm/*.go` files). **Say so.** ⚠️ **Cite 208/36, never 208/84.** ⚠️ **Only the DELTA is asserted** — the absolute (**1207**, from `BEHAVIOR_CONTRACT.md`'s ledger tail `:5118`, ⚠️ **never** `STATE.md` §Project's stale 1205) is DOC-SOURCED. **No phase from 78 through 84 has added a ledger row; a +0 states itself in the row's own subsection.**

## Task 18 — gates (a)–(f), departures **named** rather than compliance claimed

See §10.

## Task 19 — the full gate run

`go test ./... -count=1` with `INNER_EXIT` · `-race` as a **second** run · the 120-fixture differential in the prescribed 4-arm form with `INNER_EXIT` · `go vet` · `golangci-lint run` · `gofmt -l`. ⚠️ **`go test ./test/differential/...` (with `...`) matches TWO packages and BUFFERS `-v`** — an empty log is normal, do not kill it. ⚠️ **Subtest `--- PASS` lines flush only when the PARENT completes**; watch `=== RUN`.

## Task 20 — stage close

`STATE.md` §Current updated **IN PLACE** (all seven fields; lifecycle-state **3 -> DONE** for this leg's IMPL, `next-skill` → `PLAN-84.2.md`) · §Recent demotion + the eviction (§8) · `PROGRESS.md` third section · `next-prompt.txt` roll (⚠️ **`git add -f`** — tracked but gitignored) · `ROADMAP.md` row 84 **STAYS `in-progress`** with `sub-phases` prose updated at the IMPL, per rows 60/61.

---

## 5. The break roster — **SEVEN ARMS PROVEN RED AT THIS TIP; TWO NAMED VACUOUS**

Every arm below was **injected against a minimal working patch**, `-count=1`, injection-site occurrence asserted `== 1` before writing, restoration sha256-verified across all touched files.

| arm | injection | **what reddened** | collateral |
|---|---|---|---|
| **B1a** | drop the capture (`cs.respTrailers = decoded` → `_ = decoded`) | `h2` capture test **and** `router` carrier test | hcm PASS; both stacked controls PASS; H1 non-change PASS |
| **B1b** | drop the capture — ⚠️ **SITE VARIED** (the `RoundTrip` return instead) | **identical pair** | identical |
| **B2** | drop the carrier (`Trailers: resp.Trailers,`) | `router` carrier test only | ⚠️ **`hcm/h2` PASS — proves layer isolation** |
| **B3** | drop the emit | `order = [headers data], want [headers data headers]` **and** the bodyless cell | h2 + router PASS; non-change + H1 PASS |
| **B4** | `endStream = len(body)==0` restored **as a separate statement in the same function** | ⚠️ **ONLY the bodyless cell** — `endStream = [true true], want [false true]` | ⚠️ **VACUOUS against a with-body-only test** |
| **B5** | emit trailers with END_STREAM off | both emit cells | h2 + router PASS |
| **B6** | capture **OVER-FIRES** | ⚠️ **ONLY the stacked controls** — `len(Trailers) = 1, want 0 (got [":status"="200"])` | ⚠️ **both positive arms stayed GREEN** |

Plus five validation arms (A–E) proven at §1.5/§1.8: the **call-site-neutered** arm is the decisive liveness break — it reddens the **wire** table while the **direct** table stays fully green.

⚠️ **NAMED AS VACUOUS OR NON-LOCALISING, NOT REPORTED GREEN:**
1. **B4 against a with-body-only emit test** — un-reddenable. **The bodyless cell is mandatory.**
2. **B6 against the positive arms alone** — over-firing ships. **The stacked controls are mandatory.**
3. **B1a and B1b are indistinguishable at the test layer** (same two assertions, same text). Site variation confirms the test is **site-agnostic**; it does **not** localise the defect. Acceptable, but stated.
4. **The `alreadySeen` leg is unreachable in production and is DROPPED, not gated** (§1.8, shape 35).

**NOT INJECTED — deferred to 84.2 because they are cross-side:** the stats **vacuity** control (the stats legs must stay green under every arm, proving shape 31 live), the **symmetric** control (same wrong value on both sides, must PASS), and the **liveness** arm with a failing baseline.

---

## 6. Differential and fixture posture — **84.1 ADDS NO FIXTURE, AND (b) DOES (a)'s WORK**

84.1 adds no fixture; `0119-grpc-unary-trailers` at reference port **10119** on `GRPCHealthResponder` **BackendKind 34** is 84.2. But **(a) is not the strong kind of vacuous**: 84.1 changes the H2 response wire path, and §1.3's counterfactual shows **the differential is the only layer that would see an unconditional-END_STREAM regression**. **State it that way: (a) is vacuous and (b) is doing (a)'s work.**

⚠️ **A stats-only assertion is VACUOUS ON BOTH SIDES** (SPEC §10, broken-gate shape 31): the subject books `upstream_rq_2xx: 2` after two **failed** RPCs, and the pinned reference's moved-name set is **identical** with and without trailers (44 names of 375, symmetric difference ∅). A3's reference probe **restates it independently**: the reference books `upstream_rq_200`/`downstream_rq_2xx` **even for the streams it RESETS** (11 rq / 7 rejects, both counted 200). **Stats cannot discriminate a correct tree from a broken one on either side.**

**Two frame-sequence parity notes 84.2 will need, measured here so it does not re-measure them:**
1. On an **empty** trailer block the reference emits an extra `DATA len=0 END_STREAM`; a `len(trailers)==0 ⇒ END_STREAM on the body DATA` implementation emits **one frame fewer** — semantically identical, **frame-sequence divergent**.
2. ⚠️ **The response HEADER path is already an unvalidated conduit today.** The reference **502s** a response *header* block carrying `connection`/`transfer-encoding`/`keep-alive`; `router_h2.go:195-201` strips **only** pseudo-headers and `writeH2Reply` re-emits everything lowercased. **Row 84 must not be credited with fixing it, and 84.2 must not stumble into it.**

**Flake register (still live):** the driver-receiver port race (behavioural roster **42**; **phase 84 is outside the class** — `GRPCHealthResponder` is runner-spawned on a kernel-ephemeral port) · `reference_sds_init_fetch_timeout_dial_budget_flake` (**two** packages) · the `internal/cluster` `-race` outlier · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`. **Check `git worktree list` and `ps` for sibling sessions before blaming your own row.**

---

## 7. Band — **≈1103 floor / ≈2280 BUDGET / ≈3376 ceiling, TWENTY tasks. §6.1 CROSSES. RECORDED, NO FURTHER SPLIT.**

**The §6.1 LoC trigger crosses at the budget** (§1.1). **The ~25-task trigger does not fire** (20, ~20% margin). **The mid-execution trigger is at risk** on Tasks 3 and 7.

**NO FURTHER SPLIT — the SPEC's axis analysis was re-read and re-checked, and all three rejections hold:**

| axis | verdict |
|---|---|
| **B.** capture upstream / emit downstream | **REJECT** — 84.1a alone would be dead code (captured, never emitted): §6.3's *"incomplete stubs the differential can't exercise."* ⚠️ **And it is now WORSE than the SPEC knew**: A3 proved a capture-without-validation patch is an active **smuggling conduit** (§1.9), so shipping the capture half alone would land a security regression as a milestone. |
| **C.** docs/ADR as a leg | **REJECT** — not a row. |
| **D.** fold in the CONTINUATION defect | Not a split; already a §13 deferral. |
| **E.** validation as its own leg *(newly considered here)* | **REJECT** — same reason as B, from the other side: the emit half without validation is the conduit. **Capture, validate and emit are coupled by a security constraint, not merely by convenience.** |

⚠️ **THIS IS THE FOURTH CONSECUTIVE §6.1 CROSSING AND THE FIFTH CONSECUTIVE FIRING OF `reference_measured_prototype_is_a_lower_bound`.** The SPEC's diagnosis — **under-ENUMERATION, not under-SCALING** — is upheld and extended: of the ten under-enumeration categories, **nine apply to 84.1** and only "arm-count" is discharged. **The budget above was built from LANDED buckets (per-file production density, per-test-function density), not from a multiplied prototype — so the 1.81–3.07× lineage overrun multiplier must NOT be stacked on top of it.**

---

## 8. Sentinel — re-run **MECHANICALLY** at this stage. It does **NOT** fire; `stop` was **NOT** created

Input measured **BEFORE** anything was written — **234 lines / 116 data rows** — so an empty result could not read as a zero result.

| check | result at `c470cf03` |
|---|---|
| **(1)** | **`NOT DONE: row 84`** at `want=116`, denominator printed (`examined 116 data rows`) — **CORRECT while the phase is open** |
| **(2)** | **SIX** — `:194 :200 :206 :216 :222 :230` |
| **(3)** | **SILENT** |

⇒ the condition is a **CONJUNCTION**; (1) and (2) both print, so **the sentinel does NOT fire.** `ls stop` → `No such file or directory`.

**FIVE NEGATIVE CONTROLS, ALL FIRED.** Row 62 doctored ⇒ `NOT DONE: row 62` **and** `NOT DONE: row 84`, with **`NC LANDED? [ in-progress ]` inspected before the result was trusted** · `want=115` ⇒ `GATE FAIL: examined 116 data rows, expected 115` · the **mandatory** check-(3) doctoring (residual occurrences confirmed **0** first) ⇒ **`NEVER OPENED: gRPC`** restored, while `WASM`/`HTTP-filters`/`Runtime` correctly stay silent and an invented slug fires · check-(2) **one-arm** strip ⇒ **6 → 5, NOT 6 → 0** · both arms ⇒ **0**.

**Leak check — `ROADMAP.md` is BYTE-UNTOUCHED at this stage**, so every axis is trivially invariant. Baselines carried forward for the IMPL to diff against, **all measured with `--` before the pattern** (the flag trap reads `0=0` and is indistinguishable from "no change"): check-(2) union **6** · `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · **234 lines / 116 data rows**.

⚠️ **THE ONLY REMAINING SENTINEL BLOCKERS ARE CHECK (1) (row 84, which closes at the 84.2 IMPL) AND CHECK (2) (six family backlogs).**

**Archive-absence guard for Task 20's eviction** — cross-product run on **FOUR REAL targets**, because an invented-only probe is non-discriminating and looks like agreement:

| target | raw `grep -cF` | OLD colon form | "tolerant" non-asterisk | **ROBUST** |
|---|---|---|---|---|
| `phase 82 (…) IMPL done` — an ANNOTATED-label entry, PRESENT | **1** | **0 — FALSE ABSENT** | 1 | **1** ✅ |
| ⚠️ `phase 82 (…) SPEC done` — annotated, PRESENT | **1** | **0 — FALSE ABSENT** | **0 — FALSE ABSENT** | **1** ✅ |
| **`phase 83 (…) BRAINSTORM done` — THIS STAGE'S EVICTION TARGET** | **0** | 0 | 0 | **0** ✅ |
| an invented target | 0 | 0 ✅ | 0 ✅ | **0** ✅ |

⚠️ **BOTH the colon form AND the "tolerant" non-asterisk form are demonstrated FAIL-UNSAFE at this tip, on real present entries.** Coverage over the **179** `- **prior active-phase` bullets: colon sees **163** (blind to all 16 annotated), tolerant sees **176**, **ROBUST sees 178**. Cause: **7 of 16** annotated labels carry a literal `*`, defeating `[^*]*`; a `[^:]*` variant is worse (**13 of 16** carry an extra `:`). **ROBUST does not over-fire**: a 16-occurrence prose string returns **0** under it, because the target must be backtick-quoted on a bullet line.
⚠️ **DESCRIBE the guard's regex in the eviction annotation; do NOT spell it** — an entry quoting the pattern defeats its own character class and self-clears.
**`STATE_HISTORY.md` 458 → 460, strictly append-only** (`git diff --numstat` must show `N 0`, and the base file must be a byte-exact **PREFIX**).

---

## 9. Counts at this tip — re-derived mechanically, each with a control

**fixtures 120** (next-free `0119`, reference port **10119**; ⚠️ the faithful predicate is `^[0-9]{4}[a-z]?-` — a bare `^[0-9]{4}-` gives 118) · **fuzzers 55** in **48** files, ⚠️ **all under `internal/`, ZERO under `test/`** (`-- '*.go'`-scoped; unrestricted 161, delta 106 across 40 files that are **100% Markdown**) · **phase dirs 125**, of which **37** carry `REVIEW.md` and **88** do not (⚠️ **not 124 — the phase falsified its own count with its own commit again**) · **BackendKind tail 38** over **39** declarations (⚠️ do NOT "fix" 38 to 39) · `DECISIONS.md` **17956** / **305** `^## ADR-` headings / ids **0001-0306** with **exactly one gap at ADR-0209** and zero duplicates / tail **ADR-0306 `PROPOSED`** / next-free **ADR-0307 from the TAIL** (⚠️ headings+1 **COLLIDES**) / `^---$` **216**, last at `:17020` / STATUS census **19** / retained italic footers **12**, ⚠️ **non-contiguous (phase 79 has none)** / **strict `PROPOSED` guard 1 — ARMED, and correct** · `ROADMAP.md` **234 lines / 116 data rows** · `STATE.md` **64** · `STATE_HISTORY.md` **458** · `BEHAVIOR_CONTRACT.md` **5900** · **stat surface 1207**, ⚠️ **DOC-SOURCED from the contract's ledger tail `:5118` — NEVER from `STATE.md` §Project's stale 1205; only the DELTA is asserted** · production stat call sites **208 / 36 files** · `ActionResponse` **19 files / 22 sites (18 non-test)** · `H2Response` **2 files / 15 lines** · test functions **211/77/68** (`Test|Fuzz|Benchmark`) or **210/75/68** (`Test` only) — ⚠️ **state the matcher** · `BEHAVIOR_CONTRACT.md:NNNN` cites **197 / 71 files** + `BC:NNNN` **76** = **273**.

⚠️ **STILL CONTESTED, SO NO NUMBER IS CARRIED:** the `STATE_HISTORY.md` archive-gap total · production `stats.IsValidName` guard sites · the `ROADMAP.md:<line>` cite count · `allCallbacksNoOp` occurrences · **the loose `PROPOSED` matcher's whole-file count** (all false positives; completing ADR-0306 adds one more rather than restoring any prior figure).

---

## 10. Gates — a docs-only PLAN owes (a)–(f) as a **POSTURE STATEMENT**, not a green

**(a)** — **not exercised** (docs-only, zero `.go`). At the 84.1 IMPL: **VACUOUS but not the strong kind** (§6) — 84.1 adds no fixture, yet the differential is the only layer that sees an END_STREAM regression.
**(b)** — **not exercised here; baseline ESTABLISHED**: **120/120, `INNER_EXIT=0`, 388.78 s**, all four arms clean. Owed in full at the IMPL.
**(c)** — ⚠️ **`test/conformance/grpc/` NOT BUILT, deferred by name** (SPEC §4, two independent grounds). Existing suites **ASSERTED-UNAFFECTED**: proxy-wasm **10 of the cpp-host's 16 families (62.5%), 6 deferred**; h2spec **53/53** ⚠️ **stated WITH the scope caveat — the gate has never run ANY of RFC 9113 §6; the correct selector yields 42 tests / 4 pre-existing failures; NINE (not ten) slash-form selectors each match zero cases; and the gate is not run in CI at all.** **This figure is NOT evidence about frame-level behaviour.**
**(d)** — ⚠️ **VACUOUS, and the word is "vacuous", not "green".** §7.4 binds a phase introducing a *parser, codec, or filter*; 84.1 introduces none — the trailing-HEADERS HPACK decode **already runs** at `client.go:425`, the discard is at `:440`, the frame goes through already-landed code, and no filter is registered. Precedent: **0 of 7** phases (77–83) added one.
**(e)** — not exercised here; owed in full at the IMPL. ⚠️ **`INNER_EXIT` is mandatory for (e), not just (b)** — the abort lands under `go test ./...`.
**(f)** — ⚠️ **STANDING LINEAGE DEPARTURE, named not claimed.** No `REVIEW.md`; **37 of 125** phase dirs carry one and **none since 25.3**.

**Stat surface: +0 anticipated**, discharged by **call-site enumeration** (Task 17), ⚠️ **never by `TestNoNewStat*`**, proven blind by execution this stage.

---

## 11. Deferred — named so no later stage re-derives them

1. ⚠️ **The h2spec selector defect** — **NINE** malformed strings, 42 unrun cases, 4 pre-existing envoy-go failures behind them, the missing `tests == 0` guard, `ADR-0051` §2's false scope claim, and the fact that the gate is **not run in CI at all**. **One coherent future row.**
2. **The CONTINUATION decode discard** (`h2/conn.go:255-259`) and **the CONTINUATION encode hole** (`encodeAndWriteHeaders` never splits at the peer's `MAX_FRAME_SIZE`) — ⚠️ **row 84 ADDS A CALL SITE to the encode-side defect. A stated non-goal in `ADR-0306`, not silence.** Exposure begins at ~16 KB of trailer metadata; a successful unary trailer set is tens of bytes.
3. **H1→H2 = 502** — `UseH2` consulted nowhere in HCM. Forecloses `grpc_http1_bridge` and browser `grpc_web`.
4. **Full response buffering** — structural `[]byte` at three layers. Unary provably unaffected at interop size.
5. **`test/conformance/grpc/`** and the eight unregistered gRPC filter type URLs.
6. **The dead `RunEncodeTrailers` hook** — deliberately NOT wired.
7. ⚠️ **`internal/filter/hcm/h2`'s client response path has ZERO fuzz coverage** (both h2 fuzzers are server-side), and ⚠️ **all 55 fuzzers live under `internal/`, none under `test/`** — a 100% violation of §7.4's own location clause.
8. ⚠️ **The response HEADER path is an unvalidated conduit today** (§6) — newly named here.
9. **The stale `STATE.md` §Project block** and `harness_test.go:208`'s false port inventory — recorded, not fixed.

---

## 12. Gate hygiene — the broken-gate count is **THIRTY-FIVE**; TWO new shapes landed here

- ⚠️ **SHAPE 34 — A NEGATIVE CONTROL RUN WITH A BROADER PATTERN THAN THE GATE IT CONTROLS** (§1.7). It fires, and the firing reads as proof the gate discriminates; but the NC never exercised the gate's actual selector. **A firing NC is not evidence unless it fires on the SAME pattern.**
- ⚠️ **SHAPE 35 — AN ORDERED VALIDATION LEG THAT NO WIRE SEQUENCE CAN REACH** (§1.8). An earlier leg intercepts every malformed path and the legal path returns before the leg's input arrives; the table entry reads as coverage. **Distinct from a shadowed leg: this one is unreachable in PRODUCTION, not merely in the test.**

**Re-confirmed live this stage, each on the row's own seam:** `reference_positive_arm_cannot_catch_overfiring` (B6 — both positive arms green under an over-firing capture) · `reference_vacuous_break_modes` (B4 — un-reddenable against a with-body-only test) · `reference_deliberate_break_wrong_assertion` (the second-block probe reported the right rejection via the **wrong** assertion) · `reference_probe_input_is_a_claim` (the `/te` prefix-dispatch bug that silently skipped a case while printing a plausible result) · `reference_measured_prototype_is_a_lower_bound` (fifth consecutive row) · `reference_change_set_measure_not_build_measure` (third consecutive document) · `reference_harness_exit_code_is_not_command_exit_code` (`INNER_EXIT=1` under `go test ./...`) · `reference_branchpoint_roster_stale_midrow` (37 of **125**, not 124) · `reference_golangci_misspell_locale_us` (fired on an agent's own prose).

---

## 13. Self-review against the SPEC

- **Every SPEC §17 owed item is disposed** (§2), and the three items the SPEC leaves undecided are **ruled**, not inherited.
- **Nothing in this PLAN was accepted from the SPEC without re-derivation.** Thirty-one refutations; the SPEC's own §12.1 budget table is corrected in three cells; two of its line cites and one of its phrase anchors are corrected; one of its negative controls is shown not to control its gate; and **a number in the ADR the SPEC landed is wrong**.
- **`ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` are BYTE-UNTOUCHED** — 0 of 7 recent PLANs touched any of them, verified with a firing NC on the seven IMPLs and the seven SPECs.
- ⚠️ **What this PLAN does NOT gate, stated rather than hidden:** the encode-chain END_STREAM signal if Task 8 takes disposition (i); the `buildRequest` per-member request-path coverage if Task 13 is descoped; and **the conditional-vs-unconditional choice at the differential layer**, which is 84.2's to prove.

---

## 14. NEXT

**The 84.1 IMPL** — the twenty-task TDD spine above, subagent-driven, in worktrees off the then-current master tip. It lands the five-file seam, D-84-VALIDATE, the 4-cell frame matrix, the H1/H3 behavioural non-change pair, `ADR-0306`'s completion **with its ¶4 correction**, and the `BEHAVIOR_CONTRACT` reconciliation. **Row 84 STAYS `in-progress`.**

**Then `PLAN-84.2.md`** — the differential fixture `0119-grpc-unary-trailers` at reference port **10119** on `GRPCHealthResponder` **BackendKind 34**. ⚠️ **The FINAL leg: its IMPL flips ROADMAP row 84 `done`**, at which point **check (2) — six family backlogs — is the sole thing standing between this project and the termination sentinel.**
