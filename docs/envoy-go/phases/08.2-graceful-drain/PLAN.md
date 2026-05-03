# Phase 08.2 — Graceful drain (`internal/drain/`, `POST /drain_listeners`, `/ready` + `/server_info` DRAINING extensions, SIGTERM-handler upgrade) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 08.2), §8 (MVP-trunk-close — 08.2's phase-done commit closes the BOOTSTRAP_PROMPT.md §8 trunk; parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2), §9 (feature families post-trunk); `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; 1502 lines, 16 sections, **read in full**); `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` (the autonomous-brainstorm artefact at master `e7b64ac` that the SPEC distils §§1–11 from — 12 Decisions + §3 surface inventory + §5 per-state-transition flows; consult when the SPEC's "what" needs the BRAINSTORM's "why"); `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` (parent master SPEC — cross-cutting context for the 08.1 + 08.2 split; per parent §5, parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2's phase-done — this is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit); `docs/envoy-go/phases/08.1-admin-endpoints/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (closed read-only history; 08.1's PLAN is the structural precedent — task-numbering, TDD-step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections); `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` + `docs/envoy-go/phases/07.2-listener-chain-completion/PLAN.md` (additional structural precedent); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0090 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0014** `Server: envoy` header, **ADR-0015** `/ready` pre-init contract — partially superseded by ADR-0097 in this phase, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format, **ADR-0041** silent-ignore set — `?graceful=true` inherits, **ADR-0044** ADR-on-impl convention, **ADR-0045** planner-time-split discipline (~25 tasks / ~1500 LoC thresholds — both well under for this phase), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0063** cluster-scope-only metrics + per-endpoint deferral, **ADR-0072** `*HTTPRegistry` threaded constructor map, **ADR-0075** sendLocalReply Inc/Dec balance discipline (HCM markedInflight pattern inherits), **ADR-0079** `*ListenerFilterRegistry` threaded constructor map, **ADR-0085** admin-mux reuse + LBP-1 third application — 08.2's `*drain.Manager` threading is the LBP-1 fifth application, **ADR-0088** `/server_info` MVP field set + state-enum coverage — amended by ADR-0098 in this phase, **ADR-0089** admin-endpoint deferral list — `/drain_listeners` line flips to "delivered in 08.2" via in-place edit per its consequence (b), **ADR-0090** no-ACL admin-endpoint security posture — partially amended by ADR-0093 in this phase (mutating endpoints DO enforce method discrimination); ADR-0090 is the verified DECISIONS.md tail at master `0fc63f6`; phase 08.2's nine anticipated ADRs land at ADR-0091..ADR-0099); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## Admin API ### /drain_listeners` NEW subsection + `### /ready` extension + `### /server_info` extension + new `## Graceful drain` umbrella section + three new equivalence-matrix rows + ADR-0015 / ADR-0088 / ADR-0090 forward-pointer notes; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 08.2 — D-3.7 reserves pin bumps for dedicated phases; the h2spec gate at 53/53 PASS is mechanical re-run because 08.2's HCM Inc/Dec hooks are non-load-bearing for the H2 codec/framer/hpack path); `docs/envoy-go/ROADMAP.md` (rows `08`, `08.2` per the SPEC commit's row-flip; row `08.2` flips `in-progress → done` at this phase's phase-done; row `08` SIMULTANEOUSLY flips `in-progress → done` per parent SPEC §5 — MVP-trunk-close).

**Goal:** Land envoy-go's graceful-drain sub-phase — the lifecycle discipline that moves envoy-go from "kill -TERM means hard exit" to a unified three-state drain machine (`LIVE` → `DRAINING` → `DRAINED`-as-channel-close) backing both operator-driven drain-without-exit (`POST /drain_listeners`) and signal-driven drain-then-exit (`SIGTERM`/`SIGINT`) under a 30s timeout. Concretely (per SPEC §1 + §4): a new `internal/drain/` package owning the drain-state machine (`Manager` type with `atomic.Uint32` state + `atomic.Int64` inflight + `chan struct{}` rendezvous + `sync.Once` Drain-guard + `sync.Once` close-done-guard; ~150 LoC + ~250 LoC tests; ADR-0091); a `cmd/envoy-go/main.go` SIGTERM-handler upgrade replacing the existing `<-ctx.Done()` + deferred `lm.Stop()` flow with `<-ctx.Done()` → `drainMgr.Drain()` → `select { <-drainMgr.Done(): / <-time.After(timeout): }` → `cm.Drain()` → existing deferred-stop chain (~30 LoC delta; ADR-0092 records the deliberate divergence from upstream Envoy v1.37.2's SIGTERM=immediate-exit per §11.7); a `POST /drain_listeners` admin endpoint emitting `200 OK` with body `OK\n` per §11.1 + ENVOY-FAITHFUL method discrimination returning `405 Method Not Allowed` with body `Method <X> not allowed, POST required.\n` per §11.4 (`internal/admin/drain.go` ~60 LoC + ~150 LoC tests; ADR-0093 — the FIRST envoy-go endpoint with method discrimination; partially amends ADR-0090's no-method-discrimination posture by qualifying it to read-only endpoints only); `/ready` and `/server_info` DRAINING-state extensions (`/ready` returns `503 DRAINING\n` per §11.2; `/server_info` `state` field renders `"DRAINING"` per §11.2; DRAINING precedence > LIVE > PRE_INITIALIZING; ~30 + ~5 LoC delta; ADR-0097 partially supersedes ADR-0015 + ADR-0098 amends ADR-0088 purely-additively); `internal/listener.Manager.Drain()` accessor + per-runtime Accept-loop fast-path checking `m.dm.IsDraining()` AT THE TOP of each iteration (after `Accept()` returns) closing the new conn without filter-chain dispatch — the accept-then-FIN mechanism observed in §11.5 (~30 LoC delta + tests; ADR-0094); `internal/cluster.Manager.Drain()` best-effort post-drain upstream-pool close + per-`Cluster.closePool()` helper (~30 LoC delta + tests; ADR-0096 consolidates with the HCM/TCP-proxy hooks); HCM `decodeHeaders`/`encodeFinalize`-equivalent Inc/Dec hooks on `drain.Manager.Inflight` with a `markedInflight bool` sentinel on `Stream` to ensure pair-balance under the sendLocalReply path per ADR-0075 (~15 LoC delta + tests; ADR-0096); TCP-proxy `OnNewConnection`/`OnConnectionClose`-equivalent Inc/Dec hooks (~10 LoC delta + tests; ADR-0096); a constructor-widening that threads `*drain.Manager` into `admin.New` (LBP-1 fifth application after `*stats.Registry` / `*HTTPRegistry` / `*ListenerFilterRegistry` / the 08.1 `*bootstrap.Bootstrap`+`*cluster.Manager`+`*listener.Manager` triplet; ADR-0085 consequence extended in-place per the LBP-1 generalization pattern), into `listener.NewManagerWithBaseDirAndAllowH2C`, and transitively into the HCM and TCP-proxy filter constructors; an OPTIONAL new fuzzer `FuzzDrainTransitions` (~60 LoC; 30s budget per ADR-0018; eleventh fuzzer overall — per SPEC §12 #1 the SPEC author RECOMMENDS shipping; this PLAN's planner-time-deferred-decision resolution settles the recommendation as **ship**); a new differential fixture `0010-graceful-drain` (`test/fixtures/0010-graceful-drain/`) with `envoy.yaml` + `envoy-go.yaml` + slow-streaming Go HTTP backend on `:18001` (`/slow` streams 5KB at 1KB/s; `/` serves a fast 200 sanity baseline) + driver implementing the §7.2 admin-trigger path against both proxies + the §7.3 SIGTERM-trigger path against envoy-go-only (per §11.7 deliberate divergence — Envoy SIGTERM is immediate-exit; only the admin path is differentially gated) + `expectations.yaml` per §13.5 + `runner_test.go` blank-import with `RequiresReference: true` semantics per the existing fixture-registration pattern (mirrors 0007a-cors / 0009-admin-config-dump); a `BEHAVIOR_CONTRACT.md` in-place edit per SPEC §13 (NEW `### /drain_listeners` subsection + `### /ready` DRAINING extension + `### /server_info` DRAINING extension + NEW sibling `## Graceful drain` umbrella section + three new equivalence-matrix rows + ADR-0015 / ADR-0088 / ADR-0090 forward-pointer notes; ADR-0052 in-place edit authorisation carries forward); nine new ADRs ADR-0091..ADR-0099 per SPEC §8 (ADR-0091 drain state-machine shape; ADR-0092 SIGTERM-vs-Envoy divergence; ADR-0093 POST /drain_listeners + method-discrimination contract; ADR-0094 listener stop-accepting via Accept-loop fast-path; ADR-0095 hardcoded 30s drain-timeout MVP; ADR-0096 in-flight-completion HCM/TCP-proxy hooks + cluster pool-close consolidated; ADR-0097 /ready DRAINING extension + partial supersession of ADR-0015; ADR-0098 /server_info DRAINING transition + amendment of ADR-0088; ADR-0099 hot-restart deferral). After phase 08.2, the project has proven its tenth-leading-edge engineering claim per SPEC §1: *envoy-go's lifecycle discipline supports operator-driven drain-without-exit (POST /drain_listeners) and signal-driven drain-then-exit (SIGTERM/SIGINT) under a unified three-state lock-free drain manager, with /ready and /server_info DRAINING-state extensions matching upstream Envoy v1.37.2's body shapes byte-for-byte and new-connection rejection matching upstream's accept-then-FIN mechanism, while preserving the 08.1 admin-mux scaffold and the LBP-1 explicit-threading discipline.* This is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit; ROADMAP row `08.2` AND parent row `08` BOTH flip `in-progress → done` AT THE SAME COMMIT (per parent SPEC §5); STATE.md flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle.

**Architecture:** The 08.2 surface is the additive registration of one new `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` entry on the existing admin server's `*http.ServeMux` (the seventh handler post-08.1: /ready, /stats/prometheus, /config_dump, /clusters, /listeners, /server_info, **/drain_listeners**) plus the introduction of a new `internal/drain/` package whose `Manager` type is allocated once at boot in `cmd/envoy-go/main.go` and threaded as an additional constructor parameter into `admin.New` (LBP-1 fifth application per BRAINSTORM Decision 4; ADR-0085 consequence extended in-place), into `listener.NewManagerWithBaseDirAndAllowH2C` (per BRAINSTORM Decision 5), and transitively into the HCM filter constructor (per BRAINSTORM Decision 7) and TCP-proxy filter constructor (per BRAINSTORM Decision 7). Test code that does not exercise drain semantics may pass `nil` (the same nil-tolerance pattern ADR-0085 established for the four 08.1 dependencies). The drain manager's hot-path operations (`State()`, `Inc()`, `Dec()`, `IsDraining()`) are lock-free atomic operations against `atomic.Uint32` (state) and `atomic.Int64` (inflight); the only synchronization beyond atomics is `sync.Once` on the Drain trigger (so concurrent triggers from `handleDrainListeners` + the SIGTERM-handler are safe — only one transition fires) and `sync.Once`-equivalent on the `done` channel close (so the Dec→0-after-Drain rendezvous closes the channel exactly once). The Listener Accept loop in each `listenerRuntime.acceptLoop` (currently at `internal/listener/manager.go` line 783) gains a TWO-line fast-path: AT THE TOP of each iteration AFTER `ln.Accept()` returns, the loop body checks `rt.dm != nil && rt.dm.IsDraining()`; if true, immediately `_ = raw.Close()` and `continue` (no filter-chain dispatch; the existing `rt.downstreamCxTotal.Inc()` + `rt.downstreamCxActive.Inc()` accept-site lines per phase 06.1 are NOT executed for the drained-conn case — the conn never enters `serveConnection`). HCM gains a `dm *drain.Manager` field on `Filter`; the per-stream Inc/Dec lives at the request-begin/request-end edges (the implementer settles the exact file/line at impl time per the codebase reality — `connection.go::runConnection` for H1.1, `h2dispatch.go` for H2; the `markedInflight` bool sentinel ensures pair-balance under sendLocalReply per ADR-0075's discipline). TCP-proxy gains a `dm *drain.Manager` field on `Filter`; the per-conn Inc happens at the top of `Handle` (after `ctx.Err()` check, before `Dial`); the matching Dec is `defer`-d immediately after the Inc (per-connection granularity, correct because TCP-proxy has no per-request semantic). Boot-order in `cmd/envoy-go/main.go`: the new `drainMgr := drain.New(30 * time.Second)` allocation lands AFTER `bootstrap.Load` (the drain manager has no dependencies on the bootstrap proto) and BEFORE `cluster.NewManagerWithBaseDir` (the cluster manager itself does NOT take `drainMgr` per SPEC §6.1 — `cm.Drain()` is called from main AFTER `<-drainMgr.Done()` rather than threaded as a constructor dependency); `drainMgr` is then threaded into `listener.NewManagerWithBaseDirAndAllowH2C` (which threads it into HCM + TCP-proxy filter constructors via the existing `filterRegistry` map's per-typeURL constructor closures) and into `admin.New`. Concurrency model: race-detector-clean for N concurrent scrapes against all SEVEN endpoints (six 08.1 + /drain_listeners) from N goroutines plus a separate goroutine firing `Drain()` once mid-test; `TestAdminConcurrentScrapeRace` (extended from 08.1) exercises this with N=100 scrape-loop goroutines for 1 second. Differential surface: fixture `0010-graceful-drain` runs an admin-trigger driver path against both proxies (envoy-go: `POST /drain_listeners`; reference Envoy: `POST /drain_listeners` + `POST /healthcheck/fail` per §11.2 trigger-script normalization) under a slow-streaming-backend probe (5KB at 1KB/s = 5s in-flight window) and asserts five per-state-transition byte-equality claims per SPEC §7.1; the SIGTERM-trigger driver path (§7.3) is envoy-go-only and is a structural-completeness check (Envoy v1.37.2 SIGTERM is immediate-exit per §11.7; non-equivalent), NOT a differential claim.

