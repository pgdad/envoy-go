# SPEC 87 — h2-double-slash-path-routing

**Stage:** SPEC (lifecycle-state 1 -> 2). **Date:** 2026-08-13.
**Base master:** `d7fd87fde7f377f0e9e6481319f0dc2915c668f9` (from `git rev-parse master`), branch `phase-87-spec`.
**Method:** ⚠️ **NAMED DEPARTURE CONTINUES (BRAINSTORM-87 / SPEC-86 precedent): no investigation agents — every probe INLINE by the controller.** The centerpiece is a **COMPILING, TEST-GREEN PROTOTYPE of the `buildRequest` fix built in a DETACHED worktree** (`wt-87-proto` at `d7fd87fd`; diff + probe test captured to session scratch; worktree DELETED at close). Probes: a 17-case `net/url` differential (`go run`, scratch); tip + prototype binaries built with `-o` into scratch; `curl --http2-prior-knowledge --path-as-is` batteries; a **raw HEADERS-frame h2c client** (~120 lines, x/net `Framer`+`hpack`, scratch module) for the `:path` values curl cannot send; an x/net-Transport literal-path probe through the repo's own `test/helpers.H2CRoundTrip`; ONE reference container (`b87-main`, `contrib-v1.37.2`, torn down BY NAME, 0 remaining verified); the `0004-h2-routing` differential fixture EXECUTED against the prototype tree; `go test ./...` on the prototype tree. Ports **47410-47411** (subject) and **47420-47421** (reference) — bound transiently by probe processes only; nothing survives the stage. **ALL SEVEN BRAINSTORM-§4 QUESTIONS ARE DISPOSED BY EXECUTION BELOW.** `ROADMAP.md` is **BYTE-UNTOUCHED** at this stage (row 87 stays `in-progress` at `:149`, `want` stays 119).

---

## 1. Q1 — THE FIX PRIMITIVE, DECIDED BY PROTOTYPE: `url.ParseRequestURI` PLUS AN EXPLICIT FRAGMENT REJECT

### 1.1 D-87-PRIM — `url.ParseRequestURI`, not manual `&url.URL{}` construction

