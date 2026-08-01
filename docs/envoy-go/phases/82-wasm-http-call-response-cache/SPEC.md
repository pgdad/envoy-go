# SPEC 82 — wasm-http-call-response-cache

**Stage:** SPEC (lifecycle-state `1` -> `2`). **ROW 82 STAYS `in-progress`**; `ROADMAP.md` is **BYTE-UNTOUCHED** and the sentinel `want` **STAYS 114**. Base master **`61f4f5a3`**, taken from `git rev-parse master` at session start and **not** from any SHA quoted in the router. Branch `phase-82-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

Five investigation agents ran on disjoint remits — four in their own DETACHED worktrees off `61f4f5a3` with private scratch and private port bands inside `42100-42499`, one read-only over the primary tree. Every load-bearing claim below was **re-derived by the controller** rather than adopted from a brief (`feedback_brief_citations_not_evidence`). Where an agent's own claim did not survive controller re-derivation, that is recorded too (§2.11).

---

## 0. SENTINEL — RE-RUN MECHANICALLY AT THIS TIP. IT DOES **NOT** FIRE; `stop` WAS **NOT** CREATED

Input measured **230 lines / 114 data rows** before anything was written, so an empty result cannot read as a zero result (`reference_empty_output_is_not_a_zero_result`).

- **(1)** at `want=114` ⇒ **`NOT DONE: row 82`**, with the denominator printed (`examined 114 data rows`). ⚠️ **The doctored-copy NC was run anyway** even though the check is live, because the moment row 82 flips `done` the check goes silent and silence is otherwise indistinguishable from a broken check: row 62 flipped `in-progress` on a scratch copy ⇒ **`NOT DONE: row 62`** alongside row 82, with the NC **VERIFIED TO HAVE LANDED** by printing the doctored status field (`NC LANDED? [ in-progress ]`) *before* its result was trusted. Denominator NC at `want=113` ⇒ `GATE FAIL: examined 114 data rows, expected 113`.
- **(2)** **FIVE — `:192 :202 :212 :218 :226`. UNCHANGED. THIS ROW NARROWS NOTHING, STATED AND NOT FORECAST.** The **THIRTY-THIRD** consecutive phase at which it did not go down. ⚠️ A one-arm strip is **not** an NC here: `sed 's/deferred candidates://'` moves the union **5 -> 4, not 5 -> 0**, re-confirmed at this tip.
- **(3)** **`NEVER OPENED: gRPC` — ALONE.** WASM stays retired by row 82's landed cell. NC: an invented slug (`Quantum-Widget`) fires; the registered slug `Observability` correctly prints nothing.
- ⇒ checks (1), (2) and (3) all print, so **the sentinel does not fire**. `ls stop` ⇒ `No such file or directory`, and it was **not** created.

### 0.1 The leak check is **VERIFIED BY DIFF**, not "inapplicable"

This SPEC writes **no ROADMAP cell**, so the by-mention silencing hazard cannot fire. That is asserted mechanically rather than by eye:

```sh
git diff --stat master -- docs/envoy-go/ROADMAP.md   # EMPTY at this stage
```

⚠️ **The two classes remain opposite and must not be conflated.** `<slug>-family row` is check (3)'s **PASS** condition (row 82's landed cell is a genuine **use**); `deferred candidates:` / `remaining deferred (not-yet-chartered) candidates:` are check (2)'s **FAIL** conditions. This SPEC writes neither into `ROADMAP.md`.

### 0.2 Row well-formedness — the DISJUNCTION re-executed over all 114 rows

Denominator printed: **114 data rows, lines 31-144.** Escape-aware = `\|` masked before splitting.

- **ARM-A** (escape-aware `NF!=8`) catches **line 119 (row 57, NF=9)** and **line 131 (row 69, NF=10)** — and nothing else.
- **ARM-B** (escape-aware trailing piece) catches **line 140 (row 78)** only.
- **Naive `NF==8` flags SEVENTEEN rows — 15 FALSE POSITIVES + 2 true — and MISSES line 140** (compensating defects cancel). Wrong in **both** directions: **15 FP + 1 FN**.
- **Row 82 (line 144): escape-aware NF = 8, empty trailing piece, 7 pipes / 0 escaped ⇒ flagged by NEITHER arm.**

---

## 1. SCOPE — AND THE CENTRAL FINDING: THE ROW AS CHARTERED FIXES A **MASKED** DEFECT

**The chartered defect is REAL.** Re-derived directly, not read from the BRAINSTORM:

- `internal/filter/http/wasm/abi_callbacks.go` — `headerMapForType` (**symbol anchor**; the method is `func (a *abiCallbacks) headerMapForType`, so a `grep 'func headerMapForType'` returns nothing and is a broken instrument, not a zero result) returns `(nil, false)` from its `default:` for map types **4** (`HttpCallResponseHeaders`) and **5** (`HttpCallResponseTrailers`).
- `GetHeaderMap` converts that to `WasmResultNotFound` for the guest.
- `GetBuffer`'s `default:` swallows `HttpCallResponseBody` (4) and returns `(nil, nil)`.
- `internal/wasm/http_call.go` hands the guest a true `(numHeaders, bodySize, numTrailers)` tuple and the `// TODO Task 15` stash at `:411` never landed.

**But the defect is UNREACHABLE at this tip, and the BRAINSTORM's entire verification story rests on the belief that it is reachable.** Measured by two agents with *different* discriminating designs, and confirmed by the controller against the source:

1. `internal/filter/http/wasm/decode_headers.go:258-260` handles `abi.ProxyActionPause` by **logging and returning `envoyhttp.Continue`** — architectural primitive 6 (stream control) has been deferred since 25.2. `encode_headers.go:115-117` is symmetric.
2. The stream therefore completes before the HTTP-call response lands. `internal/wasm/http_call.go:342-348` then takes `if sc == nil || sc.closed.Load() { rv.stats.HttpCallResponseAfterCloseInc(); return }` — **`proxy_on_http_call_response` is never invoked into the guest.** Measured over 20 back-to-back requests in one subject process: `http_call_dispatched 20` · **`http_call_response 0`** · **`http_call_response_after_close 20`**.
3. A sentinel control settles it beyond inference: a guest crate identical to the vendored one except that `call_status` is initialised to **777** makes the subject emit **`x-httpcall-status: 777`**. **The subject's `0` is the guest's INITIAL FIELD VALUE**, never a cache miss.

⇒ **Landing the stash and the three dispatch arms changes NO observable behaviour on ANY path.** That is squarely `BOOTSTRAP_PROMPT.md` **§6.3 `:304`**'s anti-pattern — *"introducing incomplete stubs that differential tests can't exercise"*.

### 1.1 ⚠️ AND THE MASK IS HIDING A **TRAP**, NOT A WRONG VALUE — THE THREE AGENTS' "CONTRADICTION" IS THE ROW'S MOST IMPORTANT FACT

Two agents measured the subject emitting `x-httpcall-status: 0`; a third measured the subject **trapping**, with the header **absent**. Both are correct, about **different paths**, and the controller resolved it rather than picking a side:

| path | what happens | who measured it |
|---|---|---|
| **production, today** | callback **never invoked** (`http_call_response 0` / `after_close 20`) ⇒ the guest never calls `get_map` ⇒ no trap ⇒ it emits its **initial** `call_status` (`0`, or `777` under the sentinel control) | A1, A4 — **correct about production** |
| **callback forced** (direct `CallProxyOnHttpCallResponse`) | `get_map` -> envoy-go returns `NotFound` -> **the Rust SDK has NO `NotFound` arm** -> `panic!("unexpected status: 1")` -> guest TRAP -> BUG-3 `panic_already_borrowed` cascade in the next callback | A3 — **correct about the forced path** |

