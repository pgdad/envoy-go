# SPEC 83 — wasm-pause-arm-leak

**Stage:** SPEC (lifecycle-state `1` -> `2`). **ROW 83 STAYS `in-progress`**; `ROADMAP.md` is **BYTE-UNTOUCHED** and the sentinel `want` **STAYS 115**. Base master **`5d0892e4`**, taken from `git rev-parse master` at session start and **not** from any SHA quoted in the router (the router's prose still names `8a0126d2`, which is one commit stale). Branch `phase-83-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

Five investigation agents ran on disjoint remits, each in its own DETACHED worktree off `5d0892e4` with private scratch and a private port band inside `43400-43799`. Every load-bearing claim below was **re-derived by the controller** rather than adopted from a brief (`feedback_brief_citations_not_evidence`). Where an agent's claim did not survive that re-derivation, it is recorded as a refutation of the agent (§2.10), and where the **controller's own** hypothesis did not survive an agent's measurement, that is recorded too (§2.2) — a drift correction is itself a claim (`reference_a_drift_correction_is_itself_a_claim`).

---

## 0. SENTINEL — RE-RUN MECHANICALLY AT THIS TIP. IT DOES **NOT** FIRE; `stop` WAS **NOT** CREATED

Input measured **231 lines / 115 data rows** before anything was written, so an empty result cannot read as a zero result (`reference_empty_output_is_not_a_zero_result`).

- **(1)** at `want=115` ⇒ **`NOT DONE: row 83`**, with the denominator printed (`examined 115 data rows`). That is **CORRECT AND EXPECTED** while the phase is open; it goes silent only when the phase-83 IMPL flips row 83 `done`.
- **(2)** **FIVE — `:193 :203 :213 :219 :227`. UNCHANGED. THIS ROW NARROWS NOTHING, STATED AND NOT FORECAST.** The **THIRTY-SEVENTH** consecutive phase at which it did not go down.
- **(3)** **`NEVER OPENED: gRPC` — ALONE.**
- ⇒ all three checks print, so **the sentinel does not fire.** `ls stop` ⇒ `No such file or directory`, and it was **not** created. The condition is a CONJUNCTION and checks (2) and (3) are both live.

**FIVE negative controls, ALL FIRED** — the doctored-copy NC was run even though check (1) is currently loud, because the moment row 83 flips `done` the check goes silent and silence is otherwise indistinguishable from a broken check:

| NC | result | fired? |
|---|---|---|
| row 62 doctored `in-progress` on a scratch copy, with `NC LANDED? [ in-progress ]` **inspected before the result was trusted** | `NOT DONE: row 62` alongside `row 83` | ✅ |
| denominator `want=114` against the real file | `GATE FAIL: examined 115 data rows, expected 114` | ✅ |
| check (3) with an invented slug | `NEVER OPENED: Zzzz-invented`; `WASM` and `HTTP-filters` correctly silent | ✅ |
| check (2) **ONE-ARM** strip (`sed 's/deferred candidates://'`) | union moves **5 -> 4, NOT 5 -> 0** | ✅ |
| check (2) **BOTH** arms stripped | 5 -> 0 | ✅ |

⚠️ **THE ONE-ARM RESULT WAS RE-CONFIRMED LIVE AND IN A SECOND WAY.** While mapping anchors to families this stage ran a **single-phrase** matcher (`deferred candidates:`) by accident and it printed **ONE of the five** anchors, not five — the long form `remaining deferred (not-yet-chartered) candidates:` does not contain that substring. **Run the union form, never one half of it.**

⚠️ **CHECK (2) CANNOT MOVE FOR THIS ROW, AND THAT IS NOW A STRUCTURAL FACT RATHER THAN AN ASSERTION.** Mapping each anchor to its owning `^### ` heading: `:193` HTTP/3 + QUIC · `:203` xDS / dynamic config · `:213` Observability · `:219` Runtime + hot restart · `:227` Operational tooling. **The `### WASM host family` heading is at `:221` and owns NO check-(2) anchor.** The WASM family carries no deferred-candidate sentence at all, so there is nothing for a WASM row to narrow.

### 0.1 The leak check is **VERIFIED BY DIFF**, not "inapplicable"

This SPEC writes **no ROADMAP cell**, so the by-mention silencing hazard cannot fire. Asserted mechanically rather than by eye:

```sh
git diff --stat master -- docs/envoy-go/ROADMAP.md   # EMPTY at this stage
```

⚠️ **The two classes remain opposite and must not be conflated.** `<slug>-family row` is check (3)'s **PASS** condition; `deferred candidates:` / `remaining deferred (not-yet-chartered) candidates:` are check (2)'s **FAIL** conditions. **One leak silences a real failure; the other manufactures one.** This SPEC writes neither into `ROADMAP.md`.

⚠️ **THE SENTINEL'S SCOPE WAS CONFIRMED BY POSITIVE CONTROL, NOT ASSUMED.** All three checks name `docs/envoy-go/ROADMAP.md` and nothing else. Writing all four matcher strings into a scratch `SPEC.md` left checks (2) and (3) **UNCHANGED**; concatenating the same four strings onto a `ROADMAP.md` copy moved check (2) **5 -> 7** and made check (3)'s `NEVER OPENED: gRPC` go **SILENT — a false PASS**. Writing these strings into `SPEC.md` / `DECISIONS.md` / `STATE.md` is therefore **SAFE**; writing them into `ROADMAP.md` is not.

### 0.2 Row well-formedness — the DISJUNCTION re-executed over all 115 rows

Denominator printed: **115 data rows, lines 31-145.** Escape-aware = `\|` masked before splitting.

- **ARM-A** (escape-aware pipe-split, `pieces != 8`) catches **line 119 (pieces=9)** and **line 131 (pieces=10)** — and nothing else.
- **ARM-B** (escape-aware non-empty trailing piece) catches **line 140** only (trailing `**IMPL done 2026-07-27** — lifecycle-sta…`, i.e. no closing `|`).
- **Naive non-escape-aware `NF!=8` flags SEVENTEEN** — `55 56 57 58 60 63 66 72 74 105 110 111 117 119 124 131 136` — **15 FALSE POSITIVES + 2 true, and it MISSES line 140** (`NF=8` there; compensating defects cancel). Wrong in **both** directions.
- **Row 83 (line 145): escape-aware pieces = 8, empty trailing piece ⇒ flagged by NEITHER arm.**
- **BOTH ARMS NEGATIVE-CONTROLLED WITH THE DOCTORING VERIFIED TO HAVE LANDED.** Stripping one pipe from line 145 moved its pipe count **7 -> 6** and ARM-A fired (`pieces=7`); appending ` JUNK` after the trailing pipe made ARM-B fire (`trailing=[JUNK]`). ⚠️ **A first NC attempt did NOT land** (its `sub()` pattern never matched) and would have read as *"the arm is blind"* had the landing not been inspected first.

---

## 1. SCOPE — AND THE CENTRAL FINDING: THE ROW HAS **NO IN-TREE REPRODUCER**, AND THE FIX AS CHARTERED IS **INCORRECT**

The BRAINSTORM's defect is real, severe, and confirmed by three independent reproductions. But two of its framing claims and its entire remedy shape did not survive measurement.

**What survives, unqualified.** Six production `abi.ProxyActionPause` arms exist; two set the resume flag and arm the watchdog; **four return a status that PARKS the dispatch goroutine while arming nothing.** The census was re-derived symbol-anchored with the denominator stated (`^\s*case abi\.ProxyActionPause:` over non-test `.go` ⇒ **exactly 6**, all in `internal/filter/http/wasm/`), and **every BRAINSTORM line anchor is correct at this tip — no drift**:

| arm | `case` | `return` | returns | flag/watchdog |
|---|---|---|---|---|
| DecodeHeaders | `decode_headers.go:269` | `:270` | `StopIteration` via `beginDecodePause()` | **YES** |
| EncodeHeaders | `encode_headers.go:126` | `:127` | `StopIteration` via `beginEncodePause()` | **YES** |
| DecodeData | `body.go:226` | `:227` | `DataStopIterationAndBuffer` | no |
| EncodeData | `body.go:313` | `:314` | `DataStopIterationAndBuffer` | no |
| DecodeTrailers | `trailers.go:142` | `:143` | `TrailersStopIteration` | no |
| EncodeTrailers | `trailers.go:199` | `:200` | `TrailersStopIteration` | no |

*(`pause.go:11-14` cites the **return** lines 227/314/143/200 while the BRAINSTORM census cites the **case** lines. Both are correct and internally consistent — the +1 is a convention difference, **not** a defect. Do not "fix" either.)*

"Park" is the right word and the mechanism is a blocking channel receive, not a WaitGroup or condvar (`chain.go:370-377`): `select { case <-c.decodeResumeCh: case <-ctx.Done(): }`. The four claimed chain sites are exact — `:430` and `:569` are the **data** arms, `:474` and `:635` the **trailers** arms — though they are **4 of 8** `parkDecode`/`parkEncode` call sites, not the full set.

### 1.1 ⚠️ THE FIRST CENTRAL FINDING: **NO GUEST IN THIS REPOSITORY REACHES ANY OF THE FOUR UNFLAGGED ARMS**

Measured by instrumenting `body.go:226` and running the full `0036` differential five times: **zero fires**, with two firing positive controls in the same build.

```
P83A1_PROBE DECODE_HEADERS_PAUSE_ARM_FIRED stream=1          (POSITIVE CONTROL — instrumentation is alive)
P83A1_PROBE decode_data_entry stream=1 chunk=2048 accumulated=2048 cap=1024 endStream=true
P83A1_PROBE cap_branch_FIRED stream=1 accumulated=2048 cap=1024
                                                              (no decode_body_DISPATCH, no PAUSE_ARM_FIRED)
```

**Three independent reasons, each measured:**

1. **Scenario (n) trips the cap before dispatch.** `body.go:144` fires the sticky cap + 413 and returns at `:172` — **47 lines before the guest is dispatched at `:191` and 82 lines before the Pause arm at `:226`.**
2. **Scenarios (a)/(b)/(c) return `Continue` on a final chunk.** All three Pause only inside `if !end_of_stream`; `n_body_cap_exceeded` is the only unconditional Pause.
3. ⚠️ **AND (a)/(b)/(c) ARE NOT DRIVEN AT ALL.** `inputs/driver.go:511-513` replaces their probes with `emitConstantSkipToken(&buf, "a"/"b"/"c")`, which takes only `(*bytes.Buffer, string)` and cannot issue a request. **The entire unmodified `0036` run produces exactly ONE `decode_data_entry`, for scenario (n).**

**Discriminating cross-product, both directions.** Raising (n)'s cap 1024 -> 65536 so the body no longer trips it makes the arm **FIRE**, and with the fix in place the watchdog releases at **exactly 10.0 s**; with the fix removed the same probe parks **15 s** until the driver's client gives up. **That is the leak, reproduced in both directions**, and it proves the cap — not the guest — is what currently short-circuits the arm.

⇒ **The row's failing-first anchor MUST BE SYNTHETIC.** No vendored guest, and no differential fixture, can go RED on this defect.

### 1.2 ⚠️ THE SECOND CENTRAL FINDING: THE 2-LINE FIX IS **WRONG**, AND THE BRAINSTORM'S S3 DOES NOT FIX IT

`beginDecodePause()` is not idempotent and `DecodeData` runs **once per chunk** (`connection.go:632-647`, 32 KiB reads). On a multi-chunk body a guest of the (a)/(b)/(c) shape Pauses on **every** non-final chunk, so each chunk re-`Store(true)`s the flag and arms a **new** `time.AfterFunc`, overwriting `f.decodePauseTimer` (`pause.go:107-109`) and orphaning the previous handle. On the final chunk the guest returns `Continue` — which **neither clears `decodePaused` nor stops any timer.** The orphaned watchdogs then fire on an **unparked** chain, win the CAS, and call `ContinueDecoding()`, latching a token on the chain-scoped **buffered-1** `decodeResumeCh` that a *different, later* park spends.

⚠️ **AND THE TIMER-OVERWRITE DEFECT IS REACHABLE TODAY, NOT LATENT.** The BRAINSTORM says it becomes reachable only once four arms are flagged. Measured otherwise: the trigger is *two `beginDecodePause` calls without an intervening `resumeDecode`*, which the buffered-1 chain channel already permits with the two headers arms alone. S1/S2 widen reachability from "needs a sibling filter" to "any headers-then-body stream" — they do not create it.

⚠️ **`Stop()` BEFORE REASSIGN — THE BRAINSTORM'S ENTIRE S3 — IS INSUFFICIENT, PROVEN BY A CONTROLLED A/B.** `Timer.Stop()` cannot cancel a callback that has already entered; if the old timer fires between `Store(true)` and `Stop()`, its CAS sees the flag **just set for the new pause**, succeeds, and force-resumes a pause it does not own. 300 back-to-back superseding pauses at a 50 µs watchdog, same binary, guard on vs off:

| arm | `ContinueDecoding` fires (of 300 superseded generations) |
|---|---|
| `Stop()` only | **299** |
| `Stop()` + generation guard | **1** |
| NC — single pause, no supersession | **1**, exactly |

⚠️ **`-race` CATCHES NOTHING** — full package under `-race` with the prototype: `ok … 1.380s`, zero reports. This is a **logical** race over `atomic.Bool` + `sync.Mutex`, not a data race. **The gate must be a behavioral fire-count assertion.**

### 1.3 ⚠️ THE THIRD CENTRAL FINDING: A **SEVENTH** UNBOUNDED PARK, IN THE SAME FILE, THAT THE CENSUS OF SIX DOES NOT COVER

`body.go:282` — the **encode-side** body cap — logs and returns `envoyhttp.DataStopIterationNoBuffer` with **no `SendLocalReply`** (ADR-0071 makes it unavailable on that side). The encode chain parks on exactly that status. Nothing sets `localReplyDone`, so the decode side's `chain.go:406-408` short-circuit **has no encode-side counterpart**. ⇒ **an unbounded host-initiated park with no flag, no watchdog and no resumer.**

Its own comment is false twice over: *"return StopAllIteration so the chain terminates the response early"* — it returns a different status, and it terminates nothing.

⇒ **The denominator SIX is correct for GUEST Pause arms and incomplete as a LEAK census.** S1/S2 as chartered fix four guest arms and leave a seventh park untouched in the file they edit.

### 1.4 What the row must therefore contain

| item | disposition |
|---|---|
| **S1** flag+arm the two body arms | **KEEP**, but arm-once + `Continue`-path disarm (§1.2), not the 2-line form |
| **S2** flag+arm the two trailers arms | **KEEP** (§4) — needs a NEW type-section entry and TWO new capabilities (§4.2) |
| **S3** stop the previous timer | **UPGRADED to `Stop()` + generation counter** (§7). `Stop()` alone is refuted. |
| **S4** rebuild the vacuous census guard | **KEEP** — and the hole is wider than recorded (§5) |
| **S5** rewrite the false comment surface | **WIDENED from 3 sites to 11** (§6) |
| **S6** re-derive `defaultPauseWatchdog` | **KEEP** — the value survives, the derivation does not (§5) |
| **S7** de-hardcode the test port | **KEEP** — justified, diffstat corrected (§2.7) |
| **S8** bound the encode-side cap park | **NEW** (§9) |
| **S9** disposition of the stale-token hazard | **NEW** (§8) — S1/S2 triple its reachability and S3 does not close it |

---

## 2. WHAT THIS SPEC REFUTES BY EXECUTION

**TWENTY-THREE refutations, NINE load-bearing.** Load-bearing ones are marked ⚠️.

### 2.1 ⚠️ "Three vendored guests already reach the leak" — TRUE OF SOURCE, FALSE OF EXECUTION, TWICE OVER

BRAINSTORM ledger item 2 and §1.3. The source claim is accurate as written (`a`/`b`/`c` Pause on any non-final chunk). The **inference** — that the leak is reachable in-tree — is false for the two independent reasons in §1.1: they take the `Continue` path on a single end-stream chunk, **and** they are never driven. This is the `reference_code_comment_not_evidence` species: the claim was verified at the source and never walked one layer up.

### 2.2 ⚠️ AND THE CONTROLLER'S OWN CORRECTION WAS ALSO WRONG

The controller predicted zero fires and attributed it to (a)/(b)/(c) taking the `Continue` path on a single end-stream chunk. **Correct conclusion, wrong mechanism** — the stronger and simpler fact is that they are not driven at all (`emitConstantSkipToken`). Recorded because a refutation is itself a claim and this one was half wrong; the SPEC's answer is right for a reason the SPEC did not supply.

### 2.3 ⚠️ `pause.go:19-21` IS FALSE, AND THE FREEZE IT JUSTIFIES IS UNJUSTIFIED

> *"differential fixture 0036 scenario (n) … depends on the decode-body arm pausing indefinitely by design so the host reaches body_buffer_cap_bytes. Those two files are frozen."*

(n) depends on that arm **never being reached**. The stated dependency is **inverted**, and it is the entire stated basis for freezing `body.go` and `trailers.go`.

### 2.4 ⚠️ BOTH STATED CONSTRAINTS ON `defaultPauseWatchdog` FALL — NOT JUST THE UPPER ONE

The BRAINSTORM records the UPPER bound as void. Measured, the **LOWER** bound is also unsound: `pause.go:59-62` claims `defaultHttpCallTimeout` (5 s) binds because it is *"the deadline on the outbound call a paused guest is normally waiting for"*. But `http_call.go:279-281` consults that default **only when the guest passes 0**, the one cited guest passes `Duration::from_secs(5)` **explicitly** (`l_httpcall_success/src/lib.rs:57`), and there is **no cap on a guest-supplied timeout**. It is not a bound.

### 2.5 ⚠️ THE `pause.go` MITIGATION IS FALSE, AND ITS DENOMINATOR IS ALSO WRONG

`pause.go:25-28`'s *"ZERO of the 35 guest crates … reference `resume_http_request` / `resume_http_response` / `proxy_continue_stream`"* — exactly ONE does, `l_httpcall_success/src/lib.rs:43`, added by `fb93845f`, **the phase-82 IMPL commit that wrote the claim.** Confirmed independently by two agents with `git log -S`.

⚠️ **AND THE DENOMINATOR IS THE WRONG SET.** Repo-wide there are **35 `Cargo.toml` / 35 `src/lib.rs` / 36 `.wasm`** — but that 35 is **25 fixture guests** (0034: 7, 0036: 14, 0038: 4) **plus 10 `test/conformance/proxy-wasm/sources/*` conformance crates**. The sentence reasons about fixture guests and counts conformance crates. *(An agent proposed correcting 35 to 25; that correction did **not** survive controller re-derivation — its glob required a `scripts/` path component and missed the ten conformance crates. **35 is right as a repo total; 25 is the relevant subset. Both figures stand; neither is a correction of the other.**)*

⚠️ **The sourceless blob is NOT in `0036`.** Within `0036` the sets are 14/14/14 with a byte-identical basename set. The 35-vs-36 gap is **`test/fixtures/0039-http-wasm-perroute-boot-reject/bytecode/probe.wasm`**.

### 2.6 ⚠️ THE CENSUS GUARD IS VACUOUS, AND SO IS THE ENTIRE PACKAGE ON THIS AXIS

`TestFilter_Pause_CensusOfHonoredArms` (`pause_test.go:120-144`) — docstring at `:126-127`: *"This is a behavioral assertion, not a grep: it drives the real dispatch and reads the returned status."* Body: three comparisons of package constants. **Proven by break, not by reading** — flipping `body.go:227` to `DataContinue`: `--- PASS … EXIT=0`. Flipping **all four** frozen arms simultaneously: `--- PASS`, and the **full package** `ok … 0.324s`.

⚠️ **NOTHING IN `internal/filter/http/wasm/` CATCHES ALL FOUR FROZEN PAUSE DISPOSITIONS BEING FLIPPED TO CONTINUE.** That is wider than BROKEN-GATE SHAPE 25 records.

⚠️ **AND THE TEST CONTRADICTS ITSELF INTERNALLY, INDEPENDENTLY OF THE DOCSTRING.** Its inline rationale at `:132-134` claims asserting the constants *"makes an accidental change to the returned disposition a compile-or-assert failure at this site."* That is false on its own terms: changing `body.go`'s arm to return `DataContinue` leaves both constants unequal and the test green. **Two false sentences, fifteen lines apart, in the same test.**

### 2.7 ⚠️ BROKEN-GATE SHAPE 26 — A LIVENESS BARRIER PLACED UPSTREAM OF THE GATE IT CLAIMS TO PROVE

`pause_test.go:73-76` and `:95-97` install the `executions` counter as the barrier against the cap-denied vacuity: *"a 0 means the capability gate short-circuited and NEITHER headers arm was reached."* But `decode_headers.go:199-201` increments `executions` **before** `CallProxyOnRequestHeaders`, and the capability gate lives **inside** that call. Measured under `newTestCompiledConfig` (zero capabilities, guest never dispatched): **`executions = 1`.**

⚠️ **THE BARRIER IS GREEN ON EXACTLY THE FAILURE IT EXISTS TO CATCH** — and it was installed at phase-82 Task 8 *as the fix* for the previously-found cap-denied dead-code vacuity (`dispatch_test.go:645-647`). **The replacement gate inherited the defect it replaced** (`reference_replacement_gate_inherits_defect`), one row later.

*(An agent stated the barrier "can never fire". Sharpened by the controller: it would read 0 if `eff.stats` were nil or an earlier path returned. It is **NON-DISCRIMINATING for its stated purpose**, which is worse than merely dead.)*

### 2.8 ⚠️ ADR-0106 DOES NOT CONTAIN THE SOLE-LEG RULE — IT IS **ADR-0106-AS-USED**

The lineage's standing citation — *"the SOLE leg per ADR-0106, which governs the sole-leg property"*, carried verbatim in ADR-0303's and ADR-0304's own STATUS lines — is an **as-used** citation, structurally identical to ADR-0044-as-used, and nobody has written that down. Token scan over the correctly-bounded block (`:4788-4857`, input measured **70 lines / 1302 words**): `sole` **0**, `leg` **0**, `six-gate` **0**, with NCs firing (`filter` 32, `family` 42, `ADR` 19). Its actual content is *"§9 HTTP filters family expansion shape — flat top-level rows + no-sibling-stub discipline; the §9 heading … is an umbrella, not a row."*

⚠️ **AND IT IS SCOPED TO §9 HTTP FILTERS**, so rows 74-81 (TLS stats, LB, stats sinks) cited it out of family — while **phase 83 is legitimately in scope**, WASM being a §9 HTTP filter. **Write ADR-0106-as-used.**

⚠️ **A CONTROLLER PROBE THAT WAS ITSELF BROKEN, RECORDED RATHER THAN HIDDEN.** The first extraction used `awk '/^## ADR-0106/,/^## ADR-0107/'`. `DECISIONS.md` headings are **not in numeric order** (`ADR-0107` at `:4304`, `ADR-0105` at `:4582`, `ADR-0106` at `:4788`), so the range ran to EOF and returned 13,071 lines with `sole` = 118. **The NC is what exposed it** — `filter` = 3776 is absurd for a 70-line block. **Print your denominators.**

### 2.9 ⚠️ ADR-0044 DOES NOT CONTAIN THE §CONTEXT-AT-SPEC DISCIPLINE — RE-VERIFIED, NOT INHERITED

Mechanical token scan over `:1419-1462` (input measured 44 lines / 458 words): `IMPL` **0**, `append`/`APPEND` **0/0**, `in place`/`IN PLACE` **0/0**, `§Context` **0**, `§Decision` **0**, `drafted` **0**, `stage`/`lifecycle` **0/0**. The single `SPEC` hit is *"**Settles:** SPEC ADR-K, phase-04 §4.1"*, unrelated. NCs known-present: `HTTP/1.1` 6, `allow-list` 5. **Write ADR-0044-as-used.**

### 2.10 ⚠️ ADR-0304's OWN STATUS IS WRONG AT THE COMMIT THAT WROTE IT

It asserts *"302 headings spanning ids 0001-0303"*. Measured on `71fc86d7:docs/envoy-go/DECISIONS.md`: **303 headings, ids 0001-0304** — **the block's own heading made the sentence false as it landed.** The gap at ADR-0209 is real and correctly recorded, and is exactly why `next-free = headings + 1` yields ADR-0304 and **COLLIDES**. ⚠️ **DO NOT "FIX" THE 303-vs-0304 ARITHMETIC.**

### 2.11 ⚠️ THE `ROADMAP.md:<line>` CITE COUNT SELF-FALSIFIED FOR THE **FOURTH** TIME — INSIDE THE COMMIT THAT NAMED THE SPECIES

Measured across three tips: **117** at `8a0126d2`, **118** at `1a89031b`, **120** at `5d0892e4`. The last delta is located to two more `ROADMAP.md:195` occurrences in `next-prompt.txt`, added by the commit whose subject is *"correct two counts this router's own landing invalidated, and record the self-falsifying-count species for the THIRD time."* **The correction to 118 was invalidated by its own landing.**

⚠️ **THE LESSON IS NOT "RECOUNT HARDER".** This SPEC therefore carries **NO live whole-repo count of any pattern it itself spells** — see §12.3 for the exclusion list. *(`BEHAVIOR_CONTRACT.md:<line>` = 196 held across both prior tips, but is the same class and is excluded on the same grounds.)*

### 2.12 Also refuted — twelve more, each with a cite

- **`trailers.go`'s Pause arms had NO test coverage of any kind** before this stage's probes. All five tests in `trailers_test.go` (`:50 :78 :95 :117 :140`) exercise only the `nil streamCtx` / `nil cfg` pass-through.
- ⚠️ **`CallProxyOnRequestTrailers` takes TWO args, not three** (`stream_context.go:227-231`, `(streamCtxID, numTrailers)` ⇒ `(i32,i32)->i32`), so the BRAINSTORM's *"`proxy_on_request_body` has the same signature as `proxy_on_request_headers`, so no new blob is needed"* **does not extend to S2**. The no-new-Rust-blob conclusion still holds; a **new type-section entry** does not. The body claim itself is confirmed (`stream_context.go:180-184` vs `:205-209`, both three `uint64`).
- ⚠️ **`testLifecycleCapabilities` (`dispatch_test.go:113-125`) contains NEITHER trailers capability.** Any S2 test using `newTestCompiledConfigWithCaps` without `extraCaps` is **cap-denied and silently vacuous** — the exact failure mode the comment eight lines above it warns about.
- **`ContinueStream` discards `resumeDecode`'s boolean and returns `WasmResultOk` unconditionally** (`abi_callbacks.go:887-904`, deliberate per `:880-882`). The guest has **no** second channel: it is not re-entered after a Pause, and no property or stat is exposed. **`proxy_continue_stream` is indistinguishable between "redundant resume" and "resume dropped on the floor".**
- **The watchdog log string misattributes the callback.** `pause.go:103-104` hard-codes *"the guest returned PAUSE from proxy_on_request_headers"*; captured verbatim for a **body** pause with S1 in place. Operator-facing; must be parameterized.
- **`hcm/connection.go:565` and `hcm/h2dispatch.go:503`** both say *"the FilterChain does not yet expose a RunDecodeTrailers method (Task 18 will add it)"* — it has existed since Task 19 at `chain.go:455`.
- **`pause_test.go:435` and `:436` are BOTH `RunDecodeTrailers`**, not one of each, and they drive a synthetic `parkingProbeFilter`, not the wasm filter.
- **`internal/wasm/doc.go:219` carries TWO errors, not one** — *"ABICallbacks 13→20 methods"* against an AST-counted **21**, **and** it says the interface lives in `abi_callbacks.go` when it is declared at `internal/wasm/registration.go:130`.
- **`*RootVM.dispatchHttpCallResponse` does not exist** (real symbol `(*RootVM).handleHttpCallResponse`, `internal/wasm/http_call.go:365`). The carrier enumeration is *classified*, not counted: three landed `.go` code comments (`abi_callbacks.go:1101`, `stream_context.go:249`, `:254`) plus two phase-25.2 documents — **plus the phase-83 BRAINSTORM and router, which is why the raw occurrence count must never be carried.**
- **`ROADMAP.md`'s `github.com/esalaine/envoy-go/…` path occurs FIVE times, not once**, against `go.mod:1`'s `github.com/pgdad/envoy-go`; root cause is ADR-0006 at `DECISIONS.md:142`. **`esalaine` occurs in ZERO `.go` files** — purely documentary.
- **`ls -d test/fixtures/[0-9]*/` and the faithful `^[0-9]{4}[a-z]?-` predicate BOTH give 120.** The divergence is faithful **120** vs bare `^[0-9]{4}-` **118** (which drops `0007a-cors` and `0007b-iteration-probe`).
- **"a numeric-id regex gives 113" is regex-dependent and 113 is not the only wrong answer** — `^\| *[0-9]+(\.[0-9]+)? *\|` gives **113**; the plainest reading `^\| *[0-9]+ *\|` gives **84**. **State which regex.**
- **`STATE_HISTORY.md`'s "174" is the `prior active-phase` count; the total bullet-anchored entry count is 175** (one head `- **active-phase:**` at `:23`). **Say which denominator.**

---

## 3. D-83-CAP — Q1 DISPOSED BY MEASUREMENT: **ZERO RISK. THE ARM IS NEVER REACHED.**

> *Does arming a 10 s watchdog on the `0036` (n) body-cap arm change that scenario's timing?*

**NO — and not because the timing is tolerable, but because the arm is never reached.** §1.1.

| run | subtest | wall |
|---|---|---|
| baseline 1 | `--- PASS … (20.36s)` | 22.47 s |
| baseline 2 | `--- PASS … (19.33s)` | 20.14 s |
| **S1 applied** | `--- PASS … (19.25s)` | 20.14 s |

Run-to-run variance ≈ **1.03 s**; the delta is **inside** it. **Zero watchdog fires.** `[no tests to run]` grep: **0 hits** — the subtest genuinely ran (`reference_differential_run_selector`).

**Watchdog sweep, S1 active, on the real `0036`:** 10 s / 1 s / 250 ms / 50 ms / **1 ms** — **PASS at every value**, no `watchdog fired` line at any of them. **A 10,000× sweep changes nothing.**

⚠️ **AND THE DIFFERENTIAL THEREFORE CANNOT DETECT A WRONG WATCHDOG VALUE AT ALL.** That is a gate-coverage finding, not a convenience: `0036` is the **only** fixture in the repository with a wasm body callback (`git grep -ln 'on_http_request_body\|on_http_response_body' -- 'test/fixtures/**/*.rs'` ⇒ `0036` alone).

**Disposition:** the row's differential posture is **INVARIANT**, and the IMPL must assert that invariance rather than assume it. **The `0036` (n) re-run the BRAINSTORM owes is still owed — as a confirmation that nothing moved, not as a risk check.**

⚠️ **AN AGENT'S RESIDUAL CONCERN, CLOSED BY ANOTHER AGENT'S MEASUREMENT.** A3 flagged the `0036` S1 cost as *"the single largest unquantified S1 risk"*, reasoning that a sub-cap chunk reaching the Pause arm would burn 10 of the driver's 15 s. **A1 measured it: the arm is not reached, the delta is zero.** The concern was correct as a *hypothesis* and is *closed by measurement*. What survives from it is not a fixture-timing cost but the **code-correctness** defect of §1.2 — the two agents converged on multi-chunk delivery from opposite directions.

---

## 4. D-83-TRAILERS — Q2 DISPOSED: **FLAG AND ARM (Option A). THE ALTERNATIVE FAILS ON ITS OWN TERMS.**

> *Do the two trailers arms need the flag, or a documented no-op, given the hooks have no production caller?*

**Option B (documented no-op) is not available, because the current behavior is not a no-op — it is a permanent, unresumable park.** A trailers Pause returns `TrailersStopIteration`, `parkDecode` blocks, and the guest's own `proxy_continue_stream` **cannot free it**: `resumeDecode`'s CAS loses because `decodePaused` was never set. The only escape is ctx cancellation, and **there is no stream idle timeout anywhere in the tree** — `git grep -i idle_timeout -- 'internal/**/*.go'` returns only two *comments* (`buffer/doc.go:9`, `listener/manager.go:461`), with a firing control on the same file set. **A trailers park is bounded by nothing at all** — strictly worse than the headers arms were before phase 82, which at least discarded the Pause.

**Measured, RED then GREEN.** Adding one line — `f.beginDecodePause()` — at `trailers.go:142`:

```
--- FAIL: …TrailersPause_GuestResumeCannotRelease (0.80s)   [base]
    MEASURED HAZARD: the guest's proxy_continue_stream did NOT release a TRAILERS-side park
--- PASS: …TrailersPause_GuestResumeCannotRelease (0.30s)   [+ beginDecodePause()]
    resumeDecode WON: guest pause lasted 300.401215ms
```

**A failing-first anchor EXISTS despite zero production callers** — `FilterChain.RunDecodeTrailers` is exported and directly drivable from the wasm package's own tests. **Option A is testable; Option B is a comment and is not.**

**Two further options were considered and rejected:**
- **C — return `TrailersContinue` on Pause.** Silently discards a guest's Pause: precisely the defect phase 82 rewrote `decode_headers.go` to fix. Re-introducing it in two arms while the same commit removes it from two others is incoherent, and it has **no failing-first anchor**.
- **D — hard error / `envoyGoFailures`++.** Defensible only if the seam were being deleted. It is not: `RunDecodeTrailers` is live and `envoygotest/filter_test.go:332` already drives it.

### 4.1 ⚠️ A guest that Pauses from a trailers callback is BLIND, and that must be recorded rather than fixed here

`abi_callbacks.go:223-225` routes map types 1/3/4/5 to `default: return nil, false, false`. Measured: `GetHeaderMap(type=0) → ok=true`; `type=1 → ok=false`; `type=3 → ok=false`. **A guest that Pauses from `on_http_request_trailers` cannot read a single trailer.** S2 makes the park **bounded**; it does not make it **useful**. Out of scope, named in §14.

### 4.2 ⚠️ Two S2 preconditions the BRAINSTORM does not name

1. **A NEW type-section entry** — `CallProxyOnRequestTrailers` is `(i32,i32)->i32`, not the body/headers `(i32,i32,i32)->i32`. The existing `fixTypeSection` type 1 does **not** cover it. *(Still no Rust blob and no toolchain dependency; the pinned rustup 1.94.0 stays off this row's critical path.)*
2. **Two capabilities must be added to `testLifecycleCapabilities`** (`dispatch_test.go:113-125`), which today has both **body** capabilities and **neither** trailers capability. Without them every S2 test is cap-denied and **silently green**.

---

## 5. D-83-WATCHDOG — Q3 DISPOSED: **KEEP 10 s. REWRITE THE DERIVATION TO SAY WHAT IS TRUE.**

> *Should `defaultPauseWatchdog` stay 10 s, and on what surviving evidence?*

**Nothing pins it.** Both stated constraints fall (§2.4 lower, §1.4-of-the-BRAINSTORM upper). Measured:

- **The entire in-tree corpus arms the watchdog EXACTLY ONCE** — scenario (l), `stream=1` — and that pause lasts **747 µs**, released **by the guest**, not by the watchdog. The comment's predicted *"~10 s of added subject-side runtime on that one fixture"* measures **0 s**.
- **At a 1 µs watchdog the host force-resumes 448 µs before the http-call response arrives, and `0036` still PASSES** — the upstream round-trip is slower than the cluster_b call, so `call_status` is already 200 by the time `on_http_response_headers` runs.
- **No test asserts the constant's magnitude** (`git grep defaultPauseWatchdog -- '*.go'` ⇒ zero such assertions) and `f.pauseWatchdog` is written **only** from tests. **There is no xDS or config surface to override it.**

**The defensible interval, with both bounds cited:**

- **LOWER — no code-derived bound exists.** The longest legitimate in-tree pause is **747 µs**; no fixture detects a violation even at 1 µs. The design floor would be a guest's own `dispatch_http_call` timeout, which is **unbounded**. 5 s is a plausible convention, **not a derivation**.
- **UPPER — the differential's per-fixture budget of 90 s** (`test/differential/runner_test.go:237`), which is far tighter and more relevant than the 15 s client timeout the comment cites. Above that: `-timeout 20m` and `timeout-minutes: 30` (`.github/workflows/ci.yml:60,63`). Envoy-go has **no stream idle timeout**, so the watchdog is genuinely the sole bound on a never-resuming guest.

**Disposition: 10 s stays.** The IMPL must replace the two-MEASURED-CONSTRAINTS paragraph with the honest statement — *any value in roughly [1 s, 90 s) is indistinguishable to the current corpus; 10 s comfortably exceeds the 5 s `defaultHttpCallTimeout` convention and stays well inside the differential's 90 s per-fixture budget.* **Presenting it as "pinned between two MEASURED constraints" is the failure mode being removed, not the number.**

---

## 6. D-83-LOGHOME — Q4 DISPOSED: **A §CONTEXT PARAGRAPH ON ADR-0305. NOT AN ADR AMENDMENT, NOT A ROW.**

> *Where does the trap-arm logging-surface convention question get answered?*

**ADR-0305 §Context, as a named non-decision.** Three reasons, each mechanical:

1. **It is not an implementation question.** `internal/wasm` has **0** `log.Printf` and **0** `"log"` imports across 29 non-test files; the sibling `internal/filter/http/wasm` is the firing NC. All three priced shapes either create the logging surface or duplicate the ambiguity, and **none has a clean failing-first anchor** — which is disqualifying under this row's own standard (§4).
2. **An ADR amendment is not available.** `DECISIONS.md` is append-only per ADR-0288 §Decision 4 (BOOTSTRAP invariants 2 and 4). A convention question cannot be retro-fitted into a landed ADR; it can only be recorded in a new one.
3. **The stated blocker is weaker than recorded and that matters for whoever picks it up.** 31 non-test `.go` files repo-wide import `"log"`; `.golangci.yml` enables **9** linters (`govet errcheck staticcheck unused ineffassign gofmt goimports misspell revive`, `disable-all: true`) and **neither `depguard` nor `forbidigo`** is among them; and the `var logf = log.Printf` idiom already exists in two packages. **There is no mechanical enforcement to violate.**

**Disposition:** ADR-0305 §Context records the question, the three priced shapes, and the fact that none has an anchor. **It is explicitly NOT chartered by this row** (§14).

---

## 7. D-83-TIMER — A **NEW** FORK. DISPOSED: **`Stop()` + A GENERATION COUNTER, AND THE ORDERING IS NOT NEGOTIABLE**

The BRAINSTORM's S3 is `Stop()` before reassign. **Refuted by the 299-vs-1 A/B in §1.2.**

**The disposition:**

```
gen := f.decodePauseGen.Add(1)          // FIRST — before Store(true)
f.decodePaused.Store(true)
...
t := time.AfterFunc(f.watchdogTimeout(), func() {
        if f.decodePauseGen.Load() != gen { return }   // FIRST statement, AHEAD of the CAS
        if !f.decodePaused.CompareAndSwap(true, false) { return }
        ...
})
old := swap(&f.decodePauseTimer, t)      // under pauseMu
if old != nil { old.Stop() }             // return value IGNORED — it is not a signal
```

**Both parts are load-bearing, for different reasons:**
- **The generation guard is CORRECTNESS.** Incrementing **before** `Store(true)` means a timer firing after the increment is already superseded, while a timer firing before it correctly resumes its own still-current pause. Placing the check **ahead of the CAS** is what prevents a superseded timer from consuming the new pause's flag.
- **`Stop()` is RESOURCE HYGIENE, not correctness.** It releases the superseded closure — and through it the chain's filter callbacks — immediately rather than at the end of the window. ⚠️ **Its return value must be ignored;** `pause.go:169-171` already documents that discipline for `stopPauseTimer` and the same reasoning applies.

⚠️ **THE GATE CANNOT BE `-race`.** Zero reports with the prototype loaded. It must be a **behavioral fire-count assertion** of the 300-generation shape, with the single-pause NC asserting exactly 1.

⚠️ **AND S1 MUST ALSO DISARM ON THE `Continue` PATH** (§1.2) — a guest that Pauses on chunks 1..n-1 and Continues on chunk n leaves the flag set and the timer armed. Without this, the generation guard alone still leaves one live orphan per stream.

---

## 8. D-83-STALETOKEN — A **SECOND** NEW FORK. DISPOSED: **NAMED AND BOUNDED HERE; FIXED ONLY IF THE PLAN PRICES IT UNDER THE BAND**

Distinct from §7 and **not closed by it.** When a watchdog fires on an earlier pause in a stream, its `ContinueDecoding` latches a token on the chain-scoped **buffered-1** `decodeResumeCh`. A later pause in the same stream then parks and **immediately spends that stale token**. Measured: a body park with a 10 s window released after **75.4 µs**, with `decodePaused` still **true** afterwards — proving nothing consumed the body pause's own flag.

⚠️ **S1/S2 MULTIPLY ITS REACHABILITY FROM 2 PAUSE SITES PER STREAM TO 6.** The hazard is pre-existing and is the one `pause.go:140-145` already documents for the cross-filter case; this row does not create it but does widen it threefold.

**Disposition:** the fix belongs at the **chain** layer (a token is chain-scoped; a filter cannot safely reason about it), which is outside this row's file set and would change park semantics for **every** parking filter — explicitly the thing `pause.go:43-47` argues against. **The SPEC names it, the ADR records it, and the PLAN must either price a filter-local mitigation or defer it BY NAME.** It must not be discovered at the IMPL.

⚠️ **AN AGENT'S FIRST "Stop IS INSUFFICIENT" PROBE ACTUALLY HIT THIS, NOT §7** — it self-disambiguated and reported the correction. Had it stopped there, the SPEC would have shipped a wrong cite. **The §7 sufficiency claim rests SOLELY on the controlled 299-vs-1 fire-count A/B, not on that timing sweep.**

---

## 9. D-83-ENCODECAP — A **THIRD** NEW FORK. DISPOSED: **IN SCOPE (S8), BECAUSE THE ROW EDITS THE FILE AND THE COMMENT IS FALSE**

§1.3. `body.go:282` returns `DataStopIterationNoBuffer` with no local reply; the encode chain parks; nothing resumes it.

**Why in scope rather than deferred:** the row already rewrites `body.go`'s Pause arms and its comment surface, the false comment sits **four lines above** an arm S1 edits, and leaving an unbounded park in the file whose leak this row exists to close would be the exact shape of phase 82's own deferral — *"the four landed siblings … stay that way"* — which is what this row is here to undo. **Fixing latent-but-adjacent defects in the row that fixes the live ones is cheaper and more honest than leaving a tripwire.**

⚠️ **The remedy is NOT "arm the watchdog there".** This is a **host-initiated** park, not a guest Pause: no guest owes a resume, so `resumeDecode`'s CAS model does not apply. The PLAN must choose between returning a non-parking status and arming a host-owned bound, and **must state which, with the reference's behavior cited.**

---

## 10. BLAST RADIUS

**Production files touched:** `internal/filter/http/wasm/{body.go, trailers.go, pause.go, wasm.go}` — 4 files, all within one package. `decode_headers.go` and `encode_headers.go` gain comment-only edits (§6 of the S5 roster).

**Not touched:** `internal/filter/http/chain.go` (§8's fix would live there and is deferred), `internal/filter/hcm/*`, `internal/filter/http/router/*` (their false trailers-seam comments are recorded out-of-scope, §14).

**Stat surface: anticipated +0.** No `NewCounter`/`NewGauge` call site moves. **New fixtures: 0. New BackendKind: 0. New port: 0. go.mod: +0. New Rust blob: 0. Toolchain: none** — the pinned rustup 1.94.0 stays off the critical path.

**Cross-package risk: NONE measured.** With the S1+S2+S3 prototype in place: `go build ./...` OK · `go vet ./internal/filter/http/wasm/` OK · `gofmt -l` empty (gated on **output**, not exit code) · `golangci-lint run` exit 0, 0 lines · `go test ./internal/filter/http/wasm/ -count=1` **445 PASS / 0 FAIL** · `-race` `ok 1.380s` · `./internal/filter/http/`, `./internal/wasm/...`, `./test/conformance/proxy-wasm/` all ok.

⚠️ **`golangci-lint` FIRED `misspell` on a British spelling in a prototype comment** — `reference_golangci_misspell_locale_us` confirmed live at this tip, locale **US**.

---

## 11. DIFFERENTIAL AND FIXTURE POSTURE — **ZERO NEW FIXTURES**, AND THE DIFFERENTIAL IS **STRUCTURALLY BLIND** TO THIS ROW

**Zero new fixtures**, for a stronger reason than convenience: §3 measured that `0036` is invariant under the change, and `0036` is the **only** fixture with a wasm body callback at all.

⚠️ **THE ROW'S ENTIRE GATE BURDEN THEREFORE FALLS ON UNIT TESTS.** The differential cannot go RED on any of S1/S2/S3/S8, and a green `0036` is **not** evidence the row works — it is evidence the row broke nothing. **The IMPL must not present a differential green as coverage** (`reference_liveness_break_needs_failing_baseline`).

**What the IMPL owes on the differential:** a confirming `0036` re-run showing the arm still unreached and the wall time inside the measured 19.33-20.36 s envelope, plus the full 120-fixture suite. Baselines are **inherited from the phase-82 IMPL and are NOT claimed by this docs-only stage**: 120/120 in 388.961 s, `0036` alone 18.87 s, h2spec 53/53/0/0 (⚠️ **one report, no reference arm — state your own denominator**).

⚠️ **Budget ~3 differential launches** (`reference_driver_receiver_port_race_aborts_binary`), and ⚠️ **capture `INNER_EXIT` and the panic arm — the run ABORTS, it does not merely fail** (`reference_harness_exit_code_is_not_command_exit_code`).

---

## 12. CONTRACT AND ADR EDITS (owed at the IMPL, specified here)

### 12.1 ADR-0305 — §Context drafted at this SPEC

**Next-free is `ADR-0305`**, re-derived: `DECISIONS.md` **17858** lines, **303** `^## ADR-` headings, ids **0001-0304** with **exactly one gap at ADR-0209**, tail **ADR-0304 STATUS COMPLETE**. ⚠️ **`next-free = headings + 1` gives ADR-0304 and COLLIDES — the arithmetic is consistent BECAUSE of the gap. Do not "fix" it.**

**Block form, re-derived from ADR-0304 and the ADR-0299 PROPOSED exemplar:** `## ADR-NNNN — <em-dash title>` (the `## ADR-0044:` colon form is pre-0289 lineage) · one blank · a **single-line** `> **STATUS: PROPOSED — …**` blockquote · one blank · `### Context (drafted at the phase-83 SPEC)` · numbered `**§Context ¶N — …**` paragraphs · the **RETAINED** italic footer `*(§Decision + §Consequences land at the phase-83 IMPL.)*` as the block's last line. ⚠️ **NO `---` SEPARATOR** — the file's last `^---$` is at `:17020` trailing ADR-0288, so ADR-0289..0304 (sixteen blocks) carry none, and `^---$` **STAYS 216**.

**Length lineage:** ADR-0302 13 ¶ / ADR-0303 13 ¶ / ADR-0304 10 ¶. **Target 10-13 numbered paragraphs.**

⚠️ **THE RECURRENCE GUARD IS RE-ARMED BY THIS SPEC.** `^> \*\*STATUS: PROPOSED` goes **0 -> 1**; the IMPL disarms it back to 0. ⚠️ **DO NOT USE THE LOOSE FORM `^> \*\*STATUS: .*PROPOSED`** — measured at this tip it returns **4**, and **all four are FALSE POSITIVES**: COMPLETE blocks at `:17466`, `:17534`, `:17600`, `:17662` that merely narrate the word. Retained italic footers go **10 -> 11**; STATUS census **17 -> 18**.

### 12.2 `BEHAVIOR_CONTRACT.md` — **BYTE-UNTOUCHED at this stage**

Stat surface +0, so no ledger entry is owed. ⚠️ Max cited line anywhere is **5078** against a **5900**-line file, so a tail append would shift zero cites if the IMPL needs one.

### 12.3 ⚠️ COUNTS THIS SPEC AND ADR-0305 DELIBERATELY DO **NOT** CARRY

Per §2.11, no live whole-repo count of a pattern the document itself spells: **the `ROADMAP.md:<line>` cite count** (self-falsified four times), **the `BEHAVIOR_CONTRACT.md:<line>` cite count** (same class), **the `dispatchHttpCallResponse` carrier count** (this SPEC spells the wrong symbol in order to record the defect — the enumeration is *classified* instead), **the `allCallbacksNoOp` occurrence count**, and **the `WASM-family row` occurrence count** (stable only until a row 84 names the family).

---

## 13. COST AND SPLIT — ⚠️ **THE BAND IS REVISED UP ~2.3×, AND THE BRAINSTORM'S LOWER-BOUND DECLARATION IS VINDICATED IN THE SAME STAGE THAT MADE IT**

**Production, MEASURED** (S1+S2+S3-with-generation-guard prototype, `git diff --numstat`): `body.go` +2 · `pause.go` +16 · `trailers.go` +2 · `wasm.go` +8 = **28 insertions, 0 deletions.**

⚠️ **28 IS NOT THE PRODUCTION COST.** `pause.go:3-29` is a 27-line doc block that S1+S2 **falsify outright** (*"the four landed siblings carry ZERO paused-state bookkeeping, and they stay that way… Those two files are frozen."*), and the two misattributing log strings must be parameterized. With S5's eleven sites and S8: **production ≈ 63-90 net.**

**Test side, grounded in MEASURED analogues** rather than modelled — this is the third consecutive row at which test scaffolding dominates, so the roster is priced against real sibling sizes (`buildPauseProxyWasm` 24 · `TestFilter_DecodeHeaders_Pause_StopsIteration` 72 · `…WatchdogUnparksChain` 64 · `TestAbiCallbacks_ContinueStream_SixArms` 124):

| group | items | lines |
|---|---|---|
| S1 | 2 body fixtures, 2 flag+arm tests, 2 chain watchdog regressions, 1 `ContinueStream` body-resume | ≈ 371 |
| S2 | 2 trailers fixtures (+ new type section), 2 caps, 2 flag+arm tests, 2 chain regressions | ≈ 333 |
| S3 | 300-generation fire-count + NC, orphan-timer chain regression, `OnDestroy` extension | ≈ 195 |
| S4 | behavioral census replacement over all six arms | **113 MEASURED** |
| S8 | encode-cap bound + its RED arm | ≈ 60 |
| | **TEST TOTAL** | **≈ 1070** |

**GRAND TOTAL ≈ 1135-1160 net `.go`** against the chartered band 350-600 / budget ~450 — **≈ 2.4-2.6× budget**, squarely inside the lineage's two prior `reference_measured_prototype_is_a_lower_bound` firings (**3.07×** phase 81, **2.55×** phase 82), with test scaffolding at **≈ 93%** for the third consecutive row.

**REVISED BAND: 950-1400 net `.go`, budget ~1150, 12-16 tasks.**

### ⚠️ THE SPEC'S CONCLUSION ON SPLITTING — **NO SPLIT**, AND THE §6.1 ARITHMETIC IS STATED

`BOOTSTRAP_PROMPT.md` §6.1 (`:285`; triggers `:289` ~25 tasks and `:290` ~1500 LoC, `:291` BLANK, third trigger `:292`) — re-verified exact at this tip.

- **Task trigger:** 12-16 against ~25 — **does not fire.**
- **LoC trigger:** ~1150 budget against ~1500 — **does not fire at budget**, but the upper band edge (1400) is within 7% of it, and **a further 1.3× overrun crosses it.**
- **Third trigger (`:292`, sub-steps > ~10 mid-execution):** the ordering constraint in §7 (generation increment before `Store`, guard before CAS) is a **correctness** constraint, not a decomposition one, and does not blow up any single task.

⚠️ **THE PLAN OWES AN EXPLICIT §6.1 RE-EVALUATION AGAINST A MEASURED TEST SIDE** — this SPEC's test figure is grounded in analogue sizes but **not written**, which is exactly the failure mode of phases 81 and 82. ⚠️ **AND THE PRECEDENT IF IT CROSSES IS RECORD, DO NOT RETRO-SPLIT** (both prior crossings).

⚠️ **A CROSS-CHECK ON THIS SPEC'S OWN ESTIMATE, RECORDED SO IT CAN BE FALSIFIED:** the stage's probe files total **478 lines** covering roughly 6 of the 17 roster items ⇒ ~80 lines/item ⇒ 17 × 80 ≈ **1360**. **The 1135-1160 figure is therefore conservative, and 1400 is the honest band ceiling rather than a pessimistic one.**

---

## 14. WHAT THIS ROW NAMES BUT DOES NOT FIX

- ⚠️ **The stale-token hazard (§8)** — deferred **BY NAME**; the fix is chain-layer and would change park semantics for every parking filter.
- ⚠️ **A trailers-paused guest is BLIND (§4.1)** — map types 1/3 are dead at `abi_callbacks.go:223-225`. S2 bounds the park; it does not make the callback useful.
- ⚠️ **`ContinueStream` cannot report a lost resume** — returns `WasmResultOk` unconditionally by design. Changing it is an ABI-visible decision and needs its own row.
- **The four false non-test trailers-seam comments** — `hcm/connection.go:565`, `hcm/h2dispatch.go:503`, `router/router.go:284-285`, and the softer fifth at `connection.go:300`. **OUT of S5's scope**: they sit in packages this row does not otherwise touch, and `router.go:284-285` is not merely stale prose — it asserts a call that never happens, so "fixing" the comment would paper over the fact that `Run*Trailers` is dead production code. **Recorded with cites; they deserve their own row.**
- **The wasm http-call trap-arm logging convention (§6)** — routed to ADR-0305 §Context, explicitly not chartered.
- **`internal/wasm/doc.go:219`'s two errors** and **`root_abi_callbacks.go:43`'s nonexistent `allCallbacksNoOp`** — recorded, not fixed.
- **`ROADMAP.md`'s five `github.com/esalaine/…` cites** — INVARIANT-BLOCKED (append-only), recorded not fixed.
- **`ADR-0106-as-used` and `ADR-0044-as-used`** — the two standing misattributions are now written down (§2.8, §2.9) rather than propagated silently.

---

## 15. HAZARDS CARRIED INTO THE PLAN

1. ⚠️ **The failing-first anchor is SYNTHETIC and must be proven RED before the fix** — no fixture and no vendored guest can produce one (§1.1). `reference_liveness_break_needs_failing_baseline`: a green can also mean "did not run".
2. ⚠️ **`testLifecycleCapabilities` lacks BOTH trailers capabilities** — every S2 test without `extraCaps` is silently vacuous (§4.2).
3. ⚠️ **The `executions` liveness barrier is non-discriminating** (§2.7). **Do not copy it.** Use `f.streamCtx.HasGlobalFunc(...)`, which is cap-gated at `internal/wasm/root_vm.go:906-916` and was confirmed discriminating.
4. ⚠️ **`-race` is not the gate for §7** — it reports nothing. Use a fire-count assertion.
5. ⚠️ **Plugin `config.name` must be process-unique** — `-count=N>1` on a test building a compiled config fails with *"…is duplicated across PluginConfig entries"*. **And the name feeds a stat scope** guarded by `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`, so `t.Run` names containing `-` or non-ASCII are rejected at config build (`reference_dynamic_stat_name_charset_guard`, fired live).
6. ⚠️ **Port 42552 collides across parallel agents, and it fired at this stage** — one agent's package run failed with `bind: address already in use` because a sibling held it. That is the S7 justification, observed rather than argued. ⚠️ **The TIME-WAIT half of the original justification does NOT reproduce** (three consecutive `-count=1` runs passed with TIME-WAIT accumulating 1→2→3; `net.Listen` sets `SO_REUSEADDR`). **Only a live listener collides.** ⚠️ **S7's measured diffstat is 9 ins / 26 del, net −17** (minimal variant) or 20/30/net −10 (documented variant) — **the BRAINSTORM's 9/23/net −14 is wrong and UNDERSTATES the reduction. Pick one variant and state it.**
7. ⚠️ **Do NOT copy `freeTCPPort`** — all three live definitions are pick-close-rebind, which exists only because a **subprocess** binds. This test binds **in-process**: `net.Listen("tcp","127.0.0.1:0")` + `ln.Addr().(*net.TCPAddr).Port` has no race window at all.
8. ⚠️ **The S4 replacement must be re-inverted, not merely kept.** It asserts `decodePaused`/`encodePaused` stay **false** on the frozen arms — the current contract. **S1/S2 invert that contract**, so the replacement's own assertions must flip in the same commit or the row ships a self-contradicting gate.
9. ⚠️ **The S4 replacement does NOT cover** dispatch ordering/count, the park itself (it calls filter methods directly, not through `FilterChain`), or watchdog arming. **Chain-level park assertions are a separate item and are priced separately in §13.**
10. ⚠️ **`gofmt -l` NEVER exits non-zero — gate on OUTPUT.** ⚠️ **An empty lint result is not evidence the linter looked** — inject a British spelling, confirm `misspell` fires, restore byte-identically.

---

## 16. HYGIENE

Five investigation agents, each in its **own DETACHED worktree** off `5d0892e4` with **private scratch** and a **private port band** inside `43400-43799` (clear of `20000-31007`, `11000-14999`, `10000-10447`, `15000-15011`, `18000-18007`, `19000`, `19999` and `42552`). All five reported `git status --porcelain` = **0 lines** and removed their own worktrees; every probe file, prototype and squatter was reverted. **No agent created a named container**; the reference Envoy containers are testcontainers-managed and self-terminated in every log. **No `ancestor=`/image filter and no `prune` was used at any point** (`reference_parallel_agents_shared_machine_namespaces`). **Nothing was committed by any agent and nothing was pushed.**

⚠️ **THE BASH CWD RESET FIRED, OBSERVED, SIX TIMES AT THE CONTROLLER** (`Shell cwd was reset to /home/esa/git/envoy-go`). Every git command used `git -C <abs-worktree-path>` (`reference_bash_cwd_reset_commits_to_main`).

⚠️ **ONE CONTROLLER PROBE WAS BROKEN AND IS RECORDED RATHER THAN HIDDEN** — the ADR-0106 range extraction (§2.8). Its own NC exposed it.

---

## 17. NEXT

**PLAN** for phase 83, per the one-stage-per-session discipline. It owes:

1. **An explicit §6.1 re-evaluation against a MEASURED test side** — not an analogue-scaled one (§13). The precedent if it crosses is **RECORD, DO NOT RETRO-SPLIT**.
2. **A task ordering that lands S3's generation guard BEFORE or WITH S1/S2** — S1/S2 without it widen a live defect threefold (§1.2, §7).
3. **A disposition for §8's stale-token hazard** — a priced filter-local mitigation, or a deferral **BY NAME**. It must not surface at the IMPL.
4. **A remedy choice for S8** — non-parking status vs host-owned bound, with the reference's behavior cited (§9).
5. **A break roster whose arms are proven RED first**, given the differential is structurally blind to this row (§11) and the package's existing guard is vacuous (§2.6).
