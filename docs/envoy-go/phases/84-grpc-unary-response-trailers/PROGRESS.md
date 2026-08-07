# PROGRESS — phase 84 (grpc-unary-response-trailers)

## BRAINSTORM — done 2026-08-06

## What landed

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `docs/envoy-go/phases/84-grpc-unary-response-trailers/BRAINSTORM.md` (new) · `ROADMAP.md` **row 84 registered `in-progress`** plus the `**FAMILY OPEN at phase 84**` paragraph (231 -> 234 lines, 115 -> 116 data rows) · `next-prompt.txt` sentinel **`want` 115 -> 116 in the SAME commit as the row** · `STATE.md` rolled **IN PLACE** (9 insertions / 9 deletions, 64 lines unchanged) · `STATE_HISTORY.md` **454 -> 456**, verified STRICTLY APPEND-ONLY. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; this stage adds **NO ADR**.

## Method

**SELF-PICKED** per the 2026-07-12 standing directive; no banked mid-lifecycle work existed (phase 83 CLOSED, row 83 `done`). **FIVE investigation agents** on disjoint remits, each in its own **DETACHED** worktree with private scratch and a private port band inside `44300-44799`. Every agent reverted its probes and confirmed `git status --porcelain` = **0 lines**; docker containers torn down **BY NAME** (`a2-ref-grpc`, `a2-ref-grpc-tls`, `a3-ref`), never by an `ancestor=`/image filter. `go.mod`/`go.sum` untouched — **grpc-go v1.70.0 was already a direct require**.

## The stage's headline

**THIS IS THE FIRST ROW IN ~40 PHASES TO MOVE A SENTINEL CHECK.** It opens `gRPC`, the last `NEVER OPENED` family, so **check (3) goes SILENT** — and the honest family-open paragraph takes **check (2) 5 -> 6**, because a newly-opened family with a real deferred backlog records it in the phrase the sentinel matches. Wording around the matcher would be `reference_sentinel_matcher_string_self_clears` committed deliberately; the row declines. Precedent re-derived rather than assumed: the phase-77 BRAINSTORM opened Runtime in ONE commit moving check (2) **4 -> 5** and its slug **0 -> 2**.

Two further headlines: the inherited prototype cost is refuted **downward** (**4 files / +60−14**, not 5 files / +92−11), and a **live production protocol defect no document names** — CONTINUATION frames discarded at `internal/filter/hcm/h2/conn.go:255-259` behind a comment whose two clauses are both false, inside an h2spec section already reported green.

## Refutation count: **TWENTY-TWO**, of which **TEN are load-bearing**

Load-bearing: the seam confirmed live 3/3 on the harness-legal shape with two firing positive controls · the prototype refuted downward to 4 files · the CONTINUATION discard · **the WASM seams measured DECOUPLED for the minimal fix and COUPLED the moment `RunEncodeTrailers` is wired** (variant B printed the full chain walk; variant A printed **0 lines** as the negative control) · **every subject stat GREEN while the RPC fails** (`upstream_rq_2xx: 2` after two failures) · `RunEncodeTrailers` zero callers with the dead subtree deeper than recorded · both ceiling blockers proven outside the carve with their discriminators run · request trailers absent from the gRPC path with a **firing** NC · error RPCs already passing via a **Trailers-Only** response · the eight gRPC filter type URLs unregistered with a discriminating positive control.

⚠️ **Two agent claims the controller did NOT accept:** the second `BOOTSTRAP_PROMPT.md` copy was reported nonexistent (it is live — **1024** lines, offsets **NOT** a constant shift, §6.1 Δ+197 / §7.5 Δ+228), and the stat-surface absolute needed its own derivation (**1207**, not the **1205** `STATE.md:33` carries).

