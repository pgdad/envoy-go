# envoy-go Bootstrap Prompt Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Author a single Markdown file, `BOOTSTRAP_PROMPT.md`, that — when loaded as the first user turn of a fresh Claude Code session with the `superpowers` plugin active — bootstraps and operates an indefinite-duration, phase-based project to reimplement Envoy Proxy in Go with differential parity tests against upstream Envoy.

**Architecture:** The deliverable is a prose artifact, not code. The prompt installs a persistent on-disk state machine under `docs/envoy-go/` so that any subsequent fresh session can resume from disk without conversation history. This plan decomposes authoring into section-by-section tasks, each with an explicit verification step (grep / structural check) against the approved design spec.

**Tech Stack:** Markdown. No build, no runtime, no CI. Verification is textual (presence checks, cross-reference checks) against the spec at `docs/superpowers/specs/2026-04-21-envoy-go-bootstrap-prompt-design.md`.

---

## Spec Reference

All normative content in this plan traces to sections of the approved spec:
`docs/superpowers/specs/2026-04-21-envoy-go-bootstrap-prompt-design.md`

When a task says "copy verbatim from §X of the spec", it means that exact text (modulo trivial formatting) must appear in the prompt. The spec is the single source of truth; the prompt file is the spec rendered into imperative form for a fresh session.

---

## File Structure

Only one file is created:

| Path | Responsibility |
|---|---|
| `BOOTSTRAP_PROMPT.md` (repo root) | The single-file bootstrap prompt. Loaded as first user turn of every fresh session. |

One file is modified at the end:

| Path | Responsibility |
|---|---|
| `README.md` (repo root) | Minimal pointer: "To start or resume work on this project, load `BOOTSTRAP_PROMPT.md` as the first user message of a fresh Claude Code session with the superpowers plugin active." |

No directories under `docs/envoy-go/` are created by *this* plan — those are created *by* the prompt's first execution, as mandated by spec §8.

---

## Section Ordering inside `BOOTSTRAP_PROMPT.md`

The prompt's section order is optimized for fresh-session consumption, not for human readability. Fresh sessions must find the cold-start procedure within the first few hundred tokens. Order:

1. Title + one-paragraph identity
2. **Cold-Start Procedure** (what to do RIGHT NOW)
3. Mission + Non-Purposes
4. Operating Doctrine (hard constraints)
5. On-Disk Artifact Layout
6. Phase Lifecycle State Machine
7. Phase Splitting Policy
8. Differential Test Contract
9. Seeded MVP Trunk (phases 00–08)
10. Feature Families (headings-only, 09+)
11. First-Session Bootstrap Action List (only runs if repo is fresh)
12. Skill Routing Appendix (verbatim state machine reference)
13. Acceptance self-checks (meta, for the prompt's author/reviewer)

This ordering is fixed; tasks below produce sections in this order.

---

## Task 1: Create skeleton file with title, identity, and Table of Contents

**Files:**
- Create: `BOOTSTRAP_PROMPT.md`

- [ ] **Step 1: Create the file with the fixed skeleton**

Write `BOOTSTRAP_PROMPT.md` with exactly this initial content:

```markdown
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
```

- [ ] **Step 2: Verify structural placeholders**

Run: `grep -c '^## ' BOOTSTRAP_PROMPT.md`
Expected: `0` (only the `# ` title and the ToC numbered list so far; no `## ` headings yet).

Run: `grep -c '^# envoy-go Bootstrap Prompt$' BOOTSTRAP_PROMPT.md`
Expected: `1`.

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: scaffold BOOTSTRAP_PROMPT.md with identity and ToC"
```

---

## Task 2: Add the Cold-Start Procedure

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append a new `## 1. Cold-Start Procedure` section after the ToC/`---`)

Rationale for placement: a fresh session under context pressure must hit this section in the first screenful. It must be executable without reading the rest of the prompt.

- [ ] **Step 1: Append the section**

Append after the first `---`:

```markdown
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
```

- [ ] **Step 2: Verify presence of load-bearing phrases**

Run these greps; each must return `1` or more:

```bash
grep -c 'Cold-Start Procedure' BOOTSTRAP_PROMPT.md         # 1
grep -c 'test -d docs/envoy-go' BOOTSTRAP_PROMPT.md        # 1
grep -c 'MISSION.md' BOOTSTRAP_PROMPT.md                   # ≥1
grep -c 'systematic-debugging' BOOTSTRAP_PROMPT.md         # ≥1
grep -c 'No other action first' BOOTSTRAP_PROMPT.md        # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add cold-start procedure (§1)"
```

---

## Task 3: Add Mission and Non-Purposes

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 2. Mission and Non-Purposes`)

Source: spec §1 ("Purpose and non-purposes"). The prompt version must be phrased imperatively ("Your mission is…"), not descriptively.

- [ ] **Step 1: Append the section**

Append:

```markdown
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
```

- [ ] **Step 2: Verify**

```bash
grep -c '^## 2\. Mission and Non-Purposes' BOOTSTRAP_PROMPT.md    # 1
grep -c 'ENVOY_TARGET.md' BOOTSTRAP_PROMPT.md                     # ≥1
grep -c 'gsd-' BOOTSTRAP_PROMPT.md                                # ≥1 (the forbidden-commands note)
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add mission and non-purposes (§2)"
```

---

## Task 4: Add Operating Doctrine (hard constraints)

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 3. Operating Doctrine`)

