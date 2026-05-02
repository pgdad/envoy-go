# Phase 08.2 — Graceful drain (sibling SPEC stub)

**Phase id:** `08.2`
**Slug:** `08.2-graceful-drain`
**Status:** `planned` (sibling SPEC stub; full SPEC drafted at 08.2's own lifecycle-state-1 brainstorm + state-2 SPEC session)
**Depends on:** `08.1-admin-endpoints` (architecturally dependent — see BRAINSTORM §1: 08.2's `POST /drain_listeners` admin endpoint, `/ready` DRAINING-state body extension, and `/server_info` `state`-field DRAINING-state transition all consume the admin-mux extension scaffold that 08.1 lands)
**Parent phase:** `08-admin-api-and-drain` — parent-master SPEC at `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md`

---

## 1. Purpose of this stub

This file is a **sibling SPEC stub** lining up with the 06.2 / 07.2 stub precedent (`docs/envoy-go/phases/06.2-access-log/README.md` and `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` were sibling stubs from their respective parent-SPEC commits until each sub-phase's own lifecycle-state-1 entered and its full SPEC was drafted). It exists so that:

1. The phase-08 ROADMAP split (parent `08` → sub-phases `08.1` + `08.2`) has a directory home for `08.2` immediately at the parent SPEC commit, mirroring the §4.1 artifact-layout invariant.
2. Future sessions reading the brainstorm-close BRAINSTORM.md can navigate directly to a 08.2-scoped placeholder rather than guessing where 08.2 lives.
3. The split-table in BRAINSTORM §1 has an addressable target file to cite.

The full 08.2 SPEC is authored at 08.2's lifecycle-state 1 → 2 transition (its own brainstorm + SPEC session, after 08.1's phase-done commit lands), per `BOOTSTRAP_PROMPT.md` §5. **This stub is read-only history once that SPEC commits**; no edits land here after that point. Future edits target `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` (the full SPEC, drafted later).

---

## 2. Scope (forward-looking, per BRAINSTORM §1's split-table + §3 stub)

The 08.2 sub-phase covers graceful-drain semantics: the lifecycle discipline that moves envoy-go from "kill -TERM means hard exit" to "kill -TERM means stop accepting new connections, finish in-flight requests, then exit cleanly". Concretely, the in-scope items per BRAINSTORM §3 are:

1. **New `internal/drain/` package owning the drain-state machine.** A `Manager` type with three states: `LIVE` (initial), `DRAINING` (post-trigger; new connections rejected, in-flight allowed to complete), and an exit transition (when the in-flight count reaches zero or a drain timeout fires, whichever first). Drain-completion signaling is carried over a context channel that the SIGTERM-handler waits on.
2. **`cmd/envoy-go/main.go` SIGTERM-handler upgrade.** The current path (`cmd/envoy-go/main.go:152` style) is `<-ctx.Done()` followed by deferred `lm.Stop()`; the new path threads through the drain manager: signal `drainMgr.Drain()` (which transitions LIVE → DRAINING), wait for `drainMgr.Done()` (with a configurable timeout), then proceed to the existing `lm.Stop()` + `cm.Stop()` shutdown.
3. **`internal/listener.Manager.Drain` accessor.** Stop-accepting on listening sockets: closes the `net.Listener` returned by `net.Listen` (the Accept loop unblocks with `net.ErrClosed`), sets a per-listener flag that the connection-handling loop consults to reject any in-flight connection-accept races. Existing in-flight downstream connections are NOT torn down by `Drain`; they are allowed to drive their HCM filter chains to completion and close on their own.
4. **`internal/cluster.Manager.Drain` accessor.** Best-effort upstream connection close: after the drain timeout (or earlier if in-flight count reaches zero), the cluster manager closes its upstream TLS / HTTP/1.1 / HTTP/2 connection pools so the process can exit without leaking sockets.
5. **`POST /drain_listeners` admin endpoint.** The mutating admin endpoint that triggers `Manager.Drain()` WITHOUT process exit — for operator workflows that drain a fleet member before a deploy promotion gate or rolling-restart sequencer takes over the eviction. Body shape: 200 OK with body `OK\n` (empirical-pin obligation; verify against v1.37.2). Idempotency, multi-call behavior, and the `?graceful=true` query-param (Envoy supports it; 08.2 may defer) are settled at 08.2 brainstorm time.
6. **`/ready` extension — DRAINING-state body.** When `Manager.State() == DRAINING`, the existing `/ready` endpoint returns `503 Service Unavailable` with body `DRAINING\n` (or whatever Envoy emits — empirical-pin obligation; this extends ADR-0015's pre-init contract with a new state-specific body). Partially supersedes ADR-0015.
7. **`/server_info` `state` field — DRAINING transition.** When `Manager.State() == DRAINING`, the existing `/server_info` JSON body's `state` field returns `"DRAINING"` (instead of `"LIVE"`). The 08.1 SPEC §11 empirical-pin block already settled that the `state` enum value `"DRAINING"` is the right token; 08.2 wires the transition into the `/server_info` handler.
8. **`BEHAVIOR_CONTRACT.md` additions.** New `### /drain_listeners` subsection under the existing `## Admin API` umbrella (which 08.1 lands), plus an extension to `### /ready` capturing the DRAINING-state body, plus an extension to `### /server_info` capturing the `state: "DRAINING"` transition, plus a new sibling `## Graceful drain` umbrella section covering drain-state-machine semantics independent of the admin API (in-flight-completion discipline, drain timeout default, listener-Accept rejection contract).

The fixture for 08.2 is **`0010-graceful-drain`** (TBD shape, settled at 08.2 brainstorm time). Likely shape per BRAINSTORM §3: a single-listener bootstrap with an upstream cluster; driver opens a long-lived in-flight HTTP request (e.g., a slow-streaming response), then issues `POST /drain_listeners` to the proxy, then attempts a new connection (which must be rejected), then waits for the in-flight request to complete (which must succeed), and observes `/ready` body transitioning `LIVE\n → DRAINING\n` and `/server_info` `state` transitioning `"LIVE" → "DRAINING"`. The differential equivalence claim follows the existing differential discipline (per-state-transition body equivalence across envoy-go and reference Envoy v1.37.2).

---

## 3. Anticipated 08.2 ADRs (5–7, settled at 08.2 brainstorm time per BRAINSTORM §3)

The 08.2 brainstorm is expected to anticipate roughly the following ADR set (numbering starts wherever the next-free is when 08.2 brainstorm lands; 08.1 closes at ADR-0090 per its SPEC §8):

- Drain state machine shape (LIVE / DRAINING / exit; no INITIALIZING per 08.1 SPEC §11.4 empirical-pin finding).
- SIGTERM-vs-SIGINT semantics (SIGTERM = drain-then-exit; SIGINT = drain-then-exit OR immediate-exit — verified against Envoy's behavior in 08.2 brainstorm).
- `/ready` DRAINING-state body extension — partially supersedes ADR-0015.
- `/drain_listeners` POST contract (response body, idempotency, multi-call behavior, `?graceful=true` query-param disposition).
- `/server_info` `state`-field DRAINING transition timing (immediate at `Drain()` call, or after listener stop-accepting acknowledged?).
- Drain timeout default (Envoy default is 600s; envoy-go MVP likely 30s with operator-knob deferred — settle at brainstorm).
- Hot-restart deferral (ADR'd "hot restart is out of scope; runtime/hot-restart family will land it; envoy-go's drain is one-process scope only").

---

## 4. Out-of-scope (deferred to later phases per BRAINSTORM §3)

08.2 does NOT cover:

- **Hot restart / parent-child handoff.** Requires socket-passing, shared-memory state. Lives in the runtime / hot-restart family per BOOTSTRAP §9; envoy-go's drain is one-process scope only.
- **`POST /quitquitquit` admin endpoint.** Semantic overlap with SIGTERM + `/drain_listeners`; defer evaluation. May land in a future admin-extensions phase if a consumer needs it.
- **Per-listener selective drain** (`/listeners/<name>/drain` admin sub-routes). Per-listener drain is a finer-grained operator workflow; non-MVP.
- **`drain_strategy` per-listener.** Default `GRADUAL` only; `IMMEDIATE` strategy deferred.
- **Configurable `drain_time_s` via bootstrap or admin POST.** Hardcoded MVP default; operator-knob deferred.
- **Connection-level drain windows.** The Envoy concept of per-connection drain timing (drainable connections close at the next idle window) is not in MVP scope.
- **Drain manager interaction with xDS.** No xDS yet; xDS-driven drain semantics deferred to xDS family.

---

## 5. Dependencies and ordering

08.2 depends on **08.1's phase-done commit** for both architectural and ROADMAP-ordering reasons (per BRAINSTORM §1):

- **Architectural (08.1 ships first):** 08.2's `POST /drain_listeners` admin endpoint registers on the same `*http.ServeMux` that 08.1 introduces handlers on; 08.2's `/ready` and `/server_info` extensions edit handlers that 08.1 first wired (or, in the case of `/ready`, restructured the existing handler around). The mux-extension pattern that 08.1 establishes — `mux.HandleFunc(path, s.handler)` after the existing two registrations — is the pattern 08.2 follows.
- **ROADMAP-ordering:** ROADMAP row `08.2`'s `depends-on` is `08.1`. The row stays `planned` until 08.1's phase-done commit closes row `08.1`; then 08.2's lifecycle-state-1 brainstorm session may begin.

A future schedule pressure could in principle execute 08.2 before 08.1 by (a) amending the ROADMAP-row depends-on column, (b) hand-rolling the admin-mux extension pattern in 08.2 ahead of 08.1, and (c) accepting that 08.1's eventual landing then has to reconcile against 08.2's mux registrations. This is **NOT anticipated** and is recorded here only as the alternative against which the chosen ordering is justified.

---

## 6. References

- **Parent master SPEC:** `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` (this commit's parent SPEC — the cross-cutting discipline).
- **08.1 sub-phase SPEC:** `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` (this commit's sibling SPEC — the admin-mux extension pattern that 08.2 inherits; the `## Admin API` BEHAVIOR_CONTRACT umbrella that 08.2 extends; the empirical-pin verbatim evidence including the `state` enum value `"DRAINING"` token that 08.2 will emit).
- **BRAINSTORM:** `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` §1 (split table) + §3 (forward-looking 08.2 stub — the seed for this README) + §2.5 (the 08.1 `state` enum decision that defers DRAINING to 08.2).
- **Phase-01 ADRs partially superseded by 08.2:** ADR-0015 (pre-init contract for `/ready`) — 08.2 partially supersedes by adding a DRAINING-state body to `/ready`. The pre-init / not-ready / draining body taxonomy is the 08.2 SPEC's responsibility to document; 08.1 forward-only.
- **Sibling-stub precedents:** `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` (the 07.2 stub from 07.1's SPEC commit until 07.2's full SPEC drafted) and `docs/envoy-go/phases/06.2-access-log/README.md` (the 06.2 stub from 06.1's SPEC commit, mutatis mutandis). The 08.2 stub mirrors these structures.
- **STATE.md:** when 08.2 enters lifecycle-state 1, `STATE.md`'s `active-phase` flips to `08.2-graceful-drain` and a fresh BRAINSTORM session is opened against this stub. Lifecycle-state 0 → 1 transition skill is `superpowers:brainstorming`.
- **MVP trunk closure:** 08.2's phase-done commit is also phase 08's parent-row close (per parent SPEC §5), which closes the BOOTSTRAP_PROMPT.md §8 MVP trunk (phases 00–08). After 08.2's phase-done, STATE.md flips to `awaiting next planning` per the §5 lifecycle state machine; the next session brainstorms against §9's family list to pick the next phase.
