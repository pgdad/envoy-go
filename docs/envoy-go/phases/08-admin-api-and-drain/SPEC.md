# Phase 08 — Admin API and Graceful Drain (parent master SPEC)

**Phase id:** `08`
**Slug:** `08-admin-api-and-drain`
**Status:** `in-progress` (SPEC stage; split into `08.1` + `08.2` per ADR-0045 at brainstorm-close — see `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` §1)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md into formal SPEC shape and authors the 08.1 sub-phase SPEC + 08.2 sibling stub)
**Depends on:** phase 07 (done at master `01abdfe` — 07.2 phase-done commit, also closes parent row 07; SHA-fill follow-up at master `f3835a5`)
**Sub-phases:** `08.1-admin-endpoints`, `08.2-graceful-drain`
**Differential surface at end of phase:** ROADMAP rows `08.1` and `08.2` are both `done`; the parent row `08` flips `in-progress → done` AT THE SAME phase-done commit as `08.2`'s phase-done (mirroring the 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 + 07 / 07.1 / 07.2 closure pattern). Cumulatively across the two sub-phases: NEW differential fixture `0009-admin-config-dump` (08.1, four-endpoint admin-API equivalence) is differentially green; NEW differential fixture `0010-graceful-drain` (08.2, drain-state-machine + in-flight-request semantics; exact shape TBD when 08.2 brainstorms) is green; pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe`, `0008-listener-chain-match` remain green; the h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS; one new fuzzer (`FuzzConfigDumpFormat` in 08.1) plus 08.2's TBD fuzzer(s) run clean at the 30s ADR-0018 budget.

---

## 1. Mission summary

Phase 08 lands envoy-go's minimum admin API surface — the read-only operator-introspection endpoints that move envoy-go from "a proxy you can route traffic through" to "a proxy an operator can debug, audit, and reason about" — plus the graceful-drain semantics that move envoy-go from "kill -TERM means hard exit" to "kill -TERM means stop accepting new connections, finish in-flight requests, then exit cleanly". The brainstorm-close BRAINSTORM.md (§1) split the original ROADMAP row `08` into two sub-phases at planner-time per ADR-0045 + `BOOTSTRAP_PROMPT.md` §6.2; this parent SPEC carries the cross-cutting design that applies to BOTH sub-phases and points downward to each sub-phase's authoritative SPEC for the per-surface detail.

The two sub-phases ship in order (08.1 first, 08.2 second per the BRAINSTORM §1 dependency analysis). **08.1 is the unblocking phase** for 08.2's surface — 08.2's `POST /drain_listeners` admin endpoint, `/ready` DRAINING-state body extension, and `/server_info` `state`-field DRAINING-state transition all consume the admin-mux extension scaffold that 08.1 lands. Architecturally, 08.1 and 08.2 also share no production-code surface beyond the admin HTTP/1.1 mux scaffold (see §4.3); 08.1's surface is read-only against immutable-post-boot structures, while 08.2 introduces a new `internal/drain/` package, mutates the request hot path (listener Accept loop + HCM in-flight signaling), and upgrades the SIGTERM handler in `cmd/envoy-go/main.go`.

After phase 08, the project has proven its ninth central engineering claim: *envoy-go exposes the minimum operator-grade introspection surface (config dump + cluster status + listener status + server status) and lifecycle discipline (graceful-drain on SIGTERM + drain-without-exit via admin) that Envoy ships, sufficient to deploy envoy-go behind an L4 frontend that performs rolling restarts and to inspect a running envoy-go instance with the same operator workflows used against upstream Envoy — closing the MVP trunk at phases 00–08.*

---

## 2. Sub-phase scope summary

Per BRAINSTORM §1's split table:

| Sub-phase ID | Title | Scope | Differential fixture |
|---|---|---|---|
| **08.1** | `admin-endpoints` | Four new read-only admin handlers under the existing `internal/admin/` HTTP/1.1 mux: `GET /config_dump` (`application/json` via `protojson` over `*adminv3.ConfigDump`), `GET /clusters` (`text/plain; charset=UTF-8`), `GET /listeners` (`text/plain; charset=UTF-8`), `GET /server_info` (`application/json` via `protojson` over `*adminv3.ServerInfo`); `BEHAVIOR_CONTRACT.md` `## Admin API — /ready` section restructured into `## Admin API` umbrella with per-endpoint subsections; constructor-widening pattern threads `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` into `admin.New()`; new accessor `cluster.Manager.Clusters()` (snapshot of cluster + endpoint info; `*stats.Counter`/`*stats.Gauge` reads stay live per LBP-1). | fixture `0009-admin-config-dump` (differential, four-endpoint per-endpoint equivalence under a 5-request defined load against the §2.6 fixture bootstrap) |
| **08.2** | `graceful-drain` | New `internal/drain/` package owning the drain-state machine (`LIVE` → `DRAINING` → exit) + drain-completion signaling; `cmd/envoy-go/main.go` SIGTERM-handler upgrade; `internal/listener.Manager.Drain` (stop-accepting on listening sockets); `internal/cluster.Manager.Drain` (best-effort upstream connection close after drain timeout); `POST /drain_listeners` admin endpoint (drain-without-exit); `/ready` extension (DRAINING-state body); `/server_info` `state`-field DRAINING transition; `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` subsection + `### /ready` extension + new `## Graceful drain` umbrella section. | fixture `0010-graceful-drain` (TBD shape; likely an in-flight-request-completes-while-new-conns-rejected scenario plus `/ready` and `/server_info` state-transition observation) |

