# Phase 00 — Bootstrap — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §1 (cold-start), §3 (doctrine), §5 (state machine), §7 (differential contract); `docs/envoy-go/MISSION.md`; `docs/envoy-go/DECISIONS.md` (ADR-0001…0008); `docs/envoy-go/phases/00-bootstrap/SPEC.md`.

**Goal:** Land the envoy-go scaffold — Go module, minimal TCP-pump binary, differential test harness against pinned upstream Envoy, one green echo fixture, and CI — such that `docs/envoy-go/phases/00-bootstrap/SPEC.md` §3 phase-done gates all pass.

**Architecture:** A Go module rooted at `github.com/esalaine/envoy-go` (ADR-0006) hosts a thin `cmd/envoy-go` binary that parses a minimal YAML config (one listener + one upstream — ADR-0007), binds a TCP listener, and bidirectionally `io.Copy`s bytes to the upstream. A `test/differential/` Go test harness uses `testcontainers-go` to start the pinned upstream Envoy image (ADR-0008, tag/SHA captured at execution time) as the reference, builds and runs `cmd/envoy-go` as a subprocess subject, drives both with identical TCP payloads via a per-fixture driver, and diffs the response byte streams per `expectations.yaml`. CI runs lint+vet+unit on every push, then the differential job runs the harness against the echo fixture.

