# Phase 39.2 IMPL — PROGRESS

TCP + gRPC active-health-check checker codecs — the CODEC-ONLY widening of the 39.1 substrate. Executed subagent-driven per `docs/envoy-go/phases/39.2-health-check-tcp-grpc/PLAN.md` (13 tasks). **STATUS: COMPLETE / IMPL DONE**

## Baselines captured (pre-IMPL, at worktree HEAD)

- fixtures: 68 · fuzzers: 42 · DECISIONS tail: ADR-0243 · stat surface: 1132 · BackendKind tail: 33 (`TCPThriftResponder`) · build OK · `go test -count=1 -short ./internal/cluster/...` → ok

## Task checklist

- [x] Task 1 — baselines/anchors + PROGRESS.md
- [x] Task 2 — prober interface + checkerSpec dispatch (HTTP byte-stable)
- [x] Task 3 — tcpProber connect-only + tcp_health_check parse (send/receive deferred-reject)
- [x] Task 4 — 0067-health-check-tcp cross-side fixture
- [x] Task 5 — 0067 deliberate-break liveness + 20-run flake
- [x] Task 6 — grpcProber (grpc.health.v1 Check) + grpc_health_check parse + NOT_SERVING discriminator
- [x] Task 7 — gRPC cluster-must-be-H2 config-build reject
- [x] Task 8 — GRPCHealthResponder BackendKind (in-process h2c)
- [x] Task 9 — 0068-health-check-grpc cross-side fixture
- [x] Task 10 — 0068 deliberate-break liveness + 20-run flake
- [x] Task 11 — full 70-dir differential + six-gate
- [x] Task 12 — ADR-0244 body + BEHAVIOR_CONTRACT 39.2 delta
- [x] Task 13 — completion bundle (STATE/ROADMAP/README/PROGRESS + counts)

## 0067 break evidence (Task 5)

- Break A (`tcpProber.probe` returns `true, false` unconditionally — no dial): FAIL at `converge:` stage — `cluster.c_hc.membership_healthy did not converge to 2 within 30s (last seen 3) — dead host not detected?`
- Break B (`if rr.health.isHealthy(ep)` → `if true` at `loadbalancer.go:61` — admit all hosts): FAIL at `warmup:` stage — `data path did not stabilize to 10 consecutive 200s within 15s (last status 503) — dead host still in worker rotation?`
- Both breaks reverted via `git restore`; `git status --short` confirmed clean; `go build ./...` passes.
- Flake check: **20/20 PASS** (all runs ~2.1–2.4s each)

## 0068 break evidence (Task 10)

- Break A (`grpcProber.probe` returns `true, false` unconditionally — dead port never marked unhealthy): FAIL at `converge:` stage — `subject: cluster.c_hc.membership_healthy did not converge to 2 within 30s (last seen 3) — dead host not detected?`
- Break B (`if rr.health.isHealthy(ep)` → `if true` at `loadbalancer.go:61` — admit all hosts): FAIL at `warmup:` stage — `subject: data path did not stabilize to 10 consecutive 200s within 15s (last status 503, err <nil>) — dead host still in worker rotation?`
- Both breaks reverted via `git restore`; `git status --short` confirmed clean (production code intact); `go build ./...` passes.
- Flake check: **20/20 PASS** (all runs ~2.0–2.2s each).

## 0068 cx_active experiment (Task 10) — assertion deviation from "identical to 0067"

The 0068 driver deviates from the SPEC's "byte-identical to 0067" directive on ONE assertion: `cluster.c_hc.upstream_cx_active`. 0067 (HTTP/1.1, `Connection: close`) sees `cx_active == 0` on both sides (each request is a fresh dial that completes). 0068 is an **H2 upstream** (the gRPC HC requires it), so the data plane multiplexes over a connection pool whose idle-retention policy legitimately differs per implementation.

Empirical 20-run tally (TEMP `os.Stderr` instrumentation at the assertion, since removed):
- **reference: `cx_active == 2` (= backendCount) on ALL 20 runs** — Envoy keeps one pooled H2 connection per LIVE host.
- **subject: `cx_active == 0` on ALL 20 runs** — envoy-go's pool tears the idle connections down.
- Zero flap on either side; 20/20 PASS.

**DECISION: UPGRADED to a per-side exact pin** (`wantCxActiveRef = backendCount` / `wantCxActiveSubj = 0`), following the `0053-kafka-requests` `wantRef`/`wantSubj` per-side-pin precedent — replacing the prior `assertAtMost(..., backendCount)` upper bound. Rationale: the per-side data is fully stable across 20 runs, so the exact pin is more faithful (it pins each side's real H2-pool quiescence behavior, not just an upper bound) AND still bites the dead-host-pooling regression (a 3rd connection on either side — i.e. a connection held to the filtered dead host — fails the pin). The now-unused `assertAtMost` helper was removed; the TEMP instrumentation + its `os` import were fully removed. The driver diff is the ONLY production/test change from this task.

## Final six-gate GREEN (Task 11)

All six gates GREEN at the 70-dir HEAD:

- Gate 1 — `go build ./...` PASS
- Gate 2 — `go vet ./...` PASS
- Gate 3 — `golangci-lint run ./...` PASS
- Gate 4 — `go test -count=1 -race -short ./...` PASS
- Gate 5 — `ls -d test/fixtures/[0-9]* | wc -l` → **70** (tail `0068-health-check-grpc`)
- Gate 6 — `go test -count=1 -timeout 600s ./test/differential/...` → **70-dir differential GREEN**

## As-built counts (Task 13 — FINAL)

- fixtures: 68 → **70** (`0067-health-check-tcp` + `0068-health-check-grpc`)
- fuzzers: 42 → **42** (UNCHANGED — gRPC client is turnkey, no hand-rolled wire decoder to fuzz)
- stat surface: 1132 → **1132** (UNCHANGED — both codecs reuse the 39.1 codec-agnostic stat set)
- DECISIONS tail: ADR-0243 → **ADR-0244** (next-free ADR-0245)
- BackendKind tail: 33 → **34** (`GRPCHealthResponder` — in-process h2c gRPC-SERVING + 200 responder)
- packages: ZERO new · go.mod modules: ZERO new