**Verified by the controller against the authoritative source on this machine** — `~/.cargo/registry/.../proxy-wasm-0.2.4/src/hostcalls.rs:158-174`, the exact SDK the vendored blob was compiled against: `get_map` has **only** a `Status::Ok` arm plus `status => panic!("unexpected status: {}", status as u32)`.

⚠️ **CONSEQUENCE, AND IT INVERTS THE ROW'S RISK PROFILE: LANDING S1 ALONE MAKES THE PRODUCT STRICTLY WORSE.** Honoring Pause without a correct, correctly-addressed stash converts a benign wrong value into a **guest trap plus a poison cascade**. S1 must never land ahead of the cache. See §12.

### 1.2 ⚠️ **S0 — THE HEADER-MAP ENUM IS SWAPPED, AND WITHOUT FIXING IT THE ROW SHIPS VACUOUS**

`internal/wasm/abi/types.go` assigns `HttpCallResponseHeaders = 4`, `HttpCallResponseTrailers = 5`, `GrpcReceiveInitialMetadata = 6`, `GrpcReceiveTrailingMetadata = 7`. **The canonical assignment is the reverse pairwise.** Controller-verified against `proxy-wasm-0.2.4/src/types.rs:90-99` on this machine:

```
GrpcReceiveInitialMetadata = 4      HttpCallResponseHeaders = 6
GrpcReceiveTrailingMetadata = 5     HttpCallResponseTrailers = 7
```

**The vendored guest calls `proxy_get_header_map_pairs(6)`.** ⇒ wiring `headerMapForType` cases for **4** and **5**, exactly as BRAINSTORM §4 prescribes, would serve map types the guest never asks for: the guest would still hit `default:` -> `NotFound` -> **trap**. **The row would ship vacuous — or worse.** A 3-arm cross-product against the real blob confirms it: no stash ⇒ trap; stash at **4** ⇒ **trap, identically**; stash at **6** ⇒ the guest sets `x-httpcall-status`.

**S0 is therefore a PRECONDITION, not an extra.** Blast radius is small and fully enumerated: `internal/wasm/abi/types.go` (the declarations) · `internal/wasm/abi/types_test.go:144-147` (**a hand-written golden pinning the WRONG values** — it passes today and fails on exactly 4 subtests once corrected; `reference_handwritten_golden_shares_author_mistake`) · one comment mention in `internal/wasm/http_call.go` · 3 mentions in `internal/filter/http/wasm/abi_callbacks_test.go`. **`headerMapForType` itself references none of the four**, so the fix is contained.

⚠️ **S0 and S5 must land ATOMICALLY** (`reference_lifted_reject_hidden_enforcement`): correcting the enum without wiring the new cases leaves the guest trapping on 6; wiring cases at the old values does nothing.

### 1.3 What the row must therefore contain

**In scope.**

| # | item | why it is load-bearing |
|---|---|---|
| **S0** | **Correct the `WasmHeaderMapType` 4/5/6/7 assignment** + its hand-written golden | without it, every other item addresses map types the guest never requests (§1.2) |
| S1 | **Honor `ProxyActionPause`** on the decode side (`StopIteration` + paused-state bookkeeping), symmetrically on encode | without it the callback never fires and every other item is dead code — **but it must not land first** (§1.1) |
| S2 | Stash `(header, body, trailer)` on the originating `*StreamContext` at `http_call.go:411` | the chartered defect |
| S3 | **Synthesize `:status`** into the stashed header map (§6) | Go's `resp.Header` carries **no** `:status`; without it the guest's parse loop is empty and `call_status` stays `0` even once S1 lands |
| S4 | Settle the **key-count vs value-count** semantic (§5) | `numHeaders` is a key count; `GetHeaderMap` emits value pairs |
| S5 | `headerMapForType` cases **4** and **5**; `GetBuffer` case **4** | the three dispatch arms |
| S6 | Clear the stash at stream teardown (§7) | leak + cross-call bleed |
| S7 | **Rebuild the `l_httpcall_success` guest blob** to call `resume_http_request()` | `resume_http_request` has **0** occurrences corpus-wide (NC: `get_http_call_response_headers` ⇒ 1) |
| S8 | Flip fixture `0036` scenario (l) to cross-side; **tighten the (l) StatsAsserter arm** (§8.1); correct the backwards comment at `driver.go:504-509` | the row's only cross-side evidence |

`resume_http_request`'s host half is **already live** — `abi_callbacks.go:766` `ContinueStream` calls `ContinueDecoding()`/`ContinueEncoding()`. **Only the pause half is missing**, which is materially cheaper than "primitive 6 wholesale".

**Helpfully already built:** `internal/wasm/abi/body_bridge.go:104` `activatedBufferType` already admits `WasmBufferTypeHttpCallResponseBody`, so the ABI shim needs no change. **CONFIRMED.**

**Out of scope, deferred by name.** The 9 `WasmResultUnimplemented` stubs at `internal/wasm/registration.go:877-882` · `proxy_on_queue_ready` and the `proxy_on_grpc_*` callbacks · the wasm **network** filter · the 6 deferred cpp-host conformance families · **any trailers ASSERTION** (§4) · the dead **request/response** trailer map types 1/3 (`internal/filter/http/wasm/trailers.go`) · the 0036 cross-side arm restoration for scenarios (a)-(j) (§8.2).

---

## 2. WHAT THIS SPEC REFUTES BY EXECUTION

**Load-bearing:**

**2.1 ⚠️ THE REFERENCE DOES NOT EMIT `200`. IT EMITS NOTHING.** BRAINSTORM §1 and the ROADMAP row-82 cell both assert *"the subject emits 0 where the reference emits 200"*. Measured against `envoyproxy/envoy:contrib-v1.37.2`: the downstream request **hangs to the client timeout** (15.014 s / 20.007 s / 25.002 s across three arms; `curl` exit 28, zero bytes). The reference **honors** `Action::Pause`; the guest never resumes. Two independent designs agree — a per-arm cross-product (k/l/m/n: only the two `Action::Pause` guests hang, k and m return in 2.78 ms / 48.4 ms) and a standalone container probe.

**2.2 ⚠️ THE SUBJECT'S `0` IS NOT THE CACHE DEFECT — IT IS THE GUEST'S INITIAL VALUE, AND THE CALLBACK NEVER RUNS.** §1 above. This is the pick's whole premise and it does not hold. The BRAINSTORM's stated chain (*"`NotFound` -> empty vec -> `call_status` stays 0"*) presupposes `on_http_call_response` executes; measured `0/20` delivered, `20/20` after-close.

**2.3 ⚠️ "THE FAILING-FIRST ANCHOR IS ALREADY LANDED" IS FALSE AS A CROSS-SIDE CLAIM.** The blob performs the read, but nothing observes it and nothing *can*: the reference returns no response at all, so there is no reference-side value to compare against. A cross-side arm is unreachable **in either direction** without a rebuilt guest.

**2.4 ⚠️ "THE ROW NEEDS NO NEW GUEST BLOB" IS FALSE.** `resume_http_request` appears **0** times across all 14 guest crates (NC: `get_http_call_response_headers` ⇒ 1 in the same corpus). A control crate that is byte-identical to the vendored source **plus** `resume_http_request()` made the reference return **`x-httpcall-status: 200` in 5 ms** — which also proves the reference recognises `cluster_b` (`cluster.cluster_b.upstream_rq_200: 1`), directly refuting the landed comment's stated *reason*.