The two sub-phases share no production-code surface except the `internal/admin` HTTP/1.1 mux scaffold (the same shared scaffold that 06.1 extended via `mux.HandleFunc("/stats/prometheus", ...)` — this is mux registration, not code-sharing). 08.1 ships a stable admin-mux extension pattern that 08.2 then uses to register `POST /drain_listeners` and to extend the existing `/ready` and `/server_info` handlers. **08.1 ships first.** ROADMAP row 08.2 is `planned` until 08.1's phase-done commit; depends-on `08.1`.

The authoritative scope detail lives in each sub-phase's `SPEC.md`:

- `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` (full sub-phase SPEC; this commit drafts it).
- `docs/envoy-go/phases/08.2-graceful-drain/README.md` (sibling SPEC stub; this commit drafts it; superseded by the 08.2 SPEC drafted at 08.2's lifecycle-state 1).

---

## 3. Split rationale

Per BRAINSTORM §1, the original ROADMAP row `08` bundled two architecturally-distinct deliverables:

1. **A minimum read-only admin API surface** (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) extending the existing admin server beyond `/ready` (phase 01) and `/stats/prometheus` (phase 06.1) — ROADMAP row 08's first half.
2. **Graceful-drain semantics** (SIGTERM-triggered drain, `POST /drain_listeners`, listener stop-accepting + in-flight-completion, `/ready` + `/server_info` state transitions during drain) — ROADMAP row 08's second half.

These two surfaces live in disjoint code paths and have disjoint risk profiles:

- **08.1's surface lives entirely in `internal/admin/`** plus a small set of accessor methods on `internal/listener.Manager`, `internal/cluster.Manager`, and `internal/bootstrap`. Net change ~600–800 LOC + ~200–300 LOC fixture/driver. **No change to the request hot path.** No new package outside `internal/admin/`. Read-only surface — no mutation, no new lifecycle state.
- **08.2 introduces a new `internal/drain/` package** with a state machine, **mutates the request hot path** (listener Accept loop must consult drain state; HCM dispatch must signal in-flight completion to the drain manager), upgrades the SIGTERM handler in `cmd/envoy-go/main.go`, extends two admin endpoints (`/ready`, `/server_info`), adds one new mutating admin endpoint (`POST /drain_listeners`), and adds new fields to `BEHAVIOR_CONTRACT.md`'s admin section. Net change ~500–800 LOC + ~200–300 LOC fixture/driver.

Combined: ~1100–1600 LOC production + ~400–600 LOC fixture/driver. Combined task count estimated at 28–38 across both sub-phases — **crosses both ADR-0045 thresholds** (~25 tasks, ~1500 LoC). Bundling them in one phase risks the same SPEC-bloat that drove the 06.1/06.2 and 07.1/07.2 splits; the phase-07 BRAINSTORM (§1) called out "bundling them into one phase risks bloating the SPEC" as the primary rationale, and the same argument applies symmetrically here.

The 08.1+08.2 split is planner-time (ADR-0045), not mid-execution; the BRAINSTORM session resolved the split before SPEC drafting began. No new splitting ADR is anticipated for phase 08 — ADR-0045 already authorizes the discipline; this parent SPEC and the two sub-phase artifacts are the discipline's expression. (The 08.1 SPEC §8 ADR-0084 explicitly *applies* ADR-0045 to phase 08, mirroring the 05 / 06 / 07 application-ADRs; that's a documentation-of-application ADR, not a new splitting-discipline ADR.)

---

## 4. Cross-cutting decisions (apply to BOTH 08.1 and 08.2)

### 4.1 BEHAVIOR_CONTRACT.md restructure