⚠️ **The controller's own brief was wrong twice** — it sent agents to `internal/hcm/` (the package is `internal/filter/hcm/`) and to `internal/wasm/abi_callbacks.go` (the file is `internal/filter/http/wasm/abi_callbacks.go`, and **the router's prose carries the same wrong path**). **A controller brief is a claim too.**

## Counts corrected against the router and STATE.md

- **`STATE_HISTORY.md` was 454, not 455; `STATE.md` was 64, not 65.** The real phase-83 transition was **452 -> 454**. The **+2 delta and the append-only property are correct**; both absolute endpoints were wrong in two documents.
- **stat surface 1207**, not 1205 (`STATE.md:33` stale since phase 76 — **deliberately NOT fixed**, per the standing "do not source from §Project" rule).
- **`-family row` is 93 occurrences / 65 LINES.** The router's `65` is a `grep -c` line count. **State which form.**

## Gates — a docs-only BRAINSTORM owes (a)-(f) only in the posture a docs-only stage can have

(a)/(b) **not exercised** — zero `.go` changed, no fixture added or altered; the row's fixture is chartered, not built. (c) **vacuous for this stage**, and ⚠️ **flagged as this row's largest unpriced item for the SPEC**: `BOOTSTRAP_PROMPT.md:350` declares `test/conformance/grpc/` and it **does not exist**. (d) **vacuous** — no fuzzer added (repo total **55**, `-- '*.go'`-scoped). (e) **not exercised** — no Go compiled or linted; the tree is byte-identical on every `.go` path. (f) **DEPARTURE, named not claimed** — no `REVIEW.md`; **37 of 124** phase dirs carry one and none since 25.3.

## Sentinel

Input measured **231 lines / 115 data rows** BEFORE anything was written. **Before edits: (1) SILENT at `want=115` · (2) FIVE `:193 :203 :213 :219 :227` · (3) `NEVER OPENED: gRPC` alone.** **After edits: (1) `NOT DONE: row 84` at `want=116` with `examined 116 data rows` — correct while the phase is open · (2) SIX `:194 :200 :206 :216 :222 :230` · (3) SILENT.** `stop` **NOT** created (`ls stop` => `No such file or directory`) and must not be while (1) and (2) print.

**FIVE negative controls before, ALL FIRED** — row 62 doctored => `NOT DONE: row 62` with `NC LANDED? [ in-progress ]` inspected first; `want=114` => `GATE FAIL: examined 115 data rows, expected 114`; an invented slug fires while `WASM`/`HTTP-filters` correctly do not; the check-(2) **one-arm** strip moves **5 -> 4, NOT 5 -> 0**; both arms stripped => 0. **A SIXTH after** — doctoring the `gRPC-family row` mention restores `NEVER OPENED: gRPC`, which is what proves check (3)'s new silence is a result rather than a broken check.

**Leak check by whole-file before/after count, not a diff grep:** check-(2) union **5 -> 6** (deliberate), `-family row` **93 -> 95** (both `gRPC` 0 -> 2; WASM and Observability invariant), lines **231 -> 234**, data rows **115 -> 116**. Row well-formedness: ARM-A flags **only** the pre-existing lines 119 and 131; **row 84 does not appear**.

⚠️ **ONE LEAK AXIS MIS-RAN AND IS RECORDED RATHER THAN HIDDEN.** `grep -oiE '-family row'` parsed the pattern as a **flag** and printed `base=0 now=0 delta=0` — which reads exactly like *"no change"*. Only `--` made it discriminate. **A gate that reads zero on both sides is not evidence of invariance.**

⚠️ **AND THE PRESCRIBED "TOLERANT" ARCHIVE-ABSENCE GUARD IS ITSELF FAIL-UNSAFE.** Run on four REAL arms it read **0 on an ANNOTATED-label entry that IS present** (raw fixed-string = 1) — **the guard introduced to fix the fail-unsafe miss reproduced that exact miss.** The form that passed all four arms anchors on the bullet and allows ANY run of characters before the quoted target: target **0/0**, annotated **1/1**, plain **1/1**, invented **0/0**. **The next router roll should carry the corrected form.**

