# Phase 02 — TCP Proxy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract); `docs/envoy-go/phases/02-tcp-proxy/SPEC.md` (authoritative scope — every PLAN decision below traces to a SPEC section); `docs/envoy-go/DECISIONS.md` (ADR-0001…0021 — especially **ADR-0003** branch convention, **ADR-0005** autonomous-execution adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0012** YAML→proto pipeline, **ADR-0013** go-control-plane proto-types-only pin, **ADR-0016** unknown-field rejection); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (existing `## Admin API — /ready` + `## Test harness host networking` subsections — phase 02 adds a new `## TCP proxy` subsection); `docs/envoy-go/phases/01-static-bootstrap-config/PLAN.md` and `PROGRESS.md` (style reference for tasks, atomic per-task commits, PROGRESS conventions).

**Goal:** Retire the phase-00/01 ad-hoc TCP pump in `cmd/envoy-go/main.go` and land the first real envoy-go dataplane — a listener manager (`internal/listener/`) + cluster manager with round-robin LB (`internal/cluster/`) + TCP proxy filter (`internal/filter/tcpproxy/`) wired through a rewritten `cmd/envoy-go/main.go` — such that fixture `0000-tcp-echo` stays green under the new dataplane and a new fixture `0001-tcp-proxy-rr` (3-endpoint STATIC cluster + round-robin) lands green, satisfying every gate in `docs/envoy-go/phases/02-tcp-proxy/SPEC.md` §3.

**Architecture:** `cmd/envoy-go/main.go` becomes a wiring shell: it loads the bootstrap (phase-01 `internal/bootstrap.Load`, unchanged), constructs `*cluster.Manager` from `static_resources.clusters[]`, starts the admin server (phase-01 `internal/admin`, unchanged), constructs `*listener.Manager` from `static_resources.listeners[]` (each listener wired to its terminal filter via an inline filter constructor registry — exactly one URL registered: `tcp_proxy.v3.TcpProxy`), starts the listener manager (which binds every listener and launches one Accept goroutine per listener), `MarkReady`s the admin, prints one `envoy-go listener <name> ready on <addr>\n` line per listener followed by a terminal `envoy-go ready\n`, and blocks on context cancellation. The TCP proxy filter parses its `typed_config` Any into `tcpproxyv3.TcpProxy`, resolves the named cluster against the manager at build time, and on each accepted downstream connection picks an endpoint via the cluster's round-robin LB (per-cluster `atomic.Uint64` counter, formula `i := counter.Add(1) - 1; endpoints[int(i) % len(endpoints)]`), dials with `net.DialTimeout` honouring `connect_timeout`, and runs the phase-00 byte pump (`netConn` wrapper + bidirectional `io.Copy` + `CloseWrite` half-close) verbatim — lifted from `cmd/envoy-go/main.go` to `internal/filter/tcpproxy/filter.go`. Phase-01's first-only extractors `bootstrap.FirstListenerSocket` and `bootstrap.FirstClusterEndpointSocket` are deleted (they have no caller after the rewire). The differential harness extends to the new sentinel format (clean break, no backward-compat) and the `FixtureDriver` interface gains `BackendCount() int`, `SubjectListenerName() string`, and a multi-port `[]int`-indexed config-templating signature so the new fixture can declare 3 backends. Fixture `0001-tcp-proxy-rr` uses STRICT_DNS on the reference side (Envoy container reaches host backends via `host.docker.internal`, ADR-0010 V4_ONLY) and STATIC on the subject side (subject is a host subprocess, dials literal 127.0.0.1 endpoints) — the same divergence pattern fixture 0000 already carries. The new `FuzzTcpProxyFilter` target satisfies SPEC §3 gate (d) at the ADR-0018 budget (30s).

**Tech Stack:**
- Go 1.23 (unchanged from phase 01).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin) — proto types only. Phase 02 imports adds:
  - `…/config/cluster/v3` (`Cluster` proto, `Cluster_DiscoveryType`, `Cluster_LbPolicy`)
  - `…/config/endpoint/v3` (`ClusterLoadAssignment`, `LbEndpoint`, `Endpoint`)
  - `…/config/listener/v3` (`Listener`, `FilterChain`, `Filter`)
  - `…/config/core/v3` (`Address`, `SocketAddress`)
  - `…/extensions/filters/network/tcp_proxy/v3` (`TcpProxy` — already blank-imported by phase-01 `internal/bootstrap`; phase-02 takes a direct typed import in `internal/filter/tcpproxy/`)
- `google.golang.org/protobuf/types/known/anypb` and `…/durationpb` (typed Any unmarshal; duration → `time.Duration` conversion).
- Stdlib `net`, `io`, `sync`, `sync/atomic`, `context`, `time`, `os/signal`, `syscall` for the listener-manager + cmd-rewire.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy v1.37.2 @ `sha256:c5e8a68e…` (ADR-0008, consumed not modified).

---

## File Structure

Net change estimate: **~1700 LoC** across the paths below. The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1). The estimate is in the same magnitude band as the threshold; the `~` qualifier on both numbers permits judgment. Three reasons to plan as one phase rather than split into 02.1 + 02.2:

1. **Atomic-claim cohesion (SPEC §1):** the phase's claim is "envoy-go runs real Envoy dataplane primitives — listener + filter + cluster + LB — end-to-end and remains byte-equivalent to upstream Envoy on a deterministic TCP workload." A split where 02.1 ships the foundation packages without an end-to-end gate weakens this claim — the foundations are tested only at unit level until 02.2's cutover lands. SPEC §3 gate (a) ("new/changed differential fixtures green") cannot be satisfied without the cutover.
2. **No clean half-fixture seam.** Splitting at the package boundary leaves 02.1 with three new packages and zero production callers. SPEC §6.3 anti-pattern (BOOTSTRAP_PROMPT) cautions against shipping incomplete stubs that differential tests can't exercise. Unit tests + the FuzzTcpProxyFilter would be the only positive evidence in 02.1.
3. **Mid-execution split valve.** BOOTSTRAP §6.1's secondary trigger — "if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity" — is preserved. Task 7 (the cutover) plans ~10 sub-steps; if reality blows past 15, the executor splits per BOOTSTRAP §6.2 with an ADR. This is a real release valve.

