# Phase 92 — `h2-response-header-validation` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** On the HTTP/2 downstream leg, REJECT a response whose **upstream leading header block** carries a
connection-specific or otherwise malformed field — with a **502, an evicted pooled connection, and NO
retry** — instead of laundering it onto the downstream stream.

**Architecture:** A new package-level validator `validateResponseHeaders` in `internal/filter/hcm/h2`,
called from the `!cs.respHeadersSeen` arm of `(cc *ClientConn).onResponseHeaderBlock`, returning a
stream-scoped `*Error` carrying a NEW sentinel `ErrMalformedResponseHeaders`; a THIRD arm in
`doH2ClusterAction` (`internal/filter/http/router`) placed AFTER `EvictH2ConnOnError` that turns that
sentinel into a 502 **without** `localOrigin`; and ONE new cluster-scope counter so the rejection is
distinguishable from every other 502.

**Tech Stack:** Go, `golang.org/x/net/http2` + `hpack`, the in-tree differential harness
(`test/differential`, fixture `0004-h2-routing`), `envoyproxy/envoy:contrib-v1.37.2` as the reference pin.

**Spec:** `docs/envoy-go/phases/92-h2-response-header-validation/SPEC.md` (586 lines). Read §4
(D-92-POSTURE), §5 + §5.1 (D-92-VALIDATOR and the OPEN ARM), §7 (D-92-DIFF), §10 (the NC table), §13.

## Global Constraints

- Reference pin: **`envoyproxy/envoy:contrib-v1.37.2`** (`docs/envoy-go/ENVOY_TARGET.md` — note it lives at
  `docs/envoy-go/`, NOT the repo root).
- **ROW 92 flips `done` at THIS phase's IMPL, not before.** `want` stays **124**.
- **A PLAN ADDS NO ADR.** ADR tail stays **ADR-0314**, next-free **ADR-0315**, `^---$` stays **216**, and
  the strict `^> **STATUS: PROPOSED` guard stays **ARMED at 1** under `## ADR-0314`. The ADR-0231 decoy
  also reads 1 and must be LEFT ARMED.
- `go.mod` / `go.sum` **BYTE-UNTOUCHED**.
- **No new `BackendKind`** (tail stays **38**), **no new fixture directory** (fixtures stay **121**),
  **no new port allocation**, **no YAML change**.
- `golangci-lint`'s **misspell runs in locale US** — British spellings in `.go` comments FAIL the gate.
  Markdown prose may use them freely.
- Every gate is judged on **OUTPUT**, not exit code: `gofmt -l` never exits non-zero, and `golangci-lint`
  must be read the same way.

---

## 0. THE HEADLINE — this PLAN closes the SPEC's open arm and refutes **TEN** inherited claims

1. ⚠️ **THE OPEN ARM IS CLOSED AND THE BROAD RULE SURVIVED.** The reference REJECTS an **identical**
   duplicate `content-length` with 502, byte-identical to the differing-value arm. The leg ships as
   prototyped, now on a measurement. (§2)
2. ⚠️ **THE `te` LEG HAD NEVER BEEN MEASURED IN THIS DIRECTION — AND MEASURING IT ADDS *TWO* SHAPES.**
   `te: gzip` ⇒ **502** and `te: ""` ⇒ **502**, while `te: trailers` is **FORWARDED VERBATIM**.
   ⇒ **THE ROW'S DIVERGENCE SET IS *NINE* SHAPES, NOT SEVEN.** The leg was riding on RFC authority in a
   validator whose sibling's own doc comment forbids exactly that. (§2.3)
3. ⚠️ **AN EIGHTH-CLASS SHAPE FELL OUT AND IS SCOPED OUT IN WRITING.** A **single** `content-length` with a
   wrong value: reference **200-headers-then-RST_STREAM**, subject silent rewrite to a clean 200.
   ⇒ **duplicate-NESS, not value-CORRECTNESS, trips the 502.** It belongs to the banked
   `content-length`-rewrite candidate — which this makes **FOUR-legged** — not to this row. (§2.1)
4. ⚠️ **THE ROW TOUCHES A *THIRD* FILE THE SPEC DID NOT KNOW ABOUT, AND IT IS A RED GUARD, NOT A CHOICE.**
   `router_h2_trailers_test.go:528` is an **AST-audit golden** pinning the exact MULTISET of
   `ActionResponse` literal `Status` values inside `doH2ClusterAction`. **A cost prototype that skipped the
   test run would have reported the arm as `+35/−0` and missed a mandatory file entirely.** (§4.5)
5. ⚠️ **THE VALIDATOR WOULD SHIP *COMPLETELY UNGATED*.** Adding a real validator rejecting five distinct
   shapes moves **ZERO** of the suite's **655** tests, RUN SETS identical. **A `t.Skip`, an inverted leg,
   or deleting the whole function body would leave every row green.** The unit table is not a
   nice-to-have — **it is the only thing that can fail.** (§10.2)
6. ⚠️ **THE COST FIGURES ARE SUPERSEDED. `+74`/`+77` BECOME `+174 / −1`** across THREE files in TWO
   packages, measured by one compiling prototype of the frozen design. (§10.1)
7. ⚠️ **THE SPEC'S COUNTER-NAMING PREMISE IS WRONG. `http2_tx_reset` EXISTS IN NO `.go` FILE.**
   envoy-go registers **DOTTED** names (`http2.rx_reset`, `http2.tx_reset`); the underscore form is the
   Prometheus-flattened projection. The reference's exact leaf `http2.rx_messaging_error` is therefore
   **convention-native**, not a departure. (§5.1)
8. ⚠️ **`406` IS NOT THE STAT SURFACE, AND IT IS NOT REPRODUCIBLE.** It appears **ZERO** times in
   `BEHAVIOR_CONTRACT.md`, whose ledger tail reads **1207**; its provenance is an **unnamed grep** quoted
   in ADR-0313 as *"406 occurrences … by the same command"*. **Seven candidate commands at this tip read
   143 / 324 / 401 / 402 / 403 / 404 / 699 — none is 406.** ⇒ **this row asserts the `+1` DELTA
   structurally and cites NO absolute.** (§5.5)
9. ⚠️ **A NEGATIVE CONTROL THE BRIEF PROPOSED IS VACUOUS: AN UPPERCASE STAT NAME *PASSES* `IsValidName`.**
   `a-zA-Z` appears in both character classes. Any gate using uppercase as its NC proves nothing. (§5.3)
10. ⚠️ **ONE INHERITED SENTINEL FIGURE IS STALE — the UNANCHORED `PROPOSED` count reads 3, not 2** — and it
   went stale **inside the very ADR that documents it**. (§11.1)

**And two things SURVIVED re-derivation intact, recorded because a clean result is evidence too:** the
SPEC's §3 symbol table is **9/9 exact, ZERO drift**, and every other count in §11 **MATCHES**. That is a
reason to keep re-deriving, not to stop.

---

## 1. SENTINEL — RUN MECHANICALLY AT THIS TIP (`8c18100c`), ACTUAL OUTPUT

### 1.1 The three checks

- **(1)** `want=124` ⇒ **`NOT DONE: row 92`** — **ONE line**, no `GATE FAIL`.
- **(2)** **SIX**, at `:202 :208 :214 :224 :230 :238`.
- **(3)** **SILENT.**

⇒ **TWO checks block the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent
at the git root AND in the stage worktree.

⚠️ **Check (1) printing `NOT DONE: row 92` IS THE OPEN-ROW BOARD'S CORRECT SHAPE.** It is not breakage and
must not be "fixed" by flipping row 92 — **the row flips `done` at the IMPL, not at this PLAN.**

### 1.2 All four NCs FIRED — ACTUAL output

- **NC-A** (row 62 doctored to `in-progress`): `NC LANDED? [ in-progress ]` **INSPECTED FIRST**, then
  **TWO** lines — `NOT DONE: row 62` AND `NOT DONE: row 92`.
  ⚠️ **This is the FIFTH distinct NC-A shape in six stages.** The one-line expectation belongs to an
  all-`done` board and does not carry. **Read it yourself; never inherit the expected shape.**
- **NC-B** (`want=123` on the real file): **TWO** lines — `NOT DONE: row 92` AND
  `GATE FAIL: examined 124 data rows, expected 123`.
- **NC-C**: residual `gRPC-family row` **2 -> 0** ⇒ `NEVER OPENED: gRPC   <- NC FIRED`; the WASM control
  correctly stayed **SILENT** (only gRPC printed).
- **NC-D**: alternation split — long **5** / short **1** / union **6**.
  (⚠️ the short pattern is not a substring of the long one: `deferred (not-yet-chartered) candidates:`
  does not contain `deferred candidates:`, so the split is sound.)

Row 92 field-counts **NF=8 under BOTH** forms. Malformed-row baseline **2** escape-aware (ids **57**, **69**)
and **17** naive; ⚠️ the forms **DISAGREE on row 57** (naive **13** vs escape-aware **9**) but **AGREE on
row 69** (both **10**) — so **any gate must state WHICH FORM it uses**, and a gate asserting `== 0` FAILS
on pre-existing content.

---

## 2. THE OPEN ARM IS CLOSED — the BROAD rule STANDS, and an EIGHTH shape fell out

SPEC §5.1 left exactly one design question open and forbade shipping past it: **the prototype rejects ANY
second `content-length`, including two with IDENTICAL values, and whether the reference does the same was
UNMEASURED.** The measured arm had used DIFFERING values.

**MEASURED at this PLAN, on `contrib-v1.37.2`, with every control the SPEC demanded.**

| arm | upstream leading block | reference | subject (envoy-go) |
|---|---|---|---|
| **C — positive control, FIRST and LAST** | single `content-length: 5`, 5-byte body | **200**, `cl: 5` | **200**, `cl: 5` |
| **A — IDENTICAL duplicate** | `cl: 5`, `cl: 5` | **502** `reset reason: protocol error` | **200**, `cl: 5` **TWICE** |
| **B — DIFFERING duplicate** (the BRAINSTORM's known-502 control) | `cl: 5`, `cl: 99` | **502**, byte-identical to A | **200**, `cl: 5` **TWICE** |
| **D — identical duplicate, both WRONG** | `cl: 99`, `cl: 99`, 5-byte body | **502**, byte-identical to A | **200**, `cl: 5` **TWICE** |
| **E — SINGLE `content-length`, WRONG value** | single `cl: 99`, 5-byte body | **200 headers, then RST_STREAM(INTERNAL_ERROR), NO DATA** | **200**, `cl: 5` |

⇒ **DECISION: the BROAD rule SHIPS AS PROTOTYPED — reject ANY second `content-length`.** The reference
exercises RFC 9110 §8.6's *reject* option, not its *treat-as-single* option. **Do NOT narrow the leg to
"duplicate with DIFFERING values"** — narrowing it would re-introduce a divergence on arm A.

**The controls, because a result without them is not evidence:**
- The **differing-value arm B reproduced the known 502 on the SAME instrument, SAME session, SAME
  container**, so arm A's 502 cannot be a broken probe. This was the load-bearing control.
- The **direct-to-backend arm** observed `content-length-field-count=2` with the proxy bypassed entirely,
  separating *"the proxy rejected it"* from *"my backend never emitted it"*.
- Positive control ran **first AND last**, both 200.
- **FOUR reference runs are byte-identical**, `sha256` of the captured section equal across all three
  comparisons.
- Reference admin stats corroborate the mechanism, not just the status:
  `cluster.c_be.http2.rx_messaging_error: 12` / `upstream_cx_protocol_error: 12` / `upstream_rq_5xx: 12`
  over `upstream_rq_completed: 24` (4 runs × 6 arms, 3 of which reject).

### 2.1 ⚠️ TWO FINDINGS THAT REFUTE THE INHERITED WORDING — do not re-inherit it

1. ⚠️ **THE BRAINSTORM'S ROW-10 *"both forwarded"* IS ABOUT THE FIELD COUNT, NOT THE VALUES.** `writeH2Reply`
   rewrites **every** `content-length` to the observed body length and then emits them **all**, so on arm B
   the subject wire carries `content-length: 5` **twice** — not `5` and `99`. **A differential or unit
   assertion written over VALUES is blind to arms A, B and D alike.** The assertion must be over the
   **COUNT of `content-length` fields**, and this is why arm 10's pin lives at the unit layer where the
   pre-rewrite slice is visible (SPEC §3.2).

2. ⚠️ **ARM E IS AN EIGHTH DIVERGENT SHAPE, AND IT IS NOT ONE THIS ROW FIXES.** A **single**
   `content-length` whose value disagrees with the body is **not** a duplicate, is **not** a 502, and is
   **not** reached by this row's validator: the reference sends 200 headers and then RST_STREAMs with no
   DATA, while the subject silently rewrites the value to the true body length and serves a clean 200.
   ⇒ **duplicate-NESS, not value-CORRECTNESS, is what trips the reference's 502.**
   **This row scopes arm E OUT, deliberately and in writing.** It belongs to the already-banked
   **`content-length` rewrite** candidate (the THREE-LEGGED one — `304` with a body-less rewrite, the HEAD
   shape, and the mismatched value), whose in-tree site is `h2dispatch.go:1014-1016` and whose identical
   rewrite exists on **all three codecs** (`codec.go` H1, `h2dispatch.go` H2, `h3dispatch.go` H3).
   **Arm E is new evidence FOR that candidate and must be recorded on it, not smuggled into this row.**

### 2.2 Harness facts this measurement established — the IMPL inherits them

- ⚠️ **`getent ahostsv4 host.docker.internal` returns NOTHING on the host and `docker0` DOES NOT EXIST on
  this machine.** The host-gateway IP had to be resolved **from inside a throwaway container**:
  `docker run --rm --add-host=host.docker.internal:host-gateway --entrypoint /bin/sh <img> -c "getent ahostsv4 host.docker.internal"`
  ⇒ **`192.168.65.2`**, confirmed at this tip.
- ⚠️ **THE PROBE BACKEND MUST BIND `0.0.0.0`, NOT `127.0.0.1`.** A loopback bind is unreachable from the
  container via `192.168.65.2` and produces a connect failure **that looks exactly like an envoy-side
  rejection**. This is fail-unsafe and cost real time to find.
- `-p 127.0.0.1:<host>:<container>` publishing **works**, re-confirmed.
- The subject logs `hcm: h2: h2: PROTOCOL_ERROR: short preface: EOF` at shutdown — that is the probe
  killing the process mid-connection, **not an arm result**. Anyone grepping the subject log for
  `PROTOCOL_ERROR` will otherwise mis-read it.

### 2.3 ⚠️ THE `te` LEG WAS NEVER MEASURED IN THIS DIRECTION — IT IS NOW, AND IT ADDS **TWO** SHAPES

**THE GAP.** The frozen validator has a `te` leg (`name == "te" && value != "trailers"` ⇒ reject), carried
over from `validateResponseTrailers` as one of the SPEC's three "carrying" predicates. **But the
BRAINSTORM's seven measured shapes are arms 2-6 (connection-specific), 7 (uppercase) and 10 (duplicate
`content-length`). THERE IS NO `te` ARM AMONG THEM.** The leg was riding on RFC authority alone.

