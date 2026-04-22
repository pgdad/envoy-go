# Phase 00 — Bootstrap

**Phase id:** `00`
**Slug:** `00-bootstrap`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** none (this phase creates the preconditions for every subsequent phase)
**Differential surface at end of phase:** harness boots; one TCP echo fixture green.

---

## 1. Purpose

Phase 00 exists to create the scaffolding that every subsequent phase relies on:

1. A runnable envoy-go binary (minimal — just enough to produce one green differential fixture).
2. A differential test harness (`test/differential/`) that drives upstream Envoy (reference) and envoy-go (subject) against identical inputs and compares outputs per `docs/envoy-go/BEHAVIOR_CONTRACT.md`.
3. A pinned upstream Envoy Docker image (`docs/envoy-go/ENVOY_TARGET.md`), which every fixture references.
4. One trivial fixture (`test/fixtures/0000-tcp-echo/`) that the harness can run green, end-to-end, against both proxies.
5. Continuous-integration wiring (GitHub Actions) that runs `go build`, `go vet`, `golangci-lint run`, `go test ./...`, and the differential echo fixture on every push.
6. The repository layout from `BOOTSTRAP_PROMPT.md` §4, populated with the minimal set of packages and placeholder `doc.go` files so each package is importable.

After phase 00, the project has proven its central engineering claim: *we can assert differential equivalence against upstream Envoy in CI.* Every later phase works inside that claim.

## 2. Non-purposes

- Phase 00 does **not** implement a real listener manager, cluster manager, filter chain engine, or router. Those are phases 02, 07, and later.
- Phase 00 does **not** handle TLS, HTTP/1.1, HTTP/2, or HTTP/3. TCP only.
- Phase 00 does **not** produce a production-grade envoy-go binary. Its subject binary is explicitly a placeholder (see §5.2).
- Phase 00 does **not** implement stats, access log, admin API, or xDS. Those come in later phases. The harness captures access logs and stats if the subject emits them, but the echo fixture does not require either dimension to be green.
- Phase 00 does **not** run conformance suites (`h2spec`, `h3spec`, `grpc-conformance`, `proxy-wasm conformance`) — none of their protocol surfaces are implemented yet. Their *drivers* are not scaffolded here; they land in the phases that first need them.
- Phase 00 does **not** implement fuzzers. The first fuzzer ships in the phase that introduces the first parser/codec (phase 01 for bootstrap config, or phase 04 for HTTP/1.1).

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 00 lands only when every gate below is green. The generic §7.5 gate set is narrowed here:

| Gate | Specialization for phase 00 |
|---|---|
| (a) new/changed differential fixtures green | `test/fixtures/0000-tcp-echo/` passes the differential diff per §5.1 step 6 against expectations declared in §5.3 |
| (b) pre-existing differential fixtures green | N/A — this is the first fixture |
| (c) conformance suites at declared threshold | N/A — phase 00 declares threshold 0 (no suites apply yet); `SKILL_ROUTING.md` entry §7.5(c) is satisfied vacuously |
| (d) new fuzzer clean short-budget run | N/A — phase 00 introduces no parser/codec; first fuzzer lands in a later phase |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | All three in CI and locally |
| (f) `REVIEW.md` approved | Per §5 step 5 of the state machine |

Additional phase-specific exit criteria:

- `docs/envoy-go/ENVOY_TARGET.md` contains a concrete Docker tag and SHA256 digest (not a placeholder).
- `go.mod` exists and pins Go 1.23 (or a later minor version available when the phase lands).
- The CI pipeline file exists and has been run at least once green on the phase-00 implementation branch (per ADR-0003, each phase runs on its own worktree branch; the implementation branch succeeds the `bootstrap` branch that produced this SPEC).

## 4. Deliverables (files and directories)

Paths are relative to repo root. Every path listed here is created or populated by this phase; no other production paths are created.