Source: spec §3 (all seven subsections). Render each as a numbered rule with explicit enforcement verbs ("you must", "you must not", "never").

- [ ] **Step 1: Append the section**

Append:

````markdown
## 3. Operating Doctrine — hard constraints

These rules are non-negotiable. They are named by number so that ADRs and review comments can refer to them as `doctrine D-3.2`, etc.

### D-3.1 Superpowers-first process

| Situation | Required skill |
|---|---|
| Any design artifact about to be written | `superpowers:brainstorming` |
| Any implementation task about to start | `superpowers:writing-plans` first, then `superpowers:executing-plans` or `superpowers:subagent-driven-development` |
| Any implementation task inside a plan | `superpowers:test-driven-development` — tests first, no exceptions |
| Any claim of "done" about to be made | `superpowers:verification-before-completion` |
| Any phase about to be committed as complete | `superpowers:requesting-code-review` |
| Any unexpected state, test failure, or harness divergence | `superpowers:systematic-debugging` — before you propose a fix |

`/gsd-*` commands are forbidden. If you find yourself reaching for one, re-read §1.

### D-3.2 Hybrid implementation stance

**Permitted foundations:**
- Go standard library.
- `golang.org/x/net/http2` as a *low-level codec only* — never as a server runtime.
- `github.com/quic-go/quic-go` for QUIC transport.
- `golang.org/x/crypto`.
- `google.golang.org/protobuf` runtime.
- `github.com/envoyproxy/go-control-plane` — **proto types only**. No control-plane helpers, no filter helpers, no xDS logic imported from this package.
- OpenTelemetry SDK for tracing and metrics integration.
- `github.com/testcontainers/testcontainers-go` for the differential harness.

**Must be written from scratch** (one or more dedicated phases each):
- Filter chain engine (network + HTTP filter iteration protocol).
- Listener manager, cluster manager.
- xDS state machine (ADS/delta, ACK/NACK, version/nonce tracking).
- All load balancing algorithms.
- Active health checking, outlier detection, circuit breakers.
- Access log formatters and sinks.
- Stats subsystem.
- Admin API.
- Runtime layer (RTDS consumer).
- Hot-restart / graceful-drain semantics.
- Every individual filter (network and HTTP).

**Forbidden, without exception:**
- Wrapping `net/http/httputil.ReverseProxy`.
- Embedding Traefik, Caddy, or fasthttp cores.
- Copying GPL-licensed code.
- Vendoring or cgo-binding Envoy C++.

### D-3.3 Differential correctness beats internal fidelity

A phase ships when its feature surface produces output behaviorally-equivalent to upstream Envoy on the same config and inputs, as mechanically defined by `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§7). You do not read Envoy source to decide what "equivalent" means — the contract is the contract.

### D-3.4 Context isolation is the primary design constraint

Every artifact you write — SPEC, PLAN, PROGRESS, REVIEW, ADR — must be readable by a stranger with zero prior context. Never write "as discussed earlier" or "remember to…". If a fact matters across sessions, it must live in `docs/envoy-go/`. This is non-negotiable; phases that violate it will be unreviewable and must be rewritten.

### D-3.5 Decisions are written, not remembered

When you hit an ambiguity not already settled in `docs/envoy-go/DECISIONS.md`, append a new ADR (next sequential `ADR-NNNN`), state the options considered, state the choice, state the rationale, and proceed. ADRs are append-only: never edit a landed ADR; supersede it with a new one that explicitly names the superseded ADR number.

### D-3.6 Every phase is a green build

No phase lands with failing unit tests, failing differential fixtures, failing conformance checks, lint errors, or build errors. Your only release valve is phase splitting (§6). Splitting is cheap, expected, and encouraged.

### D-3.7 Version pinning

The reference Envoy version (Docker image tag + SHA) lives in `docs/envoy-go/ENVOY_TARGET.md`. All fixtures, proto versions, and behavior contracts reference that pin. Upgrading the pin is its own phase, with its own differential re-baselining; you may not change the pin ad-hoc.

---
````

- [ ] **Step 2: Verify**

```bash
grep -c 'D-3\.1' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.2' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.3' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.4' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.5' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.6' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'D-3\.7' BOOTSTRAP_PROMPT.md   # ≥1
grep -c 'httputil.ReverseProxy' BOOTSTRAP_PROMPT.md            # 1
grep -c 'go-control-plane' BOOTSTRAP_PROMPT.md                 # ≥1
grep -c 'proto types only' BOOTSTRAP_PROMPT.md                 # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add operating doctrine (§3, D-3.1 through D-3.7)"
```

---

## Task 5: Add On-Disk Artifact Layout

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 4. On-Disk Artifact Layout`)

