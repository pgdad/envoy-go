# envoy-go Bootstrap Prompt

You are operating inside a fresh Claude Code session with the `superpowers` plugin active. You have just been handed this prompt as your first user message. It is the only instruction you will receive. There is no prior conversation history. There is no human available to clarify mid-task.

This prompt drives an indefinite-duration, phase-based project to reimplement the Envoy Proxy in Go, with every phase's output verified against upstream Envoy via a differential test harness. Your job is to execute exactly one unit of progress — usually one phase, sometimes one sub-phase — advance the on-disk state, and exit cleanly. The next fresh session will read the same on-disk state and continue.

Read this entire prompt once before taking any action.

## Table of Contents

1. Cold-Start Procedure — do this first, every time
2. Mission and Non-Purposes
3. Operating Doctrine
4. On-Disk Artifact Layout
5. Phase Lifecycle State Machine
6. Phase Splitting Policy
7. Differential Test Contract
8. Seeded MVP Trunk (phases 00–08)
9. Feature Families (09+)
10. First-Session Bootstrap (runs only once, on an empty repo)
11. Skill Routing Appendix
12. Acceptance Self-Checks

---

## 1. Cold-Start Procedure — do this FIRST, every time

This is the only section you must read before acting. If any step here contradicts a later section, this section wins.

**Step A — Determine project state.** Run:

```bash
test -d docs/envoy-go && echo EXISTS || echo FRESH
```

- If output is `FRESH`: you are the first session. Jump to §10 (First-Session Bootstrap). Do not read intermediate sections first; §10 is self-contained.
- If output is `EXISTS`: continue to Step B.

**Step B — Read the persistent state, in this order, in full:**

1. `docs/envoy-go/MISSION.md`
2. `docs/envoy-go/STATE.md`
3. `docs/envoy-go/ROADMAP.md`
4. `docs/envoy-go/DECISIONS.md`
5. `docs/envoy-go/BEHAVIOR_CONTRACT.md`
6. `docs/envoy-go/SKILL_ROUTING.md`

If any of those files is missing, treat the repo as corrupted. Invoke `superpowers:systematic-debugging` with the specific missing file as the symptom before any other action. Do not attempt to recreate the file from memory — it must be reconstructed from git history or the human must be notified via a `CORRUPTED.md` file at repo root, and you exit.

**Step C — Read the active phase's artifacts.** `STATE.md` names the active phase directory (e.g. `phases/04-http-1.1/`). Read, in full:

1. `docs/envoy-go/phases/<active>/SPEC.md` (if present)
2. `docs/envoy-go/phases/<active>/PLAN.md` (if present)
3. `docs/envoy-go/phases/<active>/PROGRESS.md` (if present)
4. `docs/envoy-go/phases/<active>/REVIEW.md` (if present)

**Step D — Match your state against the Phase Lifecycle State Machine (§5) and invoke exactly the skill it indicates.** No other action first. Not a quick peek at the code. Not a `git status`. The skill invocation IS your first action.

**Step E — On unexpected state** (e.g. `STATE.md` says "in-progress" but no phase directory exists, or `PLAN.md` exists but `PROGRESS.md` claims completion without a REVIEW.md): do not improvise. Invoke `superpowers:systematic-debugging` on the specific discrepancy before any other action.

**Never**:
- Never skip Step B to "save time." The project has no conversation memory — disk is the only memory.
- Never take any file-mutating action before Step D.
- Never invent facts "from context" — if it's not on disk, it does not exist for you.

---

## 2. Mission and Non-Purposes

### 2.1 Mission

Reimplement the Envoy Proxy (https://www.envoyproxy.io/) in Go, feature-complete relative to the upstream version pinned in `docs/envoy-go/ENVOY_TARGET.md`, such that every implemented surface produces behaviorally-equivalent output to upstream Envoy under the differential test contract defined in §7.

The project is executed as an open-ended sequence of phases, each phase self-contained enough to run in a fresh session with zero prior context. Every phase ends with a green build, green tests, a green differential suite for the feature surface covered so far, and a committed review.

### 2.2 Non-purposes

- You are **not** reproducing Envoy's C++ source structure, naming, or internal ABI.
- You are **not** chasing byte-for-byte wire equivalence where the differential contract (§7) does not require it.
- You are **not** free to skip skills, tests, or reviews under time pressure. Phase splitting (§6) is the only release valve.
- You are **not** authorized to use `/gsd-*` commands. They do not belong to this project.
- You are **not** resolving ambiguities by asking a human mid-phase. Write an ADR in `docs/envoy-go/DECISIONS.md` and proceed.

---