If at execution time the cumulative landed-LoC count exceeds 2200 by Task 6 (i.e., before the cutover), invoke `superpowers:systematic-debugging` and re-evaluate — the estimate would be materially wrong and a split may be the correct response.

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `internal/cluster/doc.go` | Create | Package doc — STATIC clusters with round-robin LB; phase-02 surface bounds; references SPEC §5.4. |
| `internal/cluster/cluster.go` | Create | `Cluster`, `Endpoint` types; `Name`, `PickEndpoint`, `ConnectTimeout` accessors; `Endpoint.Addr() string`; package-level `errNoEndpoints` sentinel. |
| `internal/cluster/loadbalancer.go` | Create | Unexported `loadBalancer` interface + `roundRobin` implementation (per-cluster `atomic.Uint64` counter; formula `i := counter.Add(1) - 1; endpoints[int(i) % N]`). |
| `internal/cluster/loadbalancer_test.go` | Create | RR distribution N=30 → 10/10/10 exact; concurrent RR N=3000 (100×30) → 1000/1000/1000 exact; zero-endpoint error path. |
| `internal/cluster/manager.go` | Create | `Manager`, `NewManager(*bootstrapv3.Bootstrap) (*Manager, error)`, `Get(name string) (*Cluster, bool)`. Build-time validation per SPEC §5.4. |
| `internal/cluster/manager_test.go` | Create | Single + multi-cluster happy; `Get` unknown name; build errors on STRICT_DNS / LOGICAL_DNS / EDS / ORIGINAL_DST; non-ROUND_ROBIN lb_policy; zero clusters; zero endpoints; non-socket_address endpoint; duplicate name. |
| `internal/filter/tcpproxy/doc.go` | Create | Package doc — references ADR-B (pump lift) and SPEC §5.5. |
| `internal/filter/tcpproxy/filter.go` | Create | `Filter`, `NewFilter(*anypb.Any, *cluster.Manager) (*Filter, error)`, `Handle(ctx, downstream net.Conn)`. Pump code verbatim from `cmd/envoy-go/main.go` (ADR-B). |
| `internal/filter/tcpproxy/filter_test.go` | Create | NewFilter happy + 4 error paths (wrong type_url, unmarshal error, missing cluster ref, weighted_clusters); Handle bidirectional echo against loopback helper; Handle dial-failure closes downstream. |
| `internal/filter/tcpproxy/fuzz_test.go` | Create | `FuzzTcpProxyFilter` with 3-entry seed corpus per SPEC §4.1; CI budget `-fuzztime=30s` per ADR-0018. |
| `internal/listener/doc.go` | Create | Package doc — phase-02 listener manager surface; references SPEC §5.2 and the inline filter registry (SPEC §5.3). |
| `internal/listener/manager.go` | Create | `Manager`, `NewManager(*bootstrapv3.Bootstrap, *cluster.Manager) (*Manager, error)`, `Start(ctx) error`, `Stop()`, `Listeners() []ListenerInfo`, `ListenerInfo{Name, Addr string}`; inline `filterRegistry`. |
| `internal/listener/manager_test.go` | Create | Single-listener + multi-listener happy; bind-failure unwind; build errors per SPEC §4.1 (≥2 chains, non-empty match, ≥2 filters, populated transport_socket, unknown filter type_url, duplicate name, zero listeners). |
| `cmd/envoy-go/main.go` | Modify | Rewrite: drop `pump`/`halfClose`/`netConn` (lifted to filter package), drop direct `net.Listen`/`Accept`/`io.Copy`, wire `cluster.NewManager` → `admin.New`/`Start` → `listener.NewManager` → `lm.Start(ctx)` → `admSrv.MarkReady` → per-listener + terminal sentinels → block on ctx. Add SIGINT handler. Drop calls to `bootstrap.FirstListenerSocket` / `FirstClusterEndpointSocket`. |
| `cmd/envoy-go/main_test.go` | Modify | Rewrite to launch with a two-listener bootstrap; parse both per-listener ready sentinels + the terminal sentinel; round-trip echo against each listener address. |
| `internal/bootstrap/bootstrap.go` | Modify | Delete `FirstListenerSocket` and `FirstClusterEndpointSocket`. `Load`, `AdminSocket`, and the blank-import of `…/tcp_proxy/v3` are unchanged. |
| `internal/bootstrap/bootstrap_test.go` | Modify | Delete `TestFirstListenerSocket_*` and `TestFirstClusterEndpointSocket_*`. Other tests unchanged. |
| `test/differential/harness.go` | Modify | Replace `readyAddr(line) string` with `readyListenerAddrs(stdout reader) (map[string]string, error)` that collects every `envoy-go listener <name> ready on <addr>` line until the terminal `envoy-go ready` line. `SubjectProxy.ListenerAddr() string` becomes `ListenerAddr(name string) string` (name-free form deleted). |
| `test/differential/fixture/fixture.go` | Modify | Add `BackendCount() int` (returns the number of host-side backends the runner allocates), `SubjectListenerName() string` (which listener the driver's `Drive` targets — used by harness to look up the subject's listener address). Change `ReferenceBootstrap()` to `ReferenceBootstrap(backendPorts []int) string` and `SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string`. Add optional `DistributionAsserter` interface (`AssertDistribution(refCounts, subjCounts []uint64) error`) the runner type-asserts. |
| `test/differential/runner_test.go` | Modify | Replace single-backend allocation with `n := d.BackendCount()` loop allocating `n` backends each with its own `atomic.Uint64` accept counter (replacing the bare `acceptEcho` helper). Pass `backendPorts []int` through to `ReferenceBootstrap` + `SubjectConfig`. After `Drive`, snapshot the counters and call `AssertDistribution` if the driver implements `DistributionAsserter`. Use `subj.ListenerAddr(d.SubjectListenerName())` instead of `subj.ListenerAddr()`. The `helpers.compareAdminResponses` admin probe path is unchanged. |
| `test/fixtures/0000-tcp-echo/driver/driver.go` | Modify | `BackendCount() int` → 1; `SubjectListenerName() string` → `"l_tcp"`; `ReferenceBootstrap` and `SubjectConfig` updated to the new signatures (template `backendPorts[0]` into the same templates, no semantic change). The `port_value: 0` placeholder is rendered out at the driver layer instead of via runner-side `strings.Replace`. |
| `test/fixtures/0001-tcp-proxy-rr/envoy.yaml` | Create | Reference bootstrap: 1 listener (`l_tcp`, port 15001 in-container), 1 STRICT_DNS cluster `c_echo` with `dns_lookup_family: V4_ONLY` (ADR-0010) and 3 lb_endpoints at `host.docker.internal` with port placeholders. |
| `test/fixtures/0001-tcp-proxy-rr/envoy-go.yaml` | Create | Subject bootstrap: 1 listener (`l_tcp`), 1 STATIC cluster `c_echo` with 3 lb_endpoints at literal 127.0.0.1 with port placeholders. |
| `test/fixtures/0001-tcp-proxy-rr/expectations.yaml` | Create | `response-body: applicable`, byte-exact; LB sequence + admin probe handled by harness defaults. |
| `test/fixtures/0001-tcp-proxy-rr/README.md` | Create | Fixture purpose (RR over 3 endpoints), STATIC-vs-STRICT_DNS divergence (same pattern as 0000), distribution assertion methodology. |
| `test/fixtures/0001-tcp-proxy-rr/driver/driver.go` | Create | `init()` registers driver as `0001-tcp-proxy-rr`; `BackendCount() = 3`; `SubjectListenerName() = "l_tcp"`; `ReferenceBootstrap` + `SubjectConfig` template the 3 backend ports into the YAMLs above; `Drive` issues 9 TCP round-trips against each address and returns concatenated bytes; implements `DistributionAsserter` (each proxy's per-backend counts must be exactly `[3, 3, 3]`). |
| `test/fixtures/0001-tcp-proxy-rr/driver/driver_test.go` | Create | Logic-only unit test for `AssertDistribution` (3/3/3 passes; 4/3/2 fails; 0/0/0 fails; runner ordering is N/A — distribution is per-proxy independent). No subprocess, no Docker. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | Add new top-level H2 section `## TCP proxy` covering: (a) byte-exact response-body equivalence on tcp_proxy-terminated TCP connections; (b) explicit non-equivalence: LB endpoint-selection sequence (each proxy is RR-correct in its own right; cross-proxy sequence equivalence is not asserted because upstream's RR is per-worker with randomised offset); (c) half-close propagation semantics preserved from phase 00. |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0022 through ADR-0027 (six ADRs — listed in `## ADRs introduced by this plan` below). |
| `docs/envoy-go/ROADMAP.md` | *Not modified by this plan* | Row 02 advances to `done` at state-machine step 6 in a later session, per ADR-0005. |
| `docs/envoy-go/STATE.md` | Modify (at exit) | Advanced to `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` at this plan-authoring session's exit commit. |
| `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md` | Create (during execution) | Append-only running log per BOOTSTRAP §5 step 3, matching phase-00/01 conventions. |

---

## ADRs introduced by this plan

Six ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that needs it (per phase-00 / phase-01 precedent). All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the current tail (ADR-0021 verified at PLAN-write time). Per SPEC §4.4, the original SPEC anticipated ADRs A–F; the assigned numbers are:

- **ADR-0022 (= SPEC §4.4 ADR-A) — Retire phase-01 first-only extractors; introduce listener and cluster managers.** Decision: `internal/bootstrap.FirstListenerSocket` and `FirstClusterEndpointSocket` are deleted. Bootstrap structural traversal moves to `internal/listener/manager.go` (`NewManager` walks `static_resources.listeners[]`) and `internal/cluster/manager.go` (`NewManager` walks `static_resources.clusters[]`). Rationale: the "exactly one X" rule was a phase-01 simplification; phase 02 supports N listeners and N clusters, and dispersing per-extractor logic across the bootstrap package would duplicate the validation each manager owns. `internal/bootstrap.AdminSocket` is unchanged (admin remains a single global entity). Lands in Task 7. No prior ADR is superseded — the first-only discipline was implicit in phase 01, never ADR'd.
- **ADR-0023 (= SPEC §4.4 ADR-B) — Lift phase-00 `netConn`/`pump`/`halfClose` trio verbatim from `cmd/envoy-go/main.go` into `internal/filter/tcpproxy/filter.go`.** Decision: the three helpers move byte-for-byte (no logic change, no rename, no signature change beyond receiver/parameter shape required to live inside the filter's `Handle` method). The `netConn` wrapper preserves the splice-avoidance rationale (Linux `splice(2)` returning 0 bytes when source has data+FIN already queued, causing silent loopback data loss — see phase-00 PLAN.md and the original comments in `cmd/envoy-go/main.go:91-96`). The verbatim lift is explicit so reviewers can `git diff` and verify no semantics change at the byte level. Lands in Task 4. No supersession.
- **ADR-0024 (= SPEC §4.4 ADR-C) — Per-cluster `atomic.Uint64` counter as the round-robin state scope.** Decision: each `*Cluster` owns its own `atomic.Uint64` counter consulted only by that cluster's RR LB. Endpoints picked by `i := counter.Add(1) - 1; endpoints[int(i) % N]`. Rationale: LB state in Envoy's data model is per-cluster (a cluster is the unit that owns its endpoint pool and its load-balancer state); a per-listener counter would fragment distribution across listeners that share a cluster (e.g., a future fixture with 2 listeners both proxying to `c_echo` would observe each listener's counter restart from 0, double-loading endpoint[0]). Per-process global counter would conflate distribution across unrelated clusters. Sequence-level equivalence to upstream is **not** promised (upstream's RR is per-worker with randomised starting offset; see ADR-E and the new BEHAVIOR_CONTRACT TCP proxy section). Lands in Task 2. No supersession.
- **ADR-0025 (= SPEC §4.4 ADR-D) — Phase-02 filter-chain subset: exactly one `filter_chain` per listener, empty `filter_chain_match`, exactly one terminal filter.** Decision: `internal/listener.NewManager` build-errors on (a) `len(listener.filter_chains) != 1`; (b) any non-zero `filter_chain_match` value; (c) `len(filter_chain.filters) != 1`; (d) `transport_socket != nil` (TLS = phase 03); (e) any filter `typed_config.type_url` not registered in the inline registry (phase 02 registers exactly one URL: `type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`). `listener_filters` is silently skipped (per SPEC §2). Rationale: the full `FilterChain` matching protocol (destination port / SNI / transport protocol / server names) and the iteration-protocol primitives (read vs write filter, continue/stop/stopBuffered) are non-trivial subsystems deferred to phase 07 (filter chain framework). Phase 02 ships the simplest correct case so the dataplane shape lands in this phase. Supersedes nothing. Lands in Task 6.
- **ADR-0026 (= SPEC §4.4 ADR-E) — Extend subject ready-sentinel format: per-listener line + terminal line; clean break from phase 00/01 single-line format.** Decision: `cmd/envoy-go/main.go` prints, after every listener has bound, one line per listener `envoy-go listener <name> ready on <host:port>\n` followed by exactly one terminal line `envoy-go ready\n`. The phase-00/01 single-line format `envoy-go ready on <host:port>\n` is **retired** — no backward-compat parser, no transitional emission, no deprecation grace period. The harness's `readyAddr(line) string` parser is replaced wholesale by `readyListenerAddrs(reader) map[string]string`. The `SubjectProxy.ListenerAddr() string` accessor (no-arg) is replaced by `ListenerAddr(name string) string`. Rationale: per SPEC §10 #5, the harness is the only known consumer; retaining a transitional dual-format emitter would couple `cmd/envoy-go/main.go` to its own retired contract for no benefit. Per `BOOTSTRAP_PROMPT.md` D-3.4, this is the kind of cross-session decision that must live on disk — hence the ADR. Supersedes the phase-00 sentinel contract (which was codified in `cmd/envoy-go/main.go:79` comments and the harness's `readyAddr` function but never formally ADR'd). Lands in Task 7.
- **ADR-0027 (= SPEC §4.4 ADR-F) — Fixture `0001-tcp-proxy-rr` reference uses STRICT_DNS, subject uses STATIC.** Decision: the reference Envoy bootstrap (`test/fixtures/0001-tcp-proxy-rr/envoy.yaml`) declares its `c_echo` cluster as `STRICT_DNS` with `dns_lookup_family: V4_ONLY` (ADR-0010) and three `lb_endpoints` whose addresses are `host.docker.internal` with three distinct `port_value`s. The subject's bootstrap (`envoy-go.yaml`) declares `c_echo` as `STATIC` with three `lb_endpoints` at literal `127.0.0.1` with three distinct `port_value`s. Same divergence pattern fixture 0000 already carries (ADR-0010 documents the why); fixture 0001's only novelty is "the divergence holds when the cluster has 3 endpoints instead of 1." Rationale: the reference Envoy runs inside a Docker container and reaches host-side test backends via `host.docker.internal` (ADR-0010); the subject runs as a host subprocess and dials literal IPs. STATIC is not a viable choice for the reference because the container-internal DNS resolution behaviour is not under test, and STRICT_DNS is the cluster type that actually consumes `host.docker.internal`. Documented for traceability — the differential gate is on response-body byte equivalence and (per the new BEHAVIOR_CONTRACT TCP proxy subsection) explicitly NOT on LB sequence. Lands in Task 9.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0028+) in the same commit as the code it decides for. If such a decision would expand phase-02 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6.

---

## Settled SPEC §10 deferred decisions

SPEC §10 leaves nine implementation-detail choices to the planner. This PLAN settles them as follows (each settled here so the executor does not re-litigate; only ADR-worthy decisions become ADRs):

1. **Subpackage layout for `internal/cluster/`** — flat. Files live as siblings: `cluster.go`, `loadbalancer.go`, `manager.go` (plus `*_test.go`). No `internal/cluster/lb` subpackage. Rationale: phase 02 has one LB policy; subpackage is premature decomposition. Future phases that introduce an LB family can refactor when there are 2+ policies and the internal interface stabilises. Not ADR'd (planner-decision per SPEC §10).
2. **`connect_timeout` default** — 5 seconds when unset. Source: Envoy v1.37.2 documentation default for cluster `connect_timeout` is 5s. Codified as a package-level constant `internal/cluster.defaultConnectTimeout = 5 * time.Second`. Not ADR'd (matches upstream — the only reason to ADR would be divergence).
3. **`listener.Manager.Stop` implementation** — implemented fully. `Stop()` closes every bound listener socket; the Accept-loop goroutines exit on the resulting accept error. In-flight `filter.Handle` goroutines are NOT waited on (drain semantics = phase 08). Stop is idempotent (multiple calls close already-closed sockets without panicking; `net.Listener.Close()` returns `ErrClosed` on a closed listener, which Stop discards). `manager_test.go` exercises Stop in the multi-listener happy path test (start, sample addresses, Stop, assert `Accept()` returns an error on a fresh dial attempt). Not ADR'd (planner-decision).
4. **Fuzz seed corpus size** — 3 entries per SPEC §4.1. The corpus, written in `internal/filter/tcpproxy/fuzz_test.go`'s `f.Add(…)` calls, covers: (a) a well-formed `TcpProxy` Any with `cluster: "c_echo"`; (b) an Any with the wrong `type_url` (e.g., `type.googleapis.com/google.protobuf.StringValue`); (c) malformed proto bytes (random non-proto bytes wrapped in an Any). Rationale: the three entries seed every distinct error path in `NewFilter` (`type_url` mismatch, unmarshal failure, missing-cluster-after-unmarshal). Adding more is welcome but optional. Not ADR'd.
5. **Ready-sentinel backward-compat** — clean break per SPEC §10 #5 recommendation. ADR-0026 above codifies. The harness, the only known consumer, is updated atomically in Task 7's cutover commit. No backward-compat emission, no transitional parser.
6. **Fixture id `0001-tcp-proxy-rr`** — confirmed as the next available id after `0000-tcp-echo`. No conflict.
7. **`ctx` wiring across Accept loops** — shared `ctx` per SPEC §10 #7 recommendation. `Manager.Start(ctx context.Context)` captures `ctx` once; every Accept-loop goroutine closes over it; every accepted connection is dispatched via `go filter.Handle(ctx, conn)`. Rationale: per-listener child contexts add no observable behaviour at phase 02 (no per-listener cancellation triggers exist; SIGINT cancels the parent ctx and every loop drops out). Per-listener children would be premature plumbing for hot-restart / per-listener drain (phase 08+). Not ADR'd.
8. **`stat_prefix` storage on the `Filter` struct** — stored per SPEC §10 #8 recommendation. `Filter` carries an unexported `statPrefix string` field set at `NewFilter` time. The field is unread at phase 02 (no stats subsystem yet) but its presence makes the phase-06 stats wiring a one-line change. Costs one `string` per filter instance (typically <16 bytes); well below noise. Not ADR'd.
9. **ADR numbering** — settled above. Phase-02 ADRs land as ADR-0022 through ADR-0027 mapping 1:1 onto SPEC §4.4 ADR-A through ADR-F. Verified at PLAN-write time (highest landed ADR = ADR-0021).

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/02-tcp-proxy-plan` (this plan's authoring branch). Recommended: `.worktrees/phase-02-tcp-proxy-impl` on branch `phase/02-tcp-proxy-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for Task 10's full differential gate (`go test ./test/differential/...`). Tasks 2–9's `go test ./...` runs at unit level + the `cmd/envoy-go/main_test.go` in-process integration test, both Docker-free.
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is already the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` if missing.
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-01 gate (e) still holds). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. The `github.com/envoyproxy/go-control-plane/envoy` direct require in `go.mod` resolves to `v1.32.4` (ADR-0013). Verify with `go list -m github.com/envoyproxy/go-control-plane/envoy`. If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 02 must not silently re-pin.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

```bash
git rev-parse --abbrev-ref HEAD                              # expect: phase/02-tcp-proxy-impl (or equivalent impl branch)
git log -1 --format=%H                                       # expect: same SHA as docs/envoy-go/STATE.md last-commit field
docker version                                               # expect: client + server reported (no "Cannot connect to the Docker daemon")
go version                                                   # expect: go1.23+ (toolchain may be 1.26.x)
golangci-lint version                                        # expect: golangci-lint has version 1.64.8
go test ./...                                                # expect: every package PASS
go list -m github.com/envoyproxy/go-control-plane/envoy      # expect: github.com/envoyproxy/go-control-plane/envoy v1.32.4
```

If any line fails, stop and follow the precondition's "if fails" guidance (typically: invoke `superpowers:systematic-debugging` with the specific symptom).

- [ ] **Step 2: Create `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`**

```markdown
# Phase 02 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions".
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ docker version
<verbatim — first line of client + server sections is sufficient>
$ go version
<verbatim>
$ golangci-lint version
<verbatim>
$ go test ./...
<verbatim — last 30 lines>
$ go list -m github.com/envoyproxy/go-control-plane/envoy
<verbatim>
```
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: PROGRESS.md preamble + precondition verification"
```

---

## Task 2: `internal/cluster` — Cluster, Endpoint, round-robin LB + tests

**Files:**
- Create: `internal/cluster/doc.go`
- Create: `internal/cluster/cluster.go`
- Create: `internal/cluster/loadbalancer.go`
- Create: `internal/cluster/loadbalancer_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0024)
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md` (append Task 2 entry)

This task lands the cluster's leaf types and the round-robin LB. The `Manager` is Task 3. Following TDD: tests first.

- [ ] **Step 1: Write `internal/cluster/loadbalancer_test.go`**

```go
package cluster

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRoundRobin_DistributionExact(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
			{Host: "10.0.0.3", Port: 1003},
		},
	}
	counts := map[string]int{}
	const N = 30
	for i := 0; i < N; i++ {
		ep, err := rr.Pick()
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		counts[ep.Addr()]++
	}
	for _, ep := range rr.endpoints {
		if got := counts[ep.Addr()]; got != N/3 {
			t.Errorf("endpoint %s: got %d picks, want %d", ep.Addr(), got, N/3)
		}
	}
}

func TestRoundRobin_FirstPickIsEndpoint0(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
		},
	}
	ep, err := rr.Pick()
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if ep.Host != "10.0.0.1" {
		t.Errorf("first pick: got %s, want 10.0.0.1 (sequence-starts-at-0 invariant)", ep.Host)
	}
}

func TestRoundRobin_ConcurrentDistributionExact(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
			{Host: "10.0.0.3", Port: 1003},
		},
	}
	const goroutines = 100
	const perGoroutine = 30
	const total = goroutines * perGoroutine // 3000, divisible by 3
	var counts [3]atomic.Uint64
	addrToIdx := map[string]int{
		rr.endpoints[0].Addr(): 0,
		rr.endpoints[1].Addr(): 1,
		rr.endpoints[2].Addr(): 2,
	}
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ep, err := rr.Pick()
				if err != nil {
					t.Errorf("pick: %v", err)
					return
				}
				counts[addrToIdx[ep.Addr()]].Add(1)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < 3; i++ {
		if got := counts[i].Load(); got != total/3 {
			t.Errorf("endpoint[%d]: got %d picks, want %d (atomic.Add(1) gives unique i; mod 3 balances exactly when 3 | total)", i, got, total/3)
		}
	}
}

func TestRoundRobin_ZeroEndpoints(t *testing.T) {
	rr := &roundRobin{endpoints: nil}
	_, err := rr.Pick()
	if err == nil {
		t.Fatal("expected error on zero endpoints")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cluster/ -run TestRoundRobin -v`
Expected: FAIL with build error (`undefined: roundRobin`, `undefined: Endpoint`).

- [ ] **Step 3: Write `internal/cluster/doc.go`**

```go
// Package cluster materialises one cluster per static_resources.clusters[]
// entry of an Envoy v3 Bootstrap proto, exposes them by name, and gives each
// cluster a round-robin load balancer over its endpoints. Phase 02 supports
// only STATIC clusters with ROUND_ROBIN policy; see SPEC §5.4.
package cluster
```

- [ ] **Step 4: Write `internal/cluster/cluster.go`**

```go
package cluster

import (
	"errors"
	"fmt"
	"time"
)

// defaultConnectTimeout is used when a cluster's connect_timeout is unset.
// Matches Envoy v1.37.2's documented default (SPEC §10 #2 settled).
const defaultConnectTimeout = 5 * time.Second

// errNoEndpoints is returned by PickEndpoint when the cluster has no endpoints.
// Build-time validation in NewManager prevents this in normal operation; the
// runtime check exists for defence-in-depth.
var errNoEndpoints = errors.New("cluster: no endpoints")

// Endpoint is a single upstream socket destination.
type Endpoint struct {
	Host string
	Port uint32
}

// Addr returns the dial-string form "host:port".
func (e Endpoint) Addr() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// Cluster is a named pool of endpoints with a load-balancing policy. Phase 02
// supports only round-robin; future phases may grow the LB family.
type Cluster struct {
	name           string
	endpoints      []Endpoint
	connectTimeout time.Duration
	lb             loadBalancer
}

// Name returns the cluster's name.
func (c *Cluster) Name() string { return c.name }

// PickEndpoint selects the next upstream endpoint per the cluster's LB policy.
// Safe for concurrent use.
func (c *Cluster) PickEndpoint() (Endpoint, error) {
	return c.lb.Pick()
}

// ConnectTimeout returns the cluster's TCP connect timeout (default 5s if the
// bootstrap left connect_timeout unset).
func (c *Cluster) ConnectTimeout() time.Duration {
	return c.connectTimeout
}
```

- [ ] **Step 5: Write `internal/cluster/loadbalancer.go`**

```go
package cluster

import "sync/atomic"

// loadBalancer is the unexported per-cluster LB interface. Phase 02 has one
// implementation: roundRobin. Future phases that introduce LEAST_REQUEST,
// RANDOM, RING_HASH, MAGLEV, etc. add new types here.
type loadBalancer interface {
	Pick() (Endpoint, error)
}

// roundRobin is a per-cluster round-robin LB. ADR-0024 codifies the per-cluster
// counter scope decision. The formula i := counter.Add(1) - 1 then mod-index
// makes the first pick endpoints[0]; this property is asserted by unit tests
// and is internal correctness, not a differential equivalence claim (upstream's
// RR is per-worker with randomised starting offset — see BEHAVIOR_CONTRACT
// "## TCP proxy" subsection added by Task 8).
type roundRobin struct {
	endpoints []Endpoint
	counter   atomic.Uint64
}

func (rr *roundRobin) Pick() (Endpoint, error) {
	if len(rr.endpoints) == 0 {
		return Endpoint{}, errNoEndpoints
	}
	i := rr.counter.Add(1) - 1
	return rr.endpoints[int(i)%len(rr.endpoints)], nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/cluster/ -run TestRoundRobin -v`
Expected: PASS, four tests.

- [ ] **Step 7: Run lint + vet on the new package**

Run: `go vet ./internal/cluster/ && golangci-lint run ./internal/cluster/`
Expected: PASS. Any finding is a bug to fix now, not a TODO.

- [ ] **Step 8: Append ADR-0024 to `docs/envoy-go/DECISIONS.md`**

Append at the tail of the file, matching the existing ADR format. Use the body sketched in the `## ADRs introduced by this plan` section above for ADR-0024 — full Context / Decision / Rationale / Consequences. Tag `**Status:** Accepted`, `**Date:** <today>`, `**Doctrine:** D-3.5`.

- [ ] **Step 9: Append Task 2 entry to PROGRESS.md and commit**

PROGRESS entry follows the convention from PROGRESS.md preamble (header `## Task 2 — internal/cluster: Cluster + Endpoint + round-robin`, `**Commits:**` field, one-paragraph notes, verbatim outputs from `go test`, `go vet`, `golangci-lint run`).

```bash
git add internal/cluster/doc.go internal/cluster/cluster.go internal/cluster/loadbalancer.go internal/cluster/loadbalancer_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: internal/cluster — Cluster + Endpoint + round-robin LB [ADR-0024]"
```

---

## Task 3: `internal/cluster.Manager` — build-time materialisation + tests

**Files:**
- Create: `internal/cluster/manager.go`
- Create: `internal/cluster/manager_test.go`
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

- [ ] **Step 1: Write `internal/cluster/manager_test.go`**

Cover every build-time error path per SPEC §5.4 build-time-behaviour. Use small inline `*bootstrapv3.Bootstrap` builders constructed via the proto types directly (no YAML round-trip — tests cover the manager's logic, not the loader). Imports include `clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"`, `endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"`, `corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"`, `bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"`, `"google.golang.org/protobuf/types/known/durationpb"`.

Test cases (each its own `func TestManager_*`):
- `HappyPath_Single` — one STATIC cluster with one endpoint; `Get("c_echo")` returns it; `cluster.Name() == "c_echo"`; `cluster.ConnectTimeout() == 5*time.Second` (default — bootstrap leaves connect_timeout unset); `cluster.PickEndpoint()` returns the endpoint.
- `HappyPath_Multi` — two clusters, each multiple endpoints; `Get` works for both; absent name returns `(_, false)`; `cluster.ConnectTimeout()` reflects the explicit `1s` value when set via `durationpb.New(time.Second)`.
- `Error_ZeroClusters` — empty `clusters`; expect error containing `"cluster: zero clusters"` substring.
- `Error_DuplicateName` — two clusters both named `"c_echo"`; expect error containing `"duplicate cluster"`.
- `Error_StrictDNS` — cluster.type = `STRICT_DNS`; expect error mentioning `STRICT_DNS` and "phase 02 supports only STATIC".
- `Error_LogicalDNS` — same shape, type = `LOGICAL_DNS`.
- `Error_EDS` — same, type = `EDS`.
- `Error_OriginalDST` — same, type = `ORIGINAL_DST`.
- `Error_NonRoundRobinLB` — STATIC cluster but `lb_policy = LEAST_REQUEST`; expect error mentioning `ROUND_ROBIN`.
- `Error_ZeroEndpoints` — STATIC cluster with `load_assignment.endpoints[0].lb_endpoints` empty across all locality groups; expect error containing `"zero endpoints"`.
- `Error_NonSocketAddressEndpoint` — STATIC cluster with `endpoint.address.pipe` set instead of `socket_address`; expect error mentioning `socket_address`.

Helper builder pattern (one local helper):
```go
func mkBootstrap(clusters ...*clusterv3.Cluster) *bootstrapv3.Bootstrap {
    return &bootstrapv3.Bootstrap{
        StaticResources: &bootstrapv3.Bootstrap_StaticResources{Clusters: clusters},
    }
}
func mkStaticCluster(name string, endpoints ...*endpointv3.LbEndpoint) *clusterv3.Cluster {
    return &clusterv3.Cluster{
        Name:                 name,
        ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
        LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
        LoadAssignment: &endpointv3.ClusterLoadAssignment{
            ClusterName: name,
            Endpoints:   []*endpointv3.LocalityLbEndpoints{{LbEndpoints: endpoints}},
        },
    }
}
func mkLbEndpoint(addr string, port uint32) *endpointv3.LbEndpoint {
    return &endpointv3.LbEndpoint{
        HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
            Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
                SocketAddress: &corev3.SocketAddress{Address: addr, PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port}},
            }},
        }},
    }
}
```

(Reference: `SocketAddress.PortSpecifier` is a `oneof`; the concrete type used here is `*corev3.SocketAddress_PortValue`. Field names match the v1.32.4 proto generation; verify via `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/config/core/v3/address.pb.go` if a future re-pin changes them.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cluster/ -run TestManager_ -v`
Expected: FAIL with build error (`undefined: NewManager`, `undefined: Manager`).

- [ ] **Step 3: Write `internal/cluster/manager.go`**

```go
package cluster

import (
	"fmt"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

// Manager owns every Cluster materialised from static_resources.clusters[].
// Get is the dataplane-side lookup the TCP proxy filter uses to resolve a
// cluster name at filter-construction time.
type Manager struct {
	clusters map[string]*Cluster
}

// NewManager walks bs.GetStaticResources().GetClusters() and materialises one
// Cluster per entry. Errors are returned at the first violation; subsequent
// clusters are not validated. Every error begins with "cluster: ".
//
// Phase-02 surface (SPEC §2 + §5.4):
//   - cluster.type must be STATIC. STRICT_DNS, LOGICAL_DNS, EDS, ORIGINAL_DST
//     all error explicitly.
//   - cluster.lb_policy must be unset (proto default ROUND_ROBIN) or explicitly
//     ROUND_ROBIN. Anything else errors.
//   - load_assignment.endpoints[*].lb_endpoints[*] must collectively contain
//     ≥1 endpoint, each with endpoint.address.socket_address (no pipe, no
//     envoy_internal_address). Total endpoint count across all locality
//     groups must be ≥1.
//   - cluster.connect_timeout, when unset, defaults to 5s (defaultConnectTimeout).
func NewManager(bs *bootstrapv3.Bootstrap) (*Manager, error) {
	cs := bs.GetStaticResources().GetClusters()
	if len(cs) == 0 {
		return nil, fmt.Errorf("cluster: zero clusters in bootstrap")
	}
	m := &Manager{clusters: make(map[string]*Cluster, len(cs))}
	for i, c := range cs {
		built, err := buildCluster(c, i)
		if err != nil {
			return nil, err
		}
		if _, dup := m.clusters[built.name]; dup {
			return nil, fmt.Errorf("cluster: duplicate cluster name %q", built.name)
		}
		m.clusters[built.name] = built
	}
	return m, nil
}

// Get looks up a cluster by name. Returns (nil, false) if not found.
func (m *Manager) Get(name string) (*Cluster, bool) {
	c, ok := m.clusters[name]
	return c, ok
}

func buildCluster(c *clusterv3.Cluster, idx int) (*Cluster, error) {
	name := c.GetName()
	if name == "" {
		return nil, fmt.Errorf("cluster: clusters[%d]: missing name", idx)
	}
	t, ok := c.GetClusterDiscoveryType().(*clusterv3.Cluster_Type)
	if !ok {
		return nil, fmt.Errorf("cluster: %q: cluster_discovery_type must be Type, got %T (phase 02 supports only STATIC)", name, c.GetClusterDiscoveryType())
	}
	if t.Type != clusterv3.Cluster_STATIC {
		return nil, fmt.Errorf("cluster: %q: phase 02 supports only STATIC clusters; got %s", name, t.Type)
	}
	if c.GetLbPolicy() != clusterv3.Cluster_ROUND_ROBIN {
		return nil, fmt.Errorf("cluster: %q: phase 02 supports only ROUND_ROBIN lb_policy; got %s", name, c.GetLbPolicy())
	}
	la := c.GetLoadAssignment()
	if la == nil {
		return nil, fmt.Errorf("cluster: %q: missing load_assignment", name)
	}
	endpoints, err := extractEndpoints(la, name)
	if err != nil {
		return nil, err
	}
	timeout := defaultConnectTimeout
	if c.GetConnectTimeout() != nil {
		timeout = c.GetConnectTimeout().AsDuration()
	}
	return &Cluster{
		name:           name,
		endpoints:      endpoints,
		connectTimeout: timeout,
		lb:             &roundRobin{endpoints: endpoints},
	}, nil
}

func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error) {
	var out []Endpoint
	for gi, group := range la.GetEndpoints() {
		for ei, lbe := range group.GetLbEndpoints() {
			ep := lbe.GetEndpoint()
			if ep == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint is nil", clusterName, gi, ei)
			}
			addr := ep.GetAddress()
			if addr == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint.address is nil", clusterName, gi, ei)
			}
			sa := addr.GetSocketAddress()
			if sa == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d]: only socket_address endpoints supported in phase 02", clusterName, gi, ei)
			}
			out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue()})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cluster: %q: zero endpoints across all locality groups", clusterName)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cluster/ -v`
Expected: PASS, every test (TestRoundRobin_* from Task 2 plus TestManager_* from this task).

- [ ] **Step 5: Run lint + vet on the package**

Run: `go vet ./internal/cluster/ && golangci-lint run ./internal/cluster/`
Expected: PASS.

- [ ] **Step 6: Append Task 3 entry to PROGRESS.md and commit**

```bash
git add internal/cluster/manager.go internal/cluster/manager_test.go docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: internal/cluster.Manager — build-time materialisation"
```

---

## Task 4: `internal/filter/tcpproxy` — Filter, NewFilter, Handle (pump verbatim from phase 00)

**Files:**
- Create: `internal/filter/tcpproxy/doc.go`
- Create: `internal/filter/tcpproxy/filter.go`
- Create: `internal/filter/tcpproxy/filter_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0023)
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

This task lifts the phase-00 pump verbatim from `cmd/envoy-go/main.go:91-119` into the new filter package. The lift is byte-level identical — no rename, no comment edit, no signature change. ADR-0023 records the lift so reviewers can `git diff` the moved code.

- [ ] **Step 1: Write `internal/filter/tcpproxy/filter_test.go`**

```go
package tcpproxy

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

const tcpProxyTypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// mkClusterMgr builds a cluster manager from a bootstrap with one STATIC
// cluster pointing at a single endpoint. Tests use this for happy-path setup.
func mkClusterMgr(t *testing.T, name, host string, port uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address: host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

func mkAny(t *testing.T, msg interface{ Reset(); String() string; ProtoReflect() interface{} }) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg.(interface{ Reset(); String() string; ProtoReflect() interface{} }).(any).(interface{ ProtoMessage() }).(interface{ Reset() }).(interface{ String() string }).(any).(interface{ ProtoReflect() interface{} }).(any).(interface{}))
	_ = a
	_ = err
	t.Fatal("see Step 3 for the corrected helper")
	return nil
}
```

The `mkAny` helper above is intentionally broken in the test stub — fix it to the canonical form during Step 3. Canonical form:
```go
func mkAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}
```
(import `"google.golang.org/protobuf/proto"`.)

The full test file then has these `func Test*`:

- `TestNewFilter_Happy` — `mkAny(t, &tcpproxyv3.TcpProxy{StatPrefix: "ingress_tcp", ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"}})`; `NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))` returns non-nil filter, no error. Verify the filter retained `statPrefix` via an unexported test-only accessor (or test through `Handle` byte-throughput; the `statPrefix` field is unread at phase 02, see SPEC §10 #8 — keep the test minimal).
- `TestNewFilter_WrongTypeURL` — call `NewFilter(&anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.StringValue", Value: nil}, mkClusterMgr(...))`. Expect error containing `"wrong type_url"`.
- `TestNewFilter_UnmarshalError` — `&anypb.Any{TypeUrl: tcpProxyTypeURL, Value: []byte{0xff, 0xff, 0xff, 0xff, 0xff}}`. Expect error containing `"unmarshal"`.
- `TestNewFilter_MissingCluster` — `mkAny` with `ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_does_not_exist"}`; `NewFilter` against a manager that has only `c_echo`. Expect error containing `"cluster"` and `"not found"`.
- `TestNewFilter_WeightedClustersUnsupported` — `mkAny` with `ClusterSpecifier: &tcpproxyv3.TcpProxy_WeightedClusters{...}`. Expect error containing `"weighted_clusters"`.
- `TestHandle_BidirectionalEcho` — start an in-process loopback echo backend (small inline helper); build a filter pointed at that backend; create a `net.Pipe()`-style connection pair OR a real loopback `net.Dial` to a local listener that hands its accepted conn to `Handle`; write payload, half-close write, read response, assert byte-exact echo round-trip.
- `TestHandle_DialFailure_ClosesDownstream` — point the filter at a closed port (`net.Listen` then immediately `Close`); call `Handle` with a downstream pipe; assert downstream is closed (read returns EOF or similar) within a short timeout.

Skeleton of `TestHandle_BidirectionalEcho`:
```go
func TestHandle_BidirectionalEcho(t *testing.T) {
	// Backend echo on a random port.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEchoForTest(backend)
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"},
	})
	f, err := NewFilter(any, cm)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	// Acceptor for the simulated downstream side.
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("front listen: %v", err)
	}
	defer func() { _ = front.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		conn, err := front.Accept()
		if err != nil {
			return
		}
		f.Handle(ctx, conn)
	}()

	cli, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatalf("dial front: %v", err)
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = cli.(*net.TCPConn).CloseWrite()

	var got []byte
	buf := make([]byte, 4096)
	_ = cli.SetReadDeadline(time.Now().Add(time.Second))
	for {
		n, err := cli.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	if string(got) != "hello\n" {
		t.Errorf("got %q, want %q", got, "hello\n")
	}
}

func acceptEchoForTest(ln net.Listener) {
	type wrap struct{ net.Conn }
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			_, _ = io.Copy(wrap{c}, wrap{c})
		}(c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/filter/tcpproxy/ -v`
Expected: FAIL with build error (`undefined: NewFilter`, `undefined: Filter`).

- [ ] **Step 3: Write `internal/filter/tcpproxy/doc.go`**

```go
// Package tcpproxy implements the envoy.filters.network.tcp_proxy filter:
// parses an envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy proto from
// the listener's filter typed_config Any, resolves its cluster reference at
// filter-construction time against the cluster manager, and on each accepted
// downstream connection picks an endpoint via the cluster's LB, dials it
// (honouring the cluster's connect_timeout), and pumps bytes bidirectionally
// with half-close propagation.
//
// The byte pump (netConn wrapper + bidirectional io.Copy + halfClose helper)
// is lifted verbatim from cmd/envoy-go/main.go's phase-00 implementation per
// ADR-0023; the splice(2) avoidance rationale is preserved in the netConn
// type doc-comment. See SPEC §5.5.
package tcpproxy
```

- [ ] **Step 4: Write `internal/filter/tcpproxy/filter.go`**

```go
package tcpproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// TypeURL is the proto type URL phase 02 registers in the listener's inline
// filter constructor registry. Exported so the listener package can reference
// it without re-stringifying.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// Filter is one TCP proxy filter instance, bound at construction time to the
// resolved cluster it dispatches to. Immutable after NewFilter returns.
type Filter struct {
	cluster    *cluster.Cluster
	statPrefix string // unread at phase 02; SPEC §10 #8 settled — stored for forward-compat
}

// NewFilter parses tc as a TcpProxy proto and resolves its cluster reference
// against cm. Returns an error if (a) tc.TypeUrl is not the TcpProxy URL, (b)
// the proto bytes do not unmarshal, (c) the cluster reference is missing or
// names a cluster cm does not know, or (d) the proto uses weighted_clusters
// (phase 02 does not implement weighted dispatch).
//
// Every error begins with "tcpproxy: ".
func NewFilter(tc *anypb.Any, cm *cluster.Manager) (*Filter, error) {
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("tcpproxy: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &tcpproxyv3.TcpProxy{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("tcpproxy: unmarshal: %w", err)
	}
	switch cs := msg.GetClusterSpecifier().(type) {
	case *tcpproxyv3.TcpProxy_Cluster:
		name := cs.Cluster
		if name == "" {
			return nil, fmt.Errorf("tcpproxy: cluster reference is empty")
		}
		c, ok := cm.Get(name)
		if !ok {
			return nil, fmt.Errorf("tcpproxy: cluster %q not found", name)
		}
		return &Filter{cluster: c, statPrefix: msg.GetStatPrefix()}, nil
	case *tcpproxyv3.TcpProxy_WeightedClusters:
		return nil, fmt.Errorf("tcpproxy: weighted_clusters is not supported in phase 02")
	default:
		return nil, fmt.Errorf("tcpproxy: cluster_specifier is missing or of unsupported type %T", cs)
	}
}

// Handle pumps bytes bidirectionally between downstream and a freshly-dialled
// upstream picked via the cluster's LB. Closes downstream and upstream when
// the pump completes (or on dial failure). Logs but does not return errors.
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	ep, err := f.cluster.PickEndpoint()
	if err != nil {
		log.Printf("tcpproxy: pick endpoint: %v", err)
		return
	}
	upstream, err := net.DialTimeout("tcp", ep.Addr(), f.cluster.ConnectTimeout())
	if err != nil {
		log.Printf("tcpproxy: dial %s: %v", ep.Addr(), err)
		return
	}
	defer func() { _ = upstream.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{downstream}); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{downstream}, netConn{upstream}); halfClose(downstream) }()
	wg.Wait()
}

// netConn wraps net.Conn and hides the *net.TCPConn type, preventing
// io.Copy from using the Linux splice(2) syscall optimisation. splice can
// return 0 bytes when the source socket has data+FIN already queued, causing
// silent data loss on loopback. Using a plain Read/Write loop via a 32 KiB
// heap buffer is fast enough for the phase-01 test workload. (Lifted verbatim
// from phase 00 cmd/envoy-go/main.go per ADR-0023.)
type netConn struct{ net.Conn }

func halfClose(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/filter/tcpproxy/ -v`
Expected: PASS, all `TestNewFilter_*` and `TestHandle_*` cases.

- [ ] **Step 6: Run lint + vet**

Run: `go vet ./internal/filter/tcpproxy/ && golangci-lint run ./internal/filter/tcpproxy/`
Expected: PASS.

- [ ] **Step 7: Append ADR-0023 to `docs/envoy-go/DECISIONS.md`**

Per the body sketched in `## ADRs introduced by this plan`. The ADR's Consequences section explicitly notes the verbatim-lift property: a reviewer can `git show <Task-4-commit>:internal/filter/tcpproxy/filter.go` and `git show HEAD~N:cmd/envoy-go/main.go` (where N is the commit count back to phase 01) and inspect that the `netConn`, `pump`-body, and `halfClose` token sequences are identical (modulo `pump`'s function-extraction into the body of `Handle`). The lift is the whole rationale of the ADR.

- [ ] **Step 8: Append Task 4 entry to PROGRESS.md and commit**

```bash
git add internal/filter/tcpproxy/doc.go internal/filter/tcpproxy/filter.go internal/filter/tcpproxy/filter_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: internal/filter/tcpproxy — Filter, NewFilter, Handle [ADR-0023]"
```

---

## Task 5: `internal/filter/tcpproxy.FuzzTcpProxyFilter` — fuzz target (gate (d))

**Files:**
- Create: `internal/filter/tcpproxy/fuzz_test.go`
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

The fuzz target satisfies SPEC §3 gate (d) for phase 02. CI budget inherited from ADR-0018 (30 seconds via `-fuzztime=30s`). No new ADR.

- [ ] **Step 1: Write `internal/filter/tcpproxy/fuzz_test.go`**

```go
package tcpproxy

import (
	"strings"
	"testing"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzTcpProxyFilter feeds mutated bytes into NewFilter via an Any wrapper.
// The contract per SPEC §3 gate (d): no panic; every error begins with
// "tcpproxy: " (matches the package's error-prefix discipline).
//
// Note on Go fuzz framework: f.Add(...) seed types and counts must match the
// f.Fuzz(func(t, ...) {...}) parameter list. Here both use (string, []byte) —
// type_url and the inner Any.Value bytes — and the fuzz function reconstructs
// the *anypb.Any from those two scalars on each invocation.
//
// Seed corpus (3 entries per SPEC §4.1):
//   1. Well-formed TcpProxy referencing an extant cluster (canonical happy).
//   2. Wrong type_url (StringValue instead of TcpProxy).
//   3. Malformed proto bytes (random non-proto bytes wrapped in an Any with
//      the correct type_url).
func FuzzTcpProxyFilter(f *testing.F) {
	// Seed 1: well-formed.
	good := &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"},
	}
	goodBytes, err := proto.Marshal(good)
	if err != nil {
		f.Fatalf("seed marshal: %v", err)
	}
	f.Add(TypeURL, goodBytes)

	// Seed 2: wrong type_url.
	f.Add("type.googleapis.com/google.protobuf.StringValue", goodBytes)

	// Seed 3: malformed bytes (non-proto).
	f.Add(TypeURL, []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8})

	cm := mkClusterMgr(f.(testingHelperT), "c_echo", "127.0.0.1", 1) // port 1 — never dialled in fuzz; only NewFilter is exercised.

	f.Fuzz(func(t *testing.T, typeURL string, body []byte) {
		any := &anypb.Any{TypeUrl: typeURL, Value: body}
		_, err := NewFilter(any, cm)
		if err == nil {
			return // no error is also acceptable (the input parsed)
		}
		if !strings.HasPrefix(err.Error(), "tcpproxy: ") {
			t.Fatalf("error does not begin with %q: %v", "tcpproxy: ", err)
		}
	})
}

// testingHelperT shims testing.F into the *testing.T mkClusterMgr expects.
// Both *testing.T and *testing.F implement Helper() and Fatalf, but mkClusterMgr's
// signature names *testing.T concretely. Adjust here or change mkClusterMgr to
// accept testing.TB.
type testingHelperT = testing.TB // package-local alias; mkClusterMgr's parameter type may need to widen to testing.TB during this task.
```

If the alias breaks `mkClusterMgr` callers in `filter_test.go`, widen `mkClusterMgr`'s parameter from `*testing.T` to `testing.TB` and use `t.Helper()` (which both `*T` and `*F` implement). This is a minor follow-up touch on Task 4's test file, in the same Task-5 commit.

- [ ] **Step 2: Run the fuzz target in deterministic seed-only mode**

Run: `go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=2s`
Expected: PASS — runs the seed corpus + a few fuzz mutations within 2s, no failures, no panics.

If the run reports a panic or a discovered failing input, capture the failing input (Go writes it to `internal/filter/tcpproxy/testdata/fuzz/FuzzTcpProxyFilter/<id>`), reduce to a unit test in `filter_test.go` if appropriate, fix the bug, re-run.

- [ ] **Step 3: Run the fuzz target at the CI budget**

Run: `go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=30s`
Expected: PASS, no failures discovered. Quote the verbatim output (mutation count, time) into the PROGRESS entry.

- [ ] **Step 4: Run the broader test suite to confirm no regression**

Run: `go test ./internal/filter/tcpproxy/ -v`
Expected: PASS — every prior `Test*` continues to pass (no `Test*` runs the fuzz target by default).

- [ ] **Step 5: Run lint + vet**

Run: `go vet ./internal/filter/tcpproxy/ && golangci-lint run ./internal/filter/tcpproxy/`
Expected: PASS.

- [ ] **Step 6: Append Task 5 entry to PROGRESS.md and commit**

```bash
git add internal/filter/tcpproxy/fuzz_test.go internal/filter/tcpproxy/filter_test.go docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: internal/filter/tcpproxy — FuzzTcpProxyFilter (gate d, ADR-0018 budget)"
```

---

## Task 6: `internal/listener.Manager` — multi-listener build + Start/Stop + tests

**Files:**
- Create: `internal/listener/doc.go`
- Create: `internal/listener/manager.go`
- Create: `internal/listener/manager_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0025)
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

This is the largest single internal package in phase 02 by code volume. Follow TDD: tests first, but write the test cases as separate `func Test*` rather than one omnibus test, so each test independently fails-then-passes during incremental implementation.

- [ ] **Step 1: Write `internal/listener/manager_test.go` (build-error cases first)**

Cover (each its own `func TestManager_*`):
- `HappyPath_Single` — one listener, one filter chain, one tcp_proxy filter against an extant cluster. `NewManager` returns no error. `Listeners()` is empty before `Start` (sockets not bound until Start). `Start(ctx)` binds the socket, `Listeners()` then returns a slice of 1 with `Name == "l_tcp"` and `Addr` reflecting the bound ephemeral port. `Stop()` closes the socket; a fresh `net.Dial` to the recorded address fails within a short timeout.
- `HappyPath_Multi` — two listeners (`l_tcp_a`, `l_tcp_b`) both targeting the same cluster. After `Start`, `Listeners()` returns 2 entries; each address is dialable; both addresses differ. `Stop()` closes both.
- `Error_ZeroListeners` — empty `static_resources.listeners`. Expect error `"listener: zero listeners in bootstrap"`.
- `Error_DuplicateName` — two listeners both named `"l_tcp"`. Expect error containing `"duplicate listener"`.
- `Error_TwoFilterChains` — listener with `len(filter_chains) == 2`. Expect error containing `"expected exactly one filter_chain"`.
- `Error_NonEmptyFilterChainMatch` — listener whose single filter_chain has `filter_chain_match: { destination_port: 80 }` (or any non-zero match). Expect error containing `"filter_chain_match"`.
- `Error_TwoFilters` — single filter_chain with `len(filters) == 2`. Expect error containing `"expected exactly one filter"`.
- `Error_PopulatedTransportSocket` — single filter_chain with `transport_socket: { name: "envoy.transport_sockets.tls" }`. Expect error containing `"transport_socket"` and mentioning TLS = phase 03.
- `Error_UnknownFilterTypeURL` — filter `typed_config.type_url = "type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo"`. Expect error containing `"unknown filter type_url"`.
- `Error_FilterConstructionPropagated` — filter is tcp_proxy but with `cluster: "c_does_not_exist"`. Expect error wrapping the `tcpproxy: cluster "c_does_not_exist" not found` from `NewFilter`, prefixed with the listener name (e.g., `listener "l_tcp": tcpproxy: cluster "c_does_not_exist" not found`).
- `Error_NonSocketAddressListener` — listener with `address.pipe` instead of `address.socket_address`. Expect error containing `"socket_address"`.
- `BindUnwind` — two listeners, the second one configured to bind `address: "127.0.0.1", port: <port-already-held-by-the-test>`. Expect `Start` to return an error and `Listeners()` to report 0 entries (the first listener was bound and then closed during unwind). Use a helper that allocates and holds an `*net.TCPListener`, then takes its port and feeds the same port into the second listener's bootstrap.

Test-helper builder (similar to Task 3, lives in `manager_test.go`):
```go
func mkBoot(adminPort uint32, listeners []*listenerv3.Listener, clusters []*clusterv3.Cluster) *bootstrapv3.Bootstrap {
    return &bootstrapv3.Bootstrap{
        StaticResources: &bootstrapv3.Bootstrap_StaticResources{
            Listeners: listeners,
            Clusters:  clusters,
        },
    }
}
func mkListener(name, addr string, port uint32, filter *listenerv3.Filter) *listenerv3.Listener {
    return &listenerv3.Listener{
        Name: name,
        Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
            SocketAddress: &corev3.SocketAddress{Address: addr, PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port}},
        }},
        FilterChains: []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
    }
}
func mkTcpProxyFilter(t *testing.T, clusterName string) *listenerv3.Filter {
    t.Helper()
    msg := &tcpproxyv3.TcpProxy{StatPrefix: "ingress_tcp", ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: clusterName}}
    a, err := anypb.New(msg)
    if err != nil { t.Fatalf("anypb.New: %v", err) }
    return &listenerv3.Filter{Name: "envoy.filters.network.tcp_proxy", ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a}}
}
```

Imports: `listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"`, plus the cluster/endpoint/core imports from Task 3 and the anypb/tcp_proxy imports from Task 4.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/listener/ -v`
Expected: FAIL with build error.

