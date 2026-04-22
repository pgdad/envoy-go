# Phase 01 — Static Bootstrap Config — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract); `docs/envoy-go/phases/01-static-bootstrap-config/SPEC.md` (authoritative scope); `docs/envoy-go/DECISIONS.md` (ADR-0001…0011 — especially ADR-0003 branch convention, ADR-0005 autonomous-planning adaptation, ADR-0007 phase-00 minimal schema being superseded, ADR-0008 Envoy pin, ADR-0010 V4_ONLY); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (existing structure + header allow-list stub this phase extends); `docs/envoy-go/phases/00-bootstrap/PLAN.md` (style reference for tasks, commit message format, and PROGRESS.md convention).

**Goal:** Retire phase 00's minimal YAML config schema (ADR-0007) and land a real Envoy-bootstrap loader under `internal/bootstrap/` plus the first admin-API surface `GET /ready` under `internal/admin/`, all wired through the rewired `cmd/envoy-go` binary and the extended `0000-tcp-echo` fixture, such that every phase-01 gate in `docs/envoy-go/phases/01-static-bootstrap-config/SPEC.md` §3 passes.

**Architecture:** The subject binary now consumes the same YAML shape upstream Envoy accepts. `internal/bootstrap.Load` parses YAML via `gopkg.in/yaml.v3`, normalizes to JSON, and `protojson.Unmarshal`s into `envoy.config.bootstrap.v3.Bootstrap` from `github.com/envoyproxy/go-control-plane` (proto types only, per doctrine D-3.2). Three skeleton-depth extractors (`AdminSocket`, `FirstListenerSocket`, `FirstClusterEndpointSocket`) surface the exactly-one listener + cluster + endpoint tuples phase 01 needs. `internal/admin.Server` runs a `net/http` HTTP/1.1 server that serves `GET /ready` with a byte-exact response to upstream Envoy v1.37.2's ready-state bytes, gated behind `MarkReady()`. `cmd/envoy-go/main.go` replaces its phase-00 `loadConfig` layer with these two packages while preserving the phase-00 TCP pump (`pump`, `halfClose`, `netConn`) verbatim. The `0000-tcp-echo` fixture evolves: `envoy-go.yaml` becomes a real bootstrap, `expectations.yaml` grows applicable dimensions for admin `/ready`, and the fixture driver probes `/ready` on both proxies so the diff asserts two independent byte-pair observations (TCP echo + admin ready) per run. A new `test/helpers/http_response.go` parser splits admin response bytes into status / headers / body for the dimension-aware comparison. The first production fuzz target `FuzzBootstrapLoad` satisfies phase-done gate (d).

**Tech Stack:**
- Go 1.23 (unchanged from phase 00)
- `github.com/envoyproxy/go-control-plane` — proto types only (no xDS/control-plane helpers), pinned by ADR-0013 (written at execution time by Task 1)
- `google.golang.org/protobuf/encoding/protojson` — proto JSON codec (already indirect via transitive deps; promoted to direct in Task 1)
- `gopkg.in/yaml.v3` — YAML → `map[string]interface{}` pass (already direct from phase 00)
- `net/http` — admin HTTP/1.1 server (stdlib; D-3.2 permitted foundation)
- `testing.F` / Go native fuzzing — `FuzzBootstrapLoad`, CI short-budget invocation
- Upstream Envoy v1.37.2 @ `sha256:c5e8a68e…` (ADR-0008, consumed not modified)
- `golangci-lint` v1.64.8 (ADR-0009, unchanged)
- GitHub Actions `ubuntu-latest` runner (ADR-0009; unchanged from phase 00)

---

## File Structure

