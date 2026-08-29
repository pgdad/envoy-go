# Phase 93 — `h2-local-reply-content-length` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state **DONE -> 1**). **Opened by SELF-PICK** per the 2026-07-12 standing directive; the roller did not pause for a human pick and there was no banked mid-lifecycle work.

**Tip at open:** `40cf6766` (`git rev-parse master`, not a SHA quoted in any document).

**Row registered:** `93 | h2-local-reply-content-length | 92 | in-progress`, `want` **124 -> 125**, at this BRAINSTORM commit per the §Schema invariant.

---

## 0. What this stage refuted

Every stage owes an execution-refutation of its predecessor. This one produced **eleven**, and — unusually — **three of them refuted the controller's own instructions to its agents, mid-stage**, which is recorded here rather than smoothed over:

1. The banked candidate's **"seven call sites"** cost is the blast radius of the SYMPTOM, not the FIX.
2. The banked **"the H/1 sibling takes a `bodyLen` the H/2 version lacks"** — the parameter is **inert twice over**.
3. The banked framing as an **H/1-vs-H/2 asymmetry** is too small: **five** composers, one outlier.
4. ⚠️ **THE CONTROLLER'S OWN LEADING DESIGN WAS WRONG ON THE RULE** — a body-presence guard would suppress a header the reference demonstrably sends.
5. ⚠️ **AND WRONG ON THE PLACEMENT** — it would re-inject a header the compressor filter deliberately strips.
6. ⚠️ **THE CONTROLLER'S "the reference never produces a body-less local reply" WAS TOO STRONG** — true of the router 5xx shapes, false in general.
7. The tree contains **two contradictory ratified rules** about `content-length: 0`, and the H/3 writer is the one that disagrees with a measured reference transcript.
8. **`STATE.md` carries TWO frozen regions, both self-labelled live** — one stale by eight lifecycle stages, one by sixteen phases.
9. **A `ROADMAP` row's anticipated counts are never reconciled at row-done** — row 92 still reads a fuzzer count its own IMPL falsified.
10. **A banked LINE ANCHOR into `ROADMAP.md` rots on every row insertion** — `ssl.connection_error`'s window moved `:223 -> :224`.
11. ⚠️ **A STALE TEST CACHE SERVED A VACUOUS GREEN THROUGH THIS EXACT FIX** — with a NEW mechanism, recorded in §7.

---

## 1. The pick, and why it is defensible as "smallest first"

**CHARTER: on the HTTP/2 leg, a locally generated reply emits a `Content-Length`, ALWAYS, valued at the body length** — matching the reference's measured behaviour on every shape it produces (§2) and the tree's own ratified ADR-0085 doctrine (§3.3). The fix lands in **`h2LocalReplyHeaders()`**, deliberately **not** in `writeH2Reply`, to keep the blast radius off the proxied path (§3.4). **MEASURED cost: `+3 / -0` in ONE production file** (§6).

**And because a `content-length` ARITY pin is structurally incapable of seeing the empty-body defect underneath it (§2.4), the fixture's instrument is UPGRADED per side in the same row**, so that defect stays visible instead of being converged away. Landing the header fix without that upgrade would leave a green gate over a live divergence.

### 1.1 Why this candidate

It is the candidate phase 92 banked, and the only banked candidate whose **reference side is a MEASUREMENT rather than a prediction**. It also arrives with a **ready-made gate**: the per-side pins `p92WantRefCLFields = 1` / `p92WantSubjCLFields = 0` already sit in `0004-h2-routing`'s `AssertDistribution`, below the byte compare, under a verbatim relocation ban — and **this stage MEASURED that they redden under the fix**, naming all five arms (§7). Nothing else in the tree pins the behaviour in either direction, so without that pin the row would ship ungated.

It also lands **+0 on every count axis** — no new stat, fixture, fuzzer, BackendKind, package or module (§6.2).

### 1.2 It is an INTERNAL inconsistency before it is a cross-side one

Five paths in this tree compose a locally generated response's header set. **`h2LocalReplyHeaders()` is the only one that omits `Content-Length`:**

| composer | site | emits it? |
|---|---|---|
| `beginLocalReply` — **every** filter `SendLocalReply` | `internal/filter/http/chain.go:1313` | **YES**, unconditionally, `len(body)` |
| `directResponseAction.body()` | `internal/filter/hcm/actions.go:95` | **YES**, `strconv.Itoa(len(bodyBytes))` |
| `writeH3Reply` (HTTP/3) | `internal/filter/hcm/h3dispatch.go:56` | **YES**, but under a rule §3.3 shows is contested |
| `localReplyHeaders(bodyLen int)` (HTTP/1) | `internal/filter/http/router/router.go:675` | **YES** (its parameter is inert — §3.2) |
| **`h2LocalReplyHeaders()` (HTTP/2)** | `internal/filter/http/router/router_h2.go:294` | ⚠️ **NO** |

There is exactly **one** `SendLocalReply` definition (`chain.go:748`) and it calls `beginLocalReply` on the next line, so all **36** non-test filter files that call it already get a `Content-Length`. **On one H/2 connection, a filter-generated 403 carries one and the router's own 502 does not.** That is a defect without reference to Envoy at all.

### 1.3 Provenance is OUTSIDE every sentinel window — with the one match DISCLOSED, not a bare zero

Deleting the last deferred candidate ENDS THE PROJECT, so the pick must not silently consume one. Every check-(2) window line (`:202 :208 :214 :224 :230 :238`) was searched:

`content-length` **0** · `content_length` **0** · `Content-Length` **0** · `local reply` **0** · `local_reply` **0** · `direct_response` **0** · `h2LocalReplyHeaders` **0** · ⚠️ `local-reply` **1, at `:224`**.

⚠️ **THE ONE MATCH IS DISCLOSED RATHER THAN SMOOTHED**, on the row-92 precedent where the bare token `encode` read 1 and that row's first draft had asserted 0. The `:224` match is *"5 span-capable zero-endpoint **local-reply** emit sites that would still emit `""`"* — the tracing `upstream_cluster` framework gap, about what a **span attribute** records, not what a header block carries. **No window narrows at row-done.**

**The instrument was positive-controlled before any zero was trusted:** `grpc_web` **2** (`:208`), `RTDS` **5**, `alt-svc` **1** (`:202`), `connection_error` **1** (`:224`). A roster of zeros from an instrument never shown to return non-zero would be worthless.

### 1.4 Scope, stated as a decision

This row fixes the **header**. It deliberately does **not** fix the **empty-body** defect underneath it (§2.3), which spans **both** codec legs and has **no** coverage anywhere in the tree. That is not cramming: the two are separable, the header fix is reference-faithful on its own, and the row **upgrades the instrument** so the body defect becomes visible rather than being hidden by the fix. The body defect is banked in §4.0 with the reference's measured bodies.

