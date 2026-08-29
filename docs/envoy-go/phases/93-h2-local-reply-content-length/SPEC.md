# Phase 93 — `h2-local-reply-content-length` — SPEC

**Lifecycle state 1 -> 2.** Row 93 stays `in-progress`; `ROADMAP.md` is BYTE-UNTOUCHED by this stage and the sentinel `want` stays **125**.

**Charter, unchanged in substance:** on the HTTP/2 leg a locally generated reply emits a `Content-Length`, ALWAYS. The fix lands in `h2LocalReplyHeaders()` and deliberately NOT in `writeH2Reply`.

⚠️ **THE ONE THING THAT CHANGED: the BRAINSTORM's chosen ARM IS REVERSED.** The BRAINSTORM picked ARM 2 (a parameterless helper emitting a placeholder, `+3 / -0`, one file) and called ARM 1 (`bodyLen int`, two files) *"strictly dominated … for an inert parameter."* **Measured at this SPEC: the parameter is NOT inert, the placeholder is UNSAFE, and ARM 1 is the pick.** The BRAINSTORM compared the two arms on COST ALONE and never priced the CORRECTNESS axis on which they differ. §4 is that measurement.

---

## 0. What this stage refuted, by execution

Eleven inherited claims fell. Six were the BRAINSTORM's, two were the controller's own hypotheses, and three are cite defects propagated into `ROADMAP.md` and `STATE.md`.

1. ⚠️ **ARM 1 IS NOT DOMINATED — THE PLACEHOLDER IS UNSAFE** (§4). Reversal of the BRAINSTORM's central pick.
2. ⚠️ **`ADR-0085` IS A MIS-ATTRIBUTION, AND IT IS THE CHARTER'S LOAD-BEARING RULE A** (§3.1).
3. ⚠️ **THE "TWO CONTRADICTORY RATIFIED RULES" CLAIM IS FALSE AS STATED, AND INVERTS WHICH RULE IS OPERATIVE** (§7).
4. ⚠️ **THE `PROPOSED` GUARD INSTRUCTION CONFLATES TWO DIFFERENT REGEXES** — the form ADR-0315 will match goes **0 -> 1**, not 1 -> 2 (§9).
5. ⚠️ **THE INSTRUMENT CANNOT SEE THE DEFECT THE BRAINSTORM BUILT IT FOR** — fixture `0004` drives NONE of the body-less sites (§5.2).
6. ⚠️ **`+3 / -0` IS 1 OF AT LEAST 4 REQUIRED FILES, AND IS ONLY REACHABLE BY SHIPPING A REFUTED COMMENT** (§8).
7. ⚠️ **THE CONTROLLER'S OWN COMPRESSOR-COUNTER HYPOTHESIS WAS REFUTED AT THE DEFAULT CONFIG** — the real harm is the ACCESS LOG (§4.2).
8. ⚠️ **THE CONTROLLER'S `buffer.go` HAZARD WAS REFUTED** — that filter is `Encoder: nil`, decoder-only (§4.3).
9. ⚠️ **THE "36 FILTER FILES" FIGURE IS NOT REPRODUCIBLE** under any of six scopes; the load-bearing fact is structural (§3.2).
10. ⚠️ **THE ARCHIVE GUARD'S `163` IS NOT THE ENTRY COUNT**, and its POSITIVE CONTROL IS SELF-INCREMENTING (§10.2).
11. ⚠️ **FOUR BANKED SYMBOL ANCHORS POINT AT COMMENT LINES, AND THE TWO DRIVER CONSTANTS ARE CITED 26 LINES OFF** (§10.1).

⚠️ **A LIVE PRODUCTION BUG WAS FOUND INCIDENTALLY AND IS BANKED, NOT FIXED HERE** (§4.4): on the current tip an H/2 local reply under a default-config compressor is stamped `Content-Encoding: gzip` over an **UNCOMPRESSED** body. Either phase-93 design incidentally fixes it. It is recorded because finding it and saying nothing would be worse than not finding it.

---

## 1. Scope, restated as a decision

This row fixes the **header**. It deliberately does **not** fix the **empty-body** defect underneath it, which spans both codec legs. That separation is inherited from BRAINSTORM §1.4 and survives this stage — but §5.2 narrows what the row can honestly claim about it.

**In scope:** the seven H/2 router local-reply sites; the fixture instrument; unit coverage; ADR-0315 §Context; a `BEHAVIOR_CONTRACT.md` narrative rider.

**Explicit non-goals, each with its reason and its unmeasured arm named:**
- The **empty-body** defect (banked, BRAINSTORM §4.0). ⚠️ **And note the contract already RELAXES it:** *"Local-reply body bytes for 4xx/5xx (Envoy emits HTML/JSON local replies; envoy-go emits plain-text bodies … Status is asserted; body is relaxed)"* — `BEHAVIOR_CONTRACT.md`, `### Not asserted`. It is a ratified relaxation, not an unnamed divergence.
- The **H/3 leg** (§7) — but for a different reason than the BRAINSTORM gave.
- The **`Content-Encoding: gzip` over an uncompressed body** bug (§4.4).
- **New arms driving the 503/504 classes** (§5.2) — the only way to make the instrument see a missing body.

---

## 2. Mechanism, re-derived at this tip

All anchors below were re-verified at `56c4d255`. ⚠️ **Line anchors drift; symbol and literal-text anchors do not.** Each row gives the drift-proof anchor the PLAN must cite instead.