Net change: ~1000 LoC across these paths. Estimate; split-gate threshold is ~1500 LoC net (`BOOTSTRAP_PROMPT.md` §6.1). If any single task grows its sub-steps past 10 items or the net estimate revises past 1500 LoC, stop and execute §6.2 split per ADR-0005 §2.

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `go.mod` | Modify | Add direct deps `github.com/envoyproxy/go-control-plane` (ADR-0013 pin) and `google.golang.org/protobuf` (promoted from indirect) |
| `go.sum` | Modify | Checksums for added deps + their transitive closure |
| `internal/bootstrap/doc.go` | Modify | Replace phase-placeholder comment with phase-01 populated package doc |
| `internal/bootstrap/bootstrap.go` | Create | `Load` + `AdminSocket` + `FirstListenerSocket` + `FirstClusterEndpointSocket` |
| `internal/bootstrap/bootstrap_test.go` | Create | Unit tests for happy path + all §8 error paths |
| `internal/bootstrap/fuzz_test.go` | Create | `FuzzBootstrapLoad` + seed corpus (gate (d)) |
| `internal/admin/doc.go` | Modify | Populate phase-01 admin package doc (was phase-08 placeholder — this phase supersedes that forward-looking comment) |
| `internal/admin/admin.go` | Create | `Server` struct, `New`, `Start`, `MarkReady`, `Close`; `/ready` handler |
| `internal/admin/admin_test.go` | Create | Unit tests for ready/pre-init/race/close |
| `cmd/envoy-go/main.go` | Modify | Rewire: replace `loadConfig` + `Config` with `bootstrap.Load` + extractors; start `admin.Server`; preserve pump verbatim |
| `cmd/envoy-go/main_test.go` | Modify | Embedded YAML switches to bootstrap format; allocate admin port; assert ready sentinel and TCP echo (admin probe is the fixture's job, not cmd-level) |
| `cmd/envoy-go/config.go` | Delete | Phase-00 minimal schema, superseded by `internal/bootstrap` (ADR-0021) |
| `cmd/envoy-go/config_test.go` | Delete | Superseded with `config.go` |
| `test/helpers/http_response.go` | Create | Parse HTTP/1.1 response bytes into status-line / headers / body for the dimension-aware diff (ADR-0019) |
| `test/helpers/http_response_test.go` | Create | Table-driven tests for the parser |
| `test/differential/harness.go` | Modify | `SubjectProxy` gains `adminAddr` field + `AdminAddr()`; `StartSubjectProxy` signature grows `subjAdminAddr string` param |
| `test/differential/fixture/fixture.go` | Modify | `Driver.SubjectConfig` signature grows `subjAdminPort int` param; add `ProbeAdmin(ctx, refAdminAddr, subjAdminAddr) (refBytes, subjBytes []byte, err error)` |
| `test/differential/runner_test.go` | Modify | Allocate `subjAdminPort` via `freeTCPPort`; thread through `SubjectConfig` and `StartSubjectProxy`; second diff call against `ProbeAdmin` result |
| `test/fixtures/0000-tcp-echo/envoy-go.yaml` | Modify | Rewritten from phase-00 minimal to real Envoy bootstrap skeleton |
| `test/fixtures/0000-tcp-echo/expectations.yaml` | Modify | `response-status`, `response-body`, `response-headers` dimensions become `applicable: true` with scope annotations; allow-list reference to `BEHAVIOR_CONTRACT.md` §Admin API |
| `test/fixtures/0000-tcp-echo/README.md` | Modify | Describe the new admin observation and the three benign divergences between `envoy.yaml` and `envoy-go.yaml` |
| `test/fixtures/0000-tcp-echo/driver/driver.go` | Modify | `SubjectConfig` returns real bootstrap YAML (four port interpolation); new `ProbeAdmin` method issues `GET /ready` and returns raw response bytes |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | New top-level H2 section `## Admin API — /ready` with the authoritative response contract and header allow-list entries |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0012 through ADR-0021 (ten ADRs — listed below) |
| `docs/envoy-go/ROADMAP.md` | *Not modified by this plan* | Row 01 advances to `done` at state-machine step 6 in a later session, per ADR-0005 |
| `docs/envoy-go/STATE.md` | Modify (at exit) | Advanced to `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` at the plan-authoring session's exit commit |
| `docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md` | Create (during execution) | Append-only running log per `BOOTSTRAP_PROMPT.md` §5 step 3, matching phase-00's template |

---

## ADRs introduced by this plan

Ten ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that needs it (per phase-00 precedent — see ADR-0006, ADR-0007, ADR-0008 landing commits). All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the current tail (ADR-0011).

- **ADR-0012 — YAML → proto bootstrap loader pipeline.** Decision: three-stage — `gopkg.in/yaml.v3` into `map[string]interface{}` → `encoding/json.Marshal` → `google.golang.org/protobuf/encoding/protojson.Unmarshal` into `envoy.config.bootstrap.v3.Bootstrap`. Rationale: `protojson` is the canonical proto JSON codec; YAML's flow-style `{foo: bar}` objects are JSON-compatible; `yaml.v3` already ships in the module; no wrapper library (e.g. `sigs.k8s.io/yaml`) needed for a single-caller pipeline. Supersession path: if future phases need `!!binary` tags or YAML-native durations, the pipeline can be swapped via new ADR. Lands in Task 2.
- **ADR-0013 — `github.com/envoyproxy/go-control-plane` version pin.** Decision: pin to the version chosen by the executor at Task 1 time (recommendation: latest released tag that includes `envoy.config.bootstrap.v3`; see `https://pkg.go.dev/github.com/envoyproxy/go-control-plane` for tag list). Rationale: phase 01 imports *proto types only*, so the version choice is about proto field stability, not control-plane API compat. Refresh is its own future phase per D-3.7. Lands in Task 1.
- **ADR-0014 — `Server:` header value on envoy-go admin responses.** Decision: literal string `envoy` (byte-exact match with upstream). Rationale: the `Server:` value is part of what downstream admin consumers observe; matching byte-exact minimizes allow-list entries and avoids establishing an envoy-go-specific identity in the admin surface before a deliberate choice in phase 08. No phase-01 or declared future consumer encodes logic against this header. Lands in Task 8.
- **ADR-0015 — Pre-init admin response contract.** Decision: the phase-01 implementer observes upstream Envoy v1.37.2's actual pre-init `/ready` response shape empirically (Task 7); the phase-01 admin server reproduces that shape byte-exact BEFORE `MarkReady` is called; the phase-01 differential test does not exercise the pre-init window (subject calls `MarkReady` before printing the ready sentinel, so the harness only ever observes the ready state). Rationale: a byte-exact pre-init response is cheap to implement and forward-compatible if a later phase introduces an initialization state machine that exposes pre-init to the harness. Lands in Task 7.
- **ADR-0016 — Bootstrap loader unknown-field handling.** Decision: `protojson.UnmarshalOptions{DiscardUnknown: false}` — reject any field not defined in the proto schema. Exception: `typed_config` fields of type `google.protobuf.Any` carry implementation-specific bytes and are preserved without resolving (phase 01 does not resolve filter Any contents per SPEC §2). Rationale: fixture authors get immediate feedback on typos; `Any` preservation is required for the filter chain parse pass. Lands in Task 2.
- **ADR-0017 — Node field semantics.** Decision: (a) — the loader parses `node` into the proto as-is and does not enforce presence or content of `node.id` / `node.cluster` in phase 01. Rationale: YAGNI — no phase-01 consumer of `node` exists; enforcing fields now would couple the loader to admin semantics that only land in phase 08 or later. Lands in Task 5.
- **ADR-0018 — `FuzzBootstrapLoad` CI budget for gate (d).** Decision: 30 seconds per CI run via `-fuzztime=30s`. Rationale: short enough to not dominate the 5-minute differential job wall-clock; long enough to exercise the seed corpus and a few thousand mutations. A longer nightly lane is out of scope (no scheduled phase introduces it). Lands in Task 6.
- **ADR-0019 — Admin HTTP response parser location.** Decision: `test/helpers/http_response.go`. Rationale: anticipated reuse by fixtures 0002+ that probe HTTP surfaces; colocated with `test/helpers/tcp.go` (the phase-00 TCP round-tripper) establishes `test/helpers/` as the shared test-side protocol-primitives package. Lands in Task 11.
- **ADR-0020 — `cmd/envoy-go/main_test.go` rewrite vs replacement.** Decision: rewrite (same file, same test name, new YAML shape + admin port allocation). Rationale: keeps cmd-level unit coverage lightweight without adding a subprocess-integration dimension that the differential suite already covers. Lands in Task 12.
- **ADR-0021 — Supersession of ADR-0007 by `internal/bootstrap`.** Decision: the phase-00 minimal YAML schema codified in ADR-0007 is retired; envoy-go configuration is now the Envoy bootstrap proto as consumed by `internal/bootstrap.Load`. `cmd/envoy-go/config.go` and `config_test.go` are deleted. ADR-0007 is NOT edited (append-only); this ADR explicitly names ADR-0007 as superseded per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4. Lands in Task 13.

No other ADRs are required. If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR in the same commit as the code it decides for. If such a decision would expand phase-01 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/01-static-bootstrap-config-plan` (this plan's authoring branch). Recommended: `.worktrees/phase-01-static-bootstrap-config-impl` on branch `phase/01-static-bootstrap-config-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master.
2. Have `docker` available (verify with `docker version`). Required for Task 7 (empirical upstream Envoy observation) and the differential job in Task 17.
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is already the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` if missing.
5. `go test ./...` must be green on master at cold-start — this plan assumes a clean baseline (phase-00 gate (e) still holds). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Add go-control-plane + protojson direct dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0013)

- [ ] **Step 1: Pick the `go-control-plane` version**

Check `https://pkg.go.dev/github.com/envoyproxy/go-control-plane?tab=versions` (or `go list -m -versions github.com/envoyproxy/go-control-plane` which works offline if cached) and pick the latest released tag that includes `envoy.config.bootstrap.v3` — representative: `v0.13.x`. Record the exact version string for ADR-0013.

- [ ] **Step 2: Append ADR-0013 to `docs/envoy-go/DECISIONS.md`**

Append a new entry at the tail, matching the existing ADR format (`## ADR-NNNN: title`, `**Status:** Accepted`, `**Date:** YYYY-MM-DD`, `**Doctrine:** D-3.5`, then `### Context` / `### Decision` / `### Consequences`). Body captures: the chosen version and its commit/tag date; the fact that phase 01 imports proto types only (no control-plane helpers); the refresh-is-a-future-phase clause per D-3.7.

- [ ] **Step 3: Add the direct dependencies**

Run:
```bash
go get github.com/envoyproxy/go-control-plane@<ADR-0013 version>
go get google.golang.org/protobuf
go mod tidy
```
`google.golang.org/protobuf` is already indirect (via testcontainers-go's transitive closure); `go get` promotes it to a direct require block entry, which is what we want for ADR-0012's pipeline.

- [ ] **Step 4: Verify the bootstrap proto package is importable**

Create a throwaway file `internal/bootstrap/probe.go` with:
```go
package bootstrap

import _ "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
```
Run: `go build ./...`
Expected: PASS.
Then delete `internal/bootstrap/probe.go` (it was only a compile probe; Task 2 writes the real `bootstrap.go`).

- [ ] **Step 5: Run lint + vet**

Run: `go vet ./... && golangci-lint run ./...`
Expected: PASS. Any lint finding against the new deps is a bug to fix now, not a TODO.

- [ ] **Step 6: Create PROGRESS.md with preamble + Task 1 entry**

Use the template quoted in the `## PROGRESS.md conventions` section at the bottom of this PLAN. The preamble section records any deviation from the `## Execution preconditions` block above (expected: none). The Task 1 entry records the picked version, the `go mod tidy` output, and the build/vet/lint outputs verbatim.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: add go-control-plane + protojson direct deps [ADR-0013]"
```

---

## Task 2: `internal/bootstrap.Load` — happy path + reject dynamic_resources/layered_runtime

**Files:**
- Modify: `internal/bootstrap/doc.go`
- Create: `internal/bootstrap/bootstrap.go`
- Create: `internal/bootstrap/bootstrap_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0012, ADR-0016)

- [ ] **Step 1: Update `internal/bootstrap/doc.go`**

Replace the phase-placeholder comment with:
```go
// Package bootstrap loads an Envoy v3 Bootstrap proto from YAML (the same YAML
// shape upstream Envoy accepts) and exposes skeleton-depth extractors the
// cmd/envoy-go subject uses to wire its listener, upstream endpoint, and admin
// surface. See docs/envoy-go/phases/01-static-bootstrap-config/SPEC.md §5.1.
package bootstrap
```

- [ ] **Step 2: Append ADR-0012 and ADR-0016 to `DECISIONS.md`**

Two entries, at the tail, in the standard ADR format. ADR-0012 records the three-stage `yaml.v3 → encoding/json → protojson.Unmarshal` pipeline and names the alternatives considered (single-library `sigs.k8s.io/yaml`, direct YAML-to-proto reflection) with the rationale for rejecting each. ADR-0016 records `DiscardUnknown: false` and the `typed_config` `Any` preservation exception.

- [ ] **Step 3: Write the failing test (happy path) in `internal/bootstrap/bootstrap_test.go`**

```go
package bootstrap

import (
	"strings"
	"testing"
)

const sampleBootstrap = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

func TestLoad_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bs == nil {
		t.Fatal("Load returned nil bootstrap with nil error")
	}
	if got, want := bs.GetNode().GetId(), "test-node"; got != want {
		t.Errorf("node.id: got %q, want %q", got, want)
	}
	if got := bs.GetStaticResources(); got == nil {
		t.Fatal("static_resources missing")
	}
	if n := len(bs.GetStaticResources().GetListeners()); n != 1 {
		t.Errorf("listeners: got %d, want 1", n)
	}
	if n := len(bs.GetStaticResources().GetClusters()); n != 1 {
		t.Errorf("clusters: got %d, want 1", n)
	}
}

func TestLoad_RejectsDynamicResources(t *testing.T) {
	yaml := sampleBootstrap + `
dynamic_resources:
  ads_config:
    api_type: GRPC
`
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want error for dynamic_resources, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("error prefix: got %q, want to start with \"bootstrap: \"", err.Error())
	}
	if !strings.Contains(err.Error(), "dynamic_resources") {
		t.Errorf("error should name dynamic_resources: %q", err.Error())
	}
}

func TestLoad_RejectsLayeredRuntime(t *testing.T) {
	yaml := sampleBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want error for layered_runtime, got nil")
	}
	if !strings.Contains(err.Error(), "layered_runtime") {
		t.Errorf("error should name layered_runtime: %q", err.Error())
	}
}
```

- [ ] **Step 4: Run the test to confirm it fails**

Run: `go test ./internal/bootstrap/ -run TestLoad -v`
Expected: FAIL — `undefined: Load` (compile error) is the correct first-run failure.

- [ ] **Step 5: Write `internal/bootstrap/bootstrap.go`**

```go
package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// Load parses r as YAML (upstream Envoy's YAML shape), converts to JSON, and
// unmarshals into an Envoy v3 Bootstrap proto. Unknown fields at any depth
// cause an error (ADR-0016). The phase-01 unsupported surfaces
// dynamic_resources and layered_runtime cause an error even though the proto
// itself defines them.
//
// Every error returned by Load begins with "bootstrap: ".
func Load(r io.Reader) (*bootstrapv3.Bootstrap, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read: %w", err)
	}
	var generic map[string]interface{}
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("bootstrap: yaml parse: %w", err)
	}
	if generic == nil {
		return nil, fmt.Errorf("bootstrap: empty document")
	}
	if _, ok := generic["dynamic_resources"]; ok {
		return nil, fmt.Errorf("bootstrap: dynamic_resources not supported in phase 01 (see SPEC §2)")
	}
	if _, ok := generic["layered_runtime"]; ok {
		return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
	}
	jsonBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: to json: %w", err)
	}
	bs := &bootstrapv3.Bootstrap{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(jsonBytes, bs); err != nil {
		return nil, fmt.Errorf("bootstrap: protojson: %w", err)
	}
	return bs, nil
}
```

- [ ] **Step 6: Run the test to confirm it passes**

Run: `go test ./internal/bootstrap/ -run TestLoad -v`
Expected: PASS (all three subtests green).

- [ ] **Step 7: Run full lint/vet/test**

Run: `go vet ./... && golangci-lint run ./internal/bootstrap/ && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/bootstrap/doc.go internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: bootstrap.Load happy path + dynamic_resources/layered_runtime rejection [ADR-0012, ADR-0016]"
```

Before the commit, append a Task 2 entry to PROGRESS.md with the test output and chosen pipeline rationale verbatim.

---

## Task 3: `internal/bootstrap.Load` — error paths

**Files:**
- Modify: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Add error-path tests**

Append to `bootstrap_test.go`:
```go
func TestLoad_YAMLSyntaxError(t *testing.T) {
	_, err := Load(strings.NewReader("not: valid: yaml: at all: :::"))
	if err == nil {
		t.Fatal("Load: want yaml parse error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: yaml parse:") {
		t.Errorf("error prefix: %q", err.Error())
	}
}

func TestLoad_UnknownTopLevelField(t *testing.T) {
	yaml := sampleBootstrap + "\nnot_a_real_field: 42\n"
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want unknown-field error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: protojson:") {
		t.Errorf("error prefix: %q (expected protojson rejection)", err.Error())
	}
}

func TestLoad_EmptyDocument(t *testing.T) {
	_, err := Load(strings.NewReader(""))
	if err == nil {
		t.Fatal("Load: want empty-doc error, got nil")
	}
	if !strings.Contains(err.Error(), "empty document") {
		t.Errorf("error: %q", err.Error())
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/bootstrap/ -v`
Expected: all three new tests PASS (the production code already handles these paths; these tests lock the behavior in).

- [ ] **Step 3: Commit**

```bash
git add internal/bootstrap/bootstrap_test.go docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: bootstrap.Load error-path tests (syntax, unknown field, empty)"
```

---

## Task 4: `internal/bootstrap.AdminSocket` extractor

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `bootstrap_test.go`:
```go
func TestAdminSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := AdminSocket(bs)
	if err != nil {
		t.Fatalf("AdminSocket: %v", err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want 127.0.0.1", host)
	}
	if port != 0 {
		t.Errorf("port: got %d, want 0", port)
	}
}

func TestAdminSocket_MissingAdmin(t *testing.T) {
	yaml := `
static_resources:
  listeners: []
  clusters: []
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = AdminSocket(bs)
	if err == nil {
		t.Fatal("want error for missing admin, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("prefix: %q", err.Error())
	}
}
```

- [ ] **Step 2: Run — fail as `undefined: AdminSocket`**

Run: `go test ./internal/bootstrap/ -run TestAdminSocket -v`
Expected: FAIL (compile).

- [ ] **Step 3: Implement `AdminSocket`**

Append to `bootstrap.go`:
```go
// AdminSocket returns host and port from admin.address.socket_address. Errors
// if admin is missing or the address is not a socket_address.
func AdminSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error) {
	adm := bs.GetAdmin()
	if adm == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin")
	}
	addr := adm.GetAddress()
	if addr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin.address")
	}
	sa := addr.GetSocketAddress()
	if sa == nil {
		return "", 0, fmt.Errorf("bootstrap: admin.address is not a socket_address")
	}
	return sa.GetAddress(), sa.GetPortValue(), nil
}
```

- [ ] **Step 4: Run — both tests PASS**

Run: `go test ./internal/bootstrap/ -run TestAdminSocket -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: bootstrap.AdminSocket extractor"
```

---

## Task 5: `internal/bootstrap.FirstListenerSocket` + `FirstClusterEndpointSocket`

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `internal/bootstrap/bootstrap_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0017)

