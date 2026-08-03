# PROGRESS 83 — wasm-pause-arm-leak

Append-only stage record. Newest stage last.

---

# BRAINSTORM record (2026-08-03)

**Stage:** BRAINSTORM (lifecycle-state `DONE` -> `1`). **ROW 83 REGISTERED `in-progress`**, and the sentinel `want` bumped **114 -> 115** in the SAME commit. Base master **`8a0126d2`** (from `git rev-parse master`), branch `phase-83-brainstorm`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

## What landed

`docs/envoy-go/phases/83-wasm-pause-arm-leak/{BRAINSTORM.md,PROGRESS.md}` NEW · `ROADMAP.md` **+1 row** (230 -> 231) · `STATE.md` rolled **IN PLACE** (ADR-0288) · `STATE_HISTORY.md` **+1 evicted entry** · `next-prompt.txt` rolled forward. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED** — ⚠️ **no ADR is added at a BRAINSTORM**; ADR-0305 §Context is the SPEC's job.

## Method

**SELF-PICKED** per the 2026-07-12 standing directive — no banked mid-lifecycle work existed at this tip. **FIVE investigation agents** on disjoint remits, each in its own **detached** worktree with private scratch and a private port band inside `43000-43399`. Every load-bearing claim was re-derived by the controller; **two agent claims did not survive that re-derivation** (BRAINSTORM §5.2/§5.3).

## The stage's headline

**The row the previous phase landed to stop a leak stopped it in two of six arms, and the evidence that the other four were safe was falsified by the commit that wrote it.**

Phase 82's S9 armed the watchdog and set the resume flag in the two **headers** arms. The four remaining `abi.ProxyActionPause` arms return statuses that **park** the goroutine (`chain.go:430/474/569/635`) while arming nothing — the S9 leak verbatim. Proven by execution: a synthetic Pause-from-body guest stayed parked 1 s past a 250 ms watchdog window, `ContinueStream` returned `WasmResultOk` while resuming nothing, and the stream released only after the flag was hand-set.

⚠️ **The recorded mitigation — *"ZERO guest crates call any resume hostcall"* — is FALSE, and the call was added by `fb93845f`, the phase-82 IMPL commit that wrote the claim.** It is false a second and more important way: reaching the leak needs only a **Pause**, and three vendored guests (`a_body_read_only`, `b_body_mutate_passthrough`, `c_body_mutate_replace`) Pause from `on_http_request_body` and never resume.

⚠️ **Controller-level synthesis neither agent reported alone: the LIVE blast radius is TWO arms, on THREE protocol paths.** `Run*Trailers` has **zero** non-test call sites; `Run*Data` has **eight**, across `connection.go` (H1), `h2dispatch.go` (H2) and `h3dispatch.go` (H3). The trailers arms are latent — and the thing that would activate them is a measured in-tree prototype (below), so they are fixed in this row rather than left as a tripwire.

## Refutation count: **TWENTY-ONE**, of which **EIGHT are load-bearing**

Load-bearing: the false resume-hostcall mitigation (twice over) · the two-arms/three-paths blast radius · half the derivation of `defaultPauseWatchdog` is void (`pause.go:65-70` says scenario (l) never resumes; lines 40-43 of that guest do) · a **second production defect the fix itself creates** if written naively (the timer handle is reassigned without stopping the previous one, `pause.go:107-109`) · **BROKEN-GATE SHAPE 25** — `TestFilter_Pause_CensusOfHonoredArms` is a tautology over package constants while its docstring insists *"this is a behavioral assertion, not a grep"*, and it is the named guard against exactly the edit this row makes · **the gRPC HARD-BLOCKED verdict is REFUTED BY A WORKING PROTOTYPE** · the `TIME-WAIT` half of the hardcoded-port claim does not reproduce.

Also refuted (13), including: `*RootVM.dispatchHttpCallResponse` **does not exist** (cited in three landed code comments; the symbol is `handleHttpCallResponse`) · a **fourth** false non-test comment about the trailers seam (`router.go:285`) · gRPC **error** RPCs already work unpatched · the gRPC filter type-URL denominator is **eight**, not seven · `ssl.curves` is charset-**blocked**, not passing · `ssl.sigalgs` **is** emitted (under mTLS) but is a **Go framework gap**, not a naming gap · all four dynamic `ssl` arms diverge cross-side because the reference carries the value in a **label** · `ssl.curves` needs **Go >= 1.25** while CI pins 1.23 · `initial_fetch_timeout` is **already fully landed** · the Runtime item's `override_dir` names a field that **does not exist** in the pinned dep · the Operational-tooling cell cites the **wrong module path**.