The prototype swaps the single defect site (`buildRequest`'s `url.Parse(path)` call, `internal/filter/hcm/h2/stream.go` — cite by symbol; the line drifts) to `url.ParseRequestURI(path)`. The 17-case `net/url` differential (measured at this tip, `go run`, both primitives side by side) shows `ParseRequestURI` is **byte-identical to `url.Parse` on every form the codec accepts today EXCEPT the defective and the invalid ones**:

| `:path` | `url.Parse` → Path (today) | `url.ParseRequestURI` → Path (fix) | class |
|---|---|---|---|
| `/`, `/foo`, `/a//b`, `///x` | same | same | unchanged |
| `/foo?a=b` | `/foo` + RawQuery `a=b` | identical | unchanged (Q2) |
| `/foo%2Fbar`, `/%2e%2e/x` | Path + RawPath preserved | **identical, RawPath identical** | unchanged (escape semantics) |
| `*` | `*` | `*` | unchanged (asterisk-form survives) |
| `http://h/p` (absolute-form) | Host peeled, Path `/p` | identical | unchanged (Host overwritten by `:authority` anyway) |
| `//foo` | **`""`** (Host peeled) | **`//foo`** | REPAIRED |
| `//` | **`""`** | **`//`** | REPAIRED |
| `//foo/bar` | **`/bar`** (Host peeled) | **`//foo/bar`** | REPAIRED |
| `//foo?x=1` | **`""`** + RawQuery `x=1` | **`//foo`** + RawQuery `x=1` | REPAIRED (query still splits) |
| `foo` (rootless) | `foo` (silent 404 downstream) | **ERR `invalid URI for request`** | NEW REJECT (§1.3) |
| `?a=b` (bare query) | `""` (silent 404) | **ERR** | NEW REJECT (§1.3) |
| `/foo#frag` | Path `/foo`, Frag silently STRIPPED | Path `/foo#frag` (frag kept IN Path) | see D-87-FRAG |
| `/foo?a=b#frag` | RawQuery `a=b`, Frag stripped | RawQuery `a=b#frag` | see D-87-FRAG |

The manual `&url.URL{Path, RawQuery}` construction is **REJECTED**: it would have to re-implement the query split AND the percent-decoding/`RawPath` semantics the table shows `ParseRequestURI` already provides byte-identically (`/foo%2Fbar` → Path `/foo/bar` + RawPath `/foo%2Fbar` on BOTH primitives) — hand-rolling that is new parser surface for zero behavioral gain.

### 1.2 D-87-FRAG — an explicit `#` reject, decided by REFERENCE behavior (measured, not taste)

Neither primitive matches the reference on a fragment-bearing `:path`: `url.Parse` silently STRIPS the fragment and routes (`GET /foo#frag` → 200 `routed-ok` at the tip, measured via the raw client), `ParseRequestURI` keeps it in Path/RawQuery — but **the reference REJECTS it: `:path=/foo#frag` → 400** (measured, §4). A request-target carries no fragment (RFC 9113 §8.3.1 origin-form). The prototype therefore adds a 3-line guard before the parse: `strings.IndexByte(path, '#') >= 0` → `&Error{Code: ErrProtocolError, Msg: "fragment in :path"}`. Measured post-guard: `/foo#frag` and `/foo?a=b#frag` → `RST_STREAM PROTOCOL_ERROR` on the prototype.

### 1.3 The enumerated `:path` forms, with the two NEW rejects named as behavior changes toward reference

- **origin-form** (the common case): unchanged for every non-`//` path; REPAIRED for leading `//`.
- **asterisk-form `*`** (OPTIONS): `{Path:"*"}` on both primitives; end-to-end `OPTIONS *` → 404 route-miss on tip, prototype, AND reference (all three measured) — parity preserved, unchanged.
- **absolute-form**: parses identically on both primitives; `u.Host` is overwritten with `:authority` immediately after, as today.
- **CONNECT / authority-form: OUT OF SCOPE, CONFIRMED** — `buildRequest` rejects missing/empty `:path` upstream of the parse (`empty :path` / `missing :path`), so no authority-form value reaches the parse site.
- **rootless `foo` and bare `?a=b`**: today a **silent 404** (the tip routes Path `foo`/`""` to nothing); post-fix a `PROTOCOL_ERROR` stream reject. ⚠️ **The reference tears down the CONNECTION on both** (measured: zero response frames, EOF) — so the fix moves envoy-go from "silently absorb an invalid request-target" to "reject it", the reference's DIRECTION, with a stream-level wire shape (D-87-REJECT-SHAPE, §4.2).
- **fragment**: reject (D-87-FRAG above; reference direction, stream-level shape).

### 1.4 The prototype, measured

`wt-87-proto` diff: **`11 insertions / 1 deletion` = NET +10 production `.go`, ONE file** (`stream.go`: the swapped call, the 3-line fragment guard, 7 comment lines naming RFC 9113 §8.3.1 and the network-path-reference hazard). gofmt clean. A 63-line in-worktree unit probe (`stream_proto87_test.go`, never lands, captured to scratch) pins per-arm: all nine accept forms' `URL.Path`/`URL.RawQuery`, `RequestURI` staying the LITERAL `:path` bytes, and the four reject forms — GREEN. Test suites against the prototype: `./internal/filter/hcm/...` green; **full `go test ./...` green** (one UNRELATED known-class flake, §7.3); the `0004-h2-routing` differential fixture **PASS** (§2).

---

## 2. Q2 — QUERY PRESERVATION AND ZERO-REGRESSION, MEASURED

- `/foo?a=b` → `{Path:"/foo", RawQuery:"a=b"}` **identical on both primitives** (§1.1 table); `//foo?x=1` splits the query correctly post-fix. `req.RequestURI` keeps the literal `:path` bytes (pinned by the prototype probe).
- Escape semantics unchanged: `/foo%2Fbar` and `/%2e%2e/x` produce identical Path AND RawPath on both primitives.
- **End-to-end zero-regression evidence at the prototype tip:** `internal/filter/hcm` + `internal/filter/hcm/h2` full package suites green; **`TestDifferential/0004-h2-routing` PASS against the prototype tree** (2.46 s, the named subtest line printed — a REAL run, not a `-run` no-match); full `go test ./...` green modulo §7.3. The 121-fixture byte-stability claim for the whole suite is an IMPL gate (PLAN §gates), not re-asserted here.

---

## 3. Q3 — BOTH FAILURE ARMS PROVEN, RED AT TIP AND GREEN AT PROTOTYPE, WITH THE ROUTED PATH ASSERTED

Config: two routes — `prefix: "/bar"` → `direct_response 200 "WRONG-bar"`, `prefix: "/"` → `direct_response 200 "routed-ok"` — so the mis-route is visible in the BODY (the routed path discriminates), never just the status. `curl --http2-prior-knowledge --path-as-is` over h2c, tip binary vs prototype binary (same config, same port):

| request | TIP (RED, measured) | PROTOTYPE (GREEN, measured) | reference (§4) |
|---|---|---|---|
| `GET /` (control) | `routed-ok` 200 | `routed-ok` 200 | `routed-ok` 200 |
| `GET /a//b` (control) | `routed-ok` 200 | `routed-ok` 200 | `routed-ok` 200 |
| `GET //foo` | **404, empty** | `routed-ok` 200 | `routed-ok` 200 |
| `GET //` | **404, empty** | `routed-ok` 200 | `routed-ok` 200 |
| `GET //foo/bar` | **`WRONG-bar` 200 — the SILENT MIS-ROUTE, caught by BODY** | `routed-ok` 200 | `routed-ok` 200 |
| `GET //foo?x=1` | **404** | `routed-ok` 200 | `routed-ok` 200 |
| `GET /bar` (regression control) | `WRONG-bar` 200 | `WRONG-bar` 200 | `WRONG-bar` 200 |

The `/bar` control proves the fix does not disturb direct prefix matching; the `//foo/bar` row proves the mis-route arm is only catchable by a routed-path assertion (status is 200 on BOTH sides of the fix).

---

## 4. Q4 — THE REFERENCE CONTAINER, EXECUTED (`b87-main`, `contrib-v1.37.2`, torn down BY NAME, 0 remaining)

### 4.1 The §3 carried expectation is CONFIRMED — no reshape needed

The reference H2 downstream with DEFAULT path handling (no `merge_slashes`, no `normalize_path`, no `path_with_escaped_slashes_action`) **preserves a leading `//` and routes it as literal path bytes**: `//foo` → 200 `routed-ok`, `//` → 200, `//foo/bar` → 200 **`routed-ok` — the FULL path, NOT the `/bar` route** — `//foo?x=1` → 200, `/a//b` → 200 (full curl battery in §3's table, rightmost column). BRAINSTORM §3's carried expectation is now a measurement; the contract keeps its shape.

### 4.2 New reference facts from the raw-HEADERS battery, and D-87-REJECT-SHAPE

- `:path=/foo#frag` → **400** (the reference REJECTS a fragment — decides D-87-FRAG).
- `:path=foo` (rootless) and `:path=?a=b` → **connection torn down, zero response frames** (EOF observed by the raw client; the same client completes `//foo` → 200 on the same server, so the teardown is the server's answer, not a client artifact).
- `OPTIONS *` → **404** (route-miss; parity with subject on both sides of the fix).

**D-87-REJECT-SHAPE:** envoy-go rejects invalid request-targets at STREAM level (`RST_STREAM PROTOCOL_ERROR` — `buildRequest`'s existing idiom for every pseudo-header violation); the reference uses a 400 local reply (fragment) or a connection teardown (rootless/bare-query). **Direction parity, wire-shape divergence — NAMED, unit-level only**: a cross-side differential arm on the reject shapes would compare three different wire behaviors and is deliberately NOT chartered (consistent with the codec's existing pseudo-header reject family, none of which is differentially asserted either).

---

## 5. Q5 — THE DIFFERENTIAL PROOF SHAPE: THE FEARED COST EVAPORATED, MEASURED

### 5.1 x/net `http2.Transport` delivers a literal leading-`//` `:path` — NO raw-frame writer, NO curl subprocess needed

Measured through the repo's own `test/helpers.H2CRoundTrip` (which builds the request via `http.NewRequestWithContext` and round-trips on x/net) against the TIP binary: `//foo` → **404**, `//foo/bar` → **`WRONG-bar`**, `/a//b` → 200, `//foo?x=1` → 404 — **identical to the `curl --path-as-is` battery**, i.e. the literal bytes reached the wire (x/net's `validPseudoPath` accepts any `/`-prefixed path and does not normalize). The BRAINSTORM's load-bearing cost fear (a raw HEADERS-frame writer or a curl subprocess in the driver) is REFUTED: **the existing `H2RoundTrip`/`H2CRoundTrip` helpers drive the `//`-arms as-is.** The raw framer exists as a measured ~120-line fallback (scratch, works against tip/prototype/reference) for any FUTURE wire-level reject arm — recorded, not landed. (The reject arms themselves cannot ride x/net — `http.NewRequestWithContext` drops fragments and cannot express rootless targets — which is one more reason they are unit-level, §4.2.)

### 5.2 D-87-DIFF — EXTEND `0004-h2-routing`; no new fixture

The differential arm lands as an EXTENSION of `0004-h2-routing` (the H2 routing fixture; ADR-0057):

- **Both YAMLs** (`envoy.yaml` + `envoy-go.yaml`, and the driver's rendered bootstraps) gain ONE route above the catch-all: `match: { prefix: "//edge" }` → `direct_response 200 "edge-ok"`.
- **`drive()` gains a fourth loop, APPENDED AFTER the existing 27-request schedule** (so the existing transcript prefix stays byte-identical): `GET //edge` → expect 200, body `edge-ok` (concatenated into the Drive byte stream) and `GET //edge/health` → expect 200, body `edge-ok`. **Pre-fix the subject fails BOTH ways the defect fails**: `//edge` → 404 (drive errors — the RED), `//edge/health` → the `/health` route's `OK\n` (the mis-route, visible as a BYTE divergence in the transcript AND the wrong body) — the routed-path assertion carried into the differential layer.
- **Counts untouched:** direct_response arms, no backend involvement — `[3,3,3]` distribution logic unchanged; fixtures stay **121** (+0), BackendKind **+0**, ports **+0**, no new dir ⇒ the three-registration-gates hazard does not arise; `expectations.yaml`/README/doc.go prose updated.

Rejected alternative: a NEW fixture (`0120-h2-origin-form-path`) — a full driver + YAML pair + README + registration for two direct-response arms the existing H2 routing fixture can carry; strictly more surface for the same proof.

---

## 6. Q6 — UNIT-ANCHOR PLACEMENT

`internal/filter/hcm/h2/stream_test.go` already carries a `buildRequest` rejection table (the Table-B idiom, exact-message assertions). The IMPL adds a **path-forms table** beside it: nine accept rows (`/`, `/foo`, `/foo?a=b`, `//foo`, `//`, `//foo/bar`, `//foo?x=1`, `/a//b`, `*`) pinning `URL.Path`/`URL.RawQuery`/`RequestURI`-stays-literal, and four reject rows (`foo`, `?a=b`, `/foo#frag`, `/foo?a=b#frag`) pinning `*Error`/`ErrProtocolError` + message. The prototype's 63-line probe is the seed shape; the landed version with the package's exact-message idiom is estimated ~100-170 lines. **The two failure arms' unit REDs at the tip are already proven** (the probe's `//foo` → `Path=""` and `//foo/bar` → `Path="/bar"` expectations fail against `url.Parse` — that is the §1.1 table). Fuzz: **+0 fuzzers**; an `f.Add` seed in the existing `FuzzFrameStream` (an encoded HEADERS stream carrying `:path=//foo`) is OPTIONAL and is a seed, not a fuzzer (`reference_fuzzer_count_docs_drift`).

---

## 7. Q7 — COUNTS, EACH RE-DERIVED MECHANICALLY AT THIS TIP

### 7.1 Post-change axes (measured on the prototype)

- **go.mod/go.sum +0** — `git diff -- go.mod go.sum` EMPTY in the prototype worktree (`net/url` and `strings` already imported by `stream.go`).
- **stat surface +0** — the diff contains no `NewCounter(`/`NewGauge(` call site (DELTA form re-asserted at the IMPL per the standing discipline; no absolute carried — 1205-vs-1207 stays contested).
- **fixtures 121 +0** (D-87-DIFF extends `0004`; tail `0119-grpc-unary-trailers`, `0120` stays free) · **fuzzers 55 / 48 files +0** · **BackendKind tail 38 +0** (`H2GoawayResponder = 38` at `fixture.go:606-614`).

### 7.2 Doc-file counts at this tip (pre-edit bases for this stage's appends)

`DECISIONS.md` **18098** lines · **307** `^## ADR-` headings · tail **ADR-0308 COMPLETE** · strict `^> \*\*STATUS: PROPOSED` guard **0** (this SPEC re-arms it to **1** with ADR-0309 §Context) · `^---$` **216** (the append adds NONE) · `BEHAVIOR_CONTRACT.md` **5957** (BYTE-UNTOUCHED this stage; the `## HTTP/2` section at `:2019` gains the origin-form parity rule AT THE IMPL, riding ADR-0309 per ADR-0052) · `STATE.md` **64** · `STATE_HISTORY.md` **484** · `BOOTSTRAP_PROMPT.md` **522** · phase dirs **128** · `ROADMAP.md` **237 lines / 119 data rows**, row 87 `in-progress` at `:149`.

### 7.3 ⚠️ A FLAKE FINDING — the 86-IMPL's uncaptured `internal/boot` failure has RECURRED, and THIS TIME THE IDENTITY IS CAPTURED

The prototype's full `go test ./...` run 1 failed exactly one test: **`TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server:_validation_context_fetch_times_out,_boot_fails`** (`internal/boot/boot_sds_e2e_test.go:551`) — under full-suite load the silent-SDS-server fetch failed with the gRPC `DeadlineExceeded` recv error instead of the initial-fetch-timeout message the test asserts. **This is the recurrence-with-identity the phase-86 close asked for** (that run's failure identity went uncaptured — a recorded lapse). Cleared: scoped `-count=5` green + full `internal/boot` package green. Class: the SDS dial-budget timing family (two-package register) — now with a named member. The h2 edit cannot reach this path, and the scoped re-runs discriminate: load-correlated, not change-correlated.

---

## 8. COST, ENUMERATED BY PROTOTYPE (the BRAINSTORM floor `~2-10 prod / ~120-400 test` disposed)

- **Production: MEASURED EXACTLY +10 net `.go`, one file** (§1.4) — the BRAINSTORM floor's top edge, landed. IMPL band **~10-16** (allowance: message/comment polish only; NO second production site exists — `grep` confirms one non-test `url.Parse` in `internal/filter/hcm/`).
- **Test `.go`: band ~130-225 net**, enumerated per file: the unit path-forms table ~100-170 (`stream_test.go`) + the `0004` driver arm ~25-45 (`driver.go`) + optional fuzz seed 0-10. NON-`.go` riders: 2 route-table YAML edits (~4 lines each), `expectations.yaml` + README + `doc.go` prose (~20-30 lines).
- **Docs:** ADR-0309 completed in place at the IMPL (§Decision + §Consequences, guard 1 -> 0, no `---`), the contract `## HTTP/2` parity rule, ROADMAP row-87 flip (`numstat 1 1`, `want` stays 119).
- ⚠️ **Floor discipline:** ten consecutive `reference_measured_prototype_is_a_lower_bound` firings say the un-enumerated items are where overruns live. This enumeration ALREADY includes the differential plumbing (Q5 — measured collapsed, not guessed), the unit-table premium over the probe, and the prose riders. Known remaining un-enumerated classes: per-arm exact-message pins if the IMPL adopts Table-B's full idiom, and any reviewer-mandated arms. The band is quoted as a FLOOR per doctrine.

## 9. D-87-SEQ — ONE IMPL LEG, ONE ATOMIC COMMIT (for the PLAN to decompose)

TDD inside the one commit: (1) the unit path-forms table RED at the tip (the two arms fail exactly as §1.1 predicts — failure lines read); (2) the `0004` `//edge` arms RED at the tip (`//edge` → 404 drive error; `//edge/health` → `OK\n` byte divergence); (3) the swap + fragment guard land (the §1.4 shape); (4) all arms GREEN both layers; (5) docs (ADR-0309 completion, contract rule, ROADMAP flip); (6) gates LAST — full differential 121 with `INNER_EXIT`, anchored panic gate, h2spec **95-case scope from the IMPL's own run**, `go test ./...`, `-race` on the h2 package, gofmt + golangci-lint (misspell US), every count with its NC.

## 10. SENTINEL

See PROGRESS.md §Sentinel(SPEC) for this stage's recorded output (ONE side — a SPEC does not touch `ROADMAP.md`; verified byte-untouched by empty diff against master).

## 11. NEXT

**PLAN** — decompose §9 into ordered tasks with RED anchors re-proven at the PLAN tip, draft the contract-edit text and the ADR-0309 completion plan, freeze the error-string constraint set (`bad :path` reused for the two `ParseRequestURI` rejects; `fragment in :path` NEW; zero landed strings change), and re-derive every count.