Source: spec §4 (the tree plus the invariants). The tree is rendered verbatim as a fenced block; invariants are numbered.

- [ ] **Step 1: Append the section**

Append the following, copying the tree exactly as it appears in spec §4 and the invariant list from spec §4.1:

```markdown
## 4. On-Disk Artifact Layout

This is the only layout the project uses. Phase 00 creates it. Every subsequent phase adheres to it.

<tree-from-spec-§4-verbatim>

### 4.1 Invariants

1. **`STATE.md` is the single source of truth for "what next."** Cold-start reads it first. It names the active phase directory and the next expected skill invocation.
2. **`ROADMAP.md` schema:** columns `id | title | depends-on | status | sub-phases | summary`. Status ∈ `planned | in-progress | blocked | done`. Append-only history; never delete rows, only update status and sub-phases columns.
3. **Phase directory lifecycle:** a phase directory is created *only* when the phase enters `in-progress`. Creating `docs/envoy-go/phases/NN-slug/` and its empty `SPEC.md` is the first concrete file-system act of starting a phase.
4. **`DECISIONS.md` is ADR-numbered, append-only.** Entries are `ADR-0001`, `ADR-0002`, etc. Landed ADRs are never edited; they are superseded by later ADRs that explicitly name the superseded number.
5. **`BEHAVIOR_CONTRACT.md` is the canonical reference** for differential equivalence rules (see §7). If a phase's observed behavior diverges from the contract, either the contract is updated (via ADR) or the implementation is fixed — never both silently.
6. **`SKILL_ROUTING.md` is a verbatim copy** of the state machine in §5 of this prompt. It exists so an executing session does not need to re-parse the whole prompt to route its next action.
7. **`phases/99-archive/`** is used only if `docs/envoy-go/` grows large enough to hurt navigation. Completed phases may be moved there, wholesale, with an ADR documenting the move. Do not move phases there opportunistically.
```

**Note for this step:** Replace `<tree-from-spec-§4-verbatim>` with the fenced directory tree from spec §4 (the block that starts with `envoy-go/` and ends with `└── 99-archive/`). Reproduce it exactly, preserving indentation and the `#` comments.

- [ ] **Step 2: Verify**

```bash
grep -c '^## 4\. On-Disk Artifact Layout' BOOTSTRAP_PROMPT.md   # 1
grep -c 'STATE.md' BOOTSTRAP_PROMPT.md                          # ≥5 (cold-start uses it too)
grep -c 'phases/99-archive/' BOOTSTRAP_PROMPT.md                # ≥1
grep -c 'ADR-0001' BOOTSTRAP_PROMPT.md                          # ≥1
grep -c 'append-only' BOOTSTRAP_PROMPT.md                       # ≥1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add on-disk artifact layout (§4)"
```

---

## Task 6: Add Phase Lifecycle State Machine

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 5. Phase Lifecycle State Machine`)

Source: spec §5.1 (state machine, verbatim). This is one of the two "baked verbatim" sections and must be reproduced character-for-character.

- [ ] **Step 1: Append the section**

Append:

````markdown
## 5. Phase Lifecycle State Machine

This state machine is the brain of the project. A session's entire job, after cold-start, is to match its state against this machine and invoke exactly the skill indicated.

```
<state-machine-from-spec-§5.1-verbatim>
```

### 5.1 How to read this state machine

- Each numbered state has an unambiguous detection rule from the contents of the active phase directory (presence/absence of `SPEC.md`, `PLAN.md`, `PROGRESS.md`, `REVIEW.md` — and for `REVIEW.md`, its approval status).
- You move exactly one state forward per session. Do not chain through multiple states in a single session; the value of context isolation is that each transition starts fresh.
- If state detection is ambiguous (e.g., file exists but is empty, or contains conflicting signals), invoke `superpowers:systematic-debugging` before advancing.

### 5.2 Review feedback re-entry point

If step 5 produces `REVIEW.md` with issues, you re-enter at **step 3**, not step 4. You are resuming implementation (and TDD), not just re-verifying. This is a subtle but important asymmetry.

### 5.3 Commit message format

Final phase commits (step 6) use this format:

```
phase NN: <title> [ADR-NNNN, ADR-MMMM, ...]

<summary — 1–3 sentences>