- [ ] **Step 1: Append ADR-0017**

Decision: parse `node` into the proto as-is; do not enforce `node.id` / `node.cluster` presence or content in phase 01. Rationale: YAGNI — no phase-01 consumer of `node` exists.

- [ ] **Step 2: Write the failing tests**

Append to `bootstrap_test.go`:
```go
func TestFirstListenerSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := FirstListenerSocket(bs)
	if err != nil {
		t.Fatalf("FirstListenerSocket: %v", err)
	}
	if host != "127.0.0.1" || port != 0 {
		t.Errorf("got %s:%d, want 127.0.0.1:0", host, port)
	}
}

func TestFirstListenerSocket_ZeroListeners(t *testing.T) {
	yaml := `
admin: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }
static_resources: { listeners: [], clusters: [] }
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstListenerSocket(bs)
	if err == nil || !strings.Contains(err.Error(), "expected exactly one listener") {
		t.Errorf("err: %v", err)
	}
}

func TestFirstListenerSocket_TwoListeners(t *testing.T) {
	// Two-listener YAML (copy-paste sampleBootstrap and add a second listener entry).
	yaml := strings.Replace(sampleBootstrap,
		"  listeners:\n    - name: l_tcp",
		"  listeners:\n    - name: l_a\n    - name: l_tcp", 1)
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstListenerSocket(bs)
	if err == nil || !strings.Contains(err.Error(), "got 2") {
		t.Errorf("err: %v", err)
	}
}

func TestFirstClusterEndpointSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := FirstClusterEndpointSocket(bs)
	if err != nil {
		t.Fatalf("FirstClusterEndpointSocket: %v", err)
	}
	if host != "127.0.0.1" || port != 0 {
		t.Errorf("got %s:%d, want 127.0.0.1:0", host, port)
	}
}

