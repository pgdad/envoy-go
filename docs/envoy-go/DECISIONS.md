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

---

## ADR-0005: autonomous-planning adaptation for envoy-go phases

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.1, D-3.5

### Context

`superpowers:writing-plans` ends with an "Execution Handoff" prompt asking the user to choose between subagent-driven and inline execution. `BOOTSTRAP_PROMPT.md` §2.2 forbids asking humans mid-phase, and §5.1 requires that a session move exactly one state forward — execution always happens in a *fresh* session, by definition.

### Decision

For every phase in the envoy-go project, `superpowers:writing-plans` is invoked with the following adaptations:

1. **No Execution Handoff question.** The plan-writing session writes `PLAN.md`, runs the plan-document-reviewer subagent loop (retained verbatim from the skill), updates `STATE.md` to lifecycle-state 3 with `next-skill = superpowers:subagent-driven-development`, commits, and exits. The next fresh session, per the state machine §5 step 3, picks the executor.
2. **Plan location override.** `PLAN.md` is written to `docs/envoy-go/phases/NN-slug/PLAN.md` (the project layout per `BOOTSTRAP_PROMPT.md` §4), not the skill's default `docs/superpowers/plans/`. The skill explicitly permits this via its "user preferences override default location" clause.
3. **Reviewer subagent escalation.** If the reviewer cannot approve after three iterations, the session sets `STATE.md` `lifecycle-state` to `blocked` with a `block-reason` and exits — same escalation policy as ADR-0004's spec-reviewer.
4. **Default executor preference.** `next-skill` after a green plan is `superpowers:subagent-driven-development` (the user's standing preference for execution style); the executing session may override only with an ADR documenting why.

### Consequences

- Phase planning is deterministic in one session; no human interaction.
- The reviewer subagent gate preserves plan quality.
- Execution stance is set by ADR, not session-by-session improvisation.
- This ADR applies uniformly to phase 00 and every subsequent phase.

---

## ADR-0006: module path `github.com/esalaine/envoy-go`

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.5
**Settles:** SPEC §10 #1 deferred decision

### Context

`docs/envoy-go/phases/00-bootstrap/SPEC.md` §10 #1 proposed `github.com/envoyproxy/envoy-go` as the Go module path with the planner permitted to pick differently if the proposed path is unusable or the project owner prefers a different origin. `github.com/envoyproxy` is the upstream Envoy project's GitHub organization; squatting that path even in a `go.mod` declaration risks future name collision and is contrary to the spirit of D-3.2 (do not embed/wrap upstream).

### Decision

The Go module path is `github.com/esalaine/envoy-go`, namespaced under the project's git identity (`Esa Laine <pgdad1st@gmail.com>`).

### Consequences

- All package imports use `github.com/esalaine/envoy-go/...`.
- The path is a `go.mod` identifier only — it does not need to resolve as a Git URL during phase 00 or any phase that does not publish modules. Publication, if ever pursued, is its own ADR.
- Supersession (e.g. moving to a real published origin) is cheap: one ADR + `go mod edit -module …` + sed-rewrite of import paths.

---

## ADR-0008: pinned upstream Envoy image v1.37.2

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.3, D-3.7
**Settles:** SPEC §10 #2 deferred decision

### Context

The differential test contract (BOOTSTRAP_PROMPT §7) requires every fixture to compare against a stable, byte-identifiable upstream Envoy image. Phase 00 is the first phase to need that pin.

### Decision

The upstream Envoy reference is pinned to `envoyproxy/envoy:v1.37.2` at SHA256 `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Selection rationale per SPEC §5.6:

- Stable release tag (not `-dev`).
- Most recent stable as of 2026-04-21 within the 6-month window.
- Exposes admin and `tcp_proxy` on the documented names for v3 proto.
- Smoke-tested locally: admin `/ready` returns `LIVE` under a minimal bootstrap.

### Consequences

- All fixture configs (`envoy.yaml`) target this Envoy version's bootstrap and v3 protobuf.
- `docs/envoy-go/ENVOY_TARGET.md` documents the refresh procedure (re-pull, re-baseline differential, ADR).
- Pin changes happen only in a dedicated phase per D-3.7.

---

## ADR-0007: minimal YAML schema for `envoy-go.yaml` in phase 00

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.5
**Settles:** SPEC §10 #6 deferred decision

### Context

Phase 00's subject binary needs to read its own configuration without yet implementing Envoy's bootstrap proto (that lands in phase 01). SPEC §5.2 sketched the field set; ADR-0007 codifies it for the phase-00 lifetime.

### Decision

The minimal phase-00 schema, parsed by `cmd/envoy-go/config.go`, is exactly:

```yaml
listener:
  address: <string, required, non-empty>
  port: <int, required, 1..65535>
upstream:
  address: <string, required, non-empty>
  port: <int, required, 1..65535>
```

Unknown top-level fields are rejected (`yaml.Decoder.KnownFields(true)`). No defaults; both blocks must be present.

### Consequences

- Phase 01's bootstrap loader (`internal/bootstrap`) supersedes this schema entirely — phase 01's plan ADRs the cutover and the migration of `test/fixtures/0000-tcp-echo/envoy-go.yaml`.
- The strict-unknown-fields rule prevents silent typo regressions.
- The schema is intentionally not extensible. New fields require either (a) phase 01 landing, or (b) an explicit superseding ADR.

---

## ADR-0009: golangci-lint version pin bumped from v1.55.2 to v1.64.8

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5, D-3.6
**Supersedes:** PLAN.md Task 5 precondition (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2`) and PLAN.md Task 15 CI install command.

### Context

PLAN's Task 5 precondition specifies `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2`. This install command fails on any Go toolchain ≥ 1.22 due to golangci-lint v1.55.2's transitive dependency `golang.org/x/tools@v0.14.0`, whose `internal/tokeninternal/tokeninternal.go:78` contains a const-array-length idiom (`[...]int{-delta * delta - 1}`) that Go 1.22+ rejects under stricter constant-arithmetic rules. The PLAN's own Go floor (SPEC §10 #3) is 1.23, so the precondition is internally inconsistent: every compliant environment fails the install.

### Decision