**Tech Stack:**
- Go 1.23 (unchanged from 08.1; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `context`, `fmt`, `log`, `net`, `net/http`, `os`, `os/signal`, `strconv`, `sync`, `sync/atomic`, `syscall`, `time` — the new `internal/drain/` package + the SIGTERM-handler upgrade + the `/drain_listeners` handler consume only stdlib (no new module imports introduced by 08.2).
- `github.com/envoyproxy/go-control-plane/envoy/admin/v3` (existing; introduced by 08.1) — `*adminv3.ServerInfo_DRAINING` enum value joins the `LIVE`/`PRE_INITIALIZING` coverage already present in `internal/admin/serverinfo.go`. No proto bump.
- `github.com/esalaine/envoy-go/internal/drain` (NEW package this phase introduces) — `*drain.Manager`; consumed by `cmd/envoy-go/main.go`, `internal/admin/{admin,drain,serverinfo}.go`, `internal/listener/manager.go`, `internal/filter/hcm/{config,connection,h2dispatch,filter}.go` (exact files settled at impl-task time), `internal/filter/tcpproxy/filter.go`, `internal/cluster/manager.go`.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0010's reference image AND the source of the SPEC §11.1–§11.7 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 08.2's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 08.2 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS; phase 08.2's HCM Inc/Dec hooks add ~5 LoC and do NOT touch the H2 codec, the H2 framer, or the H2 hpack path.
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0010's reference (Envoy in a Docker container) — same harness as 06.1/06.2/07.1/07.2/08.1's fixtures consume; phase 08.2 does NOT extend `test/differential/fixture/fixture.go` with new optional interfaces (the existing `Driver` + `RequiresReference` shape is sufficient; the dual-driver-path orchestration lives entirely inside the fixture's own `driver/driver.go`).
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's drain or signal-handling implementation; any third-party drain-machinery library (`go-graceful`, `tomb.v2`, etc.). Test-side use is also forbidden. The `go.mod` post-08.2 must not list any new drain-related runtime dependencies.
- The `internal/drain/` package is wholly new but uses only stdlib `sync`, `sync/atomic`, `time`. The package-import-graph stays acyclic (drain imports nothing in the project; everything else may import drain).

---

## Scope check — why phase 08.2 ships as one sub-phase

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 PLAN's component-table convention):

- `internal/drain/doc.go` ~30
- `internal/drain/manager.go` ~150 + `manager_test.go` ~280 = ~430
- `internal/drain/fuzz_test.go` (OPTIONAL → settled SHIP per planner-time decision 1) ~60
- `internal/admin/drain.go` ~60 + `drain_test.go` ~180 = ~240
- `internal/admin/admin.go` constructor widening + new mux registration + handleReady DRAINING-branch ~+30 / -0 = ~+30
- `internal/admin/admin_test.go` extension (DRAINING-precedence tests + race-test extension to seven endpoints + Drain-mid-test goroutine) ~+150 = ~+150
- `internal/admin/serverinfo.go` deriveState signature widen + DRAINING-first check ~+5 net = ~+5
- `internal/admin/serverinfo_test.go` extension (DRAINING state-enum + precedence + nil-tolerance) ~+60 = ~+60
- `internal/listener/manager.go` `Drain()` accessor + Accept-loop fast-path + `dm` field + constructor-widen + N-1 doc-comment fix ~+40 / -0 = ~+40
- `internal/listener/manager_test.go` extension (Drain + DrainIdempotent + AcceptDuringDrainClosesConn + StopAfterDrain) ~+90 = ~+90
- `internal/cluster/manager.go` `Drain()` method + per-`Cluster.closePool()` helper ~+40 = ~+40
- `internal/cluster/manager_test.go` extension (DrainClosesPools + DrainIdempotent) ~+50 = ~+50
- `internal/filter/hcm/{config,connection,h2dispatch}.go` (exact file split settled at impl-task time) Inc/Dec + `dm` field + `markedInflight` sentinel + constructor-widen ~+25 = ~+25
- `internal/filter/hcm/filter_test.go` extension (DrainInflightBalance + DrainInflightBalance_SendLocalReply) ~+80 = ~+80
- `internal/filter/tcpproxy/filter.go` Inc/Dec + `dm` field + constructor-widen ~+15 = ~+15
- `internal/filter/tcpproxy/filter_test.go` extension (DrainInflightBalance) ~+40 = ~+40
- `cmd/envoy-go/main.go` drainMgr alloc + threading into managers + SIGTERM-handler upgrade ~+30 = ~+30
- `test/fixtures/0010-graceful-drain/` (NEW directory) — `envoy.yaml` ~50 + `envoy-go.yaml` ~50 + `expectations.yaml` ~80 + `README.md` ~80 + `driver/driver.go` ~350 + `backends/backend.go` ~60 = ~670
- `test/differential/runner_test.go` blank-import addition ~+1 = ~+1
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches ~+170 = ~+170
- `docs/envoy-go/DECISIONS.md` (nine ADRs ADR-0091..ADR-0099) ~+450 = ~+450
- `docs/envoy-go/ROADMAP.md` rows `08.2` + `08` `in-progress → done` flip (MVP-trunk close) ~+2 net = ~+2
- `docs/envoy-go/STATE.md` advance to MVP-trunk-closed `awaiting next planning` ~rewrite-in-place
- `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` (NEW; lifecycle artefact) ~600 (per-task entry)
- `docs/envoy-go/phases/08.2-graceful-drain/REVIEW.md` (NEW; lifecycle artefact) ~180

**Production code: ~600 LoC + ~880 LoC tests = ~1480 LoC total Go**, plus ~670 LoC fixture YAML/Go + ~800 LoC docs. Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~600 LoC + test ~880 ≈ ~1480 total, but production-only is only ~600 LoC, well below 1500). Task count below is **13**, comfortably under the 25 limit. The SPEC §8 split discipline (parent 08 → 08.1 + 08.2 per ADR-0084) already applied the gate at the parent level, leaving 08.2 as a single coherent graceful-drain sub-phase. STATE.md `next-skill-scope` projected ~12–15 tasks; this PLAN lands at 13 tasks.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/drain/doc.go` | NEW | Package doc enumerating the three-state drain machine, the LBP-1 fifth-application discipline, and the public API surface (`Manager`, `State`, `New`, `Drain`, `Done`, `Inc`, `Dec`, `IsDraining`, `Timeout`). Per SPEC §4.1. |
| `internal/drain/manager.go` | NEW | `State` type (`uint32`-friendly enum: `StateLive` / `StateDraining` / `StateDrained`); `Manager` struct (`state atomic.Uint32`, `inflight atomic.Int64`, `done chan struct{}`, `timeout time.Duration`, `once sync.Once`, `closeOnce sync.Once`); `New(timeout time.Duration) *Manager` constructor; lock-free public methods `State()`, `Drain()`, `Done()`, `Inc()`, `Dec()`, `IsDraining()`, `Timeout()`. Per SPEC §6.2 + §5.9. |
| `internal/drain/manager_test.go` | NEW | Unit tests per SPEC §14.1: TestStateTransitions / TestInflightBalance / TestDrainCompletionRendezvous / TestDrainTimeout_NoInflight / TestDrainTimeout_StuckInflight / TestIdempotentDrain / TestIsDrainingFastPath / TestNilSafety / TestConcurrentIncDec (race-detector under 100 goroutines × 1000 Inc/Dec pairs). |
| `internal/drain/fuzz_test.go` | NEW (OPTIONAL → SHIPPED per planner-time decision 1) | `FuzzDrainTransitions` — fuzzes a sequence of operations against `*drain.Manager` and asserts state-monotonicity + inflight-balance + Done-fires-once invariants. ~60 LoC; 30s budget per ADR-0018; eleventh fuzzer overall. Per SPEC §14.5. |
| `internal/admin/drain.go` | NEW | `(s *Server) handleDrainListeners(w, r)` http.HandlerFunc — method-discrimination check FIRST (return 405 + body `Method <X> not allowed, POST required.\n` for non-POST per §11.4); on POST, call `s.dm.Drain()`; emit 200 OK + body `OK\n` + standard six-header set per §11.6 via the existing `writeAdminHeaders` helper. Per SPEC §4.1 + §6.3. |
| `internal/admin/drain_test.go` | NEW | Unit tests per SPEC §14.2: PostFires / BodyExact / Idempotent / GraceQueryParamSilentlyIgnored / NilDrainManager / GetReturns405 / PutReturns405 / DeleteReturns405 / HeadReturns405WithEmptyBody / HeaderSet. |
| `internal/admin/admin.go` | MODIFIED | `New` constructor signature widens from 6-param (08.1) to 7-param adding `dm *drain.Manager` (SPEC §6.1); new `dm` field on `Server`; `Start()` body adds `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` after the existing six 08.1 registrations; `handleReady` body adds NEW first branch `if s.dm != nil && s.dm.State() == drain.StateDraining { write 503 + DRAINING\n; return }` PRECEDING the existing pre-init and ready branches per SPEC §6.4 + §5.4. |
| `internal/admin/admin_test.go` | MODIFIED | Existing tests preserved verbatim; existing `New(...)` call sites (lines 13, 57, 83, 115, 121, 137, 158 per 08.1 PLAN Task 5) thread `nil` as the seventh `dm` arg. New tests: `TestHandleReady_Draining`, `TestHandleReady_DrainingPrecedesPreInitializing`, `TestHandleReady_DrainingPrecedesLive`, `TestHandleReady_DrainingHeaders`. Concurrent-scrape race-test `TestAdminConcurrentScrapeRace` extended to include `/drain_listeners` in the path-set + a separate goroutine firing `s.dm.Drain()` once mid-test. |
| `internal/admin/serverinfo.go` | MODIFIED | `deriveState` signature widens from `deriveState(ready *atomic.Bool)` to `deriveState(ready *atomic.Bool, dm *drain.Manager)`; NEW first check `if dm != nil && dm.State() == drain.StateDraining { return adminv3.ServerInfo_DRAINING }`; existing LIVE / PRE_INITIALIZING checks preserved unchanged; call site at `buildServerInfo` updates from `deriveState(&s.ready)` to `deriveState(&s.ready, s.dm)`. Per SPEC §6.5. |
| `internal/admin/serverinfo_test.go` | MODIFIED | New tests: `TestHandleServerInfo_StateDraining`, `TestHandleServerInfo_StatePrecedence_LiveOverDraining`, `TestHandleServerInfo_StatePrecedence_PreInitOverDraining`, `TestDeriveState_NilDrainManager`. |
| `internal/listener/manager.go` | MODIFIED | `NewManagerWithBaseDirAndAllowH2C` signature widens to take `dm *drain.Manager` parameter (LBP-1 fifth application; SPEC §6.1); new `dm` field on `Manager`; the field is also stored on each `listenerRuntime` (so the Accept-loop fast-path is field-local rather than chasing back through `*Manager`). NEW method `(m *Manager) Drain()` per SPEC §6.6 — delegates to `m.dm.Drain()`. The Accept-loop body in `(rt *listenerRuntime) acceptLoop` (currently at line ~783) gains a TWO-line fast-path AT THE TOP of each iteration AFTER `ln.Accept()` returns: if `rt.dm != nil && rt.dm.IsDraining()` then `_ = raw.Close(); continue` (no filter-chain dispatch; the existing 06.1 +2-LoC accept-site Inc lines are NOT executed for the drained-conn case). Existing `Stop()` method preserved unchanged (post-drain teardown). N-1 carry-forward (08.1 REVIEW per SPEC §10.2): one-line doc-comment on `Listeners()` saying "order is bootstrap-declaration order; callers needing alphabetical ordering must sort." The `filterRegistry` map's HCM and TCP-proxy constructor closures (lines 66 and 73) widen to thread `dm` through to the per-typeURL filter constructors. |
| `internal/listener/manager_test.go` | MODIFIED | Existing tests update `NewManagerWithBaseDirAndAllowH2C(...)` call sites to thread `nil` as the new `dm` arg. New tests: `TestManager_Drain`, `TestManager_DrainIdempotent`, `TestManager_AcceptDuringDrainClosesConn`, `TestManager_StopAfterDrain`. |
| `internal/cluster/manager.go` | MODIFIED | NEW method `(m *Manager) Drain()` per SPEC §6.7 — walks `m.clusters` map and calls `c.closePool()` on each cluster. Best-effort; no error return; idempotent. |
| `internal/cluster/cluster.go` | MODIFIED | NEW unexported method `(c *Cluster) closePool()` per planner-time decision 6 — closes the per-cluster connection-pool resources (the cluster has no exported pool field today; the method is a forward-extensible hook that 08.2 lands as a stub closing whatever pooled resources `Cluster` carries at impl-time, with a doc-comment cross-referring to §2.1 deferral notes for HTTP/2 ClientConn close semantics). Best-effort; ignore errors. |
| `internal/cluster/manager_test.go` | MODIFIED | New tests: `TestManager_Drain_ClosesPools`, `TestManager_Drain_Idempotent`. |
| `internal/filter/hcm/config.go` | MODIFIED | `Filter` struct gains a `dm *drain.Manager` field (per BRAINSTORM Decision 7). `parseFilterWithCtx` signature widens to take `dm *drain.Manager` parameter (the listener-manager's `filterRegistry` HCM constructor closure threads it through). |
| `internal/filter/hcm/filter.go` | MODIFIED | `NewFilterWithCtxAndSinksAndRegistry` (the SOLE HCM constructor per phase 07.1 ADR-0072) signature widens to take `dm *drain.Manager` parameter. The exact request-begin/request-end Inc/Dec hook sites are settled at impl-task time per the codebase reality — H1.1 path is `connection.go::runConnection` (per request, NOT per connection — multiple keep-alive requests on one conn each Inc/Dec); H2 path is `h2dispatch.go` (per request — one Inc/Dec per H2 stream). The `markedInflight bool` sentinel field lives on whatever per-request struct is the natural lifetime owner (per planner-time decision 4 + SPEC §12 #4: on the per-request HCM "stream"-level struct; the implementer settles the exact field placement at impl-task time). |
| `internal/filter/hcm/filter_test.go` | MODIFIED | Existing tests update call sites to thread `nil` as the new `dm` arg. New tests: `TestHCM_DrainInflightBalance`, `TestHCM_DrainInflightBalance_SendLocalReply`. |
| `internal/filter/tcpproxy/filter.go` | MODIFIED | `Filter` struct gains a `dm *drain.Manager` field. `NewFilter` signature widens to take `dm *drain.Manager` parameter. `Handle` body adds Inc at top (after `ctx.Err()` check, before `Dial`) and matching Dec via `defer` (per BRAINSTORM Decision 7 + planner-time decision 5). |
| `internal/filter/tcpproxy/filter_test.go` | MODIFIED | Existing tests update call sites to thread `nil` as the new `dm` arg. New test: `TestTCPProxy_DrainInflightBalance`. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW `drainMgr := drain.New(30 * time.Second)` allocation post-`bootstrap.Load`, before `cluster.NewManagerWithBaseDir`. Threaded into `listener.NewManagerWithBaseDirAndAllowH2C(..., drainMgr)` and into `admin.New(adminAddr, bs.Stats, bs, cm, lm, drainMgr)`. The `<-ctx.Done()` block at line 170 upgrades per SPEC §6.8: `<-ctx.Done()` → `drainMgr.Drain()` → `select { <-drainMgr.Done(): / <-time.After(drainMgr.Timeout()): }` → `cm.Drain()` → existing deferred-stop chain runs (LIFO: `lm.Stop`, `admSrv.Close`, sinks-close). |
| `test/fixtures/0010-graceful-drain/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. |
| `test/fixtures/0010-graceful-drain/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port 9902 in-container; listener port 10001; cluster `c_backend` with one STATIC endpoint at `127.0.0.1:18001` — the slow-streaming Go HTTP backend). Per SPEC §7.4. |
| `test/fixtures/0010-graceful-drain/envoy-go.yaml` | NEW | Subject envoy-go bootstrap (admin port resolved from yaml at boot; listener port resolved at boot; cluster `c_backend` with one STATIC endpoint at `127.0.0.1:18001`). Identical to `envoy.yaml` modulo admin/listener port values. Per SPEC §7.4. |
| `test/fixtures/0010-graceful-drain/expectations.yaml` | NEW | Prose narrative of the per-state-transition equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-step assertions). Documents: steady-state /ready byte-equal `LIVE\n`; POST /drain_listeners response byte-equal `OK\n`; /ready DRAINING byte-equal `DRAINING\n`; /server_info DRAINING `state` field byte-equal `"DRAINING"`; in-flight-request body byte-equal across both proxies. Cross-refs SPEC §7.1 + §13.5 + ADR-0093 + ADR-0097 + ADR-0098. Per SPEC §4.3 + §7.1. |
| `test/fixtures/0010-graceful-drain/README.md` | NEW | Fixture overview + per-state-transition equivalence-claim narrative + dual-driver-path description (admin-trigger + SIGTERM-trigger) + Envoy-deviation note (envoy-go's SIGTERM triggers drain; Envoy v1.37.2's SIGTERM is immediate-exit; only the admin-trigger driver runs differentially) + planner-time decision cross-references. Per SPEC §4.3. |
| `test/fixtures/0010-graceful-drain/driver/driver.go` | NEW | Go driver implementing the §7.2 admin-trigger path (against both proxies) + §7.3 SIGTERM-trigger path (against envoy-go-only). Event-based synchronization throughout (no hardcoded sleeps per 07.2 REVIEW M-8 carry-forward + 08.1 SPEC §10). Registers `RequiresReference: true` (admin-trigger path requires reference Envoy; the SIGTERM-trigger path is structural). |
| `test/fixtures/0010-graceful-drain/backends/backend.go` | NEW | Minimal Go HTTP backend bound to port 18001. `/slow` endpoint streams 5KB at 1KB/s (5s total response time); `/` endpoint serves a fast `200 OK\nbackend1\n` for sanity. Per SPEC §7.5. |
| `test/differential/runner_test.go` | MODIFIED | Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver"` (insert in alphabetical order, after the `0009-...` import). ~1 LoC delta. **Note:** the actual fixture-registration site is `runner_test.go` (verified at master `0fc63f6` against the 0008/0009 precedent at lines 33–34); SPEC §4.3's reference to `runner.go` is a SPEC erratum — the implementer MUST add the blank-import to `runner_test.go`. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### /drain_listeners` subsection under existing `## Admin API` umbrella; (b) `### /ready` extension paragraph (DRAINING body + forward-pointer to ADR-0097 partially superseding ADR-0015); (c) `### /server_info` extension (state-enum extended + forward-pointer to ADR-0098 amending ADR-0088); (d) NEW sibling `## Graceful drain` umbrella section per §13.4 (drain-state-machine semantics independent of admin API; covers triggers, drain-semantics, drain-timeout, connection-level drain semantics, drain-manager API surface, `### Applies to` and `### Does not yet apply to` lists); (e) three new equivalence-matrix rows per §13.5; (f) ADR-0015 + ADR-0088 + ADR-0090 forward-pointer notes. ADR-0052 in-place edit authorisation carries forward. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append nine new ADRs ADR-0091..ADR-0099 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). In-place amendment edits to ADR-0085 (LBP-1 fifth-application enumeration), ADR-0088 (DRAINING enum-coverage addition), ADR-0089 (`/drain_listeners` line flip from "08.2" to "delivered in 08.2 per ADR-0093"), and ADR-0090 (no-method-discrimination posture qualified to read-only endpoints) per the ADR-0089 consequence (b) in-place-edit pattern. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `08.2` `in-progress → done` flip AT the phase-done commit; SIMULTANEOUSLY parent row `08` `in-progress → done` flip per parent SPEC §5 (BOOTSTRAP_PROMPT.md §8 MVP-trunk closure). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance to MVP-trunk-closed `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle. `next-skill: superpowers:brainstorming` (against §9's family list); `active-phase: <next-family-row-id>` (planner of next session selects); `last-commit: <08.2 phase-done SHA>`. |
| `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..08.1 PROGRESS.md structure. |
| `docs/envoy-go/phases/08.2-graceful-drain/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 cadence; populates per the requesting-code-review skill. The REVIEW must additionally close the parent 08 row (the 08-family review covers 08.1 + 08.2 jointly per parent SPEC §5). |

---

## Planner-time deferred-decision resolution (settles SPEC §12)

The planner is required by SPEC §12 to settle the SPEC's nine deferred decisions before implementation; this PLAN settles all nine plus a tenth that emerged at PLAN-drafting time (the nil-dm 500-vs-200 policy, item 10 below). The ten resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **`FuzzDrainTransitions` ship-or-skip → SHIP.** Per SPEC §12 #1 recommendation. The fuzzer asserts state-monotonicity (state never decreases) + inflight-balance under randomized Inc/Dec sequences + Done-fires-once invariant under concurrent operations. ~60 LoC; 30s budget per ADR-0018; eleventh fuzzer overall. Lands in Task 2 alongside `internal/drain/manager.go`. *Anchored: SPEC §12 #1 + §14.5; ADR-0018.*

2. **`drain.New(timeout)` argument validation → trust the caller; document the precondition.** Per SPEC §12 #2 recommendation. The Manager does NOT defensively panic or clamp on `timeout <= 0`. The doc-comment on `New` documents that callers should pass `timeout > 0`; the SIGTERM-handler in `cmd/envoy-go/main.go` always passes `30 * time.Second`; test code may pass small values like `10 * time.Millisecond`. *Anchored: SPEC §12 #2.*

3. **`Manager.Done()` semantics when Drain has not been called → return an open channel that NEVER closes (until Drain fires AND inflight reaches 0 OR the caller selects on a side-channel timeout).** Per SPEC §12 #3. The natural Go channel pattern; the doc-comment documents the precondition. The SIGTERM-handler always calls `Drain()` before `select`-ing on `Done()`, so the precondition is satisfied at every production call site. *Anchored: SPEC §12 #3.*

4. **`markedInflight` flag placement → on the per-request HCM "stream" struct.** Per SPEC §12 #4 recommendation. The Stream (or its codebase equivalent — the per-request struct that owns the request lifetime through decode and encode) is the natural lifetime owner; placing the flag elsewhere would require additional plumbing across the filter chain. The exact field placement (e.g., on a struct in `internal/filter/hcm/connection.go` for the H1.1 path; on a struct in `internal/filter/hcm/h2dispatch.go` for the H2 path) is settled at impl-task time per the codebase reality; the field is a `bool` initialized to `false`, set to `true` by the Inc-site, and consulted by the Dec-site via `if s.markedInflight { dm.Dec(); s.markedInflight = false }`. *Anchored: SPEC §12 #4 + ADR-0075.*

5. **TCP-proxy Inc/Dec anchor → OnNewConnection (i.e., at top of `Handle` after `ctx.Err()` check, before `Dial`).** Per SPEC §12 #5 recommendation. Per-connection granularity is correct because TCP-proxy has no per-request semantic. The lazy-increment-at-first-byte alternative complicates pair-balance and is not justified by any current operator workflow. The matching Dec is `defer`-d immediately after the Inc so all early-return paths (dial failure, context cancellation) decrement correctly. *Anchored: SPEC §12 #5.*

6. **`internal/cluster/manager.go` `closePool()` per-cluster method shape → `func (c *Cluster) closePool()` with no return value; iterates whatever pooled resources `Cluster` carries today (08.2 lands a stub if no exported pool field exists yet); future hot-restart family expands.** Per SPEC §12 #6 recommendation. Best-effort; ignore errors. The doc-comment cross-refs §2.1 deferred features (per-connection drainable closure at next idle window; per-listener drain stats; etc.). The implementer at Task 4 inspects `internal/cluster/cluster.go` to confirm whether HTTP/1.1 keep-alive pool, HTTP/2 ClientConn instances from phase 05.2, and TLS upstream connections from phase 03 are exported in some form; if not, `closePool()` is a no-op-with-log stub today and the operator-affordances expansion is recorded as a forward note. *Anchored: SPEC §12 #6.*

7. **`drainMgr` boot-order placement in `cmd/envoy-go/main.go` → after `bootstrap.Load`, before `cluster.NewManagerWithBaseDir`.** Per SPEC §12 #7 recommendation. The drain manager has no dependencies but is consumed by all subsequent constructors (listener manager directly, admin server directly, HCM/TCP-proxy filter constructors transitively via the listener manager); placing it after Load and before the first constructor that consumes it is the cleanest ordering. *Anchored: SPEC §12 #7.*

8. **Fixture 0010 driver framework reuse → share dual-proxy boot helpers and admin-scrape helpers with 0009; do NOT share canonicalisation.** Per SPEC §12 #8 recommendation. 0010's per-state-transition byte-equality is structurally different from 0009's structural-projection canonicalisation. If `test/differential/helpers/` is the natural shared location for boot helpers, refactor at impl-task time; otherwise, the 0010 driver duplicates a small amount of admin-scrape boilerplate (acceptable per ADR-0017's small-mechanical-fixes posture). The N-4 carry-forward (cross-reference doc-comment in fixture 0009 canonicaliser) lands inline IF AND ONLY IF the 0010 driver shares utilities with 0009; otherwise stays carry-forward per SPEC §10.2. *Anchored: SPEC §12 #8 + §10.2 (N-4).*

9. **`cm.Drain()` call ordering vs deferred-stop chain → explicit call after rendezvous, before deferred-stop chain runs.** Per SPEC §12 #9 recommendation. The deferred-call alternative would intersperse the upstream-pool close inside the listener-socket-close, which is correct but harder to read. The explicit call is grep-discoverable and easier to reason about. The deferred-stop chain (LIFO: `lm.Stop`, `admSrv.Close`, sinks-close per phase 06.2) runs AFTER `cm.Drain()` returns; this matches the SPEC §5.2 swimlane ordering. *Anchored: SPEC §12 #9 + §5.2.*

10. **`POST /drain_listeners` with `nil` drain manager → return 500 Internal Server Error.** Settles the planner-deferred policy that Task 7's `internal/admin/drain.go` body and `TestHandleDrainListeners_NilDrainManager` reference. The defensive 500 stance is preferred over a no-op-200 because: (a) production main always wires `drainMgr` per planner-time decision 7 above, so a nil dm at runtime is a configuration bug — silent 200 would mask it; (b) admin-endpoint test code constructs `admin.New(...)` with nil dependencies for read-only-endpoint isolation per the 08.1 nil-tolerance pattern (ADR-0085), so test code that exercises non-drain endpoints with `nil` dm continues to work — the nil dm only matters when `/drain_listeners` itself is hit; (c) 500 is grep-discoverable in operator logs and matches the "fail loud at integration boundaries" posture. The body for the 500 path is `drain manager not configured\n` (lowercase, terse, mirrors the SPEC §11 envoy-evidence textual style). Settles SPEC §14.2's `TestHandleDrainListeners_NilDrainManager` ambiguity inline. *Anchored: ADR-0085 nil-tolerance pattern + Task 7 body.*

These ten decisions are reproduced verbatim in `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The nine ADRs anticipated by SPEC §8 (ADR-0091..ADR-0099). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The nine ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0091 | Drain state-machine shape (LIVE / DRAINING / DRAINED-as-channel-close) + new `internal/drain/` package + LBP-1 fifth-application threading | Task 2 (`internal/drain/manager.go` first lands). |
| ADR-0092 | SIGTERM and SIGINT trigger drain-then-exit; deliberate divergence from Envoy v1.37.2's SIGTERM=immediate-exit per §11.7 | Task 11 (`cmd/envoy-go/main.go` SIGTERM-handler upgrade). |
| ADR-0093 | POST /drain_listeners contract: 200 OK with body `OK\n`; method-discrimination ENFORCED (405 on non-POST per §11.4); idempotent; ?graceful=true silent-ignored. Partially amends ADR-0090. | Task 7 (`internal/admin/drain.go` first lands). |
| ADR-0094 | Listener stop-accepting via per-runtime Accept-loop fast-path on `dm.IsDraining()`; listener-socket close stays at `Stop()` (post-drain teardown). Accept-then-FIN per §11.5. | Task 5 (`internal/listener.Manager.Drain()` + Accept-loop fast-path). |
| ADR-0095 | Drain timeout default: hardcoded 30s in envoy-go MVP; deliberate divergence from Envoy v1.37.2 default 600s (per §11.7 + 08.1 SPEC §11.4); operator-knob deferred to a future runtime/hot-restart family phase | Task 11 (`cmd/envoy-go/main.go` boot wiring — the `30 * time.Second` literal lives at the call site per ADR-0095 design). |
| ADR-0096 | In-flight-completion discipline: HCM Inc/Dec pair per request; TCP-proxy Inc/Dec pair per connection; cluster.Manager.Drain best-effort upstream-pool close after `<-drainMgr.Done()`. NO `Connection: close` on H1.1 in-flight responses per §11.3. | **Task 4** (`internal/cluster.Cluster.closePool()` + `Cluster.Manager.Drain()` accessor — the cluster-side anchor; Task 6 is a documented no-op placeholder slot consolidated INTO Task 4); Tasks 9 (HCM hooks) + 10 (TCP-proxy hooks) realize the HCM/TCP-proxy components citing ADR-0096 in their commit messages. |
| ADR-0097 | /ready DRAINING-state body `DRAINING\n` per §11.2; DRAINING-precedence-over-PRE_INITIALIZING-and-LIVE rule. PARTIALLY SUPERSEDES ADR-0015. | Task 8 (`internal/admin/admin.go::handleReady` modification). |
| ADR-0098 | /server_info `state` field DRAINING transition; `deriveState` extended to consult `*drain.Manager`. AMENDS ADR-0088 purely additively. | Task 8 (`internal/admin/serverinfo.go::deriveState` modification — landed alongside ADR-0097 in the same task because both are DRAINING-extension edits to the existing 08.1 endpoints, mirroring the SPEC §8 grouping). |
| ADR-0099 | Hot-restart deferral; envoy-go's drain is one-process scope only; future runtime/hot-restart family delivers SCM_RIGHTS-based handoff. | Task 12 (BEHAVIOR_CONTRACT umbrella — the `### Does not yet apply to` extension under `## Graceful drain` IS the deferral table that ADR-0099 codifies). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Doctrine / Lands-in-task / Context / Decision / Consequences / Supersedes / Amended-by); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Inline supersessions / amendments anticipated** (recorded inline in the listed ADRs above per the ADR-0089 consequence (b) in-place-edit pattern; NOT separate ADRs):

- **ADR-0015** (pre-init contract for /ready) — partially superseded by **ADR-0097**. ADR-0015's verbatim pre-init body (`PRE_INITIALIZING\n`) and pre-init status (503) are preserved; ADR-0097 adds the DRAINING branch and the precedence rule. Forward-pointer note appended in-place to ADR-0015 in DECISIONS.md.
- **ADR-0088** (`/server_info` state-enum coverage) — amended by **ADR-0098** (purely additive DRAINING enum coverage). Per ADR-0088 consequence (c), the amendment is purely additive; recorded as an in-place edit of ADR-0088's Consequences section.
- **ADR-0085** (admin-mux reuse + LBP-1 third application) — consequence (a) extended in-place to enumerate the 08.2 LBP-1 fifth-application threading of `*drain.Manager`. Per the LBP-1 generalization pattern, the extension is in-place; no new ADR.
- **ADR-0089** (admin-endpoint deferral list) — POST /drain_listeners line flips from "08.2 (graceful drain)" to "delivered in 08.2 per ADR-0093". Per ADR-0089 consequence (b), the table is updated in-place; no new ADR for the disposition flip. The `/healthcheck/fail` line stays in the deferral list (envoy-go does not implement that endpoint in MVP per §2.2).
- **ADR-0090** (no-ACL admin-endpoint security posture; no method discrimination) — partially amended by **ADR-0093** (mutating endpoint /drain_listeners DOES enforce method discrimination per §11.4). The amendment is recorded as an in-place edit of ADR-0090's Consequences section per the ADR-0089 consequence (b) pattern. The no-ACL posture is preserved verbatim; the no-method-discrimination posture is qualified to read-only endpoints only.

