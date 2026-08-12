# PROGRESS — phase 86 (validate-sds-nil-provider)

## BRAINSTORM — done 2026-08-10

## What landed

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `docs/envoy-go/phases/86-validate-sds-nil-provider/BRAINSTORM.md` (new) · `ROADMAP.md` **row 86 registered `in-progress`** (235 -> 236 lines, 117 -> 118 data rows; the row sits at `:148`) · `next-prompt.txt` sentinel **`want` 117 -> 118 in the SAME commit as the row** · `STATE.md` rolled **IN PLACE** · `STATE_HISTORY.md` appended (PLAN-84.2 evicted at the five-entry cap), verified STRICTLY APPEND-ONLY. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; this stage adds **NO ADR** (next-free stays ADR-0308; strict `PROPOSED` guard stays 0 — the phase-86 SPEC re-arms it).

## Method

**SELF-PICKED** per the 2026-07-12 standing directive; no banked mid-lifecycle work existed (phase 85 CLOSED, all 117 rows `done` — check (1) was SILENT at `want=117`, the second silent reading in project history, before this stage re-opened it at row 86). ⚠️ **NAMED DEPARTURE: no investigation agents this stage** — the probes (a binary built at `766d98ad` with `-o` into session scratch + four `--mode validate` CLI executions + code reads) were run INLINE by the controller; nothing in the repo tree was edited by any probe.

## The stage's headline

**The `--mode validate` SDS divergence is THREE-ARMED, not one-armed — all three arms REPRODUCED BY EXECUTION at this tip.** `tls_certificate_sds_secret_configs` / `validation_context_sds_secret_config` / `combined_validation_context` each exit 1 under `--mode validate` (reject sites `internal/tls/config.go:387-389`, `:436-438`, `:453-455`, all gated on `provider == nil`; `validate/validate.go:48-49` threads a literal nil) while the ordinary boot path HONORS all three shapes (phases 60.2/65/66); the static-TLS positive control returns `configuration OK` exit 0. Two findings beyond the carried record: **a pure reject-lift is wrong in BOTH directions** (under-rejects without boot's pre-scan parity — node arm 7, one-secret cap, ParseSDSConfig arms; over-lifts if keyed on `provider == nil`, whose OTHER consumers — QUIC, test-only constructors — must keep rejecting byte-identical), and **both fetch sites must be skipped** (`config.go:390` + `NewDownstreamConfig`'s require_client_certificate block) to preserve the phase-60.2 no-dial decision. Coverage hole measured ZERO (no SDS test in `validate_test.go` 432 lines or `main_test.go` 1502 lines).

## The pick

**Row 86 `validate-sds-nil-provider`, an Operational-tooling-family MAINTENANCE row claiming NO family ordinal** (ADR-0298/ADR-0300/row-85 precedent); provenance BRAINSTORM-83 §5.6, OUTSIDE the family windows (nothing narrows at row-done); the Op-tooling window's "RTDS/SDS validate companion" is ADJACENT but DISTINCT (feature vs repair) and deliberately untouched. Rejected with re-derived costs (BRAINSTORM §2.1): the CONTINUATION two-sided repair (strongest product defect, 2-4x row 85, gate does not exist yet — the natural next LARGE row); the stat-surface recount (cheapest but changes nothing real; rides a future +0 row); `ssl.connection_error` (+444 whole-`.go` floor); `test/conformance/grpc/` (9/26 reachable); REVIEW.md restoration (process-not-product); hygiene fold-ins (too thin; NOT folded in here either); the six window paragraphs (not a row — settled at BRAINSTORM-85 §2.1).

## Sentinel (measured, before AND after the ROADMAP edit)

BEFORE at `want=117`: (1) SILENT · (2) SIX `:195 :201 :207 :217 :223 :231` · (3) SILENT. AFTER at `want=118`: (1) **`NOT DONE: row 86`** (the single expected line while open) · (2) SIX, anchors shifted to **`:196 :202 :208 :218 :224 :232`** · (3) SILENT. Conjunction fails ⇒ **does NOT fire**; `stop` NOT created (checked at the repo root AND the worktree, before and after). All doctoring NCs fired both sides (row-62 with `NC LANDED? [ in-progress ]` inspected first — after: `row 62` AND `row 86`; want off-by-one both directions; check-(3) doctoring with residual 2 -> 0 confirmed first, WASM control silent; check-(2) one-arm 5 long / 1 short, union 6). Leak axes: union **6 -> 6**, `-family row` **95/67 -> 95/67** (the row's MAINTENANCE phrasing deliberately does not match check (3)'s pattern — rows 76/78/85 precedent), `gRPC-family row` **2 -> 2**, `Operational-tooling-family row` **3 -> 3**, ARM-A flags **{119, 131}** only (row 86 at `:148` clean), new slug occurrences **2**, both inside row 86's own line.

## NEXT

**SPEC** — the seven BRAINSTORM §4 open questions (Q2 boot-parity surface and Q3 reference-container run need EXECUTION); ADR-0308 §Context drafted STATUS `PROPOSED` (re-arms the strict guard 0 -> 1); cost ENUMERATION by prototype (the §3.3 floor is a floor — eighth consecutive lower-bound firing).