| what | line at this tip | DRIFT-PROOF anchor |
|---|---|---|
| the omitting composer | `router_h2.go:294` | `func h2LocalReplyHeaders() envoyhttp.OrderedHeaders` |
| the H/1 sibling | `router.go:675` | `func localReplyHeaders(bodyLen int) envoyhttp.OrderedHeaders` |
| the writer that recomputes | `h2dispatch.go:1004` | `func writeH2Reply(sw h2.StreamWriter, status int, …)` |
| the recompute itself | `h2dispatch.go:1014-1016` | `if ln == "content-length" {` … `val = strconv.Itoa(len(body))` |
| the proxied-path caller | `h2dispatch.go:677` | the only `writeH2Reply` call passing `resp.Trailers` |
| the encode-chain entry | `h2dispatch.go:650` | `chain.RunEncodeHeaders(ctx, merged, …)` |
| the compressor strip | `compressor.go:784` | `headers.Del("Content-Length")` |
| the compressor READ | `compressor.go:922` | `if cl := headers.Get("Content-Length"); cl != ""` |
| the framework composer | `chain.go:1288` | `func (c *FilterChain) beginLocalReply(` |
| its unconditional set | `chain.go:1313` | `merged.Set("Content-Length", fmt.Sprintf("%d", len(body)))` |
| the 502 body | `router.go:31` | `const bad502Body = "bad gateway\n"` (12 bytes) |

⚠️ **A RECEIVER WRITTEN `(c *FilterChain)` MUST BE GREPPED WITH `grep -F`, NOT `-E`.** ⚠️ **PATHSPEC-SCOPE every symbol assertion** — `beginLocalReply` lives in `internal/filter/http/chain.go`; **`internal/filter/hcm/chain.go` DOES NOT EXIST**, and the BRAINSTORM's §3.3 cite names the wrong package.

### 2.1 The seven call sites — count CONFIRMED, consequence REFUTED

Re-derived: `retry.go:374` (504) · `router_h2.go:80` (503) · `:128` (503) · `:138` (503) · `:148` (502) · `:231` (502) · `:250` (502). **Exactly seven.** ⚠️ `:128` and `:138` are TEXTUALLY IDENTICAL — they must be anchored on the preceding comment, not the line.

**Four carry no body; three carry `bad502Body` (12 bytes).** That 4/3 split is what §4 turns on.

⚠️ **THE README's `:147` SENTENCE IS REFUTED ON ITS CONSEQUENCE, NOT ITS COUNT.** It reads *"Giving it a body length and emitting `Content-Length` changes every one of them"*. The **seven** is correct. What is false is *"changes every one of them"* — and note that under the pick of §4 it is **half-true**: all seven call sites DO change (they gain an argument), but not for the reason the sentence gives. **The IMPL must correct the clause precisely, or it ships a new falsehood in the commit that fixes the old one.**

---

## 3. D-93-RULE — **ALWAYS-EMIT.** Settled, with its citation corrected.

The rule is always-emit, never emit-when-non-empty. A measured reference transcript of an **empty-body** local reply carries `content-length: 0`.

### 3.1 ⚠️ THE DOCTRINE IS RATIFIED — BUT NOT WHERE THE BRAINSTORM, THE ROADMAP ROW AND `STATE.md` ALL SAY IT IS

All three say *"ratified as ADR-0085 doctrine at `DECISIONS.md:8260`/`:8326`"*. Measured:

- `## ADR-0085` is at `DECISIONS.md:3282` and reads **"Admin-mux reuse + LBP-1 third application — `admin.New` widens to thread …"**. Its block spans `:3282-:3326` and contains **ZERO** matches for `SendLocalReply|content-length|local reply|local-reply`.
- Both quoted sentences live under **`## ADR-0155`** (`:8187`, the jwt_authn deny-path wire shape). ADR-0155 *cites* "per ADR-0085" as authority.

⇒ **The doctrine IS ratified, in ADR-0155, which attributes it to ADR-0085.** The correct form is *"recorded and ratified in **ADR-0155**, attributed there to ADR-0085"*. ⚠️ **Asserting the doctrine is IN ADR-0085 is refutable by opening `:3282`, and the PLAN must not restate it.** Drift-proof anchor for the doctrine: the literal `` still injects the 4-header standard set (`content-length: 0` + `date` + `server: envoy`) ``.

The corroborating transcript is `BEHAVIOR_CONTRACT.md`, `### Empirical evidence (cors preflight)`, Probe (a) — a `200` with an empty body carrying `content-length: 0`. ⚠️ **Two riders the BRAINSTORM did not state:** it is an **H/1** transcript (`OPTIONS / HTTP/1.1`), which is why §7's H/3 arm stays unmeasured; and in it `content-length` is **LAST**, not first — so the BRAINSTORM's incidental *"the reference emits `content-length` FIRST"* is **not what this transcript shows**. Header order is not a contract either way (`BEHAVIOR_CONTRACT.md:15`); the PLAN must not chase it.

### 3.2 The internal inconsistency, stated WITHOUT a count

`h2LocalReplyHeaders()` is the sole omitter among the local-reply composers. Verified: `beginLocalReply` (`chain.go:1313`, unconditional), `directResponseAction.body()` (`actions.go:91`, in its literal set), `localReplyHeaders` (`router.go:675`, in its literal set), `writeH3Reply` (`h3dispatch.go:33`, synthesize-if-absent). **Four emit; one does not.**

⚠️ **THE BRAINSTORM'S "ALL 36 FILTER FILES CALLING `SendLocalReply`" IS NOT REPRODUCIBLE.** Six plausible scopes at this tip read **54 / 46 / 34 / 42 / 228 / 29**, and none reads 36. Per `reference_a_drift_correction_is_itself_a_claim`, on a contested count: **NO NUMBER.** The load-bearing fact was never the count — it is structural: **`SendLocalReply` has exactly ONE definition** (`chain.go:748`), whose whole body delegates to `beginLocalReply`. Every caller, however many there are, routes through one unconditional set. **A filter's 403 already carries a `Content-Length` where the router's 502 does not.**

