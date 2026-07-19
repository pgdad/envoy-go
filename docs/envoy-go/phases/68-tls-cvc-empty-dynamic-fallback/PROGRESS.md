# PROGRESS 68 — TLS `combined_validation_context` empty-dynamic fallback

> Live task ledger for the phase-68 IMPL. The PLAN (`PLAN.md`) is the spine; this file records what ACTUALLY happened per task — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions, and the six-gate evidence. Populated at the IMPL, not the PLAN.

## Stage pointer

- **PLAN done** (2026-07-19) — the 9-task TDD spine (T1–T9) landed; row 68 STAYS `in-progress`; DECISIONS tail ADR-0290 (STATUS: IN PROGRESS; completed at the IMPL); counts UNCHANGED (fixtures 112, fuzzers 55, stat 1201, BackendKind 38, go.mod 2). **Adversarial verification (three verifiers, PLAN.md §1.2): the production design is EXECUTION-CONFIRMED SOUND (classification, fallback, provider.go untouched, breaks A–F fire); ONE SEVERE found + corrected — the draft's RD3 two-factor forced-send break was unsound (a union pool advertises the extra CA, so the polite client sends the cert too); replaced with the union-pool upper-bound break. ONE MODERATE (RD-DUPSITE dual stream.go string sites) + MINORs folded in.**
- **Next:** the phase-68 IMPL — execute T1→T9; RE-DERIVE each anchor (`feedback_brief_citations_not_evidence`).

## Task ledger (filled at the IMPL)

| Task | Status | Commit | Red-first / breaks / notes |
|---|---|---|---|
| T1 xds sentinel + dual-%w | pending | | S1→dual-sentinel; S2/S3/empty-resources→errValidation-only; Breaks A (dual-%w) / B (scope pin) |
| T2 tls fallback branch | pending | | empty-served fallback; empty-both×require; S2/S3 boot-FAIL; Breaks C/D/E; P5 byte-intact |
| T3 sdsserver Option | pending | | WithEmptyValidationContext (S1 vs S2); Break F |
| T4 fuzz | pending | | "downstream-sds-empty" + cvcEmptyFuzzProvider + seed; dispatch-verify the trap; +0 fuzzers |
| T5 fixture 0111 | pending | | CVC-primary, require=true, empty served → fallback CA_A, port 10447 |
| T6 0111 breaks | pending | | Break G (RD3 two-factor forced-send); Break H (symmetric CA swap triplet); Breaks I/J |
| T7 BEHAVIOR_CONTRACT | pending | | B1–B4 verbatim |
| T8 verify | pending | | six-gate + cycle guard + full 113-dir differential + -race + envelope audit |
| T9 ADR-0290 + close | pending | | complete IN PLACE; row 68 → done; STATE/ROADMAP/router/sentinel |

## Findings carried from the PLAN (RE-DERIVED at `fba6d385`; RE-VERIFY at the IMPL tip)

- **RD-DUAL / RD-PROV:** `stream.go:164` is `%w: %v` (flattens); dual-`%w` there preserves both sentinels; `provider.go:112` still NACKs on `errValidation` and returns the full chain → `config.go` sees the sentinel with NO `provider.go` edit.
- **RD-P5:** the CVC arm does NOT move in phase 68 (no hoist); the P5 block `:158-164` stays BYTE-INTACT in place — the SPEC's `[MOVE INTACT]` re-derives to byte-preserve.
- **RD3 (EMPIRICALLY SETTLED):** at `require_client_certificate: true`, a BARE forced-send→polite regression does NOT change the correct-impl observable (both → `untrusted=rejected`). Forced-send is NOT observably load-bearing here — a permissive `CA_A∪CA_B` pool advertises CA_B so the polite client sends `client_B` too (the union hazard is caught in BOTH modes; the draft's "upper-bound via forced-send" two-factor break was UNSOUND and is replaced). The untrusted ARM upper-bounds the pool (Break G, union-pool, fires in both modes); forced-send is retained for meaning (exercises verify-and-reject vs collapsing into the `none` arm) + cross-side symmetry. The SPEC §8 "must go vacuous-green" is 0110 (require=false) physics that does NOT transfer.
- **RD-SENT:** the sentinel gate `vc.GetTrustedCa()==nil || vc.GetTrustedCa().GetSpecifier()==nil` is precise (both route to `dataSourceBytes`'s "none set" branch); S2's set-but-empty `inline_bytes` has a SET specifier → stays "parse failure", no sentinel.
- **RD-LINES:** four sub-field rejects `:448-461`; fetch-error `if err != nil` `:184`; DEPARTURE comment `:185-193`; empty-both/inline no-anchor arm `:199-214` (reject `:203`).
