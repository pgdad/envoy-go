# envoy-go Decisions (ADR log)

Append-only architecture decision record per doctrine `D-3.5`. Entries are numbered sequentially (`ADR-0001`, `ADR-0002`, …). Landed ADRs are never edited; supersede a landed ADR with a new one that explicitly names the superseded ADR number.

---

## ADR-0001: bootstrap prompt version pin

**Status:** Accepted
**Date:** 2026-04-21

### Context

Per `BOOTSTRAP_PROMPT.md` §10 Step 2, the first ADR must record the commit SHA of the bootstrap prompt under which the scaffold was produced. This pins the prompt contents against which `docs/envoy-go/` (MISSION, ROADMAP, BEHAVIOR_CONTRACT, SKILL_ROUTING) was derived.

### Decision

The bootstrap scaffold at `docs/envoy-go/` is derived from `BOOTSTRAP_PROMPT.md` at commit SHA `db4d42686cb2a9b78812a0f27d09e054d2bbbe9b` ("prompt: address final-review feedback — D-3.3 enforcement verb, §5.1 bootstrap exemption"). That commit is the authoritative definition of the prompt for this project's initial state.

### Consequences

- If `BOOTSTRAP_PROMPT.md` is later amended, the differences between the new prompt and the pinned SHA must be reconciled via a new ADR that either (a) supersedes this one and re-derives the scaffold, or (b) records that the amendments are forward-only and do not require re-deriving existing scaffold files.
- Any change to `MISSION.md`, `SKILL_ROUTING.md`, or the §7.2 equivalence matrix in `BEHAVIOR_CONTRACT.md` that is not also reflected in the pinned prompt must be justified by an ADR.

---

## ADR-0002: pre-existing `docs/superpowers/` meta-artifacts are out-of-scope

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.5

### Context

`BOOTSTRAP_PROMPT.md` §10 Step 1 says the repo before bootstrap may contain "only the prompt itself / a README", and that anything more triggers `superpowers:systematic-debugging` before proceeding. When the first bootstrap session ran, the repo was observed to contain, in addition to `BOOTSTRAP_PROMPT.md` and `README.md`:

- `.gitignore` (one line: `.worktrees/`),
- `.worktrees/` (empty, gitignored),
- `docs/superpowers/specs/2026-04-21-envoy-go-bootstrap-prompt-design.md` (a brainstorming spec *for authoring the prompt itself*),
- `docs/superpowers/plans/2026-04-21-envoy-go-bootstrap-prompt.md` (an implementation plan *for authoring the prompt itself*).

`docs/envoy-go/` did not exist (§1 Step A returned `FRESH`, the authoritative test for prior-bootstrap state). No Go module, no `cmd/envoy-go/`, no `phases/NN-slug/`, no `ENVOY_TARGET.md`, and no other envoy-go implementation artifacts were present.

### Decision

The pre-existing files are development artifacts produced when the prompt itself was authored (via `superpowers:brainstorming` and `superpowers:writing-plans`). They are meta-artifacts of producing `BOOTSTRAP_PROMPT.md`, not residue of a prior envoy-go bootstrap. Accordingly:

1. They are declared out-of-scope for the envoy-go project and are left untouched by the bootstrap.
2. The authoritative existence test for prior bootstrap state remains `docs/envoy-go/` presence, per §1 Step A. `FRESH` from that test overrides the heuristic cleanliness guard in §10 Step 1.
3. Future sessions that find `docs/superpowers/` contents during Step A cold-start should consult this ADR and proceed. If new unexplained files appear (envoy-go implementation code outside the tracked layout, or a `docs/envoy-go/` directory whose contents contradict `STATE.md`), that is *different* and still requires `superpowers:systematic-debugging`.

### Consequences

- `.gitignore` is treated as inherited project infrastructure; the bootstrap does not rewrite it.
- The envoy-go project does not depend on `docs/superpowers/` in any way. Removing or relocating those files would not affect envoy-go's state machine.
- §10 Step 1's heuristic is retained for future re-reads of the bootstrap prompt, but this ADR formally narrows its interpretation: "something is already there" means *envoy-go artifacts* are already there, not arbitrary repo content.

---

## ADR-0003: bootstrap scaffold lands via worktree, then merges to master

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.5

### Context

The first bootstrap session ran under a project-wide standing preference (`feedback_git_worktrees`) that any isolated work is performed in a git worktree, regardless of change size, and applies even to Markdown-only work. `BOOTSTRAP_PROMPT.md` §10 Step 3 specifies a scaffold commit but does not prescribe a branch strategy; however, the state machine (§5) requires that subsequent fresh sessions can read `docs/envoy-go/STATE.md` from the repo on whatever branch those sessions land on — in practice, the default branch `master`.

