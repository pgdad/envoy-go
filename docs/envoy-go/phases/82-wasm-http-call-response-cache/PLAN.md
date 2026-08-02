# PLAN 82 — the http-call response cache: **S1 was priced at 300-600 and MEASURES 30, because FOUR of the SIX Pause sites already honor Pause** — so the split axis dissolves, while the SPEC's own replacement gate goes GREEN ON A TRAP, S5 silently opens a guest WRITE surface, and S1 alone LEAKS a connection per request

**Stage:** PLAN (lifecycle-state `2` -> `3`). **ROW 82 STAYS `in-progress`**; `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` are **BYTE-UNTOUCHED**; sentinel `want` STAYS **114**. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master **`71fc86d7`**, taken from `git rev-parse master` at session start — **not** from any SHA quoted in the router. At this tip the SPEC squash **is** the master tip, so the recurring "the router's quoted SHA sits below the tip" hazard did not bite; it was re-derived anyway. Worktree `/home/esa/git/envoy-go-wt/phase-82-plan`, branch `phase-82-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` + `STATE.md` + `STATE_HISTORY.md` + `next-prompt.txt` — **five files**, matching the phase-79/80/81 PLAN precedent.

## What was EXECUTED at this stage

**Five investigation agents on disjoint remits**, each in its own **DETACHED** worktree off `71fc86d7` with private scratch and a private port band inside `42100-42349`, plus controller re-derivation of every load-bearing claim (`feedback_brief_citations_not_evidence`).

| agent | remit | headline |
|---|---|---|
| **A1** | **the mandatory S1 prototype** | **S1's plumbing is 30 net, not 300-600 — four of six Pause sites already honor Pause**; S1 alone **leaks** a connection + goroutine per request |
| **A2** | S7, the guest blob | the committed blob is byte-reproducible **only** under rustc **1.94.0** + `log 0.4.30` — **neither pin is recorded**, and `Cargo.lock` is gitignored |
| **A3** | S8, fixture `0036` | **BREAK-C run today: GREEN** — vacuity proven by execution, not argued; and the (l) counter is a **race**, not a constant |
| **A4** | the trap + the accessor fork | **`ABICallbacks` is 21 methods, not 20**; the trap fix is **13 net** and **fail-first testable TODAY without S1** |
| **A5** | the measured items + every cite | **S0 is NET 0, not "~10 MEASURED"**; six cites wrong, incl. §2.6's own load-bearing one |

**Zero commits, zero branches, zero pushes by any agent.** Every experimental edit reverted; all five reported `git status --porcelain` = **0 lines**, controller-re-confirmed. Docker containers were created by A2 only (`p82p-a2-ref-{old,new}`, network `p82p-a2-net`) and removed **BY NAME**; no image or ancestor filter was used at any point.

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

**Fourteen refutations, of which SIX are load-bearing.** The lineage is unbroken: phase-81 BRAINSTORM 14 / SPEC 22 / PLAN 14 / IMPL 17; phase-82 BRAINSTORM 26 / SPEC 23 / **PLAN 14**.

### 1.1 ⚠️ HEADLINE — S1 WAS PRICED AT **300-600** AND MEASURES **30**, BECAUSE FOUR OF THE SIX PAUSE SITES ALREADY HONOR PAUSE

SPEC §12 calls S1 *"ESTIMATED, not prototyped — the only un-measured item, and the dominant one"* at **300-600 net**, and SPEC §16 makes prototyping it the PLAN's first job. Prototyped.

```
10	0	internal/filter/http/wasm/abi_callbacks.go
 8	7	internal/filter/http/wasm/decode_headers.go
 2	2	internal/filter/http/wasm/encode_headers.go
19	0	internal/filter/http/wasm/wasm.go
        production +39 / -9  =>  NET 30      (code-only, comments stripped: NET 9)
```

Gates on the prototype: `go build ./...` OK · `gofmt -l` **empty output** (gated on output, never exit code) · `go vet` clean · `golangci-lint run` **0 findings** · unit tests 3× `ok` · `-race` 2× `ok`.

**Why it is an order of magnitude cheaper than modelled — controller-re-derived, and this is the load-bearing part:** *"stream control has been deferred since 25.2"* is **FALSE as a blanket claim**. All six production `abi.ProxyActionPause` arms, read at this tip:

| site | disposition **today** |
|---|---|
| `body.go:226` -> `:227` | `DataStopIterationAndBuffer` — **already honors Pause** |
| `body.go:313` -> `:314` | `DataStopIterationAndBuffer` — **already honors Pause** |
| `trailers.go:142` -> `:143` | `TrailersStopIteration` — **already honors Pause** |
| `trailers.go:199` -> `:200` | `TrailersStopIteration` — **already honors Pause** |
| `decode_headers.go:258` -> `:260` | log + `Continue` — **deferred** |
| `encode_headers.go:116` -> `:118` | log + `Continue` — **deferred** |

**Only the two HEADERS sites are deferred**, and the four landed siblings carry **zero** paused-state bookkeeping. S1 is not "architectural primitive 6 wholesale"; it is two switch arms plus the bookkeeping four incumbent sites do without.

⚠️ **The SPEC scopes S1 to the two headers arms. That boundary is now load-bearing and the IMPL must not widen it** — scenario `(n)` (`n_body_cap_exceeded`) depends on the **body** arm pausing indefinitely by design so the host hits `body_buffer_cap_bytes`. Touching `body.go` would destroy it.

**Consequence: the SPEC's split axis dissolves.** Its Leg A / Leg B split was argued from S1 dominating the estimate. It does not dominate; it is now the **fifth**-largest item. See §4.

### 1.2 ⚠️ SECOND HEADLINE — S1 ALONE HANGS **AND LEAKS**, AND THE LEAK IS A COST DRIVER NO DOCUMENT NAMES

A1's standalone subject probe (ports 42101/42102/42103, the **real vendored** blob, the fixture's verbatim 45-capability block):

```
$ curl -s -m 10 -D - http://127.0.0.1:42101/
real 0m10.007s      zero bytes, curl exit 28
```

**And the callback DOES now fire** — the headline behavioural change, read off `/stats/prometheus`:

| counter | at base (SPEC §1, 20 reqs) | with S1 (1 req) |
|---|---|---|
| `http_call_dispatched` | 20 | 1 |
| `http_call_response` | **0** | **1** |
| `http_call_response_after_close` | **20** | **0** |

⚠️ **But 30+ s after the client gave up, `downstream_cx_active{...42101} = 1` and `wasm_wazero_active = 1`, with no `OnDestroy` and no `downstream_rq_xx` increment.** `parkDecode` waits on `decodeResumeCh` **or** `ctx.Done()` (controller-verified, `chain.go:368-374`), and nothing cancels that ctx on downstream disconnect. **S1 without a teardown-time unpark is an unbounded connection + goroutine leak, one per request.** This is a **NEW scope item (S9)**; §12's table has no row for it.

### 1.3 ⚠️ THIRD HEADLINE — THE SPEC'S OWN REPLACEMENT GATE GOES **GREEN ON A TRAP**

SPEC §2.8 correctly diagnoses the (l) arm as a compensating-defect cancellation and §8.1 prescribes replacing the disjunction with `http_call_response >= 1` stacked against `http_call_response_after_close == 0`.

⚠️ **Controller-verified: `rv.stats.HttpCallResponseInc()` fires at `http_call.go:423`, BEFORE the guest call at `:425`.** A callback that traps therefore still increments `http_call_response`. **The prescribed positive leg is blind to the row's worst failure mode** — the twentieth broken-gate shape recurring *inside its own remedy*.