## Handoff

**Next: the phase-84 SPEC.** It owes the four open questions, `ADR-0306` §Context at STATUS `PROPOSED`, the fixture's assertion shape, and an **ENUMERATION** of cost rather than a scaling. ⚠️ **Price or explicitly defer `test/conformance/grpc/` in writing — it is the strongest candidate to be this row's under-enumerated item.** ⚠️ **Do NOT wire `RunEncodeTrailers`.** ⚠️ **A stats-only fixture is VACUOUS here** (broken-gate shape 31) — assert the RPC's own status via the Drive hooks and `CompareBytes`, and explicitly un-assert the four cosmetic header divergences.

---

## SPEC — done 2026-08-06

## What landed

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `SPEC.md` (new) · `DECISIONS.md` **17926 -> 17956**, `ADR-0306` §Context drafted STATUS **`PROPOSED`**, verified STRICTLY APPEND-ONLY (`30 0`, base a byte-exact PREFIX) · `STATE.md` rolled **IN PLACE** (10 insertions / 10 deletions, **64 lines unchanged**) · `STATE_HISTORY.md` **456 -> 458**, strictly append-only. ⚠️ **`ROADMAP.md` and `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED; sentinel `want` STAYS 116; row 84 STAYS `in-progress`.**

## Method

**FIVE investigation agents** on disjoint remits, each in its own **DETACHED** worktree with private scratch and a private port band inside `44800-45299`. Every agent reverted its probes and confirmed `git status --porcelain` = **0 lines**; docker torn down **BY NAME** (`a1-h2spec-asgate`, `a3-ref-84`, `a5c-ref`, `a5c-backend`, `a5c-probe`, `a5c-net`). `go.mod`/`go.sum` untouched, verified by snapshot-and-restore around two module probes. ⚠️ **The controller-side cwd-reset hazard FIRED as predicted** — every git command used `git -C <abs-worktree>`.

## The stage's headline

⚠️ **THE CONFORMANCE GATE THIS ROW WAS CHARTERED TO REASON ABOUT HAS NEVER RUN THE SECTION IN QUESTION — OR ANY OF RFC 9113 §6.** The BRAINSTORM's sharpest open question asked *why* `http2/6/10` reports green over a live CONTINUATION defect. **The premise is false: the selector matches zero cases.** h2spec addresses sections with **dotted** numbers; `h2spec.go:22-36` declares **ten** slash-form strings; an unmatched positional argument is a **silent no-op**. Provable from inside the repo without running anything — the 53 decomposes as `3`=2 + `4`=9 + `5`=22 + `7`=2 + `8`=18, and **`CONFORMANCE_PINS.md`'s own audit table has recorded the 18 observed suites with ZERO 6.x rows, totalling 53, since 2026-04-25**, while **ADR-0051 §2 asserts section 6 is covered**. ⚠️ **Two authoritative documents have contradicted each other for ~80 phases and neither was noticed because both end in "53".** Running the correct selector: **42 tests / 37 passed / 1 skipped / 4 FAILED**, four envoy-go-specific failures hidden the whole time against a **disjoint** three on the reference. `assertThreshold` **cannot see a zero-case suite** (`if s.Tests == 0 { continue }`), and ⚠️ **the gate is not run in CI at all**.

Two further headlines: **Q4 inverts** — ADR-0058's §Consequences already prescribes this row's fix verbatim, so the row is a **fulfilment** and the verdict is **NARROW**; and **§6.1 crosses at the floor (≈1738 vs ~1500)**, so **the SPEC splits the row 84.1/84.2** on an axis that — unlike phase 83's — cuts **no** correctness constraint.

## Refutation count: **TWENTY-SIX**, of which **ELEVEN are load-bearing**

Load-bearing: the h2spec selector defect (five independent measurements) · ADR-0058 as charter rather than obstacle · **ADR-0052 `:1821` as a BINDING constraint no prior document named** · the ratio category error · the wrong-shape median · the rising comment fraction · **gRPC error RPCs do NOT pass unpatched** (grpc-go's http-handler transport never emits Trailers-Only — all three arms RED) · **three of the four "cosmetic header divergences" are REQUEST headers** invisible to the fixture · the trailer block is byte-exact cross-side while only the header block needs canonicalization · **`0079` is 1,039 lines, not 923, and the 923 omits exactly the README and the PKI this row also needs** · the reference books **nothing** for response trailers, so a stats gate is vacuous on **both** sides.

⚠️ **Three agent claims the controller did NOT accept**, one of them a **refutation aimed at a claim the BRAINSTORM never made** (it attacked "H2→H2 unary works at the tip"; the BRAINSTORM claims an HTTP-layer 200 as a blocker discriminator and states the RPC failure explicitly) — `reference_refutation_must_answer_the_claim_as_stated`. A second collision resolved to **"neither"**: `connection.go:467` **is** `rf.SetAction(action)`, the causal selection is at `actions.go:211-216` where there is **no branch to quote**, and the decisive measurement is that **`UseH2` has ZERO non-test non-comment occurrences anywhere in `internal/filter/hcm/`** — **the cause is an ABSENCE, not a line.**

⚠️ **The controller's own brief was wrong again** — it told two agents the stat surface is guarded by `TestNoNewStat*` tests asserting a zero delta. **All five are `internal/statssink`-scoped sink-construction guards, structurally blind to `internal/filter/hcm/`; there is no global freeze test in this repo.** **A controller brief is a claim too** — the same species the BRAINSTORM recorded of its own brief, one stage later.

## Counts corrected against the router

- ⚠️ **`STATE_HISTORY.md` was 456, not the router's 454** — 454 was the *pre*-BRAINSTORM figure; that stage's own append took it to 456 and the roll's counts block was never advanced. `STATE.md` §Current had the transition right. Every other doc count in that block verifies exactly (`STATE.md` 64, `ROADMAP.md` 234, `DECISIONS.md` 17926, `BEHAVIOR_CONTRACT.md` 5900).
- ⚠️ **The port occupancy is 146 distinct ports across 19 bands, not 39** — five of the router's entries are not reference listener ports at all, and the largest band (`19140-19172`, 33 ports) is missing entirely. **The convention is `10<fixture index>`, stated in a landed comment at `0118/driver/driver.go:29-31`** — "10443 = TLS" is a leaked mnemonic, not a rule.
- ⚠️ **The BRAINSTORM's own charter miscounts its open questions** — §4 enumerates **FIVE**; §10 and this file's own BRAINSTORM handoff say "four". **The SPEC disposes of five**; the one that would have been dropped is Q5 (PKI), the only one with a landed-cost fork.

## Gates

(a)/(b) **not exercised** — zero `.go` changed. (c) ⚠️ **`test/conformance/grpc/` NOT BUILT, deferred IN WRITING on two independent grounds** — §7.5(c) binds *"the declared threshold"* and §7.3 declares one for only **two** of its four suites (the gRPC and h3spec lines are **bare**), and **`test/conformance/h3spec/` has never existed while the HTTP/3 family opened at phase 61**, which mentions gate (c) **zero times across all eight of its documents** and whose h3spec deferral `ROADMAP.md:194` still carries **23 phases later**. Existing suites ASSERTED-UNAFFECTED: proxy-wasm **10 of 16 families (62.5%)**; h2spec **53/53 — stated with the scope caveat above, NOT as frame-level evidence**. (d) **VACUOUS** — the word is "vacuous", not "green": §7.4 binds a phase introducing a parser/codec/filter and this row introduces none (the HPACK decode already runs; the frame goes through already-landed code). (e) not exercised. (f) **DEPARTURE, named not claimed** — no `REVIEW.md`; **37 of 124** phase dirs carry one, none since 25.3.

## Sentinel

Input measured **234 lines / 116 data rows** BEFORE anything was written. **(1)** `NOT DONE: row 84` at `want=116`, denominator printed — **correct while the phase is open** · **(2) SIX** `:194 :200 :206 :216 :222 :230` · **(3) SILENT.** ⇒ CONJUNCTION; (1) and (2) print, so **the sentinel does NOT fire.** `stop` **NOT** created (`ls stop` => `No such file or directory`).

**SIX negative controls, ALL FIRED** — row 62 doctored => `NOT DONE: row 62` **and** row 84, with `NC LANDED? [ in-progress ]` inspected first · `want=115` => `GATE FAIL: examined 116 data rows, expected 115` · the **mandatory** check-(3) doctoring (residual occurrences confirmed **0** first) => `NEVER OPENED: gRPC` restored · an invented slug fires while `gRPC`/`WASM`/`HTTP-filters` correctly do not · check-(2) **one-arm** strip => **6 -> 5, NOT 6 -> 0** · both arms => **0**.

⚠️ **The `-family row` FLAG TRAP reproduced live** — without `--` it errors (`grep: amily row: No such file or directory`) and the arithmetic prints `base=0 now=0 delta=0`, **indistinguishable from "no change"**. Leak-check baselines for the PLAN and IMPL to diff: check-(2) **6** · `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · **234 lines / 116 data rows** — all invariant, `ROADMAP.md` byte-untouched.

