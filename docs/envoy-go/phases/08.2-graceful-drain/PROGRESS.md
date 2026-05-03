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

## Task 4 — `internal/cluster.Manager.Drain()` + `Cluster.closePool()` stub [ADR-0096 cluster anchor]

**Commits:** 9289c13 — this task's substantive commit
**Notes:** Added `closePool()` unexported method on `*Cluster` in `internal/cluster/cluster.go` as a forward-extensible stub (no-op-with-comment): `Cluster` carries no exported pool fields at this point in the codebase's evolution (phase 02 dials per-request without keep-alive pooling; phase 05.2 H2 `ClientConn` has no exported close hook today). Added `Drain()` exported method on `*Manager` in `internal/cluster/manager.go` that walks `m.clusters` and calls `c.closePool()` on each; best-effort, no error return, idempotent. Added three tests in `manager_test.go`: `TestManager_Drain_ClosesPools` (NewManager + two Drain calls; asserts no panic), `TestManager_Drain_Idempotent` (10× Drain calls; asserts no panic), `TestManager_Drain_EmptyClusterList` (struct-literal Manager with empty map; asserts no panic on empty). Tests follow the established `mkBootstrap` / `mkStaticCluster` / `mkLbEndpoint` inline-builder pattern from sibling tests (`TestManager_HappyPath_Single` et al.); no new helper invented. Appended ADR-0096 (consolidated in-flight-completion ADR: three-part discipline — HCM per-request Inc/Dec, TCP-proxy per-conn Inc/Dec, cluster.Manager.Drain after rendezvous) to `docs/envoy-go/DECISIONS.md`; Tasks 9 + 10 cite ADR-0096 in their commit messages without re-anchoring. Task 6 consolidated into this commit (PLAN documented Task 6 as a no-op placeholder slot). `cmd/envoy-go` broken-window persists (Task 11 fixes).
**Outputs:**
```
$ go test -count=1 ./internal/cluster/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/cluster	0.010s
$ go vet ./internal/cluster/...
(no output — clean)
$ golangci-lint run ./internal/cluster/...
(no output — clean)
$ grep -nE '^func \(m \*Manager\) Drain\(\)' internal/cluster/manager.go
179:func (m *Manager) Drain() {
$ go build ./cmd/envoy-go/... 2>&1 | head -3
cmd/envoy-go/main.go:139:51: not enough arguments in call to admin.New
(EXPECTED FAILURE — intentional broken-window; Task 11 fixes the call site)
```

## Task 3 — `internal/admin.New` constructor widening — thread `*drain.Manager` (LBP-1 fifth application)

**Commits:** 42256f0 — this task's substantive commit
**Notes:** Widened `internal/admin.New` from 6-param (08.1 form: `New(addr, registry, bs, cm, lm)`) to 7-param adding `dm *drain.Manager` per SPEC §6.1 + BRAINSTORM Decision 4. Added `dm *drain.Manager` field to `Server` struct (after existing `lm` field) with the 08.2 doc-comment per ADR-0091 + BRAINSTORM Decision 4. Updated all 12 `New(...)` call sites in `admin_test.go` and 19 additional call sites across `clusters_test.go`, `configdump_test.go`, `listeners_test.go`, and `serverinfo_test.go` — 31 total call sites updated to the 7-arg form (nil for dm at all existing sites). Added two new prescribed tests: `TestServer_NewWidenedConstructor_DrainManager` (verifies dm field threaded through New) and `TestServer_NewWidenedConstructor_NilDrainManagerTolerated` (verifies nil dm accepted). Updated `Server` struct doc-comment to mention 08.2 drain extension surface. Updated ADR-0085 Consequence (a) in-place with the LBP-1 fifth-application forward-pointer per the prescribed prose. This commit intentionally leaves `cmd/envoy-go/main.go:139` broken (`not enough arguments in call to admin.New`); Task 11 fixes the call site.
**Outputs:**
```
$ go test ./internal/admin/... 2>&1 | tail -10
ok  	github.com/esalaine/envoy-go/internal/admin	1.436s
$ go vet ./internal/admin/...
(no output — clean)
$ golangci-lint run ./internal/admin/...
(no output — clean)
$ go build ./cmd/envoy-go/... 2>&1 | tail -3
cmd/envoy-go/main.go:139:51: not enough arguments in call to admin.New
	have (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager, *drain.Manager)
(EXPECTED FAILURE — intentional broken-window; Task 11 fixes the call site)
```