**2.5 ⚠️ `resp.Header` CARRIES NO `:status`, SO A LITERAL STASH IS INERT.** Verified by the controller with a firing NC: `StatusCode=200`, `len(resp.Header)=3` (`Content-Type`, `Date`, `Content-Length`), `resp.Header[":status"]` **absent**, while `Get("Content-Type")` reads back correctly. The guest's `if k == ":status"` loop would find nothing and `call_status` would stay `0` **even after S1 and S2 land**. `numHeaders` would report **3** where reference V8 reports **4**.

**2.6 ⚠️ `numHeaders` IS A **KEY** COUNT WHILE THE CONSUMER EMITS **VALUE** PAIRS — A NEW FORK THE BRAINSTORM NEVER NAMED (§5).** `http_call.go:406` uses `len(resp.Header)`. Every sibling seam uses `numHeaderValues` (`decode_headers.go:211`, `encode_headers.go:87`, `trailers.go:116,179`), whose own doc says it matches *"the proxy-wasm pair-emission shape (one wire pair per (key, value) tuple)"*. `GetHeaderMap` **is** value-expanding. The structural cause: `numHeaderValues` is **unexported** and lives in `internal/filter/http/wasm`, which **imports** `internal/wasm` (`go list -deps` ⇒ 1) while the reverse is **0** — so `http_call.go` cannot call it without inverting the dependency.

**2.7 ⚠️ `headerMapForType` HANDLES TYPES **0 AND 2**, NOT "0 AND 1".** Its own doc comment says *"ACTIVE at 25.1 (types 0 + 2)"*: `HttpRequestHeaders=0` and `HttpResponseHeaders=2`. Type **1** (`HttpRequestTrailers`) is **not** handled — it falls to `default:` alongside 4/5. The conclusion (4 and 5 return `(nil,false)`) survives; the stated mechanism does not. ⚠️ **This is the SECOND wrong-on-arrival defect in row 82's own ROADMAP cell**, alongside the already-recorded `abi_callbacks.go:186-192` range (the function is **178-190**). Both are **INVARIANT-BLOCKED** by §Schema `:18` (*"Only `status` and `sub-phases` columns are updated in place"*) ⇒ **recorded, not fixed**, on the rows-57/69/78 precedent.

**2.8 ⚠️ THE EXISTING (l) STATS ARM IS BLIND TO THIS ROW'S ENTIRE CHANGE — A COMPENSATING-DEFECT CANCELLATION.** `driver.go:303-333` does **not** assert `http_call_response >= 1` as `README.md:71` and `expectations.yaml` both claim. It asserts a **disjunction**:
```go
respTotal := respDelivered + respPostClose   // want respTotal >= 1
```
Today `delivered=0, post_close=20` ⇒ sum 20 ⇒ PASS. After S1 lands, `delivered=20, post_close=0` ⇒ sum 20 ⇒ **PASS**. **The gate metric is INVARIANT under the row's central behavioural change** (`reference_compensating_defects_cancel_in_the_gate_metric`). Row 82 **must split the disjunction** or it ships its headline fix with no guard at all. ⚠️ **README.md:71 and `expectations.yaml:99-106` are both inaccurate about what the arm asserts** — exact as quotations, wrong as descriptions.

**2.9 ⚠️ FIXTURE 0036'S CROSS-SIDE COMPARISON IS 100% VACUOUS TODAY.** `driveProxy` emits **only** compile-time constants — 14 `emitConstantSkipToken` calls plus 4 subject-only literals, **686 bytes**, identical on both sides by construction. Five helpers (`emitScenario`, `classifyBody`, `reflectedHeaders`, `reflectedKeys`, `trim`) are `//nolint:unused`-parked at `:602 :612 :696 :719 :729`. **`CompareBytes` on this fixture carries ZERO cross-side information**, so a gate-(a)/(b) "0036 GREEN" is not parity evidence. README lines 6-9/47 and the driver package doc claim 10 live cross-side arms; **none are live.**

**2.10 ⚠️ `reference_code_comment_not_evidence` CUTS BOTH WAYS HERE, AND THE BRAINSTORM ANCHORED ON THE WRONG COMMENT.** The same file carries a **correct** comment at `driver.go:313-327` that already names the exact root cause — *"the subject's stream-control pause is NOT yet honored (parent §1 architectural primitive 6 deferred ... ) so the stream closes BEFORE the response lands"* — and a **wrong** one at `:504-509`. The BRAINSTORM walked past the accurate one. ⚠️ **A third instance points the other way:** `abi_callbacks.go:594-596` states *"ContinueStream + CloseStream STUB to no-op WasmResultUnimplemented"*, which is **false** — `:766` is live. That one *reduces* the row's cost.

**Structural and numeric:**

**2.11 ⚠️ AN AGENT'S OWN REFUTATION DID NOT SURVIVE RE-DERIVATION.** A5 reported *"a live violation of ADR-0288's invariant — two live `next-free ADR` values"*. Re-derived on the anchored form the discipline actually specifies (`^- \*\*<field>:`), **all five fields return exactly 1** (`next-free ADR`, `next-skill`, `lifecycle-state`, `active-phase`, `last-commit`). **The invariant HOLDS.** The stale `ADR-0299` sits inside the differently-named `- **DECISIONS.md tail:**` field, which only a *loose* grep surfaces as a second answer. `reference_a_drift_correction_is_itself_a_claim` — the narrower true statement is that **§Project counts is six phases stale** (it self-declares *"at this phase-76 IMPL close"* while the line above presents it as *"(Live…)"*), which is the already-standing reason to **anchor on §Current and never "fix" §Project**.

**2.12 ⚠️ FOUR BANKED COUNTS IN THE ROUTER ARE WRONG.** All re-derived by the controller:
- **phase dirs = 123, not 122**; carrying `REVIEW.md` **37**; **NOT** carrying **86, not 85**. The router counted *before* its own stage created `82-wasm-http-call-response-cache/`.
- **`STATE_HISTORY.md` tolerant archive count = 170, not 169** (naive 161; difference **9** parenthetical entries, not 8). The predecessor corrected *its* predecessor and then failed to re-derive after its own append — the ninth is the entry that stage itself wrote.
- **`ROADMAP.md:<line>` cites = 118, not 119** (two independent arms agree).
- **`DECISIONS.md` has a GAP: ADR-0209 does not exist** — `^## ADR-0209` ⇒ 0 with **both** neighbours ⇒ 1 (firing NCs on each side). 302 headings over the id range 0001-0303. **No prior document records this**; anyone deriving *"next-free = headings + 1"* gets **0303** and collides.

**2.13 ⚠️ `:212` IS NOT A SENTENCE ANCHOR.** It is a **45,959-char** line carrying **SIX** occurrences of the deferred-candidate phrase. Only the one at **offset 14459** ends in the colon-terminated form the sentinel matches; the rest read `candidates were:` / `candidates now that … :`. Citing *"the `:212` sentence"* without the offset is ambiguous across six different rosters. ⚠️ **And `:226` does not carry the long phrase at all** — it uses the SHORT `deferred candidates:` form, which is precisely the Operational-tooling blindness check (2) was broadened at phase 77 to fix.

**2.14 ⚠️ `docs/superpowers/plans/` CONTAINS NO FILE NAMED `BOOTSTRAP_PROMPT.md`.** The 1024-line "second copy" is `docs/superpowers/plans/2026-04-21-envoy-go-bootstrap-prompt.md`, and its offsets are **not** a constant shift — §6.1 at `:482` (Δ+197) but §7.5 at `:585` (**Δ+228**). A bare filename grep never surfaces it; a heading grep surfaces it at the wrong offsets. **Open the repo-root file (522 lines).**

