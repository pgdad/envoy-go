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

**Commits:** 958c059cf099e71db0d5114d7e40ebfa54fb7231
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

## Task 4 — internal/filter/tcpproxy: Filter, NewFilter, Handle (pump verbatim from phase 00)

**Commits:** aa9b43f
**Notes:** Created internal/filter/tcpproxy/{doc.go, filter.go, filter_test.go} per PLAN §Task 4. Pump code (netConn, halfClose, bidirectional io.Copy) LIFTED VERBATIM from cmd/envoy-go/main.go:91-119 per ADR-0023 — byte-level identical aside from pump() function-body inlining into Filter.Handle. Appended ADR-0023. TDD: tests fail first (undefined), pass after implementation. 7 tests including TestHandle_BidirectionalEcho (real loopback round-trip). British→US spelling fixes applied to doc.go (`honouring`→`honoring`) and filter.go (`dialled`→`dialed`) per misspell lint.
**Outputs:**
```
$ go test ./internal/filter/tcpproxy/ -v
=== RUN   TestNewFilter_Happy
--- PASS: TestNewFilter_Happy (0.00s)
=== RUN   TestNewFilter_WrongTypeURL
--- PASS: TestNewFilter_WrongTypeURL (0.00s)
=== RUN   TestNewFilter_UnmarshalError
--- PASS: TestNewFilter_UnmarshalError (0.00s)
=== RUN   TestNewFilter_MissingCluster
--- PASS: TestNewFilter_MissingCluster (0.00s)
=== RUN   TestNewFilter_WeightedClustersUnsupported
--- PASS: TestNewFilter_WeightedClustersUnsupported (0.00s)
=== RUN   TestHandle_BidirectionalEcho
--- PASS: TestHandle_BidirectionalEcho (0.00s)
=== RUN   TestHandle_DialFailure_ClosesDownstream
2026/04/23 15:49:19 tcpproxy: dial 127.0.0.1:44815: dial tcp 127.0.0.1:44815: connect: connection refused
--- PASS: TestHandle_DialFailure_ClosesDownstream (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.004s
$ go vet ./internal/filter/tcpproxy/
$ golangci-lint run ./internal/filter/tcpproxy/
```

## Task 6 — internal/listener.Manager: multi-listener build + Start/Stop + inline registry

**Commits:** 4151926
**Notes:** Created internal/listener/{doc.go, manager.go, manager_test.go} per PLAN §Task 6. ADR-0025 codifies phase-02 filter-chain subset. 12 tests: 2 happy + 10 error/unwind. Inline filter registry maps single URL (tcp_proxy.TypeURL) to its constructor. Divergence from verbatim PLAN: acceptLoop signature takes an explicit `net.Listener` argument (capturing `bl.socket` at launch time) to avoid a nil-pointer race when Stop() runs concurrently with goroutine startup; `ListenerInfo` renamed to `Info` (revive stutter lint); British spellings corrected to US (`materialises`→`materializes`, `behaviour`→`behavior`, `materialised`→`materialized`, `Cancelling`→`Canceling`, `cancelled`→`canceled`).
**Outputs:**
```
$ go test ./internal/listener/ -v
=== RUN   TestManager_HappyPath_Single
--- PASS: TestManager_HappyPath_Single (0.00s)
=== RUN   TestManager_HappyPath_Multi
--- PASS: TestManager_HappyPath_Multi (0.00s)
=== RUN   TestManager_Error_ZeroListeners
--- PASS: TestManager_Error_ZeroListeners (0.00s)
=== RUN   TestManager_Error_DuplicateName
--- PASS: TestManager_Error_DuplicateName (0.00s)
=== RUN   TestManager_Error_TwoFilterChains
--- PASS: TestManager_Error_TwoFilterChains (0.00s)
=== RUN   TestManager_Error_NonEmptyFilterChainMatch
--- PASS: TestManager_Error_NonEmptyFilterChainMatch (0.00s)
=== RUN   TestManager_Error_TwoFilters
--- PASS: TestManager_Error_TwoFilters (0.00s)
=== RUN   TestManager_Error_PopulatedTransportSocket
--- PASS: TestManager_Error_PopulatedTransportSocket (0.00s)
=== RUN   TestManager_Error_UnknownFilterTypeURL
--- PASS: TestManager_Error_UnknownFilterTypeURL (0.00s)
=== RUN   TestManager_Error_FilterConstructionPropagated
--- PASS: TestManager_Error_FilterConstructionPropagated (0.00s)
=== RUN   TestManager_Error_NonSocketAddressListener
--- PASS: TestManager_Error_NonSocketAddressListener (0.00s)
=== RUN   TestManager_BindUnwind
--- PASS: TestManager_BindUnwind (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener	0.004s
$ go vet ./internal/listener/
$ golangci-lint run ./internal/listener/
```

