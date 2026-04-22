# envoy-go Bootstrap Prompt — Design Spec

**Date:** 2026-04-21
**Status:** Draft, pending spec-review and user approval.
**Author:** brainstorming session transcript, consolidated.
**Target artifact:** a single Markdown file (the "bootstrap prompt") that, when loaded as the first user turn in a fresh Claude Code session with superpowers available, initiates and operates an indefinite-duration, phase-based project to reimplement Envoy Proxy in Go with differential parity against upstream Envoy.

---

## 1. Purpose and non-purposes

### Purpose

Produce **one self-contained Markdown prompt** that:

1. Defines the mission (reimplement Envoy in Go, feature-complete, behavior-verified).
2. Encodes the non-negotiable doctrine for how the project is executed.
3. Mandates a persistent on-disk artifact layout that serves as the project's memory across sessions.
4. Installs a strict per-phase lifecycle state machine that maps directly onto superpowers skill invocations.
5. Specifies the differential-testing contract against upstream Envoy as the phase-completion gate.
6. Seeds the initial ROADMAP (MVP trunk phases 00–08 + feature-family headings 09+).
7. Gives every future fresh session an unambiguous "cold start" procedure so that context isolation between phases is total — no conversation history required.

### Non-purposes

- The prompt does **not** contain the Go implementation itself.
- The prompt does **not** enumerate every Envoy feature at task-level granularity.
- The prompt does **not** embed final API shapes, package layouts at module level, or file-level code decisions — those emerge from per-phase brainstorming.
- The prompt does **not** rely on any `/gsd-*` commands.
- The prompt does **not** assume a particular model version or session length.

---

## 2. Consumer and operating model