func TestFirstClusterEndpointSocket_EmptyEndpoints(t *testing.T) {
	yaml := `
admin: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }
static_resources:
  listeners:
    - name: l
      address: { socket_address: { address: 127.0.0.1, port_value: 0 } }
  clusters:
    - name: c
      type: STATIC
      load_assignment: { cluster_name: c, endpoints: [] }
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstClusterEndpointSocket(bs)
	if err == nil {
		t.Fatal("want error for empty endpoints, got nil")
	}
	if !strings.Contains(err.Error(), "endpoints entry") {
		t.Errorf("error should name endpoints: %q", err.Error())
	}
}
```

- [ ] **Step 3: Run — fail as `undefined`**

Run: `go test ./internal/bootstrap/ -run "TestFirstListener|TestFirstCluster" -v`
Expected: FAIL (compile).

- [ ] **Step 4: Implement the two extractors**

Append to `bootstrap.go`:
```go
// FirstListenerSocket returns host and port of static_resources.listeners[0]
// .address.socket_address. Errors if there are zero or more than one listener,
// or if the listener's address is not a socket_address.
func FirstListenerSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error) {
	sr := bs.GetStaticResources()
	if sr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing static_resources")
	}
	ls := sr.GetListeners()
	if len(ls) != 1 {
		return "", 0, fmt.Errorf("bootstrap: phase 01: expected exactly one listener, got %d", len(ls))
	}
	addr := ls[0].GetAddress()
	if addr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing listener[0].address")
	}
	sa := addr.GetSocketAddress()
	if sa == nil {
		return "", 0, fmt.Errorf("bootstrap: listener[0].address is not a socket_address")
	}
	return sa.GetAddress(), sa.GetPortValue(), nil
}

// FirstClusterEndpointSocket returns host and port of
// static_resources.clusters[0].load_assignment.endpoints[0].lb_endpoints[0]
// .endpoint.address.socket_address. Errors on any "exactly one" violation.
func FirstClusterEndpointSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error) {
	sr := bs.GetStaticResources()
	if sr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing static_resources")
	}
	cs := sr.GetClusters()
	if len(cs) != 1 {
		return "", 0, fmt.Errorf("bootstrap: phase 01: expected exactly one cluster, got %d", len(cs))
	}
	la := cs[0].GetLoadAssignment()
	if la == nil {
		return "", 0, fmt.Errorf("bootstrap: missing cluster[0].load_assignment")
	}
	eps := la.GetEndpoints()
	if len(eps) != 1 {
		return "", 0, fmt.Errorf("bootstrap: phase 01: expected exactly one endpoints entry, got %d", len(eps))
	}
	lbs := eps[0].GetLbEndpoints()
	if len(lbs) != 1 {
		return "", 0, fmt.Errorf("bootstrap: phase 01: expected exactly one lb_endpoint, got %d", len(lbs))
	}
	ep := lbs[0].GetEndpoint()
	if ep == nil {
		return "", 0, fmt.Errorf("bootstrap: missing lb_endpoint[0].endpoint")
	}
	addr := ep.GetAddress()
	if addr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing endpoint.address")
	}
	sa := addr.GetSocketAddress()
	if sa == nil {
		return "", 0, fmt.Errorf("bootstrap: endpoint.address is not a socket_address")
	}
	return sa.GetAddress(), sa.GetPortValue(), nil
}
```

- [ ] **Step 5: Run — tests PASS**

Run: `go test ./internal/bootstrap/ -v`
Expected: PASS (all bootstrap tests).

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: bootstrap.FirstListenerSocket + FirstClusterEndpointSocket extractors [ADR-0017]"
```

---

## Task 6: `FuzzBootstrapLoad` fuzz target (gate (d))

**Files:**
- Create: `internal/bootstrap/fuzz_test.go` (all seeds inline via `f.Add`; no external corpus files)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0018)
- Modify: `.github/workflows/ci.yml` (add fuzz short-budget invocation)

- [ ] **Step 1: Append ADR-0018**

Decision: `-fuzztime=30s` per CI run. Rationale: exercises seed corpus + a few thousand mutations without dominating the 5-minute differential job wall-clock.

- [ ] **Step 2: Create `internal/bootstrap/fuzz_test.go`**

```go
package bootstrap

import (
	"bytes"
	"testing"
)

func FuzzBootstrapLoad(f *testing.F) {
	// Seed corpus: the known-good sample, plus degenerate inputs. The CI
	// short-budget invocation explores mutations of these seeds.
	f.Add([]byte(sampleBootstrap))
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("admin:"))
	f.Add([]byte("static_resources:\n  listeners: []\n  clusters: []"))
	// Deeply nested YAML.
	nested := bytes.Repeat([]byte("- "), 200)
	f.Add(nested)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic. Any input returns either (*Bootstrap, nil) or
		// (nil, err starting with "bootstrap: ").
		_, _ = Load(bytes.NewReader(data))
	})
}
```

- [ ] **Step 3: Seed the corpus from the fixture**

The fixture's `envoy.yaml` (the reference bootstrap) is already a valid Envoy bootstrap. Copy its contents into an additional `f.Add([]byte(...))` call in step 2's test — inline the full YAML literal so the fuzz test stays self-contained (no filesystem reads at fuzz time). The fixture's `envoy-go.yaml` will be added to the corpus once Task 12 lands it in bootstrap shape; for now, use the `sampleBootstrap` const from `bootstrap_test.go` (already covers the happy path).

- [ ] **Step 4: Run the fuzz target's seed-only replay**

Run: `go test ./internal/bootstrap/ -run=FuzzBootstrapLoad`
Expected: PASS (seed replay does NOT run the fuzz engine — that's reserved for `-fuzz=` invocation). This confirms the test compiles and each seed returns without panic.

- [ ] **Step 5: Run the fuzz engine short-budget**

Run: `go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$`
Expected: PASS. Any crashing input must be fixed in `bootstrap.go` (strengthen error handling, never panic). Do not add a crashing input to `testdata/fuzz/FuzzBootstrapLoad/` unless it is intentionally being kept as a regression seed after the bug is fixed.

- [ ] **Step 6: Wire fuzz into CI**

Modify `.github/workflows/ci.yml` to append a new job `fuzz-bootstrap`:
```yaml
  fuzz-bootstrap:
    runs-on: ubuntu-latest
    needs: lint-vet-test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true
      - run: go mod download
      - run: go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$
```
The job runs in parallel with (or after) `lint-vet-test` — keep it outside of `differential` since it does not need Docker.

- [ ] **Step 7: Run lint + all tests**

Run: `go vet ./... && golangci-lint run ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/bootstrap/fuzz_test.go docs/envoy-go/DECISIONS.md .github/workflows/ci.yml docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: FuzzBootstrapLoad fuzz target + CI job [ADR-0018]"
```

Quote the fuzz engine output (number of executions, zero crashes) verbatim in the PROGRESS.md entry.

---

## Task 7: Empirically observe upstream Envoy v1.37.2 `/ready` bytes

**Files:**
- Create: `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` (evidence file; committed alongside ADR-0015)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0015)

- [ ] **Step 1: Start a reference Envoy container with a minimal bootstrap**

Write a scratch YAML bootstrap to `/tmp/envoy-ready-probe.yaml`:
```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners: []
  clusters: []
```
Pull and run the pinned image (tag from `docs/envoy-go/ENVOY_TARGET.md`):
```bash
TAG=$(grep -oE 'envoyproxy/envoy:[^`]+' docs/envoy-go/ENVOY_TARGET.md | head -1)
docker run --rm -d --name envoy-ready-probe -p 9901:9901 \
  -v /tmp/envoy-ready-probe.yaml:/etc/envoy/envoy.yaml \
  "$TAG" \
  envoy -c /etc/envoy/envoy.yaml --log-level warn
sleep 2
```

- [ ] **Step 2: Capture the ready-state response**

Run: `curl -s -D - -o /tmp/envoy-ready-body.txt http://127.0.0.1:9901/ready && cat /tmp/envoy-ready-body.txt`
Expected: something like `LIVE\n` (body) plus headers (status line, Date, Content-Type, Content-Length, possibly Cache-Control, Server, X-Envoy-*). Redirect the full raw response to a capture file: `curl -s --dump-header /tmp/envoy-ready-headers.txt http://127.0.0.1:9901/ready -o /tmp/envoy-ready-body.txt`. Combine with `cat /tmp/envoy-ready-headers.txt /tmp/envoy-ready-body.txt > upstream-ready-evidence.txt`.

- [ ] **Step 3: Attempt to capture a pre-init response**

Immediately after `docker run` (before Envoy has fully initialized), loop a `curl -D -` against `/ready` until it succeeds; the first successful response may be `200 OK with LIVE\n` (Envoy is fast) or a non-200 with a body like `PRE_INITIALIZING\n`. Record whatever is observed. If ten consecutive attempts within a 500ms-spaced loop all return `200 LIVE\n`, declare the pre-init window unobservable from this configuration and proceed with ADR-0015 option (b) — but still implement byte-exact pre-init matching if any non-200 response was captured at any time.

- [ ] **Step 4: Clean up**

Run: `docker rm -f envoy-ready-probe`

- [ ] **Step 5: Write the evidence file**

Create `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` with four sections: `## Environment` (docker version, image tag + SHA), `## Ready-state response (raw)` (verbatim bytes from Step 2's capture, in a triple-backtick block), `## Pre-init response (raw)` (if captured) or `Pre-init response unobservable — recorded <N> successful probes at 500ms intervals within <duration>`, `## Header allow-list implications` (which headers are deterministic vs not).

- [ ] **Step 6: Append ADR-0015 to `DECISIONS.md`**

The ADR records the observed pre-init shape (or declares it unobservable), links to the evidence file, and locks the phase-01 admin server contract: byte-exact ready response matching the evidence; pre-init response either matching evidence byte-exact or declared out-of-scope of the phase-01 differential test with a supporting rationale.

- [ ] **Step 7: Commit**

```bash
git add docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: capture upstream Envoy /ready bytes + contract ADR [ADR-0015]"
```

---

## Task 8: `internal/admin` — Server skeleton + `/ready` ready-state

**Files:**
- Modify: `internal/admin/doc.go`
- Create: `internal/admin/admin.go`
- Create: `internal/admin/admin_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0014)

- [ ] **Step 1: Update `internal/admin/doc.go`**

Replace placeholder with:
```go
// Package admin serves the envoy-go admin API on HTTP/1.1. Phase 01 implements
// only GET /ready (see docs/envoy-go/BEHAVIOR_CONTRACT.md §Admin API). Phase
// 08 extends this package with the remaining admin endpoints.
package admin
```

- [ ] **Step 2: Append ADR-0014 to `DECISIONS.md`**

Decision: `Server: envoy` literal header value on envoy-go admin responses. Rationale: byte-exact match with upstream avoids allow-list noise; no phase-01-or-later consumer encodes logic against the identity header.

- [ ] **Step 3: Write the failing test (ready-state only)**

Create `internal/admin/admin_test.go`:
```go
package admin

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServer_ReadyState(t *testing.T) {
	s := New("127.0.0.1:0")
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	s.MarkReady()

	// Give the accept goroutine a beat.
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "LIVE\n" {
		t.Errorf("body: got %q, want %q", body, "LIVE\n")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/plain")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "5" {
		t.Errorf("Content-Length: got %q, want %q", cl, "5")
	}
	if srv := resp.Header.Get("Server"); srv != "envoy" {
		t.Errorf("Server: got %q, want %q", srv, "envoy")
	}
}

// freeAddr returns a host:port string for a port that is currently free. The
// port is released before return; the caller may race with another binder. Used
// only for tests that pass port 0 to New (Start binds port 0 atomically).
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}
```

- [ ] **Step 4: Run — fail as `undefined: New`**

Run: `go test ./internal/admin/ -run TestServer_ReadyState -v`
Expected: FAIL (compile).

- [ ] **Step 5: Write `internal/admin/admin.go`**

```go
package admin