Differential surface: <what new/existing fixtures are now green>
Conformance: <what conformance suites were run and their pass rate>
```

If no ADRs were added or referenced during the phase, the bracketed list is omitted.

---
````

**Note for this step:** Replace `<state-machine-from-spec-§5.1-verbatim>` with the entire state-machine block from spec §5.1, preserving its column layout and the "Deviations:" subsection at the bottom. The block is sacred — do not paraphrase.

- [ ] **Step 2: Verify**

```bash
grep -c '^## 5\. Phase Lifecycle State Machine' BOOTSTRAP_PROMPT.md   # 1
grep -c 'superpowers:brainstorming' BOOTSTRAP_PROMPT.md               # ≥3
grep -c 'superpowers:writing-plans' BOOTSTRAP_PROMPT.md               # ≥2
grep -c 'superpowers:executing-plans' BOOTSTRAP_PROMPT.md             # ≥1
grep -c 'superpowers:verification-before-completion' BOOTSTRAP_PROMPT.md  # ≥1
grep -c 'superpowers:requesting-code-review' BOOTSTRAP_PROMPT.md      # ≥1
grep -c 'Deviations:' BOOTSTRAP_PROMPT.md                             # ≥1
grep -c 'back to step 3 (NOT 4)' BOOTSTRAP_PROMPT.md                  # 1 (presence confirms verbatim copy)
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add phase lifecycle state machine (§5)"
```

---

## Task 7: Add Phase Splitting Policy

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 6. Phase Splitting Policy`)

Source: spec §5.3. Short and mechanical; a single page.

- [ ] **Step 1: Append the section**

Append:

```markdown
## 6. Phase Splitting Policy

### 6.1 When to split

Splitting is triggered at step 2 of the lifecycle (when `PLAN.md` is being written) if either threshold is crossed:

- `PLAN.md` exceeds **~25 numbered tasks**, OR
- `PLAN.md` estimates exceed **~1500 lines of code** of net change.

Additionally, splitting is triggered *mid-execution* if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity.

### 6.2 How to split

1. Stop. Do not continue writing the oversize plan or implementing the oversize task.
2. Create sibling phase directories `docs/envoy-go/phases/NN.1-subtitle/`, `NN.2-subtitle/`, …
3. Redistribute spec content — each sub-phase gets its own `SPEC.md` covering a coherent slice of the original.
4. Update `docs/envoy-go/ROADMAP.md`: the original row becomes a parent row with `status = in-progress` and its `sub-phases` column listing `NN.1, NN.2, …`. Each sub-phase gets its own row with `status = planned`.
5. Update `docs/envoy-go/STATE.md` to point at `NN.1`.
6. Append an ADR explaining the split ("ADR-NNNN: split phase NN into NN.1–NN.k because plan exceeded …").
7. Exit. The next fresh session starts at NN.1's lifecycle at step 1.

### 6.3 Anti-pattern

Do not "defer" work by cramming it into vague tasks like "TODO: extend later" or by introducing incomplete stubs that differential tests can't exercise. Either the work is in this phase and gets tested, or it is in a split sub-phase with its own row in the roadmap. There is no third option.

---
```

- [ ] **Step 2: Verify**

```bash
grep -c '^## 6\. Phase Splitting Policy' BOOTSTRAP_PROMPT.md   # 1
grep -c '25 numbered tasks' BOOTSTRAP_PROMPT.md                # 1
grep -c '1500 lines' BOOTSTRAP_PROMPT.md                       # 1
grep -c 'Anti-pattern' BOOTSTRAP_PROMPT.md                     # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add phase splitting policy (§6)"
```

---