## The pick, and the one that got away

**PICKED `wasm-pause-arm-leak`** on the smallest-first rule: smallest candidate that fixes a live defect, highest severity per line, completes a charter the previous row opened and left half-done, and needs **no new blob, fixture, port, BackendKind or toolchain**.

⚠️ **The strongest candidate in the project was rejected on cost alone, and the next session should know why.** `grpc-unary-interop` would retire the sentinel's **last** check-(3) failure. Its inherited HARD-BLOCKED verdict is **dead**: it rested on a grep (*"`RunEncodeTrailers` has zero non-test callers"*) that is true but was never converted into a cost. Measured — **+92/−11 across 5 production files**, `go vet ./...` clean, 3 packages green, and a real `grpc-go` client through envoy-go to a real health server goes from `Internal: server closed the stream without sending trailers` to **`SERVING`**. Re-derived at **12-16 tasks**, not 16-22+. ⚠️ **What genuinely stays blocked is NOT trailers:** an H2-upstream cluster from an H1 downstream listener returns a **measured 502**, and the proxy **fully buffers the response body** (server-streaming first `Recv`: **5 s / DeadlineExceeded** subject vs **0 s / nil** on both control and reference). Those two are the family's real ceiling.

## Gates — a docs-only BRAINSTORM owes (a)-(f) only in the posture a docs-only stage can have

(a)/(b) no fixture changed, no `.go` committed — **inherited, not re-run and not claimed**. (c) proxy-wasm **inherited**; the denominator when a stage does run it is **10 of 16 cpp-host files (62.5%), 6 deferred**. (d) **VACUOUS** — no fuzzer added (**55**, `-- '*.go'`-scoped). (e) `go.mod`/`go.sum` byte-untouched. (f) `REVIEW.md` **ABSENT — the standing lineage departure**, named: **86 of 123** phase dirs carry none.

## Sentinel

Input measured **230 lines / 114 data rows** before anything was written. **(1) SILENT** at `want=114` with the denominator printed — ⚠️ **and because silence is now indistinguishable from a broken check, THREE negative controls were run and ALL THREE FIRED** (row 62 doctored ⇒ `NOT DONE: row 62`; row 82 doctored back ⇒ `NOT DONE: row 82`; `want=113` ⇒ `GATE FAIL: examined 114 data rows, expected 113`). **(2) FIVE, unchanged — the thirty-sixth consecutive phase without a decrease; this row narrows nothing, stated not forecast.** **(3) `NEVER OPENED: gRPC` — alone**; NC on an invented slug fires. `stop` **NOT** created and must not be — the condition is a conjunction and (2) and (3) are live.

**Leak check passed:** row 83's summary carries neither of check (2)'s fail-strings, and row 82's field 7 — the sole repo-wide carrier of check (3)'s `WASM-family row` literal — is **byte-untouched**.

⚠️ **Cite-shift enumerated BEFORE the edit:** the insert shifts **exactly 38 of 117** `ROADMAP.md:<line>` cites (79 safe; NC at insertion-point-1 gives 117), and **all 38 sit at >= 184**, so the five check-(2) anchors move **`:192 :202 :212 :218 :226` -> `:193 :203 :213 :219 :227`**. Renumbered in this stage rather than left for the next.

## Handoff

**SPEC.** It drafts ADR-0305 §Context and must answer four open questions — ⚠️ **question 1 (does arming a 10 s watchdog change `0036` (n)'s timing, given that scenario's whole point is indefinite accumulation) BY MEASUREMENT, not by reasoning.** Band **350-600 net `.go`, budget ~450, 6-8 tasks**, declared explicitly as a **LOWER BOUND**: `reference_measured_prototype_is_a_lower_bound` has fired on **two consecutive rows** (3.07x, 2.55x) with test scaffolding dominating both, and this estimate is again production-measured with a modelled test side.

---

# SPEC record (2026-08-03)