**2.15 ⚠️ THE `RunEncodeTrailers`/`RunDecodeTrailers` PREMISE IS A NON-SEQUITUR (§4).** "ZERO non-test callers" is **CONFIRMED** (exactly 2 call sites, both `_test.go`). But *"2 non-test **mentions**, both in `chain.go`"* is **REFUTED**: there are **14** across **5** files. More importantly the seam is **causally irrelevant** — the http-call response never enters the FilterChain; it goes `RootVM.dispatchHttpCallGoroutine` -> `wasmHTTPDispatcher.Dispatch` -> `httpclient.Client.ClusterDispatch` -> stdlib `http.Client.Do`, and `resp.Trailer` is populated by `net/http` independently.

**2.16 ⚠️ `+0 NEW PUBLIC SURFACE` DOES NOT SURVIVE.** BRAINSTORM §4.2 and the ROADMAP cell both claim it. A measured prototype adds **three exported `*RootVM` methods** (`HTTPCallResponseHeaders`/`Trailers`/`Body`). Any design letting `internal/filter/http/wasm` read a stash owned by `internal/wasm` either adds exported surface **or** extends the `ABICallbacks` interface (**20 -> 21** methods, breaking every implementer including `rootABICallbacks` and the test fakes). **The PLAN must pick one deliberately and record it**, not discover it.

**2.17 ⚠️ ALL THREE cpp-host CITES IN `http_call.go`'s HEADER COMMENT ARE WRONG FOR v1.37.2.** Measured against `envoyproxy/envoy@v1.37.2 source/extensions/common/wasm/context.cc` (2005 lines): `:1547` (claimed cluster-lookup-miss) is actually `decoder_callbacks_->resetStream()` — real site **`:881-884`**; `:1693` (claimed defensive `find()`) is `buffering_request_body_ = false` — real sites **`:1834-1836`** / **`:1856-1858`**; `:1900-1905` (claimed destructor iterating `http_request_`) is `onGrpcCloseWrapper` reentrancy — real site **`:1328-1334`**. A landed comment block citing an external pinned dependency, wrong on every line.

**2.18 ⚠️ CANCEL-AT-DESTRUCTION IS A DEPARTURE, DOCUMENTED AS PARITY.** `http_call.go:29-34` and `stream_context.go:17-23` present it as byte-faithful to the cpp-host destructor. Because upstream populates `http_request_` only on the **root** context, `Context::~Context()` cancels at **plugin/root** teardown; envoy-go cancels at **stream** end (`cancelOutstandingHttpCalls`). Upstream, an http call **outlives** the stream that dispatched it; in envoy-go it is killed. **A real behavioral divergence recorded as parity.**

**2.19 ⚠️ envoy-go PASSES THE WRONG `plugin_context_id` TO `proxy_on_http_call_response`.** `http_call.go:427` passes `entry.streamCtxID`; upstream invokes the callback on the **root** context and passes the root id. Invisible to Rust guests (the SDK dispatcher ignores arg 0 and routes by token) but a wire divergence a C++/Go SDK guest could observe. **Named, not fixed.**

**2.20 ⚠️ `handleHttpCallResponse` SWALLOWS GUEST TRAPS, DEFEATING THE BUG-3 POISON GUARD.** `http_call.go:425` is `_ = rv.runCallWithPanicWrapper(...)` — no `noteTrapOnError`, no failure counter. `runCallWithPanicWrapper` recovers **Go** panics only; it returns the wazero trap verbatim and the caller drops it. ⇒ a trap inside `proxy_on_http_call_response` leaves `sc.trapped == false`, so `StreamContext.Close`'s poison guard (`stream_context.go:358`) does **not** engage and Close **re-enters the poisoned instance**. ⚠️ **This is directly coupled to §1.1: the moment S1 makes the callback reachable, this swallowed-trap path becomes live.** The PLAN must treat it as in-scope.

**2.21 ⚠️ THERE IS NO CAP ON THE HTTP-CALL RESPONSE BODY.** `http_call.go:302` is an unbounded `io.ReadAll(resp.Body)`. `bodyBufferCapBytes` (default 16 MiB) is consumed **only** at `body.go:144` and `:265`. Upstream bounds it (`ExceedResponseBufferLimit`). **Pre-existing at this tip — state its absence; row 82 neither adds nor fixes it.**

**2.22 ⚠️ THE `OnDestroy`-ONCE LORE APPLIES HERE AND IS ALREADY SATISFIED — and its banked cite is stale by +1.** `wasm.go:324-325` installs the **same** `f` in both `Decoder:` and `Encoder:`; `internal/filter/http/chain.go` is `destroyOnce`-guarded with `if f.Decoder != nil { … } else if f.Encoder != nil { … }`. **Live cites are `:666`/`:670`/`:672`; the banked `:665`/`:669`/`:671` are stale by +1.** With a callback-scoped stash (§7) no once-guard is needed at all.

**2.23 Cite verification.** `driver.go:504-509` and `README.md:71` resolve **verbatim**. `http_call.go:403-409` / `:411` correct. `body_bridge.go:104` correct. `ADR-0065 §Consequences (b) :2379` and `ADR-0132 §Decision (v) :6291` both **resolve exactly**. `BOOTSTRAP_PROMPT.md` §6.1 `:285`, `:289`, `:290`, `:291` blank, `:292`, §6.2 `:294`, **§6.3 `:304`** (the circulating `:306` is the §6.3 **body**; settled), §7.5 `:357`, gates `:360-365`, close `:367` — **all exact**, independently derived twice. `### gRPC family` `:194`, `### WASM host family` `:220`. ⚠️ **`abi_callbacks.go:178-191` is off by one** — the function spans **178-190**.

---

## 3. D-82-DIRECTION — DISPOSED BY EXECUTION: **BOTH PRIOR STATEMENTS ARE WRONG**

| | claim | measured |
|---|---|---|
| BRAINSTORM §1 | subject `0` / reference `200` | **half wrong** — subject `0` is right *by coincidence*, for the wrong reason (§2.2); reference emits **nothing** |
| `driver.go:504-509` | subject *"may"* `200` / reference *"may"* `0` | **wholly wrong**, on both halves, and its stated *reason* is refuted by `cluster.cluster_b.upstream_rq_200: 1` |

**Disposition.** The scenario is currently **incomparable**, not divergent. It becomes comparable only after **S1 + S7**. The corrected statement row 82 must land in `driver.go:504-509`:

> subject returns HTTP 200 with `x-httpcall-status: 0` because the guest's `Action::Pause` is logged-and-ignored (primitive 6) so `proxy_on_http_call_response` is never delivered in-stream; the reference honors Pause and, with a guest that never resumes, returns **no response at all**.

**Controls that ran** (all four discriminating, `reference_probe_must_discriminate`): a sibling scenario proving the probe reads guest-set response headers on both sides; a resume-enabled rebuild proving the reference is healthy and returns `200`; a byte-identical rebuild on the local toolchain proving rustc 1.96-vs-1.94 is **not** the confounder; and the `777` sentinel proving the subject's `0` is the guest's initial value. ⚠️ **The probe INPUT was the vendored blob and the fixture's own config throughout** (`reference_probe_input_is_a_claim`); the one hand-built crate was a *control*, explicitly labelled.

---

## 4. D-82-TRAILERS — DISPOSED: **STASH THE FIELD, SCOPE THE ASSERTION OUT**

`resp.Trailer` **is** populated on this path — measured, with a firing negative control:

| arm | proto | PRE-drain | POST-drain |
|---|---|---|---|
| announced 2, sent 2 | HTTP/1.1 chunked | len=2 (nil values) | **len=2, values present** |
| **negative control**, no trailers | HTTP/1.1 | len=0, nil | **len=0** |
| unannounced (`TrailerPrefix`) | chunked | len=0 | **len=1** |
| announced 1, **sent 0** | chunked | len=1 | **len=1, ZERO pairs** |

The code's drain order is **correct** (`io.ReadAll` + `Close` at `:302-303`, `len(resp.Trailer)` at `:408`, `resp` never reassigned between). ⚠️ **This SPEC's own working hypothesis — *"reading `len(resp.Trailer)` before the drain yields 0 regardless"* — is REFUTED for the announced case**: Go pre-seeds announced trailer keys with nil values at header-parse time. It holds only for `TrailerPrefix`.

**Disposition: keep S5's map-type-5 case (one switch arm, plumbing is live). Scope any trailers ASSERTION OUT.** Two independent reasons:
1. **No backend in the repo emits response trailers.** A repo-wide sweep for `TrailerPrefix` / `Header().Set("Trailer"` returns 8 lines, of which 4 were the probe itself (**matcher capability proven**) and 4 are a `bandwidthlimit` **config field**, not an emission. `0036`'s `cluster_b` is `echobackend`, which sets `Content-Type` and writes JSON.
2. **The anchor guest never reads them** — there is no `get_http_call_response_trailers()` anywhere in the corpus.

Asserting `num_trailers == 0` would be a textbook vacuous-green arm (`reference_vacuous_break_modes`).

⚠️ **A hazard the PLAN must carry:** the announced-but-never-sent case yields `num_trailers=1` with **zero** pairs. §5's disposition must cover it.

---

## 5. D-82-COUNT — A **NEW** FORK. DISPOSED: **VALUE-COUNT, COMPUTED IN `internal/wasm`**

The tuple `http_call.go` promises and the pairs `GetHeaderMap` delivers use **different units** (§2.6). Three options:

| | option | verdict |
|---|---|---|
| (a) | make the new type-4/5 emitter **key**-based | **REJECTED** — inconsistent with `GetHeaderMap`'s landed value-expansion for types 0/2, and with the proxy-wasm pair contract `numHeaderValues` documents |
| (b) | export `numHeaderValues` and call it from `internal/wasm` | **REJECTED** — inverts the package dependency (`internal/filter/http/wasm` imports `internal/wasm`, not the reverse); a cycle |
| (c) | **compute the value count inline in `internal/wasm` at stash time** | **ADOPTED** |

**Disposition (c).** `http_call.go` computes `numHeaders`/`numTrailers` as **total value counts** over the stashed maps, matching the pair list the guest will actually receive. ⚠️ **The `:status` synthesis of §6 happens BEFORE the count is taken**, so the count includes it. This also disposes §4's announced-but-unsent hazard: the count is taken over the **stashed** map, so a nil-valued key contributes **0**, and count and pair list agree by construction.

⚠️ **The duplication is deliberate and must be recorded, not silently introduced** — a second small value-count helper in `internal/wasm` is the price of the one-way dependency. The PLAN must add a comment at both sites naming the other.

---

## 6. D-82-STATUS — A **SECOND** NEW FORK. DISPOSED: **SYNTHESIZE `:status` AT STASH TIME**

Go's `http.Response` carries the status in `StatusCode`, **not** in `Header` (§2.5, verified with a firing NC). Reference V8 reports **4** headers where a literal stash reports **3**, and the difference is exactly the pseudo-header the anchor guest parses.

**Disposition.** At stash time, build the stashed header map as a **copy** of `resp.Header` with `":status"` inserted as `strconv.Itoa(resp.StatusCode)`. Three constraints the PLAN must honour:

1. **Copy, never mutate `resp.Header` in place** — the response object is not owned by the stash.
2. **Lower-case `:status`, inserted by direct map assignment**, not `Header.Set`. `http.CanonicalHeaderKey` leaves leading-`:` names untouched, but `GetHeaderMapValue`'s landed comment (`abi_callbacks.go`) already documents that pseudo-headers are looked up **as-stored** and that guests pass them lower-case. Direct assignment matches the incumbent convention.
3. **`:status` participates in the §5 count** and in `GetHeaderMap`'s sorted emission. Sorting places `:status` first (`:` = 0x3A < any alphanumeric), which matches the reference's pseudo-header-first ordering — **stated as a consequence, not asserted as parity**; §10 does not pin cross-side ordering.

---

## 7. D-82-LIFETIME — DISPOSED: **CALLBACK-SCOPED, NOT STREAM-SCOPED. THE QUESTION AS POSED DISSOLVES.**

The BRAINSTORM asks *"where the stash is cleared, and whether a second `dispatch_http_call` on the same stream must overwrite or queue"*, and says it *"bears on whether the row is 7 or 9 tasks"*. **Neither branch is correct.** Derived from the landed code, not from the TODO:

### 7.1 Multiple calls CAN be in flight on one stream — so a single-slot STREAM-lifetime stash would be WRONG

`rv.httpCalls` is `map[uint32]*pendingHttpCall` keyed by **token** (`root_vm.go:224`), and `pendingHttpCall` carries `streamCtxID` (`http_call.go`). `cancelOutstandingHttpCalls(streamCtxID)` **iterates the map and collects a SLICE** of cancels matching one stream — plural by construction. ⇒ the one-stash-per-`StreamContext` shape the `// TODO Task 15` comment prescribes is **not** safe as written.

### 7.2 But the ABI makes the stash CALLBACK-scoped, which dissolves the fork

`proxy_get_header_map_pairs` / `proxy_get_buffer_bytes` take **no token argument**. The guest can only mean *"the response currently being delivered"*, and the proxy-wasm contract is that the HttpCallResponse* buffers are valid **only for the duration of `proxy_on_http_call_response`**. ⇒ there is nothing to overwrite and nothing to queue.

### 7.3 The disposition, and why it needs no new locking or teardown wiring

**Set the stash under `dispatchMu` immediately before invoking `proxy_on_http_call_response`; clear it in a `defer` immediately after that call returns.**

- **Race-free with no new lock.** The dispatch already runs under `rv.dispatchMu.Lock()` with `defer Unlock()` (`http_call.go:360-361`), so **exactly one guest callback executes at a time**. `rv.currentCtxID.Store(entry.streamCtxID)` at `:376` is the landed precedent for exactly this "current dispatch state, set just before the guest call" pattern — the stash follows it.
- **No teardown wiring.** The stash never outlives the callback, so the two `delete(rv.streamCtxs, id)` sites (`root_vm.go:850`, `:866`), `sc.closed.Store(true)` (`stream_context.go:374`, `root_vm.go:995`) and `cancelOutstandingHttpCalls` all need **no change**. **S6 shrinks from "clear at teardown" to a `defer`.**
- **No leak and no bounding problem.** One response is live at a time and it is released synchronously. The body is already fully materialised in `bodyBytes` before this point, so the stash adds **no** new retention.
- **`OnDestroy`-once is not implicated.** The known hazard (one shared value in both fields; encoder-side unreachable) governs filter destruction, not this path. **Verified as inapplicable rather than assumed.**

⚠️ **This REFUTES the landed `// TODO Task 15` comment's own prescription** (*"stash … on the originating `*StreamContext`"*). The correct home is dispatch-scoped state on the `*RootVM`, reachable from `internal/filter/http/wasm` because that package **imports** `internal/wasm` (§2.6) — the plumbing direction works, and the PLAN must pin the exact accessor (§2.16).

