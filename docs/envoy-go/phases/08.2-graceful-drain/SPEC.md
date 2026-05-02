# Phase 08.2 — Graceful drain (`internal/drain/`, `POST /drain_listeners`, `/ready` + `/server_info` DRAINING extensions, SIGTERM-handler upgrade)

**Phase id:** `08.2`
**Slug:** `08.2-graceful-drain`
**Status:** `in-progress` (SPEC stage; ROADMAP row `08.2` flips `planned → in-progress` at this commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md (`docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md`) §§2–11 into formal SPEC shape, executing the seven §11 empirical-pin obligations against reference Envoy v1.37.2 in-session per ADR-0004)
**Depends on:** phase 08.1 (done at master `70e6a65` — 08.1 phase-done close; SHA-fill follow-up at master `eb3babd`). Specifically, 08.1's admin-mux scaffold (the `*http.ServeMux` allocated by `internal/admin.Server.Start()` carrying six `mux.HandleFunc(...)` registrations) is the host structure 08.2 extends with `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)`. The constructor-widening pattern (LBP-1) settled by ADR-0085 is the discipline for threading `*drain.Manager` into `admin.New`, `listener.NewManager...`, and HCM/TCP-proxy filter constructors at 08.2 (the LBP-1 fifth application).
**Parent phase:** `08-admin-api-and-drain` — parent-master SPEC at `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md`. Per parent §5, parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2's phase-done; 08.2's phase-done is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit.
**Master design document:** `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` (autonomous-brainstorm artifact per ADR-0004; this SPEC distills BRAINSTORM §§2–11 into formal contract language and executes the §11 empirical-pin obligations IN-SESSION).
**Differential surface at end of sub-phase:** ROADMAP row `08.2` flips `in-progress → done` at the phase-done commit; parent row `08` simultaneously flips `in-progress → done` (MVP-trunk closure). NEW differential fixture `0010-graceful-drain` (per-state-transition equivalence under a slow-streaming-backend driver against the §7.1 fixture bootstrap — admin-trigger and SIGTERM-trigger driver paths in one fixture per BRAINSTORM Decision 12) is differentially green. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe`, `0008-listener-chain-match`, `0009-admin-config-dump` all green. The h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS (08.2's HCM Inc/Dec hooks are non-load-bearing for H2 codec correctness; the gate re-run is mechanical). Existing 10 fuzzers re-run clean; one OPTIONAL new fuzzer `FuzzDrainTransitions` is settled per §12 deferred decision. `BEHAVIOR_CONTRACT.md ## Admin API` umbrella (08.1 restructure) gains a new `### /drain_listeners` subsection + extends `### /ready` (DRAINING-state body) + extends `### /server_info` (DRAINING-state field); a new sibling `## Graceful drain` umbrella section captures drain-state-machine semantics independent of the admin API; three new equivalence-matrix rows.

---

## 1. Purpose

Phase 08.2 lands graceful-drain semantics — the lifecycle discipline that moves envoy-go from "kill -TERM means hard exit" to "kill -TERM (or POST /drain_listeners) means stop accepting new connections, finish in-flight requests, then exit cleanly (or stay running in DRAINING for the operator-driven flow)." The five new architectural primitives:

1. A new `internal/drain/` package owning the drain-state machine. The package exports a `Manager` type with three states (`LIVE`, `DRAINING`, `DRAINED`) backed by a lock-free `atomic.Uint32` plus an `atomic.Int64` in-flight counter and a `chan struct{}` rendezvous channel that closes on `DRAINING → DRAINED`. The state machine is the LBP-1 fifth application (after `*stats.Registry`, `*HTTPRegistry`, `*ListenerFilterRegistry`, and the 08.1 `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` triplet threaded through `admin.New`); see ADR-0091.
2. A `cmd/envoy-go/main.go` SIGTERM-handler upgrade. The current `<-ctx.Done()` + deferred `lm.Stop()` flow is replaced by `<-ctx.Done()` → `drainMgr.Drain()` → `select { <-drainMgr.Done(): / <-time.After(timeout): }` → `cm.Drain()` → existing deferred-stop chain. SIGTERM and SIGINT both trigger this drain-then-exit flow (per BRAINSTORM Decision 2; see ADR-0092). This is a DELIBERATE DIVERGENCE from upstream Envoy v1.37.2 (per §11.7 empirical pin: Envoy's SIGTERM is immediate-exit-without-drain); envoy-go's choice is more operator-friendly and is documented as a contract-level divergence in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Drain triggers`. The differential equivalence claim is over `/drain_listeners` body shape + DRAINING-state body/field shapes + new-connection-rejection mechanism (per §7.1), NOT over signal-handler semantics.
3. A `POST /drain_listeners` admin endpoint. Mutating endpoint that triggers `drainMgr.Drain()` synchronously and returns 200 OK with body `OK\n` (per §11.1 empirical pin) BEFORE drain completes. ENVOY-FAITHFUL method discrimination: GET/PUT/DELETE return `405 Method Not Allowed` with body `Method <X> not allowed, POST required.` (per §11.4 empirical pin — this is the FIRST admin endpoint in envoy-go with method enforcement; partially amends ADR-0090's no-method-discrimination posture). The endpoint does NOT trigger process exit — operator-driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT (or kill -9). Idempotent.
4. `/ready` and `/server_info` DRAINING-state extensions. When `drainMgr.State() == DRAINING`, `/ready` returns `503 Service Unavailable` with body `DRAINING\n` (verbatim per §11.2 empirical pin); `/server_info` returns `state: "DRAINING"` (per §11.2 empirical pin re-test post-healthcheck-fail). DRAINING precedence: DRAINING > LIVE > PRE_INITIALIZING. ADR-0097 partially supersedes ADR-0015 (the pre-init contract); ADR-0098 amends ADR-0088 (state-enum coverage).
5. `internal/listener.Manager.Drain()` and `internal/cluster.Manager.Drain()` accessors. Listener `Drain()` triggers stop-accepting via the per-runtime Accept-loop fast-path checking `dm.IsDraining()` — accepted connections during DRAINING are immediately closed without filter-chain dispatch (matches §11.5 empirical-pin "TCP accept then connection-FIN with no HTTP-layer body"). The existing `Listener.Manager.Stop()` method stays unchanged as the post-drain teardown step. Cluster `Drain()` is a best-effort post-drain upstream-pool close (after `<-drainMgr.Done()` fires). HCM `decodeHeaders`/`encodeFinalize` and TCP-proxy `OnNewConnection`/`OnConnectionClose` carry the per-request-or-per-connection Inc/Dec hooks against the drain manager's in-flight counter (per §11.3 empirical pin: Envoy's in-flight requests complete normally with full-body delivery and NO `Connection: close` on the H1.1 response — envoy-go matches).

After phase 08.2, the project has proven its tenth-leading-edge engineering claim: *envoy-go's lifecycle discipline supports operator-driven drain-without-exit (POST /drain_listeners) and signal-driven drain-then-exit (SIGTERM/SIGINT) under a unified three-state lock-free drain manager, with /ready and /server_info DRAINING-state extensions matching upstream Envoy v1.37.2's body shapes byte-for-byte and new-connection rejection matching upstream's accept-then-FIN mechanism, while preserving the 08.1 admin-mux scaffold and the LBP-1 explicit-threading discipline.* This is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit; after 08.2 lands, STATE.md flips to `awaiting next planning` per the §5 lifecycle state machine.

---

## 2. Non-purposes

Per `BOOTSTRAP_PROMPT.md` §6.3 (scope-bounding) and ADR-0040 (out-of-scope deferrals format), the following are explicitly out of 08.2's scope:

### 2.1 Lifecycle non-goals (per BRAINSTORM §10 + sibling-stub §4)

- **Hot restart / parent-child handoff.** Requires SCM_RIGHTS file-descriptor transfer, shared-memory state for the existing-connection table, parent-shutdown-time orchestration, and a custom signal protocol (e.g. `SIGUSR1`). Multi-phase deliverable; deferred to BOOTSTRAP_PROMPT.md §9's "Runtime + hot restart family" per ADR-0099.
- **`POST /quitquitquit` admin endpoint.** Semantic overlap with SIGTERM + `/drain_listeners`; no current operator-workflow demand; deferred per the ADR-0089 deferral list extension.
- **Per-listener selective drain (`/listeners/<name>/drain`).** Finer-grained operator workflow; deferred per ADR-0089.
- **`drain_strategy` per-listener (IMMEDIATE vs GRADUAL).** Default GRADUAL only; IMMEDIATE strategy adds operator-mode plumbing not justified by current operator workflows.
- **Configurable drain timeout (CLI flag, bootstrap field, or POST body).** Hardcoded 30s in envoy-go MVP per ADR-0095; operator-knob deferred to a future operator-affordances phase.
- **Connection-level drain windows.** Envoy supports per-connection drainable closure at the next idle window; envoy-go MVP closes downstream connections at response completion (with whatever Connection-disposition header the empirical pin §11.3 settles — see §6.5 contract paragraph).
- **Drain manager interaction with xDS (SDS rotation triggering drain, RTDS-driven drain, etc.).** No xDS yet; deferred to xDS family.
- **Drain admin endpoint POST body parsing (e.g. `{"timeout":"60s"}`).** Envoy's POST /drain_listeners accepts no body at v1.37.2; envoy-go matches.
- **Per-listener drain stats counters (e.g. `listener.<name>.drain_close_total`).** No current ROADMAP row; ADR-0063's per-endpoint-stats deferral applies.
- **GOAWAY frame timing customization on H2 connections during drain.** §11.3 empirical pin's H1.1-only observation does not pin H2 GOAWAY timing; configurability deferred. envoy-go MVP emits GOAWAY at drain-trigger on existing H2 connections (the natural disposition for a server-side drain — clients learn no new streams accepted), but the timing is not asserted differentially.
- **HTTP/3 drain semantics.** No H3 in MVP; deferred to HTTP/3 + QUIC family.
- **Multi-instance drain coordination.** Cross-instance drain (e.g. fleet-wide drain via shared state) is the operator's load balancer's responsibility; never in envoy-go scope.
- **`Connection: drain` custom response header during drain.** §11.3 empirical pin shows Envoy emits no such header; envoy-go matches.
- **Drain progress JSON body on /server_info (e.g. `drain_progress: {inflight: N, started_at: ...}`).** Envoy emits no such field at v1.37.2; envoy-go matches.

### 2.2 Empirically observed cross-trigger non-purposes (per §11.2 finding)

- **Wiring `/healthcheck/fail` POST endpoint.** Per §11.2 empirical pin, upstream Envoy v1.37.2 ties `/ready` and `/server_info` DRAINING-state to `/healthcheck/fail`, NOT to `/drain_listeners` alone. envoy-go's MVP DOES NOT model `/healthcheck/fail` as a separate endpoint — instead, both `/drain_listeners` POST AND SIGTERM/SIGINT directly trigger the unified DRAINING transition (via `drainMgr.Drain()`), which is the simpler envoy-go-internal design (single drain manager, single state). The `/healthcheck/fail` endpoint is deferred per ADR-0089's existing deferral table; its absence is documented in `BEHAVIOR_CONTRACT.md ## Admin API ### Does not yet apply to`.

### 2.3 Read-only-endpoint surface non-purposes

- **`/listeners/<name>/drain` per-listener drain sub-routes.** Per ADR-0089 deferral.
- **`?graceful=true` query-param distinction.** Envoy supports `?graceful=true` to switch between drain modes; envoy-go's drain is always graceful by construction (per Decision 1's three-state machine has no non-graceful immediate-stop variant). The query-param is silently accepted (per ADR-0041's silent-ignore precedent); the differential harness asserts identical body shape with and without the query-param.

### 2.4 Transport-level non-purposes

- **HTTP/2 over admin (admin stays HTTP/1.1 — phase-01 contract; 08.1 §2.4 inherited).**
- **TLS on admin (admin stays plaintext).**
- **Compression on admin responses.**
- **Streaming responses for /drain_listeners.** The handler is fire-and-forget — body is `OK\n` (10 bytes) buffered before write.

### 2.5 Security non-purposes

- **Authentication / ACL on the new mutating endpoint.** `/drain_listeners` inherits the no-ACL plaintext HTTP/1.1 posture per ADR-0090 (08.1's security-posture decision; partially amended by ADR-0093 to record that the mutating endpoint gains no ACL but DOES gain method discrimination per §11.4 — the operator firewall is the security boundary; method discrimination is upstream-Envoy parity).
- **Request rate limiting on /drain_listeners.** No rate limiting on admin endpoints in MVP.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 08.2)

| Gate | 08.2 specialization |
|---|---|
| **(a)** `go build ./...` clean | Including the new `internal/drain/` package (`doc.go`, `manager.go`), the new `internal/admin/drain.go` handler, the modified `internal/admin/admin.go` (constructor widening; new mux registration; modified `handleReady`), the modified `internal/admin/serverinfo.go` (`deriveState` extension), the modified `internal/listener/manager.go` (Drain method; per-runtime Accept-loop fast-path; constructor widening; N-1 doc-comment fix), the modified `internal/cluster/manager.go` (Drain method), the modified HCM filter (`internal/filter/hcm/hcm.go` or equivalent — Inc/Dec hooks at decodeHeaders/encodeFinalize; constructor widening), the modified TCP proxy filter (`internal/filter/tcpproxy/filter.go` — Inc/Dec hooks at OnNewConnection/OnConnectionClose; constructor widening), and the modified `cmd/envoy-go/main.go` (drainMgr allocation; threaded into manager constructors; SIGTERM-handler block upgrade). All under `go vet ./...` clean and `golangci-lint run ./...` clean. |
| **(b)** `go test ./...` clean | New unit tests in `internal/drain/manager_test.go` covering state transitions / Inc-Dec balance / Done-rendezvous / timeout / idempotent-Drain / IsDraining-fast-path / nil-safety. New `internal/admin/drain_test.go` for the POST handler (200 + body + idempotent + method-405 + nil-tolerant). Modified `internal/admin/admin_test.go` for the DRAINING /ready branch + DRAINING-precedence-over-LIVE-and-PRE_INITIALIZING. Modified `internal/admin/serverinfo_test.go` for the DRAINING state-enum value + precedence. Modified `internal/listener/manager_test.go` for Drain/IsDraining/idempotency. Modified `internal/cluster/manager_test.go` for Drain pool-closure. Concurrent-scrape race-test extended to seven endpoints (six pre-08.2 + /drain_listeners) under 100 goroutines × 1s, plus a separate goroutine firing Drain() once mid-test. `go test -race ./...` clean. |
| **(c)** h2spec re-run clean (53/53 PASS at ADR-0051 pin) | 08.2's HCM Inc/Dec hooks are tiny (~5 LoC) and non-load-bearing for the H2 codec / framer / hpack path. The h2spec gate at 53/53 PASS must remain green; re-running is mechanical (gate (c) per ADR-0051). The CONFORMANCE_PINS pin is unchanged. |
| **(d)** new/existing fuzzers run clean for CI short-budget | Existing 10 fuzzers (per 08.1 REVIEW gate (d) appendix — `FuzzBootstrapLoad`, `FuzzTcpProxyFilter`, `FuzzTLSContextParse`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`, `FuzzPromTextFormat`, `FuzzAccessLogFormat` (or `FuzzDefaultFormatRender` per the 08.1 PLAN-doc-erratum), `FuzzFilterChainParse`, `FuzzFilterChainMatch`, `FuzzConfigDumpFormat`) re-run clean at the 30s ADR-0018 budget. **NEW (OPTIONAL):** `FuzzDrainTransitions` (~60 LoC; 30s budget) — fuzzes a sequence of operations against `*drain.Manager` (Drain, Inc, Dec, IsDraining, Done) and asserts state-monotonicity + inflight-balance + Done-fires-once invariants. Per §12 deferred decision #1, the SPEC author RECOMMENDS shipping this fuzzer (the state-machine surface is small but the concurrent operations have a non-trivial interleaving space; ADR-0018's "every parser/codec/filter ships a fuzzer" is generalized to "every concurrent state machine ships a fuzzer where reasonable"). Total fuzzer count post-08.2: **11**. |
| **(e)** Differential fixtures all green | All pre-existing fixtures `0000–0009` remain green (08.2's HCM Inc/Dec hooks are no-op when the drain manager is nil, which is the test-construction default for the existing fixtures' driver setups). **NEW:** `0010-graceful-drain` (`test/differential/0010-graceful-drain/`) is green under the per-state-transition equivalence claims of §7.1 — admin-trigger and SIGTERM-trigger driver paths in one fixture per BRAINSTORM Decision 12. The `RequiresReference: true` flag is set in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors `0007a-cors`, `0009-admin-config-dump`). |
| **(f)** `BEHAVIOR_CONTRACT.md` populated | `## Admin API` umbrella (08.1 restructure host) gains a new `### /drain_listeners` subsection + an extension paragraph appended to `### /ready` (DRAINING body) + an extension paragraph appended to `### /server_info` (DRAINING state). New sibling `## Graceful drain` umbrella section added (per §13.4) covering drain-state-machine semantics independent of the admin API. Three new rows added to `## Equivalence Matrix` per §13.5. ADR-0015 forward-pointer note (partial supersession by ADR-0097). ADR-0088 amendment forward-pointer note (amended by ADR-0098 to add DRAINING enum coverage). ADR-0090 amendment forward-pointer note (partially amended by ADR-0093 to record the /drain_listeners method-discrimination divergence from the otherwise-uniform no-method-check posture). In-place edit per ADR-0052 — lands at the phase-done commit alongside the implementation. |

The phase-done commit message body must explicitly state that ROADMAP row `08.2` flips `in-progress → done` AT this commit AND that parent row `08` simultaneously flips `in-progress → done` (MVP-trunk closure per parent SPEC §5). Per `BOOTSTRAP_PROMPT.md` §5.3 commit message format. Commit subject: `phase 08.2: graceful-drain [ADR-0091, ADR-0092, ADR-0093, ADR-0094, ADR-0095, ADR-0096, ADR-0097, ADR-0098, ADR-0099]` (or fewer ADRs if the planner consolidates per §8 consolidation candidates).

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 08.2)

- `internal/drain/doc.go` — package doc enumerating the three-state drain machine, the LBP-1 fifth-application discipline, and the public API surface (`Manager`, `State`, `New`, `Drain`, `Done`, `Inc`, `Dec`, `IsDraining`, `Timeout`). ~30 LoC.
- `internal/drain/manager.go` — `Manager` type with fields per BRAINSTORM Decision 1: `state atomic.Uint32`, `inflight atomic.Int64`, `done chan struct{}`, `timeout time.Duration`, `once sync.Once`. Constructor `New(timeout time.Duration) *Manager`. Public methods: `State() State` (atomic load); `Drain()` (sync.Once-guarded; CAS Live → Draining; arms Done-watcher goroutine OR fires Done immediately if inflight already 0); `Done() <-chan struct{}` (returns the done channel); `Inc()` (atomic add to inflight); `Dec()` (atomic add -1 to inflight; if inflight reaches 0 after a Drain has fired, close done if not already closed); `IsDraining() bool` (atomic load State == Draining); `Timeout() time.Duration` (returns the configured timeout). Lock-free hot path; the only synchronization beyond atomics is the `sync.Once` guard on Drain and the `sync.Once`-via-channel-close idempotency guard on `done`. ~120 LoC.
- `internal/drain/manager_test.go` — unit tests per §14.1. ~250 LoC.
- `internal/drain/fuzz_test.go` — OPTIONAL `FuzzDrainTransitions` fuzzer per §14.5 (per the §12 #1 deferred-decision recommendation, the SPEC ships this). ~60 LoC.
- `internal/admin/drain.go` — `handleDrainListeners` http.HandlerFunc + helpers. Method-discrimination check FIRST (return 405 with body `Method <X> not allowed, POST required.\n` for non-POST per §11.4 empirical pin); on POST, call `s.dm.Drain()`; emit 200 OK with body `OK\n` + the standard six-header set per §11.6. Body 3 bytes (`OK\n`); buffered then written. ~60 LoC.
- `internal/admin/drain_test.go` — unit tests per §14.2. ~150 LoC.

### 4.2 Changed production code (in 08.2)

- `cmd/envoy-go/main.go` — modified per BRAINSTORM Decision 2 + Decision 6 + Decision 8. New `drainMgr := drain.New(30 * time.Second)` allocation post-`bootstrap.Load`, BEFORE `cluster.NewManager...` (the drain manager has no dependencies). Threaded into `listener.NewManagerWithBaseDirAndAllowH2C(..., drainMgr)`, into `admin.New(adminAddr, registry, bs, cm, lm, drainMgr)`, and via the listener manager into the HCM filter-chain construction surface (the listener manager is the natural conduit because HCM is built from the listener bootstrap proto). The `<-ctx.Done()` block at line ~170 is upgraded per BRAINSTORM §5.1 swimlane: call `drainMgr.Drain()`; `select { <-drainMgr.Done(): / <-time.After(drainMgr.Timeout()): }`; call `cm.Drain()`; existing deferred-stop chain runs (lm.Stop / admSrv.Close / sinks-close). ~30 LoC delta.
- `internal/admin/admin.go` — `New` signature widened from `New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server` to `New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager, dm *drain.Manager) *Server`. New field on `Server`: `dm *drain.Manager`. `Start()` body adds `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` after the existing six registrations. `handleReady` body modified per BRAINSTORM Decision 9: NEW first branch — `if s.dm != nil && s.dm.State() == drain.StateDraining { write 503 + DRAINING\n; return }`; existing pre-init and ready branches preserved unchanged. ~30 LoC delta.
- `internal/admin/serverinfo.go` — `deriveState` signature widened from `deriveState(ready *atomic.Bool) adminv3.ServerInfo_State` to `deriveState(ready *atomic.Bool, dm *drain.Manager) adminv3.ServerInfo_State`. NEW first check: `if dm != nil && dm.State() == drain.StateDraining { return adminv3.ServerInfo_DRAINING }`; existing LIVE / PRE_INITIALIZING checks preserved unchanged. Call site at `buildServerInfo` updates from `deriveState(&s.ready)` to `deriveState(&s.ready, s.dm)`. ~5 LoC delta.
- `internal/admin/admin_test.go` — modified: existing tests preserved verbatim. New tests for the DRAINING /ready branch (`TestHandleReady_Draining`, `TestHandleReady_DrainingPrecedesLive`, `TestHandleReady_DrainingPrecedesPreInitializing`). Concurrent-scrape race-test (`TestAdminConcurrentScrapeRace`) extended to include `/drain_listeners` in the round-robin path-set + a separate goroutine firing `Drain()` once mid-test. ~120 LoC delta.
- `internal/admin/serverinfo_test.go` — modified: new tests for the DRAINING state-enum value (`TestHandleServerInfo_StateDraining`, `TestHandleServerInfo_StatePrecedence`). ~50 LoC delta.
- `internal/listener/manager.go` — `NewManagerWithBaseDirAndAllowH2C` signature widened to take `dm *drain.Manager` parameter (LBP-1 fifth application). New field on `Manager`: `dm *drain.Manager`. NEW method `(m *Manager) Drain()` — calls `m.dm.Drain()` (delegates to the central drain manager). The Accept loop body in each per-listener `runtime` checks `m.dm.IsDraining()` AT THE TOP of each iteration, AFTER Accept returns: if true, immediately `_ = conn.Close()` and `continue` (no filter-chain dispatch). Existing `Stop()` method preserved unchanged (post-drain teardown, called from the deferred-stop chain in `cmd/envoy-go/main.go`). N-1 carry-forward (08.1 REVIEW): one-line doc-comment on `Listeners()` saying "order is bootstrap-declaration order; callers needing alphabetical ordering must sort." ~30 LoC delta.
- `internal/listener/manager_test.go` — modified: new tests for `Drain()` and the Accept-loop fast-path (`TestManager_Drain`, `TestManager_DrainIdempotent`, `TestManager_AcceptDuringDrainClosesConn`). ~80 LoC delta.
- `internal/cluster/manager.go` — NEW method `(m *Manager) Drain()` — walks `m.clusters map[string]*Cluster` and calls `c.closePool()` (a per-cluster method also added in 08.2 — closes HTTP/1.1 keepalive pool, HTTP/2 ClientConns from phase 05.2, TLS connections from phase 03). Best-effort; no error return. ~30 LoC delta.
- `internal/cluster/manager_test.go` — modified: new test for `Drain()` (`TestManager_DrainClosesPools`). ~40 LoC delta.
- `internal/filter/hcm/hcm.go` (or equivalent — exact file path settled at PLAN time) — modified per BRAINSTORM Decision 7. HCM constructor widens to take `dm *drain.Manager` parameter. New field on `HCM`: `dm *drain.Manager`. New field on `Stream`: `markedInflight bool` (guards balanced Inc/Dec on all paths including sendLocalReply per ADR-0075). `decodeHeaders` body adds: `if h.dm != nil { h.dm.Inc(); stream.markedInflight = true }` BEFORE the filter chain runs. `encodeFinalize` body adds (AFTER the access-log emit per phase 06.2): `if h.dm != nil && stream.markedInflight { h.dm.Dec(); stream.markedInflight = false }`. ~15 LoC delta.
- `internal/filter/hcm/hcm_test.go` (or equivalent) — modified: new tests for the Inc/Dec balance under the various decode/encode paths including sendLocalReply (`TestHCM_DrainInflightBalance`, `TestHCM_DrainInflightBalance_SendLocalReply`). ~60 LoC delta.
- `internal/filter/tcpproxy/filter.go` — modified per BRAINSTORM Decision 7. TCP-proxy filter constructor widens to take `dm *drain.Manager` parameter. New field on `Filter`: `dm *drain.Manager`. `OnNewConnection` body adds `if f.dm != nil { f.dm.Inc() }`; `OnConnectionClose` body adds `if f.dm != nil { f.dm.Dec() }`. Per-connection granularity (correct for TCP-proxy because no per-request semantic exists). ~10 LoC delta.
- `internal/filter/tcpproxy/filter_test.go` — modified: new test for the Inc/Dec balance at connection lifetime (`TestTCPProxy_DrainInflightBalance`). ~30 LoC delta.

### 4.3 New harness and fixture code (in 08.2)

- `test/differential/0010-graceful-drain/README.md` — fixture overview + per-state-transition equivalence-claim narrative + dual-driver-path description (admin-trigger + SIGTERM-trigger) + the Envoy-deviation note (envoy-go's SIGTERM triggers drain; Envoy v1.37.2's SIGTERM is immediate-exit; the fixture's SIGTERM driver path therefore exercises ENVOY-GO ONLY, with the reference Envoy invocation skipped for the SIGTERM driver — only the admin-trigger driver runs against both proxies; the SIGTERM driver runs envoy-go-only as an internal-completeness check + assertion that envoy-go's drain rendezvous and exit ordering are correct). ~80 LoC.
- `test/differential/0010-graceful-drain/expectations.yaml` — per-endpoint tolerance discipline encoding the §13.5 allow-list + the per-state-transition assertion matrix (steady-state /ready byte-equal LIVE\n; POST /drain_listeners response byte-equal `OK\n`; /ready DRAINING byte-equal `DRAINING\n`; /server_info DRAINING state-field byte-equal `"DRAINING"`; in-flight-request body byte-equal). ~60 LoC.
- `test/differential/0010-graceful-drain/envoy.yaml` — reference Envoy bootstrap (admin :9902, listener :10001). Slow-streaming-backend cluster `c_backend` STRICT_DNS-pointing at the backend hostname per §7.1.
- `test/differential/0010-graceful-drain/envoy-go.yaml` — envoy-go bootstrap (admin :9901, listener :10000). Identical to `envoy.yaml` modulo admin/listener ports.
- `test/differential/0010-graceful-drain/driver/driver.go` — Go driver implementing the §7.2 dual-path orchestration: dual-proxy boot + slow-streaming-backend boot + admin-trigger path (POST /drain_listeners + /ready/server_info polling + new-conn rejection assertion + in-flight completion assertion + post-completion DRAINING assertion + cleanup) + SIGTERM-trigger path (envoy-go-only; SIGTERM injection + envoy-go's deferred-stop chain assertion + exit-status-0 assertion). Event-based synchronization throughout (no hardcoded sleeps per 07.2 REVIEW M-8 carry-forward + 08.1 SPEC §10). ~350 LoC.
- `test/differential/0010-graceful-drain/backends/backend.go` — minimal Go HTTP backend bound to port 18001. `/slow` endpoint streams 5KB at 1KB/s (5s total response time); `/` endpoint serves a fast `200 OK\nbackend1\n` for sanity. ~60 LoC.
- `test/fixtures/0010-graceful-drain/` (parallel directory under `test/fixtures/` if the project layout splits configs from drivers — single canonical location, however, per the existing 0007a / 0009 layout; SPEC does NOT prescribe two-directory split).
- `test/differential/runner.go` — `RegisterFixture("0010-graceful-drain", ..., Capabilities{RequiresReference: true})` registration line added per the existing fixture-registration pattern (mirrors 0007a-cors, 0009-admin-config-dump). ~3 LoC delta.

### 4.4 Changed documentation and state (in 08.2)

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — in-place edit per ADR-0052: (a) `## Admin API ### /drain_listeners` NEW subsection per §13.1; (b) `## Admin API ### /ready` extension paragraph per §13.2; (c) `## Admin API ### /server_info` extension paragraph per §13.3; (d) NEW sibling `## Graceful drain` umbrella section per §13.4; (e) three new equivalence-matrix rows at the head of the file per §13.5; (f) ADR-0015 forward-pointer note (partial supersession by ADR-0097); (g) ADR-0088 amendment forward-pointer note (amended by ADR-0098); (h) ADR-0090 amendment forward-pointer note (partially amended by ADR-0093 for the /drain_listeners method-discrimination divergence). Lands at phase-done commit alongside impl.
- `docs/envoy-go/DECISIONS.md` — seven to nine new ADRs (ADR-0091..ADR-0099 per §8) appended. Lands incrementally per `superpowers:executing-plans` PROGRESS preamble convention (ADRs land at the task that anchors them).
- `docs/envoy-go/ROADMAP.md` — row `08.2` flips `planned → in-progress` AT THIS COMMIT (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips `in-progress → done` at the 08.2 phase-done commit (along with parent row `08` AT THE SAME COMMIT — MVP-trunk closure per parent SPEC §5).
- `docs/envoy-go/STATE.md` — flips `lifecycle-state: 1 → 2`, `next-skill: superpowers:writing-plans` (PLAN.md authoring for 08.2), `active-phase: 08.2-graceful-drain`, `last-commit: <SPEC commit SHA>`, `last-updated: <date>`. SHA-fill follow-up commit per phase-04..08.1 convention. NOT modified in this commit (the orchestrating session handles STATE.md per the cold-start contract); the SPEC merely names what the next session will write.
- `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` — this file.
- `docs/envoy-go/phases/08.2-graceful-drain/README.md` — sibling SPEC stub from 08.1 SPEC commit; becomes read-only history at THIS commit per the stub's own §1.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 08.2)

```
cmd/envoy-go/main.go                      [modified: drainMgr alloc;
                                                    threaded into managers;
                                                    SIGTERM-handler block upgraded]
   ↓
internal/bootstrap.Load(configPath)        [unchanged]
   ↓ → *Bootstrap
internal/drain.New(30 * time.Second)       [NEW]
   ↓ → *drain.Manager
internal/cluster.NewManager(...)           [unchanged surface;
                                            cm.Drain() called from main
                                            after <-drainMgr.Done()]
   ↓ → *cluster.Manager
internal/listener.NewManagerWithBaseDirAndAllowH2C(  [WIDENED:
   ..., drainMgr)                                     dm *drain.Manager
                                                      threaded; HCM and
                                                      TCP-proxy filter
                                                      constructors widened
                                                      transitively]
   ↓ → *listener.Manager (with dm field;
        per-runtime Accept loops check dm.IsDraining())
internal/admin.New(adminAddr, registry,    [WIDENED: dm *drain.Manager
                   bs, cm, lm, drainMgr)              threaded as 6th param]
   ↓ → *admin.Server (with dm field;
        handleReady DRAINING-branch FIRST;
        deriveState DRAINING-branch FIRST;
        handleDrainListeners new handler)
admSrv.Start()                             [Start() body now registers
                                            seven handlers: the 08.1 six
                                            + the new /drain_listeners]
   ↓
... (rest of boot flow: filter registry, listener manager Start,
     MarkReady, stats.Freeze, ready sentinels, <-ctx.Done())
   ↓
<-ctx.Done()                               [SIGTERM/SIGINT received]
drainMgr.Drain()                           [Live → Draining;
                                            sync.Once-guarded;
                                            done channel armed]
select {
  <-drainMgr.Done():                       [inflight reached 0 OR timeout fired]
  <-time.After(drainMgr.Timeout()):        [30s default per ADR-0095]
}
cm.Drain()                                 [best-effort upstream pool close]
defer chain runs:                          [LIFO: lm.Stop, admSrv.Close,
                                                  sinks-close]
process exits 0
```

The drain manager is consumed by FIVE actors (per BRAINSTORM Decision 1's surface-area justification):
- `cmd/envoy-go/main.go` — owns the lifecycle; calls `Drain()` on signal; waits on `Done()`; calls `cm.Drain()` on rendezvous.
- `internal/admin.Server` — `handleDrainListeners` calls `Drain()`; `handleReady` reads `State()`; `handleServerInfo` reads `State()` via `deriveState`.
- `internal/listener.Manager` — Accept loop fast-path checks `IsDraining()`.
- `internal/filter/hcm` — `decodeHeaders` calls `Inc()`; `encodeFinalize` calls `Dec()` (per request).
- `internal/filter/tcpproxy` — `OnNewConnection` calls `Inc()`; `OnConnectionClose` calls `Dec()` (per connection).

### 5.2 Per-state-transition flow — SIGTERM → drain → exit (canonical lifecycle)

```
TIME=t0  cmd/envoy-go/main.go is blocked on <-ctx.Done() (current main.go:170 style)
TIME=t1  Operator runs `kill -TERM <pid>` (or SIGINT)
         signal.NotifyContext (line ~145) cancels ctx
TIME=t2  <-ctx.Done() unblocks
         drainMgr.Drain() called:
           - sync.Once-guarded: subsequent Drain() calls no-op
           - atomic.CompareAndSwap(state, Live, Draining) succeeds
           - if inflight already 0: close done immediately
           - else: arm a watcher goroutine that closes done when inflight → 0
TIME=t3  Listener Accept loop observes dm.IsDraining() == true on next iteration:
           - Accept returns a new conn (kernel TCP backlog already had it queued
             OR a new client just connected post-Drain — either way, Accept returns)
           - the Accept-loop body's first action: dm.IsDraining() check; if true,
             _ = conn.Close() and continue (no filter-chain dispatch)
           - the client observes connection FIN after the TCP handshake (per §11.5
             empirical evidence; envoy-go matches "accept-then-FIN with no HTTP body")
TIME=t3' Concurrently: in-flight HCM streams finish their decode/encode chain
         (per BRAINSTORM §5.5 swimlane)
         encodeFinalize calls dm.Dec() once per stream (post-access-log per phase 06.2)
         The drain manager's atomic.Int64 inflight counter decrements toward 0
TIME=t4  drain.Manager observes inflight == 0 → close drainMgr.done
         OR drainMgr.Timeout() (30s) fires first → close drainMgr.done best-effort
TIME=t5  cmd/envoy-go/main.go's select on <-drainMgr.Done() / <-time.After unblocks
         cm.Drain() called: walks per-cluster connection pools and closes them
TIME=t6  The deferred-stop chain runs (LIFO):
           - lm.Stop() closes listening sockets (idempotent — Stop is post-drain teardown)
           - admSrv.Close() shuts the admin HTTP server (Close on http.Server)
           - sinks-close flushes access logs (phase 06.2 hooks)
TIME=t7  main returns; process exits with status 0
```

**Correctness invariants (per BRAINSTORM §5.1):**
- (a) Between t2 and t6, NEW downstream connections receive accept-then-FIN per §11.5 (no HTTP-layer 503; no filter-chain dispatch).
- (b) Between t2 and t4, in-flight downstream requests COMPLETE — they run their full HCM chain; no abort-mid-flight. Access logs are emitted per phase 06.2's hooks. Per §11.3 empirical evidence: full-body delivery with NO `Connection: close` on the H1.1 response.
- (c) Between t4 and t6, upstream connection pools close. No new upstream connection is opened (because no new in-flight downstream request is dispatched to the cluster manager).
- (d) Total drain window is bounded by `drainMgr.Timeout()` (30s default per ADR-0095). Best-effort: if in-flight reaches 0 before the timeout, drain completes faster.

### 5.3 Per-state-transition flow — POST /drain_listeners → drain (no exit)

```
TIME=t0  envoy-go is in steady-state (LIVE; admin and listeners running)
TIME=t1  Operator runs `curl -X POST http://<admin>:<port>/drain_listeners`
TIME=t2  net/http dispatches to s.handleDrainListeners (internal/admin/drain.go)
         Method-discrimination check (per §11.4):
           - if r.Method != http.MethodPost: write 405 with body
             `Method <X> not allowed, POST required.\n` and the standard
             six-header set; return.
         drainMgr.Drain() called:
           - same effects as §5.2 t2
         response: 200 OK with body `OK\n` + standard six-header set
         response is FIRE-AND-FORGET: handler does NOT block on <-drainMgr.Done()
TIME=t3  curl prints the 200 response and exits
TIME=t4  envoy-go continues running in DRAINING state INDEFINITELY:
           - new connections: accept-then-FIN per §5.2 t3
           - in-flight requests complete per §5.2 t3'
           - /ready returns 503 DRAINING\n per Decision 9 (NEW DRAINING-first branch)
           - /server_info returns state: "DRAINING" per Decision 10 (deriveState
             DRAINING-first branch)
TIME=t5  Process does NOT exit. The operator separately issues SIGTERM/SIGINT
         (or kill -9) at a later time to actually exit. Until then, the proxy
         stays in DRAINING — accepting metrics scrapes, accepting /ready scrapes,
         completing any in-flight requests, but rejecting new downstream
         connections.
```

**Correctness invariants (per BRAINSTORM §5.2):**
- (a) The handler returns 200 BEFORE drain completes; the operator's curl call does NOT hang for 30s.
- (b) Subsequent `POST /drain_listeners` calls return 200 OK with the same body (sync.Once-guarded; idempotent).
- (c) No process exit. Distinguishes from §5.2 (SIGTERM → exit).
- (d) Method-discrimination: GET/PUT/DELETE/HEAD return 405 per §11.4 (verbatim body shape per §11.4 Conclusions).

### 5.4 Per-state-transition flow — /ready scrape during DRAINING

```
TIME=t0  envoy-go is in DRAINING state (post-§5.2 t2 OR post-§5.3 t2)
TIME=t1  Load balancer scrape: GET /ready
TIME=t2  net/http dispatches to s.handleReady (internal/admin/admin.go:121, modified
         per Decision 9)
         New branch FIRST: dm != nil && dm.State() == Draining
           - response: 503 Service Unavailable
           - body: DRAINING\n (verbatim per §11.2)
           - Content-Type: text/plain; charset=UTF-8
           - Cache-Control: no-cache, max-age=0
           - X-Content-Type-Options: nosniff
           - Server: envoy
           - framing: Content-Length: 9 (envoy-go) / transfer-encoding: chunked
             (Envoy per phase-01 framing deviation; existing dechunk preprocessor
             covers)
TIME=t3  Load balancer marks the instance unhealthy and stops sending traffic
```

**Correctness invariants (per BRAINSTORM §5.3):**
- (a) Before drain: /ready returns 200 LIVE\n (existing 08.1 / phase-01 behavior, unchanged).
- (b) Pre-MarkReady (boot window): /ready returns 503 PRE_INITIALIZING\n (existing per ADR-0015, unchanged).
- (c) During drain: /ready returns 503 DRAINING\n (NEW per Decision 9).
- (d) DRAINING precedence: DRAINING > LIVE > PRE_INITIALIZING. (An unlikely but defined-by-precedence case where Drain() fires before MarkReady; handler emits DRAINING.)

### 5.5 Per-state-transition flow — /server_info scrape during DRAINING

```
TIME=t0  envoy-go is in DRAINING state
TIME=t1  Operator scrape: GET /server_info
TIME=t2  net/http dispatches to s.handleServerInfo (internal/admin/serverinfo.go,
         buildServerInfo with extended deriveState per Decision 10)
TIME=t3  buildServerInfo calls deriveState(&s.ready, s.dm). NEW first check:
           if s.dm != nil && s.dm.State() == drain.StateDraining:
             return adminv3.ServerInfo_DRAINING
         Existing checks preserved unchanged.
TIME=t4  buildServerInfo returns *adminv3.ServerInfo with State = ServerInfo_DRAINING
         protojson.Marshal renders state field as the string "DRAINING"
         (per the protojson default — uses the proto-defined enum NAME)
TIME=t5  Response: 200 OK with body containing "state": "DRAINING"
         (other fields per ADR-0088 unchanged)
```

**Correctness invariants (per BRAINSTORM §5.4):**
- (a) During drain: `state: "DRAINING"`.
- (b) Before drain: `state: "LIVE"` (post-MarkReady; existing ADR-0088 behavior).
- (c) Pre-MarkReady: `state: "PRE_INITIALIZING"` (existing ADR-0088 behavior).
- (d) DRAINING precedence per Decision 10 (matches Decision 9's /ready precedence).

### 5.6 Per-state-transition flow — In-flight request completion during drain

```
TIME=t0  H1.1 keep-alive connection C1 is open with envoy-go; an in-flight request
         R1 is mid-decode. Drain has not fired.
TIME=t1  Drain fires (either §5.2 or §5.3).
         drainMgr.Drain() transitions state. atomic.Int64 inflight is currently 1 (R1).
TIME=t2  R1 continues: decodeHeaders has already run (inflight already incremented
         at t<0, before Drain fired).
         The HCM filter chain proceeds. The router filter dispatches to the upstream.
TIME=t3  The cluster manager's connection pool already has an open upstream conn for
         the route's cluster.
         The upstream request is sent. The upstream responds.
TIME=t4  HCM encodeHeaders + encodeData run. Response is written to the downstream
         conn.
TIME=t5  encodeFinalize runs (post-access-log per phase 06.2's hooks).
         dm.Dec() called. atomic.Int64 inflight goes 1 → 0.
         If draining: drainMgr observes inflight==0 → close drainMgr.done channel.
TIME=t6  The H1.1 keep-alive connection C1 is still open after R1 completes.
         Per §11.3 empirical evidence: NO `Connection: close` on R1's response;
         the H1.1 conn could be reused. envoy-go MVP does NOT mark C1 for graceful
         close; if a subsequent request arrives on C1, it is processed normally
         (and incs the inflight counter again, which extends the drain window).
         This is a deliberate envoy-go MVP simplification — the keep-alive
         connection-level graceful-drain semantics are deferred per §2.1.
TIME=t7  In the §5.2 lifecycle: drainMgr.Done() unblocks; cm.Drain() runs;
         lm.Stop() runs.
         In the §5.3 lifecycle: process stays running; C1 stays alive;
         /ready continues returning DRAINING; the operator is responsible for
         the eventual SIGTERM.
```

**Correctness invariants (per BRAINSTORM §5.5):**
- (a) R1 sees no abort. Its response is fully written to C1 with the upstream-determined status (per §11.3: 200 OK, full 5KB body for the slow-streaming probe).
- (b) Inflight count balances: 1 → 0 across decodeHeaders/encodeFinalize.
- (c) The drainMgr.Done() rendezvous is sound: it fires exactly when inflight reaches 0 (or timeout fires).
- (d) NO `Connection: close` header on the in-flight H1.1 response (per §11.3 empirical pin); this is an envoy-go MVP design choice that matches Envoy parity.

### 5.7 Per-state-transition flow — New connection during drain

```
TIME=t0  envoy-go is in DRAINING state
TIME=t1  A client opens a new TCP connection to the proxy listener port
TIME=t2  The kernel's TCP backlog accepts the connection (3-way handshake completes)
         The Listener Accept loop's blocking Accept() call returns the new net.Conn
TIME=t3  Accept-loop body's first action: check m.dm.IsDraining(). Returns true.
         The new conn is immediately Closed via conn.Close() (kernel sends FIN)
         NO filter chain dispatch
         The client observes a TCP FIN on its first read attempt → empty reply
         (per §11.5 empirical evidence: `curl: Empty reply from server`)
```

**Correctness invariants (per BRAINSTORM §5.6):**
- (a) No new connection is dispatched to the filter chain.
- (b) inflight does NOT increment for this connection (Inc lives in HCM/TCP-proxy filter, which doesn't run).
- (c) The empirical-pin §11.5 settles that the close is FIN-after-handshake (not RST-no-handshake), which is what `conn.Close()` produces in Go's `net` package.

### 5.8 Concurrency model

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `drain.New(...)` | Once | None — single-goroutine boot |
| Boot | `admin.New(..., dm)` registers handlers | Once | mux is per-Server; not shared |
| Per-request | `handleDrainListeners` calls `s.dm.Drain()` | Per scrape | `sync.Once`-guarded; lock-free fast-path on second call |
| Per-request | `handleReady` reads `s.dm.State()` | Per scrape | `atomic.LoadUint32`; lock-free |
| Per-request | `handleServerInfo` calls `deriveState(&s.ready, s.dm)` | Per scrape | `atomic.LoadUint32` for drain state; existing `s.ready.Load()` for ready |
| Per-Accept | `Manager.Drain()` Accept-loop fast-path | Per Accept | `atomic.LoadUint32`; lock-free |
| Per-request | HCM `decodeHeaders` calls `dm.Inc()` | Per request | `atomic.AddInt64`; lock-free |
| Per-request | HCM `encodeFinalize` calls `dm.Dec()` | Per request | `atomic.AddInt64`; lock-free; sentinel `markedInflight` ensures pair-balance under sendLocalReply path per ADR-0075 |
| Per-conn | TCP-proxy `OnNewConnection` calls `dm.Inc()` | Per conn | `atomic.AddInt64`; lock-free |
| Per-conn | TCP-proxy `OnConnectionClose` calls `dm.Dec()` | Per conn | `atomic.AddInt64`; lock-free |
| Boot-shutdown | `<-drainMgr.Done()` rendezvous | Once | channel close; lock-free |
| Boot-shutdown | `cm.Drain()` walks pools | Once | per-cluster pool close (best-effort) |

**Key invariant:** all hot-path drain interactions (state read, inflight Inc/Dec, IsDraining check) are lock-free atomic operations. No new mutex; no new channel beyond the single `done chan struct{}` rendezvous (which is closed exactly once via a `sync.Once`-equivalent pattern). The `drainMgr.Drain()` call is `sync.Once`-guarded so concurrent triggers from `handleDrainListeners` + the SIGTERM-handler are safe (only one transition fires).

**Race-detector contract:** `go test -race ./...` clean for N concurrent scrapes against all seven endpoints from N goroutines plus a separate goroutine firing `Drain()` once mid-test. The unit test `TestAdminConcurrentScrapeRace` (extended from 08.1) exercises this with N=100 scrape-loop goroutines for 1 second.

### 5.9 Drain-state machine state diagram

```
                          dm.Drain()
                       (sync.Once guard;
                        CAS Live → Draining)
                              │
            ┌─────────────────┼──────────────────┐
            │                 ▼                  │
            │           ┌──────────┐             │
            │           │ DRAINING │             │
            │           └──────────┘             │
   ┌──────┐ │                 │                  │
   │ LIVE ├─┘                 │                  │
   └──────┘                   │ inflight == 0    │ timeout fires
      ▲                       │ (Dec brings      │ (best-effort)
      │                       │  count to 0)     │
      │                       ▼                  ▼
      │ (initial state;       ┌──────────────────────┐
      │  set by drain.New)    │ DRAINED              │
      │                       │ (done channel        │
      │                       │  closed; observable  │
      │                       │  only via channel    │
      │                       │  close, not via      │
      │                       │  State() — State()   │
      │                       │  still returns       │
      │                       │  Draining. The       │
      │                       │  channel close is    │
      │                       │  the rendezvous.)    │
      │                       └──────────────────────┘
      │                                │
      │                                │ (consumed by SIGTERM-handler
      │                                │  via <-drainMgr.Done())
      │                                ▼
      │                           [process exit
      │                            in lifecycle §5.2]
      │
      │                       OR continue running indefinitely
      │                       in DRAINING (lifecycle §5.3 —
      │                       /drain_listeners trigger)
      │
   No transition from DRAINING back to LIVE in MVP. The drain trigger is
   one-way; recovery requires a process restart.
```

The DRAINED state is observable ONLY via the `done` channel close. `State()` continues to return `Draining` even post-rendezvous (the state machine has two observable transitions: `Live → Draining` via `State()`, and `Draining → Drained` via `Done()` channel close). This minimizes the public-API surface and keeps the contract simple — consumers of `State()` care about "should I reject new work" (Draining vs Live); consumers of `Done()` care about "is it safe to teardown" (Drained).

---

## 6. Per-endpoint and per-accessor contract summary

### 6.1 Constructor signatures (LBP-1 fifth application; per BRAINSTORM Decision 4)

```go
// internal/drain/manager.go — NEW:
package drain

type State uint32  // atomic.Uint32-friendly

const (
    StateLive State = iota
    StateDraining
    StateDrained  // NOT publicly exposed via State(); see §5.9
)

type Manager struct {
    state    atomic.Uint32
    inflight atomic.Int64
    done     chan struct{}
    timeout  time.Duration
    once     sync.Once
    closeOnce sync.Once  // guards `close(done)` to make the close idempotent
}

func New(timeout time.Duration) *Manager
func (m *Manager) State() State
func (m *Manager) Drain()
func (m *Manager) Done() <-chan struct{}
func (m *Manager) Inc()
func (m *Manager) Dec()
func (m *Manager) IsDraining() bool
func (m *Manager) Timeout() time.Duration

// internal/admin/admin.go — WIDENED (08.1 form → 08.2 form):
// 08.1: New(addr, registry, bs, cm, lm)
// 08.2:
func New(
    addr     string,
    registry *stats.Registry,
    bs       *bootstrap.Bootstrap,
    cm       *cluster.Manager,
    lm       *listener.Manager,
    dm       *drain.Manager,  // 08.2 NEW
) *Server

// internal/listener/manager.go — WIDENED:
func NewManagerWithBaseDirAndAllowH2C(
    ..., // existing parameters unchanged
    dm *drain.Manager,  // 08.2 NEW
) (*Manager, error)

// HCM constructor (path settled at PLAN time):
// signature gains a *drain.Manager parameter (LBP-1 fifth application)

// internal/filter/tcpproxy/filter.go — WIDENED:
// constructor gains a *drain.Manager parameter

// internal/cluster/manager.go — UNCHANGED constructor:
// (cluster manager does not need dm threaded; cm.Drain() is called
//  from cmd/envoy-go/main.go after <-drainMgr.Done())
```

### 6.2 `internal/drain.Manager` API (per BRAINSTORM Decision 1)

```go
// New constructs a Manager in StateLive with the given drain timeout.
// The timeout is consulted by the SIGTERM-handler in cmd/envoy-go/main.go
// (the Manager itself does not enforce the timeout — it is the caller's
// responsibility to select on time.After alongside Done()).
func New(timeout time.Duration) *Manager

// State atomically loads the current state. Lock-free.
// Returns StateLive or StateDraining (StateDrained is NOT publicly exposed
// via this method per §5.9 design choice).
func (m *Manager) State() State

// Drain transitions the state from Live to Draining. Idempotent (sync.Once-
// guarded). On the first call, it CAS's the state and starts a watcher
// goroutine that closes the done channel when inflight reaches 0 (OR
// closes it immediately if inflight is already 0). Subsequent calls no-op.
func (m *Manager) Drain()

// Done returns a channel that is closed when the drain rendezvous fires —
// i.e., when inflight reaches 0 after Drain has been called. If Drain has
// not been called, Done is open. If Drain is called when inflight is already
// 0, Done closes immediately. The channel is closed exactly once
// (closeOnce-guarded).
func (m *Manager) Done() <-chan struct{}

// Inc atomically increments the inflight counter. Called by HCM at request-
// begin (decodeHeaders) and by TCP-proxy at conn-begin (OnNewConnection).
// Lock-free.
func (m *Manager) Inc()

// Dec atomically decrements the inflight counter. Called by HCM at request-
// end (encodeFinalize) and by TCP-proxy at conn-end (OnConnectionClose).
// If the decrement brings inflight to 0 AND Drain has fired, this method
// closes the done channel (closeOnce-guarded). Lock-free.
func (m *Manager) Dec()

// IsDraining is the Listener Accept-loop fast-path check. Equivalent to
// State() == StateDraining. Lock-free; one atomic load.
func (m *Manager) IsDraining() bool

// Timeout returns the configured drain timeout (the value passed to New).
// Read-only; never changes after construction. The SIGTERM-handler in
// cmd/envoy-go/main.go calls this to set the time.After bound.
func (m *Manager) Timeout() time.Duration
```

### 6.3 `internal/admin.Server.handleDrainListeners` contract

| Property | Value |
|---|---|
| Path | `/drain_listeners` |
| Method | POST (other methods: 405 with body `Method <X> not allowed, POST required.\n` per §11.4) |
| Status (POST) | 200 OK |
| Status (non-POST) | 405 Method Not Allowed |
| Content-Type | `text/plain; charset=UTF-8` |
| Body (POST) | `OK\n` (3 bytes) per §11.1 |
| Body (non-POST) | `Method <X> not allowed, POST required.\n` (variable bytes) per §11.4 |
| Header set | Standard six-header set per §11.6 (content-type, cache-control, x-content-type-options, server, date, content-length / transfer-encoding) |
| Idempotent | Yes (sync.Once-guarded; second POST returns 200 + `OK\n` without re-firing Drain) |
| Side effects | First POST: `s.dm.Drain()` (Live → Draining); subsequent POSTs: no-op |
| Process exit | None — operator-driven drain stays in DRAINING indefinitely |
| Source of truth | `s.dm` (immutable post-boot ptr; Manager state is atomic) |

### 6.4 `internal/admin.Server.handleReady` extension contract

| State | Status | Body | Source of truth |
|---|---|---|---|
| `dm != nil && dm.State() == Draining` (NEW) | 503 Service Unavailable | `DRAINING\n` (9 bytes) per §11.2 | `s.dm.State()` (atomic) |
| `s.ready.Load() == false` (PRE_INITIALIZING; existing per ADR-0015) | 503 | `PRE_INITIALIZING\n` | `s.ready.Load()` (atomic.Bool) |
| `s.ready.Load() == true && (dm == nil || dm.State() == Live)` (LIVE; existing per phase 01) | 200 OK | `LIVE\n` | `s.ready.Load()` |

Branch precedence (top → bottom in handler code; first match wins): DRAINING > PRE_INITIALIZING > LIVE. Header set per §11.6 unchanged.

### 6.5 `internal/admin.Server.handleServerInfo` extension contract

| State | `state` field | Source of truth |
|---|---|---|
| `dm != nil && dm.State() == Draining` (NEW) | `"DRAINING"` (per protojson rendering of `adminv3.ServerInfo_DRAINING`) | `s.dm.State()` (atomic), via `deriveState` |
| `s.ready.Load() == true` (LIVE; existing per ADR-0088) | `"LIVE"` | `s.ready.Load()` |
| `s.ready.Load() == false` (PRE_INITIALIZING; existing per ADR-0088) | `"PRE_INITIALIZING"` | `s.ready.Load()` |

`INITIALIZING` enum value remains unreachable per ADR-0088 + 08.1 SPEC §11.7 (no xDS / no STRICT_DNS init phase that survives admin-bind). Other `/server_info` fields per ADR-0088 unchanged. Branch precedence (top → bottom in `deriveState` code): DRAINING > LIVE > PRE_INITIALIZING.

### 6.6 `internal/listener.Manager.Drain` accessor

```go
// Drain transitions the manager to drain mode by calling m.dm.Drain()
// (delegates to the central drain.Manager). The per-runtime Accept loops
// already check m.dm.IsDraining() at the top of each iteration; once
// Drain has been called, the next Accept return is the first conn that
// gets the accept-then-FIN treatment (per §11.5).
//
// Idempotent — calling Drain multiple times is safe (delegates to the
// sync.Once-guarded drain.Manager.Drain).
//
// This method does NOT close the listening sockets. Existing in-flight
// downstream connections continue running their HCM filter chains to
// completion. The post-drain teardown is Stop(), invoked from the
// deferred-stop chain in cmd/envoy-go/main.go AFTER <-drainMgr.Done().
func (m *Manager) Drain()
```

### 6.7 `internal/cluster.Manager.Drain` accessor

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
// Idempotent.
func (m *Manager) Drain()
```

### 6.8 SIGTERM-handler upgrade contract (`cmd/envoy-go/main.go`)

Per BRAINSTORM Decision 2, the SIGTERM/SIGINT handling upgrades from the existing hard-exit form to a drain-then-exit form. The structural shape:

```go
// boot:
drainMgr := drain.New(30 * time.Second)  // Decision 6 hardcoded default
// ... existing boot ordering: bootstrap.Load, cluster.NewManager, ...
lm, err := listener.NewManagerWithBaseDirAndAllowH2C(..., drainMgr)
// ... HCM filter-chain construction threads drainMgr through the listener manager
admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm, drainMgr)
// ... existing MarkReady, sentinels, etc.

ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()
defer admSrv.Close()
defer lm.Stop()
// ... other deferred cleanups (sinks-close, etc.)

<-ctx.Done()
log.Print("signal received; initiating graceful drain")
drainMgr.Drain()
select {
case <-drainMgr.Done():
    log.Print("drain rendezvous: in-flight reached 0")
case <-time.After(drainMgr.Timeout()):
    log.Print("drain rendezvous: timeout fired (best-effort)")
}
cm.Drain()  // best-effort upstream-pool close
// existing deferred-stop chain runs as the function unwinds
```

This is a DELIBERATE DIVERGENCE from upstream Envoy v1.37.2's SIGTERM behavior (per §11.7 empirical pin: Envoy v1.37.2 SIGTERM is immediate-exit). The divergence is documented in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Drain triggers` (§13.4); the differential equivalence claim does NOT exercise the SIGTERM path against reference Envoy.

---

## 7. Differential fixture `0010-graceful-drain` (per BRAINSTORM §7, §11)

### 7.1 Equivalence claims (per BRAINSTORM §7.3, refined per §11 empirical findings)

The fixture exercises the 08.2 surface under a slow-streaming-backend probe and asserts per-state-transition equivalence. The fixture has TWO driver paths in one binary (per BRAINSTORM Decision 12); only the admin-trigger path runs against both proxies (the SIGTERM-trigger path runs envoy-go-only because §11.7 evidence shows Envoy v1.37.2 SIGTERM is immediate-exit, which is a NON-equivalent behavior — the BEHAVIOR_CONTRACT-level divergence per §13.4 is the contract; the differential harness exercises only the ADMIN path against both).

Five per-state-transition equivalence claims (admin-trigger path only, against reference Envoy v1.37.2):

1. **Steady-state /ready (pre-drain):** byte-equal `LIVE\n` on both proxies (existing 08.1 / phase-01 baseline).
2. **POST /drain_listeners response:** byte-equal `OK\n` on both proxies. Headers structurally equivalent under the standard six-header set per §11.6.
3. **/ready DRAINING:** byte-equal `DRAINING\n` on both proxies. Status 503 on both. **NOTE:** per §11.2 empirical pin, upstream Envoy v1.37.2's `/ready` returns DRAINING ONLY after `/healthcheck/fail` — NOT after `/drain_listeners` alone. The driver therefore POSTs `/healthcheck/fail` AGAINST REFERENCE ENVOY ONLY (envoy-go does not implement `/healthcheck/fail` in MVP per §2.2; the differential gate normalizes by sending the appropriate trigger to each proxy: envoy-go gets `POST /drain_listeners` (which triggers DRAINING in envoy-go's design); reference Envoy gets `POST /drain_listeners` THEN `POST /healthcheck/fail` (which together trigger DRAINING in Envoy's design)). The driver's per-proxy trigger script is documented in §7.2.
4. **/server_info DRAINING:** the `state` field IS asserted byte-equal (`"DRAINING"`) when both proxies are in DRAINING (post-trigger-script per #3). Other fields per the ADR-0088 allow-list (08.1 baseline carries forward).
5. **In-flight request completion:** the in-flight `GET /slow` request returns 200 OK with the same body bytes on both proxies (the upstream backend serves the same content on both runs; the proxy is transparent).

The new-connection-rejection assertion is a connectivity-level check (each side: TCP connect succeeds; HTTP read returns empty / error per §11.5 on accept-then-FIN). It is not a body-level differential claim.

### 7.2 Driver outline (admin-trigger path; per BRAINSTORM §7.2)

```
Step  Driver action (against both proxies)                                     Reference  Subject
----  -------------------------------------------------------------------     ---------  ---------
 1    Boot envoy-go on admin :9901, listener :10000.                          —          envoy-go
      Boot reference Envoy on admin :9902, listener :10001.                   Envoy      —
      Boot Go HTTP backend on :18001 (slow handler streams 5KB at 1KB/s).
 2    Sanity scrape: GET /ready on each proxy → expect 200 LIVE\n.            assert     assert
 3    Sanity scrape: GET /server_info on each proxy → expect state: "LIVE".   assert     assert
 4    Open a long-lived in-flight request: GET /slow on each listener port.   start      start
      Read partial body (assert 200 OK + first chunk arrives).
 5    Trigger drain (per-proxy script):
        envoy-go:        POST /drain_listeners                                —          POST
        reference Envoy: POST /drain_listeners; POST /healthcheck/fail        POST+POST  —
      Each POST asserts 200 OK + body matching the empirical-pin verbatim.
 6    Poll GET /ready on each proxy until 503 DRAINING\n (max 1s; fail if     poll       poll
      not observed). Byte-equal claim per §7.1#3.
 7    GET /server_info on each proxy; assert state == "DRAINING".              assert     assert
      Byte-equal claim per §7.1#4.
 8    Open a NEW TCP conn to each listener port.                              attempt    attempt
      Expect immediate close (FIN-after-handshake; HTTP read returns
      empty / error). NOT a body-level diff; connectivity-level check.
 9    Wait for the in-flight GET /slow request to complete (max 6s).          wait       wait
10    Assert in-flight response: status 200 OK; body length 5KB; body bytes   assert     assert
      byte-equal across both proxies (the upstream is the same backend).
      Byte-equal claim per §7.1#5.
11    Re-scrape /server_info on each proxy; assert state STILL "DRAINING"     assert     assert
      (proxy stays in DRAINING after in-flight completes; admin-trigger
      path does NOT auto-exit per §5.3).
12    Cleanup: kill each proxy (admin-trigger path does NOT auto-exit).       SIGKILL    SIGKILL
      Wait for process exit. Drop containers / subprocess.
```

### 7.3 Driver outline (SIGTERM-trigger path; envoy-go-only per §11.7 deviation)

```
Step  Driver action (against envoy-go ONLY)                                    Subject
----  -------------------------------------------------------------------     ---------
 1    Boot envoy-go on admin :9901, listener :10000. Boot backend on :18001.  envoy-go
 2    Sanity scrape: GET /ready → LIVE\n. GET /server_info → state: "LIVE".   assert
 3    Open a long-lived in-flight request: GET /slow on listener :10000.      start
      Read partial body.
 4    Send SIGTERM to envoy-go process: kill -TERM <pid>.                     SIGTERM
 5    Poll GET /ready until 503 DRAINING\n (max 1s; fail if not observed).    poll
 6    Poll GET /server_info; assert state == "DRAINING".                       assert
 7    Open a NEW TCP conn to listener :10000. Expect immediate close.         attempt
 8    Wait for in-flight GET /slow request to complete (max 6s).              wait
 9    Assert in-flight response: status 200 OK; body length 5KB.              assert
10    Wait for envoy-go process to exit gracefully (max 30s drain timeout).   wait
11    Assert exit status: 0.                                                   assert
```

The SIGTERM-trigger path is an internal-completeness check on envoy-go's drain rendezvous + exit ordering — NOT a differential claim against reference Envoy (the deviation is documented at the contract level). The fixture's driver runs this path after the admin-trigger path completes.

### 7.4 Fixture bootstrap (verbatim, per BRAINSTORM §7.1; port-disambiguated for dual-proxy)

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

`envoy.yaml` is identical modulo `admin.address.socket_address.port_value: 9902` and `listeners[0].address.socket_address.port_value: 10001`. Both proxies share the same backend on `127.0.0.1:18001` (or a DNS alias `backend` if running under bridge-network containerization).

### 7.5 Backend shape (per BRAINSTORM §7.5)

```go
// test/differential/0010-graceful-drain/backends/backend.go (sketch)
package main

import (
    "bytes"
    "net/http"
    "time"
)

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

The `/slow` handler is the load-bearing component (it gives the in-flight assertion a stable 5s window). The `/` handler is a sanity baseline.

### 7.6 Differential gate scope clarification

The differential equivalence claim is per-state-transition byte-equality on the five claims of §7.1, exercised under the admin-trigger driver path only. The SIGTERM-trigger driver path (§7.3) is envoy-go-only and is a structural-completeness check, NOT a differential claim. The trigger-script asymmetry (envoy-go gets `/drain_listeners`; reference Envoy gets `/drain_listeners + /healthcheck/fail`) is documented in §7.2 step 5 and codified in the fixture's `expectations.yaml` per-proxy `trigger_script` field; this is an Envoy-deviation envelope similar to the existing 0009-admin-config-dump structural-projection canonicalisation discipline (08.1 SPEC §7.1) — narrow per-proxy normalization preserves the load-bearing equivalence claim while honoring upstream Envoy's specific endpoint wiring.

---

## 8. ADRs anticipated (per BRAINSTORM §9)

The 08.2 SPEC anticipates **nine ADRs** (ADR-0091 through ADR-0099) per BRAINSTORM §9, citing `DECISIONS.md` tail SHA `eb3babd` (08.1 SHA-fill follow-up commit) at ADR-0090 — verified as the next-free per ADR-0004's hard-gate discipline. Topical-vs-commit-time ordering may permute and is recorded in each ADR's `Lands-in-task` field per the 07.1 / 07.2 / 08.1 PROGRESS preamble convention.

- **ADR-0091 — Drain state-machine shape (LIVE / DRAINING / DRAINED) + new `internal/drain/` package + LBP-1 fifth-application threading.** Status: Accepted. Doctrine: D-3.2 + D-3.4 + D-3.5. Decision: a new `internal/drain/` package with a `Manager` type implementing a three-state machine backed by `atomic.Uint32` + `atomic.Int64` + `chan struct{}` + `sync.Once`. The `Manager` is allocated once at boot in `cmd/envoy-go/main.go` and threaded into `admin.New`, `listener.NewManagerWithBaseDirAndAllowH2C`, HCM filter constructor, and TCP-proxy filter constructor (the LBP-1 fifth application after `*stats.Registry` per 06.1, `*HTTPRegistry` per ADR-0072, `*ListenerFilterRegistry` per ADR-0079, and the 08.1 `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` triplet per ADR-0085). DRAINED state is observable only via `Done()` channel close, not via `State()` — minimizes public-API surface. Rationale: see BRAINSTORM Decision 1 + 4. Lands-in-task: 08.2 PLAN Task wherever `internal/drain/manager.go` lands.
- **ADR-0092 — SIGTERM and SIGINT both trigger drain-then-exit; deliberate divergence from Envoy v1.37.2's SIGTERM=immediate-exit per §11.7.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: the existing `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)` registration in `cmd/envoy-go/main.go` stays unchanged; the `<-ctx.Done()` body is upgraded to call `drainMgr.Drain()`, await `<-drainMgr.Done()` or timeout, then run `cm.Drain()` followed by the existing deferred-stop chain. Rationale: per BRAINSTORM Decision 2, drain-on-SIGTERM is the more operator-friendly default; the §11.7 empirical evidence shows Envoy's SIGTERM is immediate-exit, but envoy-go's design choice is to make SIGTERM/SIGINT drain-then-exit in symmetry with the `/drain_listeners` admin trigger. The differential equivalence claim does NOT exercise the SIGTERM path; the divergence is documented at the BEHAVIOR_CONTRACT level (§13.4). Lands-in-task: 08.2 PLAN Task wherever the SIGTERM-handler block lands.
- **ADR-0093 — POST /drain_listeners contract: 200 OK with body `OK\n`; method-discrimination ENFORCED (405 on non-POST per §11.4 empirical pin); idempotent; ?graceful=true silent-ignored. Partially amends ADR-0090.** Status: Accepted. Doctrine: D-3.3 + D-3.5 + D-3.7. Decision: the new admin endpoint at `/drain_listeners` accepts only POST; non-POST methods return `405 Method Not Allowed` with body `Method <X> not allowed, POST required.\n` per §11.4 empirical pin. POST returns 200 with body `OK\n` per §11.1 empirical pin. Idempotent (sync.Once-guarded; second POST returns identical 200 without re-firing Drain). Triggers `drainMgr.Drain()` synchronously; does NOT block on `<-drainMgr.Done()` — fire-and-forget from the operator's perspective. Does NOT trigger process exit (the operator-driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT). The `?graceful=true` query-param is silently accepted (per ADR-0041's silent-ignore precedent; envoy-go's drain is always graceful by construction). **PARTIALLY AMENDS ADR-0090** (no-method-discrimination posture): ADR-0090's posture applied uniformly to read-only endpoints in 08.1 (no method check); ADR-0093 records that the FIRST mutating endpoint in envoy-go (POST /drain_listeners) DOES enforce method discrimination per §11.4 — Envoy parity for mutating endpoints. The amendment is purely additive (the read-only endpoints' no-method-check posture is preserved verbatim). Lands-in-task: 08.2 PLAN Task wherever `internal/admin/drain.go` lands.
- **ADR-0094 — Listener stop-accepting via per-runtime Accept-loop fast-path on dm.IsDraining(); listener-socket close stays at Stop() (post-drain teardown). Accept-then-FIN per §11.5.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: `internal/listener.Manager.Drain()` is a public method that delegates to the central `drain.Manager.Drain()`. The actual stop-accepting mechanism is a per-runtime Accept-loop fast-path: at the top of each Accept iteration (after `Accept()` returns), the loop body checks `m.dm.IsDraining()`; if true, the new conn is immediately `conn.Close()`'d and the loop continues without filter-chain dispatch. This produces the accept-then-FIN behavior observed in §11.5 (TCP handshake completes; client reads empty / connection close). The existing `Listener.Manager.Stop()` method stays unchanged as the post-drain teardown step (closes the listening sockets). Rationale: per BRAINSTORM Decision 5; the empirical evidence at §11.5 confirms the accept-then-FIN choice (vs. listener-socket-close-on-Drain) matches Envoy parity. Lands-in-task: 08.2 PLAN Task wherever `internal/listener/manager.go` Drain method lands.
- **ADR-0095 — Drain timeout default: hardcoded 30s in envoy-go MVP; Envoy v1.37.2 default is 600s (per §11.7 + 08.1 SPEC §11.4); deliberate divergence; operator-knob deferred to a future runtime/hot-restart family phase.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: the drain timeout is a single proxy-wide value, hardcoded to `30 * time.Second` at the `cmd/envoy-go/main.go` boot site. The literal lives at the call site (not in the `drain` package) so test code can construct `drain.New(10 * time.Millisecond)` for fast-path tests. Rationale: per BRAINSTORM Decision 6; 30s is the established envoy-go MVP timeout shape (matches the 08.1-widened `httpSrv.WriteTimeout`); 600s is too long for a CI fixture (would block ~10 minutes on the differential gate); the equivalence claim is over drain BEHAVIOR not timeout VALUE. Lands-in-task: 08.2 PLAN Task wherever `cmd/envoy-go/main.go` boot wiring lands.
- **ADR-0096 — In-flight-completion discipline: HCM decodeHeaders/encodeFinalize Inc/Dec pair per request; TCP-proxy OnNewConnection/OnConnectionClose Inc/Dec pair per connection; cluster.Manager.Drain best-effort upstream-pool close after <-drainMgr.Done(). NO Connection: close on H1.1 in-flight responses per §11.3.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: per BRAINSTORM Decisions 7 + 8 consolidated. HCM increments inflight at request-begin (decodeHeaders, BEFORE the filter chain runs) and decrements at request-end (encodeFinalize, AFTER access-log emit per phase 06.2). A `markedInflight bool` field on `Stream` ensures Inc/Dec balance under all paths including sendLocalReply (per ADR-0075). TCP-proxy increments at conn-begin (OnNewConnection) and decrements at conn-close (OnConnectionClose) — per-connection granularity (correct because TCP-proxy has no per-request semantic). Cluster manager's Drain() is best-effort upstream-pool close after the rendezvous fires (no in-flight upstream requests are pending at that point because no in-flight downstream requests are pending). Per §11.3 empirical evidence, envoy-go does NOT mark in-flight H1.1 keep-alive responses with `Connection: close` — Envoy parity; subsequent requests on the same conn during DRAINING extend the drain window via further Inc calls (deliberate MVP simplification; per-conn drainable-close-at-next-idle-window deferred per §2.1). Lands-in-task: 08.2 PLAN Task wherever the HCM and TCP-proxy filter modifications land.
- **ADR-0097 — /ready DRAINING-state body `DRAINING\n` per §11.2; DRAINING-precedence-over-PRE_INITIALIZING-and-LIVE rule. PARTIALLY SUPERSEDES ADR-0015.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: the `handleReady` body gains a NEW first branch checking `dm != nil && dm.State() == drain.StateDraining`; if true, write 503 with body `DRAINING\n` (9 bytes) per §11.2 verbatim. The DRAINING check has precedence over both LIVE and PRE_INITIALIZING — once drain has fired, /ready returns DRAINING even if MarkReady has been called. **PARTIALLY SUPERSEDES ADR-0015** (the pre-init contract for /ready): ADR-0015's two-state coverage (LIVE + PRE_INITIALIZING) extends to three-state (LIVE + PRE_INITIALIZING + DRAINING). ADR-0015's pre-init body and pre-init status are preserved verbatim; ADR-0097 adds the DRAINING branch and the precedence rule. **NOTE:** §11.2 empirical pin reveals that upstream Envoy v1.37.2 ties the /ready DRAINING body to `/healthcheck/fail`, not to `/drain_listeners` alone. envoy-go's MVP design choice is to UNIFY these triggers under the single drain.Manager state machine — both `/drain_listeners` POST and SIGTERM/SIGINT directly transition `dm.State() == Draining`, which is what `/ready` consults. The fixture-level normalization for the upstream-Envoy difference is documented in §7.2. Lands-in-task: 08.2 PLAN Task wherever `internal/admin/admin.go` handleReady modification lands.
- **ADR-0098 — /server_info `state` field DRAINING transition; deriveState extended to consult drain.Manager. AMENDS ADR-0088 (purely additive; not superseding).** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: `deriveState` signature extends from `deriveState(ready *atomic.Bool)` to `deriveState(ready *atomic.Bool, dm *drain.Manager)`. NEW first check: `if dm != nil && dm.State() == drain.StateDraining { return adminv3.ServerInfo_DRAINING }`. Existing LIVE / PRE_INITIALIZING checks preserved unchanged. The DRAINING precedence matches ADR-0097's /ready precedence (DRAINING > LIVE > PRE_INITIALIZING). **AMENDS ADR-0088** (per ADR-0088 consequence (c) verbatim from `DECISIONS.md` lines 3441-3442 — "the amendment is purely additive; no other field changes; the ADR-0088 amendment will record the addition without superseding this ADR"). The amendment record is appended to ADR-0088 in-place per the ADR-0089 consequence (b) pattern (in-place edit per ADR-0052's BEHAVIOR_CONTRACT precedent applied to ADR text). The `INITIALIZING` enum value remains unreachable per ADR-0088 + 08.1 SPEC §11.7. Lands-in-task: 08.2 PLAN Task wherever `internal/admin/serverinfo.go` deriveState modification lands.
- **ADR-0099 — Hot-restart deferral; envoy-go's drain is one-process scope only; future runtime/hot-restart family delivers SCM_RIGHTS-based handoff.** Status: Accepted. Doctrine: D-3.5. Decision: hot restart / parent-child handoff is OUT OF SCOPE for 08.2 and the entire BOOTSTRAP_PROMPT.md §8 MVP trunk. Future work lives in BOOTSTRAP_PROMPT.md §9's "Runtime + hot restart family" — which §9 explicitly anticipates with the line "graceful-drain semantics beyond phase 08's minimum." Rationale: per BRAINSTORM Decision 11; SCM_RIGHTS file-descriptor transfer + shared-memory existing-connection table + parent-shutdown-time orchestration + custom signal protocol are all multi-phase deliverables that would inflate 08.2 past the ADR-0045 split threshold. Lands-in-task: 08.2 PLAN Task wherever the deferral table lands in BEHAVIOR_CONTRACT.md (`### Does not yet apply to` extension under `## Graceful drain`). Cross-ref: BOOTSTRAP_PROMPT.md §9 + ADR-0089 (parallel admin-endpoint deferral list).

**Inline supersessions / amendments anticipated** (recorded in the ADRs above, not as separate ADRs):

- **ADR-0015** (pre-init contract for /ready) — partially superseded by **ADR-0097** (DRAINING extension). The pre-init contract for LIVE / PRE_INITIALIZING is preserved verbatim; ADR-0097 adds the DRAINING branch and the precedence rule.
- **ADR-0088** (/server_info body shape; state enum coverage LIVE + PRE_INITIALIZING) — amended by **ADR-0098** (DRAINING enum coverage added). Per ADR-0088 consequence (c), the amendment is purely additive; recorded as an in-place edit of ADR-0088's Consequences section per the ADR-0089 consequence (b) pattern.
- **ADR-0089** (admin-endpoint deferral list) — POST /drain_listeners line flips from "08.2 (graceful drain)" to "delivered in 08.2 per ADR-0093." Per ADR-0089 consequence (b), the table is updated in-place; no new ADR for the disposition flip. The `/healthcheck/fail` line stays in the deferral list (envoy-go does not implement that endpoint in MVP per §2.2).
- **ADR-0085** (admin-mux reuse + LBP-1 third application) — consequence (a) extended in-place to enumerate the 08.2 LBP-1 fifth-application threading of `*drain.Manager`. Per the LBP-1 generalization pattern, the extension is in-place; no new ADR for the discipline.
- **ADR-0090** (no-ACL admin-endpoint security posture; no method discrimination on read-only endpoints) — partially amended by **ADR-0093** (mutating endpoint /drain_listeners DOES enforce method discrimination per §11.4). The amendment is recorded as an in-place edit of ADR-0090's Consequences section per the ADR-0089 consequence (b) pattern. The no-ACL posture is preserved verbatim; the no-method-discrimination posture is qualified to read-only endpoints only.

**Consolidation candidates (planner discretion):** if the PLAN-time review feels nine ADRs is too many, the SPEC author flags ADR-0094 (listener Drain) for consolidation into ADR-0091 (drain state machine) — both are about the LIVE → DRAINING transition observability — landing at 8 ADRs. Further consolidation (ADR-0099 hot-restart deferral folded into ADR-0089 admin-deferral-list) is technically possible but loses topical clarity (the hot-restart deferral has its own MVP-scope rationale per Decision 11). The conservative choice is to keep all nine separate; the planner settles at PLAN time. Phase 06.1 had 6 ADRs; 06.2 had 4; 07.1 had 7; 07.2 had 7; 08.1 had 7. **Nine sits at the high end** but appropriate for a phase that introduces a new package + a new admin endpoint + two endpoint extensions + two prior-ADR amendments + a SIGTERM-handler-divergence ADR.

---

## 9. Out-of-scope (explicitly deferred)

Beyond the §2 non-purposes enumeration, three cross-cutting items are explicitly deferred from 08.2 and recorded here for planner / future-phase reference:

- **Method discrimination enforcement on the read-only 08.1 endpoints** (`/config_dump`, `/clusters`, `/listeners`, `/server_info`). Per ADR-0090, these stay any-method-accepting in MVP (Envoy parity per 08.1 SPEC §11.8). ADR-0093's method discrimination on /drain_listeners is the FIRST and ONLY method-discrimination case in envoy-go; future security-hardening phase may extend.
- **`/healthcheck/fail` POST endpoint.** Per §11.2 empirical pin, upstream Envoy v1.37.2 ties the /ready DRAINING body to `/healthcheck/fail`, not to `/drain_listeners` alone. envoy-go's MVP does NOT implement `/healthcheck/fail` separately — both /drain_listeners POST and SIGTERM trigger the unified DRAINING transition (per ADR-0091). The `/healthcheck/fail` endpoint stays in ADR-0089's deferral list; the differential gate normalizes via per-proxy trigger script (§7.2).
- **N-2 / N-3 / N-5 carry-forwards from 08.1 REVIEW** (writeEndpointLines refactor; BuildVersionString memoization; FuzzConfigDumpFormat corpus expansion). 08.2 carries no inline-fix obligation; these stay in their respective future hardening passes.

---

## 10. Carry-forward dispositions (per BRAINSTORM §4)

### 10.1 Architectural carry-forward (per BRAINSTORM §4.1)

Phase 08.1 + the 08.1 REVIEW.md establish three architectural patterns 08.2 inherits:

**(a) Admin-mux extension pattern (per ADR-0085).** The existing `*http.ServeMux` allocated at `internal/admin/admin.go:78` carries six `mux.HandleFunc(...)` registrations as of 08.1 (`/ready`, `/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`). 08.2 adds one more registration: `mux.HandleFunc("/drain_listeners", s.handleDrainListeners)` — the seventh handler on the same mux (per BRAINSTORM Decision 3). No new HTTP server, no new bind, no new transport.

**(b) Constructor-widening pattern (LBP-1 fifth application).** ADR-0085's "no package globals; explicit threading" discipline extends to `*drain.Manager` per BRAINSTORM Decision 4. The `admin.New` signature, currently 6-parameter as of 08.1, widens to 7-parameter. The `listener.NewManagerWithBaseDirAndAllowH2C` signature widens to add `*drain.Manager`. The HCM and TCP-proxy filter constructors widen similarly. Test code that does not exercise drain semantics may pass `nil` (the same nil-tolerance pattern ADR-0085 established for the four 08.1 dependencies).

**(c) ADR-0088 anticipated DRAINING amendment path.** Per ADR-0088 consequence (c) verbatim: "Phase 08.2's drain implementation extends the state enum coverage to LIVE + PRE_INITIALIZING + DRAINING ... The amendment is purely additive; no other field changes. The ADR-0088 amendment will record the addition without superseding this ADR." ADR-0098 IS that amendment.

**(d) BEHAVIOR_CONTRACT umbrella in-place edit (per ADR-0052).** The `## Admin API` umbrella that 08.1's restructure landed (six per-endpoint subsections + framing-deviation paragraph + header-set paragraph + method-discrimination paragraph) is the host structure 08.2 extends. 08.2 adds `### /drain_listeners` as a new sibling subsection AND extends `### /ready` (DRAINING body) and `### /server_info` (DRAINING state) in place. ADR-0052 authorization carries forward.

**(e) Empirical-pin discipline (per ADR-0004).** 08.1 executed eight empirical pins IN-SESSION at SPEC time (per 08.1 SPEC §11.1–§11.9). 08.2 follows the same discipline; §11 of THIS SPEC documents the seven 08.2-specific pins resolved IN-SESSION against Envoy v1.37.2.

### 10.2 REVIEW.md findings (per BRAINSTORM §4.2)

| Finding | 08.1 disposition | 08.2 carry-forward action |
|---|---|---|
| **N-1** `internal/listener.Manager.Listeners()` doc-comment ordering not documented | Carry-forward to 08.2 (Listener.Manager touched by drain wiring) | **08.2 inline-fix:** add the one-line doc-comment on `Listeners()` saying "order is bootstrap-declaration order; callers needing alphabetical ordering must sort." Lands as part of BRAINSTORM Decision 5's `internal/listener/manager.go` modifications. Cost: ~3 LoC. |
| **N-2** `internal/admin/clusters.go:78-99` `writeEndpointLines` table-driven refactor opportunity | Carry-forward to a future ADR-0063-supersession phase | **08.2 NO action.** ADR-0063 is unmodified by 08.2; the refactor opportunity remains. |
| **N-3** `BuildVersionString()` memoization opportunity | Carry-forward to a future micro-optimisation pass | **08.2 NO action.** No micro-optimisation pass scheduled; the per-request cost is microsecond-scale. |
| **N-4** `wantedTypes` cross-reference doc-comment in fixture 0009 canonicaliser | Carry-forward to 08.2 (fixture 0010 likely touches the same canonicaliser) | **08.2 inline-fix candidate:** if the 0010 fixture's driver shares canonicalisation utilities with 0009, add the cross-reference doc-comment as part of the shared-util touch. Lands as part of §7 fixture design. Cost: ~5 LoC. (If 0010 does NOT share utilities — the SPEC author's surface review suggests the new fixture uses different canonicalisation per §7.1's per-state-transition byte-equality rather than 0009's structural-projection — N-4 stays carry-forward.) |
| **N-5** `FuzzConfigDumpFormat` corpus expansion | Carry-forward to a future fuzzer-hardening pass | **08.2 NO action.** No fuzzer-hardening pass scheduled. |

### 10.3 M-8 carry-forward from 07.2 REVIEW (cited via 08.1 SPEC §10)

The 07.2 REVIEW M-8 finding (200ms hardcoded drain in the 0007b fixture driver) is **directly relevant to 08.2**: the 0010 fixture driver should NOT repeat the same hardcoded sleep pattern. Per §7 fixture design, the 0010 driver uses event-based synchronization (poll `/ready` until DRAINING body observed; poll until in-flight count reaches 0 indirectly via response completion; no hardcoded sleeps anywhere in the driver flow except the backend's intentional `time.Sleep(1*time.Second)` between body chunks).

### 10.4 No 08.1-introduced regressions

The 08.1 REVIEW.md §1 final assessment was APPROVED with no Major and no Minor findings. The 08.1 implementation does not block 08.2 in any way; the carry-forward findings above are documentation-tier or test-coverage-tier, not correctness-tier.

---

## 11. Empirical-pin block (per BRAINSTORM §11 — all seven pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline (autonomous-brainstorm requires empirical evidence for design decisions that are not derivable from documentation alone). Mirrors 08.1 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md`). Server-build SHA confirmed by `/server_info` `version` field: `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches 08.1 SPEC §11.4 evidence).

**Probe configuration:** the §7.1 fixture bootstrap (STATIC cluster `c_backend` with one endpoint at `127.0.0.1:18001`; single listener `l_main` on `0.0.0.0:10000`; admin on `0.0.0.0:9901`). Reference Envoy was booted in a Docker container under `--network envoy-pins-net` (a bridge network) with `-p 9901:9901 -p 10000:10000` port forwarding, alongside a sidecar Python HTTP backend (alias `backend`; bridge-network DNS) implementing the `/slow` slow-streaming endpoint per §7.5. The 08.2 fixture's STRICT_DNS variant (using the `backend` alias) was used during the empirical-pin probes to allow cross-container DNS; the SPEC §7.4 verbatim YAML uses STATIC `127.0.0.1:18001` for the eventual differential harness layout. Probe date: 2026-05-02. Capture transcripts: `/tmp/envoy-08.2-pins/pin-N.txt` (transient artifacts; not committed; the verbatim outputs below are the durable evidence per the 08.1 SPEC §11 line 576 discipline).

### 11.1 Empirical pin #1 — POST /drain_listeners response body verbatim

**Verbatim Envoy `POST /drain_listeners` (full response, headers + body):**

```
HTTP/1.1 200 OK
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:16:51 GMT
server: envoy
transfer-encoding: chunked

OK
```

(Body is `OK\n` — 3 bytes — followed by transfer-encoding terminator.)

**Conclusions (pinned):** envoy-go's `handleDrainListeners` for the POST path MUST:
- (a) emit `Content-Type: text/plain; charset=UTF-8` (lowercase header per upstream).
- (b) emit body verbatim `OK\n` (3 bytes; capital `OK` followed by single `\n`).
- (c) emit response status `200 OK`.
- (d) emit the standard six-header set per §11.6 (content-type, cache-control: no-cache, max-age=0, x-content-type-options: nosniff, date: <IMF-fixdate>, server: envoy). The framing header is `transfer-encoding: chunked` upstream / `content-length: 3` envoy-go (the existing phase-01 framing deviation; existing dechunk preprocessor covers).
- Settles BRAINSTORM Decision 3 body shape (`OK\n` confirmed; the alternative candidate `{}\n` rejected).
- Lands in `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` body-shape paragraph (§13.1) and ADR-0093 consequence section.
- Confirms 08.1 SPEC §11.5 framing finding (transfer-encoding: chunked upstream; envoy-go emits Content-Length).
- Confirms 08.1 SPEC §11.6 header set finding (six-header set unchanged from 08.1's four read-only endpoints + /drain_listeners now joins as the seventh endpoint with the same set).

### 11.2 Empirical pin #2 — /ready DRAINING-state response body verbatim

**SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** `POST /drain_listeners` ALONE does NOT cause `/ready` to return the DRAINING body. After a single `POST /drain_listeners`, `/ready` continues to return `200 LIVE\n` indefinitely. The DRAINING body is only emitted by `/ready` after `POST /healthcheck/fail` (a separate admin endpoint). The two endpoints have distinct semantics in upstream Envoy v1.37.2:
- `POST /drain_listeners` triggers the listener-side drain (stop-accepting on listening sockets), but does NOT flip `/ready` to DRAINING.
- `POST /healthcheck/fail` triggers the load-balancer-disposition flip (`/ready` returns 503 DRAINING) but does NOT close listening sockets.

**Verbatim Envoy `/ready` (after `POST /healthcheck/fail`):**

```
HTTP/1.1 503 Service Unavailable
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:19:05 GMT
server: envoy
transfer-encoding: chunked

DRAINING
```

(Body is `DRAINING\n` — 9 bytes including the trailing newline.)

**Verbatim Envoy `/server_info` `state` field (after `POST /healthcheck/fail`):**

```
{
 "version": "5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL",
 "state": "DRAINING",
 "uptime_current_epoch": "8s",
 "uptime_all_epochs": "8s",
 "hot_restart_version": "11.104",
 ...
}
```

(`state: "DRAINING"` is the protojson-rendered form of `adminv3.ServerInfo_DRAINING`.)

**Conclusions (pinned):**
- (a) Body verbatim: `DRAINING\n` (9 bytes; uppercase `DRAINING` followed by single `\n`). Status 503 Service Unavailable.
- (b) Header set: identical to /ready's existing LIVE-state header set per §11.6 (no DRAINING-specific headers; no `Connection: close` on the /ready response itself — /ready is a one-shot probe, not a keep-alive surface).
- (c) `/server_info` `state` field renders as `"DRAINING"` (proto enum NAME, not numeric value). Other ServerInfo fields per ADR-0088 unchanged.
- **Surprise:** upstream Envoy v1.37.2's `/drain_listeners` and `/healthcheck/fail` are SEPARATE triggers — the former drains listeners, the latter flips /ready to DRAINING. envoy-go's MVP design choice (per BRAINSTORM Decision 1 and confirmed by ADR-0091 + ADR-0097 + ADR-0098) is to UNIFY these triggers under a single drain.Manager state machine. Both `/drain_listeners` POST and SIGTERM/SIGINT directly transition `dm.State() == Draining`, which is what `/ready` and `/server_info` consult. This is a deliberate divergence at the wiring level (the BODY shapes match Envoy verbatim; the TRIGGERS differ). The differential fixture's per-proxy trigger script normalizes (per §7.2 step 5).
- Settles BRAINSTORM Decision 9 body shape (`DRAINING\n` confirmed; the alternative candidate `Draining\n` rejected).
- Settles BRAINSTORM Decision 10 state-field shape (`"DRAINING"` confirmed for protojson rendering).
- Lands in `BEHAVIOR_CONTRACT.md ## Admin API ### /ready` DRAINING extension paragraph (§13.2), `## Admin API ### /server_info` DRAINING extension paragraph (§13.3), and ADR-0097 + ADR-0098 consequence sections.
- The `/healthcheck/fail` endpoint stays in ADR-0089's deferral list (§9 records the disposition).

### 11.3 Empirical pin #3 — In-flight HTTP request behavior during drain

**Probe configuration:** booted Envoy + slow-streaming backend (5KB at 1KB/s = 5s response). Opened `GET /slow` request in the background (curl in async subshell). After ~1.5s, fired `POST /drain_listeners`. Then attempted a NEW connection. Then waited for the in-flight request to complete.

**Verbatim Envoy in-flight `GET /slow` response (response headers):**

```
HTTP/1.1 200 OK
content-type: text/plain
x-envoy-upstream-service-time: 0
date: Sat, 02 May 2026 23:18:17 GMT
server: envoy
transfer-encoding: chunked

xxxxxxxxx ... [5120 bytes of 'x' streamed in 5 1KB chunks at 1KB/s] ...
```

(Total response 5279 bytes — headers + 5120 body bytes + chunked-encoding overhead. Full body delivery; no abort.)

**Verbatim NEW-connection attempt during drain (curl -v output):**

```
*   Trying 127.0.0.1:10000...
* Connected to 127.0.0.1 (127.0.0.1) port 10000
> GET / HTTP/1.1
> Host: 127.0.0.1:10000
> User-Agent: curl/8.5.0
> Accept: */*
>
* Empty reply from server
* Closing connection
curl: (52) Empty reply from server
```

**Conclusions (pinned):**
- (a) The in-flight HTTP/1.1 request COMPLETES NORMALLY during drain: full body delivery (5KB), 200 OK status, no abort.
- (b) The in-flight HTTP/1.1 response carries NO `Connection: close` header. The keep-alive connection remains open after the response completes; subsequent requests on the same conn would be processed normally (extending the drain window).
- (c) The new-connection attempt during drain: TCP handshake completes (`Connected to 127.0.0.1`), client sends GET request, server closes the connection (FIN) without writing any HTTP response — `Empty reply from server` per curl error 52.
- (d) HTTP/2 GOAWAY behavior: NOT empirically pinned in this probe (the §7 fixture is HTTP/1.1-only; envoy-go MVP emits GOAWAY at drain-trigger on existing H2 connections per design — see §2.1 deferral note; the timing is not asserted differentially).
- **Surprise:** BRAINSTORM Decision 7's hypothesis "does Envoy emit Connection: close on H1.1 in-flight responses during drain?" is answered NO — Envoy v1.37.2 does not mark the in-flight H1.1 response with Connection: close. envoy-go matches.
- Settles BRAINSTORM Decision 7 H1.1 in-flight contract (no Connection: close on H1.1 in-flight responses; full body delivery).
- Settles BRAINSTORM Decision 7 + BRAINSTORM §11.3 in-flight-completion question (full body delivery; no abort).
- Lands in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Drain semantics` connection-level paragraph (§13.4) and ADR-0096 consequence section.

### 11.4 Empirical pin #4 — POST /drain_listeners method-discrimination behavior

**Verbatim Envoy `GET /drain_listeners` response:**

```
HTTP/1.1 405 Method Not Allowed
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:19:14 GMT
server: envoy
transfer-encoding: chunked

Method GET not allowed, POST required.
```

**Verbatim Envoy `PUT /drain_listeners` response:**

```
HTTP/1.1 405 Method Not Allowed
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:19:14 GMT
server: envoy
transfer-encoding: chunked

Method PUT not allowed, POST required.
```

**Verbatim Envoy `DELETE /drain_listeners` response:**

```
HTTP/1.1 405 Method Not Allowed
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:19:14 GMT
server: envoy
transfer-encoding: chunked

Method DELETE not allowed, POST required.
```

**Verbatim Envoy `HEAD /drain_listeners` response (status line + headers, no body per HEAD semantics):**

```
HTTP/1.1 405 Method Not Allowed
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 23:19:14 GMT
server: envoy
transfer-encoding: chunked
```

**Verification side-channel:** after issuing `GET /drain_listeners`, scraped `GET /ready` — returned `200 LIVE\n`, confirming that the GET DID NOT trigger drain (the 405 is a hard rejection, not an accept-and-trigger).

**Conclusions (pinned):**
- (a) Upstream Envoy v1.37.2 ENFORCES method discrimination on `/drain_listeners`. Non-POST methods (GET, PUT, DELETE, HEAD) return 405 Method Not Allowed.
- (b) Body shape: `Method <METHOD> not allowed, POST required.\n` (38 + len(METHOD) bytes; trailing newline). Body is method-name-templated.
- (c) Header set: identical to the standard six-header set per §11.6.
- (d) GET does NOT trigger drain (the rejection is hard; no side effect).
- **Surprise:** this CONTRADICTS BRAINSTORM Decision 3's hypothesis (which expected Envoy parity = no method check, mirroring the read-only endpoints' behavior per 08.1 SPEC §11.8). `/drain_listeners` is the FIRST admin endpoint in upstream Envoy v1.37.2 with method enforcement. envoy-go MVP MUST implement the 405 path to maintain Envoy parity. ADR-0093 codifies this; ADR-0090 is partially amended (the no-method-discrimination posture applies ONLY to read-only endpoints; mutating endpoints DO enforce method discrimination).
- Settles BRAINSTORM Decision 3 method-discrimination question (Envoy enforces 405 on non-POST; envoy-go matches).
- Lands in `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` method-discrimination paragraph (§13.1) and ADR-0093 consequence section + ADR-0090 amendment.

### 11.5 Empirical pin #5 — New connection rejection mechanism during drain

**Verbatim Envoy NEW-connection during DRAINING (curl -v output, from the §11.3 probe):**

```
*   Trying 127.0.0.1:10000...
* Connected to 127.0.0.1 (127.0.0.1) port 10000
> GET / HTTP/1.1
> Host: 127.0.0.1:10000
> User-Agent: curl/8.5.0
> Accept: */*
>
* Empty reply from server
* Closing connection
curl: (52) Empty reply from server
```

**Verbatim raw nc attempt (separate probe — empty stdout, immediate exit):**

```
[no output]
```

(The `nc -w 2 127.0.0.1 10000` command sent the GET request and read 0 bytes back before nc's idle-timeout fired — confirming the server closed the connection without writing.)

**Conclusions (pinned):**
- (a) The TCP 3-way handshake COMPLETES (curl prints `Connected to 127.0.0.1 (127.0.0.1) port 10000`). This is NOT a kernel-level RST-on-no-listener.
- (b) The server READS the client's request (or at least accepts it from the kernel's TCP backlog), then CLOSES the connection (FIN) WITHOUT writing any HTTP response. curl observes "Empty reply from server" (curl error 52).
- (c) NO HTTP-layer `503 Service Unavailable` is emitted. The rejection is at the transport layer (post-handshake FIN), not at the HTTP layer.
- (d) Mechanism: accept-and-immediately-close (Envoy's Accept loop checks the drain state; on drain, the accepted conn is closed via the kernel's `close()` syscall, which sends FIN). NOT listener-socket-close (which would produce kernel RST-on-no-listener for new connections).
- Settles BRAINSTORM Decision 5 close-mechanism question (accept-then-FIN confirmed; listener-socket-close rejected).
- Lands in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Drain semantics` new-connections paragraph (§13.4) and ADR-0094 consequence section.

### 11.6 Empirical pin #6 — Header set across the /drain_listeners endpoint + DRAINING /ready response

**Header set extracted from §11.1 (POST /drain_listeners) and §11.2 (DRAINING /ready):**

Both endpoints emit IDENTICAL header set (modulo Content-Type):

```
content-type: text/plain; charset=UTF-8        (both /drain_listeners and DRAINING /ready)
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: <IMF-fixdate>
server: envoy
transfer-encoding: chunked
```

The header set MATCHES the existing 08.1 admin-endpoint umbrella header set verbatim (per 08.1 SPEC §11.5 + §11.6). No DRAINING-specific headers (no `Connection: close`, no `X-Envoy-Drain`, etc.). No method-discrimination-specific headers either (the 405 responses in §11.4 emit the same six-header set).

**Conclusions (pinned):**
- (a) The header set is UNIFORM across all seven admin endpoints (six pre-08.2 + /drain_listeners). No new header allow-list extensions in 08.2.
- (b) The framing deviation pattern (Envoy=chunked, envoy-go=Content-Length) extends from phase-01's `/ready` to all seven admin endpoints in 08.2. The differential harness's existing dechunk path covers all seven; no new harness code required.
- (c) The 405 method-discrimination responses (§11.4) emit the SAME six-header set as the 200 / 503 responses. envoy-go's `writeAdminHeaders` helper (08.1 `internal/admin/headers.go`) covers the 405 path verbatim — no separate header-helper for the error path.
- Confirms 08.1 SPEC §11.5 framing finding (transfer-encoding: chunked upstream; envoy-go Content-Length).
- Confirms 08.1 SPEC §11.6 header-set finding (six-header set; no admin-endpoint variation).
- Lands in `BEHAVIOR_CONTRACT.md ## Admin API` umbrella header-set paragraph (extended from 08.1's enumeration; §13.1 + §13.4).

### 11.7 Empirical pin #7 — SIGTERM-vs-SIGINT distinct behavior + drain timeout default

**Probe configuration:** booted Envoy with default config (no in-flight requests, no drain triggered). Sent SIGTERM via `docker kill --signal=SIGTERM <ctr>`. Observed exit timing + log evidence. Repeated for SIGINT.

**Verbatim Envoy SIGTERM log evidence:**

```
[2026-05-02 23:19:49.736][1][warning][main] [source/server/server.cc:987] caught ENVOY_SIGTERM
[2026-05-02 23:19:49.736][1][info][main] [source/server/server.cc:1128] shutting down server instance
[2026-05-02 23:19:49.736][1][info][main] [source/server/server.cc:1068] main dispatch loop exited
[2026-05-02 23:19:49.742][1][info][main] [source/server/server.cc:1120] exiting
```

(Total elapsed: 6ms from `caught ENVOY_SIGTERM` to `exiting`. Container wall-clock exit: ~250ms — Docker overhead included.)

**Verbatim Envoy SIGINT log evidence:**

```
[2026-05-02 23:19:52.336][1][warning][main] [source/server/server.cc:992] caught SIGINT
[2026-05-02 23:19:52.336][1][info][main] [source/server/server.cc:1128] shutting down server instance
[2026-05-02 23:19:52.336][1][info][main] [source/server/server.cc:1068] main dispatch loop exited
[2026-05-02 23:19:52.343][1][info][main] [source/server/server.cc:1120] exiting
```

(Total elapsed: 7ms from `caught SIGINT` to `exiting`. Container wall-clock exit: ~252ms.)

**Default drain timeout (re-confirmed from 08.1 SPEC §11.4 line 760):**

```
"drain_time": "600s",
"drain_strategy": "Gradual",
```

(Per the `/server_info` `command_line_options` field; envoy-go does NOT model this field per ADR-0088.)

**Conclusions (pinned):**
- (a) SIGTERM and SIGINT in upstream Envoy v1.37.2 follow STRUCTURALLY IDENTICAL paths: `caught X` → `shutting down server instance` → `main dispatch loop exited` → `exiting`. Total elapsed ~6–7ms (no drain delay, no wait-for-in-flight, no per-cluster pool close). Both signals trigger immediate-exit-without-drain.
- (b) Default drain timeout is 600s (10 minutes); default drain strategy is "Gradual" (the only strategy in the v1.37.2 default-config flow); these defaults are CONSULTED by the `/drain_listeners` endpoint but NOT triggered by SIGTERM/SIGINT.
- (c) **Surprise:** this CONTRADICTS BRAINSTORM Decision 2's hypothesis (which assumed Envoy's SIGTERM = drain-then-exit, treating envoy-go's choice as Envoy parity). Empirical evidence shows Envoy's SIGTERM is immediate-exit. envoy-go's design choice (per ADR-0092) is to make SIGTERM/SIGINT trigger drain-then-exit anyway — a DELIBERATE DIVERGENCE from upstream Envoy. The justification is operator-ergonomic: most Kubernetes / cluster orchestrators send SIGTERM expecting graceful drain (rolling-restart workflow), and envoy-go's in-process drain machinery is the natural way to honor this expectation. The differential equivalence claim does NOT exercise the SIGTERM path; the divergence is documented at the BEHAVIOR_CONTRACT level (§13.4 ## Graceful drain ### Drain triggers).
- Settles BRAINSTORM Decision 2 SIGTERM/SIGINT semantics (Envoy v1.37.2 SIGTERM = immediate-exit; envoy-go diverges intentionally to drain-then-exit).
- Settles BRAINSTORM Decision 6 drain timeout default (envoy-go MVP 30s; Envoy v1.37.2 600s; deliberate divergence per ADR-0095).
- Confirms 08.1 SPEC §11.4 line 760 verbatim `"drain_time": "600s"` (re-validated under the 08.2 SPEC's empirical-pin probe).
- Confirms 08.1 SPEC §11.4 line 773 verbatim `"drain_strategy": "Gradual"` (re-validated; Gradual is the only strategy in the v1.37.2 default-config flow).
- Lands in `BEHAVIOR_CONTRACT.md ## Graceful drain ### Drain triggers` paragraph (§13.4) + ADR-0092 consequence section + ADR-0095 (timeout default) consequence section.

### 11.8 Synchronization with BEHAVIOR_CONTRACT.md

The §11.1–§11.7 verbatim blocks above are paste-verbatim-synchronized with the `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` + `### /ready` (DRAINING extension) + `### /server_info` (DRAINING extension) + `## Graceful drain` (umbrella) sections that 08.2's implementation lands in §13. No drift permitted: future image bumps (per ADR-0008's pin-refresh procedure) require re-running the seven probes and updating both this SPEC §11 and BEHAVIOR_CONTRACT.md `## Admin API` + `## Graceful drain` in the same commit. (Mirrors 08.1 SPEC §11.10's resync discipline.)

---

## 12. Deferred decisions (the planner / implementer settles these)

The following decisions are NOT settled by the BRAINSTORM nor by §11's empirical evidence; they are deferred to the planning session (`superpowers:writing-plans` for PLAN.md) or to the implementer at the corresponding task:

1. **`FuzzDrainTransitions` ship-or-skip choice.** Per BRAINSTORM §8.5, the SPEC author recommends shipping the fuzzer (~60 LoC; 30s budget per ADR-0018) — the state machine is small but the concurrent-operation interleaving space is non-trivial; ADR-0018's fuzzer discipline generalizes to "every concurrent state machine ships a fuzzer where reasonable". Alternative: skip (the unit-test surface in `manager_test.go` exhaustively covers the state-transition matrix). Recommendation: **ship**, with the fuzzer asserting state-monotonicity (state never decreases) + inflight-balance under randomized Inc/Dec sequences + Done-fires-once invariant.
2. **`drain.New(timeout)` argument validation.** What does the Manager do if `timeout <= 0` is passed? Recommendation: **document that timeout should be > 0; trust the caller**; the SIGTERM-handler in `cmd/envoy-go/main.go` always passes 30s; test code may pass small values like 10ms. No defensive panic / clamp.
3. **`Manager.Done()` semantics when Drain has not been called.** Returns an open channel that NEVER closes (until Drain is called and the rendezvous condition fires). This is the natural Go channel pattern; no deferred decision needed beyond documenting the precondition. Recommendation: **document the precondition**; the SIGTERM-handler always calls Drain before selecting on Done.
4. **HCM `markedInflight` field placement.** On `Stream` (per BRAINSTORM Decision 7 sketch) or on a separate "request context" struct? Recommendation: **on `Stream`**; the Stream is the natural per-request lifetime owner; placing the flag elsewhere would require additional plumbing.
5. **TCP-proxy Inc/Dec at OnNewConnection vs. at OnFirstReadByte.** Per BRAINSTORM Decision 7, OnNewConnection is the chosen anchor. Alternative: increment lazily at first byte (avoids counting accepted-but-silent connections). Recommendation: **OnNewConnection** (per BRAINSTORM); the lazy-increment alternative complicates the OnConnectionClose pair-balance and is not justified by any current operator workflow.
6. **`internal/cluster/manager.go` `closePool()` per-cluster method shape.** What is the closePool signature and what does it actually close? Recommendation: **`func (c *Cluster) closePool()`** with no return value; iterates the cluster's HTTP/1.1 keep-alive pool (close any pooled idle conns), the HTTP/2 ClientConn instances from phase 05.2 (call `cc.Close()`), and the TLS upstream connections from phase 03 (the `tls.Conn` instances inside the H1.1/H2 pools cover this). Best-effort; ignore errors.
7. **`drainMgr` boot-order placement in `cmd/envoy-go/main.go`.** Should it be before or after `bootstrap.Load`? Recommendation: **after `bootstrap.Load` and before `cluster.NewManager...`**; the drain manager has no dependencies but is consumed by all subsequent constructors; placing it after Load and before the first constructor that consumes it is the cleanest ordering.
8. **fixture 0010 driver framework reuse.** Should the driver share utilities with 0009-admin-config-dump's driver (canonicalisation, dual-proxy boot helpers, etc.)? Recommendation: **share where natural** (dual-proxy boot helpers per N-4 carry-forward; canonicalisation: NO, because 0010's per-state-transition byte-equality is structurally different from 0009's structural-projection canonicalisation). The shared helpers move to a `test/differential/helpers/` location at PLAN time if not already there.
9. **`cm.Drain()` call ordering vs. deferred-stop chain.** Per BRAINSTORM §5.1 swimlane t5 + Decision 8, `cm.Drain()` is called AFTER the rendezvous and BEFORE the deferred-stop chain. Alternative: place `cm.Drain()` as a deferred call. Recommendation: **explicit call after rendezvous** (per BRAINSTORM); the deferred-call ordering would intersperse the upstream-pool close inside the listener-socket-close, which is correct but harder to read. The explicit call is grep-discoverable and easier to reason about.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 `## Admin API ### /drain_listeners` NEW subsection (verbatim Markdown patch)

Inserted under the `## Admin API` umbrella, after the `### /server_info` subsection (preserving alphabetical-by-path order: /clusters, /config_dump, /drain_listeners, /listeners, /ready, /server_info, /stats/prometheus):

```markdown
### /drain_listeners
   **Body shape (POST).** `text/plain; charset=UTF-8`. Body verbatim `OK\n` (3 bytes; capital `OK` followed by single newline) per 08.2 SPEC §11.1 empirical pin against Envoy v1.37.2. Status 200 OK. The handler is fire-and-forget — 200 OK is emitted BEFORE drain completes; the operator polls /ready or /server_info to observe drain progress. Idempotent — subsequent POSTs during DRAINING return 200 with the same body without re-firing the drain trigger (sync.Once-guarded internally).

   **Method discrimination.** Non-POST methods (GET, PUT, DELETE, HEAD) return `405 Method Not Allowed` with body `Method <METHOD> not allowed, POST required.\n` per 08.2 SPEC §11.4 empirical pin. This is the FIRST admin endpoint in envoy-go with method enforcement; partially amends ADR-0090's no-method-discrimination posture (which applies uniformly to read-only endpoints; ADR-0093 records the qualification).

   **`?graceful=true` query-param.** Silently accepted (per ADR-0041's silent-ignore precedent). envoy-go's drain is always graceful by construction (the three-state machine has no non-graceful immediate-stop variant); the query-param has no semantic effect.

   **Side effects.** First POST: `drain.Manager.Drain()` called (Live → Draining transition); subsequent POSTs: no-op. The endpoint does NOT trigger process exit — the operator-driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT (or kill -9).

   **Cross-trigger note.** Upstream Envoy v1.37.2 separates the listener-side drain (POST /drain_listeners — does NOT flip /ready or /server_info to DRAINING) from the load-balancer-disposition flip (POST /healthcheck/fail — DOES flip /ready and /server_info to DRAINING). envoy-go's MVP UNIFIES these triggers under a single drain manager: POST /drain_listeners DOES flip /ready and /server_info to DRAINING in envoy-go (the BODY shapes match Envoy verbatim per §11.2; the TRIGGERS differ at the wiring level). The differential gate's per-proxy trigger script normalizes per 08.2 SPEC §7.2.

   **Empirical evidence (verbatim Envoy v1.37.2 `POST /drain_listeners`):** see 08.2 SPEC §11.1.

   **Empirical evidence (verbatim Envoy v1.37.2 `GET/PUT/DELETE/HEAD /drain_listeners`):** see 08.2 SPEC §11.4.

   **Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 (after framing dechunk). Method-discrimination behavior asserted byte-equal — non-POST returns 405 with the templated body across both proxies. Header set inherits the umbrella rules.
```

### 13.2 `## Admin API ### /ready` extension (verbatim Markdown patch)

Append a new sub-block after the existing `Pre-init response` block under `### /ready`:

```markdown
**DRAINING-state response (08.2 NEW).** When `drain.Manager.State() == DRAINING`, the handler returns 503 Service Unavailable with body `DRAINING\n` (9 bytes; uppercase `DRAINING` followed by single newline) per 08.2 SPEC §11.2 empirical pin. The DRAINING check has precedence over both LIVE and PRE_INITIALIZING — once drain has fired, /ready returns the DRAINING body even if MarkReady has been called and even if /server_info would otherwise return state="LIVE". Header set inherits the umbrella rules.

**Empirical evidence (verbatim Envoy v1.37.2 `/ready` during DRAINING):** see 08.2 SPEC §11.2.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 in DRAINING state. Status 503 byte-equal.

**Forward-pointer note.** ADR-0015 (pre-init contract for /ready) is **partially superseded by ADR-0097**: the LIVE / PRE_INITIALIZING two-state coverage extends to LIVE / PRE_INITIALIZING / DRAINING three-state coverage. ADR-0015's verbatim pre-init body and pre-init status are preserved; ADR-0097 adds the DRAINING branch and the precedence rule.
```

### 13.3 `## Admin API ### /server_info` extension (verbatim Markdown patch)

Modify the State enum bullet under `### /server_info`'s body-shape paragraph:

```markdown
State enum (08.2 EXTENDED): `LIVE` (post-MarkReady, drain has not fired), `PRE_INITIALIZING` (pre-MarkReady, drain has not fired), `DRAINING` (drain has fired — supersedes LIVE and PRE_INITIALIZING). `INITIALIZING` is documented in `adminv3.ServerInfo_State` but unreachable in envoy-go's static-bootstrap-only model (08.1 SPEC §11.7).

**Equivalence claim extension (08.2):** the `state` field IS asserted byte-equal across both proxies in DRAINING (`"DRAINING"` literal, per 08.2 SPEC §11.2 empirical pin). The 08.1 byte-equal claim for `"LIVE"` post-MarkReady is unchanged.

**Forward-pointer note.** ADR-0088 is **amended** by ADR-0098 (NOT superseded — purely additive per ADR-0088 consequence (c) verbatim). The ADR-0088 amendment record adds DRAINING to the enum-coverage table and refers to ADR-0098 for the timing semantics.
```

### 13.4 `## Graceful drain` NEW umbrella section (verbatim Markdown patch)

Insert a new sibling section to `## Admin API`, placed immediately after `## Admin API` in BEHAVIOR_CONTRACT.md:

```markdown
## Graceful drain

The envoy-go drain machinery transitions the process from LIVE → DRAINING → exit (via SIGTERM/SIGINT) or LIVE → DRAINING (via POST /drain_listeners; no exit). The state machine lives in the `internal/drain` package (08.2 NEW; ADR-0091); the drain manager is a single-instance lock-free state machine with three states (LIVE / DRAINING / DRAINED) and an in-flight counter.

### Drain triggers

Two operator workflows trigger drain in envoy-go:

1. **SIGTERM or SIGINT:** drain-then-exit. The signal causes `cmd/envoy-go/main.go`'s top-level context to cancel; the main goroutine then calls `drain.Manager.Drain()`, waits on `drain.Manager.Done()` (or a 30s timeout per ADR-0095), then proceeds to per-cluster connection-pool teardown + listener-socket close + admin server close + access-log flush. The total drain window is bounded by the 30s timeout.

   **Deliberate divergence from Envoy v1.37.2** (per 08.2 SPEC §11.7 empirical pin): upstream Envoy v1.37.2 SIGTERM is immediate-exit-without-drain (the log shows `caught ENVOY_SIGTERM` → `shutting down server instance` → `exiting` within ~7ms; no drain delay). envoy-go's design choice (ADR-0092) is to honor the operator-ergonomic expectation that SIGTERM = graceful drain (the dominant Kubernetes / cluster-orchestrator workflow). The differential equivalence claim does NOT exercise the SIGTERM path; the divergence is contract-level.

2. **POST /drain_listeners admin endpoint:** drain-without-exit. The handler triggers `drain.Manager.Drain()` synchronously and returns 200 OK before drain completes. The proxy stays running in DRAINING indefinitely; the operator separately issues SIGTERM/SIGINT (or kill -9) at a later time to actually exit.

Both triggers result in the same drain BEHAVIOR (state transition, listener stop-accepting, in-flight completion, /ready and /server_info responses). They differ only in the post-drain disposition (exit vs. stay-running).

### Drain semantics

When drain fires (state transitions LIVE → DRAINING):

- **New connections rejected via accept-then-FIN.** The Listener Accept loop's fast-path checks `drain.Manager.IsDraining()` on each iteration; an Accept-ed conn during DRAINING is immediately closed (`conn.Close()` → kernel sends FIN) without filter-chain dispatch. Per 08.2 SPEC §11.5 empirical pin: the TCP 3-way handshake completes; the client observes "Empty reply from server" on its first read attempt. NOT listener-socket-close (which would produce kernel RST-on-no-listener for new connections).

- **In-flight requests complete normally.** The HCM filter chain's `decodeHeaders`/`encodeFinalize` pair calls `drain.Manager.Inc()`/`Dec()` to track per-request in-flight count; the drain manager's `Done()` channel closes when the in-flight counter reaches 0 (or the 30s timeout fires). Per 08.2 SPEC §11.3 empirical pin: in-flight HTTP/1.1 requests during drain receive full body delivery with status 200 (no abort), and the response carries NO `Connection: close` header — the keep-alive connection remains open after the response (subsequent requests on the same conn extend the drain window via further Inc calls; deliberate MVP simplification, per-conn drainable-close-at-next-idle-window deferred).

- **TCP-proxy connections complete at connection-close.** TCP-proxy filter's `OnNewConnection`/`OnConnectionClose` pair calls `Inc()`/`Dec()` per connection (correct because TCP-proxy has no per-request semantic).

- **/ready returns 503 DRAINING\n** (per 08.2 SPEC §11.2 verbatim). Operators / load balancers observe the DRAINING signal and stop sending traffic.

- **/server_info returns `state: "DRAINING"`** (per ADR-0098 amending ADR-0088).

- **Idempotent.** Subsequent Drain() calls (e.g., a second POST /drain_listeners, or SIGTERM after a prior /drain_listeners) no-op — the state transition has already fired (sync.Once-guarded).

### Drain timeout

The drain timeout is a hardcoded 30s in envoy-go MVP (per ADR-0095). Envoy v1.37.2's default is 600s (per 08.2 SPEC §11.7 + 08.1 SPEC §11.4 verbatim `"drain_time": "600s"`). The divergence is deliberate to keep test-suite cost tractable; the drain BEHAVIOR is the equivalence claim, not the timeout VALUE. Operator-knob to configure the timeout is deferred to a future runtime / hot-restart family phase.

The drain strategy in upstream Envoy v1.37.2 is `"Gradual"` (the only strategy in the v1.37.2 default-config flow per 08.2 SPEC §11.7 + 08.1 SPEC §11.4). envoy-go's drain is graceful-by-construction (no IMMEDIATE strategy); the strategy concept is not modeled.

### Connection-level drain semantics

Phase 08.2 does NOT implement per-connection drainable closure at next-idle-window (Envoy supports this via `drain_strategy: "Gradual"`'s back-off). HTTP/1.1 keep-alive connections during drain do NOT receive `Connection: close` on the in-flight response (per 08.2 SPEC §11.3 empirical pin matching Envoy parity); subsequent requests on the same conn during DRAINING are processed normally (extending the drain window). HTTP/2 connections during drain emit GOAWAY at drain-trigger (envoy-go MVP design choice; not asserted differentially per 08.2 SPEC §2.1 deferral note).

### Drain manager API surface

- `internal/drain.New(timeout time.Duration) *drain.Manager` — constructor; state initialized to Live.
- `(m *Manager).State() drain.State` — atomic load; returns Live or Draining.
- `(m *Manager).Drain()` — sync.Once-guarded; transitions Live → Draining; arms the Done rendezvous.
- `(m *Manager).Done() <-chan struct{}` — channel closes when inflight reaches 0 after Drain has fired.
- `(m *Manager).Inc()` / `(m *Manager).Dec()` — atomic increment/decrement of inflight counter.
- `(m *Manager).IsDraining() bool` — Listener Accept-loop fast-path; equivalent to State() == Draining.
- `(m *Manager).Timeout() time.Duration` — returns the configured timeout (read-only).

### Applies to

- phase 08.2 envoy-go drain subsystem.
- the SIGTERM/SIGINT-handler in `cmd/envoy-go/main.go` (ADR-0092; deliberate divergence from Envoy parity).
- the POST /drain_listeners admin endpoint (ADR-0093; method discrimination per Envoy parity).
- the /ready DRAINING-state body (ADR-0097; partially supersedes ADR-0015).
- the /server_info DRAINING-state field (ADR-0098; amends ADR-0088).
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to

- Hot restart / parent-child handoff (deferred to runtime / hot-restart family per ADR-0099).
- POST /quitquitquit endpoint (semantic overlap with SIGTERM + /drain_listeners; deferred per ADR-0089 + ADR-0099).
- POST /healthcheck/fail endpoint (envoy-go MVP unifies the listener-drain and load-balancer-disposition triggers under /drain_listeners + the drain manager; /healthcheck/fail stays deferred per ADR-0089).
- Per-listener selective drain (`/listeners/<name>/drain` admin sub-routes deferred per ADR-0089).
- `drain_strategy` per-listener (default GRADUAL only; IMMEDIATE strategy deferred).
- Configurable drain timeout (hardcoded 30s; operator-knob deferred per ADR-0095).
- Per-connection drainable closure at next-idle-window.
- Drain manager interaction with xDS (no xDS yet; deferred).
- HTTP/3 drain semantics (no H3 in MVP; deferred to HTTP/3 + QUIC family).
- Drain progress JSON body on /server_info (envoy-go matches Envoy's empty-of-this-field behavior).
- `Connection: drain` custom response header (Envoy emits no such header per 08.2 SPEC §11.3).
- Multi-instance drain coordination (operator's load-balancer responsibility).
```

### 13.5 New `## Equivalence Matrix` rows (verbatim table-row patch)

Appended to the `## Equivalence Matrix` table at the head of BEHAVIOR_CONTRACT.md:

```markdown
| Admin /drain_listeners      | Body byte-equal `OK\n` (POST); 405 + body `Method <X> not allowed, POST required.\n` (non-POST). Idempotent semantics; query-param ?graceful=true silent-ignored. | Header set inherits umbrella rules; framing per phase-01 dechunk-discipline. Method-discrimination is the FIRST envoy-go endpoint with 405 enforcement (per ADR-0093 partially amending ADR-0090).                                                                              |
| Admin /ready (DRAINING)     | Body byte-equal `DRAINING\n` to reference Envoy v1.37.2 in DRAINING state. Status 503.                                                                                  | DRAINING precedence over LIVE / PRE_INITIALIZING. Status 503 (matches PRE_INITIALIZING). Header set inherits umbrella rules. Per-proxy trigger script normalization per 08.2 SPEC §7.2 (envoy-go: /drain_listeners; ref Envoy: /drain_listeners + /healthcheck/fail).         |
| Admin /server_info (DRAINING) | The `state` field IS asserted byte-equal (`"DRAINING"`) when both proxies are in DRAINING. Other fields per ADR-0088 allow-list.                                       | Inherits ADR-0088 allow-list for non-state fields (version, uptime_*, command_line_options, hot_restart_version, node).                                                                                                                                                       |
```

(Three rows; the first is a NEW dimension; the second and third are EXTENSIONS to the existing 08.1 rows for /ready and /server_info — the implementation may choose to write three new rows OR extend the existing rows in-place; the implementer settles. Per the 08.1 SPEC §13.2 row-shape pattern.)

### 13.6 Header allow-list extensions

No new header allow-list extensions in 08.2. The phase-01 `Date` and `Server` allow-list rows (already in `## Header allow-list`) cover the new endpoint unchanged. The `Content-Length` vs `transfer-encoding: chunked` deviation is structural (handled by the differential harness's dechunk preprocessor) — no allow-list entry; see the §11.6 finding.

### 13.7 Forward-pointer notes (per BRAINSTORM §9 inline supersessions/amendments)

- **ADR-0015** (pre-init contract for `/ready`): partially superseded by **ADR-0097** (DRAINING extension). The pre-init body and status are preserved verbatim; ADR-0097 adds the DRAINING branch + precedence rule.
- **ADR-0088** (`/server_info` body shape; state enum coverage `LIVE` + `PRE_INITIALIZING`): amended by **ADR-0098** (DRAINING enum coverage added). Per ADR-0088 consequence (c), the amendment is purely additive; recorded as an in-place edit of ADR-0088's Consequences section per the ADR-0089 consequence (b) pattern.
- **ADR-0090** (no-ACL admin-endpoint security posture; no method discrimination on read-only endpoints): partially amended by **ADR-0093** (mutating endpoint /drain_listeners DOES enforce method discrimination per §11.4). The no-ACL posture is preserved verbatim; the no-method-discrimination posture is qualified to read-only endpoints only.

---

## 14. Testing strategy (per BRAINSTORM §8)

### 14.1 Unit tests (`internal/drain/`)

- `manager_test.go`:
  - `TestStateTransitions` — Live → Draining via Drain(); Draining stays in Draining (idempotent); Drained transition fires Done() channel close.
  - `TestInflightBalance` — Inc/Dec balance: 1 Inc + 1 Dec brings inflight to 0; multiple Inc + matching Dec balance.
  - `TestDrainCompletionRendezvous` — Drain() then Inc once; Dec once; Done() unblocks.
  - `TestDrainTimeout_NoInflight` — Drain() with no Inc; Done() unblocks immediately (inflight already 0). The timeout is not exercised in this case.
  - `TestDrainTimeout_StuckInflight` — Drain() with one Inc + no Dec; the caller selects on Done() with a small time.After; the time.After fires (the Manager itself does not enforce timeout per the ADR-0095 design — the caller does).
  - `TestIdempotentDrain` — multiple Drain() calls; only one transition fires; Done() unblocks once (closeOnce-guarded).
  - `TestIsDrainingFastPath` — pre-Drain: false. Post-Drain: true. Atomic load is lock-free.
  - `TestNilSafety` — calling methods on a `nil *Manager` documented behavior (panic on dereference, since methods are pointer-receiver; tests assert via `defer recover()`).
  - `TestConcurrentIncDec` — race-detector-clean test: 100 goroutines × 1000 Inc/Dec pairs; assert final inflight == 0. Run under `go test -race`.

### 14.2 Unit tests (`internal/admin/`)

- `drain_test.go` (NEW):
  - `TestHandleDrainListeners_PostFires` — POST /drain_listeners; assert 200 + verbatim body `OK\n`; assert `s.dm.State() == StateDraining` post-call.
  - `TestHandleDrainListeners_BodyExact` — assert body byte-exact `OK\n` (3 bytes; no trailing whitespace).
  - `TestHandleDrainListeners_Idempotent` — two POSTs; both 200 with same body; only one Drain() transition (assert via mock).
  - `TestHandleDrainListeners_GraceQueryParamSilentlyIgnored` — POST /drain_listeners?graceful=true; assert 200 + verbatim body `OK\n`.
  - `TestHandleDrainListeners_NilDrainManager` — handler with `s.dm == nil`; assert defensive 500 OR no-op + 200 (per planner-time decision in §12; recommended: defensive 500).
  - `TestHandleDrainListeners_GetReturns405` — GET /drain_listeners; assert 405 + body `Method GET not allowed, POST required.\n`.
  - `TestHandleDrainListeners_PutReturns405` — PUT; assert 405 + body `Method PUT not allowed, POST required.\n`.
  - `TestHandleDrainListeners_DeleteReturns405` — DELETE; assert 405 + body `Method DELETE not allowed, POST required.\n`.
  - `TestHandleDrainListeners_HeadReturns405WithEmptyBody` — HEAD; assert 405 + headers per §11.4 + empty body (HEAD semantics).
  - `TestHandleDrainListeners_HeaderSet` — assert all six standard headers present per §11.6.
- `admin_test.go` (modified):
  - `TestHandleReady_Draining` — set `s.dm` to a Manager with state=Draining; assert /ready returns 503 + body `DRAINING\n`.
  - `TestHandleReady_DrainingPrecedesPreInitializing` — set `s.dm` to Draining BEFORE MarkReady; assert /ready returns 503 + DRAINING\n (NOT PRE_INITIALIZING\n).
  - `TestHandleReady_DrainingPrecedesLive` — set `s.dm` to Draining AFTER MarkReady; assert /ready returns 503 + DRAINING\n (NOT 200 + LIVE\n).
  - `TestHandleReady_DrainingHeaders` — assert standard six-header set on the DRAINING response per §11.6.
  - `TestAdminConcurrentScrapeRace` (extended) — 100 goroutines × seven endpoints × 1s + a separate goroutine firing `s.dm.Drain()` once mid-test. Race-detector clean; no panic; no malformed responses.
- `serverinfo_test.go` (modified):
  - `TestHandleServerInfo_StateDraining` — set `s.dm` to Draining; assert state == "DRAINING".
  - `TestHandleServerInfo_StatePrecedence_LiveOverDraining` — Draining + Ready: state == "DRAINING" (not "LIVE").
  - `TestHandleServerInfo_StatePrecedence_PreInitOverDraining` — Draining + NOT Ready: state == "DRAINING" (not "PRE_INITIALIZING").
  - `TestDeriveState_NilDrainManager` — `deriveState(&ready, nil)` returns LIVE/PRE_INITIALIZING per existing logic; nil-tolerant.

### 14.3 Unit tests (`internal/listener/manager.go`, `internal/cluster/manager.go`)

- `internal/listener/manager_test.go`:
  - `TestManager_Drain` — call Drain(); assert `m.dm.IsDraining()` returns true; subsequent Accept-loop iterations close the new conn (verify via a mock listener that returns conns from Accept()).
  - `TestManager_DrainIdempotent` — multiple Drain() calls; idempotent.
  - `TestManager_AcceptDuringDrainClosesConn` — exercise the Accept-loop fast-path: trigger Drain; Accept returns a fake conn; assert the conn is closed without filter-chain dispatch.
  - `TestManager_StopAfterDrain` — Stop() works correctly post-Drain; idempotent w.r.t. itself; closes listening sockets.
- `internal/cluster/manager_test.go`:
  - `TestManager_DrainClosesPools` — call Drain(); assert per-cluster pools are closed (e.g., HTTP/2 ClientConn instances are nil after Drain; HTTP/1.1 keepalive pools are empty).
  - `TestManager_DrainIdempotent` — multiple Drain() calls; idempotent.

### 14.4 Race detector + lint

`go test -race ./...` clean. Specifically:
- The drain manager's `atomic.Uint32` state field is concurrently read by the Listener Accept loop, HCM `decodeHeaders`, admin `handleReady` / `handleServerInfo`; concurrently written by `handleDrainListeners` and the SIGTERM-handler. Race-detector clean is the contract.
- The `atomic.Int64` inflight counter is concurrently incremented by HCM and TCP-proxy; concurrently decremented likewise. Race-detector clean.
- `go vet ./...` and `golangci-lint run ./...` clean (gate (a) per `BOOTSTRAP_PROMPT.md` §7.5).

### 14.5 Fuzzers

OPTIONAL **NEW: `FuzzDrainTransitions`** (`internal/drain/`) — fuzzes a sequence of operations against `*drain.Manager` (Drain, Inc, Dec, IsDraining, Done) and asserts invariants:
- State transitions are monotonic (Live → Draining only; never Draining → Live).
- Inflight balance: total Inc count == total Dec count at every program point.
- Done() fires exactly once.
- Concurrent operations under `t.Parallel()` race-detector-clean.
~60 LoC; 30s budget per ADR-0018. Per §12 deferred decision #1, the SPEC author RECOMMENDS shipping. If shipped: total fuzzer count post-08.2 is **11**.

### 14.6 Existing fuzzers re-run

The 10 (or 11 per the 08.1 REVIEW erratum) existing fuzzers re-run at 30s budget per ADR-0018. None exercise drain machinery; all are mechanical re-runs.

### 14.7 h2spec re-run

08.2 modifies HCM (per BRAINSTORM Decision 7: Inc/Dec hooks at decodeHeaders/encodeFinalize). The hooks add ~5 LoC and do NOT touch the H2 codec, the H2 framer, or the H2 hpack path. The h2spec gate at 53/53 PASS must remain green; re-running is mechanical (gate (c) per ADR-0051). The CONFORMANCE_PINS pin is unchanged.

### 14.8 Differential 0000–0009 + 0010

All pre-existing fixtures `0000–0009` remain green (no regression). NEW fixture `0010-graceful-drain` is differentially green per the §7 driver flow (admin-trigger path against both proxies; SIGTERM-trigger path envoy-go-only).

### 14.9 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Standard six-gate sweep applies; each gate's 08.2 specialization is in §3 above.

---

## 15. Acceptance checklist (for the reviewer of this sub-phase's final state)

When phase 08.2 is `done` (post-phase-done commit), the following are all true:

- [ ] **`internal/drain/` package** lands with `doc.go` + `manager.go` + `manager_test.go` + (optional per §12 #1) `fuzz_test.go`. `Manager` type implements the §6.2 API.
- [ ] **`internal/drain.Manager` LBP-1 fifth-application** threading: `admin.New` widens to take `*drain.Manager` (7th param); `listener.NewManagerWithBaseDirAndAllowH2C` widens to take `*drain.Manager`; HCM filter constructor widens; TCP-proxy filter constructor widens. Build clean.
- [ ] **`internal/admin/drain.go`** lands `handleDrainListeners` per §6.3. POST returns 200 + body `OK\n`; non-POST returns 405 + body `Method <X> not allowed, POST required.\n`. Idempotent.
- [ ] **`mux.HandleFunc("/drain_listeners", s.handleDrainListeners)`** added to `internal/admin.Server.Start()` — seventh handler on the same mux.
- [ ] **`internal/admin/admin.go::handleReady`** modified per §6.4: NEW first branch DRAINING-check; pre-init and live branches preserved unchanged.
- [ ] **`internal/admin/serverinfo.go::deriveState`** modified per §6.5: signature widens to take `*drain.Manager`; NEW first DRAINING-check.
- [ ] **`internal/listener/manager.go::Manager`** gains `dm` field + `Drain()` method per §6.6; per-runtime Accept-loop fast-path checks `m.dm.IsDraining()` and closes the new conn on drain.
- [ ] **`internal/listener/manager.go::Listeners()`** gains the N-1 doc-comment ("order is bootstrap-declaration order; callers needing alphabetical ordering must sort").
- [ ] **`internal/cluster/manager.go::Manager`** gains `Drain()` method per §6.7; walks per-cluster pools and closes them.
- [ ] **HCM `decodeHeaders`/`encodeFinalize`** Inc/Dec pair lands per BRAINSTORM Decision 7 + ADR-0096; `markedInflight` flag on `Stream` ensures balance under sendLocalReply per ADR-0075.
- [ ] **TCP-proxy `OnNewConnection`/`OnConnectionClose`** Inc/Dec pair lands per BRAINSTORM Decision 7 + ADR-0096.
- [ ] **`cmd/envoy-go/main.go`** SIGTERM-handler block upgraded per §6.8: `drainMgr.Drain()` → `select { <-drainMgr.Done(): / <-time.After(timeout): }` → `cm.Drain()` → existing deferred-stop chain.
- [ ] **`drainMgr := drain.New(30 * time.Second)`** allocated in `cmd/envoy-go/main.go` post-bootstrap.Load and pre-cluster.NewManager; threaded into all consumers.
- [ ] **Fixture `test/differential/0010-graceful-drain/`** lands with `README.md`, `expectations.yaml`, `envoy.yaml`, `envoy-go.yaml`, `driver/driver.go` (admin-trigger + SIGTERM-trigger paths), `backends/backend.go`. Registered as `RequiresReference: true` in `runner.go`. Differentially green.
- [ ] **`BEHAVIOR_CONTRACT.md`** populated per §13: NEW `### /drain_listeners` subsection; `### /ready` extension paragraph; `### /server_info` extension paragraph; NEW `## Graceful drain` umbrella section; three new equivalence-matrix rows; ADR-0015 / ADR-0088 / ADR-0090 forward-pointer notes. ADR-0052 in-place edit; no new ADR for the in-place-edit authorization.
- [ ] **Seven to nine new ADRs** (ADR-0091..ADR-0099 per §8) appended to `DECISIONS.md`. ADR-0015 partially superseded by ADR-0097 (recorded in ADR-0097 + at the existing ADR-0015 reference site). ADR-0088 amended by ADR-0098 (purely additive in-place edit per ADR-0089 consequence (b) pattern). ADR-0090 partially amended by ADR-0093 (in-place edit; no-method-discrimination posture qualified to read-only endpoints).
- [ ] **Concurrent-scrape race-test `TestAdminConcurrentScrapeRace`** (extended to seven endpoints + Drain mid-test) clean under `go test -race ./...`.
- [ ] **`go vet ./...` clean**, **`golangci-lint run ./...` clean**, **`go test ./...` clean**, **`go test -race ./...` clean** (gate (a) + (b)).
- [ ] **h2spec re-run** clean at 53/53 PASS (gate (c); ADR-0051 pin unchanged).
- [ ] **Differential fixtures 0000–0009 + 0010** all green (gate (e)).
- [ ] **All 11 fuzzers** (or 10 if `FuzzDrainTransitions` is skipped per §12 #1) run clean at 30s budget (gate (d)).
- [ ] **ROADMAP row `08.2`** flips `in-progress → done` AT the phase-done commit. Parent row `08` SIMULTANEOUSLY flips `in-progress → done` (MVP-trunk closure per parent SPEC §5).
- [ ] **STATE.md** advanced past 08.2 phase-done; flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 (MVP-trunk closure); `next-skill: superpowers:brainstorming` against §9's family list; `last-commit: <08.2 phase-done SHA>`.
- [ ] **PROGRESS.md** + **REVIEW.md** committed per phases 06.1 / 06.2 / 07.1 / 07.2 / 08.1 cadence.
- [ ] **Phase-done commit subject:** `phase 08.2: graceful-drain [ADR-0091, ADR-0092, ADR-0093, ADR-0094, ADR-0095, ADR-0096, ADR-0097, ADR-0098, ADR-0099]` (or fewer per consolidation per §8). Body explicitly names the ROADMAP-row transition (`08.2 → done` AND parent `08 → done` AT THE SAME COMMIT — MVP-trunk closure).

When all boxes above are checked, phase 08.2 is `done`, parent row `08` is `done`, the BOOTSTRAP_PROMPT.md §8 MVP trunk is closed, and the project advances to feature-family expansion (§9) at lifecycle-state 0 (full BRAINSTORM session against the family list) per `BOOTSTRAP_PROMPT.md` §5.

---

## 16. References

- **BRAINSTORM:** `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` §§1–11 (the authoritative design source; this SPEC distills §§1–11 into formal contract language and executes the §11 empirical-pin obligations IN-SESSION per ADR-0004).
- **Parent master SPEC:** `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` (this commit's parent SPEC — the cross-cutting discipline; per §5, parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2's phase-done).
- **Sibling SPEC (08.1):** `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` — the 16-section structural shape mirror; especially §11 (empirical-pin block format), §13 (BEHAVIOR_CONTRACT verbatim Markdown patch), §7 (differential fixture). The 08.1 admin-mux scaffold (six handlers on the same mux) is the host structure 08.2 extends with the seventh handler.
- **Sibling SPEC stub:** `docs/envoy-go/phases/08.2-graceful-drain/README.md` (becomes read-only history at THIS commit per the stub's own §1).
- **08.1 REVIEW.md:** `docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md` — the carry-forward source for §10 dispositions (N-1 inline-fix; N-2/N-3/N-5 carry-forward; N-4 inline-fix candidate).
- **Structural precedent (sub-phase SPEC shape):** `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` and `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` — header layout, §-numbering conventions, acceptance-bullet shape, empirical-pin verbatim subsections.
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine), §5.3 (Commit message format — phase-done subject), §6.2 (How to split — split discipline; ADR-0084 applies to phase 08), §6.3 (Anti-pattern — bounding scope), §7.5 (Phase-done gate — six-gate checklist; §3 specializes), §4.1 (artifact-layout invariants — ROADMAP row flip discipline), §8 (MVP trunk — phase 08 closes the trunk; 08.2's phase-done is the trunk-close commit), §9 (Feature families — what comes next after MVP-trunk closure).
- **DECISIONS.md cross-references:**
  - **Inherited (cited, not amended):** ADR-0003 (per-phase worktree convention), ADR-0004 (autonomous-brainstorming hard-gate — the empirical-pin discipline traces here), ADR-0008 (Envoy v1.37.2 pin — empirical-pin SHA anchor), ADR-0014 (`Server: envoy` header value — inherited by /drain_listeners), ADR-0018 (fuzzer 30s budget — `FuzzDrainTransitions` inherits), ADR-0040 (out-of-scope deferrals format — extends), ADR-0041 (HCM silent-ignore set — `?graceful=true` silent-ignore precedent), ADR-0045 (planner-time split discipline), ADR-0051 (h2spec pin SHA — gate (c) carry-through), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization — §13 inherits), ADR-0072 / ADR-0079 / ADR-0085 (LBP-1 generalization precedents — 08.2 is the fifth application).
  - **Partially superseded:** ADR-0015 (pre-init contract for `/ready`) — partially superseded by ADR-0097 (DRAINING extension; pre-init body verbatim preserved).
  - **Amended:** ADR-0088 (`/server_info` body shape; state-enum coverage) — amended by ADR-0098 (purely additive DRAINING enum coverage); ADR-0089 (admin-endpoint deferral list) — POST /drain_listeners line flips from "08.2" to "delivered in 08.2 per ADR-0093" via in-place edit per ADR-0089 consequence (b); ADR-0085 (LBP-1 third application) — consequence extended in-place to cover the fifth application; ADR-0090 (no-ACL admin-endpoint security posture; no method discrimination) — partially amended by ADR-0093 (mutating endpoint /drain_listeners DOES enforce method discrimination per §11.4).
  - **New (this SPEC anticipates):** ADR-0091 through ADR-0099 per §8 (or fewer per consolidation).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Server-build SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (per §11.7 verbatim re-validation; also per 08.1 SPEC §11.4). All seven §11 empirical pins reference this image SHA; §11.8 specifies the resync discipline on pin refresh.
