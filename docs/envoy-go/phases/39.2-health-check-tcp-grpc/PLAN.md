# Phase 39.2 Active Health Checks (TCP + gRPC) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the TCP (`tcp_health_check`, connect-only) + gRPC (`grpc_health_check`, `grpc.health.v1.Health/Check` over H2) active-HC checker codecs over the UNCHANGED 39.1 substrate — a codec-only widening that generalizes the HTTP-only checker into a kind-tagged `prober` dispatch and completes the HTTP+TCP+gRPC checker roster.

**Architecture:** A small `prober` interface (`probe(addr string) (ok, networkFailure bool)`) with three implementations — `httpProber` (the 39.1 `probeHTTP` body, behavior-unchanged), `tcpProber` (a `net.Dialer{Timeout}.Dial("tcp", addr)` + immediate close), `grpcProber` (a unary `grpc.health.v1.Health/Check` reading the `ServingStatus`; a transport error ⇒ network_failure, a returned non-SERVING status ⇒ failure-only). `parseHealthChecks` returns a kind-tagged `[]checkerSpec`; `healthChecker` holds the timing/threshold envelope + a `prober` instead of `cfg httpHealthCheckCfg`. The `clusterHealth`/`hostHealth` registry, the `recordResult` transitions, `StartHealthChecks`/`Drain`, the +7 stats, the panic threshold, and the build-time-injected six-LB health view are REUSED VERBATIM. The only new wiring is the gRPC cluster-must-be-H2 config-build reject (reusing the existing `extractH2Mode`). Two cross-side fixtures (`0067` TCP reusing `HTTPEcho`; `0068` gRPC with a +1 `GRPCHealthResponder` BackendKind) follow the 39.1 `0066` poll-to-converge + warmup-gate + delta-counter protocol.

**Tech Stack:** Go; `internal/cluster` (`health.go` generalized + `manager.go` the H2 reject); `google.golang.org/grpc` + `google.golang.org/grpc/health/grpc_health_v1` (ALREADY a direct dep — zero new module); `golang.org/x/net/http2`/`h2c` (the `0068` backend, already in the module graph); the differential harness against `envoyproxy/envoy:contrib-v1.37.2`.

---

## Pre-flight — ADR-0045 split-gate re-check + D-S39.2 resolutions

### ADR-0045 split-gate FINAL re-check (per SPEC §3.0 / §11.6)

