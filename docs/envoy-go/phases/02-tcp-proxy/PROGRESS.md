# Phase 02 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

None.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** b6410ca83579538164a901a199e3b0e18976c15e
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions".
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/02-tcp-proxy-impl
$ git log -1 --format=%H
013daa70f31f9ec2faead839f5903ad247ca3075
$ docker version
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
?   	github.com/esalaine/envoy-go/internal/cluster	[no test files]
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
?   	github.com/esalaine/envoy-go/internal/listener	[no test files]
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	(cached)
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
$ go list -m github.com/envoyproxy/go-control-plane/envoy
github.com/envoyproxy/go-control-plane/envoy v1.32.4
```

## Task 2 — internal/cluster: Cluster + Endpoint + round-robin LB

**Commits:** 24a66688c122c55cdc1e7b847513000ffb577f21
**Notes:** Created `internal/cluster/{doc.go, cluster.go, loadbalancer.go, loadbalancer_test.go}` per PLAN §Task 2 verbatim. Appended ADR-0024 codifying per-cluster `atomic.Uint64` RR counter scope. TDD: tests written first (Step 1), FAILed as expected (Step 2, undefined types), PASS after implementation (Step 6, 4 tests). Lint + vet clean. Two comment spellings adjusted to US locale (`materialises`→`materializes`, `defence`→`defense`, `randomised`→`randomized`) and two `//nolint:unused` directives added for `defaultConnectTimeout` and `endpoints` field (both consumed by Task 3 NewManager).
**Outputs:**
```
$ go test ./internal/cluster/ -run TestRoundRobin -v
=== RUN   TestRoundRobin_DistributionExact
--- PASS: TestRoundRobin_DistributionExact (0.00s)
=== RUN   TestRoundRobin_FirstPickIsEndpoint0
--- PASS: TestRoundRobin_FirstPickIsEndpoint0 (0.00s)
=== RUN   TestRoundRobin_ConcurrentDistributionExact
--- PASS: TestRoundRobin_ConcurrentDistributionExact (0.00s)
=== RUN   TestRoundRobin_ZeroEndpoints
--- PASS: TestRoundRobin_ZeroEndpoints (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.002s
$ go vet ./internal/cluster/
$ golangci-lint run ./internal/cluster/
```

## Task 3 — internal/cluster.Manager: build-time materialisation

**Commits:** <sha — filled by SHA-fill commit>
**Notes:** Created `internal/cluster/manager.go` and `internal/cluster/manager_test.go` per PLAN §Task 3. TDD: 11 `TestManager_*` tests written first (Step 1), FAILed as expected (Step 2, undefined `NewManager`/`Manager`), PASS after implementation (Step 4, 15 tests total: 4 RR + 11 Manager). `manager.go` is verbatim from PLAN with two US-locale spelling corrections (`materialised`→`materialized`, `materialises`→`materializes`, matching Task 2 precedent). Removed both `//nolint:unused` directives from `cluster.go` (`defaultConnectTimeout` and `Cluster.endpoints`), which are now consumed by `buildCluster` in `manager.go`. Lint + vet + full build clean.
**Outputs:**
```
$ go test ./internal/cluster/ -v
=== RUN   TestRoundRobin_DistributionExact
--- PASS: TestRoundRobin_DistributionExact (0.00s)
=== RUN   TestRoundRobin_FirstPickIsEndpoint0
--- PASS: TestRoundRobin_FirstPickIsEndpoint0 (0.00s)
=== RUN   TestRoundRobin_ConcurrentDistributionExact
--- PASS: TestRoundRobin_ConcurrentDistributionExact (0.00s)
=== RUN   TestRoundRobin_ZeroEndpoints
--- PASS: TestRoundRobin_ZeroEndpoints (0.00s)
=== RUN   TestManager_HappyPath_Single
--- PASS: TestManager_HappyPath_Single (0.00s)
=== RUN   TestManager_HappyPath_Multi
--- PASS: TestManager_HappyPath_Multi (0.00s)
=== RUN   TestManager_Error_ZeroClusters
--- PASS: TestManager_Error_ZeroClusters (0.00s)
=== RUN   TestManager_Error_DuplicateName
--- PASS: TestManager_Error_DuplicateName (0.00s)
=== RUN   TestManager_Error_StrictDNS
--- PASS: TestManager_Error_StrictDNS (0.00s)
=== RUN   TestManager_Error_LogicalDNS
--- PASS: TestManager_Error_LogicalDNS (0.00s)
=== RUN   TestManager_Error_EDS
--- PASS: TestManager_Error_EDS (0.00s)
=== RUN   TestManager_Error_OriginalDST
--- PASS: TestManager_Error_OriginalDST (0.00s)
=== RUN   TestManager_Error_NonRoundRobinLB
--- PASS: TestManager_Error_NonRoundRobinLB (0.00s)
=== RUN   TestManager_Error_ZeroEndpoints
--- PASS: TestManager_Error_ZeroEndpoints (0.00s)
=== RUN   TestManager_Error_NonSocketAddressEndpoint
--- PASS: TestManager_Error_NonSocketAddressEndpoint (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.003s
$ go vet ./internal/cluster/
$ golangci-lint run ./internal/cluster/
```