---

## 4. D-93-VALUE — **TAKE THE `bodyLen int` PARAMETER.** The BRAINSTORM's pick is REVERSED.

**Decision: `h2LocalReplyHeaders(bodyLen int)`**, mirroring the H/1 sibling's signature. Four call sites pass `0`; three pass `len(bad502Body)`.

**MEASURED cost, from a compiling prototype at this tip:**

```
1	1	internal/filter/http/router/retry.go
9	7	internal/filter/http/router/router_h2.go
```
⇒ **`+10 / -8`, TWO production files** (the `9` includes the `strconv` import). Both packages green: `./internal/filter/http/router/` **129 `=== RUN`, 0 anchored FAIL**; `./internal/filter/hcm/` **323 `=== RUN`, 0 anchored FAIL**.

The BRAINSTORM's ARM 2 measured `+1 / -0` (bare) to `+3 / -0` (commented) in one file. **The delta the parameter costs is ~9 source lines. What those lines buy is below.**

### 4.1 Why the placeholder is unsafe — the carrier is OBSERVED before the writer corrects it

`writeH2Reply` recomputes an already-present `content-length` from `len(body)`. **That is CONFIRMED by execution** — a deliberate `999` against a 12-byte body came out as `12`; a pristine carrier came out with no field at all; and a negative control on the unpatched tip read arity **0**. So the **WIRE** is correct under either design.

⚠️ **BUT THE WIRE IS NOT THE ONLY OBSERVER.** Both are under the same guards in `h2dispatch.go`:

- `:624` `if rf.ActionRan() && status > 0 && actionErr == nil {` → `:650` `chain.RunEncodeHeaders(ctx, merged, …)`
- `:676` `if rf.ActionRan() && actionErr == nil && status > 0 {` → `:677` `writeH2Reply(…)`

**The encode chain sees the carrier ~26 lines before the writer corrects it.** And `writeH2Reply` builds its own `hf` slice (`h2dispatch.go:1005`) — **it never mutates `resp.Headers`**, so the correction is invisible to everything downstream that reads the carrier.

**Reachability CONFIRMED by a `panic()` control, not by reading:**
```
panic: P93-REACH-SITE-router_h2.go:148 [recovered, repanicked]
 …router.doH2ClusterAction(…) router_h2.go:148
 …router.(*Filter).RunAction(…) router.go:313
 …hcm.(*chainDispatchAction).WriteH2(…) h2dispatch.go:606
```
All seven sites satisfy the `:624` guard: each returns `err == nil` with `Status ∈ {502,503,504} > 0`, and `RunAction` sets `f.actionRan = true` **unconditionally before invoking the action** (`router.go:311`). **No site can bypass the encode chain.**

### 4.2 ⚠️ THE CONTROLLER'S OWN HAZARD WAS REFUTED; THE REAL HARM IS THE ACCESS LOG

The controller predicted the placeholder would book a **different compressor skip-reason counter**. **REFUTED at the default config:** with `min_content_length` = 30 and a 12-byte body, absent-CL and `CL:0` book **the same two counters** (`response_not_compressed` + `response_content_length_too_small`) — one via the late `EncodeData` gate, one via bucket 11 in `EncodeHeaders`. **Indistinguishable.**

**What actually settles it needs no compressor and no unusual config.** Measured through a real `doH2ClusterAction` 502 → real encode chain → `captureH2Writer`, with an access-log sink configured `additional_response_headers_to_log: [content-length]`:

| | encode chain sees | wire | access log records |
|---|---|---|---|
| **today** | `Content-Length` ABSENT | field missing | `ResponseHeaders=map[]` |
| **placeholder `"0"`** | PRESENT, value `"0"` | `content-length = "12"` | ⚠️ `map[content-length:0]`, `BytesSent=12` |
| **`len(bad502Body)`** | PRESENT, value `"12"` | `content-length = "12"` | `map[content-length:12]` |

⇒ **The placeholder produces a SELF-CONTRADICTORY access-log record** — `content-length: 0` beside `BytesSent=12`, while the wire carries 12. `emitAccessLogH2` is handed the post-encode, **pre-`writeH2Reply`** carrier, and `bytesSent` is taken from `len(resp.Body)`. **This fires on all three 502 sites, unconditionally.**

**And there IS a compressor divergence — just not at the default.** At `min_content_length ∈ [1,12]`:
- **CL absent (today):** compress path → `response_compressed=1` → **gzipped**.
- **CL `"0"`:** bucket-11 **SKIP** → not gzipped, two wrong counters. ⚠️ **Diverges from today AND from the truth.**
- **CL `"12"`:** compress path → **byte-identical to today.**

⇒ **The truthful value is behaviour-preserving; the placeholder is not.**

### 4.3 Every other encode-side observer — one of the controller's two candidates refuted

Fourteen filters implement an encode-headers hook. **Only `compressor` reads `Content-Length` to make a decision** (`compressor.go:922`), exhaustively verified.

⚠️ **The controller's `buffer.go:250-251` hazard is REFUTED:** `internal/filter/http/buffer/buffer.go:102` declares `Encoder: nil, // decoder-only per planner-time decision 5`, and its `headersRef` is set in `DecodeHeaders` — it holds the **REQUEST** map. A response carrier never reaches it. Likewise `extproc/check.go:930` is an unconditional `Set`, not a read, and `lua/bridge.go`'s constant belongs to Lua's own `respond()` construction.