The 39.2 envelope: the `prober` interface + the dispatch generalization of `healthChecker`/`parseHealthChecks` (~50–80 LoC; `httpProber` is a body-move, not new code) + `tcpProber` + its parse arm + the `send`/`receive` reject (~30–50) + `grpcProber` + its parse arm (~50–90) + the gRPC-must-be-H2 reject reusing `extractH2Mode` (~20–40) ≈ **~150–260 prod LoC / 13 tasks** (the `GRPCHealthResponder` backend + the `0067`/`0068` drivers are test-side, not counted). Both axes are well UNDER the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`). **Decision: NO FURTHER SPLIT** — 39.2 lands as a single flat leg covering BOTH the TCP and gRPC codecs. (The BRAINSTORM's "39.2 may split TCP vs gRPC at its SPEC" is resolved NO: the two probers share the parse/dispatch refactor and the runtime, so splitting would duplicate that refactor across two legs.) With 39.2 the active-HC checker roster (HTTP + TCP + gRPC) is COMPLETE.

### D-S39.2 resolutions (SPEC §12)

- **D-S39.2-1 (prober interface + envelope shape):** a `prober` interface `probe(addr string) (ok, networkFailure bool)` in `internal/cluster/health.go`. Lift the timing/threshold envelope (`interval`/`unhealthy`/`healthy`) out of `httpHealthCheckCfg` into a `checkerSpec` (the per-`health_check` parse product); the per-probe `timeout` stays inside each prober (each prober closes over what it needs). (NOTE: SPEC §3.3/§12 listed `timeout` among the lifted-envelope fields; this PLAN deliberately keeps `timeout` per-prober instead — behavior-identical, and each prober already needs its own timeout; a justified resolution of the open D-question.) `healthChecker` holds `{interval, unhealthy, healthy time.Duration/uint32; prober prober}` instead of `cfg httpHealthCheckCfg`. `httpProber` wraps the existing `httpHealthCheckCfg` ({host, path, timeout, expectedStatuses}) and calls the UNCHANGED `probeHTTP` body — `httpProber` behavior is byte-identical, so the full 68-dir differential stays GREEN after the refactor (Task 2 verifies this BEFORE any new codec lands).
- **D-S39.2-2 (gRPC authority/initial_metadata + dial):** `service_name` is SUPPORTED (→ `HealthCheckRequest.Service`); `authority` + `initial_metadata` are SILENT-IGNORED (additive, orthogonal to the SERVING check — no reject, matching the route-level header-mutation silent-ignore precedent). The prober dials a FRESH `grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))` per probe (`reuse_connection` deferred); it uses `google.golang.org/grpc` DIRECTLY (NOT `internal/grpcclient` — the prober needs only a bare insecure dial + the `grpc_health_v1` client; `internal/grpcclient` carries xDS/timeout machinery the probe does not need).
- **D-S39.2-3 (gRPC classification edge):** MVP — `Check` returns `err != nil` ⇒ `(false, networkFailure=true)`; `Check` returns a response with `Status != SERVING` ⇒ `(false, networkFailure=false)`. The empirically-probed cases (refused port ⇒ `codes.Unavailable` ⇒ both `failure`+`network_failure`; reachable NOT_SERVING ⇒ `failure` only) are both correct under this rule. The `codes.Unimplemented`-on-a-reachable-host edge (a returned gRPC status that the MVP maps to network_failure) is a RECORDED departure — the `0068` fixture exercises only refused + SERVING (the NOT_SERVING discriminator is a unit-level test, Task 6), so the edge is never asserted; a future refinement may split `codes.Unavailable`/transport ⇒ network vs a returned status ⇒ application.
- **D-S39.2-4 (`0067`/`0068` constants + the BackendKind):** both fixtures = 2 live + 1 dead (66% healthy > 50% → FILTERING not panic); `interval: 0.5s`, `timeout: 0.5s`, thresholds 1/1; N=100; `warmupStable=10`; `convergeDeadline=30s`; `warmupDeadline=15s` (all single-sourced per `fixture_workload_constant_desync`, copied from `0066`). `GRPCHealthResponder` (BackendKind 34) is an IN-PROCESS goroutine backend (the `TCPThriftResponder` precedent — `net.Listen` + `go serve…` in the runner switch, NOT a subprocess): an `h2c` server that muxes `application/grpc` requests to a `health.NewServer()`-backed `grpc.Server.ServeHTTP` (reports `""` ⇒ SERVING) and answers every other request with `200` + a `backend-<idx>:` body (the host-attribution signal the driver tallies). The NOT_SERVING discriminator (`failure++`, `network_failure` stays 0) is a UNIT-level `grpcProber` test (Task 6), NOT a fixture arm (keeps `0068` the keystone filtering proof).
- **D-S39.2-5 (gRPC-must-be-H2 reject):** reuse the EXISTING `extractH2Mode` (`manager.go:582`, already called at `:414`). `parseHealthChecks` flags whether any checker is a gRPC checker; `buildCluster` validates `hasGrpcChecker && !useH2 → reject` at the point where `useH2` is known (after the `extractH2Mode` call). Byte-stable house wording per ADR-0080: `cluster: %q: grpc_health_check requires the cluster to support HTTP/2`. UNIT-level config-build reject test (no boot-reject dir — the `0066`/§8.2 precedent).
- **D-S39.2-6 (ADR-0045):** single flat 39.2 leg (above).

## File structure

- **Modify `internal/cluster/health.go`** — add the `prober` interface + `httpProber` (wraps `probeHTTP`, unchanged) + `tcpProber` + `grpcProber`; refactor `httpHealthCheckCfg` (drop `interval`/`unhealthy`/`healthy`, keep `host`/`path`/`timeout`/`expectedStatuses`); add `checkerSpec`; change `healthChecker` to hold the envelope + a `prober`; generalize `parseHealthChecks` to return `[]checkerSpec` (lift the http oneof reject; add the tcp/grpc arms; the `send`/`receive` reject). `probeOnce`/`applyResult`/`run`/`registerStats`/`newHealthChecker` adapt to the new field shape (behavior unchanged).
- **Modify `internal/cluster/health_test.go`** — adapt the existing tests to the new shape; add `tcpProber`/`grpcProber` unit tests (incl. the NOT_SERVING discriminator) + the new parse/reject tests.
- **Modify `internal/cluster/manager.go`** — `buildCluster`: thread the kind-tagged specs into `newHealthChecker` (the `:394` loop adapts); add the gRPC-must-be-H2 reject after `extractH2Mode`.
- **Modify `test/differential/fixture/fixture.go`** — add `GRPCHealthResponder BackendKind = 34` + its doc comment.
- **Modify `test/differential/runner_test.go`** — add the `case fixture.GRPCHealthResponder` in-process backend in the spawn switch (the h2c gRPC-SERVING + 200 responder) + the two new fixture blank-imports.
- **Create `test/fixtures/0067-health-check-tcp/{driver/driver.go, driver/driver_test.go, expectations.yaml, README.md}`** — the `0066` driver with `tcp_health_check{}` replacing `http_health_check`.
- **Create `test/fixtures/0068-health-check-grpc/{driver/driver.go, driver/driver_test.go, expectations.yaml, README.md}`** — an H2 cluster + `grpc_health_check{}`, `GRPCHealthResponder` backends.
- **Modify `docs/envoy-go/{DECISIONS.md, BEHAVIOR_CONTRACT.md, STATE.md, ROADMAP.md}`** + `docs/envoy-go/phases/39.2-health-check-tcp-grpc/{README.md, PROGRESS.md}` at the doc tasks.

---

### Task 1: First-task baselines/anchors gate + PROGRESS.md

**Files:**
- Create: `docs/envoy-go/phases/39.2-health-check-tcp-grpc/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL baselines** (single-source the counts the completion task asserts against)

