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
