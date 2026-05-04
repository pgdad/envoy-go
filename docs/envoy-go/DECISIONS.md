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

> **Phase 08.2 amendment (per ADR-0097):** ADR-0015 is **partially superseded** by ADR-0097 — the LIVE/PRE_INITIALIZING two-state coverage extends to LIVE/PRE_INITIALIZING/DRAINING three-state coverage. ADR-0015's verbatim pre-init body (`PRE_INITIALIZING\n`) and pre-init status (503) are preserved; ADR-0097 adds the DRAINING branch and the precedence rule (DRAINING > PRE_INITIALIZING > LIVE).

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

**06.2 amendment** (per ADR-0067): the silently-ignored set is extended to include:
- `envoy.access_loggers.stdout` (typed_config of HCM `access_log[]` entries)
- `envoy.access_loggers.tcp_grpc` (gRPC ALS)
- `envoy.access_loggers.open_telemetry` (OTLP)
- HCM `access_log[].filter` field (per-record predicate filter)
- HCM `access_log_options`
- Listener-scope `access_log[]`
- Cluster-scope `access_log[]`

Rejected explicitly (NOT silently-ignored — fatal parse error per ADR-0067):
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.log_format`
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.format_string`
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.json_format`

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

---

## ADR-0049: Test-only `--allow-h2c` CLI flag on `cmd/envoy-go`

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-Z; phase-05.1 §4.2 (cmd/envoy-go --allow-h2c) + §10 #5 (form decision).

### Context

Phase 05.1's gate (c) — `h2spec` conformance — must drive an HTTP/2 protocol-level test against the subject. h2spec's standard mode is h2c (cleartext HTTP/2 over plaintext TCP); h2spec's TLS mode requires a custom CA setup that complicates the conformance pin. envoy-go's HCM build-time validator otherwise rejects `codec_type: HTTP2` on plaintext listeners (no TLS handshake = no ALPN selection = no way to differentiate h2 from h1 at the listener level), so a runtime escape hatch is needed for the conformance suite to drive h2c against the subject.

### Decision

Add a test-only CLI flag `--allow-h2c` to `cmd/envoy-go/main.go`. Default OFF. When ON, the listener manager threads `listenerCtx{allowH2C: true}` into HCM filter construction; HCM's build-time validator accepts `codec_type: HTTP2` on plaintext listeners under this condition. The flag is documented in `--help` output as "test-only; not for production".

**Form: CLI flag** (vs env var, vs build tag). Rationale:
- The testcontainers driver (`test/conformance/h2spec/h2spec_test.go`) constructs the subject via `os/exec`; a CLI flag is the lowest-friction option for that driver. An env var would require setting + unsetting in the test's process environment; a build tag would require a separate test binary build.
- The flag is boolean (no value form). A value-bearing form was considered (e.g., `--allow-h2c=ports:8080,8081`) and rejected as over-engineered for a single use site. If a future phase needs per-listener gating, that's a superseding ADR.

The flag is plumbed through:

1. `cmd/envoy-go/main.go`: `flag.Bool("allow-h2c", false, ...)`.
2. `internal/listener/manager.NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, allowH2C bool)`: NEW constructor variant. Existing `NewManager` and `NewManagerWithBaseDir` delegate with `allowH2C=false`.
3. `listenerCtx{hasTLS, allowH2C}` per-chain value passed into the `filterRegistry` constructors.
4. `hcm.NewFilterWithCtx(tc, cm, hcm.ListenerCtx{HasTLS, AllowH2C})`: NEW HCM constructor variant. Existing `NewFilter` delegates with the zero-value `ListenerCtx{HasTLS:false, AllowH2C:false}`.
5. `parseFilterWithCtx` consults `lc.HasTLS` and `lc.AllowH2C` to validate `codec_type: HTTP2` per Task 12.

### Consequences

- The flag's runtime cost is one boolean field on `Filter` and one branch in `Filter.Handle` (under the `codec_type=HTTP2` AND plaintext path). Negligible.
- A future doctrine-cleanup phase MAY add a `//go:build !production` build tag to strip the flag entirely from production binaries. 05.1 does not pre-empt that decision — the flag's CI cost is low enough that the production strip is over-engineering at this stage.
- The flag is NOT advertised in `README.md`, `MISSION.md`, or any operator-facing surface other than `--help`. The discipline relies on the documentation discipline; future phases may add a CI-time grep to catch stray references.
- The h2-over-TLS production path is the default-supported configuration in 05.1; `--allow-h2c` does not change anything for that path. Phase 05.2's fixture 0004 uses HTTPS h2 (real ALPN) and does not set `--allow-h2c`.

This ADR supersedes nothing.

## ADR-0050: ALPN-driven codec selection inside `Filter.Handle`

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC ADR-V; phase-05.1 §4.2 / §5.4.

### Context

Phase 05.1 introduces a second HTTP codec on the same listener side. The selection between H1 and H2 happens in one of two places:

- **At the listener-side filter-chain match step**: ALPN becomes a `filter_chain_match.application_protocols[]` dimension. Each filter chain carries one codec; the listener manager picks the chain post-handshake based on the negotiated ALPN.
- **Inside `Filter.Handle`**: HCM accepts both codecs (`codec_type: AUTO`) and dispatches at runtime by reading `*tls.Conn.ConnectionState().NegotiatedProtocol`.

ADR-0033 (phase-03's filter-chain subset) explicitly limits filter-chain match to SNI; extending it to ALPN now would expand that subset and require a superseding ADR. Phase 07's filter-chain framework is the natural home for `application_protocols` chain matching.

### Decision

Phase 05.1 implements ALPN dispatch INSIDE `Filter.Handle`, not at the listener-side filter-chain match step. The dispatch logic:

1. Switch on `f.codecType` (parsed at build time from the HCM proto):
   - `HTTP1` → call phase-04's `runConnection` (H1 driver) unchanged.
   - `HTTP2` → call `runH2` which constructs an `h2.ServerConn` and runs it. Build-time validation (in `parseFilterWithCtx`) ensures `HTTP2` is only accepted on TLS listeners OR when `listenerCtx.AllowH2C` is set.
   - `AUTO` → if downstream is `*tls.Conn`, read `ConnectionState().NegotiatedProtocol`; on `"h2"` dispatch to `runH2`; otherwise (plaintext OR TLS-h1 OR TLS-empty-ALPN) dispatch to `runConnection`.

2. Defensive `tlsConn.HandshakeContext(ctx)` no-op call before reading `NegotiatedProtocol`. Idempotent for already-completed handshakes; if a future refactor removes the listener-side handshake, the HCM still gets correct data. SPEC §11.6 mitigation.

3. Listener-side `filter_chain_match.application_protocols[]` field — silently ignored (extends the phase-04 ignored set). ALPN is NOT a chain-match dimension at phase 05.1.

### Consequences

- ADR-0033's filter-chain subset is unchanged. Phase 07's filter-chain framework is the natural close for the `application_protocols` chain match.
- The `Filter.Handle` switch is small and grep-verifiable: one type-assert + one `NegotiatedProtocol` read + one branch on `"h2"`. Easy to review; easy to test.
- A misconfigured client speaking H1 against an `HTTP2`-only listener (rare; would require an h1 client and a server config explicitly forbidding h1) lands in the H2 driver and fails the preface check immediately, returning a connection-level error. Symmetrical to upstream Envoy's posture.
- Per-listener `codec_type: AUTO` is the recommended config for production; `codec_type: HTTP2` is for h2-only listeners (and requires TLS or `--allow-h2c`).
- Phase-05.2's `routerActionH2` will land on top of this same dispatch path; no changes to `Filter.Handle` are needed in 05.2.

This ADR supersedes nothing.

## ADR-0051: h2spec conformance gate — image pin + section exclusion policy

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.5
**Settles:** SPEC §9 (conformance gate), SPEC §11.3 (PUSH_PROMISE exclusion).

### Context

Phase 05.1 requires a non-vacuous conformance gate (`gate c`) that runs
`summerwind/h2spec` against the live envoy-go binary.  Two decisions must be
made:

1. **Which image version to pin**, and how to record the pin.
2. **Which h2spec sections to include in the threshold**, given that some RFC
   sections test behaviour envoy-go deliberately does not implement.

The PUSH_PROMISE section (RFC 9113 §6.6) tests server push.  Our server
unconditionally sets `SETTINGS_ENABLE_PUSH=0`; a client that subsequently sends
a PUSH_PROMISE frame is violating the protocol.  h2spec 6.6 tests the client's
reaction to server-push, which is irrelevant when the server has disabled push.
Running 6.6 would produce vacuous passes (h2spec would receive a
PROTOCOL_ERROR and consider it a test pass, not a real conformance signal).

### Decision

1. **Image pin**: The conformance gate uses
   `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`
   (pinned by digest, not by tag).  The pin is recorded in two places:
   - `test/conformance/h2spec/h2spec.go` (`h2specDigest` constant) — machine-readable
   - `docs/envoy-go/CONFORMANCE_PINS.md` — human-readable audit trail with pull date

2. **Section inclusion**: The gate runs sections `http2/3`, `http2/4`, `http2/5`,
   `http2/6/1–6/5`, `http2/6/7–6/10`, `http2/7`, `http2/8` (53 test cases).
   Section `http2/6/6` (PUSH_PROMISE) is excluded because `SETTINGS_ENABLE_PUSH=0`
   makes those test cases vacuous.

3. **Threshold**: `failures == 0` across all included sections.  The gate uses
   `-S` (strict mode) to include strict test cases.

4. **Infrastructure**: The gate boots a real envoy-go binary from a synthetic h2c
   bootstrap config, runs h2spec via `testcontainers-go`, and parses the JUnit XML
   report.  The test skips under `-short` and when Docker is unavailable.

### Consequences

- The pin guarantees that CI and local runs use identical test tooling.
- `CONFORMANCE_PINS.md` provides an audit trail; a future phase that upgrades
  h2spec appends a new row rather than editing in-place.
- Excluding 6.6 is a conscious choice, not an oversight.  Any future phase that
  implements server push MUST add 6.6 to `thresholdSections` and append a new
  ADR documenting the inclusion.
- The `-S` (strict) flag is intentional: it exercises additional test cases
  beyond the base RFC that were not covered by the non-strict run.

This ADR supersedes nothing.

## ADR-0052: BEHAVIOR_CONTRACT `## HTTP/2` subsection (SCAFFOLD form for 05.1)

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.4, D-3.5
**Settles:** SPEC ADR-T; phase-05.1 §5.7 (BEHAVIOR_CONTRACT scope).

### Context

Phase 05.1 delivers envoy-go's downstream HTTP/2 codec and the project's first non-vacuous conformance gate. The equivalence surface — what the project now asserts, and what it deliberately defers — must be codified in `BEHAVIOR_CONTRACT.md` so that subsequent phases, reviewers, and the verification-before-completion session can audit the boundary. Without this codification, the 05.1 gate sweep is a series of green lights without a documented contract; the gate would be correct but unauditable.

SPEC §5.7 mandates this subsection land in Task 16 (the closing task) alongside the final all-gates green sweep — same commit so the contract and the evidence land atomically.

This subsection is written in SCAFFOLD form: the 05.1 scope draws the equivalence boundary at locally-generated H2 responses via `direct_response` (h2spec-verified) and explicitly defers the routed-to-upstream H2 surface to phase 05.2.

### Decision

The `## HTTP/2` subsection is added to `docs/envoy-go/BEHAVIOR_CONTRACT.md` immediately after `## HTTP/1.1`. The subsection contains:

1. **Asserted equivalence (05.1 scope):**
   - `:status` per request: required + asserted by h2spec section 8 on every `direct_response` invocation.
   - Decoded body bytes on `direct_response` 2xx paths: byte-equal to the configured `body` string (h2spec validates indirectly via response-length + END_STREAM checks; envoy-go's unit tests assert byte equality directly).
   - Per-stream response header set-equality modulo allow-list: locally-generated H2 responses carry `:status`/`Server`/`Content-Type`/`Content-Length`/`Date`. Routed-to-upstream H2 surface NOT YET ASSERTED IN 05.1 (deferred to 05.2 + fixture 0004).

2. **Not asserted (05.1 scope):**
   - Wire-byte H2 framing; SETTINGS values byte-for-byte; WINDOW_UPDATE timing or count; stream id allocation pattern; trailers; 0-RTT TLS early data; routed-to-upstream H2 request preservation / decoded body / per-cluster RR / ALPN selection equivalence at the differential level — ALL DEFERRED TO 05.2.

3. **Header allow-list extensions:**
   - `:status` (active in 05.1; locally-generated H2 responses).
   - `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to: 05.2 routed-to-upstream H2). The 05.1 scaffold inserts these rows so 05.2's brainstorming has nothing to add to the table itself; only the "applies-to" cells flip when fixture 0004 lands.

4. **h2spec threshold:** Sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin via `CONFORMANCE_PINS.md` per ADR-0051.

**Applies to (05.1):**
- Phase-05.1 `internal/filter/hcm/h2/` package (server-side only).
- The codec-neutral `directResponseAction` factoring in `internal/filter/hcm/actions.go`.
- The conformance suite under `test/conformance/h2spec/`.

**Does not yet apply to:**
- Routed-to-upstream H2 (05.2 + fixture 0004 — closes ADR-0035 H2 leg).
- HTTP/3, server push, gRPC framing, trailer forwarding, upstream H2 stream pooling, h2c production fixtures, mTLS over h2.

**SCAFFOLD-pattern in-place-edit authorisation:** ADR-0052 explicitly authorises phase 05.2's brainstorming session to EDIT the `## HTTP/2` subsection IN PLACE (not replace via supersession ADR) to flip the deferred items to active rules when fixture 0004 lands. The rationale: the scaffold is intentionally incomplete; the shape is correct; the in-place edit is the lowest-friction path for 05.2's planner and keeps the history coherent without proliferating supersession ADRs for content fills.

### Consequences

- The BEHAVIOR_CONTRACT's `## HTTP/2` subsection is authoritative for 05.1's equivalence scope. Any deviation from the h2spec `failed == 0` threshold on the included sections is a regression.
- Phase 05.2's brainstorming session may edit this subsection in place under the SCAFFOLD-pattern authorisation above. If it discovers that the asserted equivalence must be *relaxed* (not just extended), a superseding ADR is required.
- The Header allow-list table grows by 5 rows. Rows for `:method`/`:path`/`:scheme`/`:authority` carry "applies-to: 05.2" annotations; when 05.2 fixture 0004 goes green, the annotation is the only thing that changes — the rows themselves are already correct.
- Future phases that extend the H2 equivalence surface (e.g., trailers in phase 07, gRPC in a gRPC-family phase) add sub-sections here (or a subsection sibling) via a new ADR, not by editing 05.1's `### Not asserted` block silently.

This ADR supersedes nothing.

## ADR-0053: Phase-04 REVIEW Minor carry-forward triage

**Status:** Accepted
**Date:** 2026-04-25
**Doctrine:** D-3.4, D-3.5
**Settles:** SPEC ADR-X; phase-05.1 §12 (phase-04 REVIEW carry-forward disposition).

### Context

Phase 04's `REVIEW.md` (commit `04527eb`) records five Minor findings (M-2, M-4, M-5, M-6, M-7) that were not resolved before phase-04 reached `done`. Per phase-05.1 SPEC §12, these carry-forwards must be formally triaged in 05.1's closing task so that they do not silently accumulate. ADR-0045 (the phase-05 split decision) explicitly assigned phase-04 REVIEW carry-forward triage to 05.1 as a closing-task deliverable.

### Decision

Per-Minor disposition (each entry references the REVIEW.md finding by its identifier):

- **M-2** (ADR-0043 "Doctrine: D-3.4, D-3.5" mismatched against informal supersession qualifier): **DEFERRED** — cosmetic. Phase 05.1 does not touch ADR-0043; the mismatch is a prose-level annotation inconsistency, not a behavioural gap. A future doctrine-cleanup ADR supersedes ADR-0043 with corrected attribution. Phase 05.1 adds no new obligations.

- **M-4** (listener-manager `Stop()`/`Listeners()` race): **DEFERRED** to phase 08. Phase 05.1 does not touch the lock surface in `internal/listener/manager.go`. The race carries forward to phase 08's admin-api-and-drain phase as the natural close, where drain semantics require a correct `Listeners()` snapshot under concurrent `Stop()` calls. The phase-06-must-consume tag is NOT applied here (phase 06's scope is observability, not drain); phase 08's brainstorm is required to address M-4.

- **M-5** (phase-04 SPEC §7 failure-mode prose vs `defer upstreamConn.Close()` mechanism): **DEFERRED**. Phase 05.1 does NOT introduce a parallel mechanism on the H2 path because `routerActionH2` is 05.2's surface. The same prose-vs-mechanism shape WILL reappear in 05.2's `routerActionH2.do` with `defer clientConn.Close()` — 05.2's brainstorming inherits this disposition rather than re-litigating. ADR-0053's forward-looking note: phase-05.2-will-repeat-the-pattern; a future SPEC-corrections ADR closes both the phase-04 M-5 gap and the analogous 05.2 gap in one pass.

- **M-6** (fixture-0003 driver heredoc YAML pattern): **DEFERRED**. Phase 05.1 introduces no new fixture, so the structured-`expectations.yaml` plan from ADR-0019 (phase 03's fixture shape) remains unforced. The observability sweep in phase 06 is the natural close for fixture-shape standardisation. Phase 05.2's brainstorming may elect to use structured YAML for fixture 0004 (per the SPEC §10 #10 settlement note in ADR-0045), which would create organic pressure to retrofit 0003. Phase 05.1 defers without a must-consume tag on this item.

- **M-7** (`Filter.statPrefix` stored but never consumed): **DEFERRED with phase-06-must-consume tag**. Phase 05.1 does not consume `Filter.statPrefix` either; the field sits un-consumed across the H1 and H2 paths both. Phase 06's brainstorm is REQUIRED to either: (a) honour `Filter.statPrefix` in the stats emission code — lifting M-7 to RESOLVED; or (b) supersede ADR-0041 with a stat-naming policy that obviates the `statPrefix` field entirely. The phase-06-must-consume tag is explicit: failure to address M-7 in phase 06 is a phase-06 REVIEW finding.

**New H2 prose-vs-mechanism shape (05.1 scope):** Phase 05.1 introduces a new analogous shape on the H2 path — the `defer` cleanup in `serverStream.dispatch`'s action invocation (analogous to phase-04 M-5's H1 prose-vs-mechanism gap). The 05.1 SPEC §7 does not have a separate failure-mode prose section for the H2 path at the same granularity as phase-04 §7 (by design — the H2 codec's error handling is specified in §5.2's state machine, not a separate failure-mode list). The cosmetic gap between prose and mechanism on the H2 path is acknowledged and deferred to the same future SPEC-corrections ADR that resolves M-5 on the H1 path.

### Consequences

- M-2, M-4, M-5, M-6 carry forward with explicit disposition records. No phase closes them silently.
- M-7 has a hard phase-06-must-consume tag: phase 06 MUST address it. Phase-06's brainstorming session reads this ADR and includes M-7 in its acceptance checklist.
- Phase 05.2 inherits the M-5 disposition (H1 `defer upstreamConn.Close()` prose-vs-mechanism gap) without re-litigating; 05.2's brainstorming session notes the analogous `routerActionH2.do` pattern as pre-triaged under ADR-0053.
- This ADR does not create any new code obligations in 05.1 — all five dispositions are textual/deferred.

This ADR supersedes nothing.

## ADR-0054: ADR-0046 prose correction — root-http2 import file list

**Status:** Accepted
**Date:** 2026-04-26
**Doctrine:** D-3.5
**Settles:** REVIEW.md (phase 05.1) finding M-15.
**Supersedes:** ADR-0046 (textual drift on the 3-import file list only — the substantive decision in ADR-0046 stands; only the file-list claim in the Consequences section is corrected).

### Context

ADR-0046 (line 1545) names `framer.go`/`hpack.go`/`settings.go` as "the three files that legitimately import the package" `golang.org/x/net/http2`. This is incorrect: `hpack.go` imports `golang.org/x/net/http2/hpack` (the sub-package), not the root `http2` package. The actual production-code importers of root `http2` are `framer.go`, `settings.go`, `conn.go`.

The phase-05.1 REVIEW (REVIEW.md M-15) flagged this as documentation drift. The grep gate that ADR-0046 invokes runs against the actual code, not the ADR prose, so the doctrine D-3.2 boundary holds in practice; only the prose is wrong.

Per D-3.5, landed ADRs are append-only. The correction lands as this superseding ADR rather than an in-place edit of ADR-0046.

### Decision

The correct list of production files importing `golang.org/x/net/http2` (root package, not sub-packages) at HEAD of the phase 05.1 follow-up batch is:

- `internal/filter/hcm/h2/framer.go`
- `internal/filter/hcm/h2/settings.go`
- `internal/filter/hcm/h2/conn.go`

`internal/filter/hcm/h2/hpack.go` imports `golang.org/x/net/http2/hpack` only; it does NOT import the root `http2` package. (Compare: `stream.go` also imports `http2/hpack` for `hpack.HeaderField`; the boundary applies to root-`http2` imports.)

The grep-verifiability formulation of ADR-0046 stands (the gate runs against actual code). Only the prose enumeration in ADR-0046's Consequences bullet (line 1545) is amended by this ADR.

### Consequences

- The grep boundary check is unchanged. Future executors running the gate verify against the actual code, not against this prose enumeration.
- ADR-0046 remains the canonical decision on the codec source. This ADR does not change the codec source decision; it corrects only the file-list enumeration.
- Future readers of ADR-0046 see the superseded marker and follow it here for the corrected file list.
- This ADR supersedes ADR-0046 in scope **only** for the file-list claim. The substantive D-3.2 codec-vs-runtime boundary, the FORBIDDEN runtime-types list, and the driver-side test exception in ADR-0046 all stand unchanged.

This ADR is itself append-only. Future drift in the file list (e.g., if a new file imports root `http2`) is corrected by a further superseding ADR.

## ADR-0055: Flow-control discipline for the from-scratch H2 codec

**Status:** Accepted
**Date:** 2026-04-26
**Doctrine:** D-3.6 (every phase is a green build — and the from-scratch H2 codec must be RFC-correct under realistic peer settings, not just under the conformance-suite peer's defaults).
**Settles:** phase-05.1 REVIEW Important findings I-1, I-2, I-3 and Minor findings M-3, M-5, M-7, M-9, M-11. Closes the ADR-0055 anticipation in phase-05.2 SPEC §4.4.
**Supersedes:** nothing.

### Context

The phase-05.1 codec primitives implemented the RFC 9113 §5.2 baseline correctly enough to PASS h2spec at 53/53, but every shipped 05.1 path was a bodyless GET to a `direct_response` with a small body, which dormantly left three flow-control gaps the 05.1 REVIEW flagged as Important:

- I-1 — `ServerConn.writeData` did not respect `SETTINGS_MAX_FRAME_SIZE`; an outbound DATA frame larger than the peer's advertised cap would have produced a peer-side `FRAME_SIZE_ERROR` against any peer advertising a non-default cap.
- I-2 — `ServerConn.writeData` did not enforce the per-stream send window; a peer that throttled `INITIAL_WINDOW_SIZE` tightly would have observed a `FLOW_CONTROL_ERROR`.
- I-3 — receive-side flow control was allocated (`recvW` field) but never decremented or replenished; a request body larger than the initial 65535 receive window would have stalled forever.

Plus the related Minor findings M-3 (`waitFor`+`reserve` non-atomicity + a dead `if taken <= 0` recovery branch), M-5 (translation-block duplication between `framer.readFrameCtx` and `framer.tryReadFrame`), M-7 (`recvW` fields kept-but-dead), M-9 (WINDOW_UPDATE delta overflow not bounds-checked per RFC 9113 §6.9.1), and M-11 (`recvData` writes to `s.reqBody` *before* checking state-transition validity, growing memory on closed streams).

Phase 05.2's routed-to-upstream H2 surface is the load-bearing context that activates these gaps in production: real upstream peers advertise non-default settings, real request/response bodies cross the 65535 boundary, and a memory-waste path under adversarial peers becomes a denial-of-service risk. The 05.2 SPEC §4.4 anticipated ADR-0055 as the prerequisite that closes the gaps before the routed-to-upstream surface lands.

The 05.1 REVIEW recommended (`Recommendation` Path A) "a single dedicated ADR documenting flow-control discipline for the from-scratch H2 codec end-to-end" rather than per-fix ADRs. This ADR is that bundle.

### Decision

ADR-0055 enumerates seven specific code-level fixes, each landed in a discrete commit so a future supersession can target precisely:

- **I-1 — Outbound `MaxFrameSize` chunking.** `internal/filter/hcm/h2/conn.go` `writeData` chunks outgoing DATA at `min(connWindow, streamWindow, peer.MaxFrameSize)`. Lands Task 3, commit `d3de1f8`.
- **I-2 — Per-stream send-window enforcement.** Same `writeData` path enforces the per-stream send window via `serverStream.sendW.reserveBlocking(n)` before each chunk. Lands Task 3, commit `d3de1f8`.
- **I-3 — Inbound `recvW` decrement + half-window WINDOW_UPDATE emission.** `internal/filter/hcm/h2/conn.go` `onData` debits both the conn-level and per-stream `recvW` on every inbound DATA chunk and emits `WINDOW_UPDATE` once the running counter crosses the half-window threshold (`32768 = 65535/2`). Lands Task 4, commit `b951c38`.
- **M-3 — `reserveBlocking` collapse + dead-branch deletion.** `internal/filter/hcm/h2/flow.go` collapses `waitFor`+`reserve` into a single atomic `reserveBlocking(n)` and deletes the dead `if taken <= 0` recovery branch in `writeData`. Required for I-2 to be race-free under concurrent multi-stream writes. Lands Task 2, commit `964df19`.
- **M-5 — `translateFramerErr` helper extraction.** `internal/filter/hcm/h2/framer.go` extracts the common framer-error translation block from `readFrameCtx` and `tryReadFrame` into a single helper; cosmetic prerequisite so the future `ClientConn`'s framer wrapper consumes the same translation. Lands Task 2, commit `964df19`.
- **M-9 — Overflow bounds-check on WINDOW_UPDATE.** `internal/filter/hcm/h2/conn.go` `onWindowUpdate` (conn-level) and `internal/filter/hcm/h2/stream.go` `recvWindowUpdate` (stream-level) reject WINDOW_UPDATE deltas that would push the send window past `2³¹-1` per RFC 9113 §6.9.1, returning `connError(ErrFlowControlError)` → GOAWAY (conn-level) or `streamError(ErrFlowControlError)` → RST_STREAM (stream-level). Lands Task 4, commit `b951c38`.
- **M-11 — `recvData` state-before-append reorder.** `internal/filter/hcm/h2/stream.go` `recvData` validates the stream state BEFORE appending to `s.reqBody`; DATA on a closed or half-closed-remote stream returns the `STREAM_CLOSED` error first and does NOT cause server-side memory growth. Lands this Task (Task 5), this commit.

The seven fixes are interlinked: `reserveBlocking` (M-3) is required for per-stream send-window enforcement (I-2) to be race-free; the overflow bounds-check (M-9) is required for WINDOW_UPDATE emission (I-3) to be safe under adversarial peers; the `recvW` fields (M-7) become load-bearing under I-3. The bundle is the right shape because the cross-references between the fixes outweigh the per-fix isolation a separate-ADR shape would offer.

M-7 is not enumerated separately above because it is the consequence of I-3: the `recvW` field allocations become non-dead the moment `onData` debits them; no separate code change is required for M-7 beyond the I-3 fix.

### Consequences

- The 05.1 codec primitives are now load-bearing for realistic upstream H2 workloads. Each of the seven fixes ships with a regression test (per phase-05.2 SPEC §1 #6 + this ADR's Settles list). Specifically: I-1 is regression-tested by a `>16384`-byte body chunked correctly (≥2 DATA frames; no peer-side `FRAME_SIZE_ERROR`); I-2 is regression-tested by `INITIAL_WINDOW_SIZE: 16` + 100-byte response body producing ~7 DATA frames + no `FLOW_CONTROL_ERROR`; I-3 is regression-tested by a 100KB inbound body completing without deadlock with WINDOW_UPDATE frames observed on the wire; M-3 is race-tested by concurrent multi-stream writes against a window primed at boundary values; M-9 is unit-tested at the boundary (`MaxInt32 - 1`, delta `2`); M-11 is unit-tested by DATA on a `halfClosedRemote` stream not growing `s.reqBody`.
- `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold-language paragraph is extended (in-place per ADR-0052 — the SCAFFOLD form's "in-place edit at the closing task" discipline) at phase 05.2's Task 15 with non-default `MaxFrameSize` and tight-window prose. Per-section pass counts at the ADR-0051 pin remain at the 05.1 baseline (53/53); ADR-0055 introduces no new conformance section requirements.
- The bundled-vs-per-fix-ADR shape is intentional per the 05.1 REVIEW's Path A wording. Splitting into seven per-fix ADRs would create cross-references harder to read than the bundle. The seven-fix enumeration above is the precision affordance: a future supersession can name the specific fix it replaces (e.g. "supersedes ADR-0055 I-1 only").
- M-7 (formerly "`recvW` fields dead") is closed-by-consequence of I-3; no separate fix or ADR.
- The receive-side WINDOW_UPDATE emission policy (half-window threshold = 32768) is not RFC-mandated; RFC 9113 leaves the policy to the implementation. The half-window threshold is the standard implementation choice (matches `golang.org/x/net/http2` and Envoy's defaults). A future hardening ADR may re-tune this if production telemetry justifies a different threshold.
- The `closedStreams` map remains unbounded in 05.2 (M-12 from the 05.1 REVIEW). That gap is bundled neither into ADR-0055 nor into ADR-0058 — it is a long-lived-conn hardening item carried forward in PROGRESS.md only, deferred to a future hardening phase per the phase-05.2 SPEC §12.2 per-finding-disposition.

### Lands-in-task

Phase-05.2 Task 5 (the closing task of the ADR-0055 fix sequence at Tasks 2-5). The seven fixes individually land at the commits enumerated above; this ADR itself lands in the same commit as the M-11 fix (the final fix of the seven-fix sequence).

## ADR-0056: Per-request fresh upstream H2 dial

**Status:** Accepted
**Date:** 2026-04-26
**Doctrine:** D-3.5 (record cross-phase decisions).
**Settles:** phase-05.2 SPEC §10 #2.3 (newly out-of-scope at 05.2 — upstream H2 stream pooling/multiplexing across requests is deferred to the upstream-robustness family). Closes the ADR-0056 anticipation in phase-05.2 SPEC §4.4. Mirrors phase-04 ADR-0039 (per-request fresh upstream H1 dial). Resolves the carry-forward note from phase-04 ADR-0053's "phase-05.2-will-repeat-the-pattern" forward-looking clause: 05.2 introduces the analogous prose-vs-mechanism shape (`defer cc.Close()` in `routerActionH2.do`) and ADR-0056 formally acknowledges the cosmetic gap that ADR-0053 deferred.
**Supersedes:** nothing.

### Context

H2's defining benefit over H1 — multiplexing many streams onto a single conn — is fundamentally NOT realised when the upstream dial happens once per router-action invocation. The per-request fresh-conn pattern does not amortise the TLS handshake, the H2 preface + SETTINGS exchange, or the cwnd ramp; tail latency under the per-request pattern is dominated by handshake variance and the new-conn slow-start.

Phase 05.2 SPEC §2.1 enumerates upstream H2 stream pooling / multiplexing across requests as deferred to the upstream-robustness family (the phase that lands per-cluster H2 conn pools, max-concurrent-streams enforcement, GOAWAY-driven conn rotation, and per-conn settings cache). That family's scope is broader than 05.2's "land routed-to-upstream H2 differentially" charter, so ADR-0056's per-request-fresh discipline is the right phase-05.2 shape.

The phase-04 precedent (ADR-0039) made the same call for upstream H1: fresh conn per request, `defer conn.Close()` at the action site. ADR-0056 mirrors that discipline for H2, with the same "production guidance: pooling is required for production workloads" caveat.

### Decision

Every `routerActionH2.do` invocation (Task 11 — `internal/filter/hcm/actions.go`'s H2 router action) calls `r.cluster.DialH2(ctx)` (this task — `internal/cluster/dial_h2.go:DialH2`) to obtain a *fresh* `*h2.ClientConn`. Within a single invocation, exactly one stream is opened on the new conn via `cc.RoundTrip`. The conn is closed immediately after the response is consumed via `defer cc.Close()` at the action site. Cross-invocation pooling is the upstream-robustness family's deliverable, not phase-05.2's.

`Cluster.DialH2` is structured so each error branch closes the underlying conn explicitly: on a successful return the caller takes ownership of the `*h2.ClientConn` (and its underlying `*tls.Conn`), but on error there is no caller-owned wrapper to defer-close, so the underlying conn would otherwise leak file descriptors. The `defer cc.Close()` mechanism at the call site (`routerActionH2.do`) is the per-request closure ADR-0053 anticipated.

The phase-05.2 differential surface does NOT assert pool/non-pool: Envoy pools, envoy-go does not, but both produce the same per-request `:status` / response-body output and both produce the per-side `[3,3,3]` distribution under the sequential-request workload (fixture 0004's six listeners × N requests with round-robin LB across three endpoints per cluster). The cross-conn frame counts differ between Envoy and envoy-go but those frame-level counts are not in the equivalence matrix.

### Consequences

- Under load, per-request latency increases linearly with request rate; tail latency suffers from TLS handshake variance — intentional in 05.2, mitigated by this ADR's "production guidance: pooling is required for production workloads" clause. The fixture-0004 differential workload runs sequentially, so the per-request-fresh discipline does not introduce flakiness on the differential gate.
- The upstream-robustness family, when it lands, brings H2 pooling and supersedes ADR-0056 with a pooling-discipline ADR. The pooling ADR will inherit the per-conn settings cache + max-concurrent-streams enforcement + GOAWAY-driven conn rotation as the load-bearing shape.
- The carry-forward from phase-04 ADR-0053's "phase-05.2-will-repeat-the-pattern" note is now resolved. ADR-0053 deferred the prose-vs-mechanism formalisation of the per-request-fresh closure to "the next phase that introduces the same shape"; phase 05.2 is that phase, and ADR-0056's `defer cc.Close()` mechanism acknowledgement closes the carry-forward.
- The `closedStreams` map's unbounded-growth concern (M-12 from the 05.1 REVIEW) is unaffected by ADR-0056. M-12 is a long-lived-conn hardening item; under ADR-0056's per-request-fresh discipline, every H2 conn is short-lived (one stream then closed), so `closedStreams` entries are reclaimed when the conn is closed. M-12 becomes load-bearing only in the upstream-robustness family's pooling phase (where conns are long-lived and `closedStreams` may grow without bound across many request lifetimes).
- The H1 mirror (ADR-0039) continues to govern the H1-routed surface; ADR-0056 is the H2-specific symmetric ADR, not a generalisation. A future cross-codec consolidation may bundle both into a single "per-request-fresh-upstream-conn" ADR if the upstream-robustness family's pooling work makes the bundle natural; phase 05.2 keeps them separate per the per-codec-ADR precedent.

### Lands-in-task

Phase-05.2 Task 9 (first use of `Cluster.DialH2` in the production codepath — `internal/cluster/dial_h2.go:DialH2`). The ADR lands in the same commit as the dial helper itself; `routerActionH2.do`'s `defer cc.Close()` call site lands at Task 11 and carries no separate ADR (the discipline is governed by ADR-0056).

---

## ADR-0058: Trailers observed but not forwarded — H2 router

**Status:** Accepted
**Date:** 2026-04-26
**Doctrine:** D-3.4 (record durable design rationale where context-isolation requires it).
**Settles:** phase-05.2 SPEC §2.1 (Trailer support — request and response — out-of-scope), SPEC §10 #1 (the trailer rule reaffirmed for the 05.2 client surface), and the 05.1 REVIEW Minor carry-forwards M-4 (`readClientPreface` not ctx-aware — bundled per SPEC §12.2's per-finding-disposition; deferred to phase 06 or 07 with the proper fix at the listener-manager level via uniform OS read deadlines) and M-10 (`SETTINGS_TIMEOUT` absent — bundled per SPEC §12.2's per-finding-disposition; deferred to phase 06 or 08 with the proper fix at the listener-manager's per-conn timeout policies). Closes the ADR-0058 anticipation in phase-05.2 SPEC §4.4 (third ADR landing of the four-ADR contiguous block 0055..0058, per the topical-vs-commit-order ordering — non-monotonic; 0058 lands at Task 11 before 0057 lands at Task 14).
**Supersedes:** nothing.

### Context

The 05.1 H2 server-side codec correctly observes trailing HEADERS frames per RFC 9113 §8.1 (h2spec section 8 asserts this); see `internal/filter/hcm/h2/stream.go:recvTrailingHeaders` for the downstream-side observe-and-discard. Phase 05.2's H2 client-side codec also observes trailing HEADERS on the upstream conn; see `internal/filter/hcm/h2/client.go:dispatchFrame`'s HEADERS case where the second HEADERS block is observed via `cs.respHeadersSeen` and dropped on the floor (the comment explicitly references this ADR: "trailing HEADERS — observed-and-discarded per ADR-0058").

The `routerActionH2` action (this task — `internal/filter/hcm/actions.go`) discards trailers in BOTH directions:
- Downstream-from-client trailing HEADERS are observed by `ServerConn` and discarded by `serverStream`'s dispatch (the dispatcher hands a fully-buffered request body to the action; trailing HEADERS that arrive after END_STREAM on DATA never surface).
- Upstream-from-server trailing HEADERS are observed by `ClientConn`'s readLoop and discarded by `RoundTrip` (the assembled `H2Response` carries only the FIRST HEADERS block; subsequent HEADERS arriving before END_STREAM on DATA are dropped).

The router emits END_STREAM on the response HEADERS or final DATA, never via a trailing HEADERS frame on the downstream stream. The fixture-0004 driver does not exercise trailers (bodyless GETs only); the differential gate is unaffected by the asymmetry between envoy-go (discards) and Envoy reference (forwards trailers when configured).

This ADR also bundles two 05.1 REVIEW Minor carry-forwards per SPEC §12.2's per-finding-disposition, because both items are phase-bookkeeping concerns rather than discrete design choices and folding them into a separate carry-forward ADR would create cross-references harder to read than the bundle:

- **M-4 (`readClientPreface` not ctx-aware).** The 05.1 codec's `internal/filter/hcm/h2/preface.go:readClientPreface` does not honor the conn-lifetime ctx; a peer that opens a TCP connection without sending the preface bytes can hold the codec in a blocking read. The proper fix is at the listener-manager level via uniform OS read deadlines applied to every accepted conn (mirroring the H1 driver's `SetReadDeadline` discipline), which is a phase 06/07 concern (the listener-manager rewrite or the filter-chain framework). Phase 05.2 does NOT touch `preface.go`. The carry-forward is tagged "phase-06-or-07-must-consider".

- **M-10 (`SETTINGS_TIMEOUT` absent).** RFC 9113 §6.5.3 recommends a SETTINGS_TIMEOUT-class bound on how long a peer may take to ACK our SETTINGS. The 05.1 codec does not implement this timeout (h2spec sends SETTINGS_ACK promptly so the absence does not surface in the conformance gate); the proper fix lands with the listener-manager's per-conn timeout policies in phase 06 or 08 (whichever introduces the broader timeout-policy framework). Phase 05.2 does NOT introduce a SETTINGS_TIMEOUT timer. The carry-forward is tagged "phase-06-or-08-must-consider".

### Decision

Trailers are observed-and-discarded in both directions at phase 05.2; the codec correctness (h2spec section 8 PASS at 53/53 — the trailer-ordering tests pass because observation-without-forwarding is correct framing) is unaffected. The router action `routerActionH2.doH2` emits the response in HEADERS + DATA(END_STREAM) shape only; no trailing HEADERS frame is generated downstream.

Cross-references to the implementation:
- `internal/filter/hcm/h2/client.go` — `dispatchFrame`'s HEADERS case (`if !cs.respHeadersSeen { ... } else: trailing HEADERS — observed-and-discarded per ADR-0058`). The upstream-side observe-discard.
- `internal/filter/hcm/h2/stream.go:recvTrailingHeaders` — the downstream-side observe-discard, unchanged from 05.1. Validates that trailing HEADERS does NOT contain pseudo-headers (RFC 9113 §8.1.2.1) and advances state to half-closed-remote, but does not surface the trailers to the dispatcher.
- `internal/filter/hcm/actions.go:routerActionH2.doH2` — emits the response in HEADERS + DATA(END_STREAM) shape; the upstream-side discard happens BELOW this layer (in RoundTrip's H2Response assembly), so the action sees an `H2Response` that already excludes trailers.

### Consequences

- The differential surface is asymmetric in principle (envoy-go discards trailers; Envoy reference forwards them when configured) but bounded in 05.2 because fixture 0004 doesn't exercise trailers. The divergence is unobservable on the differential gate. Phase 07's filter-chain framework + the gRPC family land trailer forwarding (where `grpc-status` is carried in trailers and forwarding is the load-bearing benefit).
- `BEHAVIOR_CONTRACT.md ## HTTP/2`'s "Not asserted" subsection enumerates trailers per ADR-0058 (in-place edit at Task 15).
- M-4 carry-forward: `readClientPreface`'s ctx-unaware shape stays in 05.2; the proper fix at the listener-manager level via uniform OS read deadlines is a phase 06/07 concern, tagged "phase-06-or-07-must-consider".
- M-10 carry-forward: `SETTINGS_TIMEOUT` stays absent in 05.2; h2spec sends SETTINGS_ACK promptly per the 05.1 REVIEW; the proper fix lands with the listener-manager's per-conn timeout policies in phase 06 or 08, tagged "phase-06-or-08-must-consider".
- The router's emission shape (HEADERS + DATA(END_STREAM); never trailing HEADERS) is the simplest correct discipline under SPEC §2.1's "trailers out-of-scope" charter; future trailer-forwarding work will widen `H2Response` with a `Trailers []hpack.HeaderField` field and the router will emit a trailing HEADERS frame conditionally when the upstream provided one. The widening is forward-compatible — no field renames; only an additive field. Phase 05.2 does not pre-implement this; the bundle remains: trailers observed, discarded, never forwarded.

### Lands-in-task

Phase-05.2 Task 11 (first use of `routerActionH2.doH2`'s observe-discard trailer rule, alongside the variant-selection wiring and the `h2.Action` interface widening). Lands the third of the four-ADR contiguous block (ADR-0055..ADR-0058) in commit-time order: 0055 (Task 5) → 0056 (Task 9) → 0058 (Task 11) → 0057 (Task 14). The non-monotonic order is documented in PLAN's "ADRs introduced by this plan" section and in this ADR's `Settles` field.

---

## ADR-0057: Closes ADR-0035 H/2 leg via fixture 0004's full-stack HTTPS h2

**Status:** Accepted
**Date:** 2026-04-26
**Doctrine:** D-3.6 (every phase is a green build — and the H/2 surface is now under differential coverage), D-3.5 (durable design rationale).
**Settles:** phase-05.2 SPEC §1 #9 + §2.3 + §11.6 (carry-forward of "fixture-0003 still does not differentially exercise upstream TLS" from phase-04 REVIEW, narrowed to the H/2 leg specifically); phase-05.2 SPEC §10 #1.7 (`BEHAVIOR_CONTRACT.md` flips deferred-to-05.2 entries to active per ADR-0057, edit-in-place performed at Task 15 per ADR-0052). Closes the H/2 leg of ADR-0035 (the H/1 + upstream-TLS leg remains open). Lands fourth of the four-ADR contiguous block (ADR-0055..ADR-0058) in commit-time order: 0055 (Task 5) → 0056 (Task 9) → 0058 (Task 11) → 0057 (Task 14) — non-monotonic per PLAN's "ADRs introduced by this plan" topical-vs-commit-order ordering.
**Supersedes:** nothing — closes (settles) the H/2 leg of ADR-0035 without superseding ADR-0035 itself; the H/1 leg of ADR-0035 remains open and is carried forward under the `phase-05.2-follow-up` tag (see Consequences below).

### Context

ADR-0035 (phase 03 era) recorded that fixture 0002's plaintext upstream backends left the upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch in `internal/cluster/cluster.go:84-105`) under unit-test coverage only, not differential. Three follow-up paths were enumerated in ADR-0035's Consequences: (a) extending `test/differential/harness.go` with TLS-backend support; (b) driving upstream TLS through a naturally-TLS fixture such as phase 04's HTTPS HTTP/1.1 upstream; (c) waiting for HTTP/2 (phase 05+).

Phase 04's REVIEW carried this forward as the H/1 + upstream-TLS gap (option (b) was not pursued — phase 04's fixture 0003 stayed plaintext-upstream to keep the harness simple). Phase 05's parent SPEC anticipated 05.2 closing the H/2 leg via fixture 0004 (option (c)). Phase-05.2 SPEC §4.4 anticipated the closing ADR; this ADR is that closing.

The closure is non-vacuous: fixture 0004's driver issues 27 H/2 round-trips per side over the full chain — driver → TLS+ALPN h2 → proxy listener (HCM AUTO → ALPN h2 dispatch) → router action H2 → `Cluster.DialH2` → `Cluster.Dial`'s TLS branch → ALPN h2 negotiation → `*h2.ClientConn` → `(*ClientConn).RoundTrip` → 3 backend H/2 servers — so every layer of the upstream-H/2-over-TLS stack is exercised by both sides of the differential gate.

### Decision

Fixture 0004 has full-stack HTTPS h2 between proxy and upstream backends:
- **Subject** (`envoy-go.yaml`): cluster `c_h2_backend` is `STATIC` with 3 endpoints at `127.0.0.1:<bN>`. Cluster's `transport_socket` is `envoy.transport_sockets.tls.v3.UpstreamTlsContext` with `sni: localhost`, `alpn_protocols: ["h2"]`, and a `validation_context.trusted_ca` that inlines the fixture-local CA. Cluster's `typed_extension_protocol_options.HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` pins the upstream codec to H/2 per ADR-0056.
- **Reference** (`envoy.yaml`): cluster `c_h2_backend` is `STRICT_DNS` (per ADR-0027) with `dns_lookup_family: V4_ONLY` (per ADR-0010); 3 endpoints at `host.docker.internal:<bN>`. Same `UpstreamTlsContext` shape (`sni: localhost`, `alpn_protocols: ["h2"]`, inline trusted_ca) and the same `HttpProtocolOptions` discriminator. `--concurrency 1` (per ADR-0028) keeps RR distribution deterministic on the reference side.

The driver issues 27 sequential H/2 round-trips per side (9 × `GET /health`, 9 × `GET /api/v1/<n>`, 9 × `GET /missing/<n>`) via `helpers.H2RoundTrip` (per Task 12 — `golang.org/x/net/http2.Transport` + driver-side fresh-Transport-per-call discipline). Per-side `[3,3,3]` distribution is asserted from response-body `"backend-<idx>:"` prefix counts (subprocess HTTPSH2 backends do not increment the runner's in-process accept counter; the driver counts in-band — settled SPEC §10 #14).

The upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch + phase-05.2's `DialH2`) is NOW under differential coverage on the H/2 leg.

### Consequences

- **`BEHAVIOR_CONTRACT.md ## HTTP/2`** flips its `:method`/`:path`/`:scheme`/`:authority` allow-list rows from `applies-to: phase 05.2 routed-to-upstream H2 (forward-looking)` to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`. The "Does not yet apply to" subsection drops "routed-to-upstream H/2" + "fixture 0004" entries. The in-place edit per ADR-0052 happens at Task 15; ADR-0057 anticipates this edit but does not perform it (ADR-0057 lands at Task 14 alongside the driver).

- **The H/1 + upstream-TLS gap remains open** as the surviving carry-forward from ADR-0035. It is tagged `phase-05.2-follow-up`; the closure path is one of:
  1. A new fixture (e.g., `0005-https-h1-routing`) that mirrors fixture 0003 (`0003-http11-routing`) but with TLS upstream — most direct, minimal harness churn.
  2. An extension of fixture 0003 to add a TLS-upstream variant — equally direct.
  3. Folding the gap closure into a later phase (07's filter-chain framework, or an HTTP-filter-family phase) where TLS-upstream coverage is incidental to the broader scope.
  Phase 05.2 does not pre-decide which; the tag is the carry-forward.

- **The differential coverage of fixture 0004 is the FIRST non-vacuous gate (a) on the H/2 surface.** Gate (a) was vacuous in 05.1 per ADR-0045 (the 05.1 split deferred all H/2 fixtures to 05.2); 05.2's gate (a) is non-vacuous via fixture 0004. Phase 05's parent ROADMAP row flips to `done` at the phase-done commit per SPEC §4.4 (executed at Task 15).

- **No new SPEC, no new BEHAVIOR_CONTRACT subsection.** The ADR-0052 SCAFFOLD subsection's in-place edit at Task 15 captures the contract surface. ADR-0057 is a closure ADR, not an introduction ADR.

- **Test infrastructure side-effects landing alongside this ADR at Task 14:**
  - `test/fixtures/0004-h2-routing/envoy.yaml` + `envoy-go.yaml` gain `sni: localhost` on the upstream `UpstreamTlsContext` (Go's `crypto/tls.Client` requires non-empty `ServerName` when `InsecureSkipVerify=false`; the backend leaves carry SAN `localhost`, so this is correctness-preserving). This is a Task-13 oversight observed during Task 14's first execution of the differential gate; correcting it within Task 14's commit keeps the fix local to the gate that surfaced it.
  - `test/differential/runner_test.go`'s `startHTTPSH2Backend` gains `SysProcAttr{Setpgid: true}` and the deferred backend cleanup kills the process group via `syscall.Kill(-pid, SIGKILL)`. Without this, the `go run` parent is reaped but the orphaned backend binary holds onto the test process's stderr fd, causing `Cmd.WaitDelay` to fire on test exit and produce a spurious package-level `FAIL`. This is also a Task-13 carry-forward; the fix is local because Task 14 is the first task to actually run the gate end-to-end with subprocess backends.

### Lands-in-task

Phase-05.2 Task 14 (first invocation of fixture 0004's full-stack HTTPS h2 surface — the closing ADR before the BEHAVIOR_CONTRACT in-place edit at Task 15). The non-monotonic ADR-number-vs-commit-order sequence (0055 → 0056 → 0058 → 0057) is intentional per PLAN's `## ADRs introduced by this plan` topical-vs-commit-order rationale and per ADR-0058's own `Lands-in-task` cross-reference.

## ADR-0059: Internal Stats Store architecture

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.2 (no third-party-runtime-import for runtime-critical surfaces) + D-3.3 (own the canonical observation surface).
**Supersedes:** nothing.

### Context

Phase 06.1 introduces a `/stats/prometheus` admin endpoint (per phase-06.1 SPEC §1) that exposes the proxy's counters and gauges in the Prometheus exposition text format. The architectural question is whether to depend on `github.com/prometheus/client_golang` (the canonical Go Prometheus library, with its own `Registry`, `Counter`, `Gauge`, and exposition writer) or to build a thin in-tree atomic-counter Registry under `internal/stats` and emit Prometheus text via a hand-rolled writer.

Phase-06.1 BRAINSTORM §2.1 anticipates that future Observability-family phases (gRPC ALS, OTLP push, statsd UDP emitter) will need to hook a *registry* (something they can iterate to extract metric values), not a Prometheus client. Investing in a Prometheus client now would either (a) couple every future sink to a Prometheus-shaped intermediate, requiring conversion shims, or (b) duplicate the registry abstraction in `internal/stats` anyway, in which case the Prometheus client adds dependency surface without architectural value. This is the same architectural choice Envoy made in `source/common/stats/` (an in-tree stats store with adapters per sink).

Doctrine D-3.2 (no third-party-runtime-import for runtime-critical surfaces) and D-3.3 (own the canonical observation surface) jointly preclude option (a). Doctrine D-3.6 (every phase is a green build) precludes option (b) at this scale (the duplication would be a follow-up tech-debt entry).

### Decision

A thin in-tree atomic-counter `Registry` under `internal/stats`:
- `Registry` holds a slice of `Metric` values (the interface satisfied by `*Counter` and `*Gauge`) and a name→Metric map for duplicate-detection at registration time.
- `Counter` is backed by `sync/atomic.Uint64`; `Inc` and `Add(delta uint64)` are lock-free atomics. `Gauge` (Task 3) is backed by `sync/atomic.Int64`; `Inc`, `Dec`, `Set(int64)` are lock-free atomics.
- The Prometheus exposition text format is emitted by a hand-rolled writer (`internal/stats/prom.go`, lands at Task 5) that walks the registry, sorts the result, and writes `# HELP` / `# TYPE` / metric-line triplets directly to an `io.Writer`. No `prometheus/client_golang` runtime dependency.
- `Walk(fn func(Metric))` holds `r.mu.RLock` for the duration of the iteration; the slice is a stable snapshot under LBP-1 (see ADR-0060's companion concept the LBP-1 invariant, also introduced here for symmetry).
- The LBP-1 invariant ("list before play") — `Registry.Freeze()` is called at boot-end (after admin server starts accepting, before listener manager begins accepting connections); subsequent `NewCounter` / `NewGauge` calls panic with `"stats: registry frozen: cannot register %q post-boot"`. This is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments — once the slice is frozen, the only contention on `r.mu` is RLock-vs-RLock among concurrent scrapes (which the `RWMutex` allows).

Alternatives considered:
- **(A) `prometheus/client_golang` directly.** Rejected per the future-sink-coupling argument above. Also: `client_golang`'s `Registry` is not iteration-friendly without using its `Gather()` method (which returns `[]*dto.MetricFamily` — a Prometheus protobuf shape that's awkward to bridge to gRPC ALS or OTLP).
- **(B) `expvar` + custom serializer.** Rejected because `expvar` lacks a histogram primitive (and while ADR-0060 defers histograms from 06.1, it does not preclude them from later sub-phases — choosing `expvar` would make that future work harder).
- **(C) Build the in-tree Registry as a thin wrapper around `prometheus/client_golang`'s primitives.** Rejected for the same reason as (A): the dependency is still there, just with a façade.

### Consequences

- (a) The `internal/stats` package's external dependencies are limited to the Go stdlib (`fmt`, `regexp`, `strconv`, `sync`, `sync/atomic`). No third-party Prometheus runtime import.
- (b) The LBP-1 invariant (this ADR's companion concept) makes the read path lock-free against hot-path increments — the Walk holds RLock, the Inc holds nothing (atomic.Add), and the slice is immutable post-Freeze so the Walk's iteration is data-race-free against potential concurrent writes (there are none).
- (c) Future xDS-CDS phases that introduce dynamic cluster registration will need a copy-on-write list shape that supersedes LBP-1. The carry-forward is recognized: 06.1 does not need it because all metrics are registered at static-bootstrap-load time. When CDS lands, the LBP-1 panic-on-post-Freeze-register discipline becomes a hindrance and will be replaced with a copy-on-write `metrics atomic.Pointer[[]Metric]` shape; that supersession is a future-phase ADR's job.
- (d) The registration sites are concentrated at boot: `cluster.NewManager` and `listener.NewManager` (Tasks 8 + 10) take a `*stats.Registry` and register their per-instance metrics in their constructors. Once Freeze is called, a programming error (a stray `r.NewCounter` from a hot path) panics rather than silently corrupting the read snapshot — this is a feature, not a bug.

### Lands-in-task

Phase-06.1 Task 2 (alongside ADR-0060). The two ADRs are introduced in the same commit because they are companion architectural decisions: ADR-0059 establishes "we own the canonical observation surface" and ADR-0060 establishes "what's in 06.1's surface and what's deferred."

## ADR-0060: Histograms deferred from 06.1

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.6 (every phase is a green build) + D-3.4 (record durable design rationale).
**Supersedes:** nothing.

### Context

Envoy v1.37.2's `/stats/prometheus` exposes counters, gauges, AND histograms. The histograms include `cluster.<name>.upstream_rq_time` (request-to-upstream-response latency), `http.<stat_prefix>.downstream_rq_time` (full-request latency at the HCM boundary), and request/response size distributions. Envoy's internal histogram implementation is `circllhist` — a dynamic-bucket log-linear histogram — and the Prometheus exposition layer bridges circllhist's quantile-derived buckets to Prometheus's fixed-bucket `_bucket{le="..."}` shape. This bridging is non-trivial: the bucket boundaries Envoy emits depend on the configured Prometheus quantile set (`histogram_buckets` in `stats_config.histogram_bucket_settings`), the circllhist internal state, and the per-scrape `extractValuesFromHistogram` logic.

Byte-equivalent matching of histogram output between envoy-go and Envoy v1.37.2 (the differential-gate criterion per phase-06.1 SPEC §3) requires either (a) replicating circllhist's storage and bucket-derivation algorithms, or (b) using a Prometheus-native histogram type and accepting that bucket boundaries may diverge from Envoy's. Both options are substantial design work and want their own brainstorm, distinct from 06.1's counter+gauge surface.

Phase-06.1 BRAINSTORM §2.2 records the deferral decision: 06.1's scope is the counter+gauge surface (17 metric names enumerated in SPEC §6), and histograms are carried forward to a later sub-phase with its own brainstorm. The carry-forward also captures `server.uptime` (a gauge that depends on monotonic-clock + per-scrape recompute and pairs naturally with the histogram brainstorm because both are recompute-on-scrape rather than increment-on-event).

### Decision

Phase 06.1 emits counters + gauges only. Histograms are deferred:
- `cluster.<name>.upstream_rq_time` — deferred.
- `http.<stat_prefix>.downstream_rq_time` — deferred.
- Request/response size distributions (`upstream_rq_body_size`, `downstream_rq_body_size`, etc., per Envoy's exposition) — deferred.

The deferral is to a later 06.x sub-phase or to an upstream-robustness-family phase, with its own brainstorm covering circllhist→Prometheus bucket mapping and the byte-equivalent-vs-shape-equivalent design choice. `server.uptime` is co-deferred for the same brainstorm.

### Consequences

- (a) **Rule SN7** (in `internal/stats/name.go` and BEHAVIOR_CONTRACT.md §Stat-name mapping) reads "Histograms are not emitted by 06.1 (forward-looking)." The flattening rules SN1–SN8 stay counter+gauge-only.
- (b) The 17-name catalog in phase-06.1 SPEC §6 is exhaustive for 06.1. The differential gate (Task 14, fixture 0005) compares envoy-go's `/stats/prometheus` output against a pre-recorded Envoy v1.37.2 output that has been pre-filtered to those 17 names; histogram lines from Envoy's output are filtered out before the diff (per SPEC §3).
- (c) The future histogram-introducing sub-phase supersedes this ADR. That ADR will introduce Rule SN9 (or extend SN7) to enable histogram emission, will introduce the histogram primitive in `internal/stats`, and will widen SPEC §6's catalog.
- (d) `server.uptime` is co-deferred to the same future sub-phase. 06.1's `server.live` gauge (always 1 once boot completes) is the only `server.*` metric in SPEC §6.

### Lands-in-task

Phase-06.1 Task 2 (alongside ADR-0059). The two ADRs are companions: ADR-0059 establishes the architectural "what we own" and ADR-0060 establishes the scoping "what's in vs what's deferred."

## ADR-0061: Stat-name → Prometheus-name flattening rules SN1–SN8 (with empirically-pinned Rule SN4)

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.4 (record durable design rationale)

### Decision

The eight rules SN1–SN8 govern flattening from internal hierarchical-dotted names (e.g., `cluster.c0.upstream_rq_2xx`) to Prometheus-format `name{label="value"}` lines. Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 are settled at brainstorm-close (BRAINSTORM §7.1); Rule SN4 is empirically pinned at SPEC-drafting time per BRAINSTORM §2.3.1 against reference Envoy v1.37.2's default tag-extractor regex.

Rule summary:
- **SN1:** `cluster.<n>.<rest>` → `envoy_cluster_<rest>` + label `envoy_cluster_name=<n>`
- **SN2:** `http.<stat_prefix>.<rest>` → `envoy_http_<rest>` + label `envoy_http_conn_manager_prefix=<stat_prefix>`
- **SN3:** `listener.<addr>.<rest>` → `envoy_listener_<rest>` + label `envoy_listener_address=<addr>`
- **SN4:** `<base>_Nxx` (N ∈ 1..5) → `<base>_xx` + label `envoy_response_code_class=<N>` (single class digit as string)
- **SN5:** `server.<rest>` → `envoy_server_<rest>` + no labels
- **SN6:** HELP text best-effort English (NOT byte-equal to Envoy's HELP text — differential equivalence is on values + label keys + types only)
- **SN7:** Histograms not emitted by 06.1 (forward-looking; per ADR-0060)
- **SN8:** Per-endpoint cluster stats not emitted (forward-looking)

Rule SN4 verified form: trailing class digit STRIPPED from metric name (so `cluster.<n>.upstream_rq_2xx` flattens to `envoy_cluster_upstream_rq_xx`); label name `envoy_response_code_class`; label value the single class digit as a string (`"2"`, `"3"`, `"4"`, `"5"`).

### Context — verbatim Envoy-scrape evidence

```
# TYPE envoy_cluster_upstream_rq_xx counter
envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c_backend"} 3
envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="c_backend"} 1
envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="c_backend"} 1
# TYPE envoy_http_downstream_rq_xx counter
envoy_http_downstream_rq_xx{envoy_response_code_class="1",envoy_http_conn_manager_prefix="ingress_http"} 0
envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_http"} 3
envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_http"} 0
envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_http"} 1
envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"} 1
```

Negative-confirmation grep over the full 1181-line scrape: `grep -E 'envoy_[a-z_]*_(1xx|2xx|3xx|4xx|5xx)'` returns 0 matches — no metric ends in `_Nxx`.

Tag-extractor regex source: Envoy v1.37.2 `source/common/config/well_known_names.cc`, the `RESPONSE_CODE_CLASS` tag entry. Source-tree commit pin = the v1.37.2 release tag, server-side version-string SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches `ENVOY_TARGET.md`).

Counter-examples (NOT what Envoy emits):
- `envoy_cluster_upstream_rq_2xx{...}` (digit suffix preserved)
- `envoy_cluster_upstream_rq_xx{envoy_response_code_class="2xx",...}` (label value with literal "xx")
- `envoy_cluster_upstream_rq{envoy_response_code_class="2",...}` (`_xx` stripped entirely)

### Consequences

- (a) `internal/stats/name.go`'s `flattenToProm` MUST implement Rule SN4 in this exact verified form (regex `^(.+)_([1-5])xx$`).
- (b) `BEHAVIOR_CONTRACT.md ## Stat-name mapping`'s in-place edit at Task 15 carries Rule SN4 in the same form.
- (c) Future phases adding new stat-name patterns extend SN1–SN8 with append-only rules.

### Lands-in-task

Phase-06.1 Task 4. Supersedes nothing.

## ADR-0064: `stats_config.stats_tags` config not honored; extraction hardcoded

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.4 (record durable design rationale)

### Decision

Phase 06.1 hardcodes the stat-name → Prometheus-name extraction logic in `internal/stats/name.go` (per Rules SN1–SN5 of the SN1–SN8 set established by ADR-0061); the bootstrap proto's `stats_config.stats_tags[]` field is silently ignored if present.

### Context

Per BRAINSTORM §2.3 and §5.6, the regex-driven tag-extraction surface in Envoy is complex (~50 default regexes plus user-supplied overrides) and warrants its own phase. 06.1 ships fixed extraction that matches Envoy's default tag-extractor behavior on the 17 names in scope. The silent-ignore preserves bootstrap forward-compat — a fixture-0005 reference bootstrap with `stats_tags: []` round-trips without error.

Carry-forward: a future stats-config phase, or an xDS-RTDS revisit, will introduce the dynamic tag-extractor surface and the per-stat regex-override path.

### Consequences

- (a) The silently-ignored field set is amended to include: `stats_config.stats_tags[]`, `stats_config.stats_matcher`, `stats_config.histogram_bucket_settings`, `stats_config.use_all_default_tags`, `stats_sinks[]`, HCM `stats_flush_interval`, Cluster `track_cluster_stats`, Listener `stat_prefix`.
- (b) The original silent-ignore ADR (from phase 04) is amended (not superseded) per the 05.1 + 05.2 amendment shape.
- (c) Future stats-config phases land their own ADR superseding ADR-0064.

### Lands-in-task

Phase-06.1 Task 4 (alongside ADR-0061). Supersedes nothing.

## ADR-0063: Per-endpoint cluster stats not emitted in 06.1

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.4 (record durable design rationale where context-isolation requires it)

### Decision

Phase 06.1 emits cluster-level metrics only — the 8 names enumerated in SPEC §6 (`cluster.<n>.upstream_rq_total`, `cluster.<n>.upstream_rq_<2,3,4,5>xx`, `cluster.<n>.upstream_cx_total`, `cluster.<n>.upstream_cx_active`, `cluster.<n>.membership_total`). Per-endpoint expansion (`cluster.<n>.endpoint.<addr>.upstream_rq_total`, `cluster.<n>.endpoint.<addr>.upstream_cx_total`, etc. — equivalent of Envoy's `enable_per_endpoint_stats=true` mode) is deferred to a later phase.

### Context

Per BRAINSTORM §2.3 + §9 the per-endpoint expansion is dynamic in shape: the endpoint set churns under xDS-EDS reconfiguration. Statically-allocated per-endpoint metrics break the LBP-1 invariant established by ADR-0059 (the post-Freeze panic discipline assumes the metric list is fixed at boot — endpoint churn would require dynamic registration, which would force a copy-on-write list shape that supersedes the lock-free Walk-under-RLock-plus-atomic-Load read path). Properly handling the dynamic-shape case wants xDS-EDS semantics that are out of scope for the 06.1 observability surface.

This also matches Envoy's default tag-extraction behavior: `enable_per_endpoint_stats` defaults to `false` in upstream Envoy v1.37.2, so the omission preserves cross-implementation behavioral equivalence on the default static-cluster shape that fixture 0005 exercises.

### Consequences

- (a) Rule SN8 of the SN1–SN8 set (per ADR-0061) reads "Per-endpoint cluster stats are not emitted by 06.1 (forward-looking)".
- (b) The cluster-side metric-allocation loop in `internal/cluster/manager.go` (`registerClusterMetrics`) allocates exactly 8 metrics per cluster (not 8×N for N endpoints).
- (c) The `Cluster` struct carries 8 unexported metric-pointer fields; per-endpoint storage is not modeled.
- (d) The fixture-0005 expectations table (Task 14) includes `cluster.c0.membership_total: 1` (the per-cluster gauge Set to len(endpoints)) but no per-endpoint rows; the differential gate-(a) assertion for stats does NOT verify per-endpoint stats.
- (e) Carry-forward: the xDS-EDS phase revisits with a copy-on-write list shape that supersedes both LBP-1 and ADR-0063 jointly.

### Lands-in-task

Phase-06.1 Task 8 (the cluster-side metric-allocation loop in `internal/cluster/manager.go`; first use of the cluster-level-only metric set). Supersedes nothing.

---

## ADR-0062: Differential equivalence shape for stats output

**Status:** Accepted
**Date:** 2026-04-27
**Doctrine:** D-3.4 (record durable design rationale where context-isolation requires it)

### Context

SPEC §6 defines 17 stat names (12 counters + 5 gauges) that fixture 0005-prometheus-stats must verify. The differential harness must assert equivalence between upstream Envoy and envoy-go on these names. Two semantic problems arise:

1. **Counter semantics**: envoy-go (ADR-0056: per-request fresh upstream connection) and upstream Envoy (keepalive connection pooling) produce different absolute `upstream_cx_total` and `listener.downstream_cx_total` counts for the same workload. Absolute equality would produce false failures.

2. **Gauge semantics**: Connection-activity gauges (`upstream_cx_active`, `downstream_cx_active`) must reach 0 after the workload drains; `server.live` and `membership_total` must be snapshot-equal after the workload because their values are not traffic-driven.

3. **HELP-text noise**: Prometheus `/stats/prometheus` HELP text is documentation only — it is not a behavioral observable. Asserting HELP-text equality would produce spurious failures when upstream Envoy changes wording.

### Decision

The fixture 0005 stats differential asserts equivalence on the 17-name allow-list using three rules:

1. **Per-counter delta-equality**: For each counter in the allow-list, compute `delta = after.value − before.value` on both sides. Assert `ref_delta == subj_delta`. An exception is granted for `cx_total`-class names (see rule 3).

2. **Per-gauge snapshot-equality**: For each gauge in the allow-list, assert `ref_after == subj_after`. (Before-values are not compared because gauges can be non-zero before the drive run starts.)

3. **`cx_total` delta-min `≥ 1` (no equality)**: `cluster.<n>.upstream_cx_total` and `listener.<addr>.downstream_cx_total` use `delta_min ≥ 1` on each side independently — no equality assertion between sides. This accommodates the structural difference: envoy-go opens a fresh connection per request (N connections for N requests), while upstream Envoy's keepalive pool may reuse a single connection across the entire workload, yielding `ref_delta = 1` vs `subj_delta = N`.

4. **HELP-text values ignored (Rule SN6)**: HELP-text lines and TYPE-annotation lines in the Prometheus exposition are not compared. Only sample-line values are extracted and asserted.

5. **Unknown names ignored**: Any metric name not in the 17-name allow-list is silently dropped by the parser. The assertion only fires on the allow-listed names.

6. **In-band assertion discipline (SPEC §12 #6)**: The stats assertion is implemented as an optional `StatsAsserter` interface on the driver, analogous to `DistributionAsserter` and `HTTPExpectations`. The runner invokes it after ProbeAdmin. No generic `StatsExpectations` data structure is introduced.

### Consequences

- (a) Fixture 0005 (`test/fixtures/0005-prometheus-stats/`) implements this contract via `AssertStats` / `AssertStatsEquivalence` in `driver/driver.go`.
- (b) Only the 17 names emitted within the allow-list are gated. Any additional stat names that either implementation happens to emit are silently ignored and do not affect the gate result.
- (c) `BEHAVIOR_CONTRACT.md § Equivalence Matrix` row 19 (the placeholder "Stats output: TBD") is effectively superseded by this ADR's equivalence shape; the concrete row will be filled in at Task 15 (the BEHAVIOR_CONTRACT stats row update).
- (d) The `fixture.StatsAsserter` interface and `fixture.TB` minimal-testing interface are introduced in `test/differential/fixture/fixture.go` at Task 14.
- (e) Future phases that add new stat names to the allow-list must update both SPEC §6 and the `applyToSnapshot` dispatch table in `driver/driver.go`.

### Lands-in-task

Phase-06.1 Task 14 (fixture 0005-prometheus-stats + runner registration). Supersedes nothing.

## ADR-0065: Validate metric-name-deriving inputs at the user-input boundary

**Status:** Accepted
**Date:** 2026-04-28
**Doctrine:** D-3.4 (record durable design rationale) + D-3.6 (every phase is a green build).

### Context

Phase-06.1 verifier commit `1f94b74` surfaced a `FuzzHCMConfigParse` crasher: HCM stat-name construction at `internal/filter/hcm/config.go:164` builds `"http." + stat_prefix + ".downstream_rq_total"` from a user-controlled `stat_prefix` and feeds it to `stats.Registry.NewCounter`, which panics in `Registry.checkName` (`internal/stats/registry.go:100`) when the assembled name fails the metric-name regex `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`. The minimised seed `9ba19570cf17f59f` reproduced the panic with `stat_prefix = "0000000000 0"` (12 bytes, literal SP at index 10).

ADR-0059 establishes that `Registry.NewCounter` panics on invalid name (and on duplicate registration, and post-Freeze) because metric registration is a boot-time concern — programmer errors at boot should crash loudly, not return errors callers might silently ignore. That contract is correct for static, programmer-supplied metric names (`server.live`, `server.live`-style fixed strings). It is wrong when the caller derives the name from user input — the panic then becomes an undefined runtime crash.

The same shape exists at three call sites in the 06.1 surface: HCM (`stat_prefix`), cluster (`cluster.<name>`), and listener (`listener.<addr>`). Listener already pre-sanitises via `normalizeAddr` (`internal/listener/manager.go:196-198`, replaces `:`, `.`, `[`, `]`); HCM and cluster do not.

Two fix-shape candidates were considered, both rejected for the stated reasons:

- **(A) Sanitise `stat_prefix` per Rule SN1's invalid-character substitution** before constructing the counter name. **Rejected:** Per ADR-0061's empirically-pinned Rule SN2, `stat_prefix` is preserved verbatim as the Prometheus label `envoy_http_conn_manager_prefix=<stat_prefix>`. Sanitising would silently mutate that label vs upstream Envoy's emission. Two stat_prefixes differing only in invalid chars would collapse to the same Prometheus label value — a silent data-loss bug. Rule SN1 was empirically anchored against reference Envoy v1.37.2; extending it with substitution semantics requires its own ADR + re-pinning evidence.

- **(B) Convert `Registry.NewCounter` to return `(c, error)` instead of panicking on invalid name.** **Rejected:** ADR-0059's panic discipline is load-bearing for the LBP-1 invariant — duplicate-name and post-Freeze panic for the same boot-error reason, and converting one of the three panic paths to error-return would force the other two to follow for symmetry. That is a wider API change than the gate-(d) blocker requires.

### Decision

Validate every metric-name-deriving user input at the boundary where the input enters envoy-go's process state — before the assembled name reaches `Registry.NewCounter`. Use the same regex (`internal/stats.nameRE`) that `Registry.checkName` would enforce, so the boundary check and the registry check are guaranteed to agree (no drift risk).

The mechanism: `internal/stats` exposes a read-only `IsValidName(name string) bool` helper that wraps `nameRE.MatchString`. Callers that derive names from user input call this helper on the assembled name and return a domain-prefixed error (`hcm: invalid stat_prefix: …`, `cluster: invalid cluster name: …`) on failure.

This preserves ADR-0059's panic discipline for programmer errors (e.g., `server.live` is a static literal — if a future commit typos it to `server live`, panic-at-boot is the right response) while routing user-input failures through the normal config-parse error channel. It is also the same shape the listener path already uses (the `normalizeAddr` pre-pass guarantees the assembled name is always valid).

### Consequences

- (a) `internal/stats/registry.go` exposes `IsValidName(name string) bool`. The function is read-only (no state mutation), takes a single `string`, returns a single `bool`, and adds zero coupling. The existing `Registry.NewCounter` / `Registry.NewGauge` panic discipline is unchanged.
- (b) `parseFilterWithCtx` (`internal/filter/hcm/config.go`) gains a single guard between the existing `stat_prefix` non-empty check and the route-config build: `if !stats.IsValidName("http." + statPrefix + ".downstream_rq_total") { return nil, fmt.Errorf("hcm: invalid stat_prefix: %q (...)", statPrefix) }`. Validating the longest assembled name suffices because the other four assembled names (`downstream_rq_2xx` through `downstream_rq_5xx`) differ only in the suffix's last 4 chars (`tota` → `total`, `_2xx` etc.) which are all in the regex's permitted character class — they pass/fail together.
- (c) The fuzz target's "no panic; every error message is hcm:-prefixed" contract (`internal/filter/hcm/fuzz_test.go:38-40`) is preserved: the new error path returns a hcm:-prefixed error.
- (d) The same vulnerability latently exists in `internal/cluster/manager.go:97` where `cluster.<name>` is propagated into eight metric names without validating the cluster name's character set. This is a carry-forward — the gate-(d) fix branch is scoped to the single-blocker fix per `STATE.md`'s "focused single-issue branch" guidance. A follow-up branch will add `cluster.NewManager` validation using the same `stats.IsValidName` helper. The carry-forward is recorded in `PROGRESS.md`'s lifecycle-state-3 fix block and inherits ADR-0065's pattern by reference (no new ADR needed).
- (e) Future filter / extension authors that introduce metrics derived from user input MUST validate at their input boundary using `stats.IsValidName` (or an equivalent boundary check) — the panic discipline is correct for static names but not for user-input-derived names, and silently relying on `Registry.NewCounter`'s panic to surface invalid input is a design defect.

### Lands-in-task

Phase-06.1 lifecycle-state-3 fix branch (`phase/06.1-stats-prometheus-impl-followup-gate-d`). The cluster-name carry-forward will land in a future branch under the same phase-06.1 umbrella or in a phase-06.1 review-followup batch; it inherits this ADR's pattern by reference. Supersedes nothing; complements ADR-0059.

## ADR-0066: Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure)

**Status:** Accepted
**Date:** 2026-04-30
**Doctrine:** D-3.2 (no third-party-runtime-import for runtime-critical surfaces) + D-3.3 (own the canonical observation surface).

### Context

Phase 06.2 lands envoy-go's access-log subsystem. The architectural choice — whether to consume a third-party access-log library (logrus / zap / zerolog / fluent) or to own a thin in-tree shape — is foundational because the Sink interface choice today constrains every future Observability-family phase (gRPC ALS, OTLP) that wants to add additional sinks. Phase 06.1 made the same architectural choice for stats (own an internal `stats.Registry`, no Prometheus client library) per ADR-0059; the same reasoning applies to access logs: future sinks should hook a Sink interface envoy-go owns, not a third-party logger's specific record shape.

### Decision

A thin in-tree `internal/accesslog` package: `Sink` interface with `Submit(*Record)` + `Close() error`, `Record` struct (10 plumbed fields per Decision A), `Default` formatter (15-operator Envoy-default-format line), `AsyncFileSink` async-writer with bounded-channel drop-newest backpressure. NO third-party access-log dependency: `go.mod` MUST NOT contain `github.com/sirupsen/logrus`, `go.uber.org/zap`, `github.com/rs/zerolog`, `github.com/fluent/fluent-logger-golang`, or equivalents. The package's only non-stdlib dependency is `internal/stats` (for the drop-counter Counter type).

Hot-path discipline: lock-free on `Submit` — Go's buffered-channel non-blocking `select`-with-`default` is atomic-CAS-bounded (no mutex, no syscall) when the channel has capacity. Single-consumer writer goroutine drains the channel into per-record `os.File.Write`, atomic for sub-PAGE writes under `O_APPEND` per `man 2 write` on Linux (no `fsync`; OS page cache is the durability ceiling — matches Envoy). Drop-newest backpressure: full-channel Submit increments `server.accesslog_dropped` counter (per ADR-0069) and emits a 1-second-rate-limited `log.Printf` diagnostic. No queue-depth gauge (would force `atomic.LoadInt64` on every submit, contrary to the lock-free hot-path discipline).

### Alternatives considered

- (A) `logrus` / `zap` / `zerolog` directly — REJECTED. Future-sink coupling: binds the in-process record model to a structured-logging library's specific shape, blocking the gRPC ALS / OTLP sinks future phases will land. Same reasoning as ADR-0059's rejection of `prometheus/client_golang`.
- (B) Per-record blocking `os.File.Write` on the hot path — REJECTED. Per-request HCM finalization should not block on disk I/O.
- (C) Unbounded channel — REJECTED. OOM-on-overload is worse than drop-newest.

### Consequences

- (a) `internal/accesslog` package's external dependencies are limited to the Go stdlib + `internal/stats` (for the drop-counter Counter type). The boundary grep at the closing all-gates sweep enforces this.
- (b) The AsyncFileSink concurrency model is documented inline in `writer.go` (Submit non-blocking; writer goroutine single-consumer; Close `sync.Once`-guarded).
- (c) Future Observability-family phases that introduce additional sinks (ALS, OTLP) extend this package by implementing the `Sink` interface — no architectural churn needed.
- (d) The phase 06.2 acceptance grep `! grep -rE 'logrus|go.uber.org/zap|rs/zerolog|fluent-logger-golang' .` is the gate; both production code and `_test.go` are subject to the boundary.

### Lands-in-task

Task 2 (the Sink-interface + Record-struct introduction; the architectural shape applies to every subsequent task in the package). Supersedes nothing; complements ADR-0059.

## ADR-0069: `server.accesslog_dropped` counter naming (SN5 mapping)

**Status:** Accepted
**Date:** 2026-04-30
**Doctrine:** D-3.4 (record durable design rationale where context-isolation requires it).

### Context

Per ADR-0066, `internal/accesslog`'s AsyncFileSink uses drop-newest backpressure: full-channel Submit increments a counter and emits a rate-limited diagnostic. The counter must be allocated against 06.1's `*stats.Registry` (per ADR-0059 the Registry is the single source of truth for Prometheus exposition), and its name must follow the SN1–SN5 flattening rules from ADR-0061 so the resulting Prometheus name reads naturally and aggregates across sinks correctly.

### Decision

The drop-newest backpressure counter is allocated as `registry.NewCounter("server.accesslog_dropped")`. Per 06.1 Rule SN5 (`server.<rest>` → `envoy_server_<rest>`, no labels), the Prometheus exposition name is `envoy_server_accesslog_dropped`. **Outside the 06.1 17-name allow-list** — fixture 0005's differential explicitly ignores the metric per ADR-0062's allow-list discipline. Operator-visible at `/stats/prometheus` only.

The counter is allocated **once per process** (not once per sink) — the loop in `cmd/envoy-go/main.go` allocates exactly once even with N sinks, sharing the counter across all sinks; per-sink debug visibility comes through the per-sink `path` value in the rate-limited diagnostic log line.

The internal-stats `helpText` map in `internal/stats/name.go` gains one entry per Decision K + SPEC §12 #5: `"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."`.

### Alternatives considered

- (A) `accesslog.dropped` — REJECTED. Violates the SN1–SN5 prefix convention; would need a new SN-rule.
- (B) `http.<stat_prefix>.accesslog_dropped` — REJECTED. The per-process aggregation surface doesn't cleanly key per stat_prefix when there are multiple HCMs.
- (C) Per-sink `accesslog.<sink_path>.dropped` — REJECTED. Path strings are not metric-name-safe (filesystem characters fail `internal/stats.nameRE`); per-sink granularity is over-shaped for a backpressure indicator.

### Consequences

- (a) The counter name is a constant in `internal/accesslog/stats.go`'s `RegisterDroppedCounter` function.
- (b) The `helpText` map entry follows 06.1's discipline (per Rule SN6).
- (c) Future sink types (ALS, OTLP) introduced in later phases may add sibling counters (e.g., `server.accesslog_als_failed`) under the same SN5 mapping.
- (d) The metric is OUTSIDE the 06.1 17-name fixture-0005 allow-list per ADR-0062 — that fixture's parser silently drops it; no test changes needed in the 06.1 fixture.

### Lands-in-task

Task 5 (alongside the package skeleton; the counter wiring lives in `internal/accesslog/stats.go`; the `helpText` map extension lives in `internal/stats/name.go`). Supersedes nothing; complements ADR-0059 + ADR-0061.

## ADR-0067: Reject `log_format` at parse (option β; extends ADR-0065's boundary-validation pattern)

**Status:** Accepted
**Date:** 2026-04-30
**Doctrine:** D-3.4 (record durable design rationale; the rejection is a contract that future bootstrap consumers MUST observe).

### Context

Per Decision C, phase 06.2's bootstrap parser reads HCM `access_log[]` as a list (any length: 0 → no-op; N → emit to all N sinks per request, in registration order — no artificial 1-cap). The `envoy.extensions.access_loggers.file.v3.FileAccessLog` typed_config supports a custom format via `log_format` / `format_string` / `json_format` — features envoy-go does NOT implement in phase 06.2. The choice: silently ignore the format fields (emit Envoy-default-format always) OR reject at parse-time with a clear error.

### Decision

The bootstrap parser READS HCM `access_log[]` as a list of any length; each entry's typed-config of type `envoy.access_loggers.file` MUST have `path` (required, non-empty string); ANY presence of `log_format` / `format_string` / `json_format` produces a fatal parse error: `bootstrap: unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)`. Other typed-config types (`envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`) remain silently-ignored per the ADR-0041 amendment (Consequences (c)).

### Alternatives considered

- (A) Silent-ignore — REJECTED. A bootstrap that says "I want JSON-formatted access logs" but receives Envoy-default-formatted logs is a silent deviation from the operator's intent; failing-loud at parse-time forces the operator to remove the field (or the project to ship the parser surface in a future phase).
- (B) Honor `log_format` via a command-operator parser — REJECTED. The format-string parser surface is ~500 LoC and a non-goal of phase 06.2 (per SPEC §2.1 first bullet).

### Consequences

- (a) The silently-ignored field set is amended (per ADR-0041's amendment shape, mirroring the 05.1 + 05.2 + 06.1 amendments) to add `envoy.access_loggers.stdout` / `tcp_grpc` / `open_telemetry` entries; see the ADR-0041 amendment block in DECISIONS.md.
- (b) Parse-fail messages on `log_format` are grep-verifiable in `bootstrap_test.go`.
- (c) Future phases that ship the format-string parser supersede this ADR.

### Lands-in-task

Task 7 (the bootstrap parser extension; first use of the option-β rejection in production code). Supersedes nothing; complements ADR-0065.

---

## ADR-0068: Differential fixture 0006-access-log — three-tier equivalence matrix

**Status:** Accepted
**Date:** 2026-04-30

### Context

Phase 06.2 ships access-log emission in envoy-go. The differential suite verifies that envoy-go's per-request access-log records are equivalent to reference Envoy v1.37.2 on the 15-operator default format. However, not all 15 operators can be byte-equal across both sides:

- Some operators are inherently non-deterministic (timestamps, request IDs) or per-side (upstream host address).
- Some operators envoy-go deliberately omits per Decision A (the Tier-S operators: RESPONSE_FLAGS, BYTES_RECEIVED, X-FORWARDED-FOR, X-REQUEST-ID, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)).
- The remaining operators should be byte-equal across both sides for the same workload.

A single "byte-equal or skip" policy would either over-constrain (failing on valid divergences) or under-constrain (missing real bugs). A three-tier matrix is needed.

### Decision

Adopt a three-tier equivalence matrix for fixture 0006-access-log:

**Tier E (byte-equal, 7 operators):** `:METHOD`, `:PATH`, `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT`, `USER-AGENT`, `:AUTHORITY` — cross-side byte-equal (after host-part normalization for AUTHORITY).

**Tier F (format-only, 3 operators):** `START_TIME` (RFC3339 ms-precision UTC on both sides), `DURATION` (int ms ≥ 0 on both sides), `UPSTREAM_HOST` (either both "-" for direct_response, or both `host:port`-shaped for routed).

**Tier S (subject emits "-", 5 operators):** `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)` — subject MUST emit literal "-" (Decision A / not implemented in phase 06.2); reference unconstrained.

BYTES_SENT is Tier-E: envoy-go's `routerAction.do` reads the upstream response body into a buffer (via `io.ReadAll`), records `bytesSent = len(bodyBytes)`, then writes the full HTTP/1.1 response downstream. This isolates the body-byte count from the wire framing bytes (status line + headers), matching Envoy's BYTES_SENT semantics (body bytes only).

### Alternatives considered

- (A) Two-tier (equal vs. skip) — REJECTED. Does not distinguish "must be equal" (BYTES_SENT) from "format-check only" (START_TIME) from "subject-must-emit-dash" (X-REQUEST-ID). Conflating these loses signal on Tier-E regressions.
- (B) Skip-all non-equal fields — REJECTED. Would not catch a regression where BYTES_SENT drifts to 0 or to a wrong value.

### Consequences

- (a) `fixture.ReferenceLogMounter`, `fixture.AccessLogAsserter`, `fixture.HostMount` interfaces added to `test/differential/fixture/fixture.go`.
- (b) `StartReferenceProxyWithMounts` added to `test/differential/harness.go` using `HostConfig.Binds` (testcontainers-go v0.27.0 silently drops `ContainerMounts` bind entries in `mapToDockerMounts` — workaround documented in-code).
- (c) Fixture 0006-access-log: driver, backends, config templates, expectations.yaml, README.md.
- (d) Reference log polling uses a 30s deadline (Envoy v1.37.2 flushes its file-access-log buffer on a ~1s periodic timer; 30s guarantees ≥5 flushes within the poll window).
- (e) `DriveReference` normalizes `localhost:{port}` → `127.0.0.1:{port}` so the Go HTTP client sends `Host: 127.0.0.1:{port}`, matching the subject side and satisfying the Tier-E AUTHORITY assertion.

### Lands-in-task

Task 15 (differential fixture 0006-access-log + runner registration). Supersedes nothing; extends the differential suite architecture established by ADR-0028.

---

## ADR-0070: Phase-07 planner-time split (07.1 + 07.2)

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.5 (decisions are written), D-3.6 (every phase is a green build).

### Context

Phase 07 (filter-chain framework, BOOTSTRAP §8 row 07) covers two structurally distinct halves: (a) the HTTP filter framework — iteration protocol + extension registry + per-route config + the `cors` real filter + `envoy_go_test` test-only probe + the router-as-terminal-filter migration — anchored under the `internal/filter/hcm/` + `internal/filter/http/` package surface; and (b) the listener-chain completion — `listener_filters[]` framework, `FilterChainMatch` fields beyond SNI, `Listener.default_filter_chain` — anchored under the `internal/listener/` package surface. The two halves share no production-code surface; they share only the BOOTSTRAP §8 row identifier.

Per BRAINSTORM §1 + parent SPEC §3, the BRAINSTORM session split phase 07 along this surface axis at brainstorm-close. The split landed in the SPEC-drafting commit (master `ee45aba`) via:
- ROADMAP row `07` flipped `planned → in-progress` with sub-phases column `07.1, 07.2`.
- Row `07.1` added as `planned` with depends-on `06`.
- Row `07.2` added as `planned` with depends-on `07.1`.

This ADR formalizes the split decision durably; the ROADMAP edit is its concrete on-disk effect.

### Decision

Phase 07 is split into two sub-phases at planner-time per ADR-0045's discipline (which documented the 05.1 + 05.2 split and the 06.1 + 06.2 split):
- **07.1 — HTTP filter framework.** Surface: `internal/filter/http/` (NEW package tree) + `internal/filter/hcm/` (refactored). Differential surface at end: fixtures 0007a (cors differential) + 0007b (iteration-probe structural). Lands the iteration protocol that BOOTSTRAP §9 HTTP-filters family depends on.
- **07.2 — Listener-chain completion.** Surface: `internal/listener/`. Differential surface at end: TBD (07.2 SPEC drafts the fixtures). Lands the listener-side filter primitives.

Ordering is 07.1-first, 07.2-second because 07.1 unblocks the BOOTSTRAP §9 HTTP-filters family (every future HTTP filter — `header_manipulation`, `fault`, `jwt_authn`, `ext_authz`, etc. — depends on the iteration protocol + extension registry shipping in 07.1) while 07.2 has no §9 dependents.

The parent ROADMAP row `07` flips `planned → in-progress` at the SPEC-drafting commit (already landed at master `ee45aba`); transitions to `done` ONLY at 07.2's phase-done commit (NOT at 07.1's phase-done) — mirroring the 05/05.1/05.2 + 06/06.1/06.2 closure pattern.

### Alternatives considered

(A) Ship phase 07 as one sub-phase. Rejected: the cumulative LoC estimate (HTTP framework + listener chain + filter set + two fixtures) is ~12000 LoC, well above the §6.1 plan-size gate's ~1500-LoC OR-leg AND would push task count past the 25-task gate.

(B) Split along a different axis (e.g., filter-set first, then framework, then listener). Rejected: the iteration protocol + extension registry + chain state machine is the load-bearing primitive every filter depends on; splitting filter-set-first would require the framework's interfaces to be defined twice (placeholder in filter-set-first; real in framework-second) — wasted work.

(C) Defer the listener-chain to a feature-family phase post-08. Rejected: BOOTSTRAP §8 row 07's "filter chain framework" canonical title covers BOTH the HTTP framework AND the listener chain; deferring the listener chain would leave the BOOTSTRAP MVP trunk's row 07 incomplete on a load-bearing primitive (listener filters are needed for BOOTSTRAP §9's network-filters family).

### Consequences

(a) The phase 07 ROADMAP row carries a `sub-phases` column listing `07.1, 07.2`; status `in-progress` until BOTH sub-phases land done.

(b) 07.1's phase-done commit flips row `07.1 → done` AND leaves row `07` at `in-progress`; 07.2's phase-done commit flips BOTH rows `07.2 → done` AND `07 → done` AT THE SAME COMMIT.

(c) The 07.1 + 07.2 SPECs are siblings under a parent master SPEC at `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md`; the parent SPEC is read-only history once drafted (mirror of the 05 + 06 parent master SPECs).

(d) The seven 07.1 ADRs (ADR-0070..ADR-0076) are 07.1-scoped; 07.2 will introduce its own ADRs at its own SPEC + PLAN time.

(e) Total task count of phases 07.1 + 07.2 is bounded: 07.1 ships at 23 tasks (this PLAN); 07.2 will draft its own task count at its own PLAN time.

### Lands-in-task

07.1 PLAN Task 1 (PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at master `ee45aba` per SPEC drafting).

---

## ADR-0071: HTTP filter iteration protocol shape

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.2 (write from scratch — no third-party filter-chain library), D-3.5 (record durable design rationale).

### Context

The envoy-go HTTP filter chain framework needs an iteration protocol that lets multiple filters participate in request/response processing — each filter inspecting or mutating headers, body, and trailers — while remaining faithful to Envoy's filter-chain semantics without importing any third-party filter-chain engine. The protocol must address: (a) the method set each filter must implement (decode-side and encode-side); (b) how iteration status is signalled (status enums); (c) how filter instances are constructed per-request (factory pattern); (d) how asynchronous work in a filter (e.g., an external auth call) parks the HCM dispatch goroutine and later resumes it; and (e) what goroutine-safety guarantees exist.

Envoy's production filter chain is large (1xx headers, metadata frames, watermark-based backpressure, dual-dispatcher, etc.). For the envoy-go MVP, an Envoy-faithful *subset* is sufficient: the method set that serves cors, a test-probe filter, and the router-as-terminal-filter migration. Methods with no in-scope callers are deferred per D-3.5 (YAGNI).

### Decision

1. **Filter interfaces**: `StreamDecoderFilter` (decode-side: `DecodeHeaders`, `DecodeData`, `DecodeTrailers`, `SetDecoderCallbacks`, `OnDestroy`) and `StreamEncoderFilter` (encode-side: `EncodeHeaders`, `EncodeData`, `EncodeTrailers`, `SetEncoderCallbacks`, `OnDestroy`). A filter may implement one or both interfaces; the `HTTPFilter` struct carries nullable `Decoder`/`Encoder` fields as a tagged union — the chain dispatches per non-nil side.

2. **Status enums** — three types with the following iota values:
   - `FilterHeadersStatus`: `Continue` (0) — proceed to next filter; `StopIteration` (1) — park, resume via `ContinueDecoding`/`ContinueEncoding`. (`ContinueAndDontEndStream` out of MVP per YAGNI.)
   - `FilterDataStatus`: `DataContinue` (0) — proceed; `DataStopIterationAndBuffer` (1) — park and accumulate body chunks until end_stream; `DataStopIterationNoBuffer` (2) — park without body accumulation. (Watermark variant `StopAllIterationAndWatermark` out of MVP.)
   - `FilterTrailersStatus`: `TrailersContinue` (0) — proceed; `TrailersStopIteration` (1) — park, resume via `Continue*`.

3. **Out-of-MVP set** deferred per YAGNI: `ContinueAndDontEndStream`, `StopAllIterationAndWatermark`, `Encode1xxHeaders`, `decodeMetadata`/`encodeMetadata`.

4. **Two-step factory pattern**: `HTTPFilterFactory(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` — called once at HCM-build time to parse and validate `typed_config`; returns a `FilterInstanceFactory func() HTTPFilter` — called once per request to allocate a fresh filter instance bound to the parsed config. This separates config-parse cost (per-HCM-boot) from instance-allocation cost (per-request).

5. **Async-resume mechanics**: each per-stream `FilterChain` holds a buffered `chan struct{}` with capacity 1. When a filter returns `StopIteration` the dispatch goroutine blocks on `<-resumeCh`. When a filter's spawned goroutine calls `ContinueDecoding`/`ContinueEncoding`, it performs a non-blocking send (`select { case resumeCh <- struct{}{}: default: }`). Capacity-1 + non-blocking send means duplicate calls coalesce: the second send is a no-op if the channel is already full. This is idempotent by construction.

6. **Single-goroutine-per-request iteration invariant**: the HCM dispatch goroutine is the sole goroutine that drives `chain.runDecode*` / `chain.runEncode*`. Filter-spawned goroutines that do async work (e.g., an auth RPC) communicate back ONLY via the resume channel send — they do NOT re-enter chain iteration directly. This eliminates the need for a mutex on the iteration cursor.

### Alternatives considered

- **(A) Envoy's full method set** — REJECTED for YAGNI per D-3.5; methods we drop (`Encode1xxHeaders`, `decodeMetadata`/`encodeMetadata`, watermark callbacks) have no in-scope callers in the 07.1 task set.
- **(B) Per-filter goroutine** — REJECTED; goroutine-bloat in the common case (all filters returning `Continue` never need a goroutine); Envoy itself uses single-goroutine iteration in its HCM dispatcher (filter goroutines are opt-in via async callbacks, not the default dispatch path).

### Consequences

- (a) The framework's external dependencies are limited to Go stdlib + `google.golang.org/protobuf` + `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3` (proto types only; blank-imported in `internal/bootstrap/bootstrap.go` at Task 20) + `internal/cluster` (router sub-package only) — no third-party filter-chain-engine.
- (b) The iteration-protocol shape is documented in `internal/filter/http/doc.go` (the package overview comment).
- (c) Future family phases that introduce additional iteration features (1xx, metadata, watermark) extend this package by adding to the `StreamDecoderFilter` / `StreamEncoderFilter` interfaces — each such addition lands its own ADR.

**Supersedes ADR-0040 totally** (router-as-direct-call inside HCM connection loop is replaced by router-as-terminal-filter via the iteration protocol). **Partially supersedes ADR-0042** (the "exactly `[router]`" rule's lower bound stays as "must contain router as last entry"; the upper bound "exactly `[router]`" is lifted to "non-empty; last entry must be router").

### Lands-in-task

Task 2 (the iteration-protocol introduction). First use of the iteration-protocol shape in production code; the architectural shape applies to every subsequent task in the package.

---

## ADR-0072: HTTPRegistry threaded constructor map, no package-global

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.4 (record durable design rationale; the threading discipline is a contract that future package consumers MUST observe).

### Context

Phase 07.1 introduces the HTTP filter chain framework with a per-process registry mapping `typed_config` type_urls to filter factories. The choice: package-global with `init()`-based registration vs. an explicit threaded constructor map allocated at boot. The project has an established precedent from ADR-0059: `*stats.Registry` LBP-1 uses an explicit threaded constructor map frozen after boot — any late `Register` panics loudly.

### Decision

The `HTTPRegistry` is constructed once at boot in `cmd/envoy-go/main.go`, threaded explicitly into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)` via the listener-manager's HCM-construction path, NOT a package-global registered via `init()`. Freeze-after-boot invariant mirrors `*stats.Registry` LBP-1 from ADR-0059:

1. `HTTPRegistry.Freeze()` is called from `cmd/envoy-go/main.go` after all `Register` calls.
2. Any subsequent `Register` panics with `filter: registry frozen: cannot register %q post-boot`.
3. `Lookup` does not panic post-Freeze (read-allowed).
4. Three filters registered at boot: `router.New` (`envoy.filters.http.router`), `cors.New` (`envoy.filters.http.cors`), `envoygotest.New` (`envoy.filters.http.envoy_go_test`).

### Alternatives considered

- **(A) `init()`-based global** — REJECTED. Test isolation hard (each test wants its own filter set); ties filter-set composition to import-graph layout; contradicts the `*stats.Registry` LBP-1 precedent established in 06.1.
- **(B) Interface-injection without freeze (just a Lookup interface)** — REJECTED. The freeze-after-boot invariant is the load-bearing test for "no late filter registration"; without it, a future bug that calls `Register` post-boot fails silently rather than loudly.

### Consequences

- (a) All HCM constructors widen by one parameter (`*filter.HTTPRegistry`); pre-existing call sites in `cmd/envoy-go/main.go`, `internal/listener/manager.go`, and tests update mechanically (Decision §3.4 settles deletion of legacy constructors over forwarding shims).
- (b) The freeze-after-boot invariant is grep-verifiable in `registry_test.go` (`TestRegistry_PostFreezeRegisterPanics`).
- (c) Future Observability-family / xDS-family / WASM-family phases that introduce additional filter types extend this registry by registering their factories at boot — no architectural churn needed.

### Lands-in-task

Task 3 (the `internal/filter/http/registry.go` introduction). Supersedes nothing; complements ADR-0059.

---

## ADR-0073: typed_per_filter_config 3-tier merge model

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.5 (record durable design rationale).

### Context

Phase 07.1 introduces per-route filter configuration via Envoy's `typed_per_filter_config` map on Route, VirtualHost, and RouteConfiguration scopes. ADR-0041 (phases 04/05.1/05.2) silently-ignored these maps; phase 07.1's first real filter (cors) requires per-route policy to differ between routes (the cors differential at 0007a-cors needs different `CorsPolicy` per route).

### Decision

`typed_per_filter_config` is honored at parse-time on Route, VirtualHost, and RouteConfiguration scopes; merge order is **Route > VirtualHost > RouteConfiguration** with most-specific-override (no field-merge); lazy cache `map[cacheKey]proto.Message` on first `RequestRouteConfig()` call per request (cacheKey = filterName + routeIdx); build-time validation: keys MUST reference filter names present in the chain's `http_filters[]`; unknown filter names error at parse with `hcm: <location>: typed_per_filter_config: unknown filter name %q (chain has [...])`. **Honored at parse-time — partial supersession of ADR-0041's silent-ignore set:** `typed_per_filter_config` moves from silent-ignored to honored.

### Alternatives considered

- **(A) Eager `[]proto.Message` indexed by filter chain index** — REJECTED. Allocates one slot per filter even if no filter calls RequestRouteConfig — wasted in the common case.
- **(B) Field-level merge mode** — REJECTED. YAGNI in 07.1; no in-scope filter consumes the field-merge semantic; cors policy is most-specific-override regardless. Deferred to first family phase that demands it via Envoy-equivalent test.

### Consequences

- (a) The silently-ignored field set is amended (per ADR-0041's amendment shape, mirroring the 05.1 + 05.2 + 06.1 + 06.2 amendments) — `typed_per_filter_config` is REMOVED from the silent-ignore set on Route/VirtualHost/RouteConfiguration.
- (b) `filter.disabled` flag stays silent-ignored at parse-time (per SPEC §2.2 + §9; deferred to family phase that demands it).
- (c) Future fixtures that exercise field-level merge will land their own ADR superseding the most-specific-override discipline if needed.

### Lands-in-task

Task 4 (the `internal/filter/http/perroute.go` introduction). Supersedes nothing; **amends ADR-0041**.

## Amendment (per phase 10 ADR-0110)

The most-specific-override discipline codified above is now the DEFAULT model;
filters whose proto semantics demand multi-tier evaluation (e.g.,
envoy.filters.http.header_mutation per its `most_specific_header_mutations_wins`
flag) use `PerRouteConfig.ResolveAllTiers` per ADR-0110 — see that ADR for the
per-filter accessor-choice discipline. ADR-0073's wholesale-override semantics
remain authoritative for filters that opt into the most-specific accessor
(cors @ 07.1, fault @ 09).

---

## ADR-0075: `sendLocalReply` encode-chain semantics

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.3 (the synthesized-response shape is differentially observable; the empirical pin is the durable evidence) + D-3.5 (record the rationale durably).

### Context

When an HTTP filter calls `cb.SendLocalReply(status, body, headers)` from any callback (decode or encode side), the framework must synthesize a response and run it through the encode-side filter chain. The shape of "running through the encode-side filter chain" is differentially observable (it determines which filter's encoded headers/body are visible on the wire). Phase 07.1's `cors` filter is the first production filter that calls `SendLocalReply` (on the preflight path with `OPTIONS` requests + matching origin). The framework's discipline must replicate Envoy's behavior to ship a green differential at fixture 0007a-cors.

### Decision

When a filter calls `cb.SendLocalReply(status, body, headers)`:

(a) the chain marks decode-side aborted at the calling filter's index (cancels any pending resume);
(b) constructs the synthesized response (merging framework-injected standard headers — `content-length`, `content-type`, `date`, `server` — with the user-supplied headers; `date` + `server` are filled by the HCM wire-write path, NOT by `beginLocalReply`);
(c) enters the encode chain at `filter[len-1]` of the encode-side filter set (NOT at the calling filter's index, NOT at index 0);
(d) iterates the FULL encode chain in reverse order (every encode-side filter runs, INCLUDING the calling filter's own encode side);
(e) first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log line `hcm: filter %q called SendLocalReply after encode-side started; ignoring`.

The empirical pin in SPEC §11 #4 is the durable evidence (verified at SPEC time against reference Envoy v1.37.2 with chain `[lua_a, lua_b, lua_c, router]` where `lua_b` calls Envoy's `respond` API; observed encode order `lua_c → lua_b → lua_a` — i.e., entry at filter[len-1] of the encode-side set; ALL three Lua filters' encode sides ran).

### Alternatives considered

- **(A) Entry at the calling filter's encode index (NOT filter[len-1])** — REJECTED. Diverges from Envoy and would break differential equivalence on the cors filter's preflight path (cors is at filter[0]; if it called SendLocalReply, an entry-at-calling-index discipline would skip the router's encode side, breaking the encode-chain contract).
- **(B) Skip the calling filter's own encode side (since it produced the response)** — REJECTED per SPEC §12 #6 + §11 #4 empirical pin. Envoy uses (a): the calling filter's encode side runs.
- **(C) Parallel encode-side iteration on SendLocalReply (faster)** — REJECTED. Parallel iteration breaks the ordering contract (encode-side filters declare their order; a header-mutation filter at index 1 must observe and possibly modify what filter at index 2 emitted).

### Consequences

- (a) The `chain.beginLocalReply` implementation in `chain.go` (Task 7) honors the four sub-decisions (a–e) verbatim.
- (b) The unit test `TestChain_SendLocalReply_EntersAtLenMinus1` in `chain_test.go` asserts the encode-iteration entry point on a synthetic 4-filter chain.
- (c) The BEHAVIOR_CONTRACT addition at Task 23 carries the §11 #4 empirical-pin block verbatim (no drift permitted; the §11 block + the §13 block are paste-verbatim-synchronized).

### Lands-in-task

Task 7 (the `chain.go` `beginLocalReply` implementation; first use of the encode-chain-entry-at-`filter[len-1]` semantics in production code). Supersedes nothing.

---

## ADR-0076: Body buffer cap; 413 on decode overflow; reset on encode overflow

**Status:** Accepted
**Date:** 2026-05-01
**Doctrine:** D-3.5 (record durable rationale for the buffer cap + overflow disposition) + D-3.6 (the 413 wire shape is differentially observable; the §11 #3 empirical pin is the durable evidence).
**Amends ADR-0041** (the parse-time silent-ignore set is extended — see §"Decision" + §"Inline-supersession" below).

### Context

The HTTP filter framework's `FilterChain` per-stream state machine accumulates body chunks on the decode side when a filter returns `DataStopIterationAndBuffer`, and pipes encoded body chunks down the reverse encode chain when the upstream returns response data. Without an upper bound on per-stream buffer size, a hostile or malformed client can exhaust framework memory by sending an unbounded request body that filters opt to buffer (decode side) or by eliciting an unbounded response that has to traverse the encode chain (encode side). Reference Envoy v1.37.2 enforces a 1 MiB per-stream body-buffer cap by default and synthesizes a verbatim `413 Payload Too Large` response on decode-side overflow (verified at SPEC time — see §11 #3 empirical pin). On encode-side overflow Envoy resets the connection (H1: `connection: close` after the local reply emits, then conn close; H2: RST_STREAM after the local-reply HEADERS+DATA frames). envoy-go must match these disciplines to ship a green differential at fixture 0007a-cors and to pass the byte-shape pin at SPEC §11 #3 + the connection-reset semantics at §15 acceptance bullet 2.

The two configurable Envoy knobs that scale the cap (`per_connection_buffer_limit_bytes` on Listener, `per_request_buffer_limit_bytes` on Route) are out of scope for 07.1 (no production filter in 07.1's filter set — `cors` + `envoy_go_test` — needs them) and are deferred to a future buffer-policy phase or to whichever first family-phase that demands them.

### Decision

(a) **Hardcoded cap.** `internal/filter/http/chain.go` declares `const filterBufferLimitBytes = 1 << 20 // 1 MiB` matching Envoy's default. The constant is package-scoped (not exposed as a knob) — future-phase tunability is the dedicated buffer-policy phase's scope.

(b) **Decode-side overflow → 413 verbatim shape.** When `RunDecodeData` observes a chunk that would push `len(c.decodeBuf)+len(data)` above `filterBufferLimitBytes` on a `DataStopIterationAndBuffer` return, the chain synthesizes a local reply via `beginLocalReply` with: status `413`, body `"Payload Too Large"` (17 bytes ASCII; constant `localReply413Body`; no trailing newline per §11 #3 empirical pin), and a user-supplied header `Connection: close`. The framework's `beginLocalReply` then merges the framework-injected `Content-Length: 17` + `Content-Type: text/plain` (default) and runs the FULL encode chain in reverse declaration order per ADR-0075. The `Date` and `Server` headers are filled by the HCM wire-write path (per ADR-0075 (b) + §11 #3's "modulo `date` and `server`" footnote — those land on the wire-write layer, NOT in the framework's beginLocalReply).

(c) **Encode-side overflow → connection-reset sentinel.** When `RunEncodeData` observes a chunk that would push `c.encodeBufLen+len(data)` above `filterBufferLimitBytes`, it returns the package-private sentinel `errEncodeBufferOverflow` (with descriptive text `"chain: encode-side buffer overflow; resetting connection"`) WITHOUT iterating any encode-side filter on the overflowing chunk. The HCM dispatch path in `internal/filter/hcm/connection.go` (Task 15) and `internal/filter/hcm/h2dispatch.go` (Task 16) handles the sentinel: H1 closes the connection after writing whatever it has emitted; H2 emits RST_STREAM (`code: INTERNAL_ERROR`) and tears the stream down. The sentinel is package-private — the HCM layer imports `internal/filter/http` and uses `errors.Is(err, http.ErrEncodeBufferOverflow)` once Tasks 15 + 16 promote it to an exported alias if needed (or compares via the chain's RunEncodeData return convention). Note the asymmetry with (b): the decode-side cap gates *buffered* bytes on `DataStopIterationAndBuffer`, whereas the encode-side cap meters *total wire output* on every chunk regardless of filter status — a chain that returns `DataContinue` for every chunk of a multi-MB response still trips the reset.

(d) **Configurable knobs silently ignored.** Both `per_connection_buffer_limit_bytes` (Listener-scope) and `per_request_buffer_limit_bytes` (Route-scope) are silently ignored at parse-time per the ADR-0041 amendment in §"Inline-supersession" below. Build-time validation MUST NOT reject configs that set them; runtime behavior MUST honor only the hardcoded `filterBufferLimitBytes`.

### Inline-supersession

This ADR **amends ADR-0041** by extending the parse-time silent-ignore set with two new fields: `per_connection_buffer_limit_bytes` (Listener-scope) and `per_request_buffer_limit_bytes` (Route-scope). The amendment shape mirrors ADR-0073's amendment of the same ADR (`typed_per_filter_config` moves from silent-ignored to honored): ADR-0076 strictly EXTENDS the silent-ignore set; it does NOT remove any previously-ignored field. Per ADR-0045's inline-supersession discipline this amendment is recorded here at the amending ADR (not as a new note inside ADR-0041 itself; ADR-0041 carries a forward-pointer to the amenders).

### Alternatives considered

- **(A) Make `filterBufferLimitBytes` runtime-configurable via the Envoy proto knobs (`per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes`) at parse-time** — REJECTED for 07.1. The 07.1 filter set (`cors` + `envoy_go_test`) does not exercise the knobs; honoring them would add proto-traversal + per-stream cap-override plumbing that is dead code until a future filter-family phase needs it. Per ADR-0041's silent-ignore discipline + SPEC §6.5: defer to a future buffer-policy phase.
- **(B) Reset the connection on decode-side overflow (instead of synthesizing a 413)** — REJECTED. Diverges from Envoy's verbatim §11 #3 empirical pin, which emits a structured `413 Payload Too Large` response with `connection: close` BEFORE closing the conn. envoy-go's decode-side discipline must emit the structured response so the client can distinguish between "request body too large" (413) and "transport error" (raw close). The encode-side overflow CAN reset because by definition the upstream response is mid-stream and a structured response no longer fits the wire-protocol state.
- **(C) Synthesize a 503 (or other status) on encode-side overflow instead of resetting** — REJECTED. Envoy resets at this point per the §11 #3 footnote; emitting another status would diverge differentially. Additionally, on H2 the HEADERS frame for the response may already have been flushed by the time the overflow is detected, making a "synthesize a different status" path semantically impossible without retroactively rewriting the wire.
- **(D) Use a watermark / streaming-body model (StopAllIterationAndWatermark) instead of a hard cap** — REJECTED for 07.1 per SPEC §2.1 non-goal. Watermark backpressure is deferred to the first HTTP-filter-family phase that demands it (likely a streaming-body filter like compression).

### Consequences

- (a) `internal/filter/http/chain.go` declares `localReply413Body = "Payload Too Large"` + `errEncodeBufferOverflow` at the head of the file (Task 9). The `RunDecodeData` method implements the cap check + 413 synthesis path; `RunEncodeData` implements the encode-side cap check + sentinel return.
- (b) The unit tests `TestChain_DecodeData_OverflowSynthesizes413` (verbatim wire shape) + `TestChain_DecodeData_BelowCapDoesNotSynthesize` (body-cap-respected-on-non-overflow) + `TestChain_EncodeData_OverflowReturnsSentinel` (sentinel returned at the boundary) + `TestChain_EncodeData_BelowCapNoSentinel` assert the framework-side discipline. The HCM-side wire emission (close conn / RST_STREAM) is covered by Tasks 15 + 16 integration tests.
- (c) The BEHAVIOR_CONTRACT.md `## HTTP filter chain` section landed at phase-done (Task 23) carries the §11 #3 empirical-pin block verbatim, paste-synchronized with SPEC §11.3 (no drift permitted; future image bumps require updating both in the same commit per ADR-0052).
- (d) Future filter-family phases that need configurable caps (e.g., a streaming-compression filter that buffers more than 1 MiB legitimately) author a follow-up ADR that promotes `per_connection_buffer_limit_bytes` + `per_request_buffer_limit_bytes` from silent-ignored to honored — mirroring ADR-0073's promotion path for `typed_per_filter_config`.

### Lands-in-task

Task 9 (the `chain.go` `RunDecodeData` + buffer-overflow path + encode-overflow sentinel; first use of the framework's body-cap discipline). Amends ADR-0041; supersedes nothing.

---

## ADR-0074: `envoy.filters.http.cors` — SendLocalReply discipline + encode-side header injection

**Status:** Accepted. **Date:** 2026-05-01. **Doctrine:** D-3.3 + D-3.4. **Anchored:** SPEC §11.2 (verbatim wire-shape pin), §4.1 (filter inventory).

### Context

Phase 07.1 ships TWO filters in addition to the terminal `router`: the real-world differential filter (`envoy.filters.http.cors`) and the test-only structural-coverage probe filter (`envoy.filters.http.envoy_go_test`). ADR-0074 codifies the cors filter's behavioral contract; the probe filter's iteration-state coverage attribution is documented separately in PROGRESS Task 19.

The cors filter's behavior is non-trivial because it exercises THREE iteration-protocol surfaces simultaneously: (1) decode-side preflight detection + `SendLocalReply` for allowed-origin OPTIONS requests; (2) decode-side passthrough for disallowed-origin preflights (router 405s); (3) encode-side header injection on actual non-preflight responses when the request had an allowed Origin. The empirical wire-shape was scraped from reference Envoy v1.37.2 (per SPEC §11.2 four probes) and pinned in the spec; the filter implementation MUST match the verbatim header order on preflight responses and the verbatim header subset on actual responses.

### Decision

**(a) Filter set.** Phase 07.1 ships `envoy.filters.http.cors` (real Envoy filter, used by differential fixture `0007a`) + `envoy.filters.http.envoy_go_test` (test-only probe filter, used by structural fixture `0007b`). The `Cors` + `CorsPolicy` proto types are pulled from `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3` per SPEC §4.1 (proto types only; no runtime helpers); the `envoygotest` proto schema is envoy-go-only (hand-rolled, not in upstream go-control-plane). No new go-control-plane runtime imports.

**(b) Cors decode-side discipline (per SPEC §11.2 empirical pin).**

- **No Origin header:** filter is a no-op (`Continue`); not a CORS request.
- **Origin present + method=OPTIONS + `Access-Control-Request-Method` present:** preflight detected.
  - **Origin allowed by per-route `CorsPolicy`:** synthesize `200 OK` with empty body via `dcb.SendLocalReply(200, "", corsHeaders)`. The `corsHeaders` map carries the SIX CORS preflight response headers in `§11.2` verbatim order: `Access-Control-Allow-Origin`, `Access-Control-Allow-Credentials`, `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, `Access-Control-Max-Age`, `Access-Control-Expose-Headers`. Returns `StopIteration`.
  - **Origin disallowed:** filter does NOT inject a 4xx local reply. The request passes through to the router, which 405s the OPTIONS request (since standard route configs don't accept OPTIONS). This matches v1.37.2's empirical behavior pinned in §11.2 probe (b).
- **Origin present + non-preflight (actual GET/POST/etc.):** filter records `originAllowed` + `matchedOrigin` for encode-side use; returns `Continue` (passthrough to router).

**(c) Cors encode-side discipline (per SPEC §11.2 probe (c)).**

- If `originAllowed` was set during decode, encode-side injects THREE response headers on the upstream response: `Access-Control-Allow-Origin: <matchedOrigin>`, `Access-Control-Allow-Credentials: true` (when policy allows), `Access-Control-Expose-Headers: <policy.expose_headers>` (when set). Allow-Methods / Allow-Headers / Max-Age are PREFLIGHT-ONLY and MUST NOT appear on actual responses.
- No-op (no header injection) when origin was not allowed or not present.

**(d) Per-route policy resolution.** Cors reads its `*CorsPolicy` via `dcb.RequestRouteConfig()` which delegates to the chain's `*PerRouteConfig.Resolve(filterName, routeIdx)` (3-tier merge per ADR-0073). When no per-route config is set, the policy is empty → no origins allowed → filter is effectively disabled. Per-route override is the primary deployment shape for cors per the v1.37.2 fixture pattern.

**(e) Origin matcher support.** Phase 07.1 supports the THREE matcher variants exercised by reference Envoy's CORS examples: `exact`, `prefix`, `suffix`. Other StringMatcher variants (`safe_regex`, `contains`, `ignore_case`) are silently treated as no-match — extension to the full StringMatcher surface is deferred per the silent-ignore discipline of ADR-0041.

### Inline-supersession

None. ADR-0074 is additive; no prior ADR is amended or superseded.

### Alternatives considered

- **(A) Synthesize a 403 (or other 4xx) on disallowed-origin preflight instead of passthrough.** REJECTED. v1.37.2 empirical scrape (probe b) shows 405 Method Not Allowed coming from the ROUTER (not cors), confirmed by the `x-envoy-upstream-service-time` header on the disallowed-origin response (which would not be present if cors had short-circuited the request). Differential equivalence requires envoy-go's cors filter to passthrough on disallowed origin.
- **(B) Inject all six CORS headers on actual responses (not just three).** REJECTED. v1.37.2 probe (c) shows ONLY three headers on actual responses; allow-methods/allow-headers/max-age are preflight-only per spec. Differential equivalence requires the three-header-only shape.
- **(C) Hand-roll the CorsPolicy proto type.** REJECTED. The upstream `envoy/extensions/filters/http/cors/v3.CorsPolicy` type is already in `github.com/envoyproxy/go-control-plane`; D-3.2 forbids new runtime helpers but allows proto types. Using the upstream type avoids drift on a non-trivial schema (10 fields).
- **(D) Implement the `filter_enabled` / `shadow_enabled` runtime fractions on CorsPolicy.** REJECTED for 07.1. The 07.1 differential fixture exercises only the always-on path; the runtime-fraction surface is silent-ignored per the ADR-0041 discipline. Future runtime-fraction phases promote both fields from silent-ignored to honored.

### Consequences

- (a) `internal/filter/http/cors/cors.go` (~190 LoC) implements the three-decision shape above. Six unit tests in `cors_test.go` exercise the four wire-shape paths (preflight allowed, preflight disallowed, actual allowed, actual no-origin) + the per-route override shape + the factory roundtrip via the package's `New` HTTPFilterFactory.
- (b) `internal/filter/http/cors.TypeURL = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"` — boot-time registration in `cmd/envoy-go/main.go` (Task 20) registers `cors.New` under this key in the `*HTTPRegistry` per ADR-0072.
- (c) The differential fixture `0007a-cors` (Task 21) drives 4 sequential requests against both proxies and asserts per-request equivalence (modulo the standard `Server` / `Date` / `Content-Length` differential ignore-list).
- (d) Phase 07.1 Task 18 introduces TWO infrastructure prereqs that land alongside cors: P1 (the `router.Action` 4-tuple is reduced to the 3-tuple `(ActionResponse, picked, err)` so the chain owns wire-byte accounting); P2 (the encode chain is wired through HCM dispatch — `dispatchRequest` / `chainDispatchAction.WriteH2` run `RunEncodeHeaders` + `RunEncodeData` over the action's response BEFORE the wire-write fires, so cors's encode-side header injection takes effect on actual responses). The wire-write moves from inside the action closure into HCM dispatch via `writeH1Reply` (codec.go) + `writeH2Reply` (h2dispatch.go). These prereqs are intrinsic to cors's encode-side discipline; without P2 the encode-chain mutation has no effect on the wire.

### Lands-in-task

Task 18 (cors filter) + Tasks 21 (differential fixture 0007a) + the Task-18 prereqs P1 + P2 (wire-byte accounting refactor + encode chain wiring through HCM dispatch). The probe filter `envoy.filters.http.envoy_go_test` lands separately at Task 19 + Task 22; ADR-0074's coverage attribution table for the eight iteration-state modes is documented in Task 19's PROGRESS entry, not in this ADR (which is cors-scoped).

---

## ADR-0077: Phase-07.2 scope decision (split confirmation + listener-filter MVP boundary)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (decisions are written) + D-3.6 (every phase is a green build).

### Context

Phase 07 (filter-chain framework, BOOTSTRAP §8 row 07) was split into two sub-phases at planner-time per ADR-0070: 07.1 covered the HTTP filter framework (anchored under `internal/filter/http/` + `internal/filter/hcm/`); 07.2 covers the listener-chain completion (anchored under `internal/listener/`). The two halves share no production-code surface; they share only the BOOTSTRAP §8 row identifier. 07.1 closed at master `424485b` with all seven of its anticipated ADRs (ADR-0070..ADR-0076) landed and ROADMAP row 07.1 → done; the parent ROADMAP row 07 stayed at `in-progress` per ADR-0070's documented closure pattern, awaiting 07.2's phase-done commit.

Per the parent BRAINSTORM §1 and parent SPEC §3, the 07.2 sub-phase's design was first explored in the BRAINSTORM session, then narrowed in the SPEC drafting session (master `bb5f437`). The SPEC drafting session landed three concrete design decisions in addition to the ROADMAP edit (row 07.2 `planned → in-progress`):

1. **Listener-filter framework scope** — a new `internal/listener/listenerfilter/` package with `ListenerFilter` interface, `Pipeline` per-connection state machine, `*ListenerFilterRegistry` (mirroring 07.1 ADR-0072's threaded-constructor LBP-1 discipline), `ChainMatchInputs` carrier, and `Peeker` peek-buffer wrapper.

2. **`FilterChainMatch` algorithm scope** — full 8-dimension algorithm (`destination_port`, `prefix_ranges`, `server_names`, `transport_protocol`, `application_protocols`, `source_type`, `source_prefix_ranges`, `source_ports`) with priority-ordered specificity and `default_filter_chain` no-match fallback. Phase 03's ADR-0033 narrowing (SNI-only) is partially superseded.

3. **Concrete listener filter set** — one filter at MVP: `envoy.filters.listener.tls_inspector` (peeks ClientHello, contributes SNI + ALPN to `ChainMatchInputs`). `original_dst`, `proxy_protocol`, and `http_inspector` are explicitly deferred per Decision F (§5 of the SPEC) on the rationale that the MVP dispatch pipeline is fully exercised by `tls_inspector` alone and adding additional filters later is purely additive (a new package + a new Register call at boot).

The ROADMAP edit landed at the SPEC commit (`bb5f437`); 07.2 PLAN Task 1 (this PROGRESS preamble) is the first opportunity to land the formal scope-decision ADR after the SPEC commit's ROADMAP edit, mirroring ADR-0070's anchoring at 07.1 PLAN Task 1.

### Decision

Phase 07.2 ships the following three deliverables as scoped in SPEC §1 #1, #2, #4, #5, #6:

(a) **Listener-filter framework.** A new `internal/listener/listenerfilter/` package with: the `ListenerFilter` interface (single method `Inspect(ctx, peeker, inputs) (Status, error)` + `OnDestroy()`); a 2-state `ListenerFilterStatus` enum (`Continue`, `StopIteration`); a `ChainMatchInputs` struct holding the eight chain-match dimensions populated incrementally by listener filters and the connection-level facts already known at accept time; a `Peeker` interface + `peekerConn` concrete implementation that buffers reads internally so bytes consumed by the inspector are NOT consumed from the perspective of the downstream filter chain; a `*ListenerFilterRegistry` threaded constructor with Register / Lookup / Freeze (mirrors 07.1's `*HTTPRegistry` per ADR-0072 + 06.1 LBP-1 from ADR-0059); a `Pipeline` per-connection state machine (sequential dispatch — current filter must finish before the next one starts; no async-resume at MVP); and the two-step factory pattern (`ListenerFilterFactory` parses + validates `typed_config` once at HCM-build time; `FilterInstanceFactory` allocates per-connection). The full dispatch-protocol shape is recorded in ADR-0079.

(b) **Full 8-dimension `FilterChainMatch` algorithm.** Phase 03's ADR-0033 narrowed `filter_chain_match` to `server_names` only (errors at parse on any other dimension). 07.2 expands to all eight dimensions (`destination_port`, `prefix_ranges`, `server_names`, `transport_protocol`, `application_protocols`, `source_type`, `source_prefix_ranges`, `source_ports`). The eighth field `direct_source_prefix_ranges` (proxy-protocol original-source-IP) is silently ignored (deferred to a future xDS / proxy-protocol family phase). The chain-match precedence algorithm is priority-ordered specificity (most-specific-wins across the eight dimensions in their documented priority list) with eligibility-then-specificity 2-pass scoring. The algorithm's specifics are recorded in ADR-0081; `default_filter_chain` semantics are recorded in ADR-0080; the ADR-0033 supersession enumeration is recorded in ADR-0078.

(c) **`Listener.default_filter_chain` honored.** Phase 03's listener manager errored at parse if `default_filter_chain` was set; 07.2 honors it as the no-match fallback. An empty-match chain in `filter_chains[]` BEATS `default_filter_chain` when both coexist (per SPEC §11.2 empirical pin). The `default_filter_chain` may carry an independent `transport_socket` (TLS or plaintext) regardless of the `filter_chains[]` entries' TLS posture. Recorded in ADR-0080 (which supersedes ADR-0033 clause 3).

**Explicit deferrals** (out of scope for 07.2; each documented in SPEC §2):

- `envoy.filters.listener.original_dst` — deferred per Decision F. Rationale: the MVP dispatch pipeline is fully exercised by `tls_inspector` alone — `tls_inspector`'s contribution to `ChainMatchInputs.ServerName` + `.ApplicationProtocols` exercises the same dispatch surface `original_dst`'s contribution to `.DestinationPort` would. Future-phase pointer: a dedicated transparent-proxy phase OR the network-filters family.
- `envoy.filters.listener.proxy_protocol` — deferred. Future-phase pointer: bundled with the `direct_source_prefix_ranges` chain-match dimension.
- `envoy.filters.listener.http_inspector` — deferred (phase 05.1 ADR-0050 covers TLS H1-vs-H2; plaintext H1-vs-H2 is `http_inspector`'s niche).
- `direct_source_prefix_ranges` chain-match dimension — silently ignored at parse time. Future-phase pointer: bundled with the proxy-protocol filter phase.
- xDS LDS dynamic listener filter / chain updates — out of scope.
- Listener-level access logging on chain-match-miss — silently ignored at parse.
- Per-listener-filter metrics — none at MVP; 06.1 stats discipline supports adding them later (3-LoC per-call site change).

This ADR mirrors ADR-0070's 07.1 scope-confirmation pattern. **Anchors the ROADMAP edit landed at the SPEC commit (row 07.2 → in-progress)** — which is the 07.1 REVIEW I-3 corrected pattern continued.

### Alternatives considered

(A) Ship `original_dst` alongside `tls_inspector` at MVP. Rejected per Decision F: the dispatch pipeline is filter-agnostic (adding `original_dst` later is purely additive — a new package + a new Register call); `original_dst`'s deployment niche (transparent proxying behind iptables `REDIRECT` rules) is not on the BOOTSTRAP MVP trunk; including it would inflate the 07.2 task count without exercising new dispatch-pipeline code paths.

(B) Defer `default_filter_chain` (keep ADR-0033 clause 3's parse-time error in force at 07.2). Rejected: the parent ROADMAP row 07 explicitly enumerates `Listener.default_filter_chain` honored as one of the closure deliverables; deferring would leave row 07 incomplete on a load-bearing primitive.

(C) Ship the full `direct_source_prefix_ranges` dimension (i.e., honor it without the proxy-protocol filter). Rejected: honoring the dimension without the filter would be a no-op that confuses future readers (the dimension would never match because the source IP at the dispatch layer is always the L4 connection peer, never the proxy-protocol-recovered original source). Bundling the dimension with its enabling filter is cleaner.

### Consequences

(a) The phase 07.2 ROADMAP row carries `status: in-progress` until phase-done; both rows 07.2 and 07 flip to `done` AT THE SAME COMMIT at 07.2's phase-done (per ADR-0070 (b)).

(b) The seven 07.2 ADRs (ADR-0077..ADR-0083) are 07.2-scoped. ADR-0077 + ADR-0083 land at PLAN Task 1 (this PROGRESS preamble; ADR-0083 is paired here per SPEC §10's "lands wherever the integration is documented (likely the PROGRESS preamble; this ADR is mainly explanatory)"); ADR-0079 lands at Task 2; ADR-0082 at Task 4; ADR-0080 + ADR-0081 at Task 5 (paired); ADR-0078 at Task 9. The non-monotonic commit-time mapping (0077, 0083, 0079, 0082, 0080, 0081, 0078) is explicitly permitted per SPEC §10 and per the 05.2 + 06.1 + 06.2 + 07.1 precedents.

(c) The `internal/listener/listenerfilter/` package is the listener-side analog of 07.1's `internal/filter/http/` package: similarly small (~400-600 LoC of new machinery), similarly anchored on a freeze-after-boot threaded registry, similarly using the two-step factory pattern. Future family phases that introduce additional listener filters (e.g., `original_dst`, `proxy_protocol`) extend this package by adding the new filter package + Register call; each such addition lands its own ADR in the phase that needs it.

(d) The differential fixture surface at 07.2 close adds one new fixture (`0008-listener-chain-match`) introducing two new harness primitives: the `MultiListener` interface (a single subject/reference proxy pair binds two distinct listeners) and the `AlternateConfig` interface (a fixture's connection-`i` step swaps to a config variant — used by connection 4's `chain_other` removal). Pre-existing fixtures (0000–0007b) remain green without bootstrap changes (07.2's surface is additive on the listener-side).

### Lands-in-task

07.2 PLAN Task 1 (PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at master `bb5f437` per SPEC drafting). Supersedes nothing.

---

## ADR-0083: ADR-0050 disposition (no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (durable rationale for the non-supersession decision).
**Settles:** ADR-0050 (ALPN-driven codec selection inside `Filter.Handle`).

### Context

ADR-0050 (phase 05.1) decided that ALPN-driven codec selection happens **inside `Filter.Handle`** — the HCM type-asserts on `*tls.Conn` and reads `NegotiatedProtocol` to decide whether to dispatch the connection through the H1 codec (`runConnection`) or the H2 codec (`runH2`). The dispatch decision is made post-handshake, INSIDE the chain's terminal filter, and is independent of any listener-side chain-match consultation of ALPN.

Phase 07.2 introduces the `application_protocols` chain-match dimension on `FilterChainMatch` (SPEC §1 #4) — which IS a listener-side consultation of ALPN: at chain-match time (after the listener-filter pipeline runs but before the chain's terminal filter dispatches), the algorithm scores each chain whose `application_protocols` is set against the `ChainMatchInputs.ApplicationProtocols` populated by `tls_inspector`. So the question naturally arises (SPEC §2.5): does 07.2's `application_protocols` chain-match field SUPERSEDE ADR-0050's HCM-internal ALPN dispatch? Should ALPN dispatch move entirely from HCM-internal to chain-match, with ADR-0050 retired?

This ADR settles the question per Decision H (SPEC §14) by recording the orthogonality argument and explicitly preserving ADR-0050.

### Decision

ADR-0050 stays in force. 07.2's `application_protocols` chain-match field and ADR-0050's HCM-internal ALPN dispatch are **orthogonal mechanisms**:

- **ADR-0050's HCM-internal ALPN dispatch governs codec-selection** — which Go-level codec implementation runs the request: phase-04's `runConnection` for H1, phase-05.1's `runH2` for H2. The dispatch happens AFTER the TLS handshake completes, INSIDE the chain's terminal HCM filter, on a `codec_type: AUTO` HCM. It is independent of which chain was selected.

- **07.2's `application_protocols` chain-match governs chain-selection** — which `filter_chain` entry runs at all. The match happens BEFORE the TLS handshake completes (the `tls_inspector` listener filter peeks the ClientHello and populates `ChainMatchInputs.ApplicationProtocols` from the ALPN extension), at chain-match time, on the listener manager's pre-handshake dispatch path. It selects between distinct `filter_chain` entries which may carry distinct codec-type configurations.

The two coexist by construction:

- A user can deploy a single listener with one filter chain whose terminal filter is an HCM with `codec_type: AUTO` → ADR-0050's mechanic fires; 07.2's `application_protocols` is empty (or matches everything); behavior is unchanged from 05.1.
- A user can deploy a listener with two chains, one matched on `application_protocols: [h2]` (terminal HCM with `codec_type: HTTP2`) + one on `application_protocols: [http/1.1]` (terminal HCM with `codec_type: HTTP1`) → 07.2's chain-match selects between them; ADR-0050's HCM-internal dispatch is a no-op because each chain's HCM has a forced `codec_type` (the `AUTO` branch never fires).
- A user can deploy a listener with two chains where the chain-match is on a DIFFERENT dimension (e.g., `prefix_ranges`) and BOTH chains' terminal filters are `codec_type: AUTO` HCMs → 07.2's `application_protocols` is empty; chain-selection runs on the prefix-range dimension; codec-selection runs per-chain via ADR-0050 inside each chain's HCM. The two mechanisms operate on independent axes of the dispatch pipeline.

The orthogonality argument is the cleanest representation of the intended dispatch pipeline. The empirical-pin obligation that confirms the dispatch interaction (SPEC §11.4 carry-forward, Decision K) is resolved at 07.2 PLAN Task 16 (the fixture-0008 driver task) — the executor produces the verbatim Envoy v1.37.2 evidence by spawning a TLS bootstrap with `tls_inspector` + multi-chain `application_protocols` matching, captures the per-chain `tcp.tcp_(h2|h1).downstream_cx_total` stats, and pastes the output verbatim into both SPEC §11.4 (replacing the carry-forward placeholder) and `BEHAVIOR_CONTRACT.md ## Listener filters` at Task 17.

### Alternatives considered

(A) Supersede ADR-0050 — move ALPN dispatch entirely into the chain-match algorithm; retire ADR-0050. Rejected: ADR-0050 covers the AUTO-codec case which is independent of chain-match. A single-chain listener with no `application_protocols` still needs ADR-0050's HCM-internal mechanic to choose between H1 and H2 codecs based on the negotiated ALPN. Forcing every TLS deployment to use multi-chain + per-chain forced `codec_type` to get H1/H2 dispatch would be a needless deployment-config blowup AND would require deprecating the `codec_type: AUTO` path entirely.

(B) Supersede ADR-0050 partially — chain-match preferred when both could fire (e.g., a listener with multi-chain `application_protocols` matching AND `codec_type: AUTO` HCMs). Rejected as confusing: the orthogonality is cleaner. With per-chain forced `codec_type`, the `AUTO` branch is a structural no-op (the HCM never exercises the type-assert path because the codec is statically known); with no `application_protocols` matching, ADR-0050 fires per-chain. There's no scenario where both fire on the same connection; partial supersession would create a mental model where the implementer has to reason about precedence between the two mechanisms when in practice the configuration shape determines which fires.

(C) Add an explicit configuration knob (e.g., a listener-level `prefer_chain_match_alpn` bool) to disambiguate. Rejected: the configuration shape already disambiguates (per-chain `codec_type` vs `AUTO`); adding a knob would be redundant and would invite bug reports about "knob set wrong" misconfigurations.

### Consequences

(a) ADR-0050 is preserved verbatim. The HCM-internal ALPN dispatch path (`Filter.Handle` type-asserting on `*tls.Conn`) stays in force unchanged. No code change to `internal/filter/hcm/` is anchored by this ADR.

(b) `BEHAVIOR_CONTRACT.md ## TLS "Scope boundaries"` enumeration is amended at 07.2 phase-done (Task 17): "ALPN-driven filter-chain selection" is REMOVED from the out-of-scope list (07.2 ships it via `application_protocols` chain-match); "ALPN-driven codec selection inside Filter.Handle" REMAINS in scope and is still asserted (this is ADR-0050's purview). The new `## Listener filters` section that lands at Task 17 carries the §11.4 ALPN-dispatch empirical-pin block that resolves the carry-forward at Task 16.

(c) Future xDS phases that revisit ALPN dispatch consult both ADRs — ADR-0050 for the AUTO-codec single-chain case; ADR-0083 for the multi-chain `application_protocols` case. Should a future xDS / matcher API phase introduce a unified-matcher API that absorbs both cases, that phase's ADR would explicitly supersede both — but until then, the orthogonality is the durable contract.

### Lands-in-task

07.2 PLAN Task 1 (PROGRESS preamble alongside ADR-0077; this ADR is mainly explanatory and doesn't anchor a code change — pairing it with ADR-0077 at T1 keeps the PROGRESS preamble's ADR list cohesive). Settles ADR-0050; supersedes nothing.

---

## ADR-0079: Listener-filter dispatch protocol shape (sync-only; narrow `Inspect(peeker, inputs)` surface; freeze-after-boot registry; two-step factory pattern)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.2 (write from scratch when the surface has no Envoy-FFI legacy to mirror) + D-3.5 (record durable design rationale; the dispatch-protocol shape is a contract that future listener-filter implementers MUST observe).

### Context

Phase 07.2 introduces the listener-filter framework — `internal/listener/listenerfilter/` — which dispatches a per-connection pipeline of `ListenerFilter` instances over a peek-without-consume `net.Conn` wrapper, populating a `ChainMatchInputs` struct that the chain-match algorithm consults to select a `filter_chain` entry. SPEC §6 raised the question of the dispatch-protocol shape: how narrow should the `ListenerFilter` interface be, what status enum carries the result, how is the registry threaded, and how is per-connection allocation paid for?

The project has two parent patterns that establish the registry-shape precedent:
- **ADR-0072** (07.1's `*filter.HTTPRegistry`) — boot-populated, freeze-after-boot, threaded explicitly into HCM construction; no package-global `init()` registration; late `Register` panics loudly. The two-step factory pattern (HTTPFilterFactory parses the typed_config Any once at boot; FilterInstanceFactory allocates per-request) is the 07.1 precedent for filter parsing.
- **ADR-0059** (06.1's `*stats.Registry` LBP-1) — the original freeze-after-boot construct that ADR-0072 mirrors.

Listener filters have a much narrower surface than HTTP filters: they peek the connection's byte preamble (read without consuming), populate fields on a chain-match-inputs struct, and return Continue/StopIteration. They do not buffer bodies, do not iterate over headers, do not call back into the connection manager, and do not need to support async-resume because the operations they perform (peek + populate) are CPU-bound and bounded by `peekerBufSize`. The MVP register only `tls_inspector` (Decision D from SPEC §1 #1), but the framework supports multi-filter pipelines (sequential dispatch).

### Decision

The listener-filter dispatch protocol is shaped per the following decisions, all anchored at this ADR:

- **Sync-only dispatch (Decision A from SPEC §6.1).** `ListenerFilter.Inspect(ctx, peeker, inputs) (Status, error)` is synchronous; no async-resume token, no goroutine-park, no callback registration. A filter that needs more bytes than were peeked returns `StopIteration` (or aborts via error). The peek buffer is large enough (4096 default) that no realistic listener filter needs to "wait for more data" — the inputs the filter consults are bounded by the peek buffer size.

- **`ListenerFilter` interface — single `Inspect` method + `OnDestroy()`.** No `SetCallbacks`, no `OnNewConnection`, no `OnConnectionEvent`. The two-method surface is the minimum viable contract: `Inspect` is the work; `OnDestroy` releases per-connection resources (closed-over file handles, etc.) when the pipeline ends (either after dispatch completes or on connection close before dispatch).

- **`ListenerFilterStatus` 2-state enum.** `Continue = 0` (advance to next filter); `StopIteration = 1` (halt the pipeline; chain-match runs on partial inputs). No `WaitForData`, no `Pause`, no `ResumeWithBytes`. The 2-state enum is sufficient because the peek-buffer is bounded and filters that need more bytes than peeked simply abort.

- **`*ListenerFilterRegistry` threaded constructor (mirrors ADR-0072 + ADR-0059).** The registry is allocated at boot in `cmd/envoy-go/main.go`, threaded explicitly into the listener-manager's construction path, NOT a package-global registered via `init()`. Freeze-after-boot invariant: any post-`Freeze` `Register` panics loudly; `Lookup` is read-allowed. The 07.2 MVP registers only `tls_inspector` at boot.

- **Two-step factory pattern (mirrors ADR-0072).** `ListenerFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` parses + validates the `typed_config` Any once at `NewManager`-build time and returns a per-connection `FilterInstanceFactory func() ListenerFilter` closure. Per-config validation cost is paid once at boot; per-connection cost is one allocation. The `FactoryCtx` carrier is currently empty but reserved for future extensions (e.g., a Registry pointer for filters that compose).

- **Per-connection sequential dispatch (Decision D from SPEC §1 #1).** The MVP supports multi-filter pipelines (sequential dispatch through `filters[]` in the order they appear in the listener config). Sequential dispatch is intentional: per-filter goroutines would be goroutine-bloat (each accepted connection already has a goroutine). The SPEC's `pipeline_test.go` exercises a 2-filter case using two `tls_inspector` instances with different `initial_read_buffer_size` values — no test-only filter type pollutes the production package.

- **4096-byte default peeker buffer (Decision C from SPEC §5.3); clamped [256, 65536] via `tls_inspector.initial_read_buffer_size` proto override.** 4096 matches Envoy's `tls_inspector.initial_read_buffer_size` default. Lower bound 256 is safely above the minimum ClientHello (~50 bytes); upper bound 65536 is the maximum useful peek size before a TLS record would have to span multiple records (TLS record max is 16384, so 65536 covers four records). The clamp is implemented in `NewPeekerConnSize` (silent clamp) and at proto-config parse time in `tls_inspector/proto.go` (parse-error on out-of-range values; both checkpoints land at later 07.2 tasks).

### Alternatives considered

- **(A) Envoy's full listener-filter API (with `SetCallbacks`-style callback registration + watermark-aware buffered I/O + async-resume tokens).** REJECTED for YAGNI. The methods we drop have no in-scope callers in 07.2's filter set: `tls_inspector` does not need watermarks, does not need callback registration, does not need async-resume. Any future listener filter that needs them would re-litigate via its own ADR — at which point the framework can be extended additively. Adding the machinery now would commit to a dispatch-protocol shape that no in-scope filter uses; that is unjustified surface area.

- **(B) Per-filter goroutine (each filter runs in its own goroutine, communicates via channels).** REJECTED. Spawning a goroutine per filter per connection is goroutine-bloat: each connection already has a goroutine for the accept-loop dispatch, and the dispatch is CPU-bound (peek + parse + populate). Channel-based dispatch would add latency without buying any throughput because the pipeline is intrinsically sequential (each filter potentially mutates `ChainMatchInputs`, which is the single channel of communication). Per-pipeline `context.WithTimeout` (ADR-0082) provides the timeout discipline without per-filter goroutines.

### Consequences

- (a) The framework's external dependencies are limited to the Go stdlib (`bufio`, `context`, `net`) + `google.golang.org/protobuf` (for the `anypb.Any` typed_config carrier) + the `tls_inspector v3` proto package (for the only registered filter). No third-party listener-filter library; no cgo; no Envoy-extension Go binding. The framework is self-contained.

- (b) The dispatch-protocol shape is documented in `internal/listener/listenerfilter/doc.go` (the package-level doc-comment landed alongside this ADR) — future readers consult `doc.go` first, then this ADR for rationale.

- (c) Future family-phase listener filters (e.g., `original_dst`, `proxy_protocol`, `http_inspector`) extend this package by adding to the `ListenerFilter` interface (or, more likely, by adding to the registry without changing the interface — the `Inspect(peeker, inputs)` surface is intentionally generic over the inputs the filter populates). Each such addition lands its own ADR in the family phase that needs it; no architectural churn at 07.2's expense.

### Lands-in-task

07.2 PLAN Task 2 (the `internal/listener/listenerfilter/` package introduction; specifically the `doc.go` + `types.go` + `callbacks.go` + their tests). Supersedes nothing; complements ADR-0072 (HTTPRegistry pattern) and ADR-0059 (stats Registry LBP-1).

---

## ADR-0082: `listener_filters_timeout` honored in [1s, 60s]; default 15s; `continue_on_listener_filters_timeout` honored

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (honor proto fields widely used in real Envoy deployments) + D-3.6 (record durable design rationale; the timeout envelope + per-pipeline shared-budget mechanic is a contract that future tuning phases MUST consult).

### Context

Phase 07.2 introduces the per-connection listener-filter pipeline (`internal/listener/listenerfilter/pipeline.go`). The Envoy `Listener` proto carries a `listener_filters_timeout` field (a `google.protobuf.Duration`) that bounds how long the listener-filter pipeline may run on an accepted connection before the dispatch path either aborts the connection or proceeds to chain match with partial inputs (gated by the sibling `continue_on_listener_filters_timeout` boolean). SPEC §6.5 raised the question of how the timeout is enforced: per-filter (each filter gets its own budget) or per-pipeline (all filters share one budget); what the validation envelope is at parse time; and what the default is when the proto field is unset.

### Decision

The `listener_filters_timeout` proto field is honored, with values clamped/validated to a `[1s, 60s]` envelope. The default is `15s` when the field is unset (zero-valued duration). Values outside the envelope error at parse time with the message `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope` (matching the rest of `internal/listener/manager.go`'s error-message conventions, which all begin with `listener: %q:`).

The `continue_on_listener_filters_timeout` sibling field is honored as-is per the proto's documented semantics: `false` (the proto default) → on timeout the dispatch path aborts the connection (no chain match runs); `true` → on timeout the listener-filter pipeline is treated as having returned `Continue` and chain match proceeds against whatever inputs were populated before the deadline. Task 9 wires the dispatch-path branch on the `continue_on_listener_filters_timeout` value; Task 4 (this ADR's lands-in task) implements the timeout-detection mechanic that Task 9 consumes.

The timeout is enforced as a **per-pipeline shared budget** (per Decision N from SPEC §6.5 — NOT per-filter time-slicing). A single `context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)` is established once before the pipeline loop and shared across all filters' `Inspect` calls. A slow first filter eats subsequent filters' budget; the second filter sees an already-expired ctx if the first filter consumed the entire budget. This is correct: the user's `listener_filters_timeout` budget is the budget for all listener filters combined on a given connection.

### Alternatives considered

- **(A) Per-filter timeout (each filter gets the full budget independently).** REJECTED per Decision N. Per-filter timeouts would force `len(filters)` `context.WithTimeout` derivations per accepted connection (allocations on a hot path) AND would unfairly penalize multi-filter pipelines (a 3-filter pipeline with a 15s budget would have 45s of cumulative budget while a 1-filter pipeline gets 15s — that is not what the proto field documents). The proto's documented semantics are per-pipeline; honoring per-pipeline matches Envoy's behavior.

- **(B) Hardcoded 15s ignoring the proto field.** REJECTED because the proto field is widely used in real Envoy deployments (operators tune it to match their dispatch-path latency budget). Honoring it is doctrine D-3.5 (real-world parity over MVP simplification).

### Consequences

- (a) The bootstrap parser's envelope-check error message format (`listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope`) is consistent with the rest of `internal/listener/manager.go`'s error-message conventions (verified at PLAN time by inspection of existing errors at lines 247, 252, 257, 286, 290, 294 — all begin with `listener: %q:`). Future error-message refactors that touch `manager.go` should preserve this format.

- (b) `Pipeline.Run` takes a `timeoutMs uint32` parameter. `0 = no-op` (no `context.WithTimeout` wrapping; the caller's ctx is passed through unmodified). The listener manager passes `lfTimeoutMs` populated from the parsed proto field (15s = 15000 by default; clamped/validated at parse time per the envelope above). Test scaffolding that exercises `Pipeline.Run` directly may pass `0` to disable timeout enforcement.

- (c) A future hardening phase may revisit the `[1s, 60s]` envelope — for example, relax the upper bound for slow-network deployments (TLS over satellite, etc.) — with its own ADR. The envelope is durable for the BOOTSTRAP MVP trunk's deployment profile; relaxation requires a documented rationale + empirical pin.

### Lands-in-task

07.2 PLAN Task 4 (the `Pipeline.Run` timeout-enforcement mechanic). The bootstrap parser's envelope-check at `internal/listener/manager.go` (Task 9) cross-references this ADR. Supersedes nothing.

---

## ADR-0080: `default_filter_chain` semantics (no-match fallback; empty-match chain BEATS default_filter_chain; TLS posture independent)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.3 (differential correctness beats internal fidelity) + D-3.5 (honor proto fields widely used in real Envoy deployments).
**Supersedes:** ADR-0033 clause 3 (the parse-time error on `default_filter_chain` is totally superseded — 07.2 honors the field).

### Context

Phase 07.2 introduces `internal/listener/listenerfilter/chainmatch.go` (the `SelectChain` 2-pass eligibility-then-specificity algorithm) and is the first phase to honor the `Listener.default_filter_chain` proto field. SPEC §5.7 enumerates the supersession of ADR-0033's filter-chain whitelist; SPEC §8 raises the `default_filter_chain` semantics question: does it act as a no-match fallback, an always-preferred override, or something else; what happens when the user has BOTH an empty-match chain in `filter_chains[]` AND a `default_filter_chain`; and what are the cross-chain TLS-posture rules between `filter_chains[]` and `default_filter_chain`.

The phase-03 ADR-0033 took the conservative position that any `default_filter_chain` was a parse-time error (clause 3), reflecting MVP scope at the time. With 07.2's expanded surface — full 8-dimension `FilterChainMatch` with ALPN dispatch + listener-filter pipeline — `default_filter_chain` becomes useful as a fallback when none of the 8-dim chains match (e.g., a connection from an unexpected source with no SNI on a TLS-only listener; the operator wants a graceful TLS-rejecting fallback rather than a closed connection). The proto field is widely used in real Envoy deployments. SPEC §8 settled the semantics by pinning Envoy v1.37.2's behavior; this ADR records the decision in DECISIONS.md.

### Decision

The `Listener.default_filter_chain` proto field is honored, with the following semantics:

1. **No-match fallback only.** `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry's `filter_chain_match` is eligible against the per-connection `ChainMatchInputs`. If at least one `filter_chains[]` entry is eligible, `default_filter_chain` is NOT consulted (the eligible chain wins per the 8-dim specificity score per ADR-0081, even if the eligible chain has lower specificity than the operator might expect).

2. **Empty-match chain in `filter_chains[]` BEATS `default_filter_chain`.** A `filter_chains[]` entry with an absent or all-zero `filter_chain_match` (an "empty-match chain") is universally eligible at Pass 1 of the chain-match algorithm. When such a chain coexists with a `default_filter_chain`, the empty-match chain wins (the empty-match chain ENTERS the eligibility set, so `len(eligibleChains) >= 1`, so the no-match-fallback path is not taken). This matches Envoy's documented behavior.

3. **Independent TLS posture.** `default_filter_chain` may carry an independent `transport_socket` (TLS or plaintext) regardless of the `filter_chains[]` entries' TLS posture. The cross-chain mixed-TLS-and-plaintext rule from ADR-0033 clause 5 (preserved as ADR-0078 caveat: plaintext multi-chain disallowed when any chain populates `server_names`) applies WITHIN `filter_chains[]` only; `default_filter_chain` is a structurally-separate slot and is NOT subject to the cross-chain TLS uniformity rule.

### Empirical evidence

Both decisions are pinned to Envoy v1.37.2 at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per ENVOY_TARGET:

- **SPEC §11.1 (no-match fallback honored).** Verbatim Envoy stats from a single-chain bootstrap with `filter_chains[].filter_chain_match.destination_port = 18080` + `default_filter_chain` (TCP-proxy to a different backend) on a connection to port 18081: `tcp.tcp_dstport_18080.downstream_cx_total: 0`, `tcp.tcp_default.downstream_cx_total: 1`. Envoy honors `default_filter_chain` as the no-match fallback.

- **SPEC §11.2 (empty-match beats default).** Verbatim Envoy stats from a bootstrap with both an empty-match chain in `filter_chains[]` (`tcp_empty`) AND a `default_filter_chain` (`tcp_default`) on a connection from any source: `tcp.tcp_empty.downstream_cx_total: 1`, `tcp.tcp_default.downstream_cx_total: 0`. Envoy boots cleanly with both fields populated AND the empty-match chain wins dispatch.

### Alternatives considered

- **(A) `default_filter_chain` ALWAYS preferred (bypass `filter_chains[]` if `default_filter_chain` is set).** REJECTED per the §11.1 empirical pin (Envoy honors `filter_chains[]` first; `default_filter_chain` is consulted only on no-match). This alternative would break differential parity with Envoy on the dispatch-ordering axis and is therefore disqualified by D-3.3.

- **(B) Error if both empty-match chain in `filter_chains[]` AND `default_filter_chain` exist.** REJECTED per the §11.2 empirical pin (Envoy boots cleanly on the combined config; the empty-match chain simply wins by virtue of being eligible at Pass 1). Erroring on a config Envoy accepts would force operators to choose between the two structural slots when both are valid; the chosen semantics make the empty-match-vs-default-chain coexistence well-defined without operator intervention.

### Consequences

- (a) The `chainmatch.SelectChain(inputs, chains, defaultChain)` algorithm consults `defaultChain` ONLY when `len(eligibleChains) == 0`. Concretely, the implementation runs Pass 1 (eligibility) over `chains` first; if the eligibility set is empty, returns `defaultChain` if non-nil OR `ErrNoChainMatched` if nil. The algorithm is implemented in `internal/listener/listenerfilter/chainmatch.go` at the lands-in-task commit.

- (b) The `catchAllCount > 1` validation at `internal/listener/manager.go` (the phase-03 check that rejects a `filter_chains[]` slice with two empty-match chains) is preserved for `filter_chains[]` empty-match entries. The `default_filter_chain` is a SEPARATE structural slot and does NOT count toward `catchAllCount`. Therefore the four combinations of (empty-match-chain-count-in-filter_chains[], default-chain-presence) — `(0, no)`, `(1, no)`, `(0, yes)`, `(1, yes)` — are all valid; only `(2+, *)` errors at parse time.

- (c) The cross-chain mixed-TLS-and-plaintext rule from ADR-0033 clause 5 (preserved as ADR-0078 caveat) applies WITHIN `filter_chains[]` only. `default_filter_chain` is independent; an operator may have a TLS-only `filter_chains[]` entry coexisting with a plaintext `default_filter_chain` (e.g., a TLS listener that gracefully degrades to plaintext-rejection on no-match) without parse error.

### Lands-in-task

07.2 PLAN Task 5 (the `chainmatch.SelectChain` default-fallback path; same task as ADR-0081). The bootstrap parser's `default_filter_chain` honoring at `internal/listener/manager.go` (Task 9) and the manager's `defaultChain` plumbing into `SelectChain` (Task 10) cross-reference this ADR.

---

## ADR-0081: `FilterChainMatch` 8-dimension precedence algorithm

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (honor proto fields widely used in real Envoy deployments) + D-3.6 (record durable design rationale; the 8-dim priority order is a contract that future xDS/listener phases MUST consult).
**Supersedes (partial):** ADR-0033 clause 2 (the phase-03 `filter_chain_match` whitelist is partially superseded — only `direct_source_prefix_ranges` stays silent-skipped post-07.2, all other dimensions are honored per the 8-dim algorithm).

### Context

Phase 07.2 introduces `internal/listener/listenerfilter/chainmatch.go`'s `SelectChain` function — the per-connection chain-match algorithm. SPEC §5.5 raises the precedence-algorithm question: given 8 match dimensions on `FilterChainMatch` (`destination_port`, `prefix_ranges`, `server_names`, `transport_protocol`, `application_protocols`, `source_type`, `source_prefix_ranges`, `source_ports` — the upstream `direct_source_prefix_ranges` field is silent-skipped per ADR-0078), what is the priority order between dimensions; how are ties within a single priority slot broken; and what happens when two chains are identical on all 8 dimensions.

SPEC §7.1 enumerates the 8 dimensions; §7.2 documents the priority order matching the upstream `filter_chain_match.proto` comments; §7.3 documents the 2-pass eligibility-then-specificity algorithm; §11.3 pins the empirical evidence that `destination_port` BEATS `source_prefix_ranges` on a real Envoy v1.37.2 dispatch path.

### Decision

`SelectChain` runs a 2-pass eligibility-then-specificity algorithm over a slice of `*ChainSpec` plus an optional `*ChainSpec` default chain (per ADR-0080):

1. **Priority order (highest priority first).** `[destination_port, prefix_ranges, server_names, transport_protocol, application_protocols, source_type, source_prefix_ranges, source_ports]`. This matches the upstream `filter_chain_match.proto` documented order.

2. **Pass 1: eligibility.** A chain is eligible iff every specified (non-zero / non-empty) dimension matches the corresponding `ChainMatchInputs` field. A chain with all dimensions unspecified (the "empty-match" chain) is universally eligible. Chains with at least one specified dimension that does not match the input are eliminated. The output of Pass 1 is the eligibility set.

3. **Pass 2: specificity scoring.** Each eligible chain is scored by an 8-bit integer where bit `prioCount-1-i` is set iff the dimension at priority slot `i` is specified on the chain. Bit ordering puts the most-significant-bit on the highest-priority dimension so a numerical compare reflects the priority order. The chain with the highest specificity integer wins.

4. **Tie-breaking at finer grain.** When two chains have identical specificity vectors, ties are broken by per-dimension finer-grain criteria:
   - `prefix_ranges` / `source_prefix_ranges`: longest CIDR prefix containing the input IP wins (longer prefix = more specific).
   - `server_names`: SNI specificity per ADR-0033 clause 9 (preserved as ADR-0078 sub-ordering): exact > suffix-wildcard > universal-wildcard > catch-all (lower rank = more specific).
   - All other dimensions are exact-value match — no sub-ordering.

5. **Final ties (chains identical on all 8 dimensions AND on tie-break sub-ordering).** Error at `NewManager`-build time with `listener: %q: filter_chains[i] and filter_chains[j] have identical filter_chain_match — ambiguous selection`. The bootstrap parser duplicate-matches `*ChainSpec` entries structurally and surfaces `ErrAmbiguousChainMatch` to the operator at config-load time, NOT at per-connection dispatch time.

### Empirical evidence

The priority-order decision is pinned to Envoy v1.37.2 at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per ENVOY_TARGET. SPEC §11.3 documents verbatim Envoy stats from a 2-chain bootstrap (`tcp_dstport` with `destination_port: 8080`; `tcp_srcprefix` with `source_prefix_ranges: 127.0.0.0/8`) on a connection from `127.0.0.1` to port `8080` (BOTH chains eligible at Pass 1; Pass 2 selects on priority): `tcp.tcp_dstport.downstream_cx_total: 1`, `tcp.tcp_srcprefix.downstream_cx_total: 0`. The `destination_port` priority slot (index 0) BEATS the `source_prefix_ranges` priority slot (index 6); this is the load-bearing empirical pin for the priority order.

### Alternatives considered

- **(A) Per-priority-level eligibility (eliminate chains as soon as ANY higher-priority dimension's value differs).** REJECTED for two reasons: (1) it introduces O(N²) worst-case lookup as each priority level re-scans the surviving set; the chosen 2-pass algorithm is O(N × D) with D = 8 (constant), so O(N) per dispatch; (2) it doesn't respect Envoy's documented eligibility-then-specificity semantics — Envoy considers ALL dimensions together at Pass 1 and disambiguates by priority at Pass 2, NOT by progressive per-level elimination.

- **(B) String-based pattern matching (regex) on chain names or composite-key strings.** REJECTED as out-of-scope. The 8-dim algorithm is structural (each dimension has a typed match-function); regex matching would require a separate dimension and is not in any in-scope `FilterChainMatch` proto field. Future phases that add string-pattern dimensions (e.g., a hypothetical `path_prefix` future field) would re-litigate via their own ADR.

### Consequences

- (a) The `SelectChain` algorithm is O(N × D) where N = number of chains in `filter_chains[]` and D = 8 (constant). Per-connection dispatch latency is therefore microseconds-scale even for listeners with hundreds of chains; well below the `listener_filters_timeout` budget (per ADR-0082; default 15s).

- (b) The `*ChainSpec` slice is built at `NewManager`-build time from the bootstrap's `filter_chains[]` (per Decision O from SPEC §5.6) and is immutable thereafter. Concurrent per-connection `SelectChain` calls are read-only on this slice and lock-free by construction (no `sync.Map` overhead per the SPEC's recommendation). The chain-list-immutability invariant is documented in `internal/listener/listenerfilter/chainmatch.go`'s `ChainSpec` doc comment.

- (c) The algorithm's worst-case latency per accepted connection is microseconds (8 dimension checks × O(1) each per chain × O(N) chains). The realistic deployment profile (N < 100 chains, D = 8) yields sub-microsecond dispatch latency; well below the `listener_filters_timeout` envelope's 1s lower bound. Future hardening phases that introduce e.g. radix-tree-based prefix-range indexing would supersede this ADR's "lock-free linear scan" claim with their own ADR.

### Lands-in-task

07.2 PLAN Task 5 (the `chainmatch.SelectChain` algorithm; same task as ADR-0080). The bootstrap parser's `*ChainSpec` construction at `internal/listener/manager.go` (Task 9) and the per-connection `SelectChain` invocation in the manager's accept-loop (Task 10) cross-reference this ADR.

---

## ADR-0078: ADR-0033 partial supersession enumeration

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (record durable design rationale; the supersession enumeration is a contract that future xDS / listener phases MUST consult).
**Supersedes (partial):** ADR-0033 (Phase-03 filter-chain subset).

### Context

Phase 07.2's deliverable rewrites `internal/listener/manager.go`'s `validateFilterChainMatch` (the phase-03 narrow whitelist) into `parseChainSpec` (the phase-07.2 8-dimension parser per ADR-0081), removes the parse-time error on `Listener.default_filter_chain` (per ADR-0080), and adds `listener_filters[]` parsing (per ADR-0079). Each of these changes lifts a constraint introduced by ADR-0033 (Phase-03 filter-chain subset). SPEC §5.7 enumerates the clause-by-clause disposition: which parts of ADR-0033 stay in 07.2 vs are superseded. This ADR is the dedicated record of that enumeration so future readers of ADR-0033 see the supersession relationship via a grep on `DECISIONS.md`.

ADR-0033 has 9 clauses. The 07.2 deliverables explicitly settle each one — three are fully preserved, three are preserved with caveats, and three are superseded. Recording this in a dedicated ADR makes the relationship grep-verifiable and discoverable from either side (the future reader of ADR-0033 can find ADR-0078 by searching for "Supersedes (partial): ADR-0033"; the reader of ADR-0078 sees the canonical clause-by-clause table).

### Decision

The 9 ADR-0033 clauses receive the following 07.2 disposition (full clause-by-clause table in SPEC §5.7):

1. **Clause 1 (`filter_chains` must be ≥ 1):** **STAYS** with a wording-update caveat — 07.2 preserves the structural requirement but the union of `filter_chains[]` and `default_filter_chain` must contribute at least one chain (a default-only listener is now valid per ADR-0080).
2. **Clause 2 (`filter_chain_match` whitelist — only `server_names` + `transport_protocol == "tls"`):** **PARTIALLY SUPERSEDED.** 07.2 honors the full 8-dimension `FilterChainMatch` per ADR-0081. Only `direct_source_prefix_ranges` remains silently-ignored.
3. **Clause 3 (`Listener.default_filter_chain` set → error):** **TOTALLY SUPERSEDED.** 07.2 honors the field per ADR-0080 — `default_filter_chain` is the no-match fallback chain.
4. **Clause 4 (`transport_socket` may be nil or carry `DownstreamTlsContext`):** **STAYS.** Unchanged.
5. **Clause 5 (mixed TLS/plaintext-on-one-listener error):** **STAYS** with caveat — the mixed-TLS-and-plaintext rule is preserved WITHIN `filter_chains[]`; `default_filter_chain` MAY have its own `transport_socket` independent of the `filter_chains[]` entries' TLS posture per ADR-0080.
6. **Clause 6 (plaintext multi-chain error):** **PARTIALLY SUPERSEDED.** A plaintext listener with multiple chains is now allowed if the chains use non-SNI dimensions for matching. The legacy "no plaintext multi-chain" error is preserved as a special case for plaintext listeners where at least one chain populates `server_names[]` (SNI cannot match on plaintext).
7. **Clause 7 (`require_client_certificate=true` errors):** **STAYS.** Unchanged.
8. **Clause 8 (`listener_filters` silently skipped):** **TOTALLY SUPERSEDED.** 07.2 honors the field per ADR-0079 — `listener_filters[]` resolves through the threaded `*ListenerFilterRegistry` and dispatches before chain match.
9. **Clause 9 (SNI-internal sub-ordering):** **PRESERVED AS SPECIAL CASE.** The SNI-internal sub-ordering (exact > suffix-wildcard > universal-wildcard > catch-all) becomes the tie-breaker WITHIN the `server_names` priority slot of the new 8-dimension algorithm. The "handshake fails" no-match case is replaced by "fall through to `default_filter_chain` if set; otherwise close conn" per ADR-0080. The `chainSpecificityRank` LOGIC is preserved verbatim as `sniSpecificityRank` in `internal/listener/listenerfilter/chainmatch.go`; the original `chainSpecificityRank` symbol in `internal/listener/manager.go` is deleted at Task 9 (no remaining callers after the chain-sort and dispatch refactor).

**Net effect:** clauses 1, 4, 7 fully preserved; clauses 5, 6, 9 preserved with caveats; clauses 2, 3, 8 superseded.

### Rationale

A clause-by-clause supersession ADR is the cleanest way to record the relationship between ADR-0033 and the 07.2 ADR set. The alternative — leaving the supersession implicit, scattered across ADR-0079 / ADR-0080 / ADR-0081 — would require a future reader to grep three ADRs to reconstruct the picture. ADR-0078 consolidates the enumeration in one place; the table is the canonical source the SPEC §5.7 row points at. Future xDS / listener phases that revisit any ADR-0033 clause can cite ADR-0078 directly.

### Consequences

- (a) A future reader of ADR-0033 sees the supersession relationship enumerated in ADR-0078 (grep `DECISIONS.md` for `Supersedes (partial): ADR-0033`).
- (b) The `chainSpecificityRank` symbol is deleted from `internal/listener/manager.go` at Task 9; the LOGIC is preserved in `internal/listener/listenerfilter/chainmatch.go`'s `sniSpecificityRank`. Future hardening phases that revisit SNI specificity consult `chainmatch.go`, not the deleted manager.go function.
- (c) The `parseChainSpec` function (replacing `validateFilterChainMatch`) accepts all 8 chain-match dimensions per ADR-0081. The phase-03 7-error rejection-path block at the old `manager.go:382-398` is removed.
- (d) The `listenerRuntime` struct gains `chainSpecs []*ChainSpec`, `defaultSpec *ChainSpec`, `defaultChain *chainInfo`, `chainByName map[string]*chainInfo`, `listenerFilterFactories []FilterInstanceFactory`, `lfTimeoutMs uint32`, and `continueOnLfTimeout bool` at Task 9; Task 10 consumes these in the dispatch refactor.

### Lands-in-task

07.2 PLAN Task 9 (the `internal/listener/manager.go` rewrite of `validateFilterChainMatch` → `parseChainSpec`, the removal of the `default_filter_chain` parse-time error, the addition of `listener_filters[]` parsing — the first task that materially realizes the supersession in production code). The dispatch-path consumption of the new `listenerRuntime` fields lands at Task 10.

---

## ADR-0084: Phase-08 planner-time split (08.1 + 08.2)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.4 (plan-size gates govern phase scope), D-3.5 (decisions are written).

### Context

Phase 08 (admin + observability completion, BOOTSTRAP §8 row 08) covers two structurally distinct halves: (a) the read-only admin endpoints — `/config_dump`, `/clusters`, `/listeners`, `/server_info` — anchored under `internal/admin/` with read-only consumers of `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` snapshots; and (b) the graceful-drain mutating semantics — `/healthcheck/fail`, `/quitquitquit`, `/drain_listeners` — anchored under cross-cutting drain-state plumbing that mutates listener accept loops + the shared `*Server.draining` state machine. The two halves share no runtime mutation surface; the read-only handlers introduce no new state, while the drain handlers introduce a new state machine that affects every existing listener accept loop.

Per BRAINSTORM §1 + parent SPEC, the BRAINSTORM session split phase 08 along this read-only-vs-mutating axis at brainstorm-close. The combined LoC + task estimate (~1100–1600 LoC + ~28–38 tasks combined per BRAINSTORM §1) crosses BOTH ADR-0045 plan-size-gate thresholds (the ~1500-LoC OR-leg AND the 25-task gate); splitting is mandatory under ADR-0045's discipline. The split landed in the SPEC-drafting commit (master `1f85b07`) via:
- ROADMAP row `08` flipped `planned → in-progress` with sub-phases column `08.1, 08.2`.
- Row `08.1` added as `planned` with depends-on `07.2`.
- Row `08.2` added as `planned` with depends-on `08.1`.

This ADR formalizes the split decision durably; the ROADMAP edit is its concrete on-disk effect. The pattern mirrors ADR-0070's phase-07 split (07.1 + 07.2), ADR-0045's phase-05 + phase-06 splits (05.1 + 05.2 / 06.1 + 06.2), and is anchored at the implementation session's first commit (Task 1 PROGRESS preamble) — the first opportunity to land an ADR after the SPEC commit's ROADMAP edit.

### Decision

Phase 08 is split into two sub-phases at planner-time per ADR-0045's discipline (which documented the 05.1 + 05.2, 06.1 + 06.2, and — as ADR-0070 — the 07.1 + 07.2 splits):
- **08.1 — Admin read-only endpoints.** Surface: `internal/admin/` (the four new handler files `configdump.go`, `clusters.go`, `listeners.go`, `serverinfo.go` plus shared helpers `headers.go`, `version.go`); read-only consumers of `*bootstrap.Bootstrap` (carrying the new `ConfigPath` field), `*cluster.Manager` (carrying the new `Clusters()` snapshot accessor), and `*listener.Manager` (existing `Listeners()` accessor reused unchanged). Differential surface at end: fixture 0009-admin-config-dump (config_dump byte-equality with allow-list). Lands the read-only admin observability surface that the ADR-0045 / ADR-0063 admin family depends on.
- **08.2 — Graceful drain.** Surface: cross-cutting — listener accept-loop instrumentation, `*Server.draining` state machine, the three mutating handlers `/healthcheck/fail`, `/quitquitquit`, `/drain_listeners`, plus drain-time-budget plumbing into the connection-shutdown path. Differential surface at end: TBD (08.2 SPEC drafts the fixtures). Lands the graceful-drain semantics that the BOOTSTRAP §8 row 08 canonical title also covers.

Ordering is 08.1-first, 08.2-second because 08.1 ships read-only consumers of existing structures and depends only on phase-07.2 closure, while 08.2 depends on 08.1's `*Server` constructor widening (08.2 reuses the same `*Server` to host the mutating handlers; the `*Server.bs` field added by 08.1 is also the bootstrap reference 08.2's drain state machine consults for `drain_strategy` defaults).

The parent ROADMAP row `08` flips `planned → in-progress` at the SPEC-drafting commit (already landed at master `1f85b07`); it transitions to `done` ONLY at 08.2's phase-done commit (NOT at 08.1's phase-done) — mirroring the 05/05.1/05.2 + 06/06.1/06.2 + 07/07.1/07.2 closure pattern. 08.1's phase-done commit flips row `08.1 → done` AND leaves row `08` at `in-progress`; 08.2's phase-done commit flips BOTH rows `08.2 → done` AND `08 → done` AT THE SAME COMMIT.

### Alternatives considered

(A) Ship phase 08 as one sub-phase. Rejected: the cumulative LoC + task estimate (~1100–1600 LoC, ~28–38 tasks combined) crosses BOTH ADR-0045 plan-size-gate thresholds (the ~1500-LoC OR-leg AND the 25-task gate); splitting is mandatory under ADR-0045's discipline.

(B) Split along a different axis (e.g., per-endpoint sub-phases — one sub-phase per admin endpoint). Rejected: the four read-only endpoints share their handler scaffolding (header helpers, error-shape helpers, the widened `*Server` constructor); splitting per-endpoint would either duplicate the scaffolding across sub-phases or push the scaffolding into a zeroth sub-phase, neither of which improves on the read-only-vs-mutating axis. The mutating handlers (`/healthcheck/fail`, `/quitquitquit`, `/drain_listeners`) genuinely share the drain state machine with each other and have no shared structure with the read-only handlers — so the read-only-vs-mutating axis is the natural cut.

(C) Defer the graceful-drain handlers to a feature-family phase post-08 (e.g., post-09). Rejected: BOOTSTRAP §8 row 08's "admin + observability completion" canonical title covers BOTH the read-only endpoints AND the drain semantics; deferring the drain handlers would leave the BOOTSTRAP MVP trunk's row 08 incomplete on a load-bearing primitive (graceful drain is needed for any production-shaped deployment that expects rolling-restart correctness; xDS phases 09+ depend on the drain semantics for cluster-membership change handling).

### Consequences

(a) The phase 08 ROADMAP row carries a `sub-phases` column listing `08.1, 08.2`; status `in-progress` until BOTH sub-phases land done. The parent row 08 stays `in-progress` through both sub-phase phase-done commits and flips `done` only at 08.2's phase-done — per the parent SPEC §5 closure rule.

(b) 08.1 and 08.2 ship as independent sub-phases, each with their own `SPEC.md` + `PLAN.md` + `PROGRESS.md` + `REVIEW.md` lifecycle artefacts under `docs/envoy-go/phases/08.1-admin-endpoints/` and `docs/envoy-go/phases/08.2-graceful-drain/`. The parent SPEC at `docs/envoy-go/phases/08-admin-completion/SPEC.md` is read-only history once drafted (mirror of the 05 + 06 + 07 parent master SPECs).

(c) The seven 08.1 ADRs (ADR-0084..ADR-0090) are 08.1-scoped; 08.2 will introduce its own ADRs at its own SPEC + PLAN time. Future ADR-0091..onwards may amend either sub-phase without colliding.

(d) The BEHAVIOR_CONTRACT umbrella section for phase 08 is restructured by 08.1 (the first sub-phase to land a phase-done commit under the parent row); 08.2 extends the same umbrella section with its mutating-drain contract. The umbrella reorganization lands at 08.1 Task 15.

(e) Total task count of phases 08.1 + 08.2 is bounded: 08.1 ships at 15 tasks (this PLAN); 08.2 will draft its own task count at its own PLAN time — the BRAINSTORM §1 estimate of ~28–38 tasks combined leaves ~13–23 tasks for 08.2.

### Lands-in-task

08.1 PLAN Task 1 (PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at master `1f85b07` per SPEC drafting).

---

## ADR-0085: Admin-mux reuse + LBP-1 third application — `admin.New` widens to thread `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager`

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.2 (one-way constructor wiring at boot), D-3.4 (plan-size + scope-locality govern phase scope).
**Lands-in-task:** 08.1 PLAN Task 5 (`internal/admin/admin.go` constructor widening).

### Context

The phase-08.1 admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) are read-only operator-introspection surfaces consuming snapshots of three boot-time structures: the parsed `*bootstrap.Bootstrap` (for `/config_dump`'s body, `/server_info`'s `node` field, and `command_line_options.config_path`), the `*cluster.Manager` (for `/clusters`'s cluster snapshot via the new `Clusters()` accessor — Task 3), and the `*listener.Manager` (for `/listeners`'s listener snapshot via the existing `Listeners()` accessor). The phase-01 admin server already has a working bind, working `/ready` gate, working timeouts, working integration into the lifecycle (per phase-01 + phase-06.1 architecture); splitting into a NEW admin server for the four new endpoints would duplicate all that for zero benefit.

The constructor-widening pattern that 06.1 used for `*stats.Registry` (boot-time-allocated, one-way-threaded) and 07.1 used for `*HTTPRegistry` (boot-populated then `Freeze()`d) and 07.2 used for `*ListenerFilterRegistry` (boot-populated then `Freeze()`d) generalises to a third application here. The pattern — call it LBP-1 (Linear Boot-time Provisioning, single application) — has now been ratified across three independent surface areas and the PLAN-time third generalisation (08.1's three-thread widening of `admin.New`) confirms it as the project's canonical wiring discipline for boot-time provisioning of read-only state through the constructor graph.

Per planner-time decision 2 (PLAN), the WriteTimeout on the admin `*http.Server` widens from phase 01's 5s to 30s — the new `/config_dump` handler's protojson rendering of large bootstraps may approach the budget on slow scrape clients; 30s is generous enough for any reasonable fixture without weakening resilience under malicious-slow-loris-style abuse (the 30s ceiling still bounds resource exhaustion).

### Decision

Extend `internal/admin.Server` with four new fields — `bs *bootstrap.Bootstrap`, `cm *cluster.Manager`, `lm *listener.Manager`, `bootTime time.Time` — by widening `admin.New(addr, registry)` to `admin.New(addr, registry, bs, cm, lm)`. `bootTime` is set to `time.Now()` at `New()` call (consumed by `/server_info`'s `uptime_current_epoch` + `uptime_all_epochs` fields per Task 9). Register four new `mux.HandleFunc` entries (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) on the existing `*http.ServeMux` in `admin.Server.Start()`; no new HTTP server, no new bind, no new transport. Per planner-time decision 2, `WriteTimeout` widens from 5s to 30s. Task 5 lands placeholder handlers that return 501 Not Implemented; Tasks 6-9 each create per-endpoint files (`configdump.go` / `clusters.go` / `listeners.go` / `serverinfo.go`) that overwrite the placeholder bodies with real implementations.

The three new constructor parameters (`bs`, `cm`, `lm`) are typed against concrete `*Type` rather than interfaces — consistent with 06.1's `*stats.Registry`, 07.1's `*HTTPRegistry`, and 07.2's `*ListenerFilterRegistry` choice — because the admin handlers are read-only consumers of well-defined snapshot accessors (no swap-out / mock-out requirement at runtime; tests construct minimal real instances per the `mustMinimalBs/CM/LM` helpers introduced at Task 5).

### Alternatives considered

(A) Spin up a SECOND admin `*http.Server` for the four new endpoints. Rejected: duplicates the bind/timeouts/lifecycle scaffolding for zero benefit; the four new handlers share the existing admin authority surface (port 9901 by convention, `Server: envoy` header, the four constant headers from `writeAdminHeaders`); a second server would either require operators to scrape two ports or front-end them with a reverse proxy — both worse than mux reuse on a single server.

(B) Stash `bs`/`cm`/`lm` in package-level globals and have the four handlers read them. Rejected: violates LBP-1 (the project's three-prior-applications-strong constructor-threading discipline); package globals make test isolation harder (cannot construct two `*Server` instances with different bootstraps in the same test process); and explicit threading is the documented project convention since 06.1 (ADR-0072 for HTTP, ADR-0079 for listener-filter).

(C) Widen `New` to take only the three new args and drop the `*stats.Registry` arg (since `bs.Stats` carries it). Rejected: the existing `*stats.Registry` thread-through is consumed at `New()` time to allocate `server.live` (SPEC §12 #3); the registry must be available at constructor time before `bs` is necessarily complete (test code passes `nil` for `bs` but always passes a non-nil registry). Keeping all four constructor args explicit also preserves the existing-test-code call-site shape with one additive change rather than a rearrangement.

(D) Keep `WriteTimeout` at 5s; rely on the `/config_dump` body being small for the SPEC §7.3 fixture. Rejected: the SPEC §7.3 fixture is small but the four handlers are operator-facing and operators in production may scrape against bootstraps with O(100) clusters / O(50) listeners — `/config_dump`'s protojson rendering of those bodies may approach 5s under slow-network conditions; widening to 30s is a one-line change with no per-endpoint complexity. 30s also still bounds slowloris-style resource exhaustion.

### Consequences

(a) `cmd/envoy-go/main.go`'s `admin.New` call site widens by three args (Task 10 lands the call-site update). Between Task 5 (this ADR's lands-in-task) and Task 10, `go build ./cmd/envoy-go/...` will fail with `not enough arguments in call to admin.New`; this is intentional and is documented in PROGRESS for Tasks 5-9 (each of which lands an admin-internal change before the call-site update). Phase 08.2 (per ADR-0091) extends this consequence with the LBP-1 fifth application: `*drain.Manager` is threaded into `admin.New` (7th param), `listener.NewManagerWithBaseDirAndAllowH2C`, the HCM filter constructor, and the TCP-proxy filter constructor at the same explicit-threading discipline. The cluster manager does NOT take dm — `cm.Drain()` is called from `cmd/envoy-go/main.go` after `<-drainMgr.Done()` rather than threaded as a constructor dep.

(b) Test code that does NOT exercise the four new endpoints passes `nil` for `bs`/`cm`/`lm` — the four widened-constructor call sites in `internal/admin/admin_test.go` (the seven existing `TestServer_*` tests + the two new `TestServer_NewWidenedConstructor` + `TestAdminWriteTimeoutIs30s` tests) all pass `nil, nil, nil`. Tasks 6-9's per-endpoint test files use `mustMinimalBs(t)` / `mustMinimalCM(t, bs)` / `mustMinimalLM(t, bs, cm)` from `internal/admin/admin_helpers_test.go` (Task 5) to construct the `§7.3` fixture-shaped real instances; the four handler implementations check for nil at handler-entry and return a clear error response if one of the three references is nil (Tasks 6-9 land that defensive shape).

(c) The LBP-1 explicit-threading discipline now has FOUR sibling applications across three independent surface areas: 06.1 `*stats.Registry` (boot-allocated, one-way-threaded into `cluster.NewManager`/`listener.NewManager`/HCM), 07.1 `*HTTPRegistry` (boot-populated then `Freeze()`d, one-way-threaded into `listener.NewManager`), 07.2 `*ListenerFilterRegistry` (boot-populated then `Freeze()`d, one-way-threaded into `listener.NewManager`), and 08.1 admin's three-thread (`*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager`, all one-way-threaded into `admin.New`). The pattern is now ratified as the project's canonical wiring discipline for boot-time provisioning of read-only state through the constructor graph; future phases that add operator-introspection or boot-time-resolved configuration follow this template by default.

(d) Phase 08.2 (graceful drain) inherits the four threaded fields without further constructor widening; 08.2 may add a single `drainState atomic.Pointer[DrainState]` field on `Server` (or equivalent) without changing `New`'s signature. The four 08.1 fields (`bs`/`cm`/`lm`/`bootTime`) are reused by 08.2's mutating handlers (`/healthcheck/fail`, `/quitquitquit`, `/drain_listeners`) — the drain handlers consult `bs.Proto.Admin` for `drain_strategy` defaults, the listener manager for the per-listener accept-loop pause-then-stop sequencing, and `bootTime` for `/server_info`'s extended state field per the BEHAVIOR_CONTRACT extension at 08.2.

(e) `WriteTimeout` widens to 30s for ALL admin endpoints — `/ready` (sub-millisecond), `/stats/prometheus` (low-millisecond), `/config_dump` (the new slow-path), `/clusters`, `/listeners`, `/server_info` (all expected sub-second on the SPEC §7.3 fixture). The widening does not weaken resilience: the 30s ceiling still bounds slowloris-style resource exhaustion; the only handler that approaches the budget is `/config_dump` on large bootstraps, where 30s is generous enough.

---

## ADR-0086: `/config_dump` body shape — `protojson.MarshalOptions{Multiline, Indent:" ", UseProtoNames, EmitUnpopulated}` over `*adminv3.ConfigDump{Configs: [Bootstrap, Listeners, Clusters]}`

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.7 (planner-time consolidation of cross-cutting decisions before implementation).
**Lands-in-task:** 08.1 PLAN Task 6 (`internal/admin/configdump.go`).

### Context

The `/config_dump` endpoint is the largest of phase-08.1's four read-only operator-introspection surfaces. Its body shape is not derivable from the protobuf IDL alone — both the on-the-wire JSON formatting (indent width, field-name case, zero-valued-field elision behavior) AND the three-sub-envelope ordering inside `*adminv3.ConfigDump.Configs` are pinned by empirical observation of upstream Envoy v1.37.2 against the SPEC §7.3 fixture, captured verbatim in SPEC §11.1. The shape is load-bearing for the Task 13 differential equivalence comparator: byte-equality (modulo the §13.2 allow-list of node.user_agent_*, node.extensions[], `<*ConfigDump>.last_updated`) requires both Envoy and envoy-go to render the same field set with the same indent, the same case, the same emit-zeroed-fields behavior, AND the same envelope ordering inside the outer `Configs` slice.

The four protojson MarshalOptions values (`Multiline`, `Indent`, `UseProtoNames`, `EmitUnpopulated`) form a tuple that no single SPEC reference can fully consolidate — the first three are observable from a single Envoy scrape, but the fourth (`EmitUnpopulated: true`) is observable only from the body's "show-me-everything" character (Envoy emits zero-valued protobuf fields like `cluster.original_dst_lb_config: {}` and `cluster.cleanup_interval: "0s"` even when not user-populated; envoy-go must do the same or the comparator's field-set diff will flag spurious mismatches).

The three-sub-envelope ordering — Bootstrap (Configs[0]), Listeners (Configs[1]), Clusters (Configs[2]) — is also pinned empirically. The `*adminv3.ConfigDump` proto schema does not enforce ordering (it's a `repeated google.protobuf.Any`); upstream Envoy's source-level ordering happens to match the schema-declaration order of its admin-impl ConfigDumpHandler, but that's an implementation detail not a contract. envoy-go pins this ordering via ADR to prevent silent differential-comparator drift if a future refactor reorders `enumerateStatic*` calls in `buildConfigDump`.

Per planner-time decision 7 (PLAN preamble), the static-listener and static-cluster enumeration walks the bootstrap proto directly — `bs.Proto.GetStaticResources().GetListeners()` / `.GetClusters()` — rather than consulting the cluster/listener managers' runtime state (`cm.Clusters()` / `lm.Listeners()`). The bootstrap proto is the canonical source of static resources for `/config_dump`'s purpose (operator introspection of declared config), and walking it directly avoids any post-boot mutation drift from the manager's runtime state representation.

### Decision

The `/config_dump` body is `application/json` (per SPEC §11.6 + ADR-0014) rendered via the package-level `configDumpMarshalOptions = protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` over a `*adminv3.ConfigDump{Configs: []*anypb.Any{<BootstrapAny>, <ListenersAny>, <ClustersAny>}}` envelope, in this exact order:

1. `Configs[0]` — `*adminv3.BootstrapConfigDump` packed via `anypb.New`. Carries the parsed `bs.Proto` in `.Bootstrap` and `timestamppb.New(bootTime)` in `.LastUpdated`.
2. `Configs[1]` — `*adminv3.ListenersConfigDump` packed via `anypb.New`. `.VersionInfo = "static"`. `.StaticListeners` populated by `enumerateStaticListeners(bs.Proto, bootTime)` walking `bs.GetStaticResources().GetListeners()` and packing each into a `*adminv3.ListenersConfigDump_StaticListener{Listener: <anypb.New(l)>, LastUpdated: <timestamppb.New(bootTime)>}`.
3. `Configs[2]` — `*adminv3.ClustersConfigDump` packed via `anypb.New`. `.VersionInfo = "static"`. `.StaticClusters` populated by `enumerateStaticClusters(bs.Proto, bootTime)` mirroring the listener walker.

Static-only — no `dynamic_*` arrays anywhere (envoy-go has no xDS surface). The four other ConfigDump sub-envelope types in upstream Envoy (RoutesConfigDump, SecretsConfigDump, ScopedRoutesConfigDump, EndpointsConfigDump) are deferred per ADR-0089 (Task 15); their omission from envoy-go's body is differential-comparator-allow-listed at §13.2.

The `protojson.Marshal` error path (which can fire if a sub-envelope contains an unregistered proto type, or if `anypb.New` returns an error) writes 500 Internal Server Error with `{}` body — defensive shape since Envoy's empirical behavior on `/config_dump` failure is undocumented; an empty-JSON-object body keeps the response a valid JSON document. Errors are logged at `log.Printf` level with the `admin: /config_dump:` prefix per SPEC §5.2.

The same `configDumpMarshalOptions` package var is reused by `/server_info` (Task 9) for cross-endpoint JSON-body shape consistency — the four-value tuple is identical for both endpoints (both are protojson-rendered admin surfaces that must round-trip differentially against upstream Envoy).

### Alternatives considered

(A) Use `protojson.MarshalOptions{}` (defaults) and accept the resulting body shape. Rejected: defaults are `Multiline: false` (single-line JSON), `Indent: ""`, `UseProtoNames: false` (camelCase field names), `EmitUnpopulated: false` (zero-valued fields elided). All four diverge from upstream Envoy's empirical scrape; the differential comparator would flag every endpoint as a body mismatch.

(B) Render JSON via `encoding/json` over a hand-written Go struct mirror of `adminv3.ConfigDump`. Rejected: doubles the maintenance surface (every proto field added upstream requires a Go struct field added in envoy-go) and loses protojson's well-tested `*anypb.Any` packing semantics. The protojson approach is canonical for go-control-plane proto rendering.

(C) Walk the cluster/listener managers' runtime state (`cm.Clusters()` / `lm.Listeners()`) to populate `enumerateStatic*`. Rejected per planner-time decision 7: the bootstrap proto is the canonical source for `/config_dump`'s purpose; walking the managers introduces post-boot mutation-state coupling that the static-only contract does not need. Future phase 08.2's `/drain_listeners` does mutate listener state, but `/config_dump` reflects the declared config not the runtime state.

(D) Pin the three-sub-envelope ordering implicitly via the `buildConfigDump` source order (Bootstrap → Listeners → Clusters) without ADR ratification. Rejected: a future refactor that reorders the three `anypb.New` calls (e.g. for stylistic reasons) would silently break the differential comparator. The ADR makes the ordering a contract not an accident; Task 13's comparator can reference this ADR when asserting envelope-position equivalence.

(E) Emit `dynamic_*` arrays as empty (e.g. `dynamic_listeners: []`) to match a hypothetical "Envoy with no xDS configured" body. Rejected: empirical scrape against upstream Envoy v1.37.2 with the SPEC §7.3 fixture shows that `dynamic_*` arrays are simply ABSENT from the body when xDS is not configured, not present-but-empty. `EmitUnpopulated: true` does NOT emit the `dynamic_*` fields because they are not populated AT ALL on the Go-side sub-envelope (they're only populated when ListenersConfigDump.DynamicListeners has entries — `EmitUnpopulated` is a marshaler flag, not an "emit-default-for-empty-repeated" flag).

### Consequences

(a) The `/config_dump` body is byte-equal to upstream Envoy v1.37.2 on the SPEC §7.3 fixture modulo the §13.2 differential allow-list: `node.user_agent_name`, `node.user_agent_build_version`, `node.extensions[]` (envoy-go's node has no extensions; Envoy's has the v1.37.2 extension list), and `<BootstrapConfigDump|ListenersConfigDump_StaticListener|ClustersConfigDump_StaticCluster>.last_updated` (timestamps differ by build/run time). All four allow-list entries are documented in BEHAVIOR_CONTRACT.md §Admin API (Task 15) and the differential harness applies them at field-walk time.

(b) Task 13's differential comparator (`tools/differential/cmd/diff-config-dump`) JSON-parses both Envoy's and envoy-go's `/config_dump` bodies into `map[string]interface{}` (or `*adminv3.ConfigDump` via protojson `Unmarshal` with `DiscardUnknown: true`), then field-walks with the §13.2 allow-list applied to detect any non-allow-listed mismatch. The comparator depends on this ADR's three-sub-envelope ordering invariant: `cd.Configs[0]` MUST be Bootstrap, `[1]` MUST be Listeners, `[2]` MUST be Clusters on both sides. If a future change adds a fourth envelope (RoutesConfigDump etc., per ADR-0089's deferral), it MUST be appended at index 3 — never inserted before the existing three — so the comparator's positional indexing remains stable.

(c) Future phases that extend `/config_dump` with additional sub-envelopes (RoutesConfigDump for static route configs, SecretsConfigDump if envoy-go ever gains an SDS-on-disk shape, ScopedRoutesConfigDump if scoped-RDS lands, EndpointsConfigDump if EDS lands) MUST append to `cd.Configs` at indices >= 3 and MUST NOT renumber the existing three. The three-position invariant is the contract; extension is additive at the tail. ADR-0089 (Task 15) records the deferral of those four envelope types from phase 08.1's scope.

(d) The same `configDumpMarshalOptions` four-value tuple is reused by `/server_info` (Task 9) for cross-endpoint body-shape consistency. Both endpoints render protojson over admin/v3 messages; both differentially round-trip against upstream Envoy under the same comparator allow-list discipline. Future protojson-rendered admin surfaces (e.g. phase 08.2's `/drain_listeners` if it returns a JSON body, though current SPEC §1 has it return `OK\n` text) follow the same MarshalOptions tuple by default.

(e) Errors from `protojson.Marshal` or `anypb.New` are recovered at handler level into a 500 + `{}` body. The `{}` body keeps the response a valid JSON document for any operator tooling that parses it; the 500 status code communicates the failure cleanly. Logging is at `log.Printf` level with the `admin: /config_dump:` prefix; future phases may upgrade to structured logging (slog) without changing this contract.

---

## ADR-0087: `/clusters` and `/listeners` body shape — text/plain with full Envoy-parity line-set; per-endpoint cx_/rq_ counters emit literal `0`

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.1 PLAN Task 7 (`internal/admin/clusters.go`); covers both `/clusters` here and `/listeners` (Task 8 references this ADR rather than introducing a new one — the two text-format endpoints share a single body-shape contract).

### Context

The `/clusters` and `/listeners` admin endpoints are the two text-format read-only operator-introspection surfaces in phase 08.1 (the other two — `/config_dump` and `/server_info` — are JSON via `protojson`, settled by ADR-0086). Their body shapes are not derivable from any Envoy proto schema — neither endpoint is rendered from a top-level proto; both are produced by Envoy's text-mode admin handler with hand-written line layouts in C++. The line set, the field constants, and the per-cluster + per-endpoint emission order are pinned by empirical observation of upstream Envoy v1.37.2 against the SPEC §7.3 fixture, captured verbatim in SPEC §11.2 (`/clusters`) and §11.3 (`/listeners`).

For `/clusters`, the §11.2 verbatim scrape establishes a 28-line block per cluster: 10 cluster-level lines (`observability_name`; the 4 `default_priority::*` lines + 4 `high_priority::*` lines for circuit-breaker thresholds; `added_via_api::false`) followed by 18 lines per endpoint (`cx_active`, `cx_connect_fail`, `cx_total`, `rq_active`, `rq_error`, `rq_success`, `rq_timeout`, `rq_total` — the 8 cx_/rq_ counters; then `hostname::`, `health_flags::healthy`, `weight::1`, `region::`, `zone::`, `sub_zone::`, `canary::false`, `priority::0`, `success_rate::-1`, `local_origin_success_rate::-1` — the 10 metadata constants). The cluster-level constants `1024` and `3` are the proto defaults from `envoy.config.cluster.v3.CircuitBreakers.Thresholds` (`max_connections=1024, max_pending_requests=1024, max_requests=1024, max_retries=3`); envoy-go has no circuit-breaker machinery (deferred to the upstream-robustness phase family) but emits the same constants for byte-shape parity. The per-endpoint metadata constants (`healthy`, `1`, `0`, empty string, `false`, `-1`) are Envoy's default-when-not-configured values for fields envoy-go does not model (active health checking, locality tags, weight, priority, canary, success-rate measurement).

Per planner-time decision 8 (PLAN), all 8 per-endpoint cx_/rq_ counter values emit literal `0` rather than reading live cluster-level counters. The rationale: envoy-go's stats registry from phase 06.1 has cluster-level counters (`cluster.<name>.upstream_cx_total`, etc.) but no per-endpoint partitioning per ADR-0063 deferral (per-endpoint stats were explicitly out-of-scope for the 06.1 stats phase; round-robin LB across endpoints means no natural partition exists). Emitting cluster-level counters as the per-endpoint value would be misleading (the same value would appear under each endpoint's row, summing to 2× or 3× the cluster total when a comparator aggregates); emitting `0` is the conservative choice that the differential allow-list (SPEC §13.2 + Task 13's comparator) accommodates by fully allow-listing the 8 per-endpoint cx_/rq_ fields per endpoint on both sides.

For `/listeners`, the §11.3 verbatim scrape establishes a one-line-per-listener layout: `<name>::<bind_addr>` (e.g. `l_main::0.0.0.0:10000`). No additional fields are emitted by Envoy v1.37.2 in text mode; the JSON form (`?format=json`) is deferred per ADR-0089. Field-extension proposals from BRAINSTORM §2.2 (e.g. active connection count) are deferred since upstream Envoy v1.37.2 emits ONLY `<name>::<addr>` per listener — adding fields would diverge from byte-shape parity.

The bind address for `/listeners` is resolved via the existing `Listener.GetAddress()` proto walk surfaced by `lm.Listeners()` (which already populates `ListenerInfo.Addr` in `host:port` form per phase 02 / 07.2). No new accessor is needed.

### Decision

Both endpoints emit `Content-Type: text/plain; charset=UTF-8` (per SPEC §11.6 + ADR-0014). Both bodies are line-oriented (`\n` line terminator, no leading or trailing blank lines, no implicit framing characters).

**`/clusters`** (per SPEC §11.2 + Task 7):

For each cluster in alphabetical-by-name order (`s.cm.Clusters()` returns alphabetically sorted per SPEC §6.2), emit:

1. 10 cluster-level lines, in this exact order: `<name>::observability_name::<name>`, `<name>::default_priority::max_connections::1024`, `<name>::default_priority::max_pending_requests::1024`, `<name>::default_priority::max_requests::1024`, `<name>::default_priority::max_retries::3`, `<name>::high_priority::max_connections::1024`, `<name>::high_priority::max_pending_requests::1024`, `<name>::high_priority::max_requests::1024`, `<name>::high_priority::max_retries::3`, `<name>::added_via_api::false`.
2. 18 per-endpoint lines per endpoint (in bootstrap-declared order, NOT alphabetical), in this exact order: `<name>::<addr>::cx_active::0`, `<name>::<addr>::cx_connect_fail::0`, `<name>::<addr>::cx_total::0`, `<name>::<addr>::rq_active::0`, `<name>::<addr>::rq_error::0`, `<name>::<addr>::rq_success::0`, `<name>::<addr>::rq_timeout::0`, `<name>::<addr>::rq_total::0`, `<name>::<addr>::hostname::`, `<name>::<addr>::health_flags::healthy`, `<name>::<addr>::weight::1`, `<name>::<addr>::region::`, `<name>::<addr>::zone::`, `<name>::<addr>::sub_zone::`, `<name>::<addr>::canary::false`, `<name>::<addr>::priority::0`, `<name>::<addr>::success_rate::-1`, `<name>::<addr>::local_origin_success_rate::-1`.

The 8 cx_/rq_ counter values are emitted as literal `0` per planner-time decision 8 (envoy-go has no per-endpoint stats per ADR-0063 deferral). The cluster-level constants (`1024`, `3`, `false`) and per-endpoint constants (`healthy`, `1`, empty, `false`, `0`, `-1`) are emitted unconditionally.

For the SPEC §7.3 fixture (one cluster `c_backend` with 2 endpoints), the body has exactly 10 + 2×18 = 46 lines.

**`/listeners`** (per SPEC §11.3 + Task 8 — references this ADR):

For each listener in alphabetical-by-name order, emit one line: `<name>::<bind_addr>`. The bind address is resolved via `Listener.GetAddress()` proto walk surfaced by `ListenerInfo.Addr` (already populated in `host:port` form by phase 02 / 07.2's listener-manager construction).

### Alternatives considered

(A) Render the per-endpoint cx_/rq_ counter values from the cluster-level counters in the stats registry (`cluster.<name>.upstream_cx_total`, etc.), repeating the cluster-level value under each endpoint's row. Rejected per planner-time decision 8: the same cluster-level value duplicated under each endpoint's row would be misleading (a comparator that sums per-endpoint values would see 2× or 3× the true total); endpoint-level partitioning of the cluster total would require a fair-share computation envoy-go does not perform; and the differential comparator's allow-list approach is simpler and correct on both sides.

(B) Emit the 8 per-endpoint cx_/rq_ counter lines with envoy-go's true per-endpoint values (which would require landing per-endpoint stats infrastructure — a feature explicitly deferred per ADR-0063). Rejected: out-of-scope for phase 08.1's MVP; the fix path (post-MVP feature) lands per-endpoint stats, at which point the `/clusters` handler will emit live values without changing the line layout (the 18 per-endpoint lines remain; only the values change from `0` to the observed counter readings).

(C) Render `/clusters` in JSON (`?format=json`) for "modern" tooling. Rejected: deferred per ADR-0089. Text mode is the operator default (`curl http://localhost:9901/clusters`), is the only mode upstream Envoy v1.37.2 emits when no `?format=` query param is present, and is simpler to byte-compare in the differential harness.

(D) Emit only the lines envoy-go actually models (skip the 10 cluster-level circuit-breaker constants and the 10 per-endpoint metadata constants). Rejected: violates byte-shape parity with upstream Envoy v1.37.2; the §13.2 differential allow-list would have to allow-list 18 lines per endpoint and 9 per cluster, eclipsing the differential equivalence claim; emitting Envoy's default-when-not-configured constants is a one-shot constant-string emission per line at zero runtime cost.

(E) Emit one extension field per listener (e.g. active connection count from existing 06.1 stats) for `/listeners`. Rejected: upstream Envoy v1.37.2 emits ONLY `<name>::<addr>` in text mode; adding fields would break byte-equality. Field extension is deferred to a future phase that may evaluate the JSON-form `?format=json` shape extension (which Envoy itself does not currently extend in text mode).

(F) Resolve `/listeners` bind address through the listener manager's runtime accept-loop state (e.g. `*net.Listener.Addr().String()`) rather than through `Listener.GetAddress()` proto walk. Rejected: the runtime accept-loop address is the resolved bind address (e.g. `127.0.0.1:10000` after binding `0.0.0.0:0`); the `/listeners` text-format contract emits the configured address from the bootstrap proto (e.g. `0.0.0.0:10000` as declared). Phase 02 / 07.2's `ListenerInfo.Addr` already surfaces the configured-bind-address form via the proto walk.

### Consequences

(a) The `/clusters` body is byte-equal to upstream Envoy v1.37.2 on the SPEC §7.3 fixture modulo the §13.2 differential allow-list: the 8 per-endpoint cx_/rq_ counter fields per endpoint are fully allow-listed (envoy-go emits `0`, Envoy emits the observed value from the 5-request load); all other 38 lines per cluster (10 cluster-level + 10 per-endpoint metadata × 2 endpoints + 8 cx_/rq_ counter line-skeletons) are byte-equal. Task 13's differential comparator parses both bodies into `(cluster_name, key, value)` tuple sets and applies the allow-list to the 8 cx_/rq_ keys per endpoint before tuple-set comparison.

(b) The `/listeners` body is byte-equal to upstream Envoy v1.37.2 on the SPEC §7.3 fixture (one listener `l_main` on `0.0.0.0:10000`); no allow-list is needed for `/listeners`. The framing deviation from §6.6 (envoy-go emits `Content-Length`; Envoy emits `transfer-encoding: chunked`) is handled by the differential harness's existing dechunk preprocessor (mirrors `/ready`'s handling).

(c) ADR-0063's per-endpoint-stats-deferral is reaffirmed and explicitly cross-referenced. Future stats-hardening phase that lands per-endpoint stats supersedes the planner-time decision 8: the `/clusters` handler will then read live per-endpoint values (no line-layout change; only the 8 emitted values change from `0` to live counter readings), and the §13.2 allow-list narrows back to a tolerance band (e.g. ±1 for round-robin LB skew across the 5-request §7.3 load) on the 8 fields. The 10 cluster-level constants and 10 per-endpoint metadata constants are NOT affected by per-endpoint stats addition (they remain Envoy default-when-not-configured constants).

(d) `/listeners` stays trivial: one line per listener, no extension fields anticipated until 08.2 may evaluate (08.2's drain semantics may surface a per-listener drain-state field; that extension would be additive at the line tail, e.g. `<name>::<addr>::draining`, and would land a new ADR superseding this one's `/listeners` clause). The listener bind address is resolved via the existing `ListenerInfo.Addr` field (no new accessor; no proto-walk in the admin handler).

(e) The two text-format endpoints share a single body-shape ADR rather than two ADRs because their decisions are tightly coupled: both emit text/plain, both use `\n` line terminators, both order entries alphabetically by name, both walk the existing snapshot accessors (`s.cm.Clusters()` and `s.lm.Listeners()`), and both omit the JSON form per ADR-0089. Splitting into ADR-0087a and ADR-0087b would duplicate the rationale; ADR-0004's anti-fragmentation guidance favors consolidation when the decisions are coupled.

---

## ADR-0088: `/server_info` body shape — protojson over `*adminv3.ServerInfo` with the `LIVE`/`PRE_INITIALIZING` state-enum coverage; reuse `configDumpMarshalOptions`; partial `command_line_options{config_path}`; `hot_restart_version: "disabled"` constant

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.1 PLAN Task 9 (`internal/admin/serverinfo.go`).

### Context

`/server_info` is the fourth read-only operator-introspection admin endpoint in phase 08.1. Like `/config_dump` (settled by ADR-0086), it is JSON-rendered via protojson over a top-level admin/v3 proto — `*adminv3.ServerInfo` — but the proto's field set is much smaller (eight scalar/message fields per `envoy.admin.v3.ServerInfo`: `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `hot_restart_version`, `command_line_options`, `node`). The body shape is pinned by SPEC §11.4's empirical scrape against upstream Envoy v1.37.2 with the SPEC §7.3 fixture — duration values render as `"<N>s"` strings (durationpb's protojson form), the state enum renders as the upper-case enum-name string (`"LIVE"`, `"PRE_INITIALIZING"`), the partial `command_line_options` carries only `config_path` (envoy-go does not model the other CommandLineOptions fields — log levels, base id, restart epoch, etc.), and `hot_restart_version` is the literal string `"disabled"` (envoy-go has no hot-restart machinery, by ADR-0001).

The state-enum coverage decision was explicitly delegated to planner-time (decision 4). The four `ServerInfo_State` enum values defined by `envoy.admin.v3.ServerInfo` are `LIVE`, `DRAINING`, `PRE_INITIALIZING`, and `INITIALIZING`. envoy-go has no xDS init phase that survives admin-server bind (the admin server starts AFTER `bs.Load` + cluster/listener manager construction, which is the totality of "init" in envoy-go's static-bootstrap-only model), so `INITIALIZING` is unreachable in MVP — there is no observable window during which a request to `/server_info` could observe `INITIALIZING` rather than `LIVE` or `PRE_INITIALIZING`. `DRAINING` is 08.2's deliverable: phase 08.2 lands `POST /drain_listeners` and the corresponding state transition; until 08.2, no envoy-go process can be in the draining state. SPEC §11.7 documents the structural sibling: ADR-0015's pre-init contract for `/ready` covers the same gate transition, on a different endpoint, with the same `LIVE`/`PRE_INITIALIZING` enum coverage and the same MarkReady atomic flip.

The version-string format was settled by planner-time decision 1 + ADR-0086's `BuildVersionString()` consequence (option A — 5-token mirror of Envoy's `<sha>/1.37.2/Clean/RELEASE/BoringSSL` shape, with envoy-go substituting `Go-crypto` for `BoringSSL`); the field is allow-listed in the §13.2 differential matrix because envoy-go's revision/Go-version differs from Envoy's release-tag/C++-version on every build by construction. The uptime values use `durationpb.New(time.Since(s.bootTime))` where `bootTime` is set at `admin.New` time (per ADR-0085's constructor widening, which threads the value into the Server struct); both `uptime_current_epoch` and `uptime_all_epochs` carry the same value because envoy-go has a single epoch (no hot-restart, no epoch-rollover semantics).

The `command_line_options.config_path` value is read from `s.bs.ConfigPath`, the field added by Task 2 and assigned by `cmd/envoy-go/main.go` post-Load (Task 10). The other CommandLineOptions fields (log-level, component-log-level, base-id, restart-epoch, etc.) are emitted as zero-values via `EmitUnpopulated: true` in the reused MarshalOptions — the body shape will carry e.g. `"base_id": "0"`, `"restart_epoch": 0`, etc., matching upstream Envoy's empirical scrape on a fixture that does not configure those fields. The `node` field is sourced from `bs.Proto.GetNode()` (proto3-nil-safe; returns an empty Node when the bootstrap declares no `node`).

### Decision

The `/server_info` handler renders `application/json` via the SAME `configDumpMarshalOptions` package-level `protojson.MarshalOptions` tuple introduced by ADR-0086 and consumed by Task 6's `/config_dump` handler. The four-value tuple is `Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true`. Reuse (not redefinition) is the contract — both endpoints round-trip protojson over admin/v3 messages under one shared MarshalOptions for cross-endpoint body-shape consistency (ADR-0086 consequence (d)).

The `*adminv3.ServerInfo` value is assembled by `buildServerInfo(s *Server) *adminv3.ServerInfo` in `serverinfo.go` from the Server's threaded fields:

- `Version = BuildVersionString()` — the ADR-0086 5-token format.
- `State = deriveState(&s.ready)` — returns `adminv3.ServerInfo_LIVE` when `s.ready.Load()` is true, else `adminv3.ServerInfo_PRE_INITIALIZING`. INITIALIZING is unreachable in 08.1 MVP; DRAINING is 08.2's deliverable.
- `UptimeCurrentEpoch = UptimeAllEpochs = durationpb.New(time.Since(s.bootTime))` — same value, single epoch (no hot-restart).
- `HotRestartVersion = "disabled"` — literal constant.
- `CommandLineOptions = &adminv3.CommandLineOptions{ConfigPath: s.bs.ConfigPath}` — partial; other fields stay zero-valued (emitted via `EmitUnpopulated`).
- `Node = s.bs.Proto.GetNode()` — proto3-nil-safe; returns empty Node when bootstrap has no `node`.

Defensive nil-handling: when `s.bs == nil` (only encountered in tests that do not exercise this endpoint, but defended for code robustness), `configPath` stays empty and `node` stays nil; the body still renders as a valid JSON document. A `protojson.Marshal` error is recovered + logged + synthesized as `500 Internal Server Error` with `{}` body, mirroring `/config_dump`'s defensive shape.

The state-enum coverage in 08.1 is exactly two values: `LIVE` (post-MarkReady) and `PRE_INITIALIZING` (pre-MarkReady). 08.2 will extend the coverage by adding `DRAINING`; the extension lands as an ADR amendment to this ADR (not a superseding ADR — ADR-0004's anti-fragmentation guidance favors amendment when the addition is purely additive). `INITIALIZING` is documented as a defined enum value in `adminv3.ServerInfo_State` but never emitted by envoy-go in any phase — there is no observable code path that produces it.

### Alternatives considered

(A) Use a separate `protojson.MarshalOptions` tuple for `/server_info` (e.g. with `EmitUnpopulated: false` to elide zero-valued CommandLineOptions fields). Rejected: the differential comparator would need to negotiate two distinct body shapes (one per endpoint) against the same upstream Envoy convention, doubling the comparator's allow-list discipline. Reuse of `configDumpMarshalOptions` keeps the cross-endpoint body shape uniform; ADR-0086 consequence (d) anticipated this reuse.

(B) Cover all four `ServerInfo_State` enum values in 08.1 (including `DRAINING` and `INITIALIZING`) by introducing additional atomic flags on the Server struct. Rejected: `DRAINING` requires the drain machinery itself, which is 08.2's deliverable (the state would never transition out of `LIVE` until 08.2's `/drain_listeners` lands). `INITIALIZING` is unreachable in static-bootstrap-only mode — the admin server starts after init completes, so the gate is binary (pre-MarkReady is observable only because the admin server is up by then). Adding flags that can never flip would be code without a code path.

(C) Emit a synthetic `command_line_options.log_level` (or other CommandLineOptions fields) populated from envoy-go's runtime configuration. Rejected: envoy-go has no command-line model for log levels (logs go through `log.Printf` to stderr at default level; ADR-0067 covers access-log paths separately). Emitting a synthetic value would create a source of differential noise without operator value. The §13.2 allow-list covers the zero-valued CommandLineOptions fields uniformly.

(D) Format duration as a sub-second decimal (e.g. `0.123s`) rather than rounded integer seconds. Rejected: durationpb's protojson form is the canonical rendering; `durationpb.New(time.Since(bootTime))` produces the appropriate sub-second precision (`"0.020s"` for 20ms, `"5.012s"` for 5s 12ms, etc.) with no manual formatting. The differential comparator uses a tolerance-band on the uptime fields per §13.2 (uptime values differ by request-arrival timing on each side); the format itself is byte-shape consistent.

(E) Compute `uptime_current_epoch` from a per-request `time.Now()` minus `bootTime`, but `uptime_all_epochs` from a hypothetical "first-epoch" timestamp distinct from `bootTime`. Rejected: envoy-go has a single epoch by construction (no hot-restart). The two fields carry the same `*durationpb.Duration` reference; if a future hot-restart-equivalent feature lands (it will not — hot-restart is excluded by ADR-0001), the two values would diverge at that point.

(F) Emit `hot_restart_version` as the empty string `""` rather than the literal `"disabled"`. Rejected: upstream Envoy v1.37.2's empirical scrape on a build without hot-restart support emits a non-empty string carrying the hot-restart RPC version; envoy-go's literal `"disabled"` is the operator-readable signal that the feature is absent (ADR-0001's exclusion). The §13.2 allow-list covers the field uniformly because the value differs across the two implementations by construction.

### Consequences

(a) The `/server_info` body's equivalence claim against upstream Envoy v1.37.2 is post-MarkReady (both sides emit `"state": "LIVE"`); the pre-MarkReady body is documented but test-irrelevant in the differential harness because the cmd/envoy-go binary calls `s.MarkReady()` before any request arrives in normal operation. Pre-MarkReady is exercised only by the unit test (Task 9 step 1's `TestHandleServerInfo_StatePreMarkReady`); the structural sibling is ADR-0015's pre-init contract for `/ready`, which has the same "documented but test-irrelevant pre-init body" carry-forward.

(b) The same `configDumpMarshalOptions` four-value tuple is reused per ADR-0086 consequence (d). Future protojson-rendered admin surfaces (e.g. a hypothetical 08.2 JSON response on `/drain_listeners`, though current SPEC §1 has it return `OK\n` text) follow the same MarshalOptions tuple by default unless an explicit ADR supersedes the reuse.

(c) Phase 08.2's drain implementation extends the state enum coverage to `LIVE` + `PRE_INITIALIZING` + `DRAINING` by adding a third atomic flag (or by extending `s.ready` semantics — 08.2's PLAN settles the choice) and amending `deriveState` to return `adminv3.ServerInfo_DRAINING` when the drain flag is set. The amendment is purely additive; no other field changes. The ADR-0088 amendment will record the addition without superseding this ADR.

(d) The `version` field is allow-listed in the §13.2 differential matrix because envoy-go does not byte-compare against Envoy's version string (envoy-go's revision/Go-version differs from Envoy's release-tag/C++-version on every build). The `uptime_current_epoch` and `uptime_all_epochs` fields are tolerance-banded in the same matrix (per-side uptime differs by request-arrival timing). The `command_line_options.*` fields beyond `config_path` are allow-listed (envoy-go emits zero-values; Envoy emits its build-time defaults). The `hot_restart_version` field is allow-listed (envoy-go emits `"disabled"`; Envoy emits the build's hot-restart RPC version). The `node.user_agent_*` and `node.extensions[]` fields are allow-listed identically to ADR-0086's `/config_dump` claim. The `state` field IS byte-equal across both sides post-MarkReady (both emit `"LIVE"`).

(e) Task 13's differential comparator (or equivalent admin-endpoint comparator if `/server_info` gets its own tool) JSON-parses both Envoy's and envoy-go's `/server_info` bodies into `map[string]interface{}` (or `*adminv3.ServerInfo` via protojson `Unmarshal` with `DiscardUnknown: true`) and field-walks with the §13.2 allow-list applied. The parser depends on the field set being exactly the seven top-level fields named here; a future addition (e.g. an `envoy_*` extension field) would require a comparator update.

(f) The `INITIALIZING` enum value is documented in `adminv3.ServerInfo_State` but unreachable in envoy-go's static-bootstrap-only model. Future xDS-bearing phases (none planned in the current roadmap) would need to revisit this — at which point the state coverage would extend to all four values, and this ADR would be amended (not superseded) accordingly.

> **Phase 08.2 amendment (per ADR-0098):** the state-enum coverage extends to LIVE + PRE_INITIALIZING + **DRAINING**. The amendment is purely additive (no other field changes; per ADR-0088's own consequence (c) verbatim). `deriveState` signature widens from `(ready *atomic.Bool)` to `(ready *atomic.Bool, dm *drain.Manager)`; precedence DRAINING > LIVE > PRE_INITIALIZING. INITIALIZING remains unreachable.

## ADR-0089: Admin-endpoint deferral list — canonical enumeration of Envoy admin surface NOT modeled by envoy-go in 08.1 (per ADR-0040 deferral-ADR format)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (decisions written down). Per-ADR-0040 deferral format.
**Lands-in-task:** 08.1 PLAN Task 15 (BEHAVIOR_CONTRACT umbrella restructure + this ADR + ADR-0090 + phase-done bundle).

### Context

Phase 08.1 lands four read-only admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) on the existing `internal/admin.Server` mux per the parent BRAINSTORM §1 split (ADR-0084) and the SPEC §2 scope. Phase 08.2 lands `POST /drain_listeners`, the DRAINING transitions on `/ready` + `/server_info`, and the `## Graceful drain` BEHAVIOR_CONTRACT umbrella. Beyond the 08.1 + 08.2 combined surface (six read-only endpoints + one mutating drain endpoint + the DRAINING state extensions), the upstream Envoy v1.37.2 admin surface offers ~30 additional endpoints that envoy-go does NOT model and DOES NOT plan to model in any currently-roadmapped phase. SPEC §2.1 + §2.2 enumerate this deferred surface; the deferral list needs an explicit ADR per ADR-0040's deferral-format precedent so future admin-extensions phases can reference a single canonical list (and append to it, rather than create new per-feature deferral ADRs).

The deferral surface clusters into three families: (a) **mutating endpoints** that envoy-go would have to grow ACL machinery to safely expose (`POST /reset_counters`, `POST /quitquitquit`, `POST /healthcheck/fail`, `POST /healthcheck/ok`, `POST /reopen_logs`, `POST /runtime_modify`, `POST /logging?<level>`); (b) **read-only operator-introspection endpoints** that depend on subsystems envoy-go does not yet model (`/runtime` requires the Runtime layer / RTDS consumer; `/certs` requires SDS or static-cert introspection beyond what TLS-listener parsing exposes; `/memory` + `/heap_dump` + `/cpuprofiler` + `/heapprofiler` + `/contention` require profiling integration beyond `pprof` defaults; `/logging` mirrors `/logging?<level>` on the read side; `/listeners/<name>/*` requires per-listener stat scoping the current admin server does not implement; `/init_dump` requires the xDS init-manager subsystem); (c) **body-shape extensions on the four 08.1 endpoints** that envoy-go does not implement in MVP (`/config_dump?resource=`, `?mask=`, `?include_eds=` query-param filters; `?format=json` form on `/clusters` and `/listeners` returning structurally-richer JSON bodies; the four omitted ConfigDump envelopes — `RoutesConfigDump`, `SecretsConfigDump`, `ScopedRoutesConfigDump`, `EndpointsConfigDump`).

The trailing-slash + path-normalization deviation (Go stdlib `http.ServeMux` returns `404 page not found` on `/clusters/` etc., NOT Envoy's admin-help page) is a fourth category — body-divergent but status-code-equal — that the differential allow-lists structurally rather than carrying as a deferral. This ADR records the disposition for completeness.

### Decision

The complete enumeration of admin-surface deferrals NOT planned for any currently-roadmapped phase (08.1, 08.2, or any feature-family phase 09+) is fixed at this ADR. Each item carries a target phase (or `unscheduled` for items that have no current roadmap row). Future deferrals append to this list rather than create new ADRs unless the deferred item subsequently lands.

**(a) Mutating admin endpoints (security-hardening pre-requisite per ADR-0090):**

| Endpoint | Target phase |
|---|---|
| `POST /drain_listeners` | delivered in 08.2 per ADR-0093 |
| `POST /reset_counters` | unscheduled (security-hardening pre-requisite) |
| `POST /quitquitquit` | unscheduled (security-hardening pre-requisite) |
| `POST /healthcheck/fail` | unscheduled (active health checking family) |
| `POST /healthcheck/ok` | unscheduled (active health checking family) |
| `POST /reopen_logs` | unscheduled (operational tooling family) |
| `POST /runtime_modify` | unscheduled (Runtime layer / RTDS consumer family) |
| `POST /logging?<level>` | unscheduled (operational tooling family) |

**(b) Read-only admin endpoints (sub-system pre-requisite):**

| Endpoint | Pre-requisite | Target phase |
|---|---|---|
| `/runtime` | Runtime layer / RTDS consumer | unscheduled (Runtime + hot restart family) |
| `/certs` | SDS or static-cert introspection | unscheduled (xDS / dynamic config family) |
| `/memory` | Profiling integration | unscheduled (operational tooling family) |
| `/heap_dump` | Profiling integration | unscheduled |
| `/cpuprofiler` | Profiling integration | unscheduled |
| `/heapprofiler` | Profiling integration | unscheduled |
| `/contention` | Profiling integration | unscheduled |
| `/logging` (read-side) | Operational tooling | unscheduled |
| `/listeners/<name>/*` | Per-listener stat scoping | unscheduled |
| `/init_dump` | xDS init-manager | unscheduled (xDS / dynamic config family) |

**(c) Body-shape extensions on the four 08.1 endpoints:**

| Surface | Target phase |
|---|---|
| `/config_dump?resource=<name>` query-param filter | unscheduled |
| `/config_dump?mask=<paths>` field-mask filter | unscheduled |
| `/config_dump?include_eds=true` xDS endpoint dump | unscheduled (depends on xDS) |
| `/clusters?format=json` JSON form | unscheduled |
| `/listeners?format=json` JSON form (returns `{"listener_statuses": [...]}`) | unscheduled |
| `RoutesConfigDump` envelope on `/config_dump` | unscheduled (depends on RDS or extracted route view) |
| `SecretsConfigDump` envelope on `/config_dump` | unscheduled (depends on SDS) |
| `ScopedRoutesConfigDump` envelope on `/config_dump` | unscheduled |
| `EndpointsConfigDump` envelope on `/config_dump` | unscheduled (depends on EDS) |

**(d) Path-normalization / trailing-slash deviation (NOT deferred — allow-listed structurally):**

Go stdlib `http.ServeMux` does NOT canonicalise trailing-slash URLs (it returns `404 page not found` on `/clusters/`, `/server_info/` etc. with default body bytes, NOT Envoy's admin-help HTML body). The status-code is matched (404 vs 404); the body diverges. The differential harness allow-lists the trailing-slash body divergence structurally (the `0009-admin-config-dump` driver only scrapes the canonical paths). Adopting Envoy's admin-help HTML body would require ~200 LoC of static asset packaging and runtime path-normalization with no operator value at MVP scope. This is recorded for completeness — it is NOT a deferral, it is a documented permitted divergence.

### Alternatives considered

(A) Issue a separate per-feature deferral ADR for each of the ~25 deferred items. Rejected: the SPEC §8 anticipation table already lists this ADR as the canonical deferral list; ADR-0040's precedent (a single deferral ADR per phase rather than per-feature) supports consolidation; future readers benefit from a single grep-discoverable list rather than ~25 separate ADRs.

(B) Defer this ADR to 08.2's phase-done landing (pair it with the closing of the parent ROADMAP row). Rejected: the deferral list is already settled at 08.1 SPEC time; deferring the ADR until 08.2 would leave the BEHAVIOR_CONTRACT `### Does not yet apply to` block referencing an unanchored ADR. The §13 BEHAVIOR_CONTRACT umbrella restructure already cites `ADR-0089` as the deferral-list authority across nine bullet items; the ADR must land at 08.1 phase-done.

(C) Promote any of the deferred items into 08.1 scope. Rejected: the parent BRAINSTORM §10 + SPEC §2 already bound 08.1's scope to four read-only endpoints; widening scope at phase-done time violates the planner-time scope discipline (ADR-0045's split-gate — 08 was split into 08.1 + 08.2 specifically because the combined surface exceeded the gate threshold).

### Consequences

(a) The `BEHAVIOR_CONTRACT.md ## Admin API ### Does not yet apply to` block (per §13.1) cites `ADR-0089` for nine of the ten bullet items (the tenth, no-ACL posture, cites ADR-0090). This is the canonical cross-reference: any future reader investigating "is `/runtime` modeled?" greps `ADR-0089` and finds the deferral disposition with target-phase context.

(b) Future admin-extensions phases (none currently roadmapped) that land any of the deferred items append to this ADR's table (in-place edit per ADR-0052's BEHAVIOR_CONTRACT precedent applied to ADR text), rather than create new per-feature deferral ADRs. The lands-in disposition flips from `unscheduled` to the target phase id; the ADR text records the supersession via "ADR-0089 partially superseded by ADR-0091+" if and when it lands.

(c) The trailing-slash body divergence (item (d)) is a permitted divergence rather than a deferral — the `0009-admin-config-dump` differential driver does not scrape trailing-slash URLs; the divergence is structural (Go stdlib vs Envoy default body) with no operator surface to fix without packaging static HTML assets. This ADR records the disposition so a future security-review or operator-affordances phase can re-open the question with a new ADR if needed.

(d) The mutating-endpoint family (item (a) bullets 2-8) is gated on ADR-0090's no-ACL security posture. Adding any of these endpoints without an ACL would expose envoy-go's process to remote control by any local-network actor; the security-hardening pre-requisite is recorded explicitly in the table. ADR-0090's eventual partial supersession by an ACL-introducing ADR would unblock these endpoints' implementation.

(e) The body-shape extensions on the four 08.1 endpoints (item (c)) are non-breaking forward extensions: adding `?resource=<name>` filtering would extend the existing `/config_dump` handler with a query-param path that returns a subset of the current body; the differential equivalence claim from §13.2 (byte-equal modulo allow-list) extends naturally to the filtered output. This ADR records that the extensions are deferred for scope reasons rather than for any architectural blocker.

(f) The four omitted ConfigDump envelopes (item (c) bullets 6-9) depend on subsystems envoy-go does not model in MVP. `RoutesConfigDump` is the closest to landable (the static route configuration sits in HCM today; extracting a flat view would be ~50 LoC); the other three depend on SDS / xDS-scoped-routes / EDS that are out-of-scope for the MVP-trunk closure (per ADR-0001). This ADR records the disposition so a future routes-introspection ADR can supersede the deferral cleanly.

(g) ADR-0089's status remains `Accepted` even as items flip from `unscheduled` to scheduled; the disposition table is the live record. If the deferral list is fully exhausted (every item lands), the ADR transitions to `Historical` per ADR-0001's status taxonomy.

## ADR-0090: No-ACL admin-endpoint security posture — admin port is plaintext HTTP/1.1 with no authentication, no authorization, no method discrimination on read-only endpoints (operator firewall is the security boundary; mirrors upstream Envoy default)

**Status:** Accepted
**Date:** 2026-05-02
**Doctrine:** D-3.5 (decisions written down).
**Lands-in-task:** 08.1 PLAN Task 15 (BEHAVIOR_CONTRACT umbrella restructure + ADR-0089 + this ADR + phase-done bundle).

### Context

The envoy-go admin port is allocated by `internal/admin.Server.Start()` per the phase-01 contract (HTTP/1.1, plaintext, single-bind). Phase 06.1 added `/stats/prometheus`; phase 08.1 adds `/config_dump`, `/clusters`, `/listeners`, `/server_info`; phase 08.2 will add `POST /drain_listeners`. Throughout this evolution the admin port has carried no authentication, no ACL, no TLS, and no method discrimination on read-only endpoints — matching upstream Envoy v1.37.2's default admin posture (per BRAINSTORM §2.1 Decision G + SPEC §2.5).

The decision to mirror Envoy's no-ACL default rather than pre-emptively introduce ACL machinery is anchored on three facts: (a) upstream Envoy v1.37.2's empirical scrape (08.1 SPEC §11.8) shows POST/PUT/DELETE on the four read-only endpoints return 200 with the same body as GET — Envoy does NOT enforce method discrimination either; (b) the parent BRAINSTORM §10 commitment to "match Envoy parity at MVP scope; security hardening is a future-phase concern with its own brainstorm" — pre-emptive ACL machinery would diverge from Envoy parity for no operator benefit at MVP; (c) operator deployments universally firewall the admin port at the network boundary (k8s NetworkPolicy, hostNetwork=false + service-IP isolation, on-host iptables) — the security boundary is the operator's network policy, not application-level ACL. envoy-go does not have the request-routing primitives a real ACL would require (no JWT validation, no mTLS client-auth, no IP-list matching beyond what `Listener.address` already provides; the admin server uses the plain `http.ServeMux` from stdlib).

The mutating endpoint family (per ADR-0089 item (a)) — `POST /drain_listeners` (08.2), `POST /reset_counters`, `POST /quitquitquit`, `POST /healthcheck/*`, `POST /reopen_logs`, `POST /runtime_modify`, `POST /logging?<level>` — would be remote-control vectors if exposed to untrusted local-network actors. ADR-0089's deferral table records that these endpoints' landing is gated on this ADR's eventual partial supersession by a security-hardening ADR (ADR-0091+) that introduces an ACL primitive. Until such a phase brainstorms and lands, ADR-0090 is the operative posture: no-ACL, plaintext, operator-firewall-bounded.

### Decision

envoy-go's admin port carries the following security posture for phase 08.1 (and 06.1 + 08.2 by inheritance):

1. **No authentication.** No HTTP Basic, no JWT, no mTLS client-auth, no API token, no client-IP allowlist beyond what `Listener.address` already provides at bind time. Any actor with network reachability to the admin bind can issue any GET request to any endpoint.

2. **No authorization (ACL).** The same permission set applies to all clients: read access to the six read-only endpoints (08.1 + 06.1 + 08.2 GET surface). The 08.2 mutating endpoint (`POST /drain_listeners`) inherits the same posture — no per-method or per-role gating.

3. **No method discrimination on read-only endpoints.** POST / PUT / DELETE / HEAD on the four 08.1 read-only endpoints return 200 with the same body as GET (per 08.1 SPEC §11.8 empirical pin against Envoy v1.37.2). The Go stdlib `http.ServeMux` dispatches on path only; the `internal/admin/*.go` handlers do NOT inspect `r.Method`. This matches Envoy parity.

4. **No TLS on admin.** Admin remains plaintext HTTP/1.1 per the phase-01 contract. TLS termination is a separate `Listener` concern; the admin server is its own bind allocated by `Server.Start()`.

5. **Operator firewall is the security boundary.** The deployed-environment configuration (k8s NetworkPolicy, hostNetwork=false + service-IP isolation, on-host iptables, container-network isolation) is the load-bearing security primitive. envoy-go's documentation (`internal/admin/doc.go` + this ADR + the BEHAVIOR_CONTRACT `## Admin API` umbrella) records the no-ACL posture so operators are not surprised.

6. **Future security-hardening ADR (ADR-0091+) supersedes partially.** A future security-hardening phase (no current roadmap row; the WASM / observability / xDS families are higher priority per ROADMAP §"Feature Families") will brainstorm and land an ACL primitive. The ACL would gate the mutating endpoints from ADR-0089's table and could optionally gate the read-only endpoints. ADR-0091+ partially supersedes ADR-0090 by introducing the gating; the no-ACL default for read-only endpoints may persist as the un-configured baseline (per Envoy's `admin.access_log_path` precedent — opt-in security, not opt-out).

### Alternatives considered

(A) Pre-emptively add an HTTP Basic ACL with operator-supplied credentials at boot. Rejected: diverges from Envoy parity (Envoy does not require credentials by default); doubles the bootstrap surface (a new `Admin.basic_auth` field would need parsing + tests + ADRs); operator deployments universally firewall the admin port already, making credentials redundant in the common case and a config-burden in the uncommon case.

(B) Enforce method discrimination on read-only endpoints (return 405 on POST/PUT/DELETE). Rejected: contradicts the SPEC §11.8 empirical pin against Envoy v1.37.2 (Envoy returns 200 with the GET body on POST/PUT/DELETE for the four read-only endpoints); the differential equivalence claim from §13.2 would fail; operator-tooling (e.g. monitoring scripts that POST as a "tickle" pattern) would silently break.

(C) Add TLS to the admin port (mutual or one-way). Rejected: requires bootstrap-time cert distribution to the admin clients (Prometheus scrapers, operator CLIs); the operational burden exceeds the MVP-scope security benefit; the operator firewall already provides the analogous network-boundary protection; TLS on admin is recorded as a deferred surface in the BEHAVIOR_CONTRACT `### Does not yet apply to` block (per §13.1).

(D) Restrict admin to localhost-bind only (`address: 127.0.0.1`). Rejected: many deployment scenarios (remote stat scraping, sidecar Prometheus, cross-pod operator tooling) require non-loopback bind. The current `Listener.address` field already lets operators choose loopback or public bind per their topology; envoy-go does not over-constrain the choice. The default in the §7.3 fixture and the typical config IS loopback bind (`127.0.0.1`) — but the framework allows public bind when the operator's threat model permits it.

### Consequences

(a) The `BEHAVIOR_CONTRACT.md ## Admin API ### Does not yet apply to` block (per §13.1) cites `ADR-0090` for the no-ACL bullet ("ACL / authentication on admin port (no-ACL posture per ADR-0090)"). The "method discrimination on read-only endpoints (Envoy parity per SPEC §11.8; 405 enforcement deferred)" bullet implicitly inherits this ADR's posture decision — both bullets describe the same security-posture surface.

(b) ADR-0089's mutating-endpoint table (item (a)) is gated on this ADR's eventual partial supersession. The ADR-0089 → ADR-0090 dependency is forward-only: ADR-0089 names the deferred surface; ADR-0090 records why none of them can safely land at MVP scope; ADR-0091+ (hypothetical) would unblock specific endpoints by introducing an ACL primitive.

(c) The operator-firewall-as-security-boundary posture is recorded in `internal/admin/doc.go` package-level prose and in this ADR. Future operator-affordances brainstorms (no current roadmap row) may add CLI flags or bootstrap fields that further constrain admin bind (e.g. `--admin-bind-loopback-only` enforced flag); such changes would be additive and would not require ADR-0090 supersession.

(d) The 08.1 differential fixture `0009-admin-config-dump` exercises the no-ACL posture implicitly — the driver issues unauthenticated GET requests and asserts 200 on the four endpoints. If a future ADR introduces an ACL, the fixture's driver would need to either (i) configure the bootstrap to disable the ACL for the test run, or (ii) issue authenticated requests; the fixture is forward-compatible because the driver already supplies bootstrap files.

(e) ADR-0090 supersedes nothing (no prior ADR has the no-ACL posture explicitly). Implicit precedent in phase 01 (admin server allocated without ACL machinery) and phase 06.1 (`/stats/prometheus` added without ACL machinery) is now retro-anchored to this ADR; the implicit decisions become explicit.

(f) The BOOTSTRAP_PROMPT.md §7.5 phase-done six-gate checklist's gate (f) (BEHAVIOR_CONTRACT populated) checks for both this ADR and the corresponding `### Does not yet apply to` bullets; the cross-reference is grep-discoverable both ways.

(g) A future security-review session (per `superpowers:requesting-code-review` / `security-review` skill) may identify hardenings within the no-ACL posture (e.g. response-body redaction of cluster names that resemble secrets, `/config_dump` field-masking of TLS-context private-key references). Such hardenings extend ADR-0090's posture without superseding it; the no-ACL principle stays intact while specific data-exposure surfaces tighten.

**Phase 08.2 amendment (per ADR-0093):** the no-method-discrimination posture is qualified to **read-only endpoints only**. The mutating `POST /drain_listeners` endpoint (08.2's first mutating endpoint) DOES enforce method discrimination per SPEC §11.4 empirical pin (Envoy parity). Non-POST methods (GET, PUT, DELETE, HEAD) return 405 Method Not Allowed with the templated body `Method <METHOD> not allowed, POST required.\n`. The no-ACL posture is preserved verbatim — operator firewall remains the security boundary.

---

## ADR-0091: Drain state-machine shape — new `internal/drain/` package + `Manager` type + LBP-1 fifth application; lock-free hot path; caller-enforced timeout; DRAINED state observable only via channel close

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.2 (one-way constructor wiring at boot), D-3.4 (plan-size + scope-locality govern phase scope), D-3.5 (decisions written down).
**Lands-in-task:** 08.2 PLAN Task 2 (`internal/drain/` package — `Manager` type + `FuzzDrainTransitions`).

### Context

Phase 08.2 (graceful drain, SPEC §1) introduces drain state as a first-class boot-time concern shared across five actors: `cmd/envoy-go/main.go` (SIGTERM-handler, Task 11), `internal/admin.Server` (POST `/drain_listeners` handler, Task 7), `internal/listener.Manager` (Accept-loop fast-path, Task 5), HCM filter constructor (in-flight Inc/Dec hooks, Task 9), and TCP-proxy filter constructor (per-connection Inc/Dec hooks, Task 10). All five actors read and/or mutate a single drain state that evolves monotonically from LIVE to DRAINING to DRAINED.

The drain machinery is a hot-path concern for two of the five actors: the HCM filter's request-begin/end hooks (Inc/Dec) fire on every HTTP request; the listener.Manager's Accept-loop fast-path check (IsDraining) fires on every new connection attempt. Both must be lock-free under concurrent scrape load per SPEC §6.1's concurrency model. The other three actors fire the Drain trigger (admin handler + SIGTERM-handler) or observe the Done rendezvous (main.go teardown) at much lower frequency — sync.Once-level synchronization suffices for both.

The three-state machine shape (LIVE → DRAINING → DRAINED-as-channel-close, per SPEC §5.9) was chosen over simpler two-state (boolean `draining` flag + no rendezvous) because: (a) the Done rendezvous is the load-bearing signal for cmd/envoy-go/main.go's SIGTERM-handler to decide when teardown is safe — a boolean flag would require a polling loop; (b) the channel-close pattern is Go's canonical "broadcast to N concurrent waiters" primitive, with zero per-waiter overhead and no lock in the select path; (c) SPEC §5.9 explicitly specifies the rendezvous channel as a first-class API surface.

The LBP-1 (Linear Boot-time Provisioning) explicit-threading discipline has four prior applications: `*stats.Registry` (phase 06.1), `*HTTPRegistry` (phase 07.1), `*ListenerFilterRegistry` (phase 07.2), and the 08.1 `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` triplet threaded into `admin.New` (ADR-0085). Phase 08.2's `*drain.Manager` is the fifth LBP-1 application: allocated once at boot in `cmd/envoy-go/main.go`, threaded into `admin.New` (Task 3), `listener.NewManagerWithBaseDirAndAllowH2C` (Task 5), HCM filter constructor (Task 9), and TCP-proxy filter constructor (Task 10). The pattern is consistent and grep-discoverable via `grep -rn drain.Manager internal/ cmd/`.

The Manager does NOT enforce its configured timeout. This separation of concerns (per ADR-0095's rationale, landed at Task 11) keeps the Manager's API surface minimal and testable in isolation: the Manager exposes `Timeout()` so callers can retrieve the configured budget, but the mechanism that enforces the timeout — a `select { case <-m.Done(): case <-time.After(m.Timeout()): }` — lives in `cmd/envoy-go/main.go`'s SIGTERM-handler. This means the Manager's unit tests can verify rendezvous behavior without real-time sleeps, and the SIGTERM-handler's tests can verify timeout-enforcement independently.

DRAINED state is NOT publicly exposed via `State()` per SPEC §5.9's design rationale: making DRAINED observable via `State()` would require callers to poll (busy-loop on `m.State() == StateDrained`) rather than block on `<-m.Done()`; the channel-close rendezvous is strictly superior. `State()` continues to return `StateDraining` post-rendezvous; the only observable signal for DRAINED is the Done channel close.

Per planner-time decision 1 (PLAN §"Planner-time deferred-decision resolution"), `FuzzDrainTransitions` is SHIPPED (eleventh fuzzer overall; ~60 LoC; 30s budget per ADR-0018). The fuzzer asserts three invariants across arbitrary sequences of Inc/Dec/Drain operations: (i) state monotonicity (state never decreases — LIVE → DRAINING only); (ii) inflight balance (every Inc has a matching Dec before Done fires); (iii) Done fires exactly once after Drain has been called and inflight reaches 0.

### Decision

A new `internal/drain/` package with a `Manager` type implementing the three-state drain machine (SPEC §5.9 + §6.2). The implementation shape:

1. **Lock-free hot path.** `state atomic.Uint32` (load/store of `State` as `uint32`) + `inflight atomic.Int64` (per-request/per-connection counter). `IsDraining()` and `Inc()`/`Dec()` are single-atomic-op; no mutex, no channel write, no scheduler yield on the hot path.

2. **sync.Once Drain-guard.** `once sync.Once` on `Drain()` so that concurrent triggers from the admin handler (`POST /drain_listeners`) and the SIGTERM-handler are safe: exactly one caller transitions `state` from `StateLive` to `StateDraining`, and exactly one caller conditionally fires the rendezvous.

3. **sync.Once-equivalent close-done-guard.** `closeOnce sync.Once` on `close(done)` so that the done channel is closed exactly once regardless of whether the rendezvous fires at Drain-time (inflight already 0) or at Dec-time (last inflight Dec after Drain). A double-close of a channel panics; the `closeOnce` guard makes both code paths (Drain-time and Dec-time) safe under concurrent load.

4. **DRAINED is channel-only.** `State()` returns `StateLive` or `StateDraining`. `StateDrained` exists as a sentinel constant (for fuzzer/test use) but is never stored to `state`. The rendezvous is `done chan struct{}` (allocated at `New()` time; closed by `closeOnce`).

5. **Caller-enforced timeout.** `timeout time.Duration` is stored in the Manager and returned by `Timeout()`. The Manager itself does NOT start a timer; the SIGTERM-handler in `cmd/envoy-go/main.go` calls `m.Timeout()` and selects on `time.After(m.Timeout())` alongside `<-m.Done()`.

6. **FuzzDrainTransitions shipped.** Eleventh fuzzer; ~60 LoC; 30s budget per ADR-0018. Assets three invariants (state-monotonicity, inflight-balance, Done-fires) across arbitrary 0..8-op bit-sequences.

The Manager is the LBP-1 fifth application. Threading wiring (Tasks 3, 5, 9, 10, 11) lands in subsequent tasks; this ADR covers the package shape and invariants.

### Alternatives considered

(A) Boolean `draining` flag (atomic) + no rendezvous channel; main.go polls `m.IsDraining()` after Drain to busy-loop on inflight reaching 0. Rejected: busy-loop wastes CPU during the drain window (which may last up to 30s per ADR-0095); the channel-close pattern is Go's canonical broadcast primitive with zero per-waiter overhead; SPEC §5.9 explicitly specifies the Done rendezvous as a first-class API surface.

(B) Embed timeout enforcement in the Manager (internal goroutine started at `Drain()` time that fires `close(done)` after `timeout`). Rejected: conflates two orthogonal concerns — "did inflight reach 0" and "did the timeout expire"; makes unit tests depend on real-time sleeps (the test for timeout-enforcement must wait at least `timeout` wall-clock time); the SIGTERM-handler's select-on-two-channels idiom is cleaner and testable independently.

(C) Mutex-based state machine (sync.Mutex on all transitions). Rejected: Inc/Dec are hot-path operations called once per HTTP request (HCM, Task 9) and once per TCP connection (TCP-proxy, Task 10); under high concurrency (SPEC §6.1's 100-concurrent-scrape model), a mutex contends with every request/connection; atomic operations are strictly cheaper and sufficient for the three-state monotonic machine.

(D) Interface-based `Drainer` (`IsDraining() bool; Inc(); Dec(); Done() <-chan struct{}`) over concrete `*Manager`. Rejected: the five consumers all live within the same binary; there is no test-time swap-out requirement (test code uses real `*Manager` instances with small timeout values); interface indirection adds a vtable dispatch to every hot-path Inc/Dec call with no compensating benefit; consistent with ADR-0085's choice of concrete `*Type` over interface for boot-threaded dependencies.

(E) Package-level global `var DrainMgr *Manager`. Rejected: violates LBP-1 (the project's five-prior-applications-strong constructor-threading discipline); package globals make test isolation impossible (two `TestXxx` functions in the same test binary cannot have independent drain states); grep-discoverability of the threading wiring (`grep -rn drain.Manager`) fails on a global.

### Consequences

(a) Race-detector-clean for N concurrent goroutines calling Inc/Dec interleaved with Drain, as asserted by `TestConcurrentIncDec` (100 goroutines × 1000 Inc/Dec pairs) and by `go test -race` in the Task 2 acceptance gate. The concurrent-trigger case (two goroutines both calling `Drain()` simultaneously) is guarded by `sync.Once` and asserted by `TestIdempotentDrain` (50-goroutine fan-in). Extended `TestAdminConcurrentScrapeRace` (Task 7, Task 13) will assert race-clean across all seven admin endpoints + a separate goroutine firing Drain mid-test.

(b) The five consumers' boot wiring is grep-discoverable via `grep -rn drain.Manager internal/ cmd/`. Tasks 3, 5, 9, 10, 11 each add one threading site; the completed grep output (Task 13 phase-done gate) must show exactly five non-test-file match lines.

(c) Future hot-restart family (per ADR-0099 deferral, Task 12) extends this Manager rather than replacing it; SCM_RIGHTS-based parent-child handoff is out of MVP scope. A future hot-restart ADR would add a `HotRestart(newEpochManager *Manager)` method or an epoch-aware variant — the current Manager's API surface is forward-compatible with that extension.

(d) `FuzzDrainTransitions` is the eleventh fuzzer (per ADR-0018's fuzz-CI 30s short-budget policy). The ten prior fuzzers continue to pass; the eleventh is independently seeded with three corpus seeds (bit-patterns 0b10101010/5, 0b00000001/1, 0b11111111/8) and achieves ~49M executions in 30s on the Task 2 acceptance-gate run.

(e) ADR-0085's consequence (d) anticipated that phase 08.2 "may add a single `drainState atomic.Pointer[DrainState]` field on `Server` (or equivalent) without changing `New`'s signature." Task 2's `*drain.Manager` design supersedes that anticipation: rather than an inline atomic field on `admin.Server`, the drain state lives in a dedicated package-level Manager that is constructor-threaded into `admin.Server` (Task 3). The `admin.New` signature DOES widen by one parameter (`drainMgr *drain.Manager`), contrary to ADR-0085's consequence (d) prediction. The divergence is intentional and superior: LBP-1 threading into `admin.New` is consistent with the four prior applications; an inline atomic on `Server` would have duplicated the inflight-counter machinery that HCM and TCP-proxy also need.

## ADR-0096: In-flight-completion HCM/TCP-proxy hooks + cluster.Manager.Drain consolidated — three-part drain discipline; cluster pool-close anchor

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (defensive-correctness > optimistic-performance), D-3.5 (decisions written down).
**Lands-in-task:** Task 4 (anchor: `internal/cluster.Manager.Drain()` + `Cluster.closePool()` stub); Tasks 9, 10 realize the HCM/TCP-proxy in-flight-completion components.

### Context

SPEC §6.6 + §6.7 + §11.3 + BRAINSTORM Decisions 7 + 8 consolidated. Phase 08.2 introduces a three-state drain machine (ADR-0091) with a rendezvous channel (`drainMgr.Done()`) that fires once inflight reaches 0 after Drain has been called. Two filter families must participate in the inflight counter to make the rendezvous meaningful:

1. **HCM filter (Task 9):** Each HTTP request that passes the filter chain contributes one Inc at request-begin and one Dec at request-end. For HTTP/1.1 keep-alive connections, multiple requests on the same connection each independently Inc/Dec (per-request granularity, NOT per-connection). This is the correct granularity because each request is independently schedulable: a slow backend response keeps one inflight unit pinned; the next pipelined request on the same conn is a separate unit.

2. **TCP-proxy filter (Task 10):** Each proxied TCP connection contributes one Inc at connection-begin (top of `Handle`, after `ctx.Err()` check, before `Dial`) and one Dec via `defer` immediately after Inc (per-connection granularity). This is correct for TCP-proxy because there is no per-request signal: the connection IS the unit of work.

3. **cluster.Manager.Drain (this task):** After `<-drainMgr.Done()` fires (no in-flight downstream requests remain, therefore no in-flight upstream requests can remain), `cm.Drain()` closes upstream connection pools. This is best-effort: Go's runtime will close sockets on process exit regardless; the explicit close allows cleanest release of socket file descriptors before the deferred-stop chain runs.

SPEC §11.3 empirical evidence establishes that envoy-go does NOT mark in-flight H1.1 keep-alive responses with `Connection: close` — matching upstream Envoy behavior. Subsequent requests on the same conn during the DRAINING window each contribute additional Inc/Dec pairs, extending the drain window. This is the deliberate MVP simplification: per-conn drainable-close-at-next-idle-window (where Envoy would close the connection after the response completes during DRAINING) is deferred per SPEC §2.1.

A `markedInflight bool` sentinel field on the per-request HCM stream struct ensures Inc/Dec pair-balance under the `sendLocalReply` early-exit path per ADR-0075. Without the sentinel, a request that hits `sendLocalReply` (e.g., 502 Bad Gateway from upstream dial failure) could Dec without a prior Inc (if the early-exit fires before the request-begin Inc), or could double-Dec (if the early-exit fires in a code path that also calls the normal request-end Dec).

`closePool()` lands as a stub today because `internal/cluster.Cluster` carries no exported connection-pool field at this point in the codebase's evolution (phase 02 dials per-request without keep-alive pooling; phase 05.2 H2 `ClientConn` instances have no exported close hook today). The stub is a forward-extensible hook: the future operator-affordances phase adds a pool field, and `closePool` grows to drain it without changing `Manager.Drain()`'s API. Per planner-time decision 6.

### Decision

Three-part discipline:

1. **HCM (Task 9):** inflight `Inc` at request-begin (per stream — multiple keep-alive requests on one H1.1 conn each Inc/Dec independently); `Dec` at request-end (post-access-log per phase 06.2). A `markedInflight bool` sentinel field on the per-request struct ensures pair-balance under `sendLocalReply` per ADR-0075. When `IsDraining()` is true, the HCM does NOT inject `Connection: close` — Envoy parity per SPEC §11.3 empirical evidence; the drain window extends naturally via further Inc/Dec pairs on the existing keep-alive connection.

2. **TCP-proxy (Task 10):** inflight `Inc` at conn-begin (`Handle` top, after `ctx.Err()` check, before `Dial`); `Dec` via `defer` immediately after `Inc` (per-connection granularity). The deferred Dec fires when `Handle` returns — after both `io.Copy` directions complete and the connection is torn down.

3. **cluster.Manager.Drain (this task):** best-effort upstream-pool close after the rendezvous fires. `Manager.Drain()` walks `m.clusters` and calls `c.closePool()` on each. `closePool()` is a no-op stub today; future expansion grows the per-cluster pool-close logic without changing the `Drain()` API.

### Alternatives considered

(A) Per-connection Inc/Dec in HCM (not per-request). Rejected: HTTP/1.1 keep-alive connections carry multiple independent requests; Inc/Dec at connection granularity would cause the drain window to undercount in-flight work (a slow request on a keep-alive conn holds inflight=1, but the next pipelined request would not bump inflight=2; the rendezvous could fire while requests are still in-flight on the connection). Per-request granularity is strictly correct.

(B) Inject `Connection: close` on every response during DRAINING (to prevent keep-alive from extending the drain window). Rejected per SPEC §11.3 empirical pin: upstream Envoy v1.37.2 does NOT do this for the `/drain_listeners`-triggered DRAINING path; envoy-go matches Envoy behavior as the MVP default. Per-conn drainable-close-at-next-idle-window is deferred per SPEC §2.1.

(C) Implement pool-close immediately (even with no pool fields today) by intercepting `connWithGauge.Close` calls to track open connections. Rejected: `connWithGauge` already Decs `upstream_cx_active` gauge on Close; adding drain tracking would conflate two concerns; the architectural close-after-rendezvous pattern means no upstream requests are in-flight when `cm.Drain()` runs anyway — there are no open connections to close that aren't already being closed by the request/connection teardown.

(D) Omit `cluster.Manager.Drain()` entirely (process exit closes sockets). Rejected: explicit pool-close is the cleanest signal for future hot-restart family work (where the parent process must NOT hold onto upstream FDs that the child process is about to own); the `Drain()` API provides the correct architectural hook even if today's implementation is a no-op stub.

### Consequences

(a) Per SPEC §11.3 empirical evidence, envoy-go does NOT mark in-flight H1.1 keep-alive responses with `Connection: close` — Envoy parity. Subsequent requests on the same conn during DRAINING extend the drain window via further Inc calls (deliberate MVP simplification; per-conn drainable-close-at-next-idle-window deferred per SPEC §2.1).

(b) The `closePool()` stub today is a forward-extensible hook; future hot-restart/operator-affordances family expansion grows the per-cluster pool-close logic without changing the `Drain()` API.

(c) Race-detector-clean under `TestAdminConcurrentScrapeRace` (Task 12) extended with a Drain-mid-test goroutine.

(d) Tasks 9 and 10 cite ADR-0096 in their commit messages ("per ADR-0096") without re-anchoring; this ADR entry is the single authoritative record of the consolidated three-part discipline.

## ADR-0094: Listener-side drain plumbing — `internal/listener.Manager.Drain()` + Accept-loop fast-path

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (defensive-correctness > optimistic-performance), D-3.5 (decisions written down).
**Lands-in-task:** Task 5.

### Context

SPEC §6.6 requires `internal/listener.Manager` to expose a `Drain()` method that transitions the manager to drain mode. SPEC §11.5 records the empirical close-mechanism evidence from upstream Envoy v1.37.2: when `/drain_listeners` is called, new connection attempts are accepted at the TCP level (3-way handshake completes per `Connected to 127.0.0.1`) and then immediately closed with FIN (curl observes "Empty reply from server" — error 52; nc reads 0 bytes). This settle the choice between two candidate mechanisms:

(a) **Accept-then-FIN**: accept the connection (let the TCP handshake complete), then immediately `conn.Close()` it without dispatching to the filter chain. The client observes a TCP-established connection followed by FIN (empty body / EOF on first read).

(b) **Listener-socket-close**: close the listening socket so the kernel produces RST-on-no-listener for new connection attempts. The client observes a connection refused at the OS level before the TCP handshake.

Upstream Envoy v1.37.2 uses (a) for the `/drain_listeners`-triggered DRAINING path. BRAINSTORM Decision 5 confirmed this is the correct target behavior for envoy-go.

### Decision

`internal/listener.Manager.Drain()` is a public method that delegates to the central `drain.Manager.Drain()`. The actual stop-accepting mechanism is a per-runtime Accept-loop fast-path: at the top of each Accept iteration (after `Accept()` returns, after error-handling), the loop body checks `rt.dm.IsDraining()`; if true, the new conn is immediately `conn.Close()`'d and the loop continues without filter-chain dispatch. This produces the accept-then-FIN behavior matching §11.5 empirical pin verbatim.

The existing `listener.Manager.Stop()` method stays unchanged as the post-drain teardown step (closes the listening sockets). Stop is invoked from the deferred-stop chain in `cmd/envoy-go/main.go` AFTER `<-drainMgr.Done()`.

The `dm *drain.Manager` field is propagated field-locally onto each `listenerRuntime` (not chased through `*Manager` at Accept time) to minimize hot-path indirection. The `filterRegistry` HCM/TCP-proxy closures are widened to accept `dm`; the inner filter constructors will plumb `dm` through at Tasks 9/10 — until then the closures use `_ = dm` discard.

`NewManagerWithBaseDirAndAllowH2C` is widened to take `dm *drain.Manager` as the 9th parameter (LBP-1 fifth application carry-through; `nil` is safe for callers that do not yet wire drain).

### Alternatives considered

(A) **Listener-socket-close on drain** (mechanism b above). Rejected: upstream Envoy empirical pin at §11.5 unambiguously shows (a); envoy-go matches Envoy behavior as the MVP default. Listener-socket-close would also prevent new connections from seeing the TCP 3-way handshake complete, which is inconsistent with the observed Envoy behavior.

(B) **Chase through `*Manager` at Accept time** (access `rt.manager.dm` instead of `rt.dm`). Rejected: field-local `rt.dm` avoids one pointer dereference on every accepted connection in the hot path; the indirection is immaterial at low load but correct to avoid per ADR-0094 doctrine D-3.3.

(C) **Expose drain state directly on `listenerRuntime`** without threading through `Manager.Drain()`. Rejected: `Manager.Drain()` is the SPEC §6.6-prescribed API; the delegating-to-central-Manager pattern is consistent with `cluster.Manager.Drain()` (ADR-0096) and `admin.Server`'s drain integration (ADR-0091 + ADR-0093).

### Consequences

(a) New connections during drain receive accept-then-FIN per §11.5; the 06.1 +2-LoC accept-site Inc lines (`downstreamCxTotal` / `downstreamCxActive`) are NOT executed for the drained-conn case (the conn never enters `serveConnection`).

(b) In-flight `serveConnection` goroutines (running the HCM filter chain) continue to completion — they are not interrupted by `Drain()`. The drain window is the time between `Drain()` and all in-flight goroutines returning; `drain.Manager.Done()` fires when the inflight counter reaches zero.

(c) The fast-path is field-local (`rt.dm`) rather than chasing back through `*Manager` — minimizes the hot-path indirection per D-3.3.

(d) `cmd/envoy-go` remains broken until Task 11 wires the updated constructor (expected broken-window per LBP-1). `internal/filter/hcm/...` and `internal/filter/tcpproxy/...` are unaffected — their constructor signatures widen at Tasks 9/10.

---

## ADR-0093: POST /drain_listeners handler + method discrimination — first mutating admin endpoint; 405 on non-POST; fire-and-forget Drain(); ADR-0090 no-method-discrimination qualified to read-only endpoints

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (lock-free hot path), D-3.5 (decisions written down), D-3.7 (Envoy parity governs).
**Lands-in-task:** 08.2 PLAN Task 7 (`internal/admin/drain.go` POST handler + 405 method discrimination).

### Context

SPEC §6.3 + §11.1 + §11.4 + BRAINSTORM Decision 3. Two empirical pins settle the contract:

1. **§11.1** — POST `/drain_listeners` returns 200 OK with body `OK\n` (3 bytes: capital `OK` followed by single newline) + the standard six-header set per §11.6.
2. **§11.4** — non-POST methods (GET, PUT, DELETE, HEAD) return 405 Method Not Allowed with body `Method <METHOD> not allowed, POST required.\n` (38 + len(METHOD) bytes) + the standard six-header set per §11.6.

Both paths use `writeAdminHeaders(w, "text/plain; charset=UTF-8")` and the 08.1 pattern of net/http-auto-managed Date + Content-Length.

The §11.4 finding is a **SURPRISE** that contradicts BRAINSTORM Decision 3's hypothesis, which expected Envoy parity = no method check (mirroring the 08.1 read-only-endpoint posture per ADR-0090). Upstream Envoy v1.37.2 DOES enforce method discrimination on the mutating `/drain_listeners` endpoint — a deliberate departure from the no-method-discrimination posture it applies to read-only endpoints. This distinction is now encoded in envoy-go: read-only endpoints (per ADR-0090) remain method-agnostic; mutating endpoints (starting with `/drain_listeners`) enforce POST-only.

### Decision

Method discrimination check FIRST (return 405 with templated body `Method <METHOD> not allowed, POST required.\n` for non-POST). The 405 is a hard rejection — DOES NOT trigger drain.

On POST:

- Call `s.dm.Drain()` synchronously (sync.Once-guarded inside `drain.Manager`; subsequent POSTs no-op the state transition but return identical 200/`OK\n`).
- Fire-and-forget — does NOT block on `<-s.dm.Done()`.
- Does NOT trigger process exit (SPEC §6.3 — operator-driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT per §5.3 lifecycle).

The `?graceful=true` query-param is silently accepted per ADR-0041's silent-ignore precedent (envoy-go's drain is always graceful by construction).

Per planner-time-resolved nil-dm policy: a nil `s.dm` yields 500 Internal Server Error with body `drain manager not configured\n` (defensive — the operator gets a clear signal that the drain machinery is not wired). Production builds always thread a non-nil dm.

### Alternatives considered

(A) No method discrimination on `/drain_listeners` (mirror the read-only-endpoint no-method-discrimination posture from ADR-0090). Rejected: contradicts the SPEC §11.4 empirical pin against Envoy v1.37.2; differential equivalence claim from §13.2 would fail on non-POST method probes.

(B) Return 200 OK with no drain side-effect for non-POST (lenient passthrough). Rejected: silently swallows operator errors where a typo causes a GET to appear to "drain" but nothing drains; the 405 provides the operator a clear diagnostic.

(C) Block on `<-s.dm.Done()` (synchronous drain wait). Rejected: SPEC §6.3 explicitly separates the drain trigger from the completion signal; blocking the admin HTTP connection for the full drain window could exceed WriteTimeout; fire-and-forget + operator polling of `/server_info` state is the correct model.

(D) Return 500 with no-op (silent nil-dm) instead of 500 with body. Rejected: the nil-dm path is a wiring error visible at boot; the 500 body `drain manager not configured\n` gives the operator a direct diagnostic rather than a generic 500 that could be confused with a handler crash.

### Consequences

(a) `/drain_listeners` is the FIRST admin endpoint in envoy-go with method discrimination; the no-method-discrimination posture from ADR-0090 is qualified to **read-only endpoints only**.

(b) ADR-0090 is partially amended in-place per the ADR-0089 consequence (b) pattern (the no-ACL posture is preserved verbatim; only the no-method-discrimination posture is qualified to read-only endpoints). See ADR-0090 Consequences § Phase 08.2 amendment paragraph.

(c) The `/healthcheck/fail` endpoint stays in ADR-0089's deferral list — envoy-go MVP unifies the listener-drain (which §11.2 evidence ties to `/drain_listeners`) and load-balancer-disposition flip (which §11.2 evidence ties to `/healthcheck/fail`) under a single `drain.Manager` state machine; the differential gate's per-proxy trigger script normalizes (§7.2).

(d) Idempotent: subsequent POSTs return identical 200/`OK\n` without re-firing Drain. The `sync.Once` guard is inside `drain.Manager.Drain()` — the handler does not need its own idempotency mechanism.

(e) The ten unit tests in `internal/admin/drain_test.go` cover: POST fires + body exact + idempotent + graceful-param silently ignored + nil-dm 500 + GET/PUT/DELETE/HEAD all return 405 + header set correct.

---

## ADR-0097: `/ready` DRAINING-branch — 503 + `DRAINING\n` body; precedence DRAINING > PRE_INITIALIZING > LIVE; partial supersession of ADR-0015

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.2 PLAN Task 8 (`internal/admin/admin.go` `handleReady` DRAINING-branch).

### Context

SPEC §6.4 + §11.2 + §5.4 + BRAINSTORM Decision 9. The §11.2 empirical pin against upstream Envoy v1.37.2 verbatim settles the body shape: `/ready` returns `DRAINING\n` (9 bytes; status 503 Service Unavailable) with the standard six-header set per SPEC §11.6 when the server is in the draining state.

**SURPRISE finding:** upstream Envoy v1.37.2 ties `/ready` DRAINING to `/healthcheck/fail` (NOT `/drain_listeners` alone) — a distinct endpoint that flips the load-balancer-disposition flag independently from the listener-drain trigger. envoy-go MVP unifies these triggers under the single `drain.Manager` state machine: one `Drain()` call (fired by `POST /drain_listeners` or SIGTERM) transitions both the listener fast-path AND the `/ready` + `/server_info` state to DRAINING. The differential fixture's per-proxy trigger script normalises for upstream Envoy's separate `/healthcheck/fail` trigger per SPEC §7.2.

The precedence rule — which state wins when both drain and pre-init are possible — was left to the PLAN to settle. DRAINING takes highest precedence (DRAINING > PRE_INITIALIZING > LIVE) because: (a) a draining server should refuse new work regardless of whether MarkReady was ever called; (b) the DRAINING-first ordering is symmetric with `deriveState`'s DRAINING-first check on `/server_info` (ADR-0098) for cross-endpoint consistency; (c) the pre-init scenario where drain fires before MarkReady is a corner case (operator fires drain during boot) that should not silently resolve to PRE_INITIALIZING and mask the drain signal.

### Decision

NEW first branch in `handleReady`, inserted BETWEEN the six-header set and the pre-init check:

```go
if s.dm != nil && s.dm.State() == drain.StateDraining {
    body := []byte("DRAINING\n")
    h.Set("Content-Length", strconv.Itoa(len(body)))
    w.WriteHeader(http.StatusServiceUnavailable)
    _, _ = w.Write(body)
    return
}
```

The DRAINING branch uses the same inline header/write pattern as the existing PRE_INITIALIZING and LIVE branches (manual `Content-Length` set + `WriteHeader` + `Write`). The six-header set above the branch (Content-Type, Cache-Control, X-Content-Type-Options, Server) applies to all three state paths. `s.dm == nil` tolerance is preserved — the branch is guarded by `s.dm != nil`, so test code that passes nil for `dm` sees no behavioral change.

Precedence: DRAINING (new, highest) > PRE_INITIALIZING (existing, middle) > LIVE (existing, lowest). The existing PRE_INITIALIZING and LIVE branches are preserved verbatim.

### Alternatives considered

(A) Add DRAINING AFTER the pre-init check (precedence PRE_INITIALIZING > DRAINING > LIVE). Rejected: masks the DRAINING signal during boot; an operator who fires drain during initialisation should see DRAINING, not PRE_INITIALIZING.

(B) Separate drain and pre-init signals with a combined state (DRAINING_PRE_INIT). Rejected: over-engineering for a corner case; the three-state machine with priority ordering is sufficient and consistent with upstream Envoy's state model.

(C) Return 200 + body `DRAINING\n` during drain (mirror upstream's pre-1.37.2 behavior where drain did not affect /ready). Rejected: contradicts the §11.2 empirical pin; load-balancers that poll `/ready` for health-checking would continue sending traffic to a draining instance.

(D) Block the DRAINING response on `<-s.dm.Done()` (drain completion). Rejected: same reason as ADR-0093 alternative (C) — `/ready` is a health-check polling endpoint, not a drain-completion signal. Fire-and-forget + operator polling is the correct model.

### Consequences

(a) `/ready` returns `DRAINING\n` (503) once `Drain()` fires regardless of `MarkReady` state. Load-balancers that poll `/ready` will remove the instance from rotation immediately on drain.

(b) The differential fixture's per-proxy trigger script (SPEC §7.2) normalizes for upstream Envoy's separate `/healthcheck/fail` trigger — envoy-go fires drain via `POST /drain_listeners`; the script fires both `/drain_listeners` + `/healthcheck/fail` against upstream, then waits for `/ready` to return DRAINING on both sides before asserting equivalence.

(c) **ADR-0015 is partially superseded** by this ADR — the LIVE/PRE_INITIALIZING two-state coverage extends to LIVE/PRE_INITIALIZING/DRAINING three-state coverage. ADR-0015's verbatim pre-init body (`PRE_INITIALIZING\n`) and pre-init status (503) are preserved; this ADR adds the DRAINING branch and the precedence rule. ADR-0015 is amended in-place with a forward-pointer note per the ADR-0089 consequence (b) pattern.

(d) Four new unit tests cover the DRAINING path: `TestHandleReady_Draining` (smoke), `TestHandleReady_DrainingPrecedesLive`, `TestHandleReady_DrainingPrecedesPreInitializing`, `TestHandleReady_DrainingHeaders`.

---

## ADR-0098: `/server_info` `state` field DRAINING — `deriveState` signature widen; DRAINING-first check; purely additive amendment of ADR-0088

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.2 PLAN Task 8 (`internal/admin/serverinfo.go` `deriveState` widen + `buildServerInfo` call site).

### Context

SPEC §6.5 + §11.2 + BRAINSTORM Decision 10 + ADR-0088 consequence (c) verbatim: "Phase 08.2's drain implementation extends the state enum coverage to `LIVE` + `PRE_INITIALIZING` + `DRAINING` by adding a third atomic flag (or by extending `s.ready` semantics — 08.2's PLAN settles the choice) and amending `deriveState` to return `adminv3.ServerInfo_DRAINING` when the drain flag is set. The amendment is purely additive; no other field changes. The ADR-0088 amendment will record the addition without superseding this ADR."

The §11.2 empirical pin settles the enum render: protojson encodes the enum by name (`"DRAINING"` — the upper-case proto enum NAME), consistent with the existing `"LIVE"` and `"PRE_INITIALIZING"` renderings. No format change to the surrounding body; only the `state` field value changes.

The `deriveState` signature widen approach (threading `*drain.Manager` rather than a third atomic flag) was chosen at PLAN time for two reasons: (1) it is consistent with the LBP-1 explicit-threading discipline applied throughout phase 08.2 (ADR-0091); (2) it avoids adding a new exported flag that would need its own synchronization contract — `drain.Manager.State()` is already lock-free (atomic read per ADR-0091).

The DRAINING precedence matches ADR-0097's `/ready` precedence (DRAINING > LIVE > PRE_INITIALIZING) for cross-endpoint consistency: a load-balancer or operator observing both endpoints simultaneously sees the same state signal from both.

### Decision

`deriveState` signature widens from `(ready *atomic.Bool)` to `(ready *atomic.Bool, dm *drain.Manager)`. NEW first check:

```go
if dm != nil && dm.State() == drain.StateDraining {
    return adminv3.ServerInfo_DRAINING
}
```

Existing LIVE / PRE_INITIALIZING returns preserved verbatim. `buildServerInfo` call site updated to `deriveState(&s.ready, s.dm)`. `drain` import added to `serverinfo.go`.

The amendment is purely additive per ADR-0088 consequence (c): no other field in `*adminv3.ServerInfo` changes; the `configDumpMarshalOptions` reuse (ADR-0086 + ADR-0088) is unchanged; `INITIALIZING` remains unreachable per ADR-0088 consequence (f).

ADR-0088 is amended in-place per ADR-0089 consequence (b) pattern: the amendment record (appended to ADR-0088 Consequences) adds DRAINING to the enum-coverage table and refers to this ADR for the timing semantics.

### Alternatives considered

(A) Add a third `atomic.Bool` flag (`s.draining`) to `Server` and pass it to `deriveState` instead of `*drain.Manager`. Rejected: the flag would need its own synchronization contract and flip site; `drain.Manager.State()` already provides this — threading the Manager is simpler and consistent with LBP-1.

(B) Expose `drain.Manager.State()` as a boolean (`drain.Manager.IsDraining() bool`) and pass that boolean to `deriveState`. Rejected: a boolean snapshot loses the type-safety of `drain.StateDraining`; the `State()` method already returns a typed enum, and the DRAINING-first guard is a single one-liner either way.

(C) Supersede ADR-0088 rather than amend it. Rejected: ADR-0088 consequence (c) explicitly prescribes amendment ("the amendment is purely additive … the ADR-0088 amendment will record the addition without superseding this ADR"); ADR-0004's anti-fragmentation guidance favors amendment when the addition does not change existing decisions.

### Consequences

(a) `/server_info` `state` field renders `"DRAINING"` (proto enum NAME) when `Drain()` has fired, matching upstream Envoy v1.37.2's empirical scrape per §11.2. The existing `"LIVE"` and `"PRE_INITIALIZING"` renderings are preserved.

(b) ADR-0088 is amended in-place — Consequences § Phase 08.2 amendment paragraph appended. The state-enum coverage table extends to LIVE + PRE_INITIALIZING + DRAINING; INITIALIZING remains unreachable.

(c) Four new unit tests cover the DRAINING path: `TestHandleServerInfo_StateDraining` (smoke), `TestHandleServerInfo_StatePrecedence_DrainingOverLive`, `TestHandleServerInfo_StatePrecedence_DrainingOverPreInit`, `TestDeriveState_NilDrainManager` (nil-dm fallback to ADR-0088 two-state logic).

(d) The differential comparator (Task 13 / SPEC §13.2) can byte-compare the `state` field value post-drain across both sides: both upstream Envoy (after `/healthcheck/fail` + `/drain_listeners`) and envoy-go (after `POST /drain_listeners`) return `"state": "DRAINING"`. The per-proxy trigger script normalizes for the separate upstream trigger per SPEC §7.2.

---

## ADR-0092: SIGTERM-handler drain-then-exit — deliberate divergence from Envoy v1.37.2's SIGTERM=immediate-exit

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.2 PLAN Task 11 (`cmd/envoy-go/main.go` SIGTERM-handler upgrade).

### Context

SPEC §6.8 + §11.7 + BRAINSTORM Decision 2. The §11.7 empirical evidence pins Envoy v1.37.2's SIGTERM and SIGINT paths as STRUCTURALLY IDENTICAL: both produce immediate-exit with ~6-7ms round-trip (`caught X` → `shutting down server instance` → `exiting`; no observable drain delay). This SURPRISES and CONTRADICTS BRAINSTORM Decision 2's hypothesis, which assumed Envoy's SIGTERM = drain-then-exit and treated envoy-go's proposed drain-then-exit as Envoy parity. In reality, Envoy's drain machinery is triggered via the admin surface (`/drain_listeners` + `/healthcheck/fail`) — SIGTERM bypasses it entirely at v1.37.2.

envoy-go's existing `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` registration stays unchanged: both SIGTERM and SIGINT cancel the context and unblock `<-ctx.Done()`. The upgrade is in the body that runs AFTER `<-ctx.Done()`.

### Decision

The `<-ctx.Done()` body is upgraded from a bare block-until-signal to the drain-then-exit sequence per SPEC §6.8:

```go
<-ctx.Done()
log.Print("signal received; initiating graceful drain")
drainMgr.Drain()
select {
case <-drainMgr.Done():
    log.Print("drain rendezvous: in-flight reached 0")
case <-time.After(drainMgr.Timeout()):
    log.Print("drain rendezvous: timeout fired (best-effort)")
}
cm.Drain()
// deferred-stop chain runs as the function unwinds (LIFO: lm.Stop, admSrv.Close, sinks-close)
```

This is a **deliberate divergence** from upstream Envoy v1.37.2's SIGTERM=immediate-exit behavior. The rationale is operator ergonomics: most Kubernetes and cluster orchestrators send SIGTERM to a terminating pod expecting graceful drain (rolling-restart workflow). envoy-go's drain machinery honors this expectation.

Per planner-time decision 9: `cm.Drain()` is an explicit call after the drain rendezvous, not deferred. Deferred calls continue to run as the function unwinds (LIFO order: `lm.Stop`, `admSrv.Close`, sinks-close).

### Alternatives considered

(A) Preserve Envoy parity: bare `<-ctx.Done()` with immediate exit. Rejected: operator-unfriendly in Kubernetes rolling-restart workflows; envoy-go's drain machinery would be dead code on the SIGTERM path.

(B) Issue `drainMgr.Drain()` via SIGTERM and `drainMgr.Drain()` separately from the admin handler (two separate trigger paths). Accepted as-is: both the admin handler (`POST /drain_listeners`) and the SIGTERM path call `drainMgr.Drain()` — `Drain()` is idempotent (once-only via `sync.Once` per ADR-0091).

(C) Block `<-ctx.Done()` until drain completes without a timeout select. Rejected: drain without a timeout risks blocking indefinitely on hung in-flight connections; ADR-0095 establishes the 30s timeout bound.

### Consequences

(a) SIGTERM and SIGINT on envoy-go now trigger drain-then-exit. Kubernetes rolling-restart (SIGTERM) will wait up to 30s (per ADR-0095) for in-flight connections to complete before the process exits.

(b) The differential equivalence claim does NOT exercise the SIGTERM path — only the admin-trigger path (`POST /drain_listeners`) runs differentially (SPEC §13.2). The SIGTERM path is envoy-go-only structural-completeness.

(c) BEHAVIOR_CONTRACT.md `## Graceful drain ### Drain triggers` (§13.4 / Task 13) documents the divergence at the contract level: `SIGTERM` is listed as an envoy-go-only trigger with an explicit note that upstream Envoy v1.37.2 does not drain on SIGTERM.

(d) `drainMgr.Drain()` is idempotent: if the admin handler fires drain before SIGTERM arrives, the SIGTERM path's `drainMgr.Drain()` call is a no-op; the rendezvous select correctly picks up the already-closed `Done()` channel.

---

## ADR-0095: Drain timeout 30s envoy-go MVP default — deliberate divergence from Envoy v1.37.2's 600s default

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (capture empirical observations as ADRs when the contract source is not derivable from documentation), D-3.5 (decisions written down).
**Lands-in-task:** 08.2 PLAN Task 11 (`cmd/envoy-go/main.go` `drain.New(30 * time.Second)` boot site).

### Context

SPEC §11.7 verbatim re-validation of `"drain_time": "600s"` (Envoy v1.37.2 default; visible in the `/server_info` `command_line_options` field) + BRAINSTORM Decision 6. Envoy's drain_time default is 600 seconds (~10 minutes). Using 600s for envoy-go's drain timeout would block the differential gate for up to 10 minutes per test run on the graceful-drain fixture (Task 12) — unacceptable for a fast-feedback CI gate.

Per ADR-0091 design decision: the `drain.Manager` does NOT enforce the timeout internally. Callers select on `time.After(drainMgr.Timeout())` alongside `drainMgr.Done()`. This means the timeout value is a caller-side concern, not a Manager-side enforcement, and test code can construct `drain.New(10 * time.Millisecond)` for fast-path tests without fighting the Manager.

### Decision

The drain timeout is hardcoded `30 * time.Second` at the `cmd/envoy-go/main.go` boot site:

```go
drainMgr := drain.New(30 * time.Second)
```

The literal lives at the call site (not as a constant in the `drain` package) so test code and integration fixtures can construct `drain.New(<any duration>)` without importing a constant that encodes a production policy. The `Timeout()` accessor on `Manager` returns whatever duration was passed to `New`.

This is a **deliberate divergence** from Envoy v1.37.2's 600s default. The equivalence claim is over drain BEHAVIOR (in-flight counter mechanics, Done channel semantics, DRAINING state rendering) not timeout VALUE.

### Alternatives considered

(A) Use Envoy's 600s default. Rejected: would block CI differential gate for ~10 minutes per test run; unacceptable for a fast-feedback workflow.

(B) Use a flag (`--drain-timeout`) to make the timeout operator-configurable. Deferred: the operator-knob is a future runtime/hot-restart family phase concern. Hardcoding at the boot site keeps the MVP minimal; the literal's location (call site, not package constant) ensures refactoring to a flag later is a one-liner.

(C) Encode the 30s default as a constant in the `drain` package (`drain.DefaultTimeout`). Rejected: a package-level constant would be imported by test code and fixture runners, coupling their timing to the production default. The call-site literal keeps test construction free to choose any duration.

(D) Make the Manager enforce the timeout internally (blocking `Drain()` until either Done or timeout). Rejected: per ADR-0091 design — callers own the timeout select. Internal enforcement would make `drain.Manager` untestable at fast timescales without mocking `time.After`.

### Consequences

(a) Drain timeout is 30s in the envoy-go MVP binary. Kubernetes `terminationGracePeriodSeconds` should be set to at least 35s (30s drain + 5s headroom) for production-like deployments.

(b) The equivalence claim is over drain BEHAVIOR not timeout VALUE. The differential fixture (Task 12) uses `drain.New(10 * time.Millisecond)` in test helpers for near-instant drain completion; the production 30s literal is not exercised in the differential gate.

(c) Operator-knob to configure the timeout is deferred to a future runtime/hot-restart family phase. The boot-site literal is the single change point; a flag-parsing addition would be localized to `cmd/envoy-go/main.go`.

(d) The `Manager` itself does NOT enforce the timeout — callers select on `time.After` alongside `Done` per ADR-0091 design. This is visible in the Task 11 SIGTERM-handler block and the admin drain handler (Task 7 / ADR-0093); both use caller-side timeout selects.

## ADR-0099: Hot restart / parent-child handoff deferred to runtime + hot restart family — out of scope for 08.2 and the entire BOOTSTRAP_PROMPT.md §8 MVP trunk

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.5 (decisions written down). Per ADR-0040 deferral format.
**Lands-in-task:** 08.2 PLAN Task 13 (BEHAVIOR_CONTRACT restructure + this ADR + phase-done bundle; MVP-trunk-close commit).

### Context

SPEC §2.1 (Lifecycle non-goals) enumerates hot restart / parent-child handoff as explicitly out of 08.2's scope. BRAINSTORM Decision 11 settles the same scope boundary from the brainstorm session's first-principles analysis. BOOTSTRAP_PROMPT.md §9 (Runtime + hot restart family) is the canonical placeholder for this deliverable in the feature-family expansion post-MVP-trunk.

Hot restart in upstream Envoy v1.37.2 implements a multi-process orchestration protocol:

1. **SCM_RIGHTS file-descriptor transfer.** The parent process passes listening socket file descriptors to the child process via Unix-domain socket ancillary data (`SCM_RIGHTS`). The child binds to the inherited FDs and begins serving new connections without a rebind-gap.
2. **Shared-memory existing-connection table.** The parent and child share a region of shared memory that tracks the existing-connection state — specifically, per-connection identifiers and stats counters. This enables the child to drain the parent's in-flight connections without race conditions on connection close.
3. **Parent-shutdown-time orchestration.** After the child signals readiness (via the hot-restart protocol handshake), the parent begins its own drain-then-exit sequence. The parent's drain window is bounded by `parent_shutdown_time` (default 900s in Envoy v1.37.2). The parent exits via `exit(0)` after its in-flight connections complete or the timeout fires.
4. **Custom signal protocol (SIGUSR1 / SIGUSR2).** The hot-restart orchestration is driven by SIGUSR1 (child → parent: "I am ready; begin drain") and SIGUSR2 (parent → child: "I have drained; you may terminate me"). This signal protocol is separate from the SIGTERM/SIGINT pair handled by 08.2's SIGTERM-handler upgrade (ADR-0092).

None of these four mechanisms are present in envoy-go's MVP trunk (phases 00–08). Implementing them would require:
- A `unix.SendmsgN` / `unix.RecvmsgN` (or `syscall.RightsControlMessage`) call pair for FD transfer; only valid on Unix-family OSes.
- A `syscall.ShmOpen` / `mmap` pair for shared-memory state; or a Unix-domain socket with a custom length-prefixed protocol.
- A second goroutine (or process) managing the parent-shutdown-time orchestration, including a `time.After(parentShutdownTime)` watchdog.
- A `signal.Notify(sigsusrCh, syscall.SIGUSR1, syscall.SIGUSR2)` handler in `cmd/envoy-go/main.go`, distinct from the SIGTERM/SIGINT handler landed at 08.2 (ADR-0092).
- Modifications to `internal/listener.Manager` to accept inherited FDs and to export per-listener file descriptors for SCM_RIGHTS transfer.

This is a multi-phase deliverable. Phase 08.2's drain machinery (ADR-0091 + ADR-0092 + ADR-0094 + ADR-0096) is the prerequisite: the existing-connection drain protocol (Inc/Dec hooks, Done channel, SIGTERM-handler block) is the parent-side component that hot restart extends; it is not a substitute.

Cross-reference: ADR-0089 (admin-endpoint deferral list) records `POST /quitquitquit` and `POST /healthcheck/fail` as carrying adjacent deferrals in the same MVP scope-bounding cluster — those endpoints are semantically adjacent to hot restart (quitquitquit is the child's signal to the parent that it can exit; healthcheck/fail is the load-balancer disposition flip that complements the listener drain during a hot restart window). All three are deferred to the feature-family expansion together.

### Decision

Hot restart / parent-child handoff (SCM_RIGHTS FD transfer + shared-memory existing-connection table + parent-shutdown-time orchestration + custom signal protocol SIGUSR1/SIGUSR2) is **OUT OF SCOPE** for:

1. Phase 08.2 (graceful drain).
2. The entire BOOTSTRAP_PROMPT.md §8 MVP trunk (phases 00–08).

This decision is deferred to a future feature-family phase under BOOTSTRAP_PROMPT.md §9's "Runtime + hot restart family." The deferral is recorded in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Does not yet apply to`.

### Alternatives considered

(A) Implement a minimal hot-restart stub (FD transfer only, no shared-memory state). Rejected: a stub that transfers FDs without the shared-memory existing-connection table would produce a split-brain state where the parent and child both serve traffic on the same listeners without coordinating connection handoff. This would be WORSE than no hot restart (the operator would observe double-serving without the drain guarantee). A stub without the full protocol is not a useful deliverable.

(B) Implement hot restart in 08.2 as an optional feature behind a build tag or environment variable. Rejected: the four-component protocol (SCM_RIGHTS + shared memory + parent-shutdown orchestration + SIGUSR1/SIGUSR2) is the minimal correct implementation; a build-tag-optional stub (per (A)) would still be a broken stub. The effort is a full feature-family phase, not a Task 13 deliverable.

(C) Implement hot restart in a post-08.2 trunk phase (e.g., a hypothetical phase 09 before the feature families). Rejected: BOOTSTRAP_PROMPT.md §8 + §9 separates the MVP trunk (00–08, done at 08.2 phase-done) from the feature-family expansion (09+). Hot restart belongs in the §9 "Runtime + hot restart family" — deferring it there keeps the MVP trunk minimal and the feature families properly scoped.

### Consequences

(a) envoy-go MVP drain is one-process scope only. The SIGTERM-handler block (ADR-0092) and the POST /drain_listeners endpoint (ADR-0093) provide operator-driven single-process graceful drain without process handoff. This is sufficient for the Kubernetes `terminationGracePeriodSeconds` workflow (SIGTERM → drain window → exit), which is the primary operator workflow envoy-go targets in MVP.

(b) Future runtime / hot restart family phase delivers SCM_RIGHTS-based handoff. That phase will build on 08.2's drain machinery (ADR-0091 Inc/Dec hooks; ADR-0094 Accept-loop fast-path; ADR-0092 SIGTERM-handler block) as the parent-side component of the two-process handoff.

(c) The deferral is recorded in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Does not yet apply to` as: "Hot restart / parent-child handoff (deferred to runtime / hot-restart family per ADR-0099)."

(d) Cross-reference: ADR-0089 (parallel admin-endpoint deferral list) carries `POST /quitquitquit` and `POST /healthcheck/fail` as adjacent deferrals in the same MVP scope-bounding cluster. The `quitquitquit` endpoint is the child-to-parent signal in the hot-restart protocol (the child calls POST /quitquitquit to tell the parent it has drained and can exit); deferring quitquitquit is consequentially correct given this ADR. The `healthcheck/fail` endpoint is the load-balancer-disposition flip that complements the listener drain during hot restart; its deferral per ADR-0089 is also consequentially correct.

---

## ADR-0100: `internal/filter/http/fault/` package shape + boot registration + `FactoryCtx` framework extension

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.5 (record durable design rationale; the package shape is a contract that future filter authors mirror) + D-3.4 (the `FactoryCtx` extension is a framework-level invariant that future stat-bearing filters depend on).
**Lands-in-task:** Task 3 (phase 09); commit e80aa10. Code consequences span Task 2 (FactoryCtx extension at `internal/filter/http/types.go` — ADR text references the framework-extension that Task 2 lands) and Task 8 (boot registration line in `cmd/envoy-go/main.go`).

### Context

Phase 09 introduces `envoy.filters.http.fault` as the first member of the §9 HTTP filters family beyond the 07.x trunk three (`router`, `cors`, `envoygotest`). Per SPEC §6.1 the filter package needs (1) a public `TypeURL` constant for the boot wiring registration line, (2) a public `New` `HTTPFilterFactory` that the listener-manager threads through `parseHTTPFiltersChain`, (3) unexported types (`runtimeConfig`, `headerMatch`, `filter`) with the per-instance per-request state. The package shape mirrors `internal/filter/http/cors/` (the 07.1 precedent) — same `TypeURL` + `New` + `filter` decomposition, same dual-side `StreamDecoderFilter` + `StreamEncoderFilter` impl on the single `*filter` type, same boot-time registration via `httpReg.Register(fault.TypeURL, fault.New)` at `cmd/envoy-go/main.go`.

Fault is the first stat-bearing HTTP filter — cors / envoygotest / router register zero metrics on `*stats.Registry`; fault registers five (per ADR-0107). The pre-09 `FactoryCtx` carried only `Registry *HTTPRegistry` (the cross-filter-lookup field added at 07.1); fault's `New` needs (a) `*stats.Registry` to register its 5 stats at HCM-build time, and (b) the HCM's `stat_prefix` string to key those stats per the ADR-0061 `http.<stat_prefix>.<metric>` discipline. This is a framework-level invariant: every future stat-bearing HTTP filter (e.g., `header_to_metadata`, `local_ratelimit`, `rbac` if it carries policy_match counters) will need the same two fields.

The decision: WIDEN `FactoryCtx` to a 3-field struct `{Registry, Stats, StatPrefix}` rather than introducing a per-filter side-channel. This consolidates the boot-time framework surface, keeps the call-site ergonomic (`fault.New(tc, ctx)` continues to take a single struct), and avoids a second framework constructor signature variation.

### Decision

The `internal/filter/http/fault/` package consists of:

1. `doc.go` — package-level docs anchoring SPEC §6 + the eight ADRs (ADR-0100..ADR-0107).
2. `fault.go` — public surface (`TypeURL`, `New`) + unexported types (`faultStats`, `runtimeConfig`, `headerMatch`, `filter`) + parser (`parseRuntimeConfig` + `percentageToFloat`) + stats registration (`registerFaultStats`) + the per-request `*filter` with `StreamDecoderFilter` + `StreamEncoderFilter` method set.
3. `fault_test.go` (and follow-up `fault_route_test.go` / `fault_race_test.go` at later tasks) — TDD-discipline unit tests.

Boot registration line in `cmd/envoy-go/main.go` (Task 8): `httpReg.Register(fault.TypeURL, fault.New)`. Same shape as the existing three trunk lines for `router.New`, `cors.New`, `envoygotest.New`.

`FactoryCtx` widens to:

```go
type FactoryCtx struct {
    Registry   *HTTPRegistry
    Stats      *stats.Registry
    StatPrefix string
}
```

Per-field semantics:
- `Registry`: same as 07.1; the cross-filter lookup pointer for filters that need to inspect sibling factories. Untouched semantics.
- `Stats`: non-nil at HCM-build time per the ADR-0061 pre-Freeze discipline; nil-tolerated in test code per ADR-0085 (test code that does not exercise stat-bearing filters is not required to allocate a registry). Filters that consume `Stats` MUST guard nil per ADR-0085 (fault's `registerFaultStats` returns an all-nil `*faultStats` on nil registry; the field is unused by the four phase-09 stats but the discipline propagates).
- `StatPrefix`: the HCM's `stat_prefix` per the §13 stat-name flattening anchor; empty-tolerated in test code.

The framework's `parseHTTPFiltersChain` (in `internal/filter/hcm/config.go`) populates the two new fields from the HCM-build context. The 11 differential fixtures (0000..0010) PASS unchanged because their filters (router, cors, envoygotest) never read `Stats` / `StatPrefix`.

### Alternatives considered

(A) Add a per-filter side-channel (e.g., `fault.NewWithStats(tc, ctx, statsReg, prefix)`). REJECTED: the framework constructor signature would diverge per filter; future stat-bearing filters (`local_ratelimit`, `header_to_metadata`) would each need their own bespoke factory shape. Not scalable; future-readers of the codebase would have a per-filter mental model rather than the single framework model `HTTPFilterFactory`.

(B) Make stats registration LAZY inside the per-request `*filter` (first-DecodeHeaders allocates the `*Counter` on demand). REJECTED: violates the ADR-0061 pre-Freeze discipline — the `*stats.Registry` is frozen after boot; first-DecodeHeaders runs WAY after Freeze, so the lazy registration would panic at the first request. Stats MUST be registered at HCM-build time.

(C) Use a package-global `*stats.Registry` (mirror upstream Envoy's stats store singleton). REJECTED: contradicts the ADR-0059 LBP-1 invariant (no package-globals; explicit threaded constructor map). The threaded `FactoryCtx.Stats` is the LBP-1-compliant equivalent.

(D) Skip the `Stats` field; require fault's factory signature to widen instead (`func New(tc *anypb.Any, ctx FactoryCtx, sreg *stats.Registry, prefix string) ...`). REJECTED: the `HTTPFilterFactory` type is a single function-type alias used by the registry's `Register` API; widening the signature would require a v2 registry type and break the 07.1 ADR-0072 contract.

### Consequences

(a) The `FactoryCtx` framework extension is purely additive — partial supersession of the 07.1 ADR-0072 / ADR-0074 cluster: the trunk filter set extends from `{cors, envoygotest, router}` to `{cors, envoygotest, fault, router}`. Boot-registration order is alphabetical; `cmd/envoy-go/main.go` adds one line between `envoygotest` and `router`.

(b) The `FactoryCtx` superset contract: future stat-bearing filters consume `ctx.Stats` + `ctx.StatPrefix` directly without further framework changes. Future cross-filter-policy filters (e.g., a `compose` filter that wraps siblings) consume `ctx.Registry` directly. The 3-field `FactoryCtx` is the canonical framework-context shape for the §9 HTTP filters family.

(c) Cross-references:
   - ADR-0072 (HTTPRegistry threaded constructor) — extended; the registry's value-set grows from 3 to 4 entries.
   - ADR-0074 (boot-time three-filter set: `cors`, `envoygotest`, `router`) — extended; the boot-time set grows to 4. ADR-0074's "filters registered at boot" enumeration is now `{cors, envoygotest, fault, router}`. This ADR amends ADR-0074 in the additive-only sense (no removal of existing entries).
   - ADR-0085 (nil-tolerance for framework-injected pointers) — anchored at the new `Stats` field; fault's `registerFaultStats` exhibits the pattern.
   - ADR-0061 (pre-Freeze stat registration discipline) — anchored at fault's New-time stat registration; fault is the first HTTP filter to exercise the discipline (cors/envoygotest/router register zero stats; previously only HCM and listener-side code registered stats).

(d) The `FactoryCtx` extension is grep-verifiable in `internal/filter/http/types.go` (`Stats *stats.Registry` line); future readers tracing the Phase-09 first-use can locate the framework extension here.

(e) Boot-time fail-fast continues per ADR-0072: `fault.New` rejects `nil` typed_config + malformed Any + abort.http_status out of [200, 600) + delay.percentage > 0 without delay.fixed_delay > 0. The `cmd/envoy-go/main.go` boot path observes the error and exits non-zero before serving traffic.

---

## ADR-0101: `runtimeConfig` shape + 6-field-consumed / 11-field-silent-ignore decomposition + `abort.http_status` PGV [200, 600) validation + `delay.fixed_delay` > 0 validation + percentage-roll determinism

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.5 (record durable design rationale; the runtimeConfig shape is the load-bearing parser contract for fault's behavioral semantics) + D-3.3 (the silent-ignore decomposition is the empirically-pinned divergence from upstream Envoy and must be recorded for differential-fixture readers).
**Lands-in-task:** Task 3 (phase 09); commit e80aa10.

### Context

Per SPEC §6.2 the `runtimeConfig` is the projection of the upstream `*faultv3.HTTPFault` proto (16 fields per the v1.37.2 generated `.pb.go`) into the smaller per-instance shape that fault's DecodeHeaders consumes. The decomposition is:

**Six fields consumed at fault-eval time (8 if you count `matchHeaders` + `maxActiveFaults` separately):**
- `delayEnabled` — derived from `delay != nil && delay.fixed_delay > 0`
- `delayPercentage` — float64 in [0, 100]; from `delay.percentage` projected via FractionalPercent denominator
- `delayFixedDelay` — `time.Duration`; from `delay.fixed_delay.AsDuration()`
- `abortEnabled` — derived from `abort != nil && abort.http_status set`
- `abortPercentage` — float64 in [0, 100]; from `abort.percentage`
- `abortHTTPStatus` — int; PGV-validated [200, 600) at New time
- `matchHeaders` — `[]headerMatch{name, exactValue}`; only `string_match.exact` honored
- `maxActiveFaults` — int64; 0 = no cap

**Eleven fields silently ignored per ADR-0104 / SPEC §2 deferrals:**
- `delay.header_delay` (deferred-coupled with `abort.header_abort`)
- `abort.header_abort` (same)
- `abort.grpc_status`
- `upstream_cluster`
- `downstream_nodes`
- `disable_downstream_cluster_stats`
- `delay_percent_runtime` / `delay_duration_runtime` / `abort_percent_runtime` / `abort_http_status_runtime` / `max_active_faults_runtime` (5 runtime-key overrides; runtime layer not modeled in 09)
- `response_rate_limit` (response RL filter not in scope)
- `response_rate_limit_percent_runtime` / `abort_grpc_status_runtime` (additional runtime keys)
- `filter_metadata` (dynamic-metadata propagation deferred)

Per SPEC §11.1 PGV-empirically-pinned constraint: `abort.http_status` MUST be in [200, 600) — upstream Envoy's PGV constraint at the proto level; reference Envoy v1.37.2 rejects out-of-range values at `xds_listener` parse time. Fault's `New` mirrors this constraint at New time so the boot path fails fast (per ADR-0072 boot-time-fail-fast).

Per SPEC §11.1 secondary constraint: `delay.percentage > 0` requires `delay.fixed_delay > 0`. A delay block with non-zero rolling probability but zero duration is a configuration mistake — it would emit a 0-duration timer that fires synchronously, defeating the async-resume mechanic (ADR-0102) and producing observable behavior indistinguishable from "no delay". Fault rejects this at New time.

Per planner-time decision 12 (settled at PLAN.md): the percentage-roll RNG is a per-instance `*math/rand.Rand` seeded by `time.Now().UnixNano()` at filter-instance allocation time. 0% rolls short-circuit to false; 100% rolls short-circuit to true; intermediate values consult the per-instance RNG. This is non-deterministic across requests by design (each instance has its own seed) — the differential gate at fixture 0011 uses 0% / 100% scenarios exclusively to keep determinism; intermediate-percentage scenarios are out of differential scope per SPEC §7.4 fixture composition.

### Decision

`runtimeConfig` is an 8-scalar struct with one slice (`matchHeaders`). `parseRuntimeConfig(*faultv3.HTTPFault) (*runtimeConfig, error)` projects the proto and validates:

1. **`delay.percentage > 0` without `delay.fixed_delay > 0`** → `errors.New("fault: delay.fixed_delay required when delay.percentage > 0")`.
2. **`abort.http_status` ∉ [200, 600)** → `fmt.Errorf("fault: abort.http_status %d out of range [200, 600)", hs)`. The check fires only when `abort.error_type` is the `HttpStatus` oneof variant; `header_abort` / `grpc_status` variants are silent-ignored per ADR-0104.
3. **Header matchers** — only `HeaderMatcher_StringMatch` with non-empty `Exact` value is honored. All other variants (regex, prefix, suffix, contains, present-only, range-match) are silent-ignored at parse time. Header NAME is canonicalized via `http.CanonicalHeaderKey` so the runtime gate match is RFC-7230-correct (case-insensitive name match).

The percentage-roll discipline is per-instance:

- 0% → false (short-circuit; never consult RNG)
- 100% → true (short-circuit; never consult RNG)
- intermediate p → `f.rng.Float64() * 100 < p` (consult per-instance RNG seeded once at allocation)

This is settled in Task 3 by the `rng: rand.New(rand.NewSource(time.Now().UnixNano()))` line in the `New` factory closure; Task 4 lands the `rollPercent` helper that consumes it.

### Alternatives considered

(A) Inline parser in `New` (no extracted `parseRuntimeConfig`). REJECTED: per planner-time decision 2, the parser is shared between New (with full validation) and per-route resolution (Task 7's `parseRouteRuntimeConfig`). A separate function keeps both call sites symmetric. Per-route validation fires at HCM-build time (the RouteConfiguration parse path resolves typed_per_filter_config) so the same validation guards apply.

(B) Validate `abort.http_status` against the standard library's `http.StatusText` table (only "real" status codes pass). REJECTED: the PGV constraint is [200, 600) — non-stdlib codes like 418 / 419 / 421 are valid envoy-go inputs. Per SPEC §11.1 the constraint is a numeric range, not a set membership.

(C) Use a global `*math/rand.Rand` with `sync.Mutex` for the percentage rolls. REJECTED: lock contention on the rolling path is unnecessary; per-instance RNG is fully sufficient (no cross-request determinism requirement). The seed-per-instance design also means two simultaneous requests do not see correlated RNG sequences — per SPEC §6.4 the rolls are independent across requests.

(D) Use `crypto/rand` instead of `math/rand`. REJECTED: fault's percentage-roll is a behavioral decision, not a security decision. `math/rand` is faster and adequate for the purpose; reference Envoy uses a similar non-cryptographic PRNG.

(E) Validate the entire 16-field proto for "deprecated/unsupported" fields and emit warnings. REJECTED: silent-ignore is the SPEC-pinned discipline (per ADR-0041 + SPEC §2). Surface-noise warnings would require a logging contract that does not exist in 09; deferred to a future runtime/observability phase.

### Consequences

(a) The 6-vs-11 decomposition is grep-verifiable in `internal/filter/http/fault/fault.go` (the `runtimeConfig` struct + the `parseRuntimeConfig` body). Future readers tracing what fault honors vs. what it silent-ignores can locate the contract here.

(b) Cross-references:
   - ADR-0073 (typed_per_filter_config 3-tier merge model) — wholesale-override discipline empirically confirmed at SPEC §11.7 applies to fault's per-route resolution. A per-route HTTPFault that omits delay does NOT inherit listener-level delay; the per-route runtimeConfig is independently parsed via `parseRouteRuntimeConfig` (Task 7).
   - ADR-0104 (header-driven fault path DEFERRED) — anchored by the 11-field silent-ignore set. The `delay.header_delay` + `abort.header_abort` pair is the load-bearing element of that ADR's deferral set.

(c) The PGV [200, 600) range mirror is boot-time-fail-fast per ADR-0072: a misconfigured `abort.http_status` (e.g., 100 or 999) surfaces as a non-zero exit before the listener accepts traffic. Operators observe the failure at envoy-go startup, not at first faulted request.

(d) The percentage-roll RNG seeding is per-instance — across a long-running envoy-go process the per-request rolls are NOT cryptographically random but ARE statistically uncorrelated for the differential-fixture scope (0% / 100% scenarios short-circuit before consulting RNG; the only RNG-consulting fixture cells would be intermediate percentages, which are out of 09's differential scope).

(e) Future intermediate-percentage scenarios (e.g., a fault-statistics phase that exercises 25% / 50% / 75% rolls with a sampling window) will need a deterministic-seed override; that is deferred to the future phase. Task 3's decision is the per-instance seeded RNG; future phases supersede with an explicit seed knob if needed.

---

## ADR-0107: `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→22-name extension for FIVE `fault.*` stats + `response_rl_injected` permanently-zero counter discipline (route A)

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (the stat-name set is differentially observable and the empirical pin against reference Envoy v1.37.2 is the durable evidence) + D-3.5 (record durable design rationale).
**Lands-in-task:** Task 3 (phase 09); commit e80aa10. Stat registration code lands in Task 3; the BEHAVIOR_CONTRACT.md table extension lands in Task 15 alongside the rest of the §13 patches.

### Context

Per SPEC §11.6 + §13.2 the fault filter contributes FIVE stats to the BEHAVIOR_CONTRACT.md ## Stat-name mapping table. Pre-09 the table held 17 stat names (the trunk set: HCM-level + cluster-level + listener-level + admin-level counters/gauges). Phase 09 extends the table to 22 entries.

Empirically pinned against reference Envoy v1.37.2 with the SPEC §7.4 fixture composition:

**4 counters:**
- `http.<stat_prefix>.fault.aborts_injected` — incremented when an abort fault fires (DecodeHeaders → SendLocalReply path).
- `http.<stat_prefix>.fault.delays_injected` — incremented when a delay fault fires (DecodeHeaders → time.AfterFunc path).
- `http.<stat_prefix>.fault.faults_overflow` — incremented when a fault is SKIPPED because `max_active_faults` cap is reached.
- `http.<stat_prefix>.fault.response_rl_injected` — permanently zero in phase 09 (route A: emit for differential parity per SPEC §11.6 + §1.1 amendment).

**1 gauge:**
- `http.<stat_prefix>.fault.active_faults` — Inc/Dec around the active fault window (Inc when a delay or combined fault begins; Dec when the fault completes via timer-callback or OnDestroy).

The flattening discipline (SN1–SN8 per ADR-0061) applies unchanged: the `http.<stat_prefix>.fault.<metric>` Envoy-stat-name flattens to the Prometheus form `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<stat_prefix>"}` per SN2 (the HCM-namespace rule extracts `<stat_prefix>` as the `envoy_http_conn_manager_prefix` LABEL, NOT as part of the metric name) + the SN2 internal-dot transform (any `.` remaining in the rest segment is converted to `_` for Prometheus name-grammar compliance — phase 09 surfaces this transform for the first time because `fault.<metric>` is the first nested-rest HCM stat). Note: `<stat_prefix>` is a label, not part of the metric name (consistent with ADR-0061 SN2 / SPEC §11.6) — the metric name itself is `envoy_http_fault_<metric>`. The 22-name extension is purely additive — no existing stat-name semantics change.

`response_rl_injected` is a special case: upstream Envoy v1.37.2 emits this counter from a different filter (`envoy.filters.http.bandwidth_limit` or the downstream-rate-limit machinery inside fault's response-rate-limit code path). Phase 09 does NOT model `response_rate_limit` (it is in the 11-field silent-ignore set per ADR-0101); the counter would naturally be zero. The decision: emit the counter anyway with permanently-zero value (route A) rather than omit it (route B). Rationale:

1. **Differential parity.** Reference Envoy emits the counter (with zero value if `response_rate_limit` is unconfigured); envoy-go's `/stats/prometheus` output must match the line-set for the differential-fixture allow-list to pass. Route A keeps the line present; route B would force the differential to allow-list-skip the line, which is heavier configuration than emitting a zero-valued counter.

2. **Future-proofing.** When envoy-go gains response-rate-limit (a future small follow-up phase), the counter becomes hot without any framework change — only the increment site is added. Route A makes the counter's existence orthogonal to its semantics.

3. **Zero-cost discipline.** A registered-but-never-incremented counter has the same memory cost as any other counter (~16 bytes); the runtime cost is zero (no Inc calls). Route A is essentially free.

### Decision

The fault package registers FIVE stats at HCM-build time on `ctx.Stats` (per ADR-0100's `FactoryCtx` extension). The stat names are keyed by `"http." + ctx.StatPrefix + ".fault." + <metric>`:

```go
return &faultStats{
    abortsInjected:     reg.NewCounter(p + "aborts_injected"),
    delaysInjected:     reg.NewCounter(p + "delays_injected"),
    faultsOverflow:     reg.NewCounter(p + "faults_overflow"),
    activeFaults:       reg.NewGauge(p + "active_faults"),
    responseRLInjected: reg.NewCounter(p + "response_rl_injected"),
}
```

The `responseRLInjected` counter is allocated but never incremented in phase 09 (route A). The SN1–SN8 flattening per ADR-0061 yields five `envoy_http_fault_*{envoy_http_conn_manager_prefix="<stat_prefix>"}` Prometheus lines visible in `/stats/prometheus` (`<stat_prefix>` is a label, not part of the metric name — consistent with SN2 / SPEC §11.6).

The BEHAVIOR_CONTRACT.md ## Stat-name mapping table extension is a 5-row purely-additive amendment at Task 15:

Per SN2: `<sp>` (the HCM `stat_prefix`) is extracted as the `envoy_http_conn_manager_prefix` label and is NOT part of the Prometheus metric name. The Prometheus column below shows the metric name only; every line carries `{envoy_http_conn_manager_prefix="<sp>"}` as the label set.

| Envoy stat name | Prometheus name | Type | Notes |
|---|---|---|---|
| `http.<sp>.fault.aborts_injected` | `envoy_http_fault_aborts_injected` | counter | aborts fired by fault |
| `http.<sp>.fault.delays_injected` | `envoy_http_fault_delays_injected` | counter | delays fired by fault |
| `http.<sp>.fault.faults_overflow` | `envoy_http_fault_faults_overflow` | counter | max_active_faults cap hits |
| `http.<sp>.fault.active_faults` | `envoy_http_fault_active_faults` | gauge | active fault window |
| `http.<sp>.fault.response_rl_injected` | `envoy_http_fault_response_rl_injected` | counter | route A: permanently zero in phase 09 |

### Alternatives considered

(A) **Route B: omit `response_rl_injected` from the stat set** — REJECTED. Reference Envoy emits the line; the differential-fixture allow-list would need a per-line skip directive. The per-line skip would itself need an ADR documenting WHY only that one line is skipped — the documentation cost exceeds the registration cost. Route A consolidates the decision: emit the counter with zero value, no allow-list skip needed.

(B) **Register the counter conditionally** (only when `response_rate_limit` is configured). REJECTED: the runtimeConfig parser would need to peek at `c.GetResponseRateLimit() != nil` and gate the counter. The conditional logic adds a code path that future readers must trace; the unconditional registration is simpler.

(C) **Register a single `fault.faults_total` counter** that combines aborts + delays + overflow + response_rl. REJECTED: upstream Envoy emits the four separate counters; combining them would diverge from the differential-fixture line-set.

(D) **Use sub-registries** (`stats.Registry.NewSubRegistry("http.<sp>.fault")`) to organize the five stats. REJECTED per planner-time decision 5: sub-registries are out of scope for 06.1's `*stats.Registry`; the flat-name discipline (SN1–SN8 per ADR-0061) handles the prefix structurally.

### Consequences

(a) Cross-references:
   - ADR-0061 (SN1–SN8 flattening rules unchanged) — the 22-name extension is purely additive; SN1 (dot → underscore) + SN2 (`envoy_` prefix) apply unchanged. ADR-0061's enumeration grows from 17 to 22 entries; no rule change.
   - ADR-0100 (FactoryCtx framework extension) — the `Stats *stats.Registry` + `StatPrefix string` fields are the load-bearing inputs to fault's stat registration. The first-use of those fields lands at this ADR's registration code.

(b) `response_rl_injected` is the load-bearing route-A counter; its permanently-zero status is grep-verifiable in `internal/filter/http/fault/fault.go` (the `responseRLInjected` field exists; no `Inc()` call references it). Future phases that wire response-rate-limit will add the Inc call without touching the registration site.

(c) The 22-name table is the authoritative differential-fixture line-set for phase 09. The 0011-http-fault driver's StatsAsserter (Task 14) walks `/stats/prometheus` and asserts presence of all 22 names with the expected post-roll values; intermediate-percentage rolls are out of scope (per SPEC §7.4 fixture composition).

(d) The BEHAVIOR_CONTRACT.md ## Stat-name mapping table is the canonical reference for operators reading envoy-go's metric output. Phase 15 (a hypothetical Observability-family phase) will reference this ADR when adding additional `fault.*` stats (e.g., `fault.delay_total_us` if response-rate-limit lands).

---

## ADR-0103: Abort terminal-replace mechanics + body byte-exact `"fault filter abort"` (18 bytes, no trailing newline) + 4-header set on the wire + `OrderedHeaders` carrier discipline + status-text allow-list for non-stdlib codes

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (the abort response shape is differentially observable on the wire and its empirical pin against reference Envoy v1.37.2 is the durable evidence) + D-3.5 (record durable design rationale; the 4-header set + 18-byte body + OrderedHeaders carrier shape are load-bearing wire-protocol invariants).
**Lands-in-task:** Task 4 (phase 09); commit afea8ec.

### Context

Per SPEC §5.3 + §6.4 the abort path is a TERMINAL-REPLACE: when the abort percentage rolls hit AND the headers field matches AND `max_active_faults` is not exceeded, the fault filter aborts the upstream-bound request and synthesizes a local reply via `cb.SendLocalReply(status, body, headers)`. The chain machinery's `SendLocalReply` enters the encode chain at `filter[len-1]` per ADR-0075 and emits the synthesized response on the wire. Phase 09 must pin the EXACT shape of that synthesized response against reference Envoy v1.37.2 — the differential-fixture (Task 14) walks the wire bytes and asserts byte-equality.

Three load-bearing pieces of the wire shape:

1. **Body byte-exactness.** Reference Envoy v1.37.2 emits `"fault filter abort"` (18 bytes) as the response body. NO trailing newline. SPEC §11.4 captures the empirical byte-dump. The differential-fixture's `Content-Length: 18` line and the body bytes are the differential pin; an extra newline would break content-length AND the body byte-equality assertion. The constant `faultAbortBody = "fault filter abort"` is grep-verifiable in `internal/filter/http/fault/fault.go`.

2. **4-header set on the wire.** Reference Envoy emits exactly four response headers for the abort path: `content-length: 18`, `content-type: text/plain` (NO `; charset=utf-8` modifier — distinct from the admin endpoints' 6-header-set per phase-05a Task pin), `date: <IMF-fixdate>`, `server: envoy`. The §11.3 / §11.4 empirical pin captures the exact header set. The fault filter contributes only the `Content-Type: text/plain` override via OrderedHeaders; the other three (content-length, date, server) are framework-injected by the chain's `beginLocalReply` reconcile step + the wire-write layer per phase-04 Task 18 review.

3. **OrderedHeaders carrier.** Per ADR-0075's `SendLocalReply` contract + Phase 04 Task 18 review: the `headers` parameter is `OrderedHeaders` (slice carrier, deterministic insertion order) — NOT `http.Header` (Go map, non-deterministic iteration; net/http's `Header.Write` emits alphabetically). Fault passes the single override entry as `OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}`; the chain reconciles this carrier-supplied entry with the framework-injected three (content-length, date, server) per the post-encode reconcile step in `chain.beginLocalReply`. The reconcile preserves the caller's `Content-Type` value (text/plain WITHOUT charset modifier) — distinguishing fault's abort from cors's preflight (which carries six caller-supplied headers in the §11.2 verbatim order) and from admin endpoints (which carry `text/plain; charset=utf-8`).

A fourth piece — status-text — depends on whether the configured `abort.http_status` is in Go's `net/http`-stdlib status-text table. For the canonical 503 (`Service Unavailable`), 404 (`Not Found`), 405 (`Method Not Allowed`), and 200 (`OK`) the stdlib's `http.StatusText` returns the exact text reference Envoy emits; the differential-fixture asserts byte-equality on those four. For non-stdlib codes (e.g., 418 `I'm a teapot`, 599 custom-app-codes) reference Envoy emits its own status-text table — sometimes verbatim-stdlib, sometimes divergent. Per planner-time decision 7 the differential-fixture allow-list is NARROWED: only 200 / 503 / 404 / 405 byte-equal on the status-line; 418 and similar codes compare on STATUS CODE (the integer) only. This narrowing keeps phase 09 simple without compromising the differential-pin discipline for the canonical codes.

### Decision

The fault filter's abort-only DecodeHeaders path emits exactly:

```go
f.recordFaultEvent(eventAbortsInjected)
f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{
    {Name: "Content-Type", Value: "text/plain"},
})
return envoyhttp.StopIteration
```

where:

- `cfg.abortHTTPStatus` is the PGV-validated `[200, 600)` integer per ADR-0101.
- `faultAbortBody` is the 18-byte constant `"fault filter abort"` with NO trailing newline.
- The `OrderedHeaders` carrier holds exactly one entry: `{Name: "Content-Type", Value: "text/plain"}`. The chain's `beginLocalReply` reconciles this entry with the framework-injected `Content-Length: 18`, `Date: <IMF-fixdate>`, `Server: envoy` to land the 4-header set on the wire per the §11.3 empirical pin.
- The `recordFaultEvent(eventAbortsInjected)` call site fires BEFORE `SendLocalReply` so the counter increments regardless of any encode-chain behavior. Per planner-time decision 3, all stat Inc/Dec calls in fault are routed through `recordFaultEvent` rather than direct `f.stats.X.Inc()` calls — the consolidated dispatch makes the test surface (counter equality at the `http.<sp>.fault.<metric>` name) the single observable point.
- The return value is `envoyhttp.StopIteration` so the chain parks at the filter's index. The `SendLocalReply` machinery's first-call-wins `sync.Once` per chain guards against double-emission (Task 5's combined delay+abort path will land a timer-callback that calls `SendLocalReply` from the timer goroutine — same `sync.Once` guard handles the cross-goroutine case).

The status-text differential-fixture allow-list narrows to four canonical codes for byte-equal status-line assertions:

| Status code | Stdlib `http.StatusText` | Differential pin |
|---|---|---|
| 200 | `OK` | byte-equal status-line |
| 404 | `Not Found` | byte-equal status-line |
| 405 | `Method Not Allowed` | byte-equal status-line |
| 503 | `Service Unavailable` | byte-equal status-line |
| 418 / others | (varies) | STATUS CODE only |

The 0011-http-fault fixture (Tasks 11–14) configures abort.http_status=503 — the canonical code per the differential-pin allow-list.

### Alternatives considered

(A) **Append a trailing newline to the body** (`"fault filter abort\n"` — 19 bytes). REJECTED: reference Envoy v1.37.2's empirical byte-dump (SPEC §11.4) is 18 bytes with NO newline. The extra byte would break the differential-fixture's content-length assertion AND the body byte-equality.

(B) **Emit `Content-Type: text/plain; charset=utf-8`** (matching the admin endpoints' 6-header set). REJECTED: reference Envoy emits `text/plain` WITHOUT the charset modifier on the fault path. The differential-fixture would diverge byte-for-byte. The admin-vs-fault distinction is a wire-protocol invariant, not a code-organization preference.

(C) **Pass the headers as `http.Header` (Go map)** instead of `OrderedHeaders`. REJECTED per Phase 04 Task 18 review: `http.Header` cannot preserve insertion order on the wire — Go map iteration is non-deterministic and stdlib's `Header.Write` emits keys alphabetically sorted. The current `SendLocalReply` signature on `DecoderFilterCallbacks` accepts `OrderedHeaders` per ADR-0075's amendment; using `http.Header` would require a second SendLocalReply variant or break the framework contract.

(D) **Inline the stat Inc/Dec at the call sites** (`f.stats.abortsInjected.Inc()` directly) without `recordFaultEvent`. REJECTED per planner-time decision 3: each Inc call would need a per-counter nil-guard (`if f.stats.abortsInjected != nil`) per ADR-0085's nil-tolerance pattern. The 5 stat-call-sites would each carry the boilerplate; the consolidated `recordFaultEvent` switch handles all 5 in one place. The dispatch is sub-microsecond and not on the hot path — the abort path fires once per request that hits the percentage roll.

(E) **Defer the status-text byte-equal assertion entirely** (compare on status-code only for ALL codes). REJECTED: the canonical 200/404/405/503 stdlib texts ARE differentially observable; narrowing to "code-only" would weaken the differential pin needlessly. The narrow allow-list (planner-time decision 7) keeps the byte-equal pin where it is reliable (stdlib texts) and falls back to code-only where it diverges (418 etc.). Phase 09's 0011-http-fault fixture only exercises 503, which is in the byte-equal allow-list.

(F) **Emit the abort response with `Content-Length: 0` + a separate body chunk** (HTTP/1.1 chunked-transfer with the 18-byte body in a chunk). REJECTED: reference Envoy emits a standard non-chunked response with `Content-Length: 18` for the abort path. Chunked-transfer would diverge from the empirical wire pin and complicate the wire-write layer.

### Consequences

(a) The 4-header set on the wire is grep-verifiable: `internal/filter/http/fault/fault.go` calls `SendLocalReply` with the single `Content-Type: text/plain` override; the other three headers (content-length, content-type, date, server) are reconciled by the chain's local-reply machinery. The differential-fixture (Task 14) walks the wire bytes and asserts the 4-header set verbatim against reference Envoy v1.37.2.

(b) Body byte-exactness is grep-verifiable: `faultAbortBody = "fault filter abort"` is a package-level const; no other call site mutates it. `len(faultAbortBody) == 18` is asserted in `TestDecodeHeaders_AbortOnly_100Percent`.

(c) The `OrderedHeaders` carrier discipline mirrors phase 07.1's cors precedent: cors's preflight emits 6 entries via `OrderedHeaders` per the §11.2 verbatim order; fault's abort emits 1 entry via `OrderedHeaders` per the §11.3 verbatim shape. Both filters consume the same `SendLocalReply` contract; the chain's reconcile step handles both — there is no per-filter special-case in the chain machinery.

(d) Cross-references:
   - ADR-0075 (`SendLocalReply` enters encode chain at `filter[len-1]`; OrderedHeaders amendment) — anchored. Fault's abort-only path is the second consumer (cors's preflight is the first).
   - ADR-0072 (factory validates typed_config at boot) — referenced. The `cfg.abortHTTPStatus` consumed in DecodeHeaders is the PGV-validated `[200, 600)` integer per ADR-0101's parser gate; no runtime re-validation needed.
   - ADR-0085 (nil-tolerance) — anchored. `recordFaultEvent` tolerates nil stats + per-counter nil-guards; the abort path fires correctly even when `f.stats == nil` (test code without a registry).
   - ADR-0102 (delay async-resume) — cross-referenced. Task 5's combined delay+abort path will reuse the same `OrderedHeaders{Content-Type: text/plain}` carrier from the timer-callback goroutine; the `sync.Once` first-call-wins guard inside the chain's `beginLocalReply` handles the cross-goroutine entry safely.
   - ADR-0107 (5-stat extension) — anchored at `recordFaultEvent(eventAbortsInjected)`. The aborts_injected counter Inc happens once per fired abort; the 0011-http-fault fixture's StatsAsserter (Task 14) walks `/stats/prometheus` for the post-roll value.
   - SPEC §5.3 (abort-only flow) + §6.4 (DecodeHeaders body) + §6.6 (SendLocalReply OrderedHeaders carrier) + §11.3 (4-header set + body byte-exact) + §11.4 (body byte-dump) + §11.8 (headers-field exact-match semantics) — anchored.

(e) The status-text allow-list (200 / 404 / 405 / 503 byte-equal; others code-only) is recorded in this ADR; the 0011-http-fault fixture's expectations.yaml (Task 13) and driver (Task 14) consume the allow-list. Future phases that add fault-bearing fixtures with non-canonical codes (e.g., 418) will reference this ADR to know they fall on the code-only side of the allow-list.

(f) Tasks 5/6/7 build on this task's foundation:
   - Task 5 reuses `recordFaultEvent` for `eventDelaysInjected` + `eventActiveFaultsInc/Dec`; the abort path's call-site is the precedent.
   - Task 6 inserts the `max_active_faults` cap-check between the percentage-roll-evaluation and the abort/delay dispatch; the placeholder comment in the Task-4 DecodeHeaders body marks the insertion point.
   - Task 7 replaces `cfg := f.cfg` with `cfg := f.routeConfigOrListener()` to land the per-route 3-tier merge; the rest of the abort-only path is unchanged.

---

## ADR-0102: `time.AfterFunc`-driven async-resume — combined delay+abort fires `SendLocalReply` + `ContinueDecoding` from timer goroutine; parkDecode wake-up via chain's `localReplyDone` gate; cancel-on-OnDestroy mechanics + ±10ms timing tolerance

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.3 (the delay path's wire-observable timing fingerprint — `time_total ≈ delay ± 10ms` — is differentially observable against reference Envoy v1.37.2 and the durable timing pin) + D-3.5 (record durable design rationale; the timer-callback ordering — delay-then-abort, NOT abort-then-delay — is a load-bearing invariant of the combined path that determines whether the upstream is dialed and what response shape the wire sees).
**Lands-in-task:** Task 5 (phase 09); commit 2ec1507. Task 6 (phase 09) extends with cancel-on-OnDestroy + markedActive Inc-side wiring.

### Context

Per SPEC §5.2 + §5.4 + §11.2 + §11.3 the delay-injection path lands a configurable fixed delay on the request path BEFORE the upstream is dialed. The delay does not block the dispatch goroutine — `DecodeHeaders` returns synchronously with `StopIteration` so the chain machinery parks at the filter's index per ADR-0071's chain discipline, and a deferred mechanism re-enters via `cb.ContinueDecoding()` (delay-only path) or `cb.SendLocalReply(...)` (combined path) after the configured delay elapses.

Three load-bearing pieces of the async-resume mechanics:

1. **Timer mechanism: `time.AfterFunc`.** Go's `time.AfterFunc(d, fn)` schedules `fn` to run on its own goroutine after duration `d`; it returns a `*time.Timer` that can be canceled via `Timer.Stop()`. The callback runs on a runtime-managed goroutine, NOT on the dispatch goroutine — this is precisely what async-resume needs: `DecodeHeaders` returns immediately, the dispatch goroutine is freed, and the callback re-enters via the supplied `cb` (which is goroutine-safe by the `DecoderFilterCallbacks` contract per ADR-0071's amendment for cross-goroutine entry — the chain's `SendLocalReply` and `ContinueDecoding` are dispatched through the chain's per-stream synchronization). Alternative: `time.NewTimer(d) + go-routine-with-select` (manual goroutine + drain loop) — REJECTED in (A) below.

2. **Combined delay+abort ordering: timer fires, callback calls `SendLocalReply` THEN `ContinueDecoding` to wake the parked dispatch goroutine; the chain's `localReplyDone` gate ensures the resumed iteration short-circuits without dialing the upstream.** When BOTH the delay percentage AND the abort percentage roll hit on the dispatch goroutine, `DecodeHeaders` returns `StopIteration` and the dispatch goroutine parks in `parkDecode` waiting on `decodeResumeCh` (per ADR-0071's chain discipline). The combined path schedules a single timer at `delay.fixed_delay`; the timer's callback (running on the runtime-managed timer goroutine) calls `cb.SendLocalReply(abortHTTPStatus, faultAbortBody, headers)` followed by `cb.ContinueDecoding()`. The two calls are load-bearing in different ways: `SendLocalReply` populates the synthesized abort response and sets `c.localReplyDone` (the chain's `*atomic.Bool` gate) — but it does NOT wake the parked dispatch goroutine on its own. Without `ContinueDecoding` after `SendLocalReply`, the parked goroutine stays blocked on `decodeResumeCh` indefinitely and the request hangs even though the response is fully synthesized. `ContinueDecoding` sends on `decodeResumeCh`, unparking the dispatch goroutine; on resume, `RunDecodeHeaders`'s loop top observes `c.localReplyDone == true` and returns `(terminated=false, nil)` — short-circuiting WITHOUT advancing past the parked filter (so the next filter, including the router, is never invoked and the upstream is NEVER dialed). This invariant is verified by `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply` in `internal/filter/http/chain_test.go:848-875` (precedent test for the same pattern in chain machinery) and at the chain's resume site `internal/filter/http/chain.go:135-167`. The empirical pin per §11.3 (5 samples at 100ms delay + 503 abort: total 101.1–102.1ms) confirms reference Envoy v1.37.2 emits the abort response delay+~1.5ms after the request — consistent with the combined path's "timer-callback emits abort, no upstream dial" wire shape.

3. **±10ms timing tolerance per §11.2 conclusion (c).** The differential-fixture (Task 14) asserts `time_total ∈ [delay - 10ms, delay + 10ms]` for delay scenarios. The 10ms tolerance accommodates: (i) Go runtime scheduler jitter on the timer goroutine (typically <1ms but bursty under load); (ii) syscall + connection-accept overhead between the curl probe and envoy-go's listener; (iii) reference Envoy's own +1.5ms post-delay overhead on the abort path. A tighter tolerance (±2ms) would cause spurious differential-fixture failures on busy CI hosts; a looser tolerance (±50ms) would mask real timing regressions. Empirical testing across the 50/100/200/500ms sweep validates ±10ms as the operating point.

A fourth piece — RNG goroutine-safety — falls out of (1) + the planner-time decision 12 short-circuits documented in `rollPercent`. The percentage rolls (`f.rng.Float64()`) consult the per-instance `*rand.Rand` ON THE DISPATCH GOROUTINE before the timer is scheduled; the timer callback never touches `f.rng`. This preserves the single-goroutine-RNG-access invariant without per-instance mutex overhead — `*rand.Rand` is NOT goroutine-safe, but it is touched by exactly one goroutine per filter instance.

### Decision

The fault filter's delay-only DecodeHeaders path emits exactly:

```go
f.recordFaultEvent(eventDelaysInjected)
f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
    f.dcb.ContinueDecoding()
    f.decrementActive() // Task 6 wires markedActive guard
})
return envoyhttp.StopIteration
```

The combined delay+abort path emits exactly:

```go
f.recordFaultEvent(eventDelaysInjected)
f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
    f.recordFaultEvent(eventAbortsInjected)
    f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    f.dcb.ContinueDecoding() // wake the parked dispatch goroutine; chain's localReplyDone gate short-circuits the resumed iteration
    f.decrementActive()      // Task 6 wires markedActive guard
})
return envoyhttp.StopIteration
```

where:

- `cfg.delayFixedDelay` is the parsed `time.Duration` from `delay.fixed_delay` per ADR-0101.
- `f.delayTimer` is the `*time.Timer` field on `*filter`; Task 6's OnDestroy will call `f.delayTimer.Stop()` to cancel a pending callback when the stream closes before the delay elapses (cancel-on-OnDestroy mechanics).
- `f.recordFaultEvent(eventDelaysInjected)` fires SYNCHRONOUSLY on the dispatch goroutine (before the timer is scheduled); `f.recordFaultEvent(eventAbortsInjected)` (combined path only) fires from the TIMER GOROUTINE inside the callback. Both Inc calls go through `recordFaultEvent` which is goroutine-safe by virtue of `*stats.Counter`'s atomic Inc (per ADR-0061's Counter contract); the per-counter nil-guards inherited from Task 4 cover the cross-goroutine entry without additional synchronization.
- The return value is `envoyhttp.StopIteration` for both paths; the chain parks at the filter's index per ADR-0071 + ADR-0075 until the callback re-enters.
- The `OrderedHeaders` carrier in the combined path is identical to the abort-only path's per ADR-0103; the chain's `SendLocalReply`-from-timer-goroutine entry is handled by `chain.beginLocalReply`'s `sync.Once` first-call-wins guard per ADR-0103 cross-reference.

The ±10ms timing tolerance is a property of the differential-fixture's assertion shape (Task 14's expectations.yaml `time_total: ∈ [delay - 10ms, delay + 10ms]`), not a property of the fault filter's source code — `time.AfterFunc(d, fn)` runs `fn` "after at least duration d" per Go's runtime scheduler; the upper bound is host-dependent. The fixture's 10ms tolerance is the operating point.

### Alternatives considered

(A) **`time.NewTimer(d) + select` loop on a manually-spawned goroutine** instead of `time.AfterFunc`. REJECTED: equivalent runtime behavior but verbose — `time.AfterFunc` is the idiomatic Go API for "run f after d elapses" and avoids the boilerplate `go func() { <-t.C; fn() }()` + extra `t.Stop()`-vs-`<-t.C`-drain dance. The `time.AfterFunc`-returned `*time.Timer` exposes `Stop()` directly with the standard "returns true if the call stops the timer; false if it has already fired or been stopped" semantics — Task 6's OnDestroy uses this directly.

(B) **Synchronously sleep on the dispatch goroutine** (`time.Sleep(cfg.delayFixedDelay)` then continue). REJECTED: blocks the dispatch goroutine for the full delay duration. Per ADR-0071's single-goroutine-per-stream invariant, the dispatch goroutine handles ALL stream activity for the request — blocking it would freeze the stream's I/O AND prevent the framework from servicing OnDestroy or any subsequent decode events on the same connection. The whole purpose of the StopIteration + async-resume pattern is to free the dispatch goroutine while the timer ticks.

(C) **Combined path: timer fires, callback calls `ContinueDecoding`, the router or a downstream filter then synthesizes the abort.** REJECTED: this would require either (i) a stateful "post-delay-abort-pending" flag in fault that an EncodeHeaders-side check would consult OR (ii) a side-channel signaling mechanism through stream context. Both add code complexity for no observable benefit; the timer-callback can call `SendLocalReply` directly with the same `OrderedHeaders` carrier as the abort-only path. The empirical §11.3 pin (delay-then-abort response shape, no upstream dial) is consistent with the simpler "timer-callback calls SendLocalReply" ordering.

(D) **Combined path: emit BOTH `SendLocalReply` AND `ContinueDecoding` from the timer callback** (in that order). ACCEPTED — this is what landed in the Task 14 follow-up correction. Initial Task 5 design called only `SendLocalReply` from the timer callback, on the (incorrect) intuition that `SendLocalReply` alone would terminate the request. Task 14's end-to-end fixture run exposed the bug: the combined-path scenario hung past the 8s curl timeout because the dispatch goroutine remained parked in `parkDecode` on `decodeResumeCh`. Diagnosis: `SendLocalReply` populates the synthesized response and sets `c.localReplyDone`, but it does NOT signal the resume channel — only `ContinueDecoding` does. The fix: `ContinueDecoding` after `SendLocalReply` wakes the parked dispatch goroutine; the chain's `localReplyDone` gate (verified at `internal/filter/http/chain.go:135-167`) makes the resumed `RunDecodeHeaders` loop short-circuit on the next iteration WITHOUT invoking the next filter, so the upstream is NOT dialed. Precedent: `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply` in `internal/filter/http/chain_test.go:848-875` exercises this exact "external goroutine calls SendLocalReply + ContinueDecoding while dispatch is parked" pattern in chain-level machinery; the fault filter's combined path is the first production consumer of the pattern. The chain's `SendLocalReply` machinery is `sync.Once`-guarded per ADR-0075's amendment, so the `localReplyDone` gate is the load-bearing mechanism that distinguishes this design from "two competing local replies": there is only one local reply (the abort), and `ContinueDecoding` is purely a parkDecode wake-up signal.

(E) **Tighter timing tolerance (±2ms or ±5ms)** for the differential-fixture. REJECTED: empirical sweep across 50/100/200/500ms delays showed CI hosts under load occasionally exceed +5ms drift; the ±10ms operating point reliably distinguishes "timer fired correctly" from "timer skipped or fired in the wrong order" without false positives. The tighter tolerances would inject flake without improving the differential-pin signal.

(F) **Looser timing tolerance (±50ms or ±100ms)** to accommodate slower CI. REJECTED: would mask real timing regressions — a 50ms delay configured but firing in 0ms (no-op) or 100ms+ (wrong delay value) would slip through. The ±10ms tolerance keeps the differential-pin meaningful.

(G) **Delay path: stash `delay` on a per-stream `context.Context` deadline + signal completion via `<-ctx.Done()`** instead of `time.AfterFunc`. REJECTED: the framework's per-stream context is owned by the chain machinery, not by individual filters; injecting a deadline into it would couple fault to chain internals AND require a second goroutine to react to the context. `time.AfterFunc` is goroutine-cheap (Go's runtime pools timer-callback goroutines) and self-contained — no chain-internals coupling needed.

### Consequences

(a) The `*time.Timer` carried on the filter struct (`f.delayTimer`) is allocated lazily — it is `nil` for streams that don't fire the delay path (no-fault requests, abort-only fires, percentage-rolled-out). Task 6's OnDestroy nil-checks before calling `Stop()`. The per-stream allocation cost is one timer struct (~96 bytes on amd64) when the delay path fires — negligible.

(b) RNG goroutine-safety is grep-verifiable: `f.rng.Float64()` appears exactly once in `internal/filter/http/fault/fault.go` inside `rollPercent`, which is called from `DecodeHeaders` BEFORE `time.AfterFunc` is invoked. The timer callback closures never reference `f.rng`. The `*rand.Rand` per-instance (allocated in `New`'s factory closure) is touched by exactly one goroutine — the dispatch goroutine for that stream — preserving the non-goroutine-safe `*rand.Rand` invariant without mutex overhead.

(c) The combined path's "timer fires, callback calls SendLocalReply THEN ContinueDecoding" ordering is grep-verifiable: the timer-callback closure for the combined path calls `f.dcb.SendLocalReply` followed by `f.dcb.ContinueDecoding` (in that order); `TestDecodeHeaders_Combined` asserts `dcb.continued.Load() == 1` after the timer fires (verified in `internal/filter/http/fault/fault_test.go:517`). `ContinueDecoding` is purely a parkDecode wake-up — the chain's `localReplyDone` gate (set by `SendLocalReply` via `c.localReplyDone.Store(true)`) makes the resumed `RunDecodeHeaders` loop iteration short-circuit on its next loop-top check, returning `(terminated=false, nil)` without advancing past the parked filter. The upstream is NOT dialed. The reverse ordering ("ContinueDecoding then SendLocalReply") OR omission of `ContinueDecoding` entirely would either dial the upstream (reverse) or hang the request indefinitely (omission) — both divergent from the §11.3 differential pin.

(d) The ±10ms timing tolerance is asserted in two places: (i) `TestDecodeHeaders_DelayOnly` checks `elapsed ∈ [40ms, 200ms]` for a 50ms-configured delay — the lower bound (40ms) accommodates `time.Sleep`-based test polling jitter; the upper bound (200ms) accommodates GC pauses + scheduler stalls on busy CI; the test's tolerance is wider than the differential-fixture's because the test runs under `go test -race -short` with race-instrumentation overhead. (ii) The differential-fixture's expectations.yaml (Task 14) asserts `time_total ∈ [delay - 10ms, delay + 10ms]` for the on-the-wire delay scenario; this is the operationally-meaningful tolerance per BEHAVIOR_CONTRACT.md `## HTTP filter chain ### Asserted equivalence`'s timing-fingerprint clause. The two tolerances serve different purposes (test correctness vs. wire-observable equivalence) and do not need to agree.

(e) Cancel-on-OnDestroy mechanics anchor at Task 6 — the OnDestroy stub at Task 3 / Task 4 is a no-op; Task 6 fills in `if f.delayTimer != nil { f.delayTimer.Stop() }` + the markedActive Dec via `decrementActive()`. The `f.decrementActive()` call inside the timer callback is a Task-6 forward reference; at Task 5's commit point `decrementActive()` is the no-op stub from Task 4 (the `f.markedActive` field is always false because Task 6 hasn't wired the Inc side yet). The Task-5 timer-callback compiles and runs against the stub; Task 6 lights up the Inc side without changing Task 5's call sites.

(f) Cross-references:
   - ADR-0071 (single-goroutine-per-stream invariant + chain discipline) — anchored. The dispatch goroutine returns immediately at `StopIteration`; the timer goroutine re-enters via the chain's `cb.ContinueDecoding` / `cb.SendLocalReply` paths — both of which dispatch through the chain's per-stream synchronization per ADR-0071's amendment for cross-goroutine callback entry.
   - ADR-0075 (`SendLocalReply` enters encode chain at `filter[len-1]`; `OrderedHeaders` carrier; `sync.Once` first-call-wins guard) — anchored. The combined path's `SendLocalReply`-from-timer-goroutine entry is handled by the same first-call-wins guard as the abort-only synchronous path — no per-filter special-case in the chain machinery.
   - ADR-0103 (abort terminal-replace mechanics + body byte-exact + 4-header set) — anchored. The combined path's timer callback reuses the same `OrderedHeaders{Content-Type: text/plain}` carrier shape as the synchronous abort path; the wire response is byte-equivalent to the abort-only path, just delayed by `delay.fixed_delay`.
   - ADR-0105 (max_active_faults atomic counter + markedActive idempotency) — Task-6 forward reference. The `decrementActive()` call inside both timer callbacks is the timer-callback Dec site for the markedActive lifecycle; Task 6 wires the Inc side at the cap-check + the markedActive bool gate.
   - ADR-0107 (5-stat extension) — anchored at `recordFaultEvent(eventDelaysInjected)` (synchronous; dispatch goroutine) + `recordFaultEvent(eventAbortsInjected)` (combined path; timer goroutine). Both Inc calls go through `recordFaultEvent` per planner-time decision 3.
   - SPEC §5.2 (delay-injection flow) + §5.4 (combined delay+abort ordering) + §6.4 (DecodeHeaders body) + §6.5 (timer + OnDestroy interplay; deferred to Task 6's anchor) + §11.2 (delay timing fingerprint + ±10ms tolerance conclusion (c)) + §11.3 (combined-path empirical pin: delay+~1.5ms total) + §14.1 (ROADMAP row 09 timing-tolerance commitment) — anchored.

(g) Tasks 6/7/14 build on this task's foundation:
   - Task 6 wires OnDestroy's `f.delayTimer.Stop()` + the `markedActive` Inc/Dec balance via the cap-check insertion point. The Task-5 timer-callback's `f.decrementActive()` call site lights up at Task 6.
   - Task 7's per-route 3-tier merge is orthogonal to the timer mechanics; the timer reads `cfg.delayFixedDelay` from the resolved-route config, not from `f.cfg` directly (Task 7 swaps `cfg := f.cfg` for `cfg := f.routeConfigOrListener()` upstream of the timer scheduling — the timer captures the local `cfg` by closure).
   - Task 14's differential-fixture `time_total ∈ [delay - 10ms, delay + 10ms]` assertion is the operationally-load-bearing pin; this ADR records the tolerance contract that the fixture consumes.

(h) **Task 14 follow-up correction (post-review).** The Task 5 commit (`2ec1507`) initially landed only `f.dcb.SendLocalReply(...)` in the combined-path timer callback, with no `ContinueDecoding`. Task 14's end-to-end fixture run (commit `1550c9c`) exposed that scenario 2 (combined delay+abort 100% 100ms+503) hung past the 8s curl timeout: the dispatch goroutine remained parked in `parkDecode` on `decodeResumeCh` because `SendLocalReply` does not wake the channel. The Task 14 fix added `f.dcb.ContinueDecoding()` after `SendLocalReply` in the combined branch (verified at `internal/filter/http/fault/fault.go:298-326`); this wakes parkDecode, and the chain's `localReplyDone` gate at `internal/filter/http/chain.go:135-167` makes the resumed iteration short-circuit without dialing the upstream. Precedent test for the chain-level "external goroutine sends SendLocalReply + ContinueDecoding while dispatch is parked" pattern: `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply` in `internal/filter/http/chain_test.go:848-875`. `TestDecodeHeaders_Combined` was updated to assert `dcb.continued.Load() == 1` (was incorrectly `== 0` at Task 5 commit point). This ADR's Context §2, Decision code block, Alternatives (D), and Consequences (c) were all amended in the post-review follow-up to reflect the corrected design — the original ADR text mistakenly asserted the callback "must NOT call ContinueDecoding because that would dial the upstream"; the chain's `localReplyDone` gate makes that assertion false.

## ADR-0105: `max_active_faults` concurrency cap + LBP-1 sixth application + closure-captured `*atomic.Int64` shared counter + `markedActive atomic.Bool` per-instance idempotency guard + OnDestroy timer-cancel discipline + `fault.faults_overflow` stat semantics

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.5 (record durable design rationale; the LBP-1 closure-captured counter is the sixth application of a stable engineering pattern + the `markedActive` atomic.Bool guard is a load-bearing concurrency invariant whose race-clean discipline must survive future maintenance) + D-3.7 (race-detector evidence is durable; the `markedActive` field upgrade from plain `bool` to `atomic.Bool` is empirically motivated by `go test -race -count=10` flagging the OnDestroy-races-timer-callback case).
**Lands-in-task:** Task 6 (phase 09); commit b2174fd.

### Context

Per SPEC §5.6 + §5.7 + §6.4 the fault filter implements a `max_active_faults` concurrency cap: when configured (`max_active_faults > 0`) and the in-flight fault count has hit the cap, the fault is SKIPPED — `fault.faults_overflow` increments and `DecodeHeaders` returns `Continue` without invoking `SendLocalReply` or scheduling the delay timer. When the fault IS injected, an in-flight counter increments at fault-fire time and decrements at fault-completion time (whichever fires first: the timer callback OR `OnDestroy` driven by chain teardown).

Three load-bearing pieces of the concurrency model:

1. **Closure-captured `*atomic.Int64` shared counter (LBP-1 sixth application).** The counter is allocated by `New` (the `HTTPFilterFactory` factory function) at HCM-build time and CLOSURE-CAPTURED by the per-instance allocator returned from `New`; every `*filter` produced by the same factory shares the same `*atomic.Int64`. This is LBP-1 (the "factory-allocated, instance-shared, lock-free" pattern) — the sixth application of the same pattern, after ADR-0059 (stats Registry counter pointers — first), ADR-0072 (HTTPRegistry typed-config map — second), ADR-0079 (ListenerFilterRegistry — third), ADR-0085 (admin three-thread bootstrap+cluster+listener — fourth), and ADR-0091 (drain Manager — fifth). The hot path is a single `f.active.Load()` compared against `cfg.maxActiveFaults` (relaxed memory order) — no mutex, no contention.

2. **Per-instance `markedActive atomic.Bool` idempotency guard.** Each `*filter` instance carries an `atomic.Bool` flag; `markActive()` runs on the dispatch goroutine immediately after the cap check passes, performing `f.active.Add(1) → f.markedActive.Store(true) → recordFaultEvent(eventActiveFaultsInc)`. The Dec side (`decrementActive()`) uses `f.markedActive.CompareAndSwap(true, false)` — exactly one CAS succeeds across the racing pair (timer callback Dec'ing on the timer goroutine; `OnDestroy` Dec'ing on the chain-teardown goroutine). The atomic-Bool form is REQUIRED, not optional — empirical race-detector evidence (`go test -race -count=10 ./internal/filter/http/fault/...`) flags a plain `bool` RMW under `TestFault_DelayTimerRace` because `time.AfterFunc(d, fn)` runs `fn` on a runtime goroutine that genuinely races `OnDestroy` when `delayTimer.Stop()` returns false (the timer has already fired or is firing). The atomic.Bool form is race-clean by construction.

3. **OnDestroy timer-cancel discipline.** `OnDestroy` calls `f.delayTimer.Stop()` (best-effort; nil-checked) then `f.decrementActive()`. The `Stop()` return value is intentionally ignored — `markedActive.CompareAndSwap` handles both success cases (Stop-succeeded → callback never fires → OnDestroy Dec wins the CAS) and failure cases (Stop-failed → callback already firing or fired → whichever Dec call lands first wins the CAS; the loser observes false-already and no-ops). This is the ADR-0102 "cancel-on-OnDestroy" mechanics promised at Task 5.

A fourth piece — `fault.faults_overflow` stat semantics — falls out of (1): the counter increments exactly once per request that rolls into the fault path (delayApplies OR abortApplies hit) AND finds `f.active.Load() >= cfg.maxActiveFaults` at the cap check. Requests that don't roll into the fault path (no fault configured, percentage rolls miss, headers-field gate fails) do NOT consult the cap and do NOT increment `faults_overflow` — the counter is a measure of "would-have-fired-but-cap-blocked" cases, not a generic "rejected" counter.

### Decision

The fault filter's `DecodeHeaders` cap-check + dispatch sequence emits exactly:

```go
delayApplies := cfg.delayEnabled && f.rollPercent(cfg.delayPercentage)
abortApplies := cfg.abortEnabled && f.rollPercent(cfg.abortPercentage)
if !delayApplies && !abortApplies {
    return envoyhttp.Continue
}
if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults {
    f.recordFaultEvent(eventFaultsOverflow)
    return envoyhttp.Continue
}
f.markActive()
// ... fault dispatch (combined / delay-only / abort-only) ...
```

The `markActive` helper:

```go
func (f *filter) markActive() {
    f.active.Add(1)
    f.markedActive.Store(true)
    f.recordFaultEvent(eventActiveFaultsInc)
}
```

The `decrementActive` helper:

```go
func (f *filter) decrementActive() {
    if f.markedActive.CompareAndSwap(true, false) {
        f.active.Add(-1)
        f.recordFaultEvent(eventActiveFaultsDec)
    }
}
```

The `OnDestroy` body:

```go
func (f *filter) OnDestroy() {
    if f.delayTimer != nil {
        _ = f.delayTimer.Stop() // best-effort; markedActive guard handles double-Dec.
    }
    f.decrementActive()
}
```

Field shape on `*filter`:

```go
type filter struct {
    // ... cfg, active *atomic.Int64, stats, rng, dcb, ecb ...
    delayTimer   *time.Timer  // ADR-0102 async-resume timer
    markedActive atomic.Bool  // ADR-0105 sync.Once-equivalent guard
}
```

The closure-captured counter allocation in `New`:

```go
activeFaults := new(atomic.Int64)
// ...
return func() envoyhttp.HTTPFilter {
    f := &filter{
        cfg:    rc,
        active: activeFaults, // shared across all instances from this factory
        // ...
    }
    // ...
}, nil
```

### Alternatives considered

(A) **Plain `bool` for `markedActive`** (the original PLAN form). REJECTED EMPIRICALLY: `go test -race -count=10 ./internal/filter/http/fault/...` flags the `TestFault_DelayTimerRace` cycle test. The data race is genuine — `time.AfterFunc(d, fn)` runs `fn` on a runtime-managed goroutine, and during chain teardown the OnDestroy goroutine and the timer goroutine concurrently execute the read-then-write sequence on `markedActive`. Even though both paths converge on the same final state (markedActive=false, active counter Dec'd exactly once via the bool guard), the race detector is unhappy because the RMW is unsynchronized. The PLAN's claim "race-clean by single-goroutine-per-stream invariant per ADR-0071" was inaccurate — ADR-0071 governs the dispatch goroutine, but `time.AfterFunc` callbacks and chain-teardown's OnDestroy run on DIFFERENT goroutines. The atomic.Bool form (this ADR) supersedes the PLAN's plain-bool form.

(B) **`sync.Mutex` guarding both `markedActive` AND `f.active.Add(...)`**. REJECTED: heavyweight relative to the atomic.Bool CAS. The atomic.Bool's CompareAndSwap is a single CPU instruction (CMPXCHG on amd64) — cheaper than mutex Lock+Unlock pair. The mutex would also need to live on `*filter`, adding 16 bytes per instance vs. 4 bytes for atomic.Bool.

(C) **`sync.Once` instead of `atomic.Bool`**. REJECTED: `sync.Once.Do(fn)` runs `fn` exactly once — but it runs the FIRST caller's fn, locking out subsequent callers. This matches the "exactly-one-Dec" requirement BUT `sync.Once` allocates a 12-byte struct per instance (Go 1.21+) vs. atomic.Bool's 4 bytes, and the closure boilerplate (`f.decOnce.Do(func() { f.active.Add(-1); ... })`) is verbose. The atomic.Bool CAS is the leaner form for this exact pattern.

(D) **`atomic.Int32`** treating "marked active" as 1 and "decremented" as 0 with `CompareAndSwap`. REJECTED: equivalent to atomic.Bool but with wider semantics (4-byte signed int) for no benefit. atomic.Bool is the type-correct form for a binary flag.

(E) **Skip the `markedActive` guard entirely; rely on `f.active.Add(-1)` happening exactly once via the chain's OnDestroy-or-callback-but-not-both invariant**. REJECTED: the chain machinery does NOT guarantee "exactly one of OnDestroy/callback". When `time.Timer.Stop()` returns false the callback IS firing concurrently with OnDestroy — both will reach `f.decrementActive`, and without the markedActive guard both would Dec the counter, leaving it negative. The differential-fixture's StatsAsserter (Task 14) would see a negative `active_faults` gauge — divergent from reference Envoy. The markedActive guard is REQUIRED, not optional.

(F) **Cap the counter via `CompareAndSwap` on `*atomic.Int64` directly** (`for { v := f.active.Load(); if v >= cap { break }; if f.active.CompareAndSwap(v, v+1) { ... } }`). REJECTED: the load-then-Add sequence in the current shape (`if f.active.Load() >= cap { skip }; ... f.active.Add(1)`) has a benign race window: two concurrent decoders from different streams can both observe `Load() < cap` and both `Add(1)`, briefly overshooting the cap by one. This is acceptable per SPEC §5.6 — `max_active_faults` is a soft cap, not a hard cap; reference Envoy v1.37.2 has the same race window. The CAS-loop form would close the window but at the cost of complexity (~5 LoC per cap check) and a potential live-lock under heavy contention. The simple Load-then-Add form is the operating point.

(G) **Per-counter sub-Registry for `fault.*` stats** (instead of the flat `http.<sp>.fault.<metric>` namespace). REJECTED per planner-time decision 5: the `internal/stats.Registry` from phase 06.1 is a flat counter map per ADR-0061's pre-Freeze discipline; sub-registries are out of scope for phase 09. Threading `*stats.Registry` through `FactoryCtx` (Task 2 / ADR-0100) is sufficient for the 5 fault stats — the namespace prefix `http.<sp>.fault.<metric>` is the discriminator.

### Consequences

(a) The `*atomic.Int64` allocation per factory is grep-verifiable: `internal/filter/http/fault/fault.go::New` calls `activeFaults := new(atomic.Int64)` exactly once, before the per-instance closure. The closure captures `activeFaults` by reference; every `*filter` instance from that factory shares the same counter. Multiple-listener / multiple-route-with-distinct-fault-config deployments allocate ONE counter per factory (one per New invocation) — the cap is per-fault-config, NOT global across the listener.

(b) The `markedActive` field upgrade from plain `bool` to `atomic.Bool` is recorded in this ADR; the PLAN.md Step 3 snippet's plain-bool form is superseded by the atomic.Bool form. The decision to upgrade was empirically motivated — the race-detector flagged the plain-bool form on the FIRST `-race -count=10` invocation, and the atomic.Bool form went clean across all 10 iterations on the same run. Future maintenance MUST preserve the atomic.Bool form; downgrading to plain bool will re-introduce the data race.

(c) The `fault.faults_overflow` counter semantics are grep-verifiable: `recordFaultEvent(eventFaultsOverflow)` appears exactly once in `internal/filter/http/fault/fault.go::DecodeHeaders` — at the cap-check skip path. No other call site Inc's the counter. `TestDecodeHeaders_MaxActiveFaultsCapOverflow` asserts `faults_overflow == 1` after a 2-instance overflow scenario (cap=1; first request fires; second request hits the cap).

(d) The OnDestroy timer-cancel mechanics complete the ADR-0102 cancel-on-OnDestroy promise: Task 5's timer-callback's `f.decrementActive()` call site (the Task-6 forward reference at ADR-0102 Consequences (e)) lights up here. The `f.delayTimer.Stop()` return value is intentionally ignored — `markedActive.CompareAndSwap` handles both branches (Stop-succeeded vs. Stop-failed-with-callback-racing).

(e) Cross-references:
   - ADR-0071 (single-goroutine-per-stream invariant) — REFINED. ADR-0071 governs the dispatch goroutine; `time.AfterFunc` callbacks + OnDestroy genuinely run on different goroutines during chain teardown. The single-goroutine invariant covers RNG access (`f.rng.Float64()` in `rollPercent` runs only on the dispatch goroutine per ADR-0102 Consequence (b)) and `markActive` (also dispatch-goroutine). The Dec side (`decrementActive`) genuinely straddles goroutines — atomic.Bool CAS handles that case.
   - ADR-0059 (stats Registry counter pointers) — anchored. LBP-1 first application; this ADR is the sixth.
   - ADR-0072 (HTTPRegistry closure-captured map) — anchored. LBP-1 second application.
   - ADR-0079 (ListenerFilterRegistry) — anchored. LBP-1 third application.
   - ADR-0085 (admin three-thread bootstrap+cluster+listener) — anchored. LBP-1 fourth application.
   - ADR-0091 (drain Manager) — anchored. LBP-1 fifth application.
   - ADR-0102 (delay async-resume) — anchored at the cancel-on-OnDestroy mechanics. The Task-5 timer-callback's `f.decrementActive()` is the timer-side Dec; this ADR's `OnDestroy` is the chain-teardown-side Dec. Both converge on the markedActive CAS.
   - ADR-0107 (5-stat extension) — anchored at `recordFaultEvent(eventFaultsOverflow)` + `recordFaultEvent(eventActiveFaultsInc)` + `recordFaultEvent(eventActiveFaultsDec)`. All three Inc/Dec sites consolidate through `recordFaultEvent` per planner-time decision 3.
   - SPEC §5.6 (max_active_faults overflow flow) + §5.7 (concurrency model + markedActive guard) + §6.4 (DecodeHeaders cap check + dispatch sequence) + §6.5 (OnDestroy timer-cancel + Dec) + §14.1 (unit-test list including TestFault_DelayTimerRace) — anchored.

(f) The `TestFault_DelayTimerRace` cycle test (planner-time decision 10) is the operationally-load-bearing race-detector pin. It runs 100 iterations of `factory() → DecodeHeaders → sleep 0/1ms → OnDestroy` under `go test -race`; the 0/1ms sleep straddles the 1ms `delay.fixed_delay` so each iteration probabilistically hits Stop-succeeded, Stop-failed-callback-running, and callback-already-fired branches. `-count=10` repeats the 100-iteration loop 10 times, surfacing scheduler-dependent race-detector flakes. The test is `-short` skipped to keep the inner loop out of the default `go test -short` cycle.

(g) Tasks 7/14 build on this task's foundation:
   - Task 7's per-route 3-tier merge passes through the cap check unchanged — the timer captures the resolved-route `cfg.maxActiveFaults` by closure; per-route configs CAN override the listener-level cap (wholesale-replace per §11.7).
   - Task 14's differential-fixture exercises the cap by issuing concurrent requests against a 1-cap configuration; the StatsAsserter walks `/stats/prometheus` for `faults_overflow` and `active_faults` post-roll values. The `active_faults` gauge is asserted to return to 0 after all in-flight requests complete (Inc/Dec balance).

---

## ADR-0104: Header-driven fault path deferred — coupled to delay.header_delay / abort.header_abort proto sub-messages per phase 09 §11.5 empirical pin

**Status:** Deferred
**Date:** 2026-05-03
**Doctrine:** D-3.5 (record durable design rationale; explicit deferral with target follow-up so future readers can trace the scope choice). Per-ADR-0040 deferral-ADR format (mirrors ADR-0089's deferral-list precedent).
**Lands-in-task:** Task 15 (phase 09); commit 40db754.

### Context

Per SPEC §11.5 the empirical pin against reference Envoy v1.37.2 produced a major surprise that revised the BRAINSTORM-anticipated scope. The original BRAINSTORM §1.3 envelope listed the four documented `x-envoy-fault-{delay,abort}-request[-percentage]` request headers as a fifth fixture scenario ("header-driven abort"), with the implementation gated on the four headers landing as first-class request-side fault inputs alongside the proto-driven `percentage` rolls.

The §11.5 probe established that the header-driven path is NOT independent of the `delay.header_delay` / `abort.header_abort` proto sub-messages — reference Envoy ONLY honors the request headers when the corresponding proto sub-message is present in the HTTPFault config. With the proto sub-messages absent (the phase 09 MVP envelope per ADR-0101's 6-field-consumed / 11-field-silent-ignore decomposition), the request headers are silently ignored even though they are syntactically valid. Envoy's parser accepts both `delay.header_delay` and `abort.header_abort` as proto sub-message types, but consuming them requires runtime machinery (the per-request header parse + validation + percentage-roll + delay/status computation) that is ~150 LoC of additional implementation beyond the phase 09 §11-amended scope.

The two surfaces are therefore COUPLED: implementing the four request headers without the proto sub-messages would diverge from reference Envoy's behavior (envoy-go would honor headers that reference Envoy ignores); implementing the proto sub-messages without the request headers would leave the proto sub-messages parseable-but-dead. Both must land together for parity, OR both must defer together. Phase 09 chooses to defer both (this ADR).

### Decision

The header-driven fault path is DEFERRED from phase 09. Specifically:

(a) **Proto sub-messages `delay.header_delay` and `abort.header_abort`** are silently parsed (the runtimeConfig parser does not error on their presence in the HTTPFault Any) but NOT honored at fault-eval time. They are members of the 11-field silent-ignore set per ADR-0101.

(b) **The four documented request headers** (`x-envoy-fault-delay-request`, `x-envoy-fault-delay-request-percentage`, `x-envoy-fault-abort-request`, `x-envoy-fault-abort-request-percentage`) are silently ignored on the request path. envoy-go's fault filter never reads them, even when they are syntactically valid. Reference Envoy's parity (silent-ignore when the proto sub-message is absent) is preserved by virtue of phase 09 having NO `header_delay` / `header_abort` consumer.

(c) **Differential-fixture coverage** drops the BRAINSTORM-anticipated 5th scenario ("header-driven abort"). The 4 phase 09 fixture scenarios per SPEC §7.1 are: delay-only listener-inherited; combined delay+abort per-route; per-route wholesale-override; headers-field exact-match gate. No fixture probes the request headers in phase 09.

(d) **Future small follow-up phase** (~150 LoC) lands the coupled pair `delay.header_delay` + `abort.header_abort` proto sub-messages + the four request headers in one coherent slice. The follow-up phase is unscheduled as of phase 09 phase-done; it appends to the §9 HTTP filters family per ADR-0106's flat-row family-expansion shape (NOT a sub-phase of phase 09).

### Alternatives considered

(A) **Implement the four request headers without the proto sub-messages.** REJECTED: would diverge from reference Envoy's behavior — envoy-go would honor request headers that reference Envoy silently ignores when the proto sub-messages are absent, breaking the differential-equivalence claim from §13.1.

(B) **Implement the proto sub-messages without the request headers.** REJECTED: leaves the proto sub-messages parseable-but-dead — adds proto-parsing code with no observable effect. The follow-up phase landing the request headers would have to coordinate with this dead-but-parsed state, which is more complex than landing both at once.

(C) **Implement BOTH the proto sub-messages and the request headers in phase 09.** REJECTED: ~150 LoC of additional implementation + two new differential-fixture scenarios + at least one new ADR (header-parse mechanics) would push phase 09 past the ADR-0045 split-gate threshold. The phase-09 SPEC §1.1 amendment per §11.5 explicitly removes this from scope; the deferral preserves the phase 09 envelope.

(D) **Implement the request headers as a "permitted extension"** beyond reference Envoy parity. REJECTED: violates the differential-equivalence claim (envoy-go would emit fault behavior reference Envoy does not). The differential discipline is byte-equality on the wire response, not "envoy-go is a superset of Envoy's fault filter".

### Consequences

(a) The `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault ### Does not yet apply to` block (per §13.1) explicitly cites this ADR for the header-driven fault path bullet. Future readers asking "does envoy-go honor `x-envoy-fault-delay-request`?" grep ADR-0104 and find the deferral disposition with the coupled-pair rationale.

(b) The `runtimeConfig` parser per ADR-0101 silently accepts `header_delay` / `header_abort` proto sub-messages without erroring. The 11-field silent-ignore set per ADR-0101 explicitly lists both. No `runtimeConfig.headerDelay` or `runtimeConfig.headerAbort` field is allocated.

(c) The four `x-envoy-fault-*-request*` request headers are NOT in any allow-list, NOT consumed by any code path, NOT mentioned in any test. They flow through to the upstream as part of the request's normal header set (no header allow-list discipline applies — the fault filter does not strip them). Reference Envoy's behavior is identical when the proto sub-messages are absent.

(d) The future small follow-up phase that lands the coupled pair will:
   - Add `headerDelay` + `headerAbort` parsed fields to `runtimeConfig`.
   - Add per-request header parse + validation + percentage-roll logic to `DecodeHeaders`.
   - Add a 5th fixture scenario to `0011-http-fault` (header-driven abort) OR add a new fixture `0012-http-fault-header-driven`.
   - Add at least one ADR (header-parse mechanics + differential coverage).
   - Estimated scope: ~150 LoC + 1 new fuzzer + 1 new fixture scenario; well within the ADR-0045 split-gate threshold for a single follow-up phase.

(e) ADR-0104's status remains `Deferred` until the follow-up phase lands. At that point the ADR transitions to `Superseded by ADR-XXXX` per ADR-0001's status taxonomy; the supersession ADR records the actual implementation choices.

(f) Cross-references:
   - ADR-0040 (deferral-ADR format precedent) — anchored.
   - ADR-0089 (admin-endpoint deferral list per ADR-0040 format) — sibling deferral ADR; same format.
   - ADR-0101 (runtimeConfig 6-field-consumed / 11-field-silent-ignore) — `header_delay` + `header_abort` are members of the 11-field silent-ignore set.
   - ADR-0103 (abort terminal-replace mechanics) — the future follow-up's `header_abort` path reuses the same `OrderedHeaders` carrier + 4-header set.
   - ADR-0102 (delay async-resume) — the future follow-up's `header_delay` path reuses the same `time.AfterFunc` + parkDecode wake-up machinery.
   - ADR-0106 (§9 family-expansion shape) — the future follow-up phase lands as a flat top-level row in ROADMAP.md, NOT as a sub-phase of phase 09.
   - SPEC §11.5 (empirical pin: header-driven path requires proto sub-messages) — the load-bearing empirical evidence for the deferral.

---

## ADR-0106: §9 HTTP filters family expansion shape — flat top-level rows + no-sibling-stub discipline; the §9 heading at ROADMAP line 56 is an umbrella, not a row

**Status:** Accepted
**Date:** 2026-05-03
**Doctrine:** D-3.5 (record durable design rationale; the family-expansion shape is a load-bearing invariant of the §9 trunk that subsequent filter phases will inherit). Per-ADR-0001 template.
**Lands-in-task:** Task 15 (phase 09); commit 40db754.

### Context

Phase 09 (`envoy.filters.http.fault`) is the FIRST §9 HTTP filters family-row to land. The BOOTSTRAP_PROMPT.md §9 invariant 4 reading governs how the §9 trunk grows: each family-child filter is its own coherent phase with a flat top-level ROADMAP row (rows 09, 10, 11, ... — one per filter), NOT as a sub-phase of a parent §9 row.

The §9 family-children enumerated in ROADMAP.md line 58 are: header_mutation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. Each becomes one or more phases when it enters `in-progress`. The split-gate per ADR-0045 stays available if any filter's surface exceeds the ~1500 LoC / ~25 task threshold.

The BRAINSTORM Decisions 12 + 13 settle two related family-expansion questions:

- **Decision 12:** §9 family expansion is FLAT top-level rows. There is no parent "09 http-filters" row with sub-phases 09.1, 09.2, 09.3, etc. — each filter is its own row with its own number (09 = fault; 10 = the next filter to brainstorm; etc.). This contrasts with the trunk-phase split pattern (05 → 05.1 + 05.2; 06 → 06.1 + 06.2; 07 → 07.1 + 07.2; 08 → 08.1 + 08.2) which DID use parent-row + sub-phase. The §9 family is structurally different: family-children are independent (no shared deliverable they coordinate on), so a parent row would have no closure semantics.

- **Decision 13:** No-sibling-stub discipline. When phase 09 lands, the ROADMAP does NOT pre-populate stub rows for the other family-children (header_mutation, jwt_authn, etc.). The §9 heading at ROADMAP line 56 stays as a conceptual umbrella; rows are added at brainstorming time, NOT at phase 09 phase-done. Future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (whichever filters have already landed), rather than from a pre-populated stub.

The §9 heading at ROADMAP line 56 is therefore a CONCEPTUAL UMBRELLA, not a row. Its state field is unchanged across all family-row landings — the heading is structural Markdown, not a phase entry. Rows 09, 10, 11, ... ARE phase entries with state fields that flip planned → in-progress → done.

### Decision

The §9 HTTP filters family-expansion shape is fixed at this ADR:

(a) **Flat top-level rows.** Each §9 family-child filter is one or more flat top-level ROADMAP rows (numbered 09, 10, 11, ...). NO parent-row-with-sub-phases pattern is used for §9 family-children. The trunk-phase split pattern (05 → 05.1+05.2, etc.) does NOT apply.

(b) **No-sibling-stub discipline.** When a §9 family-row lands, the ROADMAP MUST NOT pre-populate stub rows for the other not-yet-brainstormed family-children. Stub rows are added at brainstorming time per BOOTSTRAP_PROMPT.md §6 brainstorm discipline. The §9 family-children list at ROADMAP line 58 enumerates the conceptual surface; the ROADMAP rows enumerate only the filters currently in-progress or done.

(c) **The §9 heading at ROADMAP line 56 (`### HTTP filters family`) is a conceptual umbrella, not a row.** The heading has no state field; its position in the ROADMAP is structural Markdown. Phase commits MUST NOT modify the heading's text or position; the heading stays unchanged across all family-row landings.

(d) **Family-children inherit ADR-0045's split-gate.** Any individual §9 family-row whose SPEC/PLAN exceeds the ~1500 LoC / ~25 tasks threshold splits per ADR-0045 (using the trunk-phase split pattern WITHIN that one filter — e.g., a hypothetical phase 12.1 + 12.2 if filter 12 needs splitting). The split is internal to that filter's row, NOT a fragmentation of the §9 trunk.

(e) **Brainstorm cold-start discipline.** Future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (whichever filters have already landed). The cold-start input is: BOOTSTRAP_PROMPT.md §9 invariant 4 + ROADMAP.md §9 heading + the most-recently-landed §9 family-row's PROGRESS.md + DECISIONS.md ADRs. No pre-populated stub row to reference.

### Alternatives considered

(A) **Single parent "09 http-filters" row with sub-phases for each family-child** (09.1 = fault, 09.2 = jwt_authn, etc.). REJECTED: the trunk-phase split pattern works because the parent row has a closure semantic (e.g., "minimum admin API"); a §9 parent row would have no closure semantic since the family is open-ended (new filters can always be added). The flat-row pattern is the natural shape.

(B) **Pre-populate stub rows for all 17 family-children at phase 09 phase-done.** REJECTED: each filter requires its own brainstorm before scope is clear; pre-populating stub rows creates a maintenance burden and pretends to know scope that hasn't been brainstormed. The no-sibling-stub discipline keeps the ROADMAP a record of actual scoped work, not aspirational placeholders.

(C) **Treat phase 09 as both a filter implementation AND a "family infrastructure" landing** (i.e., phase 09 contains generic filter framework code that subsequent filters reuse). REJECTED: the filter framework was already landed at phase 07.1; phase 09 is purely a filter-instance implementation. Future filters reuse the 07.1 framework, not phase-09 code.

(D) **Fold the §9 heading into the phase 09 row** (eliminate the heading; phase 09's row IS the §9 family entry). REJECTED: future family-children need a structural anchor in the ROADMAP; eliminating the heading would make their addition arbitrary. The umbrella heading is the structural anchor.

### Consequences

(a) Phase 09 is the FIRST §9 family-row to land. Subsequent filters (header_mutation, buffer, local_ratelimit, etc.) follow the same pattern — each its own coherent phase with its own row, ADR set, and PROGRESS.md.

(b) The phase 09 phase-done commit flips ROADMAP row 09 status `in-progress → done` and leaves the §9 heading at ROADMAP line 56 unchanged. The phase-done commit message body explicitly states (per the §15 acceptance checklist): (1) ROADMAP row 09 flips in-progress → done; (2) the §9 family heading at ROADMAP line 56 stays unchanged; (3) phase 09 is the FIRST §9 family-row to land.

(c) Future §9 family-row brainstorms cold-start from the §9 heading + the just-shipped artefacts. The brainstorm input does not include a pre-populated stub row; the brainstorm OUTPUT adds a new top-level row to ROADMAP.md (numbered sequentially after the previously-landed family-rows).

(d) The future small follow-up phase per ADR-0104 (header-driven fault path coupled pair) lands as a NEW top-level row (NOT as phase 09.1 or a sub-phase of phase 09). The follow-up phase reuses phase 09's `internal/filter/http/fault/` package shape but adds new files (~150 LoC) for the coupled pair. The row numbering depends on what other family-children land before the follow-up.

(e) ADR-0045's split-gate stays available WITHIN any §9 family-row. If, e.g., a hypothetical phase 12 (jwt_authn) exceeds the ~1500 LoC threshold, it splits into 12.1 + 12.2 internally — the split is per-filter, not per-§9-trunk.

(f) Cross-references:
   - ADR-0001 (template + status taxonomy) — anchored.
   - ADR-0045 (split-gate; stays available within any §9 family-row).
   - BOOTSTRAP_PROMPT.md §9 invariant 4 — the canonical reading of the §9 family-expansion discipline; this ADR records the load-bearing interpretation.
   - BRAINSTORM Decisions 12 (flat top-level rows) + 13 (no-sibling-stub discipline) — settled at brainstorm time; this ADR records the durable form.
   - ROADMAP.md line 56 (`### HTTP filters family`) — the umbrella heading; this ADR records its non-row status.
   - ROADMAP.md line 58 (the family-children enumeration) — the conceptual surface; this ADR records the no-pre-populated-stub discipline.
   - ADR-0104 (header-driven fault path deferred) — the future small follow-up phase lands as a new top-level row per this ADR's flat-row discipline.

(g) The §9 heading's state-field-unchanged invariant is grep-verifiable: future commits MUST NOT modify ROADMAP.md line 56's text or its position relative to the §9 family-children enumeration at line 58. A commit that modifies the heading is a violation of this ADR and should be reverted.

---

## ADR-0108: `header_mutation` package shape + boot registration — 4-file split mirroring `cors`/`fault`; `TypeURL` constant + `New` factory; zero-stats discipline

**Status:** Accepted
**Date:** 2026-05-04
**Doctrine:** D-3.5 (record durable design rationale) + D-3.3 (empirical pin against reference Envoy v1.37.2).
**Lands-in-task:** Task 5 (phase 10); commit in phase-10-http-filter-header-mutation-impl branch.

### Context

Phase 10 adds `envoy.filters.http.header_mutation` as the fifth HTTP filter in envoy-go. The first four are: router (phase 07), cors (phase 07.1), fault (phase 09), and the framework-internal router. Each real HTTP filter is a separate sub-package of `internal/filter/http/` with a 4-file split: `doc.go`, `<name>.go`, `<name>_test.go`, and (where present) a fuzzer. Phase 10 follows the same shape.

The boot-registration discipline (per ADR-0072) requires each filter factory to register a `TypeURL` constant + `New` factory in `cmd/envoy-go/main.go`. The cors and fault precedents each define `TypeURL` as a package-level constant and expose `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` as the single public factory.

The `FactoryCtx` shape (3-field: `Registry *HTTPRegistry`, `Stats *stats.Registry`, `StatPrefix string`) was established by ADR-0100. Phase 10 does NOT consume `ctx.Stats` or `ctx.StatPrefix` — header_mutation emits zero stats (per SPEC §11.3 confirmation + empirical Envoy v1.37.2 scrape). This is analogous to cors (ADR-0074: cors also emits zero stats).

### Decision

The `header_mutation` package is structured as:
- `internal/filter/http/header_mutation/doc.go` — package doc-comment with full algorithm description + ADR cross-references.
- `internal/filter/http/header_mutation/header_mutation.go` — `TypeURL`, `filterName`, `runtimeConfig`, `mutationOpKind`, `compiledMutationOp`, `New`, `buildRuntimeConfig`, `compileOps`, `isProtectedHeader`, `validatePerRouteHeaderMutation`, `filter`.
- `internal/filter/http/header_mutation/header_mutation_test.go` — New-time test suite (11 test functions).

`TypeURL` is the string `"type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"`. `New` is the HTTPFilterFactory. Both are public; everything else is unexported.

The `filter` struct implements both `StreamDecoderFilter` and `StreamEncoderFilter`. Both interfaces are statically asserted via blank-identifier compile-time checks (matching cors + fault precedents). `filter` holds `cfg *runtimeConfig` (read-only shared across requests), `dcb envoyhttp.DecoderFilterCallbacks`, and `ecb envoyhttp.EncoderFilterCallbacks`.

Zero stats: `ctx.Stats` and `ctx.StatPrefix` are not consumed. No `*faultStats`-equivalent struct exists. The 3-field `FactoryCtx` is passed through but only `ctx.Registry` is used (for per-route validator registration per ADR-0110).

### Alternatives considered

(A) **Merge with another filter package** — REJECTED. Each filter is independently deployable; the 4-file split keeps lint, test, and dependency graphs clean.

(B) **Consume `ctx.Stats` / `ctx.StatPrefix` for future stats** — REJECTED. Premature allocation; per ADR-0072 boot-fail-fast discipline, allocating stats that are never emitted is observable waste. If header_mutation gains stats in a future phase, that phase's Task 1 will add the allocation.

(C) **Use a shared `TypeURL` registry** in `internal/filter/http` — REJECTED per ADR-0072: each filter owns its `TypeURL` constant in its own package; the main.go registry is the single join point.

### Consequences

(a) The filter set grows from 4 to 5 real HTTP filters (router, cors, fault, router-internal, header_mutation). The `cmd/envoy-go/main.go` registry (Task 9) adds one entry.

(b) `ctx.Stats` and `ctx.StatPrefix` in `FactoryCtx` remain unconsumed by header_mutation (analogous to cors per ADR-0074). Grep-verifiable: no `ctx.Stats` reference in `header_mutation.go`.

(c) ADR-0109 defines the `runtimeConfig` + `compiledMutationOp` shapes. ADR-0111 defines the protected-header validation discipline. Both land in the same Task 5 commit as this ADR.

(d) Tasks 6/7/8 extend `header_mutation.go` with `applyOps`, `applyAppendAction`, and the full `DecodeHeaders`/`EncodeHeaders` bodies. Task 5 stubs those two methods to `return Continue`.

---

## ADR-0109: `runtimeConfig` 3-field shape + `compiledMutationOp` value-typed flat struct + AppendAction × 4 mapping + `keep_empty_value` semantics + multi-value per §11.4

**Status:** Accepted
**Date:** 2026-05-04
**Doctrine:** D-3.5 (record durable design rationale) + D-3.3 (empirical pin against §11.2–§11.4 SPEC).
**Lands-in-task:** Task 5 (phase 10); full `applyOps` + `applyAppendAction` body lands in Task 6.

### Context

The `header_mutation` filter applies a list of per-operation mutations (Append or Remove) to HTTP headers at request-eval time. The proto representation uses `HeaderMutation` (from `envoy/config/common/mutation_rules/v3`) which is a oneof over `Remove string` and `Append *HeaderValueOption`. `HeaderValueOption` carries `Header *HeaderValue` (key+value), `AppendAction HeaderValueOption_HeaderAppendAction`, and `KeepEmptyValue bool`.

The four `AppendAction` variants (per SPEC §6.6 + empirical Envoy v1.37.2 scrape):
1. `APPEND_IF_EXISTS_OR_ADD` (0, default) — append to existing value(s); add if absent.
2. `ADD_IF_ABSENT` (1) — add only if header is absent; no-op if present.
3. `OVERWRITE_IF_EXISTS_OR_ADD` (2) — overwrite if present; add if absent.
4. `OVERWRITE_IF_EXISTS` (3) — overwrite if present; no-op if absent.

`keep_empty_value` (per SPEC §11.2): if false (default), a mutation with empty value string AND `KeepEmptyValue=false` is silently dropped (the header is not added). If true, the empty value is materialized on the wire.

Multi-value semantics (per SPEC §11.4): `APPEND_IF_EXISTS_OR_ADD` preserves existing multi-value headers (net/http canonical map carries multiple values per key); the new value is appended to the slice. `OVERWRITE_IF_EXISTS_OR_ADD` replaces all existing values. `ADD_IF_ABSENT` no-ops if any value exists (even if multi-valued). `OVERWRITE_IF_EXISTS` replaces if any value exists.

The `mutations.query_parameter_mutations` field (`[]*corev3.KeyValueMutation`) is silently ignored per ADR-0112 deferral. The field is parsed by the proto library but `buildRuntimeConfig` does not project it into `runtimeConfig`.

Planner-time decision 4 chose value-typed `compiledMutationOp` (not pointer-typed) for cache locality during the apply-loop slice iteration. The struct is small (discriminator + two strings + enum + bool; ~56 bytes) and copied cheaply.

### Decision

**`runtimeConfig`** has exactly 3 fields:
```go
type runtimeConfig struct {
    requestOps                      []compiledMutationOp
    responseOps                     []compiledMutationOp
    mostSpecificHeaderMutationsWins bool
}
```
The fourth proto field (`mutations.query_parameter_mutations`) is silently ignored (ADR-0112).

**`compiledMutationOp`** is value-typed with 5 fields:
```go
type compiledMutationOp struct {
    kind           mutationOpKind
    headerName     string   // http.CanonicalHeaderKey applied at parse time
    headerValue    string   // kindAppend only; "" for kindRemove
    appendAction   corev3.HeaderValueOption_HeaderAppendAction
    keepEmptyValue bool     // kindAppend only
}
```
`kindRemove` and `kindAppend` are the two `mutationOpKind uint8` constants.

**`compileOps`** projects `[]*commonmutationrulesv3.HeaderMutation` → `[]compiledMutationOp`. Key choices:
- `http.CanonicalHeaderKey` is applied to `headerName` at parse time (once per boot, not per-request).
- Protected-header validation (ADR-0111) runs inside `compileOps` before the `compiledMutationOp` is appended.
- Unknown/nil actions are defensively skipped (no error).
- `compileOps` is called from both `buildRuntimeConfig` (listener-level) and `validatePerRouteHeaderMutation` (per-route validation).

**AppendAction × 4 mapping** (Task 6 `applyAppendAction` body, cross-referenced here):
| Proto constant | Behavior |
|---|---|
| `APPEND_IF_EXISTS_OR_ADD` | append to existing slice; add if absent |
| `ADD_IF_ABSENT` | no-op if header present; add if absent |
| `OVERWRITE_IF_EXISTS_OR_ADD` | replace all values; add if absent |
| `OVERWRITE_IF_EXISTS` | replace all values; no-op if absent |

**`keep_empty_value` semantics** (per §11.2): `applyAppendAction` (Task 6) gates on `op.keepEmptyValue || op.headerValue != ""` before mutating the header map.

### Alternatives considered

(A) **Pointer-typed `*compiledMutationOp`** — REJECTED per planner-time decision 4. Pointer typing would scatter the ops across the heap, degrading cache locality in the apply-loop. The struct is small enough that value-copy is cheap.

(B) **Keep the `AppendAction` as a proto enum in the runtime path** — ACCEPTED (the proto enum is already an `int32` alias; no translation table needed). The enum constants are used directly in `applyAppendAction`.

(C) **Expand `runtimeConfig` to include a `queryParameterOps` field** — REJECTED per ADR-0112. The `query_parameter_mutations` field is silently ignored; adding a field for it would confuse future readers about whether it has behavioral effect. The `buildRuntimeConfig` comment explicitly notes the silent-ignore.

(D) **Materialize header names in lowercase** instead of canonical form — REJECTED. The `net/http` canonical map uses `http.CanonicalHeaderKey` form; mismatched keys would silently miss mutations. Canonical form at parse time is the correct discipline.

### Consequences

(a) Cross-references ADR-0101 (fault's `runtimeConfig` precedent — fault also has a small value-typed config with a silent-ignore field; the pattern is consistent).

(b) `compileOps` is the single mutation-compilation entry point used at both listener-level (boot) and per-route (HCM-build-time) validation. The deduplication is load-bearing: a bug fix in `compileOps` applies to both paths automatically.

(c) Task 6 (`applyOps` + `applyAppendAction` + full test suite for AppendAction × 4 + keep_empty_value + multi-value) extends `header_mutation.go` in-place. The `compiledMutationOp` struct shape is fixed by this ADR; Task 6 does not change it.

(d) `mutations.query_parameter_mutations` is silently parsed by the proto library but not projected into `runtimeConfig`. Grep-verifiable: `buildRuntimeConfig` contains a comment `// mutations.query_parameter_mutations silently ignored per ADR-0112.` and no `GetQueryParameterMutations()` call produces ops.

---

## ADR-0111: Protected-header set per §11.1 + CONFIG-LOAD-TIME rejection (MAJOR amendment to BRAINSTORM Decision 11) + verbatim error format + EAGER per-route validation via `RegisterPerRouteValidator`

**Status:** Accepted
**Date:** 2026-05-04
**Doctrine:** D-3.3 (empirical pin against reference Envoy v1.37.2 §11.1 behavior) + D-3.5 (record durable design rationale).
**Lands-in-task:** Task 5 (phase 10).

### Context

SPEC §11.1 defines a 6-name protected-header set that header_mutation MUST NOT modify:
- `:method`, `:path`, `:authority`, `:scheme`, `:status` (the 5 HTTP/2 pseudo-headers).
- `host` (case-insensitive; Envoy v1.37.2 rejects `host`, `Host`, `HOST` symmetrically).

BRAINSTORM Decision 11 proposed runtime rejection (per-request, when the header is encountered). The PLAN's planner-time decisions revised this to CONFIG-LOAD-TIME rejection — a MAJOR amendment to Decision 11.

The amendment rationale:
1. **Boot-fail-fast discipline per ADR-0072.** Configuration errors that are detectable at boot time MUST be surfaced at boot time. A misconfigured protected-header mutation will fire on EVERY request if not caught at boot; catching it at boot causes exactly ONE error (the server fails to start) rather than one error per request.
2. **Symmetry with listener-level and per-route validation.** Both listener-level (`New`) and per-route (`validatePerRouteHeaderMutation` via `RegisterPerRouteValidator`) are evaluated at config-load time. The operator sees the error before any traffic is served.
3. **Simplicity in the request-eval hot path.** `applyOps` (Task 6) does NOT need to check `isProtectedHeader` at request time — the `compiledMutationOp` slice is already clean.

Planner-time decision 5 chose the predicate shape:
- `strings.HasPrefix(name, ":")` — catches all 5 pseudo-headers AND future ones (e.g., `:protocol`, `:upgrade`).
- `strings.EqualFold(name, "host")` — catches `host` / `Host` / `HOST` symmetrically.

The verbatim error format mirrors Envoy v1.37.2's `source/server/server.cc:453`:
```
"header_mutation: %q is :-prefixed or host; may not be modified"
```

The per-route validator is registered via `ctx.Registry.RegisterPerRouteValidator(filterName, validatePerRouteHeaderMutation)` inside `New`. At HCM-build time, `BuildPerRouteConfig` (ADR-0110) invokes this validator against each per-route `HeaderMutationPerRoute` proto at each tier (Route, VirtualHost, RouteConfiguration). This surfaces per-route protected-header violations as boot-time errors identical in effect to listener-level violations.

### Decision

**Protected-header predicate** (`isProtectedHeader`):
```go
func isProtectedHeader(name string) bool {
    if strings.HasPrefix(name, ":") {
        return true
    }
    return strings.EqualFold(name, "host")
}
```

**Rejection point:** `compileOps`, called from `buildRuntimeConfig` (listener-level, inside `New`) and from `validatePerRouteHeaderMutation` (per-route, inside `RegisterPerRouteValidator` callback). First violation returns:
```
fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", name)
```

**Rejection applies to both Remove and Append operations.** A mutation that removes `:path` or appends `host` is equally rejected.

**Per-route validator registration:** `New` calls `ctx.Registry.RegisterPerRouteValidator(filterName, validatePerRouteHeaderMutation)` before returning the factory. The registration is idempotent (overwriting with the same function is benign per ADR-0110's registry contract).

**`validatePerRouteHeaderMutation`** casts `proto.Message` to `*headermutationv3.HeaderMutationPerRoute`, extracts `GetMutations().GetRequestMutations()` and `GetResponseMutations()`, runs `compileOps` on each, and returns the first error. A nil `Mutations` field is a no-op (valid).

**BRAINSTORM Decision 11 amendment:** The original Decision 11 ("runtime rejection — detect at request time") is superseded by this ADR's config-load-time rejection. The change is a pure shift-left: the same set of headers is rejected, but at boot time rather than request time. No behavioral difference is observable for correctly-configured operators; incorrectly-configured operators see the error sooner and unambiguously.

### Alternatives considered

(A) **Runtime rejection (original BRAINSTORM Decision 11)** — REJECTED. Surfaces errors per-request; violates ADR-0072 boot-fail-fast discipline. A misconfigured operator would need to inspect request-time logs to discover the error; boot-time rejection is unambiguous.

(B) **Exact 6-name set (`:method`, `:path`, `:authority`, `:scheme`, `:status`, `host`) without prefix generalization** — REJECTED in favor of the prefix-check on `:`. The prefix-check future-proofs against new pseudo-headers. The 5 proto-specified names are a subset of "all :-prefixed names"; the prefix-check is strictly more protective.

(C) **Case-sensitive `host` check (`name == "host"`)** — REJECTED. Envoy v1.37.2 rejects `Host` and `HOST` symmetrically (empirically pinned per §11.1 conclusion (b)). `strings.EqualFold` is the correct predicate.

(D) **Defer protected-header validation to the codec layer** (let the underlying HTTP/2 implementation reject pseudo-header modifications) — REJECTED. Codec-layer errors are opaque and non-configurable; config-load-time errors with the verbatim format give the operator an actionable message.

(E) **Reject only Append, allow Remove** — REJECTED. SPEC §11.1 applies to both operations; removing `:path` is as invalid as appending it.

### Consequences

(a) `isProtectedHeader` is the single predicate used by both listener-level and per-route validation. Grep-verifiable: only two call sites in `header_mutation.go` — one in the `Remove` case of `compileOps`, one in the `Append` case.

(b) The `applyOps` hot path (Task 6) does NOT call `isProtectedHeader`. The absence of the check is intentional and grep-verifiable: `applyOps` trusts that `compiledMutationOp` slices were already validated by `compileOps` at boot.

(c) BRAINSTORM Decision 11 is superseded. The BRAINSTORM.md document retains the original text as historical context; this ADR is the authoritative record of the shift-left amendment.

(d) The verbatim error format `"header_mutation: %q is :-prefixed or host; may not be modified"` is tested by `TestNew_ProtectedHeader` (table-driven, 10 cases) and `TestNew_ProtectedHeader_RemoveAlsoRejected`. Any change to the format string is a breaking change requiring an ADR amendment.

(e) Cross-references:
   - ADR-0072 (boot-fail-fast discipline) — the foundational principle for config-load-time rejection.
   - ADR-0108 (package shape) — the `New` factory is where listener-level validation occurs.
   - ADR-0109 (`compileOps`) — the function that contains `isProtectedHeader` call sites.
   - ADR-0110 (`RegisterPerRouteValidator`) — the framework mechanism for per-route config-load-time validation.

---

## ADR-0110: Multi-tier per-route evaluation: PerRouteConfig.ResolveAllTiers + DecoderFilterCallbacks.RequestRouteConfigsAllTiers + HTTPRegistry.RegisterPerRouteValidator + per-filter accessor-choice discipline + cross-tier algorithm; amends ADR-0073

**Status:** Accepted
**Date:** 2026-05-04
**Doctrine:** Phase 10's multi-tier per-route evaluation; sibling to most-specific override per ADR-0073.

### Lands-in-task

Phase 10 — Tasks 2/3/4 (framework piece commits) + Task 7 (first end-to-end use; this ADR text).

### Context

The `most_specific_header_mutations_wins` proto field on `HeaderMutation` requires that per-route configs at all three tiers (Route, VirtualHost, RouteConfiguration) be evaluated and applied — not just the most-specific one. The empirical confirmation at SPEC §11.5 matches the proto comment verbatim: with the flag false, all three tiers are applied in Route→VHost→RC order; with the flag true, all three are applied in RC→VHost→Route order. The existing `PerRouteConfig.Resolve` (per ADR-0073) returns only the most-specific non-nil tier and discards the others — it cannot satisfy the header_mutation semantics. ADR-0073 was the right design for cors and fault (most-specific override), but a new accessor shape is needed for multi-tier evaluation.

### Decision

Three framework additions land in Tasks 2/3/4, first consumed end-to-end in Task 7:

1. **`PerRouteConfig.ResolveAllTiers(filterName, routeIdx) (route, vhost, rc proto.Message)`** — sibling method to `Resolve`; returns all three tiers unmerged (each may be nil if the tier has no entry for the filter). Does not replace `Resolve`; cors and fault continue to use `Resolve` unchanged.

2. **`DecoderFilterCallbacks.RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)`** — DECODER-ONLY callback per planner-time decision 1. Filters that need it on the encode side use the `f.dcb` reference set via `SetDecoderCallbacks` (the framework wires both dcb and ecb on a both-sides filter); the cors precedent at `cors.go:163` (`f.dcb.RequestRouteConfig()` called from `EncodeHeaders`) applies identically here.

3. **`HTTPRegistry.RegisterPerRouteValidator(filterName, validator func(proto.Message) error)`** — hook consumed by `BuildPerRouteConfig` to surface per-route protected-header violations as boot-time errors (per ADR-0111), mirroring the listener-level discipline in `New`.

**Per-filter accessor-choice discipline:** cors and fault use `RequestRouteConfig()` (most-specific override per ADR-0073); `envoy.filters.http.header_mutation` uses `RequestRouteConfigsAllTiers()` (multi-tier per this ADR). Future filters choose the accessor whose semantics match their proto contract; the choice is documented in the filter's ADR.

**Cross-tier ordering algorithm:** Listener-level mutations are applied FIRST always (per the proto comment at `header_mutation.pb.go:141–142`). Then per-route tiers in flag-controlled order: flag=false (default) → Route→VHost→RC (least-specific applied last, wins on overlap); flag=true → RC→VHost→Route (most-specific applied last, wins on overlap). This is the algorithm confirmed empirically at SPEC §11.5.

### Alternatives considered

- **(A) Keep using `Resolve` and treat the flag as selecting between two single-tier behaviors** — REJECTED. Loses configuration fidelity; the proto semantics require that ALL present tiers contribute mutations, not that the flag selects which single tier to read.
- **(B) Generalize the framework merger (per-filter merger interface in `PerRouteConfig`)** — REJECTED. Would require a ~300 LoC framework refactor and force re-verification of cors + fault per-route tests. The multi-tier concern is local to header_mutation in phase 10 scope; a generic merger is YAGNI.
- **(C) Push multi-tier resolution into HCM-build time (pre-merge all three tiers at parse time)** — REJECTED. Per-route config is per-request (the active route index is known only at request-eval time); resolution must be deferred to request time.
- **(D) ADD `ResolveAllTiers` as a sibling method, leaving `Resolve` + cors/fault untouched** — ACCEPTED. Minimal footprint; no regressions on existing filters; semantics match the proto contract exactly.

### Consequences

- (a) ADR-0073 is amended (not superseded) — the most-specific-override discipline remains the DEFAULT model for filters that opt into the `RequestRouteConfig()` accessor (cors @ 07.1, fault @ 09). An in-place amendment paragraph is appended to ADR-0073 below.
- (b) The `RegisterPerRouteValidator` hook is reusable by future filters with similar boot-time validation invariants (e.g., any filter that validates per-route proto fields at build time rather than request time).
- (c) The 3-tuple cache shape `(route, vhost, rc)` is incompatible with the existing single-`proto.Message` cache in `PerRouteConfig.Resolve`; per-tuple caching for `ResolveAllTiers` is deferred per planner-time decision 2 (the cost of re-compiling <5 ops per tier per request is negligible).