Run and record:
```bash
ls -d test/fixtures/[0-9]* | wc -l            # expect 68
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 42
grep -E "^## ADR-" docs/envoy-go/DECISIONS.md | tail -1   # expect tail "## ADR-0243 …" (headings are h2, not h3)
grep -n "GRPCHealthResponder\|BackendKind = 33" test/differential/fixture/fixture.go   # tail 33 = TCPThriftResponder
go build ./... && go test -count=1 -short ./internal/cluster/... 2>&1 | tail -3
```
Expected: fixtures 68, fuzzers 42, DECISIONS tail ADR-0243, BackendKind tail 33, build clean, cluster tests pass.

- [ ] **Step 2: Write PROGRESS.md** (the 13-task spine + the baselines + the counts the completion task will move: fixtures 68→70, BackendKind 33→34, DECISIONS tail ADR-0243→ADR-0244, stat surface 1132→1132, fuzzers 42→42).

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/39.2-health-check-tcp-grpc/PROGRESS.md
git commit -m "phase 39.2 Task 1: baselines + PROGRESS.md"
```

---

### Task 2: The `prober` interface + the dispatch generalization (byte-stable HTTP)

This is the load-bearing refactor: it must leave the HTTP checker behavior byte-identical (the full 68-dir differential GREEN) BEFORE any new codec lands.

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`
- Modify: `internal/cluster/manager.go` (the `:394` `newHealthChecker` loop adapts to `[]checkerSpec`)

- [ ] **Step 1: Run the existing checker tests to capture the GREEN baseline**

Run: `go test -count=1 ./internal/cluster/...`
Expected: PASS (the 39.1 health tests).

- [ ] **Step 2: Introduce the `prober` interface + `httpProber` (wrapping the unchanged `probeHTTP`)**

In `health.go`, add:
```go
// prober is one health-check codec: it probes addr and reports (ok, networkFailure).
// networkFailure (a dial/transport error) is a sub-class of failure (both stats
// increment); a non-network failure (e.g. a bad HTTP status / a NOT_SERVING gRPC
// response) sets ok=false, networkFailure=false. (ADR-0244)
type prober interface {
	probe(addr string) (ok, networkFailure bool)
}

// httpProber is the HTTP checker codec — the 39.1 probeHTTP body, behavior-unchanged.
type httpProber struct{ cfg httpHealthCheckCfg }

func (p httpProber) probe(addr string) (ok, networkFailure bool) { return probeHTTP(addr, p.cfg) }
```
Keep `probeHTTP` + `httpHealthCheckCfg.statusOK` exactly as-is, but REMOVE `interval`/`unhealthy`/`healthy` from `httpHealthCheckCfg` (they move to `checkerSpec`); keep `host`/`path`/`timeout`/`expectedStatuses`.

- [ ] **Step 3: Add `checkerSpec` + retarget `healthChecker`**

```go
// checkerSpec is one parsed health_check: the timing/threshold envelope + the codec.
type checkerSpec struct {
	interval         time.Duration
	unhealthy, healthy uint32
	prober           prober
}
```
Change `healthChecker` to hold `interval time.Duration`, `unhealthy, healthy uint32`, `prober prober` (replacing `cfg httpHealthCheckCfg`). Update `newHealthChecker(eps, ch, spec checkerSpec)`; `probeOnce` calls `hc.prober.probe(ep.Addr())`; `applyResult` uses `hc.unhealthy`/`hc.healthy`; `run` uses `hc.interval`.

- [ ] **Step 4: Generalize `parseHealthChecks` to return `[]checkerSpec`** (HTTP arm only for now — the tcp/grpc arms land in Tasks 3/6)

Keep the envelope rejects (interval/timeout/thresholds/oneof). For the HTTP arm, build `checkerSpec{interval, unhealthy, healthy, prober: httpProber{cfg: httpHealthCheckCfg{host, path, timeout, expectedStatuses}}}`. Leave the existing `tcp/grpc → "only http_health_check is supported"` reject IN PLACE for this task (it is lifted in Tasks 3/6). Update `manager.go:394`’s loop: `for _, spec := range hcSpecs { cl.checkers = append(cl.checkers, newHealthChecker(cl.endpoints, health, spec)) }`.

- [ ] **Step 5: Adapt `health_test.go` to the new shape; run unit tests**

Three existing tests break STRUCTURALLY against the moved fields/signatures (this is real restructuring, not a rename):
- `TestHTTPHealthCheckCfg_StatusOK` — drop the `interval`/`unhealthy`/`healthy` fields from its `httpHealthCheckCfg{…}` literal and any `cfg.interval`/`.unhealthy`/`.healthy` assertions (those fields moved to `checkerSpec`).
- `TestHealthChecker_ProbeOnce` — `newHealthChecker` signature changed to `newHealthChecker(eps, ch, spec checkerSpec)`; build the spec with `prober: httpProber{cfg: httpHealthCheckCfg{…}}`.
- `TestParseHealthChecks` "valid" subtest — the returned element is now a `checkerSpec`: read `out[0].interval`/`.unhealthy`/`.healthy` off the envelope, and the HTTP-specific `path`/`timeout`/`expectedStatuses` via a type assertion `out[0].prober.(httpProber).cfg.path` etc.