### Decision

The bootstrap scaffold is produced on a dedicated branch `bootstrap` in a worktree at `.worktrees/bootstrap`. All §10 commits (`bootstrap: envoy-go project scaffold` and, later, phase 00's SPEC.md commit) land on that branch. Before session exit, `bootstrap` is fast-forward-merged into `master` so that the next fresh session reading `master` finds the scaffold in place, and the worktree is then retained (not immediately removed) for any in-session follow-up but may be cleaned up via `superpowers:finishing-a-development-branch` in a later session once `master` contains the commits.

### Consequences

- The scaffold is isolated from `master` during production, satisfying the worktree preference.
- Future phases follow the same pattern: each phase runs in its own worktree branch (e.g. `phase/00-bootstrap-plan`, `phase/04-http-1.1`, etc.) and fast-forwards into `master` on session exit. This is not yet a hard doctrine; if a future session finds this pattern unwieldy, it may supersede this ADR with a new one.
- The next cold-start session reading `docs/envoy-go/STATE.md` will find that file on `master` and proceed per the state machine without needing to know about this branching detail.

---

## ADR-0004: autonomous-brainstorming adaptation for envoy-go phases

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.1, D-3.5

### Context

`superpowers:brainstorming` is designed as an interactive collaborative-dialogue skill: it asks clarifying questions one at a time, presents 2–3 approaches, requires user approval of each design section, writes a spec document, then runs a `spec-document-reviewer` subagent loop, and finally asks the human to review the spec. The `HARD-GATE` explicitly forbids writing any implementation artifact before the human approves the design.

`BOOTSTRAP_PROMPT.md` §2.2 (Non-purposes) states the envoy-go project is not authorized to resolve ambiguities by asking a human mid-phase: instead, ambiguities must be settled via an ADR in `DECISIONS.md`, and the session proceeds. §3 doctrine `D-3.1` still requires `superpowers:brainstorming` be the skill that produces any design artifact, and §5 state machine step 1 requires a SPEC.md as the output of brainstorming. These rules collide with the interactive-dialogue assumptions in the skill as published.

### Decision

For every phase in the envoy-go project (starting with phase 00), `superpowers:brainstorming` is invoked in an *adapted autonomous mode* with the following rules:

1. **No clarifying questions to a human.** The session self-answers by making engineering calls consistent with `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, and prior ADRs. Calls that have cross-phase impact are recorded as new ADRs.
2. **No interactive design presentation.** The session produces the design directly as `docs/envoy-go/phases/NN-slug/SPEC.md` — at the envoy-go path, not the skill's default `docs/superpowers/specs/` path (per the skill's "user preferences override default location" clause; the project-level location is specified in `BOOTSTRAP_PROMPT.md` §4).
3. **Spec review loop is retained, in subagent form.** After writing SPEC.md, the session dispatches the `spec-document-reviewer` subagent (using the template at the skill's `spec-document-reviewer-prompt.md`). Up to three review iterations are permitted. If the subagent cannot approve after three iterations, the session surfaces the situation by setting `STATE.md` `lifecycle-state` to `blocked`, recording a `block-reason`, and exiting — a subsequent session or human must unblock. No autonomous override of a non-approving review.
4. **User-review gate is skipped.** The skill's "user reviews spec before writing-plans" step is explicitly not applicable. Transition to writing-plans happens via the state machine in a fresh session, per §5 step 2.
5. **HARD-GATE on implementation remains in force.** The adapted mode changes *who* approves the design (the subagent reviewer instead of a human), not *whether* implementation artifacts are allowed before approval. No Go code, no CI wiring, no fixtures may be written until SPEC.md is both complete and approved by the reviewer subagent.

### Consequences

- Every phase's brainstorming step runs deterministically in one session without human interaction, satisfying `BOOTSTRAP_PROMPT.md`'s autonomy requirement.
- The reviewer subagent enforces the completeness/consistency/clarity/scope/YAGNI checks that a human would — the spec quality bar does not drop.
- Decisions that would have been elicited by a human's clarifying questions are instead either pre-answered by the prompt/ROADMAP, or ADRd as deferred to the planner.
- If the subagent reviewer escalates, the project fails *safely*: the session exits blocked rather than shipping an unreviewed spec.
- This ADR applies uniformly to phase 00 and every subsequent phase. It is a project-level operating rule, not a phase-local decision.