These five in-place edits land at Task 12 (alongside the BEHAVIOR_CONTRACT.md restructure) so the entire DECISIONS.md disposition is consistent at the phase-done commit.

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-08.2-graceful-drain-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003 + the per-phase-worktree convention: `git worktree add .worktrees/phase-08.2-graceful-drain-impl -b phase-08.2-graceful-drain-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -6` shows the PLAN.md commit (this plan) and (optionally) its SHA-fill follow-up at the head, with the SPEC.md commit `546b08a` and its SHA-fill follow-up `0fc63f6` immediately before, then the BRAINSTORM.md commit `e7b64ac` and its SHA-fill `3ae6af7`, then 08.1's phase-done at `70e6a65` (or its SHA-fill `eb3babd`). If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0090:`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0091..ADR-0099 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` returns `546b08a` (the SPEC commit). If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status -uall --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009'` returns every fixture PASS. The 11 pre-existing fixtures (0000–0009) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 10 fuzzers from phases 02–08.1 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **Pre-existing `internal/drain/` directory does NOT exist.** `test ! -d internal/drain` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
12. **Pre-existing admin server has the 6-param constructor signature this PLAN widens.** `grep -nE '^func New\(addr string, registry \*stats\.Registry, bs \*bootstrap\.Bootstrap, cm \*cluster\.Manager, lm \*listener\.Manager\) \*Server' internal/admin/admin.go` returns exactly 1 match. If 0, the constructor has already been widened by a concurrent phase — investigate.
13. **Pre-existing `Listener.Manager` does NOT yet have `Drain()` method.** `grep -nE '^func \(m \*Manager\) Drain\(\)' internal/listener/manager.go` returns empty. If non-empty, the accessor has been added by a concurrent phase.
14. **Pre-existing `Cluster.Manager` does NOT yet have `Drain()` method.** `grep -nE '^func \(m \*Manager\) Drain\(\)' internal/cluster/manager.go` returns empty.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).

If all 15 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the nine ADRs ADR-0091..ADR-0099 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the nine ADRs and records the planner-time decisions resolution. The PROGRESS preamble reproduces the nine planner-time deferred-decisions resolution items from this PLAN's `## Planner-time deferred-decision resolution` section verbatim, so any task-N reader has the full context without back-reading this PLAN.

**Precondition:** worktree exists at `phase-08.2-graceful-drain-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 15 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-08.2-graceful-drain-impl
git log --oneline master | head -8                                    # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (0fc63f6), SPEC (546b08a), BRAINSTORM SHA-fill (3ae6af7), BRAINSTORM (e7b64ac), 08.1 SHA-fill (eb3babd), 08.1 phase-done (70e6a65)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                          # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
                                                                       # expect: ADR-0090:
git log -1 --format=%H -- docs/envoy-go/phases/08.2-graceful-drain/SPEC.md
                                                                       # expect: 546b08a... or descendant
git status -uall --porcelain                                           # expect: empty
test ! -d internal/drain && echo "ok: internal/drain absent"
grep -nE '^func New\(addr string, registry \*stats\.Registry, bs \*bootstrap\.Bootstrap, cm \*cluster\.Manager, lm \*listener\.Manager\) \*Server' internal/admin/admin.go
                                                                       # expect: 1 match
grep -nE '^func \(m \*Manager\) Drain\(\)' internal/listener/manager.go
                                                                       # expect: empty
grep -nE '^func \(m \*Manager\) Drain\(\)' internal/cluster/manager.go
                                                                       # expect: empty
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md`**

```markdown
# Phase 08.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..08.1 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 15 preconditions were satisfied at cold-start>

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
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/08.2-graceful-drain/SPEC.md
<verbatim>
\`\`\`
```

- [ ] **Step 3: Run preconditions verbatim and confirm pristine state**

```bash
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
go test -race -count=1 -short ./...                           # expect: all PASS (short mode skips differential)
```

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md
git commit -m "phase 08.2: PROGRESS preamble + planner-time decision resolution"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR anticipation table), §12 (deferred decisions), §15 (acceptance criteria) and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `internal/drain/` package — `Manager` type with three-state machine + `FuzzDrainTransitions` [ADR-0091]

**Files:**
- Create: `internal/drain/doc.go`
- Create: `internal/drain/manager.go`
- Create: `internal/drain/manager_test.go`
- Create: `internal/drain/fuzz_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0091)

This task lands the new `internal/drain/` package with the `Manager` type implementing the three-state drain machine per SPEC §6.2 + §5.9. Lock-free hot path: `atomic.Uint32` state + `atomic.Int64` inflight; `chan struct{}` rendezvous; `sync.Once` Drain-guard + `sync.Once` close-done-guard. ADR-0091 (drain state-machine shape; new package; LBP-1 fifth-application discipline) lands here. Per planner-time decision 1, the OPTIONAL `FuzzDrainTransitions` fuzzer is SHIPPED (eleventh fuzzer overall; ~60 LoC; 30s budget per ADR-0018).

**Precondition:** Task 1 done; `internal/drain/` does not exist.
**Artifact:** four new files (doc + impl + unit tests + fuzz); ADR-0091 in DECISIONS.md.
**Acceptance:** `go build ./internal/drain/...` clean; `go test ./internal/drain/...` passes; `go test -race ./internal/drain/...` clean; `go test -fuzz=FuzzDrainTransitions -fuzztime=30s ./internal/drain/` clean; ADR-0091 in DECISIONS.md.

- [ ] **Step 1: Write failing tests in `internal/drain/manager_test.go`**

```go
package drain

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStateTransitions(t *testing.T) {
	m := New(10 * time.Millisecond)
	if got := m.State(); got != StateLive {
		t.Errorf("initial State: got %v, want StateLive", got)
	}
	m.Drain()
	if got := m.State(); got != StateDraining {
		t.Errorf("post-Drain State: got %v, want StateDraining", got)
	}
	// Idempotent: another Drain stays in Draining.
	m.Drain()
	if got := m.State(); got != StateDraining {
		t.Errorf("post-second-Drain State: got %v, want StateDraining", got)
	}
}

func TestInflightBalance(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Inc()
	m.Inc()
	m.Dec()
	m.Dec()
	// No public accessor for raw inflight; balance is observed via Done()
	// rendezvous: after Drain(), if balance is 0, Done() closes.
	m.Drain()
	select {
	case <-m.Done():
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire after balanced Inc/Dec + Drain")
	}
}

func TestDrainCompletionRendezvous(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Inc()
	m.Drain()
	// Done() should NOT fire while inflight > 0.
	select {
	case <-m.Done():
		t.Fatalf("Done() fired while inflight > 0")
	case <-time.After(20 * time.Millisecond):
		// expected
	}
	m.Dec()
	select {
	case <-m.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire after Dec → 0 post-Drain")
	}
}

func TestDrainTimeout_NoInflight(t *testing.T) {
	m := New(1 * time.Hour)  // huge timeout; no Inc; Done() should close immediately
	m.Drain()
	select {
	case <-m.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire when inflight already 0 at Drain time")
	}
}

func TestDrainTimeout_StuckInflight_CallerEnforces(t *testing.T) {
	// Per ADR-0095 design: the Manager itself does NOT enforce timeout.
	// The caller (cmd/envoy-go/main.go) selects on time.After alongside Done().
	m := New(10 * time.Millisecond)
	m.Inc()  // never Dec'd
	m.Drain()
	select {
	case <-m.Done():
		t.Fatalf("Done() fired with stuck inflight; Manager should NOT auto-timeout")
	case <-time.After(time.Until(time.Now().Add(50 * time.Millisecond))):
		// expected — Manager does not enforce timeout itself
	}
	if got := m.Timeout(); got != 10*time.Millisecond {
		t.Errorf("Timeout(): got %v, want 10ms", got)
	}
}

func TestIdempotentDrain(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Drain()
	m.Drain()
	m.Drain()
	// Multiple Drain calls; only one transition fires; Done() closes once
	// (closeOnce-guarded). A re-close would panic — the test passes if no
	// panic occurs across the goroutine fan-in below.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Drain() }()
	}
	wg.Wait()
	select {
	case <-m.Done():
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire under concurrent Drain")
	}
}

func TestIsDrainingFastPath(t *testing.T) {
	m := New(10 * time.Millisecond)
	if m.IsDraining() {
		t.Errorf("IsDraining() pre-Drain: got true, want false")
	}
	m.Drain()
	if !m.IsDraining() {
		t.Errorf("IsDraining() post-Drain: got false, want true")
	}
}

func TestNilSafety(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil-receiver method call; got none")
		}
	}()
	var m *Manager
	_ = m.IsDraining()  // pointer-receiver method on nil panics; this is documented behavior
}

func TestConcurrentIncDec(t *testing.T) {
	m := New(1 * time.Second)
	const N = 100
	const M = 1000
	var wg sync.WaitGroup
	var balanced atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				m.Inc()
				balanced.Add(1)
				m.Dec()
				balanced.Add(-1)
			}
		}()
	}
	wg.Wait()
	if got := balanced.Load(); got != 0 {
		t.Errorf("Inc/Dec balance: got %d, want 0", got)
	}
	// After all Inc/Dec, drain should rendezvous immediately.
	m.Drain()
	select {
	case <-m.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Done() did not fire after balanced concurrent Inc/Dec + Drain")
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/drain/... 2>&1 | head -10
```

Expected: package does not exist (`no Go files in internal/drain`).

- [ ] **Step 3: Write `internal/drain/doc.go`**

```go
// Package drain owns the envoy-go drain-state machine. Phase 08.2 (SPEC §1)
// introduces this package as the LBP-1 fifth application of the explicit-
// threading discipline (after *stats.Registry per 06.1, *HTTPRegistry per
// 07.1, *ListenerFilterRegistry per 07.2, and the 08.1 *bootstrap.Bootstrap
// + *cluster.Manager + *listener.Manager triplet threaded into admin.New).
//
// The three-state drain machine (per SPEC §5.9):
//
//	LIVE   ──Drain()──→ DRAINING ──inflight==0 OR timeout──→ DRAINED
//	                                  (Done channel closes; State() still
//	                                   returns Draining — DRAINED is observable
//	                                   ONLY via channel close, not via State())
//
// The Manager is allocated once at boot in cmd/envoy-go/main.go and threaded
// into admin.New, listener.NewManagerWithBaseDirAndAllowH2C, and (via the
// listener manager's filterRegistry) into the HCM and TCP-proxy filter
// constructors. Test code that does not exercise drain semantics may pass nil.
//
// Concurrency model: hot-path operations (State, Inc, Dec, IsDraining) are
// lock-free atomic operations against atomic.Uint32 (state) and atomic.Int64
// (inflight); the only synchronization beyond atomics is sync.Once on the
// Drain trigger and sync.Once-equivalent on the done channel close.
//
// The Manager does NOT enforce its configured timeout. The caller (the
// cmd/envoy-go/main.go SIGTERM-handler) selects on time.After alongside
// Done() to bound the drain window per ADR-0095.
//
// See SPEC §6.2 for the API surface; ADR-0091 records the design.
package drain
```

- [ ] **Step 4: Write `internal/drain/manager.go`**

```go
package drain

import (
	"sync"
	"sync/atomic"
	"time"
)

// State is the drain-state-machine state. Atomically loaded via atomic.Uint32.
type State uint32

const (
	// StateLive is the initial state at New(). The proxy is accepting new
	// connections and processing requests normally.
	StateLive State = iota
	// StateDraining is the post-Drain() state. The proxy rejects new
	// connections via accept-then-FIN; in-flight requests complete normally.
	StateDraining
	// StateDrained is the post-rendezvous state. NOT publicly exposed via
	// State() per SPEC §5.9 design; observable only via Done() channel
	// close. State() continues to return StateDraining post-rendezvous.
	StateDrained
)

// Manager is the drain-state machine. Allocated once at boot per phase 08.2
// SPEC §5.1 boot-order; consumed by admin.Server, listener.Manager, HCM
// filter, TCP-proxy filter (per BRAINSTORM Decision 4 surface-area).
//
// Lock-free hot path: State, IsDraining, Inc, Dec are atomic operations.
// The only synchronization beyond atomics is sync.Once on Drain (so
// concurrent triggers from handleDrainListeners + the SIGTERM-handler are
// safe) and a sync.Once-equivalent guard on the done-channel close.
type Manager struct {
	state     atomic.Uint32 // load/store of State as uint32
	inflight  atomic.Int64
	done      chan struct{}
	timeout   time.Duration
	once      sync.Once // guards Drain transition
	closeOnce sync.Once // guards close(done)
}

// New constructs a Manager in StateLive with the given drain timeout. Callers
// should pass timeout > 0; the Manager does NOT defensively panic or clamp on
// timeout <= 0 (per planner-time decision 2; SPEC §12 #2). The
// SIGTERM-handler in cmd/envoy-go/main.go always passes 30 * time.Second
// per ADR-0095; test code may pass small values like 10 * time.Millisecond.
//
// The timeout is consulted by the SIGTERM-handler (the Manager itself does
// NOT enforce the timeout — callers select on time.After(m.Timeout())
// alongside <-m.Done() per ADR-0095 design).
func New(timeout time.Duration) *Manager {
	return &Manager{
		done:    make(chan struct{}),
		timeout: timeout,
	}
}

// State atomically loads the current state. Lock-free. Returns StateLive or
// StateDraining (StateDrained is NOT publicly exposed via this method per
// SPEC §5.9).
func (m *Manager) State() State {
	return State(m.state.Load())
}

// IsDraining is the Listener Accept-loop fast-path check. Equivalent to
// State() == StateDraining. Lock-free; one atomic load.
func (m *Manager) IsDraining() bool {
	return State(m.state.Load()) == StateDraining
}

// Drain transitions the state from StateLive to StateDraining. Idempotent
// (sync.Once-guarded): on the first call, atomically transitions state and
// closes done if inflight is already 0. Subsequent calls no-op. On a Drain
// when inflight > 0, the matching Dec call (when it brings inflight to 0)
// closes done.
func (m *Manager) Drain() {
	m.once.Do(func() {
		m.state.Store(uint32(StateDraining))
		// If inflight is already 0, the rendezvous fires immediately.
		if m.inflight.Load() == 0 {
			m.closeOnce.Do(func() { close(m.done) })
		}
	})
}

// Done returns a channel that is closed when the drain rendezvous fires —
// i.e., when inflight reaches 0 after Drain has been called. If Drain has
// not been called, Done is open and never closes (until Drain is called).
// If Drain is called when inflight is already 0, Done closes immediately.
// The channel is closed exactly once (closeOnce-guarded).
func (m *Manager) Done() <-chan struct{} {
	return m.done
}

// Inc atomically increments the inflight counter. Called by HCM at request-
// begin and by TCP-proxy at conn-begin. Lock-free.
func (m *Manager) Inc() {
	m.inflight.Add(1)
}

// Dec atomically decrements the inflight counter. Called by HCM at request-
// end and by TCP-proxy at conn-end. If the decrement brings inflight to 0
// AND Drain has fired, this method closes the done channel (closeOnce-
// guarded). Lock-free.
func (m *Manager) Dec() {
	if m.inflight.Add(-1) == 0 && State(m.state.Load()) == StateDraining {
		m.closeOnce.Do(func() { close(m.done) })
	}
}

// Timeout returns the configured drain timeout (the value passed to New).
// Read-only; never changes after construction. The SIGTERM-handler in
// cmd/envoy-go/main.go calls this to set the time.After bound.
func (m *Manager) Timeout() time.Duration {
	return m.timeout
}
```

- [ ] **Step 5: Run unit tests; confirm they pass**

```bash
go test -count=1 ./internal/drain/... -v 2>&1 | tail -25
go test -race -count=1 ./internal/drain/... 2>&1 | tail -10
go vet ./...
golangci-lint run ./internal/drain/...
```

Expected: 9 PASS; race clean; vet clean; lint clean.

- [ ] **Step 6: Write `internal/drain/fuzz_test.go`** (per planner-time decision 1; SHIP)

```go
package drain

import (
	"sync/atomic"
	"testing"
	"time"
)

// FuzzDrainTransitions fuzzes a sequence of operations against a Manager and
// asserts (i) state-monotonicity (state never decreases — Live → Draining
// only); (ii) inflight balance (every Inc has a matching Dec); (iii) Done
// fires exactly once after Drain has been called and inflight reaches 0.
//
// Per ADR-0018 fuzz CI 30s short-budget policy. Per SPEC §14.5 + §12 #1.
func FuzzDrainTransitions(f *testing.F) {
	f.Add(uint8(0b10101010), uint8(5))
	f.Add(uint8(0b00000001), uint8(1))
	f.Add(uint8(0b11111111), uint8(8))
	f.Fuzz(func(t *testing.T, ops uint8, n uint8) {
		if n > 8 {
			n = 8
		}
		m := New(1 * time.Hour)
		var balance atomic.Int64
		drainCalled := false
		for i := uint8(0); i < n; i++ {
			op := (ops >> i) & 1
			if op == 0 {
				m.Inc()
				balance.Add(1)
			} else {
				if balance.Load() > 0 {
					m.Dec()
					balance.Add(-1)
				}
			}
		}
		// Trigger drain at the end and balance any residual inflight.
		m.Drain()
		drainCalled = true
		for balance.Load() > 0 {
			m.Dec()
			balance.Add(-1)
		}
		// State must be StateDraining post-Drain (monotonicity invariant).
		if got := m.State(); got != StateDraining {
			t.Fatalf("state monotonicity violated: got %v, want StateDraining", got)
		}
		// Inflight must be 0 (balance invariant).
		if got := balance.Load(); got != 0 {
			t.Fatalf("inflight balance violated: got %d, want 0", got)
		}
		// Done must fire (rendezvous invariant).
		if drainCalled {
			select {
			case <-m.Done():
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Done() did not fire after balanced inflight + Drain")
			}
		}
	})
}
```

- [ ] **Step 7: Run the fuzzer at the 30s short-budget**

```bash
go test -fuzz=FuzzDrainTransitions -fuzztime=30s ./internal/drain/ 2>&1 | tail -10
```

Expected: `PASS` after 30s budget; no crashers; no failed invariants.

- [ ] **Step 8: Append ADR-0091 to `docs/envoy-go/DECISIONS.md`**

Append per the ADR-0001 template (Status / Doctrine / Lands-in-task / Context / Decision / Consequences). Body content draws from SPEC §8 ADR-0091 anticipation:

- Status: Accepted.
- Doctrine: D-3.2 + D-3.4 + D-3.5.
- Lands-in-task: Task 2.
- Context: Phase 08.2 SPEC §1 + §6.2 + §5.9 + BRAINSTORM Decision 1 + 4. The drain machinery is consumed by five actors (cmd/envoy-go/main.go, admin.Server, listener.Manager, HCM, TCP-proxy); a single shared Manager threaded via the LBP-1 explicit-threading discipline is the cleanest fit. The three-state machine (LIVE → DRAINING → DRAINED-as-channel-close) minimizes public-API surface while supporting all five consumers' needs: State() answers "should I reject new work" (Draining vs Live); Done() answers "is it safe to teardown" (Drained); Inc/Dec answer "how do I balance the in-flight counter."
- Decision: A new `internal/drain/` package with a `Manager` type implementing the three-state machine. Lock-free hot path via atomic.Uint32 + atomic.Int64; sync.Once on Drain trigger; sync.Once-equivalent on done channel close. DRAINED state is NOT publicly exposed via State() — only via Done() channel close. The Manager does NOT enforce timeout (callers select on time.After alongside Done per ADR-0095). The Manager is the LBP-1 fifth application; threaded into admin.New (Task 3), listener.NewManagerWithBaseDirAndAllowH2C (Task 5), HCM filter constructor (Task 9), TCP-proxy filter constructor (Task 10).
- Consequences: (a) Race-detector-clean for N concurrent scrapes against all seven admin endpoints + a separate goroutine firing Drain mid-test (asserted by extended TestAdminConcurrentScrapeRace). (b) The five consumers' boot wiring is grep-discoverable via `grep -rn drain.Manager internal/ cmd/`. (c) Future hot-restart family (per ADR-0099 deferral) extends this Manager rather than replacing it; SCM_RIGHTS-based parent-child handoff is out of MVP scope. (d) FuzzDrainTransitions (eleventh fuzzer; ADR-0018 30s budget) shipped per planner-time decision 1.

- [ ] **Step 9: Commit**

```bash
git add internal/drain/ docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 08.2: internal/drain/ package — Manager + FuzzDrainTransitions [ADR-0091]

Lands the new internal/drain/ package with the three-state Manager (LIVE /
DRAINING / DRAINED-as-channel-close) per SPEC §5.9 + §6.2. Lock-free hot
path: atomic.Uint32 state + atomic.Int64 inflight + chan struct{} rendezvous
+ sync.Once Drain-guard + sync.Once close-done-guard. Manager does NOT
enforce timeout (callers select on time.After alongside Done per ADR-0095).
Lands FuzzDrainTransitions (eleventh fuzzer; ~60 LoC; 30s budget per
ADR-0018) per planner-time decision 1.

LBP-1 fifth application: drain.Manager joins *stats.Registry / *HTTPRegistry
/ *ListenerFilterRegistry / the 08.1 bs+cm+lm triplet threaded explicitly
into the constructors; admin.New (Task 3), listener.NewManagerWithBaseDir-
AndAllowH2C (Task 5), HCM (Task 9), and TCP-proxy (Task 10) constructors
widen in subsequent tasks.

ADR-0091 records the design.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (deliverables), §5.9 (state diagram), §6.2 (Manager API), §8 (ADR-0091 anticipation), §12 #1 + #2 (planner-time-resolved); ADR-0018; BRAINSTORM Decisions 1 + 4.*

---

## Task 3: `internal/admin.New` constructor widening — thread `*drain.Manager` (LBP-1 fifth application)

**Files:**
- Modify: `internal/admin/admin.go` (widen `New` signature; add `dm` field on `Server`)
- Modify: `internal/admin/admin_test.go` (update existing `New(...)` call sites; add `TestServer_NewWidenedConstructor_DrainManager`)

This task widens `internal/admin.New` from 6-param (08.1 form: `New(addr, registry, bs, cm, lm)`) to 7-param adding `dm *drain.Manager` per SPEC §6.1 + BRAINSTORM Decision 4. Adds a single new field `dm *drain.Manager` to `Server`. The `mux.HandleFunc("/drain_listeners", ...)` registration in `Start()` body lands at Task 7 (alongside the handler). The `handleReady` DRAINING-branch lands at Task 8. This task scope is the constructor widening + field addition only — keeps the diff focused and the LBP-1 fifth-application discipline grep-discoverable.

`cmd/envoy-go/main.go` is BROKEN intermediately by this task (the existing 6-param call site no longer compiles). Task 11 fixes this. Between Tasks 3 and 11, `go build ./cmd/envoy-go/...` will fail; this is intentional and documented in PROGRESS for Task 3.

**Precondition:** Task 2 done; `internal/admin.New` is at the 6-param 08.1 signature.
**Artifact:** widened `New`; new `dm` field on `Server`.
**Acceptance:** `go build ./internal/admin/...` clean; `go test ./internal/admin/...` passes; `go build ./cmd/envoy-go/...` FAILS (call-site breakage; Task 11 fixes); ADR-0085 consequence (a) updated in-place to enumerate the LBP-1 fifth application.

- [ ] **Step 1: Update existing `New(...)` call sites in `internal/admin/admin_test.go`**

For each existing `New("127.0.0.1:0", r, nil, nil, nil)` call site, add `nil` as the 7th arg (the `*drain.Manager`). Add a new test:

```go
func TestServer_NewWidenedConstructor_DrainManager(t *testing.T) {
	r := stats.NewRegistry()
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", r, nil, nil, nil, dm)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.dm != dm {
		t.Errorf("dm field not threaded through New")
	}
	// Existing 08.1 fields still threaded:
	if s.registry != r {
		t.Errorf("registry not threaded")
	}
	if s.bootTime.IsZero() {
		t.Errorf("bootTime not set at New time")
	}
}

func TestServer_NewWidenedConstructor_NilDrainManagerTolerated(t *testing.T) {
	// Test code that does not exercise drain semantics may pass nil per
	// ADR-0085 nil-tolerance + planner-time decision 7.
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, nil)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.dm != nil {
		t.Errorf("dm field should be nil when nil passed")
	}
}
```

Add `"github.com/esalaine/envoy-go/internal/drain"` to the imports if not present.

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/admin/... 2>&1 | head -20
```

Expected: build errors (`too many arguments in call to New`).

- [ ] **Step 3: Edit `internal/admin/admin.go` — widen `New` signature + add `dm` field**

Add `"github.com/esalaine/envoy-go/internal/drain"` to the imports.

Modify the `Server` struct to add the new field (after the existing `lm *listener.Manager` field):

```go
// 08.2 field (per ADR-0091 + BRAINSTORM Decision 4 — LBP-1 fifth application).
// May be nil in test code that does not exercise drain semantics.
dm *drain.Manager
```

Modify the `New` signature:

```go
// New returns an admin server targeting addr. The server is not running yet;
// call Start. ... 08.2 widens the signature to thread *drain.Manager (the
// LBP-1 fifth application; ADR-0085 consequence (a) extended in-place +
// ADR-0091). May be nil in test code that does not exercise drain semantics.
func New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager, dm *drain.Manager) *Server {
	return &Server{
		addr:      addr,
		registry:  registry,
		liveGauge: registry.NewGauge("server.live"),
		bs:        bs,
		cm:        cm,
		lm:        lm,
		dm:        dm,
		bootTime:  time.Now(),
	}
}
```

Update the `Server` struct doc-comment to mention 08.2's drain extension surface.

- [ ] **Step 4: Update ADR-0085 in-place to enumerate LBP-1 fifth application**

In `docs/envoy-go/DECISIONS.md`, locate ADR-0085's Consequence (a) and append a forward-pointer line per the ADR-0089 consequence (b) in-place-edit pattern: "Phase 08.2 (per ADR-0091) extends this consequence with the LBP-1 fifth application: `*drain.Manager` is threaded into `admin.New` (7th param), `listener.NewManagerWithBaseDirAndAllowH2C`, the HCM filter constructor, and the TCP-proxy filter constructor at the same explicit-threading discipline. The cluster manager does NOT take dm — `cm.Drain()` is called from `cmd/envoy-go/main.go` after `<-drainMgr.Done()` rather than threaded as a constructor dep."

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go test ./internal/admin/... 2>&1 | tail -10
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
go build ./cmd/envoy-go/... 2>&1 | tail -3   # expect: FAIL (call-site breakage; Task 11 fixes)
```