---

## 2. The reference side, MEASURED — on the live pinned image, not read out of a code comment

Phase 92 measured the reference's `content-length` arity on **one** shape: a body-bearing 502. `h2LocalReplyHeaders()` serves **three** status classes, so this stage measured all of them.

**Apparatus.** `envoyproxy/envoy:contrib-v1.37.2` in a container named `p93r2-envoy` with its own `-p` publishing (`--network host` is known not to work here), backends on a user-defined bridge, all clusters `STATIC` by IP because a non-resolving `STRICT_DNS` cluster blocks listener init entirely. The probe is a raw-wire Go client holding **one `hpack.Decoder` per connection** with `ReadMetaHeaders` unset, cross-checked against `curl --http2 -D -`.

**The positive control ran FIRST**, because a roster of arities from an instrument never shown to report a non-zero one is worthless. Proxied 200: `FIELD-COUNT: 7`, `content-length: 22`, `BODY-LEN: 22` — arity 1, value == body. Same control on the H/1 leg.

### 2.1 The result — exactly ONE `content-length` on every local reply, always equal to the body

| shape | `:status` | `#content-length` | value | body len | value == body? |
|---|---|---|---|---|---|
| upstream protocol error | **502** | **1** | 87 | 87 | yes |
| malformed response header (the row-92 shape) | **502** | **1** | 87 | 87 | yes |
| upstream connect refused | **503** | **1** | 167 | 167 | yes |
| no healthy upstream | **503** | **1** | 19 | 19 | yes |
| route timeout | **504** | **1** | 24 | 24 | yes |

Identical on the H/1 leg, same values. ⇒ **Phase 92's 502-only finding GENERALIZES to all three status classes, and that is now a measurement.**

Two incidental measurements the SPEC must not re-derive: the reference places `content-length` **first** on a local reply, ahead of `content-type`; and **no `x-envoy-*` field appears on any local reply** (`x-envoy-upstream-service-time` rides only the proxied 200). ⚠️ **Neither is a contract** — `BEHAVIOR_CONTRACT.md:15` requires response headers to be **"Set-equal modulo documented allow-list"**, so the SPEC must **not** chase the reference's field order and must **not** "fix" the helper's documented insertion order.

### 2.2 ⚠️ THE CONTROLLER'S OWN GENERALIZATION FROM THIS TABLE WAS TOO STRONG, AND AN AGENT REFUTED IT

From the table above the controller concluded, and instructed an agent, that *"the reference NEVER produces a body-less local reply, so the question of its arity on one is unanswerable."* **That is true of the router 5xx shapes and FALSE in general.** The refutation is a measured reference transcript **already in this tree** — `BEHAVIOR_CONTRACT.md:2164`, a CORS preflight 200 with an empty body:

```
< date: Fri, 01 May 2026 01:09:51 GMT
< server: envoy
< content-length: 0
```

**On a genuinely empty-body local reply the reference emits `content-length: 0`.** ⇒ **The rule is ALWAYS-EMIT, not emit-when-non-empty**, and the controller's leading design (a `len(body) > 0` guard) would have **suppressed a header the reference demonstrably sends**. ⚠️ **The refutation came from a transcript sitting in the repo the whole time — the docker probe was necessary but was not sufficient, and a wider-scoped one would have found this without it.**

### 2.3 The defect that is left over: the BODY, on both legs

envoy-go's 503/504 local replies send **nothing at all**, against the reference's diagnostic prose:

```
router_h2.go:80   ActionResponse{Status: 503, Headers: h2LocalReplyHeaders(), Body: nil}
router_h2.go:128  ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}          // no Body field
router_h2.go:138  ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}          // no Body field
retry.go:374      ActionResponse{Status: 504, Headers: h2LocalReplyHeaders(), Body: nil}
```

and all six H/1 sites do the same (`router.go:550,580,586,613,626`; `retry.go:288`, each `localReplyHeaders(0)` with `Body: nil`). Only the three H/2 502 sites carry a body at all — `bad502Body = "bad gateway\n"`, **12 bytes**, against the reference's 87.

**Once the header fix lands, envoy-go's header behaviour is reference-faithful everywhere and the ONLY remaining divergence on these paths is the missing body.** That separation is the point of the scope decision in §1.4.

### 2.4 ⚠️ THE INSTRUMENT PHASE 92 CHOSE CANNOT SEE THE DEFECT UNDERNEATH THE ONE IT WAS BUILT FOR

The fixture pins `content-length` **ARITY, NEVER VALUE**, and its README says why: the two 502 bodies differ by construction, so a value pin would be a false gate. **That was the right call for phase 92.** But it has a consequence phase 92 did not state:

**Once H/2 emits a `Content-Length`, arity reads 1 vs 1 on every site — including the four where envoy-go sends a 0-byte body against Envoy's 19- or 24-byte diagnostic. An arity pin is STRUCTURALLY INCAPABLE of seeing a missing body.**

⚠️ **AND THIS IS NOT PROSPECTIVE ON THE H/1 LEG — IT IS THE SITUATION TODAY.** Measured end-to-end through the real `writeH1Reply` with the verbatim `localReplyHeaders(0)` set and a `nil` body:

```
HTTP/1.1 503 Service Unavailable
Content-Type: text/plain
Content-Length: 0
Server: envoy
Date: ...
⇒ H1 content-length ARITY = 1
```

**envoy-go's H/1 local replies already read arity 1 against the reference's arity 1 while sending 0 bytes where the reference sends 19, 24, 87 or 167.** Nothing pins it: the reference's own strings (`no healthy upstream`, `upstream request timeout`, `upstream connect error`) appear in **zero** files under `test/`.

⇒ **The row must not merely flip `p92WantSubjCLFields` 0 -> 1 and call the departure discharged. It must add an instrument that CAN see the body**, or it converts a live, visible divergence into a green gate.

---

## 3. The mechanism, stated precisely — and it is NOT the one that was banked

### 3.1 `writeH2Reply` already recomputes, and never injects

`internal/filter/hcm/h2dispatch.go:1004`. For a field **already present** in the carrier:

```go
ln := strings.ToLower(h.Name)
val := h.Value
if ln == "content-length" {
	val = strconv.Itoa(len(body))          // :1014-1016 — RECOMPUTE
}
```

and it already **injects** two of the three standard fields when absent:

```go
if !hasServer { hf = append(hf, hpack.HeaderField{Name: "server", Value: serverHeader()}) }   // :1025
if !hasDate   { hf = append(hf, hpack.HeaderField{Name: "date",   Value: dateHeader()})   }   // :1028
```