**Stage:** SPEC (lifecycle-state `1` -> `2`). **ROW 83 STAYS `in-progress`**; `ROADMAP.md` is **BYTE-UNTOUCHED** and the sentinel `want` **STAYS 115**. Base master **`5d0892e4`** (from `git rev-parse master`, **not** the `8a0126d2` the router's prose still names), branch `phase-83-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

## What landed

`SPEC.md` NEW · `DECISIONS.md` **17858 -> 17892** (ADR-0305 §Context, STATUS **PROPOSED**) · `PROGRESS.md` +1 stage record · `STATE.md` rolled **IN PLACE** (ADR-0288) · `STATE_HISTORY.md` +1 evicted entry · `next-prompt.txt` rolled forward. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**.

## Method

**FIVE investigation agents** on disjoint remits, each in its own **DETACHED** worktree off `5d0892e4` with private scratch and a private port band inside `43400-43799`. Every load-bearing claim was re-derived by the controller; **one agent claim did not survive that re-derivation**, and **one controller hypothesis did not survive an agent's measurement** (SPEC §2.2, §2.5).

## The stage's headline

**The defect is real and the row's own principal risk measures ZERO — because no guest in this repository reaches the broken arms at all, and the fix as chartered is incorrect.**

Instrumenting `body.go:226` across five full `0036` runs yields **ZERO fires**, with two firing positive controls. Three measured reasons: (n) trips the cap 82 lines before the arm; (a)/(b)/(c) Pause only on non-final chunks; and ⚠️ **(a)/(b)/(c) are not driven at all** — `driver.go:511-513` emits a constant skip token, so the whole fixture produces **one** body chunk. ⚠️ **This refutes the BRAINSTORM's load-bearing *"three vendored guests already do it"*: true of source, false of execution.** ⇒ **the failing-first anchor must be SYNTHETIC, and the differential is structurally blind to the row.**

⚠️ **AND THE NAIVE FIX IS WRONG.** `DecodeData` runs once per chunk, so `beginDecodePause` re-arms per chunk and the final-chunk `Continue` neither clears the flag nor stops the timers. ⚠️ **`Stop()`-before-reassign — the BRAINSTORM's entire S3 — is INSUFFICIENT:** over 300 superseded generations, `Stop()` alone fires **299** spurious resumes, `Stop()` + a generation counter fires **1**. ⚠️ **`-race` reports nothing** — it is a logical race.

⚠️ **A SEVENTH UNBOUNDED PARK, in the same file, that a census of six does not cover:** `body.go:282`'s encode-side cap returns `DataStopIterationNoBuffer` with no local reply, and the encode chain has no `localReplyDone` counterpart.

## The four open questions, disposed

1. **D-83-CAP — ZERO RISK, BY MEASUREMENT.** `0036` with the fix: **19.25 s** vs a 19.33-20.36 s baseline, inside a 1.03 s variance, **zero watchdog fires**; a **10 s -> 1 ms** sweep passes at every value. Cross-product both ways: raising (n)'s cap makes the arm fire and release at **exactly 10.0 s**; removing the fix makes it park **15 s**.
2. **D-83-TRAILERS — FLAG AND ARM.** The alternative fails on its own terms: the current behavior is a **permanent unresumable park**, and ⚠️ **there is no stream idle timeout anywhere in the tree**. A one-line change flips a RED probe GREEN, so an anchor exists despite zero production callers.
3. **D-83-WATCHDOG — 10 s STAYS, THE DERIVATION IS REBUILT.** ⚠️ **Both** stated constraints fall (the lower one too, which no prior document states). The corpus arms the watchdog **once**, for **747 µs**, released by the guest. Real ceiling: the differential's **90 s per-fixture budget**, not the cited 15 s client timeout.
4. **D-83-LOGHOME — ADR-0305 §Context, explicitly NOT chartered.** No shape has a failing-first anchor, and an ADR amendment is unavailable under ADR-0288's append-only rule.

**THREE NEW FORKS the BRAINSTORM never named:** D-83-TIMER (generation guard), D-83-STALETOKEN (a hazard this row widens threefold and does not close, deferred BY NAME), D-83-ENCODECAP (the seventh park).

## Refutation count: **TWENTY-THREE**, of which **NINE are load-bearing**

Load-bearing: no in-tree reproducer · the controller's own reframing was half wrong · `pause.go:19-21`'s stated (n) dependency is **inverted** · **both** watchdog constraints fall · the census guard is vacuous **and so is the whole package on that axis** · **BROKEN-GATE SHAPE 26** — a liveness barrier placed upstream of the gate it claims to prove, installed one row earlier *as the fix* for that same failure · **ADR-0106 contains no sole-leg rule** (`sole` 0, `leg` 0, NCs firing) · ADR-0304's own STATUS is wrong at the commit that wrote it · the `ROADMAP.md:<line>` cite count self-falsified a **FOURTH** time, inside the commit that named the species.

Also refuted: the trailers signature is `(i32,i32)->i32` so S2 needs a **new type-section entry** · `testLifecycleCapabilities` has **neither** trailers capability ⇒ silently vacuous S2 tests · `ContinueStream` discards its resume result by design · the watchdog log string misattributes the callback · `trailers.go`'s Pause arms had **zero** test coverage · `pause_test.go:435/436` are both `RunDecodeTrailers` · `doc.go:219` carries **two** errors · the 35-crate denominator is 25 fixture guests + 10 conformance crates · the sourceless blob is in **0039**, not 0036 · S7's diffstat is **9/26 net −17**, not 9/23/−14 · the TIME-WAIT justification does not reproduce.

## Gates — a docs-only SPEC owes (a)-(f) only in the posture a docs-only stage can have

(a)/(b) no fixture changed and no `.go` committed — **inherited from the phase-82 IMPL, not re-run and not claimed**. (c) proxy-wasm **INHERITED**; the denominator when a stage runs it is **10 of the cpp-host's 16 files (62.5%), 6 deferred**. (d) **VACUOUS** — no fuzzer added (**55**, `-- '*.go'`-scoped; unrestricted 161). (e) `go.mod`/`go.sum` byte-untouched. (f) `REVIEW.md` **ABSENT — the STANDING LINEAGE DEPARTURE**, named: **87 of 124** phase dirs carry none, 37 carry one, none since 25.3.

§6.1 (`:285`, triggers `:289`/`:290`, `:291` BLANK, third `:292`) — **neither fires at the revised budget**; see SPEC §13.

## Sentinel

Input measured **231 lines / 115 data rows** first. **(1)** `NOT DONE: row 83` at `want=115` with the denominator printed — correct while the phase is open. **(2) FIVE, unchanged — the thirty-seventh consecutive phase without a decrease; this row narrows nothing, and that is now STRUCTURAL** (the `### WASM host family` heading at `:221` owns none of the five anchors). **(3) `NEVER OPENED: gRPC` — alone.** `stop` **NOT** created and must not be.

**FIVE negative controls, ALL FIRED**, including the doctored-copy NC with `NC LANDED? [ in-progress ]` inspected first, and both check-(2) strip arms (**one-arm 5 -> 4, both-arms 5 -> 0**). ⚠️ **The one-arm result was re-confirmed accidentally and live** when a single-phrase matcher printed ONE of five anchors.

**Leak check VERIFIED BY DIFF** (`git diff --stat master -- docs/envoy-go/ROADMAP.md` EMPTY), and the sentinel's file scope was confirmed by **positive control**: the four matcher strings written into a scratch `SPEC.md` moved nothing, while concatenated onto a `ROADMAP.md` copy they moved check (2) **5 -> 7** and made check (3) go **SILENT — a false PASS**.

**Row well-formedness** re-executed over all 115 rows: ARM-A {119, 131}, ARM-B {140}, naive `NF!=8` **17** and **misses 140**. Row 83 clean on both arms; both arms NC'd with the doctoring verified to have landed — ⚠️ **a first NC attempt did NOT land and would have read as "the arm is blind".**

## Handoff

**PLAN.** It owes an explicit **§6.1 re-evaluation against a MEASURED test side** (this SPEC's is analogue-scaled — the exact failure mode of phases 81 and 82), a task ordering that lands the generation guard **before or with** S1/S2, a disposition for the stale-token hazard, a remedy choice for the encode-side cap, and **a break roster whose arms are proven RED first** — because the differential cannot go red on this row and the package's existing guard is vacuous. **Revised band 950-1400 net `.go`, budget ~1150, 12-16 tasks.**

---

# PLAN record (2026-08-03)

## What landed

`PLAN.md` **NEW** (18 tasks). `STATE.md` rolled **IN PLACE** (63 -> 63). `STATE_HISTORY.md` **450 -> 452**. `PROGRESS.md` +1 stage record. `next-prompt.txt` rolled forward. ⚠️ **`ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED; sentinel `want` STAYS 115; the strict PROPOSED guard STAYS 1.** Docs-only: ZERO production `.go`, ZERO test `.go`.

## Method

**FIVE investigation agents**, each in its own **DETACHED** worktree off `e4563a49`, private scratch, private port bands inside `43800-44299`. Unlike a review stage, **every agent wrote real code and ran it** — that was the point: the SPEC's ~1070-line test figure was analogue-scaled, and analogue-scaling is precisely what made phases 81 and 82 overrun by 3.07x and 2.55x. All five reported `git status --porcelain` = **0 lines** and removed their own worktrees. One agent created five docker containers and tore all five down **BY NAME** plus its network; no `prune` and no `ancestor=`/image filter at any point. Nothing committed, nothing pushed.

## The stage's headline

⚠️ **THE §6.1 LoC TRIGGER IS CROSSED ON MEASUREMENT — ~1840-2080 net against ~1500 — AND IT CROSSED A BAND THE SPEC HAD ALREADY REVISED UP 2.4x IN ANTICIPATION OF EXACTLY THIS.** Measured per group: S1 test **679** vs 371 (1.83x) · S2 **453** vs 333 (1.36x) · S3 **237** vs 195 (1.22x) · S4 re-inverted **280** vs 113 (2.48x) · S8 **197 + 14 in-place** vs 60 (3.3x) · production **~140-230** vs 63-90. ⚠️ **The finding is not "the estimate was low" but "the estimate was UNDER-ENUMERATED"** — the SPEC's §13 S1 row prices seven items while its own §7 mandates three more (arm-once + two disarms), which measure **190 lines, 28% of S1's test side, priced at ZERO**; S8's 3.3x is driven by **14 pre-existing assertions pinning the old status** that no estimate counted. **A ratio-rescale is the wrong repair; the item lists had to be re-enumerated, and were.** **Disposition: RECORD, DO NOT RETRO-SPLIT** — the lineage precedent at both prior crossings, and here the scope items are coupled by measurement (S5/S6 and the `pause.go` half of S1/S2/S3 **do not apply independently**; S3 must precede S1), so **every available split axis cuts through a correctness constraint.**

⚠️ **SECOND: THE SPEC'S 299-vs-1 A/B IS PACER-DEPENDENT AND TWO AGENTS MEASURED OPPOSITE RESULTS FOR THE SAME CELL** — 299 under `time.Sleep` pacing at pace == watchdog == 50 us, **1** under a spin pacer at the same real spacing, and **A=291 / B=298 (green on both arms)** at 0.2x. `time.Sleep` has a ~1 ms floor on this host. **NO NUMBER IS CARRIED.** What survives is structural: a fire-count gate must pin the pacer, the watchdog magnitude AND their ratio, and re-prove its RED arm at that configuration. Both agents nevertheless converge on `Stop()` contributing **zero** to the fire count (A≡D, B≡E) — confirming the SPEC's "resource hygiene, not correctness" and quantifying it at **0 vs 199 pending closures** over 200 pauses.

⚠️ **THIRD: S3 HAS NO LIVE DEFECT AND THEREFORE NO RED ANCHOR OF ITS OWN.** SPEC §1.2's "reachable today" is refuted — `decodePauseGen=1` after `RunDecodeHeaders` x2 + `RunDecodeData` x3 + `RunDecodeTrailers` x2, because `beginDecodePause` has one call site and `c.decodeIdx` is monotone. **The SPEC's stated widening is itself the S3/S9 conflation it warns against**: two wasm filters give each its own generation, so a sibling filter produces the STALE-TOKEN hazard, never the TIMER-OVERWRITE one. ⇒ **SPEC §17 item 2's "BEFORE or WITH" collapses to WITH.**

⚠️ **FOURTH: THE SPEC'S §7 SNIPPET HAS A HOLE, AND PLUGGING IT IS THE S9 FIX.** `stopPauseTimer`/`stopPauseWatchdogs` do not bump the generation, so an already-entered closure fires into a torn-down stream — **400/400**; with the bump, **0/400**. **6 production lines**, inside this row's file set. The residual chain-layer hazard is deferred BY NAME and now **PROVEN** unfixable filter-locally (the resume channels are unexported on `*FilterChain`; the filter holds an 8-method interface with no park-state predicate).

## Refutation count: **NINETEEN**, of which **EIGHT are load-bearing**

Beyond the four above: **CAS-first is catastrophic, not merely wrong** (0 fires, **299 LOST RESUMES**, chain parks forever) · **the S3 gate is NOT `-race`-safe** — with the correct fix loaded, `-race` reports 0 DATA RACE and **5 of 7 assertions FAIL** · **the encode-cap arm is GUEST-INDEPENDENT**, so the SPEC's blast-radius bound rests on the wrong fact · **the "seventh park" undercounts** — three more verified by execution, so S8 as chartered closes 1 of at least 3 · **setting `localReplyDone` would not rescue the encode side** (all six checks are decode-side) · **a watchdog `SendLocalReply` DEADLOCKS** · **a wrong wasm type-section entry INSTANTIATES FINE** and fails only at call time where the host fail-OPENs · **S5 is SIXTEEN sites, not eleven**, and `StopAllIteration` is a **phantom identifier declared nowhere** · **`pause.go:72-73` is wrong in both halves and (l) IS cross-side**, so S6's "not a verdict change" no longer follows.

⚠️ **AN AGENT CLAIM DID NOT SURVIVE:** A5 concluded the two S3 mechanisms are "compensating, not complementary." Not adopted — A4's independent 2x2 refutes it, and A4's *pacer* finding explains A5's numbers without either agent being wrong. **The synthesis neither reported alone is that the A/B is configuration-dependent.**

⚠️ **AND THE CONTROLLER'S OWN COUNT WAS WRONG, FOR A REASON THAT IS A LIVE DEFECT:** the router-prescribed archive-absence guard reads **163** where the truth is **175** — twelve annotated-label entries are invisible to it, all of them the four most recent phases' evictions. **It is FAIL-UNSAFE in its own direction** (it authorizes a duplicate append). Discriminated with a real-target cross-product ⚠️ **after a first probe used an invented phase name and read 0 on both arms for the wrong reason.**

## The five owed items, disposed

1. **§6.1 vs a MEASURED test side** — CROSSED; RECORD, DO NOT RETRO-SPLIT. 2. **Ordering** — WITH, not BEFORE; justified by measurement rather than the SPEC's refuted premise. 3. **Stale token** — 6-line filter-local fix ADOPTED inside S3; residual PROVEN unfixable and deferred BY NAME. 4. **S8 remedy** — **Option A, non-parking status**, with the reference MEASURED live across four arms: a cap breach draws an **immediate stream reset + `rs_too_large: 1`, never a stall**, while `stream_idle_timeout` bounds an indefinite pause separately — envoy-go has neither. `DataContinue` **rejected by measurement** (it silently disables the cap). 5. **Break roster** — **19 of 20 arms proven RED; the 20th proven NOT reddenable and named.**

## Gates — a docs-only PLAN owes (a)-(f) only as a POSTURE STATEMENT

(a)/(b) **NOT RUN, NOT CLAIMED** — inherited from the phase-82 IMPL. (c) proxy-wasm **INHERITED**; denominator when run is **10 of 16 (62.5%), 6 deferred**. (d) **VACUOUS** — no fuzzer added (**55**, `-- '*.go'`-scoped). (e) no `.go` committed here; agent-side prototypes were `gofmt`-clean (gated on OUTPUT), `golangci-lint` exit 0, with the **misspell liveness proof** performed and byte-identical restore verified **by checksum** at four of five agents. (f) `REVIEW.md` **ABSENT — the STANDING LINEAGE DEPARTURE**, named: **87 of 124** phase dirs carry none.

## Sentinel

Input measured **231 lines / 115 data rows** first. **(1)** `NOT DONE: row 83` at `want=115`, denominator printed — correct while the phase is open. **(2) FIVE, unchanged — the THIRTY-EIGHTH consecutive phase without a decrease**; this row narrows nothing and that is STRUCTURAL. **(3) `NEVER OPENED: gRPC` — alone.** `stop` **NOT** created. **FIVE negative controls, ALL FIRED**, including the doctored-copy NC with `NC LANDED? [ in-progress ]` inspected before the result was trusted, and the one-arm check-(2) strip moving **5 -> 4, NOT 5 -> 0**. **Leak check VERIFIED BY DIFF** — this PLAN writes no ROADMAP cell.

## Handoff

**IMPL.** Eighteen tasks in order. ⚠️ **Task 2 (S3 + the S9 fix) MUST precede Task 3 (S1)** — a correctness constraint. ⚠️ **The break roster needs BOTH flavors per arm** (status-flip never trips the flag assertions; drop-the-call never trips the status assertion). ⚠️ **Two break instructions do not compile as written** (`_ = gen` / `_ = old` substitutes required). ⚠️ **ADR-0305's completion also owes the in-place correction of its OWN now-stale STATUS sentence** — its "the loose form returns 4, ALL FOUR FALSE POSITIVES" was invalidated by its own landing (it is now **5**: four false positives **plus ADR-0305 itself**). ⚠️ **The differential CANNOT go RED on this row; a green is evidence the row broke nothing, not that it works.**
