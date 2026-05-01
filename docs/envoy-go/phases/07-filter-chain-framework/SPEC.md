# Phase 07 — Filter Chain Framework (parent master SPEC)

**Phase id:** `07`
**Slug:** `07-filter-chain-framework`
**Status:** `in-progress` (SPEC stage; split into `07.1` + `07.2` per ADR-0045 at brainstorm-close — see `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` §1)
**Produced by:** `superpowers:brainstorming` (autonomous mode per ADR-0004; this commit transcribes the brainstorm-close BRAINSTORM.md into formal SPEC shape and authors the sub-phase SPECs)
**Depends on:** phase 06 (done at master `2c65fcc` — 06.2 phase-done close, also closes parent row 06)
**Sub-phases:** `07.1-http-filter-framework`, `07.2-listener-chain-completion`
**Differential surface at end of phase:** ROADMAP rows `07.1` and `07.2` are both `done`; the parent row `07` flips `in-progress → done` AT THE SAME phase-done commit as `07.2`'s phase-done (mirroring the 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 closure pattern). Cumulatively across the two sub-phases: NEW differential fixture `0007a-cors` (07.1, equivalence-with-Envoy on cors filter) is differentially green; NEW structural fixture `0007b-iteration-probe` (07.1, envoy-go-only structural assertion on the iteration protocol) is structurally green; 07.2's listener-side fixture(s) (TBD at 07.2's brainstorm time) are green; pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log` remain green; the h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS; one new fuzzer (`FuzzFilterChainParse` in 07.1) plus 07.2's TBD fuzzer(s) run clean at the 30s ADR-0018 budget.

---

## 1. Mission summary

Phase 07 lands envoy-go's filter-chain framework: a real HTTP-filter iteration protocol with async-resume + stop/buffer/continue state machine + per-route config + extension registry (sub-phase 07.1), plus the listener-side chain-match completion (`listener_filters` framework + `FilterChainMatch` fields beyond SNI + `Listener.default_filter_chain`; sub-phase 07.2). The brainstorm-close BRAINSTORM.md (§1) split the original ROADMAP row `07` into two sub-phases at planner-time per ADR-0045 + `BOOTSTRAP_PROMPT.md` §6.2; this parent SPEC carries the cross-cutting design that applies to BOTH sub-phases and points downward to each sub-phase's authoritative SPEC for the per-surface detail.

The two sub-phases ship in order (07.1 first, 07.2 second per the BRAINSTORM §1 dependency analysis); each has its own differential surface, its own ADR set, and its own BEHAVIOR_CONTRACT placeholder population. **07.1 is the unblocking phase** — once 07.1 lands, the BOOTSTRAP_PROMPT §9 HTTP-filters family (header_manipulation, fault, jwt_authn, ext_authz, oauth2, csrf, buffer, lua, wasm, local_ratelimit, rbac, etc.) becomes plannable. 07.2 is independent of 07.1's HTTP-filter machinery and does not block the family.

After phase 07, the project has proven its eighth central engineering claim: *envoy-go runs a real multi-filter HTTP chain with iteration protocol and per-route config — filters can stop, buffer, resume async, synthesize local replies that re-enter the encode chain — and matches a downstream filter chain against any FilterChainMatch dimension Envoy supports plus the listener-filters surface that runs before HCM dispatch — making the proxy a programmable middlebox aligned with Envoy's documented extensibility model.*

---

## 2. Sub-phase scope summary

Per BRAINSTORM §1's split table:

| Sub-phase ID | Title | Scope | Differential surface |
|---|---|---|---|
| **07.1** | `http-filter-framework` | HTTP filter iteration protocol + extension registry + per-route config + trivial real filter (`cors`) + test-only probe filter (`envoy_go_test`); supersedes ADR-0040 totally; partially supersedes ADR-0042; amends ADR-0041 silent-ignore set | fixture `0007a-cors` (differential, equivalence-with-Envoy) + fixture `0007b-iteration-probe` (envoy-go-only, structural assertion) |
| **07.2** | `listener-chain-completion` | `listener_filters` framework + `FilterChainMatch` fields beyond SNI (destination_port, prefix_ranges, source_*, application_protocols/ALPN) + `Listener.default_filter_chain`; supersedes parts of ADR-0033 | fixture(s) TBD when 07.2 brainstorms |

The two sub-phases share no production-code surface (07.1 lives entirely under `internal/filter/` and `internal/filter/hcm/`; 07.2 lives entirely under `internal/listener/`); their dependency profile is one-directional and weak — 07.2 may optionally use 07.1's filter-chain-iteration discipline as a model when designing the `listener_filters` framework, but this is not a required deliverable of 07.2. **07.1 ships first.**

The authoritative scope detail lives in each sub-phase's `SPEC.md`:

- `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` (full sub-phase SPEC; this commit drafts it).
- `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` (sibling SPEC stub; this commit drafts it; superseded by the 07.2 SPEC drafted at 07.2's lifecycle-state 1).

---

## 3. Split rationale

Per BRAINSTORM §1, the original ROADMAP row `07` bundled two architecturally-disjoint deliverables:

1. **HTTP filter iteration protocol + per-route config + extension registry + a trivial pluggable filter that covers all iteration states** — ROADMAP row 07's literal scope.
2. **Listener-side ADR-0033 follow-ups** pre-deferred from phase 03: full `FilterChainMatch` (destination_port, prefix_ranges, source_*, application_protocols/ALPN), `listener_filters`, and `Listener.default_filter_chain`.

These two surfaces live in disjoint code paths:

- **07.1's surface lives inside HCM**: `internal/filter/hcm/` (existing) + a new `internal/filter/http/` sub-tree. Iteration protocol, registry, per-route lookup, two new HTTP filters (`cors` + `envoy_go_test`).
- **07.2's surface lives in listener**: `internal/listener/` and the listener-manager's filter-chain-match path. Listener filters + chain-match dimension extensions. Zero overlap with 07.1's HCM-internal code.

Splitting at planner-time keeps each sub-phase's SPEC, PLAN, and REVIEW under the 06.1/06.2 review-surface budget that the project established as reviewable, AND it lets 07.1 ship and unblock the BOOTSTRAP §9 HTTP-filters family without waiting on 07.2's surface (which has no §9 dependents). The 07.1+07.2 split is planner-time (ADR-0045), not mid-execution; the BRAINSTORM session resolved the split before SPEC drafting began. No new splitting ADR is anticipated for phase 07 — ADR-0045 already authorizes the discipline; this parent SPEC and the two sub-phase artifacts are the discipline's expression.

---

## 4. Cross-cutting decisions (apply to BOTH 07.1 and 07.2)

### 4.1 BEHAVIOR_CONTRACT.md placeholders

Phase 07 introduces a new top-level section `## HTTP filter chain` to `docs/envoy-go/BEHAVIOR_CONTRACT.md`, plus an extension to `## Network filters` (the `## TCP proxy` section's surface) to cover `listener_filters` and the new `FilterChainMatch` dimensions. Per BRAINSTORM §7 + ADR-0052's in-place edit precedent established by 06.1's `## Stat-name mapping` and 06.2's `## Access log field mapping`, these subsections are **filled at impl time at the phase-done commit**, NOT at SPEC time. The SPEC reserves the placeholder shape and references the obligation; the in-place edit lands when the implementation lands.

- **07.1** introduces `## HTTP filter chain` and amends `## HTTP/1.1` + `## HTTP/2` for the router-as-direct-call → router-as-terminal-filter supersession.
- **07.2** introduces a new `## Listener filters` subsection and amends the existing `## TCP proxy` section's "Does not yet apply to" enumeration to reflect that listener filters and FilterChainMatch beyond SNI are now in scope.

Both populations are in-place edits per ADR-0052 (the same authorization 06.1 + 06.2 used). No new ADR is required to authorize this in-place edit pattern; ADR-0052 is the durable record.

### 4.2 Equivalence-matrix dimensions

BRAINSTORM §7.3 (07.1) introduces an "HTTP filter chain" dimension to `BEHAVIOR_CONTRACT.md ## Equivalence Matrix` row set. 07.2 may extend the existing TCP proxy / network-filter row family or introduce a "Listener filters" row at its own SPEC time. Both are landed in their respective sub-phases, NOT in this parent SPEC.

### 4.3 No new third-party dependencies

Both sub-phases continue the project's "from-scratch" doctrine (D-3.2): the iteration protocol, the registry, the chain state machine, listener-filter dispatch — all written in-tree. The only proto types pulled in are from `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3` (the upstream cors `Cors` + `CorsPolicy` proto messages, used as parsed config; the implementation is hand-rolled). No upstream filter-chain-engine library, no upstream listener-filter-engine library.

### 4.4 Doctrine inheritance

Both sub-phases honor doctrine `D-3.2` (hybrid implementation stance — proto types may be vendored; runtime constructs are not), `D-3.3` (differential correctness beats internal fidelity), `D-3.4` (context isolation via written ADRs), `D-3.6` (every phase is a green build), and the `BOOTSTRAP_PROMPT.md` §7.5 six-gate phase-done checklist. Both sub-phases' SPECs specialize §7.5 the way 06.1 and 06.2 did.

### 4.5 Lifecycle-state pattern

Both sub-phases follow the 06.1 / 06.2 lifecycle pattern: BRAINSTORM-close → SPEC.md → PLAN.md → executing-plans → verification-before-completion → REVIEW.md → phase-done commit. ROADMAP row flips for each sub-phase happen per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3 (`planned → in-progress` at the SPEC commit, `→ done` at the phase-done commit).

---

## 5. Phase-done gate (parent)

Per `BOOTSTRAP_PROMPT.md` §7.5 + the parent-rollup discipline established by phases 05 and 06: **the parent row `07-filter-chain-framework` is `done` only when both `07.1-http-filter-framework` AND `07.2-listener-chain-completion` are `done`.** Concretely:

1. ROADMAP row `07.1` flips `planned → in-progress` at 07.1's SPEC commit (this commit's deliverable, per the corrected pattern from `BOOTSTRAP_PROMPT.md` §4.1 invariant 3).
2. ROADMAP row `07.1` flips `in-progress → done` at 07.1's phase-done commit. ROADMAP row `07` and row `07.2` are unchanged at this commit (parent stays `in-progress` because 07.2 is still `planned`; row `07.2` is still `planned` because its SPEC has not been drafted yet).
3. ROADMAP row `07.2` flips `planned → in-progress` at 07.2's SPEC commit.
4. ROADMAP row `07.2` flips `in-progress → done` AT THE SAME COMMIT as the parent row `07`'s `in-progress → done` flip — the 07.2 phase-done commit closes both rows in one operation.

This mirrors the 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 patterns recorded in `STATE.md` at the master `2c65fcc` SHA (06.2's phase-done commit) and in the 06 + 05 parent SPECs §5. The parent SPEC inherits the discipline; the two sub-phase SPECs do the per-row work.

The 07.2 phase-done commit's commit-message body must explicitly name both ROADMAP-row transitions (`07.2 → done` AND `07 → done`) so the rollup is grep-verifiable from `git log`.

The six-gate set per `BOOTSTRAP_PROMPT.md` §7.5 applies to each sub-phase independently. The parent rollup adds nothing beyond the two sub-phases' respective gates being green.

---

## 6. References

- **BRAINSTORM:** `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` (the authoritative design source for both 07.1 and 07.2; this parent SPEC and the sub-phase SPECs distill it into formal SPEC shape).
- **07.1 SPEC:** `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` (full sub-phase SPEC; lifecycle-state 2 deliverable for 07.1; this commit drafts it).
- **07.2 SPEC stub:** `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` (sibling placeholder; superseded by the 07.2 SPEC drafted at 07.2's lifecycle-state 1).
- **Parent-master-SPEC precedents:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` and `docs/envoy-go/phases/05-http-2/SPEC.md` (the structural templates this SPEC mirrors — terser than a sub-phase SPEC, summarizes both sub-phases, defers per-surface detail to each sub-phase).
- **Sub-phase SPEC precedents:** `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` and `docs/envoy-go/phases/06.2-access-log/SPEC.md` (the structural templates the 07.1 SPEC mirrors).
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — parent / sub-phase relationship), §6.2 (How to split — planner-time-split discipline), §7.5 (Phase-done gate — six-gate checklist), §4.1 (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Both sub-phases' differential surfaces run against this pin. The 07.1 SPEC §11 cites this SHA in the four §2.6 empirical-pin obligations.
- **DECISIONS.md:** ADR-0033 (network-filter-chain subset; partially superseded by 07.2), ADR-0040 (router-as-direct-call inside HCM; totally superseded by 07.1), ADR-0041 (HCM silent-ignore set; amended by 07.1's per-route + buffer-limits decisions), ADR-0042 (exactly `[router]` in `http_filters[]`; partially superseded by 07.1's "non-empty; last entry must be router" rule), ADR-0045 (planner-time-split discipline), ADR-0051 (h2spec pin SHA — referenced for gate (c) carry-through in both sub-phases), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization — both sub-phases inherit). New ADRs anticipated for 07.1 (seven per BRAINSTORM §8); the 07.2 SPEC anticipates its own ADRs at its drafting time.
- **ROADMAP.md:** rows `07`, `07.1`, `07.2` per the split landed in this commit's ROADMAP edit.