Expected: admin tests PASS, vet clean, lint clean, cmd build FAILS as documented.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/admin.go internal/admin/admin_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 08.2: internal/admin.New widens to thread *drain.Manager — LBP-1 fifth application"
```

Note: this commit intentionally leaves cmd/envoy-go broken; Task 11 fixes the call site. SHA-fill follow-up.

*Anchored: SPEC §4.2 (admin.go modified), §6.1 (constructor signatures), §10.1(b) (LBP-1 fifth application carry-forward), ADR-0085 consequence (a) extension; planner-time decision 7.*

---

## Task 4: `internal/cluster.Cluster.closePool()` helper + `Cluster.Manager.Drain()` accessor

**Files:**
- Modify: `internal/cluster/cluster.go` (add `closePool()` helper)
- Modify: `internal/cluster/manager.go` (add `Drain()` method)
- Modify: `internal/cluster/manager_test.go` (add Drain tests)

This task adds the `internal/cluster.Manager.Drain()` accessor per SPEC §6.7 — walks `m.clusters` map and calls `c.closePool()` on each cluster. Best-effort; no error return; idempotent. Per planner-time decision 6, `closePool()` lands as a stub iterating whatever pooled resources `Cluster` carries today (the implementer at this task inspects `internal/cluster/cluster.go` to confirm exact pool fields; if no exported pool field exists at this point in the codebase's evolution, `closePool()` is a no-op-with-log stub today and the operator-affordances expansion is recorded as a forward note via SPEC §2.1 deferral cross-ref). ADR-0096 (the consolidated in-flight-completion ADR) anchors at this task because this is the first cluster-side touch; Tasks 9 + 10 (HCM / TCP-proxy) cite ADR-0096 in their commit messages without re-anchoring.

**Precondition:** Task 3 done; `cluster.Manager.Drain()` does not exist; `Cluster.closePool()` does not exist.
**Artifact:** new method on `Cluster`; new method on `Manager`; tests; ADR-0096 in DECISIONS.md.
**Acceptance:** `go build ./internal/cluster/...` clean; `go test ./internal/cluster/...` passes; `grep -nE '^func \(m \*Manager\) Drain\(\)' internal/cluster/manager.go` returns 1 match; ADR-0096 in DECISIONS.md.

- [ ] **Step 1: Write failing tests in `internal/cluster/manager_test.go`**

```go
func TestManager_Drain_ClosesPools(t *testing.T) {
	bs := mustParseBootstrap(t /* fixture YAML with one cluster + one endpoint */)
	m, err := NewManager(bs.Proto, stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Drain is a best-effort pool close; we assert no panic + idempotency.
	m.Drain()
	// Subsequent Drain calls must be safe (no double-close panics).
	m.Drain()
}

func TestManager_Drain_Idempotent(t *testing.T) {
	bs := mustParseBootstrap(t /* same fixture */)
	m, _ := NewManager(bs.Proto, stats.NewRegistry())
	for i := 0; i < 10; i++ {
		m.Drain()
	}
	// No assertions beyond "did not panic"; closePool stubs may grow more
	// invariants in future hot-restart family work (per SPEC §2.1 deferral).
}

func TestManager_Drain_EmptyClusterList(t *testing.T) {
	m := &Manager{clusters: map[string]*Cluster{}}
	m.Drain()  // must not panic on empty map
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test -run TestManager_Drain ./internal/cluster/... 2>&1 | head -10
```

Expected: build error (`m.Drain undefined`).

- [ ] **Step 3: Add `closePool()` to `internal/cluster/cluster.go`**

After the existing `Cluster` methods, add (per planner-time decision 6):

```go
// closePool closes the per-cluster connection-pool resources at drain time.
// Best-effort; no error return; idempotent.
//
// 08.2 lands this as a forward-extensible hook. The exact set of pooled
// resources to close evolves with each upstream-protocol family:
//   - HTTP/1.1 keep-alive idle conns (no exported pool field today; phase 02
//     dials per-request without keep-alive pooling — the future operator-
//     affordances phase may add a pool, in which case closePool grows to
//     drain it).
//   - HTTP/2 ClientConn instances from phase 05.2 (no exported close hook
//     today; the future operator-affordances phase may add one).
//   - TLS upstream connections from phase 03 (covered by the H1.1/H2 pool
//     close above when those land; tls.Conn instances are inside).
//
// Today, closePool is a stub with a debug log. The cm.Drain() call from
// cmd/envoy-go/main.go (post-rendezvous, before the deferred-stop chain
// runs) provides the architectural call-site for future expansion per
// SPEC §2.1 deferral note. Per planner-time decision 6.
func (c *Cluster) closePool() {
	// Future: iterate c.h1Pool / c.h2ClientConns / c.tlsUpstreamConns when
	// those fields exist. For now, a best-effort log indicating the cluster
	// is being drained at the cluster-pool layer.
	// log.Printf("cluster %q: closePool (drain hook)", c.name)
}
```

- [ ] **Step 4: Add `Drain()` method to `internal/cluster/manager.go`**

After the existing `Clusters()` method (around line 161), add:

```go
// Drain closes upstream connection pools across all configured clusters.
// Best-effort — returns no error. Walks m.clusters map and calls
// c.closePool() on each cluster.
//
// Called from cmd/envoy-go/main.go AFTER <-drainMgr.Done() fires (i.e.,
// after no in-flight downstream requests remain — therefore no in-flight
// upstream requests can remain). The pool close releases socket file
// descriptors for cleanest shutdown but is not required for correctness
// (Go's runtime will close TCP sockets on process exit regardless).
//
// Idempotent; safe under concurrent invocation (closePool is internally
// idempotent per planner-time decision 6).
//
// Phase 08.2 (Task 4) introduces this accessor; ADR-0096 records the
// design (the consolidated in-flight-completion ADR; Tasks 9 + 10 cite
// ADR-0096 without re-anchoring).
func (m *Manager) Drain() {
	for _, c := range m.clusters {
		c.closePool()
	}
}
```

- [ ] **Step 5: Append ADR-0096 to `docs/envoy-go/DECISIONS.md`**

Per the ADR-0001 template:

- Status: Accepted.
- Doctrine: D-3.3 + D-3.5.
- Lands-in-task: Task 4 (anchor); Tasks 9, 10 realize the HCM/TCP-proxy components.
- Context: SPEC §6.6 + §6.7 + §11.3 + BRAINSTORM Decisions 7 + 8 consolidated. The drain manager's inflight counter is the rendezvous primitive; HCM Inc/Dec at request boundaries balances per-request inflight; TCP-proxy Inc/Dec at conn boundaries balances per-connection inflight; cluster.Manager.Drain runs AFTER `<-drainMgr.Done()` to close upstream pools (no in-flight upstream requests remain at that point).
- Decision: Three-part discipline:
  1. HCM (Tasks 9): inflight Inc at request-begin (per stream, NOT per connection — multiple keep-alive requests on one H1.1 conn each Inc/Dec); Dec at request-end (post-access-log per phase 06.2). A `markedInflight bool` sentinel field on the per-request struct ensures pair-balance under sendLocalReply per ADR-0075.
  2. TCP-proxy (Task 10): inflight Inc at conn-begin (`Handle` top, after `ctx.Err()` check, before `Dial`); Dec via `defer` immediately after Inc (per-connection granularity).
  3. cluster.Manager.Drain (this task): best-effort upstream-pool close after the rendezvous fires.
- Consequences: (a) Per §11.3 empirical evidence, envoy-go does NOT mark in-flight H1.1 keep-alive responses with `Connection: close` — Envoy parity; subsequent requests on the same conn during DRAINING extend the drain window via further Inc calls (deliberate MVP simplification; per-conn drainable-close-at-next-idle-window deferred per §2.1). (b) The closePool stub today is a forward-extensible hook; future hot-restart/operator-affordances family expansion grows the per-cluster pool-close logic without changing the Drain() API. (c) Race-detector-clean under TestAdminConcurrentScrapeRace (Task 12) extended with a Drain-mid-test goroutine.

- [ ] **Step 6: Run tests; confirm they pass**

```bash
go test -count=1 ./internal/cluster/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/cluster/...
```

Expected: all PASS, vet clean, lint clean.

- [ ] **Step 7: Commit**

```bash
git add internal/cluster/ docs/envoy-go/DECISIONS.md
git commit -m "phase 08.2: internal/cluster.Manager.Drain() + Cluster.closePool() stub [ADR-0096]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (cluster.Manager.Drain() deliverable), §6.7 (signature + semantics), §14.3 (test list), §12 #6 (planner-time-resolved closePool stub shape); ADR-0096 anchor.*

---

## Task 5: `internal/listener.Manager.Drain()` + Accept-loop fast-path + constructor-widen [ADR-0094]

**Files:**
- Modify: `internal/listener/manager.go` (add `dm` field on `Manager` + on `listenerRuntime`; widen `NewManagerWithBaseDirAndAllowH2C`; add `Drain()` method; Accept-loop fast-path; widen `filterRegistry` HCM/TCP-proxy closures to thread `dm`; N-1 doc-comment fix on `Listeners()`)
- Modify: `internal/listener/manager_test.go` (update existing call sites; add Drain tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0094)

This task lands the listener-side drain plumbing per SPEC §6.6 + ADR-0094. The Accept-loop in `(rt *listenerRuntime) acceptLoop` (line ~783) gains a TWO-line fast-path AT THE TOP of each iteration AFTER `ln.Accept()` returns. The accept-then-FIN behavior matches §11.5 empirical-pin verbatim. The N-1 carry-forward (per SPEC §10.2: `Listeners()` doc-comment ordering) lands as part of this task's listener-manager touch. The `filterRegistry` map's HCM and TCP-proxy constructor closures (lines 66 and 73) widen to thread `dm` through to the per-typeURL filter constructors; this is intentionally landed here (not at Task 9/10) so that listener_test.go's existing call sites compile without intermediate breakage.

The HCM and TCP-proxy filter package constructor signatures are NOT YET WIDENED at the end of this task — the listener manager threads `dm` to the filter-package constructors via the `filterRegistry` closures, but each filter constructor accepts the `dm` arg as a no-op for now (the signature extension lands at Tasks 9/10 alongside the per-package field + Inc/Dec hooks). Concretely: at this task, the listener `filterRegistry` HCM constructor closure changes from `func(tc, cm, lc, registry, accessLogSinks, httpRegistry)` to `func(tc, cm, lc, registry, accessLogSinks, httpRegistry, dm)`, and the inner `hcm.NewFilterWithCtxAndSinksAndRegistry` call passes `dm` IF AND ONLY IF Tasks 9/10 have widened that signature first. The implementer settles the precise inter-task ordering at impl-task time — the cleanest sequence is **Task 5 widens the listener-side closures with a placeholder `_ = dm` discard; Task 9 widens the HCM constructor signature (then the listener's HCM closure threads dm through); Task 10 does the same for TCP-proxy.** This keeps each commit independently buildable.

**Precondition:** Tasks 2 + 3 + 4 done; `listener.Manager.Drain()` does not exist; constructor is at the existing 8-param signature.
**Artifact:** widened constructor; new `Drain()` method; Accept-loop fast-path; N-1 doc-comment fix; ADR-0094.
**Acceptance:** `go build ./internal/listener/...` clean; `go test ./internal/listener/...` passes (incl. new Drain tests); `grep -nE '^func \(m \*Manager\) Drain\(\)' internal/listener/manager.go` returns 1 match; ADR-0094 in DECISIONS.md.

- [ ] **Step 1: Update existing `NewManagerWithBaseDirAndAllowH2C` call sites in `internal/listener/manager_test.go`**

Each existing call site adds `nil` (or a test `*drain.Manager`) as the new arg. Add new tests:

```go
func TestManager_Drain(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	m, err := NewManagerWithBaseDirAndAllowH2C(/* fixture bootstrap */, cm, "", false, stats.NewRegistry(), nil, httpReg, lfReg, dm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if dm.IsDraining() {
		t.Errorf("IsDraining() pre-Drain: got true, want false")
	}
	m.Drain()
	if !dm.IsDraining() {
		t.Errorf("IsDraining() post-Drain: got false, want true")
	}
}

func TestManager_DrainIdempotent(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	m, _ := NewManagerWithBaseDirAndAllowH2C(/* fixture */, cm, "", false, stats.NewRegistry(), nil, httpReg, lfReg, dm)
	m.Drain()
	m.Drain()
	m.Drain()
	if !dm.IsDraining() {
		t.Errorf("IsDraining() post-multi-Drain: got false, want true")
	}
}

func TestManager_AcceptDuringDrainClosesConn(t *testing.T) {
	// Boot a listener; trigger Drain; dial the listener; assert the conn is
	// closed without filter-chain dispatch (i.e., empty read).
	dm := drain.New(1 * time.Hour)
	m, _ := NewManagerWithBaseDirAndAllowH2C(/* fixture with one TCP-proxy listener */, cm, "", false, stats.NewRegistry(), nil, httpReg, lfReg, dm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()
	addr := m.Listeners()[0].Addr
	m.Drain()  // fast-path activates
	// Dial; expect handshake + immediate FIN (empty body / EOF on first read).
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != io.EOF || n != 0 {
		t.Errorf("expected EOF (accept-then-FIN); got n=%d err=%v", n, err)
	}
}

func TestManager_StopAfterDrain(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	m, _ := NewManagerWithBaseDirAndAllowH2C(/* fixture */, cm, "", false, stats.NewRegistry(), nil, httpReg, lfReg, dm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = m.Start(ctx)
	m.Drain()  // sets dm.IsDraining=true
	m.Stop()   // closes listening sockets (post-drain teardown)
	m.Stop()   // idempotent
}
```

Add `"github.com/esalaine/envoy-go/internal/drain"` + `"io"` + `"net"` + `"context"` + `"time"` to imports as needed.

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/listener/... 2>&1 | head -20
```

Expected: build errors (`too many arguments`, `m.Drain undefined`).

- [ ] **Step 3: Edit `internal/listener/manager.go`**

(a) Add `"github.com/esalaine/envoy-go/internal/drain"` to imports.

(b) Modify `Manager` struct to add `dm *drain.Manager` field (new field).

(c) Modify `listenerRuntime` struct to add `dm *drain.Manager` field (so the Accept-loop fast-path is field-local rather than chasing back through `*Manager`).

(d) Widen `NewManagerWithBaseDirAndAllowH2C` signature to take `dm *drain.Manager` as the LAST parameter:

```go
func NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry, lfRegistry *listenerfilter.ListenerFilterRegistry, dm *drain.Manager) (*Manager, error) {
```

Set `m.dm = dm` in the manager construction; propagate `dm` to each `listenerRuntime` at runtime build time.

(e) Update the `filterRegistry` map's HCM and TCP-proxy constructor closures to take `dm *drain.Manager` and thread it through. Per the inter-task ordering note above, this is initially a `_ = dm` discard at this task; Tasks 9/10 widen the inner `hcm.NewFilterWithCtxAndSinksAndRegistry` and `tcpproxy.NewFilter` signatures and the closures plumb dm through. To keep this commit buildable, the closures may pass `dm` to the filter constructors only after Tasks 9/10 land — at this task, define the closure signature with `dm` and `_ = dm` it.

(f) Add the new `Drain()` method (place after `Listeners()` around line 938):

```go
// Drain transitions the manager to drain mode by calling m.dm.Drain()
// (delegates to the central drain.Manager). The per-runtime Accept loops
// already check m.dm.IsDraining() at the top of each iteration; once
// Drain has been called, the next Accept return is the first conn that
// gets the accept-then-FIN treatment per SPEC §11.5.
//
// Idempotent — calling Drain multiple times is safe (delegates to the
// sync.Once-guarded drain.Manager.Drain).
//
// This method does NOT close the listening sockets. Existing in-flight
// downstream connections continue running their HCM filter chains to
// completion. The post-drain teardown is Stop(), invoked from the
// deferred-stop chain in cmd/envoy-go/main.go AFTER <-drainMgr.Done().
//
// Phase 08.2 (Task 5) introduces this accessor; ADR-0094 records the design.
func (m *Manager) Drain() {
	if m.dm != nil {
		m.dm.Drain()
	}
}
```

(g) Modify the `(rt *listenerRuntime) acceptLoop` body (line ~783) to add the fast-path AT THE TOP of each iteration AFTER `ln.Accept()` returns:

```go
func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		raw, err := ln.Accept()
		if err != nil {
			// (existing error-handling preserved)
			...
		}
		// 08.2 (Task 5) drain fast-path per ADR-0094 + SPEC §11.5: on drain,
		// the accepted conn is immediately closed without filter-chain
		// dispatch (TCP handshake completes; the client observes accept-then-
		// FIN — "Empty reply from server" per curl error 52). The 06.1 +2-LoC
		// accept-site Inc lines are NOT executed for the drained-conn case
		// (the conn never enters serveConnection).
		if rt.dm != nil && rt.dm.IsDraining() {
			_ = raw.Close()
			continue
		}
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		go rt.serveConnection(ctx, raw)
	}
}
```

(h) Modify the `Listeners()` doc-comment (line 926) to add the N-1 carry-forward:

```go
// Listeners returns one Info per bound listener. Empty before Start or after a
// Start that errored out (the unwind clears every socket).
//
// Order is bootstrap-declaration order; callers needing alphabetical ordering
// must sort. (Per 08.1 REVIEW N-1 carry-forward, landed inline in 08.2 Task 5
// per SPEC §10.2.)
func (m *Manager) Listeners() []Info {
```

- [ ] **Step 4: Append ADR-0094 to `docs/envoy-go/DECISIONS.md`**

Per the ADR-0001 template:

- Status: Accepted.
- Doctrine: D-3.3 + D-3.5.
- Lands-in-task: Task 5.
- Context: SPEC §6.6 + §11.5 + BRAINSTORM Decision 5. The empirical evidence at §11.5 (curl observes "Empty reply from server"; nc reads 0 bytes; TCP 3-way handshake completes per `Connected to 127.0.0.1`) settles the close-mechanism choice between (a) accept-then-FIN (close the accepted conn after handshake) and (b) listener-socket-close (close the listening socket so the kernel produces RST-on-no-listener for new connections). Envoy v1.37.2 uses (a); envoy-go matches.
- Decision: `internal/listener.Manager.Drain()` is a public method that delegates to the central `drain.Manager.Drain()`. The actual stop-accepting mechanism is a per-runtime Accept-loop fast-path: at the top of each Accept iteration (after `Accept()` returns), the loop body checks `rt.dm.IsDraining()`; if true, the new conn is immediately `conn.Close()`'d and the loop continues without filter-chain dispatch. This produces the accept-then-FIN behavior. The existing `Listener.Manager.Stop()` method stays unchanged as the post-drain teardown step (closes the listening sockets); Stop is invoked from the deferred-stop chain in `cmd/envoy-go/main.go` AFTER `<-drainMgr.Done()`.
- Consequences: (a) New connections during drain receive accept-then-FIN per §11.5; the 06.1 +2-LoC accept-site Inc lines (downstreamCxTotal / downstreamCxActive) are NOT executed for the drained-conn case (the conn never enters serveConnection). (b) In-flight serveConnection goroutines (running the HCM filter chain) continue to completion — they are not interrupted by Drain. (c) The fast-path is field-local (rt.dm) rather than chasing back through *Manager — minimizes the hot-path indirection.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go test -count=1 ./internal/listener/... 2>&1 | tail -10
go test -race -count=1 ./internal/listener/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/listener/...
```

Expected: all PASS, race clean, vet clean, lint clean. Note: `go build ./cmd/envoy-go/...` STILL fails (Task 11 fixes); `go build ./internal/filter/hcm/...` is unaffected because the hcm constructor signature is not yet widened (Task 9 widens).

- [ ] **Step 6: Commit**

```bash
git add internal/listener/ docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 08.2: internal/listener.Manager.Drain() + Accept-loop fast-path [ADR-0094]

Lands the listener-side drain plumbing per SPEC §6.6: Drain() delegates to
the central drain.Manager; the per-runtime Accept loop checks dm.IsDraining()
AT THE TOP of each iteration after ln.Accept() returns and immediately
closes drained conns without filter-chain dispatch (accept-then-FIN matching
§11.5 empirical-pin verbatim). Stop() preserved unchanged as post-drain
teardown.

Carry-forward: N-1 (08.1 REVIEW) Listeners() doc-comment ordering note
landed inline per SPEC §10.2.

NewManagerWithBaseDirAndAllowH2C signature widened to take *drain.Manager
as the 9th param (LBP-1 fifth application carry-through). The filterRegistry
HCM/TCP-proxy closures take dm; the inner filter constructors will plumb
dm through at Tasks 9/10. cmd/envoy-go remains broken (Task 11 fixes).

ADR-0094 records the design.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (manager.go modifications), §6.6 (Drain method), §11.5 (close-mechanism evidence), §10.2 (N-1 carry-forward), §10.1(b) (LBP-1 carry-forward); ADR-0094.*

---

## Task 6: (consolidated into Task 4) — *placeholder slot kept for numerical alignment*

This task slot was originally projected by STATE.md `next-skill-scope` as a separate `internal/cluster.Manager.Drain()` task. The PLAN consolidates it INTO Task 4 alongside the per-Cluster `closePool()` helper (the cluster-side touch is so small that splitting it is unjustified).

The numerical position is preserved here so that subsequent Task 7..13 numbering aligns with the SPEC §15 acceptance bullets and STATE.md's projected task layout. **This task is a no-op; skip it during execution.** The implementer at Task 4 lands the entire cluster-side surface; no work is left for this slot.

*Anchored: PLAN-time consolidation per SPEC §8 ADR consolidation guidance ("the planner settles at PLAN time").*

---

## Task 7: `internal/admin/drain.go` POST handler + method-discrimination [ADR-0093]

**Files:**
- Create: `internal/admin/drain.go`
- Create: `internal/admin/drain_test.go`
- Modify: `internal/admin/admin.go` (add `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` to `Start()`)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0093; in-place amend ADR-0090)

This task lands the new `POST /drain_listeners` admin endpoint per SPEC §6.3 + §11.1 + §11.4. ENVOY-FAITHFUL method discrimination: GET/PUT/DELETE/HEAD return `405 Method Not Allowed` with body `Method <X> not allowed, POST required.\n` per §11.4 empirical pin verbatim. POST returns 200 with body `OK\n` per §11.1. Idempotent (sync.Once-guarded inside the drain.Manager). Handler is fire-and-forget — does NOT block on `<-s.dm.Done()`. The `?graceful=true` query-param is silently accepted (per ADR-0041's silent-ignore precedent). ADR-0093 partially amends ADR-0090's no-method-discrimination posture (qualifies it to read-only endpoints only — mutating endpoints DO get method discrimination; `/drain_listeners` is the FIRST envoy-go endpoint with 405 enforcement).

**Precondition:** Tasks 2 + 3 done; `internal/admin/drain.go` does not exist; the mux registration is not yet present.
**Artifact:** new handler file + tests; mux registration added; ADR-0093 in DECISIONS.md; ADR-0090 in-place-amended.
**Acceptance:** `go build ./internal/admin/...` clean; `go test ./internal/admin/...` passes (incl. all 10 new tests); ADR-0093 in DECISIONS.md; ADR-0090's Consequences extended in-place per the ADR-0089 consequence (b) pattern.

- [ ] **Step 1: Write failing tests in `internal/admin/drain_test.go`**

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/stats"
)

func TestHandleDrainListeners_PostFires(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	if got := w.Body.String(); got != "OK\n" {
		t.Errorf("body: got %q, want %q", got, "OK\n")
	}
	if dm.State() != drain.StateDraining {
		t.Errorf("dm.State post-POST: got %v, want StateDraining", dm.State())
	}
}

func TestHandleDrainListeners_BodyExact(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	body := w.Body.Bytes()
	if len(body) != 3 || body[0] != 'O' || body[1] != 'K' || body[2] != '\n' {
		t.Errorf("body byte-exact: got %q (len=%d), want %q (len=3)", body, len(body), "OK\n")
	}
}

func TestHandleDrainListeners_Idempotent(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/drain_listeners", nil)
		s.handleDrainListeners(w, r)
		if got := w.Code; got != 200 {
			t.Errorf("iteration %d status: got %d, want 200", i, got)
		}
		if got := w.Body.String(); got != "OK\n" {
			t.Errorf("iteration %d body: got %q, want %q", i, got, "OK\n")
		}
	}
}

func TestHandleDrainListeners_GraceQueryParamSilentlyIgnored(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners?graceful=true", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	if got := w.Body.String(); got != "OK\n" {
		t.Errorf("body: got %q, want %q", got, "OK\n")
	}
}

func TestHandleDrainListeners_NilDrainManager(t *testing.T) {
	// Per planner-time decision 10 (defensive 500 vs no-op 200): defensive 500
	// with body "drain manager not configured\n" — the operator gets a clear
	// signal that the drain machinery is not wired.
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 500 {
		t.Errorf("nil-dm status: got %d, want 500", got)
	}
	if got := w.Body.String(); got != "drain manager not configured\n" {
		t.Errorf("nil-dm body: got %q, want %q", got, "drain manager not configured\n")
	}
}

func TestHandleDrainListeners_GetReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("GET status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method GET not allowed, POST required.\n" {
		t.Errorf("GET body: got %q, want %q", got, "Method GET not allowed, POST required.\n")
	}
	if dm.State() != drain.StateLive {
		t.Errorf("dm.State after GET 405: got %v, want StateLive (no side effect)", dm.State())
	}
}

func TestHandleDrainListeners_PutReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("PUT status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method PUT not allowed, POST required.\n" {
		t.Errorf("PUT body: got %q", got)
	}
}

func TestHandleDrainListeners_DeleteReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("DELETE status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method DELETE not allowed, POST required.\n" {
		t.Errorf("DELETE body: got %q", got)
	}
}

func TestHandleDrainListeners_HeadReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("HEAD", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("HEAD status: got %d, want 405", got)
	}
	// HEAD semantics — we still emit the body to httptest.Recorder; net/http's
	// Server elides the body on the wire for HEAD per RFC 9110, but the
	// handler itself writes the same bytes; the headers are what matter.
}

func TestHandleDrainListeners_HeaderSet(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	h := w.Header()
	cases := []struct{ key, want string }{
		{"Content-Type", "text/plain; charset=UTF-8"},
		{"Cache-Control", "no-cache, max-age=0"},
		{"X-Content-Type-Options", "nosniff"},
		{"Server", "envoy"},
	}
	for _, c := range cases {
		if got := h.Get(c.key); got != c.want {
			t.Errorf("header %q: got %q, want %q", c.key, got, c.want)
		}
	}
	// Body should be present in the recorder
	if !strings.HasPrefix(w.Body.String(), "OK") {
		t.Errorf("body prefix: got %q, want starts with OK", w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test -run TestHandleDrainListeners ./internal/admin/... 2>&1 | head -10
```

Expected: build error (`s.handleDrainListeners undefined`).

- [ ] **Step 3: Write `internal/admin/drain.go`**

```go
package admin

import (
	"net/http"
	"strconv"
)

// handleDrainListeners implements the POST /drain_listeners contract per
// SPEC §6.3 + §11.1 (POST) + §11.4 (method discrimination):
//
//   - POST: triggers s.dm.Drain() (sync.Once-guarded internally; idempotent
//     across multiple POSTs); emits 200 OK with body "OK\n" + the standard
//     six-header set per §11.6 via writeAdminHeaders. Fire-and-forget — does
//     NOT block on <-s.dm.Done().
//   - GET / PUT / DELETE / HEAD / others: emits 405 Method Not Allowed with
//     body "Method <METHOD> not allowed, POST required.\n" per §11.4
//     empirical pin verbatim. The 405 is a hard rejection — DOES NOT trigger
//     drain.
//
// The ?graceful=true query-param is silently accepted per ADR-0041's silent-
// ignore precedent (envoy-go's drain is always graceful by construction).
//
// Per SPEC §6.3, the endpoint does NOT trigger process exit — the operator-
// driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT (per
// §5.3 lifecycle).
//
// Per planner-time-resolved nil-dm policy: a nil s.dm yields 500 Internal
// Server Error (defensive — the operator gets a clear signal that the drain
// machinery is not wired). Production builds always thread a non-nil dm.
//
// ADR-0093 records the design (partially amends ADR-0090's no-method-
// discrimination posture).
func (s *Server) handleDrainListeners(w http.ResponseWriter, r *http.Request) {
	writeAdminHeaders(w, "text/plain; charset=UTF-8")

	// Method discrimination (per §11.4). Non-POST returns 405 with the
	// templated body. The 405 is a hard rejection — no drain side effect.
	if r.Method != http.MethodPost {
		body := []byte("Method " + r.Method + " not allowed, POST required.\n")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write(body)
		return
	}

	// Defensive 500 on nil dm (per PLAN planner-time-resolved nil-dm policy).
	if s.dm == nil {
		body := []byte("drain manager not configured\n")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(body)
		return
	}

	// POST path: trigger drain (sync.Once-guarded inside drain.Manager;
	// idempotent across multiple POSTs); emit 200 OK with body "OK\n" per
	// §11.1 empirical pin verbatim.
	s.dm.Drain()
	body := []byte("OK\n")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

- [ ] **Step 4: Register the new handler in `internal/admin/admin.go::Start()`**

Add one line after the existing six 08.1 mux registrations:

```go
mux.HandleFunc("/drain_listeners", s.handleDrainListeners)
```

Update the Start() doc-comment to enumerate seven routes post-08.2.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go test -run TestHandleDrainListeners ./internal/admin/... -v 2>&1 | tail -30
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: 10 PASS, vet clean, lint clean.

- [ ] **Step 6: Append ADR-0093 to `docs/envoy-go/DECISIONS.md`**

Per the ADR-0001 template:

- Status: Accepted.
- Doctrine: D-3.3 + D-3.5 + D-3.7.
- Lands-in-task: Task 7.
- Context: SPEC §6.3 + §11.1 + §11.4 + BRAINSTORM Decision 3. Two empirical pins settle the contract: (1) §11.1 — POST returns 200 OK with body `OK\n` (3 bytes; capital `OK` followed by single newline); (2) §11.4 — non-POST returns 405 Method Not Allowed with body `Method <METHOD> not allowed, POST required.\n` (38 + len(METHOD) bytes). Both with the standard six-header set per §11.6. The §11.4 finding is a **SURPRISE** that contradicts BRAINSTORM Decision 3's hypothesis (which expected Envoy parity = no method check, mirroring the 08.1 read-only-endpoint posture per ADR-0090).
- Decision: Method discrimination check FIRST (return 405 with templated body for non-POST). On POST, call `s.dm.Drain()` synchronously (sync.Once-guarded inside the Manager; subsequent POSTs no-op the transition but return identical 200/`OK\n`). Fire-and-forget — does NOT block on `<-s.dm.Done()`. Does NOT trigger process exit. The `?graceful=true` query-param is silently accepted per ADR-0041.
- Consequences: (a) `/drain_listeners` is the FIRST admin endpoint in envoy-go with method discrimination; the no-method-discrimination posture from ADR-0090 is qualified to **read-only endpoints only**. (b) ADR-0090 is partially amended in-place per the ADR-0089 consequence (b) pattern (the no-ACL posture is preserved verbatim; only the no-method-discrimination posture is qualified). (c) The /healthcheck/fail endpoint stays in ADR-0089's deferral list — envoy-go MVP unifies the listener-drain (which §11.2 evidence ties to /drain_listeners) and load-balancer-disposition flip (which §11.2 evidence ties to /healthcheck/fail) under a single drain.Manager state machine; the differential gate's per-proxy trigger script normalizes (§7.2). (d) Idempotent: subsequent POSTs return identical 200/`OK\n` without re-firing Drain.

- [ ] **Step 7: Amend ADR-0090 in-place** per the ADR-0089 consequence (b) in-place-edit pattern

Locate ADR-0090's Consequences section in `docs/envoy-go/DECISIONS.md` and append a forward-pointer paragraph: "**Phase 08.2 amendment (per ADR-0093):** the no-method-discrimination posture is qualified to **read-only endpoints only**. The mutating `POST /drain_listeners` endpoint (08.2's first mutating endpoint) DOES enforce method discrimination per SPEC §11.4 empirical pin (Envoy parity). Non-POST methods (GET, PUT, DELETE, HEAD) return 405 Method Not Allowed with the templated body. The no-ACL posture is preserved verbatim — operator firewall remains the security boundary."

- [ ] **Step 8: Commit**

```bash
git add internal/admin/drain.go internal/admin/drain_test.go internal/admin/admin.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 08.2: internal/admin/drain.go POST /drain_listeners + method discrimination [ADR-0093]

Lands the new POST /drain_listeners admin endpoint per SPEC §6.3 + §11.1 +
§11.4. POST returns 200 OK + body "OK\n" + standard six-header set; non-POST
returns 405 Method Not Allowed + body "Method <X> not allowed, POST
required.\n" per §11.4 empirical pin verbatim. Idempotent (sync.Once-guarded
inside drain.Manager); fire-and-forget (does NOT block on <-s.dm.Done()).
?graceful=true silently accepted per ADR-0041.

ADR-0093 records the design and partially amends ADR-0090 (no-method-
discrimination posture qualified to read-only endpoints; mutating endpoints
DO get method discrimination). ADR-0090 amended in-place per ADR-0089
consequence (b) pattern.

mux.HandleFunc("/drain_listeners", s.handleDrainListeners) added to
admin.Server.Start() — seventh handler on the same mux.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (drain.go deliverable), §6.3 (handler contract), §11.1 + §11.4 (empirical pins), §10.1(a) (admin-mux extension pattern), §14.2 (test list); ADR-0093 + ADR-0090 amendment.*

---

## Task 8: `/ready` + `/server_info` DRAINING extensions [ADR-0097, ADR-0098]

**Files:**
- Modify: `internal/admin/admin.go` (`handleReady` body — add NEW first DRAINING-branch)
- Modify: `internal/admin/admin_test.go` (add DRAINING-precedence tests; extend race-test)
- Modify: `internal/admin/serverinfo.go` (`deriveState` signature widen + DRAINING-first check)
- Modify: `internal/admin/serverinfo_test.go` (add DRAINING state-enum + precedence tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0097 + ADR-0098; in-place amend ADR-0015 + ADR-0088)

This task lands the DRAINING-state extensions to `/ready` and `/server_info` per SPEC §6.4 + §6.5 + §11.2. Two ADRs land here together because both are DRAINING-extension edits to existing 08.1 endpoints — landing them in the same task mirrors the SPEC §8 grouping. ADR-0097 partially supersedes ADR-0015 (pre-init contract for /ready). ADR-0098 amends ADR-0088 (state-enum coverage) purely additively.

**Precondition:** Tasks 2 + 3 done; `handleReady` is at the 08.1 two-state form (LIVE / PRE_INITIALIZING); `deriveState` is at the 08.1 single-arg form.
**Artifact:** modified handlers + tests; ADR-0097 + ADR-0098 in DECISIONS.md; ADR-0015 + ADR-0088 in-place-amended.
**Acceptance:** `go build ./internal/admin/...` clean; `go test ./internal/admin/...` passes; ADR-0097 + ADR-0098 in DECISIONS.md.

- [ ] **Step 1: Write failing tests in `internal/admin/admin_test.go`**

```go
func TestHandleReady_Draining(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	s.MarkReady()
	dm.Drain()  // transition to DRAINING
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/ready", nil)
	s.handleReady(w, r)
	if got := w.Code; got != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", got)
	}
	if got := w.Body.String(); got != "DRAINING\n" {
		t.Errorf("body: got %q, want %q", got, "DRAINING\n")
	}
}

func TestHandleReady_DrainingPrecedesLive(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	s.MarkReady()  // LIVE state
	dm.Drain()     // DRAINING — should override LIVE
	w := httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest("GET", "/ready", nil))
	if got := w.Body.String(); got != "DRAINING\n" {
		t.Errorf("DRAINING precedence over LIVE: got %q, want %q", got, "DRAINING\n")
	}
}

func TestHandleReady_DrainingPrecedesPreInitializing(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	// MarkReady NOT called — would normally yield PRE_INITIALIZING
	dm.Drain()
	w := httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest("GET", "/ready", nil))
	if got := w.Body.String(); got != "DRAINING\n" {
		t.Errorf("DRAINING precedence over PRE_INITIALIZING: got %q, want %q", got, "DRAINING\n")
	}
}

func TestHandleReady_DrainingHeaders(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	dm.Drain()
	w := httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest("GET", "/ready", nil))
	h := w.Header()
	if got := h.Get("Content-Type"); got != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type: got %q", got)
	}
	if got := h.Get("Server"); got != "envoy" {
		t.Errorf("Server: got %q", got)
	}
	if got := h.Get("Cache-Control"); got != "no-cache, max-age=0" {
		t.Errorf("Cache-Control: got %q", got)
	}
}
```

Add similar tests to `internal/admin/serverinfo_test.go`:

```go
func TestHandleServerInfo_StateDraining(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	s.MarkReady()
	dm.Drain()
	info := buildServerInfo(s)
	if got := info.State; got != adminv3.ServerInfo_DRAINING {
		t.Errorf("state: got %v, want DRAINING", got)
	}
}

func TestHandleServerInfo_StatePrecedence_DrainingOverLive(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	s.MarkReady()
	dm.Drain()
	info := buildServerInfo(s)
	if got := info.State; got != adminv3.ServerInfo_DRAINING {
		t.Errorf("Draining > Live: got %v, want DRAINING", got)
	}
}

func TestHandleServerInfo_StatePrecedence_DrainingOverPreInit(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	dm.Drain()  // MarkReady NOT called
	info := buildServerInfo(s)
	if got := info.State; got != adminv3.ServerInfo_DRAINING {
		t.Errorf("Draining > PreInit: got %v, want DRAINING", got)
	}
}

func TestDeriveState_NilDrainManager(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)
	if got := deriveState(&ready, nil); got != adminv3.ServerInfo_LIVE {
		t.Errorf("nil dm + ready: got %v, want LIVE", got)
	}
	ready.Store(false)
	if got := deriveState(&ready, nil); got != adminv3.ServerInfo_PRE_INITIALIZING {
		t.Errorf("nil dm + not ready: got %v, want PRE_INITIALIZING", got)
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test -run 'TestHandleReady_Draining|TestHandleServerInfo_StateDraining' ./internal/admin/... 2>&1 | head -10
```

Expected: tests fail (current `handleReady` returns LIVE\n; current `deriveState` does not consult `dm`).

- [ ] **Step 3: Edit `internal/admin/admin.go::handleReady`**

Add a NEW first branch BEFORE the existing pre-init check (per SPEC §6.4 precedence: DRAINING > PRE_INITIALIZING > LIVE):

```go
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=UTF-8")
	h.Set("Cache-Control", "no-cache, max-age=0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Server", "envoy")

	// 08.2 (Task 8) DRAINING-first branch per SPEC §6.4 + §11.2 + ADR-0097
	// (partially supersedes ADR-0015 — DRAINING precedence > LIVE >
	// PRE_INITIALIZING). Body verbatim "DRAINING\n" (9 bytes) per §11.2
	// empirical pin.
	if s.dm != nil && s.dm.State() == drain.StateDraining {
		body := []byte("DRAINING\n")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
		return
	}

	if !s.ready.Load() {
		// (existing pre-init branch — unchanged)
		body := []byte("PRE_INITIALIZING\n")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
		return
	}
	// (existing LIVE branch — unchanged)
	s.liveOnce.Do(func() { s.liveGauge.Set(1) })
	body := []byte("LIVE\n")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

- [ ] **Step 4: Edit `internal/admin/serverinfo.go::deriveState`**

Widen the signature; add the NEW first DRAINING check:

```go
// deriveState returns ServerInfo_DRAINING when dm is non-nil and Draining
// (NEW first check per ADR-0098 amending ADR-0088 purely additively),
// ServerInfo_LIVE when the ready atomic is set, else
// ServerInfo_PRE_INITIALIZING per planner-time decision 4 + SPEC §11.7.
// INITIALIZING remains unreachable in MVP. DRAINING precedence > LIVE >
// PRE_INITIALIZING.
func deriveState(ready *atomic.Bool, dm *drain.Manager) adminv3.ServerInfo_State {
	if dm != nil && dm.State() == drain.StateDraining {
		return adminv3.ServerInfo_DRAINING
	}
	if ready.Load() {
		return adminv3.ServerInfo_LIVE
	}
	return adminv3.ServerInfo_PRE_INITIALIZING
}
```

Update the `buildServerInfo` call site from `deriveState(&s.ready)` to `deriveState(&s.ready, s.dm)`.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go test ./internal/admin/... -v 2>&1 | tail -30
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: all PASS, vet clean, lint clean.

- [ ] **Step 6: Append ADR-0097 + ADR-0098 to `docs/envoy-go/DECISIONS.md`**

ADR-0097: Status Accepted; Doctrine D-3.3 + D-3.5; Lands-in-task Task 8; Context SPEC §6.4 + §11.2 + §5.4 + BRAINSTORM Decision 9 — the §11.2 empirical pin verbatim settles the body shape (`DRAINING\n` 9 bytes; status 503; standard six-header set); the **SURPRISE** finding is that upstream Envoy v1.37.2 ties /ready DRAINING to /healthcheck/fail (NOT /drain_listeners alone) — envoy-go MVP unifies these triggers under the single drain.Manager state machine; Decision: NEW first branch in handleReady checking `s.dm != nil && s.dm.State() == StateDraining`; precedence DRAINING > PRE_INITIALIZING > LIVE; partially supersedes ADR-0015 (the pre-init contract — LIVE/PRE_INITIALIZING two-state coverage extends to LIVE/PRE_INITIALIZING/DRAINING three-state coverage; ADR-0015's verbatim pre-init body and status are preserved); Consequences: (a) /ready returns DRAINING\n once Drain() fires regardless of MarkReady state; (b) the differential fixture's per-proxy trigger script normalizes for upstream Envoy's separate /healthcheck/fail trigger per §7.2; (c) ADR-0015 in-place-amended with forward-pointer note.

ADR-0098: Status Accepted; Doctrine D-3.3 + D-3.5; Lands-in-task Task 8; Context SPEC §6.5 + §11.2 + BRAINSTORM Decision 10 + ADR-0088 consequence (c) verbatim ("the amendment is purely additive; no other field changes; the ADR-0088 amendment will record the addition without superseding this ADR"); Decision: `deriveState` signature widens to take `*drain.Manager`; NEW first check returns ServerInfo_DRAINING. The DRAINING precedence matches ADR-0097's /ready precedence; Consequences: (a) `state` field renders `"DRAINING"` (proto enum NAME) when Drain() has fired; (b) ADR-0088 amended in-place per ADR-0089 consequence (b) pattern (the amendment record adds DRAINING to the enum-coverage table and refers to ADR-0098 for the timing semantics; INITIALIZING remains unreachable per ADR-0088 + 08.1 SPEC §11.7).

- [ ] **Step 7: Amend ADR-0015 + ADR-0088 in-place**

ADR-0015 Consequences section: append "**Phase 08.2 amendment (per ADR-0097):** ADR-0015 is **partially superseded** by ADR-0097 — the LIVE/PRE_INITIALIZING two-state coverage extends to LIVE/PRE_INITIALIZING/DRAINING three-state coverage. ADR-0015's verbatim pre-init body (`PRE_INITIALIZING\n`) and pre-init status (503) are preserved; ADR-0097 adds the DRAINING branch and the precedence rule (DRAINING > PRE_INITIALIZING > LIVE)."

ADR-0088 Consequences section: append "**Phase 08.2 amendment (per ADR-0098):** the state-enum coverage extends to LIVE + PRE_INITIALIZING + **DRAINING**. The amendment is purely additive (no other field changes; per ADR-0088's own consequence (c) verbatim). `deriveState` signature widens from `(ready *atomic.Bool)` to `(ready *atomic.Bool, dm *drain.Manager)`; precedence DRAINING > LIVE > PRE_INITIALIZING. INITIALIZING remains unreachable."

- [ ] **Step 8: Commit**

```bash
git add internal/admin/admin.go internal/admin/admin_test.go internal/admin/serverinfo.go internal/admin/serverinfo_test.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 08.2: /ready + /server_info DRAINING extensions [ADR-0097, ADR-0098]

handleReady gains NEW first DRAINING-branch returning 503 + body "DRAINING\n"
(9 bytes) per §11.2 empirical pin. Precedence DRAINING > PRE_INITIALIZING
> LIVE; existing pre-init and LIVE branches preserved verbatim.

deriveState widens signature to take *drain.Manager and gains NEW first
DRAINING-check returning adminv3.ServerInfo_DRAINING. buildServerInfo call
site updated.

ADR-0097 partially supersedes ADR-0015 (pre-init contract — verbatim body
preserved; DRAINING branch added). ADR-0098 amends ADR-0088 purely
additively (state-enum coverage extends to LIVE + PRE_INITIALIZING +
DRAINING). ADR-0015 + ADR-0088 amended in-place per ADR-0089 consequence
(b) pattern.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (admin/serverinfo.go modifications), §6.4 (handleReady contract), §6.5 (deriveState contract), §11.2 (empirical pin), §14.2 (test list); ADR-0097 + ADR-0098 + ADR-0015 + ADR-0088 amendments.*

---

## Task 9: HCM filter Inc/Dec hooks + constructor widening (per ADR-0096)

**Files:**
- Modify: `internal/filter/hcm/config.go` (add `dm *drain.Manager` field to `Filter`; widen `parseFilterWithCtx`)
- Modify: `internal/filter/hcm/filter.go` (widen `NewFilterWithCtxAndSinksAndRegistry` signature)
- Modify: `internal/filter/hcm/connection.go` and/or `internal/filter/hcm/h2dispatch.go` (Inc at request-begin; Dec at request-end with `markedInflight` sentinel)
- Modify: `internal/filter/hcm/filter_test.go` (update existing call sites; add Inc/Dec balance tests)
- Modify: `internal/listener/manager.go` (the HCM `filterRegistry` closure threads `dm` through to the now-widened constructor — replaces the Task 5 `_ = dm` discard)

This task widens the HCM constructor to take `*drain.Manager` and adds Inc/Dec hooks at the request-begin / request-end edges. Per planner-time decision 4, the `markedInflight bool` sentinel field lives on the per-request struct (the implementer settles the exact field placement at impl-task time per the codebase reality — `internal/filter/hcm/connection.go::runConnection` for H1.1 has a per-request bookkeeping struct or per-iteration local that owns the request lifetime; the H2 path in `h2dispatch.go` likewise has a per-stream bookkeeping struct). The Inc-site sets `markedInflight = true`; the Dec-site checks `if markedInflight { dm.Dec(); markedInflight = false }` to ensure pair-balance under sendLocalReply per ADR-0075.

**Implementer-settled at impl-time:** the exact codebase paths for the Inc/Dec hooks. Read `connection.go` and `h2dispatch.go` to identify the natural per-request edges. The H1.1 path: Inc happens AFTER successful request-line + headers parse, BEFORE filter chain runs (so a malformed request-line abort does NOT Inc — that path doesn't enter the filter chain anyway); Dec happens AFTER access-log emission (per phase 06.2 hook) at the request boundary. The H2 path: Inc happens at stream-begin (after headers received); Dec happens at stream-end (after access-log per phase 06.2). The `markedInflight` flag is a single `bool` field on whichever per-request struct already exists.

**Precondition:** Tasks 2 + 3 + 5 done (Task 5's listener `filterRegistry` closure passes `dm` to the HCM constructor — initially as a `_ = dm` discard); HCM constructor is at the existing 6-arg signature.
**Artifact:** widened HCM constructor; Inc/Dec hooks; `markedInflight` field; tests; listener-side closure threads `dm` through (replaces Task 5's discard).
**Acceptance:** `go build ./internal/filter/hcm/...` clean; `go test ./internal/filter/hcm/...` passes (incl. new Inc/Dec balance tests); existing chain integration tests still pass.

- [ ] **Step 1: Write failing tests in `internal/filter/hcm/filter_test.go`**

```go
func TestHCM_DrainInflightBalance(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	cm := mustClusterManager(t)  // existing helper
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), dm)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	// Drive a single H1 request through the filter; assert dm inflight balanced.
	// Use a downstream pipe; write a minimal GET / HTTP/1.1; read response;
	// assert inflight returns to 0 (Drain + Done() fires).
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	// Read response; ignore content
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf)
	// After request completes, Drain should rendezvous immediately.
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire — inflight not balanced")
	}
}

func TestHCM_DrainInflightBalance_SendLocalReply(t *testing.T) {
	// Drive a request that triggers sendLocalReply (e.g., 404 from
	// no-route-match). Per ADR-0075 + planner-time decision 4, the
	// markedInflight sentinel ensures pair-balance even on this path.
	dm := drain.New(10 * time.Millisecond)
	cm := mustClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil /* no routes */), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), dm)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET /no-route-match HTTP/1.1\r\nHost: x\r\n\r\n"))
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf)  // expect 404 sendLocalReply
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire after sendLocalReply path — markedInflight unbalanced")
	}
}

func TestHCM_DrainInflightBalance_NilDrainManager(t *testing.T) {
	// nil dm means no Inc/Dec; the filter must not panic.
	cm := mustClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf)
	// Test passes if no panic.
}
```

Add `"github.com/esalaine/envoy-go/internal/drain"` to imports.

Update existing HCM test call sites (filter_test.go, fuzz_test.go, chain_integration_test.go, etc.) to thread `nil` as the new `dm` arg.

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/hcm/... 2>&1 | head -20
```

Expected: build error (`too many arguments in call to NewFilterWithCtxAndSinksAndRegistry`).

- [ ] **Step 3: Edit `internal/filter/hcm/config.go`**

Add `"github.com/esalaine/envoy-go/internal/drain"` to imports.

Add `dm *drain.Manager` field to `Filter` struct (after the existing `accessLog` field). Widen `parseFilterWithCtx` signature to take `dm *drain.Manager` as the last parameter; set `f.dm = dm` in the constructed Filter.

- [ ] **Step 4: Edit `internal/filter/hcm/filter.go`**

Widen `NewFilterWithCtxAndSinksAndRegistry` signature to take `dm *drain.Manager` as the 7th parameter; pass `dm` through to `parseFilterWithCtx`.

```go
func NewFilterWithCtxAndSinksAndRegistry(
	tc *anypb.Any,
	clusters *cluster.Manager,
	lc ListenerCtx,
	registry *stats.Registry,
	accessLogSinks []accesslog.Sink,
	httpRegistry *filter_http.HTTPRegistry,
	dm *drain.Manager,
) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, accessLogSinks, httpRegistry, dm)
}
```

- [ ] **Step 5: Add Inc/Dec hooks at request-begin / request-end edges**

The implementer reads `internal/filter/hcm/connection.go::runConnection` and `internal/filter/hcm/h2dispatch.go` and identifies the natural per-request edges. The expected pattern (settle exact code at impl-task time):

In `connection.go::runConnection` (H1.1 path), at the per-request loop iteration where the request-line + headers have been successfully parsed and validation has passed, add `markedInflight bool` as a per-iteration local variable; immediately before invoking the filter chain run, add:

```go
var markedInflight bool
if f.dm != nil {
	f.dm.Inc()
	markedInflight = true
}
defer func() {
	if markedInflight {
		f.dm.Dec()
		markedInflight = false
	}
}()
```

The defer ensures Dec fires on every termination path including sendLocalReply (per ADR-0075 discipline). Place the defer AFTER the access-log emit hook so the access-log records the request before the inflight counter is decremented.

For the H2 path (`h2dispatch.go`), apply the same pattern at the per-stream entry — the `markedInflight` flag lives on the per-stream bookkeeping struct (or as a closure-captured local in the per-stream handler). The implementer settles the precise field placement.

- [ ] **Step 6: Update the listener-side `filterRegistry` HCM closure** in `internal/listener/manager.go`

Replace the Task 5 placeholder `_ = dm` discard with actual threading:

```go
hcm.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry, dm *drain.Manager) (filterHandler, error) {
	f, err := hcm.NewFilterWithCtxAndSinksAndRegistry(tc, cm, hcm.ListenerCtx{HasTLS: lc.hasTLS, AllowH2C: lc.allowH2C}, registry, accessLogSinks, httpRegistry, dm)
	if err != nil {
		return nil, err
	}
	return f, nil
},
```

- [ ] **Step 7: Run tests; confirm they pass**

```bash
go test -count=1 ./internal/filter/hcm/... 2>&1 | tail -10
go test -race -count=1 ./internal/filter/hcm/... 2>&1 | tail -5
go test -count=1 ./internal/listener/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/filter/hcm/... ./internal/listener/...
```

Expected: all PASS, race clean, vet clean, lint clean.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/hcm/ internal/listener/manager.go
git commit -m "$(cat <<'EOF'
phase 08.2: HCM Inc/Dec hooks + dm field + constructor widening (per ADR-0096)

Widens NewFilterWithCtxAndSinksAndRegistry to take *drain.Manager (7th
param). Adds dm field to Filter. Inc at request-begin (after request-line +
headers parse, before filter chain run); Dec via defer at request-end
(after access-log emit per phase 06.2 hook). markedInflight bool sentinel
ensures pair-balance under sendLocalReply per ADR-0075 + planner-time
decision 4.

Listener filterRegistry HCM closure now threads dm through (replaces the
Task 5 placeholder _ = dm discard).

Realizes ADR-0096 (consolidated in-flight-completion ADR; anchored at Task
4 cluster-side).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (HCM modifications), §5.6 (in-flight completion swimlane), §11.3 (empirical pin — full body delivery, no Connection: close), §14.2 (test list), §12 #4 (planner-time-resolved markedInflight placement); ADR-0096 realization; ADR-0075 sendLocalReply discipline.*

---

## Task 10: TCP-proxy filter Inc/Dec hooks + constructor widening (per ADR-0096)

**Files:**
- Modify: `internal/filter/tcpproxy/filter.go` (add `dm *drain.Manager` field; widen `NewFilter`; Inc at `Handle` top, Dec via defer)
- Modify: `internal/filter/tcpproxy/filter_test.go` (update existing call sites; add Inc/Dec balance test)
- Modify: `internal/listener/manager.go` (the TCP-proxy `filterRegistry` closure threads `dm` through — replaces the Task 5 `_ = dm` discard)

This task widens the TCP-proxy constructor to take `*drain.Manager` and adds Inc/Dec at conn-begin / conn-end per planner-time decision 5. Per-connection granularity (correct because TCP-proxy has no per-request semantic). The Inc happens at the top of `Handle` (after the existing `ctx.Err()` check, before `Dial`); the matching Dec is `defer`-d immediately after the Inc so all early-return paths (dial failure, context cancellation) decrement correctly.

**Precondition:** Tasks 2 + 3 + 5 + 9 done; TCP-proxy constructor is at the existing 2-arg signature; the listener `filterRegistry` TCP-proxy closure passes `dm` to the constructor (initially a `_ = dm` discard from Task 5).
**Artifact:** widened TCP-proxy constructor; Inc/Dec hooks; tests; listener-side closure threads `dm` through.
**Acceptance:** `go build ./internal/filter/tcpproxy/...` clean; `go test ./internal/filter/tcpproxy/...` passes (incl. new Inc/Dec balance test); existing tests still pass.

- [ ] **Step 1: Write failing tests in `internal/filter/tcpproxy/filter_test.go`**

```go
func TestTCPProxy_DrainInflightBalance(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	cm := mustClusterManager(t)
	tc := mustTcpProxyAny(t, "c_backend")
	f, err := NewFilter(tc, cm, dm)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Handle(ctx, srv)
	// Drive a small payload; close client; wait for Handle to return.
	_, _ = client.Write([]byte("hello\n"))
	_ = client.Close()
	time.Sleep(100 * time.Millisecond)
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire — TCP-proxy inflight not balanced")
	}
}

