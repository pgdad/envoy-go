# Phase 08.2 Brainstorm — Graceful Drain

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-1 brainstorm session for sub-phase 08.2 (`graceful-drain`). The next session (lifecycle-state 1 → 2 for sub-phase 08.2, skill `superpowers:writing-plans`) authors `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §11 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-08.2-graceful-drain-brainstorm`, branch `phase-08.2-graceful-drain-brainstorm`, branched from master tip `eb3babd9b4ee71411076553318dc1b32c2ef1e7b` (the 08.1 phase-done SHA-fill commit `eb3babd`).

**Brainstorm mode:** autonomous per ADR-0004 (no live human-in-the-loop). Decisions are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0090), parent BRAINSTORM (`docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md`), parent SPEC (`docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md`), the just-shipped 08.1 sub-phase artefacts (SPEC, PLAN, PROGRESS, REVIEW), and the sibling SPEC stub (`docs/envoy-go/phases/08.2-graceful-drain/README.md`). Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §11 and deferred to SPEC-drafting time.

**Document shape:** mirrors `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` (the parent-08 BRAINSTORM that authored the 08.1 SPEC stub for §3) section-for-section with the 08.2-specific topical map specified by `STATE.md`'s `next-skill-scope` field. Sections §§1–12 are autonomous-brainstorm Decision-bearing prose; §11 enumerates the empirical-pin obligations the 08.2 SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

---

## 1. Mission and scope confirmation (08.2 only)

ROADMAP row `08.2 | graceful-drain | 08.1 | planned | | …` (per `ROADMAP.md` line 47, in the worktree at HEAD `eb3babd`) is the row this brainstorm advances to `in-progress` at the SPEC commit. The split between 08.1 (admin-endpoints) and 08.2 (graceful-drain) was settled at parent-08 brainstorm time per ADR-0084 (per `DECISIONS.md` line 3166); this brainstorm does NOT re-relitigate the split. What it DOES settle is the 08.2-internal design surface — the drain-state machine shape, the SIGTERM-handler upgrade, the new admin endpoint, the `/ready` and `/server_info` extensions, and the `BEHAVIOR_CONTRACT.md` additions — that the 08.2 SPEC will then formalize.

### 1.1 What 08.2 delivers as a self-contained whole

08.2 lands graceful-drain semantics: the lifecycle discipline that moves envoy-go from "kill -TERM means hard exit" to "kill -TERM means stop accepting new connections, finish in-flight requests, then exit cleanly." The eight in-scope items per the sibling SPEC stub (`docs/envoy-go/phases/08.2-graceful-drain/README.md` §2):

1. **New `internal/drain/` package owning the drain-state machine.** A `Manager` type with three states: `LIVE` (initial), `DRAINING` (post-trigger; new connections rejected, in-flight allowed to complete), and an exit transition (when the in-flight count reaches zero or a drain timeout fires, whichever first).
2. **`cmd/envoy-go/main.go` SIGTERM-handler upgrade.** Replaces the current `<-ctx.Done()` + deferred `lm.Stop()` flow (per `cmd/envoy-go/main.go:170` in the worktree at HEAD `eb3babd`) with a flow that signals the drain manager, waits for drain-completion (or timeout), then proceeds to listener / cluster shutdown.
3. **`internal/listener.Manager.Drain` accessor.** Stop-accepting on listening sockets while leaving in-flight downstream connections to complete via their HCM filter chains.
4. **`internal/cluster.Manager.Drain` accessor.** Best-effort upstream connection close after drain timeout (or earlier if in-flight count reaches zero).
5. **`POST /drain_listeners` admin endpoint.** Mutating admin endpoint that triggers `Manager.Drain()` WITHOUT process exit — the operator-workflow form of drain, distinct from the SIGTERM-driven drain-then-exit form.
6. **`/ready` extension — DRAINING-state body.** The `/ready` handler at `internal/admin/admin.go:121` (`handleReady`) gains a third response branch beyond the existing LIVE / PRE_INITIALIZING split: when the drain manager is in `DRAINING`, return `503 Service Unavailable` with body `DRAINING\n` (or whatever Envoy emits — see §11.2). Partially supersedes ADR-0015's pre-init contract.
7. **`/server_info` `state` field — DRAINING transition.** The `deriveState` function at `internal/admin/serverinfo.go:65` returns `adminv3.ServerInfo_LIVE` post-MarkReady, `adminv3.ServerInfo_PRE_INITIALIZING` pre-MarkReady (per ADR-0088); 08.2 extends to return `adminv3.ServerInfo_DRAINING` when the drain manager is active. Per ADR-0088 consequence (c), the extension is **purely additive** and lands as an ADR-0088 amendment, not a supersession.
8. **`BEHAVIOR_CONTRACT.md` additions.** New `### /drain_listeners` subsection under the existing `## Admin API` umbrella (which 08.1's restructure landed at `BEHAVIOR_CONTRACT.md` per ADR-0052), plus extensions to `### /ready` (DRAINING-state body) and `### /server_info` (DRAINING-state transition), plus a NEW sibling `## Graceful drain` umbrella section covering drain-state-machine semantics independent of the admin API.

### 1.2 What 08.2 does NOT deliver (forward to §10)

The exhaustive deferral list lives in §10. The summary: hot restart / parent-child handoff, `POST /quitquitquit`, per-listener selective drain, configurable `drain_strategy` per-listener, configurable `drain_time_s` via bootstrap or admin, connection-level drain windows, and any drain-manager interaction with xDS are all out-of-scope per the sibling SPEC stub §4. None are blockers for closing the BOOTSTRAP_PROMPT.md §8 MVP trunk.

### 1.3 Phase-done as MVP-trunk closure

Per parent SPEC `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` §5: 08.2's phase-done commit closes BOTH ROADMAP row `08.2` AND parent ROADMAP row `08` (the "five+ closure pattern" inherited from 05/05.1/05.2 + 06/06.1/06.2 + 07/07.1/07.2). This means **08.2's phase-done commit is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit** — the last commit of the seeded MVP trunk. After 08.2 lands, STATE.md flips to `awaiting next planning` per the §5 lifecycle state machine; the next session brainstorms against `BOOTSTRAP_PROMPT.md` §9's family list to pick the next phase.

This shape constrains 08.2's scope discipline: anything 08.2 ships becomes a load-bearing primitive for the entire feature-family expansion that follows (HTTP filters family, network filters family, LB family, upstream robustness family, xDS family, observability family, runtime/hot-restart family, WASM family). The drain-state-machine API surface that 08.2 establishes will be amended (NOT superseded) by future phases — particularly the runtime/hot-restart family which BOOTSTRAP_PROMPT.md §9 explicitly anticipates as "graceful-drain semantics beyond phase 08's minimum." Designing the `internal/drain/` package's exported surface as a forward-extensible primitive is therefore a load-bearing concern (Decision 1).

### 1.4 Seed-stub alignment

The sibling SPEC stub `docs/envoy-go/phases/08.2-graceful-drain/README.md` (committed at the 08.1 SPEC commit `1f85b07`, kept read-only at this point per the stub's own §1 once a full SPEC supersedes) is the forward-looking seed. This brainstorm expands every §-bullet of the stub into a settled Decision (or an explicit deferral to SPEC time / empirical-pin time). The §3 list of "5–7 anticipated ADRs" in the stub is the seed for §9; §4's out-of-scope list is the seed for §10; §2's eight in-scope items are the seed for §3 (this brainstorm's surface inventory).

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

This section is the brainstorm's decision log. Each Decision states **what** is chosen, **why** that option vs. its alternatives, what **deferred-pin** obligations (if any) remain for SPEC-time empirical work, and what **ADR anchor** the SPEC author should expect. ADR numbering starts at **ADR-0091** (next-free; 08.1 closed at ADR-0090 per `DECISIONS.md` line 3538).

### 2.1 Drain-state machine shape *(Decision 1 → ADR-0091)*

**Decision:** Three states — `LIVE` (initial; the post-MarkReady steady state), `DRAINING` (post-trigger; new connections rejected, in-flight allowed to complete), `DRAINED` (terminal; no in-flight remaining; the SIGTERM-handler waits on a `Done()` signal indicating this transition has fired). The state machine is housed in a NEW `internal/drain/` package with a `Manager` type:

```go
package drain

type State uint8

const (
    StateLive State = iota
    StateDraining
    StateDrained
)

type Manager struct {
    state    atomic.Uint32  // holds State
    inflight atomic.Int64   // request-counted; HCM increments on request begin, decrements on terminal response
    done     chan struct{}  // closed when state transitions Draining → Drained
    timeout  time.Duration  // hardcoded MVP default per Decision 6
    once     sync.Once      // guards the Drain() trigger
}

func New(timeout time.Duration) *Manager
func (m *Manager) State() State
func (m *Manager) Drain()                      // idempotent; transitions Live → Draining; arms the Done() signal
func (m *Manager) Done() <-chan struct{}       // closes when Drained (or timeout fires)
func (m *Manager) Inc()                        // HCM-side hook on request begin
func (m *Manager) Dec()                        // HCM-side hook on terminal response
func (m *Manager) IsDraining() bool            // listener Accept loop fast-path check
```

**Rationale:** The three-state shape mirrors Envoy's `LIVE` / `DRAINING` / `DRAINED` semantics observable in `/server_info` (per ADR-0088 + `/server_info` v1.37.2 enum coverage settled at 08.1 SPEC §11.4) without inventing a new vocabulary. The fourth Envoy-defined enum value `INITIALIZING` is documented in `adminv3.ServerInfo_State` but unreachable in envoy-go's static-bootstrap-only model (per ADR-0088 + 08.1 SPEC §11.7); 08.2 does NOT extend `INITIALIZING` coverage. The fifth Envoy-defined value `PRE_INITIALIZING` IS modeled (already covered by `deriveState` returning it pre-MarkReady; 08.2 leaves that branch unchanged).

The `atomic.Uint32 + atomic.Int64 + chan struct{} + sync.Once` shape is the lock-free state-machine pattern envoy-go uses elsewhere (`*stats.Registry` per LBP-1 in 06.1; `s.ready atomic.Bool` per 01 in `internal/admin/admin.go:32`). No new mutex; the `done` channel close is the rendezvous point between the `Drain()` trigger and the SIGTERM-handler's `<-drainMgr.Done()` wait. The `inflight atomic.Int64` is the load-bearing counter: it must be incremented at the earliest point where rejection-during-drain would be a correctness bug (HCM `decodeHeaders` entry, BEFORE the filter chain runs) and decremented at the latest point where the response has been fully written (HCM `encodeHeaders` / `encodeData` final flush, AFTER the access-log entry has been emitted per phase 06.2's hooks).

**Why three states, not four (no `PRE_DRAINING` intermediate):** Envoy v1.37.2's `/server_info` enum does not name a `PRE_DRAINING` value (per 08.1 SPEC §11.4 verbatim scrape evidence at lines 738–797 — the four values are `LIVE`, `DRAINING`, `PRE_INITIALIZING`, `INITIALIZING`); introducing one in envoy-go would diverge from the canonical schema. The "draining-with-in-flight-still-completing" condition IS `DRAINING` per Envoy; `DRAINED` is observable only as the post-drain exit transition (which envoy-go signals via channel close, not via a public state-getter — the SIGTERM-handler is the only consumer of the `DRAINED` transition).

**Why a new `internal/drain/` package, not a method on `internal/listener.Manager` or `internal/admin.Server`:** The drain manager is consumed by FOUR boot-time-coupled actors — `cmd/envoy-go/main.go` (SIGTERM-handler waiting on `Done()`), `internal/admin.Server` (the `POST /drain_listeners` handler triggering `Drain()` + the `/ready` and `/server_info` handlers reading `State()`), `internal/listener.Manager` (the Accept loop fast-path checking `IsDraining()`), and `internal/filter/hcm` (the in-flight counter `Inc()`/`Dec()` hooks). Putting the type on any one of those four would force the other three to import it transitively; a fresh top-level package cleanly localizes the concern.

**Consequences:**
- The `Manager` is an LBP-1 fourth application (after `*stats.Registry` per 06.1 SPEC §5.4, `*HTTPRegistry` per ADR-0072, `*ListenerFilterRegistry` per ADR-0079, `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` threaded into `admin.New` per ADR-0085). The constructor pattern `drain.New(timeout time.Duration) *drain.Manager` is called once at `cmd/envoy-go/main.go` boot and threaded into `admin.New`, `listener.NewManager...`, and HCM filter-chain construction.
- ADR-0085's `admin.New` signature widens AGAIN (the LBP-1 fifth application — see §2.4).
- The `inflight` counter's Inc/Dec hook discipline becomes a hot-path concern in HCM and lives in §2.7's HCM-side wiring decision.

*(Decision 1 → ADR-0091)*

### 2.2 SIGTERM-vs-SIGINT semantics *(Decision 2 → ADR-0092)*

**Decision:** SIGTERM and SIGINT both trigger graceful-drain-then-exit. The `signal.NotifyContext` registration at `cmd/envoy-go/main.go:145` (`os.Interrupt, syscall.SIGTERM`) stays unchanged structurally; the body of the `<-ctx.Done()` block at line 170 is what changes. After `<-ctx.Done()` fires (either signal):

```go
<-ctx.Done()
drainMgr.Drain()                          // Live → Draining
select {
case <-drainMgr.Done():                   // Draining → Drained (in-flight reached zero)
case <-time.After(drainMgr.Timeout()):    // timeout fired; best-effort thereafter
}
// existing deferred-stop chain runs as the function unwinds:
//   defer lm.Stop()    (closes listening sockets)
//   defer admSrv.Close()
//   defer sinks-close
```

**Rationale:** Envoy v1.37.2's documented behavior (per BRAINSTORM-time consultation of upstream Envoy operator docs — to be empirically validated at SPEC time per §11.7) treats SIGTERM and SIGINT identically as drain-and-exit triggers. Pre-08.2 envoy-go conflates them via `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)`; preserving that conflation in 08.2 is the simpler and Envoy-faithful choice. A separate behavior for SIGINT (e.g., immediate-exit-without-drain) would require a second signal channel and operator-mode-flag plumbing not justified by any current operator workflow.