- [ ] **Step 3: Write `internal/listener/doc.go`**

```go
// Package listener materialises one listener per static_resources.listeners[]
// entry of an Envoy v3 Bootstrap proto, wires each listener's single filter
// chain to its terminal filter via an inline filter constructor registry,
// binds each listener's TCP socket on Start, and runs one Accept goroutine
// per listener dispatching accepted connections into the filter's Handle.
//
// Phase 02 supports a deliberately narrow filter_chain shape (exactly one
// chain, exactly one terminal filter, empty filter_chain_match, no transport
// socket); ADR-0025 codifies the subset and points at phase 07 for the full
// filter chain framework. The inline filter registry is also a phase-02
// simplification — phase 07 generalises it into an exported package.
//
// See SPEC §5.2, §5.3, ADR-0025.
package listener
```

- [ ] **Step 4: Write `internal/listener/manager.go`**

```go
package listener

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
)

// ListenerInfo is the after-Start description of one bound listener: the name
// from the bootstrap and the actually-bound socket address (resolved from the
// configured address; differs when the bootstrap requested port_value: 0).
type ListenerInfo struct {
	Name string
	Addr string
}

// filterHandler is the abstract behaviour every constructed filter must offer.
// Inline registry value type — phase 07 lifts to an exported interface.
type filterHandler interface {
	Handle(ctx context.Context, downstream net.Conn)
}

// filterConstructor builds a filterHandler from a typed_config Any and the
// resolved cluster manager. Phase 02 has exactly one entry: tcp_proxy.
type filterConstructor func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error)

// filterRegistry maps a filter typed_config.type_url to its constructor.
// SPEC §5.3: inline; phase 07 generalises.
var filterRegistry = map[string]filterConstructor{
	tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error) {
		f, err := tcpproxy.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
}

// builtListener is an internal record between NewManager (build) and Start
// (bind + serve). The filter is constructed at NewManager time so that bad
// configurations fail before any socket is touched.
type builtListener struct {
	name     string
	bindHost string
	bindPort uint32
	filter   filterHandler

	// Set during Start.
	socket net.Listener
}

// Manager owns every listener materialised from static_resources.listeners[]
// and supervises their accept loops.
type Manager struct {
	built     []*builtListener
	startedMu sync.Mutex
	started   bool
}

// NewManager validates the listeners in bs against the phase-02 filter-chain
// subset (ADR-0025) and constructs each listener's terminal filter against cm.
// Returns an error on the first violation; subsequent listeners are not
// validated. No sockets are touched during NewManager.
//
// Every error begins with "listener: ".
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager) (*Manager, error) {
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	m := &Manager{built: make([]*builtListener, 0, len(ls))}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		bl, err := buildListener(l, i, cm)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[bl.name]; dup {
			return nil, fmt.Errorf("listener: duplicate listener name %q", bl.name)
		}
		seen[bl.name] = struct{}{}
		m.built = append(m.built, bl)
	}
	return m, nil
}

func buildListener(l *listenerv3.Listener, idx int, cm *cluster.Manager) (*builtListener, error) {
	name := l.GetName()
	if name == "" {
		return nil, fmt.Errorf("listener: listeners[%d]: missing name", idx)
	}
	addr := l.GetAddress().GetSocketAddress()
	if addr == nil {
		return nil, fmt.Errorf("listener: %q: address is not a socket_address", name)
	}
	chains := l.GetFilterChains()
	if len(chains) != 1 {
		return nil, fmt.Errorf("listener: %q: expected exactly one filter_chain, got %d", name, len(chains))
	}
	chain := chains[0]
	if m := chain.GetFilterChainMatch(); m != nil && !proto.Equal(m, &listenerv3.FilterChainMatch{}) {
		return nil, fmt.Errorf("listener: %q: filter_chain_match must be empty in phase 02 (full match protocol = phase 07)", name)
	}
	if chain.GetTransportSocket() != nil {
		return nil, fmt.Errorf("listener: %q: transport_socket is not supported in phase 02 (TLS = phase 03)", name)
	}
	filters := chain.GetFilters()
	if len(filters) != 1 {
		return nil, fmt.Errorf("listener: %q: expected exactly one filter, got %d", name, len(filters))
	}
	tc := filters[0].GetTypedConfig()
	if tc == nil {
		return nil, fmt.Errorf("listener: %q: filter typed_config is nil", name)
	}
	ctor, ok := filterRegistry[tc.GetTypeUrl()]
	if !ok {
		return nil, fmt.Errorf("listener: %q: unknown filter type_url %q", name, tc.GetTypeUrl())
	}
	fh, err := ctor(tc, cm)
	if err != nil {
		return nil, fmt.Errorf("listener: %q: %w", name, err)
	}
	return &builtListener{
		name:     name,
		bindHost: addr.GetAddress(),
		bindPort: addr.GetPortValue(),
		filter:   fh,
	}, nil
}

// Start binds every built listener, captures its bound address, and launches
// one Accept goroutine per listener. Returns after every bind succeeds (or on
// the first bind failure, after unwinding any already-bound sockets). Every
// error begins with "listener: ".
//
// Per SPEC §5.2 and SPEC §10 #7 (settled): a single ctx is captured once and
// shared across every Accept loop and dispatched filter Handle invocation.
// Cancelling ctx (or calling Stop) drops the loops within one accept call.
func (m *Manager) Start(ctx context.Context) error {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	if m.started {
		return fmt.Errorf("listener: Start already called")
	}
	for i, bl := range m.built {
		addr := fmt.Sprintf("%s:%d", bl.bindHost, bl.bindPort)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			// Unwind: close any already-bound listeners.
			for j := 0; j < i; j++ {
				_ = m.built[j].socket.Close()
				m.built[j].socket = nil
			}
			return fmt.Errorf("listener: %q: bind %s: %w", bl.name, addr, err)
		}
		bl.socket = ln
	}
	for _, bl := range m.built {
		bl := bl
		go acceptLoop(ctx, bl)
	}
	m.started = true
	return nil
}

// acceptLoop runs until the listener is closed (Stop) or ctx is cancelled.
// Each accepted connection is handed to filter.Handle in its own goroutine.
func acceptLoop(ctx context.Context, bl *builtListener) {
	for {
		conn, err := bl.socket.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("listener: %q: accept: %v", bl.name, err)
			continue
		}
		go bl.filter.Handle(ctx, conn)
	}
}

// Listeners returns one ListenerInfo per bound listener. Empty before Start
// or after a Start that errored out (the unwind clears every socket).
func (m *Manager) Listeners() []ListenerInfo {
	out := make([]ListenerInfo, 0, len(m.built))
	for _, bl := range m.built {
		if bl.socket == nil {
			continue
		}
		out = append(out, ListenerInfo{Name: bl.name, Addr: bl.socket.Addr().String()})
	}
	return out
}

// Stop closes every bound listener socket. Idempotent. In-flight Handle
// goroutines are not waited on (drain semantics = phase 08).
func (m *Manager) Stop() {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	for _, bl := range m.built {
		if bl.socket != nil {
			_ = bl.socket.Close()
			bl.socket = nil
		}
	}
	m.started = false
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/listener/ -v`
Expected: PASS, every `TestManager_*`.

