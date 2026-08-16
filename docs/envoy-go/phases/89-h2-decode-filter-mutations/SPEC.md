# Phase 89 — `h2-decode-filter-mutations` — SPEC

**Base master `c1284a03`. Stage branch `phase-89-spec`. Date 2026-08-16. Lifecycle-state 1 → 2.**

`ROADMAP.md` is **BYTE-UNTOUCHED** at this stage (proved by an EMPTY diff against master, §12). Row 89 stays `in-progress`; sentinel `want` stays **121**. `DECISIONS.md` gains **ADR-0311 §Context** with the strict `^> \*\*STATUS: PROPOSED` guard **RE-ARMED 0 → 1**. `BEHAVIOR_CONTRACT.md` is BYTE-UNTOUCHED — the contract edit lands at the IMPL.

---

## 1. The charter, and its bound

**Decode-side filter header mutations never reach the upstream request on envoy-go's HTTP/2 downstream leg.** Additions are lost; removals are ignored; value overwrites are ignored. The same filter chain on HTTP/1 and HTTP/3 works, the same chain's **encode** direction on HTTP/2 works, and real Envoy applies both directions.

**The mechanism is TWO INDEPENDENT CONTAINERS WITH NO WRITE-BACK** — `buildH2Request` (`internal/filter/hcm/h2/stream.go`, cite BY SYMBOL) builds the ordered `[]hpack.HeaderField` the upstream HEADERS block is emitted from; `buildRequest` (same file) independently builds the `http.Header` **map** the decode chain mutates.

⚠️ **THE BRAINSTORM'S TWO-CONTAINER MODEL IS INCOMPLETE, AND THE MISSING PIECE DECIDES THE ROW. THERE IS A THIRD WRITER.** The phase-46.1a tracing seam in `h2dispatch.go` writes `x-request-id` / `traceparent` / `tracestate` / `X-B3-*` onto **`h2req.Headers` (the slice) ONLY** and never onto `c.req.Header`. Measured on this branch: `awk 'NR>=422 && NR<=456' internal/filter/hcm/h2dispatch.go | grep -c 'c\.req'` reads **`0`**, and the `view` map the tracing block builds is a freshly-allocated third map. **Any fix that treats the map as authoritative DELETES the tracing headers from the upstream request** — measured three independent ways in §3. This single fact eliminates two of the three candidate shapes.

**Bounded OUT of charter, each settled by execution below:** the encode direction (§7, already correct) · H3 and H1 (§10, already correct) · decode-side **body** mutation (§9, no reachable defect) · pseudo-header mutation semantics (§5, skip is the reference-faithful and H1-parity answer).

---

## 2. Method, and what this stage refuted

Four probe agents on four disjoint detached worktrees, disjoint port bands (47600-47689), private scratch each; **none committed**, and each proved its tree clean. The controller ran the sentinel battery, re-derived every load-bearing agent claim by execution, and ran two probes of its own. **Three agent claims were corrected or refuted by that re-derivation, one of them a claim the controller itself had made first.**