import (
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// Server is the admin HTTP/1.1 server. Only /ready is implemented in phase 01;
// other admin endpoints land in phase 08.
type Server struct {
	addr     string
	ln       net.Listener
	httpSrv  *http.Server
	ready    atomic.Bool
}

// New returns an admin server bound target addr. The server is not running
// yet; call Start. The /ready gate is initially closed (MarkReady flips it).
func New(addr string) *Server {
	return &Server{addr: addr}
}

// Start binds and begins serving in a background goroutine. Returns the bound
// address (useful when addr had port 0). Error only if bind fails.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", s.handleReady)
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// MarkReady flips /ready into the ready state.
func (s *Server) MarkReady() { s.ready.Store(true) }

// Close performs best-effort shutdown. Idempotent. No graceful drain (phase 08).
func (s *Server) Close() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// handleReady implements the /ready contract. See
// docs/envoy-go/BEHAVIOR_CONTRACT.md §Admin API. Byte-exact to upstream
// Envoy v1.37.2 observed at ADR-0015 capture time.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Fixed headers: ready and pre-init share Content-Type/Server. The body
	// and status line switch on the atomic flag.
	h := w.Header()
	h.Set("Content-Type", "text/plain")
	h.Set("Server", "envoy")
	// Cache-Control must match upstream byte-exact; captured at Task 7.
	// Replace the value below with whatever upstream-ready-observation.md
	// pins if it differs from "no-cache, max-age=0".
	h.Set("Cache-Control", "no-cache, max-age=0")

	if !s.ready.Load() {
		// Pre-init contract per ADR-0015. If the evidence file captured a
		// specific body and status other than what is below, replace these
		// lines byte-exact.
		body := []byte("PRE_INITIALIZING\n")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
		return
	}
	body := []byte("LIVE\n")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

- [ ] **Step 6: Reconcile the snippet against Task 7 evidence (MANDATORY before commit)**

Open `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` (committed by Task 7). Compare each literal in `handleReady` above against the recorded upstream bytes:

1. Ready-state body: must equal the captured body byte-for-byte. If upstream captured `LIVE\n` — keep `"LIVE\n"`. If upstream captured different bytes (e.g. `"LIVE"` without LF, or `"LIVE\r\n"`), replace the literal and update the `Content-Length` constant accordingly.
2. Pre-init body + status: must equal the captured pre-init bytes if any were captured. If Task 7 declared pre-init unobservable, pick the closest-to-upstream bytes the evidence file records (even one captured sample is enough) or match the upstream documentation's literal; either way, the test `TestServer_PreInit_BeforeMarkReady` (Task 9) must agree with the chosen bytes.
3. `Cache-Control` header value: must equal the captured value. The snippet's placeholder `"no-cache, max-age=0"` is a guess — replace with the evidence bytes if different.
4. Additional headers upstream emits on `/ready` (e.g. `X-Envoy-Upstream-Service-Time`, `Server-Timing`, anything else in the captured response): either emit each byte-exact in `handleReady`, OR add an allow-list entry in Task 10's `## Admin API — /ready` section. The plan cannot predict which headers will appear; the implementer decides per header based on "is the value deterministic enough to replicate byte-exact."

Do NOT commit Task 8 until this step's reconciliation is complete. PROGRESS.md's Task 8 entry should enumerate every divergence from the snippet above and the resolution taken.

- [ ] **Step 7: Run — test PASS**

Run: `go test ./internal/admin/ -run TestServer_ReadyState -v`
Expected: PASS.

- [ ] **Step 8: Run lint/vet/all tests**

Run: `go vet ./... && golangci-lint run ./internal/admin/ && go test ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/admin/doc.go internal/admin/admin.go internal/admin/admin_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: admin.Server + /ready ready-state byte-exact [ADR-0014]"
```

---

## Task 9: `internal/admin` — pre-init, MarkReady, Close, race

**Files:**
- Modify: `internal/admin/admin_test.go`

- [ ] **Step 1: Add pre-init + MarkReady + Close + race tests**

Append to `admin_test.go`:
```go
func TestServer_PreInit_BeforeMarkReady(t *testing.T) {
	s := New("127.0.0.1:0")
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	// No MarkReady call. /ready should return the pre-init response per
	// ADR-0015.
	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 {
		t.Errorf("pre-init status: got 200, want non-200 per ADR-0015")
	}
	// Body must match whatever ADR-0015 locks. If the evidence file says
	// "PRE_INITIALIZING\n", test for that. If the evidence file declares
	// pre-init unobservable and says "any body", relax the assertion to
	// "non-empty, non-LIVE".
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "LIVE\n" {
		t.Errorf("pre-init body must not be LIVE\\n (would collide with ready state)")
	}
}

func TestServer_MarkReady_IsAtomic(t *testing.T) {
	s := New("127.0.0.1:0")
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			resp, _ := http.Get("http://" + addr + "/ready")
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
		close(done)
	}()
	s.MarkReady()
	<-done
	// Final probe should be 200/LIVE.
	resp, _ := http.Get("http://" + addr + "/ready")
	if resp == nil {
		t.Fatal("final probe: nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("final status: got %d, want 200", resp.StatusCode)
	}
}

func TestServer_Close_Idempotent(t *testing.T) {
	s := New("127.0.0.1:0")
	// Close before Start.
	if err := s.Close(); err != nil {
		t.Errorf("Close before Start: %v", err)
	}
	// Close after Start.
	s2 := New("127.0.0.1:0")
	_, err := s2.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second Close.
	if err := s2.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
```

- [ ] **Step 2: Run — tests PASS (with -race)**

Run: `go test -race ./internal/admin/ -v`
Expected: PASS. If race detector flags `ready atomic.Bool` access — it shouldn't, since `sync/atomic.Bool` is race-safe — or flags any other shared state, fix in `admin.go` before proceeding.

- [ ] **Step 3: Commit**

```bash
git add internal/admin/admin_test.go docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: admin.Server pre-init + MarkReady atomicity + Close idempotency tests"
```

---

## Task 10: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — Admin API section

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Read the current BEHAVIOR_CONTRACT structure**

Confirm the top-level H2 headings: `## Equivalence Matrix`, `## Header allow-list` (stub), `## Stat-name mapping` (stub), `## Access log field mapping` (stub), `## xDS wire state machine` (stub), `## Timing tolerances` (stub), `## Test harness host networking` (populated with DNS subsection).

- [ ] **Step 2: Add a new H2 section `## Admin API — /ready`**

Insert between `## Timing tolerances` and `## Test harness host networking`. Body:

```markdown
## Admin API — /ready

*Introduced by phase 01; justified by ADR-0015 (pre-init contract) and ADR-0014 (Server header value).*

### Ready-state response (authoritative)

Byte-exact to upstream Envoy v1.37.2 as captured in
`docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md`.

- **Status line:** `HTTP/1.1 200 OK`
- **Body:** `LIVE\n` (5 bytes including the trailing LF)
- **Content-Type:** `text/plain` (exact)
- **Content-Length:** `5` (exact)
- **Server:** `envoy` (exact — per ADR-0014)
- **Cache-Control:** <value copied verbatim from the evidence file>
- **Date:** present on both proxies; value is allow-listed (see header allow-list below)
- **Other headers observed upstream:** <enumerated from the evidence file; each either emitted byte-exact by envoy-go or allow-listed below>

### Pre-init response

Per ADR-0015, envoy-go's admin `/ready` matches upstream's pre-init response
byte-exact if the evidence captured one, OR declares the pre-init window
unobservable by the phase-01 differential test if the evidence file did not
capture a non-ready response. The current declaration (filled in at Task 7):
<one of: "byte-exact to evidence" or "unobservable — see evidence file">.

### Applies to

- Phase-01 envoy-go `admin` subsystem.
- Ready-state responses only (and pre-init only if the ADR chose byte-exact matching).

### Does not yet apply to

- HTTP/2 over admin (phase 01 is HTTP/1.1 only).
- Admin endpoints other than `/ready` (phase 08).
```

- [ ] **Step 3: Extend the existing `## Header allow-list` section**

Add entries for `Date` (on `/ready`, value non-deterministic, introduced by phase 01, ADR-0015) and for any header the evidence file captured upstream but envoy-go does not emit (e.g. an `X-Envoy-*` header that is value-allowed-to-differ). Use the existing stub's shape: `{name, permitted divergence, introducing phase, justifying ADR}`.

- [ ] **Step 4: Verify docs build / markdown lint**

No markdown linter is wired in CI (per phase-00 baseline). Eyeball the file for consistent H2 levels and ensure the new section is reachable via the repo's `docs/envoy-go/` table-of-contents expectations.

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: BEHAVIOR_CONTRACT Admin API subsection (SPEC §5.5)"
```

---

## Task 11: `test/helpers/http_response.go` — HTTP response parser

**Files:**
- Create: `test/helpers/http_response.go`
- Create: `test/helpers/http_response_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0019)

- [ ] **Step 1: Append ADR-0019**

Decision: admin response parser lives at `test/helpers/http_response.go`. Rationale: anticipated reuse by fixtures 0002+ probing HTTP surfaces; colocated with `test/helpers/tcp.go`.

- [ ] **Step 2: Write the failing tests**

```go
package helpers

import (
	"strings"
	"testing"
)

func TestParseHTTPResponse_Simple(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\nServer: envoy\r\n\r\nLIVE\n")
	r, err := ParseHTTPResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.StatusLine != "HTTP/1.1 200 OK" {
		t.Errorf("status: %q", r.StatusLine)
	}
	if string(r.Body) != "LIVE\n" {
		t.Errorf("body: %q", r.Body)
	}
	if r.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type: %q", r.Headers["Content-Type"])
	}
	if r.Headers["Server"] != "envoy" {
		t.Errorf("Server: %q", r.Headers["Server"])
	}
}

func TestParseHTTPResponse_MultiValueHeader(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nX-A: 1\r\nX-A: 2\r\n\r\n")
	r, err := ParseHTTPResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(r.Headers["X-A"], "1") || !strings.Contains(r.Headers["X-A"], "2") {
		t.Errorf("multi-value: %q", r.Headers["X-A"])
	}
}

func TestParseHTTPResponse_Malformed(t *testing.T) {
	_, err := ParseHTTPResponse([]byte("not an http response"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
```