## Task 5 — internal/filter/tcpproxy.FuzzTcpProxyFilter (gate (d), ADR-0018 budget)

**Commits:** e01161e
**Notes:** Created internal/filter/tcpproxy/fuzz_test.go. 3-entry seed corpus per SPEC §4.1: well-formed TcpProxy, wrong type_url, malformed bytes. Widened mkClusterMgr to testing.TB so *testing.F can call it. No new ADR (CI budget inherited from ADR-0018). Fuzz run at 30s CI budget clean.
**Outputs:**
```
$ go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/34 completed
fuzz: elapsed: 0s, gathering baseline coverage: 34/34 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 200128 (66709/sec), new interesting: 57 (total: 91)
fuzz: elapsed: 6s, execs: 710448 (170094/sec), new interesting: 135 (total: 169)
fuzz: elapsed: 9s, execs: 1309046 (199496/sec), new interesting: 175 (total: 209)
fuzz: elapsed: 12s, execs: 2038466 (243174/sec), new interesting: 212 (total: 246)
fuzz: elapsed: 15s, execs: 2632610 (198026/sec), new interesting: 237 (total: 271)
fuzz: elapsed: 18s, execs: 3216653 (194725/sec), new interesting: 259 (total: 293)
fuzz: elapsed: 21s, execs: 3737253 (173523/sec), new interesting: 278 (total: 312)
fuzz: elapsed: 24s, execs: 4441832 (234802/sec), new interesting: 297 (total: 331)
fuzz: elapsed: 27s, execs: 4996273 (184826/sec), new interesting: 309 (total: 343)
fuzz: elapsed: 30s, execs: 5468013 (157253/sec), new interesting: 323 (total: 357)
fuzz: elapsed: 31s, execs: 5468013 (0/sec), new interesting: 323 (total: 357)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.039s
$ go test ./internal/filter/tcpproxy/ -v
=== RUN   TestNewFilter_Happy
--- PASS: TestNewFilter_Happy (0.00s)
=== RUN   TestNewFilter_WrongTypeURL
--- PASS: TestNewFilter_WrongTypeURL (0.00s)
=== RUN   TestNewFilter_UnmarshalError
--- PASS: TestNewFilter_UnmarshalError (0.00s)
=== RUN   TestNewFilter_MissingCluster
--- PASS: TestNewFilter_MissingCluster (0.00s)
=== RUN   TestNewFilter_WeightedClustersUnsupported
--- PASS: TestNewFilter_WeightedClustersUnsupported (0.00s)
=== RUN   TestHandle_BidirectionalEcho
--- PASS: TestHandle_BidirectionalEcho (0.00s)
=== RUN   TestHandle_DialFailure_ClosesDownstream
2026/04/23 15:52:19 tcpproxy: dial 127.0.0.1:33277: dial tcp 127.0.0.1:33277: connect: connection refused
--- PASS: TestHandle_DialFailure_ClosesDownstream (0.00s)
=== RUN   FuzzTcpProxyFilter
=== RUN   FuzzTcpProxyFilter/seed#0
=== RUN   FuzzTcpProxyFilter/seed#1
=== RUN   FuzzTcpProxyFilter/seed#2
--- PASS: FuzzTcpProxyFilter (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#0 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#1 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.004s
$ go vet ./internal/filter/tcpproxy/
$ golangci-lint run ./internal/filter/tcpproxy/
```

## Task 7 — Cutover: cmd/envoy-go rewire + harness + fixture interface + bootstrap deletions

