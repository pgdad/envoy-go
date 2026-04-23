# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `02-tcp-proxy`
- **phase-directory:** `docs/envoy-go/phases/02-tcp-proxy/` — exists; contains `SPEC.md` (committed, reviewer-approved via spec-document-reviewer subagent per ADR-0004, iteration 2 approved after iteration-1 fixes and iteration-2 advisory polish).
- **lifecycle-state:** `2` — SPEC.md exists, PLAN.md does not. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 2, the next session invokes `superpowers:writing-plans` (adapted autonomous mode per ADR-0005) to produce `PLAN.md`, with the plan-document-reviewer subagent loop (max 3 iterations) enforcing quality. Split gate applies: if the plan exceeds ~25 tasks OR ~1500 LoC net change, the next session stops and splits into `02.1`, `02.2`, … per BOOTSTRAP_PROMPT §6.
- **next-skill:** `superpowers:writing-plans`
- **next-skill-scope:** Write `docs/envoy-go/phases/02-tcp-proxy/PLAN.md` implementing the design in `SPEC.md`. Key deliverables the plan must cover (see SPEC §4 for the full list): (1) `internal/listener/` — listener manager (NewManager, Start, Stop, Listeners, ListenerInfo) with inline filter-constructor registry; unit tests for multi-listener build-time error paths. (2) `internal/cluster/` — cluster manager (NewManager, Get), `Cluster` with `PickEndpoint`/`ConnectTimeout`, and the round-robin `loadBalancer` with an `atomic.Uint64` counter; unit tests asserting exact 1000/1000/1000 distribution under N=3000 concurrent picks. (3) `internal/filter/tcpproxy/` — TCP proxy filter (NewFilter parses TcpProxy Any, resolves cluster reference and stores `*Cluster` on the Filter struct; Handle picks endpoint, dials with DialTimeout, runs the phase-00 pump verbatim); plus `FuzzTcpProxyFilter` at 30s budget inherited from ADR-0018. (4) `cmd/envoy-go/main.go` — rewired to wire bootstrap → cluster manager → admin → listener manager, with per-listener ready-sentinel lines (`envoy-go listener <name> ready on <addr>`) + terminal `envoy-go ready`; no direct `net.Listen`/`io.Copy`/pump helpers in main. (5) `internal/bootstrap/` — delete `FirstListenerSocket` and `FirstClusterEndpointSocket` + their tests. (6) `test/differential/harness.go` — replace `readyAddr` with name-keyed `ListenerAddr(name)` lookup; `FixtureDriver` gains `SubjectListenerName() string`. (7) Fixture `test/fixtures/0001-tcp-proxy-rr/` — STRICT_DNS reference bootstrap over 3 host backends, STATIC subject bootstrap over 3 loopback endpoints; driver asserts byte-exact responses AND exact 3/3/3 local distribution; fixture README documents the STATIC-vs-STRICT_DNS divergence. (8) Fixture `test/fixtures/0000-tcp-echo/driver/driver.go` — updated to select listener by name ("l_tcp"). (9) `docs/envoy-go/BEHAVIOR_CONTRACT.md` — add **TCP proxy** subsection codifying byte-exact response equivalence and explicitly disclaiming LB sequence as a differential dimension. (10) `docs/envoy-go/DECISIONS.md` — ADRs A–F per SPEC §4.4 (sequential numbers assigned at landing time, expected ADR-0022+). Plan must resolve SPEC §10 deferred decisions with chosen values recorded in the PLAN or as inline ADRs: subpackage layout inside `internal/cluster/`, `connect_timeout` default, `listener.Manager.Stop` implementation depth, fuzz seed corpus size, ready-sentinel backward-compat stance (SPEC recommends clean-break), per-listener vs per-Accept-loop ctx wiring (SPEC recommends shared ctx), `stat_prefix` storage (SPEC recommends store), and ADR numbering. Depends-on: phase 01 (done). Phase 02 ROADMAP row remains `planned` until phase-done gate (f) (REVIEW.md approved) — per the state machine, ROADMAP row transitions to `done` only at state 6 commit time.
- **last-commit:** 028db17
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
