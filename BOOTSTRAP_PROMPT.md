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

A phase ships when its feature surface produces output behaviorally-equivalent to upstream Envoy on the same config and inputs, as mechanically defined by `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§7). You must not read Envoy source to decide what "equivalent" means — the contract is the contract.

### D-3.4 Context isolation is the primary design constraint

Every artifact you write — SPEC, PLAN, PROGRESS, REVIEW, ADR — must be readable by a stranger with zero prior context. Never write "as discussed earlier" or "remember to…". If a fact matters across sessions, it must live in `docs/envoy-go/`. This is non-negotiable; phases that violate it will be unreviewable and must be rewritten.

### D-3.5 Decisions are written, not remembered

When you hit an ambiguity not already settled in `docs/envoy-go/DECISIONS.md`, append a new ADR (next sequential `ADR-NNNN`), state the options considered, state the choice, state the rationale, and proceed. ADRs are append-only: never edit a landed ADR; supersede it with a new one that explicitly names the superseded ADR number.

### D-3.6 Every phase is a green build

No phase lands with failing unit tests, failing differential fixtures, failing conformance checks, lint errors, or build errors. Your only release valve is phase splitting (§6). Splitting is cheap, expected, and encouraged.

### D-3.7 Version pinning

The reference Envoy version (Docker image tag + SHA) lives in `docs/envoy-go/ENVOY_TARGET.md`. All fixtures, proto versions, and behavior contracts reference that pin. Upgrading the pin is its own phase, with its own differential re-baselining; you may not change the pin ad-hoc.

---

## 4. On-Disk Artifact Layout

This is the only layout the project uses. Phase 00 creates it. Every subsequent phase adheres to it.

```
envoy-go/
├── README.md                        # short: what this is, how to resume a session
├── go.mod / go.sum
├── cmd/envoy-go/                    # the binary
├── internal/                        # implementation (Envoy-specific, written from scratch)
│   ├── listener/
│   ├── cluster/
│   ├── filter/
│   ├── http/   tcp/   tls/
│   ├── xds/    admin/   stats/   accesslog/   runtime/   …
├── pkg/                             # stable public API (if/when any)
├── test/
│   ├── differential/                # real-Envoy-vs-envoy-go harness
│   ├── conformance/                 # h2spec, h3spec, grpc-conformance drivers
│   ├── fixtures/                    # paired configs (envoy.yaml ↔ envoy-go.yaml)
│   └── helpers/
└── docs/envoy-go/
    ├── MISSION.md                   # copy of the prompt's mission + doctrine (stable)
    ├── ROADMAP.md                   # phase list with status column; append-only history
    ├── STATE.md                     # pointer: active phase, last commit, next action
    ├── DECISIONS.md                 # ADR log (numbered, append-only)
    ├── ENVOY_TARGET.md              # pinned upstream version + how to refresh the image
    ├── BEHAVIOR_CONTRACT.md         # what "behaviorally equivalent" means, per layer
    ├── SKILL_ROUTING.md             # which superpowers skill runs at which phase boundary
    └── phases/
        ├── 00-bootstrap/
        │   ├── SPEC.md              # brainstorming output
        │   ├── PLAN.md              # writing-plans output
        │   ├── PROGRESS.md          # running log, updated by executor
        │   └── REVIEW.md            # requesting-code-review output
        ├── 01-static-bootstrap-config/
        ├── 02-tcp-proxy/
        ├── …
        └── 99-archive/              # completed phases' artifacts can be moved here if docs/ grows
```

### 4.1 Invariants

1. **`STATE.md` is the single source of truth for "what next."** Cold-start reads it first. It names the active phase directory and the next expected skill invocation.
2. **`ROADMAP.md` schema:** columns `id | title | depends-on | status | sub-phases | summary`. Status ∈ `planned | in-progress | blocked | done`. Append-only history; never delete rows, only update status and sub-phases columns.
3. **Phase directory lifecycle:** a phase directory is created *only* when the phase enters `in-progress`. Creating `docs/envoy-go/phases/NN-slug/` and its empty `SPEC.md` is the first concrete file-system act of starting a phase.
4. **`DECISIONS.md` is ADR-numbered, append-only.** Entries are `ADR-0001`, `ADR-0002`, etc. Landed ADRs are never edited; they are superseded by later ADRs that explicitly name the superseded number.
5. **`BEHAVIOR_CONTRACT.md` is the canonical reference** for differential equivalence rules (see §7). If a phase's observed behavior diverges from the contract, either the contract is updated (via ADR) or the implementation is fixed — never both silently.
6. **`SKILL_ROUTING.md` is a verbatim copy** of the state machine in §5 of this prompt. It exists so an executing session does not need to re-parse the whole prompt to route its next action.
7. **`phases/99-archive/`** is used only if `docs/envoy-go/` grows large enough to hurt navigation. Completed phases may be moved there, wholesale, with an ADR documenting the move. Do not move phases there opportunistically.

---

## 5. Phase Lifecycle State Machine

This state machine is the brain of the project. A session's entire job, after cold-start, is to match its state against this machine and invoke exactly the skill indicated.

```
0. Phase not yet in ROADMAP.md
   → superpowers:brainstorming (adds/refines row in ROADMAP)