**Commits:** 1143d8d
**Notes:** Atomic cutover landing ADR-0022 (retire `internal/bootstrap.First{Listener,ClusterEndpoint}Socket`; listener/cluster traversal moves into `internal/listener.Manager` and `internal/cluster.Manager`) and ADR-0026 (per-listener ready-sentinel format: one `envoy-go listener <name> ready on <addr>` line per listener followed by a terminal `envoy-go ready`; no backward-compat). `cmd/envoy-go/main.go` rewired to build `cluster.NewManager` + `listener.NewManager(bs, cm)`, start admin, `lm.Start(ctx)`, emit per-listener + terminal sentinels, block on SIGINT. The phase-00 `pump`/`halfClose`/`netConn` copies in main.go were already lifted to `internal/filter/tcpproxy/` in Task 4; Task 7 deletes the redundant copies (main.go: 119 → 80 lines). `internal/bootstrap/bootstrap.go` loses `FirstListenerSocket` (22 lines) and `FirstClusterEndpointSocket` (38 lines); `bootstrap_test.go` loses the five corresponding `TestFirst*` tests. `test/differential/harness.go` swaps `scanForLine`+`readyAddr` for `readyListenerAddrs(ctx, r) (map[string]string, error)` and `SubjectProxy.ListenerAddr(name) string`; `test/differential/fixture.Driver` gains `BackendCount() int` + `SubjectListenerName() string` and an optional `DistributionAsserter` interface; `test/differential/runner_test.go` allocates N backends with per-backend `*atomic.Uint64` counters, drives ref and subj separately (empty-string sentinel), and optionally calls `AssertDistribution`. Fixture 0000 driver rewritten for the new interface; `refBootstrap` const deleted (template now rendered inline with `fmt.Sprintf`). Net diff: -377 / +297 lines across 9 files.
**Outputs:**
```
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.502s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.039s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.004s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.005s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.005s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	0.060s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
```

## Task 8 — BEHAVIOR_CONTRACT.md: TCP proxy subsection

**Commits:** de2f06e
**Notes:** Appended new `## TCP proxy` top-level H2 subsection to BEHAVIOR_CONTRACT.md covering: response-body byte-equivalence (asserted), half-close propagation (asserted), LB endpoint-selection sequence (NOT asserted with rationale), listener-bind error semantics (asserted). References ADR-0023, ADR-0024, SPEC §5.4/5.5/5.8. No code change.

## Task 9 — Fixture 0001-tcp-proxy-rr: bootstraps + driver + AssertDistribution [ADR-0027]

**Commits:** 9fc9be8
**Notes:** Created test/fixtures/0001-tcp-proxy-rr/{envoy.yaml, envoy-go.yaml, expectations.yaml, README.md, driver/driver.go, driver/driver_test.go}. Driver declares BackendCount=3, SubjectListenerName=l_tcp, ReferenceListenerPort=15001, implements DistributionAsserter (per-proxy exact [3,3,3] over 9 requests). Extracted phase-01 probeReady into test/helpers.HTTPGetReadyRaw (shared by fixtures 0000 + 0001 — ADR-0027). Added blank import in test/differential/runner_test.go. ADR-0027 codifies STRICT_DNS (ref) / STATIC (subj) divergence. 4 driver_test.go cases pass; -short test suite all green.
**Outputs:**
```
$ go test ./test/fixtures/0001-tcp-proxy-rr/driver/ -v
=== RUN   TestAssertDistribution_Exact
--- PASS: TestAssertDistribution_Exact (0.00s)
=== RUN   TestAssertDistribution_Imbalanced
--- PASS: TestAssertDistribution_Imbalanced (0.00s)
=== RUN   TestAssertDistribution_AllZero
--- PASS: TestAssertDistribution_AllZero (0.00s)
=== RUN   TestAssertDistribution_WrongLength
--- PASS: TestAssertDistribution_WrongLength (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
$ go vet ./...
$ golangci-lint run ./...
$ go test -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.517s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	(cached)
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	(cached)
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	(cached)
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	0.069s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
```

## Task 10 — All-gates green run