**Measured by execution** (a throwaway overlay probe against the real writer): a deliberate lie `content-length: 999` in the carrier with a 12-byte body came out on the wire as **`content-length: 12`**; a pristine carrier with the same body came out with **no `content-length` at all**.

⇒ **A correct value reaches all seven call sites from a single edit to the helper, with ZERO call-site changes and no `bodyLen` parameter.** The banked *"seven call sites … changes every one of them"* is the blast radius of the SYMPTOM.

⚠️ **THE TRAILER PATH IS DELIBERATELY EXCLUDED AND MUST STAY SO** — `writeH2Reply`'s own doc comment (`:1000-1003`) states that for the trailing block *"no date/server defaults are injected, and Content-Length is not recomputed against it."*

### 3.2 The H/1 sibling is not the role model it was banked as

`localReplyHeaders(bodyLen int)` (`router.go:675`) does emit one — but its parameter is **inert twice over**. All six callers pass the literal `0`:

```
retry.go:288:localReplyHeaders(0)      router.go:580:localReplyHeaders(0)      router.go:613:localReplyHeaders(0)
router.go:550:localReplyHeaders(0)     router.go:586:localReplyHeaders(0)      router.go:626:localReplyHeaders(0)
```

and `writeH1Reply` (`internal/filter/hcm/codec.go:87-89`) overwrites the value from `len(body)` regardless. **What makes H/1 correct is the PRESENCE OF THE FIELD, not the parameter.** Mirroring H/1's signature would import a dead parameter and cost seven call-site edits for nothing — measured as ARM 1 in §6, and strictly dominated.

### 3.3 ⚠️ THE TREE CONTAINS TWO CONTRADICTORY RATIFIED RULES, AND THE H/3 WRITER IS THE ONE THAT LOSES

**Rule A — ADR-0085, ratified, quoted at `DECISIONS.md:8326`:**
> *"The framework's local-reply path still injects the 4-header standard set (`content-length: 0` + `date` + `server: envoy`) per ADR-0085."*

and at `:8260`: *"The 4-header standard set (`content-length: 0` when body empty, …)"*.

**Rule B — `internal/filter/hcm/h3dispatch.go:47-49`, pinned:**
> *"Content-Length is synthesized ONLY when the body is non-empty (an empty-body response gets no Content-Length, never Content-Length: 0)."*

with a compile-time pin at `h3dispatch_test.go:155`: `content-length = %q, want empty (no content-length on empty body)`.

**Rule A is corroborated by a measured reference transcript (§2.2). Rule B is not corroborated by anything.** ⇒ **This row follows Rule A.** ⚠️ **It does NOT "fix" the H/3 leg**, because whether the reference emits `content-length: 0` on an **H/3** empty-body local reply is **UNMEASURED** — the corroborating transcript is an H/1 CORS preflight. **The contradiction is RECORDED and its missing arm NAMED, not resolved by assumption** (§4.0).

### 3.4 ⚠️ WHY THE FIX GOES IN THE HELPER AND NOT IN THE WRITER — A MEASURED HAZARD

Putting the synthesis in `writeH2Reply` looks tidier (it already has the `server`/`date` injection block). **It is not safe.** That writer has three callers:

| caller | what flows through it | effect of a writer-side injection |
|---|---|---|
| `h2dispatch.go:317` | synthesized no-route 404, empty body | none (measured: arity 0) |
| `h2dispatch.go:561` | filter `SendLocalReply` | mostly a fidelity gain |
| **`h2dispatch.go:677`** | ⚠️ **the PROXIED-UPSTREAM path** — upstream headers copied verbatim (`router_h2.go:264-270`) | ⚠️ **a proxied 200 whose upstream carrier lacked `content-length` GAINS one** — measured |

The sharpest case: **the compressor filter DELIBERATELY deletes `Content-Length` on the compress path** — `internal/filter/http/compressor/compressor.go:784`, `headers.Del("Content-Length")`, under a comment saying it *"mirrors Envoy's compressor_filter.cc behavior on the compress path."* A writer-side injection would **re-inject what that filter deliberately stripped**, undoing an Envoy-faithful behaviour and **diverging the H/2 leg from its own H/1 sibling**, which after the strip emits none.

⇒ **The helper is the correct site. Blast radius: the seven router local-reply sites, and nothing else.**

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

`reference_deferred_candidate_cost_restale` says a banked cost goes stale. Every figure below was re-measured at `40cf6766` and carries an explicit **MEASURED / ESTIMATED / UNMEASURABLE-STATICALLY** label. Where a banked figure could not be re-derived it is labelled a **stale prediction**, not restated as a measurement.

### 4.0 ⚠️ NEWLY BANKED BY THIS STAGE — the empty-body defect, on BOTH legs

**The strongest small candidate after this row, and its reference side is a MEASUREMENT.** envoy-go's local replies send a 0-byte body where the reference sends diagnostic prose, on **both** codec legs — ten sites in all (four H/2 body-less at `router_h2.go:80/:128/:138` + `retry.go:374`, and all six H/1 at `router.go:550/:580/:586/:613/:626` + `retry.go:288`) — plus the three H/2 502 sites whose 12-byte `bad502Body` faces the reference's 87.

**Reference bodies, MEASURED (§2.1):** `upstream connect error or disconnect/reset before headers. reset reason: protocol error` (87 B, 502) · the same prose plus `transport failure reason: delayed connect error: Connection refused` (167 B, 503 connect-failure) · `no healthy upstream` (19 B, 503 no-healthy) · `upstream request timeout` (24 B, 504).

**A prototype giving the four body-less H/2 sites bodies was BUILT, RUN and REVERTED: `+20 / -4` across FOUR production files, suite RC=0, 0 FAIL, `gofmt` and `go vet` clean.** Its measured wire output hit the reference's values exactly — 502 -> `content-length: 12`, 503 -> `19`, 504 -> `24`.

⚠️ **Three things a taker must handle, each discovered by that prototype:** (a) the reference's 167-byte string is the **connect-failure** body, **not** the load-shed one, so the three 503 sites do **not** share one string and a single placeholder is a prototype simplification, not a proposal; (b) adding a body changes H/2 **framing** — those 503s emit a **DATA frame** where they previously carried `END_STREAM` on HEADERS; (c) `bytesSent` in the access log moves `0 -> 19`, which is an observable the access-log fixtures may pin. **REJECTED for this row: it spans both legs, changes framing, and owes a `BEHAVIOR_CONTRACT.md` treatment for body composition that this row does not.** **MEASURED.**

