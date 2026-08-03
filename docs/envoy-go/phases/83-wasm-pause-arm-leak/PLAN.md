# PLAN 83 — wasm-pause-arm-leak: **the §6.1 LoC trigger is CROSSED ON MEASUREMENT — ~1840-2080 net against ~1500 — and it crossed a band this SPEC had ALREADY revised up 2.4× in anticipation of exactly this**; S3 has **no live defect and therefore no RED anchor of its own**; the stale-token hazard has a **6-line filter-local fix that the SPEC's own §7 snippet omits**; and the SPEC's headline A/B is **PACER-DEPENDENT**, so its prescribed gate is not reliably discriminating

**Stage:** PLAN (lifecycle-state `2` -> `3`). **ROW 83 STAYS `in-progress`**; `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` are **BYTE-UNTOUCHED** and the sentinel `want` **STAYS 115**. Base master **`e4563a49`**, taken from `git rev-parse master` at session start and **not** from any SHA quoted in the router. Branch `phase-83-plan`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

**ADR-0305 is already drafted, STATUS `PROPOSED`, §Context-only. A PLAN adds no ADR and does not touch `DECISIONS.md`.** The strict recurrence guard `^> \*\*STATUS: PROPOSED` is **1** at this tip and **stays 1** through this stage; the IMPL disarms it.

---

## What was EXECUTED at this stage

**FIVE investigation agents**, each in its own **DETACHED** worktree off `e4563a49`, private scratch, private port bands inside `43800-44299`. Every agent **wrote real code and ran it** — this stage's whole purpose was to replace the SPEC's analogue-scaled test figure with a measured one, because that is the failure mode that made phases 81 and 82 overrun by 3.07× and 2.55×.

All five reported `git status --porcelain` = **0 lines** and removed their own worktrees. Nothing was committed by any agent; nothing was pushed. One agent created five docker containers and tore all five down **BY NAME** (`a3-backend`, `a3-ref-arm1/2/3/3b`) plus its network; no `prune` and no `ancestor=`/image filter was used at any point. A pre-existing unrelated container was left untouched.

Every load-bearing claim below was **re-derived by the controller**. Where an agent's claim did not survive, it is recorded as such (§1.11). Where **two agents conflict**, the conflict is recorded and **no number is carried** (§1.2).

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

**NINETEEN refutations, EIGHT load-bearing.** Load-bearing marked ⚠️.

### 1.1 ⚠️ HEADLINE — THE §6.1 LoC TRIGGER IS **CROSSED**, ON MEASUREMENT, AGAINST A BAND ALREADY REVISED UP 2.4×

The SPEC priced the test side at **≈1070** and the grand total at **≈1135-1160**, explicitly flagging that its test figure was *"grounded in analogue sizes but not written."* It was written this stage. Measured, per group, by `git diff --numstat` on real code that compiles, lints and goes RED-then-GREEN:

| group | SPEC estimate | **MEASURED** | ratio |
|---|---|---|---|
| S1 body arms (test) | ≈371 | **679** | **1.83×** |
| S2 trailers arms (test) | ≈333 | **453** | **1.36×** |
| S3 generation guard (test) | ≈195 | **237** | **1.22×** |
| S4 census replacement, RE-INVERTED (test) | 113 "MEASURED" | **280** | **2.48×** |
| S8 encode-cap (test) | ≈60 | **197 new + 14 in-place** | **3.3×** |
| S7 port (test) | −17 / −10 | **−18 / −12** | direction right |
| **TEST TOTAL (deduplicated, see below)** | **≈1070** | **≈1700-1850** | **≈1.6-1.7×** |
| **PRODUCTION** | 63-90 | **≈140-230** | **≈1.6-3.6×** |
| **GRAND TOTAL** | ≈1135-1160 | **≈1840-2080** | **≈1.6-1.8×** |

**The deduplication is stated, not hidden.** The five agents' rosters overlap: A5's 344-line roster re-covers anchors A1, A3 and A4 measured independently, and A1's 79 lines of file overhead + 56 of helpers partly duplicate A2's 28-line banner + 33-line helper. The totals above take each group from **the agent that owned it**, exclude A5's overlapping roster, and subtract ~100-150 for helpers a single IMPL would share. **The production range is wide because the agents' prototypes differ in comment density** — this package's house style is comment-heavy (A1: 57 of 87 added `pause.go`/`body.go` lines are doc comment, 30 are code), and `pause.go` is edited by S1, S2, S3, S5 **and** S6 whose patches **do not apply independently** (§1.7).

⚠️ **THE FINDING IS NOT "THE ESTIMATE WAS LOW". IT IS "THE ESTIMATE WAS UNDER-ENUMERATED."** The SPEC's §13 S1 row prices *seven* items, but the SPEC's own §7 mandates three more — arm-once, decode `Continue`-path disarm, encode `Continue`-path disarm — which measure **190 lines, 28% of S1's test side, priced at ZERO**. Likewise S8's 3.3× is driven almost entirely by **14 pre-existing assertions that pin the old status** and which no estimate counted. **A ratio-rescale is therefore the wrong repair; the group item-lists had to be re-enumerated, and were.**

⚠️ **AND `reference_measured_prototype_is_a_lower_bound` FIRED FOR THE THIRD CONSECUTIVE ROW — THIS TIME AGAINST A BAND THAT HAD ALREADY BEEN REVISED UP 2.4× IN ANTICIPATION OF IT.** The SPEC raised 350-600/~450 to 950-1400/~1150 *before any code landed*, and the measurement still lands **above the revised ceiling of 1400**. The SPEC's own cross-check (478 probe lines / ~6 of 17 items ⇒ ~80 lines/item ⇒ ≈1360) called 1400 "the honest ceiling, not a pessimistic one." **It was still low.** That is the lesson worth carrying: *anticipating the lower-bound effect does not neutralise it.*

### 1.2 ⚠️ SECOND HEADLINE — THE SPEC'S 299-vs-1 A/B IS **PACER-DEPENDENT**, TWO AGENTS MEASURED OPPOSITE RESULTS FOR THE SAME CELL, AND **NO NUMBER IS CARRIED**

The SPEC's §7 rests its entire "both parts are load-bearing" conclusion on one table: over 300 superseded generations at a 50 µs watchdog, `Stop()` alone fires **299**, `Stop()`+guard fires **1**.

Two agents measured it independently and **disagree on the decisive cell**:

| agent | configuration | `Stop()` only (guard OFF) |
|---|---|---|
| A4 | `time.Sleep` pacing, pace == watchdog == **50 µs**, n=300, 5 repeats | **299, 299, 299, 299, 299** |
| A5 | watchdog **200 µs**, 7 spacings 0-250 µs, 3 repeats each | **1** at every spacing ≤ 200 µs; **19** at 250 µs |

**The mechanism that reconciles them is A4's, and it is the load-bearing part:** `time.Sleep` has a **~1 ms floor on this host**, so a requested 50 µs pace really sleeps ~1 ms, and the 299 arises only when the `AfterFunc` deadline and the pacing sleep land on the **same runtime timer tick** and the timer callback loses. Under a **spin/busy-wait** pacer at the *same real 50 µs spacing* (offsets verified 55.3 / 106.1 / 156.8 µs) A4 also read **1, not 299** — broken and fixed arms indistinguishable. A5's independent sweep found the same shape from the other direction.

⇒ **Per `reference_a_drift_correction_is_itself_a_claim` and the contested-count rule, this PLAN carries NO number for that cell.** What it carries instead is structural and survives both measurements:

- **A fire-count gate's discrimination depends on the pacer implementation, the watchdog magnitude, and their ratio.** A gate that does not pin all three is **VACUOUS**, and vacuous in a way that reads as thorough.
- **A4's independent finding (ii):** at nominal pace 0.2× watchdog the sweep read A=291 / B=298 with SPURIOUS=0 — **green on both arms.**
- **A4's independent finding (iii):** the guarded arm reads **1 *or* 2**, not exactly 1. The SPEC's "1" is not a stable pin.
- ⇒ **Task 2 must prove its gate RED at its own pinned configuration**, and Task 2's step 2 exists solely to do that. `reference_probe_input_is_a_claim`.

⚠️ **BOTH AGENTS AGREE ON THE OPERATIONAL CONCLUSION, BY DIFFERENT ROUTES:** `Stop()` contributes **zero** to the fire count. A4's 2×2 cross-product measured A≡D and B≡E. **The SPEC's "`Stop()` is RESOURCE HYGIENE, not correctness" is CONFIRMED** — and quantified: over 200 back-to-back pauses `Stop()` leaves **0** superseded closures pending vs **199** without it, each retaining the chain's `*decoderCB` and through it the whole `*FilterChain`.

### 1.3 ⚠️ THIRD HEADLINE — **S3 HAS NO LIVE DEFECT AND NO RED ANCHOR OF ITS OWN.** SPEC §17 ITEM 2's "BEFORE **or** WITH" COLLAPSES TO **WITH**

SPEC §1.2 asserts the timer-overwrite defect is *"REACHABLE TODAY, NOT LATENT"* — that two `beginDecodePause` calls without an intervening `resumeDecode` are already permitted by the two headers arms alone.

**Refuted by execution.** Driving the full decode lifecycle at the current tip against one wasm filter whose guest Pauses:

```
after RunDecodeHeaders#1   : decodePauseGen=1
after RunDecodeHeaders#2   : decodePauseGen=1
after 3x RunDecodeData     : decodePauseGen=1
after 2x RunDecodeTrailers : decodePauseGen=1
```

`beginDecodePause` has exactly **one** call site (`decode_headers.go:270`), reachable only from `DecodeHeaders`; `c.decodeIdx` is monotone in `RunDecodeHeaders` (`chain.go:333-360`) so a second call iterates nothing; the body/trailers arms are frozen and never call it.