⚠️ **THAT IS EXACTLY WHAT THIS CODEBASE FORBIDS.** `validateResponseTrailers`'s own doc comment says the
enforced set is *"MEASURED against the ADR-0008 reference pin … **not derived from the RFC text, because
the two disagree**"* — and then lists `host` and `trailer` as fields the RFC bars but the reference
**FORWARDS**. Shipping an unmeasured reject leg risks the divergence running the OTHER WAY: the subject
rejecting what the reference forwards, which would redden the differential against a correct implementation.

**MEASURED, on the same instrument, in the same session, with the same controls:**

| arm | upstream leading block | **reference** | **subject** | verdict |
|---|---|---|---|---|
| **T1** | `te: trailers` | **200**, forwarded **verbatim** | **200**, forwarded | **PARITY — must NOT be rejected** |
| **T2** | `te: gzip` | **502** `reset reason: protocol error` | **200, LAUNDERED** | **DIVERGENT — new** |
| **T3** | `te: ""` (present, EMPTY value) | **502** | **200, LAUNDERED** | **DIVERGENT — new** |
| **T4** | no `te` (positive control, FIRST and LAST) | **200** both | — | control OK |
| **T5a/b** | `connection: keep-alive` / duplicate `cl` | **502** / **502** | — | known-502 controls **REPRODUCED** |

⇒ **THE LEG SHIPS AS WRITTEN, now on a measurement.** `te` with any value other than `trailers` is
rejected; `te: trailers` is forwarded. **Do NOT broaden it to reject `te` unconditionally** — T1 proves
that would create the opposite-direction divergence.

⚠️ **THE ROW'S DIVERGENCE SET IS THEREFORE **NINE** SHAPES AT 502, NOT SEVEN.** The inherited "seven
shapes" wording is superseded: five connection-specific names, an uppercase name, a duplicate
`content-length`, **`te` with a wrong value**, and **`te` with an EMPTY value**.

⚠️ **PIN `te: ""` SEPARATELY. IT IS THE ARM AN IMPLEMENTER IS MOST LIKELY TO GET WRONG** — writing
`value != "" && value != teTrailersValue` looks like defensive hygiene and is **measurably wrong**. The
frozen predicate in §4.2 already handles it correctly; the test must prove it stays that way.

**Controls, again in full:** direct-to-backend observed `te="gzip"`, `te="trailers"` and `te=""` on the
wire with the proxy bypassed (so "the backend never emitted it" is excluded, and the EMPTY value is
confirmed as a PRESENT field rather than an absent one); positive control 200 first and last; **three
reference runs byte-identical** (`sha256` equal); subject runs byte-identical across two.
Admin stats corroborate the mechanism — `http2.rx_messaging_error`, `upstream_cx_protocol_error` and
`upstream_rq_5xx` each **12** over `upstream_rq_completed: 21` = 3 runs × the **4** rejecting arms of 7.
⇒ **the `te` rejects land in the same upstream-CODEC bucket as the connection-specific and duplicate-CL
shapes**, which is what makes them the same defect rather than a coincidence of status codes.

---

## 3. THE MECHANISM — re-derived at this tip

**Every symbol cite in this PLAN was re-verified at `8c18100c`: 9/9 EXACT, ZERO DRIFT (§11.4).** Cite BY
SYMBOL, not by line — and ⚠️ **a symbol assertion whose receiver is written `(cc *ClientConn)` MUST use
`grep -F`**: ERE reads the parentheses as a group and returns a **FAIL-UNSAFE ZERO** (re-reproduced here:
`-F` reads **1**, `-E` reads **0**).

**THE ASYMMETRY, which is the whole defect.** `(cc *ClientConn).onResponseHeaderBlock`
(`client.go:609`) branches on `cs.respHeadersSeen`:

- the **TRAILING** block is validated by `validateResponseTrailers` (`:788`) and rejected with a
  stream-scoped `*Error` carrying `ErrMalformedTrailers` (`:716`);
- the **LEADING** block is stored verbatim as `cs.respHeaders = decoded` and **validated by NOTHING.**

`doH2ClusterAction` (`router_h2.go:73`) then forwards the stored set **stripping only `:`-prefixed names**,
and `writeH2Reply` (`h2dispatch.go:1004`) applies **no RFC 9113 §8.2.2 filter** — it only lowercases names
and rewrites `content-length` to `len(body)` (`:1014-1016`).

⇒ **THERE IS NO VALIDATION ANYWHERE ON THE LEADING-BLOCK ENCODE PATH**, and envoy-go therefore generates a
message that RFC 9113 §8.2.2 requires every conformant client to treat as malformed:
*"An endpoint MUST NOT generate an HTTP/2 message containing connection-specific header fields"*, and a
receiver *"MUST treat a message containing connection-specific header fields as malformed."*
The reference detects the same bytes **at its upstream codec** and answers **502 before anything reaches
the downstream stream**.

⚠️ **THE CHARTER IS CLOSED UNDER ONE CODEC PAIR, AND THAT WAS RE-DERIVED, NOT ASSUMED.**
`AcquireH2Stream` has exactly **two** non-test occurrences repo-wide — its definition
(`internal/cluster/h2pool.go`) and its single call site in `doH2ClusterAction`. Two nuances are recorded
rather than smoothed:

- ⚠️ **"Unconditionally" is true of the CODEC PAIR and FALSE of the CALL COUNT.** `H2ClusterAction`
  dispatches through hedge / retry / direct arms, **all three converging on `doH2ClusterAction`**, so the
  validator is **RE-ENTERED ONCE PER ATTEMPT.** This is load-bearing for D-92-POSTURE (§4.4): it is what
  turns a retry misclassification into three backend-observed attempts rather than two.
- ⚠️ **AN H3-IN / H1-OUT PATH DOES EXIST** (`h3dispatch.go` calls the H1 `asRouterAction()` builder, with
  an in-tree comment saying so). H/3 is **outside this charter**, but **no prose in this row may claim the
  tree is free of downstream/upstream codec mismatch in general.**

⚠️ **`isConnectionSpecificField` lives in `h2/stream.go:392`, CO-LOCATED with `IsIllegalH2RequestHeader`
(`:414`)** — the decode-direction mirror this row mirrors on the encode side. The new validator has a
natural home beside them, and D-92-VALIDATOR (§4.2) explains why it does **not** go there.

---

## 4. THE FROZEN DESIGN — production

### 4.1 `internal/filter/hcm/h2/client.go` — the sentinel and its constructor

Placed immediately after the existing `ErrMalformedTrailers` block so the two sentinels read as a pair.

```go
// ErrMalformedResponseHeaders is the sentinel every LEADING response header
// block rejection carries in the Underlying field of its stream-scoped *Error.
//
// ⚠️ IT IS DELIBERATELY NOT ErrMalformedTrailers, AND THE DIFFERENCE IS A
// MEASURED BEHAVIOUR, NOT A NAMING PREFERENCE. router_h2.go's sentinel arm for
// ErrMalformedTrailers returns Status: 0 — a downstream STREAM RESET. The
// reference answers a malformed LEADING block with 502 (measured on
// contrib-v1.37.2, posture-INVARIANT), so this sentinel selects a THIRD arm.
// Reusing ErrMalformedTrailers would silently ship the wrong status.
var ErrMalformedResponseHeaders = errors.New("malformed response headers")

// malformedResponseHeadersError builds the stream-scoped rejection, mirroring
// malformedTrailersError exactly: INTERNAL_ERROR, a NON-ZERO stream id (a
// connection-scoped error carries 0 and would tear the pooled conn down from
// inside the codec, which is NOT how the reference books this), and the
// ErrMalformedResponseHeaders sentinel in Underlying.
func malformedResponseHeadersError(streamID uint32, msg string) *Error {
	return &Error{
		Code:       ErrInternalError,
		Stream:     streamID,
		Msg:        msg,
		Underlying: ErrMalformedResponseHeaders,
	}
}
```

### 4.2 `validateResponseHeaders` — the validator, and why every leg is where it is

**D-92-VALIDATOR: a SECOND, INDEPENDENT function sharing the three carrying predicates.** Not a mode
flag on `validateResponseTrailers`, not a thin wrapper. The SPEC's three reasons, restated because an
implementer will be tempted by the "share more" move:

1. Of `validateResponseTrailers`'s six legs, **three carry and three INVERT** — the END_STREAM framing
   leg (a leading block has no such requirement), the pseudo-header ban (`:status` is REQUIRED here), and
   the flat `content-length` ban (exactly ONE `content-length` is LEGAL here). A boolean that changes the
   majority of a function's legs is a second function wearing one name.
2. **The duplicate-`content-length` rule is a CROSS-FIELD COUNT, not a predicate.** It cannot be
   expressed in a `switch` over one `(name, value)` pair at all. That alone forecloses "one body, one flag".
3. The error CONSTRUCTOR must differ, because sentinel selection in `router_h2.go` is what picks the arm.

⚠️ **DO NOT collapse the connection-specific and `te` legs into `IsIllegalH2RequestHeader`.** That is the
other tempting share, and it REDDENS an existing test: `TestValidateResponseTrailers_Table/te_gzip`
asserts `wantMsgSubstr: "not 'trailers'"` while each connection-specific member asserts its own quoted
name. Merging collapses the two messages into one.

```go
// validateResponseHeaders enforces RFC 9113 §8.2.2 / §8.2.1 on an inbound
// LEADING response header block, returning nil when the block is legal and a
// STREAM-SCOPED *Error (INTERNAL_ERROR, carrying streamID) when it is not.
//
// This is the ENCODE-direction mirror of IsIllegalH2RequestHeader (row 89,
// DECODE direction) and the leading-block sibling of validateResponseTrailers
// above. It shares that function's three CARRYING predicates —
// hasUppercaseHeaderChar, isConnectionSpecificField, and the `te` value rule —
// and deliberately does NOT share its other three legs, which INVERT for a
// leading block:
//
//   - END_STREAM: a leading block carries no framing requirement. A bodyless
//     200 sets END_STREAM; a 200 with a body does not. Both are legal.
//   - pseudo-headers: `:status` is REQUIRED here, not barred.
//   - `content-length`: exactly ONE is legal. Only a DUPLICATE is not, and
//     that is a CROSS-FIELD COUNT across the loop — not a predicate over one
//     (name, value) pair. It is why this cannot be a flag on the trailer
//     validator.
//
// LEG ORDER IS LOAD-BEARING, for the reason the trailer validator documents:
// hasUppercaseHeaderChar runs FIRST because every leg below it is a
// CASE-SENSITIVE string comparison. Without it "Connection" and
// "Content-Length" fall through every other leg untouched.
//
// The message names the offending field QUOTED and in TRAILING position. The
// quoting is load-bearing for falsifiability: an unquoted name is
// unfalsifiable when it also appears inside the message's own fixed prefix.
//
// ⚠️ `host` and `trailer` are barred by RFC 9110 §6.5.1 but FORWARDED VERBATIM
// by the reference — they PASS here, exactly as they do in the trailer
// validator. Do not "complete" this list against RFC 9110.
func validateResponseHeaders(streamID uint32, fields []hpack.HeaderField) *Error {
	contentLengthSeen := false
	for _, hf := range fields {
		name := hf.Name
		switch {
		case hasUppercaseHeaderChar(name):
			return malformedResponseHeadersError(streamID,
				"uppercase character in header field name not permitted in a response header block: "+strconv.Quote(name))
		case isConnectionSpecificField(name):
			return malformedResponseHeadersError(streamID,
				"connection-specific header field not permitted in a response header block: "+strconv.Quote(name))
		case name == "te" && hf.Value != teTrailersValue:
			return malformedResponseHeadersError(streamID,
				"te header field value not 'trailers': te="+strconv.Quote(hf.Value))
		case name == "content-length":
			if contentLengthSeen {
				return malformedResponseHeadersError(streamID,
					"duplicate content-length header field in a response header block: "+strconv.Quote(hf.Value))
			}
			contentLengthSeen = true
		}
	}
	return nil
}
```

⚠️ **`:status` is NOT validated here and that is deliberate.** A missing or malformed `:status` is a
different divergence with its own reference behaviour, unmeasured by this row. Adding it would be an
unpriced behaviour change. The BRAINSTORM's seven-shape reproduction contains no `:status` arm.

---

### 4.3 The CALL SITE — `onResponseHeaderBlock`, and why the mechanics are copied verbatim

`onResponseHeaderBlock` (`client.go:609`) branches on `cs.respHeadersSeen`. The TRAILING block is validated
by `validateResponseTrailers`; **the LEADING block is stored verbatim as `cs.respHeaders = decoded` and
validated by nothing.** `doH2ClusterAction` then forwards the stored set stripping only `:`-prefixed names,
and `writeH2Reply` applies no §8.2.2 filter. ⇒ **there is no validation anywhere on the leading-block
encode path.**

The guard goes inside `if !cs.respHeadersSeen {`, **BEFORE `cs.respHeaders = decoded` retains the block**,
and mirrors the trailer path's reject mechanics **exactly**:

```go
if verr := validateResponseHeaders(streamID, decoded); verr != nil {
	cc.markReset(streamID)
	cc.mu.Lock()
	_ = cc.fr.WriteRSTStream(streamID, http2.ErrCodeInternal)
	cc.mu.Unlock()
	if cc.onTxReset != nil {
		cc.onTxReset()
	}
	cs.finish(verr)
	return nil
}
cs.respHeadersSeen = true
cs.respHeaders = decoded
// … the existing :status scan …
```

⚠️ **`markReset` FIRST is LOAD-BEARING, and the reason is written at the existing trailer site.** Further
peer frames on this id are **guaranteed** when the rejected block did not carry END_STREAM (the peer never
terminated the stream). Without the early `markReset` they fall through to the "stream gone" arms, which
return **CONNECTION-level** errors and **tear the pooled conn down from inside the codec** — which is not
how the reference books this. `markReset` must also precede `cs.finish`, whose deferred
`cc.streams.Delete` removes the map entry.

⚠️ **`cc.onTxReset()` fires OUTSIDE `cc.mu`** so the codec mutex is never held across a cluster callback.

⚠️ **`cs.respHeadersSeen` is NOT set on the reject path.** The stream is finished; nothing further may
read it.

### 4.3.1 ⚠️ THE CRUX IS CLOSED IN THE FIX SITE'S FAVOUR — MEASURED, NOT REASONED

The BRAINSTORM's open worry was arm 7: `writeH2Reply` lowercases every name, so **if anything normalized
the name before the fix site, a guard there could not fire on an uppercase arm and the whole fix-site
choice would be wrong.** Instrumenting the branch directly and driving a real response through `RoundTrip`:

```
PROBE-SITE-LEAD[1] name="X-Upper-Case" value="yes"      <-- UPPERCASE INTACT
PROBE-SITE-LEAD[3] name="content-length" value="5"
PROBE-SITE-LEAD[4] name="content-length" value="99"     <-- BOTH PRESENT, wire order
```

`writeH2Reply`'s lowercasing (`h2dispatch.go:1012`, `:1023`) is **DOWNSTREAM** of the guard; `x/net`'s
decoder does not case-fold and `ReadMetaHeaders` is not in play on this path. **Nothing normalizes upstream
of the guard, and duplicate `content-length` is fully visible as a slice preserving both fields in wire
order.** ⇒ **the fix site catches all seven arms and arm 10 needs no second site.**

### 4.4 THE THIRD ROUTER ARM — placement RESOLVED, and a THIRD FILE the SPEC did not know about

`doH2ClusterAction` (`router_h2.go:73`) has these non-success arms **in source order**:

| # | line | condition | disposition |
|---|---|---|---|
| 1 | `:80` | `!a.cluster.TryAcquireRequest()` | 503 |
| 2 | `:127` | grant-race retries exhausted | 503 |
| 3 | `:133` | `cluster.IsConnPoolOverflow(err)` | 503 |
| 4 | `:148` | acquire/dial error | 502, `localOrigin: true` |
| 5 | `:188` | `errors.Is(err, h2.ErrMalformedTrailers)` | `Status: 0` — **ABOVE the evict** |
| — | **`:197`** | **`a.cluster.EvictH2ConnOnError(cc, ep)`** | — |
| 6 | `:202` | `ctx.Err() != nil && (Canceled \|\| DeadlineExceeded)` | `Status: 0`, CANCEL |
| 7 | `:213` | fall-through transport error | 502, `localOrigin: true` |

**DECISION: the new arm goes BETWEEN `:197` AND `:202` — AFTER the evict, BEFORE the ctx-cancel check.**

- **After `:197`** — mandatory per D-92-POSTURE. **The eviction IS the parity behaviour**: measured at
  default posture the reference also destroys the upstream conn (`upstream_cx_destroy_local` 0->1) and
  resets in-flight siblings.
- **Before `:202`** — ⚠️ **this is a REAL decision the SPEC's "before/beside" phrasing left open, and it is
  resolved here.** The ctx-cancel arm **branches on `ctx.Err()` ALONE and never inspects the error
  identity.** Placed after it, a downstream cancel racing a malformed block would **launder the rejection
  into a `Status: 0` CANCEL and lose the 502 NON-DETERMINISTICALLY** — exactly the failure the trailer
  arm's own comment warns about (*"THE DISCRIMINATOR IS THE SENTINEL, NOT THE CODE"*). Placing it first
  makes the outcome deterministic on sentinel identity.
  ⚠️ **THE CONSEQUENCE, STATED IN WRITING RATHER THAN ASSUMED: a genuine client cancel that coincides with
  a detected malformed block now reports 502 rather than CANCEL.** That is the correct trade — the
  validator demonstrably fired — but it is a behaviour choice and is recorded as one. **Cost is +0 lines
  either way.**
- Placing it **before `:197`** and calling `EvictH2ConnOnError` inside the arm is a same-cost alternative
  that **duplicates the evict call and is strictly worse.** Rejected.

The arm (`errors`, `h2` and `cluster` are **already imported** — no new imports):

```go
if errors.Is(err, h2.ErrMalformedResponseHeaders) {
	a.cluster.IncStatusClass(502)
	a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: 502, LocalOriginErr: true})
	return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders()}, picked, nil
}
```

**Every symbol verified at this tip:** `IncStatusClass` (`cluster.go:336`), `RecordUpstreamResult`
(`:351`), `UpstreamResult{StatusCode int; LocalOriginErr bool}` (`:344`), `bad502Body`
(`router.go:31`), `h2LocalReplyHeaders` (`router_h2.go:259`), `ActionResponse` (`router.go:80`).
**Zero missing, zero mismatched — the SPEC's sketch compiles verbatim.**

⚠️ **`localOrigin` IS DELIBERATELY LEFT UNSET WHILE `LocalOriginErr` IS `true`. DO NOT TIDY THEM.**
They are **different consumers**: `RecordUpstreamResult` feeds **outlier detection**, where a
locally-detected upstream fault is exactly the signal to eject the host; `ActionResponse.localOrigin`
feeds **retry classification** at `retry.go:129`
(`if rp.on&(retryConnectFail|retryReset) != 0 && localOrigin {`), where `true` would classify a perfectly
**REACHABLE** but malformed upstream as a **CONNECT FAILURE** — measured cost **3 backend-observed attempts
on 3 separate TCP conns** against the reference's **1**. **D-92-POSTURE exists to kill exactly this.**
⚠️ Supporting evidence, and a stale comment recorded not fixed: `cluster.go:343` still says
*"LocalOriginErr is unread at 40.1"*, but `:355` does pass it to `c.outlier.record(...)`. **The field IS
read.**

### 4.5 ⚠️ THE THIRD FILE — an AST-AUDIT GOLDEN THAT REDDENS ON A CORRECT CHANGE

**MEASURED, not anticipated.** With only the sentinel and the arm in place — no test edit — the two-package
suite went **RC=1, RUN=655, FAIL=4**:

```
=== RUN   TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites
    router_h2_trailers_test.go:528: doH2ClusterAction non-Trailers ActionResponse Status set =
        [0 0 502 502 502 503 503 503] (n=8), want [0 0 502 502 503 503 503] (n=7)
--- FAIL: TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites
```

`internal/filter/http/router/router_h2_trailers_test.go` **walks the router package's AST** and pins the
exact **MULTISET** of `ActionResponse` literal `Status` values inside `doH2ClusterAction`
(`want := []int{0, 0, 502, 502, 503, 503, 503}` at `:528`), plus a doc-comment enumeration at `:505-513`.
It is deliberately a **SET** guard, not a count guard — *"a count guard is blind to one site being swapped
for another."*

⇒ **The IMPL MUST edit it. This is mandatory, not optional, and it makes the row a THREE-file production
change where the SPEC said two.** Minimum edit: **`+2 / −1`** (one `want` element, one doc-comment line).

⚠️ **A COST PROTOTYPE THAT SKIPPED THE TEST RUN WOULD HAVE REPORTED THE ARM AS `+35 / −0` AND MISSED A
MANDATORY FILE ENTIRELY.** This is `reference_measured_prototype_is_a_lower_bound` firing with an
IDENTIFIED MECHANISM for the second row running, and the mechanism is the same one both times:
**under-enumeration of files, not of lines.**

⚠️ A sibling in the same file, `TestActionResponseLiterals_OnlySuccessSitePopulatesTrailers`, logs a
package-wide literal census that moves **17 -> 18** — but only via `t.Logf`, **so it does NOT redden.**
**Expect that number to move and do not read its silence as invariance.**

---

## 5. THE NEW COUNTER — named, validated BY EXECUTION, and three hazards confirmed

SPEC §6.1 decided **exactly ONE** new counter: the `http2.rx_messaging_error` analogue at cluster scope.
It is needed because the only counter that moves today on this path is **already shared with the
trailer-reject path** and therefore cannot discriminate a leading-block rejection from a trailer one.

### 5.1 ⚠️ THE INHERITED NAMING PREMISE IS WRONG — envoy-go's registered names are DOTTED

**`http2_tx_reset` DOES NOT EXIST IN ANY `.go` FILE IN THIS REPO.** `git grep 'http2_tx_reset' -- '*.go'`
returns **ZERO** hits. It appears only in `DECISIONS.md:18459` and in this phase's own `SPEC.md`
(`:220`, `:325`, `:326`, `:337`, `:340`) — where it is the **Prometheus-FLATTENED PROJECTION**, not the
registered name.

The **actually registered** names, `internal/cluster/manager.go`:

```go
:195   c.upstreamCxHTTP2Total = r.NewCounter(prefix + "upstream_cx_http2_total")
:196   c.http2StreamsActive   = r.NewGauge(prefix   + "http2.streams_active")
:200   c.http2RxReset         = r.NewCounter(prefix + "http2.rx_reset")
:201   c.http2TxReset         = r.NewCounter(prefix + "http2.tx_reset")
```

with `prefix := "cluster." + c.name + "."` (`manager.go:113`), inside `registerClusterMetrics`
(`manager.go:112`), behind the H2 gate `if c.useH2 {` at `:194`. Live full names confirmed in
`test/fixtures/0080-h2-goaway-rotation/driver/driver_test.go:39-41`: `cluster.c_h2gw.http2.tx_reset`.

**The convention is:** `cluster.<name>.` + a **`<family>.<snake_leaf>`** leaf for codec-scoped stats, and a
**bare snake leaf** for cluster-level ones. ⇒ **the reference's exact leaf spelling is the
CONVENTION-NATIVE choice here, not a departure.**

### 5.2 DECISION: the leaf is **`http2.rx_messaging_error`**

Full name **`cluster.<name>.http2.rx_messaging_error`**. It sits next to `manager.go:200-201`, matches its
`http2.rx_reset` / `http2.tx_reset` siblings, and **flattens to `cluster_http2_rx_messaging_error` — byte
for byte the reference name recorded at `DECISIONS.md:18459`.**

Rejected: `http2.rx_response_header_error` (narrower than the reference's event ⇒ a named departure for no
gain); `http2_rx_messaging_error` (valid, but **breaks the local convention** — every other `http2.*` stat
is dotted, and it would flatten to the same Prometheus name while diverging in the internal registry).

### 5.3 `stats.IsValidName` — the ACTUAL rule, and the proof

`internal/stats/registry.go:48`/`:60`:

```go
const NamePattern = `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`
var nameRE = regexp.MustCompile(NamePattern)
func IsValidName(name string) bool { return nameRE.MatchString(name) }
```

**Dots ARE permitted** (segment separator); a **trailing** dot is not. Enforced identically by `checkName`,
which **PANICS** from `register`/`getOrRegister` on a bad name.

**PROVEN BY RUNNING THE REAL FUNCTION**, not by reading the regex:

```
IsValidName("cluster.c_h2s.http2.rx_messaging_error") = true    <- the chosen name, FULL
IsValidName("http2.rx_messaging_error")               = true    <- the chosen LEAF
IsValidName("cluster.c_h2gw.http2.tx_reset")          = true    CTRL+ existing registered name
IsValidName("http2.rx messaging error")               = false   NEG space
IsValidName("http2.rx-messaging-error")               = false   NEG hyphen
IsValidName("http2.rx_messaging_error.")              = false   NEG trailing dot
IsValidName("2http2.rx_messaging_error")              = false   NEG leading digit
IsValidName("")                                       = false   NEG empty
IsValidName("http2.rx_messaging_error\n")             = false   NEG trailing newline
```

⚠️ **SIX negative controls return `false`, so the green is discriminating rather than a stub.**

⚠️ **AND ONE PREDICTED NEGATIVE CONTROL IS VACUOUS: UPPERCASE *PASSES*.**
`IsValidName("HTTP2.RX_MESSAGING_ERROR") = true` — `a-zA-Z` appears in **both** character classes.
**No gate in this row may use an uppercase name as its `IsValidName` negative control.** Use a hyphen, a
space, a trailing dot, a leading digit, an empty string, or a trailing newline.

### 5.4 ⚠️ THREE HAZARDS, ALL CONFIRMED BY EXECUTION — the IMPL must design around them

1. ⚠️ **A NIL `*stats.Counter` `.Inc()` IS A PROCESS CRASH.** `internal/stats/counter.go:22` is
   `func (c *Counter) Inc() { c.v.Add(1) }` — **no nil guard**, and the probe panicked with a nil-pointer
   dereference. It survives in production only because the tree wraps it, `internal/cluster/connpool.go:211-215`:
   ```go
   func incCounter(c *stats.Counter) {
   	if c != nil {
   		c.Inc()
   	}
   }
   ```
   and the reset hooks route through it (`h2pool.go:201`/`:205`).
   ⇒ **The new counter is `useH2`-GATED exactly like `http2TxReset`, so it is NIL on every non-H2
   cluster. The increment MUST route through `incCounter`.** A bare `c.http2RxMessagingError.Inc()` is a
   **process crash on any non-H2 cluster**, with no `recover()` anywhere above it.
   **The IMPL must ALSO assert the POINTER non-nil on the H2 path** — a nil-guarded `Inc` that silently
   does nothing is exactly the vacuous pin `reference_nil_stats_counter_inc_crashes_goroutine` warns about.