**Also banked with it: the ADR-0085-vs-`writeH3Reply` contradiction (§3.3), whose H/3 arm is UNMEASURED.** A taker's first job is to probe whether `contrib-v1.37.2` emits `content-length: 0` on an **H/3** empty-body local reply. Until then neither rule may be "tidied" into the other.

### 4.1 The pooled-upstream-lifetime defect — **STILL THE BIGGEST, STILL DEFERRED, and its hazard GREW this row**

All three sites re-verified PRESENT AND UNFIXED: **D1** ctx ownership (`internal/filter/hcm/h2/client.go:250-251`), **D2** evict-on-own-cancel (`router_h2.go:197` still firing *before* the ctx discrimination; `internal/cluster/h2pool.go:660-670` still ending in a hard `Close()`), **D3** the empty reaper arm (`h2pool.go:733-734`). Corroborated: `git grep -c 'WithoutCancel' -- '*.go'` ⇒ **0**, so the banked *"first use in the tree"* still holds.

⚠️ **THE HAZARD IS LONGER THAN WHEN IT WAS BANKED.** The phase-92 IMPL inserted its new `ErrMalformedResponseHeaders` 502 arm **between** the evict and the ctx check, so D2's ordering window **grew**. A banked defect got worse while nobody was looking at it.

**REJECTED on unchanged grounds that are stronger for being re-measured:** its reference side is still a **PREDICTION** (no docker probe has ever been run against it), it spans **three packages** against this row's one, it is three defects and a probable **SPLIT**, and its H/1 sibling is unmeasured. The banked `+24/-12` is a **reverted-prototype figure not re-derivable from this tree**, and `reference_measured_prototype_is_a_lower_bound` has now fired **five consecutive rows**. **MEASURED (site presence); ESTIMATED (size); reference side UNMEASURABLE-STATICALLY.**

### 4.2 `ssl.connection_error` — **the runner-up, and its cost is now MEASURED**

The strongest **IN-WINDOW** candidate (`ROADMAP.md:224`). ⚠️ **Its banked anchor `:223` is STALE** — row 92's `+1` data row shifted all six windows.

Re-verified: `connection_error` occurs in `*.go` **only inside comments** (4 hits: `internal/listener/manager.go:373/:411/:1292`, `internal/listener/quic_test.go:234`), with no registration and no `Inc`. `classifyHandshakeErr` (`manager.go:444-456`) returns `outcomeOther` at `:455`; the consuming switch at `manager.go:1294-1299` has exactly **two** cases, under a comment saying *"outcomeOther deliberately increments NOTHING — the reference books those under ssl.connection_error, a name this row does not land."*

**Cost, MEASURED against the phase-75 `ssl.no_certificate` precedent:** one struct field (`:187`), one `NewCounter` (`:388`), one `case outcomeOther:` (`:1294`), one Prometheus help-text entry (`internal/stats/name.go:560`) — **~4-6 production lines across 2 files**, with `manager_test.go:4394-4402` already carrying **five** `outcomeOther` rows to assert against.

**REJECTED on three grounds, none of them size.** (a) Its **reference side is a doc claim, not a measurement** — nobody has probed what `contrib-v1.37.2` books under that name. (b) It lands **+1 on the stat surface**, forcing a ledger edit into a ledger **known discontinuous in two places** plus an ADR about a stat name; this row lands **+0 on every axis**. (c) It has **no existing gate**; this row reddens a pin already in the tree. ⚠️ **It is nevertheless the best next IN-WINDOW pick and its cost is now measured.** **MEASURED.**

### 4.3 The `content-length` REWRITE candidate — **its banked leg-scope is REFUTED**

Distinct from this row: that candidate is about `writeH2Reply` **rewriting** an existing `content-length` unconditionally — method-blind and status-blind (`h2dispatch.go:1014`), twin at `codec.go:87`. ADR-0314 (iii) added a fourth leg (a single wrong-valued `content-length`: reference **200-then-RST_STREAM**, subject silent rewrite).

⚠️ **THE BANKED "the identical rewrite exists on ALL THREE codecs" IS REFUTED.** H/1 and H/2 are identical unconditional rewrites; **H/3 is not a rewrite at all** — `h3dispatch.go:55-56` is a *conditional set* that never overwrites an existing value. A taker inheriting "identical" would mis-scope the H/3 leg. **REJECTED as larger and cross-codec.** **MEASURED (sites + the discrepancy); ESTIMATED (size).**

### 4.4 1xx interim responses — **REAL and cheap; the fix is now located precisely**

`respHeadersSeen` is declared at `client.go:199`, read at `:655`, **set unconditionally at `:695`**. The phase-92 restatement at `:729-751` states the corrected target verbatim — the reference **SWALLOWS** 1xx, so the target is **DROP-AND-DELIVER**, needing *"only that a 1xx leading block leave `respHeadersSeen` UNSET."* The `:status` parse already exists at `:697-700` but runs **after** the flag is set, so the fix is an ordering move plus a guard. **~5-15 production lines, one file, one symbol. REJECTED only because this row is smaller and already has its gate.** **MEASURED (site, quote, mechanism); ESTIMATED (size).**

### 4.5 The H/1 no-`Host` divergence — **its headline figure is a STALE PREDICTION**

Site confirmed: `connection.go:155` `bufio.NewReader(downstream)` -> `:163` `http.ReadRequest(br)`, **no `Host` guard between them**; `bytes` already imported at `:5`. But **`+90/-0` is a reverted-prototype prediction, NOT re-derivable** — no such code exists in the tree. **REJECTED: a strictness change (200 -> 400) in the CWE-444 surface owing an ADR and a contract edit this row does not.** **MEASURED (site); the `+90/-0` and `+7` figures are STALE PREDICTIONS.**

### 4.6 The `allocateOTLPPort` harness defect — **its blast radius is 26 FILES, not one**

`0084/driver/driver.go:141` probes `net.Listen("tcp", "127.0.0.1:0")` (loopback, and `:0` draws from the kernel ephemeral range — `/proc/sys/net/ipv4/ip_local_port_range` = `32768 60999`) while `:169` binds `0.0.0.0:%d`; `:167` `panic`s on failure, aborting the whole test binary. It violates **both** properties `test/differential/harness_test.go:318-321` says the 2026-07-30 CI failure taught.

⚠️ **THE BANKED DESCRIPTION UNDERSTATES IT AS ONE SITE. Measured: 26 driver files carry BOTH a `127.0.0.1:0` probe and a `0.0.0.0:%d` bind**, across 21 `allocate*Port()` declarations in 20 files, with 15 bind-failure `panic`s. **REJECTED: a test-harness row touching 26 files, not a behaviour row.** **MEASURED (sites, counts, band evidence); ESTIMATED (fix size).**