### 7.4 Reference fidelity — the per-ROOT home is what upstream does

Upstream sets `http_call_response_` one statement before `ContextBase::onHttpCallResponse` and clears it via `addAfterVmCallAction([...]{ http_call_response_ = nullptr; … })` — i.e. **exactly set-before / clear-after**. It reads it off `rootContext()`, and `proxy_http_call` is itself retargeted to the root (`contextOrEffectiveContext()->root_context()`). `context.h` documents the field verbatim as **"Only available during onHttpCallResponse."** ⇒ per-`*RootVM`, callback-scoped is the **faithful** mapping, not merely the convenient one. Upstream needs no lock because responses are never delivered re-entrantly; **`dispatchMu` is envoy-go's analogue.**

### 7.5 ⚠️ USE `atomic.Pointer`, NOT A PLAIN FIELD — CAUGHT BY `-race` ON THE FIRST PROBE RUN

A prototype using a plain struct field was flagged by `-race` immediately: the accessor read races the `defer` write. **Production reads are all inside the `dispatchMu` frame, so the plain field is arguably safe today — but any exported accessor (§2.16) puts an out-of-frame read one line away**, and `-race` is a second full gate pass in this project. `atomic.Pointer[httpCallResponse]` costs the same LOC and is race-free by construction. **Adopted.**