**Three filters EXPORT the carrier verbatim**, so a placeholder becomes an observable lie without changing envoy-go's own control flow: **extproc** (ships the response header set to an external processor), **tap** (writes it to the sink), **lua/wasm** (hand the map to operator code).

### 4.4 ⚠️ BANKED, NOT FIXED — a live H/2-only body corruption on the current tip

At the default config with `Accept-Encoding: gzip`, an H/2 local reply on the tip takes the compress path (`Content-Type: text/plain` is in the default list and `uncompressibleResponseCodes` is empty in MVP, so 502 does not skip), gets `Content-Encoding: gzip` + `Vary` **stamped**, and then the late `EncodeData` gate reverts the body — **shipping `Content-Encoding: gzip` over an UNCOMPRESSED body**, with `OverwriteBody calls = 0`. **The H/1 leg does not have this**, because `localReplyHeaders(0)` makes the field PRESENT and the header-stage gate skips first.

**Either phase-93 design incidentally fixes it**, because both make the field present. It is recorded here, banked as its own candidate, and **NOT claimed as this row's deliverable** — the row must not take credit for a fix it did not design, and the reference-side comparison for it is unmeasured.

### 4.5 ⚠️ BRAINSTORM §3.2's "MIRRORING H/1 BUYS NOTHING" IS REFUTED

The BRAINSTORM called the H/1 `bodyLen` *"inert twice over"* — all six callers pass `0`, and `writeH1Reply` overwrites it. **Both halves are true. The conclusion is not.**

H/1 is structurally symmetric to H/2: `connection.go:738 RunEncodeHeaders` → `:766 writeH1Reply`. So the H/1 value **is** observed at the encode seam before the wire correction. Measured counterfactual on the body-less pair:

- **H/1 today** (`CL: 0`, body 0): bucket-11 skip, clean headers.
- **H/1 with the parameter removed** (CL absent, body 0): **compress path** → `Content-Encoding: gzip` + `Vary` stamped on a **bodyless** 503, zero counters.

⇒ **The H/1 parameter is inert on the WIRE and LOAD-BEARING at the ENCODE SEAM** — not for its *value* but because it makes the field **PRESENT**. *"Inert twice over"* holds on one axis only. **This is the general lesson: a value that a downstream layer overwrites is not thereby unobserved.**

### 4.6 What the reversal does NOT change

⚠️ **THE WIRE BYTES ARE IDENTICAL UNDER BOTH DESIGNS** (`writeH2Reply` recomputes either way). ⇒ **any `test/differential` golden churn is the SAME for both, and the cost delta really is just those ~9 source lines.** That churn is UNMEASURED at this stage (Docker out of scope) and applies to both designs equally — the PLAN owes it.

---

## 5. D-93-INSTRUMENT — record BOTH observables, per side, below the byte compare

### 5.1 The design

Today `test/fixtures/0004-h2-routing/driver/driver.go` pins `content-length` **ARITY** per side (`p92WantRefCLFields = 1`, `p92WantSubjCLFields = 0`). Once the fix lands, arity reads **1 vs 1** on every arm and the pin stops discriminating.

`p92DriveArm` returns `(fields, status, failure)` and **explicitly discards DATA payloads** — *"The body is NOT recorded: a forwarded 200 `p92-ok` and a locally generated 502 carry different, side-specific text."* ⚠️ **That comment justifies not recording the TEXT. It does not justify not recording the LENGTH**, and the length is what an instrument needs.

**The row records TWO observables per arm, and the PAIR is the point:**
- **`declared`** — the value the proxy CLAIMED in `content-length`. A claim, not a fact.
- **`bodyLen`** — the summed length of every DATA frame payload actually delivered.

Value alone is blind to a 0-byte body under a `content-length: 12` header. Length alone is blind to a header that disagrees with the body. **Together they support one genuinely side-independent invariant plus two per-side departure pins:**

1. **`declared == delivered`, asserted PER SIDE as a plain equality** (RFC 9110 §8.6). ⚠️ **This is the ONE property that is not a departure — it must hold on both sides independently, so it is NOT relaxed.** It is also the property that gates the row's own mechanism: it is the only assertion that can see a `content-length` lying about its own body, since arity reads 1 either way.
2. **Per-side body-length pins** at measured values (`ref 87`, `subj 12`), recording the departure in both directions — the phase-92 shape, and consistent with the contract's ratified relaxation of local-reply body bytes.

**Placement:** all four assertions in `AssertDistribution`, appended after the p92 pins, **BELOW `CompareBytes`**, joined by `errors.Join`. ⚠️ **NEVER in `DriveReference`/`DriveSubject`** — phase 92 measured that a `Drive*` placement `t.Fatalf`s before the byte compare and MASKED its own gate. **Non-fail-fast: every mismatching arm is named.**

⚠️ **THE CROSS-SIDE BYTE TRANSCRIPT IS UNCHANGED** — measured: no `Fprintf(&out` line moves. **No re-baselining.**

⚠️ **A DUPLICATED `content-length` MUST YIELD `declaredOK = false`, NOT "the first value"** — a duplicate is RFC 9113 §8.1.1 malformed and its value is meaningless; returning the first would launder a real defect into a plausible number. **And "absent" must stay distinguishable from "`content-length: 0`"** — the empty-vs-zero trap, which is precisely this row's subject.

⚠️ **A ROSTER BARRIER IS MANDATORY.** Every per-arm assertion is a range loop, so **zero observations satisfies all of them**. The barrier asserts `len(got) == len(p92Arms())` and per-index arm-name identity, **read from the LIVE `p92Arms()`, never a literal**. Measured without it: `got 0 observations, want 5` on all four pins, with one wantErr row **passing for the wrong reason**.