- **Consumer:** a fresh Claude Code session with the superpowers plugin active. Any such session, any time.
- **Operating model:** the prompt is loaded once per fresh session as the first user message. It directs the session to:
  1. Read the on-disk state (`docs/envoy-go/`).
  2. Determine the current lifecycle position.
  3. Invoke the single superpowers skill indicated by that position.
  4. Advance exactly one lifecycle step (which may be an entire phase's worth of work, or a sub-step inside one).
  5. Persist all progress to disk.
  6. Exit cleanly.
- **Re-entry model:** a subsequent fresh session loads the *same* prompt again, re-reads disk, and continues from wherever disk says the project is. No conversation memory is ever required.

---

## 3. Hard constraints encoded in the prompt

### 3.1 Process discipline (superpowers-first)

- `superpowers:brainstorming` runs before any design artifact is written.
- `superpowers:writing-plans` runs before any implementation task.
- `superpowers:test-driven-development` applies to every implementation task (tests before code, no exceptions).
- `superpowers:verification-before-completion` runs before any "done" claim.
- `superpowers:requesting-code-review` runs before any phase is committed as done.
- `superpowers:systematic-debugging` runs on any unexpected state (failing test, corrupted disk state, divergence from differential reference) before proposing fixes.
- No `/gsd-*` commands are used, suggested, or tolerated.

### 3.2 Implementation stance (hybrid)

- **Permitted foundations:**
  - Go stdlib.
  - `golang.org/x/net/http2` as a *low-level codec only* (never as a server runtime).
  - `github.com/quic-go/quic-go` for QUIC transport.
  - `golang.org/x/crypto`.
  - Protobuf runtime (`google.golang.org/protobuf`).
  - `github.com/envoyproxy/go-control-plane` — **proto types only**, no control-plane logic, no filter logic, no helpers that encapsulate Envoy semantics.
  - OpenTelemetry SDK for tracing/metrics where relevant.
  - `github.com/testcontainers/testcontainers-go` for the differential harness.
- **Must be written from scratch:**
  - Filter chain engine (network + HTTP filter iteration protocol).
  - Listener manager, cluster manager.
  - xDS state machine (ADS/delta, ACK/NACK, version/nonce tracking).
  - All load balancing algorithms.
  - Active health checking, outlier detection, circuit breakers.
  - Access log formatters and sinks.
  - Stats subsystem (counters, gauges, histograms, stat names).
  - Admin API.
  - Runtime layer (RTDS consumer).
  - Hot-restart / graceful-drain semantics.
  - Every individual filter (network and HTTP).
- **Forbidden:**
  - Wrapping `net/http/httputil.ReverseProxy`.
  - Embedding Traefik, Caddy, or fasthttp cores.
  - Copying GPL-licensed code.
  - Vendoring or cgo-binding Envoy C++.

### 3.3 Differential correctness beats internal fidelity

A phase ships when its feature surface produces behaviorally-equivalent output to upstream Envoy on the same config and inputs. "Behaviorally equivalent" is mechanically defined by `BEHAVIOR_CONTRACT.md` (see §6), not by inspection of Envoy source.

### 3.4 Context isolation is the primary design constraint

- Every instruction in the prompt assumes zero prior conversation memory in the executing session.
- No instruction ever says "as discussed earlier" or "remember to…".
- If a fact matters across sessions, it is written to a file under `docs/envoy-go/`.
- Ambiguities mid-phase are resolved by writing an ADR and proceeding, never by stalling for a human.

### 3.5 Every phase is a green build

- No phase lands with failing unit tests, failing differential fixtures, failing conformance checks, lint errors, or build errors.
- The mechanism for avoiding red phases is **phase splitting** (see §5.3). Splitting is cheap and expected.

### 3.6 Version pinning

- `docs/envoy-go/ENVOY_TARGET.md` pins the reference Envoy version (image tag + SHA) that differential tests run against.
- All fixtures, proto versions, and behavior contracts reference that pin.
- Upgrading the pin is itself a phase, with its own differential re-baselining.

---

## 4. On-disk artifact layout

The prompt mandates this exact tree. Phase 00 creates the skeleton.

```
envoy-go/
├── README.md                        # short: what this is, how to resume
├── go.mod / go.sum
├── cmd/envoy-go/                    # the binary
├── internal/
│   ├── listener/  cluster/  filter/
│   ├── http/      tcp/       tls/
│   ├── xds/       admin/     stats/
│   ├── accesslog/ runtime/   ...
├── pkg/                             # stable public API if/when any
├── test/
│   ├── differential/                # real-Envoy-vs-envoy-go harness
│   ├── conformance/                 # h2spec, h3spec, grpc-conformance drivers
│   ├── fixtures/                    # paired configs + inputs + expectations
│   └── helpers/
└── docs/envoy-go/
    ├── MISSION.md                   # mission + doctrine (copy of prompt's stable core)
    ├── ROADMAP.md                   # phase list, append-only history, status column
    ├── STATE.md                     # pointer to active phase + next skill to invoke
    ├── DECISIONS.md                 # ADR log, numbered, append-only
    ├── ENVOY_TARGET.md              # pinned Envoy version
    ├── BEHAVIOR_CONTRACT.md         # per-layer definition of "equivalent"
    ├── SKILL_ROUTING.md             # state-machine reference card
    └── phases/
        ├── 00-bootstrap/
        │   ├── SPEC.md
        │   ├── PLAN.md
        │   ├── PROGRESS.md
        │   └── REVIEW.md
        ├── 01-.../
        ├── ...
        └── 99-archive/              # completed phases can be moved here to keep docs/ small
```

### 4.1 Invariants

- `STATE.md` is the **single source of truth** for "what next." Fresh sessions read it first.
- `ROADMAP.md` schema: `id | title | depends-on | status | sub-phases | summary`, status ∈ `planned | in-progress | blocked | done`.
- A phase directory is created **only** when the phase enters `in-progress`. Creating the directory + SPEC.md is the first concrete act of starting a phase.
- `DECISIONS.md` entries are ADR-numbered (`ADR-0001`, `ADR-0002`, …), never edited after write; later ADRs supersede earlier ones by explicit reference.
- `BEHAVIOR_CONTRACT.md` is the canonical reference for differential equivalence rules. See §6.
- `SKILL_ROUTING.md` is a verbatim copy of the state machine in §5.1.

---

## 5. Phase lifecycle

### 5.1 State machine

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

### 5.2 Cold start procedure (every fresh session)

1. Read `docs/envoy-go/MISSION.md`, `STATE.md`, `ROADMAP.md`, `DECISIONS.md`, `BEHAVIOR_CONTRACT.md`, `SKILL_ROUTING.md`.
2. If active phase exists, read `SPEC.md`, `PLAN.md`, `PROGRESS.md` for that phase.
3. Match state to §5.1 state machine.
4. Invoke the indicated skill. Take no other action first.

### 5.3 Phase splitting policy

- Triggered at step 2 of the lifecycle when PLAN.md exceeds ~25 tasks or ~1500 estimated LoC.
- Also triggered mid-execution if a task's subtasks themselves exceed ~10 items.
- Procedure:
  1. Stop.
  2. Create sibling directories `NN.1-subtitle/`, `NN.2-subtitle/`, … under `phases/`.
  3. Move / redistribute SPEC content; each sub-phase gets its own SPEC.
  4. Update ROADMAP.md: original row marked as a parent with sub-phase rows.
  5. Update STATE.md to point at NN.1.
  6. Exit; new session starts at NN.1's lifecycle.

---

## 6. Differential testing contract

### 6.1 Harness

- Location: `test/differential/`.
- Mechanism: Go test binary orchestrating two proxies:
  - **Reference:** upstream Envoy, Docker image at the tag pinned in `ENVOY_TARGET.md`, managed via `testcontainers-go`.
  - **Subject:** envoy-go built from current tree, run as subprocess.
- Per test case (`test/fixtures/NNNN-name/`):
  - `envoy.yaml` — reference config.
  - `envoy-go.yaml` — equivalent config (initially identical; any divergence documented in an ADR).
  - `inputs/` — HTTP requests, raw TCP payloads, gRPC calls, or a small Go driver.
  - `expectations.yaml` — allow-lists, ignore-lists, stats-name mappings, timing tolerances derived from `BEHAVIOR_CONTRACT.md`.
- Per run: start both, drive identical inputs, capture responses + access logs + stats snapshots, diff under contract rules.

### 6.2 Equivalence matrix (summarized; authoritative in `BEHAVIOR_CONTRACT.md`)

| Dimension | Requirement |
|---|---|
| Response status | Exact |
| Response body | Byte-exact for deterministic handlers; semantically equal for filter-modified bodies |
| Response headers | Set-equal modulo documented allow-list (`server`, `date`, timing/identity headers) |
| Response trailers | Set-equal under same allow-list discipline |
| HTTP/2 & HTTP/3 framing | Structurally equivalent (frame-type/order on equivalent events); not byte-equal |
| Access log records | Semantically equal after field-mapping |
| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |
| xDS wire | ADS message sequences match protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |

### 6.3 Conformance suites (independent of real Envoy)

- `test/conformance/h2spec/` — runs once HTTP/2 lands; pass threshold is a phase gate.
- `test/conformance/h3spec/` — runs once HTTP/3 lands.
- `test/conformance/grpc/` — gRPC interop client.
- `test/conformance/proxy-wasm/` — proxy-wasm ABI conformance when WASM host lands.

### 6.4 Negative and fuzz testing

- Every phase that adds a parser/codec/filter ships a Go fuzz target under `test/`.
- Fuzzers run short-budget in CI, long-budget nightly.
- Malformed/adversarial inputs must produce the *same class* of response as upstream Envoy (status + Envoy-style `x-envoy-local-reply` behavior), not identical error text.

### 6.5 Phase-done gate (verbatim wording baked into the prompt)

> A phase is not done until:
> (a) all new/changed differential fixtures are green,
> (b) all pre-existing differential fixtures are still green,
> (c) the phase's conformance suites pass at the declared threshold,
> (d) any new fuzzer has run clean for its short-budget CI run,
> (e) `go vet`, `golangci-lint run`, `go test ./...` are all clean,
> (f) REVIEW.md is approved.

---

## 7. Initial ROADMAP seeded by the prompt

### 7.1 MVP trunk (fully seeded as rows)

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

### 7.2 Feature families (seeded as headings only; phases added lazily)

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

## 8. Bootstrap instruction (first-session action list encoded in the prompt)

1. Confirm repo is empty/fresh. If not, run `superpowers:systematic-debugging` on what's there before touching anything.
2. Create `docs/envoy-go/` skeleton:
   - `MISSION.md` — copy of the prompt's mission + doctrine sections verbatim.
   - `ROADMAP.md` — scaffold with phases 00–08 as rows and families 09+ as headings.
   - `STATE.md` — points at phase 00, next skill = `superpowers:brainstorming`.
   - `DECISIONS.md` — with `ADR-0001: bootstrap prompt version X committed`.
   - `ENVOY_TARGET.md` — empty placeholder, to be filled in phase 00.
   - `BEHAVIOR_CONTRACT.md` — skeleton populated with §6.2 matrix.
   - `SKILL_ROUTING.md` — verbatim copy of §5.1 state machine.
3. Commit `docs/envoy-go/` scaffolding as a single commit (`bootstrap: envoy-go project scaffold`).
4. Enter phase 00 lifecycle at step 1: invoke `superpowers:brainstorming` scoped to phase 00.

From step 4 onward, the prompt is no longer the driver. The on-disk artifacts and the state machine take over. Any subsequent fresh session re-enters via §5.2 (cold start).

---

## 9. Acceptance criteria for the prompt itself

The bootstrap prompt is considered done when:

1. Loaded into a fresh Claude Code session with superpowers, it produces the §8 bootstrap without further human input beyond initial send.
2. A second fresh session loaded with the same prompt correctly resumes from disk state (i.e., does not re-run bootstrap).
3. Every doctrine rule in §3 is stated in the prompt with explicit enforcement language.
4. The state machine in §5.1 is present verbatim in the prompt.
5. The differential contract in §6 is present verbatim.
6. The MVP trunk in §7.1 is seeded as concrete ROADMAP rows.
7. The prompt is self-contained — references nothing outside the superpowers skill set and the target repo.

---

## 10. Open items deferred to phase 00 (NOT to be resolved in the prompt)

- Exact pinned Envoy version and image SHA.
- Exact Go toolchain version.
- CI provider and exact lint configuration.
- Whether envoy-go ships a single binary or a modular layout.
- License file contents.
- Stat-name mapping table details.

These are intentionally left open so the prompt does not over-prescribe.

---

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Prompt is too long for a fresh session to load usefully | §3–§6 are the core; §7 seeds data, §8 is action. Aim ~800 lines; front-load the cold-start procedure. |
| Fresh sessions diverge from doctrine under pressure | Every rule phrased as a hard constraint with explicit enforcement verbs. State machine leaves no room for improvisation. |
| Differential harness becomes flaky (timing, container startup) | `expectations.yaml` is the opt-in layer for tolerances; flakiness forces either allow-list additions with ADR rationale, or phase rework. |
| Envoy upstream moves faster than phases land | Version pinning. Upgrading the pin is its own phase with its own re-baselining. |
| Phases get too large despite split rule | Split rule is mechanical (~25 tasks / ~1500 LoC). Enforced at step 2 of the lifecycle. Reviewers flag overruns. |
| ADR log grows unbounded | Acceptable. Search-over-grep is fine; supersession references keep currency clear. |
| WASM host is too big to fit the model | Explicitly called out as a multi-phase sub-project in §7.2. |

---
