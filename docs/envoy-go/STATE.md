# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `02-tcp-proxy`
- **phase-directory:** `docs/envoy-go/phases/02-tcp-proxy/` — does not yet exist; the next session creates it as its first file-system act (per BOOTSTRAP_PROMPT §4.1 invariant 3).
- **lifecycle-state:** `1` — Phase in ROADMAP, directory does not exist. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 1, the next session creates the phase directory and runs `superpowers:brainstorming` scoped to THIS phase, producing `SPEC.md`.
- **next-skill:** `superpowers:brainstorming`
- **next-skill-scope:** Brainstorm `docs/envoy-go/phases/02-tcp-proxy/SPEC.md` for phase 02. ROADMAP row 02 summary: *"Listener + TCP proxy filter + static cluster + round-robin LB (plaintext). TCP proxy fixture green."* (ROADMAP.md:33). The SPEC must: (i) define a listener manager that consumes `static_resources.listeners[]` at more than the phase-01 first-only skeleton, handling `filter_chains[].filters[]` typed-config dispatch — specifically the TCP proxy filter (`envoy.filters.network.tcp_proxy`) as the phase's first real typed-config surface; (ii) define a cluster manager for the `STATIC` cluster type handling multiple endpoints with round-robin load balancing (phase-01's first-endpoint extractor discipline from ADR-0015 is retired here for listener/cluster/endpoint scopes — an ADR must explicitly name the supersession, mirroring ADR-0021 ↔ ADR-0007 from phase 01); (iii) enumerate phase-done gates (SPEC §3 equivalent) including at minimum a new differential fixture that drives traffic through the TCP proxy filter to a multi-endpoint static cluster and verifies round-robin distribution byte-exactly against upstream Envoy; (iv) preserve without regression the phase-01 admin `/ready` byte-exact surface AND the phase-00 TCP echo surface (gate (b) — pre-existing fixtures green); (v) define how the `cmd/envoy-go/main.go` pump (phase-00 netConn/pump/halfClose triple) is retired in favor of a listener-manager-driven TCP proxy dataplane, with a rationale ADR if any byte-level semantics change; (vi) resolve phase-02 deferred decisions (round-robin index state semantics — per-cluster atomic vs per-listener scope; filter chain match semantics — phase-02 subset vs full; per-listener vs per-process lifecycle). Depends-on: phase 01 (done). SPEC §9 carries the phase-01 deferrals picked up here: multiple listeners, multiple clusters, multiple endpoints per cluster, filter chain typed_config dispatch (TCP proxy), real cluster type dispatch (STATIC), round-robin LB. TLS (phase 03), HTTP/* (phases 04–05), observability (phase 06), filter chain framework (phase 07), full admin/drain (phase 08), and dynamic xDS remain out of scope.
- **last-commit:** c1b32df
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