- [ ] **Step 6: Run lint + vet**

Run: `go vet ./internal/listener/ && golangci-lint run ./internal/listener/`
Expected: PASS.

- [ ] **Step 7: Append ADR-0025 to `docs/envoy-go/DECISIONS.md`**

Per the body in `## ADRs introduced by this plan`. The ADR's Decision section lists every build-time error case; the Consequences section names phase 07 as the supersession owner.

- [ ] **Step 8: Append Task 6 entry to PROGRESS.md and commit**

```bash
git add internal/listener/doc.go internal/listener/manager.go internal/listener/manager_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: internal/listener.Manager — multi-listener build + Start/Stop [ADR-0025]"
```

---

## Task 7: Cutover — rewire cmd/envoy-go/main.go + harness + fixture interface + bootstrap deletions (atomic)

**Files (all in one commit — see "Why this task is atomic" below):**
- Modify: `cmd/envoy-go/main.go`
- Modify: `cmd/envoy-go/main_test.go`
- Modify: `internal/bootstrap/bootstrap.go` (delete `FirstListenerSocket`, `FirstClusterEndpointSocket`)
- Modify: `internal/bootstrap/bootstrap_test.go` (delete the deleted functions' tests)
- Modify: `test/differential/harness.go` (clean-break sentinel parser; `ListenerAddr(name)` accessor)
- Modify: `test/differential/fixture/fixture.go` (interface gains `BackendCount`, `SubjectListenerName`; `[]int` backend ports in templating signatures; optional `DistributionAsserter`)
- Modify: `test/differential/runner_test.go` (multi-backend allocation; counter wiring; `subj.ListenerAddr(name)` lookup; optional AssertDistribution call)
- Modify: `test/fixtures/0000-tcp-echo/driver/driver.go` (new interface usage; `BackendCount = 1`; `SubjectListenerName = "l_tcp"`)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0022 + ADR-0026)
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