### 4.7 Counting the stat surface MECHANICALLY — **BLOCKED, not merely deferred**

Both documentary discontinuities re-confirmed: the **`1198 -> 1200`** step is narrated at `BEHAVIOR_CONTRACT.md:818` but **has no ledger line**, and the **`1200 -> 1201`** step is **accounted nowhere in the file in any form**. The contract says of itself that a re-derived absolute *"should expect the re-derived figure to disagree."* A grep cannot do the job (dynamic `Sprintf` names, conditionally-registered families, the H2-vs-non-H2 dual totals); the honest shapes are an AST walk with a hand-maintained allow-list, or a boot-and-scrape harness. **REJECTED as a full maintenance row.** **MEASURED (both discontinuities); ESTIMATED (build cost).**

### 4.8 The six family windows as a pool — **REJECTED as a pool**

All six name family-scale candidates (upstream H/3 clusters, gRPC streaming, CDS/EDS/LDS/RDS/ADS, the dynamic TLS `ssl` surface, RTDS + hot restart, xDS-sourced dry-validation). None is smaller than the pick, and opening a family is the opposite of "smallest defensible first". ⚠️ **DELETING THE LAST DEFERRED CANDIDATE ENDS THE PROJECT** — check (2) is the only remaining sentinel blocker (§8), so a window is narrowed deliberately and never tidied.

---

## 5. Family attribution

**Core-HCM / HTTP-2-dispatch MAINTENANCE row claiming NO family ordinal** — the row-85/86/87/88/89/90/91/92 precedent. A maintenance row repairs a landed deliverable and does not extend a charter. The deliverable repaired dates from **phase 07.1** (`c5485320`, 2026-05-01, `http-filter-framework`), where `localReplyHeaders` and `h2LocalReplyHeaders` were introduced **in the same commit with the asymmetry already present at birth** — measured by `git log -S`, not inferred.

---

## 6. The cost FLOOR — four prototypes BUILT, RUN and REVERTED

### 6.1 The measured arms

All four: `gofmt -l` **empty**, `go vet ./...` **RC=0**, **zero test files touched**. Baseline at the pristine tip: `go test ./...` **RC=0**, `ok` **127** + `no test files` **109** = **236** packages, `^FAIL` **0**.

| arm | design | `--numstat` | files | suite |
|---|---|---|---|---|
| **1** | `bodyLen` parameter + all 7 call sites (the banked design) | `1 1` retry.go · `9 7` router_h2.go | 2 prod | RC=0 — ⚠️ but see §7 |
| **2** | **always-emit, in the helper** — **THE PICK** | `3 0` router_h2.go | **1 prod** | RC=1 (the pin fired — §7) |
| **3** | body-presence-conditional synthesis in `writeH2Reply` | `8 0` h2dispatch.go | 1 prod | RC=0 |
| **4** | ARM 3 + diagnostic bodies for the four body-less sites | `8 0` h2dispatch.go · `1 1` retry.go · `8 0` router.go · `3 3` router_h2.go | 4 prod | RC=0 |

**ARM 1 is strictly dominated** — two files against one, for an inert parameter (§3.2). **ARM 3 is REJECTED on the rule (§2.2) and on the placement (§3.4).** **ARM 4's body half is banked as §4.0.**

⇒ **THE PICK IS ARM 2: `+3 / -0` in ONE production file, `internal/filter/http/router/router_h2.go`.**

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE, AND `reference_measured_prototype_is_a_lower_bound` HAS NOW FIRED FIVE CONSECUTIVE ROWS — always by UNDER-ENUMERATING FILES.** The floor covers production only. The SPEC must enumerate, by compiling prototype, across **six named gaps** it does not cover: (i) unit coverage, of which the tree currently has **none** (§7); (ii) the fixture pin flip and its NC; (iii) **the new body-visible instrument §2.4 requires**; (iv) the fixture README's prose, including the `:147` sentence this stage refutes; (v) ADR-0315 §Context; (vi) the `BEHAVIOR_CONTRACT.md` rider.

### 6.2 Anticipated counts — every axis RE-DERIVED at this tip

⚠️ **NOT COPIED FROM `STATE.md`, WHOSE §Project counts block is a phase-76-era snapshot presented under a "(Live…)" header** (§9.5).

| axis | MEASURED at `40cf6766` | at row-93 done | NC observed |
|---|---|---|---|
| ROADMAP data rows | **124** | **125** (`want` 124 -> 125) | `want=123` ⇒ one line, `examined 124 … expected 123` |
| phase directories | **133** | **134** (this stage creates one) | no non-dir entries |
| differential fixtures | **121** (tail `0119-grpc-unary-trailers`; **`0120` FREE**) | **121, +0** — `0004` EXTENDED | ⚠️ the known-bad `grep -cE '^[0-9]{4}-'` reads **119**, dropping `0007a`/`0007b` |
| fuzz targets / FILES | ⚠️ **56 / 48** | **56 / 48, +0** | occurrence form agrees at 56 |
| BackendKind **tail value** | **38** (`H2GoawayResponder`); the file declares **39** constants, values 0-38 | **38, +0** | ⚠️ `grep -oE '[A-Za-z]+ +BackendKind = 38'` TRUNCATES the name to `GoawayResponder` |
| `go.mod` requires | **67** (18 direct + 49 indirect) | **67, +0** | ⚠️ the known-bad `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` reads **62** |
| `DECISIONS.md` `^---$` | **216** | **216, +0** — the ADR is APPENDED IN PLACE | — |
| `^## ADR-` / bare `^## ` | **313 / 321** | 313 / 321 now; **314 / 322** once ADR-0315 lands at the SPEC | 313 + 8 `## Amendment` headings = 321 |
| ADR tail / next-free | **ADR-0314 / ADR-0315** | ADR-0315 drafted at the SPEC | ⚠️ TAIL-derived: `grep -c '^## ADR-0315'` ⇒ **0**; a full id sweep finds **exactly one gap, 0209**, so headings+1 = **314 = a TAKEN id** |
| anchored `PROPOSED` guard | **1**, at `DECISIONS.md:14866` under `## ADR-0231` (the decoy) | 1 now; **2** once the SPEC drafts ADR-0315 | ⚠️ **NEVER gate on the unanchored form** — **90 lines / 101 occurrences** |
| stat surface | ⚠️ **NO NUMBER IS QUOTED** | **+0** — the row registers no counter | see below |
| `BEHAVIOR_CONTRACT.md` | **5966 lines** | **+2-4** (a narrative rider; **no** ledger line) | — |
| `STATE_HISTORY.md` strict guard | **163** | **163, DELTA 0** | a wrong-shaped append reads **164**; a correct `(evicted at …)` append reads **163** — both run |
| `-family row` | **95 occurrences / 67 lines** | unchanged | ⚠️ omitting `--` makes grep parse `-f`: `grep: amily row: No such file or directory` |