**Tech Stack:**
- Go 1.23 (floor; SPEC §10 #3)
- `golangci-lint` v1.55+ with the lint set declared in SPEC §5.5
- `github.com/testcontainers/testcontainers-go` v0.27+ (D-3.2 permitted foundation)
- `gopkg.in/yaml.v3` for the minimal YAML parse (subject binary; no Envoy proto until phase 01)
- Docker (CI runners and developer workstations) for upstream Envoy image
- GitHub Actions, `ubuntu-latest` runner (SPEC §10 #5 default)

---

## File Structure

Net change: ~1195 LoC across these paths.

| Path | Created/Modified | Purpose |
|---|---|---|
| `go.mod` | Create | Module path + Go version pin |
| `go.sum` | Create | Dependency checksums (testcontainers, yaml.v3, transitive) |
| `.golangci.yml` | Create | Lint config (SPEC §5.5 baseline) |
| `.github/workflows/ci.yml` | Create | Two jobs: `lint-vet-test`, `differential` |
| `cmd/envoy-go/main.go` | Create | Flag parse, config load, listener bind, Accept loop, ready sentinel |
| `cmd/envoy-go/config.go` | Create | Minimal YAML schema + parser (ADR-0007) |
| `cmd/envoy-go/config_test.go` | Create | Unit tests for parser + validation |
| `cmd/envoy-go/main_test.go` | Create | End-to-end echo test against in-process backend |
| `internal/{bootstrap,listener,cluster,tcp,http,tls,filter,xds,admin,stats,accesslog,runtime}/doc.go` | Create | Twelve placeholder packages — name, future-phase note, no code |
| `test/helpers/tcp.go` | Create | TCP client helpers used by fixtures + harness self-tests |
| `test/helpers/tcp_test.go` | Create | Unit tests for TCP helpers |
| `test/helpers/doc.go` | Create | Package doc |
| `test/differential/diff.go` | Create | `Compare(ref, subj, expectations) Verdict` + hex-dump helper |
| `test/differential/diff_test.go` | Create | Table-driven tests for `Compare` |
| `test/differential/harness.go` | Create | `ReferenceProxy`, `SubjectProxy`, `Fixture`, pin loader |
| `test/differential/harness_test.go` | Create | Unit tests for pin loader, ready detection, diff invocation (SPEC §8) |
| `test/differential/runner_test.go` | Create | `TestDifferential` discovers fixtures, runs each as `t.Run` subtest |
| `test/differential/doc.go` | Create | Package doc |
| `test/conformance/doc.go` | Create | Empty package — phase 05+ adds drivers |
| `test/fixtures/0000-tcp-echo/README.md` | Create | Fixture purpose + invariants |
| `test/fixtures/0000-tcp-echo/envoy.yaml` | Create | Real Envoy bootstrap (one listener, one cluster, tcp_proxy) |
| `test/fixtures/0000-tcp-echo/envoy-go.yaml` | Create | Minimal-schema config matching the same backend |
| `test/fixtures/0000-tcp-echo/expectations.yaml` | Create | All §7.2 dimensions enumerated (one applicable, rest not-applicable+reason) |
| `test/fixtures/0000-tcp-echo/driver/driver.go` | Create | `Drive(ctx, refAddr, subjAddr) (refBytes, subjBytes, error)` |
| `test/fixtures/0000-tcp-echo/driver/doc.go` | Create | Package doc |
| `docs/envoy-go/ENVOY_TARGET.md` | Modify | Replace placeholder with real tag, SHA256, refresh procedure |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0005, 0006, 0007, 0008 (and 0009 only if execution-time pin requires non-default runner) |
| `docs/envoy-go/ROADMAP.md` | Modify | Status row 00 stays `in-progress` (advanced to `done` only at state-machine step 6, in a later session) |
| `docs/envoy-go/STATE.md` | Modify | Advanced at session exit per state-machine step it reaches |
| `docs/envoy-go/phases/00-bootstrap/PROGRESS.md` | Create (during execution) | Append-only running log per state-machine step 3 |
| `README.md` | Modify | Add a one-paragraph "how to resume a session" pointer at the top |

---

## ADRs introduced by this plan

These ADRs are written **at execution time** by the task that earns them. They are listed here so the planner-of-record (this PLAN) is honest about what gets settled.

- **ADR-0005 — autonomous-planning adaptation for envoy-go phases.** Parallel to ADR-0004 (which adapted brainstorming). The `superpowers:writing-plans` skill's "Execution Handoff" interactive question is N/A; per state machine §5.1 the next fresh session handles execution. The plan-document-reviewer subagent loop in writing-plans is retained verbatim; the plan-author–subagent-reviewer-vs-human gate works exactly as ADR-0004 already established for spec review.
- **ADR-0006 — module path `github.com/esalaine/envoy-go`.** SPEC §10 #1 proposed `github.com/envoyproxy/envoy-go`; the planner declines to squat on the upstream org and picks an owner-namespaced path tied to the project's git identity (`Esa Laine <pgdad1st@gmail.com>`). The path is a `go.mod` identifier only — it does not need to resolve as a Git URL during phase 00. Future supersession is cheap (one ADR + `go mod edit -module …` + import-path rewrites).
- **ADR-0007 — minimal YAML schema for `envoy-go.yaml` in phase 00.** Schema:
  ```yaml
  listener:
    address: 0.0.0.0
    port: 10000
  upstream:
    address: 127.0.0.1
    port: 19000
  ```
  No additional fields. No extensions. Explicitly superseded by phase 01's real Envoy bootstrap loader.
- **ADR-0008 — pinned upstream Envoy image.** Filled at execution time by Task 4: tag (planner picks current stable release per SPEC §5.6 selection criteria), SHA256 digest captured from `docker pull`, upstream release-notes URL, Envoy proto major version (expected v3), and the refresh procedure. The plan only pre-commits the *procedure* for choosing; the *choice* is recorded by Task 4's commit.
- **ADR-0009** — *write only if* Task 16's CI run shows the pinned Envoy image cannot run on `ubuntu-latest` and Task 16 must therefore switch to a different runner image (per SPEC §10 #5). *If `ubuntu-latest` works, do nothing — no ADR is required, no TODO is left behind.*

The lint set stays at the SPEC §5.5 baseline (no ADR per SPEC §10 #4). The Go version stays at the 1.23 floor (no ADR per SPEC §10 #3).

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a worktree on a phase-implementation branch (NOT `phase/00-bootstrap-plan`, which is the planning branch). Recommended: `phase/00-bootstrap-impl`. Created with `git worktree add .worktrees/phase-00-bootstrap-impl -b phase/00-bootstrap-impl master`. The plan landed on `master` first.
2. Have `docker` available (verify with `docker version`). Required for Tasks 4, 14, and the CI differential job.
3. Have Go 1.23+ installed (verify with `go version`).
4. Have `golangci-lint` installed (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2` if missing.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path.

---

## Task 1: Initialize Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Create `go.mod`**

```
module github.com/esalaine/envoy-go

go 1.23
```

- [ ] **Step 2: Verify it parses**

Run: `go mod tidy && go build ./...`
Expected: PASS (no source files yet, so this is a no-op build that confirms the module is well-formed).

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "phase 00: initialize Go module"
```

---

## Task 2: Append ADR-0005 (autonomous-planning) and ADR-0006 (module path)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append two ADRs)

- [ ] **Step 1: Append ADR-0005 to `docs/envoy-go/DECISIONS.md`**

ADR text (paste verbatim, append after ADR-0004):

```markdown
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
```

- [ ] **Step 2: Append ADR-0006 to `docs/envoy-go/DECISIONS.md`**

ADR text:

```markdown
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
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/DECISIONS.md
git commit -m "phase 00: ADR-0005 (autonomous-planning), ADR-0006 (module path) [ADR-0005, ADR-0006]"
```

---

## Task 3: Create internal/ package placeholders

**Files:**
- Create: `internal/{bootstrap,listener,cluster,tcp,http,tls,filter,xds,admin,stats,accesslog,runtime}/doc.go` (12 files)

- [ ] **Step 1: Create each `doc.go` with a package comment**

Template (substitute `<package>` and `<future-phase>`):

```go
// Package <package> is a phase-00 placeholder. The real implementation lands
// in phase <future-phase>. See docs/envoy-go/ROADMAP.md and
// docs/envoy-go/phases/<future-phase>-*/SPEC.md once that phase enters
// in-progress.
package <package>
```

Future-phase mapping (per SPEC §4):

| Package | Future phase |
|---|---|
| `bootstrap` | 01 |
| `listener` | 02 |
| `cluster` | 02 |
| `tcp` | 02 |
| `tls` | 03 |
| `http` | 04 |
| `accesslog` | 06 |
| `stats` | 06 |
| `filter` | 07 |
| `admin` | 08 |
| `xds` | xDS family (09+) |
| `runtime` | runtime family (09+) |

- [ ] **Step 2: Verify all packages compile**

Run: `go vet ./internal/...`
Expected: no output (clean exit code 0).

- [ ] **Step 3: Commit**

```bash
git add internal/
git commit -m "phase 00: internal/ package placeholders"
```

---

## Task 4: Pin upstream Envoy image and write ENVOY_TARGET.md + ADR-0008

**Files:**
- Modify: `docs/envoy-go/ENVOY_TARGET.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0008)

This task requires Docker.

- [ ] **Step 1: Pick a candidate tag**

Per SPEC §5.6 selection criteria: stable release tag on `envoyproxy/envoy` (not `-dev`), within the last 6 months as of 2026-04-21, exposing admin and `tcp_proxy` on documented names with no API transition in flight. As of 2026-04-21, candidate: `v1.34.0` (or the most recent `v1.3X.0` tag visible on hub.docker.com/r/envoyproxy/envoy/tags). The executor verifies currency and adjusts.

- [ ] **Step 2: Pull and capture digest**

```bash
docker pull envoyproxy/envoy:v1.34.0
docker inspect --format='{{index .RepoDigests 0}}' envoyproxy/envoy:v1.34.0
```

Expected output: `envoyproxy/envoy@sha256:<64-hex-chars>`. Record both the tag and the SHA256.

- [ ] **Step 3: Smoke-test admin and tcp_proxy on the pulled image**

```bash
docker run --rm -d --name envoy-smoke -p 9901:9901 envoyproxy/envoy:v1.34.0 \
  envoy --config-yaml '
admin: { address: { socket_address: { address: 0.0.0.0, port_value: 9901 } } }
'
sleep 3
curl -fsS http://127.0.0.1:9901/ready
docker stop envoy-smoke
```

Expected: `LIVE` (HTTP 200). If anything else: pick a different candidate tag and retry, or invoke `superpowers:systematic-debugging`.

- [ ] **Step 4: Update `docs/envoy-go/ENVOY_TARGET.md`**

Replace the placeholder with:

```markdown
# envoy-go Reference Envoy Pin

**Tag:** `envoyproxy/envoy:v1.34.0`
**SHA256:** `<digest captured in Step 2>`
**Upstream release notes:** https://www.envoyproxy.io/docs/envoy/v1.34.0/version_history/v1.34/v1.34.0
**Envoy proto major version:** `v3`
**Pinned in:** ADR-0008
**Last verified:** 2026-04-21

## Refresh procedure

Per doctrine D-3.7, the pin is changed only via a dedicated phase that re-baselines the differential suite. To execute that phase:

1. Pick a new candidate tag per SPEC §5.6 selection criteria (stable, current within 6 months, no API transition in flight).
2. `docker pull envoyproxy/envoy:<new-tag>`; capture the SHA256 with `docker inspect --format='{{index .RepoDigests 0}}'`.
3. Run all differential fixtures against the new image: `go test ./test/differential/...`. Investigate any divergence — fix envoy-go to match, or extend `BEHAVIOR_CONTRACT.md` (with an ADR), or revert.
4. Update this file with the new tag, SHA, release-notes URL, and `Last verified` date.
5. Append a new ADR superseding ADR-0008 (and any contract-extension ADRs from step 3).
6. Land as a single commit on the pin-refresh phase branch.

The pin is never changed ad-hoc — every change is a phase with a green differential surface.
```

- [ ] **Step 5: Append ADR-0008 to `docs/envoy-go/DECISIONS.md`**

ADR text (substitute the captured SHA256):

```markdown
## ADR-0008: pinned upstream Envoy image v1.34.0

**Status:** Accepted
**Date:** 2026-04-21
**Doctrine:** D-3.3, D-3.7
**Settles:** SPEC §10 #2 deferred decision

### Context

The differential test contract (BOOTSTRAP_PROMPT §7) requires every fixture to compare against a stable, byte-identifiable upstream Envoy image. Phase 00 is the first phase to need that pin.

### Decision

The upstream Envoy reference is pinned to `envoyproxy/envoy:v1.34.0` at SHA256 `<digest>`. Selection rationale per SPEC §5.6:

- Stable release tag (not `-dev`).
- Most recent stable as of 2026-04-21 within the 6-month window.
- Exposes admin and `tcp_proxy` on the documented names for v3 proto.
- Smoke-tested locally: admin `/ready` returns `LIVE` under a minimal bootstrap.

### Consequences

- All fixture configs (`envoy.yaml`) target this Envoy version's bootstrap and v3 protobuf.
- `docs/envoy-go/ENVOY_TARGET.md` documents the refresh procedure (re-pull, re-baseline differential, ADR).
- Pin changes happen only in a dedicated phase per D-3.7.
```

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/ENVOY_TARGET.md docs/envoy-go/DECISIONS.md
git commit -m "phase 00: pin upstream Envoy v1.34.0 [ADR-0008]"
```

---

## Task 5: Configure golangci-lint

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Create `.golangci.yml`**

```yaml
# Lint configuration for envoy-go.
# Baseline derived verbatim from docs/envoy-go/phases/00-bootstrap/SPEC.md §5.5.
# Removals from this baseline require an ADR; additions do not.

run:
  timeout: 5m
  tests: true

linters:
  disable-all: true
  enable:
    - govet
    - errcheck
    - staticcheck
    - unused
    - ineffassign
    - gofmt
    - goimports
    - misspell
    - revive

linters-settings:
  goimports:
    local-prefixes: github.com/esalaine/envoy-go
  misspell:
    locale: US
  revive:
    rules:
      - name: package-comments
      - name: exported

issues:
  exclude-use-default: false
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 2: Run lint to confirm clean baseline**

Run: `golangci-lint run ./...`
Expected: no issues (project is mostly empty placeholders at this point; if `revive` complains about a missing package comment, fix the offending `doc.go` from Task 3).

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "phase 00: golangci-lint config (SPEC §5.5 baseline)"
```

---

## Task 6: Define and test the minimal YAML config (`cmd/envoy-go/config.go`)

**Files:**
- Create: `cmd/envoy-go/config.go`
- Create: `cmd/envoy-go/config_test.go`

This task introduces the first tests in the project. Follow `superpowers:test-driven-development`: failing test first, then minimal implementation, then commit.

- [ ] **Step 1: Add `gopkg.in/yaml.v3` dependency**

Run: `go get gopkg.in/yaml.v3@v3.0.1`

- [ ] **Step 2: Write the failing test (`cmd/envoy-go/config_test.go`)**

```go
package main

import (
	"strings"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	src := `
listener:
  address: 0.0.0.0
  port: 10000
upstream:
  address: 127.0.0.1
  port: 19000
`
	cfg, err := loadConfig(strings.NewReader(src))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Listener.Address != "0.0.0.0" || cfg.Listener.Port != 10000 {
		t.Errorf("listener: got %+v", cfg.Listener)
	}
	if cfg.Upstream.Address != "127.0.0.1" || cfg.Upstream.Port != 19000 {
		t.Errorf("upstream: got %+v", cfg.Upstream)
	}
}

func TestLoadConfig_RejectsMissingFields(t *testing.T) {
	cases := map[string]string{
		"missing listener":         `upstream: { address: 127.0.0.1, port: 19000 }`,
		"missing upstream":         `listener: { address: 0.0.0.0, port: 10000 }`,
		"missing listener address": `listener: { port: 10000 }
upstream: { address: 127.0.0.1, port: 19000 }`,
		"port zero":                `listener: { address: 0.0.0.0, port: 0 }
upstream: { address: 127.0.0.1, port: 19000 }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadConfig(strings.NewReader(src)); err == nil {
				t.Fatalf("loadConfig succeeded; want error")
			}
		})
	}
}

