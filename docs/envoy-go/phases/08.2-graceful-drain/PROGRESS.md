# Phase 08.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..08.1 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 15 preconditions were satisfied at cold-start: branch is `phase-08.2-graceful-drain-impl`, master log matches expected sequence (ca9fc35 PLAN SHA-fill, 72ddc23 PLAN, 0fc63f6 SPEC SHA-fill, 546b08a SPEC, 3ae6af7 BRAINSTORM SHA-fill, e7b64ac BRAINSTORM, eb3babd 08.1 SHA-fill, 70e6a65 08.1 phase-done), Docker client+server reported (28.4.0 / 28.1.1), Go 1.26.2, golangci-lint 1.64.8, all packages PASS short-mode, all 11 differential fixtures PASS (TestDifferential subtests), ADR tail is ADR-0090:, SPEC.md commit is 0fc63f6 (descendant of 546b08a), `git status` empty, `internal/drain/` absent, admin.New 6-param signature present, neither listener nor cluster Manager has a Drain() method, envoyproxy/envoy:v1.37.2 pull success (already up to date), CONFORMANCE_PINS.md diff vs master empty.

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The nine ADRs anticipated by SPEC §8 (ADR-0091..ADR-0099). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0091** Drain state-machine shape + new `internal/drain/` package + LBP-1 fifth-application threading — Task 2
- **ADR-0092** SIGTERM/SIGINT drain-then-exit divergence from Envoy v1.37.2 — Task 11
- **ADR-0093** POST /drain_listeners contract + method-discrimination ENFORCED (partially amends ADR-0090) — Task 7
- **ADR-0094** Listener stop-accepting via Accept-loop fast-path; accept-then-FIN per §11.5 — Task 5
- **ADR-0095** Drain timeout default 30s envoy-go MVP (vs 600s Envoy default) — Task 11
- **ADR-0096** In-flight-completion HCM/TCP-proxy hooks + cluster.Manager.Drain consolidated — **Task 4** (cluster-side anchor; Task 6 is a documented no-op placeholder slot consolidated INTO Task 4) + Tasks 9, 10 (HCM/TCP-proxy realizing components)
- **ADR-0097** /ready DRAINING extension; partially supersedes ADR-0015 — Task 8
- **ADR-0098** /server_info DRAINING transition; amends ADR-0088 — Task 8
- **ADR-0099** Hot-restart deferral — Task 12

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The ten planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`FuzzDrainTransitions` ship-or-skip = SHIP** (eleventh fuzzer; ~60 LoC; 30s budget per ADR-0018; lands in Task 2).
2. **`drain.New(timeout)` validation = trust the caller; document the precondition** (no defensive panic/clamp; doc-comment notes timeout > 0 expected).
3. **`Manager.Done()` semantics when Drain not called = open channel that NEVER closes** (until Drain fires AND inflight reaches 0; doc-comment documents the precondition).
4. **`markedInflight` flag placement = on per-request HCM stream struct** (exact field placement settled at impl-task time per codebase reality).
5. **TCP-proxy Inc/Dec anchor = OnNewConnection** (per-connection granularity; matching Dec via defer immediately after Inc).
6. **`internal/cluster.Cluster.closePool()` shape = `func (c *Cluster) closePool()` no-return; iterates whatever pooled resources Cluster carries today; stub if no exported pool field exists yet** (best-effort; ignore errors).
7. **`drainMgr` boot-order placement = after `bootstrap.Load`, before `cluster.NewManagerWithBaseDir`** (drain manager has no deps; consumed by all subsequent constructors).
8. **Fixture 0010 driver framework reuse = share dual-proxy boot helpers and admin-scrape helpers with 0009; do NOT share canonicalisation** (per-state-transition byte-equality vs structural-projection are structurally different).
9. **`cm.Drain()` call ordering vs deferred-stop chain = explicit call after rendezvous, before deferred-stop chain runs** (LIFO: lm.Stop, admSrv.Close, sinks-close per phase 06.2).
10. **`POST /drain_listeners` with `nil` drain manager = return 500 Internal Server Error with body `drain manager not configured\n`** (defensive-loud over silent-200; aligns with the ADR-0085 nil-tolerance pattern only for read-only endpoints, not for the mutating `/drain_listeners`; settles SPEC §14.2's `TestHandleDrainListeners_NilDrainManager` ambiguity).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-08.2 SPEC + 08.2 PLAN confirmed present in HEAD; SPEC at 546b08a; ADR tail at 0090 (next-free 0091); internal/drain/ absent (Task 2 lands); listener/cluster Manager.Drain() not yet present (Tasks 5/6); admin.New constructor at 6-param 08.1 form (Task 3 widens to 7-param). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-08.2-graceful-drain-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0090:
$ git log -1 --format=%H -- docs/envoy-go/phases/08.2-graceful-drain/SPEC.md
0fc63f677776449e3be66c54f4f3b5be1f0bf128
```