- [ ] **Step 3: Run — fail (undefined)**

Run: `go test ./test/helpers/ -run TestParseHTTPResponse -v`
Expected: FAIL (compile).

- [ ] **Step 4: Implement `ParseHTTPResponse`**

```go
package helpers

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/textproto"
)

// HTTPResponse is the structured form of an HTTP/1.1 response on the wire.
// The dimension-aware differential diff compares each field per fixture's
// expectations.yaml.
type HTTPResponse struct {
	StatusLine string
	Headers    map[string]string // canonical name → joined value; multi-value joined by ", " per textproto
	Body       []byte
}

// ParseHTTPResponse parses raw HTTP/1.1 response bytes (status line + headers
// + body) into a structured form. Uses net/http for status+headers and then
// reads the body to EOF. Errors on malformed status line.
func ParseHTTPResponse(raw []byte) (*HTTPResponse, error) {
	br := bufio.NewReader(bytes.NewReader(raw))
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return nil, fmt.Errorf("http_response: parse: %w", err)
	}
	body := make([]byte, 0, len(raw))
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	_ = resp.Body.Close()
	// Reconstruct the status line (http.ReadResponse consumes it).
	status := fmt.Sprintf("%s %s", resp.Proto, resp.Status)
	hdrs := map[string]string{}
	for k, vs := range resp.Header {
		hdrs[textproto.CanonicalMIMEHeaderKey(k)] = joinHeaderValues(vs)
	}
	return &HTTPResponse{StatusLine: status, Headers: hdrs, Body: body}, nil
}

func joinHeaderValues(vs []string) string {
	if len(vs) == 1 {
		return vs[0]
	}
	out := ""
	for i, v := range vs {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
```

- [ ] **Step 5: Run — tests PASS**

Run: `go test ./test/helpers/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add test/helpers/http_response.go test/helpers/http_response_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: test/helpers/http_response.go parser [ADR-0019]"
```

---

## Task 12: Cutover — rewire cmd/envoy-go to bootstrap + admin; rewrite fixture envoy-go.yaml

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Modify: `cmd/envoy-go/main_test.go`
- Modify: `test/fixtures/0000-tcp-echo/envoy-go.yaml`
- Modify: `test/fixtures/0000-tcp-echo/driver/driver.go`
- Modify: `test/differential/fixture/fixture.go`
- Modify: `test/differential/harness.go`
- Modify: `test/differential/runner_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0020)

**Scope note:** this task is larger than typical because the cutover cannot be decomposed into smaller green-build commits — all callers of the phase-00 config contract must switch to the phase-01 bootstrap contract simultaneously for `go test ./...` and the differential run to stay green. `config.go` and `config_test.go` are kept in place here (they compile as unused code since `main.go` no longer imports from them); Task 13 deletes them. This is a deliberate choice to make the cutover reviewable — the deletion commit then has a purely mechanical diff.

- [ ] **Step 1: Append ADR-0020 to `DECISIONS.md`**

Decision: `cmd/envoy-go/main_test.go` is rewritten in place (same file, same test name, bootstrap-shaped YAML) rather than replaced with a subprocess-integration test. Rationale: keeps cmd-level unit coverage lightweight; differential suite provides the subprocess-integration dimension.

- [ ] **Step 2: Rewrite `cmd/envoy-go/main.go`**

Replace the file's contents with:
```go
// envoy-go is the phase-01 subject binary. It loads an Envoy v3 Bootstrap
// proto from YAML (internal/bootstrap), starts the admin /ready server
// (internal/admin), binds a TCP listener at static_resources.listeners[0], and
// bidirectionally io.Copy's bytes between each accepted connection and the
// single endpoint of static_resources.clusters[0]. Phase 00's minimal YAML
// schema is retired (see ADR-0021 superseding ADR-0007). Phase 02 retires
// this binary and replaces it with a real listener manager + TCP proxy filter
// + cluster manager.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml>")
		os.Exit(2)
	}
	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	bs, err := bootstrap.Load(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	adminHost, adminPort, err := bootstrap.AdminSocket(bs)
	if err != nil {
		log.Fatalf("extract admin: %v", err)
	}
	listenerHost, listenerPort, err := bootstrap.FirstListenerSocket(bs)
	if err != nil {
		log.Fatalf("extract listener: %v", err)
	}
	upstreamHost, upstreamPort, err := bootstrap.FirstClusterEndpointSocket(bs)
	if err != nil {
		log.Fatalf("extract cluster endpoint: %v", err)
	}

	adminAddr := fmt.Sprintf("%s:%d", adminHost, adminPort)
	listenAddr := fmt.Sprintf("%s:%d", listenerHost, listenerPort)
	upstreamAddr := fmt.Sprintf("%s:%d", upstreamHost, upstreamPort)

	admSrv := admin.New(adminAddr)
	// The harness records admin addr from the pre-allocated value threaded
	// through SubjectProxy, so main.go discards Start's bound-addr return —
	// preserving the phase-00 ready sentinel format verbatim keeps
	// harness.readyAddr's parse logic unchanged.
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	admSrv.MarkReady()

	// Ready sentinel — harness contract, byte-exact from phase 00. Format is
	// part of the harness contract; do not change without also updating
	// test/differential/harness.go:226 (readyAddr).
	_, _ = fmt.Fprintf(os.Stdout, "envoy-go ready on %s\n", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go pump(conn, upstreamAddr)
	}
}

// netConn wraps net.Conn and hides the *net.TCPConn type, preventing
// io.Copy from using the Linux splice(2) syscall optimisation. splice can
// return 0 bytes when the source socket has data+FIN already queued, causing
// silent data loss on loopback. Using a plain Read/Write loop via a 32 KiB
// heap buffer is fast enough for the phase-01 test workload. (Preserved
// verbatim from phase 00 — SPEC §5.3 requires the pump be untouched.)
type netConn struct{ net.Conn }

func pump(client net.Conn, upstreamAddr string) {
	defer func() { _ = client.Close() }()
	upstream, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		log.Printf("dial upstream %s: %v", upstreamAddr, err)
		return
	}
	defer func() { _ = upstream.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{client}); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{client}, netConn{upstream}); halfClose(client) }()
	wg.Wait()
}