func TestTCPProxy_DrainInflightBalance_NilDrainManager(t *testing.T) {
	cm := mustClusterManager(t)
	tc := mustTcpProxyAny(t, "c_backend")
	f, err := NewFilter(tc, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("hello\n"))
	_ = client.Close()
	time.Sleep(50 * time.Millisecond)
	// Test passes if no panic.
}
```

Update existing TCP-proxy test call sites to thread `nil` as the new `dm` arg.

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/tcpproxy/... 2>&1 | head -10
```

Expected: build error (`too many arguments in call to NewFilter`).

- [ ] **Step 3: Edit `internal/filter/tcpproxy/filter.go`**

Add `"github.com/esalaine/envoy-go/internal/drain"` to imports.

Add `dm *drain.Manager` field to `Filter` struct.

Widen `NewFilter` signature:

```go
func NewFilter(tc *anypb.Any, cm *cluster.Manager, dm *drain.Manager) (*Filter, error) {
	// (existing parse logic preserved)
	// ...
	return &Filter{cluster: c, statPrefix: msg.GetStatPrefix(), dm: dm}, nil
}
```

Modify `Handle` body to add Inc + defer-Dec at the top (after the `ctx.Err()` check, before `Dial`):

```go
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	if err := ctx.Err(); err != nil {
		return
	}
	// 08.2 (Task 10) drain Inc/Dec per ADR-0096 + planner-time decision 5:
	// per-connection granularity (TCP-proxy has no per-request semantic).
	// Inc at conn-begin (after ctx.Err check, before Dial); Dec via defer
	// for pair-balance on all early-return paths (dial failure, etc.).
	if f.dm != nil {
		f.dm.Inc()
		defer f.dm.Dec()
	}
	upstream, _, err := f.cluster.Dial(ctx)
	// ... (existing logic preserved)
}
```