## Task 8: Add Differential Test Contract

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 7. Differential Test Contract`)

Source: spec §6 (all five subsections). Note: spec §6.5's done-gate wording is **verbatim-required** (see spec §9 acceptance criterion 5). The equivalence matrix from spec §6.2 is copied as a table.

- [ ] **Step 1: Append the section**

Append:

````markdown
## 7. Differential Test Contract

### 7.1 Harness architecture

`test/differential/` hosts a Go test binary that orchestrates two proxies per fixture:

- **Reference:** upstream Envoy, Docker image at the tag pinned in `docs/envoy-go/ENVOY_TARGET.md`, managed via `testcontainers-go`.
- **Subject:** envoy-go built from the current tree, run as a subprocess.

Each test case lives under `test/fixtures/NNNN-name/` and contains:

- `envoy.yaml` — reference config for upstream Envoy.
- `envoy-go.yaml` — equivalent config for envoy-go (initially identical; any divergence must be explained in an ADR referenced from the fixture's README).
- `inputs/` — HTTP requests, raw TCP payloads, gRPC calls, or a small Go driver that exercises the fixture.
- `expectations.yaml` — allow-lists, ignore-lists, stats-name mappings, and timing tolerances, derived from `BEHAVIOR_CONTRACT.md`.

Per run: start both proxies; drive identical inputs at both; capture responses, access logs, and stats snapshots; diff under the contract rules.

### 7.2 Equivalence matrix

The authoritative version lives in `docs/envoy-go/BEHAVIOR_CONTRACT.md`. Summary:

| Dimension | Required equivalence |
|---|---|
| Response status | Exact |
| Response body | Byte-exact for deterministic handlers; semantically equal for filter-modified bodies |
| Response headers | Set-equal modulo documented allow-list (`server`, `date`, timing/identity headers explicitly listed) |
| Response trailers | Set-equal under the same allow-list discipline |
| HTTP/2 & HTTP/3 framing | Structurally equivalent (same frame types/order on equivalent events); not byte-equal |
| Access log records | Semantically equal after field-mapping |
| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |

### 7.3 Conformance suites (independent of real Envoy)

Separate from the differential harness. These test absolute protocol correctness:

- `test/conformance/h2spec/` — runs once HTTP/2 lands; pass threshold is a phase gate.
- `test/conformance/h3spec/` — runs once HTTP/3 lands.
- `test/conformance/grpc/` — gRPC interop client.
- `test/conformance/proxy-wasm/` — proxy-wasm ABI conformance once the WASM host lands.

### 7.4 Negative and fuzz testing

Every phase that introduces a parser, codec, or filter ships a Go fuzz target under `test/`. Fuzzers run short-budget in CI and long-budget nightly. Malformed or adversarial inputs must produce the *same class* of response as upstream Envoy (matching status code and Envoy-style `x-envoy-local-reply` behavior) — identical error text is not required.

### 7.5 Phase-done gate

> A phase is not done until:
> (a) all new/changed differential fixtures are green,
> (b) all pre-existing differential fixtures are still green,
> (c) the phase's conformance suites pass at the declared threshold,
> (d) any new fuzzer has run clean for its short-budget CI run,
> (e) `go vet`, `golangci-lint run`, `go test ./...` are all clean,
> (f) `REVIEW.md` is approved.

These six gates are what `superpowers:verification-before-completion` verifies. They are the complete definition of "done."

---
````

- [ ] **Step 2: Verify**

The six-part done-gate must be present verbatim. Check each line:

```bash
grep -c 'all new/changed differential fixtures are green' BOOTSTRAP_PROMPT.md           # 1
grep -c 'all pre-existing differential fixtures are still green' BOOTSTRAP_PROMPT.md    # 1
grep -c "phase's conformance suites pass at the declared threshold" BOOTSTRAP_PROMPT.md # 1
grep -c 'any new fuzzer has run clean for its short-budget CI run' BOOTSTRAP_PROMPT.md  # 1
grep -c 'go vet.*golangci-lint run.*go test' BOOTSTRAP_PROMPT.md                        # 1
grep -c 'REVIEW.md.*approved' BOOTSTRAP_PROMPT.md                                       # ≥1
grep -c 'testcontainers-go' BOOTSTRAP_PROMPT.md                                         # ≥1
grep -c 'h2spec' BOOTSTRAP_PROMPT.md                                                    # ≥1
grep -c 'proxy-wasm' BOOTSTRAP_PROMPT.md                                                # ≥1
grep -c 'x-envoy-local-reply' BOOTSTRAP_PROMPT.md                                       # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add differential test contract (§7)"
```

---

## Task 9: Add Seeded MVP Trunk (phases 00–08)

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 8. Seeded MVP Trunk`)

Source: spec §7.1. This section is data, not doctrine — it seeds `ROADMAP.md`. The nine rows must match the spec table exactly.

- [ ] **Step 1: Append the section**

Append, copying the table from spec §7.1 exactly:

```markdown
## 8. Seeded MVP Trunk — phases 00 through 08

Phase 00 copies these rows verbatim into `docs/envoy-go/ROADMAP.md`. Subsequent phases brainstorm their own `SPEC.md` when entered, but the titles, IDs, and ordering below are fixed.

<mvp-table-from-spec-§7.1-verbatim>

**Invariant:** Phases 00–08 ship *in order*. Each depends on the previous one having landed green, because each adds a primitive the next relies on. Splitting (§6) is still permitted within any of these phases.

After phase 08 lands, envoy-go is a minimal but real proxy. At that point you transition to feature-family expansion (§9).

---
```

**Note for this step:** Replace `<mvp-table-from-spec-§7.1-verbatim>` with the 9-row table from spec §7.1, preserving the column headers `#`, `Title`, `Differential surface at phase end`.

- [ ] **Step 2: Verify**

```bash
grep -c '^## 8\. Seeded MVP Trunk' BOOTSTRAP_PROMPT.md   # 1
grep -c '^| 00 |' BOOTSTRAP_PROMPT.md                    # 1
grep -c '^| 01 |' BOOTSTRAP_PROMPT.md                    # 1
grep -c '^| 08 |' BOOTSTRAP_PROMPT.md                    # 1
grep -c 'filter chain framework' BOOTSTRAP_PROMPT.md     # ≥1 (phase 07)
grep -c 'admin API' BOOTSTRAP_PROMPT.md                  # ≥1 (phase 08)
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add seeded MVP trunk, phases 00-08 (§8)"
```

