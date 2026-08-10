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
