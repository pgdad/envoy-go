# Phase 05.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/PROGRESS.md structure.

## Preamble — execution preconditions

Minor deviation on precondition 1: STATE.md `last-commit` is `634bcb6` (the PLAN.md-draft commit) but HEAD is `5bd2cf4` (the subsequent lifecycle-transition commit that fast-forwarded `634bcb6` into master). This is the expected impl-worktree shape — the impl branch is cut from master tip `5bd2cf4`, which subsumes `634bcb6`. Minor deviation on precondition 9: `1542102` (referenced alongside `671a059`) only touches `STATE.md` and so does not appear in `git log --oneline -- internal/filter/hcm/actions.go internal/filter/hcm/connection.go internal/cluster/manager.go`; the operative code-fix commit `671a059` is confirmed present. All other preconditions satisfied at cold-start. Docker client and server both present and responsive. Go 1.26.2 satisfies 1.23+ requirement. golangci-lint v1.64.8 matches ADR-0009. go-control-plane/envoy pinned at v1.32.4 per ADR-0013. DECISIONS.md tail is `## ADR-0045:` — ADRs 0046..0053 assigned as planned. SPEC at `4b45941` matches PLAN authoring commit. golang.org/x/net v0.34.0 resolvable (indirect via go-control-plane). `go test ./...` all PASS, no FAIL, no compile errors.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** e8989c0
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-04 I-1..I-4 fixes confirmed present in HEAD (commit 671a059 visible in log); SPEC at 4b45941; ADR tail at 0045 (next-free 0046).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/05.1-downstream-h2-impl
$ git log -1 --format=%H
5bd2cf4d7cebe7a0d8c202487e1bf10ce90f2c1f
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
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	(cached)
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
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
$ go list -m github.com/envoyproxy/go-control-plane/envoy
github.com/envoyproxy/go-control-plane/envoy v1.32.4
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0045: Split phase 05 into 05.1 (downstream H2 + h2spec) + 05.2 (upstream H2 + fixture 0004)
$ git log -1 --format=%H -- docs/envoy-go/phases/05.1-downstream-h2/SPEC.md
4b45941c359edb70759ddde6c104e45bb57a9777
$ git log --oneline -- internal/filter/hcm/actions.go internal/filter/hcm/connection.go internal/cluster/manager.go | head -20
671a059 phase 04: REVIEW.md follow-ups (I-1..I-4 + M-1 from REVIEW.md 04527eb)
7359397 phase 04: internal/filter/hcm — per-conn loop (runConnection)
95ea7e8 phase 04: internal/filter/hcm — directResponseAction + routerAction [ADR-0039]
e252dbe phase 03: internal/cluster — Cluster.Dial(ctx) + upstream TLS [ADR-0032]
958c059 phase 02: internal/cluster.Manager — build-time materialisation
$ go list -m golang.org/x/net
golang.org/x/net v0.34.0
```