func TestLoadConfig_RejectsUnknownFields(t *testing.T) {
	src := `
listener: { address: 0.0.0.0, port: 10000 }
upstream: { address: 127.0.0.1, port: 19000 }
extra: nope
`
	if _, err := loadConfig(strings.NewReader(src)); err == nil {
		t.Fatalf("loadConfig accepted unknown field; want error")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/envoy-go/ -run TestLoadConfig`
Expected: FAIL — `undefined: loadConfig`.

- [ ] **Step 4: Write the implementation (`cmd/envoy-go/config.go`)**

```go
package main

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Config is the phase-00 minimal subject-binary configuration. Per ADR-0007,
// this schema is replaced by phase 01's real Envoy bootstrap loader.
type Config struct {
	Listener Endpoint `yaml:"listener"`
	Upstream Endpoint `yaml:"upstream"`
}

// Endpoint is a network address + port pair.
type Endpoint struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

func loadConfig(r io.Reader) (*Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true) // reject unknown keys per phase-00 strictness
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Listener.Address == "" {
		return fmt.Errorf("listener.address is required")
	}
	if c.Listener.Port <= 0 || c.Listener.Port > 65535 {
		return fmt.Errorf("listener.port must be 1..65535")
	}
	if c.Upstream.Address == "" {
		return fmt.Errorf("upstream.address is required")
	}
	if c.Upstream.Port <= 0 || c.Upstream.Port > 65535 {
		return fmt.Errorf("upstream.port must be 1..65535")
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/envoy-go/ -run TestLoadConfig -v`
Expected: PASS for `TestLoadConfig_Valid`, `TestLoadConfig_RejectsMissingFields/*`, `TestLoadConfig_RejectsUnknownFields`.

- [ ] **Step 6: Append ADR-0007 to `docs/envoy-go/DECISIONS.md`**

ADR text:

```markdown
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
```

- [ ] **Step 7: Commit**

```bash
git add cmd/envoy-go/config.go cmd/envoy-go/config_test.go go.mod go.sum docs/envoy-go/DECISIONS.md
git commit -m "phase 00: minimal config schema + parser [ADR-0007]"
```

---

## Task 7: Implement and test the TCP-pump main (`cmd/envoy-go/main.go`)

**Files:**
- Create: `cmd/envoy-go/main.go`
- Create: `cmd/envoy-go/main_test.go`
- Create: `test/helpers/tcp.go`
- Create: `test/helpers/tcp_test.go`
- Create: `test/helpers/doc.go`

The main test exercises the full subject-binary path against an in-process echo backend, using a small TCP helper. The helper is created here because Task 7's test is the first consumer.

- [ ] **Step 1: Write `test/helpers/doc.go`**

```go
// Package helpers provides shared TCP and process utilities used by both the
// differential harness and individual fixture drivers. Phase-00-internal;
// API is not stable.
package helpers
```

- [ ] **Step 2: Write the failing test (`test/helpers/tcp_test.go`)**

```go
package helpers

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPRoundTrip_EchoBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 1024)
		n, _ := c.Read(buf)
		_, _ = c.Write(buf[:n])
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := TCPRoundTrip(ctx, ln.Addr().String(), []byte("hello"), 200*time.Millisecond)
	if err != nil {
		t.Fatalf("TCPRoundTrip: %v", err)
	}
	if string(resp) != "hello" {
		t.Errorf("got %q, want %q", resp, "hello")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./test/helpers/`
Expected: FAIL — `undefined: TCPRoundTrip`.

- [ ] **Step 4: Implement `test/helpers/tcp.go`**

```go
package helpers

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// TCPRoundTrip dials addr, sends payload, half-closes the write side, then
// reads the response until EOF or until idleTimeout elapses with no new bytes.
// The returned slice is the full response stream.
func TCPRoundTrip(ctx context.Context, addr string, payload []byte, idleTimeout time.Duration) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	tcp, ok := conn.(*net.TCPConn)
	if ok {
		_ = tcp.CloseWrite()
	}

	var resp []byte
	buf := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		n, err := conn.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
		}
		if err == io.EOF {
			return resp, nil
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return resp, nil
		}
		if err != nil {
			return resp, fmt.Errorf("read: %w", err)
		}
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
	}
}
```

- [ ] **Step 5: Run helper test to verify it passes**

Run: `go test ./test/helpers/ -v`
Expected: PASS.

- [ ] **Step 6: Write the failing main test (`cmd/envoy-go/main_test.go`)**

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/helpers"
)

// TestEnvoyGoBinary_EchoesThroughUpstream is a fast in-process integration
// test (no Docker, ~2s end-to-end) that runs under both `go test ./...` and
// `go test -short ./...` — i.e. on every CI run including the unit job. It is
// the only end-to-end exercise of the subject binary outside the differential
// suite; do not add a -short skip here.
func TestEnvoyGoBinary_EchoesThroughUpstream(t *testing.T) {
	// 1. Start an in-process echo backend on a random port.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer backend.Close()
	go acceptEcho(backend)

	backendAddr := backend.Addr().(*net.TCPAddr)

	// 2. Pick a free port for the subject's listener.
	listenerPort := freeTCPPort(t)

	// 3. Build the subject binary into a temp file.
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// 4. Write the subject config.
	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
listener:
  address: 127.0.0.1
  port: %d
upstream:
  address: 127.0.0.1
  port: %d
`, listenerPort, backendAddr.Port)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// 5. Start the subject and wait for the ready sentinel.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	waitForReady(t, stdout, fmt.Sprintf("127.0.0.1:%d", listenerPort), 5*time.Second)

	// 6. Drive a payload through the subject and verify echo.
	resp, err := helpers.TCPRoundTrip(ctx,
		fmt.Sprintf("127.0.0.1:%d", listenerPort),
		[]byte("ping-7-fixture\n"), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if string(resp) != "ping-7-fixture\n" {
		t.Errorf("got %q, want %q", resp, "ping-7-fixture\n")
	}
}

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForReady(t *testing.T, r io.Reader, expectAddr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	br := bufio.NewReader(r)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			t.Fatalf("ready: %v", err)
		}
		if strings.Contains(line, "envoy-go ready on "+expectAddr) {
			return
		}
	}
	t.Fatalf("ready sentinel not seen within %s", timeout)
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./cmd/envoy-go/ -run TestEnvoyGoBinary`
Expected: FAIL (build succeeds, but the binary either does nothing or panics — `main.go` does not exist yet).

- [ ] **Step 8: Implement `cmd/envoy-go/main.go`**

```go
// envoy-go is the phase-00 subject binary. It is intentionally minimal: parse a
// minimal YAML config (ADR-0007), bind a TCP listener, and bidirectionally
// io.Copy bytes between each accepted connection and a single fixed upstream.
// Phase 02 retires this binary and replaces it with a real listener manager +
// TCP proxy filter + cluster manager.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml>")
		os.Exit(2)
	}
	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	cfg, err := loadConfig(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	listenAddr := fmt.Sprintf("%s:%d", cfg.Listener.Address, cfg.Listener.Port)
	upstreamAddr := fmt.Sprintf("%s:%d", cfg.Upstream.Address, cfg.Upstream.Port)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}
	defer ln.Close()

	// Ready sentinel: harness consumes this line from stdout to know the
	// listener is bound. Format is part of the harness contract; do not
	// change without updating test/differential/harness.go.
	fmt.Fprintf(os.Stdout, "envoy-go ready on %s\n", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go pump(conn, upstreamAddr)
	}
}

func pump(client net.Conn, upstreamAddr string) {
	defer client.Close()
	upstream, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		log.Printf("dial upstream %s: %v", upstreamAddr, err)
		return
	}
	defer upstream.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(upstream, client); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(client, upstream); halfClose(client) }()
	wg.Wait()
}

func halfClose(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
```

- [ ] **Step 9: Run main test to verify it passes**

Run: `go test ./cmd/envoy-go/ -run TestEnvoyGoBinary -v`
Expected: PASS.

- [ ] **Step 10: Run full unit suite**

Run: `go test ./...`
Expected: PASS (all tests, no skips beyond `-short`-gated ones).

Run: `golangci-lint run ./...`
Expected: no issues.

- [ ] **Step 11: Commit**

```bash
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go test/helpers/
git commit -m "phase 00: subject TCP-pump binary + TCP test helpers"
```

---

## Task 8: Differential diff package (`test/differential/diff.go`)

**Files:**
- Create: `test/differential/diff.go`
- Create: `test/differential/diff_test.go`
- Create: `test/differential/doc.go`

- [ ] **Step 1: Write `test/differential/doc.go`**

```go
// Package differential is the envoy-go differential test harness. It starts
// the pinned upstream Envoy image (reference) and an envoy-go subprocess
// (subject), drives both with identical inputs from per-fixture drivers, and
// compares outputs per docs/envoy-go/BEHAVIOR_CONTRACT.md. See SPEC §5.1 in
// docs/envoy-go/phases/00-bootstrap/SPEC.md for the harness lifecycle.
package differential
```

- [ ] **Step 2: Write the failing test (`test/differential/diff_test.go`)**

```go
package differential

import "testing"

func TestCompareBytes_Equal(t *testing.T) {
	v, err := CompareBytes([]byte("hello"), []byte("hello"))
	if err != nil {
		t.Fatalf("CompareBytes: %v", err)
	}
	if !v.Equal {
		t.Errorf("verdict: %+v; want Equal=true", v)
	}
}

func TestCompareBytes_DivergesAtFirstByte(t *testing.T) {
	v, err := CompareBytes([]byte("hello"), []byte("Hello"))
	if err != nil {
		t.Fatalf("CompareBytes: %v", err)
	}
	if v.Equal {
		t.Errorf("verdict: %+v; want Equal=false", v)
	}
	if v.FirstDiffOffset != 0 {
		t.Errorf("FirstDiffOffset: got %d, want 0", v.FirstDiffOffset)
	}
	if v.HexDump == "" {
		t.Errorf("HexDump empty")
	}
}

func TestCompareBytes_DifferentLengths(t *testing.T) {
	v, _ := CompareBytes([]byte("hello"), []byte("hello!"))
	if v.Equal {
		t.Errorf("verdict: %+v; want Equal=false", v)
	}
	if v.FirstDiffOffset != 5 {
		t.Errorf("FirstDiffOffset: got %d, want 5", v.FirstDiffOffset)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./test/differential/ -run TestCompareBytes`
Expected: FAIL — undefined identifiers.

- [ ] **Step 4: Implement `test/differential/diff.go`**

```go
package differential

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Verdict is the result of comparing two byte streams.
type Verdict struct {
	Equal           bool
	FirstDiffOffset int
	HexDump         string // empty when Equal
}

// CompareBytes is the byte-exact comparator used by phase-00's echo fixture.
// More elaborate comparisons (semantic header diff, stat tree intersection)
// land in later phases as their dimensions are added to BEHAVIOR_CONTRACT.md.
func CompareBytes(ref, subj []byte) (Verdict, error) {
	if len(ref) == len(subj) {
		eq := true
		for i := range ref {
			if ref[i] != subj[i] {
				return Verdict{
					Equal:           false,
					FirstDiffOffset: i,
					HexDump:         hexWindow(ref, subj, i),
				}, nil
			}
		}
		if eq {
			return Verdict{Equal: true}, nil
		}
	}
	// Different lengths: first divergence is at min length.
	off := len(ref)
	if len(subj) < off {
		off = len(subj)
	}
	for i := 0; i < off; i++ {
		if ref[i] != subj[i] {
			return Verdict{Equal: false, FirstDiffOffset: i, HexDump: hexWindow(ref, subj, i)}, nil
		}
	}
	return Verdict{Equal: false, FirstDiffOffset: off, HexDump: hexWindow(ref, subj, off)}, nil
}

func hexWindow(ref, subj []byte, off int) string {
	const window = 32
	start := off - window/2
	if start < 0 {
		start = 0
	}
	end := func(b []byte) int {
		e := off + window/2
		if e > len(b) {
			e = len(b)
		}
		return e
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "first divergence at offset %d\n", off)
	fmt.Fprintf(&sb, "ref [%d..%d]:\n%s\n", start, end(ref), hex.Dump(ref[start:end(ref)]))
	fmt.Fprintf(&sb, "subj[%d..%d]:\n%s", start, end(subj), hex.Dump(subj[start:end(subj)]))
	return sb.String()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./test/differential/ -run TestCompareBytes -v`
Expected: PASS for all three cases.

- [ ] **Step 6: Commit**

```bash
git add test/differential/diff.go test/differential/diff_test.go test/differential/doc.go
git commit -m "phase 00: differential byte-compare + hex-dump helper"
```

---

## Task 9: Differential harness — pin loader and shared types

**Files:**
- Create: `test/differential/harness.go`
- Modify: `test/differential/harness_test.go` (created here, extended in Tasks 10 and 11)

- [ ] **Step 1: Write the failing test (`test/differential/harness_test.go`)**

```go
package differential

import (
	"strings"
	"testing"
)

func TestParseEnvoyTarget_PullsTagAndDigest(t *testing.T) {
	src := `# envoy-go Reference Envoy Pin

**Tag:** ` + "`envoyproxy/envoy:v1.34.0`" + `
**SHA256:** ` + "`sha256:abc123def456`" + `
`
	pin, err := parseEnvoyTarget(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseEnvoyTarget: %v", err)
	}
	if pin.Tag != "envoyproxy/envoy:v1.34.0" {
		t.Errorf("Tag: got %q", pin.Tag)
	}
	if pin.SHA256 != "sha256:abc123def456" {
		t.Errorf("SHA256: got %q", pin.SHA256)
	}
}

func TestParseEnvoyTarget_RejectsMissingTag(t *testing.T) {
	src := "no tag here\n**SHA256:** `sha256:abc`\n"
	if _, err := parseEnvoyTarget(strings.NewReader(src)); err == nil {
		t.Fatalf("parseEnvoyTarget accepted input without Tag")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/differential/ -run TestParseEnvoyTarget`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement the parser in `test/differential/harness.go`**

```go
package differential

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// EnvoyPin captures the upstream image identity from ENVOY_TARGET.md.
type EnvoyPin struct {
	Tag    string // e.g. envoyproxy/envoy:v1.34.0
	SHA256 string // e.g. sha256:<hex>
}

var (
	tagLineRE    = regexp.MustCompile(`(?m)^\*\*Tag:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
	sha256LineRE = regexp.MustCompile(`(?m)^\*\*SHA256:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
)

func parseEnvoyTarget(r io.Reader) (*EnvoyPin, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	tagM := tagLineRE.FindSubmatch(src)
	if tagM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **Tag:** line")
	}
	shaM := sha256LineRE.FindSubmatch(src)
	if shaM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **SHA256:** line")
	}
	return &EnvoyPin{Tag: string(tagM[1]), SHA256: string(shaM[1])}, nil
}

// (More to come in Tasks 10–11.)

// readyTimeout is the wall-clock budget the harness allows each proxy to
// declare itself ready (admin /ready 200 for the reference, ready sentinel on
// stdout for the subject). Generous on purpose; SPEC §11 mitigates flakiness
// by surfacing failures, not retrying.
const readyTimeout = 30 * time.Second

// scanForLine reads lines from r until one of `needle` substrings appears or
// ctx is done. Returns the matching full line.
func scanForLine(ctx context.Context, r io.Reader, needle string) (string, error) {
	br := bufio.NewReader(r)
	out := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if strings.Contains(line, needle) {
				out <- line
				return
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case line := <-out:
		return line, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/differential/ -run TestParseEnvoyTarget -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/differential/harness.go test/differential/harness_test.go
git commit -m "phase 00: differential pin loader + ready-line scanner"
```

---

## Task 10: Differential harness — reference proxy via testcontainers

**Files:**
- Modify: `test/differential/harness.go` (extend)
- Modify: `test/differential/harness_test.go` (extend)
- Modify: `go.mod`, `go.sum`

This task pulls the testcontainers dependency. Requires Docker.

- [ ] **Step 1: Add testcontainers**

Run: `go get github.com/testcontainers/testcontainers-go@v0.27.0`

(If a newer minor is current at execution time, prefer the newest 0.x; major version 1.x requires re-validation per SPEC §10 deferred decisions.)

- [ ] **Step 2: Write the failing test (`test/differential/harness_test.go` — append)**

```go
func TestReferenceProxy_Starts(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test; skipped under -short")
	}
	ensureDocker(t)

	pin := loadPinFromRepo(t)
	const cfg = `
admin: { address: { socket_address: { address: 0.0.0.0, port_value: 9901 } } }
`
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ref, err := StartReferenceProxy(ctx, pin, cfg)
	if err != nil {
		t.Fatalf("StartReferenceProxy: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	if ref.AdminAddr() == "" {
		t.Errorf("AdminAddr empty")
	}
}
```

After this task, the **complete** import block in `harness_test.go` is:

```go
import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)
```

(Task 9 introduced `strings` and `testing`; this task adds `context`, `net`, `os`, `path/filepath`, `runtime`. Verify with `goimports -l` if uncertain.)

Helpers used (added to the same `_test.go`):

```go

func ensureDocker(t *testing.T) {
	t.Helper()
	if _, err := net.Dial("unix", "/var/run/docker.sock"); err != nil {
		t.Fatalf("docker unavailable: %v", err)
	}
}

func loadPinFromRepo(t *testing.T) *EnvoyPin {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	f, err := os.Open(filepath.Join(repoRoot, "docs", "envoy-go", "ENVOY_TARGET.md"))
	if err != nil {
		t.Fatalf("open pin: %v", err)
	}
	defer f.Close()
	pin, err := parseEnvoyTarget(f)
	if err != nil {
		t.Fatalf("parse pin: %v", err)
	}
	return pin
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./test/differential/ -run TestReferenceProxy_Starts`
Expected: FAIL — `StartReferenceProxy` undefined.

- [ ] **Step 4: Implement `StartReferenceProxy` in `test/differential/harness.go` (append)**

Append to `harness.go` (the existing import block plus new types and functions). After this task the full import block in `harness.go` is:

```go
import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ReferenceProxy is the upstream Envoy container managed by the harness.
type ReferenceProxy struct {
	container testcontainers.Container
	adminAddr string
	tcpAddrs  map[int]string // listener port (in-container) → host:port
}

// StartReferenceProxy launches the pinned Envoy image with the supplied
// bootstrap YAML, waits for admin /ready to return 200, and returns a handle.
// listenerPorts are container-internal TCP ports that should be exposed and
// looked up by AdminAddr / ListenerAddr.
func StartReferenceProxy(ctx context.Context, pin *EnvoyPin, bootstrap string, listenerPorts ...int) (*ReferenceProxy, error) {
	exposed := []string{"9901/tcp"}
	for _, p := range listenerPorts {
		exposed = append(exposed, fmt.Sprintf("%d/tcp", p))
	}
	req := testcontainers.ContainerRequest{
		Image:        pin.Tag,
		ExposedPorts: exposed,
		Cmd:          []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn"},
		WaitingFor:   wait.ForHTTP("/ready").WithPort("9901/tcp").WithStartupTimeout(readyTimeout),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start reference: %w", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	adminMapped, err := c.MappedPort(ctx, "9901/tcp")
	if err != nil {
		return nil, err
	}
	tcp := map[int]string{}
	for _, p := range listenerPorts {
		mapped, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", p)))
		if err != nil {
			_ = c.Terminate(ctx)
			return nil, err
		}
		tcp[p] = fmt.Sprintf("%s:%s", host, mapped.Port())
	}
	return &ReferenceProxy{
		container: c,
		adminAddr: fmt.Sprintf("%s:%s", host, adminMapped.Port()),
		tcpAddrs:  tcp,
	}, nil
}

// AdminAddr returns the host:port for the container's admin listener (9901/tcp).
func (r *ReferenceProxy) AdminAddr() string { return r.adminAddr }

// ListenerAddr returns the host:port for an exposed in-container listener port.
func (r *ReferenceProxy) ListenerAddr(containerPort int) string { return r.tcpAddrs[containerPort] }

// Stop terminates the container.
func (r *ReferenceProxy) Stop(ctx context.Context) error {
	return r.container.Terminate(ctx)
}
```

After writing the code, run `go mod tidy` so `nat` (transitively provided by testcontainers) is recorded.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./test/differential/ -run TestReferenceProxy_Starts -v -timeout 60s`
Expected: PASS (image pulled if first run, container started, admin /ready 200).

- [ ] **Step 6: Commit**

```bash
git add test/differential/harness.go test/differential/harness_test.go go.mod go.sum
git commit -m "phase 00: differential reference proxy (upstream Envoy via testcontainers)"
```

---

## Task 11: Differential harness — subject proxy as subprocess

**Files:**
- Modify: `test/differential/harness.go` (extend)
- Modify: `test/differential/harness_test.go` (extend)

- [ ] **Step 1: Write the failing test (append to `test/differential/harness_test.go`)**

Add `"fmt"` to the import block of `harness_test.go` (Task 10's complete-import-block listing did not include it; this task introduces the first `fmt.Sprintf` call). After this task the import block is:

```go
import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)
```

Then append the body:

```go
func TestSubjectProxy_StartsAndReports(t *testing.T) {
	if testing.Short() {
		t.Skip("subject subprocess test; skipped under -short")
	}
	port := freeTCPPort(t) // helper from cmd/envoy-go/main_test.go-style
	cfg := fmt.Sprintf(`
listener: { address: 127.0.0.1, port: %d }
upstream: { address: 127.0.0.1, port: 65535 }
`, port)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	subj, err := StartSubjectProxy(ctx, repoRoot(t), cfg)
	if err != nil {
		t.Fatalf("StartSubjectProxy: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	if got, want := subj.ListenerAddr(), fmt.Sprintf("127.0.0.1:%d", port); got != want {
		t.Errorf("ListenerAddr: got %q, want %q", got, want)
	}
}

// repoRoot returns the absolute path to the repository root. Used by both the
// subject-proxy starter (build.Dir) and the runner (loadPinFromRepo path
// resolution).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	return abs
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/differential/ -run TestSubjectProxy_Starts`
Expected: FAIL — `StartSubjectProxy` undefined.

- [ ] **Step 3: Implement `StartSubjectProxy` (append to `test/differential/harness.go`)**

```go
import (
	"os"
	"os/exec"
	"path/filepath"
)

// SubjectProxy is the envoy-go subprocess managed by the harness.
type SubjectProxy struct {
	cmd          *exec.Cmd
	listenerAddr string
	tmpDir       string
}

// StartSubjectProxy builds cmd/envoy-go from repoRoot, writes cfg to a temp
// file, starts the subject as a subprocess, waits for the ready sentinel, and
// returns a handle. The harness owns the subprocess lifetime; callers must
// call Stop to release.
func StartSubjectProxy(ctx context.Context, repoRoot, cfg string) (*SubjectProxy, error) {
	tmp, err := os.MkdirTemp("", "envoy-go-subject-*")
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("build subject: %w\n%s", err, out)
	}
	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("start subject: %w", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	line, err := scanForLine(readyCtx, stdout, "envoy-go ready on ")
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("subject ready: %w", err)
	}
	addr := readyAddr(line)
	if addr == "" {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("subject ready line malformed: %q", line)
	}

	return &SubjectProxy{cmd: cmd, listenerAddr: addr, tmpDir: tmp}, nil
}

// ListenerAddr returns the host:port the subject is listening on (parsed from
// the ready sentinel).
func (s *SubjectProxy) ListenerAddr() string { return s.listenerAddr }

// Stop kills and reaps the subject and cleans up its temp directory.
func (s *SubjectProxy) Stop() error {
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	return os.RemoveAll(s.tmpDir)
}

func readyAddr(line string) string {
	const prefix = "envoy-go ready on "
	i := strings.Index(line, prefix)
	if i < 0 {
		return ""
	}
	return strings.TrimRight(line[i+len(prefix):], "\r\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/differential/ -run TestSubjectProxy_Starts -v -timeout 30s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/differential/harness.go test/differential/harness_test.go
git commit -m "phase 00: differential subject proxy (envoy-go subprocess)"
```

---

## Task 12: Differential runner — fixture discovery (`runner_test.go`)

**Files:**
- Create: `test/differential/runner_test.go`

The runner is itself a test entry point. It discovers `test/fixtures/NNNN-*/` directories and dispatches to per-fixture drivers via a small registry — drivers register themselves via `init()` so each fixture's Go code is self-contained.

- [ ] **Step 1: Define the registry contract (extend `test/differential/harness.go`)**

```go
// FixtureDriver is the contract a fixture under test/fixtures/NNNN-*/driver
// implements. Drivers register themselves in init(); the runner discovers
// them by name (which must match the fixture directory).
type FixtureDriver interface {
	// Drive sends fixture-specific traffic at refAddr and subjAddr (each is a
	// host:port for the listener under test in each proxy). Returns the
	// captured byte streams for diffing.
	Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)

	// ReferenceBootstrap returns the YAML to feed upstream Envoy.
	ReferenceBootstrap() string

	// SubjectConfig returns the YAML to feed envoy-go.
	SubjectConfig(refListenerPort, subjListenerPort, backendPort int) string

	// ReferenceListenerPort is the in-container TCP port the reference proxy
	// must expose (the listener the driver dials).
	ReferenceListenerPort() int
}

var driverRegistry = map[string]FixtureDriver{}

// RegisterFixture is called from a driver's init().
func RegisterFixture(name string, d FixtureDriver) {
	if _, dup := driverRegistry[name]; dup {
		panic(fmt.Sprintf("duplicate fixture driver registration: %s", name))
	}
	driverRegistry[name] = d
}
```

- [ ] **Step 2: Write the runner (`test/differential/runner_test.go`)**

```go
package differential

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// (Task 13 step 7 adds "fmt" and "strings" to this import block when it
// introduces the bootstrap-template substitution. Do not pre-import them
// here — `goimports` and `golangci-lint` reject unused imports.)

// TestDifferential is the differential suite entry point. It discovers
// fixture directories under test/fixtures/, runs each as a subtest, and fails
// the suite if any fixture's diff verdict is not Equal.
func TestDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("differential suite; skipped under -short")
	}
	if _, err := net.Dial("unix", "/var/run/docker.sock"); err != nil {
		t.Fatalf("docker unavailable: %v", err)
	}

	root := repoRoot(t)
	fixtures := discoverFixtures(t, filepath.Join(root, "test", "fixtures"))
	pin := loadPinFromRepo(t)

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx, func(t *testing.T) {
			driver, ok := driverRegistry[fx]
			if !ok {
				t.Fatalf("no driver registered for fixture %q (did its driver package get imported?)", fx)
			}
			runFixture(t, root, pin, fx, driver)
		})
	}
}