2. ⚠️ **REGISTERING A STAT INSIDE `Registry.Walk` DEADLOCKS — CONFIRMED, NOT REASONED.** `Walk` holds
   `r.mu.RLock()` across the whole iteration while `getOrRegister`/`register` take `r.mu.Lock()`; Go's
   `sync.RWMutex` is not reentrant. The probe goroutine **never returned within 2 s**.
   ⇒ **Register at BOOT in `registerClusterMetrics` (`manager.go:112`), NEVER lazily from a scrape or
   `Walk` callback.** Same shape as the phase-79 D-SPP-3 record.
3. ⚠️ **A COUNTER CANNOT GATE A VALUE.** It pins that a rejection HAPPENED, never WHICH FIELD caused it.
   The field-level assertion belongs to the unit table (§6.1) and the differential arm (§8).
   ⚠️ **Assert it SUBJECT-SIDE ONLY** — cross-side stat scope is a known divergence axis and the
   reference's spelling differs even where the event matches. ⚠️ **Pin the DELTA from a baseline read,
   NEVER the absolute** (§7 NC8).

### 5.5 ⚠️ DO NOT RESTATE `406 -> 407`. THE `406` IS NOT THE STAT SURFACE.

The BRAINSTORM (`:368`) and the SPEC (`:122`, `:338`, `:522`) all carry **stat surface 406**. **It is not
the stat surface, and this stage could not reproduce it.**

- ⚠️ **`406` APPEARS ZERO TIMES IN `BEHAVIOR_CONTRACT.md`** — the document that IS the stat-surface record.
  Its ledger tail is **`**Phase 77 - 1205 -> 1207**`**, so the documentary absolute is **1207**.
- ⚠️ **THE PROVENANCE IS AN UNNAMED GREP.** `DECISIONS.md:18226` (ADR-0313 §Context ¶6) says
  *"stat surface unchanged (**406 occurrences on both sides by the same command**)"* — **an OCCURRENCE
  COUNT used as an INVARIANCE PROXY, and the command is never named.** It was then restated as
  *"stat surface"* through the BRAINSTORM, the SPEC and ADR-0313 §Consequences (xiv).
- ⚠️ **SEVEN CANDIDATE COMMANDS WERE RUN AT THIS TIP AND NONE READS 406:** `143` · `324` · `401` · `402` ·
  `403` · `404` · `699`, depending on anchoring, package scope and whether `_test.go` is excluded.
  **The figure is not reproducible from the record.**
- ⚠️ **AND `STATE.md` §Project counts says `1205`** — a THIRD number, stale from the phase-76 era and
  superseded by the contract's own `1207`.

⇒ **THIS ROW ASSERTS THE `+1` DELTA STRUCTURALLY AND CITES NO ABSOLUTE.** The delta is mechanically
checkable: **exactly ONE new `NewCounter` call site**, added inside the existing `if c.useH2 {` gate in
`registerClusterMetrics`. That is the same discipline `BEHAVIOR_CONTRACT.md` already applies to itself —
its own ledger entries say the absolute is **DOCUMENTARY ONLY**, inheriting a chain **known to be
discontinuous in TWO places** (`1198 -> 1200` and `1200 -> 1201`), and that *"a future phase needing an
authoritative absolute total must re-derive it MECHANICALLY and should expect the re-derived figure to
disagree."*

⚠️ **The IMPL must NOT write `406 -> 407` into any artifact.** Write the **+1 name delta** and, if an
absolute is wanted, add a ledger line against **1207** in the contract's own form. **Counting the surface
mechanically is a maintenance row nobody has chartered — record it as a named deferral rather than an
idle observation.**

---

## 6. THE TEST ROSTER

New file: **`internal/filter/hcm/h2/response_headers_validate_test.go`**, modelled directly on the
in-tree `trailers_validate_test.go` (which is the SAME shape one direction over: a direct TABLE A, a wire
TABLE B, and a sentinel-discrimination test).

⚠️ **THE TABLE-A / TABLE-B SPLIT IS LOAD-BEARING AND MUST BE PRESERVED.** `trailers_validate_test.go:24-30`
states it: Table A exercises the validator **in isolation**, so **a neutered CALL SITE leaves Table A fully
green**. Table B is the liveness gate for the call site; Table A is the correctness gate for the rule set.
A row that shipped only Table A would pass with the validator never wired in.

### 6.1 TABLE A — `TestValidateResponseHeaders_Table` (direct)

Signature under test: `validateResponseHeaders(streamID uint32, fields []hpack.HeaderField) *Error`.
Use a non-zero constant stream id so the returned `*Error`'s `Stream` field can be asserted — a
stream-scoped error MUST carry the id; a connection-scoped one carries 0 and would tear the conn down.

**REJECT arms — NINE, one per MEASURED divergent shape (§2.3 raised this from seven):**

| arm name | fields (after `:status: 200`) | `wantMsgSubstr` |
|---|---|---|
| `connection` | `connection: keep-alive` | `"connection"` |
| `transfer_encoding` | `transfer-encoding: chunked` | `"transfer-encoding"` |
| `keep_alive` | `keep-alive: timeout=5` | `"keep-alive"` |
| `upgrade` | `upgrade: websocket` | `"upgrade"` |
| `proxy_connection` | `proxy-connection: keep-alive` | `"proxy-connection"` |
| `uppercase_name` | `X-Upper-Case: yes` | `"X-Upper-Case"` |
| `duplicate_content_length` | `content-length: 5`, `content-length: 5` | `duplicate content-length` |
| `te_gzip` | `te: gzip` | `not 'trailers'` |
| `te_empty` | `te: ""` | `not 'trailers'` |

⚠️ **`te_empty` IS THE ARM AN IMPLEMENTER IS MOST LIKELY TO GET WRONG.** Writing
`value != "" && value != teTrailersValue` looks like defensive hygiene and is **measurably wrong** — the
reference answers **502** for a present-but-empty `te` (§2.3). ⚠️ **`te_gzip` and `te_empty` share the
message `"not 'trailers'"` with the EXISTING trailer table's `te_gzip` arm — that shared string is exactly
why the `te` leg must NOT be collapsed into `IsIllegalH2RequestHeader`** (§4.2).

⚠️ Every `wantMsgSubstr` is the **QUOTED** form (`strconv.Quote` output, i.e. with the `"` characters), for
the reason the trailer validator's own comment gives: an unquoted name is unfalsifiable when it also
appears inside the message's fixed prefix.

**ORDER arms — because a leg may be SHADOWED rather than absent (SPEC §10):**

| arm name | fields | asserts |
|---|---|---|
| `uppercase_beats_connection` | `Connection: keep-alive` | the **uppercase** message, NOT the connection-specific one |
| `uppercase_beats_content_length` | `Content-Length: 5` | the **uppercase** message — proves the case-sensitive legs below it are unreachable for this input |
| `connection_beats_te` | `connection: x`, `te: gzip` | the **connection** message (first field wins) |
| `te_beats_dup_cl` | `te: gzip`, `content-length: 1`, `content-length: 1` | the **te** message |