### Why this task is atomic (SPEC §10 #5 + dependency graph)

The cutover changes the ready-sentinel format AND the harness parser AND the FixtureDriver interface AND the runner's backend-allocation strategy AND deletes the now-unused `internal/bootstrap.First*` helpers AND lifts `cmd/envoy-go/main.go` from the ad-hoc pump to the listener-manager dispatch. Every one of these depends on every other:

- The harness can't switch parser format without `cmd/envoy-go/main.go` emitting the new format.
- `cmd/envoy-go/main.go` can't emit the new format without the listener manager driving it (which builds on Tasks 3 + 6).
- `cmd/envoy-go/main.go` can't drop `FirstListenerSocket` calls without lifting them into the listener manager (done in Task 3 + 6 via `cluster.NewManager` + `listener.NewManager`).
- The fixture-0000 driver can't consume `subj.ListenerAddr("l_tcp")` until the harness exposes that method, and can't NOT consume it once the harness deletes the no-arg form.
- The runner can't allocate N backends by `BackendCount()` until the FixtureDriver interface exposes it; until the runner does, the new fixture (Task 9) cannot run.

SPEC §10 #5 forbids backward-compat sentinel emission. Therefore: one commit. The PROGRESS entry for this task is the longest one in phase 02 and quotes every command output verbatim.