⚠️ **THE STAT SURFACE IS QUOTED AS A DELTA AND NEVER AS AN ABSOLUTE.** Three different absolutes are live in this tree at this one tip, and the contract warns a mechanical re-derivation *"should expect the re-derived figure to disagree."* Per `reference_a_drift_correction_is_itself_a_claim`, on a contested count: **no number.** This row's claim is **+0**, and it is structural — the row adds no `NewCounter`/`NewGauge` call site.

⚠️ **AND THE FIGURE `406` (or any `406 -> 407` form) MUST NOT BE RESTATED.** It was refuted at the phase-92 PLAN, `grep -c '406' BEHAVIOR_CONTRACT.md` reads **0**, and the phase-92 IMPL deliberately declined to spell it. This document does not spell it either.

---

## 7. The differential measurement — and a VACUOUS GREEN with a new mechanism

### 7.1 The pin reddens, and it names every arm

Under ARM 2 the full suite went **RC=1** with **exactly ONE reddening test in the entire tree**:

```
--- FAIL: TestDifferential (391.21s)
    --- FAIL: TestDifferential/0004-h2-routing (2.45s)
        runner_test.go:1295: distribution: p92 subj content-length fields: want 0 on every arm,
        got keepalive=1,upgrade=1,proxyconn=1,te-gzip=1,te-empty=1 (5 of 5 arms)
```

Three things this establishes, none of them previously measured:

1. **The row is gated, by exactly one inherited assertion.** **ZERO Go unit tests redden under any arm** — nothing in 126 unit-test packages encodes the current behaviour. ⇒ **the row must bring its own unit coverage; it inherits none.**
2. **Phase 92's placement decision is VINDICATED BY A LIVE RED RUN.** The failure surfaced through `AssertDistribution` (`t.Errorf`, step 8, **below** `CompareBytes` at step 7) — exactly where phase 92 moved it after finding a `DriveReference`/`DriveSubject` placement masked the row's own gate.
3. **The non-fail-fast design works**: it named **all five** arms, not just the first.

### 7.2 ⚠️ A STALE TEST CACHE SERVED A VACUOUS GREEN THROUGH THIS EXACT FIX — AND THE MECHANISM IS NEW

Under ARM 1, `test/differential` reported **`ok (cached)`** despite `router_h2.go` being modified. **The differential harness builds envoy-go as a SUBPROCESS, so the router package is not a compile-time input to that test binary** — Go's test cache therefore sees no change and serves the prior PASS.

⚠️ **ARM 1's differential row NEVER EXECUTED, and its headline green is not evidence.** It is recorded here as **UNMEASURED**, not predicted. ARM 2 happened to miss the cache, which is the only reason the pin was seen to fire at all.

⇒ **EVERY phase-93 gate touching `test/differential` MUST pass `-count=1`.** This is `reference_differential_break_protocol_count1` firing again, but by a mechanism the memory does not record: not a repeated identical run, but a **production edit that is invisible to the test binary's cache key**. **The most dangerous form of this trap is a fix whose own gate cannot see it.**

### 7.3 The instrument that must change

`test/fixtures/0004-h2-routing/driver/driver.go`: `:1339 p92ContentLengthFields()` (arity only) · `:1399 p92AssertCLFields` (non-fail-fast, names every mismatching arm) · **`:1408 p92WantRefCLFields = 1`, `:1409 p92WantSubjCLFields = 0`** · called from `AssertDistribution` at `:1515` (ref) and `:1518` (subj).

**What the IMPL must change:** `p92WantSubjCLFields` **0 -> 1**; the doc block at `:1355-1384`; README `:40, :137, :139, :141-153` — **including `:147`, whose *"seven call sites … changes every one of them"* sentence this stage refutes (§3.1)**; and **a NEW per-side instrument that can see a missing body** (§2.4), because arity alone cannot.

⚠️ **`driver_test.go:82-85` exercises the assertion with SYNTHETIC observations, so it does NOT redden** — measured: that package stayed `ok` under every arm. **A unit table over a shared assertion is not a gate on production behaviour** (`reference_shared_assertion_vacuates_unit_table`).

---

## 8. Sentinel — RUN MECHANICALLY, ON BOTH SIDES OF THIS STAGE'S OWN EDIT

Nothing below is inherited from `next-prompt.txt`. Every figure is this stage's own run, recorded not predicted.

### 8.1 PRE-INSERTION, at `40cf6766`

| check | ACTUAL output |
|---|---|
| (1) every ROADMAP row `done` — the field-parsed form used **verbatim**, `want=124` | **SILENT** |
| (2) no family carries deferred candidates | **SIX** — `202 208 214 224 230 238` |
| (3) every WORK family opened — the eleven-slug loop | **SILENT** |

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent at the git root (`ls: cannot access 'stop': No such file or directory`) and in every stage worktree.

⚠️ **CHECK (2) WAS THE ONLY BLOCKER PRE-INSERTION — A ONE-DEEP MARGIN.** Check (1) went silent at the row-92 flip. **This stage's own insertion re-arms check (1)**, so the margin is two while row 93 is open and returns to one the moment row 93 flips `done`. **A future roller must not read a two-deep margin as comfort.**

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

A silent check is indistinguishable from a broken one, so all four were run rather than reasoned about.

- **NC-A** (doctor row 62 to `in-progress`). Landing confirmed by inspection **before** trusting the result: `NC LANDED? [ in-progress ]`. Result **ONE line**, `NOT DONE: row 62`.
- **NC-B** (`want=123` against the real file): **ONE line**, `GATE FAIL: examined 124 data rows, expected 123`.
- **NC-C** (the mandatory check-(3) NC, because check (3) is silent): occurrences in the doctored copy **0**, and the check fired — `NEVER OPENED: gRPC`. **The WASM control on the same copy still read 2**, so the NC narrowed exactly the slug it was aimed at.
- **NC-D** (decompose check (2)): long **5** / short **1** / union **6**, reconciling with the live check exactly.

### 8.3 The insertion itself, measured on BOTH sides

Registering row 93 is the only sentinel-affecting edit this stage makes. **Field counts were taken BEFORE installing**, off-line, on the composed row: **NF=8 under BOTH the naive and the escape-aware form**, `id=[93]`, `status=[in-progress]`, one line.