**PARITY / POSITIVE-CONTROL arms — must PASS (the SPEC's three, plus four this PLAN adds):**

| arm name | fields | why |
|---|---|---|
| `plain_200` | `:status: 200`, `content-type: text/plain` | baseline; a validator rejecting everything reddens here |
| `status_pseudo_passes` | `:status: 200` alone | ⚠️ **`:status` is REQUIRED here.** The trailer validator BARS pseudo-headers; this one must not. **This is the single arm that proves the inverted leg really inverted.** |
| `single_content_length` | `content-length: 5` | ONE is legal |
| `single_content_length_wrong_value` | `content-length: 99` over a notional 5-byte body | ⚠️ **arm E (§2.1 item 2) is OUT OF SCOPE and this arm PINS that** — the validator must NOT reject it |
| `te_trailers_passes` | `te: trailers` | ⚠️ **MEASURED PARITY — the reference FORWARDS it VERBATIM.** This arm is what stops a later broadening of the `te` leg to an unconditional reject, which would redden the differential against a correct reference (§2.3) |
| `host_passes` / `trailer_passes` | `host: x` / `trailer: y` | reference-parity controls — barred by RFC 9110, FORWARDED by the reference. Rejecting them makes the differential RED on a correct implementation |
| `ows_and_empty_value` | `x-a: " v "`, `x-b: ""` | BRAINSTORM arms 8 and 9, measured PARITY on both sides |

⚠️ **`empty_block` is deliberately NOT an arm.** A leading block with no fields is not a legal H/2
response at all (no `:status`), and this validator does not enforce `:status` presence (§4.2). An
`empty_block` arm would document a non-decision as coverage.

### 6.2 TABLE B — `TestClientConn_RoundTrip_ResponseHeaderValidation_Wire`

Drives a real `RoundTrip` against `fakeH2ServerPeer` and asserts the rejection reaches the caller.

⚠️ **MUST USE `dialClientConnTCP`, NOT `dialClientConn`.** The leading-block reject writes RST_STREAM
**from the readLoop goroutine** while the peer may be mid-write of its next frame. `net.Pipe` is
synchronous and unbuffered, so that is a **guaranteed deadlock** — a first probe over `net.Pipe` at the
SPEC stage **deadlocked and was killed at 120 s**. `trailers_validate_test.go:281-290` documents exactly
this for the trailer path and the helper already exists at `:291`. Kernel socket buffers absorb both writes.

Per arm: peer reads the request HEADERS, writes a leading response block containing the illegal field;
the test asserts `RoundTrip` returns an error, `errors.As` it to `*h2.Error`, and asserts
`errors.Is(err, ErrMalformedResponseHeaders)`.

⚠️ **THE PEER MUST RUN IN ITS OWN GOROUTINE** and the test must bound itself with a
`context.WithTimeout` — a synchronous peer blocks the read loop.

### 6.3 `TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject`

Three sub-tests, because this is the pin that D-92-POSTURE rests on and a two-way version is vacuous:

1. `peer_reset_is_NEITHER` — peer sends `RST_STREAM(INTERNAL_ERROR)`; assert `Code == ErrInternalError`
   **and** `errors.Is(err, ErrMalformedResponseHeaders) == false` **and**
   `errors.Is(err, ErrMalformedTrailers) == false`.
   ⚠️ Asserting the CODE explicitly is what proves the code is a **non-discriminator** — all three
   outcomes carry `INTERNAL_ERROR`. Without it the sentinel's whole rationale is unproven.
2. `malformed_leading_IS_response_headers_NOT_trailers` — assert `Is(…, ErrMalformedResponseHeaders)` is
   true **and** `Is(…, ErrMalformedTrailers)` is **false**.
3. `malformed_trailers_IS_trailers_NOT_response_headers` — the mirror. ⚠️ **This third sub-test is what
   makes the pair non-vacuous.** Without it, a validator that returned `ErrMalformedResponseHeaders` for
   trailers too would pass.

---

## 7. THE NEGATIVE-CONTROL CAMPAIGN — SPEC §10's table, made executable

⚠️ **AT THE PHASE-91 IMPL, THREE OF THAT ROW'S OWN NEW PINS WERE NON-DISCRIMINATING AND ALL THREE PASSED
WHEN WRITTEN** — one structurally unable to fire, one vacuous, one a coin flip. **A test that passes is
not thereby a guard.** Every NC below is RUN and its ACTUAL result RECORDED. An NC that does not redden
is a FINDING about the pin, not a formality to skip.

⚠️ **COMMIT FIRST before every break** (`git checkout --` restores from HEAD and wipes uncommitted work),
`sha256sum` before, `sha256sum -c` after the restore, and `git diff --stat master` EMPTY at the end.
⚠️ **Every NC run needs `-count=1`** or the cache serves a stale PASS.
⚠️ **ONE ARM PER RUN.** A fail-fast driver masks later RED arms: the run fails on the first and reads as
proof for all of them.

| # | pin | the break | expected RED | ⚠️ why it could be VACUOUS |
|---|---|---|---|---|
| NC1 | the **FOUR** legs behind the **NINE** reject arms | **delete that ONE leg** from `validateResponseHeaders` | ⚠️ **the legs and the arms are NOT 1:1 — state the expected arm set per leg BEFORE running.** `isConnectionSpecificField` ⇒ **5** arms · the `te` rule ⇒ **2** · `hasUppercaseHeaderChar` ⇒ **1** (+ two ORDER arms flip) · the duplicate-`content-length` count ⇒ **1** | a leg may be **SHADOWED** by an earlier leg rather than absent — this is why §6.1 has ORDER arms. Deleting the uppercase leg must redden `uppercase_name` **AND** flip `uppercase_beats_connection` to the connection message |
| NC2 | the 4 ORDER arms | **move `hasUppercaseHeaderChar` to LAST** in the switch | `uppercase_beats_connection` + `uppercase_beats_content_length` | presence-only NCs cannot catch an ordering regression at all |
| NC3 | the parity arms | make the validator `return malformedResponseHeadersError(...)` unconditionally | ALL parity arms | **a parity arm that never reaches the validator is vacuous** — this NC is what proves they do |
| NC4 | the call site (Table B liveness) | **comment out the call** in `onResponseHeaderBlock`, leaving the validator intact | **Table B ONLY; Table A stays FULLY GREEN** | ⚠️ **that asymmetry IS the assertion.** If Table A also reddens, the split has been lost and Table B is not a call-site gate |
| NC5 | sentinel discrimination | return `ErrMalformedTrailers` from `malformedResponseHeadersError` | §6.3 sub-tests 2 **and** 3, **and** the router arm's 502 pin (it takes the WRONG arm) | ⚠️ must redden **on the ARM TAKEN**, not merely on the error value — assert the resulting STATUS, not just `errors.Is` |
| NC6 | **the no-retry pin** | restore `localOrigin: true` on the new router arm | the retry-count pin | ⚠️ **needs `retry_on` CONFIGURED in the test's route.** Without a retry policy the pin **cannot fire under any input** and passes vacuously. **Configure `retry_on: connect-failure, num_retries: 2` and assert attempts == 1** |
| NC7 | the eviction pin | remove `EvictH2ConnOnError` from the path | the eviction pin | ⚠️ **needs a STACKED CONTROL — a passing arm cannot catch an OVER-firing evict.** Pair "one malformed response evicts once" with "one LEGAL response evicts zero times" in the same test |
| NC8 | the new counter | **delete the `Inc`** | the counter pin | ⚠️ **assert the DELTA from a baseline read, NEVER the absolute.** An absolute passes on a dirty registry and fails on a clean one |
| NC9 | the counter's over-firing | leave the `Inc` but ALSO `Inc` on the legal path | the counter pin | a delta-of-1 pin with no legal-path control cannot see an over-firing counter |
| NC10 | the differential arm | **revert the production guard ONLY** (keep the fixture edits) | ⚠️ **EACH of the 3 wire shapes SEPARATELY** | a shared code path defeats per-arm counts — assert the **SET** of illegal names, and give **each shape its OWN backend path**. One path emitting all three is blind to a fix that catches one and launders another |
| NC11 | trailer invariance | — | ⚠️ **NOT a red run.** This is a NON-regression: its "NC" is the **RUN-SET DIFF** (`diff` of the sorted `=== RUN` name lists before and after), which must be EMPTY | a green run does not prove the set did not change; a renamed or dropped test is invisible to a count |
| NC12 | reachability of the router arm | `panic("PROBE-P92-ROUTER-ARM")` inside the new arm | something must HIT it | ⚠️ **MANDATORY before concluding any site is exercised.** A green run is not evidence a site is live — it may be dead code, and grep cannot tell assertion blindness from dead code |

⚠️ **NC1 IS FOUR BREAKS, NOT ONE — AND EACH HAS A MULTI-ARM EXPECTED SET.** ⚠️ **`grep -c` on the failing
arm names is NOT the gate: assert the SET of reddened subtests, because a shared code path defeats per-arm
counts** (`reference_shared_codepath_defeats_per_arm_counts`). **Do not batch NCs into one run** — a
fail-fast driver masks later RED arms and the run's failure then reads as proof for all of them.

---

## 8. DIFFERENTIAL — D-92-DIFF, carried forward with the arm-shape rules made explicit

**EXTEND fixture `0004-h2-routing`. DO NOT mint `0120`.** Priced at the SPEC by a COMPILING prototype:

```
31   0  test/fixtures/0004-h2-routing/backends/main.go
162  0  test/fixtures/0004-h2-routing/driver/driver.go
```

**+193 / −0, two files.** Plus **0 YAML lines** (the new paths fall through the existing
`- match: { prefix: "/api" }`), **0 new BackendKind**, **0 registration gates**, **0 port allocations**, and
`BackendCount()` unchanged at **3** so `AssertDistribution`'s `[3,3,3]` is untouched.
Minting `0120` was priced at ≈ **1440 lines across ~8 files** — ~7.4× — and ⚠️ **its expected port `10120`
is TAKEN** (fixture `0028-http-lua-multi-script-and-per-route`, `inputs/driver.go:65`); free in-band is
**10126-10129**.

**IN-FIXTURE PRECEDENT TO FOLLOW EXACTLY:** phase 90 added `p90Arm` / `p90Arms()` / `p90Fields` /
`p90EncodeHeaderBlock` / `p90DriveArm` to this same driver. Phase 92 adds the `p92*` sibling set. Read
`driver.go:764-1000` before writing a line.

### 8.1 ⚠️ THE DEPARTURE, STATED PRECISELY — and it now has an UNMEASURED edge

Measured against the pinned `x/net` from the ordinary `net/http` backend `0004` already spawns:

- **EXPRESSIBLE (3):** `keep-alive`, `upgrade`, `proxy-connection` — they leak because
  `x/net/http2/server.go:2757` carries a live `TODO: remove more Connection-specific header fields here`.
- **NOT EXPRESSIBLE (4):** `connection` (deleted at `server.go:2759`, `delete(rws.snapHeader, "Connection")`);
  `transfer-encoding` (does not survive); an uppercase wire name (`http.Header` canonicalization + the
  encoder's `lowerHeader`); a duplicate `content-length` (collapsed — `snapHeader.Del` then a single re-add).

⚠️ **UNMEASURED (2) — THE TWO NEW `te` SHAPES OF §2.3.** The SPEC's expressibility census was taken over
the ORIGINAL SEVEN shapes and **predates the `te` measurement entirely**, so nothing in the record says
whether a `net/http` backend can emit `te: gzip` or `te: ""` onto the wire.
**T2 MUST DETERMINE THIS BY MEASUREMENT, NOT BY READING `server.go`.**
- If they ARE expressible, the differential gains **two more wire arms at near-zero marginal cost** (one
  handler path each), and the departure narrows from six unit-only shapes to four.
- If they are NOT, they join the unit-layer set and the departure widens to six.
⚠️ **DO NOT ASSUME EITHER.** A plausible reading of `x/net` — that `te` is absent from the deletion set at
`server.go:2759` and would therefore leak, canonicalized to `Te` and lowercased by the encoder — is a
HYPOTHESIS. **Arm 7 is the standing proof that a code-reading of this exact file misses the answer.**

These four are unreachable for a **STRUCTURAL reason in the library, not a budget reason**, so the
departure is precisely statable rather than an omission. They are pinned at the **unit layer only**.
Reaching them on the wire needs **BackendKind 39**, a raw-framer illegal-response responder (≈ +303 runner
lines) — **deliberately not bought**, and banked as a candidate.

⚠️ **IN-TREE PRECEDENT FOR EXACTLY THIS TRADE-OFF, ONE ROW OLD**, `0004/driver/driver.go:445`:
*"Recovering it would need a raw-framer backend (a new BackendKind for one assertion). Wire ORDER is
pinned at the UNIT layer instead."*

### 8.2 Arm-shape rules — both already cost a probe

- ⚠️ **EACH SHAPE NEEDS ITS OWN BACKEND PATH** (`/p92-keepalive`, `/p92-upgrade`, `/p92-proxyconn`).
  A single path emitting all three is **blind to a fix that catches one and launders another**.
- ⚠️ **THE TRANSCRIPT LINE MUST RECORD THE *SET* OF ILLEGAL NAMES PRESENT, NOT ONE NAME.** A shared code
  path defeats per-arm counts (`reference_shared_codepath_defeats_per_arm_counts`).
- ⚠️ **THE CONTENT-LENGTH ASSERTION, IF ANY, IS OVER THE FIELD *COUNT*, NEVER THE VALUES** — §2.1 item 1.
- The transcript must be **byte-comparable cross-side**; scrub the ephemeral address as `p90ScrubAddr`
  already does.

### 8.3 Gate discipline — ⚠️ GATE 2'S FAILURE MODE IS A SILENT PASS

⚠️ **ASSERT THE FIXTURE SET, never "the suite was green."** Verified at the SPEC and re-verified here: the
runner `t.Skipf`s an **unregistered** fixture at `test/differential/runner_test.go:200`, `DriverRegistry` is
read at exactly ONE site (`:194`), and **there is NO fixture-count gate anywhere in the tree.** That
absence is credible only because the same search form **DOES** find real count gates elsewhere
(`0070/driver_test.go:58`, `0004/driver.go:1061`, `internal/runtime/snapshot_test.go:85`) — a POSITIVE
CONTROL, per `reference_absence_claim_needs_positive_control`.

⇒ the invocation, and the line that must appear:

```sh
go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v
# MUST print:  === RUN   TestDifferential/0004-h2-routing
```

⚠️ **A `-run` selector matching nothing prints `[no tests to run]` and EXITS 0.**
⚠️ **The ANCHORED panic gate `^panic:|DATA RACE|SIGSEGV` runs on every differential launch.**
⚠️ **Redirect `2>&1`** — the differential log DOES carry subject stderr, but only if you capture it.
⚠️ **`0004` is TLS+ALPN-h2, NOT h2c** — do not "simplify" it to plaintext.
⚠️ Full-suite startup flake: **TWO reserved bands** exist; an in-band recurrence is a FINDING, not noise.

---

## 9. FUZZ — D-92-FUZZ, one target

**Ship `FuzzValidateResponseHeaderBlock`** in `internal/filter/hcm/h2/fuzz_test.go` (the existing file —
no new file, so the FILE count moves only if a new file is created; state which). ~60 LoC.

The precedent argument is the **OPPOSITE** of the one originally hypothesised, and it argues **FOR** the
target: **`IsIllegalH2RequestHeader` has NO fuzz target**, nor does `isConnectionSpecificField`,
`hasUppercaseHeaderChar`, or `validateResponseTrailers`.
⚠️ **REACHABILITY IS NOT COVERAGE:** `FuzzFrameStream` transitively reaches `isConnectionSpecificField`
via `buildRequest` (`stream.go:494`), but its only assertion is *"no panic + every error begins with
`h2:`"* — **it can never observe a wrong classification.** `FuzzHPACKDecode` reaches no predicate at all.
The encode/response direction has **no fuzz reach whatsoever**; there is no `ClientConn` fuzz target.

Assertions, all three:
1. **no panic**;
2. every rejection message carries the `h2:` prefix **and names the offending field QUOTED, in TRAILING
   position** — the discipline `client.go:770-774` documents as load-bearing for falsifiability;
3. ⚠️ **the accept/reject verdict agrees with an INDEPENDENTLY-WRITTEN ORACLE** over the closed name set.
   **The oracle must NOT call the predicates the validator calls** — an oracle sharing
   `isConnectionSpecificField` proves only that a function equals itself.

Corpus: `[]byte` decoded into `[]hpack.HeaderField` by the fuzz body (the seed shape `FuzzFrameStream`
already uses), seeded with a legal 200, each of the seven reject shapes, and a duplicate-`content-length`
pair. Precedent: `FuzzDrainTransitions` (ADR-0018). CI's `fuzz-smoke` matrix drives 10 of the targets for
30 s each.

---

## 10. COST — MEASURED, and THE SPLIT GATE EVALUATED

### 10.1 ⚠️ PRODUCTION IS `+174 / −1`, MEASURED BY ONE COMPILING PROTOTYPE OF THE FROZEN DESIGN

**The `+74` and `+77` figures are SUPERSEDED and must not be restated.** They priced ONE file. The
complete frozen design (§4.1-§4.5) was built as a single compiling prototype, gated, run, and reverted:

```
137	0	internal/filter/hcm/h2/client.go
35	0	internal/filter/http/router/router_h2.go
2	1	internal/filter/http/router/router_h2_trailers_test.go
```

| file | added | removed | code | comment | blank |
|---|---|---|---|---|---|
| `internal/filter/hcm/h2/client.go` | 137 | 0 | 44 | 90 | 3 |
| `internal/filter/http/router/router_h2.go` | 35 | 0 | 5 | 30 | 0 |
| `internal/filter/http/router/router_h2_trailers_test.go` | 2 | 1 | 1 | 1 | 0 |
| **TOTAL** | **174** | **1** | **50** | **121** | **3** |

By package: **h2 `+137 / −0`**, **router `+37 / −1`**. `client.go` by design piece, from `-U0` hunks:
call site **31** (code 11) · sentinel + constructor **25** (code 9) · validator + doc comment **81** (code 24).
Reconciliation: `44+90+3 = 137`, and `-U0 | grep -c '^+[^+]'` = 134 with `137−134 = 3` = the blank count.

Gates on the combined prototype, all on OUTPUT: `gofmt -l` **EMPTY** · `go vet ./internal/filter/http/router/...
./internal/filter/hcm/...` **rc=0, zero output** · `golangci-lint` (v1.64.8, British spellings swept first)
**rc=0, ZERO BYTES** · `go build ./...` **rc=0** · `go test -c` **rc=0** both packages.

⚠️ **AN INFERENCE, LABELLED AS ONE:** the prototype's `client.go` **code-only count is 44**, *exactly* the
banked comment-free `+44`. That suggests the earlier prototypes covered the same surface at partial
commenting density, and that the feared sentinel double-count resolves as *"they overlapped fully, they
were just under-commented."* **This was NOT verified against those prototypes — they are not in the tree.
Repeat it only as INFERRED.**

### 10.2 ⚠️ THE VALIDATOR SHIPS COMPLETELY UNGATED BY THE EXISTING SUITE — the row's most important measurement

**Adding a real validator that rejects five distinct shapes moved ZERO tests.** Baseline and final are both
**RC=0, RUN=655, FAIL=0, RUN SETS IDENTICAL by `diff`**. **Not one existing test drives a leading block the
validator rejects.**

⇒ **A `t.Skip`, an inverted leg, or deleting the entire function body would leave all 655 rows GREEN.**
**The unit table of T10-T12 is not a nice-to-have; it is the ONLY thing that can fail.** This is why §7's
NC campaign is sized the way it is, and why NC4's Table-A/Table-B asymmetry is load-bearing.

**The golden edit was isolated by counterfactual:** reverting ONLY the test file while keeping all
production code gives **RC=1, RUN=655, FAIL=4** with **exactly one** failing test —
`TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites`. The `+2 / −1` edit is **sufficient AND
necessary**, and **nothing else moves.**

**SIBLING-GUARD SWEEP — an absence claim WITH a positive control.** Every AST/parser-driven test in the
repo was enumerated (`go/ast|go/parser|token.NewFileSet` over `*_test.go`) and each one's **WALK TARGET**
checked, not merely its imports:

| file | parses | touches this row's files? |
|---|---|---|
| `router_h2_trailers_test.go` | the **router package** source dir | **YES — the one guard** |
| `cmd/envoy-go/main_test.go` | `cmd/envoy-go/main.go` only | no |
| `internal/stats/segmentcount_test.go` | a **synthetic in-memory string** | no |
| `test/fixtures/0061-lb-ring-hash/driver/linkage_test.go` | fixture-local | no |

No test censuses the h2 package's exported surface or its error sentinels (`errors_test.go`'s four tests
are per-error shape assertions, **no roster and no count**). ⇒ **exactly ONE forced golden, and the claim
is falsifiable because the same search form DID find it.**

### 10.3 THE REMAINING COST — what `+174 / −1` does NOT include

| item | cost | basis |
|---|---|---|
| production, frozen design (a)-(e) | **`+174 / −1`** | **MEASURED**, §10.1 |
| `isConnectionSpecificField`'s doc comment | **~`+1 / −1`** | MEASURED as needed — it names its callers **exhaustively** at `stream.go:377-391`, and `validateResponseHeaders` is a **THIRD** caller. ⚠️ Deliberately excluded above so `+174` is exactly (a)-(e); **it must land** |
| the new counter (T8) | **~+15-25** | ESTIMATE — field + registration + `incCounter`-routed hook. **UNPRICED; T8 measures it** |
| D-92-1XX comment (T16) | **`10 / 3`** | MEASURED at the SPEC, gofmt clean, lint rc=0 |
| **production subtotal** | **~+200-215 / −5** | |
| differential arm (T2+T3) | **`+193 / −0`** | MEASURED at the SPEC by a compiling prototype |
| test-backend extension (T9) | **~+57**, or a threading change across **5 call sites** | MEASURED from the probe backend. ⚠️ **UNPRICED BY ANY PRIOR STAGE** |
| unit tables + NC budget (T10-T15, T17-T19) | **~+250-450** | ESTIMATE |
| fuzz target (T20) | **~+60** | ESTIMATE |
| **test subtotal** | **~+560-760** | |
| **GRAND TOTAL** | **~+760-975 net** | |

⚠️ **THE PRODUCTION FIGURE IS THE ONLY MEASURED ONE. THE TEST FIGURES ARE ESTIMATES AND HAVE BEEN LOWER
BOUNDS SIX ROWS RUNNING** (`reference_measured_prototype_is_a_lower_bound`). **This row has now seen the
mechanism twice with a name: UNDER-ENUMERATION OF FILES, not of lines** — the AST golden (§4.5) and the
test-backend extension (T9) were both invisible to a prototype that did not run the suite.

### 10.4 ⚠️ THE SPLIT GATE — EVALUATED, NOT ASSUMED

BOOTSTRAP §6.1 triggers a split at **~25 numbered tasks** OR **~1500 LoC** of net change.

| threshold | this row | verdict |
|---|---|---|
| tasks | **23** (§14) | **UNDER** — margin 2 |
| net LoC | **~760-975** | **UNDER** — margin ~525-740 even at the top of the range |

⇒ **NO SPLIT. Phase 92 stays a single phase; `want` stays 124 and no `92.1 / 92.2` rows are created.**

⚠️ **BUT THE TASK MARGIN IS ONLY TWO, AND BOOTSTRAP §6.1 ALSO TRIGGERS A SPLIT *MID-EXECUTION*** if any
single task's sub-steps blow past ~10 items. **The three named split risks, with their triggers stated in
advance so the decision is not made under pressure:**

1. **T9 (the test-backend extension).** If threading a caller-supplied leading-header slice through
   `runH2TrailerBackend`/`startH2TrailerBackend` turns out to touch materially more than the **5** known
   call sites, or forces a third backend plus its own harness, **stop and split** — T9+T13+T14 become
   `92.2 (h2-response-header-validation-router-disposition)`.
2. **T17-T19 (the NC campaign).** NC1 alone is **twelve runs**. If the campaign's recorded results force
   redesigns of the pins rather than confirmations, **stop and split** rather than letting one task grow
   an open-ended sub-list.
3. **The counter (T8/T14).** If `incCounter` routing or the `useH2` gate turns out to need a new
   registration path rather than a line beside `manager.go:200-201`, re-price before continuing.

⚠️ **Splitting is NOT a way to defer work.** BOOTSTRAP §6.3: no vague "TODO: extend later" tasks and no
incomplete stubs the differential cannot exercise. **Either the work is in this phase and gets tested, or
it is in a split sub-phase with its own ROADMAP row. There is no third option.**

---

## 11. COUNTS AT THIS TIP — RE-DERIVED MECHANICALLY, NEVER COPIED

Every figure below was measured at `8c18100c` with the command shown. **Claimed vs measured is stated so a
stale inheritance is visible rather than laundered** (`reference_verification_table_launders_wrong_cites`:
"HOLDS" rows get trusted).

| figure | claimed | **measured** | verdict |
|---|---|---|---|
| `DECISIONS.md` `wc -l` | 18467 | **18467** | MATCHES |
| `^---$` | 216 | **216** | MATCHES |
| `^## ADR-` | 313 | **313** | MATCHES |
| bare `^## ` | 321 | **321** | MATCHES |
| ADR **tail** | `ADR-0314` | **`ADR-0314`** | MATCHES |
| `^## ADR-0315` | 0 | **0** | MATCHES |
| ADR id-space audit `0001..0314` | one gap `0209` | **exactly one missing: `0209`; ZERO duplicates** | MATCHES |
| **strict** `^> \*\*STATUS: PROPOSED` | 1 | **1**, at **`:18443`**, nearest preceding heading **`## ADR-0314` at `:18441`** | MATCHES |
| ADR-0231 decoy `^\*\*Status:\*\* PROPOSED` | 1 @ `:14866` | **1 @ `:14866`**, heading `## ADR-0231` @ `:14864` — **STILL ARMED** | MATCHES |
| **UNANCHORED** `-F '> **STATUS: PROPOSED'` | 2 | **⚠️ 3** | **STALE** |
| `STATE.md` | 63 | **63** | MATCHES |
| `STATE_HISTORY.md` | 526 | **526** | MATCHES |
| strict `^- \*\*prior active-phase:\*\*` | 163 | **163** | MATCHES |
| loose `prior active-phase` | 213 | **213** | MATCHES |
| `BEHAVIOR_CONTRACT.md` | 5962 | **5962** | MATCHES |
| phase dirs | 133 | **133** | MATCHES |
| fixtures `ls -d test/fixtures/*/` | 121 | **121** | MATCHES |
| fixture numeric tail / `0120` | — | **`0119-grpc-unary-trailers`**; **`0120` ABSENT ⇒ FREE** | — |
| fuzz targets / files in `internal/` | 55 / 48 | **55 / 48** | MATCHES |
| `^func Fuzz` outside `internal/` | — | **NONE** (repo total 55) | — |
| blank imports — fixture drivers in `runner_test.go` | 121 | **121** | MATCHES |
| blank imports — fixture drivers REPO-WIDE | 122 | **122** | MATCHES |
| blank imports — ALL in `runner_test.go` | 123 | **123** | MATCHES |
| blank imports — ALL repo-wide | 145 | **145** | MATCHES |
| `go.mod` requires | 67 (18 direct + 49 indirect) | **67 (18 + 49)** | MATCHES |
| BackendKind **tail value** | 38 `H2GoawayResponder` @`fixture.go:614` | **38, `H2GoawayResponder`, `:614`** | MATCHES |
| BackendKind **constants declared** | (distinct from the tail) | **39** (values 0..38); **39 is FREE** | — |
| ROADMAP denominator | 124 | **124** | MATCHES |
| row 92 status / line | `in-progress` @~154 | **`in-progress` @ `:154`** | MATCHES |
| `-family row` OCCURRENCES / LINES | 95 / 67 | **95 / 67** | MATCHES |
| `BRAINSTORM.md` / `SPEC.md` | 627 / 586 | **627 / 586** | MATCHES |
| `PLAN.md` | absent | **ABSENT** (dir holds exactly 2 files) | MATCHES |

### 11.1 ⚠️ THE ONE STALE FIGURE, and why it will keep going stale

`grep -c -F '> **STATUS: PROPOSED'` reads **3**, not the inherited **2**:

- `:18342` — **PROSE**, under `## ADR-0312`: *"Two plausible strict `PROPOSED` guard forms both read exactly `1`…"*
- `:18371` — **PROSE**, under `## ADR-0313`: *"The strict `^> **STATUS: PROPOSED` guard moves 1 -> 0 at this entry…"*
- `:18443` — **the REAL guard**, under `## ADR-0314`

The **2** was the **pre-arm** reading (2 prose, 0 real); ADR-0314's arming added the third. ⚠️ **The number
2 written INSIDE ADR-0314's own text went stale the moment that text landed.**
⇒ **The anchor is load-bearing and the unanchored count INFLATES every time an ADR documents the form.
Never gate on the unanchored count, and never restate it as a standing absolute.**

### 11.2 Strict-vs-loose `prior active-phase` — the Δ50 explained

The loose form catches lines carrying a **parenthetical before the colon**, which the strict `:**` anchor
rejects, e.g. `STATE_HISTORY.md:420` and `:424`:
`- **prior active-phase (evicted at the phase-80 BRAINSTORM close, 2026-07-29, ADR-0288 five-entry cap):**`
⇒ **this row's archive line must NOT match the strict form**, so the guard stays at **163, DELTA 0**.

### 11.3 ⚠️ TRAPS RE-REPRODUCED AT THIS TIP — every one is live

- **`\t`-in-ERE is TOOL-DEPENDENT. NAME THE TOOL.** `grep -cE '^\t_ "' test/differential/runner_test.go`
  ⇒ harness **`grep` (ugrep 7.8.4)** prints **123**; **`/usr/bin/grep` (GNU 3.11)** prints **0 and exits 1**.
  **`-P` and `$(printf '\t')` give 123 under BOTH.** ⇒ the standing record's blanket *"ERE reads 0"* is true
  **only of GNU grep**.
- **The `--` FLAG TRAP.** `grep -oiE '-family row' …` ⇒ `ugrep: error: option -f: cannot read amily row`,
  **no count emitted**, and surrounding arithmetic then prints `0` — **which reads exactly like "no change."**
  **Always pass `--` before a pattern beginning with `-`.**
- **DIGIT-BLIND CLASS.** `grep -oE '[A-Za-z]+ +BackendKind = 38'` prints `GoawayResponder …` — **truncating
  the leading `H2`**.
- **`go.mod` CHAR CLASS.** `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` reads **62**, silently dropping the five
  modules whose path carries an uppercase letter or underscore.
- **OVER-ANCHORED FIXTURE FORM.** `^[0-9]{4}-` reads **119**, dropping `0007a-cors` and `0007b-iteration-probe`.
- **`-F` vs `-E` ON A PARENTHESISED RECEIVER.** `grep -c -F 'func (cc *ClientConn) onResponseHeaderBlock'`
  ⇒ **1**; the same with `-E` ⇒ **0**, a **FAIL-UNSAFE ZERO**.
- **HARNESS `grep` IS BLIND TO `next-prompt.txt`** — reproduced BY FILE NAME, not by count: `command grep -rlF`
  returns four paths including `./next-prompt.txt`; the harness returns the same three minus it.
  `.gitignore:2` lists the file, `git ls-files --error-unmatch` says **TRACKED**. ⚠️ **The harness also
  STRIPS the leading `./`**, so a naive path-equality diff shows spurious differences on every row.
- ⚠️ **`grep -c` counts LINES, not OCCURRENCES**, and **exits 1 on zero matches** — capture as
  `v=$(… || true)`, never chain with `&&`.
- ⚠️ **NO GATE MAY REST ON A ` + ` SPLIT** — both the trap and its recorded remedy are fail-unsafe.

### 11.4 SPEC §3 SYMBOL TABLE — 9/9 EXACT, ZERO DRIFT

`onResponseHeaderBlock` **609** · `validateResponseTrailers` **788** · `hasUppercaseHeaderChar` **779** ·
`isConnectionSpecificField` **392** · `IsIllegalH2RequestHeader` **414** · `ErrMalformedTrailers` **716** ·
`doH2ClusterAction` **73** · `writeH2Reply` **1004** · `reconcileH2DecodeDelta` **866**.
Plus, all verified verbatim: `client.go:673` carries the wrong 1xx claim; `h2dispatch.go:1014-1016` is the
`content-length` rewrite; `router_h2.go:188` is the trailer sentinel arm; `retry.go:128` is
`RetryPolicy.matches`, whose `:129` is `if rp.on&(retryConnectFail|retryReset) != 0 && localOrigin {` —
**the exact line the unset `localOrigin` avoids.**

### 11.5 What this PLAN must NOT have changed — verify by EMPTY DIFF

`ROADMAP.md` · `BEHAVIOR_CONTRACT.md` · `go.mod` · `go.sum` · every `.go` file in the tree.
**A PLAN LANDS DOCS ONLY: ZERO production `.go`, ZERO test `.go`.**
`git diff --name-only master -- '*.go'` must print **NOTHING**.
ADR tail stays **ADR-0314**, next-free **ADR-0315**, `^---$` **216**, `^## ADR-` **313**, bare `^## ` **321**,
strict `PROPOSED` guard **ARMED at 1** under ADR-0314, the ADR-0231 decoy **ARMED at 1**.

---

## 12. GATES — the six-gate posture. **NAME DEPARTURES; DO NOT CLAIM COMPLIANCE.**

| gate | this row |
|---|---|
| **(a)/(b)** | differential **121 fixtures** + full `go test ./...`, gated on **`PIPESTATUS[0]`** and a **SET RECONCILIATION**. ⚠️ **NOT `INNER_EXIT` — THAT WRAPPER DOES NOT EXIST IN THIS REPO.** |
| **(c)** | h2spec cited **ONLY from this row's own run** (⚠️ h2spec is MEASURED BLIND to burst-drain ordering); grpc-conformance **deferred in writing**; proxy-wasm **10/16** |
| **(d)** | fuzzers **55 / 48** today ⇒ **56 / 49** at row-done (⚠️ **49 only if the target lands in a NEW file**; if it goes into the existing `fuzz_test.go` the FILE count stays **48** — **state which, and measure it**) |
| **(e)** | the **ANCHORED** panic gate `^panic:\|DATA RACE\|SIGSEGV` on **every** differential launch |
| **(f)** | **no `REVIEW.md` — standing departure** |

### 12.1 Per-run method rules — every one has drawn blood in this lineage

- ⚠️ **`go test` WITHOUT `-v` PRINTS ZERO `=== RUN`.** `RUN=0` beside `RC=0` is a **VACUOUS GREEN**.
- ⚠️ **A `-run` SELECTOR MATCHING NOTHING PRINTS `[no tests to run]` AND EXITS 0.** Assert the subtest
  name actually appeared.
- ⚠️ **ON `-v` OUTPUT, UNANCHORED `grep -c 'FAIL'` READS 11 ON A FULLY GREEN TREE.** Use
  `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`.
- ⚠️ **`-count=1` ON EVERY RUN** — the cache serves a stale PASS, and a break protocol without it is
  worthless.
- ⚠️ **`gofmt -l` NEVER EXITS NON-ZERO — GATE ON OUTPUT.** Same for `golangci-lint`.
- ⚠️ **`rc=$?` AFTER A PIPE RETURNS THE LAST COMMAND'S STATUS** — use `out=$(…); rc=$?` or `PIPESTATUS`.
- ⚠️ **ASSERT THE SYMBOL, NOT THE BUILD.** A build is not evidence the edit landed. Use `grep -F` for any
  parenthesised-receiver anchor.
- ⚠️ **`pgrep -f` / `pkill -f` MATCH YOUR OWN SHELL AND KILL THE TOOL CALL (exit 144).** Kill only PIDs
  captured from `$!`; verify with `kill -0`.
- ⚠️ **BAND PROBE PORTS BELOW 32768** (`ip_local_port_range = 32768 60999`); `21000-24999` is clear.
  **Check with `ss -tan` (ALL states), never `ss -ltn`.**
- ⚠️ **`golangci-lint`'s misspell runs in LOCALE US** — sweep British spellings in `.go` comments BEFORE
  the gate. Markdown prose may use them freely.
- ⚠️ **NEVER TEAR DOWN A CONTAINER THIS SESSION DID NOT CREATE.** By NAME only. Foreign containers left
  untouched through phases 90-92 and again at this PLAN: `infallible_booth`, `crazy_kare`, `golink-ai`,
  `quizzical_goldstine`.
- ⚠️ **NO `phase-*` / `wt-*` WORKTREE MAY OUTLIVE ITS STAGE** — `git worktree list` must show only
  `master` at close.

---

## 13. DOCS — text CONSTRAINED here, LANDED at the IMPL

### 13.1 `ADR-0314` — §Decision + §Consequences

⚠️ **APPEND IN PLACE, AFTER THE RETAINED ITALIC FOOTER** `*§Decision and §Consequences follow at the
phase-92 IMPL.*`, which the SPEC already wrote. **NO renumber. NO `---` separator** (the ADR-0294-0314
shared-block form) — `^---$` stays **216**.
⚠️ **The phase-90 IMPL discovered its ADR's STATUS line named a footer that did not exist and had to add
one before appending. That defect is NOT repeated here — the footer is already present. Verify it is
before appending anyway.**
⚠️ **DISARM the strict guard 1 -> 0, verified BY LINE AND BY ADR.** The ADR-0231 decoy at `:14866` reads 1
and must be **LEFT ARMED**. ⚠️ **Do NOT gate on the unanchored count — it reads 3 and inflates** (§11.1).

§Consequences must carry, at minimum: the closed open arm and the surviving broad rule (§2); arm E as an
explicitly out-of-scope EIGHTH shape assigned to the banked `content-length`-rewrite candidate (§2.1);
the AST-audit golden as a third file (§4.4); the dotted-name correction (§5.1); the vacuous-uppercase-NC
finding (§5.3); and the three-way disposition already recorded at the SPEC.

### 13.2 The other doc edits

- `PROGRESS.md` — **NEW** in the phase dir (state 3 creates it). Every gate's ACTUAL output quoted.
- `ROADMAP.md` — row 92 `in-progress` -> **`done`** at the IMPL. ⚠️ **`want` stays 124** — a row is
  updated, never added. ⚠️ **Row 92 field-counts NF=8 under both forms; keep it that way** — count the
  fields BEFORE installing any edit, because an unescaped `|` silently passes the gate.
- `BEHAVIOR_CONTRACT.md` — a rider for the new counter; **state the line delta** (it is **5962** today).
- `STATE.md` — rolled **IN PLACE**, never prepended. Oldest §Recent entry evicted by a **DIRECT DATE READ**.
  ⚠️ **Read the dates yourself every time** — four stages running have produced four DIFFERENT tie shapes,
  most recently a TWO-WAY tie at 2026-08-22 broken by LAST LIST POSITION. **The shape does not carry.**
- `STATE_HISTORY.md` — the evicted entry archived as **ONE INLINE LINE** whose label does **NOT** match
  `^- \*\*prior active-phase:\*\*`, so the strict guard stays **163, DELTA 0** (§11.2).
- `next-prompt.txt` — rolled. ⚠️ **TRACKED BUT GITIGNORED: `git add -f next-prompt.txt`.**

### 13.3 Documentary defects — RECORDED, deliberately NOT fixed

- ⚠️ `STATE.md` **§Project counts is STALE** (phase-76 era: fixtures 119, ADR tail 0298, BackendKind
  lineage prose) and **SELF-CONTRADICTS §Current with no label saying so.** Fixing it is a maintenance row,
  not this one. **Recorded so the next reader does not anchor on it.**
- ⚠️ `internal/cluster/cluster.go:343` still says *"LocalOriginErr is unread at 40.1 (reserved for 40.2)"*,
  but `RecordUpstreamResult` **does** pass it to `c.outlier.record(...)` at `:355`. **The comment is stale;
  the field IS read.** This SUPPORTS D-92-POSTURE's "different consumers" reasoning and is left alone.
- ⚠️ The SPEC's own `http2_tx_reset` spellings (`:220`, `:325`, `:326`, `:337`, `:340`) name a string that
  **exists in no `.go` file**. They are correct AS PROMETHEUS PROJECTIONS and wrong as registered names.
  **Not edited — a SPEC is a landed artifact.** The correction lives here and in ADR-0314.

---

## 14. THE ORDERED IMPL TASKS — **TWENTY-THREE**

⚠️ **A NOTE ON TDD, STATED HONESTLY RATHER THAN PERFORMED.** The unit tables of T10-T12 name symbols the
row itself creates (`validateResponseHeaders`, `ErrMalformedResponseHeaders`), so writing them against the
unmodified tip yields a **BUILD BREAK, not an assertion failure** — and a build break proves nothing
(`reference_config_counterfactual_is_not_implementation_counterfactual`: you cannot tell what a broken
build ships).

⇒ **THIS ROW'S RED ANCHOR IS T2+T3 AND IT IS A GENUINE ONE**: the differential arm compiles and runs
against the **unmodified** subject and goes **RED**, because the subject forwards the illegal fields with
200 where the reference answers 502. It is recorded **before any production byte moves**. The unit tables
are written test-first *within* their tasks and are **regression pins, not the row's proof**. Claiming
otherwise would be the shape `reference_liveness_break_needs_failing_baseline` warns about.

- [ ] **T1 — Record the pre-edit baselines. ZERO production bytes.** Every figure with its **DENOMINATOR
  ASSERTED** and its **AXIS STATED**; `-v -count=1` on every run.
  - T1a `go test -v -count=1 ./internal/filter/hcm/... ./internal/filter/http/router/...` — expect
    **RC=0, RUN=655, anchored FAIL=0**. ⚠️ Capture the **sorted `=== RUN` name list** to a file; T15's
    invariance gate diffs against it.
  - T1b `go test -v -count=1 ./internal/cluster/...` — the counter's package.
  - T1c `gofmt -l` on every file this row will touch — ⚠️ **gate on OUTPUT; it never exits non-zero.**
  - T1d `golangci-lint run ./internal/filter/hcm/h2/... ./internal/filter/http/router/... ./internal/cluster/...`
    — ⚠️ **gate on OUTPUT, not rc.**
  - T1e `go vet` on the same three package trees.
  - T1f the differential baseline: `go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v 2>&1`
    — ⚠️ **confirm `=== RUN   TestDifferential/0004-h2-routing` APPEARS**; a `-run` matching nothing prints
    `[no tests to run]` and EXITS 0.
  - T1g fuzz counts **55 / 48**; fixtures **121**; BackendKind tail **38**.

- [ ] **T2 — RED ANCHOR, part 1: the differential BACKEND paths.** `test/fixtures/0004-h2-routing/backends/main.go`,
  **+31 / −0** for the three known-expressible shapes. Three NEW handler paths — `/p92-keepalive`,
  `/p92-upgrade`, `/p92-proxyconn` — **one shape per path** (§8.2). ⚠️ **A single path emitting all three
  is blind to a fix that catches one and launders another.** No YAML change: the paths fall through the
  existing `- match: { prefix: "/api" }`.
  ⚠️ **ALSO MEASURE, DO NOT INFER, WHETHER `te: gzip` AND `te: ""` SURVIVE A `net/http` BACKEND ONTO THE
  WIRE** (§8.1). Add two probe paths, drive them, and **read the actual downstream bytes**. If they leak,
  keep them as wire arms (+2 paths, +2 driver arms, near-zero marginal cost) and narrow the departure; if
  they do not, delete the probe paths and record them in the unit-layer set. ⚠️ **Arm 7 is the standing
  proof that a code-reading of `x/net/http2/server.go` gets this wrong.** **Record the measured answer
  either way — the `+31` moves if they leak.**

- [ ] **T3 — RED ANCHOR, part 2: the driver arms, and RECORD THE RED.**
  `test/fixtures/0004-h2-routing/driver/driver.go`, **+162 / −0**. Add the `p92Arm` / `p92Arms()` /
  `p92DriveArm` set, modelled on the in-fixture `p90*` precedent at `driver.go:764-1000`. ⚠️ **The
  transcript line records the SET of illegal names present, never one name.** ⚠️ **Scrub the ephemeral
  address** as `p90ScrubAddr` does. **`BackendCount()` stays 3** so `AssertDistribution`'s `[3,3,3]` is
  untouched. **Run it. It MUST be RED, and the transcript diff must be recorded PER SHAPE** — three
  separate divergences, not one.

- [ ] **T4 — production: the sentinel and its constructor.** `internal/filter/hcm/h2/client.go`, after the
  existing `ErrMalformedTrailers` block (`:716`). Code frozen at **§4.1**.

- [ ] **T5 — production: `validateResponseHeaders`.** Same file, beside `validateResponseTrailers`
  (`:788`). Code frozen at **§4.2**. ⚠️ **LEG ORDER IS LOAD-BEARING — `hasUppercaseHeaderChar` FIRST.**
  ⚠️ **Do NOT collapse the connection-specific and `te` legs into `IsIllegalH2RequestHeader`** — it
  reddens `TestValidateResponseTrailers_Table/te_gzip`. ⚠️ **ASSERT THE SYMBOL, not the build**
  (`grep -F` for parenthesised receivers).
  ⚠️ **ALSO UPDATE `isConnectionSpecificField`'s DOC COMMENT** (`stream.go:377-391`). It names its callers
  **EXHAUSTIVELY** — *"shared by the request-decode path (buildRequest…) and the response-trailer
  validation path (validateResponseTrailers…)"* — and `validateResponseHeaders` is a **THIRD** caller.
  ~`+1 / −1`. **Leaving it is shipping a comment this row falsifies.**

- [ ] **T6 — production: the CALL SITE.** `onResponseHeaderBlock` (`:609`), inside `if !cs.respHeadersSeen {`,
  **BEFORE `cs.respHeaders = decoded` retains the block**. Mirror the trailer path's reject mechanics
  exactly: `cc.markReset(streamID)` **FIRST**, then the `cc.mu`-locked `WriteRSTStream(streamID,
  http2.ErrCodeInternal)`, then `cc.onTxReset()` **outside** the mutex, then `cs.finish(verr)`, then
  `return nil`. ⚠️ **The `markReset`-first ordering is load-bearing** — the capture site's own comment
  explains why: further peer frames on that id are guaranteed when the rejected block carried no
  END_STREAM, and without the early `markReset` they fall through to arms that return CONNECTION-level
  errors and tear the pooled conn down.

- [ ] **T7 — production: the THIRD router arm AND its forced golden edit.** `router_h2.go` (after
  `EvictH2ConnOnError` at `:197`, **BEFORE** the ctx-cancel check at `:202` — §4.4) **and**
  `router_h2_trailers_test.go:528`'s `want` multiset **and** its doc-comment enumeration at `:505-513`.
  ⚠️ **THESE LAND TOGETHER OR THE SUITE IS RED.** ⚠️ **`localOrigin` STAYS UNSET while
  `LocalOriginErr` STAYS `true` — different consumers. Do NOT tidy.**

- [ ] **T8 — production: the counter.** `internal/cluster/`: the `Cluster` field, the registration in
  `registerClusterMetrics` (`manager.go:112`) inside the `if c.useH2 {` gate at `:194`, next to `:200-201`,
  named **`http2.rx_messaging_error`** (§5.2); and the increment hook. ⚠️ **ROUTE THE INCREMENT THROUGH
  `incCounter` (`connpool.go:211-215`) — the counter is `useH2`-gated and is NIL on every non-H2 cluster,
  and a bare `.Inc()` is a PROCESS CRASH with no recover().** ⚠️ **REGISTER AT BOOT, NEVER FROM A `Walk`
  CALLBACK — that deadlocks (confirmed by execution).**

- [ ] **T9 — test harness: let the h2 test backend serve a caller-supplied LEADING header block.**
  ⚠️ **THIS IS A REAL, PREVIOUSLY UNPRICED COST ITEM AND IT BLOCKS T12/T13.**
  `startH2TrailerBackend` / `runH2TrailerBackend` **cannot** serve this shape: their leading block is
  hard-coded to `:status` / `content-type` / `content-length` and `h2TrailerBehavior` has no leading-block
  arm. Either add a fifth `h2TrailerBehavior` value plus a caller-supplied leading-header slice threaded
  through both functions (**touching 5 existing call sites**), or add a third backend (~**57 lines**
  standalone). ⚠️ **Choose and RECORD which, with the measured cost.**

- [ ] **T10 — test: TABLE A.** NEW `internal/filter/hcm/h2/response_headers_validate_test.go`,
  `TestValidateResponseHeaders_Table` per **§6.1** — **NINE** reject arms (including `te_gzip` and
  `te_empty`, both MEASURED at this PLAN), **4** ORDER arms, **8** parity arms (including the MEASURED
  `te_trailers_passes`). ⚠️ **THIS TABLE IS THE ONLY THING THAT CAN FAIL** — §10.2 measured that the
  validator otherwise ships completely ungated.
  ⚠️ **Every `wantMsgSubstr` is the QUOTED form.**

- [ ] **T11 — test: TABLE B (the CALL-SITE liveness gate).** Same file, per **§6.2**.
  ⚠️ **`dialClientConnTCP`, NOT `dialClientConn` — `net.Pipe` DEADLOCKS this path** (a SPEC-stage probe was
  killed at 120 s). ⚠️ **Peer in its own goroutine; bound with `context.WithTimeout`.**

- [ ] **T12 — test: the sentinel discrimination, THREE sub-tests.** Per **§6.3**. ⚠️ **The third
  sub-test (trailers are NOT response-headers) is what makes the pair non-vacuous.** ⚠️ **Assert the CODE
  explicitly** — it is a NON-discriminator, and that is the sentinel's whole rationale.

- [ ] **T13 — test: the ROUTER DISPOSITION pins — 502, EVICTING, NOT RETRIABLE.** Uses T9's harness.
  Three properties, **one `t.Errorf` per property** — ⚠️ **`t.Fatalf` makes every later assertion dead
  code.**
  - **502** downstream, not `Status: 0`.
  - **NO RETRY.** ⚠️ **The route MUST CONFIGURE `retry_on: connect-failure, num_retries: 2`, or the pin
    CANNOT FIRE UNDER ANY INPUT and passes vacuously.** Assert backend-observed attempts **== 1**.
  - **EVICTION**, with a **STACKED CONTROL**: one malformed response evicts once AND one legal response
    evicts zero times. ⚠️ **A passing arm alone cannot catch an OVER-firing evict.**

- [ ] **T14 — test: the counter pin.** ⚠️ **DELTA from a baseline read, NEVER the absolute.**
  ⚠️ **SUBJECT-SIDE ONLY** — cross-side stat scope diverges and the reference's spelling differs.
  ⚠️ **Assert the POINTER is non-nil on the H2 path** — a nil-guarded `Inc` that silently does nothing is
  a vacuous pin. ⚠️ **Include the legal-path control** (delta 0) or an over-firing counter is invisible.

- [ ] **T15 — test: TRAILER INVARIANCE, as a RUN-SET DIFF.** ⚠️ **This is a NON-REGRESSION: its gate is
  the `diff` of the sorted `=== RUN` name lists against T1a, which must be EMPTY — NOT a red run and NOT a
  count.** A renamed or dropped test is invisible to a count. Re-run all three denominators the SPEC names
  (the full two-package set, `-run 'Trailer'`, and the five named h2 trailer tests) and **show the SETS**.

- [ ] **T16 — production comment: D-92-1XX.** `client.go:673` — *"the reference FORWARDS 1xx"* is **WRONG**
  (measured: the reference **SWALLOWS** 1xx, delivering only the final response). Take the **accurate
  10/3 restatement**, not the 1/1 word-swap, so a future 1xx row designs against **drop-and-deliver**.
  ⚠️ **EXACTLY ONE wrong instance tree-wide** — `ROADMAP.md:154`, this row's `SPEC.md:470` and
  `next-prompt.txt` carry the string **only to REFUTE it** and **MUST NOT be "fixed"**.

- [ ] **T17 — NC CAMPAIGN 1/3: the validator and the call site.** NC1 (seven legs, **one deletion per
  run**), NC2 (leg-order flip), NC3 (reject-everything), NC4 (**call site commented out ⇒ Table B RED,
  Table A FULLY GREEN — that asymmetry IS the assertion**). ⚠️ **COMMIT FIRST**, sha256 before, restore,
  `sha256sum -c` ⇒ `OK`. ⚠️ **`-count=1` every run. ONE arm per run.**

- [ ] **T18 — NC CAMPAIGN 2/3: the sentinel, the router arm and the counter.** NC5 (must redden **on the
  ARM TAKEN** — assert the STATUS, not just `errors.Is`), NC6 (**restore `localOrigin: true`** — needs
  `retry_on` configured), NC7 (remove the evict), NC8 (delete the `Inc`), NC9 (over-fire the `Inc`).

- [ ] **T19 — NC CAMPAIGN 3/3: the differential and reachability.** NC10 — **revert the production guard
  ONLY, keeping the fixture edits, and confirm EACH of the three wire shapes reddens SEPARATELY.**
  NC12 — the **`panic()` REACHABILITY CONTROL** on the router arm, **with its discriminating negative
  control** (a legal response must NOT panic). ⚠️ **MANDATORY: a green run is not evidence a site is live,
  and grep cannot separate assertion blindness from dead code.**

- [ ] **T20 — fuzz: `FuzzValidateResponseHeaderBlock`.** Per **§9**. ⚠️ **The oracle must be
  INDEPENDENTLY WRITTEN and must NOT call the predicates the validator calls** — an oracle sharing
  `isConnectionSpecificField` proves only that a function equals itself. **State whether the target lands
  in the existing `fuzz_test.go` (files stay 48) or a new file (49) and MEASURE it.**

- [ ] **T21 — differential GREEN.** Re-run T1f's invocation. ⚠️ **ASSERT THE FIXTURE SET** — the runner
  `t.Skipf`s an unregistered fixture at `runner_test.go:200` and **no fixture-count gate exists anywhere in
  the tree**. ⚠️ **Confirm the `=== RUN   TestDifferential/0004-h2-routing` line APPEARS.**
  ⚠️ **Anchored panic gate `^panic:|DATA RACE|SIGSEGV` on the launch; redirect `2>&1`.**

- [ ] **T22 — the six-gate posture.** Per **§12**. ⚠️ **NAME DEPARTURES; DO NOT CLAIM COMPLIANCE.**
  Gated on **`PIPESTATUS[0]`** and a **SET RECONCILIATION** — ⚠️ **NOT `INNER_EXIT`, which does not exist
  in this repo.**

- [ ] **T23 — docs.** Per **§13**: ADR-0314 §Decision + §Consequences appended **IN PLACE after the
  RETAINED italic footer** (no renumber, **NO `---`** — `^---$` stays **216**) with the strict guard
  **DISARMED 1 -> 0 verified BY LINE AND ADR** and the **ADR-0231 decoy LEFT ARMED**; `PROGRESS.md` (NEW);
  `ROADMAP.md` row 92 -> **`done`** (`want` STAYS **124**, NF stays 8); `BEHAVIOR_CONTRACT.md` rider with
  its line delta stated; `STATE.md` rolled **IN PLACE** with a **DIRECT DATE READ**; `STATE_HISTORY.md`
  **ONE INLINE LINE** that does NOT match the strict guard (stays **163, DELTA 0**); `next-prompt.txt`
  (**`git add -f`**).

---

## 15. EXPLICITLY NOT MEASURED AT THIS PLAN — stated so it is never inferred

1. **The differential arm was NOT re-run at this tip.** Its `+193 / −0` is the SPEC's compiling-prototype
   figure, re-stated, not re-derived. **T1 re-derives it and T3 executes it.**
2. **The `0004` baseline transcript was NOT captured here.** No docker differential run was launched at
   this PLAN.
3. **h2spec, proxy-wasm and the full `go test ./...` were NOT run.** Only
   `./internal/filter/hcm/... ./internal/filter/http/router/...` was, and only as a prototype
   non-regression (**RUN=655 / FAIL=0**, sets identical).
4. ⚠️ **THE STAT SURFACE WAS NOT COUNTED, AND THE `406` FIGURE WAS *DISPROVEN* RATHER THAN VERIFIED**
   (§5.5). Seven candidate commands read 143/324/401/402/403/404/699 — **none is 406** — and the ADR that
   introduced it never names the command. **This PLAN did not find the right command either; it
   established only that the inherited one is unreproducible.** The contract's documentary absolute is
   **1207**, itself known-discontinuous in two places. **Only the +1 DELTA is asserted, structurally.**
5. **Outlier-detection consequences of `LocalOriginErr: true` were NOT measured.** The SPEC flags the
   `LocalOriginErr` / `localOrigin` split as deliberate; **this PLAN carries it forward unmeasured and the
   IMPL must NOT "tidy" them into agreement without measuring outlier detection separately.**
6. **Arm E's reference behaviour beyond the first 200+RST was NOT explored** (no retry/lifetime axis).
7. **The H1 and H3 siblings of this defect are UNMEASURED.** The identical `content-length` rewrite exists
   on all three codecs; only H2 was probed.
8. **`0004`'s three wire shapes were NOT re-driven at this tip** — the `keep-alive` / `upgrade` /
   `proxy-connection` leak set is the SPEC's measurement, inherited.
9. ⚠️ **WHETHER A `net/http` BACKEND CAN EMIT `te: gzip` OR `te: ""` ONTO THE WIRE IS UNMEASURED.** The
   SPEC's expressibility census predates the `te` measurement (§2.3). **T2 must determine it; do not infer
   it from `x/net/http2/server.go`.** It decides whether the differential carries three wire arms or five.

---

## 16. BANKED CANDIDATES — carried forward, with what this PLAN changed about them

1. ⚠️ **THE POOLED-UPSTREAM-LIFETIME DEFECT — still the strongest, TAKE IT NEXT.** Three defects at three
   sites, reproduced end to end; cost `24 added / 12 removed` over three files. ⚠️ **Its reference side is
   a PREDICTION, not a measurement.** ⚠️ Phase 92 measured part of its territory and **the result cuts both
   ways**: the reference DOES tear down the upstream conn and kill in-flight siblings at default posture,
   so *"hard-closing a shared conn is FALSE for every sibling stream"* is **right about the harm and wrong
   as a divergence claim**. **A taker must re-frame D2 around the MISSING OPT-OUT, not the teardown.**
2. **HTTP/1.1 with no `Host`** — subject 200 forwarding a literal empty `Host: `, reference 400
   `missing_host_header`. Fix priced `+90 / −0`, ONE file. ⚠️ **Its observability half is BLOCKED** —
   `missing_host_header` needs `%RESPONSE_CODE_DETAILS%`, which does not exist in envoy-go
   (`log_format` is BOOT-REJECTED).
3. ⚠️ **THE `content-length` REWRITE — NOW FOUR-LEGGED, NOT THREE.** `writeH2Reply` rewrites
   `content-length` to `len(body)` method-blind and status-blind: a `304` with CL 42 ships 0; a bodyless
   200 with CL 5 ships 0; a mismatched 999 over a 5-byte body is laundered. ⚠️ **PHASE 92 ADDS A FOURTH
   LEG, MEASURED: arm E** — a SINGLE `content-length` disagreeing with the body, where the reference sends
   **200 headers then RST_STREAM(INTERNAL_ERROR) with no DATA** and the subject serves a clean rewritten
   200. ⚠️ The identical rewrite exists on **all three codecs** (`codec.go` H1, `h2dispatch.go:1014-1016`
   H2, `h3dispatch.go` H3). Deletion prototype is `0 3` and does **not** reach parity.
4. **1xx interim responses** — subject RST_STREAM(INTERNAL_ERROR), reference **SWALLOWS** 1xx and delivers
   the final response. ⚠️ Design against **drop-and-deliver**, not forward. **Phase 92 corrects the wrong
   in-code comment in passing (D-92-1XX), so a taker inherits accurate prose.**
5. **`ssl.connection_error`** — the cheapest in-window candidate, the only strong one **inside a live
   sentinel window** (`:224`, item 2 of 3), reference side already measured, under 20 production lines.
   **It would NARROW a sentinel window at row-done — which no maintenance row can do.**

**NEW / SHARPENED BY THIS PLAN:** the cluster-level `stream_error_on_invalid_http_messaging` **posture
flag** (envoy-go parses neither spelling; the reference's opt-out is unreachable) · **`%RESPONSE_CODE_DETAILS%`**
as a multi-row surface · **BackendKind 39**, a raw-framer illegal-response responder (≈ +303 runner lines)
that would put the four structurally-unreachable shapes on the wire (**39 is confirmed FREE**) ·
**the STALE `STATE.md` §Project counts block** (§13.3) · **the stale `cluster.go:343` comment** (§13.3).

---

## 17. NEXT

**Lifecycle-state 2 -> 3.** The next session runs the **phase-92 IMPL**
(`superpowers:subagent-driven-development` or `superpowers:executing-plans`), working T1 -> T23 in order.

**It inherits, and must not re-derive:**
- the closed OPEN ARM and the surviving broad `content-length` rule (§2);
- the `te` leg's measurement and the **NINE**-shape divergence set (§2.3);
- arm E as an explicitly OUT-OF-SCOPE shape assigned to the banked `content-length`-rewrite candidate (§2.1);
- the frozen production design and its **`+174 / −1`** price (§4, §10.1);
- the resolved router-arm placement and the forced AST golden (§4.4, §4.5);
- the counter's name, its `IsValidName` proof and its three hazards (§5).

**It must NOT inherit, and must re-derive at its own tip:** every count in §11, every line cite, the
differential `+193`, and all four sentinel NCs.

⚠️ **THE STAGE'S JOB IS TO REFUTE ITS PREDECESSOR BY EXECUTION.** The 91 SPEC refuted seven claims, the 91
PLAN ten, the 91 IMPL four, the 92 BRAINSTORM five, the 92 SPEC nine, **and this PLAN ten — including two
of its own SPEC's load-bearing figures (`+74/+77` and `406`) and one leg its own SPEC shipped unmeasured.**
**Do the same. A stage that refutes nothing has probably not executed enough.**

⚠️ **AT THE IMPL, AND NOT BEFORE:** `ROADMAP.md` row 92 flips `in-progress` -> **`done`** (`want` STAYS
**124**), ADR-0314 gains §Decision + §Consequences **IN PLACE after the retained footer** with the strict
guard **DISARMED 1 -> 0**, and `PROGRESS.md` is created.