func runFixture(t *testing.T, root string, pin *EnvoyPin, _ string, d FixtureDriver) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. Backend echo on a random port.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer backend.Close()
	go acceptEcho(backend)
	backendPort := backend.Addr().(*net.TCPAddr).Port

	// 2. Reference proxy.
	ref, err := StartReferenceProxy(ctx, pin, d.ReferenceBootstrap(), d.ReferenceListenerPort())
	if err != nil {
		t.Fatalf("ref start: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	refAddr := ref.ListenerAddr(d.ReferenceListenerPort())

	// 3. Subject proxy.
	subjPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPort)
	subj, err := StartSubjectProxy(ctx, root, subjCfg)
	if err != nil {
		t.Fatalf("subj start: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	// 4. Drive both, diff, report.
	refBytes, subjBytes, err := d.Drive(ctx, refAddr, subj.ListenerAddr())
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	v, err := CompareBytes(refBytes, subjBytes)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !v.Equal {
		t.Errorf("differential mismatch:\n%s", v.HexDump)
	}
}

func discoverFixtures(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		// No fixtures yet is a valid intermediate state (e.g. between Task 12
		// landing the runner skeleton and Task 13 landing the first fixture).
		return nil
	}
	if err != nil {
		t.Fatalf("read fixtures dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Fixture names start with a 4-digit prefix (NNNN-name).
		if len(e.Name()) >= 5 && isNumeric(e.Name()[:4]) && e.Name()[4] == '-' {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					_, _ = c.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}(c)
	}
}

// repoRoot is defined in harness_test.go and shared across the package's
// _test.go files. acceptEcho is duplicated below — it is package-private to
// the differential package; the cmd/envoy-go/main_test.go copy is in package
// main and unrelated.
```

> Driver-registration blank imports are deferred to Task 13 (after the driver
> packages exist). Adding them here would break `go build ./...` because the
> referenced packages do not yet exist on disk. Task 13 step 6.5 wires them up.

- [ ] **Step 3: Run the runner test (no fixture drivers registered yet)**

Run: `go build ./test/differential/...`
Expected: PASS — the package compiles cleanly. The runner is wired but no fixture is registered, so:

Run: `go test ./test/differential/ -run TestDifferential -v`
Expected: PASS with zero subtests (the for-loop over `fixtures` finds no `NNNN-*` directories under `test/fixtures/`, so no `t.Run` is invoked). Discovery of the actual fixture and the registry hookup land in Task 13.

- [ ] **Step 4: Commit**

```bash
git add test/differential/harness.go test/differential/runner_test.go
git commit -m "phase 00: differential runner (fixture discovery + per-fixture orchestration)"
```

---

## Task 13: Echo fixture (`test/fixtures/0000-tcp-echo/`)

**Files:**
- Create: `test/fixtures/0000-tcp-echo/README.md`
- Create: `test/fixtures/0000-tcp-echo/envoy.yaml`
- Create: `test/fixtures/0000-tcp-echo/envoy-go.yaml` (sample only — actual config is templated by the driver)
- Create: `test/fixtures/0000-tcp-echo/expectations.yaml`
- Create: `test/fixtures/0000-tcp-echo/driver/driver.go`
- Create: `test/fixtures/0000-tcp-echo/driver/doc.go`

- [ ] **Step 1: Write `test/fixtures/0000-tcp-echo/README.md`**

```markdown
# 0000-tcp-echo

The trivial fixture that proves the differential harness works end-to-end.

## What it tests

- Both proxies (upstream Envoy reference, envoy-go subject) terminate a TCP
  connection and pump bytes bidirectionally to the same backend.
- For a deterministic echo backend, both proxies' response byte streams are
  byte-exact.

## Configs

- `envoy.yaml` — real Envoy bootstrap with one listener (port `15000` in-container),
  one cluster (the in-host echo backend, address resolved at runtime by the
  reference container's gateway), and a `tcp_proxy` network filter routing
  listener → cluster.
- `envoy-go.yaml` — sample envoy-go-minimal config; the runner generates the
  effective config at test time (host-port substitutions).

## Driver

`driver/driver.go` opens a TCP connection to each proxy's listener, sends
ten payloads `ping-N-<uuid>\n` for N ∈ {0..9}, half-closes write, reads to
EOF or 1s idle, returns the concatenated response stream.

## Expectations

`expectations.yaml` enumerates every §7.2 dimension. Only `response-body` is
applicable; the rest are not-applicable with one-line reasons (no HTTP, no
filter chain, no stats subsystem in the subject yet).
```

- [ ] **Step 2: Write `test/fixtures/0000-tcp-echo/envoy.yaml`**

```yaml
# Reference upstream Envoy bootstrap for fixture 0000-tcp-echo.
#
# IN-CONTAINER PORT MAP:
#   15000 — TCP listener exposed to the host (the differential runner dials it
#           at the host-mapped port returned by testcontainers).
#   9901  — admin (used by harness wait.ForHTTP("/ready")).
#
# The cluster's endpoint address is "host.docker.internal" so the in-container
# Envoy can reach the host-side echo backend started by the runner. The driver
# substitutes the backend port in via the runner's per-fixture template hook
# (see runFixture / SubjectConfig in test/differential).
#
# The reference Envoy image must have host-gateway aliasing (modern Docker
# Desktop and recent docker-ce on Linux both honor host.docker.internal via
# extra_hosts; the testcontainers wait.ForHTTP exercises this path).

admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }

static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: host.docker.internal
                      port_value: 0   # placeholder; runner regenerates with backend port via Cmd override
```

> Note: the runner's `ReferenceBootstrap()` returns this YAML with the `port_value: 0` line replaced by the actual backend port at test time. See `driver/driver.go` step 4 for the substitution logic.

- [ ] **Step 3: Write `test/fixtures/0000-tcp-echo/envoy-go.yaml`** (sample only; runner templates the real config)

```yaml
# Sample envoy-go config for fixture 0000-tcp-echo. The runner overrides
# listener.port and upstream.port at test time.
listener:
  address: 127.0.0.1
  port: 16000
upstream:
  address: 127.0.0.1
  port: 19000
```

- [ ] **Step 4: Write `test/fixtures/0000-tcp-echo/expectations.yaml`**

```yaml
# Equivalence dimensions for fixture 0000-tcp-echo.
# Every dimension from BOOTSTRAP_PROMPT.md §7.2 (also captured in
# docs/envoy-go/BEHAVIOR_CONTRACT.md) is enumerated. Phase 00's diff is
# byte-exact on response-body only; the rest are not-applicable for this
# fixture and explicitly justified.

dimensions:
  response-status:
    applicable: false
    reason: pure TCP fixture — no HTTP layer
  response-body:
    applicable: true
    rule: byte-exact
  response-headers:
    applicable: false
    reason: pure TCP fixture — no HTTP layer
  response-trailers:
    applicable: false
    reason: pure TCP fixture — no HTTP layer
  http2-http3-framing:
    applicable: false
    reason: TCP only
  access-log:
    applicable: false
    reason: subject does not emit access logs in phase 00
  stats:
    applicable: false
    reason: subject does not emit stats in phase 00
  xds:
    applicable: false
    reason: static config; no xDS in phase 00
  timing:
    applicable: false
    reason: timing not opt-in for echo fixture
```

- [ ] **Step 5: Write `driver/doc.go`**

```go
// Package driver implements the 0000-tcp-echo fixture's traffic driver. It
// registers itself with the differential runner at init() time.
package driver
```

- [ ] **Step 6: Write `driver/driver.go`**

```go
package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/esalaine/envoy-go/test/differential"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0000-tcp-echo"
const refContainerListenerPort = 15000

func init() {
	differential.RegisterFixture(fixtureName, &echoDriver{})
}

type echoDriver struct{}

func (echoDriver) ReferenceListenerPort() int { return refContainerListenerPort }

func (echoDriver) ReferenceBootstrap() string {
	// host.docker.internal resolves to the host gateway from inside the
	// Envoy container; the harness sets the backend port via Cmd override
	// indirection. We bake the bootstrap at registration; the runner
	// substitutes the placeholder.
	return refBootstrap
}

func (echoDriver) SubjectConfig(refListenerPort, subjListenerPort, backendPort int) string {
	return fmt.Sprintf(`
listener:
  address: 127.0.0.1
  port: %d
upstream:
  address: 127.0.0.1
  port: %d
`, subjListenerPort, backendPort)
}

func (echoDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	uid := randHex(6)
	var payload []byte
	for n := 0; n < 10; n++ {
		payload = append(payload, []byte(fmt.Sprintf("ping-%d-%s\n", n, uid))...)
	}
	refBytes, err = helpers.TCPRoundTrip(ctx, refAddr, payload, time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("ref drive: %w", err)
	}
	subjBytes, err = helpers.TCPRoundTrip(ctx, subjAddr, payload, time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("subj drive: %w", err)
	}
	return refBytes, subjBytes, nil
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// refBootstrap is the reference Envoy bootstrap with a placeholder
// `port_value: 0` that the runner replaces with the backend port at test
// time. The string-replacement is intentional and trivial; phase 01 replaces
// this with proper templating once a config loader exists.
const refBootstrap = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: host.docker.internal
                      port_value: 0
`

// The runner (test/differential/runner_test.go) performs a strings.Replace
// on `port_value: 0` before passing the bootstrap to StartReferenceProxy.
// The placeholder is the per-fixture contract; the substitution is trivial
// today and will be replaced by proper templating in phase 01.
```

- [ ] **Step 6.5: Wire driver registration via blank import (deferred from Task 12)**

The driver package now exists on disk. Add the blank-import to `test/differential/runner_test.go`'s import block so the driver's `init()` runs and registers itself before `TestDifferential` discovers fixtures. The resulting import block in `runner_test.go` is:

```go
import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver"
)
```

(`fmt` and `strings` are added in Step 7 when the bootstrap-template substitution lands; do not pre-import them here. Each future fixture appends one underscore-import line below the existing one.) Verify it builds:

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 7: Update the runner to substitute the backend port placeholder**

In `test/differential/runner_test.go`, modify `runFixture` so the bootstrap passed to `StartReferenceProxy` substitutes `port_value: 0` with the actual backend port. Add this immediately before the `StartReferenceProxy` call:

```go
bootstrap := strings.Replace(d.ReferenceBootstrap(), "port_value: 0", fmt.Sprintf("port_value: %d", backendPort), 1)
ref, err := StartReferenceProxy(ctx, pin, bootstrap, d.ReferenceListenerPort())
```

Add `"fmt"` and `"strings"` imports to the `runner_test.go` import block if not already present.

- [ ] **Step 8: Make `host.docker.internal` work for Linux containers**

Extend `StartReferenceProxy` (in `test/differential/harness.go`) so `host.docker.internal` resolves to the host gateway on Linux runners. Add the field below to the existing `req := testcontainers.ContainerRequest{ ... }` literal alongside `Image`, `ExposedPorts`, `Cmd`, `WaitingFor`:

```go
req := testcontainers.ContainerRequest{
    // ...existing fields (Image, ExposedPorts, Cmd, WaitingFor)...
    HostConfigModifier: func(hc *container.HostConfig) {
        hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
    },
}
```

(Import `"github.com/docker/docker/api/types/container"`.) The package is a transitive dependency of `testcontainers-go` but is not yet listed as a direct dependency. Run:

```bash
go get github.com/docker/docker/api/types/container
go mod tidy
```

Expected: `go.mod` gains a direct `require github.com/docker/docker v...` line; `go build ./...` succeeds.

- [ ] **Step 9: Run the differential test**

Run: `go test ./test/differential/ -run TestDifferential -v -timeout 120s`
Expected: PASS for `TestDifferential/0000-tcp-echo`. The diff should be byte-equal.

If it fails on backend reachability: confirm `host-gateway` aliasing works on the executor's Docker (Linux daemons need Docker 20.10+).

- [ ] **Step 10: Commit**

```bash
git add test/fixtures/ test/differential/harness.go test/differential/runner_test.go go.mod go.sum
git commit -m "phase 00: 0000-tcp-echo fixture (configs, driver, expectations)"
```

---

## Task 14: Doc.go for `test/conformance/`

**Files:**
- Create: `test/conformance/doc.go`

- [ ] **Step 1: Write the placeholder**

```go
// Package conformance hosts protocol-conformance drivers (h2spec, h3spec,
// grpc-conformance, proxy-wasm conformance). Phase 00 creates the package
// skeleton so the directory tree matches BOOTSTRAP_PROMPT.md §4. The first
// driver lands in the phase that introduces the protocol it tests:
//
//   - h2spec: phase 05 (HTTP/2)
//   - h3spec: HTTP/3 family
//   - grpc:   gRPC family
//   - proxy-wasm: WASM host family
package conformance
```

- [ ] **Step 2: Verify clean build**

Run: `go vet ./test/conformance/ && golangci-lint run ./test/conformance/`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add test/conformance/
git commit -m "phase 00: test/conformance/ package placeholder"
```

---

## Task 15: GitHub Actions CI (`.github/workflows/ci.yml`)

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: ci

on:
  push:
  pull_request:

jobs:
  lint-vet-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true
      - name: go mod download
        run: go mod download
      - name: go vet
        run: go vet ./...
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.55.2
      - name: go test (unit, -short)
        run: go test -short ./...

  differential:
    needs: lint-vet-test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true
      - name: docker available
        run: docker version
      - name: go mod download
        run: go mod download
      - name: differential suite
        run: go test ./test/differential/... -timeout 5m -v
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "phase 00: CI (lint/vet/test + differential)"
```

---

## Task 16: First green CI run on the implementation branch

This task is environment-dependent (requires the executing session to be able to push and observe a CI run). If no GitHub remote is configured, the executor records that fact in PROGRESS.md and the §3 phase-done gate "CI pipeline file exists and has been run at least once green on the phase-00 implementation branch" is satisfied by a *local* equivalent run (per ADR-0009-or-equivalent that the executor writes).

- [ ] **Step 1: If a remote exists, push the branch**

Run:

```bash
git push -u origin phase/00-bootstrap-impl
```

Open the GitHub Actions UI and watch both jobs.

- [ ] **Step 2: Local equivalent (always run, regardless of remote)**

```bash
# Mirror the lint-vet-test job
go vet ./...
golangci-lint run ./...
go test -short ./...

# Mirror the differential job
go test ./test/differential/... -timeout 5m -v

# Match SPEC §3 gate (e) verbatim ("go test ./..." clean, no -short).
# This re-runs everything including the differential suite; on green it is
# the single command quoted into PROGRESS.md as proof of gate (e).
go test ./... -timeout 10m
```

Expected: all seven commands exit 0; the differential job logs `--- PASS: TestDifferential/0000-tcp-echo`; the final full-suite run logs no FAIL lines.

- [ ] **Step 3: Capture all outputs verbatim**

Append the full stdout/stderr of each command into the running `docs/envoy-go/phases/00-bootstrap/PROGRESS.md` (created at first commit by the executor — see §"PROGRESS.md conventions" below). This is consumed by `superpowers:verification-before-completion` (state machine step 4) in a subsequent session.

- [ ] **Step 4: Commit any fixes**

If any command fails, invoke `superpowers:systematic-debugging` *before* proposing a fix (per D-3.1 row 6). Re-run the full local equivalent after any fix. Commit fixes individually so the PROGRESS.md history is granular.

---

## PROGRESS.md conventions

The executor creates `docs/envoy-go/phases/00-bootstrap/PROGRESS.md` after Task 1's first commit and appends to it after every subsequent task. Format:

```markdown
# Phase 00 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Task 1 — Initialize Go module

**Commits:** <sha>
**Notes:** <one paragraph max>
**Outputs:**
```
$ go mod tidy && go build ./...
<output>
```

## Task 2 — ADR-0005, ADR-0006

**Commits:** <sha>
**Notes:** ...
```

This file is not committed until Task 1's first commit lands; thereafter it is updated and committed as part of each task's commit (or in an immediately-following commit if the task already concluded).

---

## Out-of-scope for this plan

These are explicitly NOT part of phase 00's plan and must not be added during execution:

- Any package code under `internal/{listener,cluster,tcp,http,...}` beyond the placeholder `doc.go`.
- Any second fixture under `test/fixtures/`.
- Any conformance driver under `test/conformance/`.
- Any fuzzer.
- TLS, HTTP, stats, access log, admin, xDS — see SPEC §9.
- Any dependency outside the D-3.2 permitted-foundations list.

If reality during implementation pushes toward any of these, invoke `superpowers:systematic-debugging` and either re-scope the offending task in-place or initiate a §6 split per `BOOTSTRAP_PROMPT.md`.

---

## Exit criteria for this PLAN's executor (state-machine step 4 inputs)

When all 16 tasks are complete, the next session (running `superpowers:verification-before-completion`) verifies:

1. All §3 phase-done gates green: differential fixture green, `go vet`, `golangci-lint run`, `go test ./...`, `REVIEW.md` (later step), CI green at least once.
2. `docs/envoy-go/ENVOY_TARGET.md` populated (tag + SHA256, refresh procedure).
3. `go.mod` pins Go 1.23 (or higher per ADR if executor raised it).
4. ADR-0005, 0006, 0007, 0008 (and 0009 if applicable) all landed in `DECISIONS.md`.
5. `docs/envoy-go/phases/00-bootstrap/PROGRESS.md` quotes all gate command outputs verbatim.
6. `STATE.md` advanced to lifecycle-state 4 (verification) at the executor's session-exit, with `next-skill = superpowers:verification-before-completion`.

---

*End of PLAN.*