| observable | PRE | POST |
|---|---|---|
| `ROADMAP.md` lines | 242 | **243** |
| data rows | 124 | **125** |
| check (2) anchors | `202 208 214 224 230 238` | **`203 209 215 225 231 239`** |
| check (2) COUNT | 6 | **6** |
| check (3) | SILENT | **SILENT** |
| malformed rows (escape-aware) | 2 — ids 57 (NF=9), 69 (NF=10) | **2 — ids 57 (NF=9), 69 (NF=10)** |

⚠️ **THE SIX WINDOW ANCHORS SHIFTED `+1` WITHOUT CHANGING THEIR COUNT OR CONTENT**, and **row 93 introduces no new window** (its own line matches the check-(2) pattern **0** times, measured). **This is why a banked LINE anchor into `ROADMAP.md` rots** — `ssl.connection_error` was banked at `:223`, was `:224` when this stage opened, and is `:225` now.

**Check (1) across the insertion, both denominators run:**

```
want=124 (the OLD value, on the NEW file):     NOT DONE: row 93
                                               GATE FAIL: examined 125 data rows, expected 124
want=125 (the NEW value):                      NOT DONE: row 93
```

⇒ **`want` MUST GO 124 -> 125**, and it is updated in `next-prompt.txt` at this close.

### 8.4 The four NCs re-run POST-INSERTION — and NC-A has a NEW SHAPE

⚠️ **NC-A NOW READS **TWO** LINES. This is the SEVENTH distinct NC-A shape in eight stages, and the phase-92 one-line shape DOES NOT CARRY.**

```
NC-A (row 62 doctored, want=125):   NOT DONE: row 62
                                    NOT DONE: row 93
NC-B (want=124 on the real file):   NOT DONE: row 93
                                    GATE FAIL: examined 125 data rows, expected 124
NC-C:                               occurrences 0; NEVER OPENED: gRPC  <- FIRED; WASM control 2
NC-D:                               long 5 / short 1 / union 6
```

⚠️ **NC-B IS ALSO A NEW SHAPE — TWO lines now, where it was one pre-insertion.** **Re-measure both every stage; never inherit.**

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 ⚠️ THE CONTROLLER WAS REFUTED BY ITS OWN AGENTS, TWICE, ON THE CENTRAL DESIGN — AND THAT IS RECORDED, NOT SMOOTHED

The controller read the reference measurement (§2.1), generalized *"the reference never produces a body-less local reply"*, and instructed an agent that the leading design was a **body-presence-conditional** synthesis mirroring `writeH3Reply`. **Both halves were wrong.**

- **The rule was wrong.** `BEHAVIOR_CONTRACT.md:2164` holds a measured reference transcript of an **empty-body** local reply carrying **`content-length: 0`**, ratified as ADR-0085 doctrine at `DECISIONS.md:8260`/`:8326`. The guard would have suppressed a header the reference sends.
- **The placement was wrong.** A `writeH2Reply`-side injection would re-inject what `compressor.go:784` deliberately strips on the proxied path.

⚠️ **THE FIRST REFUTATION CAME FROM A TRANSCRIPT SITTING IN THIS REPO THE WHOLE TIME.** The docker probe was necessary and was **not sufficient**: it measured the shapes the charter names and the controller over-generalized from them. ⇒ **Before generalizing a measured table into a RULE, grep the tree for a counter-example to the rule — not just to the table.**

### 9.2 ⚠️ THE TREE HOLDS TWO CONTRADICTORY RATIFIED RULES ABOUT `content-length: 0`, AND NOBODY HAD NOTICED

ADR-0085 (inject it on an empty body) against `writeH3Reply` (*"never Content-Length: 0"*, pinned at `h3dispatch_test.go:155`). **Only ADR-0085 is corroborated by a measured reference transcript, and that transcript is H/1.** ⚠️ **The H/3 arm is UNMEASURED, so neither rule may be tidied into the other** — a taker's first job is to probe whether the reference emits `content-length: 0` on an **H/3** empty-body local reply. **RECORDED, NOT RESOLVED.**

### 9.3 ⚠️ AN ARITY PIN IS STRUCTURALLY INCAPABLE OF SEEING A MISSING BODY — AND THAT BLINDNESS IS ALREADY LIVE ON THE H/1 LEG

§2.4. The fixture's **ARITY, NEVER VALUE** rule was right for phase 92 and has a consequence phase 92 did not state: once H/2 emits the header, arity reads **1 vs 1** even where envoy-go sends 0 bytes against Envoy's 19 or 24. **On H/1 that is not prospective — it is today**, measured end-to-end (`Content-Length: 0` with `Body: nil`, arity 1), and **pinned by nothing**: the reference's own strings appear in **zero** files under `test/`. ⇒ **When a fix converges a metric, ask whether it removed the DEFECT or removed the metric's ABILITY TO SEE IT** — the exact inverse of the question ADR-0314 (xii) asked about a fix that turned a metric red. **This row therefore upgrades the instrument in the same commit as the fix.**

### 9.4 ⚠️ A STALE TEST CACHE SERVED A VACUOUS GREEN THROUGH THIS EXACT FIX, BY A MECHANISM NOT PREVIOUSLY RECORDED

§7.2. `test/differential` reported **`ok (cached)`** with a modified `router_h2.go`, because **the harness builds envoy-go as a SUBPROCESS and the router is therefore not a compile-time input to that test binary.** `reference_differential_break_protocol_count1` records the repeated-identical-run form of this trap; this is a **production edit invisible to the cache key**, which is worse: **the fix's own gate could not see the fix.** ⇒ **`-count=1` on every `test/differential` invocation, unconditionally.**

### 9.5 ⚠️ `STATE.md` CARRIES **TWO** FROZEN REGIONS, BOTH SELF-LABELLED AS LIVE

- **§Recent's preamble (`:44`)** says *"§Current carries the newest stage (**the phase-90 IMPL** …)"* while §Current reads **phase 92**, and still presents the phase-90 close's eviction as current. `git log -L44,44` puts its last modification at **`b312fc95` (2026-08-22, the phase-90 IMPL)**; **TWELVE commits have touched the file since, spanning EIGHT lifecycle stage closes**, and every one rolled the ENTRIES while none rolled the PROSE ABOUT them.
- **§Project counts (`:27-40`)** is headed *"(Live. Re-verify at session start …)"* while its own body says *"at this phase-76 IMPL close"* — **sixteen phases back.** Re-measured: fixtures **119 -> 121**, fuzzers **55 -> 56**, ADR tail **ADR-0298 -> ADR-0314**.

