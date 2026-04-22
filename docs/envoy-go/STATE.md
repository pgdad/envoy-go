# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `01-static-bootstrap-config`
- **phase-directory:** `docs/envoy-go/phases/01-static-bootstrap-config/` — does not yet exist; the next session creates it as its first file-system act (per BOOTSTRAP_PROMPT §4.1 invariant 3).
- **lifecycle-state:** `1` — Phase in ROADMAP, directory does not exist. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 1, the next session creates the phase directory and runs `superpowers:brainstorming` scoped to THIS phase, producing `SPEC.md`.
- **next-skill:** `superpowers:brainstorming`
- **next-skill-scope:** Brainstorm `docs/envoy-go/phases/01-static-bootstrap-config/SPEC.md` for phase 01. ROADMAP row 01 summary: *"Static bootstrap config loader (node, admin, static_resources skeleton). Config parses; admin `/ready` behaves like Envoy."* (ROADMAP.md:32). The SPEC must: define the bootstrap YAML subset parsed in this phase (node, admin, static_resources at skeleton depth); specify how `envoy-go` advances from phase 00's placeholder binary to a real bootstrap loader that consumes the same `envoy.yaml` format upstream Envoy accepts; define the admin `/ready` equivalence rule (per BEHAVIOR_CONTRACT.md equivalence-matrix discipline); enumerate phase-done gates (SPEC §3 equivalent); and land without breaking phase 00's differential fixture. Depends-on: phase 00 (done). The existing `test/fixtures/0000-tcp-echo` fixture's `ReferenceBootstrap()` placeholder-replacement pattern (codified in BEHAVIOR_CONTRACT.md's "Test harness host networking" subsection) is the substitution mechanism that phase 01 replaces with proper templating. ADR check: the phase may need ADRs for bootstrap-loader ergonomics beyond ADR-0007's YAML-library choice (`gopkg.in/yaml.v3`), admin-framework shape, and the `/ready` state-machine scope; each materialized ambiguity lands as a new sequential `ADR-NNNN` per D-3.5.
- **last-commit:** 4445768
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
