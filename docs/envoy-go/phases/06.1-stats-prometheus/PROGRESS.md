# Phase 06.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2 PROGRESS.md structure.

## Preamble — execution preconditions

Two minor deviations from PLAN.md's "Execution preconditions" block, neither blocking: (1) ADR tail in `docs/envoy-go/DECISIONS.md` is `## ADR-0057:` (closes ADR-0035 H/2 leg), not `## ADR-0058:` as PLAN expected — therefore next-free ADR slot is `ADR-0058` and the PLAN's referenced ADR numbers (0059, 0060, 0061, 0062, 0063, 0064) must each shift down by one (→ 0058..0063) when authored in subsequent tasks; (2) `git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` returns `ad46930` (the actual SPEC drafting commit), whereas PLAN expected `be99b42` — `be99b42` is a STATE.md-only SHA-fill follow-up that did not touch SPEC.md, so `ad46930` is the correct answer to the question and the codebase state matches the documented SPEC. All other preconditions green: branch `phase/06.1-stats-prometheus-impl` at PLAN commit `4820c99`; docker client+server reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; `go test ./...` green for every package (no FAIL, no compile error); `github.com/envoyproxy/go-control-plane/envoy v1.32.4`; phase-05.2 REVIEW close present in HEAD (`b9810ad phase 05.2: REVIEW.md — APPROVED WITH FOLLOW-UPS`); `internal/stats/` contains only `doc.go`; `admin.New(addr string) *Server` matches; cluster has `NewManager`+`NewManagerWithBaseDir`; listener has `NewManager`+`NewManagerWithBaseDir`+`NewManagerWithBaseDirAndAllowH2C`; `BEHAVIOR_CONTRACT.md` has exactly one `## Stat-name mapping` section.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD (SHA-fill follow-up)
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-05.2 close confirmed present in HEAD; SPEC at ad46930; ADR tail at ADR-0057 (next-free ADR-0058) — PLAN's referenced ADR numbers (0059..0064) must shift down by one to 0058..0063 in subsequent tasks; internal/stats/ contains only doc.go (registry.go etc. land at Task 2).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/06.1-stats-prometheus-impl
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0057: Closes ADR-0035 H/2 leg via fixture 0004's full-stack HTTPS h2
$ git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md
ad469309eb673a738956f67c93a70bd70894c496
$ ls internal/stats/
doc.go
```