If during execution any single sub-step here blows up past 10 internal items (e.g., `cmd/envoy-go/main.go` reorganisation discovers a non-trivial wiring need that wasn't planned), STOP and invoke `superpowers:systematic-debugging` per BOOTSTRAP §6.1's mid-execution split valve. The natural split is then: 7a = harness + fixture interface + runner + fixture-0000 driver (mechanical interface plumbing); 7b = `cmd/envoy-go/main.go` rewire + bootstrap deletions + main_test.go (the substantive wiring change). Document the split with a new ADR (ADR-0028+).

### Sub-steps (TDD-aligned where possible)

- [ ] **Step 1: Write the failing `cmd/envoy-go/main_test.go` rewrite**

Replace the existing `TestEnvoyGoBinary_EchoesThroughUpstream` with a two-listener variant that asserts the new sentinel format and per-listener echo round-trip:

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/helpers"
)

// TestEnvoyGoBinary_TwoListenerCutover exercises the phase-02 dataplane: two
// listeners both proxying to the same single-endpoint cluster. Asserts the
// new per-listener ready-sentinel format (one `envoy-go listener <name> ready
// on <addr>` per listener, then a terminal `envoy-go ready`) and that each
// listener echoes byte-exact through the shared backend.
func TestEnvoyGoBinary_TwoListenerCutover(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEcho(backend)
	backendPort := backend.Addr().(*net.TCPAddr).Port

	listenerPortA := freeTCPPort(t)
	listenerPortB := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp_a
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp_a
                cluster: c_echo
    - name: l_tcp_b
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp_b
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, adminPort, listenerPortA, listenerPortB, backendPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_tcp_a", "l_tcp_b"}, 5*time.Second)

	for name, addr := range addrs {
		resp, err := helpers.TCPRoundTrip(ctx, addr, []byte("ping-7-cutover\n"), 500*time.Millisecond)
		if err != nil {
			t.Fatalf("%s round-trip: %v", name, err)
		}
		if string(resp) != "ping-7-cutover\n" {
			t.Errorf("%s: got %q, want %q", name, resp, "ping-7-cutover\n")
		}
	}
}

// waitForReadySentinels reads stdout line-by-line until every listener in
// `names` has a `envoy-go listener <name> ready on <addr>` line followed by
// the terminal `envoy-go ready` line. Returns the name → addr map.
func waitForReadySentinels(t *testing.T, r io.Reader, names []string, timeout time.Duration) map[string]string {
	t.Helper()
	want := map[string]struct{}{}
	for _, n := range names {
		want[n] = struct{}{}
	}
	got := map[string]string{}
	re := regexp.MustCompile(`^envoy-go listener (\S+) ready on (\S+)$`)
	deadline := time.Now().Add(timeout)
	br := bufio.NewReader(r)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			t.Fatalf("ready: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "envoy-go ready" {
			if len(got) == len(names) {
				return got
			}
			t.Fatalf("terminal sentinel before all listeners (%d/%d)", len(got), len(names))
		}
		if m := re.FindStringSubmatch(line); m != nil {
			if _, expected := want[m[1]]; !expected {
				t.Fatalf("unexpected listener name in sentinel: %q", m[1])
			}
			if _, dup := got[m[1]]; dup {
				t.Fatalf("duplicate sentinel for listener %q", m[1])
			}
			got[m[1]] = m[2]
		}
	}
	t.Fatalf("ready sentinels not seen within %s; got=%v", timeout, got)
	return nil
}

// echoConn / acceptEcho / freeTCPPort are kept as in phase 01; deletion of the
// old TestEnvoyGoBinary_EchoesThroughUpstream removes the old waitForReady.
type echoConn struct{ net.Conn }

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer func() { _ = c.Close() }(); _, _ = io.Copy(echoConn{c}, echoConn{c}) }(c)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}
```

Run: `go test ./cmd/envoy-go/ -run TestEnvoyGoBinary_TwoListenerCutover -v`
Expected: FAIL — the current `cmd/envoy-go/main.go` uses the old sentinel format and is single-listener.

- [ ] **Step 2: Rewrite `cmd/envoy-go/main.go`**

```go
// envoy-go is the phase-02 subject binary. It loads an Envoy v3 Bootstrap
// proto from YAML (internal/bootstrap), builds the cluster manager
// (internal/cluster) and the listener manager (internal/listener) which wires
// each listener to its terminal TCP proxy filter (internal/filter/tcpproxy),
// starts the admin /ready server (internal/admin), binds every listener, marks
// admin ready, prints per-listener + terminal ready sentinels, and blocks on
// SIGINT. The phase-00 ad-hoc TCP pump is gone — its byte-level logic now
// lives in internal/filter/tcpproxy/filter.go (ADR-0023).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/listener"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml>")
		os.Exit(2)
	}
	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	bs, err := bootstrap.Load(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	adminHost, adminPort, err := bootstrap.AdminSocket(bs)
	if err != nil {
		log.Fatalf("extract admin: %v", err)
	}
	adminAddr := fmt.Sprintf("%s:%d", adminHost, adminPort)

	cm, err := cluster.NewManager(bs)
	if err != nil {
		log.Fatalf("cluster manager: %v", err)
	}

	admSrv := admin.New(adminAddr)
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

	lm, err := listener.NewManager(bs, cm)
	if err != nil {
		log.Fatalf("listener manager: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := lm.Start(ctx); err != nil {
		log.Fatalf("listener start: %v", err)
	}
	defer lm.Stop()

	admSrv.MarkReady()

	// Per-listener ready sentinels + terminal sentinel (ADR-0026).
	for _, info := range lm.Listeners() {
		fmt.Fprintf(os.Stdout, "envoy-go listener %s ready on %s\n", info.Name, info.Addr)
	}
	fmt.Fprintln(os.Stdout, "envoy-go ready")

	<-ctx.Done()
}
```

Note: the `pump`/`halfClose`/`netConn` definitions previously at the bottom of `main.go` are GONE — they live in `internal/filter/tcpproxy/filter.go` (Task 4). Deleting them shrinks `main.go` from 119 lines to ~80.

- [ ] **Step 3: Delete `internal/bootstrap.FirstListenerSocket` and `FirstClusterEndpointSocket`**

In `internal/bootstrap/bootstrap.go`, delete the two functions (lines 74–134 in the phase-01 file). `Load` and `AdminSocket` remain. Re-verify imports — none of the Go imports become unused (the bootstrapv3 / yaml / json / protojson / fmt / io imports are still used by `Load` and `AdminSocket`).

In `internal/bootstrap/bootstrap_test.go`, delete every `TestFirstListenerSocket_*` and `TestFirstClusterEndpointSocket_*` block. Other tests are unchanged.

- [ ] **Step 4: Update `test/differential/harness.go`**

Replace the `readyAddr(line) string` parser (lines 233–240 in the phase-01 file) with a `readyListenerAddrs` parser that walks lines until the terminal `envoy-go ready` sentinel:

```go
// readyListenerAddrs reads lines from r until the terminal `envoy-go ready`
// sentinel is observed, collecting every `envoy-go listener <name> ready on
// <addr>` line into a name→addr map. ADR-0026 codifies the phase-02 sentinel
// contract.
func readyListenerAddrs(ctx context.Context, r io.Reader) (map[string]string, error) {
	br := bufio.NewReader(r)
	out := make(chan map[string]string, 1)
	errCh := make(chan error, 1)
	go func() {
		addrs := map[string]string{}
		re := regexp.MustCompile(`^envoy-go listener (\S+) ready on (\S+)$`)
		for {
			line, err := br.ReadString('\n')
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "envoy-go ready" {
				out <- addrs
				return
			}
			if m := re.FindStringSubmatch(trimmed); m != nil {
				addrs[m[1]] = m[2]
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case a := <-out:
		return a, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
```

In `StartSubjectProxy`, replace the `scanForLine(readyCtx, stdout, "envoy-go ready on ")` + `readyAddr(line)` pair with:
```go
readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
defer cancel()
addrs, err := readyListenerAddrs(readyCtx, stdout)
if err != nil { /* kill, cleanup, return */ }
```

`SubjectProxy.listenerAddr string` (single) becomes `SubjectProxy.listenerAddrs map[string]string`. The accessor changes:
```go
// ListenerAddr returns the host:port the subject is listening on for the
// named listener (parsed from the per-listener ready sentinel). Returns "" if
// the name is unknown.
func (s *SubjectProxy) ListenerAddr(name string) string {
	return s.listenerAddrs[name]
}
```

Delete the old `readyAddr` function entirely. Delete the `scanForLine` helper if it's now unused (it was only called from the deleted single-line path); confirm with `grep -n scanForLine test/differential/`.

- [ ] **Step 5: Update `test/differential/fixture/fixture.go`**

```go
// Driver is the contract a fixture under test/fixtures/NNNN-*/driver
// implements. Drivers register themselves in init(); the runner discovers
// them by name (which must match the fixture directory).
type Driver interface {
	// BackendCount is the number of host-side TCP echo backends the runner
	// allocates per fixture run. Each backend gets its own random port and
	// its own atomic.Uint64 accept counter that the runner snapshots after
	// Drive completes.
	BackendCount() int

	// SubjectListenerName is the listener name the driver's Drive targets.
	// The runner uses subj.ListenerAddr(SubjectListenerName()) to look up
	// the subject's bound address per the ADR-0026 sentinel format.
	SubjectListenerName() string

	// ReferenceBootstrap returns the YAML to feed upstream Envoy. The
	// runner passes the slice of allocated backend ports; the driver
	// templates them into its config however it wants.
	ReferenceBootstrap(backendPorts []int) string

	// SubjectConfig renders the subject's bootstrap. backendPorts is the
	// same slice the runner generated for ReferenceBootstrap.
	SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string

	// ReferenceListenerPort is the in-container TCP port the reference
	// proxy must expose (the listener the driver dials).
	ReferenceListenerPort() int

	// Drive sends fixture-specific traffic at refAddr and subjAddr (each is
	// a host:port for the listener under test in each proxy). Returns the
	// captured byte streams for diffing.
	Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)

	// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
	// returns the raw response bytes (status line + headers + body) for the
	// differential diff. Phase 01.
	ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error)
}

// DistributionAsserter is an optional driver-side check the runner invokes
// after Drive when the driver implements it. The runner passes per-backend
// accept counts in the same order as backendPorts.
type DistributionAsserter interface {
	AssertDistribution(refCounts, subjCounts []uint64) error
}
```

- [ ] **Step 6: Update `test/fixtures/0000-tcp-echo/driver/driver.go`**

```go
func (echoDriver) BackendCount() int           { return 1 }
func (echoDriver) SubjectListenerName() string { return "l_tcp" }

func (echoDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0000-tcp-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	// Same STRICT_DNS reference template as before, but render the port
	// directly instead of relying on the runner's strings.Replace.
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: host.docker.internal
                      port_value: %d
`, backendPorts[0])
}

func (echoDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0000-tcp-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0000, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, subjAdminPort, subjListenerPort, backendPorts[0])
}
```

The old `refBootstrap` const is deleted (its content lives inline in `ReferenceBootstrap` now). The bottom-of-file comment about "the runner performs a strings.Replace" is also deleted (the runner no longer does that).

- [ ] **Step 7: Update `test/differential/runner_test.go` for multi-backend allocation**

```go
func runFixture(t *testing.T, root string, pin *EnvoyPin, _ string, d FixtureDriver) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. N backends, each with its own atomic.Uint64 accept counter.
	n := d.BackendCount()
	if n < 1 {
		t.Fatalf("BackendCount() returned %d; must be ≥1", n)
	}
	type backend struct {
		ln      net.Listener
		port    int
		accepts *atomic.Uint64
	}
	backends := make([]*backend, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatalf("backend[%d] listen: %v", i, err)
		}
		defer func(ln net.Listener) { _ = ln.Close() }(ln)
		bo := &backend{ln: ln, port: ln.Addr().(*net.TCPAddr).Port, accepts: new(atomic.Uint64)}
		backends[i] = bo
		go acceptEchoCounting(ln, bo.accepts)
	}
	backendPorts := make([]int, n)
	for i, b := range backends {
		backendPorts[i] = b.port
	}

	// 2. Reference proxy.
	bootstrap := d.ReferenceBootstrap(backendPorts)
	ref, err := StartReferenceProxy(ctx, pin, bootstrap, d.ReferenceListenerPort())
	if err != nil {
		t.Fatalf("ref start: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	refAddr := ref.ListenerAddr(d.ReferenceListenerPort())

	// 3. Subject proxy.
	subjPort := freeTCPPort(t)
	subjAdminPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPorts, subjAdminPort)
	subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
	if err != nil {
		t.Fatalf("subj start: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	// 4. Reset reference-side accept counts (the reference container started
	// during step 2 may have triggered some accepts via its admin probe to
	// host.docker.internal — those count against the backends. Snapshot here
	// is the baseline; we only credit the difference.
	refBaseline := make([]uint64, n)
	for i, b := range backends {
		refBaseline[i] = b.accepts.Load()
	}

	// 5. Drive ref.
	refBytes, _, err := d.Drive(ctx, refAddr, "")
	if err != nil {
		// `Drive`'s convention: when subjAddr is "", drive only ref.
		// (Phase-02 fixtures honour this. Phase-01 fixture-0000's Drive
		// drove both at once; it's updated in this commit to honour the
		// "" sentinel — see test/fixtures/0000-tcp-echo/driver/driver.go.)
		t.Fatalf("ref drive: %v", err)
	}
	refCounts := make([]uint64, n)
	for i, b := range backends {
		refCounts[i] = b.accepts.Load() - refBaseline[i]
	}
	subjBaseline := make([]uint64, n)
	for i, b := range backends {
		subjBaseline[i] = b.accepts.Load()
	}

	// 6. Drive subj.
	_, subjBytes, err := d.Drive(ctx, "", subj.ListenerAddr(d.SubjectListenerName()))
	if err != nil {
		t.Fatalf("subj drive: %v", err)
	}
	subjCounts := make([]uint64, n)
	for i, b := range backends {
		subjCounts[i] = b.accepts.Load() - subjBaseline[i]
	}

	// 7. Diff response bytes.
	v, err := CompareBytes(refBytes, subjBytes)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !v.Equal {
		t.Errorf("differential mismatch:\n%s", v.HexDump)
	}

	// 8. Optional distribution assertion.
	if da, ok := d.(fixture.DistributionAsserter); ok {
		if err := da.AssertDistribution(refCounts, subjCounts); err != nil {
			t.Errorf("distribution: %v", err)
		}
	}

	// 9. Admin /ready observation (phase 01 carry-over).
	refAdm, subjAdm, err := d.ProbeAdmin(ctx, ref.AdminAddr(), subj.AdminAddr())
	if err != nil {
		t.Fatalf("admin probe: %v", err)
	}
	vAdm, err := compareAdminResponses(refAdm, subjAdm, d)
	if err != nil {
		t.Fatalf("admin compare: %v", err)
	}
	if !vAdm.Equal {
		t.Errorf("admin differential mismatch:\n%s", vAdm.HexDump)
	}
}

func acceptEchoCounting(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					_, _ = c.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}(c)
	}
}
```

**Important convention change:** the runner now drives ref and subject *separately* (so it can snapshot per-side counters between drives). The `Drive(ctx, refAddr, subjAddr)` signature stays as-is for compatibility, but the runner passes `""` for the side it doesn't want driven on a given call. Drivers honour `""` by no-op'ing the corresponding side. Update fixture-0000 driver in Step 6 above to honour this:

```go
func (echoDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	uid := randHex(6)
	var payload []byte
	for n := 0; n < 10; n++ {
		payload = append(payload, []byte(fmt.Sprintf("ping-%d-%s\n", n, uid))...)
	}
	if refAddr != "" {
		refBytes, err = helpers.TCPRoundTrip(ctx, refAddr, payload, time.Second)
		if err != nil {
			return nil, nil, fmt.Errorf("ref drive: %w", err)
		}
	}
	if subjAddr != "" {
		subjBytes, err = helpers.TCPRoundTrip(ctx, subjAddr, payload, time.Second)
		if err != nil {
			return nil, nil, fmt.Errorf("subj drive: %w", err)
		}
	}
	return refBytes, subjBytes, nil
}
```

The old `acceptEcho` helper at the bottom of `runner_test.go` is replaced by `acceptEchoCounting`. Delete the old helper if no other caller exists (it was only called from `runFixture`'s old single-backend code).

- [ ] **Step 8: Run unit + cmd tests (no Docker yet)**

Run: `go build ./... && go vet ./... && golangci-lint run ./... && go test -short ./...`
Expected: PASS — every package compiles, lints clean, and the cmd-level integration test (`TestEnvoyGoBinary_TwoListenerCutover`) passes. The differential test is skipped under `-short`.

If any compile error references `internal/bootstrap.FirstListenerSocket` or `FirstClusterEndpointSocket` from outside `cmd/envoy-go/main.go`, that means a stale caller exists — `grep -rn "FirstListenerSocket\|FirstClusterEndpointSocket"` to find and update.

- [ ] **Step 9: Append ADR-0022 and ADR-0026 to `docs/envoy-go/DECISIONS.md`**

Per the bodies in `## ADRs introduced by this plan`. Both ADRs land in this commit.

- [ ] **Step 10: Append Task 7 entry to PROGRESS.md and commit**

PROGRESS entry header: `## Task 7 — Cutover: cmd/envoy-go rewire + harness + fixture interface + bootstrap deletions`. Quote `go build`, `go vet`, `golangci-lint run`, `go test -short ./...` outputs verbatim. Mention the deleted lines (LoC removed) for traceability.

```bash
git add -A   # this commit touches many files; -A is appropriate here, but verify with `git status` first
git status   # expect: ~10 modified, the new lines in DECISIONS, the new PROGRESS entry
git commit -m "phase 02: cutover — cmd/envoy-go rewire to listener+cluster managers; harness + fixture interface for multi-backend [ADR-0022, ADR-0026]"
```

Verify the commit's tree:
```bash
git show --stat HEAD
```
Expected to touch: `cmd/envoy-go/main.go`, `cmd/envoy-go/main_test.go`, `internal/bootstrap/bootstrap.go`, `internal/bootstrap/bootstrap_test.go`, `test/differential/harness.go`, `test/differential/fixture/fixture.go`, `test/differential/runner_test.go`, `test/fixtures/0000-tcp-echo/driver/driver.go`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`. If a file is unexpectedly absent, investigate before proceeding.

---

## Task 8: `BEHAVIOR_CONTRACT.md` — TCP proxy subsection

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

No code change; documentation only. The new subsection codifies what the differential gate asserts and disclaims for tcp_proxy-terminated TCP fixtures. The text is the BEHAVIOR_CONTRACT consequence of ADR-0024 (per-cluster RR scope) and the SPEC §1 invariant 9.

- [ ] **Step 1: Append a new top-level H2 section to `docs/envoy-go/BEHAVIOR_CONTRACT.md`**

Inserted after the existing `## Test harness host networking` section (the file's current tail). Content:

```markdown
## TCP proxy

*Introduced by phase 02. Justified by ADR-0024 (per-cluster RR scope) and SPEC §5.4 / §5.5 / §5.8.*

### Response-body byte-equivalence (asserted)

For any fixture whose subject and reference both terminate a TCP connection through `envoy.filters.network.tcp_proxy` (proto `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) backed by a STATIC (subject) or STRICT_DNS (reference) cluster of echo backends, the differential harness compares the concatenated **response bodies** byte-for-byte. Trivially holds for echo backends (each backend reflects the request bytes); for non-echo backends a phase-specific subsection extends this rule.

### Half-close propagation (asserted)

Both proxies must propagate `CloseWrite()` (FIN on the write side) from downstream-to-upstream and from upstream-to-downstream independently — i.e., the dataplane is a true bidirectional pipe with independent half-closes, not a request-response pair. Phase 02 inherits this property from phase 00's `netConn` + `halfClose` byte pump (lifted verbatim per ADR-0023).

### Load-balancer endpoint-selection sequence (NOT asserted)

Cross-proxy LB endpoint-selection sequence is **not** a differential equivalence dimension. Each proxy must be RR-correct **in its own right** (per-proxy distribution): for an N-pick run against a cluster of M endpoints, the per-backend accept count distribution must equal a perfect mod-M partition when `M | N`. This is a **local correctness property** asserted via the optional `DistributionAsserter` interface in `test/differential/fixture/fixture.go`.

The cross-proxy sequence is not asserted because:
- Upstream Envoy's RR LB is per-worker-thread with a randomised starting offset; the absolute sequence of endpoints selected for N consecutive connections is not reproducible across runs or workers.
- The envoy-go subject's RR is per-cluster with a deterministic starting point at index 0 (ADR-0024 + SPEC §5.4 sequence-starts-at-0 invariant); the sequence is reproducible within a single subject process but does not match upstream's randomised sequence.

A phase that needs cross-proxy LB sequence equivalence (e.g., a hash-based LB phase under the load-balancing family) supersedes this subsection with a new ADR documenting the assertion mechanism.

### Listener-bind error semantics (asserted)

If any listener fails to bind, neither proxy should partially serve. Upstream Envoy aborts startup with a non-zero exit and a diagnostic on stderr; phase-02 envoy-go's `cmd/envoy-go/main.go` calls `log.Fatalf("listener start: %v", err)` with the same effect. The two are not byte-compared (`log.Fatalf` and Envoy's startup-error format differ visibly), but both proxies' `/ready` admin endpoint never reports ready in this case (the subject's `MarkReady()` is never reached).

### Applies to

- Phase-02 envoy-go `internal/listener` + `internal/cluster` + `internal/filter/tcpproxy` packages, exercised via fixtures `0000-tcp-echo` and `0001-tcp-proxy-rr`.

### Does not yet apply to

- Filter chain matching (`filter_chain_match` non-empty) — phase 07.
- Multiple filters in a chain — phase 07.
- TLS — phase 03.
- HTTP-aware proxying (`HttpConnectionManager`) — phase 04+.
- LB policies other than ROUND_ROBIN — load-balancing family.
- Cluster types other than STATIC (subject side) / STRICT_DNS (reference side, per ADR-0010) — later phases.
- Health-check-driven endpoint selection — upstream-robustness family.
```

- [ ] **Step 2: Append Task 8 entry to PROGRESS.md and commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: BEHAVIOR_CONTRACT — TCP proxy subsection"
```

---

## Task 9: Fixture `0001-tcp-proxy-rr` — bootstraps + driver + AssertDistribution

**Files:**
- Create: `test/fixtures/0001-tcp-proxy-rr/envoy.yaml`
- Create: `test/fixtures/0001-tcp-proxy-rr/envoy-go.yaml`
- Create: `test/fixtures/0001-tcp-proxy-rr/expectations.yaml`
- Create: `test/fixtures/0001-tcp-proxy-rr/README.md`
- Create: `test/fixtures/0001-tcp-proxy-rr/driver/driver.go`
- Create: `test/fixtures/0001-tcp-proxy-rr/driver/driver_test.go`
- Modify: `test/differential/runner_test.go` (blank import the new driver package)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0027)
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

- [ ] **Step 1: Write `test/fixtures/0001-tcp-proxy-rr/envoy.yaml`** (documentation mirror; runner uses the driver-rendered version)

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15001 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY     # ADR-0010
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: host.docker.internal, port_value: 0 }   # backend 1 — runner-rendered
              - endpoint:
                  address:
                    socket_address: { address: host.docker.internal, port_value: 0 }   # backend 2 — runner-rendered
              - endpoint:
                  address:
                    socket_address: { address: host.docker.internal, port_value: 0 }   # backend 3 — runner-rendered