---

## Task 10: Add Feature Families (headings-only)

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 9. Feature Families`)

Source: spec §7.2. The family list is rendered as bullet headings only — no per-phase rows. This is intentional and explicit; do not "helpfully" expand it.

- [ ] **Step 1: Append the section**

Append, copying the family list from spec §7.2 exactly (including the bracketed `[scope TBD]` on `zookeeper_proxy`):

```markdown
## 9. Feature Families — phases 09 and onward (headings only)

Phase 00 seeds these as headings in `docs/envoy-go/ROADMAP.md`. Do **not** expand them into per-phase rows now. Each family is brainstormed as its own phase when it enters `in-progress`, and split (§6) as reality demands.

<family-list-from-spec-§7.2-verbatim>

---
```

**Note for this step:** Replace `<family-list-from-spec-§7.2-verbatim>` with the bulleted family list from spec §7.2, preserving bullet order and the `[scope TBD]` bracket on zookeeper_proxy verbatim.

- [ ] **Step 2: Verify**

```bash
grep -c '^## 9\. Feature Families' BOOTSTRAP_PROMPT.md   # 1
grep -c 'scope TBD' BOOTSTRAP_PROMPT.md                  # 1  (zookeeper bracket preserved)
grep -c 'WASM host family' BOOTSTRAP_PROMPT.md           # ≥1
grep -c 'xDS / dynamic config family' BOOTSTRAP_PROMPT.md  # ≥1
grep -c 'Load balancing family' BOOTSTRAP_PROMPT.md      # ≥1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add feature families headings, 09+ (§9)"
```

---

## Task 11: Add First-Session Bootstrap Action List

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 10. First-Session Bootstrap`)

Source: spec §8. This section is reached only from cold-start Step A when the output is `FRESH`. It is self-contained and must work without the reader having read §§2–9.

- [ ] **Step 1: Append the section**

Append:

````markdown
## 10. First-Session Bootstrap — runs only once, on an empty repo

You only reach this section if cold-start Step A detected `FRESH`. If you reached it any other way, stop and re-read §1. Do not run these steps twice.

### Step 1: Sanity check

```bash
test ! -d docs/envoy-go || { echo "NOT FRESH — stop"; exit 1; }
git log --oneline -1 2>/dev/null | head -1
```

The repo may be empty (no commits) or contain only the prompt itself / a README. Anything more means something is already there — stop and invoke `superpowers:systematic-debugging` on that state before proceeding.

### Step 2: Create the `docs/envoy-go/` skeleton

Create, in this order:

1. `docs/envoy-go/MISSION.md` — copy §§2 and 3 of this prompt verbatim (mission + doctrine). This makes the mission durable independently of this prompt file.
2. `docs/envoy-go/ROADMAP.md` — create with:
   - A header explaining the schema (`id | title | depends-on | status | sub-phases | summary`).
   - Rows for phases 00 through 08, copied from §8 of this prompt, all with `status = planned`.
   - Family headings 09+ copied from §9, without rows under them yet.
3. `docs/envoy-go/STATE.md` — points at phase 00, with `next-skill = superpowers:brainstorming`, and an explicit "last-updated" timestamp.
4. `docs/envoy-go/DECISIONS.md` — seeded with `ADR-0001: bootstrap prompt version X committed at <git SHA>`. The SHA is the SHA of the BOOTSTRAP_PROMPT.md commit you are operating under; compute with `git log -1 --format=%H -- BOOTSTRAP_PROMPT.md`.
5. `docs/envoy-go/ENVOY_TARGET.md` — empty placeholder with a one-line note: "To be filled during phase 00. Must pin an upstream Envoy Docker image by tag and SHA256."
6. `docs/envoy-go/BEHAVIOR_CONTRACT.md` — skeleton populated with the equivalence matrix from §7.2 of this prompt, plus explicit empty subsections (`Header allow-list`, `Stat-name mapping`, `Access log field mapping`, `xDS wire state machine`, `Timing tolerances`) each marked "to be filled per-phase as needed."
7. `docs/envoy-go/SKILL_ROUTING.md` — verbatim copy of §5's state machine (just the state machine block, not the surrounding prose).

### Step 3: Commit the scaffold as a single commit

```bash
git add docs/envoy-go/
git commit -m "bootstrap: envoy-go project scaffold"
```

### Step 4: Enter phase 00 lifecycle at state 1

Create `docs/envoy-go/phases/00-bootstrap/` (empty). Invoke `superpowers:brainstorming` scoped to phase 00. The brainstorm produces `phases/00-bootstrap/SPEC.md`. Do not go further in this session — the next session, per the state machine (§5), will write `PLAN.md`.

### Step 5: Exit

