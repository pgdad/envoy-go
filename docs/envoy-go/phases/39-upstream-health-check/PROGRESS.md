# Phase 39.1 IMPL — PROGRESS

Active HTTP health checks (`Cluster.health_checks`) — the host health-state dimension + the cluster-level checker runtime + the health-aware LB pick. Executed subagent-driven per `docs/envoy-go/phases/39-upstream-health-check/PLAN.md` (16 tasks). **STATUS: COMPLETE — six-gate GREEN, full 68-dir differential GREEN.**

## Baselines captured (pre-IMPL, at master `bcd0345`)

- fixtures: 67 · fuzzers: 42 · DECISIONS tail: ADR-0241 · stat surface: 1125 · BackendKind tail: 33 · build OK

## Task checklist

- [x] Task 1 — baselines/anchors + PROGRESS.md (`7184a99`)
- [x] Task 2 — `hostHealth` registry + first-check-immediate transitions (`34a2098`)
- [x] Task 3 — `clusterHealth` view + strict-`<` panic threshold (`61d57e9`)
- [x] Task 4 — the HTTP probe codec (`GET path` + `expected_statuses`) (`89cfcbd`)
- [x] Task 5 — the `healthChecker` runtime (probeOnce + applyResult + stats) (`4fbc8c8`)
- [x] Task 6 — parse `health_checks` + panic threshold + the reject roster (`4d93fb3`)
- [x] Tasks 7–9 — thread `*clusterHealth` into `buildLeafLB` + health-aware pick across all six constructs (`7894177`; code-reviewer APPROVED, byte-stability verified)
- [x] Tasks 10–11 — register the +7 stats + `StartHealthChecks`/`Drain` lifecycle + `main.go` boot (`e5b295f`)
- [x] Task 12 — the `0066-health-check-http` poll-to-converge differential (`27c59e3`)
- [x] Task 13 — deliberate-break liveness + warmup gate → 20/20 flake-free (`a70e9c2`)
- [x] Task 14 — full 68-dir differential + six-gate GREEN (verification; no artifact)
- [x] Task 15 — ADR-0242/0243 bodies + BEHAVIOR_CONTRACT delta (`e0b4e67`)
- [x] Task 16 — completion bundle (counts + STATE/ROADMAP/README/PROGRESS) (this commit)

## As-built (六-gate evidence)

- **gofmt** `-l internal/ cmd/ test/` → empty · **golangci-lint** `./internal/... ./cmd/... ./test/...` → clean · **go build ./...** → OK · **go mod tidy -diff** → empty
- **unit** `go test ./internal/... ./cmd/...` → green · **-race -short** (cluster + cmd) → clean
- **full differential** `go test ./test/differential/ -count=1` → **ok 224.043s** (all 68 dirs; the existing 67 byte-identical — health-aware pick is inert when no `health_checks`; the new `0066`)
- **`0066` flake** → 20/20 PASS (with the warmup gate); 2 deliberate breaks (A: `isHealthy`→true → convergence timeout; B: `Pick` ignores health → warmup never stabilizes) both bite under `-count=1`
- h2spec 53/53 + proxy-wasm 10/10 asserted-unaffected by change-scope (touches only `internal/cluster/*` + `cmd/envoy-go/main.go` + the `0066` fixture + docs — no HTTP framing / wasm path; the wire path is byte-identical when no `health_checks`)

## As-built counts

- fixtures: 67 → **68** (`0066-health-check-http`)
- fuzzers: 42 → **42** (the probe response reuses the fuzzed H1/H2 parser; threshold-transition property tests are unit-level)
- stat surface: 1125 → **1132** (+7: `health_check.{attempt,success,failure,network_failure}` counters + `health_check.healthy` gauge + `membership_healthy` gauge + `lb_healthy_panic` counter; emitted only on clusters with `health_checks`)
- DECISIONS tail: ADR-0241 → **ADR-0243** (ADR-0242 state+runtime / ADR-0243 health-aware pick, both ACCEPTED in-place per ADR-0044; next-free ADR-0244)
- BackendKind tail: 33 → **33** (`0066` reuses the HTTP backends; the dead host is an unbound port)
- packages: ZERO new · go.mod modules: ZERO new (`go mod tidy -diff` empty)