⚠️ **THE ARCHIVE GUARD FAILED UNSAFE AGAIN, AND THEN THE ENTRY THIS STAGE APPENDED REPRODUCED THE DEFECT.** Cross-product on **four REAL targets**: the OLD colon-anchored form read **0 on an annotated-label entry that IS present** (raw `grep -cF` = 1); the **ROBUST** form was correct on all four. Post-append, this stage's own entry reads **1 raw / 1 robust / 0 colon**, because its label carries a parenthetical. **The guard's regex is DESCRIBED, not spelled, inside the label** — an entry quoting it defeats its own character class and self-clears.

## Two new broken-gate shapes

**THE THIRTY-SECOND: A GATE THAT DECLARES SECTIONS IT NEVER MATCHED, AND CANNOT SEE THAT IT MATCHED NOTHING.** Ten selector strings in the wrong syntax silently select zero cases; the threshold check skips zero-case suites without comment; the reported total is arithmetically honest while **44% of the declared scope has never run**. The missing guard is *"assert every declared section produced at least one case."* Compounded because the audit-trail document recorded the truth for ~80 phases while the ADR recorded the opposite — **and both ended in the same number.**

**THE THIRTY-THIRD: A `git grep` PATHSPEC ENDING IN `/` READS A FAIL-UNSAFE ZERO.** The quoted git-pathspec form returns **0 lines / 0 files** where the shell-glob form returns 70 — same family as the leading-hyphen flag trap, and it reads exactly like "the pattern is absent".

## Handoff

**Next: `PLAN-84.1.md`** (the seam), then `PLAN-84.2.md` (the differential fixture — **the FINAL leg, whose IMPL flips row 84 `done`**). ⚠️ **The PLAN must OPEN with §6.1 already crossed** — the floor is **≈1738** and §6.1 gates *"`PLAN.md` estimates"*; **phases 80 and 81 escaped the trigger purely by writing down a number that was too low.** ⚠️ **Do NOT wire `RunEncodeTrailers`.** ⚠️ **D-84-ENDSTREAM is disposed CONDITIONAL** (unconditional changes the wire for every H2 response; **40 of 120** fixtures declare `http2_protocol_options`). ⚠️ **The break roster must be RE-DERIVED AT THE IMPL TIP**, and it needs a **vacuity control** (the stats legs must stay green under every arm) and a **symmetric control** (same wrong value both sides, must PASS).