⚠️ **AND THE SPEC'S STATED WIDENING IS ITSELF THE S3/S9 CONFLATION IT WARNS AGAINST.** Two wasm filters give `A.decodePauseGen=1`, `B.decodePauseGen=1` — each filter owns its own timer field. **A sibling filter produces the STALE-TOKEN hazard (S9), never the TIMER-OVERWRITE hazard (S3).**

⚠️ **A1 REACHED THE SAME PLACE FROM THE OTHER SIDE, INDEPENDENTLY.** It measured that chunk 1's park is released by the watchdog, whose CAS *clears* the flag, so chunk 2 enters with `decodePaused=false` and the replaced timer has **already fired** — not an orphan. A live orphan requires a release that does **not** clear the flag, i.e. a stale token. **A test written to the SPEC's §1.2 shape would have been vacuous.**

⇒ **S3 is prophylactic.** Its RED anchor must be created by the fire-count harness itself, and it **must land in the same commit as S1's body arms**, which create the first reachable trigger.

*(One genuinely reachable path the SPEC misses, on the other side: `RunEncodeHeaders` **resets** `c.encodeIdx = len(filters)-1` on every call (`chain.go:490-491`), unlike the decode cursor — driven three times gives `encodePauseGen=3`. **Not production-reachable** — `beginLocalReply` is `localReplyOnce`- and `encodeStarted`-guarded (`chain.go:1239-1245`) and every HCM `RunEncodeHeaders` site sits behind a `LocalReplyDone()` early-return. **A latent `FilterChain`-API trap, recorded not fixed.**)*

### 1.4 ⚠️ FOURTH — THE SPEC'S §7 SNIPPET HAS A HOLE, AND PLUGGING IT **IS** THE S9 FILTER-LOCAL FIX

`stopPauseTimer` / `stopPauseWatchdogs` (`pause.go:172-192`) do **NOT** bump the generation counter. So a watchdog closure that has **already entered** when the disarm runs passes the S3 guard and fires into a torn-down stream. `Timer.Stop()` cannot cancel it.

| arm | late fires |
|---|---|
| SPEC §7 snippet as written | **400 / 400** |
| + generation bump inside `stopPauseTimer` | **0 / 400** |

**Cost: 6 production lines in `pause.go`, inside this row's file set, no new API, no chain edit.** It is simultaneously the answer to owed item 3 (§2.3).

### 1.5 ⚠️ FIFTH — CAS-FIRST IS NOT MERELY WRONG, IT IS **CATASTROPHIC**, AND THE COUNTER-EXAMPLE WAS BUILT

SPEC §7 says placing the generation check ahead of the CAS is "not negotiable" but does not say what CAS-first costs. Measured:

| ordering | fires | **lost resumes** |
|---|---|---|
| generation check **before** CAS (correct) | 1 | 0 |
| CAS **first** | **0** | **299** |

A superseded timer's CAS **consumes the current pause's flag** and then bails on the generation check. The current pause then has **no resumer at all**: `resumeDecode`'s CAS fails, the guest's `proxy_continue_stream` signals nothing, and **the chain parks forever.** ⇒ **CAS-first converts a spurious resume into the unbounded park this row exists to close.** The SPEC's conclusion is right; its stated stakes were an order of magnitude too mild.

### 1.6 ⚠️ SIXTH — THE S3 GATE IS **NOT `-race`-SAFE**, AND THE SPEC'S "`-race` CATCHES NOTHING" IS TRUE ONLY BECAUSE ITS PROTOTYPE SHIPPED NO ASSERTION

With the **correct fix loaded**, `-race` reports **0 DATA RACE** — and **5 of 7 fire-count assertions FAIL**: 133/300, 2, 150/300, 207/400, 101/200. `-race` widens the atomic window so superseded timers fire *legitimately* before being superseded; **fixed code reads in the same band as broken code.**

⚠️ **A5 INDEPENDENTLY FOUND THE SAME GATE FLAKES UNDER FULL-PACKAGE LOAD** with `t.Parallel()`: FAIL 3/3 in the full package, PASS 3/3 under `-run` isolation. **Any timing-shaped gate must be non-parallel and must be validated on the FULL package, not under `-run`** (`reference_change_set_measure_not_build_measure`).

⇒ **The PLAN must choose a `-race` posture.** Three options, priced (§2.5). This is a decision the SPEC does not anticipate and it must not surface at the IMPL.

### 1.7 ⚠️ SEVENTH — S5, S6 AND THE `pause.go` HALF OF S1/S2/S3 **DO NOT APPLY INDEPENDENTLY**

Generated against the same base, the S5+S6 patch and the S1/S2/S3 patch **conflict on `pause.go`**: S6's derivation rewrite (`pause.go:54-77`) sits **inside** S5's rewrite region, and S5's file-header rewrite describes behavior S1/S2/S3 introduce. **This is a hard constraint on the task decomposition, discovered by attempting it, not by reading.**

### 1.8 ⚠️ EIGHTH — THE ENCODE-CAP ARM IS **GUEST-INDEPENDENT**, SO THE SPEC'S BLAST-RADIUS BOUND RESTS ON THE WRONG FACT

The SPEC bounds live exposure with *"`0036` is the ONLY fixture repo-wide with a wasm body callback."* But the cap check does **not** sit behind the guest gate. Measured order inside `EncodeData`: sticky check → **accumulate, always** → **cap fires** → *then* `HasGlobalFunc(proxyOnResponseBody)`. Proven by constructing `&filter{cfg: cc}` with **`streamCtx == nil` — no guest at all** — where `encodeBodyCapExceeded` still became true with `body_buffer_cap_exceeded == 1`.

⇒ **Every wasm-filtered stream accumulates the response body and is exposed.** The corpus is safe for a *different* reason the row should record instead: `0036`'s driver sets `BodyBufferCapBytes = 1024` on **listener `n` only** (the other 13 get 0 ⇒ 16 MiB default), and listener n's response body is the 17-byte 413 from the *decode* side. **Encode-cap firings in the corpus: ZERO.**

### 1.9 Also refuted — eleven more, each with a cite

- ⚠️ **The "SEVENTH unbounded park" UNDERCOUNTS.** Three more verified by execution, all in the file the row edits: **`body.go:260`** (encode sticky, 2nd chunk after the cap), **`body.go:172`** (decode cap when `decoderCb == nil` at `:166-171` — no `SendLocalReply`, so no `localReplyDone`), and **`body.go:309`** (encode `sentLocalResponse`, same `chain.go:599` arm). **S8 as chartered closes 1 of at least 3.** The decode-side *normal* cap path is genuinely safe: `chain.go` re-checks `localReplyDone` at `:394`/`:406` **before** the status switch.
- ⚠️ **Setting `localReplyDone` would NOT rescue the encode side.** All **six** checks are decode-side (`chain.go:335, 350, 394, 406, 458, 467`); `RunEncodeHeaders`/`RunEncodeData`/`RunEncodeTrailers` contain **zero**, and none of the four `parkEncode` sites (`503, 569, 599, 635`) is guarded. **A fix that only sets the flag lands on a loop that never reads it — this eliminates the cheapest-looking remedy.**
- ⚠️ **A candidate the SPEC does not list, REFUTED BY EXECUTION: having the watchdog `SendLocalReply` instead of `ContinueDecoding` DEADLOCKS.** `beginLocalReply` sets `localReplyDone=true` and runs the encode chain but **never sends on `decodeResumeCh`**, so the dispatch goroutine stays parked forever. It converts a *bounded* park into an *unbounded* one. **Recorded so the IMPL does not rediscover it.**
- ⚠️ **A wrong wasm type-section entry INSTANTIATES FINE.** It fails only at call time inside wazero (`expected 3 params, but passed 2`), where the host **fail-OPENs** and returns `TrailersContinue`. Every downstream S2 test then **silently skips the arm and reads green**, distinguishable only by `envoy_go.failures == 1` — and under caps-denied, **completely invisible** (`err=<nil>`, `failures=0`).
- ⚠️ **The chain regression's park barrier must NOT be the pause flag.** A `t.Fatalf` on `decodePaused` fires first on the unfixed tree and makes the `MEASURED HAZARD` assertion **dead code** (`reference_fatalf_makes_assertions_unreachable`). The correct barrier is *"`Run*Trailers` has not returned after 200 ms"*, true on **both** trees.
- ⚠️ **S9's current reachability is ZERO, not "2 sites widening to 6".** The stale-token spend reproduces (an unrelated trailers park released in **86.9 µs**, `decodePauseGen == 1` throughout — which is how it discriminates from S3) **only when HCM's `defer chain.Destroy()` is suppressed**. All three dispatchers defer it (`connection.go:447`, `h2dispatch.go:383`, `h3dispatch.go:203`), and `OnDestroy → stopPauseWatchdogs` disarms long before the watchdog fires. In the production shape: **0 spends in 60 trials, with and without the mitigation.** ⇒ **S1/S2 CREATE S9's first reachable trigger rather than widening it threefold — which strengthens the case for landing S3 with them.**
- ⚠️ **`pause.go:72-73` is wrong in BOTH halves, and it is a NEW S5 site.** It claims (l)'s result is discarded via `_ = runScenarioGet(...)`. Measured: `driver.go:551` is `emitScenario(&buf, runScenarioGet(...))` — **not discarded**; the `_ =` arms are (k) `:526`, (m) `:554`, (n) `:561`. **(l) is CROSS-SIDE as of phase 82** and `driver.go:352-359` asserts `http_call_response >= 1` **and** `http_call_response_after_close == 0`. ⇒ **the "not a verdict change" conclusion for a 10 s watchdog against the 15 s client timeout NO LONGER FOLLOWS. Load-bearing for S6.**
- **The S5 surface is SIXTEEN sites, not eleven.** Five the SPEC does not name: `body.go:25-28`, `trailers.go:154-157`, `wasm.go:218-222`, `body.go:41-42`, `trailers.go:24-25`. All 16 rewritten, all 16 landed. **`"terminates the response early"` occurs FOUR times** (`body.go:27`, `body.go:279`, `trailers.go:157`, `wasm.go:222`), and ⚠️ **`StopAllIteration` is a PHANTOM IDENTIFIER declared NOWHERE** — 7 comment lines across 3 files name it; the real roster is `DataContinue` / `DataStopIterationAndBuffer` / `DataStopIterationNoBuffer`.
- **The false comment is wrong THREE ways, not two:** it additionally **cites ADR-0071 for a claim ADR-0071 does not make.** Bounded extraction (heading `:2586`, next `^## ADR-` of any id `:2634`, **48-line denominator**): `SendLocalReply` **0 hits**. ADR-0071 §1/§3 never mention it, and **ADR-0073 §Context (`DECISIONS.md:2733`) says the opposite** — *"decode **or encode side**"*.
- **`Run*Trailers` is dead production code, confirmed with the command:** every non-test hit is a comment or a definition. Consequence for S2 the SPEC misses: the per-stream `StreamContext` is built **only** in `DecodeHeaders`, so **both** sides' trailers tests must run `DecodeHeaders` first — which is what forces the shared setup helper the SPEC's roster omits.
- **Line-cite corrections:** `trailers_test.go`'s five tests are at **51, 77, 95, 122, 146** (SPEC: 50/78/95/117/140 — 3 of 5 wrong); `_ = trailers` is at **`:101`**, not `:100`; `idle_timeout` returns **4 hits in 3 files**, not 2 (the conclusion survives — there is still no idle-timeout implementation).