⇒ **The IMPL must move `HttpCallResponseInc()` to fire only on a non-trapping return** (T4), or the split gate is theatre. Recorded as a correction to §8.1, not as a re-litigation of it.

### 1.4 ⚠️ FOURTH HEADLINE — S5 SILENTLY OPENS A GUEST **WRITE** SURFACE ONTO THE STASH

`headerMapForType` returns `(h, active)`. Controller-enumerated: **SEVEN** consumers of that flag, not one —

- **getters (3):** `GetHeaderMap` `:201` · `GetHeaderMapValue` `:235` · `GetHeaderMapSize` `:331`
- **mutators (4):** `AddHeaderMapValue` `:257` · `ReplaceHeaderMapValue` `:272` · `RemoveHeaderMapValue` `:287` · `SetHeaderMapPairs` `:305`

Adding `case 6/7` activates **all seven**. A4 measured the consequence directly: `AddHeaderMapValue(HttpCallResponseHeaders)` returns `Ok` (0) instead of `Unimplemented` (12), and **the guest mutated the stashed http-call response map**. Upstream documents the field read-only (*"Only available during onHttpCallResponse"*, SPEC §7.4).

⚠️ **And the landed golden at `abi_callbacks_test.go:333` requiring `Unimplemented` passes VACUOUSLY under the SPEC's own accessor design** — the fixture leaves `filter.cfg`/`filter.eff` nil, so `headerMapForType` never reaches the new arm. **A green there is not evidence of absence.** S5 needs an explicit read-only guard (T1).

### 1.5 ⚠️ FIFTH — THE ACCESSOR FORK HAS FOUR OPTIONS, NOT TWO, AND THE SPEC'S PREFERRED ONE IS DOMINATED

SPEC §2.16 offers (A) three exported `*RootVM` methods at 93 net, or (B) `ABICallbacks` **20 -> 21**.

⚠️ **`ABICallbacks` has 21 methods TODAY** — controller-verified, `go doc ./internal/wasm ABICallbacks` ⇒ **21** with a firing NC (`NoSuchIface` ⇒ `no symbol`). Option B is **21 -> 22**. The "20" traces to a landed **wrong** comment at `internal/wasm/doc.go:219` (*"ABICallbacks 13→20 methods (Task 15)"*) — `reference_code_comment_not_evidence`, firing for the **third** time this phase.

A4 built and compiled four options from a clean tip, each `go build ./... && go vet ./...` clean:

| option | shape | net prod `.go` | test-fake churn | new exported surface | implementers touched |
|---|---|---:|---:|---|---:|
| **A** (SPEC's) | 3 exported `*RootVM` methods | 93 | 0 | +3 methods | 0 |
| **C2** | **1** exported `*RootVM` method, 3 named returns | **76** | 0 | **+1 method** | 0 |
| **B** (SPEC's) | `ABICallbacks` 21 -> 22 | 59 | 9 | +1 interface method | **4** |
| **C4** | registered `func` sink | 63 | 0 | +1 method +1 func type | 0 |

**DECISION: C2.** It costs **+17 net over the cheapest option (B)**, and the tradeoff is stated as measured, not preferred: B's 9 lines of fake churn are a one-time cost, but widening a 21-method interface to 22 is an obligation **every future implementer inherits permanently**, and the compiler-enumerated blast radius is 4 types across 3 packages including `test/conformance/proxy-wasm/conformance.go:555` (test-support, **not** `_test.go`). C2 also keeps the stash on the `*RootVM`, which is the **upstream-faithful** home SPEC §7.4 already argued for, and permits `atomic.Pointer` per §7.5. **Option A is dominated by C2 on every axis and is NOT adopted.**

### 1.6 ⚠️ SIXTH — S0 IS **NET 0**, IN A ROW THE SPEC LABELS "MEASURED"

`internal/wasm/abi/types.go:68-71` assigns HttpCallResponse **4/5** and Grpc **6/7**; the vendored SDK's `types.rs:95-98` is the reverse. Both controller-verified verbatim. The fix reassigns four values and four golden rows:

```
4	4	internal/wasm/abi/types.go
4	4	internal/wasm/abi/types_test.go        =>  +8 / -8, NET 0
```

SPEC §12's **"~10 | MEASURED"** is not what a numstat produces. The band is unaffected; a figure labelled MEASURED that disagrees with the measurement must not carry forward.

⚠️ **The golden is at `types_test.go:146-149`, NOT `:144-147`** (`:142-145` are types 0-3). The **"exactly 4 subtests"** figure IS correct and was measured before and after.

⚠️ **S0 alone changes NO test outcome** (`go build ./...` clean, all three package suites `ok`) — which is precisely why **S0+S5 must land atomically** (`reference_lifted_reject_hidden_enforcement`): neither half is observable alone.

### 1.7 ⚠️ SEVENTH — THE BLOB IS BYTE-REPRODUCIBLE ONLY UNDER **TWO PINS THAT ARE RECORDED NOWHERE**

The committed blob's `producers` section names `rustc "1.94.0 (4a4ef493e 2026-03-02)"`. A2's ladder, against `sha256 4e630adf…` / 139655 B:

| build | rustc | `log` | result |
|---|---|---|---|
| **the documented recipe** | 1.96.0 (installed default) | — | **LINK FAILURE** — `undefined symbol: proxy_continue_stream` ×15+ |
| `--allow-undefined` | 1.96.0 | 0.4.33 | 139046 B — differs |
| plain recipe | 1.94.0 | 0.4.33 | same size, **49791 bytes differ** |
| **plain recipe** | **1.94.0** | **0.4.30** | **`cmp` ⇒ BYTE-IDENTICAL** |

Controller-verified: `scripts/.gitignore:7` excludes **`Cargo.lock`** (0 tracked repo-wide), so the transitive `log` floats; and `scripts/README.md:24-25` says *"`rustc 1.94.0` (or newer compatible)"* — **measurably false in both directions**. The NC fires: same lock + rustc 1.96.0 ⇒ a different hash, so neither pin alone suffices. Three sibling `0036` blobs rebuilt byte-identical under the same pin set, so the blobs are deterministic — **the recipe just under-specifies its inputs by two pins.**

⚠️ **And nothing would catch a wrong blob.** Controller-verified: `git grep '4e630adf\|139655'` ⇒ **zero hits repo-wide** — no sha256 roster, no size assertion, no golden over `bytecode/`. CI carries **no Rust toolchain**, so the committed `.wasm` is the only artefact any gate ever sees. ⇒ **S7 must land the pins, or its +26-byte binary diff is unreviewable by construction.**

### 1.8 ⚠️ EIGHTH — `0036`'s CROSS-SIDE VACUITY IS NOW PROVEN BY EXECUTION, NOT ARGUED

**BREAK-C was RUN at this stage and is GREEN.** A3 injected `scenario l status=999 body=x-httpcall-status=BOGUS` **symmetrically** — a flagrantly wrong value on both sides, since one `driveProxy` serves `DriveReference*` and `DriveSubject*` — and the fixture still reported `--- PASS: TestDifferential/0036 (34.34s)`. SPEC §2.9's 686-byte figure is confirmed exactly (10×59 + 4×24), verified live by a stream dump on both sides.

⚠️ **§2.9's composition is misstated**: it is **10** `emitConstantSkipToken` calls (arms a-j) + 4 `Fprintf` literals = 14 **lines**, not "14 calls plus 4 literals" = 18. Byte total right, composition wrong.

⚠️ **§8.2's "revives `emitScenario` and `classifyBody`" is REFUTED.** Removing **all five** `//nolint:unused` directives leaves `golangci-lint` with **zero** findings, because `classifyBody` calls `reflectedHeaders`/`reflectedKeys`/`trim` transitively in cases a/b/c/i. A firing NC (an injected unused func ⇒ `driver.go:880:6: func 'a3NegativeControlUnused' is unused`) proves the clean run discriminates. **All five go live.**

⚠️ **BREAK-B is NOT RUNNABLE TODAY** — `headerMapForType` has only `case 0` / `case 2` / `default:`; there is no `case HttpCallResponseHeaders` to revert. It is gated on T1.

### 1.9 ⚠️ NINTH — THE (l) COUNTER IS A **RACE**, SO `delivered=0` IS NOT A FAILING-FIRST ANCHOR

A3 observed the split over five fixture runs: delivered **0,1,1,0,1** — **3/5 delivered**. The landed comment itself hedges (*"driving the response to the after_close branch on most runs"*) and is wrong about which way the coin lands.

⚠️ **THIS IS AN AGENT MEASUREMENT THE CONTROLLER COULD NOT RE-DERIVE, AND IT IS LABELLED AS SUCH.** The fixture prints those counters **only on failure**, and the differential log does not carry the subject's stderr — the controller's own three `0036` runs produced 29-line logs with **zero** `unreachable` / `CallProxyOn` matches. **The channel, not the claim, is what failed; a successor wanting to re-derive F1/F2 must capture the subject's stderr directly.** What the controller DID re-derive, over **three** `-count=1` runs with `INNER_EXIT=0` each and `=== RUN TestDifferential/0036-...` positively asserted in all three (no `[no tests to run]`): **3/3 PASS at 35.27 / 34.19 / 34.16 s** — consistent with A3's 34.2-34.5 s band and refuting the SPEC's ~37 s.

⚠️ **F2's MECHANISM is doubly confirmed even though its INTERMITTENCY is not.** A4 read the arm-less `get_map` verbatim in the vendored SDK, and A1 captured the live trap trace on the forced-resume path (`get_map` -> `panic_fmt` -> `abort`). What remains solely A3's is the claim that it fires **intermittently in the fixture TODAY, without S1**. **The IMPL must settle that before relying on any (l)-arm baseline.**

**Consequences the IMPL must carry regardless of which figure is right:** (a) `http_call_response >= 1` is **not** a deterministic failing-first anchor today; (b) **BREAK-D would be a coin flip** and must not be relied on until S1 makes the path deterministic; (c) the SPEC's "0+20 ⇒ 20+0" arithmetic is wrong for this fixture, though **the invariance conclusion survives** — A1 measured the disjunction absorbing a full operand inversion, with `0036` PASSING both at base (34.54 s) and with S1 (49.54 s, `+15.0 s` = exactly one added client-timeout hang). **§2.8 is now confirmed BY EXECUTION.**

### 1.10 ⚠️ TENTH — THE TRAP IS REAL, **INVISIBLE**, AND FAIL-FIRST TESTABLE **TODAY**

A4 read the vendored `hostcalls.rs:158-174` verbatim: `get_map` has **exactly one `Status::Ok` arm plus `status => panic!("unexpected status: {}")`**. **No `NotFound` arm.** SPEC §1.1 HOLDS at the stated range. `get_map_bytes` (`:176-192`) is identically shaped — the same trap applies to the bytes variant.

A1 surfaced the live trace on the forced-resume path, which is the same chain §1.1 predicts:
```
l_httpcall_success.wasm.abort() ... _ZN4core9panicking9panic_fmt ...
_ZN10proxy_wasm9hostcalls7get_map ... Context30get_http_call_response_headers
... proxy_on_http_call_response(i32,i32,i32,i32,i32)
```
⚠️ **A1 first read `envoy_go.failures = 0` and no log line and nearly concluded "no trap."** `http_call.go:425` discards the error, `noteTrapOnError` is never called, and `HttpCallResponseInc()` fires *first* so the counter reads success. **S1 makes a guest trap reachable with ZERO observability.**

**Incumbent pattern, denominator printed.** Five `runCallWithPanicWrapper` call sites (NC: the sibling `runWithPanicWrapper` has 31, so the matcher is not broken). **Three have a `*StreamContext` in scope: two PROPAGATE** (`stream_context.go:142` -> `noteTrapOnError` at `:150`; `root_vm.go:860`), **one SWALLOWS** — `http_call.go:425`. The two tick sites swallow because there is no `sc` in scope. ⇒ **a lone deviation from a 2-of-2 pattern, not a considered choice.**

**A4 wrote and RAN the fail-first test TODAY, without S1:**
```
--- FAIL: TestA4_HttpCallResponseTrap_PoisonsStreamContext
    sc.trapped = false after proxy_on_http_call_response TRAPPED; want true
```
with two firing NCs (the cap gate did not short-circuit; the `httpCalls` entry was consumed) proving it is not vacuous. **Measured fix: `17 4` ⇒ +13 net production**, against SPEC §12's 40-80. ⇒ **trap propagation is a Leg-B-equivalent task and does NOT wait on S1.**

### 1.11 ⚠️ ELEVENTH — S1 BREAKS ADR-0071's SINGLE-GOROUTINE INVARIANT, AND `-race` PROVES IT

A1's negative control — plain `bool` instead of `atomic.Bool` for the paused flag — fires immediately:
```
WARNING: DATA RACE
Read at ... by goroutine 48:   wasm.(*abiCallbacks).ContinueStream()  abi_callbacks.go:779
                               internal/wasm.(*RootVM).handleHttpCallResponse()  http_call.go:441
Previous write ... by goroutine 47:  wasm.(*filter).DecodeHeaders()
```
With `atomic.Bool`: **0 races** across a single-request arm, a 12-concurrent-request arm (3 genuinely reaching the paused path), and `go test -race` on both packages.

**This is a SECOND, independent atomicity requirement beyond SPEC §7.5's `atomic.Pointer` for the stash.** ADR-0071's *"single-goroutine-per-stream, no synchronization on `*filter` fields"* invariant **does not survive S1** — the http-call dispatch goroutine now reaches `*filter` state. **§11's ADR list does not carry this consequence; ADR-0304 §Consequences must.**

⚠️ **And `decodeResumeCh` is CHAIN-scoped, not filter-scoped** — controller-verified: declared on `FilterChain` at `chain.go:59`, constructed once per chain at `:303`. An unmatched `ContinueDecoding` **latches a token that spuriously releases the NEXT `parkDecode` by ANY filter in the chain** (extauthz, ratelimit, oauth2, bandwidthlimit all park on it). An idempotence gate (`CompareAndSwap(true,false)` before firing) is required. Conversely the buffered-1 channel means an early resume is **latched, not lost** — there is no lost-wakeup race. *(Code-read, not a run: the latch experiment was not executed.)*

### 1.12 ⚠️ TWELFTH — THE PAUSE UNIT TEST HAS **NEVER** TESTED THE PAUSE ARM, AND 122 OF 128 DISPATCH SITES ARE CAP-DENIED

A1's discriminating probe: a `panic` in the Pause branch **did not fire**; a `panic` in the `default:` branch **did**, reporting `action=0 err=<nil>`. Root cause instrumented in `dispatchGuest`: `CAP-DENIED CallProxyOnRequestHeaders`. `newTestCompiledConfig` (`dispatch_test.go:68`) sets **no** `capability_restriction_config`, and `SandboxConfig.IsAllowed` is StrictDefaultDeny — the guest is never invoked, so the switch always sees `Continue`. **`TestFilter_DecodeHeaders_Pause_LogAndContinue` is vacuous and always has been.**

**Scale, denominator printed:** across the package's **423** tests, `CallProxyOnRequestHeaders` reaches its gate **128** times — **122 CAP-DENIED, 6 dispatched, 0 no-export** — from only **FOUR** distinct tests. ⇒ **every S1 unit test must first build a capability-enabled config.** This is the dominant test-cost driver and no phase-82 document names it. It is why T8 exists as its own task.

### 1.13 ⚠️ THIRTEENTH — SIX CITES ARE WRONG, INCLUDING §2.6's OWN LOAD-BEARING ONE

All controller-re-derived at `71fc86d7`:

| SPEC cite | actual |
|---|---|
| `http_call.go:406` = `len(resp.Header)` | **`:405`**; `:406` is `bodySize` — **this is §2.6's load-bearing cite** |
| `types_test.go:144-147` (the golden) | **`:146-149`**; `:142-145` are types 0-3 |
| §6.3 anti-pattern phrase at `:304` | `:304` is the **HEADING**; the phrase is at **`:306`**. **§2.23 declared this "settled" in the WRONG direction** — §1 and §12 both cite `:304` *for the phrase* |
| `abi_callbacks.go:178-190` (§1) | opens 178, **closes 191**. The SPEC's *other* cite `:178-191` (§2.23) was correct, and §2.23 "corrected" the right one into the wrong one |
| `encode_headers.go:115-117` | **`:116-118`**; `:115` is `switch action {`. Independently caught by A1 and A5 |
| `registration.go:877-882` | **`:873-882`** — the "9 stubs" sentence is at `:875-876`, **outside** the cited span |
| `25.1 SPEC:481-483` | **`:481-484`** — `:484` (`GrpcReceiveTrailingMetadata = 7`) is equally wrong and falls outside the span |

⚠️ **`ROADMAP.md:<line>` cites = 117, NOT 118.** Controller-verified with three arms agreeing and a firing NC: **117** occurrences across **46** files; a bogus pattern ⇒ 0. A `git grep -c` sum gives **107** because it counts **LINES** and ten lines carry two cites (`reference_grep_c_zero_is_a_broken_gate`, same species). **SPEC §2.12 corrected 119 -> 118 and overshot by one** — `reference_a_drift_correction_is_itself_a_claim`, firing on the SPEC's own correction.

⚠️ **The `chain.go:666/670/672` "stale by +1" framing is WRONG.** `:666` is the func decl, `:667` `destroyOnce.Do`, `:669` `if f.Decoder != nil`, `:670` `f.Decoder.OnDestroy()`, `:671` `else if f.Encoder`, `:672` `f.Encoder.OnDestroy()`. The two banked sets index **two different constructs** (if-guards vs OnDestroy calls), not one shifted pair.

### 1.14 ⚠️ FOURTEENTH — THE S0 BLAST RADIUS IS UNDERSTATED, AND TWO SPEC MEASUREMENTS DO NOT REPRODUCE

SPEC §1.2 calls S0's blast radius *"small and fully enumerated"*. Controller/A5 `git grep` over the four names finds carriers the list omits: `abi_callbacks_test.go` has **5** mentions (`:244 :245 :246 :247 :333`), **not 3**; and `25.1 PROGRESS.md` (8 hits), `25.2 PROGRESS.md`, `DECISIONS.md:17812`, `ROADMAP.md` row 82 and `next-prompt.txt:103` all carry the values and are unlisted. The doc carriers are append-only, so the **fix size** is unchanged — but *"fully enumerated"* is not true as written.

Two softer non-reproductions, recorded rather than laundered:
- **The 93-net cache prototype does not reproduce exactly** — A5's independent rebuild measured **108** (+16%). Both are lower bounds; **93 is an order-of-magnitude anchor, not a point estimate.** ⚠️ Note also that `types.go 4/4` is **double-counted**, appearing in both §12's "S0 ~10" row and inside the "93 MEASURED" numstat. Net-zero, so 93 is unaffected — but the rows are not disjoint.
- **§7.5's `-race` trigger does not fire as stated.** *"A prototype using a plain struct field was flagged by `-race` immediately"* — A5's cross-product: plain field + standing suites ⇒ **GREEN, no race**; plain field + an out-of-frame accessor read ⇒ **RACE at the accessor**; `atomic.Pointer` + same probe (firing NC) ⇒ PASS. **The conclusion (`atomic.Pointer`) survives and the mechanism is real; the first sentence is overstated.** §7.5's own second sentence says the correct thing.

**A finding for the IMPL that no document carries:** `TestAbiCallbacks_GetHeaderMap_DeferredMapTypes_NotFound` (`abi_callbacks_test.go:237-255`) asserts `ok == false` for six deferred map types **including 6 and 7**. With S5 wired it **still passes**, but only because the fixture leaves `filter.cfg` nil. **After T1 it reports "deferred" for the very types the row activates.** It needs a stacked control or it silently stops guarding — the same vacuity species as §1.4.

### 1.15 CONFIRMED, SO THE IMPL CAN RELY ON IT

- SPEC §1.1's arm-less `get_map` — verbatim, at the stated lines. The **ordering constraint is sound.**
- SPEC §2.5 (`resp.Header` has no `:status`; `StatusCode=200`, `len=3`, keys `Content-Type`/`Date`/`Content-Length`) — reproduced exactly, with a firing NC.
- SPEC §2.6 both halves — `len(resp.Header)` is a KEY count, `GetHeaderMap` is value-expanding (`abi_callbacks.go:200-227`); `go list -deps` ⇒ **1** in one direction, **0** in the other. `numHeaderValues` is unexported with all 10 occurrences inside `internal/filter/http/wasm`. **Disposition (c) is sound.**
- **Q1/Q5, measured:** `ContinueDecoding` resumes a **headers**-phase `StopIteration` through the **same** `parkDecode`/`decodeResumeCh` as the data phase (`chain.go:355-358`), proven by four passing chain tests including a genuine cross-goroutine resume arm. `FilterHeadersStatus` has exactly **two** values, so `StopIteration` is the only option. `RunDecodeHeaders` does `decodeIdx++` after the park, so the filter is not re-invoked — matching Envoy.
- **Q4:** the **encode-side pause is unreachable** — of the 6 `.rs` files returning `Action::Pause`, all are request-headers or request-body; **zero** return it from response headers. ⚠️ **A1's denominator of 25 guest crates is WRONG — it is 35** (controller-verified: 10 conformance + 7 in `0034` + 14 in `0036` + 4 in `0038`, by `Cargo.toml` count). The conclusion survives the correction. **The encode half ships with no exercising guest**; T6 must state that, not claim coverage.
- **S7's minimal diff is ONE line** — `self.resume_http_request();` at the end of `on_http_call_response`, a default trait method on `HttpContext` (`traits.rs:424-426`); no import, no `Cargo.toml` change. **Live probe: committed blob ⇒ 12.002 s hang / curl exit 28; rebuilt blob ⇒ `HTTP 200` + `x-httpcall-status: 200` in 4.6 ms.** Both arms booked `cluster.cluster_b.upstream_rq_200: 1`, independently reproducing §2.1 and §2.4 with a fresh artefact. ⚠️ **Do not gate on the import list** — both blobs import `proxy_continue_stream` (36 imports each); only a runtime probe discriminates.
- **S7 has no blast radius** — `l_httpcall_success` maps 1:1 to listener index 11 only (`driver.go:117-118,144,166,240-241`). ⚠️ `0037` bind-mounts a **different** `0036` blob (`a_body_read_only`) with a *"MUST NOT truncate it"* guard at `runner_test.go:2166-2168`; `l_httpcall_success.wasm` is borrowed by nothing.
- **All five registration gates hold** — dir exists; `RegisterFixture` at `driver.go:86-88` (`init()` is `:85-89`; `:90` is blank — the SPEC's `:86-90` is loose, not wrong); blank import `runner_test.go:63` exact; `BackendKind HTTPWasmAdvanced = 26` at `fixture.go:482` exact; switch case `runner_test.go:853` exact. **No `t.Skipf` silent-green risk** — every run printed `=== RUN` and a real verdict, never `[no tests to run]`.
- **Stat surface +0 SURVIVES — conditionally.** `EnvoyGoFailuresInc()` is already on `RootStatsRecorder` (`stats_recorder.go:140`), already implemented by both recorders, already registered as `envoy_go.failures` (`stats.go:109,464`), and its own doc **mandates** this co-increment. ⚠️ **If the IMPL instead mints a dedicated `http_call_response_trap` counter, §9's `+0` BREAKS.** T3 pins the reuse.
- The `defer` clear runs even when the guest traps — `runCallWithPanicWrapper`'s `recover()` cannot skip an outer `defer`, verified by building it.
- `.golangci.yml` enables `unused` but **not** `nolintlint`; the flip's `+4` is exactly `6 2`; `runner_test.go:1288` uses `t.Errorf`; the flip's first divergence is at **offset 632**, inside the (l) line (spans 614-637), so arms (a)-(k) provably do not mask it.

---

## 2. Global constraints

1. **ORDERING IS A CORRECTNESS CONSTRAINT.** S1 must never become effective before S0+S5+the cache. The Rust SDK has no `NotFound` arm, so honoring Pause against an unwired cache converts a benign wrong header into a **guest trap plus a poison cascade**. Enforced here as **task order within a single row** (T1-T4 before T5), which SPEC §12 explicitly permits: *"if the row is not split, S0/S2-S6 must land in the same commit as S1 or earlier within the row."*
2. **S0 + S5 land ATOMICALLY** (T1). Neither half is observable alone.
3. **`ROADMAP.md` BYTE-UNTOUCHED at this stage**, `want` stays **114**, row 82 stays `in-progress`. Verified by `git diff --stat master -- docs/envoy-go/ROADMAP.md` returning EMPTY.
4. **`DECISIONS.md` and `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED at the PLAN.** ADR-0304 §Decision + §Consequences land at the IMPL; the recurrence guard `^> \*\*STATUS: PROPOSED` must stay armed at exactly **1** and the IMPL flips it to COMPLETE. ⚠️ **Do NOT use the loose form** — controller-verified 5 hits, four of them COMPLETE blocks (ADR-0299/0300/0301/0302).
5. **Every `.go` probe is deleted and the tree proven clean** before any commit.
6. Per-task `gofmt` + `golangci-lint` on touched packages. ⚠️ **`gofmt -l` never exits non-zero — gate on OUTPUT.** ⚠️ **misspell runs `locale: US`.** ⚠️ **An empty lint result is not evidence the linter looked** — inject a British spelling, confirm it fires, restore byte-identically.
7. **Build with `-o <scratch>/bin`**, never into the worktree root, and note `-o` precedes the package path.

---

## 3. File structure — the IMPL's edit surface, RE-DERIVED at `71fc86d7`

**Production `.go` (6 files):**
- `internal/wasm/abi/types.go` — S0, the four enum values
- `internal/wasm/http_call.go` — the stash, `:status` synthesis, value-count, trap propagation, `HttpCallResponseInc` ordering
- `internal/wasm/root_vm.go` — the `atomic.Pointer` slot + the single C2 accessor
- `internal/filter/http/wasm/abi_callbacks.go` — `headerMapForType` cases **6/7**, `GetBuffer` case **4**, the four-mutator read-only guard, `ContinueStream` bookkeeping
- `internal/filter/http/wasm/decode_headers.go` — S1 decode arm
- `internal/filter/http/wasm/encode_headers.go` — S1 encode arm
- `internal/filter/http/wasm/wasm.go` — paused-state fields (A1's prototype put 19 lines here)

⚠️ **`*StreamContext` needs NO new field and NO teardown change** (SPEC §7.3) — but **S9 (the parked-stream leak) may reopen that**, and T7 must settle it by measurement rather than inherit the SPEC's conclusion.

**Test `.go`:** `internal/wasm/abi/types_test.go` (4 golden rows) · `internal/filter/http/wasm/{dispatch,abi_callbacks,wasm_fixtures}_test.go` · `internal/wasm/http_call_test.go` · `internal/filter/http/chain_test.go` · `test/fixtures/0036-.../inputs/driver.go`

**Non-Go:** `scripts/l_httpcall_success/src/lib.rs` (+1 line) · `bytecode/l_httpcall_success.wasm` (+26 bytes) · `scripts/README.md` (the pins) · `scripts/.gitignore` (−1 line if `Cargo.lock` is committed) · `0036/README.md` · `0036/expectations.yaml`

---

## Task 1 — **S0 + S5, ATOMIC**: correct the enum, wire cases 6/7 and buffer 4, and GUARD the four mutators

Reassign `types.go:68-71` to the canonical `Grpc 4/5, HttpCall 6/7`; update the four golden rows at `types_test.go:146-149`. In the same commit add `case 6/7` to `headerMapForType` and `case 4` to `GetBuffer`, **and add an explicit read-only guard so the four mutators (`AddHeaderMapValue`, `ReplaceHeaderMapValue`, `RemoveHeaderMapValue`, `SetHeaderMapPairs`) return `WasmResultUnimplemented` for map types 6 and 7** (§1.4).

⚠️ **Derive the wire constants from `proxy-wasm-0.2.4/src/types.rs:95-98`, NEVER from the code under test** (`reference_handwritten_golden_shares_author_mistake`). ⚠️ **Single-source derivation — the upstream C++ host was NOT obtained; record that.** ⚠️ Fix the two vacuous tests (`abi_callbacks_test.go:237-255` and `:333`) by giving them a capability-enabled config, or they silently stop guarding.

**Verify:** exactly **4** subtests flip red then green. Landed 25.1 SPEC/PROGRESS records are **recorded, not fixed** (append-only).

## Task 2 — the cache: stash, `:status`, value-count, `defer` clear, via the **C2** accessor

`atomic.Pointer[httpCallResponse]` on `*RootVM`; set under the existing `dispatchMu` frame immediately before the guest call, cleared by `defer` immediately after. **One** exported method (§1.5). Synthesize `":status"` into a **copy** of `resp.Header` by direct map assignment (never `Header.Set`, never mutating `resp.Header`), **before** the count is taken. Compute `numHeaders`/`numTrailers` as **value** counts inline in `internal/wasm`, with a comment at both sites naming the `numHeaderValues` twin (§5's deliberate duplication).

⚠️ `len(resp.Header)` is at **`:405`**, not `:406`.

## Task 3 — trap propagation, reusing `EnvoyGoFailuresInc()`

`sc.noteTrapOnError(callErr)` + `rv.stats.EnvoyGoFailuresInc()` at `http_call.go:425`, mirroring `dispatchGuest`. **+13 net production, measured.** ⚠️ **Reuse the existing counter — a dedicated one BREAKS §9's `+0`** (§1.15). **Fail-first anchor exists TODAY without S1** (§1.10); land the failing test first and keep A4's two NCs.

## Task 4 — move `HttpCallResponseInc()` to fire only on a NON-TRAPPING return

Without this, T14's positive stats leg goes **green on a trap** (§1.3). ⚠️ **Confirm the direction by execution**: with T3 landed and the guest trapping, `http_call_response` must read **0** and `envoy_go.failures` **≥1**.

## Task 5 — **S1 decode**: `StopIteration` + `atomic.Bool` paused state + the idempotence gate

Return `envoyhttp.StopIteration` from the `ProxyActionPause` arm at `decode_headers.go:258-260`. Add `atomic.Bool` paused state (⚠️ **a plain `bool` fires `-race` on the first request** — §1.11) and gate the resume with `CompareAndSwap(true,false)` so an unmatched `ContinueDecoding` cannot latch a token that un-parks a **different** filter on the chain-scoped `decodeResumeCh` (§1.11).

⚠️ **Do NOT touch `body.go` or `trailers.go`** — those four sites already honor Pause and scenario `(n)` depends on the body arm pausing indefinitely (§1.1).

## Task 6 — **S1 encode**, stated as UNEXERCISED

Symmetric change at `encode_headers.go:116-118`. ⚠️ **Zero of the 35 guest crates return `Action::Pause` from response headers** (§1.15), so this half ships with **no exercising guest**. It needs a synthetic unit fixture; `buildPauseProxyWasm` (`wasm_fixtures_test.go:320`) exports only `proxy_on_request_headers` and needs a response-headers sibling. **Say "unexercised", not "covered."**

## Task 7 — **S9 (NEW)**: the parked-stream leak

S1 alone leaves `downstream_cx_active = 1` and `wasm_wazero_active = 1` for 30+ s after the client disconnects, with no `OnDestroy` (§1.2). Settle the teardown-time unpark by **measurement**, not by inheriting SPEC §7.3's "no teardown wiring" conclusion — that conclusion was derived for the *stash*, not for the *park*. **Assert the gauges return to 0 after client disconnect.**

## Task 8 — the capability-enabled test harness

**122 of 128 `CallProxyOnRequestHeaders` gate-sites across 423 tests are CAP-DENIED, from only FOUR distinct tests** (§1.12). Build the shared capability-enabled `compiledConfig` helper first (templates at `dispatch_test.go:381`, `bugfix_test.go:139`); every subsequent test task depends on it. ⚠️ **`TestFilter_DecodeHeaders_Pause_LogAndContinue` is vacuous today** — rewrite it and prove the rewrite fires with a discriminating probe (a `panic` in the arm under test must fire).

## Tasks 9-12 — unit tests

**T9** decode/encode pause arms · **T10** the cache end-to-end through production `abiCallbacks` dispatch with the real vendored blob · **T11** `ContinueStream` (6 arms; today only the nil-cb + BadArgument arms exist at `abi_callbacks_test.go:816`, and **nothing asserts `ContinueDecoding` actually fires** — a counting `DecoderFilterCallbacks` must be written; none exists) · **T12** the cross-goroutine `-race` regression, with the plain-`bool` negative control retained as the discriminator.

## Task 13 — **S7**: rebuild the blob **and land the pins**

One line: `self.resume_http_request();`. ⚠️ **The row MUST also land the two pins** (§1.7) — pin `rustc 1.94.0` and either commit `Cargo.lock` (delete `scripts/.gitignore:7`) or pin `log 0.4.30`; correct the false *"or newer compatible"* at `scripts/README.md:24-25`; record the 1.96.0 `--allow-undefined` link failure. **Without the pins the +26-byte binary diff is unreviewable, and no gate in the repo would catch a wrong blob** (`git grep '4e630adf\|139655'` ⇒ zero hits repo-wide).

⚠️ **Verify by RUNTIME probe, never by import list** — both blobs import `proxy_continue_stream`.

## Task 14 — **S8**: the fixture, measured at **+26 net across 3 files**

Per-site, each figure a real `--numstat` from A3: cross-side flip `+4` (⚠️ **including a NEW `case "l":` in `classifyBody` — there is none today, and without it the flip emits `body=skip` and stays vacuous**) · (l) stats split **−4** · backwards comment `:504-509` **+6** · five `//nolint:unused` removals **−7** · package doc **+8** · `README.md` **+8** · `expectations.yaml` **+11**.

⚠️ Both stats legs use `t.Errorf` so each reports independently — A3 proved both fire in one run. ⚠️ README `:71` and `expectations.yaml:105` are **wrong today in both directions** and must be corrected to describe what the code asserts.

## Task 15 — the break roster, each arm proving its OWN assertion fired

**BREAK-B is mandatory.** ⚠️ **Order T13 before ANY break verification** — until the blob lands, a red is misattributed to the reference hang (§1.9). ⚠️ **BREAK-C is already GREEN and is the negative control** proving BREAK-A must be **side-asymmetric**; `driveProxy(ctx, addrs, side string)` already threads `side`, so the injection is mechanical. ⚠️ **BREAK-D is a coin flip until S1 lands** (§1.9) — do not run it before T5. **Confirm WHICH assertion fired, not merely that one did.**

## Task 16 — gates, ADR-0304 completion, row 82 -> `done`, the sentinel, the stage close

Six-gate per `BOOTSTRAP_PROMPT.md` §7.5 `:357`, gates `:360-365`, close `:367`. ADR-0304 §Decision + §Consequences appended **IN PLACE** after the retained footer, no renumber, **no `---` separator**; **§Consequences must carry the ADR-0071 invariant break** (§1.11). Row 82 flips `done`; `want` stays **114**; re-run the sentinel with firing NCs and the leak check.

---

## 4. Band — **~800-1300 net, budget ~1050, SIXTEEN tasks. NO SPLIT.**

**The estimate is MEASURED on six of nine items, not modelled.**

| item | SPEC | **THIS PLAN** | basis |
|---|---|---|---|
| S0 enum + golden | ~10 "MEASURED" | **0** | **MEASURED** — pure reorder, +8/−8 |
| S1 pause half | **300-600** ESTIMATED | **30** | **MEASURED** (A1 prototype, gates green) |
| S9 parked-stream leak | **not named** | 40-100 | ESTIMATED — new |
| S2-S6 cache via C2 | 93 MEASURED | **76** | **MEASURED** (A4 built + compiled) |
| S4 value-count | 30-50 | folded above | |
| trap propagation | 40-80 | **13** | **MEASURED** (A4, fail-first test run) |
| S5 mutator guard | **not named** | 15-30 | ESTIMATED — new |
| S7 blob + pins | absent from the `.go` table | **0 `.go`** | **MEASURED** — +1 Rust line, ~11-22 doc lines |
| S8 fixture | 30-60 | **+26** | **MEASURED** (A3, per-site) |
| **production subtotal** | | **~200-275** | |
| unit tests | 200-400 | **~600-900** | S1 alone enumerated at **~445** per-site by A1; T8's harness is the multiplier |
| **TOTAL** | ~710-1300 | **~800-1175, budget ~1050** | |

**NEITHER §6.1 TRIGGER FIRES.** Tasks **16** against `:289`'s ~25 (1.6× margin). LoC ~1050 against `:290`'s ~1500 (1.4× margin).

⚠️ **THE COMPOSITION INVERTED EVEN THOUGH THE TOTAL DID NOT.** The SPEC put **300-600 on S1** and **200-400 on tests**; measurement puts **30 on S1** and **600-900 on tests**. The band survives by coincidence, not by confirmation — and the residual risk has moved from an unprototyped primitive to **test scaffolding**, which is exactly what ran phase 81 to 3.07×.

### ⚠️ WHY THIS PLAN DOES **NOT** SPLIT, AND WHAT WOULD CHANGE THAT

**The SPEC's split axis was priced on S1 dominating the estimate. It measures 30 net and is now the fifth-largest item — the axis dissolves.** The ordering constraint the split existed to enforce is satisfied by **task order within one row** (T1-T4 before T5), which SPEC §12 explicitly permits. Splitting now would create a `82.1`/`82.2` pair whose first leg is ~120 net production lines — below the size at which this lineage has ever split — and would park row 82 `in-progress` behind a second full lifecycle for no correctness gain.

⚠️ **A MEASURED PROTOTYPE IS A LOWER BOUND** (`reference_measured_prototype_is_a_lower_bound`). Phase 81 landed **3.07×** over a nine-prototype measured basis; phase 80 ~2.6×. At 1.5× this row lands ~1575 — **across `:290`**. At 3× it lands ~3150. **The IMPL MUST record a §6.1 crossing as a FINDING if it occurs, and must NOT retro-split** (the phase-81 precedent). The mid-execution trigger (`:292`, any single task's sub-steps past ~10) stays live — **T8 and T13 are the two most likely to trip it.**

⚠️ **The split convention this project practices — legs in the parent's `sub-phases` PROSE, no sub-phase ROW since 32.2 — conflicts with `ROADMAP.md` §Schema `:18`'s written *"Sub-phases get their own rows"*.** Controller-verified at this tip: rows 36/39/40/42/43/60 all carry prose legs and **zero** dotted rows exist above 32.2. **Recorded, not relitigated.** Had this PLAN split, `want` would still stay 114 and only the `sub-phases` cell would be written — but the leak check would go live and row 82's summary cell (which carries check (3)'s literal `WASM-family row` at line 144, field 7) must not be disturbed.

---

## 5. Sentinel — re-run MECHANICALLY at this stage. It does **NOT** fire; `stop` was **NOT** created

Input measured **230 lines / 114 data rows** before anything was written, so an empty result cannot read as a zero result. `ls stop` ⇒ `No such file or directory`.

- **(1)** at `want=114` ⇒ **`NOT DONE: row 82`**, denominator printed (`examined 114 data rows`). ⚠️ **Both NCs run and both fired**, even though the check is live — row 62 doctored on a scratch copy ⇒ **`NOT DONE: row 62`** alongside row 82, with the doctored field printed first (`NC LANDED? [ in-progress ]`); `want=113` ⇒ `GATE FAIL: examined 114 data rows, expected 113`.
- **(2)** **FIVE — `:192 :202 :212 :218 :226`. UNCHANGED. THIS ROW NARROWS NOTHING, STATED AND NOT FORECAST.** The **THIRTY-FOURTH** consecutive phase without a decrease. A one-arm strip moves the union **5 -> 4, not 5 -> 0**, re-confirmed here.
- **(3)** **`NEVER OPENED: gRPC` — ALONE.** NC: an invented slug fires; the registered `WASM-family row` correctly prints nothing.
- **Leak check:** `git diff --stat master -- docs/envoy-go/ROADMAP.md` ⇒ **EMPTY**.

**Row well-formedness — the DISJUNCTION re-executed over all 114 rows.** ARM-A (escape-aware) catches **119** and **131**; ARM-B catches **140**; **naive `NF==8` flags SEVENTEEN** (15 FP + 2 true, and **misses 140**) ⇒ wrong in both directions. **Row 82 (line 144) is flagged by NEITHER arm** — escape-aware 8 pieces, empty trailing piece, 7 pipes / 0 escaped.

⚠️ **A gate-authoring trap the controller hit live at this stage and is recording rather than hiding:** the first ARM-A implementation used the wrong pipe-split threshold (`!=9` instead of `!=8`) and flagged **113 of 114 rows** — a gate that fires on almost everything reads as "thorough" and is worthless. **The denominator saved it.** `reference_gate_command_negative_control`: the NC itself can be broken.

---

## 6. Counts at this tip — re-derived, each with a negative control

fixtures **120** (`^[0-9]{4}[a-z]?-`; bare `^[0-9]{4}-` gives **118**, the delta being `0007a-cors` + `0007b-iteration-probe`; next-free **0119**) · fuzzers **55** (⚠️ **scoped to `-- '*.go'`; unrestricted gives 161** — 106 hits are Markdown code blocks) · internal packages **73** · phase dirs **123**, `REVIEW.md` **37**, without **86** · BackendKind tail **38** over **39** declarations · `DECISIONS.md` **17824** lines / **303** `^## ADR-` / tail **ADR-0304 PROPOSED** @`:17798-17800` / next-free **ADR-0305** / **exactly one gap at ADR-0209** (firing NCs on both neighbours + a full-range 0001-0304 diff; **the 303-vs-0304 arithmetic is CONSISTENT *because of* the gap — do not "fix" it**) / retained footers **10** / `^---$` **216** / STATUS census **17** · `ROADMAP.md` **230 lines / 114 data rows** (⚠️ a numeric-id regex gives **112**, silently dropping `28.1a`/`28.1b`) · `BEHAVIOR_CONTRACT.md` **5870** · `STATE.md` **63** · `STATE_HISTORY.md` **442**, tolerant **171** / naive **161** (⚠️ an over-tolerant anchor gives **172** by swallowing §Current's own live pointer at `:23`) · **`ROADMAP.md:<line>` cites 117** ⚠️ **NOT 118** · stat registration **208 code sites**; ⚠️ **35 code-bearing files of 36 matched** (`internal/stats/doc.go` contributes 0) — **and the origin of the discredited "84" is now identified: `git grep -l … -- '*.go' | wc -l`, a FILE count WITH TESTS INCLUDED, wrong on both axes.**

⚠️ **Guest crates: 35** (10 conformance + 7 `0034` + 14 `0036` + 4 `0038`), **36 blobs** (`0039` carries a crate-less `probe.wasm`). ⚠️ **ADR-0303's "EIGHT footers" is a DIFFERENT DENOMINATOR (0293-0302), not an error**; SPEC §11's *"state 9, or say 8 predecessors"* is **stale at this tip — it is 10**, exactly as §11 itself predicted.

⚠️ **DO NOT SOURCE ANY COUNT FROM `STATE.md` §Project** — six phases stale and wrong on at least three axes. **Anchor on §Current; do not "fix" §Project.**

---

## 7. Deferred — named so no later stage re-derives them

The 9 `WasmResultUnimplemented` stubs (`registration.go:873-882`, ⚠️ **not `:877-882`**) · `proxy_on_queue_ready` / `proxy_on_grpc_*` · the wasm **network** filter · the 6 deferred cpp-host conformance families · **any trailers ASSERTION** · the dead request/response trailer map types 1/3 · the `0036` cross-side restoration for scenarios (a)-(j) · the driver-owned receiver port race (**42** fixtures) · `stats-name-empty-segment-guards` (24 sites) · the RBAC policy-name PROJECTION divergence · the lua `statNameRegexLiteral` (~40 net, 1 task) · the matcher-tree `Action`-name boot walk · **three** stale incumbent guard cites (`lua/compiled_config.go:272` ×2, `redisproxy/config.go:51`) · **the unbounded `io.ReadAll` at `http_call.go:302`** (upstream bounds it; pre-existing) · **the wrong `plugin_context_id` at `:427`** · **cancel-at-destruction as a DEPARTURE documented as parity** · **the three wrong cpp-host cites in `http_call.go`'s header** · **the stale `allCallbacksNoOp` comment at `root_abi_callbacks.go:43`** (the type occurs exactly once repo-wide — in that comment; NC: `rootABICallbacks` ⇒ 6 files) · **the wrong `doc.go:219` ABICallbacks count** · **the 25.1/25.2 records carrying the swapped enum** (append-only; record, do not fix).

---

## 8. Gate hygiene — the broken-gate count is **TWENTY-THREE**; TWO new shapes landed here

**The twenty-second: A REPLACEMENT GATE THAT INHERITS THE DEFECT IT REPLACES.** SPEC §8.1 replaces a disjunction that is invariant under the row's change with a positive leg that is **green on a trap**, because the counter increments before the guest call (§1.3). **Fixing a gate is not the same as fixing the gate's blindness — re-derive the new gate's failure modes, not just the old one's.**

**The twenty-third: A GATE THAT FIRES ON ALMOST EVERY ROW READS AS THOROUGH AND IS WORTHLESS.** The controller's own first ARM-A used the wrong pipe-split threshold and flagged **113 of 114** rows (§5). Only the printed denominator exposed it.

**Priors that fired LIVE at this stage:** **a landed CODE COMMENT is not evidence** — three times (`doc.go:219`'s wrong ABICallbacks count; `root_abi_callbacks.go:43`'s nonexistent type; `ContinueStream`'s doc naming only the data-phase resume) · **a hand-written golden shares its author's mistake** (`types_test.go:146-149`) · **a drift CORRECTION is itself a claim** (SPEC §2.12's 119→118 overshot to 117; §2.23 "settled" the `:304`/`:306` dispute the wrong way and "corrected" the right `headerMapForType` cite into the wrong one) · **`grep -c` counts LINES not occurrences** (107 vs 117) · **an empty output is not a zero result** (A1 read `envoy_go.failures = 0` and nearly concluded "no trap") · **a probe must DISCRIMINATE** (the vacuous Pause test; the vacuous `Unimplemented` golden) · **vacuous break modes** (BREAK-C green by construction) · **`Shell cwd was reset`** fired repeatedly.

**The twenty-one carried forward, unchanged:** a guard is a claim at TRANSCRIPTION time · two defects that CANCEL in the gate metric · an inert gate cell · a full-suite recipe without `-v` is VACUOUS · a sha256 roster desynced against a DELETED file · `gofmt -l` NEVER exits non-zero · `go doc -all <A> <B>` swallows arg2 · a `+0 exported symbols` gate over an EMPTY package reds on a CORRECT tree · a RANGE gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` exits 0 on a DELETED file · a count-only stat guard PASSES a build with BOTH names wrong · a `-run` no-match exits 0 with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests exceeds the fixture denominator · a stat-delta claim cannot be discharged by guards scoped to another package · a stderr-VOLUME assertion passes on the hang · `golangci-lint` misspell locale US · a harness's exit code is not the command's · a GOLDEN ROSTER that omits the family under test · a NEGATIVE CONTROL POINTED AT A TARGET THAT DOES NOT EXIST · a gate metric INVARIANT under the change it guards · a hand-written ENUM golden pinning WRONG wire values.

**Gate posture for a DOCS-ONLY PLAN** — stated as the posture a docs-only stage can have, not claimed green: **(a)/(b)** differential **INAPPLICABLE** (zero `.go` committed; measurement runs were made and reverted — `0036` PASSED **3/3** at 35.27 / 34.19 / 34.16 s in the controller's own runs, `INNER_EXIT=0` each) · **(c)** conformance **INAPPLICABLE**; ⚠️ when re-run, **state the denominator: 10 of the cpp-host's 16 files (62.5%), 6 deferred** · **(d)** fuzzers **VACUOUS** — this row adds none (**55**) · **(e)** **INAPPLICABLE** — zero `.go` bytes, `go.mod`/`go.sum` untouched · **(f)** `REVIEW.md` **ABSENT — a STANDING LINEAGE DEPARTURE**: **86 of 123** phase dirs carry none (**37** do); the last authored was 25.3.

**Measured baselines for the IMPL:** full 120-fixture differential **120/120 in 402 s** · `0036` alone **34.2-35.3 s** (⚠️ **not 37**), of which **30.014 s** is dead reference client-timeout ((l) 15.000 + (n) 15.013); **S1 pushes it to 49.5 s** until T13 lands · h2spec **106 passed / 0 failed / 0 skipped** (⚠️ **the lineage's "53/53" is ONE HALF — state your own denominator**) · `go test ./cmd/envoy-go/` **8.7 s** · touched packages ~1 s. `-race` is a SECOND run. ⚠️ **CI runs the differential at `-timeout 20m`, job `timeout-minutes: 30`.** ⚠️ **Budget ~3 differential launches per green pass**; the driver-owned receiver race is measured at **42** fixtures (it did **not** fire in any of the controller's or A3's 11 `0036` runs at this tip).

---

## 9. Self-review against the SPEC

**Honored:** S1 prototyped and re-measured first (§16's mandate) · the ordering constraint preserved · S0+S5 atomic · the accessor picked deliberately and recorded · trailers assertion still out of scope · `ROADMAP.md`/`DECISIONS.md`/`BEHAVIOR_CONTRACT.md` byte-untouched · S7 ordered before break verification · zero `.go` committed.

**Departed, with the reason:** **NO SPLIT** — the SPEC handed the PLAN an axis and a mandatory order and required the PLAN to decide on measurement; the measurement removed the axis. **The order is preserved as task order, which §12 explicitly permits.**

**Corrected in the SPEC:** S0's cost · S1's cost · the §8.1 replacement gate · S5's scope (the mutators) · the accessor option set · seven cites · the ROADMAP-cite count · the `emitScenario` nolint claim · the `0036` wall time · the guest-crate denominator · S1's blanket "deferred since 25.2".

**Added that no phase-82 document carries:** the parked-stream leak (S9) · the two missing blob pins · the cap-denied test population (122/128) · the ADR-0071 invariant break · the chain-scoped `decodeResumeCh` latch.

---

## 10. Operative memories

`feedback_execution_style` · `feedback_git_worktrees` · `feedback_subagents_no_push` · `feedback_push_to_origin` · `reference_bash_cwd_reset_commits_to_main` (**fired repeatedly**) · `reference_parallel_subagents_private_scratch` · `reference_parallel_agents_shared_machine_namespaces` · `feedback_brief_citations_not_evidence` (**five briefs re-derived, five corrected**) · `reference_code_comment_not_evidence` (**three firings**) · `reference_handwritten_golden_shares_author_mistake` · `reference_compensating_defects_cancel_in_the_gate_metric` · `reference_measured_prototype_is_a_lower_bound` · `reference_a_drift_correction_is_itself_a_claim` · `reference_probe_must_discriminate` · `reference_empty_output_is_not_a_zero_result` · `reference_vacuous_break_modes` · `reference_break_arm_injection_site_is_a_claim` · `reference_liveness_break_needs_failing_baseline` · `reference_deliberate_break_wrong_assertion` · `reference_gate_command_negative_control` · `reference_lifted_reject_hidden_enforcement` · `reference_grep_c_zero_is_a_broken_gate` · `reference_sample_is_not_an_audit` · `reference_stale_cite_recurs_fix_by_pattern` · `reference_recursive_grep_blind_to_gitignored_tracked_file` · `reference_sentinel_matcher_string_self_clears` · `reference_roadmap_split_phase_row_done` · `feedback_pertask_gofmt_lint`.