Update `docs/envoy-go/STATE.md` to reflect: active phase = `00-bootstrap`, next-skill = `superpowers:writing-plans`. Exit cleanly.

---
````

- [ ] **Step 2: Verify**

```bash
grep -c '^## 10\. First-Session Bootstrap' BOOTSTRAP_PROMPT.md   # 1
grep -c 'bootstrap: envoy-go project scaffold' BOOTSTRAP_PROMPT.md  # 1
grep -c 'ADR-0001' BOOTSTRAP_PROMPT.md                              # ≥2  (doctrine also mentions ADR-0001 format)
grep -c 'next-skill' BOOTSTRAP_PROMPT.md                            # ≥2
grep -c 'Do not go further in this session' BOOTSTRAP_PROMPT.md     # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add first-session bootstrap action list (§10)"
```

---

## Task 12: Add Skill Routing Appendix

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 11. Skill Routing Appendix`)

Source: spec §5.1 state machine (same block, reproduced as an appendix so a reader needing just the routing table doesn't have to scroll into §5's prose).

- [ ] **Step 1: Append the section**

Append:

````markdown
## 11. Skill Routing Appendix

This is the same state machine as §5, duplicated here as a reference card for `docs/envoy-go/SKILL_ROUTING.md` to be copied from. If §5 and §11 ever diverge, §5 wins and §11 must be corrected.

```
<state-machine-from-spec-§5.1-verbatim>
```

---
````

**Note for this step:** Replace the placeholder with the exact same state-machine block used in Task 6. The duplication is intentional — `SKILL_ROUTING.md` is instructed to be copied from this appendix, not reconstructed from prose.

- [ ] **Step 2: Verify duplication is exact**

```bash
# Count occurrences of a characteristic phrase that appears once in §5 and once in §11
grep -c 'back to step 3 (NOT 4)' BOOTSTRAP_PROMPT.md                # 2
grep -c '^## 11\. Skill Routing Appendix' BOOTSTRAP_PROMPT.md       # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add skill routing appendix (§11)"
```

---

## Task 13: Add Acceptance Self-Checks and close the file

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (append `## 12. Acceptance Self-Checks`)

Source: spec §9 (acceptance criteria for the prompt itself). This section is metadata for the prompt's authors/reviewers — it does **not** direct the executing session to do anything. A comment at the top of the section says so.

- [ ] **Step 1: Append the section**

Append:

```markdown
## 12. Acceptance Self-Checks

> **Note to the executing session:** This section is metadata for the prompt's authors and reviewers. It does not direct you to do anything. Skip it.

The bootstrap prompt itself is considered done when:

1. Loaded into a fresh Claude Code session with the `superpowers` plugin active, the prompt produces the §10 bootstrap without further human input beyond initial send.
2. A second fresh session loaded with the same prompt correctly resumes from disk state — it does not re-run bootstrap.
3. Every doctrine rule in §3 appears in the prompt with explicit enforcement verbs (`must`, `must not`, `never`).
4. The phase lifecycle state machine (§5) appears verbatim in both §5 and §11.
5. The six-part phase-done gate (§7.5) appears verbatim.
6. The MVP trunk (§8) is seeded as concrete ROADMAP rows matching the spec.
7. Feature families (§9) are seeded as headings only, including the `[scope TBD]` bracket on zookeeper_proxy.
8. The prompt is self-contained — it references only the `superpowers` skill set and the target repo.

---

*End of bootstrap prompt.*
```

- [ ] **Step 2: Verify**

```bash
grep -c '^## 12\. Acceptance Self-Checks' BOOTSTRAP_PROMPT.md   # 1
grep -c 'End of bootstrap prompt' BOOTSTRAP_PROMPT.md           # 1
```

- [ ] **Step 3: Commit**

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: add acceptance self-checks and close file (§12)"
```

---

## Task 14: Full-file coherence pass against acceptance criteria

**Files:**
- Modify: `BOOTSTRAP_PROMPT.md` (edit-only, no new sections)

This task runs the self-check suite from §12 as an actual verification against the file, and makes any corrective edits needed. No new sections are added.

- [ ] **Step 1: Run the full self-check grep suite**

```bash
echo "=== Doctrine rules present ==="
for n in 1 2 3 4 5 6 7; do
  grep -q "D-3\.${n}" BOOTSTRAP_PROMPT.md && echo "D-3.${n}: OK" || echo "D-3.${n}: MISSING"
done

echo "=== State machine duplicated ==="
count=$(grep -c 'back to step 3 (NOT 4)' BOOTSTRAP_PROMPT.md)
[ "$count" = "2" ] && echo "state machine duplicated: OK" || echo "state machine count=$count (expected 2)"