**The common mechanism: PROSE THAT DESCRIBES ROLLED DATA IS ITSELF NEVER ROLLED.** ADR-0288 hardened this file so *"a session may trust a grep of it again"*; the bullets stayed honest and the narrative around them fossilised — the same failure mode, one layer out, in the file that was hardened against it. ⚠️ **AND THE GUARD CANNOT CATCH IT**: `^- \*\*prior active-phase:\*\*` reads **163** and is structurally blind to a line beginning `*(`. **An archive-shape guard is not a freshness guard.** **Both regions are corrected at this close** — see 9.7 for what was deliberately NOT renumbered.

### 9.6 ⚠️ A `ROADMAP` ROW'S ANTICIPATED COUNTS ARE NEVER RECONCILED AT ROW-DONE — SO THE ROADMAP IS NOT A COUNT SOURCE

Row 92 still reads **`fuzzers 55 / 48`** as an anticipated `+0`. The measured value is **56 / 48**, and **the commit that made it 56 is row 92's own IMPL** — `40cf6766` added `FuzzValidateResponseHeaderBlock` to `internal/filter/hcm/h2/fuzz_test.go` and flipped the row to `done` in the same commit. ⚠️ **The row is not "wrong" — it is a correctly-labelled ANTICIPATION that its own outcome falsified.** But **three regions of this repo disagree about one figure at one tip**, and a reader grepping `ROADMAP.md` gets the anticipation, not the outcome. **Take counts from a command, never from a row.**

### 9.7 ⚠️ ON DE-ROTTING A COUNT THAT IS ITSELF CONTESTED

`STATE.md`'s stat-surface figure is stale, but **three different absolutes are live in the tree** and the contract warns a mechanical re-derivation *"should expect the re-derived figure to disagree."* Per `reference_a_drift_correction_is_itself_a_claim`, this stage **relabels it as contested and quotes NO number**, rather than replacing one unverifiable absolute with another. **A de-rot that invents a figure is worse than the rot.** The three mechanically re-derivable figures beside it WERE corrected, each with the command that produced it.

### 9.8 Method findings, each found by execution

- ⚠️ **`grep -c` PRINTS `0` *AND* EXITS 1, so `$(cmd || echo 0)` EMITS TWO ZEROS.** The controller's own composer-roster command did exactly this. Capture with `v=$(cmd || true)`; never chain the fallback.
- ⚠️ **`\t` inside a regex is TOOL-DEPENDENT and FAIL-UNSAFE.** `grep -c '^\t"bytes"'` reads **0** under GNU BRE against a file that does contain the import; `$(printf '\t')` reads **1**. Naming the tool is not optional.
- ⚠️ **Omitting `--` before `-family row` makes GNU grep parse `-f`**: `grep: amily row: No such file or directory`, and the surrounding arithmetic then prints `0`, **which reads exactly like "no change."**
- ⚠️ **A banked LINE ANCHOR into `ROADMAP.md` rots on every row insertion** — `ssl.connection_error`'s window was banked `:223`, was `:224` at open, is `:225` now (§8.3). **Anchor on window CONTENT.**
- ⚠️ **`go test ./...` INCLUDES `test/differential` AND THEREFORE DRIVES DOCKER.** A cost-measurement agent ran it unaware and launched containers twice while a sibling owned Docker. **Scope cost runs with `go list ./... | grep -v '/test/differential$'` unless the differential is the thing being measured**, and serialize Docker ownership across parallel agents.
- **Response header ORDER is NOT a contract** — `BEHAVIOR_CONTRACT.md:15` says *"Set-equal modulo documented allow-list."* The reference emits `content-length` FIRST on a local reply; envoy-go's helper emits `Content-Type, Date, Server`. **The SPEC must not chase that ordering** and must not "fix" the helper's documented insertion order.

### 9.9 On the §Recent eviction shape at this close

**UNIQUE OLDEST, NO TIE** — read directly from all five entry dates: `2026-08-26`, `2026-08-25`, `2026-08-25`, `2026-08-24`, **`2026-08-23`**. The evictee is `phase 91 (h2-framer-partial-frame-desync) PLAN done` **on its DATE**. ⚠️ **It happens to also sit last, and that coincidence is exactly what makes position-by-convention dangerous — it would have given the right answer here and the wrong one at three of the last six closes.** The shape is not inherited and is not carried forward.

---

## 10. What the SPEC owes

1. **Choose the emission site and JUSTIFY IT AGAINST §3.4** — the helper, not the writer. If it proposes the writer, it must dispose of the compressor-strip hazard by measurement, not by argument.
2. **Enumerate BY COMPILING PROTOTYPE across the six named gaps in §6.1.** The `+3/-0` floor is production-only and `reference_measured_prototype_is_a_lower_bound` has fired five consecutive rows by under-enumerating FILES.
3. **Design the body-visible instrument (§2.4).** Arity cannot see a missing body. State what the new per-side observable is, where it lives (`AssertDistribution`, below the byte compare — never `DriveReference`/`DriveSubject`), and how it is negative-controlled in BOTH directions.
4. **Bring unit coverage.** The row inherits **none** (§7.1) — zero Go unit tests redden under any arm.
5. **Draft ADR-0315 §Context** and arm the strict `PROPOSED` guard **1 -> 2**, verified BY LINE AND BY ADR because the ADR-0231 decoy already reads 1 at `:14866`.
6. **State the H/3 contradiction (§3.3) as a named non-goal with its arm UNMEASURED** — do not tidy either rule into the other.
7. **Price the `BEHAVIOR_CONTRACT.md` rider** — a narrative rider only; the stat surface does not move, so **no ledger line** and **no absolute**.
8. **Mandate `-count=1` on every `test/differential` invocation** in the plan's gate commands (§9.4).

---

## 11. Probe hygiene

- **Seven worktrees at open** (one stage + four recon), all created off `git rev-parse master`, none off a quoted SHA. **All recon agents committed NOTHING**; each reported an empty `git status --porcelain` verbatim.
- **Four prototypes were built, run and REVERTED** with `sha256sum` captured before patching and `sha256sum -c` after — **every restore reported `OK`**, over three `git checkout --` rounds across four production files, plus `--untracked-files=all` proving both throwaway probe files removed.
- **Docker:** containers were named with a distinctive `p93r2-` prefix and torn down **BY NAME**. ⚠️ **The foreign-container roster is NOT closed** — `infallible_booth`, `crazy_kare`, `golink-ai`, `quizzical_goldstine` persisted, and `jolly_nobel`, `curl-world-tls-1`, `curl-world-httpbin-1` and a `reaper_*` were also present and **left untouched**.
- ⚠️ **A cost agent inadvertently drove Docker** via `go test ./...` while a sibling owned it (§9.8). No corruption resulted, and the accident produced the only live differential measurement of the fix — but the ownership discipline failed and is recorded as such.
- **No `pgrep -f` / `pkill -f` was used.** No `stop` file was created.