Run: `go test -count=1 ./internal/cluster/...`
Expected: PASS (behavior unchanged; only the internal shape moved).

- [ ] **Step 6: Verify HTTP byte-stability against the differential**

Run: `go test -count=1 -run 'TestDifferential/0066' ./test/differential/`
Expected: PASS (the HTTP checker is byte-identical after the refactor). (Optionally run the full suite here; it is the Task 11 gate regardless.)

- [ ] **Step 7: Commit**
```bash
git add internal/cluster/health.go internal/cluster/health_test.go internal/cluster/manager.go
git commit -m "phase 39.2 Task 2: prober interface + checkerSpec dispatch (HTTP byte-stable)"
```

---

### Task 3: `tcpProber` (connect-only) + the `tcp_health_check` parse arm + the `send`/`receive` reject

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing tests**

In `health_test.go`: (a) `tcpProber{timeout}.probe` against a live `net.Listen` ⇒ `(true, false)`; against an unbound port ⇒ `(false, true)`. (b) `parseHealthChecks` on a cluster with `tcp_health_check{}` (+ full envelope) returns a spec with a `tcpProber` (no error); (c) a `tcp_health_check` with `send:` or `receive:` set ⇒ error matching `tcp_health_check send/receive payload matching is not supported`.

- [ ] **Step 2: Run to verify failure**

Run: `go test -count=1 -run 'TestTcpProber|TestParseHealthChecks_Tcp' ./internal/cluster/`
Expected: FAIL (tcpProber undefined / tcp arm rejects).

- [ ] **Step 3: Implement `tcpProber` + the parse arm**

```go
// tcpProber is the connect-only TCP checker codec: a successful TCP connect proves
// liveness; a dial failure (refused/timeout) is a network failure. (ADR-0244)
type tcpProber struct{ timeout time.Duration }

func (p tcpProber) probe(addr string) (ok, networkFailure bool) {
	d := net.Dialer{Timeout: p.timeout}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return false, true
	}
	_ = conn.Close()
	return true, false
}
```
In `parseHealthChecks`, REPLACE the 39.1 `only http_health_check is supported` reject with a per-arm dispatch: `GetHttpHealthCheck() != nil` ⇒ the http arm; `GetTcpHealthCheck() != nil` ⇒ reject `send`/`receive` if set (`cluster: %q: tcp_health_check send/receive payload matching is not supported`), else `checkerSpec{… prober: tcpProber{timeout}}`; `GetGrpcHealthCheck() != nil` ⇒ (Task 6); none set ⇒ the existing `a health_checker is required` reject.

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 -run 'TestTcpProber|TestParseHealthChecks_Tcp' ./internal/cluster/`
Expected: PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/cluster/health.go internal/cluster/health_test.go
git commit -m "phase 39.2 Task 3: tcpProber connect-only + tcp_health_check parse (send/receive deferred-reject)"
```

---

### Task 4: The `0067-health-check-tcp` differential fixture

**Files:**
- Create: `test/fixtures/0067-health-check-tcp/driver/driver.go`
- Create: `test/fixtures/0067-health-check-tcp/driver/driver_test.go`
- Create: `test/fixtures/0067-health-check-tcp/expectations.yaml`
- Create: `test/fixtures/0067-health-check-tcp/README.md`
- Modify: `test/differential/runner_test.go` (blank-import)

- [ ] **Step 1: Author the driver** — COPY `test/fixtures/0066-health-check-http/driver/driver.go` verbatim, then change: `fixtureName = "0067-health-check-tcp"`; `refContainerListenerPort = 19156`; the `healthChecksBlock` to use the TCP checker:
```go
const healthChecksBlock = `      health_checks:
        - interval: 0.5s
          timeout: 0.5s
          unhealthy_threshold: 1
          healthy_threshold: 1
          tcp_health_check: {}`
```
Everything else is IDENTICAL (the live `HTTPEcho` backends answer a bare TCP connect for the HC AND the HTTP data-plane load; the dead host is the same unbound-port trick; `BackendKind() = fixture.HTTPEcho`; NO new BackendKind). **KEEP the cluster name `c_hc`** (do NOT rename to `c_tcp`) — the copied driver hard-codes `cluster.c_hc.*` stat keys throughout `AssertStats`; a half-rename desyncs them (`reference_fixture_workload_constant_desync`). Update the package doc comment to reference the TCP checker + SPEC §8.1 / this PLAN Task 4.

- [ ] **Step 2: Author `driver_test.go`, `expectations.yaml`, `README.md`** — copy the `0066` versions, retargeting names/ports to `0067`.

- [ ] **Step 3: Register the fixture** — add to `test/differential/runner_test.go` (after the `0066` import):
```go
_ "github.com/esalaine/envoy-go/test/fixtures/0067-health-check-tcp/driver"
```

- [ ] **Step 4: Run the fixture**