The project pins `golangci-lint` at v1.64.8 (the last v1.x stable release, installed via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8`).

- v1.64.8 installs cleanly under Go 1.23 and Go 1.26.x developer toolchains.
- v1.64.8 supports every SPEC §5.5 baseline linter (govet, errcheck, staticcheck, unused, ineffassign, gofmt, goimports, misspell, revive) with the same configuration schema as v1.55.2, so `.golangci.yml` (committed by Task 5) is version-agnostic across the v1.x series.
- v2.x was considered but rejected: v2 changes the `.golangci.yml` format, adds migration cost, and provides no phase-00 feature needed.

### Consequences

- CI's golangci-lint install command (Task 15's `.github/workflows/ci.yml`) uses v1.64.8.
- The `.golangci.yml` at repo root remains unchanged.
- Future version bumps (e.g., if v1.64.8 becomes incompatible with a later Go release) land as a new ADR superseding this one, following the same pattern as the Envoy pin refresh procedure (ADR-0008 refresh-procedure clause).
- PLAN's conditional ADR-0009 slot (Task 16 runner fallback, per PLAN "ADRs introduced by this plan") shifts to ADR-0012 if that contingency materializes.

---

## ADR-0010: `dns_lookup_family: V4_ONLY` for fixtures using `host.docker.internal`

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.3, D-3.5

### Context

Fixture `0000-tcp-echo` (Task 13) needs the reference Envoy container to reach a test backend running on the host. The test binds the backend with `net.Listen("tcp", "0.0.0.0:<random port>")` and uses `host.docker.internal` inside the container's bootstrap (cluster's `socket_address.address`). Docker Desktop on Linux resolves `host.docker.internal` to both IPv4 (e.g. `192.168.65.2`) and IPv6 (e.g. `fdc4:f303:9324::254`) addresses via the container's DNS. Envoy's `STRICT_DNS` cluster, without further configuration, picks one — and observationally picks the IPv6 record. The host side, however, does not route IPv6 from container-bound traffic (Docker Desktop Linux does not bridge IPv6 by default), so the connection fails with "Network is unreachable" and the differential test hangs until the 90s context deadline elapses.

### Decision

All fixtures whose reference bootstrap uses `host.docker.internal` in a `STRICT_DNS` cluster set `dns_lookup_family: V4_ONLY` on that cluster. This is codified by:

1. The reference bootstrap for fixture `0000-tcp-echo` (in `test/fixtures/0000-tcp-echo/driver/driver.go`'s `refBootstrap` constant, and in the documentation mirror `test/fixtures/0000-tcp-echo/envoy.yaml`).
2. A note in `docs/envoy-go/BEHAVIOR_CONTRACT.md` (see Consequences) making this the standard for future TCP/HTTP/1.1/HTTP/2 fixtures using `host.docker.internal`.

### Consequences

- Future fixtures follow the same pattern. When a phase introduces HTTP/3 / QUIC fixtures (per SPEC §9), the V4_ONLY rule must be re-evaluated: QUIC is UDP, and the dual-stack concern may differ. The re-evaluation is an ADR superseding this one.
- The test-backend bind address is `0.0.0.0` (not `127.0.0.1`) because the Docker-provided host gateway on Docker Desktop maps to a non-loopback host IP; loopback-only binding would be unreachable from the container.
- CI runners (GitHub Actions `ubuntu-latest`) typically expose `host.docker.internal` with extra_hosts via `--add-host=host.docker.internal:host-gateway`; Task 13 codifies this by setting `HostConfigModifier` in `StartReferenceProxy`.

---

## ADR-0011: pin `docker/docker` at `v24.0.7+incompatible`

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

### Context

Task 13 requires configuring `testcontainers-go` with `HostConfigModifier` so that `host.docker.internal:host-gateway` is added to the container's extra_hosts. This uses the type `github.com/docker/docker/api/types/container.HostConfig`. When `go get github.com/docker/docker/api/types/container` was first run (Task 13 Step 8), Go resolved `docker/docker` to `v28.5.2`, which changed several APIs (notably `types.ExecConfig` moved, `archive.Compression` renamed) that `testcontainers-go@v0.27.0` depends on in its vendored form. The result was compile errors in `testcontainers-go`'s internal packages that the user could not fix without upgrading testcontainers-go — which is out of Phase 00 scope (v1.x requires SPEC re-validation per ADR-0003 and PLAN's Task 10 note).

### Decision

`github.com/docker/docker` is pinned at `v24.0.7+incompatible` — the version that `testcontainers-go@v0.27.0` was developed against and that provides a compatible API surface. The `+incompatible` suffix indicates the module is Go-module-unaware at v24 (docker/docker introduced `/v25` later), but this is the version that ships with tcg@v0.27.0 lock-step.

The pin is codified in `go.mod` as `require github.com/docker/docker v24.0.7+incompatible` (direct require, promoted from indirect after `HostConfigModifier` introduction added a direct import).

### Consequences

- `go mod tidy` is idempotent on the committed state — provided the toolchain is invoked with `GOTOOLCHAIN=local` so Go 1.26.2 does not rewrite the `go` directive to `1.26`. This is why `go.mod` says `go 1.23.0` (patch suffix is semantically equivalent to `1.23` per Go's toolchain rules).
- Upgrading testcontainers-go beyond v0.27.0 (e.g., to v0.30+ or v1.x) is the preferred long-term resolution: newer testcontainers-go versions track `docker/docker` v26+. The upgrade is a future phase's decision, not phase 00's.
- Security updates to `docker/docker` (CVE backports beyond v24.0.7) will not land automatically. A future phase that tracks container-security upgrades should re-open this ADR if a CVE forces the jump.
- The pin is test-only: no production (runtime) code imports `docker/docker`. The blast radius is limited to the differential test harness.

---

## ADR-0013: `github.com/envoyproxy/go-control-plane/envoy` version pin (proto types only)

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.2, D-3.5, D-3.7

### Context

Phase 01 (static-bootstrap-config, SPEC §3) requires the subject binary to consume the same YAML shape upstream Envoy accepts. Per ADR-0012, the loader is `yaml.v3 → json.Marshal → protojson.Unmarshal` into `envoy.config.bootstrap.v3.Bootstrap`. The `Bootstrap` message (and its transitive proto types — `Admin`, `Listener`, `Cluster`, `SocketAddress`, etc.) is generated Go code that ships in the `github.com/envoyproxy/go-control-plane` project. Doctrine D-3.2 permits this dependency **as proto types only**: no xDS / control-plane helpers, no filter helpers, no `pkg/server`, no `pkg/cache`. Phase 01 is the first phase to take a direct dependency on it, so the version pin needs to land now.

The upstream project has been split into nested Go modules:

- `github.com/envoyproxy/go-control-plane` — the parent module; at `v0.13.x` it contains control-plane helpers (under `pkg/…`) but **no longer** vendors the envoy protos.
- `github.com/envoyproxy/go-control-plane/envoy` — a nested module (independent semver, current line `v1.32.x`) that owns the generated proto packages under `envoy/…`, including `envoy/config/bootstrap/v3`.

The PLAN.md hint ("representative: `v0.13.x`") pre-dated the observation of the module split. `go get github.com/envoyproxy/go-control-plane@v0.13.4` followed by `go mod tidy` resolves the actual import path `github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3` via the nested module and records the nested module (not the parent) in the direct-require block. The parent module's helper packages are never imported by envoy-go (doctrine D-3.2), so only the nested module needs to be pinned.

### Decision

Pin **`github.com/envoyproxy/go-control-plane/envoy` at `v1.32.4`** (release date 2024-12-19, commit `71abaaad06c63d4ef7bf6fca87b1d75183b32e27` in the upstream repo's `envoy/` subtree). This is the current tip of the `v1.32.x` line and is the version that `go get github.com/envoyproxy/go-control-plane/envoy@v1.32.4` resolves to as of 2026-04-22; it exposes `envoy.config.bootstrap.v3.Bootstrap` with the field shape expected by upstream Envoy `v1.37.2` (the ADR-0008 reference image) for the `static_resources.{listeners,clusters}` + `admin` subset that phase 01 exercises. No `replace` directive is used.

Phase 01 imports **only** generated proto types from this module, initially `github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3` (Task 2), with transitive proto types reached via field types (`envoy/config/core/v3`, `envoy/config/listener/v3`, `envoy/config/cluster/v3`, `envoy/config/endpoint/v3`, …). The parent module `github.com/envoyproxy/go-control-plane` is **not** pinned as a direct require: its control-plane / xDS helper packages are excluded by doctrine D-3.2, so pinning it would imply a dependency envoy-go does not take.

`google.golang.org/protobuf` is promoted from indirect to direct concurrently (Task 2 adds a direct import of `google.golang.org/protobuf/encoding/protojson`). Its version is whatever `go get google.golang.org/protobuf` resolves to at Task 1 time (observed: `v1.36.11`); this is not a separate ADR because `protojson` is the canonical proto JSON codec and the version choice is not behavior-salient at phase-01 granularity.

### Consequences

- Only `github.com/envoyproxy/go-control-plane/envoy` appears in the direct-require block — not the parent `github.com/envoyproxy/go-control-plane`. This is the D-3.2 boundary made legible in `go.mod`: any future PR that adds an import of `github.com/envoyproxy/go-control-plane/pkg/...` (control-plane helpers) becomes visible as a new direct require and must be justified by a superseding ADR.
- Refreshing the pin (to `v1.32.5`, `v1.35.x`, `v1.36.x`, `v1.37.x`, etc.) is its own future phase per doctrine D-3.7 (version pinning = deliberate act). The refresh procedure mirrors ADR-0008 (Envoy image pin): (a) a new ADR explicitly names this ADR-0013 as superseded; (b) the refresh is accompanied by a re-run of every phase's differential gate still live on the roadmap; (c) any proto field shape delta that invalidates fixtures is either absorbed (fixture update) or triggers a BEHAVIOR_CONTRACT §7.2 matrix update.
- `go mod tidy` is idempotent on the committed state **provided** a consumer of the proto package exists in the module (Task 2 satisfies this by importing `bootstrapv3` in `internal/bootstrap/bootstrap.go`). Between Task 1's commit (deps added) and Task 2's commit (first real import), `go mod tidy` would demote the direct require to indirect — this is a transient state internal to the phase-01 commit sequence and is resolved by Task 2's import. The commit-at-rest Task 1 state compiles (`go build ./...` passes because there is no consumer yet) and vets (`go vet ./...` passes for the same reason); tidy-idempotency returns at Task 2 commit.
- The transitive closure that arrives with `v1.32.4` includes `github.com/cncf/xds/go`, `github.com/envoyproxy/protoc-gen-validate`, `github.com/planetscale/vtprotobuf`, `cel.dev/expr`, `google.golang.org/genproto/googleapis/api` + `…/rpc`, and `google.golang.org/grpc`. These are recorded as `// indirect` in `go.mod`; none are imported by envoy-go code, and the D-3.2 boundary holds as long as that remains true.
- The `go` directive in `go.mod` is unchanged (`go 1.23.0`). The nested module's own `go.mod` declares `go 1.22`, which is below the envoy-go module's floor, so no toolchain bump is forced.

---

## ADR-0012: YAML-to-proto pipeline for bootstrap loader

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.2, D-3.5

### Context

Phase 01 (static-bootstrap-config, SPEC §5.1) requires `internal/bootstrap.Load` to accept the same YAML shape upstream Envoy accepts and materialize it as an `envoy.config.bootstrap.v3.Bootstrap` proto. Upstream Envoy itself uses a YAML→proto pipeline internally (`Envoy::MessageUtil::loadFromYaml`), but there is no canonical Go implementation of that pipeline: the generated Go proto types ship with `protojson` (the official proto JSON codec) but not a proto YAML codec. Several paths were considered:

1. **`gopkg.in/yaml.v3` → `encoding/json.Marshal` → `google.golang.org/protobuf/encoding/protojson.Unmarshal`** — three-stage decode through JSON as an intermediate representation. `yaml.v3` is already a direct dependency (phase-00 `envoy-go.yaml` loader). YAML flow-style (the subset upstream Envoy uses heavily in its examples) is JSON-compatible; `yaml.v3`'s default decode of a flow map yields `map[string]interface{}`, which `encoding/json.Marshal` emits as JSON, which `protojson.Unmarshal` accepts.
2. **`sigs.k8s.io/yaml`** — a Kubernetes wrapper that does essentially the same thing but through a different YAML library (`yaml.v2`) and with its own opinions. Adds a transitive dependency for no new capability.
3. **Direct YAML-to-proto via proto reflection** — walk the `yaml.v3` node tree and populate proto fields through `protoreflect`. Not canonical, significantly more code, and reinvents what `protojson` already provides.

### Decision

The `internal/bootstrap.Load` loader uses a three-stage pipeline:

```
io.Reader → gopkg.in/yaml.v3 (Unmarshal into map[string]interface{})
          → encoding/json.Marshal
          → google.golang.org/protobuf/encoding/protojson.Unmarshal into *bootstrapv3.Bootstrap
```

Rationale:

- `protojson` is the canonical Go codec for proto JSON — it understands `@type` Any-URL wrapping, proto field-name → JSON-name mapping (snake_case → camelCase), and well-known types (`Duration`, `Timestamp`, `Struct`, `BoolValue`, …). Writing any part of this by hand would duplicate `protojson`'s behavior and diverge under spec ambiguity.
- YAML flow-style is a superset of JSON syntax; the vast majority of bootstrap configs upstream Envoy users write (and the phase-01 fixture) stay within the flow-style-compatible subset. Non-flow YAML features (anchors, aliases, tags) that do not have JSON equivalents are out of scope for phase 01 and will be ADRd if a fixture ever needs them.
- `yaml.v3` is already a direct require (phase-00 `cmd/envoy-go/config.go`), so no new module is added by this pipeline.
- Single-caller pipeline — only `internal/bootstrap` uses it, so introducing a wrapper library (`sigs.k8s.io/yaml`) is unjustified overhead.

Alternatives considered and rejected:

- `sigs.k8s.io/yaml`: extra module, no new capability, wraps `yaml.v2` which is less maintained than `yaml.v3`.
- Direct YAML-to-proto via `protoreflect`: not canonical; duplicates `protojson`'s Any/well-known-type handling; higher maintenance cost.

Supersession path: if a future phase needs YAML-native features the three-stage pipeline cannot express (e.g., `!!binary` tags for inline certificate bytes, YAML-native duration tags, or anchor-based DRY in large bootstraps), this ADR is superseded by a new one that either (a) swaps `yaml.v3 → JSON` for a direct YAML-to-proto library when one becomes canonical, or (b) documents a YAML preprocessor that rewrites the unsupported tags into JSON-compatible form before the pipeline runs.

### Consequences

- `google.golang.org/protobuf` becomes a direct require in `go.mod` (Task 2 phase 01): the first real consumer of `protojson` lands with the loader.
- `gopkg.in/yaml.v3` remains a direct require and is shared with the (phase-00) `cmd/envoy-go/config.go` phase-00 schema. When ADR-0021 deletes that schema at phase-01 cutover, `yaml.v3` remains a direct require via `internal/bootstrap`.
- Error surfaces are layered: YAML syntax errors surface as `bootstrap: yaml parse: …`, JSON marshal errors as `bootstrap: to json: …`, proto unmarshal errors as `bootstrap: protojson: …`. Every error begins with `bootstrap: ` so callers can distinguish loader errors from other packages.
- Because YAML flow-style is JSON-compatible, the test fixture `test/fixtures/0000-tcp-echo/envoy.yaml` and the reference Envoy container parse the same bytes with divergent parsers (upstream Envoy's C++ YAML library vs. envoy-go's `yaml.v3`) and must produce equivalent `Bootstrap` semantics. The differential gate (BEHAVIOR_CONTRACT §7.2) catches any divergence.

---

## ADR-0016: unknown-field rejection in bootstrap loader; Any preservation exception

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

### Context

The YAML-to-proto pipeline (ADR-0012) ends in `protojson.Unmarshal` into an `envoy.config.bootstrap.v3.Bootstrap`. `protojson.UnmarshalOptions` exposes a `DiscardUnknown` flag: when `true`, JSON fields that do not map to any proto field are silently dropped; when `false`, the parse errors on any unknown field. The choice is behaviorally significant for phase-01 fixture authoring.

A second concern is that `Bootstrap.static_resources.listeners[].filter_chains[].filters[].typed_config` is of proto type `google.protobuf.Any` — an opaque wrapper whose contents are identified by a `@type` URL. For `protojson` to decode an Any, the concrete message type named by the URL must be resolvable via the `UnmarshalOptions.Resolver` (default: `protoregistry.GlobalTypes`). Phase 01 does not interpret the Any payload (SPEC §2 — filter wiring lands in later phases), but `protojson` still needs the descriptor to round-trip the bytes without erroring.

### Decision

The loader uses `protojson.UnmarshalOptions{DiscardUnknown: false}` — any field that does not correspond to a proto schema field causes the load to error. Exception: `typed_config` fields of proto type `google.protobuf.Any` carry implementation-specific bytes (e.g., `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) that phase-01 envoy-go code does not inspect or act on. Their contents are preserved inside the `Bootstrap` proto (via `protojson`'s Any round-tripping) but are not semantically resolved by `internal/bootstrap`: the loader neither extracts fields from them nor validates them against a phase-01 whitelist.

To make `protojson` capable of round-tripping the Any values it encounters in phase-01 fixtures, the loader blank-imports the generated proto packages for each filter extension type that appears in `typed_config` within those fixtures. Phase 01 imports only `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3`, registering `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy` with `protoregistry.GlobalTypes` at process init. Later phases append blank imports as new filter types enter their fixtures; each such addition is an amendment documented in the owning phase's PROGRESS, not a new ADR, because the blank-import list is a registry-population mechanism and does not change the semantic contract of this ADR.

Rationale:

- **`DiscardUnknown: false` catches typos fast.** If a fixture author writes `stat_prefx` instead of `stat_prefix`, or `connection_timeout` instead of `connect_timeout`, the loader errors with a clear `unknown field` message instead of silently dropping the value and surfacing a subtle misbehavior at runtime. For a project whose correctness bar is byte-identical differential behavior (BEHAVIOR_CONTRACT §7.2), silent field drop is incompatible with the quality bar.
- **Any preservation is required for the filter chain parse pass.** Even though phase 01 does not interpret the Any contents, the parse must succeed on valid bootstraps so that downstream phases (Task 4+ extractors; filter-factory phases) can walk the proto tree without re-parsing. Failing the parse on unresolved Any URLs would force every phase to disable Any decoding or to implement a bespoke traversal, both of which are worse than the blank-import registration path.
- **Phase 01 does not resolve Any semantics.** The blank-import registers descriptors; envoy-go code does not call `anyMessage.UnmarshalTo(&TcpProxy{})` or otherwise introspect filter configs during phase 01. Later phases that implement filter wiring will introduce their own resolution steps under their own ADRs.

### Consequences

- Fixture authors writing `envoy-go.yaml` (phase 01 and beyond) get immediate feedback on typos at load time, not silent drops.
- The loader's public surface (`Load(io.Reader) (*bootstrapv3.Bootstrap, error)`) is independent of which filter types are registered: callers interact only with the `Bootstrap` proto. The blank-import list inside `internal/bootstrap` is an implementation detail of "which bootstraps parse successfully", not part of the loader's API contract.
- Each subsequent phase that introduces a new filter type in its fixtures extends the blank-import list in `internal/bootstrap/bootstrap.go` with an accompanying PROGRESS entry naming the filter. The phase's differential gate validates that the new filter's `typed_config` round-trips.
- Supersession path: if envoy-go ever needs to accept arbitrary unregistered filter Any types (e.g., for a generic config linter that wraps third-party filters), this ADR is superseded by a new one that either (a) substitutes a `dynamicpb`-backed custom `protojson` resolver for unknown URLs, or (b) swaps to `DiscardUnknown: true` on the inner Any parse specifically. Both options have concrete trade-offs that phase 01 does not need to weigh.
- Runtime cost of the blank imports is the `init()` registration of each proto file descriptor with `protoregistry.GlobalTypes`: O(number of imported filter packages × their descriptor size), paid once at process start. This is indistinguishable from the cost any Envoy-integrating Go binary pays.

---

## ADR-0017: `node` field semantics in phase 01 bootstrap loader

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

### Context

The `envoy.config.bootstrap.v3.Bootstrap` proto has a `node` field (of type `envoy.config.core.v3.Node`) with subfields `id`, `cluster`, `metadata`, `locality`, `user_agent_name`, and others. Upstream Envoy consumes `node.id` / `node.cluster` primarily in xDS / ADS control-plane interactions (to identify the data-plane instance to its control plane) and in admin-endpoint metadata (to label the served process). The phase-01 test fixture `sampleBootstrap` includes `node: { id: test-node, cluster: test-cluster }` to exercise the loader on a realistic shape, and `TestLoad_HappyPath` asserts `bs.GetNode().GetId() == "test-node"` — i.e., phase 01 confirms the field round-trips through the pipeline. It does not, however, wire `node` to any consumer: there is no xDS client in phase 01 (ADR-0012 consequences; SPEC §2 bans `dynamic_resources`), no admin endpoint that surfaces node metadata (admin lands as a feature in phase 08+ per ROADMAP), and no runtime behavior keyed on `node.id` or `node.cluster`.

The question is whether the phase-01 loader should enforce anything about `node` — e.g., require it to be present, require `node.id` non-empty, require both `id` and `cluster` together, or reject unfamiliar subfields like `user_agent_name`. Two positions were considered:

1. **Enforce now.** Future phases will consume `node`; a bootstrap missing `node.id` will eventually fail somewhere deeper in the stack, and catching it at load time gives a clearer error. This aligns with the ADR-0016 "catch typos fast" philosophy.
2. **Defer.** No phase-01 consumer of `node` exists, so any enforcement is speculative — the specific shape the first consumer needs is unknown (admin may want `node.id` non-empty; xDS may want both `id` and `cluster`; neither is in phase 01's scope). Enforcing now couples the loader to admin/xDS semantics that will be refined when those features land, risking an ADR-superseding churn.

### Decision

The phase-01 loader parses `node` into the `Bootstrap` proto as-is (via the standard `yaml → json → protojson` pipeline of ADR-0012) and does not enforce presence or content of `node.id`, `node.cluster`, or any other `node` subfield. Unknown subfields inside `node` are rejected by the `DiscardUnknown: false` rule from ADR-0016 (consistent with every other message in the tree); known subfields round-trip verbatim without semantic validation. The field is available for future phases via `bs.GetNode()` on the loaded proto, but `internal/bootstrap` exposes no `node`-specific extractor at phase 01.

Rationale:

- **YAGNI (D-3.5).** No phase-01 consumer of `node` exists: admin lands in phase 08+, xDS in a later phase, and neither's exact requirements on `node` are settled. Enforcing fields now would couple the loader to guesses about those consumers.
- **Consistency with ADR-0016.** Unknown-field rejection already applies inside `node` — `node.not_a_real_field: 42` errors at load time via `DiscardUnknown: false`. This gives fixture authors the same typo-catching bar for `node` subfields as for the rest of the bootstrap.
- **No feature regression.** The `sampleBootstrap` fixture carries `node.id` and `node.cluster` values, so the happy-path test pins that the field is parsed and accessible via proto getters. Future phases that consume `node` build on that foundation without a loader change.

Alternatives considered and rejected:

- **Require `node.id` non-empty** — would pre-empt admin's (phase 08+) decision about whether `node.id` is mandatory for admin-metadata display. If admin ends up synthesizing a default (e.g., hostname) when `node.id` is empty, the phase-01 requirement becomes a speculative constraint the project then has to supersede.
- **Require both `node.id` and `node.cluster`** — same problem, one layer up: xDS semantics are not phase-01's call.
- **Make `node` mandatory** — upstream Envoy treats `node` as optional at the bootstrap level (it synthesizes defaults in some paths); enforcing presence would diverge from upstream for no phase-01 benefit.

### Consequences

- `internal/bootstrap.Load` accepts bootstraps that omit `node` entirely, and bootstraps whose `node` has only some subfields populated. The `TestAdminSocket_MissingAdmin` test (Task 4) already exercises the "omit top-level field" path and passes without modification.
- When a future phase (admin, xDS, or similar) introduces a real consumer of `node`, that phase owns the field-validation ADR: e.g., "ADR-00XX: admin requires `node.id` non-empty; loader enforces at Load time" or "ADR-00YY: xDS extractor reads `node.{id,cluster}`, errors on empty". The new ADR either supersedes this one or layers on top — the loader's "parse-only" behavior from this ADR remains the baseline unless explicitly replaced.
- `internal/bootstrap` adds no `NodeID(bs)` / `NodeCluster(bs)` extractors at phase 01. Callers that need those values today call `bs.GetNode().GetId()` directly; a typed extractor would be a premature abstraction without a consumer to shape it.
- The phase-01 `sampleBootstrap` fixture continues to populate `node.{id,cluster}` — documenting by example that the fields are recognized — but no test asserts they are *required*. If a future refactor removes `node` from the fixture, the happy-path test's `GetId()` assertion updates accordingly; nothing else breaks.

---

## ADR-0018: `FuzzBootstrapLoad` CI budget for gate (d)

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

**Decision:** 30 seconds per CI run via `-fuzztime=30s`.

**Rationale:** short enough to not dominate the 5-minute differential job wall-clock; long enough to exercise the seed corpus and a few thousand mutations. A longer nightly lane is out of scope (no scheduled phase introduces it).

---

## ADR-0015: pre-init and ready-state admin `/ready` response contract

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

**Decision:** Phase-01 `internal/admin` reproduces upstream Envoy v1.37.2's `/ready` responses byte-exact where captured empirically, and emits a documented-but-test-irrelevant pre-init response where upstream's pre-init window was unobservable.

Concretely, the admin server MUST emit:

- **Ready state (after `MarkReady` is called):** the exact bytes captured in `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` §"Ready-state response (raw)" — status line `HTTP/1.1 200 OK`, lowercase headers `content-type: text/plain; charset=UTF-8`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `date: <IMF-fixdate>`, `server: envoy`, `transfer-encoding: chunked` (see framing exception below), and body `LIVE\n` (5 bytes: `0x4c 0x49 0x56 0x45 0x0a`, trailing LF, no CRLF). Header names lowercase, charset token exactly `UTF-8`. `date` value is non-deterministic and is on the differential harness allow-list.
- **Pre-init state (before `MarkReady` is called):** `HTTP/1.1 503 Service Unavailable` with body `PRE_INITIALIZING\n`. This is a documented phase-01 choice — not captured from upstream — because Task 7's 60-probe tight-loop attempts against an empty-`listeners`/empty-`clusters` bootstrap did NOT capture any non-200 HTTP response: Envoy's initialisation completes faster than the network probe can race it. Per this ADR's option (b) (see Alternatives), that is acceptable because the phase-01 differential harness never observes the subject's pre-init window — `cmd/envoy-go` calls `MarkReady` before printing the ready sentinel that the harness waits on. Later phases that capture upstream's real pre-init bytes (e.g., by configuring a bootstrap that defers init) supersede this pre-init choice via a new ADR, and the admin server updates to match.

Framing exception: upstream emits `transfer-encoding: chunked` with no `Content-Length` header. The phase-01 subject is permitted to emit `Content-Length: 5` instead of chunked framing as a documented BEHAVIOR_CONTRACT deviation; the Task 14 differential harness normalises both forms to the dechunked/length-decoded body before byte comparison. Upgrading the subject to emit chunked framing is a phase-02+ follow-up, not a phase-01 gate.

**Rationale:**

- Byte-exact equivalence on the ready-state response is cheap (a constant handler) and gives the phase-01 differential gate concrete, verifiable teeth. Testing against fingerprinted bytes — rather than against an abstract "returns 200 OK with some LIVE-ish body" predicate — catches header-casing regressions, charset-token drift, and accidental CRLF-body terminators that would silently degrade upstream-compatibility claims.
- The pre-init window is outside the phase-01 differential scope by construction (`MarkReady` precedes the ready sentinel), so choosing a pre-init body that is easy to unit-test in `internal/admin` (a literal constant, not a network-captured fixture) is strictly superior to blocking the phase on re-running upstream probes with non-trivial bootstraps. Later phases with slower initialisation inherit a working pre-init contract they can refine.
- Anchoring the ready-state contract to a single committed evidence file (`upstream-ready-observation.md`) means any later disagreement between subject and upstream has one authoritative source. When upstream pins bump (ADR-0008 successor), Task 7 is re-run and both the evidence file and this ADR are updated together under a new ADR superseding this one.

**Alternatives considered:**

- *(a) Byte-exact the pre-init response as well.* Rejected — would require either (i) re-running Task 7 against multiple bootstrap shapes until a pre-init response is captured, which is out of scope and unscheduled, or (ii) reading Envoy's C++ source to transcribe the pre-init handler's output, which is brittle against patch-level upstream changes and is the kind of cross-source-tree coupling doctrine D-3.2 warns against.
- *(b) Document upstream pre-init as unobservable at this capture layer; emit a chosen pre-init body; harness does not exercise pre-init.* — **chosen**. The phase-01 subject still has a well-defined pre-init state for unit tests (Task 9 atomicity coverage) without blocking the gate on a capture that the evidence file shows is unobservable at this layer.
- *(c) Emit upstream's ready-state chunked framing exactly, including TE framing.* Deferred — the subject's framing path would need a chunked encoder in the admin handler. That is a pure implementation concern (the logical body is identical under chunked or length framing), and phase 02+ can switch without an ADR if the harness's dechunk normaliser remains in place. Phase 01 avoids the dependency by allowing the length-framed variant.

**Consequences:**

- `internal/admin` (Task 8) wires a constant ready-state body and header set matching the evidence file byte-for-byte, plus a constant pre-init response. No dynamic formatting beyond the `date` header (regenerated per request) and chunk framing if the subject chooses that path.
- `BEHAVIOR_CONTRACT.md` Admin API subsection (Task 10) quotes the evidence-file bytes verbatim, names `date` as the sole value-level allow-list entry for `/ready`, and records the chunked-vs-length framing deviation.
- `test/helpers/http_response.go` (Task 11) parses responses in both framings; the differential diff (Task 14) compares the dechunked body byte-exact and diffs headers modulo the `date` allow-list.
- Re-capture procedure: upgrade of ENVOY_TARGET.md pin triggers re-running Task 7's probe sequence against the new tag, updating `upstream-ready-observation.md`, and superseding this ADR if the bytes change. If the bytes are identical across pins, this ADR remains authoritative and the evidence file records the new digest alongside the old bytes.

**Link to evidence:** `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` — captured 2026-04-22 against `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`.

---

## ADR-0014: `Server:` header value on envoy-go admin responses

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

**Decision:** The envoy-go admin server emits the literal string `envoy` as the value of the `Server:` response header on every admin response, byte-exact with upstream Envoy v1.37.2.

**Rationale:** Matches upstream character-for-character per the Task-7 evidence file (`docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` §"Observations" — `server: envoy`, lowercase, no version suffix). This minimises allow-list entries on the differential gate — the `Server` header becomes a byte-exact equality check rather than a value-level allow-list entry. No phase-01 or declared future consumer encodes logic against identity headers, so mirroring upstream's self-description carries zero behavioural cost. Alternatives — a distinguishing value such as `envoy-go` or omitting the header — would either require an allow-list entry (reducing gate teeth) or diverge visibly from upstream on the wire without a matching functional benefit.

**Consequences:**

- `internal/admin` (Task 8 `handleReady`) sets `Server: envoy` unconditionally on `/ready` responses.
- Task 10 BEHAVIOR_CONTRACT Admin API subsection lists `server` under the byte-exact header set, not under the `date` value-level allow-list.
- Later admin endpoints (phase 08+) inherit this Server value; no per-endpoint override is introduced without a superseding ADR.
- If a later phase distinguishes envoy-go from upstream via a user-agent-style identity header, it adds a new header (e.g., `X-Envoy-Go-Version`) rather than modifying `Server`; the `Server` contract remains pinned to upstream.

---

## ADR-0019: Admin HTTP response parser location

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

**Decision:** `test/helpers/http_response.go` + `_test.go`.

**Rationale:** anticipated reuse by fixtures 0002+ that probe HTTP surfaces; colocated with `test/helpers/tcp.go` (phase-00 TCP round-tripper) establishes `test/helpers/` as the shared test-side protocol-primitives package.

---

## ADR-0020: `cmd/envoy-go/main_test.go` rewrite vs replacement

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5

**Decision:** rewrite (same file, same test name, bootstrap-shaped YAML + adminPort allocation).

**Rationale:** keeps cmd-level unit coverage lightweight without adding a subprocess-integration dimension that the differential suite already covers.

---

## ADR-0021: Supersession of ADR-0007 by `internal/bootstrap`

**Status:** Accepted
**Date:** 2026-04-22
**Doctrine:** D-3.5
**Supersedes:** ADR-0007

### Context

ADR-0007 codified the phase-00 minimal YAML schema for `envoy-go.yaml` (top-level `listener` / `upstream` blocks with `address` + `port`, parsed by `cmd/envoy-go/config.go`). Phase 01 replaces that contract with the real Envoy v3 Bootstrap proto consumed through `internal/bootstrap.Load` (ADR-0012, ADR-0013, ADR-0016). Task 12 of phase 01 (`08e09a9`) rewired `cmd/envoy-go/main.go` to call `bootstrap.Load` + the `AdminSocket` / `FirstListenerSocket` / `FirstClusterEndpointSocket` extractors, leaving `cmd/envoy-go/config.go` and `config_test.go` as orphans (no caller, no importer). Phase 01 completion requires retiring the phase-00 schema so the subject consumes real Envoy bootstrap YAML and nothing else; the orphan files must be deleted.

### Decision

The phase-00 minimal YAML schema codified in ADR-0007 is retired. envoy-go configuration is now the Envoy v3 Bootstrap proto as consumed by `internal/bootstrap.Load`. `cmd/envoy-go/config.go` and `cmd/envoy-go/config_test.go` are deleted. ADR-0007 itself is NOT edited (append-only per doctrine D-3.5); this ADR explicitly names ADR-0007 as superseded per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4, which mandates an explicit `**Supersedes:** ADR-XXXX` header on the retiring ADR.

### Rationale

Phase 01's charter (ROADMAP row 01) is to replace the phase-00 placeholder configuration contract with the real Envoy bootstrap. ADR-0012 (yaml.v3 → json → protojson pipeline) and ADR-0013 (`github.com/envoyproxy/go-control-plane/envoy` proto-types pin) codify the replacement parser and schema. ADR-0016 (`DiscardUnknown: false`) codifies the strict-parsing rule that preserves the phase-00 no-silent-typos guarantee at the proto layer. With the cutover landed and no callers remaining, keeping the phase-00 files would leave dead code that still compiles and still runs its own tests — a divergence between what the binary accepts and what the tree claims to accept. Deletion is the only end-state consistent with doctrine D-3.6 (green build) and with the single-contract principle.

### Consequences

- `cmd/envoy-go/` contains exactly `main.go` and `main_test.go` at phase 01 completion.
- ADR-0007 remains in the ADR log, unedited, as historical record; its `Status` line stays `Accepted` because supersession is recorded here rather than mutating the original entry.
- Any future phase that revives a non-bootstrap configuration contract must explicitly ADR around both ADR-0007 (historical) and ADR-0021 (this entry) — no silent revival.
- The new contract is fully specified by ADR-0012 (pipeline), ADR-0013 (proto types pin), and ADR-0016 (unknown-field rejection + Any preservation exception).

### Cross-references

- **ADR-0012** — YAML-to-proto pipeline (yaml.v3 → json → protojson).
- **ADR-0013** — `github.com/envoyproxy/go-control-plane/envoy` version pin (proto types only).
- **ADR-0016** — unknown-field rejection (`DiscardUnknown: false`) in the bootstrap loader.
- **ADR-0020** — `cmd/envoy-go/main_test.go` rewrite-vs-replacement decision that preceded this deletion.

---

## ADR-0024: Per-cluster `atomic.Uint64` counter as round-robin state scope

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

Phase 02 introduces a round-robin load balancer over a cluster's endpoints (SPEC §5.4). Envoy's data model places LB state at the cluster level — a cluster owns its endpoint pool and its load-balancer state. The implementation must choose the scope and primitive for the round-robin counter. The candidates are: (a) a process-global counter, (b) a per-listener counter, or (c) a per-cluster counter. Each has observable distribution consequences across multi-listener/multi-cluster configurations. The phase also imports `sync/atomic` for the first time in the project.

### Decision

Each `*Cluster` owns its own `atomic.Uint64` counter. The `roundRobin` LB consults only that cluster's counter. Endpoint selection uses `i := counter.Add(1) - 1; endpoints[int(i) % len(endpoints)]` — the subtract-one trick makes the first pick `endpoints[0]`, which unit tests pin as an internal correctness property but which is explicitly NOT promised to upstream Envoy (upstream's RR is per-worker with randomised starting offset; see ADR-0026 and the new BEHAVIOR_CONTRACT TCP proxy subsection added by phase-02 Task 8).

### Rationale

Per-cluster scope matches Envoy's data model and prevents the two failure modes of the alternatives:

- **Per-listener counter:** a future fixture where two listeners proxy to the same cluster `c_echo` would observe each listener's counter restart from 0, double-loading `endpoints[0]` at each accept burst. Endpoint load would depend on which listener accepted each connection — unrelated to the cluster's true load-balancing intent.
- **Process-global counter:** a multi-cluster bootstrap would conflate distribution across unrelated clusters. A burst of picks on cluster A would shift cluster B's starting index, making distribution non-stationary in a cross-cluster-coupled way that has no mapping to the cluster abstraction.

`atomic.Uint64.Add(1)` guarantees every goroutine observes a unique `i`, and `i mod N` is exactly balanced when `N | total_picks`. Unit tests exercise 100 goroutines × 30 picks each = 3000 picks and assert exact 1000/1000/1000 distribution across 3 endpoints (no tolerance). The subtract-one formula is preferred over starting at 1 because it makes the first-pick invariant (`endpoints[0]`) easy to state and pin in tests.

### Consequences

- Phase 02 LB state lives on `*Cluster` in `internal/cluster/loadbalancer.go`. No shared state across clusters.
- Sequence-level equivalence to upstream Envoy is NOT a differential dimension. The fixture-level assertion for phase-02's new fixture (`0001-tcp-proxy-rr`) is per-proxy distribution correctness (each proxy balances 3/3/3 over 9 requests), not cross-proxy sequence match.
- Future LB policies (LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV — all deferred to the load-balancing family, phase 09+) will be added alongside `roundRobin` as new types implementing the unexported `loadBalancer` interface; each owns its own state per-cluster. No existing code changes when they land.
- The `sync/atomic` import becomes a project-level dependency; the phase-01 `internal/bootstrap` package does not use it. Lint and vet coverage is the same as any other stdlib import.

### Cross-references

- **SPEC §5.4** — cluster manager + LB interface specification.
- **ADR-0026** — ready-sentinel format change (introduces the per-listener line format, referenced here only for the cross-ADR link to "per-worker LB state").
- **BEHAVIOR_CONTRACT.md `## TCP proxy`** (added by phase-02 Task 8) — codifies that LB sequence is NOT a differential dimension.

---

## ADR-0023: Lift phase-00 `netConn`/`pump`/`halfClose` trio verbatim from `cmd/envoy-go/main.go` into `internal/filter/tcpproxy/filter.go`

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

Phase 00 implemented an ad-hoc TCP pump directly in `cmd/envoy-go/main.go` to serve the `0000-tcp-echo` differential fixture (see phase-00 PLAN.md). The pump comprises three pieces: the `netConn` wrapper type (defeats Linux `splice(2)` loopback-data-loss behaviour; see the type's own doc-comment at `cmd/envoy-go/main.go:91-96`), the `pump` function body (bidirectional `io.Copy` + `halfClose` on each direction), and the `halfClose` helper (`CloseWrite` on `*net.TCPConn`). Phase 02 moves the dataplane from `main.go` into a proper `internal/filter/tcpproxy/` package (SPEC §5.5). The question: does the moved code deserve a redesign — tightening signatures, introducing a `pumper` type, adding metrics hooks, switching to an `io.CopyBuffer` variant — or is byte-for-byte fidelity the right choice?

### Decision

Byte-for-byte verbatim lift. The `netConn` type, the `halfClose` helper, and the pump body (now inlined into `Filter.Handle`'s method body rather than called as a free function) move with zero logic changes and zero comment edits beyond function-extraction mechanics (`pump(client, upstreamAddr)` becomes the contents of `Handle` after `net.DialTimeout` replaces the ad-hoc `net.Dial` — but the two goroutines doing the directional `io.Copy`+`halfClose` dance are character-identical). The splice-avoidance comment on `netConn` is preserved verbatim.

### Rationale

The phase-00 fixture is the baseline gate every subsequent phase must keep green. Any behaviour-affecting edit to the pump, no matter how well-intentioned, risks a differential regression whose bisect target is obscured by the simultaneous package move. A verbatim lift makes the lift itself reviewable by `git diff` — a reviewer can compare `cmd/envoy-go/main.go` at phase-01-tip against `internal/filter/tcpproxy/filter.go` at Task-4-tip and see the tokens are identical. The move is bureaucratic (package change, method-receiver change) rather than editorial (algorithm change, wrapper-shape change), and bureaucratic-only changes can land without re-validating the fixture. Task 7 removes the phase-00 original atomically with the `main.go` rewrite; the phase-00 fixture re-runs at that point to prove no regression.

### Consequences

- `internal/filter/tcpproxy/filter.go` is the authoritative location of the pump going forward. `cmd/envoy-go/main.go` retains its copy until Task 7's atomic cutover; Task 7 deletes the original so there is exactly one definition at phase-02 tip.
- The `netConn` wrapper's exported name is lowercase (unexported) in both locations. The filter package does not export the wrapper; callers never wrap connections themselves.
- Future pump optimisations (e.g., vectored I/O, `io.CopyBuffer` with a tuned buffer pool) land as their own phases or tasks, with their own fixture re-validation. Phase 02 does not touch the bytes.
- ADR-0023 supersedes nothing — the phase-00 pump was never ADR'd (the phase-00 PLAN.md §5.3 discussed it prose-only). This is its first ADR entry.

### Cross-references

- **SPEC §5.5** — TCP proxy filter responsibility and pump semantics.
- **phase-00 PLAN.md §5.3** — historical record of the pump's introduction and the splice-avoidance rationale.
- **Task 7** — atomic cutover that deletes the phase-00 original from `cmd/envoy-go/main.go`.

---

## ADR-0025: Phase-02 filter-chain subset — exactly one filter_chain, empty filter_chain_match, exactly one terminal filter

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

The Envoy listener model supports a rich filter-chain protocol: multiple filter_chains per listener each with a `filter_chain_match` (destination port / prefix / SNI / transport protocol / server names) selecting at connection-accept time, each chain itself a sequence of filters following an iteration protocol (`Continue` / `StopIteration` / `StopIterationNoBuffer` etc.). Phase 02's job is to land the first real dataplane; the full filter-chain framework is phase 07's charter. This ADR codifies which strict subset of the listener proto phase 02 accepts, so a reviewer and a future maintainer both know where the line is drawn and why.

### Decision

`internal/listener.NewManager` build-errors on every violation of the following subset:

1. `len(listener.filter_chains) == 1` — exactly one chain per listener.
2. `filter_chain_match` — absent or `proto.Equal` to the zero-value `&listenerv3.FilterChainMatch{}`. Any populated match errors.
3. `transport_socket` — must be nil. Non-nil errors with a message naming TLS and phase 03 for traceability.
4. `len(filter_chain.filters) == 1` — exactly one terminal filter.
5. `filter.typed_config.type_url` — must be registered in the inline filter registry. Phase 02 registers exactly one URL: `type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`. Unknown URLs error.
6. `listener.listener_filters` — silently skipped, NOT errored (SPEC §2 notes this; `tls_inspector`, `original_dst`, etc. are deferred to a later phase and a fixture that declares them today can stay declarative without breaking phase-02 acceptance).

The inline filter registry lives inside `internal/listener/manager.go` as a package-private `var filterRegistry = map[string]filterConstructor{...}`. Phase 07 generalises it into an exported registry package (`internal/filter/registry/`) with external registration support.

### Rationale

The full `FilterChain` protocol (match rules + iteration + read vs write filter distinction + per-route config + continue/stop/stopBuffered state) is a non-trivial subsystem. Phase 02's claim — "envoy-go runs real Envoy dataplane primitives — listener + filter + cluster + LB — end-to-end and remains byte-equivalent to upstream Envoy on a deterministic TCP workload" (SPEC §1) — is satisfied by the simplest correct case: one filter_chain, one terminal filter, no match rules. Accepting anything more would invite fixtures that depend on match behaviour that phase 02 does not implement correctly, and the resulting differential regressions would not be bisectable to a single phase.

Ignoring `listener_filters` (rather than erroring on it) is a deliberate asymmetry with the filter-chain rules above. Listener filters are a pre-filter-chain layer; fixtures that declare them do so for informational or future-use reasons and should continue to parse. By contrast, a populated `transport_socket` materially changes the bytes on the wire (TLS); silently ignoring it would diverge from upstream, so it errors.

### Consequences

- Phase-02 listeners have a single code path: one chain, one filter, no match. `internal/listener/manager.go` does not grow a chain-selection loop or an iteration-protocol state machine.
- Phase 07 supersedes this ADR when it lands the full framework; the phase-07 ADR explicitly names ADR-0025 as superseded and replaces the inline registry + the six-rule gate with the full protocol.
- Fixtures that currently carry `listener_filters` in their YAML continue to work unchanged (they are skipped at build time and the listener proceeds). Fixtures that want to test listener filters must land in a later phase.
- The error-message discipline (`listener: <name>: <violation>`) is what `manager_test.go` asserts on. Changing any of those strings breaks tests — which is the intended behaviour for a contract-defining ADR.

### Cross-references

- **SPEC §5.2** — listener manager responsibility and build-time behaviour.
- **SPEC §5.3** — inline filter registry spec.
- **SPEC §2** — non-purposes bullet list, including the filter-chain framework deferral and the listener_filters skip rule.
- **ADR-0023** — pump lift; provides the `Filter.Handle` method the registered filter exposes.
- **Phase 07 — filter chain framework** — eventual superseder of this ADR.

---

## ADR-0022: Retire phase-01 first-only extractors; introduce listener and cluster managers

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

Phase 01 landed `internal/bootstrap.FirstListenerSocket` and `internal/bootstrap.FirstClusterEndpointSocket` as a minimal extractor pair so the phase-01 subject could run with exactly one listener pointing at exactly one upstream endpoint. This was never ADR'd — the "exactly one" shape was a phase-01 simplification documented only in the `First*` doc-comments (`internal/bootstrap/bootstrap.go:74-134`). Phase 02 ships the first real dataplane: N listeners, N clusters, each listener wired to a terminal filter, each cluster a pool of endpoints. The `First*` extractors cannot represent this; either they expand (into full iteration walkers) or they are retired.

### Decision

The `First*` extractors are retired and deleted. Bootstrap traversal moves into the managers that own the concepts:

- `internal/listener.NewManager` walks `static_resources.listeners[]` (SPEC §5.2, ADR-0025).
- `internal/cluster.NewManager` walks `static_resources.clusters[]` (SPEC §5.4, ADR-0024).

`internal/bootstrap.AdminSocket` is NOT retired — admin remains a single global entity; there is exactly one `admin` in a bootstrap and a top-level extractor for it is still the right surface. `internal/bootstrap.Load` is also unchanged.

The `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"` blank import in `internal/bootstrap/bootstrap.go` stays — the filter package now also imports it via its direct typed import, but the bootstrap-side blank import costs nothing and keeps the bootstrap loader usable by tooling that does not pull in the filter package.

### Rationale

Phase 01's "exactly one X" discipline was implicit — callers were expected to read the `First*` doc-comments and not pass bootstraps with more than one listener or cluster. Phase 02 needs N-of-each and would have to either (a) add `AllListenerSockets(bs) []Socket` alongside `FirstListenerSocket` (growing the bootstrap package's surface without owning the cluster/listener abstractions), or (b) move walking into the managers that already own validation (filter-chain subset rules for listeners per ADR-0025; STATIC / ROUND_ROBIN rules for clusters per ADR-0024). Option (b) is the clean separation: `internal/bootstrap/` parses YAML→proto; `internal/listener/` + `internal/cluster/` impose phase-02 semantics and produce dataplane objects. No function in `internal/bootstrap/` owns domain validation after this ADR.

### Consequences

- `cmd/envoy-go/main.go` calls `cluster.NewManager(bs)` and `listener.NewManager(bs, cm)` directly; the bootstrap package is consulted only for `Load` (parse) and `AdminSocket` (admin-address lookup).
- `internal/bootstrap/bootstrap_test.go` loses the `TestFirstListenerSocket_*` and `TestFirstClusterEndpointSocket_*` groups. Remaining coverage: `TestLoad_*`, `TestAdminSocket_*`.
- No prior ADR is superseded — the first-only discipline was never ADR'd. This is the ADR that names and retires it.
- Any tool or test that imported the `First*` symbols now fails to compile. The only pre-Task-7 caller was `cmd/envoy-go/main.go`; that file is rewritten in the same commit.

### Cross-references

- **SPEC §5.2** — listener manager responsibility.
- **SPEC §5.4** — cluster manager responsibility.
- **ADR-0024** — per-cluster atomic.Uint64 RR counter scope.
- **ADR-0025** — phase-02 filter-chain subset.
- **ADR-0026** — ready-sentinel format change (lands in same Task-7 commit).

---

## ADR-0026: Ready-sentinel format change — per-listener line + terminal line; clean break from phase 00/01 single-line format

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5
**Supersedes:** (informal) phase-00 sentinel contract encoded in cmd/envoy-go/main.go:79 comment and test/differential/harness.go:readyAddr

### Context

Phase 00 and 01 emitted a single ready sentinel line on stdout: `envoy-go ready on <host:port>\n`. The harness parsed it via `readyAddr(line) string` and `SubjectProxy` exposed a no-arg `ListenerAddr() string`. This shape assumes exactly one listener — fine for phase 00 / 01, broken for phase 02 where a fixture may declare N listeners and the harness needs to look each one up by name.

### Decision

`cmd/envoy-go/main.go` emits, after every listener has bound, one line per listener `envoy-go listener <name> ready on <host:port>\n` followed by exactly one terminal line `envoy-go ready\n`. The phase-00/01 single-line format is retired — no backward-compat parser, no transitional emission, no deprecation grace period.

The harness's `readyAddr(line) string` parser is replaced by `readyListenerAddrs(ctx, reader) (map[string]string, error)` that walks lines until the terminal sentinel, collecting every `envoy-go listener <name> ready on <addr>` line into a name→addr map. `SubjectProxy.ListenerAddr() string` (no-arg) becomes `ListenerAddr(name string) string`.

The `test/differential/fixture.Driver` interface gains `SubjectListenerName() string` so the runner knows which listener's address to look up per fixture.

### Rationale

Per SPEC §10 #5, the harness is the only known consumer of the sentinel format. Retaining a transitional dual-format emitter would couple `cmd/envoy-go/main.go` to its own retired contract for no benefit, and would force every subsequent phase's main to preserve both formats. A clean break is simpler code, simpler test setup, and simpler ADR surface.

Per D-3.4, cross-session decisions must live on disk. The format change is a cross-session decision (the harness at session N consumes the subject's output at session N; any mismatch at session boundaries is a regression). Hence the ADR.

### Consequences

- `cmd/envoy-go/main.go` post-Task-7: after `lm.Start` succeeds and admin is marked ready, iterate `lm.Listeners()` and `Fprintf(os.Stdout, "envoy-go listener %s ready on %s\n", info.Name, info.Addr)` for each, then `Fprintln(os.Stdout, "envoy-go ready")`.
- `test/differential/harness.go`: `readyListenerAddrs` replaces `readyAddr`; `SubjectProxy.listenerAddrs map[string]string` replaces `listenerAddr string`; `ListenerAddr(name string) string` replaces the no-arg form.
- `test/differential/fixture.Driver`: `SubjectListenerName() string` added.
- `test/differential/runner_test.go`: `subj.ListenerAddr(d.SubjectListenerName())` replaces `subj.ListenerAddr()`.
- Fixture 0000 driver: `SubjectListenerName() string { return "l_tcp" }` declared in the Task-7 commit. Fixture 0001 (Task 9) declares the same.
- `cmd/envoy-go/main_test.go`: new test `TestEnvoyGoBinary_TwoListenerCutover` uses a two-listener bootstrap and parses both per-listener sentinels. The old single-listener `TestEnvoyGoBinary_EchoesThroughUpstream` is retired.
- The informal phase-00 sentinel contract (embedded only in a comment at `cmd/envoy-go/main.go:79` and the implementation of `readyAddr`) is superseded. Because that contract was never ADR'd, this ADR names the supersession under the informal `(informal)` qualifier rather than referencing an ADR number.

### Cross-references

- **SPEC §10 #5** — settled decision on clean-break vs transitional.
- **SPEC §5.2 / §5.7** — listener manager and cmd/envoy-go rewire.
- **ADR-0022** — first-only extractor retirement (lands same commit).
- **ADR-0025** — phase-02 filter-chain subset (upstream reason listeners need to be iterated at all).

---

## ADR-0027: Fixture `0001-tcp-proxy-rr` reference uses STRICT_DNS, subject uses STATIC

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

Phase 02's new differential fixture `0001-tcp-proxy-rr` exercises round-robin load balancing across a 3-endpoint cluster. The two sides of the fixture (reference Envoy container, envoy-go subject subprocess) run in different network topologies: the reference lives inside a Docker container and reaches host-side test backends via `host.docker.internal`; the subject lives as a subprocess on the test host and dials literal loopback addresses. The fixture must declare a cluster on each side that resolves to the same three test backends. Two questions: (a) what Envoy cluster `type` supports `host.docker.internal` resolution inside the container, and (b) what cluster `type` should the subject side declare.

### Decision

- **Reference** (`test/fixtures/0001-tcp-proxy-rr/envoy.yaml` + the driver's `ReferenceBootstrap(backendPorts)`): `type: STRICT_DNS`, `dns_lookup_family: V4_ONLY` (ADR-0010), three `lb_endpoints` at `host.docker.internal` with three distinct runner-rendered `port_value`s.
- **Subject** (`test/fixtures/0001-tcp-proxy-rr/envoy-go.yaml` + the driver's `SubjectConfig(...)`): `type: STATIC`, three `lb_endpoints` at literal `127.0.0.1` with three distinct runner-rendered `port_value`s.

### Rationale

STATIC is not a viable choice for the reference side because the container-internal DNS path (Docker Desktop's `host.docker.internal` → host-gateway) must be exercised to reach host backends, and STATIC endpoints are resolved at bootstrap-parse time with no DNS lookup. STRICT_DNS is the cluster type that actually consumes `host.docker.internal` with a V4-family lookup at cluster-init time (ADR-0010 codifies the V4_ONLY discipline).

STATIC is the right choice for the subject because the subject is a host subprocess and can dial literal 127.0.0.1 endpoints; STATIC resolves endpoints once at bootstrap time with no runtime DNS dependency, and phase 02 explicitly defers STRICT_DNS on the subject side to a later phase (SPEC §2). No differential gate is lost — the new BEHAVIOR_CONTRACT `## TCP proxy` subsection (phase-02 Task 8) explicitly excludes LB endpoint-selection sequence from the differential dimensions, so sequence divergence caused by the two different cluster types is not a failure.

### Consequences

- `test/fixtures/0001-tcp-proxy-rr/envoy.yaml` and `envoy-go.yaml` carry visibly different cluster declarations; the README documents the divergence and cross-references this ADR.
- The runner's multi-backend allocation produces a `backendPorts []int` slice that both sides template into their cluster's `lb_endpoints` list. No cross-side coupling beyond the port values.
- Same pattern fixture `0000-tcp-echo` already carries for its single endpoint (historical — predates ADR-0027 but is the same discipline).
- Any future fixture targeting the host-gateway from inside the Envoy container inherits this rule; any future fixture that has the subject resolve DNS at runtime requires its own ADR under the subject-side-DNS phase.
- The subject side deliberately keeps the 3-endpoint sequence deterministic-starting-at-0 (per ADR-0024); the reference side's per-worker randomized offset is not coordinated, and that is the BEHAVIOR_CONTRACT-level reason cross-proxy sequence equivalence is not asserted.

### Cross-references

- **ADR-0010** — V4_ONLY DNS rule; applies to every reference-side STRICT_DNS cluster.
- **ADR-0024** — per-cluster RR counter scope; subject-side sequence property.
- **ADR-0026** — ready-sentinel format; unrelated to the cluster-type divergence but lands in the same phase.
- **BEHAVIOR_CONTRACT.md `## TCP proxy`** — phase-02 Task 8 — documents the LB-sequence-not-asserted rule.
- **SPEC §4.4 ADR-F** — this ADR's pre-assignment in the SPEC phase.

---

## ADR-0028: Reference Envoy `--concurrency 1` for deterministic single-worker round-robin

**Status:** Accepted
**Date:** 2026-04-23
**Doctrine:** D-3.5

### Context

Upstream Envoy's round-robin load balancer holds its RR counter per-worker-thread with a randomized starting offset. When the reference Envoy container runs with its default worker count (autodetected from CPU count, typically >1), each worker accepts a subset of the N connections from the test client and runs its own RR counter over those. Over N=9 connections to a 3-endpoint cluster, the aggregate per-backend distribution is therefore NOT guaranteed to be exactly [3, 3, 3] — two workers starting at different offsets and unevenly receiving accepts can produce skews like [5, 3, 1].

Phase-02 SPEC §5.8 asserts "each proxy independently: forall i: counts[i] == 3 (exact, not tolerance). The N % 3 == 0 design makes the RR distribution exact." This assertion is satisfiable only when the reference proxy uses a single RR counter for all connections — i.e., single worker. Task 10's first differential run confirmed the skew empirically: reference-side distribution was `[5 3 1]`, failing the per-proxy assertion.

Two options exist to reconcile SPEC and reality: (A) force single-worker operation on the reference via Envoy's `--concurrency 1` CLI flag, making the SPEC's assumption hold; (B) relax AssertDistribution to assert exactness on the subject only, acknowledging the per-worker reference asymmetry. Option (A) preserves SPEC and BEHAVIOR_CONTRACT without edit and is a one-flag change with no observable behavior change on the fixtures' other gates (response-body byte-exactness holds whatever the worker count).

### Decision

The reference Envoy container in `test/differential/harness.go`'s `StartReferenceProxy` is invoked with `--concurrency 1` appended to the existing `envoy --config-yaml ... --log-level warn` command line. Every differential fixture (phase-01 carryover `0000-tcp-echo` and phase-02's new `0001-tcp-proxy-rr`) inherits this setting.

### Rationale

`--concurrency 1` forces the reference to run a single worker thread. The single worker owns a single RR counter per cluster; all N connections are dispatched through that one counter; the mod-M distribution over N when `M | N` is exactly N/M per endpoint regardless of the counter's starting offset. This matches the subject side (ADR-0024's per-cluster `atomic.Uint64` deterministic RR) at the distribution level without making any claim about sequence — sequence equivalence remains NOT asserted per the BEHAVIOR_CONTRACT TCP proxy subsection.

Option (B) — subject-only assertion — was rejected because:
- It weakens SPEC §5.8's per-proxy guarantee with no observable benefit in subsequent phases.
- The BEHAVIOR_CONTRACT explicitly mentions sequence as non-equivalent but frames distribution as a local correctness property of each proxy; dropping reference-side distribution asserts a double standard where the reference is held to weaker correctness than the subject.
- `--concurrency 1` is a lower-effort, lower-ambiguity change than editing the SPEC or the assertion shape.

### Consequences

- `test/differential/harness.go`:112 carries the `--concurrency` flag. No other harness code changes.
- Every fixture driver's `AssertDistribution` (optional per the `DistributionAsserter` interface) can assume single-worker reference semantics. Fixture 0001's `AssertDistribution` asserts exact [3, 3, 3] on both sides.
- Reference Envoy's single-worker operation is an observable property under the `--concurrency` CLI flag; any future fixture that exercises concurrency-dependent phenomena (e.g., multi-worker stat aggregation, hot-restart worker-count assertions) requires either a per-fixture concurrency override or a successor ADR raising the baseline.
- Response-body byte-equivalence and admin `/ready` byte-equivalence (phase-01 baseline) are unaffected — they are behavioral surfaces independent of worker count.
- Fixture 0000-tcp-echo's historical gate is preserved: its single-endpoint cluster + single-connection echo has no distribution assertion; `--concurrency 1` changes the reference's internal scheduling but not any observable byte on the wire.
- The flag is scoped to the differential harness's reference container. Envoy-go's subject is already per-cluster single-counter RR (ADR-0024) regardless of OS-thread count, so no subject-side change is needed.
- Unrelated phase-02 fix: the `randHex(6)` per-call uid in both fixture drivers' `Drive` methods was removed at the same time. Under the post-Task-7 runner pattern that calls `Drive` once per side, the per-call uid produced different payloads on the two calls, which diverged at the byte-diff gate. Deterministic payloads (`ping-0\n...ping-9\n` for fixture 0000; `rr-0\n...rr-8\n` for fixture 0001) restore byte-equivalence with no loss of debuggability. This fix is a bug repair rather than a cross-session decision; the ADR is mentioned here rather than its own ADR because it is pure Task-7 fallout, not a doctrine-level change.

### Cross-references

- **ADR-0024** — per-cluster RR counter scope on the subject side.
- **ADR-0026** — ready-sentinel format change (no LB relation; referenced only to cross-link the phase-02 ADR set).
- **SPEC §5.8** — fixture 0001 distribution assertion.
- **BEHAVIOR_CONTRACT.md `## TCP proxy`** (phase-02 Task 8) — codifies distribution as a local-per-proxy correctness property and sequence as non-asserted.
- **Envoy CLI reference** — `--concurrency N` documented in Envoy's operations guide; N=1 is always valid.

---

## ADR-0029: DataSource handling policy (phase 03 scope)

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

`internal/tls` parses Envoy v3 DownstreamTlsContext / UpstreamTlsContext whose cert and CA material is carried via `envoy.config.core.v3.DataSource`. DataSource has four specifiers: `inline_bytes`, `inline_string`, `filename`, `environment_variable`, plus the SDS-bound forms on CommonTlsContext (`tls_certificate_sds_secret_configs`, etc.). Phase 03 must pick a subset consistent with SPEC §2 (non-purposes) and SPEC §5 (in-scope surface).

### Decision

`internal/tls.loadDataSource(ds, baseDir)` supports `inline_bytes`, `inline_string`, and `filename` only. `filename` is resolved relative to `baseDir` when not absolute; the caller passes the bootstrap file's directory. `environment_variable` errors with `tls: data source: environment_variable is not supported in phase 03`. Zero-value DataSource errors. SDS-bound secret configs error at the `internal/tls/config.go` caller layer (outside this function), keeping this function branch-minimal.

### Consequences

- Phase-03 fixtures can inline every PEM via `inline_bytes` or `inline_string`, matching the committed-PEM + deterministic-generator discipline of `test/fixtures/0002-tls-tcp/pki/`.
- Filename support is included from phase 03 rather than deferred because the implementation cost is trivial and future phases (xDS family, dynamic secret reload) will need it. No dynamic reload (file-watch / inotify) is implemented — phase 03 reads each file exactly once at listener-manager build time.
- `environment_variable` + SDS-bound secrets are bounded deferrals: phase 03 errors at parse time, preserving the "errors begin with `tls: `" discipline so callers can surface them uniformly.
- `baseDir` is a plan-level contract between the bootstrap loader (which knows the config file path) and this function. Tests pass an explicit `t.TempDir()` to avoid CWD-dependence.

---

## ADR-0030: TLS parameter mapping scope (phase 03)

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

Envoy's `common_tls_context.tls_params` exposes four configuration knobs: `tls_{minimum,maximum}_protocol_version`, `cipher_suites`, `ecdh_curves`, `signature_algorithms`. Go's stdlib `crypto/tls` does not surface every one of these as a public configuration: TLS 1.3 cipher selection is not permitted (RFC 8446 design choice — the spec selects AEAD ciphers), and `signature_algorithms` is not settable on `tls.Config`. Phase 03 must declare which fields are honoured, which error, and which are silently dropped with a diagnostic.

### Decision

`internal/tls.applyTLSParams` maps per-field as follows (this section is the authoritative surface; duplicates SPEC §5.5 for traceability):

| Envoy field | Phase-03 behaviour |
|---|---|
| `tls_minimum_protocol_version` | TLSv1_2/TLSv1_3 → `stdtls.VersionTLS12/TLS13`; TLSv1_0/TLSv1_1 → error; TLS_AUTO → no-op (treat as unset). |
| `tls_maximum_protocol_version` | Same mapping. |
| `cipher_suites` | TLS 1.2 IANA/OpenSSL names → `stdtls.CipherSuites()` IDs; unknown → error; TLS-1.3-only names → diagnostic-logged and dropped (not applied to cfg). |
| `ecdh_curves` | `X25519`/`P-256`/`P-384`/`P-521` → `stdtls.CurveID`; unknown → error. |
| `signature_algorithms` | Populated → error (stdlib has no public configuration knob). |

### Consequences

- Fixtures that pin TLS 1.2 ciphers get per-cipher selection parity with Envoy. Fixtures that pin TLS 1.3 ciphers see a diagnostic log but negotiation proceeds with Go's default TLS 1.3 cipher list (AEAD ciphers per RFC 8446). The BEHAVIOR_CONTRACT TLS subsection (ADR-0035) explicitly does not assert encrypted-side byte equivalence, so TLS 1.3 cipher divergence between Go and Envoy's BoringSSL does not break any asserted gate.
- A fixture that sets `signature_algorithms` fails fast with a clear error rather than silently no-op'ing. Future phases can revisit if Go's crypto/tls exposes the knob publicly (none as of Go 1.23).
- The cipher-name table is deliberately narrow (6 TLS 1.2 AEAD suites + 3 TLS 1.3 names as the silent-drop list). Adding suites is a trivial follow-on PR when a fixture needs one.

---

## ADR-0031: TLS stack selection — stdlib crypto/tls (phase 03)

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

Phase 03 introduces envoy-go's first cryptographic surface: downstream TLS termination, upstream TLS origination, and SNI-based filter-chain dispatch. The choice of TLS stack is foundational — every later phase that touches the wire (HTTP/1.1 over TLS, HTTP/2, HTTP/3, gRPC) builds on it. Three options considered:

- (A1) Go stdlib `crypto/tls`.
- (A2) BoringSSL via cgo (e.g., via `github.com/google/boringssl` or a vendored build).
- (A3) Third-party pure-Go or bound stacks (`rustls` via cgo; `github.com/refraction-networking/utls`).

### Decision

**(A1) stdlib `crypto/tls` is the phase-03 (and project-default) TLS stack.**

### Rationale

- **No cgo.** The project's pure-Go build posture simplifies cross-compilation and container base-image choices. (A2) and (A3-cgo) would pull in a C toolchain.
- **TLS 1.2 / 1.3 parity on asserted surface.** `crypto/tls` implements TLS 1.2 and 1.3. The phase-03 differential contract asserts plaintext-after-decryption byte equivalence only; encrypted-side observables (TLS record boundaries, session ticket material, TLS 1.3 cipher selection) are explicitly excluded from the contract (see ADR-0035 / BEHAVIOR_CONTRACT TLS subsection). Any divergence in these observables between `crypto/tls` and Envoy's BoringSSL is a *permitted* divergence under the contract.
- **ALPN + SNI + peer validation natively supported.** `stdtls.Config.NextProtos`, `GetConfigForClient`, `Certificates`, `RootCAs`, `ServerName`, `VerifyConnection` are all first-class — no wrappers needed.
- **License-clean.** `crypto/tls` is BSD-3-Clause (Go's license); no GPL copy-paste risk (D-3.2).
- **No vendoring.** `crypto/tls` ships with the Go toolchain; no dependency-pin worry.

### Known tradeoffs (documented in ADR-0030 and BEHAVIOR_CONTRACT TLS)

- **TLS 1.3 cipher selection not configurable.** RFC 8446 design. Envoy's `cipher_suites` becomes a no-op for TLS 1.3 ciphers; ADR-0030 records the silent-drop + diagnostic.
- **`signature_algorithms` not publicly configurable.** Stdlib omission. ADR-0030 errors if a fixture sets it.
- **Handshake timing / record-boundary divergence.** Go vs BoringSSL differ on both. BEHAVIOR_CONTRACT TLS subsection explicitly excludes these from assertion.

### Consequences

- Every `internal/tls/*.go` file imports `crypto/tls` as `stdtls` to avoid name collision with the package itself.
- Phase 04 (HTTP/1.1) layers `net/http.Server` on TLS via `stdtls.Listen` (or manual composition) — no TLS-stack decision at that phase.
- Phase 05 (HTTP/2) uses `golang.org/x/net/http2` on top of `crypto/tls` listeners.
- Phase 06 (stats) emits TLS-subsystem stats observable through Go's `crypto/tls.ConnectionState` — no stdlib hook changes required.
- If a later phase requires a capability `crypto/tls` doesn't expose (e.g., post-quantum hybrid key exchange before Go adds it), a superseding ADR re-scopes. Phase 03 does not anticipate this need.

---

## ADR-0032: Upstream TLS dialer model — Cluster.Dial(ctx) (net.Conn, error)

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

Phase 02's TCP proxy filter dialed endpoints directly via `net.DialTimeout` inside `Filter.Handle`. Phase 03 introduces upstream TLS origination — the filter must not branch on transport type (plaintext vs TLS) because phase 04 will add HTTP/TLS and phase 05 HTTP/2-over-TLS, and the filter body should stay transport-agnostic.

### Decision

`*Cluster` grows `Dial(ctx context.Context) (net.Conn, error)` returning a ready-to-read/write `net.Conn`. Plaintext clusters return `*net.TCPConn` (from `net.Dialer.DialContext`). TLS clusters return `*stdtls.Conn` after `HandshakeContext(ctx)` succeeds. The filter calls `Cluster.Dial(ctx)` regardless of transport.

`connect_timeout` applies to the TCP dial (via `net.Dialer.Timeout`). TLS handshake is bounded by `ctx` — if the caller has a deadline-bounded context, the handshake inherits it; otherwise it blocks until completion (matching Envoy's behaviour with no configured handshake timeout).

### Consequences

- `internal/filter/tcpproxy/filter.go` loses its direct `net.DialTimeout` call (Task 11, ADR-0032 aftermath). The filter body becomes two lines shorter and transport-agnostic.
- Phase-02 REVIEW Minor 4 (`ctx` unused in `Filter.Handle`) is resolved: the early `ctx.Err()` guard + `Cluster.Dial(ctx)` call fully consume `ctx`.
- The `halfClose` helper in the filter gains a `*stdtls.Conn.CloseWrite` case (Task 11) — unrelated to this ADR but a consequence of uniformly wrapping upstream conns.
- Cluster construction (`NewManager` → `buildCluster`) now threads `baseDir` through so `internal/tls.NewUpstreamConfig` can resolve filename-based DataSources against a well-defined root. Phase-02 test harness uses `""` baseDir (plaintext only; no DataSource); phase-03 main passes `filepath.Dir(configPath)`.
- `*stdtls.Conn.CloseWrite` sends a close_notify alert + TCP FIN, preserving the half-close propagation that ADR-0023's `netConn` wrapper relies on.

---

## ADR-0033: Phase-03 filter-chain subset (supersedes ADR-0025)

**Supersedes: ADR-0025**
**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

ADR-0025 (phase 02) constrained `internal/listener.NewManager` to accept exactly one `filter_chain` per listener with empty `filter_chain_match` and no `transport_socket`. Phase 03 introduces SNI-based filter-chain dispatch — multiple chains per listener, each bound to a set of SNI patterns — as its core new surface. ADR-0025's one-chain constraint is obsolete.

### Decision

Phase-03 subset:

1. `filter_chains` must be ≥ 1 (unchanged structural requirement).
2. `filter_chain_match` may be nil/empty (catch-all, at most one per listener) OR populate only `server_names[]` and optionally `transport_protocol == "tls"`. Any other `FilterChainMatch` field populated (destination_port, prefix_ranges, source_type != ANY, source_ports, source_prefix_ranges, application_protocols) errors at build.
3. `Listener.default_filter_chain` set → error.
4. `transport_socket` on any chain may be nil (plaintext) or carry a `DownstreamTlsContext` (TLS).
5. If any chain's `transport_socket` is non-nil, every chain on that listener must carry one — mixed TLS/plaintext listeners error.
6. Plaintext listeners with more than one `filter_chain` error — SNI cannot match on plaintext connections, so multiple plaintext chains is almost always a misconfiguration.
7. `require_client_certificate=true` on any chain errors (propagated from `tls.NewDownstreamConfig`).
8. `listener_filters` is silently skipped (phase-02 carryover; phase 07 filter-chain framework revisits).
9. Selection at handshake, in priority order: most-specific exact SNI match > suffix-wildcard match > universal wildcard match > catch-all (empty-match chain) > no match (handshake fails via `GetConfigForClient` returning `(nil, error)`; the connection closes).

### Chain-selection propagation (implementation)

Dispatching to the correct filter after a successful handshake is a pure function of the handshake-observed SNI. The worker goroutine, after `HandshakeContext` returns successfully, reads `tlsConn.ConnectionState().ServerName` and re-runs the same chain-match logic the `GetConfigForClient` callback ran, picking the first match. This is simpler than the `sync.Map` shuttle initially contemplated in SPEC §10 #2 approach (A) and avoids any per-connection state outside the `*stdtls.Conn` itself. Deterministic: SNI is fixed from the ClientHello through the connection's lifetime.

### Rationale

- SNI dispatch is the minimum complexity increment over ADR-0025 needed for phase 03. Full `FilterChainMatch` — including port ranges, source IP, ALPN, transport protocol beyond `"tls"` — remains deferred to phase 07 (filter chain framework).
- Rejecting `Listener.default_filter_chain` (Envoy's alternate catch-all form) bounds phase-03's match-resolution surface. Phase 07 supports both forms.
- Rejecting plaintext multi-chain catches a configuration class that's almost always a bug — SNI cannot match on plaintext connections, so the intent is ambiguous.
- Single mechanism for chain selection (pure-function dispatch post-handshake) reduces the surface area of "how chain selection happens" from two places (callback + shuttle) to one (match logic reused in callback and worker).

### Consequences

- Fixture 0002 can build a 2-chain TLS listener with `alpha.envoy-go.test` → `c_alpha` and `beta.envoy-go.test` → `c_beta` — the phase's core demonstration.
- A fixture later in phase 03 or after needing more than "exact + suffix wildcard + catch-all" must wait for phase 07.
- `internal/listener.Manager.Stop` is unchanged (closes every bound listener socket; accept loops exit on `net.ErrClosed`).
- `internal/listener/manager.go` grew by ~250 lines (build-time validation + chain-sort + `GetConfigForClient` + serveTLS worker + dispatch). The `sync.Map` shuttle from SPEC §10 #2 was not implemented; pure-function dispatch post-handshake is the locked mechanism.
- `NewManagerWithBaseDir` is introduced (mirrors cluster package pattern) so filename-based DataSources in transport_socket can be resolved relative to the config file; `NewManager` delegates with `""` baseDir for phase-02 compat.

---

## ADR-0034: Fixture driver interface — retire Drive, introduce DriveReference + DriveSubject

**Supersedes (informal):** the phase-02 `fixture.Driver.Drive(ctx, refAddr, subjAddr)` interface method codified in `test/differential/fixture/fixture.go`. No prior formal ADR — hence the `(informal)` qualifier.
**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.6

### Context

The phase-02 `fixture.Driver` interface exposed a single `Drive(ctx, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)` method. The runner called it twice per fixture run — once as `Drive(ctx, refAddr, "")` to drive only the reference side, and once as `Drive(ctx, "", subjAddr)` to drive only the subject side. Drivers were required to no-op whichever address argument was empty.

This design had three problems:

1. **Implicit sentinel protocol.** The empty-string convention was undocumented at the call site and easy to misread. A driver author seeing `Drive(ctx, "", subjAddr)` had no indication that `refAddr == ""` was a caller-controlled no-op signal rather than a configuration error.
2. **Unnecessary branching in every driver.** Each driver body contained two `if addr != "" { ... }` guards that existed purely to satisfy the interface contract, not to implement fixture logic.
3. **Two return values per call, one always nil.** Each invocation allocated a `(refBytes, subjBytes)` pair but the caller only consumed one side; the other was always `nil`. This is a mild waste but more importantly a confusing signature.

Phase 03 is the first phase to introduce a third fixture (0002-tls-tcp) whose driver needs to dial with a `*tls.Config`. The split interface makes the per-side method signatures simpler to extend (e.g., a future `DriveReferenceTLS` overload or context-carrying variant) and removes the no-op convention entirely.

### Decision

Retire `Drive(ctx, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)` from the `fixture.Driver` interface. Replace it with two focused methods:

```go
DriveReference(ctx context.Context, addr string) ([]byte, error)
DriveSubject(ctx context.Context, addr string)   ([]byte, error)
```

The runner calls `DriveReference` for the reference side and `DriveSubject` for the subject side, each with the correct address. No empty-string sentinel is passed; each method receives a valid `host:port` address unconditionally.

All existing drivers (0000-tcp-echo, 0001-tcp-proxy-rr) are updated atomically in the same commit. The compile-time interface guard in 0001's driver (`var _ fixture.Driver = (*rrDriver)(nil)`) enforces completeness.

### Consequences

- The empty-string no-op convention is eliminated from all drivers and the runner.
- Each driver method has a single responsibility: drive one side.
- The shared payload helper (`echoPayload()`, `rrPayloads()`) is extracted as a package-level function so both `DriveReference` and `DriveSubject` remain deterministic and byte-identical across calls.
- Phase-02 REVIEW Minor 6 is resolved.
- Any future fixture driver must implement both methods; there is no default or wrapper. This is intentional — the interface is small (two methods) and forcing explicit implementation prevents accidental omission.
- The `DistributionAsserter` optional interface and `ProbeAdmin` method are unaffected.

---

## ADR-0035: Fixture 0002 differential scope — downstream TLS + SNI only; upstream TLS unit-tested

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5

### Context

Phase-03 PLAN.md §"File Structure" and §"Task 13" described fixture `0002-tls-tcp` as exercising both downstream TLS termination (2 SNI-indexed filter chains) and upstream TLS origination (2 STATIC/STRICT_DNS clusters each with an UpstreamTlsContext). During Task 13 execution, these two goals were not simultaneously achievable under the PLAN's other constraint that `test/differential/harness.go` stays unchanged this phase:

- The harness's `runFixture` loop (`test/differential/runner_test.go`) allocates backends via `net.Listen("tcp", "0.0.0.0:0")` and serves them with `acceptEchoCounting`, a plain-TCP echo loop. There is no facility for TLS-wrapped backends.
- The fixture driver receives those backend ports via `ReferenceBootstrap(backendPorts []int)` / `SubjectConfig(..., backendPorts []int, ...)` and must use them — the harness tracks per-backend accept counts there for the `[3,3,3]` distribution assertion.
- Making the clusters TLS (UpstreamTlsContext with inline PEMs) would cause handshake failures against the plain-TCP backends, so the distribution gate would never fire.
- Extending the harness to spawn TLS-wrapped backends would violate PLAN.md's explicit "harness unchanged this phase" directive.

The PLAN's two requirements (fixture exercises upstream TLS; harness unchanged) were therefore structurally contradictory.

### Decision

Fixture `0002-tls-tcp` as landed in Task 13 exercises:

- **Downstream TLS termination** with 2 SNI-indexed filter chains (`alpha.envoy-go.test` → `c_alpha`, `beta.envoy-go.test` → `c_beta`).
- **SNI-based filter-chain dispatch** via the listener manager's `GetConfigForClient` callback (ADR-0033).
- **Per-cluster round-robin distribution** assertion `[3,3,3]` per SNI per side.
- **Byte-exact plaintext response-body equivalence** across 18 TLS round-trips per proxy.

Upstream TLS origination is NOT exercised in the differential fixture. The upstream-TLS code paths are covered by:

- `internal/cluster/cluster_test.go` — `TestCluster_Dial_TLS`, `TestCluster_Dial_TLS_HandshakeFailure`, `TestCluster_Dial_CtxCanceled` (Task 9).
- `internal/tls/config_test.go` — `TestNewUpstreamConfig_Happy` + error subtests of `TestNewUpstreamConfig_Errors` (Task 5).
- `internal/tls/fuzz_test.go` — `FuzzTLSContextParse` seed (b) stresses UpstreamTlsContext parsing (Task 6).

### Rationale

Three options considered:

1. **Land fixture 0002 with downstream TLS + SNI only; document scope reduction in this ADR.** Preserves "harness unchanged" invariant; preserves SPEC §3 gate (a) wording ("byte-exact plaintext + per-cluster [3,3,3] distribution per side" — no explicit upstream-TLS requirement at gate level); relies on unit-test coverage for upstream TLS code paths. **Chosen.**

2. Extend the harness with TLS-backend support. Would require per-fixture opt-in or protocol discovery; grow harness by ~80 LoC; violates PLAN's "harness unchanged" directive; expands phase-03 scope.

3. Split phase 03 per §6.2 into 03.1 (downstream TLS + SNI, landed) and 03.2 (upstream TLS differential gate, needs harness work). Most honest, but the PLAN was already committed as a single phase; splitting retroactively adds ceremony without clear benefit — phase 04 or a later phase can drive upstream TLS differentially when HTTPS fixtures become natural (HTTP/1.1 upstream TLS, phase 04+; HTTP/2 upstream TLS, phase 05+).

Option 1 minimizes churn, preserves the SPEC §3 gate wording, and documents the gap explicitly.

### Consequences

- **SPEC §1 scope claim** — "downstream TLS termination + upstream TLS origination + SNI" — is delivered in CODE but only two of three are exercised by the phase-03 differential fixture. Upstream TLS is demonstrably functional (unit-tested) but not differentially asserted.
- **SPEC §3 gate (a)** remains satisfied as worded: fixture 0002 is green with byte-exact plaintext + per-cluster [3,3,3] distribution per side.
- **BEHAVIOR_CONTRACT TLS subsection** (Task 14) must reflect the actual differential surface. Asserted dimensions are plaintext-after-decryption byte equivalence and per-SNI chain-selection equivalence (via distribution assertion). The Task-9/Task-5 upstream-TLS code paths are explicitly noted as *unit-tested only*, not differentially asserted. The subsection must NOT claim "upstream SNI + CA equivalence" as a differential assertion.
- **Task 14's PLAN-assigned ADR number shifts.** PLAN.md assigned ADR-0035 to Task 14's BEHAVIOR_CONTRACT TLS subsection. This ADR (landing during Task 13 aftermath, before Task 14) takes the next sequential number 0035 to preserve file-order monotonicity. Task 14's ADR will land as **ADR-0036** with a PROGRESS note recording the shift.
- **A future phase** that needs upstream-TLS differential coverage will either: (a) extend `test/differential/harness.go` with TLS-backend support (own ADR when that phase lands); or (b) drive upstream TLS through a naturally-TLS fixture such as phase 04's HTTPS HTTP/1.1 upstream.
- **Committed PKI** (`test/fixtures/0002-tls-tcp/pki/upstream-*.pem`) remains as-committed — the upstream leaf PEMs are used by Task 9 unit tests and are forward-compatible with a later harness-extension phase.
- **No rollback of landed code.** Task 9's `Cluster.Dial` TLS branch, Task 5's `NewUpstreamConfig`, Task 10's TLS listener path all remain in the production binary. The scope reduction affects only the differential-fixture coverage, not the runtime surface.

This ADR supersedes nothing — it documents a scope adjustment that became apparent only during Task 13 execution. PLAN.md is not amended (plans are frozen at landing per phase-02 precedent); this ADR is the authoritative rationale.

---

## ADR-0036: BEHAVIOR_CONTRACT TLS subsection (phase 03) + TCP-proxy ADR-0028 cross-reference (Minor 8)

**Status:** Accepted
**Date:** 2026-04-24
**Doctrine:** D-3.5, D-3.3

### Context

Phase 03 introduces envoy-go's first cryptographic surface. The differential contract must codify which TLS-related observables are asserted across reference and subject and which are permitted to differ — without this, a reviewer cannot say whether a cipher-level divergence is a gate failure or a permitted variance. Phase-02 REVIEW Minor 8 additionally flagged that the TCP-proxy subsection did not cross-reference ADR-0028's `--concurrency 1` reference-container pin, which is a precondition for the distribution assertions in fixtures 0001 and (inherited) 0002.

### Decision

A new `## TLS` subsection lands in `BEHAVIOR_CONTRACT.md`, phrased so that (a) every *asserted* rule has a fixture gate that witnesses it, (b) every *not-asserted* rule names the specific observable and the reason it's excluded, (c) the upstream-TLS scope reduction from ADR-0035 is reflected (upstream SNI + CA equivalence is listed as unit-tested only, not differentially asserted), (d) tradeoffs with Go's `crypto/tls` (ADR-0030) are noted so future reviewers don't read divergence as a gate regression.

In the same commit, the existing `## TCP proxy` subsection's "LB endpoint-selection sequence (NOT asserted)" paragraph gains a one-sentence cross-reference to ADR-0028 (phase-02 REVIEW Minor 8 resolution).

### Note on ADR numbering

PLAN.md §"ADRs introduced by this plan" assigned ADR-0035 to this task. That number was consumed by the Task-13 deviation ADR (fixture-0002 differential scope reduction) landed in commit `ddbe63e` before this task. To preserve sequential file-order monotonicity in DECISIONS.md, this task's ADR takes the next available number: ADR-0036. The PLAN is not amended; this note is the authoritative pointer.

### Consequences

- Phase-03 gates are now fully traceable to written rules. Fixture 0002's byte-exact plaintext assertion is under "Plaintext-after-decryption byte equivalence"; its distribution assertion is under "Per-SNI chain-selection equivalence."
- A future reviewer encountering a TLS 1.3 cipher divergence between Go and Envoy can point at "Encrypted-side byte equivalence: not asserted" and close the ticket without a gate investigation.
- Phase-02 REVIEW Minor 8 is resolved. Minor 7 (prose-heavy `expectations.yaml`) remains deferred per ADR-0019.
- Phase 04+ TLS-touching phases extend this subsection (or add siblings — e.g., a `## HTTP over TLS` subsection in phase 04) rather than rewriting it.
- The upstream-TLS "unit-tested only" clause is expected to be superseded by a later phase that adds TLS-backend support to the harness (phase 04 HTTPS fixtures, or a dedicated harness-extension phase).

---

---

## ADR-0037: HTTP/1.1 wire codec source — stdlib `net/http`

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-H, phase-04 §4.1 codec.go

### Context

Phase 04 introduces envoy-go's first HTTP-aware dataplane. The wire codec for HTTP/1.1 request parsing, response readback (for the router action), and local-reply generation must be doctrine-compatible: D-3.2 forbids `net/http/httputil.ReverseProxy`, embedding 3rd-party server cores (Caddy, Traefik, fasthttp), and copying GPL-licensed code. It permits Go standard library use as foundation. Three candidate sources were considered:

- (H1) Handcrafted RFC 7230 / RFC 9112 parser+writer.
- (H2) Stdlib `net/http.ReadRequest`, `Request.Write`, `ReadResponse`, `Response.Write`.
- (H3) Build on `net/http.Server` + `http.Handler`.

### Decision

(H2) is selected. Phase 04's wire codec consumes only stdlib parsers/serializers, never `net/http.Server` and never `http.Handler`.

Rationale:

- (H1) carries an unbounded RFC-corner-case tax that stdlib already pays (chunked encoding, header continuation, Host enforcement, request-target form, header field validation). Hand-rolling these is one to several phases of work for no asserted-surface benefit at phase 04.
- (H3) is forbidden in spirit by D-3.2: `net/http.Server` injects `Date` and `Content-Length` automatically, strips per-RFC headers, enforces RedactHeaders, and assumes HTTP-server semantics that are wrong for a proxy (an Envoy proxy must preserve upstream headers verbatim and is not the canonical authority for response Date/Server values on routed responses). Using `http.Server` would silently introduce divergences from upstream Envoy that cannot be patched out without forking stdlib.
- (H2) keeps the doctrine intent (no `httputil.ReverseProxy`, no third-party server core) while sidestepping (H1)'s tax. The stdlib parsers/serializers are loose enough to use as primitives without inheriting `http.Server`'s magic.

### Consequences

Documented residual stdlib-driven divergences from upstream Envoy that ADR-0044's BEHAVIOR_CONTRACT subsection records:

- Header canonicalization (`textproto.CanonicalMIMEHeaderKey` capitalises header names — `Content-Type`, not `content-type`). Envoy emits lowercase. The phase-04 differential allow-list compares header names case-insensitively (already true in the runner per `helpers.HTTPHeaderDiff`).
- `Host` header validation: stdlib `http.ReadRequest` rejects malformed Host values; Envoy accepts a wider grammar. Phase-04 fixtures issue only well-formed Host values, so the divergence is not exercised.
- Method whitelist: stdlib does not reject custom methods but normalises certain method spellings (e.g., `GET`/`get` round-trip to `GET`). Envoy preserves wire-form. Phase-04 fixtures issue only canonical-spelling methods.
- `Connection` header handling: stdlib's `Request.Write` may add or remove `Connection`-related headers based on the request's `Close` field. Phase-04 router action passes the original request through `Request.Write` after `http.ReadRequest`, so any stdlib-driven change is documented as part of the upstream-side request preservation rule's "bounded normalisation" caveat.

`net/http.Server` and `http.Handler` are NEVER imported by `internal/filter/hcm/`. Code review enforces. Future phases that need HTTP-server-style handling for a non-proxy purpose (e.g., the admin API in phase 08) may use `http.Server` in their own packages — this ADR scopes only the HCM dataplane.

Lands in Task 3 (first use site of `writeStatusReply`).

---

## ADR-0038: Phase-04 route match subset — `match.prefix` (bytewise) + `match.path` (case-sensitive exact)

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-J, phase-04 §4.1 route.go

### Context

Envoy's `route.RouteMatch` proto carries a `path_specifier` oneof with seven variants (`prefix`, `path`, `safe_regex`, `path_separated_prefix`, `connect_matcher`, plus deprecated `regex`, plus the never-set `path_match_policy` extension point) and side fields (`headers[]`, `query_parameters[]`, `dynamic_metadata[]`, `runtime_fraction`, `case_sensitive`, `tls_context`, `grpc`). Phase 04's fixture exercises exactly two predicates: an exact-path route for `/health` and a prefix route for `/api`. Implementing the full match surface is at least one phase of work and pulls in a regex engine, segment parser, header-match grammar, and runtime-substitution machinery — all out of phase-04 scope per SPEC §2.

### Decision

Phase 04 supports exactly two match predicates:

- `match.path` (`*routev3.RouteMatch_Path`) — case-sensitive exact comparison on `req.URL.Path`.
- `match.prefix` (`*routev3.RouteMatch_Prefix`) — bytewise prefix match on `req.URL.Path`.

`match.case_sensitive` is honoured only as the Envoy default (`true` or unset/nil pointer); explicitly setting `case_sensitive: false` errors at parse with `hcm: route %d: match.case_sensitive=false is not supported in phase 04`.

Every other `path_specifier` variant errors at parse: `safe_regex`, `path_separated_prefix`, `connect_matcher`, the deprecated `regex`, and any future variant the proto adds. Side fields error: `headers[]` non-empty, `query_parameters[]` non-empty, `dynamic_metadata[]` non-empty, `runtime_fraction` set, `tls_context` set, `grpc` set.

### Documented divergence

Envoy's `match.prefix` is path-segment-aware: `prefix: "/api"` matches `/api`, `/api/`, `/api/x` but NOT `/apifoo`. Phase 04 implements bytewise prefix: `/apifoo` WOULD match. The phase-04 fixture driver does not exercise non-segment-boundary paths; every router-action request uses `/api/v1/<n>`. Therefore:

- The differential gate does not exercise the divergence.
- A future phase that fixes the divergence (by introducing segment-aware prefix matching) does not need to supersede this ADR — it simply tightens the implementation while keeping the proto-level surface (`match.prefix`) the same. ADR-0038's "permitted predicates" list does not change.
- A future fixture that DOES exercise non-segment-boundary paths must either rely on the segment-aware tightening or extend BEHAVIOR_CONTRACT with a fixture-specific assertion.

### Consequences

- The phase-04 ignored-set (ADR-0041) does NOT include `match.case_sensitive` because phase 04 explicitly errors on `case_sensitive: false`.
- BEHAVIOR_CONTRACT's HTTP/1.1 subsection (ADR-0044) records "route-match selection equivalence" as an asserted dimension; the divergence above is permitted because the asserted surface is path-equivalence on segment-boundary inputs.
- Phase 07's filter-chain framework + HTTP-filter family supersede this ADR's "what predicates are supported" list (not the underlying ADR; phase 07's ADR records the expanded predicate set + tightened semantics).

Lands in Task 4 (first use site of `routeTable.match`).

---

## ADR-0039: Per-request fresh upstream dial in phase-04 router

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.3, D-3.5
**Settles:** SPEC ADR-L, phase-04 §4.1 actions.go

### Context

The router action in phase 04 must move bytes from the downstream request to a selected upstream endpoint and back. Two upstream-connection strategies are possible: (a) per-request fresh dial — every routed request opens a new TCP connection via `Cluster.Dial(ctx)` and closes it after the response is written; (b) connection pooling — keep a per-endpoint pool of upstream connections, idle-evict, max-streams, etc. Upstream Envoy uses (b). Implementing (b) faithfully is at least one phase of upstream-robustness work (timeouts, idle eviction, max-streams, idle-stream-cleanup, max-concurrent-streams-per-connection, draining-on-shutdown).

### Decision

Phase 04 picks (a) — per-request fresh dial. The router action calls `cluster.Dial(ctx)` on every request, defers `upstream.Close()`, and lets the connection close after the response is fully written.

### Consequences

- Performance is suboptimal vs upstream Envoy (extra TCP handshake per request, no upstream-side keep-alive). The differential gate does not assert connection-reuse, so the divergence is permitted.
- Per-request `cluster.Dial` is what makes the round-robin distribution `[3,3,3]` deterministic on the subject side: every request takes one endpoint pick from the cluster's RR state, mod-3 partition over 9 requests. A pooled implementation would need different distribution-witness arithmetic.
- Pool semantics land in the upstream-robustness family. That phase's ADR supersedes this one for the dial-strategy choice.
- BEHAVIOR_CONTRACT's HTTP/1.1 subsection (ADR-0044) explicitly enumerates "upstream connection re-use" under "Not asserted" so future phases can change the strategy without breaking the contract.

Lands in Task 5 (first use site of `routerAction.do`).

---

## ADR-0040: Phase-04 HTTP-filter framework subset

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-I, phase-04 §4.1 config.go (http_filters validation)

### Context

Envoy's HCM consumes a chain of HTTP filters: each filter is a proto with a name + typed_config; at runtime each filter is invoked through the iteration protocol (decode-headers, decode-data, decode-trailers, encode-headers, encode-data, encode-trailers, with stop/continue/buffer iteration directives). Implementing the full filter framework is at least one phase of work and pulls in stream buffering, stop-iteration semantics, and a full HTTP-filter SDK surface.

### Decision

Phase 04 permits exactly one HTTP filter, named `envoy.filters.http.router` with `typed_config.type_url == "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"`. The Router proto's body is unmarshalled but every Router-proto field is silently ignored (`dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`).

The filter-iteration protocol is NOT introduced. Instead, the router is invoked by direct function call inside the HCM connection loop: `entry.action.do(ctx, req, bw)` where `entry.action` is a `routerAction`. There is no `decode_headers`, no `Continue`/`StopIteration`, no per-filter buffering.

### Consequences

- The phase-04 HCM is a degenerate filter framework by construction: the chain has exactly one entry, the iteration protocol is absent, and the router is the only filter that can run.
- Phase 07's filter-chain framework supersedes this ADR with the actual iteration protocol + multi-filter chain support. ADR-0040 records the chosen subset; the supersession is total (not partial).
- Router-proto fields silently ignored at phase 04 may be moved to "honoured" by future ADRs; each such promotion lands in the phase that consumes the field, not in this ADR.
- The router's "do everything in one call" shape is what makes the per-request fresh-dial in ADR-0039 expressible without a buffering layer between `decode_headers` and the upstream dial.

Lands in Task 7 (first use site of `parseFilter`'s `requireRouterOnlyHTTPFilters` validation). **Supersedes:** none — phase 04 is the first HCM phase.

---

## ADR-0041: HCM `stat_prefix` + silently-ignored field set

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-N, SPEC §10 #9, phase-04 §4.1 config.go

### Context

The `HttpConnectionManager` proto carries dozens of fields, only a small subset of which phase 04 exercises. Every field falls into exactly one of three categories: REQUIRED (must be set, validated), ERRORED (set means hard fail), or SILENTLY IGNORED (set is accepted but no behaviour change). The category for each field needs to be ADR'd because future phases may move members between categories.

### Decision

REQUIRED fields:

- `codec_type` (must be `HTTP1` or `AUTO`).
- `stat_prefix` (must be non-empty string; stored on `Filter` for forward use; phase 06 stats consumer; settles SPEC §10 #9 to `string` field).
- `route_specifier` (must be `route_config`).
- `http_filters` (must be exactly one `[router]` per ADR-0042).

ERRORED fields:

- `route_specifier=Rds`, `route_specifier=ScopedRoutes`, `route_specifier=ScopedRds`.
- `codec_type=HTTP2`, `codec_type=HTTP3`.

Every other top-level HCM proto field is SILENTLY IGNORED. The phase-04 ignored-set is enumerated at the proto-package field set in v1.32.4: `tracing`, `access_log[]`, `http_protocol_options`, `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`, `idle_timeout`, `max_request_headers_kb`, `request_headers_kb_limit`, `add_user_agent`, `set_current_client_cert_details`, `mutex_tracing`, `proxy_status_config`, `early_header_mutation_extensions`, `header_validation_config`, `append_local_overload`, `pass_through_is_optional`, `request_block_size`, `strip_matching_host_port`, `strip_any_host_port`, `strip_trailing_host_dot`, `add_proxy_protocol_connection_state`.

Route-level silently-ignored: `request_headers_to_add`, `request_headers_to_remove`, `response_headers_to_add`, `response_headers_to_remove`, `metadata`, `decorator`, `tracing`, `per_request_buffer_limit_bytes`.

### Consequences

- Phase-04 fixtures may inherit upstream-Envoy bootstraps that include any of the above fields without scrubbing — config.go silently ignores them. Matches Envoy's forward-compatible posture on irrelevant-to-the-asserted-surface fields.
- Phase 06+ may move members from "ignored" to "honoured" with a superseding ADR landed in the same commit as the new behaviour.
- Phase 07's filter-chain framework partially supersedes the `http_filters` REQUIRED rule (allowing >1 entry); the remainder of ADR-0041 stays.

Rationale for silent-ignore (vs error): the alternative — erroring on every unknown-but-present field — would force fixture authors to scrub upstream-Envoy bootstraps to phase-04's exact field set, which is brittle, high-friction, and surfaces no real misconfiguration.

Lands in Task 7 alongside ADR-0040. **Supersedes:** none.

---

## ADR-0042: Phase-04 HTTP-filter chain shape — exactly `[router]`

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-O, phase-04 §4.1 config.go (http_filters validation)

### Context

Envoy's HCM has an HTTP-filter chain (`http_filters[]` field) separate from the network-filter chain (which phase 02's ADR-0033 covers). Phase-04 exercises only the router action; no other HTTP filter is consumed.

### Decision

`http_filters[]` must be exactly one entry, named `envoy.filters.http.router` with the Router proto type_url. `http_filters` empty, `http_filters` with two entries (even if both router), or `http_filters[0]` named/typed differently — all error at build with `hcm: http_filters: ...`. `typed_per_filter_config` on routes (per-route filter override) errors at build (SPEC §2).

### Consequences

- Phase-04's filter sub-domain is degenerate by construction. The router-only constraint is the smallest shape that makes "the router action runs" expressible.
- Phase 07's filter-chain framework supersedes this with the multi-filter shape + iteration protocol.
- ADR-0033 (network-filter chains, phase 02) and ADR-0042 (HTTP-filter chains, phase 04) share a "minimal chain shape" theme but address disjoint protocol layers.

Lands in Task 7 alongside ADR-0040 + ADR-0041. **Supersedes:** none — disjoint from ADR-0033's network-filter-chain coverage.

---

## ADR-0043: Fixture-driver `HTTPExpectations` extension

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.4, D-3.5
**Settles:** SPEC ADR-M, SPEC §10 #4, phase-04 §4.1 fixture interface

### Context

The phase-04 differential gate must assert per-request equivalence dimensions (status code, decoded body, header set) on top of the existing byte-stream comparison the runner does after `Drive*`. Two design paths existed: (a) internalize per-request comparison logic inside each HTTP fixture's driver (so each future HTTP fixture re-implements its own status/body/header diff), or (b) extend the `Driver` interface with an optional `HTTPExpectations()` method that lets the runner own the per-request orchestration.

### Decision

(b). Add an OPTIONAL interface `type HTTPExpectations interface { HTTPExpectations() []HTTPRequestExpectation }` and a struct `type HTTPRequestExpectation struct { Method, Path string; ExpectStatus int; ExpectBodyEquivalent bool }` to `test/differential/fixture/fixture.go`. The `Driver` interface itself is unchanged; the new interface is type-asserted at the runner per phase-02's `DistributionAsserter` precedent.

The runner's per-fixture orchestration loop gains a new branch: when `d.(fixture.HTTPExpectations)` succeeds, after byte-comparison and distribution assertion, the runner re-issues each expectation against ref and subject via `helpers.HTTPRoundTrip` and compares status + body (when `ExpectBodyEquivalent`) + header set (under `helpers.HTTPHeaderDiff` with `PhaseFourHTTPAllowList`).

### Consequences

- Existing TCP-only fixtures (0000, 0001, 0002) do NOT implement this interface; the runner's code path is gated on the type assertion, so they are unaffected. Backward compatibility is preserved by construction.
- Phase 05 (HTTP/2) will reuse the same struct + interface shape — phase 05's helpers will issue HTTP/2 round-trips via a different helper while populating the same `HTTPRequestExpectation` struct. The amortization across phases 04 and 05 justifies the typed-extension cost over per-driver duplication.
- The `HTTPExpectations` extension is informally superseding the implicit "byte-comparison is the only assertion" contract of the runner; that contract was never ADR'd, so the supersession header on this ADR is informal (mirroring ADR-0034's informal qualifier on the `Drive` split).

---

## ADR-0044: BEHAVIOR_CONTRACT HTTP/1.1 subsection

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.4, D-3.5
**Settles:** SPEC ADR-K, phase-04 §4.1 BEHAVIOR_CONTRACT.md

### Context

Phase 04 introduces HTTP/1.1 routing. The differential gate against upstream Envoy needs an explicit codification of which equivalence dimensions are asserted (and which are intentionally relaxed) so that future fixture authors and reviewers can reason about the gate's scope without re-deriving it from code.

### Decision

Add a `## HTTP/1.1` subsection to `docs/envoy-go/BEHAVIOR_CONTRACT.md` enumerating Asserted equivalence, Not asserted, Header allow-list extensions, Applies to, and Does not yet apply to. Extend the `## Header allow-list` table with six new rows: `Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`.

**Asserted equivalence:**

- Response status code per request.
- Decoded response body bytes for `direct_response` 2xx paths.
- Route-match selection (same method + path → same matched route on both proxies), witnessed by per-cluster RR distribution `[3,3,3]` over the router-action subset.
- Upstream-side request preservation (verbatim Host, method, path-with-query, body — modulo stdlib HTTP/1.1 parsing's documented normalisation per ADR-0037).

**Not asserted:**

- Decoded response body bytes for routed-to-upstream requests. The reference (STRICT_DNS) and subject (STATIC) round-robin LBs may start at different endpoint indices; both maintain `[3,3,3]` overall but request[i] may hit a different backend on each side. Status + distribution are the witnesses.
- Local-reply body bytes for 4xx/5xx (envoy-go uses plain text; Envoy uses HTML/JSON).
- Response-header **value** equality (set-equality modulo allow-list).
- `Content-Length` vs `Transfer-Encoding: chunked` framing per response.
- Upstream connection re-use (envoy-go does not pool — ADR-0039).
- `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (allow-listed).

**Header allow-list extensions:** `Server` (presence-only), `Content-Length` and `Transfer-Encoding` (framing-divergence-permitted), `x-envoy-*` / `x-forwarded-*` / `x-request-id` (presence-not-required on subject).

**Applies to:** phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture `0003-http11-routing`. The phase-04 HCM-filter chain shape `[router]` (ADR-0042). `match.prefix` (bytewise) and `match.path` (case-sensitive exact) only.

**Does not yet apply to:** HTTP/2 (phase 05); HTTP/3 (later); HCM filter chain beyond `[router]` (phase 07); upstream connection pooling (upstream-robustness family); HTTPS (phase 04.x or 05.x); `match.regex` / `match.path_separated_prefix` / `match.connect_matcher` / header-aware match / query-parameter-aware match (subset enforcement per ADR-0038); HTTP-filter iteration protocol (phase 07).

### Consequences

- Future HTTP fixtures inherit the equivalence dimensions enumerated above. New dimensions (e.g., per-request body equivalence under synchronised RR) need superseding ADRs landing in their phase.
- The "decoded body bytes for routed-to-upstream" relaxation is a phase-04 limitation, not a permanent architectural choice. A future phase that synchronises RR start indices across STATIC and STRICT_DNS (e.g., by seeding both proxies' LBs from the same deterministic source) may tighten this back to "asserted" with a superseding ADR.
- Phase 05 (HTTP/2) will reuse the same `## HTTP/1.1` subsection's structure for its `## HTTP/2` subsection. The header allow-list is shared across HTTP versions.

Lands in Task 17. **Supersedes:** none — first phase to assert HTTP/1.1 equivalence.

---

## ADR-0045: Split phase 05 into 05.1 (downstream H2 + h2spec) + 05.2 (upstream H2 + fixture 0004)

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5, D-3.6
**Settles:** phase-05 SPEC §11.1 split-decision deferral; `BOOTSTRAP_PROMPT.md` §6.1 plan-size gate

### Context

Phase 05's `superpowers:writing-plans` session opened on the SPEC at `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea`). Per `BOOTSTRAP_PROMPT.md` §5 state 2, the planner must apply the split gate ("if `PLAN.md` > ~25 tasks OR > ~1500 LoC estimated → split into NN.1, NN.2, …; update ROADMAP + STATE; stop") before writing the plan. Phase-05 SPEC §11.1 explicitly anticipated the gate would trip ("Phase 05 is the largest phase to date — the H2 codec alone is ~1500 LoC of state-machine plumbing, plus the cluster-side dial helper, plus the fixture, plus h2spec integration") and enumerated three plausible split axes with a recommended axis ("split by surface").

The planner's own task-count + LoC estimate, derived from SPEC §4 + §11.1 + a TDD red/green/commit cycle per file, is:

- **Codec sub-package** (`internal/filter/hcm/h2/`): 10 source files (`errors`, `preface`, `framer`, `hpack`, `settings`, `flow`, `stream`, `conn`, `client`, `h2_test` + `fuzz_test`); ~12–15 TDD tasks; ~1500 LoC per SPEC §11.1's own estimate.
- **HCM integration**: `config.go` + `filter.go` (ALPN dispatch) + `actions.go` (`routerActionH2`) + `listener/manager.go` (listenerCtx) + `bootstrap.go` blank import; ~5 tasks; ~250 LoC.
- **Cluster integration**: `cluster.go` (`UseH2()`) + `manager.go` (`HttpProtocolOptions` parsing + validation) + `dial_h2.go` (DialH2); ~3 tasks; ~150 LoC.
- **Conformance suite**: `--allow-h2c` flag wiring + `test/conformance/h2spec/h2spec.go` + `h2spec_test.go` + `CONFORMANCE_PINS.md`; ~4 tasks; ~200 LoC.
- **Fixture 0004**: PKI gen + backend + YAMLs + driver + helpers/h2 + runner wiring; ~6 tasks; ~850 LoC (PKI gen ~200, backend ~50, driver ~250, YAMLs ~250, expectations ~50, helpers ~100, runner glue ~50).
- **Cross-cutting docs**: `BEHAVIOR_CONTRACT.md ## HTTP/2` subsection + 10 ADRs (ADR-P..ADR-Z); ~3–4 tasks; ~500 LoC.

**Total: ~33–40 tasks (gate threshold 25); ~3450 LoC (gate threshold 1500). Both legs of the gate trip.** Per `BOOTSTRAP_PROMPT.md` §6.1 the planner must stop, split, and exit; per §6.3 the only release valve is splitting (not deferral via TODO/stub tasks).

### Decision

Phase 05 is split into two sequential sub-phases on the **split-by-surface axis** recommended by SPEC §11.1:

- **Phase 05.1 — `downstream-h2`** — depends-on `04`; status `planned`. Scope: full server-side H2 codec sub-package under `internal/filter/hcm/h2/` (`errors.go`, `preface.go`, `framer.go`, `hpack.go`, `settings.go`, `flow.go`, `stream.go`, `conn.go` — but NOT `client.go`); HCM ALPN dispatch (`internal/filter/hcm/filter.go`); `codec_type: HTTP2` permitted (`internal/filter/hcm/config.go`); `listenerCtx` plumbing (`internal/listener/manager.go`); codec-neutral `directResponseAction.body()` + `writeH2` adapter (per phase-05 SPEC §5.5) — required in 05.1 because the h2spec gate exercises `direct_response`; `cmd/envoy-go --allow-h2c` test-only flag; `test/conformance/h2spec/`; NEW `docs/envoy-go/CONFORMANCE_PINS.md`; `BEHAVIOR_CONTRACT.md ## HTTP/2` SUBSECTION SCAFFOLD; fuzz targets `FuzzFrameStream` + `FuzzHPACKDecode`; phase-04 REVIEW Minor carry-forward triage (per phase-05 SPEC §12 + ADR-X). Differential surface at end of 05.1: gate (a) is vacuously green (no new fixture); gate (b) pre-existing fixtures still green; gate (c) **non-vacuous for the first time in the project** — h2spec runs against a `--allow-h2c` h2c listener and reports `failed == 0` over the threshold sections per ADR-U.

- **Phase 05.2 — `upstream-h2`** — depends-on `05.1`; status `planned`. Scope: `internal/filter/hcm/h2/client.go` (from-scratch `ClientConn` + `RoundTrip`); `internal/cluster/dial_h2.go` (`Cluster.DialH2`); `Cluster.UseH2()` accessor; `internal/cluster/manager.go` `HttpProtocolOptions` parsing + validation (per phase-05 SPEC §5.8); blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"`; `routerActionH2` variant in `internal/filter/hcm/actions.go`; full fixture `test/fixtures/0004-h2-routing/` (PKI gen + backends + driver + envoy.yaml + envoy-go.yaml + expectations.yaml + README); `test/helpers/h2.go`; `test/differential/runner_test.go` blank-import for the new fixture; extension of `BEHAVIOR_CONTRACT.md ## HTTP/2` with the upstream-H2 + fixture-0004 differential rules (header allow-list extensions for H2 pseudo-headers; closes ADR-0035 H2 leg). Differential surface at end of 05.2: NEW fixture `0004-h2-routing` differentially green (gate a); pre-existing fixtures still green (gate b); conformance still green (gate c).

The two sub-phases are sequential because both touch the same Go package (`internal/filter/hcm/h2/`) — the server-side codec internals are 05.1's deliverable and 05.2's foundation, so 05.1 must reach `done` before 05.2 brainstorming opens.

The phase-05 SPEC at `docs/envoy-go/phases/05-http-2/SPEC.md` (`612cdea`) **remains the master design document** for both sub-phases. Sub-phase SPECs (drafted by `superpowers:brainstorming` per ADR-0004 in the next two brainstorming sessions) carve coherent slices of the master SPEC's §4 deliverables; they do not replace or supersede it.

### Rationale

- **SPEC §11.1's authoring** already enumerated three split axes and recommended split-by-surface as the strongest. The planner concurs: split-by-surface keeps each sub-phase coherent (one codec direction per sub-phase), maintains an honest phase-done gate set per sub-phase (05.1 has gate (c) non-vacuous; 05.2 has gate (a) non-vacuous), and minimises cross-sub-phase coupling (the only coupling is at the codec sub-package's package boundary, which is type-only).
- **Split-by-transport (option 2 in SPEC §11.1)** would put h2spec under 05.1 with no differential fixture (gate a vacuously green) — but so does split-by-surface. The transport split's downside is that it splits the codec by transport (TLS vs h2c) rather than by direction (server vs client), which means the same code paths land twice across sub-phases (once for h2c, once for HTTPS). Split-by-surface keeps each codec-direction file in exactly one sub-phase.
- **Split-by-ends (option 3 in SPEC §11.1)** would put fixture 0004 under 05.1 with H1 upstream backends (h2 → h1 router), shipping a degenerate fixture that doesn't close ADR-0035. Strictly worse than split-by-surface.
- **No-split** is unavailable: BOOTSTRAP §6.3 forbids cramming work into "TODO: extend later" tasks or incomplete stubs. The gate trip is structural, not optional.

### Consequences

- **ADR numbering shift.** Phase-05 SPEC §4.4 anticipated ADR-P..ADR-Z (10 ADRs) landing at numbers ADR-0045..ADR-0054. ADR-0045 is consumed by this split-decision ADR, so the lettered placeholders shift by one and land at **ADR-0046..ADR-0055** when their respective sub-phase planners write them. The mapping is (per the split-time scope assignment): ADR-P/Q/S/T/U/V/Z/X under 05.1 (8 ADRs); ADR-R/W/Y under 05.2 (3 ADRs). Total 11, not 10 — the split itself is the 11th ADR (ADR-0045). The SPEC's 10-ADR estimate stands; this ADR is the meta-decision wrapper.
- **ROADMAP:** row `05` stays `in-progress` with `sub-phases = 05.1, 05.2`. Two new rows: `05.1` (depends-on `04`, planned) and `05.2` (depends-on `05.1`, planned). Per ROADMAP schema invariants, sub-phase rows are append-only and the parent row is never deleted; phase-05 reaches `done` when both 05.1 and 05.2 are `done`.
- **Sub-phase directories:** `docs/envoy-go/phases/05.1-downstream-h2/` and `docs/envoy-go/phases/05.2-upstream-h2/` are created in this commit per `BOOTSTRAP_PROMPT.md` §6.2 step 2. Each holds a single `README.md` placeholder that names the master SPEC and enumerates the sub-phase's narrowed scope; the proper `SPEC.md` is drafted by `superpowers:brainstorming` per ADR-0004 in the next two sessions.
- **Phase-05 directory:** `docs/envoy-go/phases/05-http-2/` retains its `SPEC.md` (`612cdea`) as the master design document. No `PLAN.md`, `PROGRESS.md`, or `REVIEW.md` will land under that directory — those land under the sub-phase directories. The phase-05 directory becomes closed read-only history at the parent level once both sub-phases reach `done`.
- **Worktrees:** the closing `phase/05-http-2-plan` worktree (this session) leaves no PLAN.md per the §6.2 short-circuit. The next two brainstorming sessions branch fresh `phase/05.1-downstream-h2-spec` and (later, after 05.1 completes) `phase/05.2-upstream-h2-spec` worktrees from master tip per ADR-0003 + the project's per-phase-worktree convention.
- **Scope of `--allow-h2c` flag (ADR-Z):** the flag lands in 05.1 because 05.1's h2spec gate requires it. 05.2 inherits the flag without re-deciding it; 05.2's fixture 0004 uses HTTPS h2 (real ALPN) and does not set `--allow-h2c`.
- **Scope of `directResponseAction.body()` codec-neutral factoring (phase-05 SPEC §5.5):** lands in 05.1 because h2spec's threshold sections include section 8 (HTTP Message Exchanges) which exercises basic request-response shapes — `direct_response` is the simplest such shape and h2spec's conformance behaviour against it is sensitive to the response shape. The H1 adapter (`writeH1`) keeps its phase-04 behaviour; the H2 adapter (`writeH2`) is new in 05.1.
- **Scope of phase-04 REVIEW carry-forward triage (phase-05 SPEC §12 + ADR-X):** lands in 05.1 because the dispositions (M-2/M-4/M-5/M-6/M-7) are textual / cosmetic + a forward-looking "phase-06-must-consume" tag; none of them touches upstream-H2 surface, so 05.1 is the natural landing point. 05.2's planner does not re-disposition.
- **Per-cluster RR counter scope (phase-05 SPEC §10 #7):** the question only surfaces in 05.2 (the only sub-phase that introduces an H2-using cluster). 05.1's brainstorming need not address it; 05.2's brainstorming records the choice in its SPEC.
- **Streaming-body-vs-wait-for-END_STREAM dispatch (phase-05 SPEC §10 #1):** decided in 05.1 (the dispatch decision lives in `serverStream.dispatch` per phase-05 SPEC §5.2 step 4). SPEC prescribes wait-for-END_STREAM; 05.1's planner records the choice.
- **`H2Request`/`H2Response` shape vs stdlib re-use (phase-05 SPEC §10 #3):** decided per-direction. The SERVER-side request/response types live in 05.1 (used by `serverStream.dispatch` → action interface); the CLIENT-side request/response types live in 05.2 (used by `ClientConn.RoundTrip`). SPEC's recommendation (stdlib for action surface, internal types for codec) is followed in both sub-phases.
- **`routerActionH2`/`routerAction` interface vs concrete switch (phase-05 SPEC §10 #6):** the question only surfaces in 05.2 (`routerActionH2` is 05.2's deliverable). 05.1 keeps the phase-04 small-interface shape unchanged; 05.2's brainstorming records whether to flatten or keep.
- **`expectations.yaml` structured vs heredoc for fixture 0004 (phase-05 SPEC §10 #10):** 05.2 question; the SPEC prescribes heredoc (per phase-04 M-6 carry-forward); 05.2's planner may overrule.
- **Phase-05 SPEC's gates (a)–(f) per §3** specialise per sub-phase: 05.1 inherits gates (b)/(c)/(d)/(e)/(f) with gate (a) vacuous; 05.2 inherits gates (a)/(b)/(c)/(d)/(e)/(f) all live (gate (c) inherits the threshold pinned by 05.1's `CONFORMANCE_PINS.md` + ADR-U; 05.2 does not move the threshold). The phase-05 §13 acceptance checklist is split across the two sub-phases' acceptance checklists (drafted at brainstorming time).

This ADR supersedes nothing. It is the project's first plan-time-split ADR; future plan-time splits follow the same shape (estimate-driven gate trip + axis selection + sub-phase scope assignment + ADR-numbering-shift acknowledgement).

### Cross-references

- **`BOOTSTRAP_PROMPT.md` §5 state 2 GATE** — the source-of-truth for the planner's split mandate.
- **`BOOTSTRAP_PROMPT.md` §6** — splitting policy (when, how, and the anti-pattern guard).
- **`docs/envoy-go/phases/05-http-2/SPEC.md` §11.1** — the SPEC's own anticipation of the gate trip and its three split-axis enumeration with the split-by-surface recommendation.
- **ADR-0004** — autonomous-brainstorming adaptation; the next two sub-phase brainstormings inherit this rule.
- **ADR-0005** — autonomous-planning adaptation; the next two sub-phase planning sessions inherit this rule.
- **ADR-0003** — worktree pattern; the next two brainstorming sessions follow this pattern.
- **ADR-0035** — fixture-0002 differential scope; ADR-W (under 05.2) closes its H2 leg.
- **`docs/envoy-go/phases/05.1-downstream-h2/README.md`** + **`docs/envoy-go/phases/05.2-upstream-h2/README.md`** — placeholder scope docs created by this commit; superseded by each sub-phase's `SPEC.md` when brainstorming next runs.

## ADR-0046: HTTP/2 codec source — `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack`

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-P, phase-05.1 §4.1 codec sub-package.

### Context

Phase 05.1 introduces envoy-go's downstream HTTP/2 codec. `BOOTSTRAP_PROMPT.md` D-3.2 permits `golang.org/x/net/http2` "as a low-level codec only — never as a server runtime." The phase-05.1 SPEC §4.1 names two specific entry points within that package: `http2.Framer` (frame byte-layout serialisation) and `http2/hpack` (HPACK encoder/decoder with dynamic-table state). The decision to use them — vs handcrafting from RFC 9113 — needs explicit codification because the HPACK dynamic-table state machine has CVE history that argues against re-implementation.

### Decision

Phase 05.1's `internal/filter/hcm/h2/` codec sub-package consumes:

- `http2.Framer` for frame read/write — wrapped by `framer` in `framer.go` to add context-aware reads via `conn.SetReadDeadline` translation.
- `http2/hpack.Encoder` + `hpack.Decoder` for header block encode/decode — held per-connection in `hpackState` (hpack.go) so the dynamic-table state is per-conn.

Three runtime constructs in `golang.org/x/net/http2` are FORBIDDEN even at phase 05.1: `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`. They carry their own request-routing, header-canonicalization, response-header injection, and error policies that diverge from Envoy's; envoy-go's connection lifecycle (preface check, settings handshake, frame dispatch, GOAWAY emission, RST_STREAM/PING semantics) is owned by `ServerConn` (conn.go) and `serverStream` (stream.go) — both written from scratch. ADR-0048 codifies the from-scratch-server-connection-manager decision.

Driver-side test use of `x/net/http2.Transport` (in `cmd/envoy-go/main_test.go` H2 smoke variant and `internal/filter/hcm/h2/conn_test.go` end-to-end tests) is permitted because that is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code.

### Consequences

- The boundary is grep-verifiable: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go` (excluding `_test.go`) returns zero hits OUTSIDE `internal/filter/hcm/h2/framer.go`/`hpack.go`/`settings.go` — the three files that legitimately import the package. Task 16's gate-sweep verifies this.
- `golang.org/x/net` is promoted from an indirect dependency (transitively held via go-control-plane) to a direct dependency. No new module SHA — the same version go-control-plane already pins.
- A future codec-related ADR (e.g., a phase-09 HTTP/3 ADR for `quic-go`) follows this same shape: low-level codec only, with the runtime owned by envoy-go. ADR-0046 is the template for that pattern.
- The three FORBIDDEN runtime types do not carry through to test code; tests may use `http2.Transport` as a peer driver. The boundary is in the package import graph (test files vs production files) and in CI lint rules (the grep gate above).

This ADR supersedes nothing.

## ADR-0047: Phase-05.1 H2 server settings defaults

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-S; phase-05.1 §4.1 / §5 / §11 (settings handshake).
**Amends:** ADR-0041 (HCM silent-ignore set extended with `http2_protocol_options`).

### Context

Phase 05.1 needs concrete numeric values for every SETTINGS the server announces in its initial SETTINGS frame. RFC 9113 §6.5.2 defines defaults for the standard settings; envoy-go matches Envoy's documented `Http2ProtocolOptions` defaults where they diverge from RFC defaults, and matches RFC defaults where Envoy doesn't override.

### Decision

Phase 05.1 hardcodes the following ServerSettings (in `internal/filter/hcm/h2/settings.go`):

- **MAX_CONCURRENT_STREAMS = 100.** Envoy's documented default for `max_concurrent_streams`. The 101st concurrent stream from the client → REFUSED_STREAM (RFC 9113 §5.1.2).
- **INITIAL_WINDOW_SIZE = 65535.** RFC 9113 §6.9.2 protocol default. Envoy does not override.
- **MAX_FRAME_SIZE = 16384.** RFC 9113 §6.5.2 protocol default. Envoy does not override.
- **ENABLE_PUSH = 0.** Phase 05.1 disables server push entirely (SPEC §2.1). Disabling on our SETTINGS prevents the client from sending PUSH_PROMISE either.
- **SETTINGS_NO_RFC7540_PRIORITIES = 1.** Informs the client we discard PRIORITY frames (RFC 9113 §6.3 / SPEC §2.1). RFC 9218 (`SETTINGS_NO_RFC7540_PRIORITIES`) numeric ID 0x9.
- **HEADER_TABLE_SIZE = 4096.** RFC 9113 §6.5.2 protocol default + Envoy default. We do not advertise a different value; the encoder side at the peer is also 4096 unless the peer changes it via its own SETTINGS.

### HCM `http2_protocol_options` silent-ignore amendment

ADR-0041 (phase-04 HCM silent-ignore set) is amended to add the directly-on-HCM `http2_protocol_options` field. Phase 05.1 reads this field via the unmarshalled HCM proto but does NOT honour any sub-field — the values stay at the ADR-0047 defaults regardless of what the bootstrap declares. Future phases (06+) may move members from "ignored" to "honoured" via a superseding ADR. The cluster-side `HttpProtocolOptions` typed-extension is 05.2's surface and remains in the phase-04 silent-ignore set in 05.1.

### Consequences

- Differential equivalence: the gate does not assert SETTINGS values byte-for-byte (those are inside the structurally-equivalent framing rule per ADR-0052). h2spec section 6.5 only validates RFC 9113 compliance, not Envoy-specific values — the threshold accepts the values above.
- The `ServerSettings` value type is exported so future phases (or test fixtures) can vary the values per construction; the `DefaultServerSettings` global is the project-wide canonical instance.
- A future phase that needs configurable per-listener SETTINGS (e.g., to honour `http2_protocol_options.max_concurrent_streams`) supersedes ADR-0047 + ADR-0041's silent-ignore amendment with a new ADR.

This ADR supersedes nothing on its own; ADR-0041 is amended (not superseded) per the additive shape of the silent-ignore set.

---

## ADR-0048: HCM H2 server connection manager from scratch

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-Q; phase-05.1 §4.1 / §5.2 / §10 #1.

### Context

`golang.org/x/net/http2` exposes `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, and `http2.Transport.NewClientConn`. These types ostensibly fit the "low-level codec only" framing because they live in the same package as `Framer` and `hpack` — but they are RUNTIMES, not codecs. They carry per-request routing, header canonicalization, response-header injection, error policies, and timeout machinery that diverge from Envoy's behaviour. ADR-0046 explicitly forbids using them.

But "don't use the runtimes" is one half of the decision. The other half: build the runtime ourselves. ADR-0048 codifies the from-scratch decision and the architectural shape.

### Decision

Phase 05.1's `internal/filter/hcm/h2/` sub-package implements:

- **`ServerConn` (conn.go)** — per-downstream-conn state machine. One `ServerConn` value owns one downstream `net.Conn` after ALPN selects "h2" (or after the `--allow-h2c` h2c path bypasses TLS). `Run()` performs the connection preface read + server-initial SETTINGS + client-initial SETTINGS exchange, then enters the frame-dispatch loop. Connection-level errors (bad preface, malformed SETTINGS, HPACK COMPRESSION_ERROR, FRAME_SIZE_ERROR on a non-DATA frame, PUSH_PROMISE received from client, stream-id reuse, even-numbered client stream id) emit GOAWAY with the appropriate code and close.

- **`serverStream` (stream.go)** — per-stream state machine implementing RFC 9113 §5.1: idle → open → half-closed (remote/local) → closed. Server-side stream IDs are odd-numbered client-initiated; even-numbered IDs from the client → PROTOCOL_ERROR. Stream-id reuse → PROTOCOL_ERROR. The dispatch helper waits for END_STREAM-on-headers OR END_STREAM-on-data before invoking the matched action (SPEC §10 #1 settled to wait-for-END_STREAM).

- **No `client.go` in 05.1.** The from-scratch `ClientConn` + `RoundTrip` is 05.2's deliverable per ADR-0045. The h2 sub-package compiles and is unit-tested in 05.1 with server-side surfaces only.

### Consequences

- The discipline is grep-verifiable: `! ls internal/filter/hcm/h2/client.go` (the file does not exist) is part of the 05.1 acceptance check (SPEC §13). Task 16's gate sweep verifies.
- A `routerAction` matched on the H2 path (theoretically possible via misconfiguration but unreachable in 05.1's production bootstraps per SPEC §5.2 step 4c) produces a per-stream INTERNAL_ERROR + RST_STREAM at runtime — the protective shape. Build-time enforcement of "no `routerAction` on H2 listener" is deferred to 05.2 because `Cluster.UseH2()` does not exist yet.
- The H2 connection manager is the project's first multi-stream concurrent state machine. The flow-control window helper (flow.go) is the synchronization primitive; the stream + conn mutexes are minimal and per-instance. SPEC §11.5 + §11.4 mitigations (tiny-window stress, HPACK table-size update propagation) are exercised in `flow_test.go` and `hpack_test.go`.

This ADR supersedes nothing.
