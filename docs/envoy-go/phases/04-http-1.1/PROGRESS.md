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

## Task 13 — test/differential — HTTPExpectations interface + runner orchestration

**Commits:** 5bcce5f
**Notes:** Additive optional interface; existing fixtures 0000/0001/0002 unaffected (type assertion fails-silently). ADR-0043 documents the typed-extension choice.
**Outputs:**
```
$ go vet ./test/differential/...
<no output>
$ golangci-lint run ./test/differential/...
<no output>
$ go build ./test/differential/...
<no output>
```

## Task 14 — test/differential — HTTP-echo backend kind

**Commits:** 0acb263
**Notes:** BackendKindAware optional interface added; default TCPEcho preserves fixtures 0000/0001/0002. HTTPEcho branch handcrafts bufio per SPEC §10 #6 settled choice. backend-<idx>:<lastSegmentOfPath> body format settles SPEC §10 #14.
**Outputs:**
```
$ go build ./test/differential/...
<no output>
$ go vet ./test/differential/...
<no output>
$ golangci-lint run ./test/differential/...
<no output>
```

## Task 15 — fixture 0003-http11-routing — bootstraps + driver + tests + README + expectations

**Commits:** 1a87738
**Notes:** End-to-end differential gate green. 27 requests/side, [3,3,3] RR distribution on subject, byte-equivalent decoded /health body stream (9 × "OK\n"), HTTPExpectations status-200 per /api request (body not compared per-request — RR offset diverges between STRICT_DNS and STATIC; distribution asserted via accept counts), HTTPExpectations status-404 per /missing request, admin /ready differential. Deviation from PLAN: /api bodies not concatenated in Drive byte stream (RR starting offset differs between ref STRICT_DNS and subj STATIC); routing correctness proven by AssertDistribution [3,3,3] + HTTPExpectations status-200.
**Outputs:**
```
$ go test ./test/fixtures/0003-http11-routing/...
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.002s
$ go test ./test/differential/... -run 'TestDifferential/0003-http11-routing' -v -timeout=10m
=== RUN   TestDifferential
=== RUN   TestDifferential/0003-http11-routing
2026/04/25 09:27:22 github.com/testcontainers/testcontainers-go - Connected to docker: 
  Server Version: 28.1.1
  API Version: 1.43
  Operating System: Docker Desktop
  Total Memory: 64296 MB
  Resolved Docker Host: unix:///home/esa/.docker/desktop/docker.sock
  Resolved Docker Socket Path: /var/run/docker.sock
  Test SessionID: e79af3f8585643edc4ac093368cee18b2f9467d430999c1067ec333238d1d4d8
  Test ProcessID: c4ac58a8-9897-4eed-bf19-51f3fe2fbe4b
2026/04/25 09:27:22 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/25 09:27:22 ✅ Container created: 26673063c84e
2026/04/25 09:27:22 🐳 Starting container: 26673063c84e
2026/04/25 09:27:22 ✅ Container started: 26673063c84e
2026/04/25 09:27:22 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 09:27:22 ✅ Container created: 55660ad224ec
2026/04/25 09:27:22 🐳 Starting container: 55660ad224ec
2026/04/25 09:27:22 ✅ Container started: 55660ad224ec
2026/04/25 09:27:23 🐳 Terminating container: 55660ad224ec
2026/04/25 09:27:23 🚫 Container terminated: 55660ad224ec
--- PASS: TestDifferential (1.51s)
    --- PASS: TestDifferential/0003-http11-routing (1.51s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.592s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
$ go vet ./test/fixtures/0003-http11-routing/... ./test/differential/...
<no output>
$ golangci-lint run ./test/fixtures/0003-http11-routing/... ./test/differential/...
<no output>
```

## Task 16 — internal/filter/hcm — FuzzHCMConfigParse