Run: `go test -count=1 -run 'TestDifferential/0067' ./test/differential/`
Expected: PASS (the dead host converges to unhealthy via the connect-refused TCP probe on both sides; 100%-live load).

- [ ] **Step 5: Commit**
```bash
git add test/fixtures/0067-health-check-tcp/ test/differential/runner_test.go
git commit -m "phase 39.2 Task 4: 0067-health-check-tcp cross-side fixture (connect-only TCP HC)"
```

---

### Task 5: `0067` deliberate-break liveness + ≥20-run flake

**Files:** (no production change — verification only; record evidence in PROGRESS.md)

- [ ] **Step 1: Break A — `tcpProber` always healthy.** Temporarily edit `tcpProber.probe` to `return true, false` unconditionally. Run `go test -count=1 -run 'TestDifferential/0067' ./test/differential/`. Expected: FAIL at `converge:` (membership_healthy never reaches 2 — the dead host is never marked unhealthy). REVERT (`git restore internal/cluster/health.go`).

- [ ] **Step 2: Break B — Pick ignores health.** Temporarily force the `roundRobin` health filter to admit every host (change the `if rr.health.isHealthy(ep)` guard at `loadbalancer.go:61` to `if true`). Run `go test -count=1 -run 'TestDifferential/0067' ./test/differential/`. Expected: FAIL at `warmup:` (the dead host stays in rotation → never `warmupStable` consecutive 200s). REVERT.

- [ ] **Step 3: Flake check** — `for i in $(seq 1 20); do go test -count=1 -run 'TestDifferential/0067' ./test/differential/ || break; done`. Expected: 20/20 PASS.

- [ ] **Step 4: Record evidence in PROGRESS.md + commit** (`git restore` confirmed clean; `git diff --stat` empty for production files).
```bash
git add docs/envoy-go/phases/39.2-health-check-tcp-grpc/PROGRESS.md
git commit -m "phase 39.2 Task 5: 0067 deliberate-break liveness + 20/20 flake"
```
> Honor `reference_differential_break_protocol_count1` (use `-count=1` — go-test caching serves a stale PASS otherwise) and `feedback_subagent_worktree_detach` (`git restore`, never checkout-sha/amend).

---

### Task 6: `grpcProber` + the `grpc_health_check` parse arm + the NOT_SERVING discriminator test

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing tests** (use an in-process `health.NewServer()` on a real `net.Listen` + `grpc.NewServer`)

In `health_test.go`: (a) `grpcProber{serviceName:"", timeout}.probe` against a SERVING server ⇒ `(true, false)`; (b) against a server reporting NOT_SERVING for `serviceName:"svc.Bad"` ⇒ `(false, false)` — **the discriminator: networkFailure is FALSE** (reachable, app-unhealthy); (c) against an unbound port ⇒ `(false, true)`; (d) `parseHealthChecks` on `grpc_health_check{service_name:"x"}` (+ envelope) returns a `grpcProber` spec.

- [ ] **Step 2: Run to verify failure**

Run: `go test -count=1 -run 'TestGrpcProber|TestParseHealthChecks_Grpc' ./internal/cluster/`
Expected: FAIL (grpcProber undefined).

- [ ] **Step 3: Implement `grpcProber` + the parse arm** (per SPEC §3.2; imports `google.golang.org/grpc`, `google.golang.org/grpc/credentials/insecure`, `google.golang.org/grpc/health/grpc_health_v1`)