- [ ] **Step 4: Update the listener-side `filterRegistry` TCP-proxy closure** in `internal/listener/manager.go`

Replace the Task 5 placeholder `_ = dm` discard with actual threading:

```go
tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, _ listenerCtx, _ *stats.Registry, _ []accesslog.Sink, _ *filter_http.HTTPRegistry, dm *drain.Manager) (filterHandler, error) {
	f, err := tcpproxy.NewFilter(tc, cm, dm)
	if err != nil {
		return nil, err
	}
	return f, nil
},
```

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go test -count=1 ./internal/filter/tcpproxy/... 2>&1 | tail -5
go test -race -count=1 ./internal/filter/tcpproxy/... 2>&1 | tail -5
go test -count=1 ./internal/listener/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/filter/tcpproxy/... ./internal/listener/...
```

Expected: all PASS, race clean, vet clean, lint clean.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/tcpproxy/ internal/listener/manager.go
git commit -m "$(cat <<'EOF'
phase 08.2: TCP-proxy Inc/Dec hooks + dm field + constructor widening (per ADR-0096)

Widens NewFilter to take *drain.Manager (3rd param). Adds dm field to
Filter. Inc at Handle top (after ctx.Err check, before Dial); matching
Dec via defer immediately after Inc — pair-balance on all early-return
paths (dial failure, context cancellation, etc.). Per-connection granularity
per planner-time decision 5.

Listener filterRegistry TCP-proxy closure now threads dm through (replaces
the Task 5 placeholder _ = dm discard).

Realizes ADR-0096 (consolidated in-flight-completion ADR; anchored at Task
4 cluster-side).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (TCP-proxy modifications), §6.1 (constructor signatures), §14.2 (test list), §12 #5 (planner-time-resolved Inc-anchor); ADR-0096 realization.*

---

## Task 11: `cmd/envoy-go/main.go` SIGTERM-handler upgrade + boot wiring [ADR-0092, ADR-0095]

**Files:**
- Modify: `cmd/envoy-go/main.go` (drainMgr alloc + threading + SIGTERM-handler block upgrade)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0092 + ADR-0095)

This task wires the drain machinery into `cmd/envoy-go/main.go` and upgrades the SIGTERM-handler block per SPEC §6.8 + BRAINSTORM Decision 2 + 6 + 8. Lands two ADRs: ADR-0092 (SIGTERM-vs-Envoy divergence — envoy-go's SIGTERM/SIGINT triggers drain-then-exit; Envoy v1.37.2 SIGTERM is immediate-exit per §11.7) and ADR-0095 (drain timeout default 30s envoy-go MVP vs 600s Envoy default; operator-knob deferred to a future runtime/hot-restart family phase). Per planner-time decision 7, the `drainMgr` allocation lands AFTER `bootstrap.Load` and BEFORE `cluster.NewManagerWithBaseDir`. Per planner-time decision 9, `cm.Drain()` is an explicit call after the rendezvous (not deferred).

**Precondition:** Tasks 2 + 3 + 4 + 5 + 7 + 8 + 9 + 10 done; `cmd/envoy-go/main.go` is currently broken since Task 3 (admin.New 7-param call site).
**Artifact:** drainMgr alloc + SIGTERM-handler upgrade; ADR-0092 + ADR-0095 in DECISIONS.md.
**Acceptance:** `go build ./cmd/envoy-go/...` clean; `go test ./cmd/envoy-go/...` passes; ADR-0092 + ADR-0095 in DECISIONS.md.

- [ ] **Step 1: Edit `cmd/envoy-go/main.go`**

Add `"github.com/esalaine/envoy-go/internal/drain"` and `"time"` to imports (time may already be imported).

Insert the drainMgr allocation after `bootstrap.Load` (after the existing `bs.ConfigPath = *cfgPath` line) and before `cluster.NewManagerWithBaseDir`:

```go
// Phase 08.2 (Task 11) drain manager allocation per SPEC §5.1 boot-order
// + planner-time decision 7: after bootstrap.Load (no dependencies on the
// bootstrap proto) and before cluster.NewManagerWithBaseDir (the drain
// manager is consumed by all subsequent constructors). The 30s timeout is
// the hardcoded envoy-go MVP default per ADR-0095 (Envoy v1.37.2 default
// is 600s per §11.7 + 08.1 SPEC §11.4 — deliberate divergence to keep test-
// suite cost tractable; operator-knob deferred per ADR-0095).
drainMgr := drain.New(30 * time.Second)
```

Update `listener.NewManagerWithBaseDirAndAllowH2C(...)` call site to thread `drainMgr` as the new last arg:

```go
lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr)
```

Update `admin.New(...)` call site to thread `drainMgr`:

```go
admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm, drainMgr)
```

Replace the `<-ctx.Done()` block at line 170 with the upgraded form per SPEC §6.8:

```go
<-ctx.Done()
log.Print("signal received; initiating graceful drain")
// Phase 08.2 (Task 11) drain rendezvous per SPEC §5.2 + §6.8 + ADR-0092:
// drain-then-exit on SIGTERM/SIGINT (deliberate divergence from Envoy
// v1.37.2's SIGTERM=immediate-exit per §11.7 — operator-ergonomic choice).
// Bound by drainMgr.Timeout() (30s default per ADR-0095).
drainMgr.Drain()
select {
case <-drainMgr.Done():
	log.Print("drain rendezvous: in-flight reached 0")
case <-time.After(drainMgr.Timeout()):
	log.Print("drain rendezvous: timeout fired (best-effort)")
}
// Per planner-time decision 9: explicit cm.Drain() call after rendezvous,
// before deferred-stop chain runs (LIFO: lm.Stop, admSrv.Close, sinks-close).
// Best-effort upstream-pool close per ADR-0096.
cm.Drain()
// Existing deferred-stop chain runs as the function unwinds.
```

- [ ] **Step 2: Append ADR-0092 + ADR-0095 to `docs/envoy-go/DECISIONS.md`**

ADR-0092: Status Accepted; Doctrine D-3.3 + D-3.5; Lands-in-task Task 11; Context SPEC §6.8 + §11.7 + BRAINSTORM Decision 2; the §11.7 empirical evidence shows Envoy's SIGTERM/SIGINT are STRUCTURALLY IDENTICAL paths producing immediate-exit (~6–7ms log: `caught X` → `shutting down server instance` → `exiting`; no drain delay). The **SURPRISE** is that this CONTRADICTS BRAINSTORM Decision 2's hypothesis (which assumed Envoy's SIGTERM = drain-then-exit, treating envoy-go's choice as Envoy parity); Decision: envoy-go's existing `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)` registration stays unchanged; the `<-ctx.Done()` body upgrades to `drainMgr.Drain()` → select on `Done()` / `time.After(Timeout())` → `cm.Drain()` → existing deferred-stop chain. This is a **DELIBERATE DIVERGENCE** from upstream Envoy; Consequences: (a) most Kubernetes / cluster orchestrators send SIGTERM expecting graceful drain (rolling-restart workflow); envoy-go's drain machinery honors this expectation; (b) the differential equivalence claim does NOT exercise the SIGTERM path — only the admin-trigger path runs differentially; the SIGTERM path is envoy-go-only structural-completeness; (c) BEHAVIOR_CONTRACT.md `## Graceful drain ### Drain triggers` (§13.4) documents the divergence at the contract level.

