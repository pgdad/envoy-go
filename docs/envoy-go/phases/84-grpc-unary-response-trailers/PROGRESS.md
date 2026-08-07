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
