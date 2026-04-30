# Phase 06.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 12 preconditions satisfied at cold-start: branch `phase/06.2-access-log-impl` at HEAD `54a31c2b6d4c5c333a8fb19ae015fdd4ee808d25` (matching STATE.md last-commit field); Docker client (28.4.0) + server (28.1.1) both reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; all 6 differential fixtures (0000–0005, including Docker-dependent 0004 and 0005) PASS; `github.com/envoyproxy/go-control-plane/envoy v1.32.4`; `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0065:` (next-free ADR-0066 per PLAN); SPEC.md last-commit is `7bbf4a2` (the spec-reviewer follow-up); `internal/accesslog/` contains only `doc.go`; the four action-method signatures (`directResponseAction.do`, `routerAction.do`, `routerActionH2.doH2`, `h2DirectResponseAdapter.WriteH2`) are present in `internal/filter/hcm/`; `docs/envoy-go/BEHAVIOR_CONTRACT.md` has the `## Access log field mapping` heading with placeholder body present.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `015fc0c`
**Notes:** Created PROGRESS.md; verified all 12 preconditions per PLAN §"Execution preconditions"; phase-06.1 close confirmed present in HEAD; SPEC at `7bbf4a2`; ADR tail at 0065 (next-free 0066); `internal/accesslog/` contains only `doc.go` (the package implementation lands at Task 2+).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/06.2-access-log-impl
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0065: Validate metric-name-deriving inputs at the user-input boundary
$ git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/SPEC.md
7bbf4a2389b94d75061a746fb7db079f820211c0
$ ls internal/accesslog/
doc.go
```

## Task 2 — `internal/accesslog/accesslog.go` — Sink interface + Record struct + doc.go rewrite [ADR-0066]

**Commits:** TBD — this task's commit
**Notes:** Created `internal/accesslog/accesslog.go` with `Sink` interface (`Submit(*Record)` + `Close() error`) and `Record` struct (10 plumbed fields: StartTime, Method, Path, Protocol, ResponseCode, BytesSent, Duration, Authority, UserAgent, UpstreamHost). Rewrote `internal/accesslog/doc.go` from phase-00 stub to reference ADR-0066 and lifecycle context. Appended ADR-0066 to `docs/envoy-go/DECISIONS.md` (Access-log architecture decision: thin in-tree primitive, no third-party access-log dependency). TDD discipline followed: test file written first, RED confirmed, then implementation to GREEN.
**Outputs:**
```
# RED — go test ./internal/accesslog/ -count=1 -v (before accesslog.go)
# github.com/esalaine/envoy-go/internal/accesslog [github.com/esalaine/envoy-go/internal/accesslog.test]
internal/accesslog/accesslog_test.go:9:8: undefined: Record
internal/accesslog/accesslog_test.go:24:7: undefined: Record
internal/accesslog/accesslog_test.go:35:34: undefined: Record
internal/accesslog/accesslog_test.go:36:33: undefined: Record
internal/accesslog/accesslog_test.go:40:8: undefined: Sink
internal/accesslog/accesslog_test.go:41:8: undefined: Record
FAIL	github.com/esalaine/envoy-go/internal/accesslog [build failed]
FAIL

# GREEN — go test ./internal/accesslog/ -count=1 -v (after accesslog.go)
=== RUN   TestRecord_AllFieldsZeroValueWellDefined
--- PASS: TestRecord_AllFieldsZeroValueWellDefined (0.00s)
=== RUN   TestRecord_PopulatedShape
--- PASS: TestRecord_PopulatedShape (0.00s)
=== RUN   TestSink_InterfaceImplementation
--- PASS: TestSink_InterfaceImplementation (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# grep -nE '^## ADR-0066:' docs/envoy-go/DECISIONS.md
2335:## ADR-0066: Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure)

# git diff --stat HEAD (after staging)
 docs/envoy-go/DECISIONS.md           | 33 +++++++++++++++++++++++++
 internal/accesslog/accesslog.go      | 40 ++++++++++++++++++++++++++++++
 internal/accesslog/accesslog_test.go | 47 ++++++++++++++++++++++++++++++++++++
 internal/accesslog/doc.go            | 14 ++++++++---
 4 files changed, 130 insertions(+), 4 deletions(-)
```