### 5.2 ⚠️ THE INSTRUMENT CANNOT SEE THE DEFECT THE BRAINSTORM BUILT IT FOR — AND THE ROW MUST SAY SO

The BRAINSTORM's §2.4 argues the instrument is needed because arity would read 1 vs 1 *"including the four where envoy-go sends a 0-byte body against Envoy's 19- or 24-byte diagnostic."*

⚠️ **FIXTURE `0004` DRIVES NONE OF THOSE SITES.** All five p92 arms terminate on the **502** path, where envoy-go **does** send a body (`bad502Body`, 12 bytes). Measured: `driver.go` contains exactly **one** mention of `503` or `504`, **and it is a comment.**

⇒ **The instrument makes the 12-vs-87 divergence visible and pinned. It is STRUCTURALLY INCAPABLE of seeing the 0-vs-19/24/167 sites, because the fixture never drives them.** Closing that needs new arms driving the 503/504 classes, or a new fixture. **It is named here as an open gap and is NOT claimed as closed.** The row's honest claim is narrower than the BRAINSTORM's: it prevents the arity pin from going vacuous, and it pins the body departure **on the arms that exist**.

### 5.3 The reference-side pin is a CLAIM until the first live run

⚠️ **`p93WantRefBodyLen = 87` WAS NOT RE-DERIVED AT THIS STAGE** — no Docker. Its only provenance is the BRAINSTORM's probe. It ships with that provenance stamped in the comment and must be **CONFIRMED on the IMPL's first `-count=1` differential run**. Per `feedback_brief_citations_not_evidence`, treat it as a claim. `p93WantSubjBodyLen = 12` is derived from the tree (`len(bad502Body)`), not measured.

---

## 6. D-93-UNIT — the row brings its own coverage, proven in BOTH directions

The row inherits **none**: with the fix applied, zero Go unit tests redden anywhere.

**File:** `internal/filter/http/router/router_h2_local_reply_cl_test.go` (NEW, ~117 lines). **Test:** `TestH2LocalReplyContentLengthAlwaysEmitted`, three arms.

- **`helper`** — asserts `h2LocalReplyHeaders(…)` carries **exactly one** parseable `content-length`. ⚠️ **Arity exactly one, not "at least one".**
- **`h1_sibling_control`** — asserts the same over `localReplyHeaders(0)`. ⚠️ **This arm must be GREEN in BOTH directions.** It proves the assertion helper itself can pass, so a red `helper` arm is a statement about H/2 and not about a broken assertion.
- **`live_502_dial_failure`** — drives the REAL `doH2ClusterAction` against a closed port so the assertion runs over a header set an actual production path composed. ⚠️ **Required by `reference_shared_assertion_vacuates_unit_table`:** a table over the helper's return value alone pins the helper in isolation.

⚠️ **NO VALUE IS PINNED beyond "parses as a non-negative integer."** Pinning the literal would pin an implementation detail the writer overwrites.

**MEASURED, both directions:**
- **With the fix:** PASS, **4 `=== RUN`** (selector matched; no `[no tests to run]`), RC=0.
- **Fix reverted, test present:** RC=1, **two arms redden and the H/1 control stays green**, both failures reported in one run (non-fail-fast confirmed):
```
router_h2_local_reply_cl_test.go:86: h2LocalReplyHeaders(): content-length field arity = 0 [], want exactly 1
router_h2_local_reply_cl_test.go:115: doH2ClusterAction dial-failure 502: content-length field arity = 0 [], want exactly 1
```

⚠️ **TREE-WIDE NC, AND IT RE-CONFIRMS A STANDING MEMORY BY MEASUREMENT.** With the fix reverted across 234 packages: **exactly ONE package reddens**, and `test/fixtures/0004-h2-routing/driver` stays **`ok`** — its 18-row table is synthetic and is **not** a gate on production behaviour. **The differential is the only production gate on the driver side.**

### 6.1 A pin the tree does not have, and the row's correctness rests on it

⚠️ **NOTHING IN THE TREE PINS `writeH2Reply`'s RECOMPUTE-FOR-PRESENT-FIELD BEHAVIOUR.** The row's entire mechanism depends on it, and the only execution evidence is a throwaway probe. **The PLAN should land that probe permanently** (~+60 lines in `internal/filter/hcm/`), with the negative control that reads arity **0** on an absent field. **ESTIMATED, not measured.**

---

## 7. D-93-H3 — **the "contradiction" does NOT exist.** Named non-goal, with the real gap named instead.

The BRAINSTORM claims the tree holds *"two contradictory ratified rules"* and instructs the SPEC to record the contradiction unresolved. ⚠️ **MEASURED: the claim is FALSE as stated, and it inverts which rule is operative.**

The two rules govern **different layers** and compose without conflict:
- **Rule A — carrier construction** (`chain.go:1288`; `router.go:675`): unconditional `Content-Length: len(body)`, hence `0` on an empty body.
- **Rule B — synthesis fallback** (`h3dispatch.go:55`, `if len(body) > 0 && h.Get("Content-Length") == ""`): governs only what `writeH3Reply` synthesizes **when the carrier omitted it**. It is a synthesize-if-absent, **never a reconcile**.

**Measured end-to-end through a real `FilterChain`, no reconstruction:** `SendLocalReply(403, "", nil)` → `LocalReplyResponse()` → `writeH3Reply` ⇒ wire `Content-Length: 0`. And `runH3` calls only `rf.SetAction(…)`, never `SetH2Action` (**0 occurrences in `h3dispatch.go`**), so the H/1 branch always runs.