**Alternatives considered:**
- **(A) SIGTERM = drain; SIGINT = immediate-exit-without-drain.** Rejected: introduces an asymmetry not anchored by any Envoy-documented behavior; the operator workflow for "force kill the proxy NOW" is `kill -9` (SIGKILL), which the kernel handles uncatchable.
- **(B) Read a `--drain-on-sigterm` operator flag to enable/disable.** Rejected: drain-on-SIGTERM should be the default and only behavior; an operator who needs immediate exit uses SIGKILL. Adding a flag for "go back to the pre-08.2 hard-exit" surface contradicts the §1.3 MVP-trunk-closure goal.
- **(C) Keep the current `signal.NotifyContext` shape but add a second signal handler watching for a "second signal within N seconds = force exit" pattern (the `kill -TERM` twice-in-a-row escape hatch).** Deferred: a useful operator affordance but non-MVP; the drain-timeout (Decision 6) is the bounded-completion guarantee. A future `runtime/hot-restart` family phase may add the double-signal pattern.

**Consequences:**
- `cmd/envoy-go/main.go:145` registration is unchanged; the `<-ctx.Done()` body at line 170 changes per the decision sketch above.
- The drain-on-SIGINT-too behavior is mentioned in `BEHAVIOR_CONTRACT.md ## Graceful drain` umbrella prose (§6) so future readers know SIGINT is not a "leave fast" escape hatch.
- An empirical-pin obligation (§11.7) verifies Envoy v1.37.2 actually behaves the way this Decision asserts.

*(Decision 2 → ADR-0092)*

### 2.3 `POST /drain_listeners` contract *(Decision 3 → ADR-0093)*

**Decision:** Body shape, idempotency, and the `?graceful=true` query-param are settled as follows (the verbatim response body is an empirical-pin obligation — see §11.1):