```go
type grpcProber struct {
	serviceName string
	timeout     time.Duration
}

func (p grpcProber) probe(addr string) (ok, networkFailure bool) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false, true
	}
	defer func() { _ = conn.Close() }()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: p.serviceName})
	if err != nil {
		return false, true // transport/RPC error (codes.Unavailable on a refused port) = network failure (D-S39.2-3 MVP)
	}
	return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING, false // NOT_SERVING/other = application failure (network_failure stays 0)
}
```
In `parseHealthChecks`, the `GetGrpcHealthCheck() != nil` arm: `checkerSpec{… prober: grpcProber{serviceName: g.GetServiceName(), timeout}}`. `authority`/`initial_metadata` are silent-ignored (D-S39.2-2). (The cluster-must-be-H2 reject is Task 7 — it lives in `buildCluster` where `useH2` is known, not in `parseHealthChecks`.) `parseHealthChecks` flags `hasGrpc` for Task 7 (e.g. return an extra `bool` or store it on a returned struct) — this changes its signature a SECOND time (after Task 2), so update the `manager.go:364` call site again here. **Do NOT split transport-vs-returned-status in 39.2:** the MVP maps every `err != nil` to `network_failure` (D-S39.2-3 recorded departure); a `status.FromError`/`codes`-based refinement is explicitly out of scope.

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 -run 'TestGrpcProber|TestParseHealthChecks_Grpc' ./internal/cluster/`
Expected: PASS (incl. the NOT_SERVING discriminator: `networkFailure == false`).

- [ ] **Step 5: Confirm `go mod tidy -diff` is still empty** (the grpc imports add no module — grpc is already direct)

Run: `go mod tidy -diff` ; Expected: exit 0, EMPTY.

- [ ] **Step 6: Commit**
```bash
git add internal/cluster/health.go internal/cluster/health_test.go
git commit -m "phase 39.2 Task 6: grpcProber (grpc.health.v1 Check) + grpc_health_check parse + NOT_SERVING discriminator"
```

---

### Task 7: The gRPC cluster-must-be-H2 reject (reuse `extractH2Mode`)

**Files:**
- Modify: `internal/cluster/manager.go`
- Modify: `internal/cluster/health_test.go` (or `manager_test.go` — match where the existing cluster-reject tests live)

- [ ] **Step 1: Write the failing test** — `buildCluster` on a cluster with `grpc_health_check{}` but NO http2 ⇒ error matching `grpc_health_check requires the cluster to support HTTP/2`; the SAME cluster WITH `http2_protocol_options`/`HttpProtocolOptions{http2}` ⇒ no error.

- [ ] **Step 2: Run to verify failure**

Run: `go test -count=1 -run 'TestBuildCluster_GrpcHealthCheck' ./internal/cluster/`
Expected: FAIL (no H2 reject yet).

- [ ] **Step 3: Implement the reject** — in `buildCluster`, AFTER `useH2, err := extractH2Mode(...)` (`manager.go:414`), add: `if hasGrpcHC && !useH2 { return nil, fmt.Errorf("cluster: %q: grpc_health_check requires the cluster to support HTTP/2", name) }` where `hasGrpcHC` came from `parseHealthChecks` (Task 6). (Place it after `extractH2Mode` so `useH2` is known; the checkers are already built at `:394` but the reject fails the whole `buildCluster` so no checker leaks.)

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 -run 'TestBuildCluster_GrpcHealthCheck' ./internal/cluster/`
Expected: PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/cluster/manager.go internal/cluster/health_test.go
git commit -m "phase 39.2 Task 7: gRPC cluster-must-be-H2 config-build reject (reuse extractH2Mode)"
```

---

### Task 8: The `GRPCHealthResponder` BackendKind (the in-process h2c gRPC-SERVING + 200 responder)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (the enum + doc)
- Modify: `test/differential/runner_test.go` (the spawn switch + the `serveGRPCHealth` helper + NEW imports: `google.golang.org/grpc`, `google.golang.org/grpc/health`, `google.golang.org/grpc/health/grpc_health_v1`, `golang.org/x/net/http2`, `golang.org/x/net/http2/h2c` — all under existing direct deps, zero new module)

- [ ] **Step 1: Add the BackendKind constant** — in `fixture.go`, after `TCPThriftResponder BackendKind = 33`:
```go
	// GRPCHealthResponder is an in-process h2c backend (phase 39.2): it answers
	// grpc.health.v1.Health/Check ⇒ SERVING (for the active gRPC HC probe) AND
	// returns HTTP 200 with a "backend-<idx>:" body to plain-H2 data-plane requests
	// (so the load-phase 100%-live assertion holds). NEW BackendKind per
	// reference_differential_fixture_dispatch_constraint.
	GRPCHealthResponder BackendKind = 34
```

- [ ] **Step 2: Add the spawn case + the `serveGRPCHealth` helper** — in `runner_test.go`'s backend switch, mirror `case fixture.TCPThriftResponder` (in-process `net.Listen` + `go serve…`):
```go
		case fixture.GRPCHealthResponder:
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go serveGRPCHealth(ln, i)
```
Add the helper (uses `google.golang.org/grpc/health`, `grpc_health_v1`, `golang.org/x/net/http2`, `golang.org/x/net/http2/h2c`):
```go
func serveGRPCHealth(ln net.Listener, idx int) {
	gs := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)
	mux := func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			gs.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "backend-%d:%s", idx, r.URL.Path)
	}
	srv := &http.Server{Handler: h2c.NewHandler(http.HandlerFunc(mux), &http2.Server{})}
	_ = srv.Serve(ln)
}
```
> NOTE: `serveGRPCHealth` intentionally omits the `bo.accepts` accept-counter argument that `acceptThriftResponder`/etc. take — `0068` attributes hosts via the `backend-<idx>:` response body (the `0066` `backendIdxFromBody` precedent), not the runner's accept counter. Use `bo.idx` (matching `acceptHTTPEchoCounting(ln, bo.accepts, bo.idx)`, `runner_test.go:184`) rather than the raw loop `i` if `bo.idx` is set in this switch arm.

- [ ] **Step 3: Build the test binary**

Run: `go test -count=1 -run 'TestNothing' ./test/differential/` (compile-only; expect "no tests to run", no build error).
Expected: compiles clean (the new imports resolve; `go mod tidy -diff` still empty).

- [ ] **Step 4: Commit**
```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 39.2 Task 8: GRPCHealthResponder BackendKind (in-process h2c gRPC-SERVING + 200 responder)"
```

---

### Task 9: The `0068-health-check-grpc` differential fixture

**Files:**
- Create: `test/fixtures/0068-health-check-grpc/driver/driver.go`
- Create: `test/fixtures/0068-health-check-grpc/driver/driver_test.go`
- Create: `test/fixtures/0068-health-check-grpc/expectations.yaml`
- Create: `test/fixtures/0068-health-check-grpc/README.md`
- Modify: `test/differential/runner_test.go` (blank-import)

- [ ] **Step 1: Author the driver** — COPY the `0067` driver (KEEP the cluster name `c_hc` — the `cluster.c_hc.*` stat keys are hard-coded in `AssertStats`), then change: `fixtureName = "0068-health-check-grpc"`; `refContainerListenerPort = 19157`; `BackendKind() = fixture.GRPCHealthResponder`; the `healthChecksBlock` to the gRPC checker:
```go
const healthChecksBlock = `      health_checks:
        - interval: 0.5s
          timeout: 0.5s
          unhealthy_threshold: 1
          healthy_threshold: 1
          grpc_health_check: {}`