**Commits:** 56b29a8
**Notes:** Short-budget fuzz target landed; 30s wall-time per ADR-0018. Three seed corpus entries (well-formed, truncated, wrong type_url). No crashers found in 30s session.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run=^$ -fuzz=FuzzHCMConfigParse -fuzztime=30s .
fuzz: elapsed: 0s, gathering baseline coverage: 0/3 completed
fuzz: elapsed: 0s, gathering baseline coverage: 3/3 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 244102 (81353/sec), new interesting: 42 (total: 45)
fuzz: elapsed: 6s, execs: 639869 (131906/sec), new interesting: 105 (total: 108)
fuzz: elapsed: 9s, execs: 1057174 (139125/sec), new interesting: 142 (total: 145)
fuzz: elapsed: 12s, execs: 1413065 (118622/sec), new interesting: 169 (total: 172)
fuzz: elapsed: 15s, execs: 1847977 (144954/sec), new interesting: 195 (total: 198)
fuzz: elapsed: 18s, execs: 2188299 (113470/sec), new interesting: 214 (total: 217)
fuzz: elapsed: 21s, execs: 2825708 (212415/sec), new interesting: 230 (total: 233)
fuzz: elapsed: 24s, execs: 3229342 (134576/sec), new interesting: 249 (total: 252)
fuzz: elapsed: 27s, execs: 4229961 (333503/sec), new interesting: 259 (total: 262)
fuzz: elapsed: 30s, execs: 4905149 (225094/sec), new interesting: 275 (total: 278)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.054s
$ go test ./internal/filter/hcm/...
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.007s
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```

## Task 17 — BEHAVIOR_CONTRACT update + ADR-0044 + all-gates green final sweep

**Commits:** dc079c2
**Notes:** Phase-04 closing task. BEHAVIOR_CONTRACT extended with `## HTTP/1.1` subsection + Header allow-list rows; ADR-0044 codifies the equivalence surface with the documented body-byte-equivalence relaxation for routed-to-upstream requests (rationale: RR start-index divergence between STATIC and STRICT_DNS).

Gate (a) differential fixtures: PASS — every fixture (0000, 0001, 0002, 0003) green.
Gate (b) unit tests + race: PASS — every package green, no data races.
Gate (c): N/A — phase 04 introduces no new ready-sentinel format.
Gate (d) fuzz targets: every fuzz target completed 30s budget with no crashers.
Gate (e) vet + lint: clean across module.
Gate (f): deferred to verification-before-completion session per BOOTSTRAP §5 step 6.

**Outputs:**
```
$ go test ./test/differential/... -timeout=12m
ok  	github.com/esalaine/envoy-go/test/differential	5.957s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
$ go test -race ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.148s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.062s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.036s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.028s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.029s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.026s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	(cached)
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.076s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	7.203s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.008s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.008s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.007s
ok  	github.com/esalaine/envoy-go/test/helpers	1.014s
$ go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 27s, execs: 792369 (0/sec), new interesting: 33 (total: 905)
fuzz: elapsed: 30s, execs: 792369 (0/sec), new interesting: 33 (total: 905)
fuzz: elapsed: 31s, execs: 792369 (0/sec), new interesting: 33 (total: 905)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.082s
$ go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 27s, execs: 3086279 (113518/sec), new interesting: 8 (total: 508)
fuzz: elapsed: 30s, execs: 3392708 (102135/sec), new interesting: 8 (total: 508)
fuzz: elapsed: 31s, execs: 3392708 (0/sec), new interesting: 8 (total: 508)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.064s
$ go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 27s, execs: 5013301 (347826/sec), new interesting: 32 (total: 449)
fuzz: elapsed: 30s, execs: 7271162 (752552/sec), new interesting: 39 (total: 456)
fuzz: elapsed: 31s, execs: 7271162 (0/sec), new interesting: 39 (total: 456)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.045s
$ go test ./internal/filter/hcm -run=FuzzHCMConfigParse -fuzz=FuzzHCMConfigParse -fuzztime=30s
fuzz: elapsed: 27s, execs: 3916255 (129564/sec), new interesting: 114 (total: 392)
fuzz: elapsed: 30s, execs: 4310890 (131539/sec), new interesting: 120 (total: 398)
fuzz: elapsed: 31s, execs: 4310890 (0/sec), new interesting: 120 (total: 398)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.048s
$ go vet ./...
<no output>
$ golangci-lint run ./...
<no output>
```

## Verification (lifecycle-state 4)

Per `BOOTSTRAP_PROMPT.md` §5 state 4 and `STATE.md`'s `next-skill-scope`: a fresh-session re-run of every SPEC §3 phase-done gate, with each command's verbatim output captured here. This session's HEAD on branch `phase/04-http-1.1-impl` is `13d0373` (= `24a9c88` Task-17 all-gates green sweep + three subsequent STATE.md/PROGRESS.md text-only commits `24a9c88`→`c214d25`→`11a50ce`→`13d0373`; no production code, test, or fixture file changed across those three commits, so the gates evaluated here are evaluating the same compiled artefact set as Task 17's sweep). Worktree: `.worktrees/phase-04-http-1.1-impl`. Verifier date: 2026-04-25.