- **Method:** POST. GET is also accepted (Envoy parity per the no-method-discrimination posture at ADR-0090; verified empirically — §11.4); POST is the canonical operator method documented in `BEHAVIOR_CONTRACT.md`.
- **Response status:** 200 OK on the first call; 200 OK on every subsequent call (idempotent — no transition-error response).
- **Response body:** TBD verbatim (see §11.1). The two candidate shapes are `OK\n` (text/plain) or `{}\n` (application/json) — Envoy v1.37.2 emits one verbatim shape; envoy-go must match it byte-for-byte under the differential equivalence claim.
- **Behavior:** triggers `drainMgr.Drain()` synchronously; returns 200 BEFORE drain completes (the call is fire-and-forget from the operator's perspective; the operator polls `/ready` or `/server_info` to observe the drain progress). The handler does NOT block on `<-drainMgr.Done()`.
- **Idempotency:** subsequent calls observe `drainMgr.State() == DRAINING` and return 200 with the same body without invoking `Drain()` a second time. The `sync.Once` guard inside `Manager.Drain()` enforces single-fire semantics.
- **`?graceful=true` query-param:** Envoy supports `?graceful=true` to switch from the default (which Envoy calls "modify_only" in some doc-strings — TBD verbatim against v1.37.2) to a strictly-graceful mode. **08.2 ignores the query-param** — the envoy-go drain is always graceful (per Decision 1's three-state machine; there is no non-graceful immediate-stop variant in MVP). The query-param is silently accepted (per ADR-0041's silent-ignore precedent) and the handler emits the standard graceful-drain trigger regardless.
- **NO process exit on this endpoint.** Per the sibling SPEC stub §2 item 5 verbatim: "the mutating admin endpoint that triggers `Manager.Drain()` WITHOUT process exit." The operator-workflow distinction from SIGTERM-driven drain is that `POST /drain_listeners` lets the proxy continue running in DRAINING state indefinitely (until the operator separately issues SIGTERM, or kills the process); the proxy will never auto-exit from a `/drain_listeners` trigger.

**Rationale:** The fire-and-forget shape matches Envoy's documented `/drain_listeners` semantics — the endpoint is a state-transition trigger, not a synchronous-drain-completion barrier. Operators who want "drain and wait" build that out of "POST /drain_listeners + poll /ready until 503 DRAINING + poll until in-flight=0" externally. Idempotency is the natural contract for any state-transition endpoint: a successful retry is indistinguishable from a successful first call. The `?graceful=true` silent-ignore is the cleanest approach because (a) envoy-go's drain is always graceful by construction, (b) introducing two-mode drain mid-MVP would inflate the test surface, and (c) ADR-0041 establishes the silent-ignore-of-known-Envoy-fields precedent.

**Alternatives considered:**
- **(A) Return 202 Accepted instead of 200 OK** to signal "queued for drain, not yet complete." Rejected: Envoy emits 200 (empirical-pin obligation §11.1 will confirm); diverging would break the differential equivalence claim.
- **(B) Block the response until drain completes** (return 200 only when `drainMgr.Done()` fires). Rejected: Envoy's behavior is fire-and-forget; the long-poll alternative would couple the admin client's lifetime to the drain timeout, surfacing test-rigging cliff-edges.
- **(C) Reject `?graceful=true` with 400 Bad Request as an unsupported query-param.** Rejected: ADR-0041's silent-ignore precedent says envoy-go matches Envoy's tolerance for query-param surface variation; 400ing would diverge.
- **(D) Implement a `?graceful=true` distinct path with different drain semantics.** Deferred: 08.2 has only the graceful path; a future phase may revisit if a non-graceful immediate-stop variant proves necessary.

**Deferred to SPEC author / empirical-pin obligation:**
- §11.1: response body verbatim against Envoy v1.37.2 (`OK\n` vs `{}\n` vs other).
- §11.4: method-discrimination on `/drain_listeners` (does Envoy reject GET, or accept any method like the read-only endpoints?).
- §11.6: response headers (Content-Type, framing — does Envoy chunk or set Content-Length?).

**Consequences:**
- A new file `internal/admin/drain.go` houses `handleDrainListeners`. Registered on the existing `*http.ServeMux` per ADR-0085's mux-extension pattern (`mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` added to `admin.Server.Start()` per `internal/admin/admin.go:78`).
- `internal/admin.Server` gains a new field `dm *drain.Manager` per the LBP-1 fifth-application constructor widening (Decision 4).
- BEHAVIOR_CONTRACT.md gains a new `### /drain_listeners` subsection under the existing `## Admin API` umbrella (per §6) plus a new equivalence-matrix row.

*(Decision 3 → ADR-0093)*

### 2.4 Constructor-widening pattern (LBP-1 fifth application) *(Decision 4 → no separate ADR; consolidated into ADR-0091)*

**Decision:** `admin.New` widens to take a sixth parameter — `dm *drain.Manager`. The signature transitions from the 08.1 form `New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server` (per `internal/admin/admin.go:51` at HEAD `eb3babd`) to the 08.2 form:

```go
func New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap,
         cm *cluster.Manager, lm *listener.Manager, dm *drain.Manager) *Server
```

Similarly:
- `listener.NewManagerWithBaseDirAndAllowH2C` widens to take a `dm *drain.Manager` parameter (the Accept loop fast-path checks `dm.IsDraining()` to reject new connections — see §2.5).
- The HCM filter-chain construction surface (currently called from inside `listener.NewManager...` — per `internal/listener/manager.go:122` style) widens to thread `dm *drain.Manager` so HCM `decodeHeaders`/`encodeData` can call `dm.Inc()`/`dm.Dec()` (see §2.7).
- `cluster.NewManagerWithBaseDir` does NOT widen — the cluster manager's `Drain()` method is a public-API call invoked from `cmd/envoy-go/main.go` after `<-drainMgr.Done()` fires; it does not need the drain manager threaded in (the drain manager would ask the cluster manager to close, not the other way around).

**Rationale:** ADR-0085 settled the LBP-1 explicit-threading discipline ("no package globals; every dependency is threaded explicitly through the constructor"). The 08.2 widening is symmetric: `*drain.Manager` is allocated once at boot in `cmd/envoy-go/main.go` after `bootstrap.Load` and BEFORE `cluster.NewManager...` (the drain manager has no dependencies; it can be the first allocation). It is then threaded into every actor that needs it — admin, listener, HCM. Test code that does not exercise drain semantics may pass `nil` (the same nil-tolerance pattern ADR-0085 established for the four 08.1 dependencies).

The fifth-application status is recorded in ADR-0091 (the drain-state-machine ADR) rather than getting its own ADR — the "fifth application" is mechanical at this point; the discipline is settled by ADR-0085, and a separate ADR for each application would inflate the ADR log.

**Alternatives considered:**
- **(A) Use a package-global `drain.Singleton *drain.Manager` allocated in `init()`.** Rejected: violates ADR-0085's explicit-threading discipline; package globals are forbidden in this codebase per LBP-1.
- **(B) Plumb the drain manager through a `*context.Context` value.** Rejected: context-value plumbing is an anti-pattern for non-request-scoped dependencies (per Go community guidance); the drain manager outlives any single request's context.
- **(C) Allocate the drain manager inside `internal/admin.Server` and let admin own it.** Rejected: HCM and listener also need it; making admin the owner would force admin to expose a getter and the other two to import admin transitively.

**Consequences:**
- `cmd/envoy-go/main.go` boot wiring grows: `drainMgr := drain.New(30 * time.Second)` (timeout per Decision 6) before `cluster.NewManager...`; threaded into `listener.NewManager...` and `admin.New`. The signal-handler block at line 170 calls `drainMgr.Drain()` then `<-drainMgr.Done()`.
- `internal/admin.Server` gains a `dm *drain.Manager` field. `handleDrainListeners` (new, per Decision 3) calls `s.dm.Drain()`. `handleReady` (existing, modified per Decision 5) reads `s.dm.State()`. `handleServerInfo` (existing, modified per Decision 7 via `deriveState`) reads `s.dm.State()`.
- All tests that construct `*admin.Server` either thread a non-nil `*drain.Manager` (when exercising drain paths) or pass `nil` (when not — the handlers nil-check defensively, mirroring the ADR-0085 nil-tolerance pattern).

*(Decision 4 → consolidated into ADR-0091; no separate ADR)*

### 2.5 Listener stop-accepting contract *(Decision 5 → ADR-0094)*

**Decision:** `listener.Manager.Drain()` is the public method that triggers stop-accepting on every bound listener. Two-step shape:

1. **Per-listener Accept-loop fast-path:** during `Drain()`, every `runtime`'s Accept loop (per `internal/listener/manager.go` Accept loop) consults a per-Manager `dm *drain.Manager` and on `dm.IsDraining()` returning true, REJECTS any newly-Accept-ed connection by closing it immediately. The connection is accepted (because `net.Listener.Accept` returns the `net.Conn` before the loop body runs), then the per-listener filter chain is NOT invoked; instead the conn is closed. This matches Envoy's "accept-and-immediately-close" pattern observed in operator workflows (empirical-pin obligation §11.5 settles whether Envoy actually does accept-and-close vs. listener-socket-close).
2. **Per-listener listener-socket close:** the existing `Stop()` method (per `internal/listener/manager.go:941` — `for _, rt := range m.runtimes { _ = rt.netLn.Close() }`) is invoked AFTER drain completes (i.e., after `<-drainMgr.Done()` fires in `cmd/envoy-go/main.go`). 08.2 does NOT change `Stop()`; it adds `Drain()` as a NEW method and keeps `Stop()` as the post-drain teardown.

**Rationale:** The two-step (drain-trigger + post-drain-stop) shape preserves the existing `Stop()` semantics unchanged; new code paths are additive. The "accept-and-immediately-close during DRAINING" pattern (rather than "close the listening socket immediately on Drain()") matches what operators expect — a connection that arrives during drain should fail fast, not get queued behind a half-shut socket. The empirical-pin obligation §11.5 settles whether this matches Envoy v1.37.2 verbatim.

The per-Manager threading of `dm *drain.Manager` is the LBP-1 fifth-application path (see Decision 4). The Accept loop's fast-path check is `dm.IsDraining()` — a single atomic load against the `drain.Manager.state atomic.Uint32`, which is lock-free and adds <10ns per Accept call (the load is cache-warm under the typical request-rate).

**Why not close the listening socket on `Drain()` directly:** Closing the socket would unblock the Accept loop with `net.ErrClosed`; the loop would exit; subsequent connections would get TCP RST from the kernel (no listener bound on the port). This is observably different from "accept-and-immediately-close" — the latter shows up in client telemetry as "connection accepted then refused," while the former shows up as "connection refused at TCP layer." Empirical-pin §11.5 settles which Envoy emits.

**Alternatives considered:**
- **(A) Close the listening socket immediately on Drain().** Deferred to §11.5 empirical evidence. If Envoy actually does close-the-socket-on-drain, this becomes the chosen path.
- **(B) Add a `drainDeadline time.Time` field to each `runtime` and have the Accept loop reject after the deadline.** Rejected: the timeout is per-Manager (one drain timeout for the whole proxy), not per-listener; introducing per-listener deadlines duplicates state. Decision 6 confirms a single proxy-wide drain timeout.
- **(C) Have `internal/admin.Server.handleDrainListeners` call `lm.Drain()` directly.** Adopted-as-mechanism: the drain handler at `internal/admin/drain.go` calls `drainMgr.Drain()` which transitions state; the listener Accept loop independently observes the state change via its own `dm *drain.Manager` reference. This decouples the admin handler from the listener manager (the admin handler doesn't need to import listener).

**Deferred to SPEC author / empirical-pin obligation:**
- §11.5: does Envoy close the listening socket or accept-and-immediately-close during drain?
- §11.5: HTTP/2 GOAWAY frame timing during drain (does Envoy emit GOAWAY on existing H2 connections at drain trigger, or only when each connection's idle window opens?).

**Consequences:**
- `internal/listener/manager.go` gains a `(m *Manager) Drain()` method (~5 LoC; the Accept-loop check is a single atomic-load added to the loop body, not in `Drain()` itself).
- The `Drain()` method is idempotent (it just delegates to the drain manager's `IsDraining()` semantics).
- Existing in-flight downstream connections are NOT torn down by `Drain()`; they continue running their HCM filter chains to completion. The `dm.Inc()` / `dm.Dec()` HCM-side hooks (Decision 7) are what signal completion to the drain manager.

*(Decision 5 → ADR-0094)*

### 2.6 Drain timeout default *(Decision 6 → ADR-0095)*

**Decision:** Hardcoded MVP default `30 * time.Second`. NOT operator-configurable in 08.2. Operator-knob deferred to a future hardening phase (likely the runtime/hot-restart family that BOOTSTRAP_PROMPT.md §9 anticipates).

**Rationale:** Envoy v1.37.2's documented default is 600s (10 minutes; per the empirical-pin obligation §11.7 which verifies this against v1.37.2 — the upstream `command_line_options.drain_time` field at 08.1 SPEC §11.4 line 760 emits `"drain_time": "600s"` confirming the upstream default). 600s is too long for an MVP test suite (the 08.2 differential fixture `0010-graceful-drain` would block CI for 10 minutes if the in-flight count never decremented). 30s is:
- Long enough to drain any reasonable-shape in-flight request (HCM body-buffer cap is 8KB per ADR-0076; even a slow client at 1KB/s drains in ~10s).
- Short enough that a stuck-drain failure mode (e.g., a streaming request that never closes) does not hang the test.
- Mathematically matches the existing `httpSrv.WriteTimeout = 30 * time.Second` widened in 08.1 per `internal/admin/admin.go:88` (planner-time decision 2 of 08.1) — no new timeout-budget reasoning needed; 30s is the established envoy-go MVP timeout shape.

The differential equivalence claim does NOT require envoy-go's drain timeout to match Envoy's 600s default. The drain-completion signal (via `drainMgr.Done()` channel close) IS observable, but the drain-timeout-fired branch is best-effort in both implementations. The fixture `0010-graceful-drain` driver completes its in-flight request in <1s, so neither implementation's timeout is exercised under the equivalence claim.

**Alternatives considered:**
- **(A) Match Envoy's 600s default verbatim.** Rejected: makes test runs intolerable. The 30s envoy-go MVP value is documented as a deliberate divergence in `BEHAVIOR_CONTRACT.md ## Graceful drain`; the equivalence claim is over the drain BEHAVIOR (state transitions, in-flight completion, new-connection rejection), not the timeout VALUE.
- **(B) Make the timeout operator-configurable via a CLI flag (`--drain-timeout 30s`).** Deferred: operator-knob is non-MVP. The hardcoded value is sufficient for the §1.3 MVP-trunk-closure goal; a future operator-affordances phase may add the flag.
- **(C) Make the timeout operator-configurable via the bootstrap proto (e.g., a `drain_time` field on the admin section).** Rejected: the bootstrap proto's `Admin` message at v1.37.2 has no `drain_time` field; introducing an envoy-go-specific bootstrap field would diverge from Envoy's schema. The v1.37.2 `command_line_options.drain_time` field at SPEC §11.4 line 760 IS the upstream surface; adding it would require a CLI flag (see (B)) or a synthetic bootstrap field.
- **(D) Use a shorter timeout (e.g., 5s) to match the `httpSrv.ReadTimeout`.** Rejected: 5s is too aggressive for streaming-response in-flight requests; 30s gives a comfortable margin without inflating CI cost.

**Consequences:**
- `cmd/envoy-go/main.go` boot wiring includes `drainMgr := drain.New(30 * time.Second)`. The literal 30s lives here (no constant in `internal/drain/` package; the value is provided BY the caller, not OWNED by the package — this lets test code construct `drain.New(10 * time.Millisecond)` for fast-path tests).
- `BEHAVIOR_CONTRACT.md ## Graceful drain` documents the 30s value as a deliberate divergence from Envoy's 600s default; the equivalence claim is over behavior, not timeout magnitude.
- Future hardening phase that adds the operator-knob amends this ADR (additively); supersession not required.

*(Decision 6 → ADR-0095)*

### 2.7 In-flight-completion discipline (HCM hook semantics) *(Decision 7 → ADR-0096)*

**Decision:** HCM `decodeHeaders` increments the drain manager's `inflight` counter at request-begin (BEFORE the filter chain runs); HCM `encodeData` (or equivalent terminal-flush hook) decrements at request-end (AFTER access-log emission per phase 06.2's hooks). Specifically:

```go
// internal/filter/hcm/hcm.go (sketch — actual code lands in 08.2 PLAN)
func (h *HCM) decodeHeaders(stream *Stream) decoderControl {
    if h.dm != nil {
        h.dm.Inc()
        stream.markedInflight = true
    }
    // ... existing decode-chain dispatch ...
}

func (h *HCM) encodeFinalize(stream *Stream) {
    // ... existing access-log emit per phase 06.2 ...
    if h.dm != nil && stream.markedInflight {
        h.dm.Dec()
        stream.markedInflight = false
    }
}
```

The `markedInflight` flag on `Stream` ensures the Inc/Dec are paired exactly once per request even if the request fails mid-chain (e.g., decoder error → sendLocalReply path per ADR-0075 also runs `encodeFinalize`).

For **TCP proxy** flows (no HCM, no filter chain — direct `internal/filter/tcpproxy.Filter` dispatch per phase 02): the in-flight discipline is per-connection rather than per-request. The TCP proxy `OnNewConnection` hook calls `dm.Inc()`; `OnConnectionClose` calls `dm.Dec()`. Connection-scoped Inc/Dec is correct because TCP proxy has no concept of "request" within the connection.

**Rationale:** Per-request Inc/Dec is the granularity Envoy uses for HTTP-level drain (per BRAINSTORM-time consultation of upstream Envoy operator docs; empirical-pin obligation §11.3 verifies). The `markedInflight` flag handles the all-paths-converge-to-encodeFinalize discipline that ADR-0075 established for sendLocalReply. The per-connection Inc/Dec for TCP proxy is the correct simplification — TCP proxy has no per-request granularity to model. The drain manager's `Done()` channel closes when `inflight.Load() == 0`, so the channel close signal is sound regardless of whether the inflight count was tracking requests or connections.

**Alternatives considered:**
- **(A) Increment at listener Accept (per-connection granularity for ALL listeners, including HCM-bearing ones).** Rejected: too coarse; HTTP/1.1 keep-alive connections would block drain indefinitely (a long-lived TCP connection with one in-flight request would be counted as 1 in-flight forever instead of decrementing on the request's completion). Per-request is the correct granularity for HCM.
- **(B) Increment at each filter's onDecodeHeaders (per-filter granularity).** Rejected: violates the Inc/Dec balance — filters can short-circuit (per ADR-0071 iteration protocol), which would leave an unbalanced Inc.
- **(C) Track inflight as a sync.WaitGroup instead of an atomic counter.** Rejected: WaitGroup's `Wait()` blocks; the drain manager needs a `Done()` channel that the SIGTERM-handler can `select` on alongside the timeout. atomic.Int64 + chan struct{} achieves this naturally.
- **(D) Use the existing 06.1 `*stats.Counter` instances (e.g., `cluster.<name>.upstream_rq_active`) as the in-flight signal.** Rejected: the Prometheus stats counters are per-cluster; the drain inflight is per-proxy. Different aggregation level.

**Deferred to SPEC author / empirical-pin obligation:**
- §11.3: does Envoy emit a `Connection: close` header on in-flight HTTP/1.1 responses during drain (so the connection is closed after the response, even if it was a keep-alive connection)? envoy-go's HCM may need to mirror this.
- §11.3: does Envoy emit a GOAWAY frame on existing HTTP/2 connections at drain trigger? If yes, when (immediately, or after the current stream completes)?

**Consequences:**
- `internal/filter/hcm/hcm.go` (or equivalent) gains the Inc/Dec pair around the request lifecycle. Net delta ~10 LoC + the `markedInflight bool` field on `Stream`.
- `internal/filter/tcpproxy/filter.go` gains the per-connection Inc/Dec (~6 LoC).
- The HCM and TCP proxy constructors widen to take a `dm *drain.Manager` parameter (LBP-1 fifth-application threading per Decision 4).
- The TCP proxy's per-connection Inc and HCM's per-request Inc co-exist on the same `drain.Manager` — the inflight counter is the union (a bare-TCP listener increments per-conn; an HCM listener increments per-req; both decrement appropriately). This is correct because both dispositions accurately reflect "work the proxy must finish before exit."

*(Decision 7 → ADR-0096)*

### 2.8 Cluster-pool drain contract *(Decision 8 → ADR-0096 amendment, NOT separate ADR)*

**Decision:** `cluster.Manager.Drain()` is a public method that closes upstream connection pools after the drain manager fires. The method is best-effort — it walks the per-cluster `*Cluster` instances and closes each cluster's connection pool (HTTP/1.1 keep-alive pool, HTTP/2 ClientConn instances per phase 05.2, TLS connections per phase 03). It does NOT wait for in-flight upstream requests to complete (those are tracked by the HCM Inc/Dec discipline of Decision 7, which already guarantees that no in-flight downstream request is pending — therefore no in-flight upstream request can be pending either).

**Call ordering in `cmd/envoy-go/main.go`:**

```go
<-ctx.Done()                              // SIGTERM/SIGINT received
drainMgr.Drain()                          // Live → Draining; new conns rejected by listener
select {
case <-drainMgr.Done():                   // in-flight reached 0 OR timeout fired
case <-time.After(drainMgr.Timeout()):
}
// At this point: no in-flight requests; safe to teardown.
// Defers run in LIFO order; explicit ordering for the new drain step:
cm.Drain()                                // close upstream pools
// existing deferred chain handles the rest:
//   defer lm.Stop()
//   defer admSrv.Close()
//   defer sinks-close
```

`cm.Drain()` is invoked explicitly in `main.go` after the `<-drainMgr.Done()` rendezvous, BEFORE the deferred stop chain. This places the upstream-pool close inside the drain window (so it can close upstream connections in parallel with the listener teardown) while keeping the existing defer chain intact.

**Rationale:** Closing upstream connection pools is a best-effort cleanup step — the drain manager's `inflight` counter (Decision 7) already guarantees no work is in-flight when `Done()` fires, so the upstream pools have no pending operations. Closing them releases socket file descriptors for cleanest shutdown but is not required for correctness (Go's runtime will close TCP sockets on process exit regardless).

The Decision lives under ADR-0096 (Decision 7) rather than getting its own ADR because the upstream-side discipline is the natural mirror of the downstream-side discipline; one ADR captures both.

**Alternatives considered:**
- **(A) Make `cluster.Manager` self-trigger Drain on observing the drain manager's state change** (e.g., via a goroutine watching `<-drainMgr.Done()`). Rejected: introduces a long-lived goroutine that lives only to wait for one channel; the explicit-call shape from `cmd/envoy-go/main.go` is cleaner.
- **(B) Skip cluster-side drain entirely** (let the OS clean up sockets at process exit). Rejected: best-effort cleanup is good hygiene; the cost is ~5 LoC in `internal/cluster/manager.go` and a few extra microseconds at exit.

**Consequences:**
- `internal/cluster/manager.go` gains a `(m *Manager) Drain()` method. ~10 LoC. Walks `m.clusters map[string]*Cluster` and calls `c.closePool()` (a per-cluster method that 08.2 also adds — closing HTTP/1.1 keepalive pool, HTTP/2 ClientConns, TLS connections).
- `cmd/envoy-go/main.go` SIGTERM-handler block gains the explicit `cm.Drain()` call after `<-drainMgr.Done()`.

*(Decision 8 → ADR-0096 amendment; no separate ADR)*

### 2.9 `/ready` DRAINING-state body extension *(Decision 9 → ADR-0097, partially supersedes ADR-0015)*

**Decision:** When `drainMgr.State() == DRAINING`, the `/ready` handler returns `503 Service Unavailable` with body verbatim per §11.2 empirical-pin obligation. The two candidate body shapes are `DRAINING\n` (mirror of `LIVE\n` and `PRE_INITIALIZING\n`) or `Draining\n` (sentence-case as some Envoy responses use). Verbatim TBD.

The handler's branching shape becomes (sketch — body shapes pinned at §11.2):

```go
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
    h := w.Header()
    h.Set("Content-Type", "text/plain; charset=UTF-8")
    h.Set("Cache-Control", "no-cache, max-age=0")
    h.Set("X-Content-Type-Options", "nosniff")
    h.Set("Server", "envoy")

    // 08.2 NEW: drain check FIRST (DRAINING wins over PRE_INITIALIZING and LIVE).
    if s.dm != nil && s.dm.State() == drain.StateDraining {
        body := []byte("DRAINING\n")  // verbatim TBD §11.2
        h.Set("Content-Length", strconv.Itoa(len(body)))
        w.WriteHeader(http.StatusServiceUnavailable)
        _, _ = w.Write(body)
        return
    }

    // existing: pre-init branch
    if !s.ready.Load() { /* PRE_INITIALIZING\n + 503 */ }
    // existing: ready branch (LIVE\n + 200)
}
```

**Rationale:** The DRAINING check goes FIRST (before the ready check) because once drain has fired, the proxy should not re-emit `LIVE` even if MarkReady returns true — `DRAINING` supersedes `LIVE` for /ready's purposes (the operator shutting down the proxy needs the load balancer to stop sending traffic). The 503 status matches the existing PRE_INITIALIZING branch (Envoy distinguishes "not yet ready" and "draining" by body, not by status code — both return 503 per Envoy docs; empirical-pin §11.2 confirms).

**Partially supersedes ADR-0015** (the pre-init contract for /ready). ADR-0015's two-state coverage (LIVE + PRE_INITIALIZING) extends to three-state (LIVE + PRE_INITIALIZING + DRAINING). The supersession is partial — ADR-0015's pre-init body and pre-init status are preserved verbatim; ADR-0097 adds the DRAINING-state body and asserts the precedence (DRAINING > PRE_INITIALIZING > LIVE for /ready response selection).

**Alternatives considered:**
- **(A) Return 200 OK with body `DRAINING\n` instead of 503.** Rejected: 503 is the operator signal that "this instance should not be load-balanced to"; emitting 200 would defeat the operator workflow. Empirical-pin §11.2 will confirm Envoy emits 503 for DRAINING.
- **(B) Check ready FIRST, then drain.** Rejected: pre-init proxies are not draining (Drain has not fired yet); and post-init proxies should report DRAINING when drain has fired even though `s.ready.Load()` returns true. The DRAINING-first ordering captures both correctly.
- **(C) Emit a richer body** (e.g., JSON with timestamp + drain progress). Rejected: Envoy emits a single-line text body; envoy-go matches.

**Deferred to SPEC author / empirical-pin obligation:**
- §11.2: verbatim DRAINING body shape (`DRAINING\n` vs `Draining\n` vs other).
- §11.2: status code (503 vs other).
- §11.2: header set (any DRAINING-specific headers? `Connection: close`?).
- §11.6: framing (chunked vs Content-Length for DRAINING response).

**Consequences:**
- `internal/admin/admin.go` `handleReady` body grows by ~10 LoC (the new DRAINING branch).
- `BEHAVIOR_CONTRACT.md ### /ready` subsection (under `## Admin API` umbrella) gains a third-state-body block + an empirical-evidence pointer to the 08.2 SPEC §11.2.
- ADR-0015 is partially superseded by ADR-0097 (the pre-init contract still holds for the LIVE / PRE_INITIALIZING branches; DRAINING is a new branch that ADR-0015 did not cover).

*(Decision 9 → ADR-0097, partially supersedes ADR-0015)*

### 2.10 `/server_info` DRAINING transition timing *(Decision 10 → ADR-0098, amends ADR-0088)*

**Decision:** `/server_info`'s `state` field returns `"DRAINING"` (the protojson-rendered form of `adminv3.ServerInfo_DRAINING`) IMMEDIATELY when `drainMgr.State() == DRAINING`. Specifically: the moment `drainMgr.Drain()` is called (whether from the `POST /drain_listeners` handler or from the SIGTERM-handler), the state machine transitions LIVE → DRAINING via `atomic.CompareAndSwap`; the next `/server_info` scrape observes `state: "DRAINING"`.

The `deriveState` function at `internal/admin/serverinfo.go:65` extends:

```go
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

The DRAINING check is FIRST (matching Decision 9's /ready precedence — DRAINING > PRE_INITIALIZING > LIVE).

**Rationale:** Immediate transition is the simpler and Envoy-faithful choice. There is no "pre-DRAINING" intermediate state in Envoy's enum (per 08.1 SPEC §11.4 verbatim scrape — the four enum values are LIVE, DRAINING, PRE_INITIALIZING, INITIALIZING). The transition is atomic with the `Drain()` call.

**Amends ADR-0088** (`/server_info` body shape). ADR-0088 consequence (c) explicitly anticipated: "Phase 08.2's drain implementation extends the state enum coverage to LIVE + PRE_INITIALIZING + DRAINING by adding a third atomic flag (or by extending s.ready semantics — 08.2's PLAN settles the choice) and amending `deriveState` to return `adminv3.ServerInfo_DRAINING` when the drain flag is set. The amendment is purely additive; no other field changes. The ADR-0088 amendment will record the addition without superseding this ADR." Decision 10 IS that amendment. ADR-0098 is the separate amendment record (the on-disk ADR amendment is appended to ADR-0088 per the in-place-edit-of-ADR pattern that ADR-0089 consequence (b) authorizes). Per ADR-0088's anti-fragmentation guidance, this is an amendment, not a supersession.

The choice between "third atomic flag on Server" vs. "extend s.ready semantics" (ADR-0088 consequence (c)) is settled here: **third atomic, but in `*drain.Manager`, not on `*Server`** — the drain state lives in the drain manager (single source of truth), and `deriveState` consults both `s.ready` and `s.dm.State()`. This avoids duplicating drain state on the Server struct.

**Alternatives considered:**
- **(A) Delay the DRAINING transition until the listener stop-accepting completes** (i.e., return LIVE for some grace window after Drain() until all listening sockets are confirmed not-accepting). Rejected: Envoy transitions immediately; envoy-go matches. The grace window would introduce a race between the admin handler observing `dm.State()` and the listener Accept loop observing `dm.IsDraining()` — both are atomic loads against the same field, so the transition is observably instant.
- **(B) Add a fourth state `PRE_DRAINING` to envoy-go's enum to model the "Drain() called but not yet propagated to listeners" intermediate.** Rejected: Envoy's enum has no PRE_DRAINING; introducing one would diverge.

**Consequences:**
- `internal/admin/serverinfo.go:65` `deriveState` signature extends to take `*drain.Manager`. The call site at `internal/admin/serverinfo.go:50` (`State: deriveState(&s.ready)`) updates to `State: deriveState(&s.ready, s.dm)`.
- ADR-0088's enum-coverage table extends from `{LIVE, PRE_INITIALIZING}` to `{LIVE, PRE_INITIALIZING, DRAINING}`. `INITIALIZING` remains unreachable.
- `BEHAVIOR_CONTRACT.md ### /server_info` subsection's state-enum coverage block gains the DRAINING entry.
- The differential equivalence claim for `/server_info` extends to assert `state: "DRAINING"` byte-equality when both proxies are in DRAINING (the existing 08.1 fixture only asserts LIVE; the 08.2 fixture `0010-graceful-drain` extends).

*(Decision 10 → ADR-0098, amends ADR-0088)*

### 2.11 Hot-restart deferral *(Decision 11 → ADR-0099)*

**Decision:** Hot restart / parent-child handoff is OUT OF SCOPE for 08.2. envoy-go's drain is one-process scope only. Future work lives in BOOTSTRAP_PROMPT.md §9's "Runtime + hot restart family" — which the §9 prose explicitly anticipates with the line "graceful-drain semantics beyond phase 08's minimum."

**Rationale:** Hot restart requires socket-passing between parent and child processes (UNIX SCM_RIGHTS file-descriptor transfer), shared-memory state for the existing-connection table, parent-shutdown-time orchestration, and a custom signal protocol. These are all multi-phase deliverables; bundling any of them into 08.2 would inflate the phase past the ADR-0045 split threshold (which is precisely why 08 was already split into 08.1 + 08.2 per ADR-0084). The MVP-trunk closure (per §1.3) does NOT require hot restart; the operator workflow that 08.2 unblocks is "rolling restart with external load balancer" (the operator drains the proxy, the LB stops sending traffic, the operator deploys the new version, the new version starts; total downtime per pod is the drain timeout).

**Alternatives considered:**
- **(A) Implement minimal hot restart (just SCM_RIGHTS socket transfer; defer shared-memory state and parent-shutdown orchestration).** Rejected: even the minimal slice is ~500 LoC and requires a custom CLI flag (`--restart-epoch`); it doubles the 08.2 surface and crosses the ADR-0045 split threshold.
- **(B) Add a stub `--restart-epoch` flag that no-ops** (forward-compat for a future hot-restart phase). Rejected: stubs that no-op are an anti-pattern per BOOTSTRAP_PROMPT.md §6.3 — they are a vague "TODO: extend later" without a tested code path. The future hot-restart phase will add the flag.

**Consequences:**
- `BEHAVIOR_CONTRACT.md ## Graceful drain` umbrella section's "Does not yet apply to" sub-block enumerates "hot restart / parent-child handoff" with a forward pointer to the runtime/hot-restart family.
- ADR-0099 records the disposition. The ADR is forward-only (no supersession); a future hot-restart phase adds an ADR-0100+ that introduces the surface.

*(Decision 11 → ADR-0099)*

### 2.12 Single-fixture vs. multi-fixture *(Decision 12 → no separate ADR; consolidated into ADR-0093)*

**Decision:** ONE differential fixture, `0010-graceful-drain`. Fixture shape per §7.

**Rationale:** The 06.1 single-fixture precedent (`0005-prometheus-stats`) and the 08.1 single-fixture precedent (`0009-admin-config-dump`) show that one well-designed fixture can exercise multiple equivalence claims simultaneously. The 08.2 surface — `POST /drain_listeners`, `/ready` DRAINING body, `/server_info` DRAINING state, listener stop-accepting, in-flight-completion — is best exercised in one driver that orchestrates all the state transitions in a single test scenario.

**Alternatives considered:**
- **(A) Two fixtures: `0010-drain-via-admin` (POST /drain_listeners flow) and `0011-drain-via-sigterm` (SIGTERM flow).** Rejected: doubles the fixture-build cost; the SIGTERM flow shares 90% of the driver code with the admin flow (the only difference is the trigger). One fixture exercises both via a `triggerType` parameter.
- **(B) Three fixtures: above plus `0012-drain-with-in-flight-h2`. Rejected: HTTP/2 in-flight-during-drain is a special case the single fixture covers via an explicit H2 driver path (the single fixture has both H1 and H2 driver paths under one bootstrap).

**Consequences:**
- Single fixture, two driver paths (admin-trigger + SIGTERM-trigger). See §7.
- The fuzzer count post-08.2 rests at 10 fuzzers (the 08.1-introduced FuzzConfigDumpFormat plus the existing 9). 08.2 may add ONE drain-state-machine fuzzer (`FuzzDrainTransitions`) — see §8.

*(Decision 12 → consolidated into ADR-0093)*

---

## 3. Surface inventory (08.2 deliverable list)

The 08.2 sub-phase ships one new package, modifies five existing files, lands one new admin handler file, and adds one new differential fixture + BEHAVIOR_CONTRACT additions + ADRs.

### 3.1 New production code

```
internal/drain/                                     -- NEW PACKAGE
  doc.go                                            -- package doc
  manager.go                                        -- Manager type + State enum + New + State + Drain
                                                       + Done + Inc + Dec + IsDraining + Timeout
                                                       (~120 LoC; lock-free state machine per Decision 1)
  manager_test.go                                   -- unit tests for state transitions, Inc/Dec balance,
                                                       Done() rendezvous, idempotent Drain
                                                       (~200 LoC)
  fuzz_test.go                                      -- FuzzDrainTransitions (~60 LoC; 30s budget per ADR-0018)
                                                       NOTE: §8 may downgrade to "no new fuzzer" if the
                                                       state machine is too narrow to fuzz meaningfully

internal/admin/                                     -- existing; expanded
  drain.go                                          -- NEW: handleDrainListeners (~30 LoC + tests)
  drain_test.go                                     -- NEW: unit tests for the POST handler
                                                       (~80 LoC)
```

### 3.2 Changed production code

```
cmd/envoy-go/main.go                                -- modified: SIGTERM-handler block at line 170
                                                       upgraded per Decision 2; drain.New(30s) allocated
                                                       at boot per Decision 6; threaded into
                                                       listener.NewManager... + admin.New per Decision 4;
                                                       cm.Drain() called after <-drainMgr.Done() per
                                                       Decision 8.
                                                       ~30 LoC delta

internal/admin/admin.go                             -- modified: New() signature widens to take
                                                       *drain.Manager (Decision 4); Server gains dm field;
                                                       Start() body adds mux.HandleFunc(/drain_listeners, ...);
                                                       handleReady gains DRAINING branch (Decision 9).
                                                       ~20 LoC delta

internal/admin/serverinfo.go                        -- modified: deriveState extended to take *drain.Manager
                                                       and check DRAINING first (Decision 10).
                                                       ~5 LoC delta

internal/listener/manager.go                        -- modified: Manager gains dm field; constructor
                                                       NewManagerWithBaseDirAndAllowH2C signature widens to
                                                       take *drain.Manager (Decision 4); Drain() method
                                                       added (Decision 5); per-runtime Accept loop fast-path
                                                       checks dm.IsDraining() (Decision 5).
                                                       ~30 LoC delta + N-1 carry-forward (08.1 REVIEW
                                                       finding): doc-comment on Listeners() ordering.

internal/cluster/manager.go                         -- modified: Manager gains Drain() method (Decision 8);
                                                       per-cluster connection pool close.
                                                       ~30 LoC delta

internal/filter/hcm/hcm.go (or equivalent)          -- modified: HCM constructor widens to take
                                                       *drain.Manager; decodeHeaders calls Inc;
                                                       encodeFinalize calls Dec (Decision 7).
                                                       ~15 LoC delta

internal/filter/tcpproxy/filter.go                  -- modified: TCP proxy filter constructor widens to
                                                       take *drain.Manager; OnNewConnection calls Inc;
                                                       OnConnectionClose calls Dec (Decision 7).
                                                       ~10 LoC delta
```

### 3.3 New harness and fixture code

```
test/fixtures/0010-graceful-drain/                  -- NEW fixture
  README.md                                         -- fixture overview + driver flow + equivalence claims
  expectations.yaml                                 -- per-state-transition tolerance discipline
  envoy.yaml                                        -- reference Envoy bootstrap (admin :9902)
  envoy-go.yaml                                     -- envoy-go bootstrap (admin :9901)
  driver/driver.go                                  -- Go driver: opens long-lived in-flight request,
                                                       triggers drain, asserts new-conn rejected,
                                                       asserts in-flight completes,
                                                       asserts /ready + /server_info transitions
                                                       (~300 LoC)
  backends/backend.go                               -- minimal Go HTTP backend with slow-streaming
                                                       response endpoint (~50 LoC)

test/differential/runner.go                         -- registration: RegisterFixture("0010-graceful-drain", ...,
                                                       Capabilities{RequiresReference: true})
                                                       ~3 LoC delta
```

### 3.4 Changed documentation

```
docs/envoy-go/BEHAVIOR_CONTRACT.md                  -- in-place edit per ADR-0052:
                                                       (a) ## Admin API ### /drain_listeners NEW subsection
                                                       (b) ## Admin API ### /ready DRAINING-body extension
                                                       (c) ## Admin API ### /server_info DRAINING-state extension
                                                       (d) ## Graceful drain NEW umbrella section
                                                       (e) ## Equivalence Matrix two new rows

docs/envoy-go/DECISIONS.md                          -- ADRs ADR-0091..ADR-0099 (or fewer per consolidation
                                                       in §9); ADR-0088 + ADR-0015 amended (ADR-0098 +
                                                       ADR-0097 partial supersessions)

docs/envoy-go/ROADMAP.md                            -- row 08.2 flips planned → in-progress at SPEC commit;
                                                       in-progress → done at phase-done commit;
                                                       row 08 (parent) flips in-progress → done AT THE SAME
                                                       COMMIT as 08.2 phase-done (per parent SPEC §5).

docs/envoy-go/STATE.md                              -- session-by-session updates per BOOTSTRAP_PROMPT §5;
                                                       FINAL state at 08.2 phase-done flips to
                                                       "awaiting next planning" per BOOTSTRAP_PROMPT §5
                                                       (MVP trunk closure).
```

---

## 4. Carry-forward from 08.1 (architectural + REVIEW findings)

### 4.1 Architectural carry-forward

The 08.1 SPEC + the 08.1 REVIEW.md (`docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md`) together establish three architectural patterns 08.2 inherits:

**(a) Admin-mux extension pattern (per ADR-0085).** The existing `*http.ServeMux` allocated at `internal/admin/admin.go:78` carries five `mux.HandleFunc(...)` registrations as of 08.1 (`/ready`, `/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`). 08.2 adds one more registration: `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` — the seventh handler on the same mux (per Decision 3). No new HTTP server, no new bind, no new transport.

**(b) Constructor-widening pattern (LBP-1).** ADR-0085's "no package globals; explicit threading" discipline extends to `*drain.Manager` per Decision 4. The `admin.New` signature, currently 5-parameter at `internal/admin/admin.go:51`, widens to 6-parameter. The `listener.NewManagerWithBaseDirAndAllowH2C` signature widens to add the `*drain.Manager` parameter. The HCM and TCP proxy constructors widen similarly (per Decision 7). Per the 08.1 REVIEW.md §2.1 recap: "the constructor-widening pattern is a clean generalisation of 06.1's `*stats.Registry` and 07.1's `*HTTPRegistry` / 07.2's `*ListenerFilterRegistry` precedents" — 08.2 is the fifth application.

**(c) ADR-0088 anticipated DRAINING amendment path.** Per ADR-0088's consequence (c) verbatim from `DECISIONS.md` lines 3441-3442: "Phase 08.2's drain implementation extends the state enum coverage to LIVE + PRE_INITIALIZING + DRAINING by adding a third atomic flag (or by extending s.ready semantics — 08.2's PLAN settles the choice) and amending `deriveState` to return `adminv3.ServerInfo_DRAINING` when the drain flag is set. The amendment is purely additive; no other field changes. The ADR-0088 amendment will record the addition without superseding this ADR." Decision 10 IS that amendment. The choice between "third atomic flag on Server" vs. "extend s.ready semantics" is settled here: third atomic, but in `*drain.Manager` (the single source of truth), not on `*Server`.

**(d) BEHAVIOR_CONTRACT umbrella in-place edit (per ADR-0052).** The `## Admin API` umbrella that 08.1's restructure landed (`### /ready`, `### /stats/prometheus`, `### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info` per 08.1 SPEC §13.1) is the host structure 08.2 extends. 08.2 adds `### /drain_listeners` as a new sibling subsection AND extends `### /ready` (DRAINING body) and `### /server_info` (DRAINING state) in place. ADR-0052's authorization for in-place BEHAVIOR_CONTRACT edits is the durable record; no new ADR for the in-place edit pattern.

**(e) Empirical-pin discipline (per ADR-0004).** 08.1 executed eight empirical pins IN-SESSION at SPEC time (per 08.1 SPEC §11.1–§11.9). 08.2 follows the same discipline; §11 of THIS BRAINSTORM enumerates the seven 08.2-specific pins; the 08.2 SPEC author resolves them in-session against Envoy v1.37.2.

### 4.2 REVIEW.md findings (carry-forward dispositions)

The 08.1 REVIEW.md §4.2 enumerated five Note-tier findings (N-1 through N-5). 08.1 REVIEW.md §5 carry-forward table dispositions four of the five to 08.2 (carry-forward to 08.2) or to a future hardening pass:

| Finding | 08.1 disposition | 08.2 carry-forward action |
|---|---|---|
| **N-1** `internal/listener.Manager.Listeners()` doc-comment ordering not documented | Carry-forward to 08.2 (Listener.Manager touched by drain wiring) | **08.2 inline-fix:** add the one-line doc-comment on `Listeners()` saying "order is bootstrap-declaration order; callers needing alphabetical ordering must sort." Lands as part of Decision 5's `internal/listener/manager.go` modifications. Cost: ~3 LoC. |
| **N-2** `internal/admin/clusters.go:78-99` `writeEndpointLines` table-driven refactor opportunity | Carry-forward to a future ADR-0063-supersession phase | **08.2 NO action.** ADR-0063 is unmodified by 08.2; the refactor opportunity remains. |
| **N-3** `BuildVersionString()` memoization opportunity | Carry-forward to a future micro-optimisation pass | **08.2 NO action.** No micro-optimisation pass scheduled; the per-request cost is microsecond-scale. |
| **N-4** `wantedTypes` cross-reference doc-comment in fixture 0009 canonicaliser | Carry-forward to 08.2 (fixture 0010 likely touches the same canonicaliser) | **08.2 inline-fix candidate:** the 0010 fixture's driver may share canonicalisation utilities with 0009; if so, add the cross-reference doc-comment as part of the shared-util touch. Lands as part of §7 fixture design. Cost: ~5 LoC. |
| **N-5** `FuzzConfigDumpFormat` corpus expansion | Carry-forward to a future fuzzer-hardening pass | **08.2 NO action.** No fuzzer-hardening pass scheduled; the existing fuzzer's coverage is adequate per ADR-0018 30s budget. |

Additionally, the 08.1 SPEC §10 carry-forward of M-8 from 07.2 REVIEW (200ms hardcoded drain in 0007b fixture driver) is **directly relevant to 08.2**: the 0010 fixture driver should NOT repeat the same hardcoded sleep pattern. Per §7 fixture design, the 0010 driver uses event-based synchronization (poll `/ready` until DRAINING body observed; poll until `inflight=0` observed — no hardcoded sleeps).

### 4.3 No 08.1-introduced regressions

The 08.1 REVIEW.md §1 final assessment was APPROVED with no Major and no Minor findings. The 08.1 implementation does not block 08.2 in any way; the carry-forward findings above are documentation-tier, not correctness-tier.

---

## 5. Per-state-transition data flow

This section walks the load-bearing flows the 08.2 implementation must realize. Each flow is a per-actor swimlane with per-step state changes annotated. Empirical-pin obligations are flagged inline; the SPEC author resolves them at SPEC time.

### 5.1 SIGTERM → drain → exit (the canonical lifecycle flow)

```
TIME=t0  cmd/envoy-go/main.go is blocked on <-ctx.Done() (line 170, current)
TIME=t1  Operator runs `kill -TERM <pid>`
         signal.NotifyContext (line 145) cancels ctx
TIME=t2  <-ctx.Done() unblocks
         drainMgr.Drain() called:
           - atomic.CompareAndSwap(state, Live, Draining) succeeds
           - sync.Once-guarded: subsequent Drain() calls no-op
TIME=t3  Listener Accept loop observes dm.IsDraining() == true on next iteration:
           - any Accept() call returns a conn; the conn is immediately Closed
           - empirical-pin §11.5: does Envoy actually do accept-and-close, or
             listener-socket-close? envoy-go matches per the verbatim evidence.
TIME=t3' Concurrently: in-flight HCM streams finish their decode/encode chain.
         encodeFinalize calls dm.Dec() once per stream.
         The drain manager's atomic.Int64 inflight counter decrements toward 0.
TIME=t4  drain.Manager polls (or callback-fires) inflight==0 → close drainMgr.done channel
         OR
         drainMgr.Timeout() (30s) fires first → close drainMgr.done channel best-effort
TIME=t5  cmd/envoy-go/main.go's select on <-drainMgr.Done() / <-time.After(timeout) unblocks.
         cm.Drain() called: walks per-cluster connection pools and closes them.
TIME=t6  The deferred-stop chain runs (LIFO):
           - lm.Stop() closes listening sockets (idempotent if listener-socket-close
             was the §11.5 chosen mechanism — Stop() is the post-drain teardown)
           - admSrv.Close() shuts the admin HTTP server
           - sinks-close flushes access logs
TIME=t7  main returns; process exits with status 0.
```

**Correctness invariants:**
- (a) Between t2 and t6, NEW downstream connections receive accept-and-immediate-close (or TCP RST per §11.5). No new request is dispatched into the HCM filter chain.
- (b) Between t2 and t4, in-flight downstream requests COMPLETE — they run their full HCM chain; no abort-mid-flight. Access logs are emitted per phase 06.2's hooks.
- (c) Between t4 and t6, upstream connection pools close. No new upstream connection is opened (because no new in-flight downstream request is dispatched to the cluster manager).
- (d) Total drain window is bounded by `drainMgr.Timeout()` (30s default per Decision 6). The timeout is best-effort: if in-flight reaches 0 before the timeout, drain completes faster.

### 5.2 POST /drain_listeners → drain (no exit)

```
TIME=t0  envoy-go is in steady-state (LIVE; admin and listeners running)
TIME=t1  Operator runs `curl -X POST http://<admin>:<port>/drain_listeners`
TIME=t2  net/http dispatches to s.handleDrainListeners (internal/admin/drain.go)
         drainMgr.Drain() called:
           - atomic.CompareAndSwap(state, Live, Draining) succeeds (or no-ops if already Draining)
           - response: 200 OK with body per §11.1 verbatim (TBD)
           - response is FIRE-AND-FORGET: handler does NOT block on <-drainMgr.Done()
TIME=t3  curl prints the 200 response and exits.
TIME=t4  envoy-go continues running in DRAINING state INDEFINITELY:
           - new connections rejected (per Listener Accept loop fast-path; same as §5.1 t3)
           - in-flight requests complete (per HCM Inc/Dec; same as §5.1 t3')
           - /ready returns 503 DRAINING\n (per Decision 9; same as §5.3)
           - /server_info returns state: "DRAINING" (per Decision 10; same as §5.4)
TIME=t5  Process does NOT exit. The operator is responsible for issuing SIGTERM/SIGINT
         (or kill -9) at a later time to actually exit; until then, the proxy stays
         in DRAINING — accepting metrics scrapes, accepting /ready scrapes, completing
         any in-flight requests (which by t5 should all be done), but rejecting new
         downstream connections.
```

**Correctness invariants:**
- (a) The handler returns 200 BEFORE drain completes; the operator's curl call does NOT hang for 30s.
- (b) Subsequent `POST /drain_listeners` calls return 200 OK with the same body (idempotent).
- (c) No process exit. Distinguishes from §5.1 (SIGTERM → exit).

### 5.3 /ready scrape during drain

```
TIME=t0  envoy-go is in DRAINING state (post-§5.1 t2 OR post-§5.2 t2)
TIME=t1  Load balancer scrape: GET /ready
TIME=t2  net/http dispatches to s.handleReady (internal/admin/admin.go:121, modified per Decision 9)
         New branch FIRST: dm != nil && dm.State() == Draining
           - response: 503 Service Unavailable
           - body: DRAINING\n (verbatim TBD §11.2)
           - Content-Type: text/plain; charset=UTF-8
           - Cache-Control: no-cache, max-age=0; X-Content-Type-Options: nosniff; Server: envoy
           - Content-Length: 9 (envoy-go) / chunked (Envoy per phase-01 framing deviation)
TIME=t3  Load balancer marks the instance unhealthy and stops sending traffic.
```

**Correctness invariants:**
- (a) Before drain: /ready returns 200 LIVE\n (existing 08.1 behavior, unchanged).
- (b) Pre-MarkReady (boot window): /ready returns 503 PRE_INITIALIZING\n (existing 01 behavior per ADR-0015, unchanged).
- (c) During drain (drain has fired but in-flight not yet 0): /ready returns 503 DRAINING\n (NEW per Decision 9).
- (d) The DRAINING branch wins over PRE_INITIALIZING (an unlikely but defined-by-precedence case where Drain() fires before MarkReady; handler emits DRAINING).

### 5.4 /server_info scrape during drain

```
TIME=t0  envoy-go is in DRAINING state
TIME=t1  Operator scrape: GET /server_info
TIME=t2  net/http dispatches to s.handleServerInfo (internal/admin/serverinfo.go:21,
         buildServerInfo with extended deriveState per Decision 10)
TIME=t3  buildServerInfo returns *adminv3.ServerInfo with State = ServerInfo_DRAINING
         protojson.Marshal renders state field as the string "DRAINING"
TIME=t4  Response: 200 OK with body containing "state": "DRAINING" (other fields per ADR-0088 unchanged)
```

**Correctness invariants:**
- (a) During drain: state is "DRAINING".
- (b) Before drain: state is "LIVE" (post-MarkReady).
- (c) Pre-MarkReady: state is "PRE_INITIALIZING" (existing per ADR-0088).
- (d) The DRAINING precedence (DRAINING > LIVE > PRE_INITIALIZING) per Decision 10.

### 5.5 In-flight request completion during drain

```
TIME=t0  H1 keep-alive connection C1 is open with envoy-go; an in-flight request R1 is mid-decode.
         Drain has not fired.
TIME=t1  Drain fires (either §5.1 or §5.2).
         drainMgr.Drain() transitions state. atomic.Int64 inflight is currently 1 (R1).
TIME=t2  R1 continues: decodeHeaders has already run (inflight already incremented at t<0).
         The HCM filter chain proceeds. The router filter dispatches to the upstream.
TIME=t3  The cluster manager's connection pool already has an open upstream conn for the route's cluster.
         The upstream request is sent. The upstream responds.
TIME=t4  HCM encodeHeaders + encodeData run. Response is written to the downstream conn.
TIME=t5  encodeFinalize runs (per phase 06.2's access-log hooks). dm.Dec() called.
         atomic.Int64 inflight goes 1 → 0.
         If draining:
           drainMgr observes inflight==0 → close drainMgr.done channel.
TIME=t6  The H1 keep-alive connection C1 is still open after R1 completes.
         Empirical-pin §11.3: does Envoy emit Connection: close on R1's response during drain?
         If yes: envoy-go matches; the conn closes after R1's response writes.
         If no: envoy-go matches; the conn stays open but rejects further requests via
                an empirical-pin-defined mechanism (perhaps just a timeout-driven close;
                the §11.3 evidence settles).
TIME=t7  In the §5.1 lifecycle: drainMgr.Done() unblocks; cm.Drain() runs; lm.Stop() runs.
         In the §5.2 lifecycle: process stays running; C1 stays alive (or closes per §11.3);
                                /ready continues returning DRAINING; the operator is
                                responsible for the eventual SIGTERM.
```

**Correctness invariants:**
- (a) R1 sees no abort. Its response is fully written to C1 with a 2xx (or whatever status is appropriate per the upstream's response).
- (b) Inflight count balances: 1 → 0 across decodeHeaders/encodeFinalize.
- (c) The drainMgr.Done() rendezvous is sound: it fires exactly when inflight reaches 0 (or timeout fires).

### 5.6 New connection during drain

```
TIME=t0  envoy-go is in DRAINING state
TIME=t1  A client opens a new TCP connection to the proxy listener port.
TIME=t2  The kernel's TCP backlog accepts the connection (3-way handshake completes).
         The Listener Accept loop's blocking Accept() call returns the new net.Conn.
TIME=t3  Accept-loop body's first action: check dm.IsDraining(). Returns true.
         The new conn is immediately Closed via conn.Close(). NO filter chain dispatch.
         The client observes a TCP FIN (or RST per §11.5 — empirical TBD) on its
         first read attempt.
```

**Correctness invariants:**
- (a) No new connection is dispatched to the filter chain.
- (b) inflight does NOT increment for this connection (Inc lives in HCM/TCP-proxy filter, which doesn't run).
- (c) The empirical-pin §11.5 settles whether the close is FIN (graceful close-after-3-way) or RST (kernel-level close).

---

## 6. BEHAVIOR_CONTRACT.md additions

Phase 08.2 extends the existing `## Admin API` umbrella section that 08.1 landed (per 08.1 SPEC §13.1) and adds a NEW sibling `## Graceful drain` umbrella section. All edits land in-place per ADR-0052. No new ADR for the in-place-edit authorization.

### 6.1 `## Admin API ### /drain_listeners` NEW subsection

Inserted under the `## Admin API` umbrella, after the `### /server_info` subsection (preserving alphabetical-by-path order: /clusters, /config_dump, /drain_listeners, /listeners, /ready, /server_info, /stats/prometheus). Verbatim shape (body details TBD per §11.1):

```markdown
### /drain_listeners
**Body shape.** `<content-type TBD §11.1>`. Body verbatim TBD §11.1 (likely `OK\n` or `{}\n`). The handler is fire-and-forget — 200 OK is emitted BEFORE drain completes; the operator polls /ready or /server_info to observe drain progress. Idempotent — subsequent calls during DRAINING return 200 with the same body without re-firing the drain trigger. The `?graceful=true` query-param is silently accepted (Envoy supports it; envoy-go's drain is always graceful — there is no non-graceful variant in MVP); per ADR-0041's silent-ignore precedent.

**Empirical evidence (verbatim Envoy v1.37.2 `POST /drain_listeners`):** see 08.2 SPEC §11.1.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 (after framing dechunk). Header set inherits the umbrella rules (Content-Type, Cache-Control, X-Content-Type-Options, Date, Server). Method-discrimination posture: POST is canonical; GET/PUT/DELETE behavior matches Envoy parity (likely accepted with same body — empirical pin §11.4).
```

### 6.2 `## Admin API ### /ready` extension

Append a new sub-block after the existing `Pre-init response` block:

```markdown
**DRAINING-state response (08.2 NEW).** When `drain.Manager.State() == DRAINING`, the handler returns 503 Service Unavailable with body `<verbatim TBD §11.2>` (likely `DRAINING\n`). The DRAINING check has precedence over both LIVE and PRE_INITIALIZING — once drain has fired, /ready returns the DRAINING body even if MarkReady has been called and even if /server_info would otherwise return state="LIVE". Header set inherits the umbrella rules.

**Empirical evidence (verbatim Envoy v1.37.2 `/ready` during drain):** see 08.2 SPEC §11.2.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 in DRAINING state.

ADR-0015 (pre-init contract for /ready) is **partially superseded by ADR-0097**: the LIVE/PRE_INITIALIZING two-state coverage extends to LIVE/PRE_INITIALIZING/DRAINING three-state coverage. ADR-0015's verbatim pre-init body and pre-init status are preserved; ADR-0097 adds the DRAINING branch and the precedence rule.
```

### 6.3 `## Admin API ### /server_info` extension

Modify the State enum bullet under `### /server_info`'s body-shape paragraph:

```markdown
State enum (08.2 EXTENDED): `LIVE` (post-MarkReady, drain has not fired), `PRE_INITIALIZING` (pre-MarkReady, drain has not fired), `DRAINING` (drain has fired — supersedes LIVE and PRE_INITIALIZING). `INITIALIZING` is documented in `adminv3.ServerInfo_State` but unreachable in envoy-go's static-bootstrap-only model (08.1 SPEC §11.7).

**Equivalence claim extension (08.2):** the state field IS asserted byte-equal across both proxies in DRAINING (`"DRAINING"` literal). The 08.1 byte-equal claim for `"LIVE"` post-MarkReady is unchanged.

ADR-0088 is **amended** by ADR-0098 (NOT superseded — purely additive). The ADR-0088 amendment record adds DRAINING to the enum-coverage table and refers to ADR-0098 for the timing semantics.
```

### 6.4 `## Graceful drain` NEW umbrella section

A new sibling section to `## Admin API`, placed immediately after `## Admin API` in BEHAVIOR_CONTRACT.md. Verbatim shape:

```markdown
## Graceful drain

The envoy-go drain machinery transitions the process from LIVE → DRAINING → exit (via SIGTERM/SIGINT) or LIVE → DRAINING (via POST /drain_listeners; no exit). The state machine lives in the `internal/drain` package (08.2 NEW; ADR-0091); the drain manager is a single-instance lock-free state machine with three states and an in-flight counter.

### Drain triggers

Two operator workflows trigger drain:

1. **SIGTERM or SIGINT:** drain-then-exit. The signal causes `cmd/envoy-go/main.go`'s top-level context to cancel; the main goroutine then calls `drain.Manager.Drain()`, waits on `drain.Manager.Done()` (or a 30s timeout), then proceeds to per-cluster connection-pool teardown + listener-socket close + admin server close + access-log flush. The total drain window is bounded by the 30s timeout.

2. **POST /drain_listeners admin endpoint:** drain-without-exit. The handler triggers `drain.Manager.Drain()` synchronously and returns 200 OK before drain completes. The proxy stays running in DRAINING indefinitely; the operator separately issues SIGTERM/SIGINT (or kill -9) at a later time to actually exit.

Both triggers result in the same drain BEHAVIOR (state transition, listener stop-accepting, in-flight completion, /ready and /server_info responses). They differ only in the post-drain disposition (exit vs. stay-running).

### Drain semantics

When drain fires (state transitions LIVE → DRAINING):

- **New connections rejected.** The Listener Accept loop's fast-path checks `drain.Manager.IsDraining()` on each iteration; an Accept-ed conn during DRAINING is immediately closed without filter-chain dispatch. Empirical pin (08.2 SPEC §11.5) settles whether the close is graceful FIN or kernel RST.

- **In-flight requests complete.** The HCM filter chain's `decodeHeaders`/`encodeFinalize` pair calls `drain.Manager.Inc()`/`Dec()` to track per-request in-flight count. The drain manager's `Done()` channel closes when the in-flight counter reaches 0 (or the 30s timeout fires).

- **/ready returns 503 DRAINING\n** (verbatim TBD per 08.2 SPEC §11.2). Operators / load balancers observe the DRAINING signal and stop sending traffic.

- **/server_info returns state: "DRAINING"** (per ADR-0098 amending ADR-0088).

- **Idempotent.** Subsequent Drain() calls (e.g., a second POST /drain_listeners, or SIGTERM after a prior /drain_listeners) no-op — the state transition has already fired.

### Drain timeout

The drain timeout is a hardcoded 30s in envoy-go MVP (per ADR-0095). Envoy v1.37.2's default is 600s (per 08.1 SPEC §11.4 verbatim scrape); the divergence is deliberate to keep test-suite cost tractable. Operator-knob to configure the timeout is deferred to a future runtime / hot-restart family phase.

### Connection-level drain semantics

Phase 08.2 does NOT implement per-connection drain windows (Envoy supports per-connection drainable closure at the next idle window). HTTP/1.1 keep-alive connections during drain emit `Connection: close` on the in-flight response per empirical pin (08.2 SPEC §11.3), then close. HTTP/2 connections during drain emit GOAWAY per empirical pin (08.2 SPEC §11.3), allowing the remote endpoint to learn that no new streams will be accepted.

### Applies to

- phase-08.2 envoy-go drain subsystem.
- the SIGTERM-handler in `cmd/envoy-go/main.go` (Decision 2).
- the POST /drain_listeners admin endpoint (Decision 3).
- the /ready DRAINING-state body (Decision 9; ADR-0097 partially supersedes ADR-0015).
- the /server_info DRAINING-state field (Decision 10; ADR-0098 amends ADR-0088).
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to

- Hot restart / parent-child handoff (deferred to runtime / hot-restart family per ADR-0099).
- POST /quitquitquit endpoint (semantic overlap with SIGTERM + /drain_listeners; deferred per ADR-0089 + ADR-0099).
- Per-listener selective drain (`/listeners/<name>/drain` admin sub-routes deferred per ADR-0089).
- `drain_strategy` per-listener (default GRADUAL only; IMMEDIATE strategy deferred).
- Configurable drain timeout (hardcoded 30s; operator-knob deferred per ADR-0095).
- Connection-level drain windows configurability.
- Drain manager interaction with xDS (no xDS yet; deferred per ADR-0089).
```

### 6.5 New equivalence-matrix rows

Append to the `## Equivalence Matrix` table:

```
| Admin /drain_listeners      | Body byte-equal to reference Envoy v1.37.2 (after framing dechunk). Idempotent semantics; query-param ?graceful=true silent-ignored. | Header set inherits umbrella rules; framing per phase-01 dechunk-discipline.                                                                                |
| Admin /ready (DRAINING)     | Body byte-equal to reference Envoy v1.37.2 in DRAINING state.                                                                                  | DRAINING precedence over LIVE/PRE_INITIALIZING. Status code 503 (matches PRE_INITIALIZING). Header set inherits umbrella rules.                          |
| Admin /server_info (DRAINING) | The state field IS asserted byte-equal (`"DRAINING"`) when both proxies are in DRAINING. Other fields per ADR-0088 allow-list.            | Inherits ADR-0088 allow-list for non-state fields (version, uptime_*, command_line_options, hot_restart_version, node).                                  |
```

(Three rows; the first is a NEW dimension; the second and third are EXTENSIONS to existing 08.1 rows for /ready and /server_info — implementation may choose to write three new rows OR extend the existing rows in-place; the SPEC author settles.)

---

## 7. Fixture design — `0010-graceful-drain`

Single fixture per Decision 12. Dual-proxy (envoy-go on admin :9901 + listener :10000; reference Envoy on admin :9902 + listener :10001), inheriting the §7.3 fixture-bootstrap shape from 08.1 SPEC §7.3 with one structural modification: the upstream backend's response is slow-streaming (1KB/s, 5s total response time) so the in-flight request's behavior during drain is observable.

### 7.1 Bootstrap shape

```yaml
# envoy-go.yaml (admin :9901, listener :10000)
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}

static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 127.0.0.1, port_value: 10000}}
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
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18001}}}
```

`envoy.yaml` is identical modulo `admin.address.port_value: 9902` and `listeners[0].address.port_value: 10001`.

### 7.2 Driver flow (combined admin-trigger + SIGTERM-trigger paths)

The driver tests two paths in the same fixture (per Decision 12 alternatives consideration). The driver is parameterized on `triggerType ∈ {admin, sigterm}` and runs both paths back-to-back (against fresh proxy invocations each time).

**Admin-trigger path:**

1. Boot envoy-go on admin :9901 + listener :10000; reference Envoy on admin :9902 + listener :10001; backend on :18001 (slow-streaming 1KB/s, 5s response).
2. Sanity scrape: GET /ready against both proxies → 200 LIVE\n on both.
3. Sanity scrape: GET /server_info against both proxies → `state: "LIVE"` on both.
4. Open a long-lived in-flight request: `GET /slow` against the listener (port 10000 / 10001). Response is streaming; the driver reads partial body bytes.
5. Trigger drain via admin: `POST /drain_listeners` against the admin port. Assert 200 OK with body matching the Envoy verbatim (per §11.1).
6. Assert /ready transitions: poll GET /ready until 503 DRAINING\n (max 1s; fail if not observed).
7. Assert /server_info transitions: GET /server_info, parse JSON, assert `state == "DRAINING"`.
8. Assert new-conn rejected: open a new TCP conn to the listener port; expect immediate close (FIN or RST per §11.5).
9. Wait for the in-flight `GET /slow` request to complete (max 6s; fail if not).
10. Assert in-flight response: status 200 OK, body length matches expected (5KB total).
11. Assert /server_info post-completion: state STILL `"DRAINING"` (the proxy stays in DRAINING after in-flight completes; drain-without-exit per §5.2).
12. Cleanup: kill the proxy (the admin-trigger path does NOT auto-exit); wait for process exit.

**SIGTERM-trigger path:**

1. Boot envoy-go + reference Envoy + backend (same as admin-trigger step 1).
2. Sanity scrape (same as admin-trigger steps 2 + 3).
3. Open a long-lived in-flight request (same as admin-trigger step 4).
4. Trigger drain via signal: `kill -TERM <pid>` against each proxy.
5. Assert /ready transitions: poll until 503 DRAINING\n.
6. Assert /server_info transitions: state == "DRAINING".
7. Assert new-conn rejected (same as admin-trigger step 8).
8. Wait for the in-flight `GET /slow` request to complete.
9. Assert in-flight response: status 200 OK, body intact.
10. Assert proxy exits within drain-timeout window (30s envoy-go MVP; Envoy 600s — but the in-flight count reached 0, so Done() fires immediately and the proxy exits in <1s in both implementations).
11. Assert exit status: 0 on both proxies (graceful).

### 7.3 Equivalence claims (per state transition)

The differential equivalence claim follows the existing per-state-transition discipline of 08.1's structural-projection canonicalisation (per 08.1 SPEC §7.1 + 08.1 REVIEW §2.5). Five per-transition claims:

1. **Steady-state /ready:** byte-equal LIVE\n on both proxies (existing 08.1 baseline).
2. **POST /drain_listeners response:** byte-equal verbatim per §11.1.
3. **/ready DRAINING:** byte-equal DRAINING-body verbatim per §11.2.
4. **/server_info DRAINING:** the state field byte-equal `"DRAINING"`; other fields per the ADR-0088 allow-list (08.1 baseline carries forward).
5. **In-flight request completion:** the in-flight `GET /slow` request returns 200 OK with the same body bytes on both proxies (the upstream backend serves the same content on both runs; the proxy is transparent).

The new-connection-rejection assertion (step 8 of either path) is a connectivity-level check (TCP-level FIN or RST), not a body-level differential claim. Per §11.5, the empirical-pin obligation settles the exact mechanism.

### 7.4 Driver-implementation guidance

- Use event-based synchronization (poll until DRAINING body observed; poll until inflight=0 observed); NO hardcoded sleep patterns. Carry-forward from 07.2 REVIEW M-8 + 08.1 SPEC §10.
- Implement two driver paths in the same `driver/driver.go` under a `triggerType` parameter; share the dual-proxy boot + slow-streaming-backend bootstrap + cleanup utilities.
- The per-state-transition canonicalisers may share utilities with the 0009 fixture (per 08.1 REVIEW N-4 carry-forward — the wantedTypes cross-reference doc-comment gets added as part of the share-touch).
- Register as `RequiresReference: true` per the existing fixture-registration pattern (mirrors 0007a-cors and 0009-admin-config-dump).

### 7.5 Backends shape

```go
// test/fixtures/0010-graceful-drain/backends/backend.go (sketch)
package main

func main() {
    http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        // Stream 5KB at 1KB/s = 5s total
        for i := 0; i < 5; i++ {
            _, _ = w.Write(bytes.Repeat([]byte{'x'}, 1024))
            if f, ok := w.(http.Flusher); ok { f.Flush() }
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

The slow handler is the load-bearing component; the fast handler is a sanity baseline.

---

## 8. Testing strategy

### 8.1 Unit tests — `internal/drain/`

- `manager_test.go`:
  - `TestStateTransitions` — Live → Draining via Drain(); Draining stays in Draining (idempotent); Drained transition fires Done() channel close.
  - `TestInflightBalance` — Inc/Dec balance: 1 Inc + 1 Dec brings inflight to 0; multiple Inc + matching Dec balance.
  - `TestDrainCompletionRendezvous` — Drain() then Inc once; Dec once; Done() unblocks.
  - `TestDrainTimeout` — Drain() with no Inc; Done() unblocks immediately (inflight already 0). Drain() with one Inc + no Dec; Done() unblocks after timeout.
  - `TestIdempotentDrain` — multiple Drain() calls; only one transition fires; Done() unblocks once.
  - `TestIsDrainingFastPath` — pre-Drain: false. Post-Drain: true. Atomic load is lock-free (covered implicitly; explicit benchmark optional).
  - `TestNilSafety` — `(m *Manager).IsDraining` does NOT nil-check m (it's a pointer receiver; nil receiver panics — tests assert nil-receiver via `defer recover()`).

### 8.2 Unit tests — `internal/admin/`

- `drain_test.go`:
  - `TestHandleDrainListeners_PostFires` — POST /drain_listeners; assert 200 + verbatim body; assert drainMgr.State() == Draining post-call.
  - `TestHandleDrainListeners_Idempotent` — two POSTs; both 200 with same body; only one Drain() transition.
  - `TestHandleDrainListeners_GraceQueryParamSilentlyIgnored` — POST /drain_listeners?graceful=true; assert 200 + verbatim body (ADR-0041 silent-ignore).
  - `TestHandleDrainListeners_NilDrainManager` — handler with `s.dm == nil`; assert defensive 500 OR no-op + 200 (per planner-time decision in SPEC).
  - `TestHandleDrainListeners_AcceptAnyMethod` — GET / PUT / DELETE all 200 (Envoy parity per ADR-0090; empirical pin §11.4 confirms).
- `admin_test.go` (modified): existing tests preserved; new tests for the DRAINING /ready branch (Decision 9):
  - `TestHandleReady_Draining` — set `s.dm` to a Manager with state=Draining; assert /ready returns 503 DRAINING\n (verbatim per §11.2).
  - `TestHandleReady_DrainingPrecedesPreInitializing` — set `s.dm` to Draining BEFORE MarkReady; assert /ready returns 503 DRAINING\n (NOT PRE_INITIALIZING).
  - `TestHandleReady_DrainingPrecedesLive` — set `s.dm` to Draining AFTER MarkReady; assert /ready returns 503 DRAINING\n (NOT LIVE).
- `serverinfo_test.go` (modified): new tests for the DRAINING /server_info branch (Decision 10):
  - `TestHandleServerInfo_StateDraining` — set `s.dm` to Draining; assert state == "DRAINING".
  - `TestHandleServerInfo_StatePrecedence` — Draining + Ready: state == "DRAINING" (not "LIVE").

### 8.3 Unit tests — `internal/listener/manager.go` + `internal/cluster/manager.go`

- `internal/listener/manager_test.go`:
  - `TestManager_Drain` — call Drain(); assert dm.IsDraining() returns true; subsequent Accept-loop iterations close the new conn.
  - `TestManager_DrainIdempotent` — multiple Drain() calls; idempotent.
- `internal/cluster/manager_test.go`:
  - `TestManager_DrainClosesPools` — call Drain(); assert per-cluster pools are closed (e.g., HTTP/2 ClientConn instances are nil after Drain; HTTP/1.1 keepalive pools are empty).

### 8.4 Race-detector contract

`go test -race ./...` clean. Specifically:
- The drain manager's `atomic.Uint32` state field is concurrently read by the Listener Accept loop, HCM `decodeHeaders`, and admin `handleReady` / `handleServerInfo`; concurrently written by `handleDrainListeners` and the SIGTERM-handler. Race-detector clean is the contract.
- The `atomic.Int64` inflight counter is concurrently incremented by HCM and TCP-proxy; concurrently decremented likewise. Race-detector clean.
- Concurrent-scrape race-test similar to 08.1's `TestAdminConcurrentScrapeRace`: 100 goroutines scraping all seven admin endpoints (six pre-08.2 + the new /drain_listeners) in tight loops while a separate goroutine fires Drain() once mid-test. Asserts no race-detector finding, no panic, no malformed responses.

### 8.5 Fuzz testing

**Decision deferred to SPEC author** (§8.5 candidate): a `FuzzDrainTransitions` fuzzer (~60 LoC; 30s budget per ADR-0018) that fuzzes a sequence of operations against `*drain.Manager` (Drain, Inc, Dec, IsDraining, Done) and asserts invariants (state transitions are monotonic; inflight balance; Done() fires exactly once). Alternative: skip the fuzzer (the state machine is small enough that exhaustive unit tests cover it; ADR-0018's "every parser/codec/filter ships a fuzzer" rule does NOT clearly apply to a state machine). The 08.2 SPEC author settles based on whether the state machine surface area justifies a fuzzer.

### 8.6 Existing fuzzers re-run

The 10 existing fuzzers (per 08.1 REVIEW gate (d) — `FuzzBootstrapLoad`, `FuzzTcpProxyFilter`, `FuzzTLSContextParse`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`, `FuzzPromTextFormat`, `FuzzAccessLogFormat` (or `FuzzDefaultFormatRender` per the 08.1 PLAN gate-command erratum), `FuzzFilterChainParse`, `FuzzFilterChainMatch`, `FuzzConfigDumpFormat`) re-run at 30s budget. None exercise drain machinery; all are mechanical re-runs.

### 8.7 h2spec re-run

08.2 modifies HCM (per Decision 7: Inc/Dec hooks at `decodeHeaders`/`encodeFinalize`). The hooks add ~5 LoC and do NOT touch the H2 codec, the H2 framer, or the H2 hpack path. The h2spec gate at 53/53 PASS must remain green; re-running is mechanical (gate (c) per ADR-0051). The 08.2 SPEC §3 (gate-checklist specialization) confirms.

### 8.8 Differential 0000–0009 + 0010

All pre-existing fixtures `0000–0009` remain green (no regression). NEW fixture `0010-graceful-drain` is differentially green per the §7 driver flow.

### 8.9 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Standard six-gate sweep applies; the 08.2 SPEC's §3 will specialize per-gate (mirrors 08.1 SPEC §3).

---

## 9. ADRs anticipated

The 08.2 SPEC anticipates **seven to nine ADRs** (depending on consolidation choices) starting at **ADR-0091** (next-free per `STATE.md`'s `last-commit: 70e6a65` which closed at ADR-0090 per `DECISIONS.md` line 3538). The nine candidates are listed below; the SPEC author may consolidate to seven (the first-pass recommendation matches the parent BRAINSTORM §3 stub's "5–7 ADRs" target).

- **ADR-0091 — Drain state-machine shape (LIVE / DRAINING / DRAINED) + new `internal/drain/` package + LBP-1 fifth-application threading.** Anchors Decision 1 + Decision 4. The `internal/drain.Manager` type, three-state shape, atomic state + atomic inflight + chan Done, and the constructor-widening of `admin.New` + `listener.NewManager...` + HCM + TCP proxy. (Decision 1 + Decision 4.)
- **ADR-0092 — SIGTERM and SIGINT both trigger drain-then-exit; signal.NotifyContext registration unchanged structurally; <-ctx.Done() body upgraded.** Anchors Decision 2. (Decision 2.)
- **ADR-0093 — POST /drain_listeners contract: 200 OK fire-and-forget; idempotent; ?graceful=true silent-ignored; body verbatim per empirical pin.** Anchors Decision 3. (Decision 3.)
- **ADR-0094 — Listener stop-accepting via per-runtime Accept-loop fast-path on dm.IsDraining(); listener-socket close stays at Stop() (post-drain teardown).** Anchors Decision 5. (Decision 5.)
- **ADR-0095 — Drain timeout default: hardcoded 30s in envoy-go MVP; Envoy v1.37.2 default is 600s; deliberate divergence; operator-knob deferred.** Anchors Decision 6. (Decision 6.)
- **ADR-0096 — In-flight-completion discipline: HCM decodeHeaders/encodeFinalize Inc/Dec pair per request; TCP-proxy OnNewConnection/OnConnectionClose Inc/Dec pair per connection; cluster.Manager.Drain best-effort upstream-pool close after <-Done().** Anchors Decision 7 + Decision 8. (Decisions 7 + 8 consolidated.)
- **ADR-0097 — /ready DRAINING-state body extension + DRAINING-precedence-over-PRE_INITIALIZING-and-LIVE rule. Partially supersedes ADR-0015.** Anchors Decision 9. (Decision 9.)
- **ADR-0098 — /server_info `state` field DRAINING transition; deriveState extended to consult drain.Manager. Amends ADR-0088 (purely additive; not superseding).** Anchors Decision 10. (Decision 10.)
- **ADR-0099 — Hot-restart deferral; envoy-go's drain is one-process scope only; future runtime/hot-restart family delivers SCM_RIGHTS-based handoff.** Anchors Decision 11. (Decision 11.)

**Consolidation candidates (SPEC author may merge):**
- ADR-0091 + ADR-0094 (drain-state machine + listener stop-accepting) into a single "drain state machine + Listener Accept-loop fast-path" ADR — both are about the LIVE → DRAINING transition observability.
- ADR-0096 already consolidates Decisions 7 + 8; further consolidation with ADR-0091 is possible but loses topical clarity.
- ADR-0099 (hot-restart deferral) could be folded into ADR-0089 (the 08.1 deferral list) as an in-place amendment per ADR-0089's consequence (b). The SPEC author chooses based on whether the deferral has its own justification (it does — Decision 11's MVP-scope rationale) or simply extends 08.1's list.

If the SPEC author consolidates aggressively, the count lands at **seven** (ADR-0091, ADR-0092, ADR-0093, ADR-0095, ADR-0096, ADR-0097, ADR-0098); ADR-0094 folds into ADR-0091; ADR-0099 folds into ADR-0089. The consolidation choice is a SPEC-time decision not load-bearing for this brainstorm.

**Inline supersessions / amendments anticipated:**
- ADR-0015 (pre-init contract for /ready) — partially superseded by ADR-0097 (DRAINING extension). The pre-init contract for LIVE/PRE_INITIALIZING is preserved verbatim; ADR-0097 adds the DRAINING branch and the precedence rule.
- ADR-0088 (/server_info body shape) — amended by ADR-0098. Per ADR-0088 consequence (c) verbatim, the amendment is purely additive (DRAINING enum value + deriveState extension); no other field changes. The amendment is recorded as an in-place edit of ADR-0088's Consequences section per the ADR-0089 consequence (b) pattern (in-place edit per ADR-0052's BEHAVIOR_CONTRACT precedent applied to ADR text).
- ADR-0089 (admin-endpoint deferral list) — POST /drain_listeners line flips from "08.2 (graceful drain)" to "delivered in 08.2 per ADR-0093." Per ADR-0089 consequence (b), the table is updated in-place; no new ADR for the disposition flip.
- ADR-0085 (admin-mux reuse + LBP-1 third application) — consequence (a) extended to enumerate the 08.2 fifth-application threading of `*drain.Manager`. Per the LBP-1 generalization pattern, the extension is in-place; no new ADR for the discipline.
- ADR-0090 (no-ACL admin-endpoint security posture) — extended to cover POST /drain_listeners (the new mutating endpoint) with the same no-ACL discipline. Per ADR-0090's existing consequence (b) ("ADR-0089's mutating-endpoint table is gated on this ADR's eventual partial supersession"), the POST /drain_listeners endpoint inherits ADR-0090's posture without amendment. The 08.2 SPEC's BEHAVIOR_CONTRACT additions cite ADR-0090 for the new endpoint's no-ACL posture.

(Phase 06.1 had 6 ADRs; 06.2 had 4; 07.1 had 7; 07.2 had 7; 08.1 had 7. **7–9 sits at the high end** — appropriate for a phase that introduces a new package + a new admin endpoint + two endpoint extensions + two amendments to existing ADRs (ADR-0015 + ADR-0088). The SPEC author may consolidate at SPEC time.)

---

## 10. Out-of-scope (deferred per ADR-0040 format)

Beyond the §1.2 summary, the exhaustive deferral list (mirroring 08.1 SPEC §2's structure):

| Item | Deferred to | Rationale |
|---|---|---|
| Hot restart / parent-child handoff (SCM_RIGHTS, shared-memory state, parent-shutdown orchestration) | Runtime / hot-restart family per BOOTSTRAP_PROMPT.md §9 | Multi-phase; ~500 LoC minimum; would inflate 08.2 past ADR-0045 split threshold. ADR-0099 records. |
| `POST /quitquitquit` admin endpoint | Future admin-extensions phase or never | Semantic overlap with SIGTERM + /drain_listeners; no current operator workflow demand. ADR-0089 extended. |
| Per-listener selective drain (`/listeners/<name>/drain`) | Future admin-extensions phase | Finer-grained operator workflow; non-MVP. ADR-0089 already records. |
| `drain_strategy` per-listener (IMMEDIATE vs GRADUAL) | Future admin-extensions phase | Default GRADUAL only; IMMEDIATE strategy adds operator-mode plumbing. |
| Configurable drain timeout (CLI flag or bootstrap field) | Future operator-affordances phase | Hardcoded 30s in MVP; ADR-0095 records the divergence from Envoy's 600s. |
| Connection-level drain windows (per-conn drainable closure at next idle window) | Future runtime / hot-restart family | Envoy supports per-conn drain timing; envoy-go MVP closes downstream-conn at response completion (with Connection: close header per §11.3). |
| Drain manager interaction with xDS (xDS-driven drain / SDS rotation triggering drain / etc.) | xDS family | No xDS yet. |
| `POST /quit` / `POST /shutdown` variant endpoints | Never (semantic overlap; no Envoy precedent at v1.37.2) | |
| Per-listener drain stats counters (e.g., `listener.<name>.drain_close_total`) | Future stats-hardening phase | No current ROADMAP row; ADR-0063's per-endpoint-stats deferral applies. |
| Drain admin endpoint POST body parsing (e.g., `{"timeout":"60s"}` payload) | Future admin-extensions phase | Envoy's POST /drain_listeners accepts no body at v1.37.2 per empirical pin §11.1. |
| GOAWAY frame timing customization on H2 connections during drain | Future H2-tuning phase or runtime/hot-restart family | Default GOAWAY-on-Drain timing per empirical pin §11.3; configurability deferred. |
| HTTP/3 drain semantics | HTTP/3 + QUIC family per BOOTSTRAP_PROMPT.md §9 | No H3 in MVP. |
| Multi-instance drain coordination (e.g., shared-state coordination across envoy-go instances during a fleet-wide drain) | Never (cross-instance coordination is the operator's load balancer's responsibility) | |
| `Connection: drain` custom header on responses during drain | Future H1-tuning phase | Empirical pin §11.3 settles whether Envoy emits this; if no, envoy-go matches. |
| Drain progress JSON body on /server_info (e.g., `drain_progress: {inflight: 3, started_at: ...}`) | Future admin-extensions phase | Envoy emits no such field at v1.37.2; envoy-go matches. |

---

## 11. Empirical-pin obligations (SPEC-author work)

The 08.2 SPEC author (next session, lifecycle-state 1 → 2, skill `superpowers:writing-plans`) executes the seven empirical pins below IN-SESSION against reference Envoy v1.37.2 per ADR-0004's hard-gate discipline. Reference image SHA: `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per ENVOY_TARGET.md). Server-build SHA confirmed by 08.1 SPEC §11.4 line 739: `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`. The probe configuration uses the §7.1 fixture-bootstrap shape with the slow-streaming backend.

Each pin lists: **(a)** what to scrape, **(b)** what to verify, **(c)** where the verbatim evidence lands in BEHAVIOR_CONTRACT.md and the 08.2 SPEC.

### 11.1 POST /drain_listeners response body verbatim

**(a) Scrape:** `curl -i -X POST http://127.0.0.1:9901/drain_listeners` against an Envoy v1.37.2 boot. Capture the full HTTP response (status line + headers + body) verbatim.
**(b) Verify:** the body bytes (likely `OK\n` or `{}\n`); the response status (likely 200); the Content-Type header (likely `text/plain; charset=UTF-8` if `OK\n`); the framing (transfer-encoding: chunked vs Content-Length). The empirical evidence settles Decision 3's body-shape candidate.
**(c) Lands in:** 08.2 SPEC §11.1 verbatim; BEHAVIOR_CONTRACT.md `## Admin API ### /drain_listeners` body-shape paragraph (§6.1 above).

### 11.2 /ready DRAINING-state response body verbatim

**(a) Scrape:** boot Envoy v1.37.2; `curl -X POST http://127.0.0.1:9901/drain_listeners` to trigger drain; immediately `curl -i http://127.0.0.1:9901/ready`. Capture full response.
**(b) Verify:** the body bytes (likely `Draining\n` or `DRAINING\n` — case TBD); the response status (likely 503 — matching the existing PRE_INITIALIZING branch); any DRAINING-specific headers (does Envoy emit `Connection: close` on the /ready response itself? Likely no; /ready is a one-shot probe, not a keepalive surface).
**(c) Lands in:** 08.2 SPEC §11.2 verbatim; BEHAVIOR_CONTRACT.md `## Admin API ### /ready` DRAINING extension paragraph (§6.2 above).

### 11.3 In-flight HTTP request behavior during drain

**(a) Scrape:** boot Envoy v1.37.2 + slow-streaming backend; open a long-lived `GET /slow` request (5KB streaming at 1KB/s); mid-stream, trigger `POST /drain_listeners`; capture the response stream + headers as it completes (a packet-capture or `curl -v` may be needed to see the connection-disposition header).
**(b) Verify:**
- Does Envoy emit `Connection: close` on the in-flight HTTP/1.1 response during drain?
- Does Envoy emit a GOAWAY frame on existing HTTP/2 connections at drain trigger? If yes, when (immediately, or after the current stream completes)?
- Does the in-flight request complete normally (full body delivery, 200 status), or is it aborted with some error status?

**(c) Lands in:** 08.2 SPEC §11.3 verbatim; BEHAVIOR_CONTRACT.md `## Graceful drain ### Drain semantics` connection-level paragraph (§6.4 above).

### 11.4 POST /drain_listeners method-discrimination behavior

**(a) Scrape:** issue `GET /drain_listeners` and `PUT /drain_listeners` and `DELETE /drain_listeners` against Envoy v1.37.2. Capture each response.
**(b) Verify:** Does Envoy enforce method discrimination (returning 405 for non-POST)? Or does it accept any method and trigger drain (mirroring the read-only-endpoint Envoy-parity posture per 08.1 SPEC §11.8)? If GET is accepted, does it ALSO trigger drain (the operator could accidentally drain via a routine GET scrape — important to know).
**(c) Lands in:** 08.2 SPEC §11.4 verbatim; BEHAVIOR_CONTRACT.md `## Admin API ### /drain_listeners` method-discrimination paragraph + ADR-0093 consequence section.

### 11.5 New connection rejection mechanism during drain

**(a) Scrape:** boot Envoy v1.37.2; trigger drain; `curl -v` against the listener port to attempt a new connection. Capture connect-level error semantics: TCP RST (kernel-level refused), TCP FIN after handshake (graceful close), or connection-pool-exhausted style 503.
**(b) Verify:** does Envoy close the listening socket, or accept-and-immediately-close the new conn? If accept-and-close, does the close emit any HTTP-layer response (e.g., `503 Service Unavailable`)?
**(c) Lands in:** 08.2 SPEC §11.5 verbatim; BEHAVIOR_CONTRACT.md `## Graceful drain ### Drain semantics` new-connections paragraph + ADR-0094 consequence section.

### 11.6 Header set across the /drain_listeners endpoint + DRAINING /ready response

**(a) Scrape:** capture the full header set on the `POST /drain_listeners` response (from §11.1) and on the DRAINING `/ready` response (from §11.2).
**(b) Verify:** does the header set match the existing 08.1 admin-endpoint umbrella header set (`content-type`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `date: <IMF-fixdate>`, `server: envoy`)? Are there DRAINING-specific headers? Is the framing `transfer-encoding: chunked` (matching 08.1 SPEC §11.5 across the four 08.1 endpoints) or `Content-Length`?
**(c) Lands in:** 08.2 SPEC §11.6 verbatim; BEHAVIOR_CONTRACT.md `## Admin API` umbrella header-set paragraph (extended from 08.1's enumeration) + the `## Graceful drain` umbrella's framing-deviation paragraph.

### 11.7 SIGTERM-vs-SIGINT distinct behavior in Envoy + drain timeout default

**(a) Scrape:** boot Envoy v1.37.2; observe behavior under `kill -TERM` vs `kill -INT`. Specifically: does each signal trigger graceful drain identically? Does either signal trigger immediate exit (no drain)? Does the timeout differ between the two?
**(b) Verify:**
- Both SIGTERM and SIGINT trigger graceful drain → exit (Decision 2's hypothesis).
- The drain timeout default is 600s (per 08.1 SPEC §11.4 line 760 verbatim `"drain_time": "600s"`).
- The `command_line_options.drain_strategy: "Gradual"` (per 08.1 SPEC §11.4 line 773 verbatim) is the default — confirm Gradual is the only strategy in the v1.37.2 default-config flow, not Immediate.

**(c) Lands in:** 08.2 SPEC §11.7 verbatim (signal-handling evidence); BEHAVIOR_CONTRACT.md `## Graceful drain ### Drain triggers` paragraph + ADR-0092 consequence section + ADR-0095 (timeout default) consequence section.

### Synchronization with BEHAVIOR_CONTRACT.md

The §11.1–§11.7 verbatim blocks above are paste-verbatim-synchronized with the BEHAVIOR_CONTRACT.md `## Admin API ### /drain_listeners` + `### /ready` + `### /server_info` + `## Graceful drain` sections that 08.2's implementation lands. No drift permitted: future image bumps (per ADR-0008's pin-refresh procedure) require re-running the seven probes and updating both the 08.2 SPEC §11 and BEHAVIOR_CONTRACT.md `## Graceful drain` in the same commit. (Mirrors 08.1 SPEC §11.10's resync discipline.)

---

## 12. Hand-off to writing-plans

Next session (lifecycle-state 1 → 2 for sub-phase 08.2, skill `superpowers:writing-plans`) authors:

- `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` — full sub-phase SPEC derived from §§1–11 of this BRAINSTORM, including the §11 empirical-pin obligations executed IN-SESSION per ADR-0004. The SPEC supersedes the existing sibling stub `docs/envoy-go/phases/08.2-graceful-drain/README.md` (the stub becomes read-only history at the SPEC commit per its own §1).

The SPEC's section-numbering convention is expected to mirror the 08.1 SPEC structure (per `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md`):
- §1 Purpose
- §2 Non-purposes (08.2 deferral list per §10 here)
- §3 Phase-done gates (specialization of BOOTSTRAP_PROMPT.md §7.5)
- §4 Deliverables (files; per §3 surface inventory here)
- §5 Architecture and components (per §1 + §3 here, with the per-actor swimlanes promoted to formal Architecture diagrams)
- §6 Per-endpoint contract summary (per §6 BEHAVIOR_CONTRACT additions here, in formal contract-language)
- §7 Differential fixture (per §7 here)
- §8 ADRs anticipated (per §9 here)
- §9 Out-of-scope (per §10 here)
- §10 Carry-forward dispositions (per §4 here)
- §11 Empirical-pin block (per §11 here, with verbatim Envoy v1.37.2 scrape evidence pasted in)
- §12 Deferred decisions (the planner / implementer settles)
- §13 BEHAVIOR_CONTRACT.md additions (verbatim Markdown patch; per §6 here in formal patch language)
- §14 Testing strategy (per §8 here)
- §15 Acceptance checklist
- §16 References

After the SPEC lands, lifecycle-state advances to 2 (PLAN.md authoring) per `BOOTSTRAP_PROMPT.md` §5; STATE.md flips `next-skill: superpowers:writing-plans` (PLAN.md), `lifecycle-state: 2`, `active-phase: 08.2-graceful-drain`, `last-commit: <08.2 SPEC commit SHA>`. The PLAN.md authoring session estimates task count + LoC; per ADR-0045's split-gate, if PLAN.md exceeds ~25 tasks OR ~1500 LoC, the SPEC author must split 08.2 further (unlikely given 08.2's surface is well-bounded — estimated 12–15 tasks + ~600–800 LoC production + ~300–400 LoC fixture/driver per the §3 surface inventory).

This BRAINSTORM.md is committed as the brainstorm-close artifact and is read-only history once the next session starts. Future sessions consult it as the authoritative record of the design decisions made here. Per D-3.4 (context isolation), the SPEC author reads this BRAINSTORM in full + the parent-08 BRAINSTORM + the 08.1 closed artefacts + the sibling SPEC stub + the relevant code state to produce the SPEC; no conversational context is presumed.