Phase 08 restructures the existing `BEHAVIOR_CONTRACT.md ## Admin API — /ready` section (introduced at phase 01) into a `## Admin API` umbrella with per-endpoint subsections, mirroring the existing `## HTTP/1.1`, `## HTTP/2`, `## TCP proxy`, `## HTTP filter chain` umbrella sections. The restructure lands in two passes:

- **08.1** restructures the umbrella header, preserves the `### /ready` content verbatim under the umbrella, and adds `### /stats/prometheus` (short summary; full pin in phase 06.1's section) plus `### /config_dump` + `### /clusters` + `### /listeners` + `### /server_info` (each populated with body-shape rules, header-set rules, an empirical-pin block, and an equivalence claim).
- **08.2** adds `### /drain_listeners` (mutating endpoint contract), extends `### /ready` (DRAINING-state body — partially supersedes ADR-0015), and adds a sibling `## Graceful drain` umbrella section covering drain-state-machine semantics independent of the admin API.

Both populations are in-place edits per the ADR-0052 precedent (the same authorization 06.1 + 06.2 + 07.1 + 07.2 used). No new ADR is required to authorize this in-place edit pattern; ADR-0052 is the durable record. The 08.1 SPEC §13 lays out the verbatim-Markdown patch; 08.2's SPEC will lay out its own verbatim patch when it is drafted.

### 4.2 Equivalence-matrix dimensions

BRAINSTORM §8.3 (08.1) introduces four new rows to `BEHAVIOR_CONTRACT.md ## Equivalence Matrix`: `Admin /config_dump`, `Admin /clusters`, `Admin /listeners`, `Admin /server_info`. 08.2 will introduce one more row (`Admin /drain_listeners`) plus extensions to the existing `Admin /ready` row family. All five new/extended rows are landed in their respective sub-phases, NOT in this parent SPEC.

### 4.3 No new third-party dependencies

Both sub-phases continue the project's "from-scratch" doctrine (D-3.2): the four 08.1 admin handlers are stateless go-stdlib `http.Handler`s registered on the existing `*http.ServeMux`; the 08.2 drain-state machine is an in-tree `internal/drain` package; neither sub-phase introduces an admin-server library or a process-supervisor library. The only protobuf vendor pulls are existing — `github.com/envoyproxy/go-control-plane/envoy/admin/v3` (already a dependency of `bootstrap`) for the `*ConfigDump` + `*ServerInfo` envelope types, and `google.golang.org/protobuf/encoding/protojson` (stdlib-adjacent) for JSON marshaling. No new go.mod entries.

### 4.4 Doctrine inheritance

Both sub-phases honor doctrine `D-3.2` (hybrid implementation stance — proto types may be vendored; runtime constructs are not), `D-3.3` (differential correctness beats internal fidelity — every body-shape claim is empirically pinned against v1.37.2), `D-3.4` (context isolation via written ADRs), `D-3.5` (decisions are written, not remembered — the seven 08.1 ADRs and the 08.2 ADR-set are the durable record), `D-3.6` (every phase is a green build), `D-3.7` (version pinning — all empirical pins reference the `ENVOY_TARGET.md` SHA), and the `BOOTSTRAP_PROMPT.md` §7.5 six-gate phase-done checklist. Both sub-phases' SPECs specialize §7.5 the way 06.1, 06.2, 07.1, 07.2 did.

### 4.5 Lifecycle-state pattern

Both sub-phases follow the 06.1 / 06.2 / 07.1 / 07.2 lifecycle pattern: BRAINSTORM-close → SPEC.md → PLAN.md → executing-plans (or subagent-driven-development) → verification-before-completion → REVIEW.md → phase-done commit. ROADMAP row flips for each sub-phase happen per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3 (`planned → in-progress` at the SPEC commit, `→ done` at the phase-done commit). Per-sub-phase worktree convention per ADR-0003: each sub-phase branches a fresh worktree from the master tip at the start of each lifecycle session (SPEC, PLAN, impl, verify, review, phase-done), squash-merging back to master at session close.

---

## 5. Phase-done gate (parent)

Per `BOOTSTRAP_PROMPT.md` §7.5 + the parent-rollup discipline established by phases 05, 06, 07: **the parent row `08-admin-api-and-drain` is `done` only when both `08.1-admin-endpoints` AND `08.2-graceful-drain` are `done`.** Concretely:

1. ROADMAP row `08.1` flips `planned → in-progress` at 08.1's SPEC commit (this commit's deliverable, per the corrected pattern from `BOOTSTRAP_PROMPT.md` §4.1 invariant 3).
2. ROADMAP row `08.1` flips `in-progress → done` at 08.1's phase-done commit. ROADMAP row `08` and row `08.2` are unchanged at this commit (parent stays `in-progress` because 08.2 is still `planned`; row `08.2` is still `planned` because its SPEC has not been drafted yet).
3. ROADMAP row `08.2` flips `planned → in-progress` at 08.2's SPEC commit.
4. ROADMAP row `08.2` flips `in-progress → done` AT THE SAME COMMIT as the parent row `08`'s `in-progress → done` flip — the 08.2 phase-done commit closes both rows in one operation.