```
Make the UPSTREAM CLUSTER H2 on BOTH sides (the gRPC checker requires it; the proxy then speaks cleartext-H2/h2c to the `GRPCHealthResponder` for both the HC probe AND the forwarded data-plane request). Add to each cluster block:
```yaml
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
```
**The DOWNSTREAM listener STAYS `codec_type: HTTP1`** (unchanged from `0067`): the driver sends HTTP/1.1 `GET /` via the SAME `helpers.HTTPRoundTrip` as `0066`/`0067`, and the proxy translates H1-downstream → H2-upstream. So NO H2 client helper is needed — `loadAndTally`/`warmupUntilStable` are byte-identical to `0067`. Only the cluster block changes (the `http2_protocol_options` above + the `grpc_health_check` block). The `backend-<idx>:` body attribution is unchanged (the `GRPCHealthResponder` h2c handler writes it for the non-gRPC data-plane request).

- [ ] **Step 2: Author `driver_test.go`, `expectations.yaml`, `README.md`** — copy the `0067` versions, retargeting to `0068` + noting the gRPC checker + the +1 BackendKind + the H2 data plane.

- [ ] **Step 3: Register the fixture** — add to `runner_test.go`:
```go
_ "github.com/esalaine/envoy-go/test/fixtures/0068-health-check-grpc/driver"
```

- [ ] **Step 4: Run the fixture**

Run: `go test -count=1 -run 'TestDifferential/0068' ./test/differential/`
Expected: PASS (the SERVING backends stay healthy; the dead port converges to unhealthy via the gRPC-transport-failure probe on both sides; 100%-live H2 load). Honor `reference_docker_probe_bridge_network` (the reference is a container — the SERVING backends must be reachable from it via `host.docker.internal`; verify `upstream_rq_total > 0` on the reference side).

- [ ] **Step 5: Commit**
```bash
git add test/fixtures/0068-health-check-grpc/ test/differential/runner_test.go
git commit -m "phase 39.2 Task 9: 0068-health-check-grpc cross-side fixture (gRPC HC over H2)"
```

---

### Task 10: `0068` deliberate-break liveness + ≥20-run flake

**Files:** (verification only; record in PROGRESS.md)

- [ ] **Step 1: Break A — `grpcProber` ignores `resp.Status`** (`return true, false` after the Check). Run `go test -count=1 -run 'TestDifferential/0068' ./test/differential/`. Expected: FAIL at `converge:` (the dead port: actually `grpcProber` returns network-failure on the dead port regardless — so break A must instead force the dead host healthy; simplest break: make `grpcProber.probe` `return true, false` unconditionally → the dead port is never unhealthy → converge times out). REVERT.

- [ ] **Step 2: Break B — Pick ignores health** (as Task 5 Break B). Expected: FAIL at `warmup:`. REVERT.

- [ ] **Step 3: Flake check** — `for i in $(seq 1 20); do go test -count=1 -run 'TestDifferential/0068' ./test/differential/ || break; done`. Expected: 20/20 PASS. (gRPC + h2c + Docker is heavier than HTTP — if flaky, widen `convergeDeadline`/`warmupDeadline`, NEVER the assertion, per `reference_health_check_propagation_warmup`.)

- [ ] **Step 4: Record evidence + commit**
```bash
git add docs/envoy-go/phases/39.2-health-check-tcp-grpc/PROGRESS.md
git commit -m "phase 39.2 Task 10: 0068 deliberate-break liveness + 20/20 flake"
```

---

### Task 11: Full differential re-verify (70-dir) + the six-gate

**Files:** (verification only)

- [ ] **Step 1: Full differential** — `go test -count=1 ./test/differential/`. Expected: 70/70 dirs PASS (the pre-existing 68 byte-stable + `0067` + `0068`).

- [ ] **Step 2: The six-gate (ADR-0052)**
```bash
gofmt -l internal/ test/ cmd/                     # expect EMPTY
golangci-lint run ./internal/cluster/... ./test/differential/...   # expect clean
go build ./...                                     # expect clean
go mod tidy -diff                                  # expect EMPTY (zero new module)
go test -count=1 ./...                             # expect PASS
go test -count=1 -race -short ./internal/cluster/...   # expect PASS (no goroutine leak)
```
Expected: all gates GREEN.

- [ ] **Step 3: Confirm counts** — `ls -d test/fixtures/[0-9]* | wc -l` ⇒ 70; `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` ⇒ 42; `grep -n "BackendKind = 34" test/differential/fixture/fixture.go` ⇒ GRPCHealthResponder.

- [ ] **Step 4: Commit** (if any gofmt/lint fixes were needed)
```bash
git add -A && git commit -m "phase 39.2 Task 11: full 70-dir differential + six-gate GREEN"
```
> Honor `feedback_pertask_gofmt_lint` (run gofmt -l + golangci-lint on the touched pkgs every task, not just at the gate).

---

### Task 12: ADR-0244 body + BEHAVIOR_CONTRACT 39.2 delta + DECISIONS

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0244 IN-PLACE: promote the SPEC §13 §Context draft + add §Decision + §Consequences per ADR-0044; tail ADR-0243 → ADR-0244, next-free ADR-0245)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (extend the `### Cluster — active health checks` subsection: the TCP connect-only codec [success/refused classification; payload deferred-reject], the gRPC codec [SERVING ⇒ healthy; the application-vs-network `network_failure` discriminator; service_name; the cluster-must-be-H2 reject], the LIFT of the `only http_health_check is supported` reject; the stat-surface block STAYS 1132)