⇒ **Rule B's comment — *"an empty-body response gets no Content-Length, never Content-Length: 0"* — is NOT a wire-level claim and is MATERIALLY MISLEADING AS WRITTEN.** On every live H/3 local reply the wire **does** carry `content-length: 0`, supplied by Rule A one layer up. `TestWriteH3Reply_EmptyBody` passes a **nil carrier**, so it pins only the synthesis arm: it cannot contradict Rule A, and it cannot detect it either — **it is not a guard on the live path at all.**

**⇒ The SPEC's disposition: the `h3dispatch.go` comment is a DEFECT TO REWORD, not a ratified counter-rule to reconcile.** It is left as a named non-goal for this row (it changes no behaviour) and banked.

⚠️ **NO H/2 CARRIER CAN REACH `writeH3Reply`** — call-graph verified across four pathspec-scoped greps. So this row's fix cannot affect the H/3 leg.

⚠️ **WHAT IS GENUINELY UNMEASURED IS NARROWER THAN THE BRAINSTORM SAID:** what the **reference** emits for `content-length` on an **HTTP/3 empty-body local reply** has never been observed in this repo, on either the fixture or the contract side. The repo's only H/3 fixture (`0104-http3-downstream-get`) is a `direct_response` GET→200 **with a body**, and its README and `expectations.yaml` place response-header-set equality under `## UNasserted`. **Closing that needs a new H/3 fixture driving a local reply, and the H/3 wire is QPACK-compressed below the seam.**

⚠️ **AND ONE MORE HAZARD, RECORDED:** `writeH3Reply` propagates a stale carrier `content-length: 0` verbatim alongside a real body (measured: `CL: 0` shipped beside a 58-byte body). It neither corrects nor rejects. **This is why a placeholder is unsafe as a general pattern in this tree** — §4's decision is the H/2 instance of the same principle. *(Rider: `httptest.ResponseRecorder` does not enforce Content-Length, so what quic-go's real `http3` writer does is unmeasured.)*

---

## 8. Cost — the complete file set, MEASURED and ESTIMATED labelled separately

⚠️ **`reference_measured_prototype_is_a_lower_bound` HAS NOW FIRED SIX CONSECUTIVE ROWS, ALWAYS BY UNDER-ENUMERATING *FILES*.** The BRAINSTORM's `+3 / -0` floor is **one of at least seven files**.

### MEASURED

| # | file | +add / -del | note |
|---|---|---|---|
| 1 | `internal/filter/http/router/router_h2.go` + `retry.go` | **`+10 / -8`** (2 files) | §4's pick, compiling prototype |
| 2 | `internal/filter/http/router/router_h2_local_reply_cl_test.go` **(NEW)** | **`+117 / -0`** | §6, both directions proven |
| 3 | `test/fixtures/0004-h2-routing/driver/driver.go` | **`+300 / -44`** | §5; +121 code / +171 comment / +8 blank |
| 4 | `test/fixtures/0004-h2-routing/driver/driver_test.go` | **`+57 / -10`** | table 10 → 18 rows |

⚠️ **ITEM 1 IS THE §4 PICK, MEASURED SEPARATELY FROM THE DOC DE-ROT.** The helper's doc comment claims it *"preserves the three-header insertion order (Content-Type, Date, Server)"* — **which the fix falsifies.** A de-rot was measured at `+2 / -2` on the ARM-2 base. ⇒ **the honest combined production patch is ~`+12 / -10`, ESTIMATED**, because the two were not measured as one patch. ⚠️ **This is also why the BRAINSTORM's `-0` cannot survive an honest patch:** any de-rot costs deletions.

### ESTIMATED — labelled, not measured

| # | file | estimate | basis |
|---|---|---|---|
| 5 | `test/fixtures/0004-h2-routing/README.md` | ~`+18 / -8` | 8 measured lines need rewriting: `:40, :137, :139, :141, :143, :145, :147, :153` |
| 6 | `docs/envoy-go/DECISIONS.md` — ADR-0315 §Context | ~`+12 / -0` | comparable: ADR-0313's §Context shape |
| 7 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` — narrative rider | ~`+3 / -0` | §11 |
| 8 | `internal/filter/hcm/` — the recompute pin | ~`+60 / -0` | §6.1; nobody has priced this |

**Excludes** the lifecycle files (`ROADMAP.md`, `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`, this `SPEC.md`).

---

## 9. ADR-0315 and the guard — ⚠️ THE ARMING DIRECTION IS **0 -> 1**, NOT 1 -> 2

**ADR-0315 — HTTP/2 local-reply `Content-Length`: always emit, AT THE COMPOSER, WITH THE BODY LENGTH (phase 93).** §Context drafted at this SPEC; §Decision + §Consequences land IN PLACE at the IMPL after the RETAINED italic footer, no renumber and no `---` separator. ⇒ `^---$` STAYS **216**; `^## ADR-` **313 -> 314**; bare `^## ` **321 -> 322**; tail becomes **ADR-0315**.

⚠️ **THE ROUTER'S INSTRUCTION TO "ARM THE STRICT `PROPOSED` GUARD 1 -> 2" CONFLATES TWO DIFFERENT REGEXES.** Measured at this tip:

| form | reads | what it matches |
|---|---|---|
| `^> \*\*STATUS: PROPOSED` — **the house form, the one ADR-0315 will match** | **0** | the ADR-0294-0314 block shape |
| `^\*\*Status:\*\* PROPOSED` — **the ADR-0231 decoy** | **1**, at `:14866` under `## ADR-0231` (`:14864`) | a phase-33-era line, byte-untouched |
| union of both | 1 | — |