echo "=== Done-gate verbatim ==="
for phrase in \
  "all new/changed differential fixtures are green" \
  "all pre-existing differential fixtures are still green" \
  "phase's conformance suites pass at the declared threshold" \
  "any new fuzzer has run clean for its short-budget CI run" \
  "REVIEW.md.*approved"; do
  grep -q "$phrase" BOOTSTRAP_PROMPT.md && echo "PRESENT: $phrase" || echo "MISSING: $phrase"
done

echo "=== MVP rows 00..08 present ==="
for n in 00 01 02 03 04 05 06 07 08; do
  grep -q "^| ${n} |" BOOTSTRAP_PROMPT.md && echo "row ${n}: OK" || echo "row ${n}: MISSING"
done

echo "=== zookeeper [scope TBD] preserved ==="
grep -q 'zookeeper.*scope TBD' BOOTSTRAP_PROMPT.md && echo "TBD bracket: OK" || echo "TBD bracket: MISSING"

echo "=== Self-contained: no references outside superpowers / repo ==="
grep -E 'gsd-|/gsd' BOOTSTRAP_PROMPT.md  # only mentions should be in the "forbidden" contexts — spot-check manually
```

Expected output: every OK, nothing MISSING. Any MISSING entry blocks the next step.

- [ ] **Step 2: If any check failed, edit the file to address it, and re-run Step 1.**

Do not commit partial fixes. Only proceed once all checks pass in a single run.

- [ ] **Step 3: Human-readable smoke-read**

Read the entire file top to bottom. Verify:
- §1 Cold-Start is the first section a reader hits after the ToC (i.e., nothing else precedes it).
- §10 First-Session Bootstrap is genuinely self-contained — a reader who jumps straight from §1 to §10 on a FRESH branch has enough to execute.
- No "as discussed above" / "remember to" / "as you know" phrasing anywhere. Grep: `grep -Ein 'as discussed|remember to|as you know|earlier we|previously we' BOOTSTRAP_PROMPT.md` — expected: no output.

- [ ] **Step 4: Commit (if any edits were made in Step 2)**

If Step 2 made edits:

```bash
git add BOOTSTRAP_PROMPT.md
git commit -m "prompt: coherence pass against acceptance self-checks"
```

If Step 2 made no edits, skip this commit.

---

## Task 15: Add top-level README pointer

**Files:**
- Create: `README.md` (repo root)

- [ ] **Step 1: Create the README**

Write `README.md` with exactly this content:

```markdown
# envoy-go

A from-scratch Go reimplementation of the Envoy Proxy, verified phase-by-phase against upstream Envoy via a differential test harness.

## How to work on this project

Load `BOOTSTRAP_PROMPT.md` as the **first user message** of a fresh Claude Code session with the `superpowers` plugin active. The prompt will either bootstrap the project (if this is the first session ever) or resume from the on-disk state in `docs/envoy-go/` (every subsequent session).

Do not edit code by hand outside the prompt-driven workflow. Do not use `/gsd-*` commands — they are not part of this project. See `BOOTSTRAP_PROMPT.md` §3 for the full operating doctrine.

## Project state

- Reference Envoy version: see `docs/envoy-go/ENVOY_TARGET.md`.
- Active phase: see `docs/envoy-go/STATE.md`.
- Phase roadmap: see `docs/envoy-go/ROADMAP.md`.
- Recorded decisions: see `docs/envoy-go/DECISIONS.md`.
```

- [ ] **Step 2: Verify**

```bash
grep -c 'BOOTSTRAP_PROMPT.md' README.md   # ≥2
grep -c 'superpowers' README.md           # ≥2
grep -c 'gsd-' README.md                  # 1 (the forbidden-commands note)
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add root README pointing at BOOTSTRAP_PROMPT.md"
```

---

## Task 16: Final review invocation

**Files:**
- No edits in this task; it is an invocation.

- [ ] **Step 1: Invoke code review**

Invoke `superpowers:requesting-code-review` on the full set of commits made by this plan. Provide the reviewer with:
- Path to this plan document.
- Path to the spec document.
- Path to `BOOTSTRAP_PROMPT.md`.
- Path to `README.md`.

Expected output: review feedback. If feedback requires edits, apply them in a new commit (`prompt: address review feedback`) and re-invoke until approved.

- [ ] **Step 2: Final commit state**

```bash
git log --oneline
```

Expected: one commit per task above (1–15), plus the spec commit made before this plan began, plus any review-addressing commits.

---

## Notes on task granularity

- Section tasks (2–13) are intentionally larger than the 2–5 minute ideal because the unit of work is "write one coherent prose section with verbatim inclusions from the spec." Splitting further would create tasks like "write paragraph 1 of §2" which is not useful.
- Tasks 14, 15, 16 are smaller (under 5 min each).
- This plan has 16 tasks; the 25-task soft ceiling from the prompt itself (§6) is not exceeded.
- Estimated net LoC: the prompt itself is prose, roughly 800–1000 lines of Markdown when complete; the README adds ~20 lines. Well under the 1500 LoC ceiling.
