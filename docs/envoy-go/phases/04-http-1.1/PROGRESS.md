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