⇒ **The guard ADR-0315 arms goes `0 -> 1`. The decoy stays `1`. Only a UNION form reads `1 -> 2`.** ⚠️ **VERIFY BY LINE AND BY ADR, NEVER BY THE COUNT ALONE** — a session gating on the decoy form will read `1` after a correct arming and conclude it failed.

This is `ADR-0312 §Consequences (x)` firing again — it recorded exactly this two-form hazard in the **disarm** direction; **this stage is the first to hit it in the ARM direction.**

⚠️ **NEVER GATE ON THE UNANCHORED FORM** — it reads **90 lines / 101 occurrences**. ⚠️ **And a third, middle-ground form `^\*\*Status:\*\*.*PROPOSED` reads 23** — anchored yet still fail-unsafe. **Any gate must NAME WHICH FORM it uses.**

**Next-free ADR is TAIL-derived:** tail `## ADR-0314`; `grep -c '^## ADR-0315'` ⇒ **0**; a full id sweep finds **exactly one gap (`0209`)**, so headings+1 reads **314 — the TAIL ITSELF, a TAKEN id.** ⚠️ **NEVER derive from the heading count.**

### 9.1 §Context outline (drafted at this SPEC)

¶1 the defect and its internal-inconsistency framing · ¶2 the mechanism by symbol · ¶3 **the doctrine's correct citation (ADR-0155, attributed to ADR-0085)** · ¶4 **why the composer and not the writer** (the compressor-strip hazard on the proxied path) · ¶5 **why the body length and not a placeholder** — the access log, the compressor at `min_content_length ∈ [1,12]`, and the carrier-exporting filters · ¶6 **the refutation of "inert twice over"** · ¶7 the instrument and its named blind spot (§5.2) · ¶8 the H/3 layer composition and the comment defect · ¶9 cost and the sixth consecutive lower-bound firing.

---

## 10. Cite hygiene — what the PLAN must NOT inherit

### 10.1 Anchors that are wrong at this tip

⚠️ **FOUR BANKED ANCHORS POINT AT COMMENT LINES OR AT THE WRONG DECLARATION**, and two constants are cited **26 lines off**:

| symbol | banked | ACTUAL | drift-proof anchor |
|---|---|---|---|
| `p92WantRefCLFields` | `:1408` | **`:1382`** | the literal `p92WantRefCLFields  = 1` |
| `p92WantSubjCLFields` | `:1409` | **`:1383`** | the literal `p92WantSubjCLFields = 0` |
| `p92ContentLengthFields` | `:1339` | **`:1345`** | `func p92ContentLengthFields(` |
| `writeH3Reply` | `:56` | **`:33`** | `func writeH3Reply(` |
| `beginLocalReply` | `chain.go:1313` | **`:1288`** | `func (c *FilterChain) beginLocalReply(` — and the file is `internal/filter/http/chain.go` |
| `directResponseAction.body()` | `:95` | **`:91`** | `func (a *directResponseAction) body()` |

⚠️ **`:1408` in reality is `bad = append(bad, …)` inside a loop.** This is the `ADR-0313 §Consequences (xii)` species: *"an off-by-one that lands on a comment reads as a correct cite."*

⚠️ **`codec.go:87-89` IS DUPLICATED AS A LINE-ANCHOR INSIDE PRODUCTION CODE** at `compressor.go:780`. Renumbering `codec.go` rots a shipped comment too.

### 10.2 Two archive-guard findings the close must apply

⚠️ **THE STRICT `163` IS NOT THE ENTRY COUNT.** Measured: bullet-anchored form **216** = strict **163** + parenthetical-shape **53**, exactly. **216 is the true entry count; 163 is a subset.** The guard remains SOUND as a **delta-0 shape check** — a correctly-shaped parenthetical append moves it by 0 — but **any prose calling 163 "the entry count" is wrong.** The three forms disagree (strict 163 / colon 165 / loose 216) and **any gate must name WHICH it uses**; the colon form carries **two false positives** (lines that quote the label inside their own body).

⚠️ **THE ARCHIVE'S POSITIVE CONTROL IS SELF-INCREMENTING.** `phase 88 (h2-continuation-frames) SPEC done` read **7** at `40cf6766` and reads **8** now, because the phase-93 BRAINSTORM's own archive line QUOTES the control's subject. **16 archive lines name a control this way.** ⇒ **a control figure recorded in the file it measures is invalidated by the act of recording it.** The close must either not name its control, or name it AND disclose the +1-per-use, so the next session does not read the drift as a broken control.

---

## 11. D-93-CONTRACT — a narrative rider, priced

A `## HTTP/2 local-reply Content-Length (phase 93)` section appended at the file tail, on the `## HTTP/2 response trailer forwarding (phase 84.1)` precedent. ~`+3 / -0`, **ESTIMATED**.

⚠️ **NO LEDGER LINE AND NO ABSOLUTE.** The row registers no counter, so the stat surface moves **+0**, and that claim is **structural**, not arithmetic. ⚠️ **Three different stat-surface absolutes are live in this tree at one tip and the contract warns of itself that a re-derivation *"should expect the re-derived figure to disagree."* Per `reference_a_drift_correction_is_itself_a_claim`, on a contested count: NO NUMBER.**

The rider states: the H/2 local-reply composer emits `Content-Length` unconditionally, valued at the body length; the value is observed at the encode seam and by the access log before the wire writer recomputes it; local-reply **body bytes** remain the ratified relaxation they already are.

---

## 12. Gate discipline the PLAN must carry