- [ ] **Step 1: Write ADR-0244** (§Context from SPEC §13; §Decision = the `prober` dispatch + the three codecs + the H2 reject; §Consequences = roster complete, zero new module/stats, +1 BackendKind, the prober extension point).
- [ ] **Step 2: Extend BEHAVIOR_CONTRACT** (the TCP/gRPC codec rows; the stat surface STAYS 1132).
- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 39.2 Task 12: ADR-0244 body + BEHAVIOR_CONTRACT 39.2 delta"
```

---

### Task 13: Completion bundle (counts + STATE/ROADMAP/README/PROGRESS)

**Files:**
- Modify: `docs/envoy-go/STATE.md` (active-phase → `phase 39.2 IMPL done`; the count footer → fixtures 70, BackendKind 34, DECISIONS tail ADR-0244, next-free ADR-0245; stat surface 1132; fuzzers 42)
- Modify: `docs/envoy-go/ROADMAP.md` (row 39 — the 39.2 sub-leg `in-progress → done` note; NO parent rollup per ADR-0106)
- Modify: `docs/envoy-go/phases/39.2-health-check-tcp-grpc/README.md` (status 2 → 3 → 4; IMPL done) + `PROGRESS.md` (13 tasks complete + the six-gate + 70-dir evidence)

- [ ] **Step 1: Update STATE/ROADMAP/README/PROGRESS** with the as-built counts.
- [ ] **Step 2: Final consistency check** — grep the counts across STATE/ROADMAP/README/PROGRESS/BEHAVIOR_CONTRACT all agree (fixtures 70 / fuzzers 42 / stat surface 1132 / BackendKind 34 / DECISIONS tail ADR-0244).
- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/
git commit -m "phase 39.2 Task 13: completion bundle (STATE/ROADMAP/README/PROGRESS + counts)"
```

---

## Notes for the executor

- **Worktree + branch:** execute in a fresh worktree (`feedback_git_worktrees`); subagents commit LOCAL-ONLY (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`).
- **Path-targeting:** write to the WORKTREE paths, not the main checkout (`feedback_subagent_worktree_path_targeting`); after any deliberate-break task, `git restore` (never checkout-sha/amend — `feedback_subagent_worktree_detach`); the controller re-verifies the branch + a clean main repo after each task.
- **Per-task hygiene:** every task runs `gofmt -l` + `golangci-lint` on the touched packages, not just at the six-gate (`feedback_pertask_gofmt_lint`).
- **Differential selectors:** ALWAYS `-run 'TestDifferential/0067'` / `'TestDifferential/0068'` (NOT `-run '0067'`, which matches zero subtests — `reference_differential_run_selector`); ALWAYS `-count=1` for break/flake (go-test caching — `reference_differential_break_protocol_count1`).
- **Docker discipline:** the reference is a container; `0068`'s SERVING backends must be reachable via `host.docker.internal` on a shared bridge, and the driver must verify `upstream_rq_total > 0` on the reference side (`reference_docker_probe_bridge_network`). Gate on `membership_healthy` convergence + the warmup pattern, NEVER raw `attempt` deltas (`reference_health_check_propagation_warmup`).
- **`0068` is H2 UPSTREAM-ONLY:** the downstream listener stays `codec_type: HTTP1` (the driver reuses the H1 `helpers.HTTPRoundTrip`); only the cluster gains `http2_protocol_options` (proxy→backend cleartext-H2/h2c). The `GRPCHealthResponder` is an h2c server, so it accepts the proxy's cleartext-H2 for both the gRPC HC probe and the forwarded data-plane request. No TLS, no downstream H2, no `H2RoundTrip` helper.
- **ADR-0045 re-check:** done (single flat leg). If the gRPC backend or the H2 data-plane plumbing balloons the task count past ~25 or the prod LoC past ~1500 (it should not), re-invoke the gate.