**Measured cost for the stash portion alone** (prototype built, `go build ./...` OK, both packages' tests run, `-race` e2e green through production `abiCallbacks` dispatch with the real vendored blob, then reverted):

```
40  0  internal/filter/http/wasm/abi_callbacks.go
 4  4  internal/wasm/abi/types.go
52  8  internal/wasm/http_call.go
 9  0  internal/wasm/root_vm.go
              TOTAL +105 -12   NET 93
```

**93 net production `.go`, a LOWER BOUND** — zero tests, zero fixture work, zero docs, and **excluding S1 entirely**.

### 7.6 Task-count call on this axis: **7, not 9**

Callback scoping **deletes** four tasks the BRAINSTORM's framing implies — teardown-time clear, cross-stream-leak test, cross-call-leak test, and a memory bound. **Nothing about lifetime justifies 9.** The row may still land at 8-9, but for the independent reasons in §1.2 (S0) and §2.20 (the swallowed trap), not for D-82-LIFETIME.

⚠️ **A hazard the PLAN must carry:** the `defer` clear must run even when the guest **traps**. `sc.trapped` is set by any `CallProxyOnX` whose guest call errors, and a poisoned instance must not be re-entered; a `defer`-based clear is correct under both the normal and the trap path, but the PLAN must place it so `runCallWithPanicWrapper`'s recovery cannot skip it.

---

## 8. D-82-BREAK — DISPOSED: **THREE ARMS, AND THE BASELINE CONSTRAINT IS DECISIVE**

`0036`'s README §"Deliberate-break liveness verification" mandates breaks for **StatsAsserter** arms specifically. ⚠️ **Its literal scope does NOT cover cross-side arms** — the obligation for a cross-side break is **created by row 82**, not inherited. Recorded as a scope extension rather than claimed as compliance.

**The dominant vacuity mode, named.** `driveProxy` is **ONE** function serving both `DriveReference*` and `DriveSubject*`. **Any symmetric edit keeps the streams equal and PASSES** — that is exactly why the constant-token trick works. A fixture-layer break **must be side-asymmetric** or it is vacuous by construction (`reference_break_arm_injection_site_is_a_claim`).

| arm | layer | injection | expected |
|---|---|---|---|
| **BREAK-A** | fixture, **side-asymmetric** | force a wrong `x-httpcall-status` on the subject side only | FAIL — proves the cross-side comparison is live |
| **BREAK-B** | **production** | revert the row's own `case HttpCallResponseHeaders` in `headerMapForType` | FAIL — **the only arm that proves the assertion tracks the DEFECT** |
| **BREAK-C** | fixture, **symmetric** | same wrong value on **both** sides | **GREEN** — negative control proving BREAK-A failed on asymmetry, not on the value |
| **BREAK-D** | production | revert **S1** (pause) alone | FAIL — proves the stats arm split of §8.1 is live |

⚠️ **BREAK-A alone is vacuous with respect to row 82's claim** — it proves the plumbing is live, not that anything observes the response cache. **BREAK-B is mandatory.**

⚠️ **THE DECISIVE CONSTRAINT ON THE FAILING BASELINE.** A failing baseline exists **today** (the flip fails), but it fails **for the reference-hang reason**, not the cache reason. Until S7 lands, a green is unreachable and a red is **misattributed** — cycling a break against that baseline would satisfy the ritual while proving nothing (`reference_liveness_break_needs_failing_baseline`, `reference_deliberate_break_wrong_assertion`). ⇒ **The PLAN must order S7 BEFORE any break-arm verification**, and each break must confirm **WHICH** assertion fired, not merely that one did.

**Ordered-leg hazard: discharged by measurement.** `CompareBytes` reports only the first divergence and arms (a)-(k) precede (l); a measured flip run put the first divergence **inside the (l) line** (offset 632), so (l) is provably not masked. `runner_test.go:1288` uses `t.Errorf`, not `Fatalf`, so downstream assertions still run (`reference_fatalf_makes_assertions_unreachable`).

### 8.1 The (l) StatsAsserter arm must be SPLIT

Per §2.8 the disjunction is invariant under the row's change. Row 82 replaces it with a **positive, direction-bearing** pair:

- `http_call_response >= 1` (delivered), **and**
- `http_call_response_after_close == 0`

⚠️ **A positive arm alone cannot catch an over-firing counter** (`reference_positive_arm_cannot_catch_overfiring`); the `== 0` leg is the stacked control. Both legs use `t.Errorf` so each reports independently. README `:71` and `expectations.yaml:99-106` must be corrected to describe what the code asserts — **in both directions**, since they are wrong today too.

### 8.2 What the flip does NOT restore

The five `//nolint:unused` helpers exist for *"cross-side arm restoration in a follow-up phase"*. Row 82 revives `emitScenario` and `classifyBody` **for scenario (l) only**. Scenarios (a)-(j) stay constant-token. ⚠️ **`.golangci.yml` enables no `nolintlint`**, so stale directives will not fail lint — the leftover directives are hygiene, not a gate.

---

## 9. BLAST RADIUS

**Production `.go`:** `internal/wasm/abi/types.go` (**S0**, the enum) · `internal/wasm/http_call.go` (stash, `:status`, value-count, trap propagation) · `internal/wasm/root_vm.go` (the `atomic.Pointer` slot + accessors) · `internal/filter/http/wasm/abi_callbacks.go` (`headerMapForType` **6/7**, `GetBuffer` **4**) · `internal/filter/http/wasm/decode_headers.go` + `encode_headers.go` (**S1**, the pause half).

⚠️ **`*StreamContext` needs NO new field and NO teardown change** (§7.3) — the BRAINSTORM's prescription is refuted.

**Test `.go`:** `internal/wasm/abi/types_test.go` (**the wrong-on-arrival golden, 4 subtests**) · unit tests in `internal/filter/http/wasm` and `internal/wasm` · `0036/inputs/driver.go`.

**Non-Go artefacts:** `scripts/l_httpcall_success/src/lib.rs` + rebuilt `bytecode/l_httpcall_success.wasm` (**vendored, committed**; Rust **1.96.0** + `wasm32-wasip1` verified installed, recipe at `0036/scripts/README.md:26-51`, `proxy-wasm-rust-sdk =0.2.4`; **CI carries no Rust toolchain**) · `README.md` · `expectations.yaml`.

**Registration gates: the flip touches NONE.** All three are already satisfied — the fixture dir exists, `RegisterFixture` is at `driver.go:86-90`, and the blank import is at `runner_test.go:63`. `BackendKind HTTPWasmAdvanced=26` and its switch case at `runner_test.go:853` are in place. **No `t.Skipf` silent-green risk from the flip** (`reference_differential_fixture_three_registration_gates`).

**Stat surface: +0.** `http_call_dispatched`, `http_call_response` and `http_call_response_after_close` are already registered (`internal/filter/http/wasm/stats.go:146,156,200,471-477`). The row registers nothing new. ⚠️ **Assert the delta by CALL-SITE ENUMERATION at the IMPL**, not by the `TestNoNewStat*` guards, which are proven blind and scoped to another package.

---

## 10. DIFFERENTIAL AND FIXTURE POSTURE — **ZERO NEW FIXTURES**

Row 82 extends `0036` and adds none. Fixture count **stays 120**; next-free stays `0119`. No new port, no new `BackendKind` (tail **38** over **39** declarations), no new package (**73**), no new go.mod module.

⚠️ **`0036` burns ~30 s of dead reference client-timeout per run** — arms (l) **and** (n) both use `Action::Pause` and both hang the reference for 15 s, which is most of the fixture's ~37 s wall time. **Undocumented anywhere in the corpus before this SPEC.** S1 + S7 remove it for (l); **(n) is out of scope and keeps its 15 s.**

⚠️ **The known driver-owned receiver port race fired once during this stage's measurement runs** (`admin start 0.0.0.0:11002: bind: address already in use` -> `retrying with fresh ports`, self-healed). Classified as the standing 42-fixture race, **not** caused by this work. **Budget ~3 differential launches per green pass.**

---

## 11. CONTRACT AND ADR EDITS (owed at the IMPL, specified here)

**`DECISIONS.md`** — **ADR-0304 §Context lands at THIS SPEC** (ADR-0044-**as-used**; ⚠️ **ADR-0044 does not itself contain that discipline** — see ADR-0297 §Context ¶8, which measured the misattribution). §Decision + §Consequences land at the IMPL. Block form: em-dash heading, one `> **STATUS: PROPOSED …** ` blockquote, **no `---` separator** (the last `^---$` is `:17020`, trailing ADR-0288; **fifteen** blocks since carry none), retained italic footer `*(§Decision + §Consequences land at the phase-82 IMPL.)*`.

⚠️ **The append shifts ZERO existing line cites — VERIFIED, not assumed:** appending to a scratch copy and running `head -17796 … | cmp -` against the original returns **byte-identical**, with `:2379 :6291 :17726` spot-checked unmoved.

⚠️ **Retained-footer count is NINE at this tip, and the figure a successor would inherit is wrong.** ADR-0303's own STATUS asserts *"EIGHT blocks carry the footer"*; measured at that same commit the grep already returned **9** — the enumeration silently excludes ADR-0303's own footer. **State 9, or say "8 predecessors" explicitly.** ADR-0304 makes it **10**.

⚠️ **RE-ARM the recurrence guard, anchored on the STATUS WORD.** Verified in **both** directions by the controller and independently: `^> \*\*STATUS: PROPOSED` is **SILENT** on the clean tree and **fires exactly once** on a doctored copy (with the doctored line printed before the result was trusted). ⚠️ **The LOOSE form (`^> \*\*STATUS: .*PROPOSED`) is a FALSE-POSITIVE GATE — it fires on FOUR `COMPLETE` blocks (ADR-0299, 0300, 0301, 0302)** whose STATUS lines merely narrate the historical defect. **Do not use it.**

⚠️ **CARRY NO WHOLE-FILE GREP COUNT IN THE ADR** — that species self-falsified in ADR-0296 ¶3 and again in ADR-0302 ¶11. **Enumerate by site.**

**`BEHAVIOR_CONTRACT.md`** — a subsection on http-call response readback owed at the **IMPL**, not here. ⚠️ **Relocate rather than insert mid-file** where possible: the phase-81 precedent (nine cites, all append-only; relocated to `### Phase 16 forward-pointer notes`, shifting nothing) is the habit to copy.

**`ROADMAP.md`** — **BYTE-UNTOUCHED at this stage.** Row 82 flips `done` at the IMPL six-gate.

---

## 12. COST AND SPLIT — ⚠️ **THE ROW HAS OUTGROWN ITS CHARTERED BAND**

The BRAINSTORM's **~110-350 net `.go`, budget ~250, 7-9 tasks, NEITHER §6.1 trigger fires, NO SPLIT** was costed against a scope that **excluded S1, S3, S4 and S7** — three of which are now known to be load-bearing and one of which (S1) is an architectural primitive.

**This SPEC states its own basis and inherits nothing** (`reference_measured_prototype_is_a_lower_bound`):

| item | net `.go` LOWER BOUND | basis |
|---|---|---|
| **S0** enum fix + golden | **~10** | **MEASURED** (4 ins / 4 del in `types.go`, plus 4 golden lines) |
| S1 pause half (both sides + bookkeeping + tests) | 300-600 | ⚠️ **ESTIMATED, not prototyped** — the only un-measured item, and the dominant one |
| **S2+S3+S5+S6** cache, `:status`, 3 arms, `defer` clear | **93 MEASURED** | prototype built, `-race` green e2e, reverted (§7.5) |
| S4 value-count | 30-50 | code read |
| §2.20 trap propagation (becomes live with S1) | 40-80 | code read |
| S8 fixture flip, mechanical | **+4 MEASURED** | built, vetted, run (`git diff --numstat` = 6 ins / 2 del) |
| S8 full (comment, package doc, nolint, stats split, README, yaml) | 30-60 | enumerated per site |
| unit tests across both packages | 200-400 | lineage ratio |
| **TOTAL LOWER BOUND** | **~710-1300** | **three of eight items MEASURED, not estimated** |

⚠️ **THAT IS A LOWER BOUND, NOT AN ESTIMATE.** Phase 81 measured **nine** prototypes and still landed **3.07x** over them (725 -> 2229); phase 80 landed 1636 against ~640. **At even 1.5x the midpoint (~1040) this row lands ~1560 — ACROSS `BOOTSTRAP_PROMPT.md:290`'s ~1500-LoC trigger.** At the lineage's realized 3x it lands ~3100.

**Task count: 14-20.** Under `:289`'s ~25 trigger.

### ⚠️ THE SPEC'S CONCLUSION ON SPLITTING — AND THE ORDERING CONSTRAINT IS NOT NEGOTIABLE

**§6.1 `:285` places the split decision at the PLAN** (*"triggered at step 2 of the lifecycle (when `PLAN.md` is being written)"*), so this SPEC does **not** split. It hands the PLAN an evidence-backed axis **and a mandatory order**:

> **Leg B (FIRST) — S0 + S2-S6: the enum fix and the http-call response cache.** Safe to land alone: the callback is still unreachable, so the code is inert, the guest keeps emitting its initial value, and nothing regresses. Unit-testable in full.
> **Leg A (SECOND) — S1 + S7 + S8: stream control, the rebuilt blob, the cross-side arm.** Activates the callback against an already-correct cache.

⚠️ **THE REVERSE ORDER IS A REGRESSION, AND THIS SPEC'S OWN FIRST DRAFT HAD IT BACKWARDS.** Landing S1 first makes `proxy_on_http_call_response` reachable while `headerMapForType` still returns `NotFound` for the type the guest requests. The Rust SDK has **no `NotFound` arm** (§1.1, controller-verified in the vendored SDK source), so the guest **traps**, and §2.20's swallowed-trap path then leaves `sc.trapped == false` so `Close` re-enters a poisoned instance. **A benign wrong header becomes a trap plus a poison cascade.** ⇒ **Leg A must never precede Leg B**, and if the row is *not* split, S0/S2-S6 must land in the same commit as S1 or earlier within the row.

⚠️ **The PLAN MUST re-measure S1 by prototype before deciding.** S1 is the only unprototyped item and it dominates the estimate; a split argued from this SPEC's arithmetic alone would not be evidence. **Leg B's production half is already measured at 93 net (§7.5), a lower bound.**

⚠️ **The split convention this project actually practices** — legs live in the parent's `sub-phases` PROSE, no sub-phase ROW since 32.2 — **conflicts with `ROADMAP.md` §Schema `:18`'s written *"Sub-phases get their own rows"***. **Recorded, not relitigated.** `ADR-0045` carries the per-bucket task+LoC cost form to imitate.

**The alternative this SPEC REJECTS, and why.** *Narrow row 82 to the cache alone (S2-S6), unit-test anchored, deferring S1/S7/S8.* Smaller and defensible on cost — but it lands code **no differential fixture can exercise**, which is §6.3 `:304`'s named anti-pattern; and it destroys the pick's own severity argument, since with the callback never firing there is no wrong answer visible to any guest today. **Recorded as considered and rejected, not omitted.**

---

## 13. WHAT THIS ROW NAMES BUT DOES NOT FIX

- **`internal/filter/http/wasm/trailers.go` is internally contradictory in three tenses** — the file header says types 1+3 "are ACTIVATED at Task 16"; `:65` says `headerMapForType` **routes** type 1 (present tense, **false** — §2.7); `:90-99` says the activation lands at Task 16 and then at Task 18. The capture itself is a no-op (`_ = trailers // captured … by the framework` — nothing captures it). **The type-1/3 trailer map surface is DEAD.**
- **Two stale non-test comments** at `internal/filter/hcm/connection.go:565` and `h2dispatch.go:503` claim `RunDecodeTrailers` does not exist. It does, at `internal/filter/http/chain.go:455`. **CONFIRMED stale.**
- **`abi_callbacks.go:594-596`** claims `ContinueStream` is an Unimplemented stub. It is live at `:766`.
- **All three cpp-host cites in `http_call.go`'s header comment are wrong for v1.37.2** (§2.17).
- **Cancel-at-destruction is a DEPARTURE documented as parity** (§2.18) — an http call outlives its stream upstream and is killed here.
- **The wrong `plugin_context_id` on `proxy_on_http_call_response`** (§2.19) — invisible to Rust guests, a wire divergence for others.
- **No cap on the http-call response body** (§2.21) — unbounded `io.ReadAll`; upstream bounds it. Pre-existing.
- **`docs/.../25.1-*/SPEC.md:481-483` carries the same wrong enum values** in a landed, append-only record. **Record, do not fix.**
- **`BEHAVIOR_CONTRACT.md:1967`** asserts a blocker its own successor phase killed (`ssl.connection_error` *"still blocked on enumerating its membership"*).
- **ROADMAP row 82's two wrong-on-arrival cites** (§2.7) — INVARIANT-BLOCKED.
- **`ROADMAP.md` malformation: three rows lose summary content in GFM render** (lines 119, 131, 140), INVARIANT-BLOCKED ⇒ defensible as a GUARD with the DISJUNCTION gate, not a fix. **There is no doc-invariant test anywhere in this repo**; the reusable precedent is `test/differential/harness_test.go:123-137`'s `runtime.Caller(0)` -> repoRoot idiom.
- **`STATE.md` §Project counts is six phases stale** (§2.11). **Do NOT "fix" it.**
- **The driver-owned receiver port race (42 fixtures)** · **`stats-name-empty-segment-guards` (24 sites)** · the RBAC policy-name PROJECTION divergence · the lua `statNameRegexLiteral` (~40 net, 1 task) · the matcher-tree `Action`-name boot walk · **three** stale incumbent guard cites (`lua/compiled_config.go:272` ×2, `redisproxy/config.go:51`).

---

## 14. HAZARDS CARRIED INTO THE PLAN

1. ⚠️ **ORDERING IS A CORRECTNESS CONSTRAINT, NOT A PREFERENCE.** S1 must never land before S0+S2-S6, or a benign wrong header becomes a guest trap plus a poison cascade (§1.1, §12).
2. ⚠️ **S0 is a precondition.** Wiring cases at the old enum values does nothing; the guest asks for **6** (§1.2).
3. **S1 is unprototyped and dominates the cost.** Prototype it first (§12).
4. ⚠️ **§2.20's swallowed trap becomes LIVE the moment S1 lands** — treat it as in-scope, not as a named-but-deferred defect.
5. **The break baseline is misattributed until S7 lands.** Order S7 before break verification (§8).
6. **The (l) stats arm is invariant under the row's change** until split (§8.1).
7. **`0036`'s cross-side stream is 100% constants** — a green there is not parity evidence (§2.9).
8. **The announced-but-unsent trailer case** yields a count with no pairs (§4).
9. **`:status` ordering** is a consequence, not a pinned parity claim (§6).
10. **A second value-count helper is deliberate duplication** and must be commented at both sites (§5).
11. **`+0 new PUBLIC surface` is refuted** — pick the accessor shape deliberately (§2.16).
12. **`0036` costs ~37 s and the port race fired once during this stage.** Budget ~3 launches (§10).

---

## 15. HYGIENE

Fresh worktree off the CURRENT master tip **`61f4f5a3`** (`git rev-parse master`), branch `phase-82-spec`. Five agents on disjoint remits — four in **DETACHED** worktrees off the same base with private scratch and private port bands inside `42100-42499`, clear of **both** reserved bands (`20000-31007` subject blocks, `11000-14999` backends) and the static fixture ports; one read-only over the primary tree. Zero commits, zero branches and zero pushes by any agent; **all reported `git status --porcelain` = 0 lines** (the primary tree shows only the pre-existing `?? .claude/`), controller-re-confirmed. Every throwaway `.go` probe was deleted and the tree proven clean. Docker containers were created **and removed BY NAME** (`p82s-a1-1..3`); ⚠️ **no image or ancestor filter was ever used**, and no container this session did not create was touched (`reference_parallel_agents_shared_machine_namespaces`).

⚠️ **`Shell cwd was reset to /home/esa/git/envoy-go` FIRED LIVE at this stage**, as it has for thirty consecutive sessions. Every git command used `git -C <abs-worktree-path>`; branch confirmed `phase-82-spec`, never `master`, before any commit.

---

## 16. NEXT

**The phase-82 PLAN.** Its first job is to **prototype S1 (the stream-control pause half) and re-measure**, because S1 is the only unprototyped item of eight, it dominates the cost, and the §6.1 `:290` ~1500-LoC trigger is live at ~1.5x this SPEC's own lower bound. It must then decide the split on that measurement rather than on this SPEC's arithmetic — and if it splits, **Leg B (S0 + the cache) MUST precede Leg A (S1 + blob + fixture)**, because the reverse order converts a benign wrong header into a guest trap plus a poison cascade (§1.1, §12). ⚠️ **Order S7 (the blob rebuild) before any break-arm verification** — until it lands, a red on the cross-side arm is misattributed to the reference hang (§8). ⚠️ **S0 first of all**: the guest asks for map type **6**, and every downstream arm is vacuous until the enum is corrected. **Budget ~3 differential launches (~20 min) per green pass.**