```
envoy-go/
├── go.mod                              # module path: github.com/envoyproxy/envoy-go (tentative — see §8 ADR-deferred-decisions)
├── go.sum
├── .github/
│   └── workflows/
│       └── ci.yml                      # lint + vet + test + differential echo fixture, on push and pull_request
├── .golangci.yml                       # lint config (see §5.5)
├── cmd/
│   └── envoy-go/
│       └── main.go                     # minimal binary — see §5.2
├── internal/
│   ├── bootstrap/doc.go                # placeholder; real loader lands in phase 01
│   ├── listener/doc.go                 # placeholder; real manager lands in phase 02
│   ├── cluster/doc.go                  # placeholder; real manager lands in phase 02
│   ├── tcp/doc.go                      # placeholder; real proxy filter lands in phase 02
│   ├── http/doc.go                     # placeholder; lands in phase 04
│   ├── tls/doc.go                      # placeholder; lands in phase 03
│   ├── filter/doc.go                   # placeholder; lands in phase 07
│   ├── xds/doc.go                      # placeholder; lands in xDS family
│   ├── admin/doc.go                    # placeholder; lands in phase 08
│   ├── stats/doc.go                    # placeholder; lands in phase 06
│   ├── accesslog/doc.go                # placeholder; lands in phase 06
│   └── runtime/doc.go                  # placeholder; lands in runtime family
├── pkg/                                # empty in phase 00; added when a stable API surface appears
├── test/
│   ├── differential/
│   │   ├── harness.go                  # starts reference + subject, drives inputs, captures outputs
│   │   ├── diff.go                     # equivalence checks per BEHAVIOR_CONTRACT.md
│   │   ├── runner_test.go              # Go test entry: iterates test/fixtures/NNNN-*, runs each as a subtest
│   │   └── doc.go
│   ├── conformance/doc.go              # empty directory with doc.go; populated by later phases
│   ├── fixtures/
│   │   └── 0000-tcp-echo/
│   │       ├── README.md               # purpose, invariants
│   │       ├── envoy.yaml              # reference config (upstream Envoy)
│   │       ├── envoy-go.yaml           # subject config (same shape, minimal)
│   │       ├── expectations.yaml       # allow-lists, ignore-lists, tolerances
│   │       └── driver/                 # small Go package that sends the test payload
│   └── helpers/
│       ├── tcp.go                      # TCP client helper used by fixtures
│       └── doc.go
└── docs/envoy-go/
    ├── ENVOY_TARGET.md                 # UPDATED by this phase with real tag + SHA256
    └── phases/00-bootstrap/
        ├── SPEC.md                     # this file
        ├── PLAN.md                     # produced in the next session
        ├── PROGRESS.md                 # produced during implementation
        └── REVIEW.md                   # produced at review
```

Directories beyond phase 00's scope (e.g. `internal/http/server/`, `test/conformance/h2spec/`) are **not** created here. Each phase that introduces a new subsystem creates its own packages.

## 5. Architecture and components

### 5.1 Differential harness (`test/differential/`)

The harness is a Go test binary. `go test ./test/differential/...` is the single entry point. It discovers fixtures under `test/fixtures/NNNN-*`, runs each as a `t.Run` subtest, and per fixture performs:

1. Resolve the Envoy pin from `docs/envoy-go/ENVOY_TARGET.md` (parsed at test startup).
2. Start the reference proxy: pull the pinned Envoy image, run via `testcontainers-go`, mount `envoy.yaml`, wait for admin `/ready` to return 200.
3. Start the subject proxy: build envoy-go via `go build ./cmd/envoy-go`, run as a subprocess, pass `-c envoy-go.yaml`, wait for it to print a ready sentinel to stdout (phase 00's stand-in for admin `/ready`).
4. Execute the fixture driver. The driver is a Go function under `test/fixtures/NNNN-*/driver/` that knows how to exercise its fixture. For TCP fixtures, the driver opens two TCP connections (one to each proxy's listener port), sends identical payloads, and reads responses.
5. Capture dimensions relevant to this fixture. For phase 00's echo fixture: response bytes only (see §5.3's expectations bullet, which declares every other §7.2 dimension `not-applicable` with a reason). Access logs and stats are **not** captured in phase 00 because neither proxy emits them under the bootstrap config used by the echo fixture — and the subject doesn't support them at all.
6. Run `diff.Compare(ref, subj, expectations.yaml)` to produce an equivalence verdict per `BEHAVIOR_CONTRACT.md` §7.2.
7. Tear down both proxies deterministically.

The harness exits cleanly on skip (e.g., Docker daemon unreachable in CI): it emits `t.Skipf` with a machine-readable reason, and CI treats the skip as a failure. Locally, developers may `go test -short` to skip differential tests while iterating on unit code.

### 5.2 Subject binary (`cmd/envoy-go/main.go`)

Phase 00's binary is intentionally minimal. It implements **only** what the echo fixture needs:

- Parse `-c <path>` and load a YAML file with a narrow schema: one listener (address + port) and one upstream (address + port). The YAML schema is *not* Envoy's bootstrap proto yet — that lands in phase 01 via `internal/bootstrap`. Phase 00 uses an envoy-go-local minimal schema defined inline in `main.go`. `envoy-go.yaml` in the fixture uses this minimal schema; `envoy.yaml` uses real Envoy bootstrap.
- Bind a TCP listener on the listener address.
- For each accepted connection, dial the upstream address and bidirectionally pump bytes (`io.Copy` in both directions until one side closes).
- Print `envoy-go ready on <addr>` to stdout when the listener is bound, so the harness can detect readiness.
- No filter chain. No listener manager. No cluster manager. No admin API. No stats. No access log. No TLS. No HTTP awareness. Just a TCP pump.

This binary is a placeholder. Phase 02 replaces it with a real listener manager + TCP proxy filter + cluster manager. A future ADR landed in phase 02 (number assigned at landing time per §4.1 invariant 4) will formally retire this placeholder.

### 5.3 Echo fixture (`test/fixtures/0000-tcp-echo/`)

- **Backend:** the fixture driver starts a Go goroutine echo server on a random free TCP port.
- **Reference config (`envoy.yaml`):** a minimal Envoy bootstrap with one static listener, one static cluster (the echo backend), one tcp_proxy network filter routing listener→cluster. Admin runs on a separate port.
- **Subject config (`envoy-go.yaml`):** envoy-go's minimal schema with `listener.address`, `listener.port`, `upstream.address`, `upstream.port` pointing to the same backend.
- **Driver:** opens a TCP connection to each proxy's listener, sends `[]byte("ping-<N>-<uuid>\n")` for N ∈ {0…9}, reads back until EOF or 1s idle timeout, records the byte stream.
- **Expectations (`expectations.yaml`):** equivalence dimensions exercised by this fixture:
  - `response-body`: byte-exact.
  - All other dimensions from §7.2: declared `not-applicable` with a one-line reason each (no HTTP status; no response headers; no framing; no access log; no stats; no xDS; no timing gate).
- **Verdict:** the diff compares byte streams. Any mismatch is a fixture failure.

### 5.4 CI (`.github/workflows/ci.yml`)

Runs on push to any branch and on every pull_request.

Jobs (single Linux runner, latest `ubuntu-*` available in GitHub Actions):

1. `lint-vet-test`:
   - Install Go 1.23.
   - `go mod download`.
   - `go vet ./...`.
   - `golangci-lint run` (using `.golangci.yml`).
   - `go test -short ./...` — runs unit tests, skips differential.
2. `differential`:
   - Depends on `lint-vet-test`.
   - Install Go 1.23, Docker.
   - `go test ./test/differential/...` — runs the echo fixture against the pinned upstream Envoy image.

Both jobs must pass for the phase to land. `differential` is the gate that demonstrates phase 00's core claim.

### 5.5 Lint config (`.golangci.yml`)

A strict-but-practical baseline. Enabled linters (chosen to catch correctness and style issues without fighting idiomatic Go):

- `govet`, `errcheck`, `staticcheck`, `unused`, `ineffassign`, `gofmt`, `goimports`, `misspell`, `revive`.

Disabled by default: none of the opinionated style linters (`gochecknoglobals`, `wsl`, `nlreturn`) until the project has opinions to encode in `DECISIONS.md`.

### 5.6 Envoy version pin (`docs/envoy-go/ENVOY_TARGET.md`)

The pin is resolved during phase 00 implementation — not at SPEC time — because it requires a `docker pull` to capture the SHA256 digest of the image. Selection criteria the planner applies during implementation:

1. Must be a released stable tag on Docker Hub `envoyproxy/envoy` (not `envoyproxy/envoy-dev`).
2. Must be current within the last 6 months as of the phase-landing date.
3. Must expose admin and tcp_proxy on their documented names for that major version (no API transition in flight).
4. The pin records: tag, SHA256 digest, upstream release notes URL, Envoy proto major version (expected: v3), and a one-paragraph refresh procedure (re-pull, capture new SHA256, re-run all differential fixtures, ADR the change per doctrine `D-3.7`).

The implementation task for this pin includes writing the refresh procedure; it is not a future TODO.

## 6. Data flow (end-to-end for the echo fixture)

```
     ┌────────────┐ payload                 ┌───────────────────┐
     │  driver    │ ──────────────────────► │ upstream Envoy     │ ──► echo backend
     │ (go test)  │                         │ (testcontainers)   │ ◄── responds
     └────────────┘ ◄─────── response ───── └───────────────────┘
           │
           │ same payload
           ▼
     ┌────────────┐                         ┌───────────────────┐
     │  driver    │ ──────────────────────► │    envoy-go        │ ──► same echo backend
     │ (go test)  │                         │ (subprocess)       │ ◄── responds
     └────────────┘ ◄─────── response ───── └───────────────────┘
           │
           │ compare
           ▼
     ┌────────────┐
     │    diff    │  byte-exact per expectations.yaml
     └────────────┘
```

Both proxies terminate TCP connections, dial the same backend, and pump bytes. The driver owns timing: it sends payload chunks, waits for idle, closes its send side, reads until EOF from the proxy. Because the backend is a simple echo goroutine and both proxies are single-connection pumps for phase 00, responses are deterministic and byte-exact.

## 7. Error handling and failure modes

The harness treats these as failures and reports them cleanly (no flaky retries, no best-effort ignores):

| Failure | Harness response |
|---|---|
| Docker daemon unreachable | `t.Fatalf` with "docker unavailable: %v". CI treats as a red run. No auto-skip. |
| Envoy image pull fails | `t.Fatalf` with tag + SHA mismatch context. |
| Reference or subject fails readiness check within timeout (30s) | Capture last stdout/stderr, `t.Fatalf`. |
| Backend echo server fails to bind | `t.Fatalf`. |
| Byte-stream divergence | `t.Errorf` with hex-dump of first divergent window; fixture fails. |
| Either proxy crashes during test | `t.Errorf` with crash exit code and captured stderr. |

Envoy-go's own error handling in phase 00 is minimal by design: the binary is a proof-of-concept pump. Errors encountered during accept/dial/copy are logged to stderr and the affected connection is dropped; the listener keeps accepting. A later phase (04 at earliest) introduces structured error handling; phase 00 deliberately does not.

## 8. Testing scope for phase 00

Phase 00 produces:

- One unit-test file `harness_test.go` covering harness helpers (container wait, subject readiness detection, diff helpers) with mocked containers.
- One integration/differential test `runner_test.go` that discovers fixtures and runs each. In phase 00, only the echo fixture exists.
- Zero fuzzers (no parser/codec).
- Zero conformance drivers.

The differential test body gates on `testing.Short()` (at test-function entry, calling `t.Skip("skipping differential test in -short mode")`) so `go test -short ./...` skips the differential path. Unit tests do not gate on `-short` and are always in scope.

## 9. Out-of-scope (explicitly deferred)

The following are called out so phase 00 does not accidentally grow them. Each lands in a future phase:

| Feature | Lands in |
|---|---|
| Envoy bootstrap proto parsing | Phase 01 |
| Static config → listener/cluster wiring (real) | Phase 02 |
| Round-robin LB | Phase 02 |
| Downstream/upstream TLS | Phase 03 |
| HTTP/1.1, routes, router filter | Phase 04 |
| HTTP/2 | Phase 05 |
| Access log, stats, Prometheus endpoint | Phase 06 |
| Filter chain engine | Phase 07 |
| Admin API, drain | Phase 08 |
| xDS | xDS family |
| Conformance suites | Respective protocol phases |
| Fuzzers | First phase with a parser |

If reality during implementation pushes phase 00 toward any of these, the planner must either (a) re-scope the fixture or the subject binary to stay within phase 00 (and write an ADR explaining the choice), or (b) split (§6) phase 00 into sub-phases `00.1`, `00.2`, …

## 10. Deferred decisions (the planner / implementer settles these)

These decisions are intentionally not made at SPEC time. The PLAN (next session) makes them, records each as an ADR where they have cross-phase impact, and proceeds.

1. **Module path.** Proposed `github.com/envoyproxy/envoy-go`. If that path is unusable or the project's owner prefers a different origin, the planner picks and ADRs.
2. **Exact Envoy version pin.** Chosen per §5.6 selection criteria; must include both tag and SHA256. ADR-pinned.
3. **Go version.** Floor is Go 1.23. If a later toolchain is installed on CI runners by default, the planner may raise the floor; the choice is ADRd if it goes beyond 1.23.
4. **`.golangci.yml` exact lint set.** §5.5 lists the baseline. The planner may add linters that catch classes of issues likely to bite early (e.g., `gosec` for anything that shells out) but must justify any removals from the baseline via ADR.
5. **CI runner choice.** Default is `ubuntu-latest` (GitHub Actions). If the pinned Envoy image has platform requirements not satisfied there, the planner ADRs an alternative (pinned runner image, self-hosted).
6. **Minimal YAML schema for `envoy-go.yaml` in phase 00.** §5.2 sketches the fields; the exact field names and types are the planner's call and will be superseded by phase 01's real bootstrap loader.

None of these decisions require human consultation (per doctrine `D-3.5`). Each is a standard engineering call the planner resolves and records.

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Docker not available in some CI environments | Stick with GitHub Actions where Docker is standard. ADR if changed. |
| Envoy image tag drifts during phase 00's lifetime | Pin by SHA256 from day one. |
| Phase 00's minimal subject binary becomes a maintenance burden | Deliberately time-boxed: phase 02 retires it. The retirement ADR (number assigned at landing time) formalizes the cutover. |
| Harness flakiness on slow CI (timeouts) | Readiness timeouts are generous (30s) and failures are surfaced, not retried. Use `testcontainers-go`'s `wait.ForListeningPort` where applicable. |
| Fixture coverage creep (temptation to add more fixtures "while we're here") | One fixture in phase 00. Additional fixtures are their own phases' responsibility. |
| Envoy-go's tests depend on network for `docker pull` | CI runners have network. Local `go test -short` skips the differential path, so offline development is unaffected. |

## 12. Acceptance checklist (for the reviewer of this phase's final state)

When phase 00's REVIEW is written, the reviewer confirms:

- [ ] All §4 paths exist with the described contents.
- [ ] `docs/envoy-go/ENVOY_TARGET.md` is populated (tag + SHA256, not placeholder).
- [ ] `go vet ./...`, `golangci-lint run`, `go test ./...`, and `go test ./test/differential/...` all run green locally and on CI.
- [ ] The echo fixture's `expectations.yaml` enumerates each §7.2 dimension explicitly (applicable or not-applicable with reason).
- [ ] The CI workflow file runs on push and pull_request and enforces both jobs.
- [ ] ADRs for every deferred decision in §10 are landed in `DECISIONS.md`.
- [ ] `STATE.md` advances to phase 01 and `ROADMAP.md` row 00 is set to `done`.
- [ ] `PROGRESS.md` contains a full log and the gate outputs from §3 quoted verbatim (per state machine §4).
- [ ] `REVIEW.md` is approved.