1. ⚠️ **`-count=1` ON EVERY `test/differential` INVOCATION, UNCONDITIONALLY.** The harness builds envoy-go as a **subprocess**, so a router edit is **not a compile-time input** to that test binary and Go's cache serves a stale PASS. **A fix whose own gate cannot see it is the most dangerous form of this trap.**
2. ⚠️ **`go test ./...` DRIVES DOCKER — AND THERE ARE **TWO** DOCKER DRIVERS, NOT ONE.** Besides `test/differential`, `test/conformance/h2spec/h2spec_test.go` also drives Docker. **The exclusion must name both:** `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`. *(The BRAINSTORM's recipe named only the first.)*
3. ⚠️ **`go test` WITHOUT `-v` PRINTS ZERO `=== RUN`** — `RUN=0` beside `RC=0` is a VACUOUS GREEN. **Assert the denominator.**
4. ⚠️ **A `-run` SELECTOR MATCHING NOTHING PRINTS `[no tests to run]` AND EXITS 0.** Assert the selector matched.
5. ⚠️ **UNANCHORED `grep -c 'FAIL'` READS NONZERO ON A GREEN TREE** — use `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`.
6. ⚠️ **`gofmt -l` NEVER EXITS NON-ZERO — GATE ON OUTPUT.** `golangci-lint` does exit non-zero here, but **still gate on output**. ⚠️ **Its misspell runs in locale US and FIRED ON THIS STAGE'S OWN PROTOTYPE** (`favour` → `favor`). **Sweep British spellings in `.go` comments before the gate.**
7. ⚠️ **THE FIXTURE SET MUST BE RECONCILED BY NAME, BOTH DIRECTIONS** — the runner `t.Skipf`s an unregistered fixture and **no fixture-count gate exists anywhere in the tree**. Assert `--- PASS: TestDifferential/0004-h2-routing` **explicitly**.
8. **The anchored panic gate `^panic:|DATA RACE|SIGSEGV` on every differential launch.** ⚠️ **`-race` on the differential suite is VACUOUS** — the subject is an unraced subprocess.
9. ⚠️ **NC EVERY NEW ASSERTION, AND NC THE FIXES TOO** — neutralise rather than build-break, so the package still compiles.

---

## 13. Sentinel — RUN MECHANICALLY AT `56c4d255`, ACTUAL OUTPUT

- **(1)** `NOT DONE: row 93` — **ONE** line at `want=125`. ⇒ correct while row 93 is open.
- **(2)** **SIX** windows, at `:203 :209 :215 :225 :231 :239`. ⚠️ **The SPEC adds no ROADMAP row, so anchors and content are UNCHANGED** — per-line md5 baselined, with a doctoring NC proving the comparator discriminates.
- **(3)** SILENT.

**All four mandated NCs run, ACTUAL output:**
- **NC-A** (doctor row 62): **TWO** lines — `NOT DONE: row 62` then `NOT DONE: row 93`.
- **NC-B** (`want=124` on the real file): **TWO** lines — `NOT DONE: row 93` + `GATE FAIL: examined 125 data rows, expected 124`.
- **NC-C** (check-3 doctored): **FIRED** — `NEVER OPENED: gRPC`, WASM control **2**.
- **NC-D**: long **5** / short **1** / union **6**.

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent at the git root and in all six worktrees.

⚠️ **THE MARGIN IS TWO ONLY WHILE ROW 93 IS OPEN.** The moment row 93 flips `done`, check (2) is again the ONLY thing blocking termination. **Do not "tidy" a deferred-candidate line.**

**Counts re-derived at this tip** (every axis measured, with its named NC): ROADMAP **243** lines / **125** rows / tail **93** · phase dirs **134** · fixtures **121** (tail `0119`, **`0120` FREE**) · fuzzers **56 / 48** · BackendKind tail **38** (39 constants, values 0-38) · `go.mod` **67** (18 direct + 49 indirect) · `DECISIONS.md` **18646**, `^---$` **216**, `^## ADR-` **313**, bare `^## ` **321** (= 313 + 8 `## Amendment`) · `BEHAVIOR_CONTRACT.md` **5966** · `STATE.md` **63** · `STATE_HISTORY.md` **532** · `-family row` **95 occurrences / 67 lines** · malformed rows escape-aware **2** (ids 57 NF=9, 69 NF=10; **naive reads 17**), **row 93 NF=8 under BOTH**.

⚠️ **The stat surface is quoted as a DELTA (+0) and NEVER as an absolute.** ⚠️ **THE REFUTED STAT-SURFACE FIGURE IS DELIBERATELY NOT SPELLED HERE, IN EITHER ITS BARE OR ITS ARROW FORM.** A prohibition that quotes the token it prohibits FALSIFIES ITSELF on its own grep — the species ADR-0297 ¶7 records, and this sentence was written the wrong way first and corrected by running the gate on this file.

---

## 14. What the PLAN owes

1. **Land the §4 reversal explicitly** — the PLAN must not silently re-adopt ARM 2. If it disagrees, it must refute §4.2's access-log measurement **by execution**.
2. **Measure the differential golden churn** for the fixture instrument. §4.6 establishes it is the same for both designs; **it is UNMEASURED in absolute terms.**
3. **Confirm `p93WantRefBodyLen` on a live `-count=1` run** (§5.3). It is a claim today.
4. **Price and land the `writeH2Reply` recompute pin** (§6.1) — the row's mechanism rests on behaviour nothing pins.
5. **Correct README `:147` on its CONSEQUENCE, not its count** (§2.1) — and note the clause becomes half-true under §4's pick.
6. **Decide whether the H/3 comment de-rot** (§7) rides this row or is banked separately. **It changes no behaviour**; the SPEC's default is banked.
7. **Re-derive every anchor in §10.1 at the PLAN's own tip.** They drifted once; they will drift again.
8. **Carry §12's nine gate rules verbatim**, especially the TWO Docker drivers.