```

The `port_value: 0` placeholders are documentary — the actual ports are templated by the driver's `ReferenceBootstrap(backendPorts)` method.

- [ ] **Step 2: Write `test/fixtures/0001-tcp-proxy-rr/envoy-go.yaml`** (documentation mirror)

```yaml
node: { id: envoy-go-subject-0001, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }     # subjAdminPort — driver-rendered
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 } # subjListenerPort — driver-rendered
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }   # backend 1 — driver-rendered
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }   # backend 2 — driver-rendered
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }   # backend 3 — driver-rendered
```

- [ ] **Step 3: Write `test/fixtures/0001-tcp-proxy-rr/expectations.yaml`**

```yaml
# Phase-02 fixture 0001-tcp-proxy-rr: differential expectations.
#
# response-body: byte-exact required (echo backend; each backend reflects bytes
# verbatim, so concatenated 9-request response is byte-equivalent across the
# two proxies regardless of which backend serves which request).
#
# response-status / response-headers / response-trailers: N/A (TCP, not HTTP).
#
# Admin /ready observation is exercised by the runner's compareAdminResponses
# path (phase-01 baseline; same allow-list as fixture 0000).
#
# Distribution assertion is the driver's AssertDistribution method (per-proxy
# counts must equal exactly [3, 3, 3] when 9 requests are sent against a
# 3-endpoint cluster). The runner invokes it via the DistributionAsserter
# interface assertion in test/differential/runner_test.go.
response-body:
  applicable: true
  scope: byte-exact
```

- [ ] **Step 4: Write `test/fixtures/0001-tcp-proxy-rr/README.md`**

```markdown
# Fixture 0001 — TCP proxy + round-robin (3 endpoints)

**Purpose:** end-to-end exercise the phase-02 dataplane (listener manager + STATIC cluster + round-robin LB + TCP proxy filter) and prove the per-proxy round-robin distribution is exact across a 9-request workload against a 3-endpoint cluster.

**Differential surface:** concatenated response bodies (9 echo round-trips) are byte-equivalent between upstream Envoy and envoy-go.

**Local-correctness surface:** each proxy's per-backend accept counts must be exactly `[3, 3, 3]` (asserted via `AssertDistribution` per the new BEHAVIOR_CONTRACT TCP proxy subsection).

**Topology:**

```
client (test) ──> [proxy listener 127.0.0.1:<subjPort>] ──RR──> [backend 1 / 2 / 3 on host:0.0.0.0:<random>]
client (test) ──> [container-mapped <hostPort>     ──Envoy──> [host.docker.internal:<random> 1/2/3 (V4_ONLY)]
```