ADR-0095: Status Accepted; Doctrine D-3.3 + D-3.5; Lands-in-task Task 11; Context SPEC §11.7 verbatim re-validation of `"drain_time": "600s"` (Envoy default; per /server_info command_line_options field) + BRAINSTORM Decision 6; Decision: drain timeout is hardcoded `30 * time.Second` at the cmd/envoy-go/main.go boot site. The literal lives at the call site (not in the drain package) so test code can construct `drain.New(10 * time.Millisecond)` for fast-path tests; Consequences: (a) deliberate divergence from Envoy default (600s would block ~10 minutes on the differential gate); (b) the equivalence claim is over drain BEHAVIOR not timeout VALUE; (c) operator-knob to configure the timeout is deferred to a future runtime/hot-restart family phase; (d) the Manager itself does NOT enforce timeout — callers select on time.After alongside Done (per ADR-0091 design).

- [ ] **Step 3: Run tests; confirm clean**

```bash
go build ./cmd/envoy-go/...
go test -count=1 ./cmd/envoy-go/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./cmd/envoy-go/...
```

Expected: build clean, tests PASS, vet clean, lint clean.

- [ ] **Step 4: Commit**

```bash
git add cmd/envoy-go/main.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 08.2: cmd/envoy-go SIGTERM-handler upgrade + drain wiring [ADR-0092, ADR-0095]

drainMgr := drain.New(30 * time.Second) alloc lands post-bootstrap.Load,
pre-cluster.NewManagerWithBaseDir per planner-time decision 7. Threaded
into listener.NewManagerWithBaseDirAndAllowH2C and admin.New.

SIGTERM-handler block upgraded per SPEC §6.8: <-ctx.Done() →
drainMgr.Drain() → select { <-drainMgr.Done() / <-time.After(Timeout) } →
cm.Drain() → existing deferred-stop chain (LIFO: lm.Stop, admSrv.Close,
sinks-close).

ADR-0092 records the deliberate divergence from Envoy v1.37.2's SIGTERM=
immediate-exit per §11.7. ADR-0095 records the 30s envoy-go MVP timeout
(vs Envoy 600s default; operator-knob deferred).

go build ./cmd/envoy-go/... is clean again (broken since Task 3).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (main.go modifications), §5.1 (boot-order swimlane), §5.2 (lifecycle swimlane), §6.8 (SIGTERM-handler contract), §11.7 (empirical pin), §12 #7 + #9 (planner-time-resolved); ADR-0092 + ADR-0095.*

---

## Task 12: `test/fixtures/0010-graceful-drain/` differential fixture

**Files:**
- Create: `test/fixtures/0010-graceful-drain/envoy.yaml`
- Create: `test/fixtures/0010-graceful-drain/envoy-go.yaml`
- Create: `test/fixtures/0010-graceful-drain/expectations.yaml`
- Create: `test/fixtures/0010-graceful-drain/README.md`
- Create: `test/fixtures/0010-graceful-drain/driver/driver.go`
- Create: `test/fixtures/0010-graceful-drain/backends/backend.go`
- Modify: `test/differential/runner_test.go` (blank-import addition)

This task lands the new differential fixture per SPEC §7. The fixture has TWO driver paths in one binary (per BRAINSTORM Decision 12): the admin-trigger path runs against both proxies with per-proxy trigger-script normalization (envoy-go: `POST /drain_listeners`; reference Envoy: `POST /drain_listeners` + `POST /healthcheck/fail` per §11.2); the SIGTERM-trigger path runs envoy-go-only (per §11.7 deviation). The five per-state-transition equivalence claims per SPEC §7.1: (1) steady-state /ready byte-equal `LIVE\n`; (2) POST /drain_listeners response byte-equal `OK\n`; (3) /ready DRAINING byte-equal `DRAINING\n`; (4) /server_info DRAINING `state` field byte-equal `"DRAINING"`; (5) in-flight `GET /slow` request body byte-equal across both proxies.

The fixture mirrors the `test/fixtures/0009-admin-config-dump/` structural template per SPEC §4.3. The driver registers with `RequiresReference: true` (admin-trigger path requires reference Envoy) per the existing fixture-registration pattern (mirrors 0007a-cors / 0009-admin-config-dump). Per planner-time decision 8, the driver shares dual-proxy boot helpers and admin-scrape helpers with 0009 where natural; canonicalisation utilities are NOT shared (0010's per-state-transition byte-equality is structurally different from 0009's structural-projection canonicalisation).

**Precondition:** Tasks 2 + 3 + 4 + 5 + 7 + 8 + 9 + 10 + 11 done; the differential surface is fully wired; `test/fixtures/0010-graceful-drain/` does not exist.
**Artifact:** new fixture directory with 6 files; runner_test.go blank-import added.
**Acceptance:** `go test -run 'TestDifferential/0010-graceful-drain' ./test/differential/...` PASSES; pre-existing fixtures 0000–0009 still PASS.

- [ ] **Step 1: Create `test/fixtures/0010-graceful-drain/envoy.yaml`** per SPEC §7.4

```yaml
admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: 9902}
static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 0.0.0.0, port_value: 10001}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /slow}
                          route: {cluster: c_backend, timeout: 30s}
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: {{.BackendPort}}}}}
```

(Per ADR-0010, V4_ONLY is required for the reference side; the driver templates the backend port at runtime.)

- [ ] **Step 2: Create `test/fixtures/0010-graceful-drain/envoy-go.yaml`** per SPEC §7.4

```yaml
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: {{.AdminPort}}}
static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 127.0.0.1, port_value: {{.ListenerPort}}}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /slow}
                          route: {cluster: c_backend, timeout: 30s}
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: {{.BackendPort}}}}}
```

- [ ] **Step 3: Create `test/fixtures/0010-graceful-drain/expectations.yaml`** per SPEC §13.5 + §7.1 (prose; not machine-evaluated per ADR-0019)

```yaml
# Phase 08.2 fixture 0010-graceful-drain — expectations
#
# This file is prose, not machine-evaluated (per ADR-0019). The runner
# enforces the assertions described below via the driver's per-step
# assertions in driver.go.
#
# Asserted per-state-transition equivalence (per SPEC §7.1):
#
#   1. Steady-state /ready (pre-drain): byte-equal "LIVE\n" on both proxies.
#   2. POST /drain_listeners response: byte-equal "OK\n" on both proxies.
#      Status 200 on both. Headers structurally equivalent under the
#      standard six-header set per SPEC §11.6.
#   3. /ready DRAINING: byte-equal "DRAINING\n" on both proxies. Status 503.
#      NOTE: per §11.2 empirical pin, upstream Envoy v1.37.2's /ready returns
#      DRAINING ONLY after POST /healthcheck/fail. The driver's per-proxy
#      trigger script normalizes:
#        - envoy-go gets POST /drain_listeners (which triggers DRAINING in
#          envoy-go's design — unified drain.Manager).
#        - reference Envoy gets POST /drain_listeners + POST /healthcheck/fail
#          (which together trigger DRAINING in Envoy's design).
#   4. /server_info DRAINING: state field byte-equal "DRAINING" when both
#      proxies are in DRAINING. Other fields per the ADR-0088 allow-list
#      (08.1 baseline carries forward).
#   5. In-flight GET /slow request: 200 OK with body length 5KB; body bytes
#      byte-equal across both proxies (the upstream backend serves the same
#      content on both runs; the proxy is transparent).
#
# Connectivity-level checks (NOT body-level diff):
#   - New TCP connect attempt during drain: TCP handshake succeeds; HTTP
#     read returns empty/error per §11.5 accept-then-FIN. Both proxies
#     exhibit this behavior.
#
# SIGTERM-trigger driver path (per SPEC §7.3):
#   - Envoy-go ONLY (per §11.7 deviation — Envoy v1.37.2 SIGTERM is
#     immediate-exit, NON-equivalent). Asserts envoy-go's drain rendezvous
#     + exit ordering are correct. Exit status 0 expected.
#
# Cross-references:
#   - SPEC §7 (differential fixture); §11.1–§11.7 (verbatim Envoy scrapes)
#   - ADR-0093 (POST /drain_listeners contract)
#   - ADR-0097 (/ready DRAINING extension)
#   - ADR-0098 (/server_info DRAINING transition)
#   - ADR-0094 (listener accept-then-FIN)
#   - planner-time decision 8 (driver framework reuse: shared boot helpers;
#     canonicalisation NOT shared)
```

- [ ] **Step 4: Create `test/fixtures/0010-graceful-drain/README.md`** per SPEC §4.3

```markdown
# Fixture 0010 — graceful-drain differential

This fixture asserts per-state-transition equivalence between envoy-go's
graceful-drain surface and reference Envoy v1.37.2 under a slow-streaming-
backend probe (5KB at 1KB/s = 5s in-flight window). Two driver paths in one
binary per SPEC §7 + BRAINSTORM Decision 12.

## Driver paths

1. **Admin-trigger path (against both proxies):** boot envoy-go + reference
   Envoy + slow-streaming Go HTTP backend on :18001; sanity-scrape /ready
   on each proxy (expect LIVE\n); start a long-lived `GET /slow` request
   on each listener; trigger drain (per-proxy script — see §11.2 deviation
   note); poll /ready until DRAINING\n; scrape /server_info; attempt new
   conn (expect accept-then-FIN); wait for in-flight to complete; assert
   in-flight body byte-equal; cleanup via SIGKILL.
2. **SIGTERM-trigger path (envoy-go only):** boot envoy-go + backend; sanity-
   scrape; start in-flight; SIGTERM envoy-go; poll /ready until DRAINING;
   wait for in-flight to complete; wait for envoy-go to exit; assert exit
   status 0. Per §11.7 deviation — Envoy v1.37.2 SIGTERM is immediate-exit;
   only envoy-go has the drain-then-exit semantics.

## Per-proxy trigger script (per SPEC §7.2 + §11.2 deviation)

- envoy-go: `POST /drain_listeners` (single trigger; unifies listener drain
  and load-balancer-disposition flip per ADR-0091).
- reference Envoy: `POST /drain_listeners` + `POST /healthcheck/fail` (two
  triggers; Envoy separates listener drain from load-balancer-disposition
  flip per §11.2 finding).

## Backend shape

Minimal Go HTTP backend on :18001. `/slow` streams 5KB at 1KB/s (5s total
response time); `/` serves a fast 200 OK + `backend1\n` for sanity. Per
SPEC §7.5.

## Cross-references

- SPEC §7 (differential fixture); §11 (empirical pins)
- ADR-0091 (drain state machine); ADR-0093 (/drain_listeners contract);
  ADR-0094 (listener accept-then-FIN); ADR-0097 (/ready DRAINING);
  ADR-0098 (/server_info DRAINING)
- BEHAVIOR_CONTRACT.md ## Graceful drain (the contract umbrella the fixture
  exercises)
- planner-time decision 8 (framework reuse: shared boot helpers; per-state-
  transition byte-equality NOT shared with 0009's structural-projection)
```

- [ ] **Step 5: Create `test/fixtures/0010-graceful-drain/backends/backend.go`** per SPEC §7.5

```go
package main

import (
	"bytes"
	"net/http"
	"time"
)

// backend is the slow-streaming Go HTTP backend for fixture 0010-graceful-
// drain. /slow streams 5KB at 1KB/s for a stable 5s in-flight window; / is
// a fast sanity baseline. Per SPEC §7.5.
func main() {
	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 5; i++ {
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, 1024))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1 * time.Second)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend1\n"))
	})
	_ = http.ListenAndServe(":18001", nil)
}
```

- [ ] **Step 6: Create `test/fixtures/0010-graceful-drain/driver/driver.go`** per SPEC §7.2 + §7.3 (skeleton; the implementer fleshes out per the existing 0009-admin-config-dump driver structural template)

The driver's structural skeleton (~350 LoC) — the implementer adapts to whatever exact harness conventions exist in `test/differential/fixture/fixture.go` and `test/differential/runner_test.go` per planner-time decision 8 (share boot/admin-scrape helpers with 0009; do NOT share canonicalisation):

```go
// Package driver registers the 0010-graceful-drain fixture with the
// differential runner. Asserts per-state-transition equivalence between
// envoy-go's graceful-drain surface and reference Envoy v1.37.2 under a
// slow-streaming-backend probe per SPEC §7.
//
// Two driver paths in one binary per BRAINSTORM Decision 12:
//   1. Admin-trigger path (against both proxies; per-proxy trigger-script
//      normalization for §11.2 deviation).
//   2. SIGTERM-trigger path (envoy-go only per §11.7 deviation).
//
// RequiresReference: true (admin-trigger path requires reference Envoy).
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"syscall"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const (
	fixtureName              = "0010-graceful-drain"
	refContainerListenerPort = 10001
	backendPort              = 18001
	inflightWindowMs         = 5000
)

type drainDriver struct{}

func init() {
	fixture.RegisterFixture(fixtureName, &drainDriver{})
}

func (drainDriver) BackendCount() int                      { return 1 }
func (drainDriver) BackendKind() fixture.BackendKind       { return fixture.HTTPHello /* or a custom slow-stream kind if the harness has one */ }
func (drainDriver) SubjectListenerName() string            { return "l_main" }
func (drainDriver) ReferenceListenerPort() int             { return refContainerListenerPort }
func (drainDriver) RequiresReference() bool                { return true }

// SubjectConfig + ReferenceBootstrap render the YAML templates with the
// allocated ports per the existing 0009 driver pattern.
func (drainDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	// (template-render envoy-go.yaml with AdminPort=subjAdminPort,
	//  ListenerPort=subjListenerPort, BackendPort=18001)
	// ...
}

func (drainDriver) ReferenceBootstrap(_ []int) string {
	// (template-render envoy.yaml with BackendPort=18001)
	// ...
}

// DriveSubject and DriveReference are the runner-invoked entry points.
// DriveSubject orchestrates BOTH the admin-trigger path AND the SIGTERM-
// trigger path against envoy-go; DriveReference orchestrates ONLY the
// admin-trigger path against reference Envoy. The byte-streams returned
// by each Drive* are the per-state-transition assertion log (per-step
// outcome lines) which the runner's CompareBytes pass enforces.

func (drainDriver) DriveSubject(ctx context.Context, listenerAddr string, adminAddr string) ([]byte, error) {
	var out bytes.Buffer
	if err := driveAdminTriggerPath(ctx, &out, listenerAddr, adminAddr, "envoy-go"); err != nil {
		return nil, fmt.Errorf("subject admin-trigger: %w", err)
	}
	// SIGTERM-trigger path runs against envoy-go only; the runner's harness
	// must expose a hook to send SIGTERM to the subject subprocess. The
	// implementer adapts to whatever the runner exposes (e.g., a callback
	// passed in via context, or a separate fixture.Driver method).
	// ...
	return out.Bytes(), nil
}

func (drainDriver) DriveReference(ctx context.Context, listenerAddr string, adminAddr string) ([]byte, error) {
	var out bytes.Buffer
	if err := driveAdminTriggerPath(ctx, &out, listenerAddr, adminAddr, "ref-envoy"); err != nil {
		return nil, fmt.Errorf("ref admin-trigger: %w", err)
	}
	return out.Bytes(), nil
}

// driveAdminTriggerPath implements SPEC §7.2 step-by-step (event-based
// synchronization throughout; no hardcoded sleeps per 07.2 REVIEW M-8).
func driveAdminTriggerPath(ctx context.Context, out *bytes.Buffer, listenerAddr, adminAddr, side string) error {
	// Step 1: Sanity scrape /ready (expect 200 + LIVE\n).
	if err := scrapeAndExpect(ctx, out, adminAddr, "/ready", 200, "LIVE\n"); err != nil {
		return err
	}
	// Step 2: Sanity scrape /server_info (expect state: "LIVE").
	if err := scrapeServerInfoExpectState(ctx, out, adminAddr, "LIVE"); err != nil {
		return err
	}
	// Step 3: Open long-lived in-flight GET /slow.
	inflightDone := make(chan inflightResult, 1)
	go runInflightSlow(ctx, listenerAddr, inflightDone)
	// Step 4: Wait for first chunk to confirm in-flight is established
	// (event-based: poll the backend for the request log, OR use a small
	// channel handshake from the goroutine when the first byte arrives).
	// ...
	// Step 5: Trigger drain per-proxy script.
	if side == "envoy-go" {
		if err := scrapeAndExpect(ctx, out, adminAddr, "/drain_listeners", 200, "OK\n", postMethod()); err != nil {
			return err
		}
	} else {
		if err := scrapeAndExpect(ctx, out, adminAddr, "/drain_listeners", 200, "OK\n", postMethod()); err != nil {
			return err
		}
		if err := scrapeAndExpect(ctx, out, adminAddr, "/healthcheck/fail", 200, "OK\n", postMethod()); err != nil {
			return err
		}
	}
	// Step 6: Poll /ready until DRAINING\n (max 1s).
	if err := pollReadyDraining(ctx, out, adminAddr, 1*time.Second); err != nil {
		return err
	}
	// Step 7: Scrape /server_info; assert state: "DRAINING".
	if err := scrapeServerInfoExpectState(ctx, out, adminAddr, "DRAINING"); err != nil {
		return err
	}
	// Step 8: New TCP conn → expect accept-then-FIN.
	if err := assertAcceptThenFIN(ctx, out, listenerAddr, 1*time.Second); err != nil {
		return err
	}
	// Step 9: Wait for in-flight to complete (max 6s).
	select {
	case res := <-inflightDone:
		// Step 10: Assert in-flight body shape (5KB; status 200; body bytes
		// will be byte-compared across both proxies by the runner).
		fmt.Fprintf(out, "inflight: status=%d, body_len=%d, body_sha=%s\n", res.status, res.bodyLen, res.bodySHA)
	case <-time.After(6 * time.Second):
		return fmt.Errorf("inflight did not complete within 6s")
	}
	// Step 11: Re-scrape /server_info; assert state STILL "DRAINING".
	if err := scrapeServerInfoExpectState(ctx, out, adminAddr, "DRAINING"); err != nil {
		return err
	}
	// Step 12: cleanup happens via the runner's harness teardown (SIGKILL).
	return nil
}

// (helper functions: scrapeAndExpect, pollReadyDraining, assertAcceptThenFIN,
// runInflightSlow, scrapeServerInfoExpectState, postMethod — implementer
// fleshes out per existing 0009 driver helpers; per planner-time decision 8
// the boot helpers + admin-scrape helpers may move to test/differential/
// helpers/ if natural.)
```

The implementer adapts to whatever exact harness conventions exist (e.g., the runner's `RequiresReference` plumbing may need a small tweak; the SIGTERM injection path may need an additional fixture.Driver method or context callback). Two possible gotchas: (1) the runner may not yet support per-driver SIGTERM injection — if so, the SIGTERM-trigger path of this fixture lands as a separate `*_test.go` file directly in `test/fixtures/0010-graceful-drain/driver/` that exec's a subject envoy-go process and sends SIGTERM directly (NOT via the differential runner); (2) the inflight-body byte-equality check requires the runner's harness to surface both bytes-streams — the standard differential harness CompareBytes pass already does this for the byte-output of Drive* methods. Iterate at impl-task time.

- [ ] **Step 7: Add the blank-import to `test/differential/runner_test.go`**

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver"
```

(Insert in alphabetical order, after the `0009-...` import.)

- [ ] **Step 8: Run the new fixture**

```bash
go test -count=1 -v -run 'TestDifferential/0010-graceful-drain' ./test/differential/... 2>&1 | tail -40
```

Expected: PASS. If FAIL, the failure is one of:
- the per-proxy trigger script normalization is incorrect (revisit step 5);
- the in-flight goroutine's first-byte handshake races with the drain trigger (refine the event-based sync);
- the accept-then-FIN assertion times out (extend the wait, or check the listener fast-path is firing);
- the runner's harness does not expose a SIGTERM injection path (move the SIGTERM-trigger path to a sibling `*_test.go` file as documented in Step 6 gotcha 1).

Iterate until green; record each iteration in PROGRESS.md.

- [ ] **Step 9: Run the full differential suite to verify no regression**

```bash
go test -count=1 -v ./test/differential/... 2>&1 | tail -30
```

Expected: every fixture (0000–0010) PASS.

- [ ] **Step 10: Commit**

```bash
git add test/fixtures/0010-graceful-drain/ test/differential/runner_test.go
git commit -m "phase 08.2: test/fixtures/0010-graceful-drain — graceful-drain differential fixture"
```

SHA-fill follow-up.

*Anchored: SPEC §3 gate (e) (differential fixtures green), §4.3 (fixture deliverables), §7 (driver outline + bootstrap + backend), §13.5 (equivalence-matrix rows), §11.2 + §11.5 + §11.7 (empirical-pin evidence the fixture exercises); planner-time decision 8.*

---

## Task 13: BEHAVIOR_CONTRACT restructure + ADR-0099 + six-gate verification + REVIEW + phase-done commit (closes 08.2 + parent 08; MVP-trunk close)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (per SPEC §13 verbatim Markdown patches)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0099; in-place amend ADR-0089)
- Modify: `docs/envoy-go/ROADMAP.md` (rows `08.2` AND `08` BOTH `in-progress → done` flip; MVP-trunk closure)
- Modify: `docs/envoy-go/STATE.md` (rewrite to MVP-trunk-closed `awaiting next planning`)
- Create: `docs/envoy-go/phases/08.2-graceful-drain/REVIEW.md` (end-of-phase review)
- Modify: `docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md` (final task entry)

