# Phase 09 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..08.2 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 15 preconditions were satisfied at cold-start: branch is `phase-09-http-filter-fault-impl`, master log matches expected sequence (9bd8d0d PLAN SHA-fill, b963c1b PLAN, 80b3f9f SPEC SHA-fill, da29807 SPEC, 8506a3c BRAINSTORM SHA-fill, 4f44a03 BRAINSTORM, 14a68e7 08.2 SHA-fill, b33e04f 08.2 phase-done), Docker client+server reported (28.4.0 / 28.1.1), Go 1.26.2, golangci-lint 1.64.8, all packages PASS short-mode, all 12 differential fixtures PASS (TestDifferential/0000..0010 subtests under TestDifferential parent), ADR tail is `ADR-0099:`, SPEC.md commit is da29807 (HEAD-of-master at PLAN.md SHA-fill follow-up; descendant of itself), `git status --porcelain` empty, `internal/filter/http/fault/` absent, FactoryCtx single-field 2-param form (one `Registry *HTTPRegistry` line in `internal/filter/http/types.go`), `parseHTTPFiltersChain` 2-param signature present at `internal/filter/hcm/config.go:273`, envoyproxy/envoy:v1.37.2 pull success (already up to date), CONFORMANCE_PINS.md diff vs master empty.

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The eight ADRs anticipated by SPEC §8 (ADR-0100..ADR-0107). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0100** `internal/filter/http/fault/` package shape + boot registration + FactoryCtx framework extension — Task 3 (ADR text) + Task 2 (FactoryCtx extension code) + Task 8 (boot registration code)
- **ADR-0101** runtimeConfig shape + 6/11-field decomposition + abort.http_status PGV mirror + percentage-roll determinism — Task 3
- **ADR-0102** Delay async-resume mechanics + combined delay+abort timer-callback decision — Task 5
- **ADR-0103** Abort terminal-replace + body byte-exact + 4-header set + status-text allow-list — Task 4
- **ADR-0104** Header-driven fault path DEFERRED (per ADR-0040 deferral format) — Task 15
- **ADR-0105** max_active_faults concurrency cap + LBP-1 sixth + markedActive idempotency — Task 6
- **ADR-0106** §9 HTTP filters family expansion shape (flat top-level rows + no-sibling-stub) — Task 15
- **ADR-0107** 17→22-name extension + response_rl_injected route A — Task 3 (consolidated; per PLAN refinement note)

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The thirteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`FuzzFaultConfigParse` ship-or-skip = SHIP** (twelfth fuzzer; ~50 LoC; 30s budget per ADR-0018; lands in Task 9).
2. **`runtimeConfig` parser refactor = KEEP separate** (parseRuntimeConfig + parseRouteRuntimeConfig two-function split; New-time has additional validation that does not apply at per-route resolve time).
3. **Stat-counter call-site organization = consolidate into `recordFaultEvent(kind, increment bool)` helper** (cleaner test surface; ~15 LoC; lands in Task 3 alongside the stat registration).
4. **Per-route runtimeConfig caching = SKIP** (chain's RequestRouteConfig already lazy-cached; per-request projection cost is sub-microsecond).
5. **Fault stats = USE existing `internal/stats.Registry` (06.1)** (sub-registries out of scope; FactoryCtx extension threads *stats.Registry per Task 2).
6. **`fault.response_rl_injected` route A vs B = SETTLED at SPEC + ADR-0107 (route A: emit permanently-zero counter)** — not a planner decision.
7. **Allow-list discipline for abort-status text = narrow allow-list scoped to non-stdlib codes only** (200/503/404/405 byte-equal; 418 etc. compare on STATUS CODE only; lands in Task 12 expectations.yaml + Task 13 driver).
8. **Fixture cluster type = STRICT_DNS pointing at the harness backend hostname** (mirrors 0007a-cors precedent; ADR-0010 dns_lookup_family V4_ONLY).
9. **OrderedHeaders carrier from fault's SendLocalReply = SETTLED at SPEC §6.6 (option A: pass `OrderedHeaders{Content-Type: text/plain}`)** — not a planner decision.
10. **Race-detector cycle test for timer-driven async-resume = ADD `TestFault_DelayTimerRace`** (~30 LoC; lands in Task 6).
11. **Fixture path = `test/fixtures/0011-http-fault/`** (NOT `test/differential/0011-http-fault/` per SPEC §4.3 erratum; mirrors 0010-graceful-drain precedent).
12. **Percentage-roll RNG source = per-instance `*math/rand.Rand` seeded by `time.Now().UnixNano()` at filter-instance allocation time** (per-request seed for non-deterministic-across-requests rolls; 0% / 100% scenarios short-circuit before consulting RNG).
13. **Fixture's new BackendKind enum value name = `HTTPFault BackendKind = 8`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** 29c0958 — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-09 SPEC + 09 PLAN confirmed present in HEAD; SPEC at da29807; ADR tail at 0099 (next-free 0100); internal/filter/http/fault/ absent (Task 3 lands); FactoryCtx single-field 2-param form (Task 2 widens); parseHTTPFiltersChain 2-param signature (Task 2 widens). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-09-http-filter-fault-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0099:
$ git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/SPEC.md
da29807d83db9fe1816a8f52c5e34c1fa3b602d7
```
