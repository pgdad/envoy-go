# Phase 07.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1/06.2 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 14 preconditions satisfied at cold-start: branch `phase/07.1-http-filter-framework-impl` at HEAD `0bfaaf1` (master tip at branch creation); Docker client (28.4.0) + server (28.1.1) both reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; all 7 differential fixtures (0000–0006, including Docker-dependent 0004/0005/0006) PASS via `TestDifferential` subtests (precondition #6's regex did not match Go subtests directly; verified substantive intent by running `TestDifferential` directly); `github.com/envoyproxy/go-control-plane/envoy v1.32.4` present in `go.mod`; `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0069:` (next-free 0070); SPEC.md last-commit is `f2dd6593ceb2f74fff73120b053c32bb0c0b1486`; `internal/filter/http/` absent; the four refactor-target action-method signatures (`directResponseAction.do`, `routerAction.do`, `routerActionH2.doH2`, `h2DirectResponseAdapter.WriteH2`) present in `internal/filter/hcm/`; `BEHAVIOR_CONTRACT.md` has both `## HTTP/1.1` and `## HTTP/2` anchor headings present; HTTPRegistry symbol absent in `internal/` and `cmd/`; reference Envoy image `envoyproxy/envoy:v1.37.2` pulled successfully.

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0070]

**Commits:** 9d59c6d — this task's commit
**Notes:** Created PROGRESS.md; verified all 14 preconditions per PLAN §"Execution preconditions"; phase-06.2 close confirmed present in HEAD; SPEC at f2dd6593ceb2f74fff73120b053c32bb0c0b1486; ADR tail at 0069 (next-free 0070); internal/filter/http/ absent (the package implementation lands at Task 2+); HTTPRegistry symbol absent. Landed ADR-0070 (phase-07 planner-time split per ADR-0045's pattern).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/07.1-http-filter-framework-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0069:
$ git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md
f2dd6593ceb2f74fff73120b053c32bb0c0b1486
$ test ! -d internal/filter/http && echo OK
OK
```

## Task 2 — internal/filter/http/types.go + callbacks.go [ADR-0071]

**Commits:** 0a6526b — this task's commit
**Notes:** Created internal/filter/http/{doc,types,callbacks}.go + test pairs. Defined StreamDecoderFilter + StreamEncoderFilter interfaces, three status enums (FilterHeadersStatus/FilterDataStatus/FilterTrailersStatus), DecoderFilterCallbacks + EncoderFilterCallbacks interfaces, two-step HTTPFilterFactory + FilterInstanceFactory pattern. Landed ADR-0071 (HTTP filter iteration protocol shape; supersedes ADR-0040 totally; partially supersedes ADR-0042). go test ./internal/filter/http/ green; package compiles standalone (registry.go + chain.go + perroute.go land in subsequent tasks).
**Outputs:**
```
$ go test ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.002s
$ grep -nE '^## ADR-0071:' docs/envoy-go/DECISIONS.md
2547:## ADR-0071: HTTP filter iteration protocol shape
```

## Task 3 — internal/filter/http/registry.go [ADR-0072]

**Commits:** TBD — this task's commit
**Notes:** Created internal/filter/http/{registry,registry_test}.go. Removed the Task-2 forward-declaration stub of HTTPRegistry from types.go now that registry.go defines the real type. HTTPRegistry exposes Register / Lookup / Freeze / KnownTypeURLs; freeze-after-boot invariant via atomic.Bool; duplicate Register and post-Freeze Register both panic with verbatim messages per SPEC §5.3 + ADR-0072. Six new tests (registry shape + duplicate-panic + post-Freeze-panic + Freeze-idempotent + Lookup-after-Freeze + concurrent-Lookup-race-clean); all pass under -race. Landed ADR-0072 (*HTTPRegistry threaded constructor map; mirrors *stats.Registry LBP-1 from ADR-0059).
**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.009s
$ grep -nE '^## ADR-0072:' docs/envoy-go/DECISIONS.md
2595:## ADR-0072: HTTPRegistry threaded constructor map, no package-global
```