**Commits:** 40464d2 (differential-gate fix — deterministic payloads + `--concurrency 1` [ADR-0028]); <sha> (this Task-10 PROGRESS commit)
**Notes:** All six SPEC §3 phase-done gates pass locally. Gate-run exposed two real bugs in the Task 7 cutover; fixed in commit 40464d2 under ADR-0028 before the final green run below. (a) Fixture `0001-tcp-proxy-rr` byte-exact response-body equivalence + per-proxy AssertDistribution exactly [3,3,3] (requires reference `--concurrency 1`). (b) Fixture `0000-tcp-echo` unchanged pass. (c) No conformance suites apply (vacuously green). (d) `FuzzBootstrapLoad` + `FuzzTcpProxyFilter` both clean at the ADR-0018 30s CI budget. (e) `go build`, `go vet`, `golangci-lint run`, `go test -short ./...` all clean. (f) deferred to review (state 5). The verification session (lifecycle state 4) will re-run these.
**Outputs:**
```
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.478s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.038s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.007s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.004s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.005s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.004s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	0.066s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s

$ go test ./internal/bootstrap/ -run "^TestNothing$" -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/634 completed
fuzz: elapsed: 3s, gathering baseline coverage: 591/634 completed
fuzz: elapsed: 3s, gathering baseline coverage: 634/634 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 185649 (61723/sec), new interesting: 30 (total: 664)
fuzz: elapsed: 9s, execs: 246341 (20195/sec), new interesting: 49 (total: 683)
fuzz: elapsed: 12s, execs: 276015 (9866/sec), new interesting: 56 (total: 690)
fuzz: elapsed: 15s, execs: 283713 (2577/sec), new interesting: 61 (total: 695)
fuzz: elapsed: 18s, execs: 352210 (22834/sec), new interesting: 63 (total: 697)
fuzz: elapsed: 21s, execs: 447889 (31900/sec), new interesting: 71 (total: 705)
fuzz: elapsed: 24s, execs: 458440 (3517/sec), new interesting: 75 (total: 709)
fuzz: elapsed: 27s, execs: 601935 (47832/sec), new interesting: 76 (total: 710)
fuzz: elapsed: 30s, execs: 739357 (45794/sec), new interesting: 77 (total: 711)
fuzz: elapsed: 31s, execs: 739357 (0/sec), new interesting: 77 (total: 711)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.121s

$ go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/357 completed
fuzz: elapsed: 2s, gathering baseline coverage: 357/357 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 24677 (8219/sec), new interesting: 0 (total: 357)
fuzz: elapsed: 6s, execs: 328266 (101276/sec), new interesting: 3 (total: 360)
fuzz: elapsed: 9s, execs: 601005 (90912/sec), new interesting: 7 (total: 364)
fuzz: elapsed: 12s, execs: 862716 (87141/sec), new interesting: 12 (total: 369)
fuzz: elapsed: 15s, execs: 1047070 (61390/sec), new interesting: 17 (total: 374)
fuzz: elapsed: 18s, execs: 1273589 (75665/sec), new interesting: 19 (total: 376)
fuzz: elapsed: 21s, execs: 1455881 (60697/sec), new interesting: 23 (total: 380)
fuzz: elapsed: 24s, execs: 1639630 (61308/sec), new interesting: 24 (total: 381)
fuzz: elapsed: 27s, execs: 1981813 (114051/sec), new interesting: 30 (total: 387)
fuzz: elapsed: 30s, execs: 2277737 (98638/sec), new interesting: 33 (total: 390)
fuzz: elapsed: 31s, execs: 2277737 (0/sec), new interesting: 33 (total: 390)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.063s

$ go test ./test/differential/ -v -timeout=10m
=== RUN   TestCompareBytes_Equal
--- PASS: TestCompareBytes_Equal (0.00s)
=== RUN   TestCompareBytes_DivergesAtFirstByte
--- PASS: TestCompareBytes_DivergesAtFirstByte (0.00s)
=== RUN   TestCompareBytes_DifferentLengths
--- PASS: TestCompareBytes_DifferentLengths (0.00s)
=== RUN   TestParseEnvoyTarget_PullsTagAndDigest
--- PASS: TestParseEnvoyTarget_PullsTagAndDigest (0.00s)
=== RUN   TestParseEnvoyTarget_RejectsMissingTag
--- PASS: TestParseEnvoyTarget_RejectsMissingTag (0.00s)
=== RUN   TestReferenceProxy_Starts
--- PASS: TestReferenceProxy_Starts (0.87s)
=== RUN   TestSubjectProxy_StartsAndReports
--- PASS: TestSubjectProxy_StartsAndReports (0.48s)
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
=== RUN   TestDifferential/0001-tcp-proxy-rr
--- PASS: TestDifferential (2.22s)
    --- PASS: TestDifferential/0000-tcp-echo (1.09s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.12s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	3.648s
```
