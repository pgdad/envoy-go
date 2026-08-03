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