ADR-0044 runner-vs-contract re-confirmation (per `STATE.md`'s explicit verifier instruction): the 0003-fixture driver at `test/fixtures/0003-http11-routing/driver/driver.go` enforces exactly the contract ADR-0044 prescribes — `/health` direct_response 200 with body byte-equal across both proxies (`ExpectBodyEquivalent: true`, body bytes also concatenated into the per-proxy `Drive` byte stream that the harness diffs); `/api/v1/N` routed-to-upstream with status-only equivalence (`ExpectBodyEquivalent: false`, body intentionally NOT concatenated — the file-level comment names the rationale: "ref and subj RR may start at different endpoints (STRICT_DNS vs STATIC initial pick)"); `/missing/N` 404 catch-all direct_response with status-only equivalence (`ExpectBodyEquivalent: false`, "404 local-reply body differs between Envoy (HTML/JSON) and envoy-go ('not found\n')"). Per-cluster RR distribution `[3, 3, 3]` enforced on the subject side via `AssertDistribution` over the 9 router-action requests. No state-3 re-entry trigger.

Fuzz-seed corpus discipline (ADR-0018): `git status --porcelain` reported empty after all four fuzz runs (verbatim output below); no new interesting inputs persisted under `testdata/fuzz/` (none would be persisted absent a crasher, and no fuzz target crashed).

Gate (a) new differential fixture green — see `go test ./test/differential/ -v -timeout=12m` output below; subtest `TestDifferential/0003-http11-routing` PASS.
Gate (b) all pre-existing differential fixtures green — same output below; subtests `TestDifferential/0000-tcp-echo`, `TestDifferential/0001-tcp-proxy-rr`, `TestDifferential/0002-tls-tcp` all PASS.
Gate (c) conformance suites — N/A at phase 04 per SPEC §3 (h2spec is phase 05; no project-internal h1spec). Vacuously green.
Gate (d) new + carryforward fuzz targets clean for CI short-budget — all four targets PASS at 30s budget; outputs below.
Gate (e) `go vet`, `golangci-lint run`, `go test ./...` clean — outputs below; `go build ./...` also captured (per `STATE.md`'s command list, prepended for completeness).
Gate (f) `REVIEW.md` approved — deferred to lifecycle-state 5 per `BOOTSTRAP_PROMPT.md` §5.

All five applicable gates are green. Lifecycle-state advances to 5 (verified, not reviewed); next-skill is `superpowers:requesting-code-review`.

**Outputs:**
```
$ git -C . rev-parse --abbrev-ref HEAD
phase/04-http-1.1-impl
$ git -C . log -1 --format=%H
13d0373cab73b1b8fbf21b7adc0988094aa42e41
$ go build ./...
<no output>
$ go vet ./...
<no output>
$ golangci-lint run ./...
<no output>
$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	1.162s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.039s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	0.008s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.010s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.009s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.016s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	6.066s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.002s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.005s
$ go test ./internal/bootstrap/ -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/905 completed
fuzz: elapsed: 3s, gathering baseline coverage: 587/905 completed
fuzz: elapsed: 5s, gathering baseline coverage: 905/905 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 147891 (49098/sec), new interesting: 8 (total: 913)
fuzz: elapsed: 9s, execs: 250803 (34309/sec), new interesting: 16 (total: 921)
fuzz: elapsed: 12s, execs: 264908 (4701/sec), new interesting: 17 (total: 922)
fuzz: elapsed: 15s, execs: 264908 (0/sec), new interesting: 17 (total: 922)
fuzz: elapsed: 18s, execs: 264908 (0/sec), new interesting: 17 (total: 922)
fuzz: elapsed: 21s, execs: 398989 (44723/sec), new interesting: 18 (total: 923)
fuzz: elapsed: 24s, execs: 398989 (0/sec), new interesting: 18 (total: 923)
fuzz: elapsed: 27s, execs: 398989 (0/sec), new interesting: 18 (total: 923)
fuzz: elapsed: 30s, execs: 398989 (0/sec), new interesting: 18 (total: 923)
fuzz: elapsed: 31s, execs: 398989 (0/sec), new interesting: 18 (total: 923)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.085s
$ go test ./internal/filter/tcpproxy/ -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/508 completed
fuzz: elapsed: 3s, gathering baseline coverage: 351/508 completed
fuzz: elapsed: 4s, gathering baseline coverage: 508/508 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 296853 (98814/sec), new interesting: 0 (total: 508)
fuzz: elapsed: 9s, execs: 780530 (161276/sec), new interesting: 1 (total: 509)
fuzz: elapsed: 12s, execs: 1277329 (165574/sec), new interesting: 2 (total: 510)
fuzz: elapsed: 15s, execs: 1776067 (166120/sec), new interesting: 3 (total: 511)
fuzz: elapsed: 18s, execs: 2245764 (156690/sec), new interesting: 3 (total: 511)
fuzz: elapsed: 21s, execs: 2709378 (154532/sec), new interesting: 4 (total: 512)
fuzz: elapsed: 24s, execs: 3164813 (151817/sec), new interesting: 5 (total: 513)
fuzz: elapsed: 27s, execs: 3620507 (151904/sec), new interesting: 6 (total: 514)
fuzz: elapsed: 30s, execs: 4071288 (150252/sec), new interesting: 7 (total: 515)
fuzz: elapsed: 31s, execs: 4071288 (0/sec), new interesting: 7 (total: 515)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.056s
$ go test ./internal/tls/ -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/456 completed
fuzz: elapsed: 2s, gathering baseline coverage: 456/456 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 247287 (82421/sec), new interesting: 2 (total: 458)
fuzz: elapsed: 6s, execs: 444887 (65854/sec), new interesting: 9 (total: 465)
fuzz: elapsed: 9s, execs: 532127 (29083/sec), new interesting: 11 (total: 467)
fuzz: elapsed: 12s, execs: 541532 (3135/sec), new interesting: 12 (total: 468)
fuzz: elapsed: 15s, execs: 1407038 (288522/sec), new interesting: 16 (total: 472)
fuzz: elapsed: 18s, execs: 3571200 (721327/sec), new interesting: 28 (total: 484)
fuzz: elapsed: 21s, execs: 4096198 (175028/sec), new interesting: 31 (total: 487)
fuzz: elapsed: 24s, execs: 4333981 (79252/sec), new interesting: 33 (total: 489)
fuzz: elapsed: 27s, execs: 5110512 (258805/sec), new interesting: 37 (total: 493)
fuzz: elapsed: 30s, execs: 8759524 (1216525/sec), new interesting: 47 (total: 503)
fuzz: elapsed: 31s, execs: 8759524 (0/sec), new interesting: 47 (total: 503)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.047s
$ go test ./internal/filter/hcm/ -run=FuzzHCMConfigParse -fuzz=FuzzHCMConfigParse -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/398 completed
fuzz: elapsed: 3s, gathering baseline coverage: 398/398 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 48003 (16001/sec), new interesting: 1 (total: 399)
fuzz: elapsed: 6s, execs: 466208 (139390/sec), new interesting: 6 (total: 404)
fuzz: elapsed: 9s, execs: 936232 (156651/sec), new interesting: 14 (total: 412)
fuzz: elapsed: 12s, execs: 1356509 (140121/sec), new interesting: 19 (total: 417)
fuzz: elapsed: 15s, execs: 1869398 (170929/sec), new interesting: 26 (total: 424)
fuzz: elapsed: 18s, execs: 2294621 (141651/sec), new interesting: 31 (total: 429)
fuzz: elapsed: 21s, execs: 2650238 (118634/sec), new interesting: 36 (total: 434)
fuzz: elapsed: 24s, execs: 3139750 (162969/sec), new interesting: 42 (total: 440)
fuzz: elapsed: 27s, execs: 3564349 (141682/sec), new interesting: 49 (total: 447)
fuzz: elapsed: 30s, execs: 3923538 (119756/sec), new interesting: 53 (total: 451)
fuzz: elapsed: 31s, execs: 3923538 (0/sec), new interesting: 53 (total: 451)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.051s
$ git status --porcelain
<no output>
$ go test ./test/differential/ -v -timeout=12m
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
2026/04/25 11:43:28 github.com/testcontainers/testcontainers-go - Connected to docker: 
  Server Version: 28.1.1
  API Version: 1.43
  Operating System: Docker Desktop
  Total Memory: 64296 MB
  Resolved Docker Host: unix:///home/esa/.docker/desktop/docker.sock
  Resolved Docker Socket Path: /var/run/docker.sock
  Test SessionID: a50a329bf5200fea876ee163907ca1aab0c167450e84daae9133e9f2116e688d
  Test ProcessID: 848a8210-c4c9-493b-8165-9aded1039726
2026/04/25 11:43:28 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/25 11:43:28 ✅ Container created: 82a1302f4f04
2026/04/25 11:43:28 🐳 Starting container: 82a1302f4f04
2026/04/25 11:43:28 ✅ Container started: 82a1302f4f04
2026/04/25 11:43:28 🚧 Waiting for container id 82a1302f4f04 image: testcontainers/ryuk:0.6.0. Waiting for: &{Port:8080/tcp timeout:<nil> PollInterval:100ms}
2026/04/25 11:43:28 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 11:43:28 ✅ Container created: 1a91a7144f9a
2026/04/25 11:43:28 🐳 Starting container: 1a91a7144f9a
2026/04/25 11:43:28 ✅ Container started: 1a91a7144f9a
2026/04/25 11:43:28 🚧 Waiting for container id 1a91a7144f9a image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x1852447de5c0 Port:9901/tcp Path:/ready StatusCodeMatcher:0x864ac0 ResponseMatcher:0x9592a0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/25 11:43:28 🐳 Terminating container: 1a91a7144f9a
2026/04/25 11:43:29 🚫 Container terminated: 1a91a7144f9a
--- PASS: TestReferenceProxy_Starts (0.86s)
=== RUN   TestSubjectProxy_StartsAndReports
--- PASS: TestSubjectProxy_StartsAndReports (0.51s)
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
2026/04/25 11:43:29 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 11:43:29 ✅ Container created: 24e1edca07da
2026/04/25 11:43:29 🐳 Starting container: 24e1edca07da
2026/04/25 11:43:29 ✅ Container started: 24e1edca07da
2026/04/25 11:43:29 🚧 Waiting for container id 24e1edca07da image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x185244765e08 Port:9901/tcp Path:/ready StatusCodeMatcher:0x864ac0 ResponseMatcher:0x9592a0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/25 11:43:30 🐳 Terminating container: 24e1edca07da
2026/04/25 11:43:30 🚫 Container terminated: 24e1edca07da
=== RUN   TestDifferential/0001-tcp-proxy-rr
2026/04/25 11:43:30 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 11:43:30 ✅ Container created: f8afba78be8d
2026/04/25 11:43:30 🐳 Starting container: f8afba78be8d
2026/04/25 11:43:30 ✅ Container started: f8afba78be8d
2026/04/25 11:43:30 🚧 Waiting for container id f8afba78be8d image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x1852445f77d8 Port:9901/tcp Path:/ready StatusCodeMatcher:0x864ac0 ResponseMatcher:0x9592a0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/25 11:43:31 🐳 Terminating container: f8afba78be8d
2026/04/25 11:43:31 🚫 Container terminated: f8afba78be8d
=== RUN   TestDifferential/0002-tls-tcp
2026/04/25 11:43:31 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 11:43:31 ✅ Container created: e44abc6bf054
2026/04/25 11:43:31 🐳 Starting container: e44abc6bf054
2026/04/25 11:43:32 ✅ Container started: e44abc6bf054
2026/04/25 11:43:32 🚧 Waiting for container id e44abc6bf054 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x1852446c61e8 Port:9901/tcp Path:/ready StatusCodeMatcher:0x864ac0 ResponseMatcher:0x9592a0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/25 11:43:32 🐳 Terminating container: e44abc6bf054
2026/04/25 11:43:32 🚫 Container terminated: e44abc6bf054
=== RUN   TestDifferential/0003-http11-routing
2026/04/25 11:43:32 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/25 11:43:33 ✅ Container created: 451fede92251
2026/04/25 11:43:33 🐳 Starting container: 451fede92251
2026/04/25 11:43:33 ✅ Container started: 451fede92251
2026/04/25 11:43:33 🚧 Waiting for container id 451fede92251 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x185244892b90 Port:9901/tcp Path:/ready StatusCodeMatcher:0x864ac0 ResponseMatcher:0x9592a0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/25 11:43:33 🐳 Terminating container: 451fede92251
2026/04/25 11:43:34 🚫 Container terminated: 451fede92251
--- PASS: TestDifferential (4.58s)
    --- PASS: TestDifferential/0000-tcp-echo (1.11s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.13s)
    --- PASS: TestDifferential/0002-tls-tcp (1.17s)
    --- PASS: TestDifferential/0003-http11-routing (1.17s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	6.015s
```