This mirrors the 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 + 07 / 07.1 / 07.2 patterns recorded in `STATE.md` at master `01abdfe` SHA (07.2's phase-done commit) and in the 06 + 07 parent SPECs §5. The parent SPEC inherits the discipline; the two sub-phase SPECs do the per-row work.

The 08.2 phase-done commit's commit-message body must explicitly name both ROADMAP-row transitions (`08.2 → done` AND `08 → done`) so the rollup is grep-verifiable from `git log`. After 08.2's phase-done commit closes the parent row, **the MVP trunk (phases 00–08 per `BOOTSTRAP_PROMPT.md` §8) is complete** and the project transitions to feature-family expansion (BOOTSTRAP_PROMPT.md §9). STATE.md at that point flips to `awaiting next planning` per the §5 lifecycle state machine; the next session's first action is a brainstorm against §9's family list to pick the next phase.

The six-gate set per `BOOTSTRAP_PROMPT.md` §7.5 applies to each sub-phase independently. The parent rollup adds nothing beyond the two sub-phases' respective gates being green.

---

## 6. References

- **BRAINSTORM:** `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` (the authoritative design source for both 08.1 and 08.2; this parent SPEC and the sub-phase SPECs distill it into formal SPEC shape).
- **08.1 SPEC:** `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` (full sub-phase SPEC; lifecycle-state 2 deliverable for 08.1; this commit drafts it).
- **08.2 SPEC stub:** `docs/envoy-go/phases/08.2-graceful-drain/README.md` (sibling placeholder; superseded by the 08.2 SPEC drafted at 08.2's lifecycle-state 1).
- **Parent-master-SPEC precedents:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` and `docs/envoy-go/phases/06-observability-baseline/SPEC.md` (the structural templates this SPEC mirrors — terser than a sub-phase SPEC, summarizes both sub-phases, defers per-surface detail to each sub-phase).
- **Sub-phase SPEC precedents:** `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` and `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` (the structural templates the 08.1 SPEC mirrors — header layout, §-numbering conventions, acceptance-bullet shape, empirical-pin verbatim subsections).
- **Sibling-stub precedent:** `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` (the 07.2 stub from 07.1's SPEC commit until 07.2's full SPEC drafted; the 08.2 stub mirrors this structure).
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — parent / sub-phase relationship), §6.2 (How to split — planner-time-split discipline), §7.5 (Phase-done gate — six-gate checklist), §8 (MVP trunk roster — phase 08 closes the trunk), §4.1 (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Both sub-phases' differential surfaces run against this pin. The 08.1 SPEC §11 cites this SHA in the seven §2.7 empirical-pin obligations; 08.2's SPEC will do the same for its drain-semantics empirical pins when it is drafted.
- **DECISIONS.md:** ADR-0003 (per-phase worktree convention — each lifecycle session branches a fresh worktree), ADR-0004 (autonomous-brainstorming hard-gate — the empirical-pin discipline traces to this), ADR-0008 (Envoy v1.37.2 pin — referenced for empirical-pin SHA anchor in both sub-phases), ADR-0014 (`Server: envoy` header value — inherited by all four 08.1 endpoints + 08.2's `/drain_listeners`), ADR-0015 (pre-init contract for `/ready` — partially superseded by 08.2's DRAINING-state body extension; 08.1 forward-only), ADR-0018 (fuzzer 30s budget — both sub-phases inherit), ADR-0040 (out-of-scope deferrals format — 08.1's deferral-list ADR uses this format), ADR-0045 (planner-time-split discipline — applied to phase 08), ADR-0051 (h2spec pin SHA — referenced for gate (c) carry-through in both sub-phases), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization — both sub-phases inherit), ADR-0061 (stat-name flattening rules SN1–SN8 — 08.1's `/clusters` text format consumes the same `*stats.Counter`/`*stats.Gauge` instances; consistency required), ADR-0083 (last numbered ADR — phase-07.2 closure; next-free for 08.1 is ADR-0084). New ADRs anticipated for 08.1 (seven per BRAINSTORM §9; ADR-0084..ADR-0090); the 08.2 SPEC anticipates its own ADRs at its drafting time (likely 5–7 per BRAINSTORM §3).
- **ROADMAP.md:** rows `08`, `08.1`, `08.2` per the split landed in this commit's ROADMAP edit.