1. Phase in ROADMAP, directory does not exist
   → create docs/envoy-go/phases/NN-slug/
   → superpowers:brainstorming (scoped to THIS phase)
   → output: SPEC.md

2. SPEC.md exists, PLAN.md does not
   → superpowers:writing-plans
   → output: PLAN.md
   → GATE: if PLAN.md > ~25 tasks OR > ~1500 LoC estimated
           → split into NN.1, NN.2, …; update ROADMAP + STATE; stop

3. PLAN.md exists, implementation incomplete
   → superpowers:executing-plans (or subagent-driven-development for independent tasks)
   → TDD per superpowers:test-driven-development on every task
   → append to PROGRESS.md on each task completion

4. Implementation complete, not verified
   → superpowers:verification-before-completion
   → run: go build, go vet, golangci-lint, go test ./...,
          differential suite for phase's feature surface, conformance suites
   → quote all command outputs into PROGRESS.md

5. Verified, not reviewed
   → superpowers:requesting-code-review
   → output: REVIEW.md
   → if issues → back to step 3 (NOT 4) until REVIEW.md approved

6. Reviewed and approved
   → commit (message format: "phase NN: <title> [ADR-xxxx,...]")
   → ROADMAP.md status → done
   → STATE.md advanced to next phase or "awaiting next planning"
   → phase ends; session may exit

Deviations:
  * Ambiguity           → ADR + proceed
  * Blocked by upstream → ROADMAP status=blocked, STATE note, exit clean
  * Unexpected state    → superpowers:systematic-debugging FIRST
```

### 5.1 How to read this state machine

- Each numbered state has an unambiguous detection rule from the contents of the active phase directory (presence/absence of `SPEC.md`, `PLAN.md`, `PROGRESS.md`, `REVIEW.md` — and for `REVIEW.md`, its approval status).
- You move exactly one state forward per session. Do not chain through multiple states in a single session; the value of context isolation is that each transition starts fresh. (The sole exception is §10's first-session bootstrap, which traverses states 0 and 1 in one session because it creates the scaffolding preconditions for the state machine; this exception is unavailable to any subsequent session.)
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

## 8. Seeded MVP Trunk — phases 00 through 08

Phase 00 copies these rows verbatim into `docs/envoy-go/ROADMAP.md`. Subsequent phases brainstorm their own `SPEC.md` when entered, but the titles, IDs, and ordering below are fixed.

| # | Title | Differential surface at phase end |
|---|---|---|
| 00 | Bootstrap: repo layout, CI, Docker reference Envoy, differential harness skeleton, `ENVOY_TARGET.md` pin, trivial echo fixture | harness boots; one TCP echo fixture green |
| 01 | Static bootstrap config loader (node, admin, static_resources skeleton) | config parses; admin `/ready` behaves like Envoy |
| 02 | Listener + TCP proxy filter + static cluster + round-robin LB (plaintext) | TCP proxy fixture green |
| 03 | Downstream TLS termination + upstream TLS origination + SNI | TLS TCP fixture green |
| 04 | HTTP connection manager (HTTP/1.1) + route match + router filter + direct_response | HTTP/1.1 routing fixture green |
| 05 | HTTP/2 downstream + upstream (low-level framer, own conn mgr) | HTTP/2 fixture green; `h2spec` above threshold |
| 06 | Access log (file sink, Envoy default format) + stats + Prometheus admin endpoint | access log + Prometheus fixtures green |
| 07 | Filter chain framework: iteration protocol, per-route config, extension registry | framework fixtures green; trivial pluggable filter covers all iteration states |
| 08 | Minimum admin API (config_dump, stats, clusters, listeners, ready, server_info) + graceful drain | admin + drain fixtures green |

**Invariant:** Phases 00–08 ship *in order*. Each depends on the previous one having landed green, because each adds a primitive the next relies on. Splitting (§6) is still permitted within any of these phases.

After phase 08 lands, envoy-go is a minimal but real proxy. At that point you transition to feature-family expansion (§9).

---

## 9. Feature Families — phases 09 and onward (headings only)

Phase 00 seeds these as headings in `docs/envoy-go/ROADMAP.md`. Do **not** expand them into per-phase rows now. Each family is brainstormed as its own phase when it enters `in-progress`, and split (§6) as reality demands.

- HTTP filters family (header manipulation, cors, compression, fault, local+global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit).
- Network filters family (redis, mongo, kafka_broker, thrift, zookeeper [scope TBD], echo, direct_response, sni_cluster, rbac network).
- Load balancing family (least_request, random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds).
- Upstream robustness family (active health checks HTTP/TCP/gRPC/custom, outlier detection variants, circuit breakers, retries + hedging, per-protocol connection pooling).
- HTTP/3 + QUIC family (quic-go transport, downstream H3 listener, upstream H3 cluster, `h3spec` gate).
- gRPC family (gRPC bridge, gRPC-Web, gRPC-JSON transcoding, interop conformance).
- xDS / dynamic config family (ADS, delta xDS, LDS, CDS, RDS, EDS, SDS, RTDS, reconnection, initial-fetch timeout).
- Observability family (gRPC ALS, OTLP access log, OTel/Zipkin/Jaeger/Datadog/XRay tracing, stats sinks, tap filter).
- Runtime + hot restart family.
- WASM host family (own multi-phase sub-project; ABI, engine binding, proxy-wasm conformance).
- Deprecated / edge features (explicit out-of-scope ADRs unless later re-opened).

---

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

## 11. Skill Routing Appendix

This is the same state machine as §5, duplicated here as a reference card for `docs/envoy-go/SKILL_ROUTING.md` to be copied from. If §5 and §11 ever diverge, §5 wins and §11 must be corrected.

```
0. Phase not yet in ROADMAP.md
   → superpowers:brainstorming (adds/refines row in ROADMAP)