### 1.10 CONFIRMED, so the IMPL can rely on it

- **The census is exactly 6** `abi.ProxyActionPause` arms at the SPEC's exact lines — **no drift at this tip**, re-derived symbol-anchored with the denominator printed.
- **`OnDestroy` already disarms BOTH sides correctly.** `encode_headers.go:165` calls `stopPauseWatchdogs()` first and unconditionally, ahead of the `nil streamCtx` early return; `chain.go:666-674` prefers the `Decoder` branch so it fires once, which suffices because one call clears both fields. Verified through `chain.Destroy()`: `armed true/true → post false/false`. ⚠️ **`reference_ondestroy_fires_once_encoder_unreachable` does NOT bite here. No S1 work is owed.**
- **`pause.go:19-21`'s `0036` (n) dependency is INVERTED**, independently confirmed twice: (n) depends on the arm **never being reached**. `body.go:138` accumulates and `:144` checks the cap **before** any dispatch; a 2048-byte body vs a 1024-byte cap returns at `:172`. **The entire stated basis for freezing `body.go`/`trailers.go` is void.**
- **Both stated `defaultPauseWatchdog` bounds fall.** `http_call.go:279-282` consults the 5 s default **only when the guest passes 0**; the cited guest passes `Duration::from_secs(5)` explicitly; there is no cap anywhere on the path. `git grep defaultPauseWatchdog -- '*.go'` ⇒ 4 hits, all non-test, **zero magnitude assertions**.
- **`pause.go:25-28`'s "ZERO of the 35 guest crates" is FALSE** — exactly one does, `l_httpcall_success/src/lib.rs:43`, added by the commit that wrote the claim.
- **`abi_callbacks.go:854-856` is load-bearing-false**, confirmed one layer up by execution: `DecodeData = 1`, `decodePaused = false` after the return, `ContinueStream` returns `Ok`, **`ContinueDecoding` calls actually fired = 0.**
- **The `0036`-only claim SURVIVES, now with the denominator and control the SPEC omitted:** 4 files, all in `0036`; the glob sees **25** `.rs` files with a firing control at 25/25; widening to all **35** repo-wide returns the same 4.
- **No sibling vacuous gate of the same species exists repo-wide.** 444 `_test.go` files swept: 34 syntactic hits, 31 with a runtime value on the left, firing control at 343. **The constant-vs-constant species is bounded at `pause_test.go:135/138/141`.**
- **Adding the two trailers capabilities is NOT a global change:** 445 → 445 PASS, **zero-line per-test result diff**, because none of the 7 pre-existing fixture builders exports either trailers callback.
- **`-race` finds no data race** in any prototype. It is not the gate (§1.6).

### 1.11 ⚠️ An agent claim that did NOT survive, and a controller position that changed

- **A5 reported the SPEC's 299-vs-1 "is not reproducible" and concluded the two mechanisms are "compensating, not complementary."** The controller did **not** adopt that. A4's independent 2×2 measured `Stop()`-only at 299 in its configuration, which refutes "compensating"; A4's *pacer* finding then explains A5's numbers without either agent being wrong. **The synthesis neither agent reported alone is that the A/B is configuration-dependent — so the right output is no number and a pinned-configuration requirement** (§1.2).
- **The controller's own first count of `STATE_HISTORY.md` was WRONG, and the reason is a live defect.** Using the guard this router prescribes — `^- \*\*prior active-phase:\*\* \`<name>\`` — gives **163**. The true figure is **175**: twelve entries carry an annotated label (`- **prior active-phase (evicted at the phase-NN close, …):**`) the anchored form cannot match. ⚠️ **The guard is FAIL-UNSAFE in its own direction** — it exists to prove an entry is *absent* before appending, so a false "absent" **authorizes a duplicate append**, and all twelve blind entries are the most recent evictions (phases 80-83). Discriminating cross-product on **real** targets (a first probe used an invented phase name and read 0 on both arms for the wrong reason — `reference_probe_input_is_a_claim`):

| target | raw presence | router guard | tolerant guard |
|---|---|---|---|
| `phase 79 (stats-prometheus-projection) BRAINSTORM done` (annotated) | **1 — PRESENT** | **0 — FALSE ABSENT** | 1 ✅ |
| `phase 77 (runtime-static-layer) IMPL done` (plain) | — | 1 ✅ | 1 ✅ |
| invented target | — | 0 ✅ | 0 ✅ |

  **Use `^- \*\*prior active-phase[^*]*:\*\* \`<name>\`` instead.** The router's **175/176** figures are correct; the command it prescribes for checking them is not.

---

## 2. THE FIVE ITEMS THE SPEC SAYS THIS PLAN OWES — ALL FIVE DISPOSED

### 2.1 Owed item 1 — the §6.1 re-evaluation against a MEASURED test side

**DONE, §1.1. The LoC trigger CROSSES.** Arithmetic stated:

- **Task trigger (`:289`, ~25):** **18 tasks**. Does **not** fire, but the margin is 28% rather than the SPEC's 36-52%.
- **LoC trigger (`:290`, ~1500):** measured **≈1840-2080 net**. ⚠️ **FIRES.**
- **Third trigger (`:292`, sub-steps > ~10 mid-execution):** no single task below exceeds 8 steps. Does not fire.