Same client driver targets both proxies; the host-side backends serve both runs (with per-side counter snapshots taken between drives so the runner can credit each proxy's distribution independently).

**STATIC vs STRICT_DNS divergence (ADR-0010, ADR-0027):** the reference Envoy runs inside a Docker container and reaches host-side backends via `host.docker.internal` (which requires `STRICT_DNS` + `dns_lookup_family: V4_ONLY` per ADR-0010). The envoy-go subject runs as a host subprocess and dials literal 127.0.0.1 endpoints. The cluster *behaviour* is equivalent — three echo endpoints in round-robin order — but the *config shape* diverges by ADR. Same pattern fixture 0000 carries.

**Why distribution is not a differential dimension (BEHAVIOR_CONTRACT § TCP proxy):** upstream Envoy's RR LB is per-worker-thread with a randomised starting offset; the cross-proxy sequence of endpoint selections is not reproducible. Each proxy is asserted RR-correct in its own right (3/3/3 per proxy); cross-proxy sequence equivalence is explicitly NOT asserted.

**Run locally:**

```bash
go test ./test/differential/ -run TestDifferential/0001-tcp-proxy-rr -v
```

**Re-baseline:** if upstream Envoy's pin (ADR-0008) bumps and the differential gate fails, follow ADR-0008 §"refresh procedure" to re-record evidence and supersede the failing ADR if the bytes change.
```

- [ ] **Step 5: Write `test/fixtures/0001-tcp-proxy-rr/driver/driver.go`**

```go
// Package driver implements the 0001-tcp-proxy-rr fixture's driver: 9 TCP
// round-trips against a 3-endpoint cluster, with per-proxy distribution
// asserted to be exactly [3, 3, 3].
package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0001-tcp-proxy-rr"
const refContainerListenerPort = 15001
const requestsPerSide = 9

func init() {
	fixture.RegisterFixture(fixtureName, &rrDriver{})
}

type rrDriver struct{}

func (rrDriver) BackendCount() int                { return 3 }
func (rrDriver) SubjectListenerName() string      { return "l_tcp" }
func (rrDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

func (rrDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0001: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15001 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (rrDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0001: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0001, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// Drive sends 9 TCP round-trips against whichever side(s) are non-"".
func (rrDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	uid := randHex(6)
	var payloads [][]byte
	for n := 0; n < requestsPerSide; n++ {
		payloads = append(payloads, []byte(fmt.Sprintf("rr-%d-%s\n", n, uid)))
	}
	if refAddr != "" {
		var sb strings.Builder
		for i, p := range payloads {
			b, err := helpers.TCPRoundTrip(ctx, refAddr, p, time.Second)
			if err != nil {
				return nil, nil, fmt.Errorf("ref drive[%d]: %w", i, err)
			}
			sb.Write(b)
		}
		refBytes = []byte(sb.String())
	}
	if subjAddr != "" {
		var sb strings.Builder
		for i, p := range payloads {
			b, err := helpers.TCPRoundTrip(ctx, subjAddr, p, time.Second)
			if err != nil {
				return nil, nil, fmt.Errorf("subj drive[%d]: %w", i, err)
			}
			sb.Write(b)
		}
		subjBytes = []byte(sb.String())
	}
	return refBytes, subjBytes, nil
}

// AssertDistribution: each proxy's per-backend counts must be exactly [3,3,3]
// (9 requests / 3 endpoints, deterministic mod-index distribution).
func (rrDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	want := [3]uint64{3, 3, 3}
	for side, counts := range map[string][]uint64{"reference": refCounts, "subject": subjCounts} {
		if len(counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", side, len(counts))
		}
		var got [3]uint64
		copy(got[:], counts)
		if got != want {
			return fmt.Errorf("%s: distribution %v != %v", side, got, want)
		}
	}
	return nil
}

// ProbeAdmin reuses the phase-01 raw-socket /ready probe shape (mirrors
// fixture 0000's implementation; not extracted to a shared helper because the
// per-fixture admin-probe customisation lands in later phases).
func (rrDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Compile-time check the driver implements both required and optional ifaces.
var (
	_ fixture.Driver               = (*rrDriver)(nil)
	_ fixture.DistributionAsserter = (*rrDriver)(nil)
)
```

(If any import becomes temporarily orphaned during incremental TDD development, the executor uses a local `var _ = …` keepalive — but it must NOT survive into the commit; lint rejects it.)

The `helpers.HTTPGetReadyRaw` referenced above does NOT yet exist in `test/helpers/`. Two options:
- (a) add it as a new helper in `test/helpers/http_response.go` (which exists from phase 01) by extracting fixture-0000's `probeReady` and exporting it — the cleanest move; the new helper simplifies fixture 0000's driver too.
- (b) inline the probe logic in this driver — duplicates phase-01's `probeReady`.

Choose (a). The change is: in `test/helpers/http_response.go`, add `func HTTPGetReadyRaw(ctx context.Context, addr string) ([]byte, error) { … }` (the body is fixture-0000's existing `probeReady` verbatim, with the `addr` param). Then update `test/fixtures/0000-tcp-echo/driver/driver.go`'s `probeReady` to call the helper. This is a small additional touch on Task 9; document in the PROGRESS entry.

- [ ] **Step 6: Write `test/fixtures/0001-tcp-proxy-rr/driver/driver_test.go`**

```go
package driver

import "testing"

func TestAssertDistribution_Exact(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3}); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestAssertDistribution_Imbalanced(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{4, 3, 2}, []uint64{3, 3, 3}); err == nil {
		t.Fatal("expected error on imbalanced reference counts")
	}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{4, 3, 2}); err == nil {
		t.Fatal("expected error on imbalanced subject counts")
	}
}

func TestAssertDistribution_AllZero(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{0, 0, 0}, []uint64{0, 0, 0}); err == nil {
		t.Fatal("expected error on zero counts (sentinel of 'no traffic flowed')")
	}
}

func TestAssertDistribution_WrongLength(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{3, 3}, []uint64{3, 3, 3}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
```

- [ ] **Step 7: Add the blank import for the new driver to `test/differential/runner_test.go`**

```go
import (
    // … existing imports …
    _ "github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver"
    _ "github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver"
)
```

- [ ] **Step 8: Run unit tests and lint**

Run: `go vet ./... && golangci-lint run ./... && go test -short ./...`
Expected: PASS — fixture 0001's driver test passes; nothing else regresses; differential is skipped under `-short`.

- [ ] **Step 9: Append ADR-0027 to `docs/envoy-go/DECISIONS.md`**

Per `## ADRs introduced by this plan`. The Decision section names both fixture YAMLs explicitly and the divergence pattern.

- [ ] **Step 10: Append Task 9 entry to PROGRESS.md and commit**

```bash
git add test/fixtures/0001-tcp-proxy-rr/ test/differential/runner_test.go test/helpers/http_response.go test/fixtures/0000-tcp-echo/driver/driver.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: fixture 0001-tcp-proxy-rr — RR over 3 endpoints + AssertDistribution [ADR-0027]"
```

---

## Task 10: All-gates green run + PROGRESS final entry

**Files:**
- Modify: `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`

No code change. Runs every phase-02 gate locally and quotes outputs verbatim. The next session (lifecycle state 4 per ADR-0005) re-runs these as part of `superpowers:verification-before-completion`; this task's purpose is to make sure the dataplane works end-to-end before plan-execution exits, so the verification session has high signal that PROGRESS quotes are accurate.

- [ ] **Step 1: Build + vet + lint clean**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
```
Expected: PASS, no output beyond `go vet`'s normal silence.

- [ ] **Step 2: Unit tests + cmd-level integration test (no Docker)**

```bash
go test -short ./...
```
Expected: PASS — every package, including `cmd/envoy-go.TestEnvoyGoBinary_TwoListenerCutover`. The differential job is skipped under `-short`.

- [ ] **Step 3: Both fuzz targets at the ADR-0018 budget**

```bash
go test ./internal/bootstrap/        -run "^TestNothing$" -fuzz=FuzzBootstrapLoad   -fuzztime=30s
go test ./internal/filter/tcpproxy/  -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter  -fuzztime=30s
```
Expected: PASS, no failures discovered. Quote both verbatim outputs (mutation count + time) into PROGRESS.

- [ ] **Step 4: Differential suite (Docker required)**

```bash
go test ./test/differential/ -v -timeout=10m
```
Expected: PASS — `TestDifferential/0000-tcp-echo` AND `TestDifferential/0001-tcp-proxy-rr`. The 0001 sub-test verifies byte-exact response equivalence, AssertDistribution per-proxy, and the admin-probe equivalence (via the runner's `compareAdminResponses`). Wall-clock budget: ~90s per fixture (the harness's `context.WithTimeout`); two fixtures sequentially is ~3 minutes plus container startup overhead. The explicit `-timeout=10m` overrides Go's default 10-minute test timeout to leave headroom for cold-cache image pulls (the v1.37.2 image is ~250 MB).

If the differential job fails:
- Capture the failing diff (HexDump from `CompareBytes`).
- Inspect both proxies' stdout/stderr (the harness prints subject stderr to the test output; reference stderr is in the testcontainers logs).
- Most likely failure modes:
  - Listener bind race — `port_value: 0` in the subject bootstrap's listener should produce ephemeral binds; the harness reads the bound address from the per-listener sentinel, so no race should occur. If one does, see `Step 5` below.
  - DNS lookup family — the reference `STRICT_DNS` cluster MUST have `dns_lookup_family: V4_ONLY` per ADR-0010 or `host.docker.internal` may resolve to an unreachable IPv6.
  - LB sequence asymmetry — NOT a differential dimension (per BEHAVIOR_CONTRACT §TCP proxy); if the diff complains about *content* mismatch it is the response bytes (echoes are by-construction identical), not the order. Investigate as a real bug.
  - Distribution skew — per-proxy AssertDistribution failure is a real RR bug or a shared-counter bug; check ADR-0024's per-cluster scope holds.
- Per BOOTSTRAP §1 Step E, invoke `superpowers:systematic-debugging` on the symptom before any fix.

- [ ] **Step 5: Final PROGRESS entry — Task 10 with full verbatim outputs**

```markdown
## Task 10 — All-gates green run

**Commits:** <sha — this task's commit; touches only PROGRESS.md>
**Notes:** All phase-02 gates pass locally. Verification session (lifecycle state 4) will re-run these and capture for REVIEW.md.
**Outputs:**
```
$ go build ./...
<verbatim — typically silent on success>
$ go vet ./...
<verbatim>
$ golangci-lint run ./...
<verbatim>
$ go test -short ./...
<verbatim — last 30 lines>
$ go test ./internal/bootstrap/ -run "^TestNothing$" -fuzz=FuzzBootstrapLoad -fuzztime=30s
<verbatim — Go fuzzing summary lines>
$ go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=30s
<verbatim>
$ go test ./test/differential/ -v
<verbatim — including PASS lines for 0000-tcp-echo and 0001-tcp-proxy-rr subtests>
```
```

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md
git commit -m "phase 02: PROGRESS — Task 10 all-gates green run"
```

---

## PROGRESS.md conventions

The executor creates `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md` as part of Task 1's commit and appends to it on every subsequent task's commit. Format follows the phase-00/01 conventions exactly:

```markdown
# Phase 02 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha>
**Notes:** <one paragraph>
**Outputs:**
```
<verbatim command outputs>
```

## Task 2 — internal/cluster: Cluster + Endpoint + round-robin LB

**Commits:** <sha>
**Notes:** <one paragraph>
**Outputs:**
```
<verbatim>
```

…etc through Task 10…
```

PROGRESS.md is created in Task 1 and is part of every subsequent task's commit. Task header format: `## Task N — <short title>` (em-dash, not colon). Commits field is one SHA or a comma-separated list if a task hit a hook-fail-and-retry. Outputs are quoted verbatim — no summarization.

---

## Out-of-scope for this plan

These are explicitly NOT part of phase 02's plan and must not be added during execution. Each is deferred per SPEC §2 or §9:

- Filter chain framework (iteration protocol, per-route config, multi-filter chains, multi-chain matching). → phase 07.
- Cluster types other than STATIC (subject side) — STRICT_DNS subject support, EDS, LOGICAL_DNS, ORIGINAL_DST. → later phase when the subject needs DNS.
- LB policies other than ROUND_ROBIN. → load-balancing family.
- TLS (downstream termination, upstream origination, SNI). → phase 03.
- HTTP, HTTP/1.1 routing, HTTP/2, HTTP/3. → phases 04–05.
- Access log, stats, Prometheus integration. → phase 06.
- Admin endpoints other than `/ready`. → phase 08.
- Health checks, outlier detection, circuit breakers, retries. → upstream-robustness family.
- Connection pooling (per-protocol). → upstream-robustness family.
- `idle_timeout`, `max_connect_attempts`, `tunneling_config`, `hash_policy`, `access_log` on `TcpProxy`. → ignored at parse, deferred to later phases.
- `weighted_clusters` in `TcpProxy`. → later phase. Build-time error at phase 02.
- `listener_filters` on listeners. → silently ignored at parse, deferred.
- Graceful drain, SIGTERM, hot restart. → phase 08 (drain), runtime family (hot restart).
- Multi-listener / multi-cluster *differential* fixture. → unit-tested only at phase 02; a future phase may add a fixture without an ADR (the code path supports it).
- Any dependency outside the D-3.2 permitted-foundations list. `github.com/envoyproxy/go-control-plane/envoy` is imported as proto types only.

If reality during implementation pushes toward any of these, invoke `superpowers:systematic-debugging` and either re-scope the offending task in-place or initiate a §6 split per `BOOTSTRAP_PROMPT.md`.

---

## Exit criteria for this PLAN's executor (state-machine step 4 inputs)

When all 10 tasks are complete, the next session (running `superpowers:verification-before-completion` per ADR-0005 §4) verifies:

1. All SPEC §3 phase-done gates green: `(a)` differential fixture `0001-tcp-proxy-rr` green on response-body byte-exact AND per-proxy AssertDistribution; `(b)` differential fixture `0000-tcp-echo` green on TCP echo + admin `/ready` byte-exact (no regression); `(c)` N/A (phase 02 declares no conformance suites); `(d)` `FuzzTcpProxyFilter` clean at ADR-0018's 30s budget AND `FuzzBootstrapLoad` (phase-01 carry-over) clean at the same budget; `(e)` `go vet`, `golangci-lint run`, `go test ./...` all green locally and on CI; `(f)` `REVIEW.md` approved (later step).
2. `docs/envoy-go/BEHAVIOR_CONTRACT.md` contains a populated `## TCP proxy` section (not a `_to be filled_` placeholder) with the response-body byte-exact rule, the half-close propagation rule, the explicit non-equivalence of LB endpoint-selection sequence, and the listener-bind error semantics rule.
3. `docs/envoy-go/DECISIONS.md` contains ADR-0022 through ADR-0027 — six new ADRs — with the mapping ADR-0022=A, ADR-0023=B, ADR-0024=C, ADR-0025=D, ADR-0026=E, ADR-0027=F per SPEC §4.4.
4. `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md` quotes all 10 tasks' command outputs verbatim, including both fuzz runs and the differential suite.
5. `cmd/envoy-go/main.go` contains no direct `net.Listen`, `Accept`, `io.Copy`, `pump`, `halfClose`, or `netConn` definitions. Wiring only.
6. `internal/bootstrap.FirstListenerSocket` and `internal/bootstrap.FirstClusterEndpointSocket` and their tests do not exist in the tree (`grep -rn "FirstListenerSocket\|FirstClusterEndpointSocket" .` returns empty under tracked files).
7. `internal/cluster/`, `internal/filter/tcpproxy/`, `internal/listener/` packages exist with unit-test coverage of every build-time error path enumerated in SPEC §4.1.
8. `test/fixtures/0001-tcp-proxy-rr/` exists with `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, and `driver/driver_test.go`.
9. `STATE.md` will be advanced to `lifecycle-state: 4` (verification) at the executor's session-exit, with `next-skill: superpowers:verification-before-completion`.

The plan-authoring session (this session) exits at `lifecycle-state: 3` per ADR-0005 §1, with `next-skill: superpowers:subagent-driven-development`.

---

*End of PLAN.*