This task is the phase-done landing AND the BOOTSTRAP_PROMPT.md §8 MVP-trunk close. It runs the six-gate verification sweep per SPEC §3 + BOOTSTRAP §7.5; populates `BEHAVIOR_CONTRACT.md` + ADR-0099 in place; runs the requesting-code-review skill to populate REVIEW.md; and commits the phase-done bundle in one commit per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 closure pattern. **ROADMAP rows `08.2` AND `08` BOTH flip `in-progress → done` AT THIS COMMIT** (parent SPEC §5 — MVP-trunk closure). The 08.2 REVIEW.md additionally closes the parent 08 row (the 08-family review covers 08.1 + 08.2 jointly).

**Precondition:** Tasks 1-12 done; all unit tests + differential suite + fuzzers + h2spec re-run green; `go vet ./... && golangci-lint run ./... && go test -race ./...` clean.
**Artifact:** the BEHAVIOR_CONTRACT.md restructure; ADR-0099 + ADR-0089 in-place amendment; ROADMAP double-flip; STATE advance to MVP-trunk-closed; REVIEW.md; phase-done commit.
**Acceptance:** `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` subsection present + `### /ready` extension + `### /server_info` extension + new `## Graceful drain` umbrella + three new equivalence-matrix rows + ADR-0015/ADR-0088/ADR-0090 forward-pointer notes; ADR-0099 in DECISIONS.md; ADR-0089 in-place-amended; ROADMAP rows `08.2` AND `08` read `done`; STATE.md `lifecycle-state: post-MVP-trunk` + `next-skill: superpowers:brainstorming` (against §9 family list); phase-done commit subject `phase 08.2: graceful-drain [ADR-0091, ADR-0092, ADR-0093, ADR-0094, ADR-0095, ADR-0096, ADR-0097, ADR-0098, ADR-0099]`.

- [ ] **Step 1: Six-gate verification sweep**

Run all six gates per BOOTSTRAP §7.5 + SPEC §3. Record outputs in PROGRESS.md.

```bash
# Gate (a): build clean
go build ./...
go vet ./...
golangci-lint run ./...

# Gate (b): unit tests + race
go test -count=1 ./...
go test -count=1 -race ./...

# Gate (c): h2spec re-run at ADR-0051 pin (53/53 PASS unchanged)
go test -count=1 -v ./test/conformance/h2spec/... 2>&1 | tail -20

# Gate (d): all 11 fuzzers (10 from 08.1 + FuzzDrainTransitions from 08.2) at 30s
for fuzz in FuzzBootstrapLoad FuzzTcpProxyFilter FuzzTLSContextParse FuzzHCMConfigParse FuzzFrameStream FuzzHPACKDecode FuzzPromTextFormat FuzzDefaultFormatRender FuzzFilterChainParse FuzzConfigDumpFormat FuzzDrainTransitions; do
  echo "=== $fuzz ==="
  pkg=$(grep -lr "func $fuzz" --include='*.go' | head -1 | xargs dirname)
  go test -fuzz=$fuzz -fuzztime=30s ./$pkg/ 2>&1 | tail -3
done

# Gate (e): differential 0000-0010 all green
go test -count=1 -v ./test/differential/... 2>&1 | tail -40

# Gate (f): BEHAVIOR_CONTRACT.md to be populated by Steps 2-3 of THIS task
```

If any gate fails, fix the failure (NOT in this task; document the root-cause and either backport to the failing task's commit or land a small bridge commit before continuing). Re-run the failing gate to verify green.

- [ ] **Step 2: Edit `docs/envoy-go/BEHAVIOR_CONTRACT.md`** per SPEC §13.1–§13.7

Apply the verbatim Markdown patches per SPEC §13:

(a) Insert NEW `### /drain_listeners` subsection under `## Admin API` umbrella, after `### /server_info`, preserving alphabetical-by-path order. Body content per SPEC §13.1 verbatim Markdown patch.

(b) Append new sub-block under `### /ready` per SPEC §13.2 verbatim Markdown patch (DRAINING-state response paragraph + ADR-0015 forward-pointer note).

(c) Modify the State enum bullet under `### /server_info` per SPEC §13.3 verbatim Markdown patch (extends to LIVE/PRE_INITIALIZING/DRAINING; adds ADR-0088 forward-pointer note).

(d) Insert NEW sibling `## Graceful drain` umbrella section per SPEC §13.4 verbatim Markdown patch (drain triggers + drain semantics + drain timeout + connection-level drain semantics + drain manager API surface + `### Applies to` and `### Does not yet apply to` lists).

(e) Append three new rows to `## Equivalence Matrix` table per SPEC §13.5 verbatim table-row patch.

(f) Header allow-list extensions: NONE (per SPEC §13.6).

(g) Forward-pointer notes per SPEC §13.7: ADR-0015 partially superseded by ADR-0097; ADR-0088 amended by ADR-0098; ADR-0090 partially amended by ADR-0093.

- [ ] **Step 3: Append ADR-0099 to `docs/envoy-go/DECISIONS.md`** + in-place amend ADR-0089

ADR-0099: Status Accepted; Doctrine D-3.5; Lands-in-task Task 13; Context SPEC §2.1 + BRAINSTORM Decision 11 + BOOTSTRAP_PROMPT.md §9 (Runtime + hot restart family); Decision: hot restart / parent-child handoff (SCM_RIGHTS file-descriptor transfer + shared-memory existing-connection table + parent-shutdown-time orchestration + custom signal protocol like SIGUSR1) is OUT OF SCOPE for 08.2 and the entire BOOTSTRAP_PROMPT.md §8 MVP trunk; deferred to a future feature-family phase under §9's Runtime + hot restart family; Consequences: (a) envoy-go MVP drain is one-process scope only; (b) future family phase delivers SCM_RIGHTS-based handoff; (c) the deferral is recorded in BEHAVIOR_CONTRACT.md `## Graceful drain ### Does not yet apply to` per §13.4; (d) cross-ref ADR-0089 (parallel admin-endpoint deferral list — `/quitquitquit` and `/healthcheck/fail` carry adjacent deferrals in the same MVP scope-bounding cluster).

ADR-0089 in-place amendment: locate ADR-0089's deferral table in `docs/envoy-go/DECISIONS.md`; flip the `POST /drain_listeners` entry from "08.2 (graceful drain)" to "delivered in 08.2 per ADR-0093" per ADR-0089 consequence (b) in-place-edit pattern. The `/healthcheck/fail` entry stays in the deferral list (envoy-go MVP unifies the listener-drain + load-balancer-disposition triggers under /drain_listeners + the drain manager).

- [ ] **Step 4: Run `superpowers:requesting-code-review` (or equivalent) and create `REVIEW.md`**

Per BOOTSTRAP §7.5 gate (f) cadence. The REVIEW.md follows the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 shape:
- Headline assessment (1 paragraph)
- Strengths (3-5 bullets)
- Findings (Major / Minor / Note tiers; numbered M-1, M-2, ... and N-1, N-2, ...)
- Carry-forward dispositions (which Minor findings carry to post-MVP-trunk feature-family work vs which are landed inline)
- **Parent 08 row close** — the REVIEW additionally addresses the 08-family closure (08.1 closed at master `70e6a65`; 08.2 closes at THIS commit; parent row `08` flips done at THIS commit per parent SPEC §5; the REVIEW.md confirms no cross-sub-phase regressions)
- Six-gate verification appendix (all six gates verbatim outputs from Step 1)

The implementer drafts REVIEW.md by:
1. Reading the entire 08.2 commit history (`git log --reverse master.. -- internal/drain internal/admin internal/listener internal/cluster internal/filter cmd/envoy-go test/fixtures/0010-graceful-drain`).
2. Spawning a code-reviewer subagent (per `superpowers:requesting-code-review` skill) with the SPEC + the diff + the 08.1 REVIEW.md as a structural template.
3. Capturing the subagent's findings into REVIEW.md.
4. For each Major finding: stop the session, re-open the impl task, fix, and re-verify. Major findings BLOCK phase-done.
5. For each Minor finding: decide carry-forward vs inline-fix; record in §10 carry-forward.
6. Confirm parent 08 row close: 08.1 carry-forwards N-2/N-3/N-5 stay carry-forward (08.2 takes no inline action); N-1 + N-4 status updated (N-1 landed inline at Task 5; N-4 cross-reference doc-comment lands inline IFF Task 12 fixture shares utilities with 0009).

- [ ] **Step 5: Update `docs/envoy-go/ROADMAP.md`** — rows `08.2` AND `08` BOTH flip `in-progress → done`

Locate row `08.2`; flip status from `in-progress` to `done`. **Locate row `08`; flip status from `in-progress` to `done`** (parent SPEC §5 MVP-trunk-close discipline). All earlier rows (00–07.2 + 08.1) remain `done`.

- [ ] **Step 6: Rewrite `docs/envoy-go/STATE.md`** to advance to MVP-trunk-closed `awaiting next planning`

```markdown
# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `awaiting next planning` (BOOTSTRAP_PROMPT.md §8 MVP trunk closed at THIS commit; next-phase brainstorm session selects a feature-family row from §9).
- **phase-directory:** N/A — the MVP trunk (phases 00–08) is closed read-only history. The next session creates `docs/envoy-go/phases/<NN-family-slug>/` per §9's family-by-family expansion.
- **lifecycle-state:** `0` for the next phase (per BOOTSTRAP §5 — Phase not yet in ROADMAP.md). The next session's first action: `superpowers:brainstorming` against the §9 family list to pick the next row.
- **next-skill:** `superpowers:brainstorming` — autonomous brainstorm session selecting and scoping the first §9 feature-family phase. Inputs: `BOOTSTRAP_PROMPT.md` §9 (feature-family headings); `docs/envoy-go/ROADMAP.md` (current state); `docs/envoy-go/phases/08.2-graceful-drain/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (just-closed sub-phase artefacts; the 08.2 brainstorm-and-spec patterns are the structural precedent for §9 family work).
- **next-skill-scope:** Lifecycle-state 0 → 1 deliverables: `docs/envoy-go/phases/<NN-family-slug>/BRAINSTORM.md` + selecting which §9 family to enter first per the user's stated priority. The §9 families: HTTP filters, Network filters, Load balancing, Upstream robustness, HTTP/3 + QUIC, gRPC, xDS / dynamic config, Observability, Runtime + hot restart, WASM host, Deprecated/edge features. The brainstorm scopes the first family as a parent phase (likely needing further splitting per ADR-0045) and adds a row to ROADMAP.md. After BRAINSTORM, advance STATE to lifecycle-state 1 with `next-skill: superpowers:writing-plans` for SPEC drafting.
- **last-commit:** `<phase-done commit SHA — TBD; SHA-fill follow-up>` — `phase 08.2: graceful-drain [ADR-0091..ADR-0099]`. Lands the new internal/drain/ package + drain wiring + POST /drain_listeners + /ready + /server_info DRAINING extensions + listener Accept-loop fast-path + cluster Drain + HCM/TCP-proxy Inc/Dec hooks + cmd/envoy-go SIGTERM-handler upgrade + differential fixture 0010-graceful-drain + BEHAVIOR_CONTRACT umbrella restructure + nine new ADRs ADR-0091..ADR-0099. **ROADMAP rows 08.2 AND 08 BOTH flipped `in-progress → done` at this commit (BOOTSTRAP_PROMPT.md §8 MVP-trunk closure per parent SPEC §5).** SHA filled in a follow-up commit per the phase-04..08.1 SHA-fill convention.
- **last-updated:** <date>

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
```

- [ ] **Step 7: Update PROGRESS.md** with the Task 13 entry

Append the Task 13 entry summarising the BEHAVIOR_CONTRACT restructure + the ADR-0099 + ADR-0089-amendment landings + the ROADMAP double-flip + the STATE advance to MVP-trunk-closed + the six-gate verification.

- [ ] **Step 8: Phase-done commit + MVP-trunk-close commit (one commit; covers both)**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/08.2-graceful-drain/PROGRESS.md docs/envoy-go/phases/08.2-graceful-drain/REVIEW.md
git commit -m "$(cat <<'EOF'
phase 08.2: graceful-drain [ADR-0091, ADR-0092, ADR-0093, ADR-0094, ADR-0095, ADR-0096, ADR-0097, ADR-0098, ADR-0099]

Lands the new internal/drain/ package (Manager with three-state machine
LIVE/DRAINING/DRAINED-as-channel-close per ADR-0091); the new POST
/drain_listeners admin endpoint with method-discrimination ENFORCED per
ADR-0093 (partially amends ADR-0090); /ready DRAINING-state body
"DRAINING\n" extension per ADR-0097 (partially supersedes ADR-0015);
/server_info "state": "DRAINING" extension per ADR-0098 (amends ADR-0088);
internal/listener.Manager.Drain() + per-runtime Accept-loop fast-path
per ADR-0094 (accept-then-FIN per §11.5); internal/cluster.Manager.Drain()
+ per-Cluster.closePool() helper consolidated into ADR-0096; HCM
decodeHeaders/encodeFinalize-edge Inc/Dec hooks + markedInflight sentinel
per ADR-0096 + ADR-0075 sendLocalReply discipline; TCP-proxy
OnNewConnection/OnConnectionClose Inc/Dec hooks per ADR-0096;
cmd/envoy-go/main.go SIGTERM-handler block upgrade per ADR-0092
(deliberate divergence from Envoy v1.37.2 SIGTERM=immediate-exit per
§11.7) + 30s drain-timeout default per ADR-0095; LBP-1 fifth application
*drain.Manager threading per ADR-0085 consequence extension; differential
fixture 0010-graceful-drain (admin-trigger path against both proxies +
SIGTERM-trigger path envoy-go-only per §11.7 deviation); BEHAVIOR_CONTRACT
## Admin API ### /drain_listeners NEW subsection + ### /ready DRAINING
extension + ### /server_info DRAINING extension + NEW sibling ##
Graceful drain umbrella section + three new equivalence-matrix rows per
ADR-0052; ADR-0099 hot-restart deferral; FuzzDrainTransitions (eleventh
fuzzer; 30s budget per ADR-0018) per planner-time decision 1.

ROADMAP row 08.2 flips in-progress → done at THIS commit. **Parent row
08 SIMULTANEOUSLY flips in-progress → done at THIS commit (parent SPEC §5
MVP-trunk closure — BOOTSTRAP_PROMPT.md §8 MVP trunk is now closed).**
Phases 00–08 are all done. Next session: §9 feature-family expansion.

Six-gate verification all green:
  (a) go build ./... + go vet + golangci-lint clean
  (b) go test -race ./... clean (incl. extended TestAdminConcurrentScrapeRace
      with seven endpoints + Drain mid-test goroutine)
  (c) h2spec 53/53 PASS at ADR-0051 pin (unchanged)
  (d) 11 fuzzers run clean at 30s short-budget (ADR-0018) — incl. new
      FuzzDrainTransitions
  (e) differential 0000-0010 all green (incl. new 0010-graceful-drain)
  (f) BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners +
      ## Graceful drain umbrella populated
EOF
)"
```

SHA-fill follow-up commit per the phase-04..08.1 convention.

*Anchored: SPEC §3 (phase-done gates), §13 (BEHAVIOR_CONTRACT additions), §15 (acceptance checklist), parent SPEC §5 (MVP-trunk close at 08.2 phase-done), BOOTSTRAP §5.3 (commit-message format), §7.5 (six-gate checklist), §8 (MVP trunk completion).*

---

## Refinement

The PLAN above is sized at 13 tasks (Task 6 is a no-op consolidation slot — see Task 6 header) per the STATE.md projection of ~12–15 tasks. Notes the implementer should be aware of:

1. **Task 3 leaves cmd/envoy-go broken intermediately** (between Tasks 3 and 11). This is intentional — the alternative is to land main.go's call-site update as part of Task 3, which conflates the admin-package widening with the cmd-package wiring. PROGRESS.md documents the intermediate breakage explicitly. Task 11 fixes it.

2. **Task 5 leaves the listener filterRegistry HCM/TCP-proxy closures with `_ = dm` discards** (between Tasks 5 and 9/10). Each closure widens its signature to take `dm` but discards it via `_ = dm` until the inner filter constructors widen at Tasks 9/10. PROGRESS documents this.

3. **Task 9's HCM Inc/Dec hook sites are settled at impl-task time** per the codebase reality. The PLAN names the natural edges (request-begin after parse, before chain run; request-end after access-log emit) but does not pin file:line because the exact code structure of `connection.go` and `h2dispatch.go` may have evolved. The implementer reads the current code, picks the natural edges, and uses the markedInflight sentinel pattern to ensure pair-balance under sendLocalReply per ADR-0075.

4. **Task 12's driver may need iteration on the first run.** The per-proxy trigger script normalization, the in-flight goroutine handshake, the accept-then-FIN assertion, and the SIGTERM injection (if the runner does not yet support it as a fixture.Driver method) all require contact with reality. The implementer iterates: run fixture, observe diff, refine, re-run. Each iteration committed separately.

5. **Task 13's REVIEW.md may surface findings that block phase-done.** Per the closure pattern, Major findings BLOCK; Minor findings carry to post-MVP-trunk feature-family work unless inline-fixable. The implementer follows the requesting-code-review skill's guidance. Because Task 13's commit is also the **MVP-trunk-close commit**, any Major finding here is exceptionally costly — the implementer should be especially rigorous through Tasks 1-12 to avoid late-cycle blockers.

---

## Post-plan handoff

After Task 13's phase-done commit + SHA-fill follow-up:

- ROADMAP row `08.2`: `done`. Row `08`: `done` (MVP-trunk closure). All earlier rows (00–07.2 + 08.1) remain `done`. Next-phase row TBD per the §9 family the next-session brainstorm picks.
- STATE.md: `active-phase: awaiting next planning`, `lifecycle-state: 0` (for the next phase), `next-skill: superpowers:brainstorming`.
- BEHAVIOR_CONTRACT.md: `## Admin API ### /drain_listeners` populated; `### /ready` + `### /server_info` DRAINING extensions populated; new `## Graceful drain` umbrella populated; three new equivalence-matrix rows.
- DECISIONS.md tail: `ADR-0099`. Next-free: `ADR-0100` (next-family ADRs anticipated).
- Production code count post-08.2: ~2300 LoC (08.1's ~1700 + 08.2's ~600) + ~2260 LoC tests + ~1220 LoC fixture; fuzzer count 11; differential fixture count 12 (0000-0010 + 0007a-cors + 0007b-iteration-probe).
- The envoy-go MVP trunk is closed: a minimal but real proxy with bootstrap, listener + TCP proxy + RR LB, TLS termination + origination + SNI, HCM + route match + router filter + direct_response, HTTP/2 down + up (low-level framer + own conn mgr), access log + stats + Prometheus, filter chain framework + extension registry, admin API (8 endpoints: 6 read-only from 08.1 + /drain_listeners + /ready inherits) + graceful drain. Phase 09+ enters feature-family expansion per BOOTSTRAP_PROMPT.md §9.

---

## References

- **SPEC:** `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` — the authoritative source; every PLAN task traces to one or more SPEC sections (§§1–16).
- **BRAINSTORM:** `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` — autonomous brainstorm artefact the SPEC distils §§1–11 from.
- **Parent master SPEC:** `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` — cross-cutting context for the 08.1 + 08.2 split; per parent §5, parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2's phase-done (this PLAN's Task 13 phase-done IS the MVP-trunk-close commit).
- **Sibling SPEC stub:** `docs/envoy-go/phases/08.2-graceful-drain/README.md` — placeholder; read-only history as of the 08.2 SPEC commit.
- **Sibling sub-phase (08.1):** `docs/envoy-go/phases/08.1-admin-endpoints/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` — closed read-only history; the 08.1 PLAN is the structural precedent (task-numbering, TDD step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections); the 08.1 admin-mux scaffold (six handlers on the same mux) is the host structure 08.2 extends with the seventh handler.
- **Structural precedent (PLAN shape):** `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` + `07.2-listener-chain-completion/PLAN.md` — task-numbering, heredoc-style task headers, ADR-with-first-use-commit, "Anchored:" footers.
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — sub-phase position), §5.3 (commit-message format — phase-done subject), §6.2 (planner-time-split discipline; ADR-0084 applies to the parent), §7.5 (phase-done gate — six-gate checklist; SPEC §3 specialises), §4.1 (artifact-layout invariants — ROADMAP row flip discipline), §8 (MVP trunk — phase 08 closes the trunk; 08.2's phase-done IS the trunk-close commit), §9 (Feature families — what comes next after MVP-trunk closure).
- **DECISIONS.md cross-references:**
  - **Inherited (cited, not amended):** ADR-0003, ADR-0004, ADR-0005, ADR-0008, ADR-0014, ADR-0017, ADR-0018, ADR-0040, ADR-0041, ADR-0044, ADR-0045, ADR-0051, ADR-0052, ADR-0063, ADR-0072, ADR-0079, ADR-0083.
  - **Partially superseded:** ADR-0015 (pre-init contract for /ready) — partially superseded by ADR-0097 (DRAINING extension); pre-init body and status preserved verbatim.
  - **Amended:** ADR-0085 (LBP-1 third application — consequence (a) extended in-place to enumerate the fifth application); ADR-0088 (state-enum coverage — amended by ADR-0098 purely additively); ADR-0089 (admin-endpoint deferral list — POST /drain_listeners line flips from "08.2" to "delivered in 08.2 per ADR-0093" via in-place edit per ADR-0089 consequence (b)); ADR-0090 (no-method-discrimination posture — partially amended by ADR-0093, qualified to read-only endpoints); ADR-0075 (sendLocalReply Inc/Dec balance discipline — markedInflight sentinel pattern realizes the discipline at the drain-counter layer).
  - **New (this PLAN lands):** ADR-0091 through ADR-0099 per SPEC §8.
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Server-build SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (per SPEC §11.7 verbatim re-validation; also per 08.1 SPEC §11.4). All seven SPEC §11 empirical pins reference this image SHA.
- **CONFORMANCE_PINS.md:** UNCHANGED in 08.2 — D-3.7 reserves pin bumps for dedicated phases. The h2spec gate at 53/53 PASS is mechanical re-run because 08.2's HCM Inc/Dec hooks are non-load-bearing for the H2 codec/framer/hpack path.
- **ROADMAP.md:** rows `08`, `08.2` per the SPEC commit's row-flip; rows `08.2` AND `08` BOTH flip `in-progress → done` at this PLAN's Task 13 phase-done (MVP-trunk closure per parent SPEC §5).