⚠️ **DISPOSITION: RECORD, DO NOT RETRO-SPLIT.** That is the lineage precedent at both prior crossings (phase 81 at 3.07×, phase 82 at 2.55× — the latter also crossed `:290` and was explicitly recorded rather than split). The §6.2 split machinery costs a new ROADMAP row (which moves `want` off 115), a new ADR, and a redistributed SPEC — against a row whose scope items are **coupled** (§1.7 forces S1/S2/S3/S5/S6 to share `pause.go`; §1.3 forces S3 into S1's commit). **A split along any available axis would cut through a correctness constraint.** This is the third consecutive crossing and it is recorded as a finding, not absorbed silently.

### 2.2 Owed item 2 — task ordering that lands the generation guard BEFORE or WITH S1/S2

**DISPOSED: WITH, and "BEFORE" is no longer available.** §1.3 refutes the premise that S3 has a live defect: it has no RED anchor of its own, so it cannot land first on its own merits. Its synthetic anchor is the fire-count harness (Task 1), and the guard lands in Task 2, in the **same commit** as S1's body arms (Task 3) is **not** required — but Task 2 **must precede** Task 3, because S1 without the guard takes S9 reachability from 0 to N. **This is a correctness constraint, not a preference**, and it is now justified by measurement rather than by the SPEC's refuted reachability claim.

### 2.3 Owed item 3 — the stale-token hazard (D-83-STALETOKEN)

**DISPOSED: PARTIAL FILTER-LOCAL FIX ADOPTED INSIDE S3; THE RESIDUAL CHAIN-LAYER HAZARD DEFERRED BY NAME.**

> A wasm filter can emit an *unmatched* `ContinueDecoding` in exactly one way: a watchdog closure that has already entered when `stopPauseTimer` runs. Bumping the generation *inside* `stopPauseTimer` makes the disarm authoritative — **400/400 late fires → 0/400**, at a cost of **6 production lines in `pause.go`**, inside this row's file set, with no new API and no chain edit. **This eliminates every stale token a wasm filter is capable of latching.**
>
> **Deferred BY NAME:** a token latched by any *other* parking filter, or spent by a filter that did not latch it. **Unfixable filter-locally, and now PROVEN so rather than asserted:** `decodeResumeCh`/`encodeResumeCh` are **unexported** fields on `*FilterChain` (`chain.go:59-60`) in a package the wasm filter imports externally; the filter holds only a `DecoderFilterCallbacks` whose **entire surface is 8 methods**, none of which observes or drains park state; and `*FilterChain`'s 32 exported methods expose no park-state predicate (the only reachable one is `LocalReplyDone()`, which is not park state). Candidates (a) per-filter epoch and (d) "I latched this token" are unactionable — the filter can know it latched a token but cannot un-latch it. Candidate (j) — watchdog `SendLocalReply` instead of `ContinueDecoding` — **DEADLOCKS** (§1.9).

⚠️ **AND ITS REACHABILITY IS CORRECTED DOWNWARD TO ZERO TODAY** (§1.9): S1/S2 create the first reachable trigger rather than widening an existing one. **It does not surface at the IMPL: the fix is Task 2, the deferral is written here.**

### 2.4 Owed item 4 — the encode-side cap park (D-83-ENCODECAP), **with the reference's behavior cited**

**DISPOSED: OPTION A — A NON-PARKING STATUS, in the `DataTerminateStream` shape.**

**THE REFERENCE'S BEHAVIOR, MEASURED — not sourced from documentation.** `body_buffer_cap_bytes` is envoy-go-strict-only (`BEHAVIOR_CONTRACT.md:4039`: *"Upstream Envoy v1.37.2 does NOT emit any of these"*), so there is no reference knob to probe directly; the upstream mechanism with the same semantics is `per_connection_buffer_limit_bytes` against a paused encoder filter. A guest returning `Action::Pause` from `on_http_response_body` was built (**no guest in this repo overrides it** — 0 hits over 36 blobs) with the pinned `cargo +1.94.0 … --target wasm32-wasip1`, and run against `envoyproxy/envoy:contrib-v1.37.2`, fresh container per arm:

| arm | config | client result | stats |
|---|---|---|---|
| **1** cap breach | limit 1024, guest **Pause**, body 64 KiB | 200 + headers delivered, then **stream reset, ZERO body bytes**; curl (18), `SIZE_DOWNLOAD=0`, **1.007 s** | **`http.hcm_a3.rs_too_large: 1`**, `downstream_rq_tx_reset: 1` |
| **2** NC | limit 1024, guest **Continue** | `200`, **65536 bytes**, 0.003 s | `rs_too_large: 0`, `tx_reset: 0` |
| **3** pause, no breach | limit 8 MiB, guest Pause | headers, then **HANGS** — curl (28) at 20 s, 0 bytes | `rs_too_large: 0` |
| **3b** + `stream_idle_timeout: 3s` | as 3 | terminated at **4.005 s** | **`downstream_rq_idle_timeout: 1`** |

⇒ **Upstream answers an encode-side buffer-limit breach with an IMMEDIATE STREAM RESET and `rs_too_large` — never a stall.** Separately and orthogonally, it bounds *any* indefinite encode pause with `stream_idle_timeout`. **These are two mechanisms for two failures; envoy-go has neither.** Arm 2 is the control proving arm 1's reset is caused by pause+limit, not by the filter's presence.

**Why A and not B:**
1. The reference answers a cap breach with arm 1, **not** arm 3b. A cap breach is deterministic and immediately detectable; converting it into a multi-second timeout is the wrong semantic. Arm 3b's timeout is the *guest-pause* bound — S1/S3 territory, not S8's.
2. ⚠️ **Arming the existing watchdog is WORSE THAN THE BUG.** `beginEncodePause`'s fire path calls **`cb.ContinueEncoding()`** (`pause.go:130`), which would unpark the chain and *continue iteration*, **delivering the full over-cap body after a 10 s stall.**
3. A real host bound needs the `idle_timeout` knob envoy-go has **explicitly deferred** (`listener/manager.go:461`) — proto plumbing, an ADR, and all 18 park sites. Tight estimate: **≥250 production + ≥300 test**, ≥4× option A, *and it would still need option A's status change* to avoid resuming into a body delivery.

⚠️ **`DataContinue` IS REJECTED BY MEASUREMENT, NOT BY REASONING:** flipping the cap arm to `DataContinue` and driving the real chain gives `terminated=true err=<nil> encodeBodyOverride_present=false`, so `connection.go:753-755` writes `resp.Body` **in full** — **it silently disables the cap.**

**Departure to record:** the reference flushes response **headers** before resetting; envoy-go runs the whole encode chain before `writeH1Reply`, so the client gets **nothing** — connection close, no headers. Both are "no usable body + torn-down connection"; the byte shape differs. **A real, defensible departure — written down, not papered over.**

### 2.5 Owed item 5 — a break roster whose arms are proven RED first

**DONE, §4.** 20 arms, **19 proven RED by execution**, one proven **NOT reddenable** and recorded as such. ⚠️ **And the `-race` posture decision this stage discovered (§1.6) is made here, not deferred:**

**CHOSEN: option (a) — build-tag the fire-count arms out of the `-race` run, with the blindness RECORDED.** The alternatives were priced and rejected: (b) widening the bound is fragile (299 vs 207 barely separate); (c) routing the watchdog through the package's `clock.Clock` seam (`tick_clock.go:72-116`) is **not free** — `clock.Clock` (`internal/clock/clock.go:60-72`) exposes `Now()` and `After(d)` but **no `AfterFunc`**, so `beginDecodePause` would become a goroutine + `select`, adding a goroutine per pause and changing the teardown shape. **(c) is the right long-term answer and is named as a deferred candidate; it is not this row's work.**

---

## 3. Global constraints

- **Go 1.23** (`.github/workflows/ci.yml:14,39,92`). **golangci-lint v1.64.8** via `golangci-lint-action@v6.5.2` (`:21-23`), `disable-all: true` with **9** linters: `govet errcheck staticcheck unused ineffassign gofmt goimports misspell revive`. ⚠️ **`gosec`, `nolintlint`, `depguard`, `forbidigo` are NONE of them** — the two `//nolint:gosec` directives in `body.go` are **INERT**, and S7's would be too.
- ⚠️ **`misspell` runs locale US.** A British spelling in a `.go` comment FAILS. It fired live at four of five agents this stage.
- ⚠️ **`gofmt -l` NEVER exits non-zero — gate on OUTPUT.** And an empty lint result is not evidence the linter looked: **inject a British spelling, confirm `misspell` fires, restore byte-identically and verify by checksum, not by eye.**
- ⚠️ **Plugin `config.name` must be process-unique** (`-count=N>1` fails *"…is duplicated across PluginConfig entries"*) **and feeds a stat scope** guarded by `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` — `t.Run` names containing `-` or non-ASCII are **rejected at config build**. Use `p83_<arm>` shapes.
- ⚠️ **A nil `*stats.Counter` `.Inc()` is a PROCESS CRASH** with no `recover()`. Assert the pointers.
- **New fixtures: 0. New BackendKind: 0. New port: 0 (S7 REMOVES one). go.mod: +0. New Rust blob: 0. Toolchain: none** — the pinned rustup 1.94.0 stays off the critical path.
- **Stat surface: +0.** No `NewCounter`/`NewGauge` call site moves.
- `ROADMAP.md` is touched **only** by the IMPL, **in place**, `status` field only, per §Schema `:18`. `want` **stays 115**.

---

## 4. File structure — the IMPL's edit surface, re-derived at `e4563a49`

**Production (8 files, 2 packages):**

| file | what changes | owed by |
|---|---|---|
| `internal/filter/http/wasm/pause.go` | generation counters, guard ahead of the CAS, `Stop()`, **generation bump in `stopPauseTimer`**, arm-once + disarm helpers, `cbName` parameterization, S6 derivation rewrite, S5 header rewrite | S1,S2,S3,S5,S6,S9 |
| `internal/filter/http/wasm/body.go` | 2 Pause arms flag+arm; `Continue`-path disarm; fail-OPEN disarm; 3 park sites → `DataTerminateStream`; comment rewrites | S1,S5,S8 |
| `internal/filter/http/wasm/trailers.go` | 2 Pause arms flag+arm; comment rewrites | S2,S5 |
| `internal/filter/http/wasm/wasm.go` | 2 `atomic.Uint64` generation fields; comment rewrites | S3,S5 |
| `internal/filter/http/wasm/decode_headers.go` | `cbName` at the call site; stale file header | S5 |
| `internal/filter/http/wasm/encode_headers.go` | `cbName` at the call site | S5 |
| `internal/filter/http/wasm/abi_callbacks.go` | the load-bearing-false `:854-856` comment | S5 |
| `internal/filter/http/types.go` | `DataTerminateStream` enum value | S8 |
| `internal/filter/http/chain.go` | `errStreamTerminatedByFilter` + a `case` arm in **both** `RunDecodeData` and `RunEncodeData` | S8 |

**Test (7 files):** `wasm_fixtures_test.go` (body + trailers fixtures, WASM-DSL primitives, **new two-arg type-section entry**), `dispatch_test.go` (**two trailers capabilities**), `pause_body_test.go` **NEW**, `trailers_test.go`, `pause_gen_test.go` **NEW**, `p83_census_test.go` **NEW** (S4 replacement), `encodecap_park_test.go` **NEW**, `chain_test.go`, `http_call_response_cache_test.go` (S7), plus **14 in-place assertion updates** in `body_test.go`/`dispatch_test.go`.

⚠️ **`wasm.go` is +0 for S1.** The SPEC's `wasm.go +8` belongs to **S3**. A task that carries it inside S1 will show a phantom struct edit.

---

## Task 1 — the S3 fire-count harness and its three NCs, **RED FIRST**, against unmodified `pause.go`

**Files:** Create `internal/filter/http/wasm/pause_gen_test.go`.
**Interfaces:** Produces `p83FireCount(t, opts) (fires, spurious int)`; consumed by Tasks 2 and the break roster.

- [ ] **Step 1: Write the harness driving `beginDecodePause` directly**, n=300 superseded generations, plus a single-pause NC and a disarm-authority probe. ⚠️ **PIN THE PACER EXPLICITLY**: `time.Sleep` pacing at **pace == watchdog**, and record both values in the test name and the failure message. §1.2 proves a gate that leaves these unpinned is vacuous. ⚠️ **NO `t.Parallel()`** (§1.6).
- [ ] **Step 2: PROVE THE GATE DISCRIMINATES AT ITS OWN CONFIGURATION.** Run the 2×2 (guard on/off × `Stop()` on/off) at the pinned pacing and **record all four cells in the commit message**. If the broken arm does not separate from the fixed arm by ≥10×, **the pacing is wrong and the gate is vacuous — fix the pacing, not the bound.** This step exists because the SPEC's own numbers did not reproduce.
- [ ] **Step 3: Run RED.** Expected: the superseded-generation arm fires far above 1; the single-pause NC fires **exactly 1**; the disarm-authority probe reports **400/400 late fires**.
- [ ] **Step 4: Assert with a stacked control.** `1 <= fires <= 10` (a ≥30× margin from the measured broken arm) **plus** the single-pause NC at exactly 1. ⚠️ **A positive arm alone cannot catch an over-firing counter** (`reference_positive_arm_cannot_catch_overfiring`); the guarded arm reads **1 or 2**, not exactly 1, so do not pin it at 1.
- [ ] **Step 5: Add the `-race` exclusion** per §2.5 option (a), with a comment recording that `-race` widens the atomic window so fixed code reads in the broken band — **not** that the test is flaky.
- [ ] **Step 6: Run the FULL package**, not `-run` (§1.6). Commit.

---

## Task 2 — S3 + S9: generation counters, guard **ahead of the CAS**, `Stop()`, and the **generation bump in `stopPauseTimer`**

**Files:** Modify `internal/filter/http/wasm/pause.go`, `internal/filter/http/wasm/wasm.go`.
**Interfaces:** Consumes Task 1's harness. Produces `decodePauseGen`/`encodePauseGen atomic.Uint64`; consumed by Tasks 3 and 4.
**Measured cost: 51 ins / 2 del production.**

- [ ] **Step 1: Add the two generation fields** to `filter` in `wasm.go`:

```go
// decodePauseGen / encodePauseGen supersede watchdog closures. A closure
// captures the generation current when it was armed and returns early if a
// later pause — or a DISARM — has since bumped it. Bumping on disarm is what
// makes stopPauseTimer authoritative against an already-entered closure:
// Timer.Stop() cannot cancel one, and without the bump it fires into a
// torn-down stream (measured 400/400; with the bump, 0/400).
decodePauseGen atomic.Uint64
encodePauseGen atomic.Uint64
```

- [ ] **Step 2: Rewrite `beginDecodePause`** — the ordering is a correctness constraint, not style:

```go
gen := f.decodePauseGen.Add(1)   // FIRST — before Store(true)
f.decodePaused.Store(true)
cb := f.decoderCb
if cb == nil {
        return
}
t := time.AfterFunc(f.watchdogTimeout(), func() {
        if f.decodePauseGen.Load() != gen {
                return // superseded by a later pause OR by a disarm
        }
        if !f.decodePaused.CompareAndSwap(true, false) {
                return // already resumed by the guest
        }
        logf("WARN wasm: decode-side pause watchdog fired (stream=%d) — the guest returned PAUSE from %s and never called proxy_continue_stream; force-resuming the stream to avoid a parked-connection leak",
                f.streamContextID, cbName)
        cb.ContinueDecoding()
})
f.pauseMu.Lock()
old := f.decodePauseTimer
f.decodePauseTimer = t
f.pauseMu.Unlock()
if old != nil {
        old.Stop() // return value IGNORED — it is not a signal
}
```

⚠️ **The generation check MUST precede the CAS.** CAS-first measured **0 fires and 299 LOST RESUMES**, because a superseded timer consumes the current pause's flag and then bails — leaving the chain parked forever (§1.5). Mirror all of this in `beginEncodePause`.

- [ ] **Step 3: Bump the generation inside `stopPauseTimer`** — this is the S9 filter-local fix (§2.3), 6 lines:

```go
func (f *filter) stopPauseTimer(decode bool) {
        if decode {
                f.decodePauseGen.Add(1) // make the disarm authoritative
        } else {
                f.encodePauseGen.Add(1)
        }
        f.pauseMu.Lock()
        ...
}
```

- [ ] **Step 4: Run Task 1's harness GREEN**, on the full package. Expected: superseded arm within `[1,10]`, NC exactly 1, disarm-authority **0/400**.
- [ ] **Step 5: Run the stacked break** — remove the guard **and** the `Stop()` together. ⚠️ **The single-arm breaks do NOT redden** (§4), so a single-arm break here is not evidence. ⚠️ **Removing the guard orphans `gen` and removing `Stop()` orphans `old`** — both need `_ = x` substitutes or you get a build failure, not a RED (`reference_plan_break_instructions_dont_compile`).
- [ ] **Step 6: Commit.**

---

## Task 3 — S1: the two body arms, arm-once, and the `Continue`-path disarm

**Files:** Modify `internal/filter/http/wasm/body.go`, `pause.go`; `wasm_fixtures_test.go`; Create `pause_body_test.go`.
**Interfaces:** Consumes Task 2's generation counters. Produces `beginDecodePauseOnce`/`beginEncodePauseOnce`, `disarmDecodePause`/`disarmEncodePause`.
**Measured cost: 87 ins / 5 del production; 679 test.**

- [ ] **Step 1: Write the WASM-DSL primitives and the two body fixtures** (`wasm_fixtures_test.go`, **99 measured lines**). A body fixture needs an extra export, a `1 − end_of_stream` body, and 13 lines of new DSL (`local.get`, `i32.sub`) the headers fixtures never needed — this is why the fixture analogue was the SPEC's worst estimate (2.06×). ⚠️ **S1a MUST precede S1b: there is no in-tree guest, no vendored blob and no fixture that reaches either arm, so without a synthetic fixture there is literally nothing to make RED.**
- [ ] **Step 2: Write the failing-first anchors RED** — 2 chain-level unbounded-park regressions + the `ContinueStream` body-resume. ⚠️ **Drive through `FilterChain`, not the filter methods directly** — the S4 replacement does not cover the park (§1.9 hazard). ⚠️ **The park barrier must be "`RunDecodeData` has not returned", never the pause flag.**
- [ ] **Step 3: Run and confirm RED.** Expected, verbatim from the prototype: `RunDecodeData never returned: the guest paused from proxy_on_request_body and nothing bounded the park` — with the test taking **10.00 s**, i.e. the park actually hanging. **Grep the log for `no tests to run`; it must be 0.**
- [ ] **Step 4: Implement** `beginDecodePauseOnce` (no-op when `decodePaused` is already true), `disarmDecodePause`, their encode mirrors, and wire all four `body.go` sites — the two Pause arms, the two `Continue`/default arms, **and the two fail-OPEN `err != nil` arms** (which return `DataContinue` and therefore release the chain).
- [ ] **Step 5: Rewrite `body.go:222-224`'s false comment** in the same commit. It claims the chain accumulates further chunks the guest then sees; `chain.go:430` appends to `c.decodeBuf` and then **blocks in `parkDecode`**, and the goroutine that would deliver the next chunk is the one blocked. **Proven by the 10 s hang in step 3, not by reading.**
- [ ] **Step 6: Parameterize the watchdog log strings** — `pause.go:103-104` **and** `:128-129` hard-code `proxy_on_request_headers`/`proxy_on_response_headers`, captured verbatim for a **body** pause and (independently) for an **encode-trailers** pause. ⚠️ **This must land here, not in S5: S1's landing is what makes the lie fire.** ~8 lines, in no other estimate.
- [ ] **Step 7: Write the arm-once and both `Continue`-path disarm tests** (**190 measured lines — the three items the SPEC's §13 table priced at zero**) and run the two isolating NCs: reverting arm-once alone must trip only the arm-once assertion; removing the two disarms alone must trip only the two disarm assertions. **The "no fix at all" RED does not discriminate between them.**
- [ ] **Step 8: Full package + `-race`. Commit.**

---

## Task 4 — S2: the two trailers arms, the new type-section entry, and the two missing capabilities

**Files:** Modify `internal/filter/http/wasm/trailers.go`, `wasm_fixtures_test.go`, `dispatch_test.go`, `trailers_test.go`.
**Measured cost: 8 ins / 0 del production; 453 test.**

- [ ] **Step 1: Add the two-arg type-section entry** and the two trailers fixtures (**94 measured lines**). ⚠️ **`CallProxyOnRequestTrailers` is `(i32,i32)->i32`** (`stream_context.go:227-231`), not the headers/body `(i32,i32,i32)->i32` (`:180-184`, `:205-209`). The existing `fixTypeSection` type 1 does not cover it.
- [ ] **Step 2: GATE THE TYPE SECTION SPECIFICALLY.** ⚠️ **A wrong type section INSTANTIATES FINE** and fails only at call time, where the host fail-OPENs and returns `TrailersContinue` — so every downstream test **silently skips the arm and reads green**. Assert `CallProxyOnRequestTrailers` returns `action=1, err=nil`; **do not assert merely that the module instantiates.**
- [ ] **Step 3: Add both trailers capabilities to `testLifecycleCapabilities`** (`dispatch_test.go:113-125`), which today has both **body** capabilities and **neither** trailers capability. Verify the full-package PASS count is **unchanged 445 → 445** with a **zero-line per-test result diff**; a non-empty diff means something else depends on trailers being denied and is a new finding.
- [ ] **Step 4: Write the four anchors RED** — 2 chain regressions + 2 flag+arm unit tests. ⚠️ **Both sides must run `DecodeHeaders` first**: the per-stream `StreamContext` is built only there (`encode_headers.go:57-60` short-circuits on `f.streamCtx == nil`), which is what forces the shared setup helper. ⚠️ **Use `f.streamCtx.HasGlobalFunc(...)` as the liveness barrier — NOT the `executions` counter**, which is BROKEN-GATE SHAPE 26: `decode_headers.go:199-201` increments it *before* `CallProxyOnRequestHeaders` and the capability gate lives *inside* that call (measured `executions = 1` under zero capabilities). ⚠️ Add a `t.Cleanup` force-release, or every RED run leaks a parked dispatch goroutine for the rest of the package.
- [ ] **Step 5: Run RED and capture the four failures separately** — they fail for four different reasons and one aggregated FAIL hides which.
- [ ] **Step 6: Implement** `f.beginDecodePause()` at the decode arm and `f.beginEncodePause()` at the encode arm, with the comment recording that the park is otherwise **permanently unresumable** (the guest's `proxy_continue_stream` CAS loses because the flag was never set, and there is **no stream idle timeout anywhere in the tree**).
- [ ] **Step 7: Run the caps NC** — strip the two capabilities and confirm all four tests fail **at the barrier** rather than passing vacuously. Restore.
- [ ] **Step 8: Full package. Commit.**

---

## Task 5 — S4: the census replacement, **RE-INVERTED**, with per-arm isolation

**Files:** Create `internal/filter/http/wasm/p83_census_test.go`; delete `TestFilter_Pause_CensusOfHonoredArms` from `pause_test.go`.
**Measured cost: 280 test lines (not 113 — the pre-S1/S2 figure does not survive re-inversion).**

- [ ] **Step 1: Reproduce the vacuity by BREAK, and record it.** Flip `body.go:227` to `DataContinue`: the existing census test **passes**, and the full package passes at `445 PASS / 0 FAIL`. Flip **all four** frozen arms: still `445 PASS / 0 FAIL`, **which assertion fired: NONE.** ⚠️ **And it is wider than the package — under the four-arm break, 30 packages all pass**, including the chain itself and `test/conformance/proxy-wasm`.
- [ ] **Step 2: Write the replacement asserting the POST-fix contract** — `decodePaused`/`encodePaused` become **true** on all six arms. ⚠️ **The 113-line pre-S1/S2 version asserts they stay FALSE; S1/S2 invert that contract, so a kept-not-flipped version ships a self-contradicting gate.** It needs a 6-callback guest module, the new type-section entry, both trailers caps, and a per-arm setup helper with a cap liveness barrier.
- [ ] **Step 3: Prove per-arm isolation with TWELVE breaks — two flavors per arm.** ⚠️ **Flavor A (flip the returned status) never trips the flag/watchdog assertions, and flavor B (drop the `begin*Pause` call) never trips the status assertion. A single-flavor roster leaves the arming unproven.** Each of the 12 must fail **exactly one** subtest — its own.
- [ ] **Step 4: Full package. Commit.**

---

## Task 6 — S8: the encode-cap park and the two sibling parks the SPEC does not name

**Files:** Modify `internal/filter/http/types.go`, `internal/filter/http/chain.go`, `internal/filter/http/wasm/body.go`; Create `encodecap_park_test.go`; modify `chain_test.go`.
**Measured cost: 43 ins / 7 del production (9 executable lines); 197 new test + 14 in-place updates.**

- [ ] **Step 1: Add `DataTerminateStream` + `errStreamTerminatedByFilter`.** One enum value in `types.go`; one sentinel var and a `case` arm in **both** `RunDecodeData` and `RunEncodeData`. ⚠️ **Do not touch the `default:` arms** — they stay as the unknown-status guard. Verify the only two `FilterDataStatus` switch sites are the ones you edited. **No HCM change is required**: `serveOneRequest` (`connection.go:250-263`) already closes on any non-`errCloseAfterAction` error.
- [ ] **Step 2: Chain-level RED/GREEN for the new arm** (`chain_test.go`, **44 measured lines**), table over {decode, encode}, asserting `errors.Is(err, errStreamTerminatedByFilter)` and that it **never parks**. NC: delete either `case` arm and it fails on the `default:` unknown-status error instead of the sentinel.
- [ ] **Step 3: Write the three park anchors RED** (`encodecap_park_test.go`, **153 measured lines**): the encode cap, the encode sticky chunk, and the decode sticky chunk. ⚠️ **Assert the returned ERROR, not merely that the call returned** — the `DataContinue` variant returns nil and would pass. ⚠️ **Assert `encodeBodyCapExceeded` and the counter too**, or a green where the arm never fired reads as coverage. Use a `context.Background()`-derived ctx with no deadline so the probe models production, and `cancel()` in cleanup to reap the goroutine.
- [ ] **Step 4: Run RED.** Expected: `chain.RunEncodeData PARKED: did not return within 2s`. ⚠️ **The 2 s is the probe deadline, not the park — the park is UNBOUNDED, and only `cancel()` reaped it.**
- [ ] **Step 5: Change the three return statuses** and rewrite the comment. ⚠️ **The comment is false THREE ways** (§1.9): drop the phantom `StopAllIteration`, drop the false ADR-0071 attribution, and cite the **measured** reference disposition (`rs_too_large` + reset). ⚠️ **Sweep all 7 `StopAllIteration` occurrences across 3 files, not just S8's.**
- [ ] **Step 6: Update the 14 pre-existing assertions** that pin `DataStopIterationNoBuffer` (`body_test.go` ×12, `dispatch_test.go` ×2). **This is the line-count overrun no estimate counted — name it in the row.**
- [ ] **Step 7: Fix the `decoderCb == nil` decode-cap park** (`body.go:169-171`) — one line, latent (test-double reachable only) but proven RED. ⚠️ **Do not claim a live decode-side leak**: `beginLocalReply` sets `localReplyDone` synchronously and `RunDecodeData` checks it at `:406-408` **before** the status switch, so the production decode path is genuinely rescued.
- [ ] **Step 8: Full package + `-race`. Commit.**

---

## Task 7 — S5 + S6 + the `pause.go` doc surface, **ONE TASK, NON-NEGOTIABLE**

**Files:** Modify `pause.go`, `body.go`, `trailers.go`, `wasm.go`, `decode_headers.go`, `abi_callbacks.go`.
**Measured cost: 144 ins / 90 del (+54 net) across 6 files.**

⚠️ **This cannot be split from Tasks 2-4's `pause.go` edits** (§1.7): S6's derivation rewrite sits *inside* S5's rewrite region, and S5's header rewrite describes behavior S1/S2/S3 introduce. The patches do not apply independently — **discovered by attempting it.**

- [ ] **Step 1: Rewrite all SIXTEEN S5 sites** (not eleven — the five the SPEC misses are `body.go:25-28`, `trailers.go:154-157`, `wasm.go:218-222`, `body.go:41-42`, `trailers.go:24-25`). ⚠️ **`pause.go:112-117`'s "zero of the 35 guest crates return Action::Pause from proxy_on_response_headers" is VERIFIED TRUE — do NOT "fix" it.**
- [ ] **Step 2: Correct the four `pause.go` sentences this stage refuted** — `:19-21` (the INVERTED `0036` (n) dependency, the entire stated basis for the freeze), `:25-28` (the false "ZERO of the 35 guest crates" mitigation), `:59-62` (the unsound LOWER bound), and ⚠️ **`:72-73`, a NEW site: (l)'s result is NOT discarded, (l) IS cross-side as of phase 82, and the driver asserts `http_call_response >= 1` and `http_call_response_after_close == 0`** — so the "not a verdict change" conclusion no longer follows.
- [ ] **Step 3: Rewrite the S6 derivation.** **10 s stays; the derivation goes.** Replace the two-MEASURED-CONSTRAINTS paragraph with what is true: *any value in roughly [1 s, 90 s) is indistinguishable to the current corpus; 10 s comfortably exceeds the 5 s `defaultHttpCallTimeout` convention and stays well inside the differential's 90 s per-fixture budget* (`runner_test.go:237` — **not** the 15 s client timeout the comment cites). ⚠️ **Presenting it as "pinned between two MEASURED constraints" is the failure mode being removed, not the number.**
- [ ] **Step 4: Correct `abi_callbacks.go:854-856`**, which claims `ContinueStream` resumes after `DataStopIterationAndBuffer` — precisely the arm that sets no flag. Confirmed one layer up: `ContinueDecoding` calls actually fired = **0** while `ContinueStream` returned `Ok`.
- [ ] **Step 5: `gofmt` + `golangci-lint` with the misspell liveness proof.** ⚠️ **This task is comment-only and therefore NOT reddenable by construction** — say so in the commit; do not claim a test proves it.
- [ ] **Step 6: Commit.**

---

## Task 8 — S7: de-hardcode the test port, **documented variant**

**Files:** Modify `internal/filter/http/wasm/http_call_response_cache_test.go`.
**Measured cost: 19 ins / 31 del, net **−12**.** *(Minimal variant measured 12/30, net −18. The SPEC's −17/−10 are each ~1-2 lines off; its **direction** — that the BRAINSTORM's −14 understates the reduction — is correct.)*

**Chosen: the documented variant.** Strip the rationale and the next editor re-introduces pick-close-rebind, because every other port helper in the tree is that shape. −6 lines is not worth re-opening the question.

- [ ] **Step 1:** Delete `const httpCallBackendPort = 42552` and the now-unused `func itoa(int) string` (14 lines; its only caller was the port formatting).
- [ ] **Step 2:** `startHttpCallBackend(t) → int` using `net.Listen("tcp","127.0.0.1:0")` + `ln.Addr().(*net.TCPAddr).Port`, holding the listener. ⚠️ **Do NOT copy `freeTCPPort`** — all three live definitions are pick-close-rebind, which exists only because a **subprocess** binds. This binds **in-process**: there is no race window at all.
- [ ] **Step 3:** Thread the port through `mkHttpCallClusterMgr(t, name string, port int)`. ⚠️ **Do not add `//nolint:gosec`** on the `uint32(port)` conversion — `gosec` is not among the 9 enabled linters, and removing the existing directive can leave the next line mis-indented and trip `gofmt`.
- [ ] **Step 4:** Verify `git grep 42552 -- internal/` ⇒ **zero hits**; the affected tests PASS ×3 at `-count=1`; full package green. ⚠️ **The TIME-WAIT half of the original justification is REFUTED** (three consecutive `-count=1` runs pass with TIME-WAIT accumulating 1→2→3, because `net.Listen` sets `SO_REUSEADDR`). **Only a live listener collides — and one did, across sibling agents, at the phase-83 SPEC. That is the justification: observed, not argued.**
- [ ] **Step 5: Commit.** ⚠️ Fully independent — land first or last; it unblocks parallel agents either way.

---

## Tasks 9-18 — the remaining ladder

| # | task | notes |
|---|---|---|
| **9** | The break roster, run against the **landed** code, **both flavors per arm** | §5 |
| **10** | `0036` (n) confirming re-run | ⚠️ **A NO-OP GUARD, NOT A RISK CHECK** — (n) never reaches the arm (§1.10). Wall time inside the measured **19.33-20.36 s** envelope. |
| **11** | Full 120-fixture differential | ⚠️ **Cannot go RED on this row** (§6). A green is evidence the row broke nothing, **not** that it works. |
| **12** | h2spec | **State your own denominator** — the phase-82 PLAN's "106 passed" does not reproduce. |
| **13** | `go test ./...` + `-race` as a second run | |
| **14** | Stat surface **+0**, asserted structurally | No `NewCounter`/`NewGauge` call site moves. |
| **15** | **ADR-0305 completion**: §Decision + §Consequences appended IN PLACE after the RETAINED italic footer, **no renumber, NO `---` separator** (`^---$` **stays 216**) | ⚠️ **Also owed: the in-place §Context/STATUS correction** — ADR-0305's own STATUS blockquote still carries *"the loose form returns 4 … ALL FOUR ARE FALSE POSITIVES"*, which **its own landing invalidated** (it is now **5**: four false positives **+ ADR-0305 itself**, the one true positive at `:17862`). Precedent: ADR-0297 ¶7/¶9 + ADR-0298, four §Context paragraphs corrected in place while completing the block. **Lead with what survives: the gate IS a false-positive gate and the strict form IS right; only the arithmetic moved.** |
| **16** | Row 83 `in-progress` → `done`, **IN PLACE**, `status` field only | `want` **stays 115**; the diff is exactly one line. |
| **17** | Sentinel re-run + NCs; `STATE.md` rolled IN PLACE; `STATE_HISTORY.md` eviction | ⚠️ **Use the TOLERANT archive-absence guard** (§1.11) — the prescribed one is fail-unsafe. |
| **18** | `PROGRESS.md` IMPL record, `next-prompt.txt` roll-forward (`git add -f`) | |

---

## 5. The break roster — **19 of 20 arms proven RED; the 20th proven NOT reddenable**

Every arm below was executed this stage. Restore verified by **SHA-256 comparison**, not by eye. Injection site **varied per arm**.

| # | arm | injection | assertion that fired | RED? |
|---|---|---|---|---|
| 1-6 | the six Pause arms, **flavor A** (flip the status) | `decode_headers.go:269`, `encode_headers.go:126`, `body.go` ×2, `trailers.go` ×2 | `armN status: … = 0; want …` — **exactly one subtest each** | ✅ ×6 |
| 7-12 | the six Pause arms, **flavor B** (drop the `begin*Pause` call) | same six | `armN flag: …` **+** `armN watchdog: …` — **exactly one subtest each** | ✅ ×6 |
| 13 | S1 `Continue`-path disarm | drop `clearDecodePause()` | `disarm flag: decodePaused = true after the guest Continued` | ✅ |
| 14 | S1 arm-once | revert to `beginDecodePause` | `decodePauseTimer moved 0x…150 → 0x…1c0 across two paused chunks` | ✅ |
| 15 | chain-level body park liveness | drop `beginDecodePauseFrom` | `the BODY pause arm was never reached` | ✅ |
| 16 | S8 encode-cap bound | drop the host-park bound | `RunEncodeData never returned: chain.go:598 parked, and NOTHING resumes it` | ✅ |
| 17 | S8 OnDestroy disarm | drop the teardown | `the S8 host-park watchdog survived OnDestroy` | ✅ |
| 18 | OnDestroy disarm (**existing** gate) | drop `encode_headers.go:165` | `a pause watchdog survived OnDestroy` | ✅ |
| 19 | S3 generation guard + `Stop()`, **STACKED** | drop both, with `_ = gen` / `_ = old` | `S3 fire-count: ContinueDecoding fired 38 / 35 / 34 …; want exactly 1` | ✅ |
| 20 | S5 watchdog log misattribution | revert `%s`+`cbName` to the hard-coded string | — | ❌ **NOT REDDENABLE** |

⚠️ **ARM 20 IS THE MOST VALUABLE ROW.** **Nothing in the package asserts any log string.** A gate *is* constructible — `decode_headers.go:98` declares `var logf = log.Printf`, so it is swappable — but none exists, and this PLAN does not charter one. **The IMPL must state that the log-string correction ships UNGATED rather than implying the roster covers it.**

⚠️ **THE THREE S3 SINGLE-ARM BREAKS ALSO DO NOT REDDEN** — the guard alone, `Stop()` alone, and the ordering (increment after `Store`) each stay GREEN. **Arm 19 is a STACKED break by necessity, and the PLAN says so rather than presenting it as an isolating one.** The ordering break in particular has a window of two adjacent statements and no probe built this stage hits it.

⚠️ **All S5/S6 comment rewrites are unreddenable by construction.** Say "comment-only, ungated", never "green".

---

## 6. Differential and fixture posture — **the differential is STRUCTURALLY BLIND to this row**

**Zero new fixtures.** `0036` is the only fixture repo-wide with a wasm body callback (4 `.rs` files, all in `0036`, denominator 25, firing control 25/25), and (n) never reaches the arm. **The differential cannot go RED on S1/S2/S3/S8.** The row's entire gate burden falls on unit tests.

⚠️ **The IMPL must not present a differential green as coverage** (`reference_liveness_break_needs_failing_baseline`).

Baselines are **inherited from the phase-82 IMPL and are NOT claimed by this docs-only stage**: 120/120 in 388.961 s; `0036` alone 18.87 s (⚠️ the SPEC measured **19.33-20.36 s** on the subtest with ~1.03 s variance — **use the envelope, not the point**); h2spec 53/53/0/0 (one report, no reference arm). ⚠️ **Budget ~3 differential launches; capture `INNER_EXIT` and the panic arm — the run ABORTS, it does not merely fail.**

---

## 7. Band — **~1840-2080 net, budget ~1950, EIGHTEEN tasks. NO SPLIT — RECORDED AS A §6.1 CROSSING.**

| trigger | `BOOTSTRAP_PROMPT.md` | this row | fires? |
|---|---|---|---|
| tasks | `:289` ~25 | **18** | no |
| LoC | `:290` ~1500 | **~1840-2080** | ⚠️ **YES** |
| sub-steps mid-execution | `:292` ~10 | max 8 | no |

**Why no split** (§6.2 `:294`): the scope items are **coupled by measurement, not by preference**. §1.7 forces S1/S2/S3/S5/S6 to share `pause.go`; §1.3 forces S3 ahead of S1; §2.3's S9 fix lives inside S3's function family. **Any available split axis cuts through a correctness constraint.** The split machinery would also add a ROADMAP row (moving `want` off 115) and an ADR, for a row already fully specified. **This is the THIRD consecutive §6.1 crossing in this lineage and the third consecutive firing of `reference_measured_prototype_is_a_lower_bound` — recorded as a finding, not absorbed.**

---

## 8. Sentinel — re-run MECHANICALLY at this stage. It does **NOT** fire; `stop` was **NOT** created

Input measured **231 lines / 115 data rows** first, so an empty result cannot read as a zero result.

- **(1)** at `want=115` ⇒ **`NOT DONE: row 83`**, denominator printed (`examined 115 data rows`). **CORRECT AND EXPECTED** while the phase is open.
- **(2)** **FIVE — `:193 :203 :213 :219 :227`. UNCHANGED.** The **THIRTY-EIGHTH** consecutive phase without a decrease. ⚠️ **THIS ROW NARROWS NOTHING AND THAT IS STRUCTURAL** — the `### WASM host family` heading at `ROADMAP.md:221` owns **none** of the five anchors.
- **(3)** **`NEVER OPENED: gRPC` — ALONE.**
- ⇒ all three print; the condition is a CONJUNCTION; **the sentinel does not fire.** `ls stop` ⇒ `No such file or directory`.

**FIVE negative controls, ALL FIRED:**

| NC | result | fired? |
|---|---|---|
| row 62 doctored, `NC LANDED? [ in-progress ]` **inspected before the result was trusted** | `NOT DONE: row 62` alongside row 83 | ✅ |
| `want=114` against the real file | `GATE FAIL: examined 115 data rows, expected 114` | ✅ |
| check (3) invented slug | `NEVER OPENED: ZZZ-invented`; `WASM`/`HTTP-filters` correctly silent | ✅ |
| check (2) **ONE-ARM** strip | **5 → 4, NOT 5 → 0** | ✅ |
| check (2) **BOTH** arms stripped | 5 → 0 | ✅ |

**Leak check:** this PLAN writes **no ROADMAP cell**. `git diff --stat master -- docs/envoy-go/ROADMAP.md` is **EMPTY**. ⚠️ Writing the four matcher strings *here* is safe; writing them into `ROADMAP.md` is not — confirmed by positive control at the SPEC (a `ROADMAP.md` copy moved check (2) **5 → 7** and made check (3) go **SILENT — a false PASS**).

---

## 9. Counts at this tip — re-derived mechanically, each with a control

`DECISIONS.md` **17892** lines · **304** `^## ADR-` headings, ids **0001-0305**, ⚠️ **exactly one gap, after ADR-0208** ⇒ `next-free = headings + 1` **COLLIDES; do not "fix" it** · tail **ADR-0305 PROPOSED**, next-free **ADR-0306** · block bounded by the next heading of **any** id: **33 lines**, 1 `### Context` / 0 `### Decision` / 0 `### Consequences` / **13** `**§Context ¶N` / 1 retained footer (NCs `ADR` 5, `wasm` 3) · `^---$` **216** · STATUS census **18** (⚠️ `^\*\*STATUS:\*\*` matches **0** and reads as a false zero) · **strict PROPOSED guard 1** at `:17862`; ⚠️ **loose form 5 — four false positives at `:17466 :17534 :17600 :17662` PLUS ADR-0305 itself.**

`ROADMAP.md` **231 lines / 115 data rows** (⚠️ **state which regex**: `^\| *[0-9]+(\.[0-9]+)? *\|` gives **113**, the plainest `^\| *[0-9]+ *\|` gives **84**) · `WASM-family row` ×2 at lines 144, 145 · `### WASM host family` at `:221`.

`BEHAVIOR_CONTRACT.md` **5900**, max cited line anywhere **5078** ⇒ a tail append shifts zero cites · `STATE.md` **63** · `STATE_HISTORY.md` **450**, ⚠️ **175 `prior active-phase` + 1 head `active-phase` at `:23` = 176 — and the prescribed guard reads 163 (§1.11).**

fixtures **120** (faithful `^[0-9]{4}[a-z]?-`; bare `^[0-9]{4}-` gives **118**; `ls -d` also **120**) · fuzzers **55** (`-- '*.go'`-scoped) · phase dirs **124**, **37** with `REVIEW.md`, **87** without · Pause-arm census **6**, symbol-anchored, no drift · CI `go-version '1.23'` ×3 (`:14 :39 :92`), `golangci-lint-action@v6.5.2` + `v1.64.8` (`:21-23`), `-timeout 20m` (`:60`), `timeout-minutes: 30` (`:63`), per-fixture budget **90 s** (`runner_test.go:237`).

⚠️ **CARRIED DELIBERATELY WITHOUT A NUMBER:** the `ROADMAP.md:<line>` and `BEHAVIOR_CONTRACT.md:<line>` cite counts, the `dispatchHttpCallResponse` carrier count, `allCallbacksNoOp` occurrences, `WASM-family row` occurrences repo-wide, and ⚠️ **the `Stop()`-only fire count (§1.2, contested between two agents)**.

---

## 10. Gates — a docs-only PLAN owes (a)-(f) as a POSTURE STATEMENT, not a green

- **(a)/(b)** differential: **NOT RUN** at this stage. Inherited baselines named in §6; the IMPL owes them.
- **(c)** conformance: **INHERITED**, not run. ⚠️ **When the IMPL runs it, STATE THE DENOMINATOR: 10 of the cpp-host's 16 families (62.5%), 6 deferred.**
- **(d)** fuzzers: **VACUOUS** for a row that adds none — say "vacuous", not "green". **55**, `-- '*.go'`-scoped (unrestricted gives 161).
- **(e)** `go vet` / `golangci-lint` / `go test ./...`: run by the agents on their prototypes, **not claimed by this docs-only stage** (no `.go` is committed here). Agent-side: `gofmt -l` **empty, gated on OUTPUT**; `golangci-lint` **exit 0, 0 lines**, with the **misspell liveness proof** performed and byte-identical restore verified by checksum at four of five agents.
- **(f)** `REVIEW.md`: ⚠️ **STANDING LINEAGE DEPARTURE — none since 25.3; 87 of 124 phase dirs carry none.** Named, not claimed.

---

## 11. Deferred — named so no later stage re-derives them

- **The residual chain-layer stale-token hazard** (§2.3) — filter-local mitigation adopted; the general case is **proven** unfixable filter-locally.
- **Routing the pause watchdog through the `clock.Clock` seam** (§2.5 option c) — the right long-term answer to the `-race` blindness; blocked on `clock.Clock` having no `AfterFunc`.
- **`chain.go:490-491`'s encode-cursor reset** — a latent `FilterChain`-API trap, not production-reachable (§1.3).
- **A trailers-paused guest is BLIND** — `abi_callbacks.go:223-225` routes map types 1/3/4/5 to `default`. ⚠️ **And there is a SECOND independent cause the SPEC does not name: `trailers.go` never captures the trailers at all** (`_ = trailers` at `:101`/`:165`; the documented `requestTrailers`/`responseTrailers` fields **do not exist** — the string occurs once in the whole package, inside a comment). **Fixing the routing alone would still hand the guest an empty map. Any future row must price TWO seams.**
- **`ContinueStream` cannot report a lost resume** — ABI-visible, needs its own row.
- **The four false non-test trailers-seam comments** — out of this row's packages; `router.go:284-285` asserts a call that never happens, so "fixing" it would paper over `Run*Trailers` being dead production code.
- **`internal/wasm/doc.go:219`'s two errors**, **`root_abi_callbacks.go:43`'s nonexistent `allCallbacksNoOp`**, **`ROADMAP.md`'s five `github.com/esalaine/…` cites** (INVARIANT-BLOCKED) — recorded, not fixed.
- **A `logf` swap gate for operator-facing log strings** (§5 arm 20) — constructible, not chartered.

---

## 12. Gate hygiene — the broken-gate count is **TWENTY-EIGHT**; TWO new shapes landed here

**THE TWENTY-SEVENTH: A FIRE-COUNT GATE WHOSE DISCRIMINATION DEPENDS ON AN UNPINNED PACER.** Two agents measured opposite results for the same cell of the same A/B because one used `time.Sleep` (with a ~1 ms host floor) and one did not; a third configuration read **A=291 / B=298** — green on both arms. **The gate reads as rigorous — 300 generations, a controlled A/B — and is vacuous unless the pacer, the watchdog magnitude and their ratio are all pinned and the RED arm is re-proven at that configuration.** (§1.2)

**THE TWENTY-EIGHTH: AN ARCHIVE-ABSENCE GUARD BLIND TO ITS OWN FILE'S LABEL VARIANTS.** `^- \*\*prior active-phase:\*\*` misses the **12** annotated-label entries — all of them the four most recent phases' evictions. The guard exists to prove an entry is *absent* before appending, so the miss is **FAIL-UNSAFE**: it authorizes a duplicate append. Found by the controller's own count disagreeing with the router's, then discriminated with a real-target cross-product after a first probe used an invented phase name and read 0 on both arms for the wrong reason. (§1.11)

**The twenty-six carried forward, unchanged:** a liveness barrier UPSTREAM of its gate · a counter that proves a callback RAN cannot prove it returned the RIGHT ANSWER · a replacement gate that INHERITS the defect it replaces · a gate that FIRES ON ALMOST EVERY ROW reads as thorough and is worthless · a guard is a claim at TRANSCRIPTION time · two defects that CANCEL in the gate metric · an inert gate cell · a full-suite recipe without `-v` is VACUOUS · a sha256 roster desynced against a DELETED file · `gofmt -l` NEVER exits non-zero · `go doc -all <A> <B>` swallows arg2 · a `+0 exported symbols` gate over an EMPTY package reds on a CORRECT tree · a RANGE gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` exits 0 on a DELETED file · a count-only stat guard PASSES a build with BOTH names wrong · a `-run` no-match exits 0 with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests exceeds the fixture denominator · a stat-delta claim cannot be discharged by guards scoped to another package · a stderr-VOLUME assertion passes on the hang · `golangci-lint` misspell locale US · a harness's exit code is not the command's · a GOLDEN ROSTER that omits the family under test · a gate metric INVARIANT under the change it guards · a hand-written ENUM golden pinning WRONG wire values · a stat COUNT guard blind to a RENAME · a `t.Fatalf` liveness precondition that makes the hazard assertion DEAD CODE.

---

## 13. Self-review against the SPEC

- **§17 item 1** (§6.1 against a measured test side) → §1.1, §2.1, §7. **The trigger CROSSES.** ✅
- **§17 item 2** (ordering, guard before or with S1/S2) → §2.2, Task 2 before Task 3. **"BEFORE" collapses to "WITH"** on measurement. ✅
- **§17 item 3** (stale-token disposition) → §2.3. **Priced filter-local fix adopted (6 lines); residual PROVEN unfixable filter-locally and deferred BY NAME.** ✅
- **§17 item 4** (S8 remedy with the reference cited) → §2.4. **Option A, with a four-arm live measurement against the pinned image.** ✅
- **§17 item 5** (break roster proven RED first) → §5, Task 9. **19 of 20 RED; the 20th proven NOT reddenable and named.** ✅
- **Scope S1-S9** → Tasks 2-8. All nine covered; S8 widened from 1 park to 3; S5 widened from 11 sites to 16. ✅
- **§15 hazards 1-10** → 1 (synthetic anchor) Task 3 step 1; 2 (caps) Task 4 step 3; 3 (`executions`) Task 4 step 4; 4 (`-race`) §2.5 + Task 1 step 5; 5 (`config.name`) §3; 6/7 (port, `freeTCPPort`) Task 8; 8 (re-inversion) Task 5 step 2; 9 (chain-level coverage) Task 3 step 2 + Task 6 step 2; 10 (`gofmt`/misspell) §3. ✅

**Placeholder scan:** no `TBD`, no "similar to Task N", no "add appropriate error handling". Every code step carries real code. **Type consistency:** `decodePauseGen`/`encodePauseGen`, `beginDecodePauseOnce`/`disarmDecodePause`, `DataTerminateStream`/`errStreamTerminatedByFilter` used identically in every task that names them.

---

## 14. NEXT

**IMPL** for phase 83, per the one-stage-per-session discipline. It owes: the eighteen tasks in order; the break roster with **both flavors per arm**; ADR-0305's completion **plus the in-place correction of its own now-stale STATUS sentence**; row 83 → `done` in place; and the sentinel with its NCs.