| # | Claim as received | Verdict at this tip |
|---|---|---|
| 1 | ADR-0071 forecloses a slice-native decode chain | **REFUTED.** ADR-0071's body contains ZERO occurrences of `http.Header`, `OrderedHeaders`, `signature` or `stability` — with `DecodeHeaders` reading **1** as the live positive control. §4.3 |
| 2 | `SetH2Request` has zero unit-test coverage, unlike `SetRequest` | **HALF-REFUTED.** Both read 0. The apparent 4 `SetRequest` test files are all `SetRequestCtx`, a different symbol on a different type. §6.4 |
| 3 | (controller's own) `0004` is the only downstream-H2 fixture | **REFUTED BY THE CONTROLLER'S OWN SECOND PROBE.** Three more exist with driver-inline configs and no `envoy-go.yaml`. §6.1 |

---

## 3. Q1 — **D-89-SHAPE**: the reconciler shape, DECIDED BY COMPILING PROTOTYPE

**FROZEN: a DELTA against a pre-decode snapshot of `c.req.Header`.** Reject verbatim reuse of `filter_http.ReconcileOrderedHeaders`; reject any whole-map projection (the full-rebuild shape); reject the slice-native chain.

All three were BUILT. The discrimination is not an argument, it is a measured 4-of-4 test flip on both sides.

### 3.1 Is `filter_http.ReconcileOrderedHeaders` reusable VERBATIM? **NO — and it COMPILES, so reading types answers this WRONG.**

`ReconcileOrderedHeaders(original OrderedHeaders, merged http.Header) OrderedHeaders` (`internal/filter/http/types.go`, cite BY SYMBOL) canonicalizes key case and does not skip `:`-prefixed keys. `hpack.HeaderField` and `filter_http.HeaderField` are structurally identical, so the conversion is trivial and **`go build ./...` returns 0 with zero diagnostics** (symbol asserted in the built binary at **2** occurrences). The failure is entirely at runtime. Against a raw-framer backend that prints decoded fields verbatim in wire order:

```
BACKEND-HDR  0 :method: GET            <- the router's own correct pseudo prefix
BACKEND-HDR  1 :path: /probe-path
BACKEND-HDR  2 :scheme: http
BACKEND-HDR  3 :authority: 127.0.0.1:47602
BACKEND-HDR  4 User-Agent: curl/8.5.0  <- CASE violation (RFC 9113 §8.2)
BACKEND-HDR  5 Accept: */*
BACKEND-HDR  6 X-Client: c1
BACKEND-HDR  7 X-Zzz-Last: z
BACKEND-HDR  8 :authority: 127.0.0.1:47602   <- pseudo-header AFTER regular headers,
BACKEND-HDR  9 :method: GET                     AND duplicated (RFC 9113 §8.3, twice over)
BACKEND-HDR 10 :path: /probe-path
BACKEND-HDR 11 X-Probe: seen
```

Against a **conformant** `h2c.NewHandler` backend the same binary yields **HTTP/2 502 with ZERO backend requests** (backend log shows only its READY line), while the tip binary yields 200 with `n=1`. This is a hard connection-level break, not a cosmetic divergence.

⚠️ **A raw-framer backend is mandatory to see this.** A `net/http`+`h2c` backend normalizes case, hides pseudo-headers and destroys order — it would have shown a green-looking result for an illegal wire encoding.

### 3.2 Patching those two edges is STILL insufficient — the tracing deletion

With an OTLP tracing provider configured, across three binaries:

| | `x-request-id` | `traceparent` |
|---|---|---|
| tip | present | present |
| verbatim-reuse arm | **DELETED** | **DELETED** |
| DELTA prototype | present | present |

**Any whole-map projection — including a "fixed" one that skips `:` keys and lowercases — sees these names as absent from the map and drops them**, because the tracing seam writes them onto the carrier and never onto the map (§1).

### 3.3 The same failure, reproduced INDEPENDENTLY through the unit suite — the isolating discriminator

A second agent built the **FULL REBUILD** shape (a separate worktree, `+39/−0`, 17 substantive code lines, `go build` RC=0, `gofmt`/`go vet` clean). Its census, controller-re-run with the denominator asserted:

| tree | `-run TestWriteH2_Tracing`, `=== RUN` count | result |
|---|---|---|
| tip (a worktree proved clean) | — | `ok` |
| **FULL REBUILD** | **4** | **4 of 4 FAIL** |
| **DELTA prototype** | **4** | **4 of 4 PASS** |

Failure lines read `forwarded x-b3-spanid = "", want a 16-hex id` and `forwarded x-request-id = "", want a 36-char id`.

**This is a 4-of-4 isolating discrimination between the two candidate shapes, measured on both sides with a green baseline.** It is the decisive evidence for D-89-SHAPE, and it was produced by a shape the BRAINSTORM had not flagged as risky at all.

⚠️ **The DELTA shape is immune for a structural reason, not by luck:** a delta only applies *changes observed in the map* to the slice, so a field that exists only on the slice is never a delta member and is never touched.

### 3.4 The slice-native chain — REJECTED ON MEASURED BLAST RADIUS, **NOT** on ADR-0071

The signature swap (`http.Header` → `*OrderedHeaders` on `FilterChain.RunDecodeHeaders` and the `StreamDecoderFilter` interface) is 2 lines. Measured consequences:

- **21 distinct production `DecodeHeaders` implementations** across 21 files; `go build ./...` fails with **138 lines across 22 distinct files** — ⚠️ **a LOWER BOUND: the build aborts before `internal/filter/hcm/*` and before any `_test.go` is typechecked**, which is why the test-file error count reads 0 despite **315** `_test.go` lines referencing `DecodeHeaders(`.
- Union of files that must actually change: **33 production, 38 test**.
- **The hidden multiplier: `OrderedHeaders` has NO mutation API.** It carries exactly two methods, `Get` and `ToHTTPHeader`. There is no `Set`/`Add`/`Del`/`Values`. Production call sites needing one: **101 lines** in `internal/filter/http/`. The type must be written before the swap can even be attempted.
- `wasm`, `lua` and `extproc` expose the header map across a guest/plugin boundary, so the ordered type must be marshalled there too (**NOT MEASURED**).

⚠️ **AND THE FORECLOSURE EVERYONE CITES DOES NOT EXIST.** `internal/filter/http/types.go` carries the comment *"per ADR-0071's filter API stability: EncodeHeaders takes http.Header, not OrderedHeaders"*, echoed in six `PROGRESS.md` sites, three `REVIEW.md` sites, one fixture README and one `h2dispatch.go` comment. **ADR-0071's body contains the phrase nowhere.** Controller-measured with a live positive control:

```
ADR-0071 spans DECISIONS.md lines 2586..2634 (48 lines)
  'http.Header':     0
  'OrderedHeaders':  0
  'signature':       0
  'stability':       0
  'DecodeHeaders':   1     <- POSITIVE CONTROL: the extraction is live
```

This is `reference_code_comment_not_evidence` and `reference_brainstorm_adjective_acquires_adr_authority` in one artifact: a Task-19 code comment that acquired ADR authority through eleven citations. **The PLAN and IMPL must write "rejected on measured blast radius (33 production + 38 test files, plus a mutation API that does not exist)" and MUST NOT write "rejected per ADR-0071".** Recorded as a documentary defect in §14; **not fixed here** (fixing it is a doc row, not this charter).

---

## 4. Q2 — **D-89-SITE**: where the snapshot is re-issued

**FROZEN: keep `rf.SetH2Request(h2req)` exactly where it is, and RE-ISSUE it after the reconcile. Do NOT move the existing call.**

### 4.1 The value copy is real, and its failure mode is CORRUPTION, not a clean no-op

`router.Filter` stores `req *http.Request` (POINTER) but `h2Req h2.H2Request` (**VALUE**); `SetH2Request(r h2.H2Request)` takes a value; `RunAction`'s H2 arm passes `f.h2Req` by value. Confirmed by reading the field type, not the comment.

⚠️ But `H2Request.Headers` is a **slice**, so the copy **aliases the backing array**. The controller ran this against the REAL repo types, observed through the router's OWN action closure — the exact seam the upstream HEADERS block is built from:

| post-Set mutation | what the upstream actually sees |
|---|---|
| **APPEND** a field | `len=3`, the appended field **INVISIBLE** — matches the observed defect |
| **IN-PLACE VALUE EDIT** of an existing field | **VISIBLE.** `a: "MUTATED"` propagates through the value copy |
| **REMOVE via the `fields[:0]` idiom** | ⚠️ **`[a, c, c]`, len=3 — the removal does NOT take effect AND `c` is DUPLICATED on the wire** |
| **REMOVE into a FRESH slice** | original 3 unchanged; removal simply lost, no corruption |

**The third row is the trap, and the repo's own helper walks into it:** `upsertH2Header` begins `out := fields[:0]` and compacts in place over the caller's backing array. A reconciler that reused that idiom after the Set would convert a lost-mutation bug into a **wrong-bytes-on-the-wire** bug.

⇒ **Two binding constraints on the IMPL: (a) re-issue the Set; (b) build a FRESH slice — never compact over the array the router already holds.**

### 4.2 The early-exit enumeration was INCOMPLETE — there are THREE, not two

| # | anchoring string | in the BRAINSTORM? |
|---|---|---|
| 1 | `if _, err := chain.RunDecodeHeaders(ctx, c.req.Header, endStreamOnHeaders); err != nil {` → `return err` | yes |
| 2 | `if chain.LocalReplyDone() {` → `return werr` | yes |
| 3 | `if _, err := chain.RunDecodeData(ctx, h2req.Body, true); err != nil {` → `return err` | ⚠️ **NO — MISSED** |

Checked and excluded: ctx cancellation is not a separate path (it surfaces as #1); `recover()` appears **nowhere** in `internal/filter/hcm/`, so a panic unwinds past `RunAction` unhandled — an exit, but not a reachable-and-handled one.

**None of the three issues an upstream request**, so none needs a reconcile — but the re-issue site must sit **after `RunDecodeData`**, not merely after `RunDecodeHeaders`, so path 3's success case is covered. Executed confirmation: the `LocalReplyDone()` exit was driven with a `local_ratelimit` (`max_tokens: 1`) two-request arm — tip and prototype **identical** (200 then 429, exactly 1 backend request each). ⚠️ The `RunDecodeHeaders`-error exit is **CODE-READ, NOT EXECUTED** — no config lever reaches it; the PLAN owes a unit-level arm.

### 4.3 Why moving the Set is REJECTED

`RunAction`'s H1 arm **panics** on a missing `SetRequest` (`panic("router.Filter.RunAction: SetAction set without SetRequest …")`). **The H2 arm has no such guard** — it comments *"h2Req is a value type so the zero value is allowed."* Moving the Set below the decode chain means any path reaching `RunAction` without it round-trips a **zero `H2Request`** — no `:method`, no `:path` — converting a header bug into a silently-malformed-upstream-request bug, unguarded.

### 4.4 A second-order effect the PLAN must decide about deliberately

`emitAccessLogH2(req h2.H2Request, …)` takes the H2Request **by value** at **five** call sites and scans `req.Headers` twice — for `x-client-trace-id` and, via `h2UserAgent`, for `user-agent`. **A reconciler that changes `h2req.Headers` therefore also changes access-log and span content.** That is arguably correct (the log should reflect what was sent), but it is a behaviour change the PLAN must state rather than discover.

---

## 5. Q3 — **D-89-PSEUDO**: skip every `:`-prefixed key. **Do NOT route to the scalar fields.**

The BRAINSTORM named this "the row's most likely hidden cost." **There IS a hidden cost and it runs the opposite way: projecting pseudo-headers is not an edge case, it is TOTAL, and it breaks the fix outright.**

### 5.1 The reference

| arm | result |
|---|---|
| `header_mutation` **append** `:path` / `:authority` / `:method` / `:scheme` / `host` | **BOOT-REJECT**, exit 1, verbatim: `:-prefixed or host headers may not be modified` |
| `header_mutation` **remove** `:path` / `host` | **config-ACCEPTS** (exit 0) — and applies it at runtime: **503 `missing required header: :path`**, backend count unchanged, identical on the H1 and H2 legs |
| **Lua** `replace(":path", …)` / `replace(":authority", …)` | **APPLIED** — upstream sees `/mutated-path` and `mutated.example`, **and the route re-selects** (both requests landed on a different cluster). No H1/H2 difference. |

### 5.2 The subject

envoy-go's `header_mutation` boot-rejects pseudo-headers *and* `host` via `isProtectedHeader`, which returns true for **any** `:`-prefix plus a case-insensitive `host`. Message: `header_mutation: ":path" is :-prefixed or host; may not be modified`.

⚠️ **A pre-existing divergence found and deliberately NOT chartered:** the subject rejects **remove** of a protected header too; the reference config-accepts it. Recorded in §14.

**The unguarded surface is the programmatic filters.** `http.Header.Set/Add/Del` does **not** canonicalize a colon key away — measured: `CanonicalMIMEHeaderKey(":path")` returns `":path"`, and the raw map after `Set` holds `":path"` verbatim. Writers into the decode map with **no** `isProtectedHeader` consultation: **lua** (`headersAdd`/`headersRemove`/`headersReplace` — script-supplied names), **wasm** (`AddHeaderMapValue`/`ReplaceHeaderMapValue`/`RemoveHeaderMapValue` — guest-supplied), **extauthz** (`applyUpstreamMutations` — authz-server-supplied), plus `jwtauthn` and `ratelimit` with config/RLS-supplied names. **Lua, wasm and extauthz can put a raw `:path` into `c.req.Header` today.**

### 5.3 Why SKIP, on three independent measured grounds

1. **H1 parity — the charter's own target — says skip.** Measured: on the subject's H1 leg, a Lua `:path`/`:authority` map mutation is **silently ignored** (upstream path and authority unchanged, route unchanged) while the regular-header mutation lands. H1 writes the upstream request from the frozen `req.URL`/`req.Host` scalars, not the map. Routing pseudo-headers to the H2 scalars would make **H2 exceed H1**, minting a new asymmetry in the opposite direction — and reference fidelity would additionally demand route re-selection, far outside this charter.

2. **Projecting them is CATASTROPHIC AND UNIVERSAL.** `h2dispatch.go` injects `:method` / `:authority` / `:path` into `c.req.Header` **unconditionally on every request**, whether or not any filter touches them. The upstream HEADERS block is built as the four scalars then `append(headers, req.Headers...)`. A projecting reconciler therefore emits duplicate pseudo-headers *after* the regular headers on **every H2 request**. Fired at a conformant `x/net/http2` server:

   ```
   A control: correct pseudo-first order                        -> HEADERS :status=200 + DATA (200 OK)
   B hazard:  3 pseudo-headers duplicated after regular headers -> RST_STREAM code=PROTOCOL_ERROR
   C hazard:  a single extra :path after regular headers        -> RST_STREAM code=PROTOCOL_ERROR
   ```

   The skip is not a preference; **the fix does not function without it.**

3. **Skipping widens no existing divergence.** Via `header_mutation` both sides boot-reject (parity). Via Lua the reference applies and the subject ignores **on both legs** — a pre-existing, codec-independent gap that skipping leaves exactly where it is.

### 5.4 The one open sub-question the PLAN must close

**`Host`/`host` is NOT settled and must not be settled by omission.** A Lua `replace("host", …)` writes canonical `Host` into the map; projecting it onto the slice duplicates authority information on the H2 block (legal HTTP/2, unlike the pseudo case, but divergent). **NOT MEASURED.** The PLAN decides explicitly: skip `host` alongside the `:` prefix, or project it and measure the reference. Defaulting silently is what produced the row being fixed.

---

## 6. Q4 — **D-89-PROOF**: extend `0004-h2-routing` IN PLACE. **Fixtures stay 121; every count axis +0.**

### 6.1 The corpus fact that forces the shape

The set of fixtures with a downstream-H2 listener and the set whose backend **enumerates request headers** are **DISJOINT**. Measured across BOTH backend families:

- Downstream H2: **four** fixtures — `0004-h2-routing` (yaml) plus `0079-h2-multiplex-pool`, `0080-h2-goaway-rotation`, `0119-grpc-unary-trailers` (⚠️ **driver-inline configs, no `envoy-go.yaml` at all** — a yaml-only scan is blind to them; the controller's first probe was, and was corrected by its second).
- Backends enumerating `r.Header`: **two** — `0012-http-header-mutation` and `0015-http-buffer`, **both `codec_type: HTTP1`**. There are only 12 per-fixture backend `.go` files; the rest are in-process BackendKinds in `test/differential/`, and **none of those enumerates request headers either**.

⇒ **No existing fixture has both halves of what this row must observe.** Something must be added whichever option is chosen.

Census re-derived at this tip in OCCURRENCE form: `HTTP1 270 · HTTP2 0 · HTTP3 3 · AUTO 6`. ⚠️ `codec_type: HTTP2` **does** exist repo-wide (`test/conformance/h2spec/h2spec_test.go`, `cmd/envoy-go/main_test.go`) — the zero is scoped to `test/fixtures/`. Do not call the shape unprecedented repo-wide.

### 6.2 Four options, priced

| | **A** extend `0004` | **B** mint `0120` | **C** extend `0012` | **D** plumb `--allow-h2c` |
|---|---|---|---|---|
| feasible | **YES** | yes | ⚠️ **NO — structurally blocked** | yes, but buys little |
| measured anchor | **phase 88 did the identical shape at the identical fixture: `+172/−7`, 4 files** | `0079` = 1039 lines, `0119` = 1134 (incl. PEMs) | ≥ B | harness edit = **2 lines** |
| est. insertions | **~145-200** | ~1150-1300 | > 1300 + regression risk | 2 + all of B's backend cost |
| fixtures 121 → | **121 (+0)** | 122 | 121 | 121 |
| blank imports 121 → | **121 (+0)** | 122 | 121 | 121 |
| BackendKind tail 38 → | **38 (+0)** | 39 | 39 | 39 |
| new port | **none** | 10120 | none | none |
| PKI | **already present** | 3 PEMs copied (precedented) | 3 PEMs copied | none |

**Option C is structurally impossible, and this is the sharpest finding of §6.** On a downstream-H2 listener `h2dispatch` unconditionally takes the H2 action path into `doH2ClusterAction` → `AcquireH2Stream` — **downstream H2 FORCES upstream H2**, with no `Cluster.UseH2()` consultation. And a fixture declares exactly **one** `BackendKind`. `0012`'s header-echoing backend is plaintext HTTP/1.1, so adding a downstream-H2 listener to `0012` forces an H2 upstream its sole backend cannot serve, and `0012` cannot declare a second kind. C would require minting the new backend anyway **plus** regressing a landed driver.

⚠️ **Silver lining, measured: upstream H2 does NOT need TLS** (`H2HoldResponder` and `H2GoawayResponder` are documented in-process **h2c prior-knowledge** responders). Only the DOWNSTREAM leg needs TLS+ALPN.

**Option D is priced and rejected.** The flag is provably permissive-only — exactly **one** decision site, `if !lc.HasTLS && !lc.AllowH2C {` — and MEASURED discriminating on the subject (`--mode validate` without it: `codec_type HTTP2 requires TLS transport_socket (or --allow-h2c …)`, exit 1; with it: `configuration OK`, exit 0). The reference needs no flag at all (executed: real Envoy boots and serves h2c prior-knowledge with a plaintext `codec_type: HTTP2` listener). But proving inertness across the corpus requires a full 121-fixture run, and it makes a **conformance-test-only flag part of the differential subject's standard launch**, weakening the differential's own claim that the subject runs as shipped. **For ~50-70 saved lines of TLS config against a cross-cutting gate risk, the trade is poor.**

### 6.3 Why A works, and what it costs nothing

`0004`'s backend reads two named probe headers today and does **not** enumerate. The surface to add is precedented **one phase ago**: phase 88 added an exact-match handler to that same backend behind the comment *"Exact-match patterns beat the `/api/v1/` subtree handler above, so no existing behavior moves."* A second exact-match handler that enumerates and sorts `r.Header` (copying `0012`'s ten-line pattern) **cannot disturb the existing 31 round-trips by the same mechanism**.

⚠️ **The H1 positive control costs ZERO and needs no new fixture: it is the already-landed, already-green `0012-http-header-mutation`** — the H1 arm of the same filter, and already the contract's named gate fixture. The row cites `0012` as the control rather than minting one.

### 6.4 The unit surface the row owes — this is NOT optional

⚠️ **`0004` configures no tracing, so NO fixture arm on `0004` can catch the tracing-header-loss regression of §3.2/§3.3.** That regression is the row's largest hazard and it must be pinned by a **unit** test.

| file | obligation |
|---|---|
| `internal/filter/hcm/tracing_zipkin_dispatch_test.go` | ⚠️ **MANDATORY** — the only test file referencing `upsertH2Header`; the tracing-loss pin lives here. It is what reddens if a future author replaces the delta with a projection. |
| `internal/filter/hcm/h2dispatch_test.go` | the reconciler table-test (add / remove / value-change / multi-value / pseudo-skip / case / order / no-op passthrough) |
| `internal/filter/hcm/h2/stream_test.go` | the only file covering `buildH2Request` / `buildRequest` |
| `internal/filter/http/router/router_test.go` | ⚠️ **a `SetH2Request` arm — currently ZERO exists** |

**The setter-coverage gap, measured with the QUALIFIED name and a live positive control** (`reference_symbol_assertion_needs_qualified_name`):

```
SetH2Request : 0 test files   | substring forms present: []
SetRequest   : 0 test files   | substring forms present: [SetRequestCtx]
SetH2Action  : 0 test files   | substring forms present: []
SetAction    : 2 test files   | substring forms present: [SetAction]     <- POSITIVE CONTROL
```

⚠️ **A bare-substring form reads 4 files for `SetRequest` and every one is `SetRequestCtx` — a different symbol on a different type.** The real asymmetry between the H1 and H2 setters is **not** test coverage (neither has any); it is the runtime guard of §4.3.

### 6.5 The three registration gates (Option A needs none of them, but the PLAN must not forget them if it deviates)

1. **Directory-name gate** — `discoverFixtures` accepts only `NNNN-…`/`NNNNa-…`. **The runner never reads `envoy-go.yaml`; the driver does** — which is why driver-inline fixtures work identically.
2. **Registry gate** — `fixture.RegisterFixture(fixtureName, drv)` in the driver package's `init()`, byte-equal to the directory name.
3. ⚠️ **Blank-import gate** — `_ ".../test/fixtures/NNNN-…/driver"` in `test/differential/runner_test.go`. **This is the silent one:** without it the registry misses and the runner fires `t.Skipf("no driver registered for fixture …")` — a **SKIP, not a FAIL** ⇒ silently green. The gate is the NARROWED form `grep -cP '^\t_ "[^"]*test/fixtures/'` = **121**; the unnarrowed `^\t_ "` reads 123 and is REFUTED (two non-fixture imports).

Port convention if the PLAN ever deviates to B: **`10<index>`**, not max+1 — measured on four consecutive siblings (`0119→10119`, `0118→10118`, `0117→10117`, `0012→10012`), with two documented family-banded outliers (`0004→15004`, `0111→10447`).

---

## 7. Q5 — the encode-side bound: **CONFIRMED CORRECT, charter cannot widen**

Same binary, same run, both legs, `response_mutations: append x-resp-test=encode-ok`:

```
SUBJECT H2 leg                     SUBJECT H1 leg
HTTP/2 200                         HTTP/1.1 200 OK
x-resp-test: encode-ok   <---      X-Resp-Test: encode-ok   <---
```

The encode side reconciles via `filter_http.ReconcileOrderedHeaders` at its existing call site and is correct on H2. **The charter is bounded to the decode direction.**

The same run independently re-confirmed **both** decode arms: on H2 the backend saw only `Accept`/`User-Agent` (**addition lost**) while H1 saw `X-Test`; and `user-agent` survived on H2 (**removal ignored**) while H1 showed the remove taking effect.

---

## 8. Q6 — **D-89-DOC**: close the carve-out IN PLACE; the mirror sentence stays

### 8.1 The equivalence row — verified verbatim; it is an HONEST CARVE-OUT

The `envoy.filters.http.header_mutation` row asserts *"Per-request equivalence on post-mutation request headers (visible at upstream backend) and post-mutation response headers (visible at downstream client)"* … and ends:

> `NOT asserted: header-value formatter substitution (deferred — ADR-0113), `query_parameter_mutations` (deferred — ADR-0112), H2 differential coverage.`

**Every asserted clause is true at this tip.** ⚠️ **There is NO false sentence to correct** — reporting one would repeat the phase-88 error of asserting a divergence the reference does not have.

**The exact substring the IMPL removes: `, H2 differential coverage.`** — deleted from the NOT-asserted list (the other two items STAY), with the asserted half extended to name the H2 arm. ⚠️ **This is an IN-PLACE edit of one markdown table row: ZERO line delta**, so no by-line citation into `BEHAVIOR_CONTRACT.md` (**5960** lines) shifts. That matters: phase 88 shifted these anchors +2 and phase 87 +1. **If the IMPL instead adds a bullet anywhere, that IS line-adding and it must say so.**

### 8.2 The mirror sentence — **EXACTLY TRUE. It needs NO edit. Here is the mechanism, not an opinion.**

Under `### Does not yet apply to` in the tracing section:

> `- H2 decode-side observation of the injected headers (the H2 inject mutates the upstream-forwarded header set only, not the decode-side map; H1 mutates `req.Header` which is both).`

1. **Its factual half is measured true** — the tracing block touches `c.req.Header` **zero** times (§1).
2. **A reconciler runs strictly AFTER `RunDecodeHeaders` returns.** "Decode-side observation" is what filters see *during* that call. **A post-hoc reconciliation cannot retroactively change what already-executed filters observed.** The ordering makes the sentence immune.
3. **The data flow is the INVERSE.** This row moves map → slice; the tracing gap is the missing slice → map direction. Fixing one leaves the other exactly as documented.

⇒ **No edit.** It would become stale only if the row *additionally* mirrored the tracing inject onto `c.req.Header` — a separate change, out of charter. ⚠️ Conversely, a projection reconciler would not make it stale either; it would create a **NEW** divergence needing a **NEW** bullet. One more argument for the delta.

---

## 9. Q7 — **D-89-BODY**: decode-side body mutation is OUT OF CHARTER. There is no reachable defect.

`DecoderFilterCallbacks` has **20 methods and none writes the decode body** (`SendLocalReply`'s `body` synthesizes a *response*). `EncoderFilterCallbacks` carries `OverwriteBody`; there is **no `DecodeBodyOverride` symbol anywhere in the tree**. Every `DecodeData` implementation returns only `FilterDataStatus` — there is no return channel for mutated bytes.

The project has already landed its own admission, verbatim in `internal/filter/http/extproc/extproc.go`:

> *"the body bytes themselves are NOT delivered upstream because envoy-go has no decode-side analog of `OverwriteBody` … the HCM reads the upstream-bound bytes from its own `bodyBuf` (H1) or `h2req.Body` (H2), both of which are captured BEFORE the filter chain mutation lands."*

The two filters that could mutate a request body write to **filter-owned** accumulators, not the HCM's slice.

⇒ **"No reachable defect", not "a defect". H1 and H2 do NOT diverge on this axis; the charter is not widened.** *(Established by an executed `go doc` method-set census plus code reading; no wire arm was run, because no filter exists that could drive one.)*

⚠️ One latent inversion recorded so a future decode-body primitive does not assume symmetry: H1 copies each chunk into `bodyBuf` **before** `RunDecodeData`, so even an in-place same-length mutation is lost; H2 passes `h2req.Body` — the very slice the router later hands to `RoundTrip` — so a hypothetical in-place mutation **would** land on H2 and not on H1. Unreachable today.

---

## 10. Q8 — H1 and H3 are clean. **Two-of-three HOLDS.**

Located by symbol; all four anchors exact at this tip.

| codec | snapshot | decode-chain call | verdict |
|---|---|---|---|
| H1 | `connection.go:468` `rf.SetRequest(req)` | `:571` `RunDecodeHeaders(ctx, req.Header, …)` | **CORRECT — same object** |
| H3 | `h3dispatch.go:217` `rf.SetRequest(r)` | `:263` `RunDecodeHeaders(ctx, r.Header, …)` | **CORRECT — same object** |
| H2 | `h2dispatch.go:457` `rf.SetH2Request(h2req)` | `:518` `RunDecodeHeaders(ctx, c.req.Header, …)` | **BROKEN — different container** |

`SetRequest` stores the **POINTER**, and `RunDecodeHeaders` receives the `http.Header` map reachable through it. Filters mutating that map mutate exactly what `RunAction`'s H1 arm hands upstream. **H3 stays explicitly OUT OF CHARTER.**

---

## 11. Q9 — h2spec is NOT an anchor, and the reason is stronger than ADR-0307

ADR-0307 records h2spec as MEASURED not to gate either side, and the phase-88 IMPL re-confirmed the mechanism (§6.10 CONTINUATION passed 6/6 at a tip that discarded every CONTINUATION frame). **This row can say something sharper, measured here:**

The h2spec harness's HCM configures `http_filters:` with **`envoy.filters.http.router` ALONE**, and every route is a `direct_response` — the harness never goes upstream at all. **There is no decode-mutating filter in the chain and no upstream request, so there is no mutation to lose.** h2spec is *structurally* incapable of anchoring this row.

**This SPEC cites NO h2spec figure.** The IMPL runs the suite as a regression gate and cites only from its own run.

---

## 12. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT RECORDED

⚠️ **A SPEC edits no ROADMAP row. The binding proof is an EMPTY `ROADMAP.md` diff against master** — `git diff master -- docs/envoy-go/ROADMAP.md` returned **0 bytes** at stage start and is re-asserted at close. `want` stays **121**; row 89 stays `in-progress`.

**Measured at this tip — 239 lines / 121 data rows:**

| check | output |
|---|---|
| (1) `want=121` | **`NOT DONE: row 89`** ALONE, denominator silent |
| (2) union form | **SIX** at `:199 :205 :211 :221 :227 :235` |
| (3) | **SILENT** |

⇒ the conjunction FAILS; **the sentinel does NOT fire; `stop` was NOT created** (verified absent at the git root).

**ALL FOUR NCs FIRED:**

- **row-62 doctoring** — `NC LANDED? [ in-progress ]` INSPECTED FIRST, then `NOT DONE: row 62` **AND** `NOT DONE: row 89` (the correct two-line output once a real row is in-progress).
- **denominator** — `want=120` on the real file gave `NOT DONE: row 89` **plus** `GATE FAIL: examined 121 data rows, expected 120`.
- **check-(3) doctoring** — residual `gRPC-family row` **2 → 0** confirmed on the doctored copy FIRST, then `NEVER OPENED: gRPC` fired ALONE with WASM correctly silent.
- **check-(2) one-arm** — long **5** / short **1** / union **6**. A one-arm strip is NOT an NC for the union.

**Every leak axis at this tip:** `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.

**Row 89 is WELL-FORMED**: `fields=8`, with row 88 as the control also reading `fields=8` (the phase-89 BRAINSTORM's trap 1 — a malformed row passes check (1) silently because stray pipes fall after field 5).

---

## 13. Counts and cost

### 13.1 Counts re-derived MECHANICALLY at this tip

`DECISIONS.md` **18208** lines · **309** headings · tail **ADR-0310** · next-free **ADR-0311** (TAIL-derived; `grep -c '^## ADR-0311'` = **0**; ⚠️ headings+1 COLLIDES at the ADR-0209 gap) · STATUS census **23** · `^---$` **216** ⚠️ **a new ADR takes NO `---` separator** · strict `PROPOSED` guard **0 → 1 at this stage**. ⚠️ **Carry no whole-file count of the LOOSE `PROPOSED` matcher** — it reads 1 at an ADR-0231 entry in the older non-blockquote form; only the strict form is the guard.

`BEHAVIOR_CONTRACT.md` **5960** · `STATE.md` **64** · `STATE_HISTORY.md` **500** · `BOOTSTRAP_PROMPT.md` **522** · phase dirs **130** · `REVIEW.md` **37** (standing departure).

Fixtures **121** at `test/fixtures/` (⚠️ **not** `test/differential/fixtures/`, which returns a silent 0); numeric tail `0119-grpc-unary-trailers`; **`0120` STAYS UNCONSUMED** under D-89-PROOF. Narrowed blank imports **121**. Fuzzers **55 / 48**. BackendKind tail **38**. Archive labels in `STATE_HISTORY.md` **199**.

**Stat surface — the COMMAND binds, not the number:** `grep -ro --include='*.go' -e 'NewCounter(' -e 'NewGauge(' . | wc -l` = **406** (`NewCounter(` 327 + `NewGauge(` 79); `-rn` form = **404** lines. **Measured 406 on the DELTA prototype too ⇒ DELTA 0.**

**Anticipated at the IMPL, each +0:** stat surface · fuzzers · BackendKind tail · `go.mod` (`git diff --stat -- go.mod go.sum` EMPTY on the prototype) · config fields · fixture count · blank imports.

### 13.2 Cost — MEASURED, and quoted as a FLOOR

```
git -C <prototype> diff --numstat
162     0       internal/filter/hcm/h2dispatch.go
```

**+162 / −0 net production `.go`, ONE file, ZERO test files, ZERO other files.** Controller-re-measured independently of the agent that produced it. Composition, counted not estimated: **69 comment lines (42.6%)**, 5 blank, 24 brace/paren-only, **64 substantive executable lines**.

⚠️ **This is 1.8×-3.2× over the BRAINSTORM's ~+50-90 estimate.** Even the comment-free reading (64) sits at the top of that band, and the touched file's own comment density is ~43%, so a comment-free reading is not honest.

⚠️ **AND IT IS A FLOOR** (`reference_measured_prototype_is_a_lower_bound`, which fired on BOTH axes at the phase-88 IMPL: +284 against a ~+190-240 band). **Enumerated, not gestured at — the prototype does NOT implement:**

1. **Outbound RFC 9113 §8.2.2 validation.** `buildRequest` rejects uppercase names, connection-specific fields and `te != trailers` on the **inbound** path. The prototype re-emits whatever a filter wrote. ⚠️ **MEASURED on the sibling shape: `CONNECTION_SPECIFIC_LEAKED=[connection transfer-encoding]`** — and `connection`/`transfer-encoding` are **not** in `header_mutation`'s protected set, so this is reachable through the very filter the row's fixture uses. ⚠️ **This hazard does not exist at the tip** (nothing reaches upstream today), so **the fix INTRODUCES it** — the phase-88 shape where the mitigation is owed by the change itself.
2. **The `Host`/`host` decision** (§5.4).
3. **Duplicate-name collapse** — a name appearing at two non-adjacent wire positions collapses to the first. Deliberate, but a wire-order divergence that needs a reference measurement before it is contracted.
4. **Zero tests** — the four files of §6.4.
5. **Zero differential arms** — the `0004` extension of §6.3.
6. **The `RunDecodeHeaders`-error early exit is unexercised** (§4.2).
7. **Zero ADR / contract / PROGRESS lines.**
8. No `-race`, no full differential suite, no H3-parity re-check.

**IMPL bands, stated as bands and labelled:** production **~+165-230** (the +162 floor plus items 1-3, ESTIMATED) · unit tests **~+150-350** (ESTIMATED) · differential **~+145-200** (anchored on phase 88's MEASURED `+172/−7` into the same fixture, plus a `header_mutation` block on both yaml sides).

### 13.3 Gates on the prototype — all green, controller-re-run

`go build ./...` rc=0 · `go test ./internal/filter/hcm/... ./internal/filter/http/...` **rc=0, 25 `ok`, 0 `FAIL`** · the four `TestWriteH2_Tracing*` rows **4/4 PASS with the `=== RUN` denominator asserted at 4** · `gofmt -l` OUTPUT empty · `go vet` clean · `golangci-lint` clean · `go.mod`/`go.sum` diff EMPTY · stat surface **406 both sides**.

---

## 14. Findings the PLAN must not re-learn

1. ⚠️ **THE TWO-CONTAINER MODEL IS INCOMPLETE — THERE IS A THIRD WRITER, AND IT DECIDES THE SHAPE.** The tracing seam writes to the slice only. Any whole-map projection deletes `x-request-id`/`traceparent`/`tracestate`/`X-B3-*` from the upstream request. Measured three independent ways: an OTLP wire probe across three binaries, a 4-of-4 unit-test flip with a green baseline and the denominator asserted, and a `grep -c 'c\.req'` = 0 over the tracing block. **A slice-only-writer inventory is an explicit PLAN deliverable** — a future author adding another slice-only producer re-opens this.
2. ⚠️ **A LANDED CODE COMMENT ACQUIRED ADR AUTHORITY ACROSS ELEVEN CITATIONS.** ADR-0071 does not contain the `http.Header`-vs-`OrderedHeaders` sentence; `internal/filter/http/types.go` does. **Reject the slice-native shape on measured blast radius, never "per ADR-0071".** Verified with a live positive control.
3. ⚠️ **THE VALUE COPY ALIASES ITS BACKING ARRAY, SO THE NAIVE REMOVE CORRUPTS RATHER THAN NO-OPS.** In-place value edits DO propagate; appends do not; and a `fields[:0]` compaction — **the idiom the repo's own `upsertH2Header` uses** — leaves the router's snapshot reading `[a, c, c]` with a duplicated field. **Build a fresh slice and re-issue the Set.**
4. ⚠️ **PSEUDO-HEADER PROJECTION IS TOTAL, NOT AN EDGE CASE.** The three injections happen on **every** request, so a projecting reconciler emits duplicate pseudo-headers on **every** H2 request — `RST_STREAM PROTOCOL_ERROR` at a conformant peer, 502 with **zero** backend requests end-to-end.
5. ⚠️ **A RAW-FRAMER BACKEND IS MANDATORY.** A `net/http`+`h2c` backend normalizes case, hides pseudo-headers and destroys order — it shows green for an illegal wire encoding. And **assert the backend RECEIVED something (n>0) before interpreting what it received**; the BRAINSTORM's 502-with-zero-requests artifact is one probe cycle away at all times.
6. ⚠️ **`codec_type: AUTO` + `--allow-h2c` DOES NOT GIVE PLAINTEXT H2** — the BRAINSTORM's own probe recipe is wrong. `AUTO` on a plaintext listener serves H1 and curl reports `Remote peer returned unexpected data while we expected SETTINGS frame`. Only the `HTTP2` arm consults `AllowH2C`. **Use `codec_type: HTTP2` + `--allow-h2c`.** Two agents hit this independently.
7. ⚠️ **`hcm: h2: EOF` APPEARS ON THE TIP BINARY TOO.** It is a benign pooled-conn teardown artifact and is **not** evidence of anything. One agent nearly mis-attributed it.
8. ⚠️ **THE EARLY-EXIT LIST WAS 2 OF 3** — the `RunDecodeData` error return was missed. The re-issue site must sit after `RunDecodeData`.
9. ⚠️ **A BARE-NAME TEST-COVERAGE GREP COLLIDED WITH A DIFFERENT SYMBOL.** `SetRequest` reads 4 test files by substring and **all four are `SetRequestCtx`**. Use the qualified/word-bounded form with a live positive control (`SetAction` = 2). Both router setters actually have **zero** direct coverage.
10. ⚠️ **A YAML-ONLY FIXTURE SCAN IS BLIND TO DRIVER-INLINE FIXTURES.** Three downstream-H2 fixtures configure themselves from Go and have no `envoy-go.yaml`. The controller's first probe was blind and was corrected by its second. **Search the drivers too.**
11. ⚠️ **DOWNSTREAM H2 FORCES UPSTREAM H2, AND A FIXTURE HAS EXACTLY ONE BackendKind.** Together these make the "reuse `0012`'s header-echoing backend" option structurally impossible. Upstream H2 does **not** need TLS; only the downstream leg does.
12. ⚠️ **`--network host` DID NOT SHARE THE HOST NETNS IN THIS ENVIRONMENT.** The reference bound `47701`/`47702` inside its own namespace (visible in the container's `/proc/net/tcp`, absent from the host's `ss -ltn`), and published ports were unreachable too. The controller's reference order-probe is therefore **NOT MEASURED** — see §15. Sibling agents reached the reference successfully with the documented `--add-host=host.docker.internal:host-gateway` recipe; **prefer it over host networking.**

---

## 15. Explicitly NOT MEASURED (stated, never inferred)

- **Whether reference Envoy preserves inbound request header ORDER on the upstream leg.** The controller's probe failed on the container-networking issue of §14.12. ⚠️ **Not load-bearing:** D-89-SHAPE preserves order by construction (measured — every survivor's relative position identical between tip and prototype), so the SPEC never relies on the reference's order behaviour. It matters only as a residual argument against the REJECTED full-rebuild shape, which loses **7 of 10** positions on a browser-shaped request and is **86% nondeterministic without a sort** (172 of 200 runs differ).
- Whether the reference applies decode-side mutations over downstream **H2 with a Lua filter** (the `header_mutation` and H1/H2 reference arms WERE measured, §5.1).
- `Host`/`host` projection semantics (§5.4).
- Duplicate-name collapse against the reference (§13.2 item 3).
- The `wasm`/`lua`/`extproc` guest-boundary cost of the slice-native shape (§3.4).
- The insertion estimates in §6.2 for options A/B/D are **LOWER BOUNDS derived from measured yardsticks**, not measured prototypes. The exactly-measured figures are phase 88's `+172/−7` into `0004`, `0079`'s 1039 and `0119`'s 1134 total lines, and D's 2-line harness edit.

---

## 16. Documentary defects — recorded, deliberately NOT fixed

Carried forward from the BRAINSTORM, plus **three new at this stage**:

- ⚠️ **NEW: `internal/filter/http/types.go`'s "per ADR-0071's filter API stability" comment is FALSE** — ADR-0071 contains no such sentence. Echoed across six `PROGRESS.md`, three `REVIEW.md`, one fixture README and one `h2dispatch.go` site. **Fixing it is a doc row, not this charter.**
- ⚠️ **NEW: envoy-go's `header_mutation` rejects `remove` of a protected header where the reference config-ACCEPTS it** (and applies it, yielding 503 `missing required header: :path`). A real, measured, pre-existing divergence — **not chartered here**, and the reference's behaviour is arguably the worse one.
- ⚠️ **NEW: `router.Filter.RunAction`'s H2 arm has no `SetH2Request`-was-called guard** while the H1 arm panics. Named, not fixed; §4.3 uses it as the reason not to move the Set.
- Carried: `ADR-0051` §2's false gate-scope sentence · `ADR-0058`'s dead `routerActionH2.doH2` location · `ROADMAP.md` rows 119/131 malformed (ARM-A guard) · `STATE.md` §Project counts frozen at phase 76 · `harness_test.go` port inventory stale · `body.go` nolint inert · xDS cycle guard not automated · `wasm/doc.go:219` two errors · ROADMAP cites `esalaine` five times · `rbac.go:50` token `F2` · root `PROGRESS.md` stray phase-32.1 doc · SPEC-86/PLAN-86's nonexistent `internal/xds/xdsgrpc/...` path · `BEHAVIOR_CONTRACT.md`'s "no stdlib net/http parsing" sentence (retained, ridden by ADR-0309) · `ADR-0057`'s "27 round-trips" (now 31; ⚠️ `README.md:34`'s 27 is CORRECT for the pre-phase-87 prefix — do NOT fix) · the two riders citing ADR-0052 at a drifted `:1821` · window `:221`'s TWO closed candidates (the H2 `//`-path bug closed by row 87 — re-verified here, `h2/stream.go:478` now calls `url.ParseRequestURI`; and the `/stats/prometheus` gap consumed by rows 79/80).

---

## 17. What the PLAN owes

1. **Task decomposition for D-89-SHAPE** with TDD ordering — the RED census must be observed **before** the production edit, and the tracing pin must be in the RED set.
2. **The slice-only-writer inventory** (§14.1) as a named deliverable.
3. **The outbound RFC 9113 §8.2.2 validation decision** — in or out, with the reachability measured through `header_mutation` (§13.2 item 1). ⚠️ Remember the fix **introduces** this hazard.
4. **The `Host`/`host` decision** (§5.4), settled explicitly.
5. **The `0004` extension arm roster**, with a break protocol per arm and the injection site named (`reference_break_arm_injection_site_is_a_claim`).
6. **The unit roster** of §6.4, including the currently-zero `SetH2Request` coverage.
7. **A duplicate-name-collapse reference measurement**, or an explicit deferral in writing.
8. **The contract edit's exact substring and its zero line delta** (§8.1), plus the written finding that the mirror sentence stays (§8.2).
9. **Cost re-measured at the PUBLISHING commit** — `reference_cost_figure_measured_at_publishing_commit` went stale TWICE inside the phase-88 IMPL, the second time in the sentence correcting the first.