## SPEC — done 2026-08-10

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `SPEC.md` (new) · `DECISIONS.md` +ADR-0308 §Context STATUS `PROPOSED` (18050 -> 18066, strictly append-only, strict guard ARMED 0 -> 1, tail ADR-0308, next-free ADR-0309) · `STATE.md` rolled in place · `STATE_HISTORY.md` 476 -> 478 (IMPL-84.2 evicted, two-way tie resolved by list position) · `next-prompt.txt` rolled to the PLAN. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**.

**Method:** the BRAINSTORM's named departure CONTINUED — probes inline, no agents; PLUS a **compiling, test-green prototype of the chosen mechanism** in a detached worktree (deleted at close, diff in session scratch). All seven §4 questions DISPOSED: Q1 decided by MEASUREMENT (no-fetch sentinel provider; net +88 production across 4 files; option (b) dead at ~108 call sites); Q2 by EXECUTION (six build-time parity arms — incl. the NEW n7 `http2_protocol_options` arm — + the fetch-time exempt class); Q3 by EXECUTION (reference validates all three arms OK; node-absent is a BOTH-sides reject; no new departure minted); Q4 sweep (the `BEHAVIOR_CONTRACT.md:1062` "stays that way" REVERSAL rides ADR-0308); Q5 placement; Q6 zero landed strings change; Q7 all +0 verified post-change. Cost floors REFUTED as central (ninth `reference_measured_prototype_is_a_lower_bound` firing): IMPL budget ~110-160 net production + ~400-680 test.

**Sentinel (measured):** (1) `NOT DONE: row 86` at `want=118` alone · (2) SIX `:196 :202 :208 :218 :224 :232` · (3) SILENT ⇒ no fire; `stop` NOT created. All four NCs fired.

## NEXT

**PLAN** — task decomposition of the four edit sites + test placement under D-86-SEQ (one leg); TDD order with RED anchors re-proven at the IMPL tip; guard-preservation NC roster; the contract-edit text riding ADR-0308; the budget carried as a FLOOR.