func halfClose(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
```

*Ready-sentinel note:* the sentinel format is kept **byte-exact** to phase 00's `"envoy-go ready on <listener>\n"`. The harness's existing `readyAddr` in `test/differential/harness.go:226` does `strings.TrimRight(line[i+len(prefix):], "\r\n")` and would misparse the listener address if the sentinel grew an ` admin <addr>` suffix (it would store `"<listener> admin <addr>"` as the listener addr and subsequent `net.Dial` calls would fail). The admin address instead flows to the harness via the pre-allocated `subjAdminAddr` string threaded through `StartSubjectProxy` — see Step 7.

- [ ] **Step 3: Rewrite `cmd/envoy-go/main_test.go`**

Replace the embedded YAML block and allocate both ports:

Change the body of `TestEnvoyGoBinary_EchoesThroughUpstream`:
- After `listenerPort := freeTCPPort(t)`, add `adminPort := freeTCPPort(t)`.
- Replace the `cfg := fmt.Sprintf(\`...\`, listenerPort, backendAddr.Port)` block with:
```go
cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, adminPort, listenerPort, backendAddr.Port)
```
- Leave `waitForReady(t, stdout, fmt.Sprintf("127.0.0.1:%d", listenerPort), 5*time.Second)` unchanged — the sentinel format is byte-exact to phase 00 so the existing substring match (`strings.Contains(line, "envoy-go ready on "+expectAddr)`) still matches.

- [ ] **Step 4: Rewrite `test/fixtures/0000-tcp-echo/envoy-go.yaml`**

Replace the entire file with:
```yaml
# Subject bootstrap for fixture 0000-tcp-echo.
#
# Divergences from envoy.yaml (benign; each documented):
#  1. cluster.type = STATIC (vs reference STRICT_DNS): subject reaches the
#     backend via literal IP; no DNS resolver in phase 01 (SPEC §5.4 #1).
#  2. No dns_lookup_family: subject has no DNS resolver (SPEC §5.4 #2;
#     reference keeps V4_ONLY per ADR-0010).
#  3. Addresses are 127.0.0.1: subject runs as a host subprocess; no docker
#     bridge (SPEC §5.4 #3; reference uses 0.0.0.0 + host.docker.internal).
#
# The driver (driver/driver.go) substitutes port_value: 0 at test time.
node:
  id: envoy-go-subject-0000
  cluster: envoy-go-differential
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
```

The checked-in `port_value: 0`s are the template; the runner substitutes the allocated ports via `driver.SubjectConfig`, not via string replace on this file.

- [ ] **Step 5: Update `test/differential/fixture/fixture.go`**

Change the `Driver` interface:
```go
type Driver interface {
	Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)
	ReferenceBootstrap() string
	// SubjectConfig now takes four ports: reference listener (in-container),
	// subject listener (host), backend, subject admin (host).
	SubjectConfig(refListenerPort, subjListenerPort, backendPort, subjAdminPort int) string
	ReferenceListenerPort() int
}
```

The `ProbeAdmin` method is deliberately NOT added here — Task 14 adds it. Keeping the Driver interface extension split across Task 12 and Task 14 keeps each task's scope reviewable.

- [ ] **Step 6: Rewrite `test/fixtures/0000-tcp-echo/driver/driver.go`'s `SubjectConfig`**

Replace the phase-00 `fmt.Sprintf` with a four-port template:
```go
func (echoDriver) SubjectConfig(refListenerPort, subjListenerPort, backendPort, subjAdminPort int) string {
	_ = refListenerPort // phase 01 does not wire the reference listener port into the subject bootstrap
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0000, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, subjAdminPort, subjListenerPort, backendPort)
}
```

- [ ] **Step 7: Update `test/differential/harness.go` SubjectProxy + StartSubjectProxy**

In `test/differential/harness.go`:

After `type SubjectProxy struct { ... }` add field `adminAddr string`:
```go
type SubjectProxy struct {
	cmd          *exec.Cmd
	listenerAddr string
	adminAddr    string
	tmpDir       string
}
```

Change `StartSubjectProxy` signature and body to accept a pre-allocated admin addr and record it:
```go
func StartSubjectProxy(ctx context.Context, repoRoot, cfg, subjAdminAddr string) (*SubjectProxy, error) {
	// ... existing body ...
	// After parsing the ready sentinel into addr:
	return &SubjectProxy{cmd: cmd, listenerAddr: addr, adminAddr: subjAdminAddr, tmpDir: tmp}, nil
}
```

Add the getter:
```go
// AdminAddr returns the subject's admin host:port (pre-allocated by the caller
// and interpolated into the subject bootstrap).
func (s *SubjectProxy) AdminAddr() string { return s.adminAddr }
```

- [ ] **Step 8: Update `test/differential/runner_test.go`**

In `runFixture`:
- After `subjPort := freeTCPPort(t)` add `subjAdminPort := freeTCPPort(t)`.
- Change `subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPort)` to `subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPort, subjAdminPort)`.
- Change `subj, err := StartSubjectProxy(ctx, root, subjCfg)` to `subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))`.

No admin probe yet (Task 14 adds it). The existing `d.Drive + CompareBytes` on TCP bytes continues to be the sole assertion in this task's green state.

- [ ] **Step 9: Run the full test suite**

Run: `go vet ./... && golangci-lint run ./... && go test ./...`
Expected: PASS. `go test -short ./...` passes with the new embedded YAML in main_test.go. `go test ./test/differential/... -v` (without `-short`) still passes because the fixture's TCP echo observation is unchanged and the new admin port threading is inert.

- [ ] **Step 10: Commit**

```bash
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go test/fixtures/0000-tcp-echo/envoy-go.yaml test/fixtures/0000-tcp-echo/driver/driver.go test/differential/fixture/fixture.go test/differential/harness.go test/differential/runner_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: rewire subject binary to bootstrap.Load + admin.Server (cutover) [ADR-0020]"
```

---

## Task 13: Delete phase-00 `cmd/envoy-go/config.go` + `config_test.go`

**Files:**
- Delete: `cmd/envoy-go/config.go`
- Delete: `cmd/envoy-go/config_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0021)

- [ ] **Step 1: Append ADR-0021 to `DECISIONS.md`**

ADR-0021 explicitly names ADR-0007 as superseded (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4). Body: phase-01 `internal/bootstrap` loader replaces the phase-00 minimal YAML schema; `cmd/envoy-go/config.go` and `config_test.go` are deleted; ADR-0007 is not edited (append-only). Cross-references the ADR-0012 pipeline, ADR-0013 proto-type dep, and ADR-0016 unknown-field rule as the new contract.

- [ ] **Step 2: Delete the files**

Run:
```bash
git rm cmd/envoy-go/config.go cmd/envoy-go/config_test.go
```

- [ ] **Step 3: Run full lint/vet/test**

Run: `go vet ./... && golangci-lint run ./... && go test ./...`
Expected: PASS. Nothing imports from the deleted files after Task 12.

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: delete phase-00 minimal config schema files [ADR-0021]"
```

(The `git rm` from Step 2 staged the deletions already; the `git add` here picks up the ADR + PROGRESS changes.)

---

## Task 14: Admin probe wiring — Driver.ProbeAdmin + runner diff

**Files:**
- Modify: `test/differential/fixture/fixture.go`
- Modify: `test/fixtures/0000-tcp-echo/driver/driver.go`
- Modify: `test/differential/runner_test.go`

- [ ] **Step 1: Extend the `Driver` interface**

In `test/differential/fixture/fixture.go`, add:
```go
// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes (status line + headers + body) for the
// differential diff. Introduced in phase 01; see SPEC §5.6.
ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error)
```

- [ ] **Step 2: Implement `ProbeAdmin` on `echoDriver`**

In `test/fixtures/0000-tcp-echo/driver/driver.go`, add:
```go
func (echoDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = probeReady(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = probeReady(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// probeReady issues a raw-socket GET /ready and reads the full wire response.
// Not using net/http.Client because the diff needs the status line and
// headers as on-the-wire bytes (net/http's response object discards some wire
// detail like header ordering that the diff's set-equal allow-list tolerates
// but the body/status exact-match rule does not).
func probeReady(ctx context.Context, addr string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	req := "GET /ready HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, rerr := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return buf, nil
}
```

Add imports to the driver file: `net`, `time`. The existing imports already include `context`.

- [ ] **Step 3: Wire the admin diff into `runner_test.go`**

In `runFixture`, after the existing `CompareBytes(refBytes, subjBytes)` block, add a second observation:
```go
// 5. Admin /ready observation (phase 01 addition — SPEC §5.6).
refAdm, subjAdm, err := d.ProbeAdmin(ctx, ref.AdminAddr(), subj.AdminAddr())
if err != nil {
	t.Fatalf("admin probe: %v", err)
}
vAdm, err := compareAdminResponses(refAdm, subjAdm, d)
if err != nil {
	t.Fatalf("admin compare: %v", err)
}
if !vAdm.Equal {
	t.Errorf("admin differential mismatch:\n%s", vAdm.HexDump)
}
```

Add a local helper `compareAdminResponses` in the same file that:
1. Calls `helpers.ParseHTTPResponse` on both byte streams.
2. Asserts status lines are equal (exact).
3. Asserts bodies are byte-exact equal.
4. Asserts the header set is set-equal modulo an allow-list. The allow-list is hardcoded in this helper for phase 01 (`Date` is the only allow-listed header per BEHAVIOR_CONTRACT — plus any additional headers the Task 7 evidence file captured as non-deterministic); fixtures 0002+ will read this from `expectations.yaml` once dimension-aware diff is productized.
5. Returns a `Verdict` shaped like `CompareBytes`'s output, with `HexDump` populated when unequal.

Implementation sketch (append to `runner_test.go` private section):
```go
func compareAdminResponses(refRaw, subjRaw []byte, _ FixtureDriver) (Verdict, error) {
	refResp, err := helpers.ParseHTTPResponse(refRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("ref parse: %w", err)
	}
	subjResp, err := helpers.ParseHTTPResponse(subjRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("subj parse: %w", err)
	}
	// Status line: exact.
	if refResp.StatusLine != subjResp.StatusLine {
		return Verdict{Equal: false, HexDump: fmt.Sprintf("status: ref=%q subj=%q", refResp.StatusLine, subjResp.StatusLine)}, nil
	}
	// Body: byte-exact.
	bv, err := CompareBytes(refResp.Body, subjResp.Body)
	if err != nil {
		return Verdict{}, err
	}
	if !bv.Equal {
		return bv, nil
	}
	// Headers: set-equal modulo allow-list.
	allowList := map[string]struct{}{"Date": {}} // extend per Task 7 evidence
	mismatch := diffHeaders(refResp.Headers, subjResp.Headers, allowList)
	if mismatch != "" {
		return Verdict{Equal: false, HexDump: mismatch}, nil
	}
	return Verdict{Equal: true}, nil
}

func diffHeaders(ref, subj map[string]string, allow map[string]struct{}) string {
	// For each header in ref: if not in allow, require subj has it with equal value.
	var sb strings.Builder
	for k, v := range ref {
		if _, a := allow[k]; a {
			// presence-only (or value-allow); require present in subj.
			if _, ok := subj[k]; !ok {
				fmt.Fprintf(&sb, "header %q: present in ref, absent in subj\n", k)
			}
			continue
		}
		sv, ok := subj[k]
		if !ok {
			fmt.Fprintf(&sb, "header %q: absent in subj (ref=%q)\n", k, v)
			continue
		}
		if sv != v {
			fmt.Fprintf(&sb, "header %q: ref=%q subj=%q\n", k, v, sv)
		}
	}
	// Reverse: headers in subj but not ref (outside allow-list).
	for k, v := range subj {
		if _, a := allow[k]; a {
			continue
		}
		if _, ok := ref[k]; !ok {
			fmt.Fprintf(&sb, "header %q: absent in ref (subj=%q)\n", k, v)
		}
	}
	return sb.String()
}
```

Add imports to `runner_test.go` as needed. `strings` is already imported (used by phase-00 at line 61 for `strings.Replace`); new imports required by the snippets above are `fmt` (already imported) and `github.com/esalaine/envoy-go/test/helpers` (new). No import cycle risk: `helpers` has no dependency on `test/differential`.

- [ ] **Step 4: Run the differential suite**

Run: `go test ./test/differential/... -timeout 5m -v`
Expected: PASS. Subject's `/ready` bytes match reference byte-exact on body + status, set-equal on headers (modulo `Date` and any other Task 7 evidence-captured allow-list entries).

If the diff fails, read the hex dump in the test output and reconcile: either envoy-go is emitting a header upstream doesn't (remove from handler), upstream is emitting one envoy-go isn't (add to handler or extend allow-list), or a value differs. Record every reconciliation in PROGRESS.md.

- [ ] **Step 5: Run lint/vet/full test**

Run: `go vet ./... && golangci-lint run ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add test/differential/fixture/fixture.go test/fixtures/0000-tcp-echo/driver/driver.go test/differential/runner_test.go docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: admin /ready differential observation wired through runner (SPEC §5.6)"
```

---

## Task 15: Extend fixture `expectations.yaml`

**Files:**
- Modify: `test/fixtures/0000-tcp-echo/expectations.yaml`

- [ ] **Step 1: Rewrite the `dimensions:` block per SPEC §5.4**

Replace the file's `dimensions:` block with:
```yaml
dimensions:
  response-status:
    applicable: true
    rule: exact
    scope: admin-/ready
  response-body:
    applicable: true
    rule: byte-exact
    scope: tcp-echo + admin-/ready
  response-headers:
    applicable: true
    rule: set-equal-modulo-allow-list
    scope: admin-/ready
    allow-list: BEHAVIOR_CONTRACT.md § "Admin API — /ready"
  response-trailers:
    applicable: false
    reason: no trailers on /ready; no HTTP layer on TCP path
  http2-http3-framing:
    applicable: false
    reason: admin is HTTP/1.1; TCP has no framing
  access-log:
    applicable: false
    reason: phase 01 does not emit access logs (phase 06)
  stats:
    applicable: false
    reason: phase 01 does not emit stats (phase 06)
  xds:
    applicable: false
    reason: static config; no xDS
  timing:
    applicable: false
    reason: not opt-in
```

- [ ] **Step 2: Run the differential suite one more time**

Run: `go test ./test/differential/... -timeout 5m -v`
Expected: PASS. No code consumes `expectations.yaml` directly yet (ADR-0019 only anticipates dimension-aware diff productization later), but any future reader reflecting this file must see the phase-01 reality.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0000-tcp-echo/expectations.yaml docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: extend 0000-tcp-echo expectations.yaml with admin dimensions (SPEC §5.4)"
```

---

## Task 16: Update fixture `README.md`

**Files:**
- Modify: `test/fixtures/0000-tcp-echo/README.md`

- [ ] **Step 1: Rewrite the README to reflect phase-01 reality**

Replace the file's body with two short paragraphs (per SPEC §5.4 "two-paragraph update") preserving the existing `# 0000-tcp-echo` heading:

Paragraph 1 — what the fixture tests now (TCP echo through both proxies on their listeners; admin `/ready` byte-exact on both, body + status exact, headers set-equal modulo the BEHAVIOR_CONTRACT allow-list).

Paragraph 2 — the three benign divergences between `envoy.yaml` (reference) and `envoy-go.yaml` (subject): (1) cluster type STATIC vs STRICT_DNS, (2) no `dns_lookup_family`, (3) loopback vs `host.docker.internal`. Each with a one-line rationale linking SPEC §5.4 items 1–3.

Keep the `## Driver` and `## Expectations` sections; update `## Expectations` to point at the new `expectations.yaml` dimensions.

- [ ] **Step 2: Commit**

```bash
git add test/fixtures/0000-tcp-echo/README.md docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: refresh 0000-tcp-echo README for admin observation"
```

---

## Task 17: Green local CI equivalent

**Files:**
- None (verification-only; PROGRESS.md gets the evidence)

- [ ] **Step 1: Run `go vet`**

Run: `go vet ./...`
Expected: empty output, exit 0.

- [ ] **Step 2: Run `golangci-lint run`**

Run: `golangci-lint run ./...`
Expected: empty output, exit 0.

- [ ] **Step 3: Run `go test ./...`**

Run: `go test ./... -timeout 10m`
Expected: all packages green. (This includes `cmd/envoy-go` with the new bootstrap YAML, `internal/bootstrap`, `internal/admin`, `test/helpers`, `test/differential` — though under `-short` differential skips, `./...` without `-short` runs it.)

- [ ] **Step 4: Run the differential suite explicitly**

Run: `go test ./test/differential/... -timeout 5m -v`
Expected: `TestDifferential/0000-tcp-echo` green, both TCP and admin observations asserting equal.

- [ ] **Step 5: Run the fuzz target short-budget**

Run: `go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$`
Expected: PASS, zero crashes, fuzz engine output quoted in PROGRESS.md.

- [ ] **Step 6: Quote every command + output verbatim in PROGRESS.md**

Append a final Task 17 entry to PROGRESS.md with all five command outputs. This is the gate-evidence the verification session (state-machine step 4) will quote.

- [ ] **Step 7: Commit**

```bash
git add docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md
git commit -m "phase 01: Task 17 — green local gate sweep (lint/vet/test/differential/fuzz)"
```

---

## PROGRESS.md conventions

The executor creates `docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md` as part of Task 1's commit and appends to it on every subsequent task's commit. Format (follows the phase-00 convention exactly):

```markdown
# Phase 01 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Add go-control-plane + protojson direct deps

**Commits:** <sha>
**Notes:** <one paragraph: picked go-control-plane version, rationale>
**Outputs:**
```
$ go mod tidy
<verbatim output>
$ go build ./...
<verbatim>
$ go vet ./... && golangci-lint run ./...
<verbatim>
```

## Task 2 — bootstrap.Load happy path + dynamic_resources rejection

**Commits:** <sha>
**Notes:** <one paragraph>
**Outputs:** <verbatim>

...etc...
```

PROGRESS.md is created in Task 1 and is part of every subsequent task's commit. Task header format: `## Task N — <short title>` (em-dash, not colon). Commits field is one SHA or a comma-separated list if a task hit a hook-fail-and-retry. Outputs are quoted verbatim — no summarization.

---

## Out-of-scope for this plan

These are explicitly NOT part of phase 01's plan and must not be added during execution. Each is deferred per SPEC §2 or §9:

- Any listener-manager logic beyond reading `static_resources.listeners[0]`'s socket address. No multi-listener, no filter-chain dispatch, no per-listener lifecycle. → phase 02.
- Any cluster-manager logic beyond reading `static_resources.clusters[0]`'s first endpoint's socket address. No load balancing, no DNS resolver, no cluster-type dispatch. → phase 02 + xDS family.
- Resolving `typed_config` Any bytes in filter chains. The loader parses them as `Any` and preserves the bytes untouched. → phase 02 (TCP proxy) / phase 07 (framework).
- TLS, HTTP/1.1 proxy layer, HTTP/2, HTTP/3. → phases 03–05.
- Access log, stats, Prometheus integration. → phase 06.
- Admin endpoints other than `/ready`. → phase 08.
- `dynamic_resources`, `layered_runtime`, xDS. Presence at load time is a rejection. → xDS and runtime families.
- Pre-initialization state machine beyond the phase-01 ADR-0015 decision. → later (xDS or filter-chain framework, whichever comes first).
- `envoy-go.yaml` ↔ `envoy.yaml` unification. → deferred; no phase target yet.
- Graceful drain / SIGTERM handling. → phase 08.
- Second fixture, conformance driver scaffolding, any non-bootstrap fuzzer.
- Any dependency outside the D-3.2 permitted-foundations list. `github.com/envoyproxy/go-control-plane` is imported as proto types only.

If reality during implementation pushes toward any of these, invoke `superpowers:systematic-debugging` and either re-scope the offending task in-place or initiate a §6 split per `BOOTSTRAP_PROMPT.md`.

---

## Exit criteria for this PLAN's executor (state-machine step 4 inputs)

When all 17 tasks are complete, the next session (running `superpowers:verification-before-completion` per ADR-0005 §4) verifies:

1. All SPEC §3 phase-done gates green: `(a)` differential fixture `0000-tcp-echo` green on both TCP echo and admin `/ready` byte-exact; `(b)` phase-00 TCP echo surface unchanged and green; `(c)` N/A (phase 01 declares threshold 0); `(d)` `FuzzBootstrapLoad` clean at the ADR-0018 budget; `(e)` `go vet`, `golangci-lint run`, `go test ./...` all green locally and on CI; `(f)` `REVIEW.md` approved (later step).
2. `docs/envoy-go/BEHAVIOR_CONTRACT.md` contains a populated `## Admin API — /ready` section (not a `_to be filled_` placeholder) with the `/ready` rule and a justifying ADR reference (ADR-0014, ADR-0015).
3. `docs/envoy-go/DECISIONS.md` contains ADR-0012 through ADR-0021 — ten new ADRs — with ADR-0021 explicitly superseding ADR-0007.
4. `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md` exists and contains verbatim evidence bytes from Task 7.
5. `docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md` quotes all 17 tasks' command outputs verbatim.
6. `cmd/envoy-go/config.go` and `cmd/envoy-go/config_test.go` do not exist in the tree (`git ls-files | grep cmd/envoy-go/config` returns empty).
7. `STATE.md` will be advanced to `lifecycle-state: 4` (verification) at the executor's session-exit, with `next-skill: superpowers:verification-before-completion`.

The plan-authoring session (this session) exits at `lifecycle-state: 3` per ADR-0005 §1, with `next-skill: superpowers:subagent-driven-development`.

---

*End of PLAN.*
