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