## PLAN — done 2026-08-11

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `PLAN.md` (new) · `STATE.md` rolled in place · `STATE_HISTORY.md` 478 -> 480 (BRAINSTORM-85 evicted — a UNIQUE oldest, no tie; strictly append-only, controls 10/10, 8/8, 0/0) · `next-prompt.txt` rolled to the IMPL. `ROADMAP.md`, `DECISIONS.md`, `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED** (a PLAN adds no ADR; strict `PROPOSED` guard STAYS 1).

**Method:** the 86-lineage inline-probe departure CONTINUED (the "cheap probes" clause, named): the tip binary + SEVEN `--mode validate` executions at `23ad1232` — the three RED anchors + control re-proven with failure lines read, n1/n7 confirmed MASKED (message-transition RED side), n3 confirmed already-parity. **NO SPEC claim moved.** Ports 47200-47203 template values only; nothing bound; no docker.

**What the PLAN decided/landed (PLAN.md §2-§7):** EIGHT ordered IMPL tasks under D-86-SEQ (ONE leg, one atomic commit; TDD RED census held inside — census -> xds sentinel -> boot split -> tls skips + `:518` interplay -> validate+CLI -> comment rewrites -> docs -> gates); the guard-preservation NC roster (three pins BYTE-UNTOUCHED + NINE new NCs incl. the QUIC-wrapped shapes and `IsNoFetch(nil)==false`); the §6a-6d contract-edit TEXT drafted (`:1062` reversal, `:1050` THREE->TWO, `:1034` roster drop, `:948-958` extension) + ADR-0308 completion plan (guard 1 -> 0 at IMPL); the frozen error-string set (zero landed strings change); the eleven-shape regeneration recipe; budget per-task **~117-165 prod / ~440-760 test** on top of the SPEC floor — overrun recordable. One form-dependence finding: the ARM-A {119, 131} figure binds only with its escape-aware command; a PLAN's invariance proof is the empty ROADMAP diff.

**Sentinel (measured):** (1) `NOT DONE: row 86` at `want=118` alone · (2) SIX `:196 :202 :208 :218 :224 :232` · (3) SILENT ⇒ no fire; `stop` NOT created. All four NCs fired (row-62 => both rows with `[ in-progress ]` inspected first; want=117 => GATE FAIL at 118; check-(3) 2 -> 0 => `NEVER OPENED: gRPC`, WASM silent; check-(2) 5/1 union 6).

## NEXT

**IMPL** — the single leg: Tasks 0-7 in PLAN §3 order, ONE atomic commit; eleven-shape census at its tip; parity-string diff vs normal-mode boot; NC roster green with pins untouched; contract text finalized; ADR-0308 completed (guard 1 -> 0); row 86 `done` (`want` stays 118, check (1) goes silent); realized cost vs the floor recorded.

## IMPL — done 2026-08-12

**PHASE 86 CLOSED.** ONE atomic commit (D-86-SEQ held): production **+150 net `.go`** over 5 files (INSIDE the PLAN's 117-165 band) — `internal/xds/provider_nofetch.go` (NEW, the no-fetch sentinel) · the `internal/boot` `newSDSProviderAndClient` split + `NewValidateSDSProvider` (ENTIRE body moved, verified 89-vs-89 lines with only the seven return sites differing; the never-dialed client CLOSED, D-86-CONN — ⚠️ that Close is UNGATED, a named future candidate) · three `internal/tls` `IsNoFetch` fetch-site skips + the `sdsCertPromised` interplay (reject bytes untouched) · `validate/validate.go` threading. Tests **+1116 net `.go`** over 6 files — ⚠️ **OVERRAN the PLAN §7 ceiling (760) by 356: the TENTH consecutive `reference_measured_prototype_is_a_lower_bound` firing, recorded in ADR-0308 §Consequences (iii)**. Docs: the four `BEHAVIOR_CONTRACT.md` edits (5955 -> 5957; the item-7 REVERSAL at `:1064`, the TWO-unconditional-plus-ONE-conditional roster at `:1052`, the `:1036` drop, the `:950-952` extension) · **ADR-0308 COMPLETED IN PLACE** (18066 -> 18098; strict `PROPOSED` guard **1 -> 0**; headings 307; `^---$` 216) · `ROADMAP.md:148` row 86 -> `done` (numstat `1 1`, `want` stays 118, every leak axis invariant).

**The parity contract, demonstrated:** the full eleven-shape battery re-run at the tip — control/armA/armB/armC/n5 `configuration OK` exit 0; n1/n2/n4/n6/n7 message-TRANSITIONED to the boot strings; n3 already-parity — and a per-shape parity-string diff vs normal-mode boot, core messages BYTE-IDENTICAL (both raw lines recorded per shape in the session artifacts).

**Five execution findings** (every PLAN claim that did not survive): (a) PLAN §6's n2 recipe AMBIGUOUS — "two positions" = TWO SDS-bound CONTEXTS, not two entries in one list (census fix round 1); (b) n3's WRAP format changed (listener-wrapped pre-fix -> bare pre-scan post-fix; frozen substring unchanged); (c) "validate never passes nil" FALSE unscoped — at seen==0 validate threads `(nil, nil)` exactly like boot; the roster is TWO unconditional + ONE conditional (caught at review; scoped everywhere; negative form on the record in ADR-0308); (d) three ADR-0308 citation defects caught pre-squash — ⚠️ including the **wrong `internal/xds/xdsgrpc/...` path for the grpcclient Close pins, which ALSO lives in this phase's SPEC/PLAN prose: prior-stage records are NOT rewritten, ADR-0308 §Decision (b) carries the correction** (correct: `internal/grpcclient/grpcclient_test.go:1911/:1982`); (e) two `:518` locators minted stale within the branch, fixed by the final-review fix wave (cite-by-string). ⚠️ One probe caveat: `validate_test.go`'s `deadPort` helper transiently binds an ephemeral loopback socket (closed, never dialed) — "validate binds nothing" is true of the product, not of that helper.

**Method:** subagent-driven per `feedback_execution_style` — a NAMED RETURN from the 86-lineage inline departure (6 implementers + 6 task reviewers + 4 scoped re-reviews + a gates agent + a most-capable-model final whole-branch review; 3 fix rounds + 1 fix wave, all findings addressed or ledgered with rulings; the controller ran the sentinel battery itself and squashed). Probe ports 47210-47299 template values only; no docker; the disposable base worktree for count NCs removed with proof.

**Gates:** differential **121/121 `INNER_EXIT=0`** (anchored panic gate silent, 392.3 s) · `go test ./...` rc=0 (gates battery; at close-verification run 1 failed `internal/boot` — identity NOT captured, a recorded lapse — run 2 green 127/127, cleared package x4 + scoped x5, likely the SDS dial-budget class, UNCLASSED) · `-race` clean x4 · gofmt/golangci-lint clean x6 · fixtures 121 +0 · fuzzers 55/48 +0 · BackendKind tail 38 +0 · `go mod tidy -diff` empty · `go list -deps ./validate` edge set unchanged · stat-surface DELTA 0 (145/21 both sides, `NewCounter(|NewGauge(` form) — every count with its NC observed.

**Sentinel (measured, BOTH sides of the flip):** pre-flip (1) `NOT DONE: row 86` alone · post-flip (1) **SILENT — the THIRD silent reading in project history** · (2) SIX `:196 :202 :208 :218 :224 :232` both sides · (3) SILENT both sides ⇒ the conjunction FAILS, `stop` NOT created (verified absent at repo root AND worktree). All four NCs fired both sides (row-62 doctoring: pre `row 62`+`row 86` / post `row 62` ALONE — the silent side still LOOKS; want=117 GATE FAIL at 118; check-(3) doctoring 2 -> 0 fired with WASM silent; check-(2) one-arm 5/1, union 6).

## NEXT

**BRAINSTORM — SELF-PICK** per the 2026-07-12 standing directive (all 118 rows `done`; no banked work). Candidates ledger carried in `STATE.md` §Current and `next-prompt.txt` (re-derive every cost at that tip); the CONTINUATION two-sided repair remains the strongest product defect on the board.