## Task 2 — `internal/drain/` package — `Manager` + `FuzzDrainTransitions` [ADR-0091]

**Commits:** e884998a1a5b5b3a7bfbc2f2e84a4f9374d6d3ac — this task's substantive commit
**Notes:** Created four files: `internal/drain/doc.go`, `internal/drain/manager.go`, `internal/drain/manager_test.go`, `internal/drain/fuzz_test.go`. Three-state Manager (LIVE/DRAINING/DRAINED-as-channel-close) per SPEC §5.9 + §6.2. Lock-free hot path: `atomic.Uint32` state + `atomic.Int64` inflight; `chan struct{}` rendezvous; `sync.Once` Drain-guard + `sync.Once` close-done-guard. Manager does NOT enforce timeout (callers select on `time.After(m.Timeout())` alongside `<-m.Done()` per ADR-0095). 9/9 unit tests PASS; race clean; vet clean; lint clean (one gofmt deviation corrected: three double-space inline comments normalized to single-space by gofmt requirement). FuzzDrainTransitions (eleventh fuzzer; 30s budget per ADR-0018) PASS — ~49.7M executions, no crashers, three invariants verified. ADR-0091 appended to DECISIONS.md. LBP-1 fifth application: drain.Manager joins stats.Registry/HTTPRegistry/ListenerFilterRegistry/08.1-bs+cm+lm as boot-threaded dependency; wiring into admin.New (Task 3), listener.Manager (Task 5), HCM (Task 9), TCP-proxy (Task 10), main.go (Task 11) lands in subsequent tasks.
**Outputs:**
```
$ go test -count=1 ./internal/drain/... -v 2>&1 | tail -25
=== RUN   TestStateTransitions
--- PASS: TestStateTransitions (0.00s)
=== RUN   TestInflightBalance
--- PASS: TestInflightBalance (0.00s)
=== RUN   TestDrainCompletionRendezvous
--- PASS: TestDrainCompletionRendezvous (0.02s)
=== RUN   TestDrainTimeout_NoInflight
--- PASS: TestDrainTimeout_NoInflight (0.00s)
=== RUN   TestDrainTimeout_StuckInflight_CallerEnforces
--- PASS: TestDrainTimeout_StuckInflight_CallerEnforces (0.05s)
=== RUN   TestIdempotentDrain
--- PASS: TestIdempotentDrain (0.00s)
=== RUN   TestIsDrainingFastPath
--- PASS: TestIsDrainingFastPath (0.00s)
=== RUN   TestNilSafety
--- PASS: TestNilSafety (0.00s)
=== RUN   TestConcurrentIncDec
--- PASS: TestConcurrentIncDec (0.00s)
=== RUN   FuzzDrainTransitions
=== RUN   FuzzDrainTransitions/seed#0
=== RUN   FuzzDrainTransitions/seed#1
=== RUN   FuzzDrainTransitions/seed#2
--- PASS: FuzzDrainTransitions (0.00s)
    --- PASS: FuzzDrainTransitions/seed#0 (0.00s)
    --- PASS: FuzzDrainTransitions/seed#1 (0.00s)
    --- PASS: FuzzDrainTransitions/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/drain	0.076s
$ go test -race -count=1 ./internal/drain/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/drain	1.128s
$ go test -fuzz=FuzzDrainTransitions -fuzztime=30s ./internal/drain/ 2>&1 | tail -10
fuzz: elapsed: 27s, execs: 44854118 (1657699/sec), new interesting: 8 (total: 11)
fuzz: elapsed: 30s, execs: 49755659 (1634565/sec), new interesting: 8 (total: 11)
fuzz: elapsed: 30s, execs: 49755659 (0/sec), new interesting: 8 (total: 11)
PASS
ok  	github.com/esalaine/envoy-go/internal/drain	30.176s
```

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** 1ce41cc8ca4b9776992abdc24b7aff6178a451ea — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-08.2 SPEC + 08.2 PLAN confirmed present in HEAD; SPEC at 0fc63f6 (descendant of 546b08a; typo-fix follow-up touched SPEC.md); ADR tail at 0090 (next-free 0091); internal/drain/ absent (Task 2 lands); listener/cluster Manager.Drain() not yet present (Tasks 5/6); admin.New constructor at 6-param 08.1 form (Task 3 widens to 7-param). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
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