1. Phase in ROADMAP, directory does not exist
   → create docs/envoy-go/phases/NN-slug/
   → superpowers:brainstorming (scoped to THIS phase)
   → output: SPEC.md

2. SPEC.md exists, PLAN.md does not
   → superpowers:writing-plans
   → output: PLAN.md
   → GATE: if PLAN.md > ~25 tasks OR > ~1500 LoC estimated
           → split into NN.1, NN.2, …; update ROADMAP + STATE; stop

3. PLAN.md exists, implementation incomplete
   → superpowers:executing-plans (or subagent-driven-development for independent tasks)
   → TDD per superpowers:test-driven-development on every task
   → append to PROGRESS.md on each task completion

4. Implementation complete, not verified
   → superpowers:verification-before-completion
   → run: go build, go vet, golangci-lint, go test ./...,
          differential suite for phase's feature surface, conformance suites
   → quote all command outputs into PROGRESS.md

5. Verified, not reviewed
   → superpowers:requesting-code-review
   → output: REVIEW.md
   → if issues → back to step 3 (NOT 4) until REVIEW.md approved

6. Reviewed and approved
   → commit (message format: "phase NN: <title> [ADR-xxxx,...]")
   → ROADMAP.md status → done
   → STATE.md advanced to next phase or "awaiting next planning"
   → phase ends; session may exit

Deviations:
  * Ambiguity           → ADR + proceed
  * Blocked by upstream → ROADMAP status=blocked, STATE note, exit clean
  * Unexpected state    → superpowers:systematic-debugging FIRST
```

---

## 12. Acceptance Self-Checks

> **Note to the executing session:** This section is metadata for the prompt's authors and reviewers. It does not direct you to do anything. Skip it.

The bootstrap prompt itself is considered done when:

1. Loaded into a fresh Claude Code session with the `superpowers` plugin active, the prompt produces the §10 bootstrap without further human input beyond initial send.
2. A second fresh session loaded with the same prompt correctly resumes from disk state — it does not re-run bootstrap.
3. Every doctrine rule in §3 appears in the prompt with explicit enforcement verbs (`must`, `must not`, `never`).
4. The phase lifecycle state machine (§5) appears verbatim in both §5 and §11.
5. The six-part phase-done gate (§7.5) appears verbatim.
6. The MVP trunk (§8) is seeded as concrete ROADMAP rows matching the spec.
7. Feature families (§9) are seeded as headings only, including the `[scope TBD]` bracket on zookeeper.
8. The prompt is self-contained — it references only the `superpowers` skill set and the target repo.

---

*End of bootstrap prompt.*
