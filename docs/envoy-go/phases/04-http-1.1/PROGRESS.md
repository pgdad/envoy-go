# Phase 04 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/PROGRESS.md structure.

## Preamble — execution preconditions

None. All preconditions were satisfied at cold-start. Docker client and server both present and responsive. Go 1.26.2 satisfies 1.23+ requirement. golangci-lint v1.64.8 matches ADR-0009. go-control-plane/envoy pinned at v1.32.4 per ADR-0013. DECISIONS.md tail is `## ADR-0036:` — no re-numbering needed. `--concurrency` in `test/differential/harness.go` is unconditional (line 117, inside `StartReferenceProxy` function, no fixture gate). Phase-03 I-1..I-4 fixes confirmed present in HEAD (commits 98cc35b and cbfe275 visible in log).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** ae52f36
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-03 I-1..I-4 fixes confirmed present in HEAD.
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/04-http-1.1-impl
$ git log -1 --format=%H
c6f13c3f47a2d99e1e39564be07a1a7ee5351ada
$ docker version
Client: Docker Engine - Community
 Version:           28.4.0
Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
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
ok  	github.com/esalaine/envoy-go/internal/tls	(cached)
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	(cached)
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	(cached)
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
$ go list -m github.com/envoyproxy/go-control-plane/envoy
github.com/envoyproxy/go-control-plane/envoy v1.32.4
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0036: BEHAVIOR_CONTRACT TLS subsection (phase 03) + TCP-proxy ADR-0028 cross-reference (Minor 8)
$ git log --oneline -- internal/tls/params.go internal/listener/manager.go test/fixtures/0002-tls-tcp/driver/driver.go | head -20
98cc35b phase 03: REVIEW.md follow-ups (I-1..I-4 from REVIEW.md d45c467)
9b5baa4 phase 03: fixture 0002-tls-tcp — 2-SNI downstream TLS termination + SNI routing + RR distribution
1c7dc31 phase 03: internal/listener — multi-chain + SNI routing via GetConfigForClient [ADR-0033 supersedes ADR-0025]
71b4972 phase 03: internal/tls — applyTLSParams + TLS parameter mapping [ADR-0030]
4151926 phase 02: internal/listener.Manager — multi-listener build + Start/Stop [ADR-0025]
$ grep -n -- '--concurrency' test/differential/harness.go
112:		// --concurrency 1 forces a single worker thread so round-robin LB
117:		Cmd:        []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn", "--concurrency", "1"},
```

## Task 2 — internal/filter/hcm — package skeleton + internal/http amendment

**Commits:** c33d3c8
**Notes:** Doc-only kickoff; no symbols. Settles SPEC §10 #10 (kept the placeholder, amended the doc).
**Outputs:**
```
$ go build ./internal/filter/hcm/... ./internal/http/...
<no output>
$ go vet ./internal/filter/hcm/... ./internal/http/...
<no output>
```

## Task 3 — internal/filter/hcm — codec.go + ADR-0037

**Commits:** dcc6b40
**Notes:** Wire-codec helpers landed; ADR-0037 documents the H2 (stdlib net/http) choice and the residual divergences from upstream Envoy.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run 'TestServerHeader|TestDateHeader|TestWriteStatusReply' -v .
=== RUN   TestServerHeader
--- PASS: TestServerHeader (0.00s)
=== RUN   TestDateHeader
--- PASS: TestDateHeader (0.00s)
=== RUN   TestWriteStatusReply
=== RUN   TestWriteStatusReply/200_OK_with_body
=== RUN   TestWriteStatusReply/400_Bad_Request_empty_body
=== RUN   TestWriteStatusReply/404_Not_Found
=== RUN   TestWriteStatusReply/417_Expectation_Failed_empty
=== RUN   TestWriteStatusReply/500_Internal_Server_Error_empty
=== RUN   TestWriteStatusReply/502_Bad_Gateway_empty
=== RUN   TestWriteStatusReply/503_Service_Unavailable_empty
=== RUN   TestWriteStatusReply/501_Not_Implemented_empty
--- PASS: TestWriteStatusReply (0.00s)
    --- PASS: TestWriteStatusReply/200_OK_with_body (0.00s)
    --- PASS: TestWriteStatusReply/400_Bad_Request_empty_body (0.00s)
    --- PASS: TestWriteStatusReply/404_Not_Found (0.00s)
    --- PASS: TestWriteStatusReply/417_Expectation_Failed_empty (0.00s)
    --- PASS: TestWriteStatusReply/500_Internal_Server_Error_empty (0.00s)
    --- PASS: TestWriteStatusReply/502_Bad_Gateway_empty (0.00s)
    --- PASS: TestWriteStatusReply/503_Service_Unavailable_empty (0.00s)
    --- PASS: TestWriteStatusReply/501_Not_Implemented_empty (0.00s)
=== RUN   TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason
--- PASS: TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	(cached)
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```
## Task 4 — internal/filter/hcm — route.go + ADR-0038

**Commits:** d57ae8b
**Notes:** Route table + match engine landed; ADR-0038 records the prefix/path subset + the bytewise-vs-segment-aware divergence on prefix.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run 'TestMatch|TestRouteTable' -v .
=== RUN   TestMatchPath
--- PASS: TestMatchPath (0.00s)
=== RUN   TestMatchPrefix
--- PASS: TestMatchPrefix (0.00s)
=== RUN   TestRouteTableMatch_FirstMatchWins
--- PASS: TestRouteTableMatch_FirstMatchWins (0.00s)
=== RUN   TestRouteTableMatch_QueryStringExcluded
--- PASS: TestRouteTableMatch_QueryStringExcluded (0.00s)
=== RUN   TestRouteTableMatch_NoMatch
--- PASS: TestRouteTableMatch_NoMatch (0.00s)
=== RUN   TestRouteTableMatch_EmptyTable
--- PASS: TestRouteTableMatch_EmptyTable (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.002s
$ go vet ./internal/filter/hcm/...
$ golangci-lint run ./internal/filter/hcm/...
```
## Task 5 — internal/filter/hcm — actions.go + ADR-0039

**Commits:** 95ea7e8
**Notes:** directResponseAction + routerAction landed; ADR-0039 records the per-request fresh-dial choice.
**Outputs:**
```
$ cd internal/filter/hcm && go test -v .
=== RUN   TestDirectResponseAction_Do
--- PASS: TestDirectResponseAction_Do (0.00s)
=== RUN   TestRouterAction_DoHappy
--- PASS: TestRouterAction_DoHappy (0.00s)
=== RUN   TestRouterAction_DoDialFailureReturns503
--- PASS: TestRouterAction_DoDialFailureReturns503 (0.00s)
=== RUN   TestRouterAction_DoCtxCancel
--- PASS: TestRouterAction_DoCtxCancel (0.00s)
=== RUN   TestServerHeader
--- PASS: TestServerHeader (0.00s)
=== RUN   TestDateHeader
--- PASS: TestDateHeader (0.00s)
=== RUN   TestWriteStatusReply
=== RUN   TestWriteStatusReply/200_OK_with_body
=== RUN   TestWriteStatusReply/400_Bad_Request_empty_body
=== RUN   TestWriteStatusReply/404_Not_Found
=== RUN   TestWriteStatusReply/417_Expectation_Failed_empty
=== RUN   TestWriteStatusReply/500_Internal_Server_Error_empty
=== RUN   TestWriteStatusReply/502_Bad_Gateway_empty
=== RUN   TestWriteStatusReply/503_Service_Unavailable_empty
=== RUN   TestWriteStatusReply/501_Not_Implemented_empty
--- PASS: TestWriteStatusReply (0.00s)
    --- PASS: TestWriteStatusReply/200_OK_with_body (0.00s)
    --- PASS: TestWriteStatusReply/400_Bad_Request_empty_body (0.00s)
    --- PASS: TestWriteStatusReply/404_Not_Found (0.00s)
    --- PASS: TestWriteStatusReply/417_Expectation_Failed_empty (0.00s)
    --- PASS: TestWriteStatusReply/500_Internal_Server_Error_empty (0.00s)
    --- PASS: TestWriteStatusReply/502_Bad_Gateway_empty (0.00s)
    --- PASS: TestWriteStatusReply/503_Service_Unavailable_empty (0.00s)
    --- PASS: TestWriteStatusReply/501_Not_Implemented_empty (0.00s)
=== RUN   TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason
--- PASS: TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason (0.00s)
=== RUN   TestMatchPath
--- PASS: TestMatchPath (0.00s)
=== RUN   TestMatchPrefix
--- PASS: TestMatchPrefix (0.00s)
=== RUN   TestRouteTableMatch_FirstMatchWins
--- PASS: TestRouteTableMatch_FirstMatchWins (0.00s)
=== RUN   TestRouteTableMatch_QueryStringExcluded
--- PASS: TestRouteTableMatch_QueryStringExcluded (0.00s)
=== RUN   TestRouteTableMatch_NoMatch
--- PASS: TestRouteTableMatch_NoMatch (0.00s)
=== RUN   TestRouteTableMatch_EmptyTable
--- PASS: TestRouteTableMatch_EmptyTable (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	(cached)
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```
## Task 6 — internal/filter/hcm — connection.go

**Commits:** 7359397
**Notes:** Per-conn HTTP/1.1 loop landed; out-of-scope guards (Expect→417, Upgrade→501) + 404 + 400 + bodydrain + keep-alive verified by table-driven loopback tests.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run 'TestRunConnection' -v .
=== RUN   TestRunConnection_DirectResponseHappy
--- PASS: TestRunConnection_DirectResponseHappy (0.00s)
=== RUN   TestRunConnection_KeepAliveTwoRequests
--- PASS: TestRunConnection_KeepAliveTwoRequests (0.00s)
=== RUN   TestRunConnection_RouteNotFoundReturns404
--- PASS: TestRunConnection_RouteNotFoundReturns404 (0.00s)
=== RUN   TestRunConnection_ExpectHeaderReturns417
--- PASS: TestRunConnection_ExpectHeaderReturns417 (0.00s)
=== RUN   TestRunConnection_UpgradeReturns501
--- PASS: TestRunConnection_UpgradeReturns501 (0.00s)
=== RUN   TestRunConnection_BadRequestReturns400
--- PASS: TestRunConnection_BadRequestReturns400 (0.00s)
=== RUN   TestRunConnection_BodyDrainedBetweenRequests
--- PASS: TestRunConnection_BodyDrainedBetweenRequests (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.004s
$ cd internal/filter/hcm && go test -race -run 'TestRunConnection' .
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.013s
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```

## Task 7 — internal/filter/hcm — config.go + ADR-0040 + ADR-0041 + ADR-0042

**Commits:** ab6dc50
**Notes:** Typed_config parser landed; three ADRs in one commit (per phase-02 Task 7 precedent). Filter struct declared in config.go (deviation from PLAN's literal "filter.go declares Filter") to preserve the phased-commit compile-clean discipline; Task 8 will add NewFilter + Handle methods on top of this struct.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run 'TestParseFilter' -v .
=== RUN   TestParseFilter_Happy
--- PASS: TestParseFilter_Happy (0.00s)
=== RUN   TestParseFilter_WrongTypeURL
--- PASS: TestParseFilter_WrongTypeURL (0.00s)
=== RUN   TestParseFilter_CodecTypeHTTP2
--- PASS: TestParseFilter_CodecTypeHTTP2 (0.00s)
=== RUN   TestParseFilter_CodecTypeHTTP3
--- PASS: TestParseFilter_CodecTypeHTTP3 (0.00s)
=== RUN   TestParseFilter_CodecTypeAUTO
--- PASS: TestParseFilter_CodecTypeAUTO (0.00s)
=== RUN   TestParseFilter_MissingStatPrefix
--- PASS: TestParseFilter_MissingStatPrefix (0.00s)
=== RUN   TestParseFilter_RDSRouteSpecifier
--- PASS: TestParseFilter_RDSRouteSpecifier (0.00s)
=== RUN   TestParseFilter_ScopedRoutes
--- PASS: TestParseFilter_ScopedRoutes (0.00s)
=== RUN   TestParseFilter_ZeroVirtualHosts
--- PASS: TestParseFilter_ZeroVirtualHosts (0.00s)
=== RUN   TestParseFilter_TwoVirtualHosts
--- PASS: TestParseFilter_TwoVirtualHosts (0.00s)
=== RUN   TestParseFilter_VHostDomainsEmpty
--- PASS: TestParseFilter_VHostDomainsEmpty (0.00s)
=== RUN   TestParseFilter_VHostDomainsNotStarOnly
--- PASS: TestParseFilter_VHostDomainsNotStarOnly (0.00s)
=== RUN   TestParseFilter_HTTPFiltersEmpty
--- PASS: TestParseFilter_HTTPFiltersEmpty (0.00s)
=== RUN   TestParseFilter_HTTPFiltersTwoEntries
--- PASS: TestParseFilter_HTTPFiltersTwoEntries (0.00s)
=== RUN   TestParseFilter_HTTPFiltersWrongName
--- PASS: TestParseFilter_HTTPFiltersWrongName (0.00s)
=== RUN   TestParseFilter_HTTPFiltersWrongTypeURL
--- PASS: TestParseFilter_HTTPFiltersWrongTypeURL (0.00s)
=== RUN   TestParseFilter_RouteUnknownAction
--- PASS: TestParseFilter_RouteUnknownAction (0.00s)
=== RUN   TestParseFilter_RouteSafeRegex
--- PASS: TestParseFilter_RouteSafeRegex (0.00s)
=== RUN   TestParseFilter_RoutePathSeparatedPrefix
--- PASS: TestParseFilter_RoutePathSeparatedPrefix (0.00s)
=== RUN   TestParseFilter_RouteCaseSensitiveFalse
--- PASS: TestParseFilter_RouteCaseSensitiveFalse (0.00s)
=== RUN   TestParseFilter_RouteHeadersSet
--- PASS: TestParseFilter_RouteHeadersSet (0.00s)
=== RUN   TestParseFilter_RouteQueryParamsSet
--- PASS: TestParseFilter_RouteQueryParamsSet (0.00s)
=== RUN   TestParseFilter_RouteRuntimeFraction
--- PASS: TestParseFilter_RouteRuntimeFraction (0.00s)
=== RUN   TestParseFilter_DirectResponseStatusZero
--- PASS: TestParseFilter_DirectResponseStatusZero (0.00s)
=== RUN   TestParseFilter_DirectResponseStatus600
--- PASS: TestParseFilter_DirectResponseStatus600 (0.00s)
=== RUN   TestParseFilter_DirectResponseInlineBytes
--- PASS: TestParseFilter_DirectResponseInlineBytes (0.00s)
=== RUN   TestParseFilter_DirectResponseFilename
--- PASS: TestParseFilter_DirectResponseFilename (0.00s)
=== RUN   TestParseFilter_DirectResponseEmptyBody
--- PASS: TestParseFilter_DirectResponseEmptyBody (0.00s)
=== RUN   TestParseFilter_RouterActionWeightedClusters
--- PASS: TestParseFilter_RouterActionWeightedClusters (0.00s)
=== RUN   TestParseFilter_RouterActionClusterHeader
--- PASS: TestParseFilter_RouterActionClusterHeader (0.00s)
=== RUN   TestParseFilter_RouterActionUnknownCluster
--- PASS: TestParseFilter_RouterActionUnknownCluster (0.00s)
=== RUN   TestParseFilter_RouterActionHappy
--- PASS: TestParseFilter_RouterActionHappy (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.005s
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```

## Task 8 — internal/filter/hcm — filter.go + listener manager HCM registration

**Commits:** c308cb8
**Notes:** NewFilter + Handle landed on top of the Task-7 Filter struct; listener manager filterRegistry now registers HCM type_url alongside tcpproxy. Listener-wrap discipline (`listener: %q: filter_chains[%d]: hcm: ...`) verified end-to-end. One deviation from spec text: "cancelled" → "canceled" (US spelling) required by golangci-lint misspell linter.
**Outputs:**
```
$ cd internal/filter/hcm && go test -v .
=== RUN   TestParseFilter_RouteHeadersSet
--- PASS: TestParseFilter_RouteHeadersSet (0.00s)
=== RUN   TestParseFilter_RouteQueryParamsSet
--- PASS: TestParseFilter_RouteQueryParamsSet (0.00s)
=== RUN   TestParseFilter_RouteRuntimeFraction
--- PASS: TestParseFilter_RouteRuntimeFraction (0.00s)
=== RUN   TestParseFilter_DirectResponseStatusZero
--- PASS: TestParseFilter_DirectResponseStatusZero (0.00s)
=== RUN   TestParseFilter_DirectResponseStatus600
--- PASS: TestParseFilter_DirectResponseStatus600 (0.00s)
=== RUN   TestParseFilter_DirectResponseInlineBytes
--- PASS: TestParseFilter_DirectResponseInlineBytes (0.00s)
=== RUN   TestParseFilter_DirectResponseFilename
--- PASS: TestParseFilter_DirectResponseFilename (0.00s)
=== RUN   TestParseFilter_DirectResponseEmptyBody
--- PASS: TestParseFilter_DirectResponseEmptyBody (0.00s)
=== RUN   TestParseFilter_RouterActionWeightedClusters
--- PASS: TestParseFilter_RouterActionWeightedClusters (0.00s)
=== RUN   TestParseFilter_RouterActionClusterHeader
--- PASS: TestParseFilter_RouterActionClusterHeader (0.00s)
=== RUN   TestParseFilter_RouterActionUnknownCluster
--- PASS: TestParseFilter_RouterActionUnknownCluster (0.00s)
=== RUN   TestParseFilter_RouterActionHappy
--- PASS: TestParseFilter_RouterActionHappy (0.00s)
=== RUN   TestRunConnection_DirectResponseHappy
--- PASS: TestRunConnection_DirectResponseHappy (0.00s)
=== RUN   TestRunConnection_KeepAliveTwoRequests
--- PASS: TestRunConnection_KeepAliveTwoRequests (0.00s)
=== RUN   TestRunConnection_RouteNotFoundReturns404
--- PASS: TestRunConnection_RouteNotFoundReturns404 (0.00s)
=== RUN   TestRunConnection_ExpectHeaderReturns417
--- PASS: TestRunConnection_ExpectHeaderReturns417 (0.00s)
=== RUN   TestRunConnection_UpgradeReturns501
--- PASS: TestRunConnection_UpgradeReturns501 (0.00s)
=== RUN   TestRunConnection_BadRequestReturns400
--- PASS: TestRunConnection_BadRequestReturns400 (0.00s)
=== RUN   TestRunConnection_BodyDrainedBetweenRequests
--- PASS: TestRunConnection_BodyDrainedBetweenRequests (0.00s)
=== RUN   TestNewFilter_HappyPath
--- PASS: TestNewFilter_HappyPath (0.00s)
=== RUN   TestNewFilter_PreservesParseErrorPrefix
--- PASS: TestNewFilter_PreservesParseErrorPrefix (0.00s)
=== RUN   TestFilter_Handle_OneRequestThenEOF
--- PASS: TestFilter_Handle_OneRequestThenEOF (0.00s)
=== RUN   TestFilter_Handle_CtxAlreadyCancelledShortCircuits
--- PASS: TestFilter_Handle_CtxAlreadyCancelledShortCircuits (0.00s)
=== RUN   TestMatchPath
--- PASS: TestMatchPath (0.00s)
=== RUN   TestMatchPrefix
--- PASS: TestMatchPrefix (0.00s)
=== RUN   TestRouteTableMatch_FirstMatchWins
--- PASS: TestRouteTableMatch_FirstMatchWins (0.00s)
=== RUN   TestRouteTableMatch_QueryStringExcluded
--- PASS: TestRouteTableMatch_QueryStringExcluded (0.00s)
=== RUN   TestRouteTableMatch_NoMatch
--- PASS: TestRouteTableMatch_NoMatch (0.00s)
=== RUN   TestRouteTableMatch_EmptyTable
--- PASS: TestRouteTableMatch_EmptyTable (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.007s
$ go test -race ./internal/listener/...
ok  	github.com/esalaine/envoy-go/internal/listener	1.025s
$ go vet ./internal/listener/... ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/listener/... ./internal/filter/hcm/...
<no output>
```

## Task 9 — internal/bootstrap — HCM/router/route-config blank imports

**Commits:** 6857383
**Notes:** Three blank imports added per ADR-0016 (registry-population mechanism, not new ADR). HCM-bootstrap protojson round-trip test added.
**Outputs:**
```
$ go test -run TestLoad_HCMRoundTrip -v ./internal/bootstrap/...
=== RUN   TestLoad_HCMRoundTrip
--- PASS: TestLoad_HCMRoundTrip (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s

$ go test ./internal/bootstrap/...
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.007s

$ go vet ./internal/bootstrap/...
<no output>

$ golangci-lint run ./internal/bootstrap/...
<no output>
```

## Task 10 — cmd/envoy-go — HCM smoke variant

**Commits:** 951c90f
**Notes:** End-to-end test confirms binary serves an HCM direct_response over HTTP/1.1. Minimal bootstrap includes a dummy cluster (required by cluster manager validation) but traffic routes to direct_response without cluster lookup.
**Outputs:**
```
$ go test -run TestEnvoyGoBinary_HCMSmoke -v ./cmd/envoy-go/
=== RUN   TestEnvoyGoBinary_HCMSmoke
--- PASS: TestEnvoyGoBinary_HCMSmoke (0.57s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.576s

$ go test ./cmd/envoy-go/...
=== RUN   TestEnvoyGoBinary_TwoListenerCutover
--- PASS: TestEnvoyGoBinary_TwoListenerCutover (0.54s)
=== RUN   TestEnvoyGoBinary_HCMSmoke
--- PASS: TestEnvoyGoBinary_HCMSmoke (0.51s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	1.048s

$ go vet ./cmd/envoy-go/...
<no output>

$ golangci-lint run ./cmd/envoy-go/...
<no output>
```

## Task 11 — test/helpers — HTTPRoundTrip

**Commits:** ab4520f
**Notes:** HTTP/1.1 single-request round-trip helper landed; sets Connection: close by default for deterministic per-request RR partitioning per ADR-0039.
**Outputs:**
```
$ cd test/helpers && go test -run TestHTTPRoundTrip -v .
=== RUN   TestHTTPRoundTrip_Happy
--- PASS: TestHTTPRoundTrip_Happy (0.00s)
=== RUN   TestHTTPRoundTrip_CtxCanceledBeforeDial
--- PASS: TestHTTPRoundTrip_CtxCanceledBeforeDial (0.00s)
=== RUN   TestHTTPRoundTrip_ConnectionRefused
--- PASS: TestHTTPRoundTrip_ConnectionRefused (0.00s)
=== RUN   TestHTTPRoundTrip_BodyClosedAfterReturn
--- PASS: TestHTTPRoundTrip_BodyClosedAfterReturn (0.00s)
=== RUN   TestHTTPRoundTrip_SetHeaders
--- PASS: TestHTTPRoundTrip_SetHeaders (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
$ go vet ./test/helpers/...
<no output>
$ golangci-lint run ./test/helpers/...
<no output>
```

## Task 12 — test/helpers — HTTPHeaderDiff

**Commits:** d81b38b
**Notes:** Symmetric-difference helper + phase-04 default allow-list. Settles SPEC §10 #7 to fixed in-code list.
**Outputs:**
```
$ go test -run 'TestHTTPHeaderDiff|TestPhaseFourHTTPAllowList' -v ./test/helpers/
=== RUN   TestHTTPHeaderDiff_Identical
--- PASS: TestHTTPHeaderDiff_Identical (0.00s)
=== RUN   TestHTTPHeaderDiff_RefOnlyAndSubjOnly
--- PASS: TestHTTPHeaderDiff_RefOnlyAndSubjOnly (0.00s)
=== RUN   TestHTTPHeaderDiff_AllowListExact
--- PASS: TestHTTPHeaderDiff_AllowListExact (0.00s)
=== RUN   TestHTTPHeaderDiff_AllowListPrefix
--- PASS: TestHTTPHeaderDiff_AllowListPrefix (0.00s)
=== RUN   TestHTTPHeaderDiff_CaseInsensitive
--- PASS: TestHTTPHeaderDiff_CaseInsensitive (0.00s)
=== RUN   TestHTTPHeaderDiff_AllowListCaseInsensitive
--- PASS: TestHTTPHeaderDiff_AllowListCaseInsensitive (0.00s)
=== RUN   TestPhaseFourHTTPAllowList_DefaultEntries
--- PASS: TestPhaseFourHTTPAllowList_DefaultEntries (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
$ go vet ./test/helpers/...
<no output>
$ golangci-lint run ./test/helpers/...
<no output>
```
