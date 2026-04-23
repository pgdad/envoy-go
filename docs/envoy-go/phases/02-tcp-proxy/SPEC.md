# Phase 02 — TCP Proxy

**Phase id:** `02`
**Slug:** `02-tcp-proxy`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 01 (done)
**Differential surface at end of phase:** pre-existing fixture `0000-tcp-echo` green, now driven by a real listener-manager-hosted TCP proxy filter (not the phase-00 ad-hoc pump); new fixture `0001-tcp-proxy-rr` green, exercising a multi-endpoint static cluster with round-robin load balancing over echo backends.

---

## 1. Purpose

Phase 02 retires the phase-00/01 ad-hoc `cmd/envoy-go/main.go` TCP pump (one hard-coded listener, one hard-coded endpoint, invoked directly from `main`) and lands the first real dataplane in envoy-go: a listener manager that consumes every `static_resources.listeners[]` entry in the bootstrap, a cluster manager that resolves every `static_resources.clusters[]` entry, a filter chain that dispatches one typed network filter per listener (the TCP proxy filter, `envoy.filters.network.tcp_proxy`), and a round-robin load balancer over the cluster's endpoints.

Concretely, phase 02 produces:

1. A listener manager under `internal/listener/` that constructs one listener per bootstrap entry, wires each listener's single filter chain to a terminal network filter resolved from its `typed_config`, binds each listener's TCP socket, and accepts connections into the filter.
2. A cluster manager under `internal/cluster/` that constructs one cluster per bootstrap entry (type `STATIC` only), materialises its endpoints from `load_assignment.endpoints[].lb_endpoints[].endpoint.address.socket_address`, and exposes a round-robin load balancer that each accepted connection consults for an upstream endpoint.
3. A TCP proxy filter under `internal/filter/tcpproxy/` that parses `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy` from its `typed_config` Any, resolves the named cluster against the cluster manager at listener-manager build time, and for each accepted downstream connection picks an endpoint via round-robin, dials it, and pumps bytes bidirectionally with half-close propagation (byte-level semantics preserved verbatim from the phase-00 pump via ADR).
4. A rewired `cmd/envoy-go/main.go` whose runtime is: load bootstrap → build cluster manager → build listener manager → start admin server → start listener manager → mark admin ready → print per-listener ready sentinels. No direct `net.Listen` / `Accept` / `io.Copy` in `main`.
5. Retired first-only extractors: `internal/bootstrap.FirstListenerSocket` and `internal/bootstrap.FirstClusterEndpointSocket` are deleted; `internal/bootstrap.AdminSocket` remains (admin wiring is unchanged). The retirement is ADR'd.
6. An extended differential harness ready-sentinel protocol: the subject prints one `envoy-go listener <name> ready on <addr>\n` line per bound listener, followed by a terminal `envoy-go ready\n`. The harness gains a name-keyed `listenerAddr(name)` lookup; the legacy single-line `envoy-go ready on <addr>` format is retired.
7. Fixture `0000-tcp-echo` updated: its driver now selects listener `l_tcp` by name (no other behavioural change). The fixture's `envoy-go.yaml` and `envoy.yaml` are unchanged (they already carry a tcp_proxy filter, because phase 01 wrote them with the right shape in anticipation of phase 02 — phase 01 just ignored the filter and drove the pump ad-hoc).
8. A new fixture `test/fixtures/0001-tcp-proxy-rr/`: one listener, one STATIC cluster with three echo endpoints, driver sends nine TCP round-trip requests per run. Differential byte-exactness on response bodies (trivially holds — backends echo). Distribution uniformity (3/3/3 over 9 requests) is asserted locally per proxy via per-backend accept counters. Multi-endpoint behaviour is covered end-to-end.
9. A new `BEHAVIOR_CONTRACT.md` subsection, **TCP proxy**, codifying the byte-for-byte response-body equivalence on tcp_proxy-terminated connections and stating explicitly that load-balancer endpoint-selection ordering is **not** a differential equivalence dimension (it is a local correctness property of each proxy; upstream's RR is per-worker with randomised starting offset, so sequence-level equivalence is not asserted).
10. A small fuzz target `internal/filter/tcpproxy.FuzzTcpProxyFilter` that feeds random Any bytes into the filter constructor, satisfying the §7.4 discipline for the phase's new parse surface.

After phase 02, the project has proven its third central engineering claim: *envoy-go runs real Envoy dataplane primitives — listener + filter + cluster + LB — end-to-end and remains byte-equivalent to upstream Envoy on a deterministic TCP workload.* Every subsequent phase (TLS, HTTP/*, observability) layers on top of this dataplane without re-litigating its shape.

## 2. Non-purposes

Phase 02 does **not** do any of the following. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

- **Filter chain framework.** Phase 02 supports exactly ONE filter in a listener's single filter chain — the terminal TCP proxy filter. No iteration protocol, no chain-position state, no continue/stop/stopBuffered semantics, no per-route config. Multi-filter chains, read/write filter distinction, and the iteration protocol → phase 07.
- **Filter chain matching.** Phase 02 accepts exactly one `filter_chain` per listener, with no `filter_chain_match` set (or `filter_chain_match` that is an empty message). Fixtures whose listeners declare multiple chains or populate match criteria error at listener-manager build time. Destination-port / SNI / transport-protocol / server-names matching → phase 07.
- **Cluster types other than `STATIC`.** `STRICT_DNS`, `LOGICAL_DNS`, `EDS`, `ORIGINAL_DST`, `REDIS` and others error at cluster-manager build time. STRICT_DNS (needed for the reference container's `host.docker.internal` path — see ADR-0010) is *already* used on the reference side of fixture 0000-tcp-echo; phase 02 does not need STRICT_DNS on the subject side because the subject runs as a host subprocess and fixtures pin literal IPs. → a later phase (TBD) when subject-side DNS is needed.
- **Load-balancer policies other than round-robin.** `LEAST_REQUEST`, `RANDOM`, `RING_HASH`, `MAGLEV`, subset LB, locality-weighted LB, priority LB, panic thresholds → load balancing family (phase 09+).
- **Health checks, outlier detection, circuit breakers.** A picked endpoint is dialled unconditionally; dial failure closes the downstream connection and logs; no retry, no ejection. → upstream-robustness family.
- **Connection pooling.** Each downstream connection opens its own upstream `net.Dial`. No per-protocol connection pool. → upstream-robustness family.
- **`idle_timeout`, `max_connect_attempts`, `tunneling_config`, `hash_policy`, `access_log`.** Every `TcpProxy` proto field other than `stat_prefix` and `cluster` is *ignored* (not errored) at phase 02. `stat_prefix` is read and recorded on the filter struct but does not drive any stat emission (stats land in phase 06). Ignoring rather than erroring matches upstream Envoy's forward-compatibility stance and keeps fixtures portable across phases; fuzz coverage (gate (d)) ensures no ignored field causes a crash. A phase 06+ ADR revisits this when stats actually consume `stat_prefix`.
- **TLS.** `filter_chains[].transport_socket`, `DownstreamTlsContext`, `UpstreamTlsContext`, `Sni*` are not supported. If present in a fixture, an ADR-deferred error policy applies: phase 02 treats a populated `transport_socket` as a build-time error (distinct from "ignored future field") because TLS termination materially changes the bytes on the wire — silently ignoring would diverge from upstream. → phase 03.
- **HTTP, HTTP/2, HTTP/3.** No `HttpConnectionManager`, no router, no route match. → phases 04–05.
- **Access log, stats, Prometheus.** TCP proxy's `stat_prefix` is recorded but unused. Connection counters for the fixture's *distribution* assertion are emitted by the *test backends*, not by the subject's stats subsystem. → phase 06.
- **Admin endpoints beyond `/ready`.** Unchanged from phase 01. → phase 08.
- **Dynamic resources, runtime layer, xDS.** `dynamic_resources` and `layered_runtime` continue to error at bootstrap load (phase 01 carry-over). → xDS family.
- **Graceful drain / SIGTERM handling.** Ctrl-C / SIGINT exits the process; listeners are abandoned, in-flight connections are dropped. → phase 08.
- **Listener `listener_filters` chain.** Phase 02 ignores `listener.listener_filters` if present. Listener filters (e.g., `tls_inspector`, `original_dst`) → a later phase under the filter-chain-framework family or when a fixture requires them.
- **Multi-listener / multi-cluster in a differential fixture.** The code path supports N listeners and N clusters; a differential fixture that exercises both surfaces simultaneously is not part of phase 02's gate. Unit tests on the listener and cluster managers cover the N>1 shapes. Multi-listener/multi-cluster differential fixture → a later phase can add it without an ADR (the code already supports it).

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 02 lands only when every gate below is green. The generic `BOOTSTRAP_PROMPT.md` §7.5 gate set is narrowed:

| Gate | Specialization for phase 02 |
|---|---|
| (a) new/changed differential fixtures green | New fixture `test/fixtures/0001-tcp-proxy-rr/` passes: byte-exact response equivalence on 9 TCP round-trip requests through a 3-endpoint STATIC cluster; local distribution assertion (3 requests per backend on each proxy independently, within tolerance) passes. |
| (b) all pre-existing differential fixtures still green | Fixture `0000-tcp-echo` passes without regression under its existing `expectations.yaml`: TCP echo byte-exact AND admin `/ready` byte-exact still green. The fixture's driver is updated to select listener `l_tcp` by name (the only behavioural change). |
| (c) conformance suites pass | No conformance suite applies to phase 02 (h2spec is phase 05; h3spec is later; grpc is later). This gate is vacuously green. |
| (d) new fuzzer runs clean for CI short-budget | New fuzz target `internal/filter/tcpproxy.FuzzTcpProxyFilter` runs clean for its CI short-budget run (budget inherited from ADR-0018's 30-second policy unless a phase-02 ADR opts into a different budget). The phase-01 `internal/bootstrap.FuzzBootstrapLoad` also runs clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/listener/`, `internal/cluster/` (including the round-robin LB), and `internal/filter/tcpproxy/` are part of `go test ./...`. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed.

### 4.1 New production code

- **`internal/listener/manager.go`** — `Manager`, `NewManager`, `Start`, `Stop`, `Listeners`, `ListenerInfo`. Consumes `*bootstrapv3.Bootstrap` and a `*cluster.Manager`. Builds one `listener` per bootstrap entry, each with exactly one filter chain and exactly one terminal filter resolved through the filter constructor registry (inline; see §5.3).
- **`internal/listener/manager_test.go`** — unit tests: single listener happy path; multi-listener happy path; build error on ≥2 filter chains; build error on non-empty `filter_chain_match`; build error on ≥2 filters in a chain; build error on unknown filter `typed_config` type URL; build error on populated `transport_socket`; build error on duplicate listener name; bind error surface (try to bind a port already held by the test to exercise error path).
- **`internal/cluster/manager.go`** — `Manager`, `NewManager`, `Get`. Consumes `*bootstrapv3.Bootstrap`. Builds one `*Cluster` per bootstrap entry.
- **`internal/cluster/cluster.go`** — `Cluster`, `Endpoint`. Holds name, endpoint slice, load balancer. `PickEndpoint() (Endpoint, error)` is the dataplane entry point.
- **`internal/cluster/loadbalancer.go`** — unexported `loadBalancer` interface (`Pick() (Endpoint, error)`; endpoints closed over at construction) and the round-robin implementation. The round-robin uses a per-cluster `atomic.Uint64` counter; the authoritative formula is `i := counter.Add(1) - 1; return endpoints[int(i) % len(endpoints)]`, so the first pick is `endpoints[0]`, the second `endpoints[1 % N]`, and so on. This "sequence-starts-at-0" property is asserted by unit tests but is **not** part of the differential contract — upstream Envoy's RR is per-worker with a randomised starting offset, so sequence-level equivalence to upstream is not promised. See §5.4 for the authoritative interface definition.
- **`internal/cluster/manager_test.go`** — unit tests: single-cluster + single-endpoint happy; multi-cluster + multi-endpoint happy; Get by unknown name returns `ok=false`; build errors on `STRICT_DNS`, `LOGICAL_DNS`, `EDS`, `ORIGINAL_DST`; build error on unknown type; build error on duplicate cluster name; build error on zero endpoints (`lb_endpoints` empty); build error on non-socket_address endpoint address; build error on `lb_policy` != `ROUND_ROBIN` (phase 02 only supports ROUND_ROBIN, see §5.4).
- **`internal/cluster/loadbalancer_test.go`** — unit tests for round-robin: distribution over N=30 picks with 3 endpoints yields 10/10/10 exactly (the mod-index is deterministic); correctness under concurrent `Pick` from 100 goroutines (no panic, no out-of-range, distribution within ±2 of uniform over N=3000 picks).
- **`internal/filter/tcpproxy/filter.go`** — `Filter`, `NewFilter(tc *anypb.Any, clusterMgr *cluster.Manager) (*Filter, error)`, `Handle(ctx context.Context, downstream net.Conn)`. `NewFilter` unmarshals the Any into `*tcpproxyv3.TcpProxy`, reads `cluster` and `stat_prefix`, resolves the cluster against the manager (error if missing), returns. `Handle` picks an endpoint, `net.Dial`s it (with the cluster's `connect_timeout` if set; see §5.6 for default), and runs the phase-00 pump (netConn-wrap + bidirectional io.Copy + CloseWrite half-close) verbatim.
- **`internal/filter/tcpproxy/filter_test.go`** — unit tests: `NewFilter` happy; `NewFilter` errors on wrong Any type URL; `NewFilter` errors on missing cluster reference; `NewFilter` errors on unresolvable cluster; `Handle` bidirectional byte passthrough against a loopback echo helper (using `test/helpers.TCPRoundTrip` if reusable, or an ad-hoc inline server); `Handle` closes downstream on dial failure.
- **`internal/filter/tcpproxy/fuzz_test.go`** — `FuzzTcpProxyFilter`. Seed corpus: one well-formed `TcpProxy` Any; one malformed Any; one Any with wrong type URL. The fuzz body calls `NewFilter` with the mutated bytes and asserts "no panic, returns either ok or an error beginning `tcpproxy:`". Short-budget `-fuzztime=30s` inherited from ADR-0018 policy.
- **`internal/filter/tcpproxy/doc.go`** — package doc comment; references ADR for the lifted pump.

### 4.2 Changed production code

- **`cmd/envoy-go/main.go`** — rewritten. New flow:
  1. Parse `-c` flag.
  2. Open + `bootstrap.Load` the file.
  3. `cluster.NewManager(bs)` — errors surface as `cluster manager: %v`.
  4. `admin.New(adminAddr)` + `admSrv.Start()`.
  5. `listener.NewManager(bs, clusterMgr)` — builds listeners, resolves filter `typed_config`s against the filter registry, does NOT yet bind sockets.
  6. `lm.Start(ctx)` — binds each listener's TCP socket; returns after all are bound (or an error if any bind fails). Internally launches an Accept loop per listener.
  7. `admSrv.MarkReady()`.
  8. For each `info := range lm.Listeners()`, print `envoy-go listener <name> ready on <addr>\n`. Then print `envoy-go ready\n`.
  9. Block on `<-ctx.Done()` (or on an unexported "exit on SIGINT" signal channel — phase 02 keeps it minimal; `os.Interrupt` triggers `cancel()`, the Accept loops exit, main returns).
  The phase-00 `pump`, `halfClose`, and `netConn` helpers are **removed** from `cmd/envoy-go/main.go`. Their byte-level logic is transplanted to `internal/filter/tcpproxy/filter.go` (ADR records the lift).
- **`cmd/envoy-go/main_test.go`** — updated to launch the subject with a two-listener bootstrap and parse both per-listener ready sentinels + the terminal `envoy-go ready`. Verifies each listener address is dialable (the test binds ephemeral ports via `port_value: 0` and reads the actual addresses from the ready sentinels).
- **`internal/bootstrap/bootstrap.go`** — delete `FirstListenerSocket` and `FirstClusterEndpointSocket`. `Load` and `AdminSocket` are unchanged. The blank-import of `envoy/extensions/filters/network/tcp_proxy/v3` stays (the filter package now also blank-imports it via its direct typed import, but leaving the bootstrap's blank-import in place costs nothing and keeps parse coverage for bootstraps loaded by other tools).
- **`internal/bootstrap/bootstrap_test.go`** — delete `TestFirstListenerSocket_*` and `TestFirstClusterEndpointSocket_*`. Other tests unchanged.

### 4.3 New harness and fixture code

- **`test/differential/harness.go`** — replace the `readyAddr(line string) string` single-listener parser with a `readyListenerAddrs(lines []string) map[string]string` routine that collects every `envoy-go listener <name> ready on <addr>` line until a terminal `envoy-go ready` line is observed, returning the `name → addr` map. Update `StartSubjectProxy` to return a `SubjectProxy` that exposes `ListenerAddr(name string) string` instead of `ListenerAddr() string` (name-free form is deleted).
- **`test/differential/harness_test.go`** — update existing tests to consume the new API (e.g., `sp.ListenerAddr("l_tcp")` instead of `sp.ListenerAddr()`).
- **`test/differential/fixture/driver.go`** (if the interface needs a shape change) — `FixtureDriver` already has `Drive(ctx, refAddr, subjAddr)` which receives the two addresses directly. The runner (`test/differential/runner_test.go`) looks up each side's listener from its driver's `ReferenceListenerPort()` / a new `SubjectListenerName()`. Phase 02 adds `SubjectListenerName() string` to the `FixtureDriver` interface; fixture 0000 returns `"l_tcp"`, fixture 0001 returns `"l_tcp"`.
- **`test/fixtures/0001-tcp-proxy-rr/`** — new fixture directory. Contents:
  - `envoy-go.yaml` — 1 listener (`l_tcp`), 1 cluster (`c_echo`) with `lb_policy: ROUND_ROBIN` and 3 `lb_endpoints`, all loopback 127.0.0.1 with `port_value: 0` placeholders.
  - `envoy.yaml` — reference bootstrap. 1 listener on `0.0.0.0:15000`. 1 STRICT_DNS cluster (with `dns_lookup_family: V4_ONLY` per ADR-0010) containing 3 `lb_endpoints`, each at `host.docker.internal` with a distinct `port_value`. STRICT_DNS rather than STATIC because the reference runs inside a container and must reach host-side backends via `host.docker.internal` (same discipline as fixture 0000). The subject-side config (`envoy-go.yaml`) uses STATIC with three literal `127.0.0.1` endpoints because the subject runs as a host subprocess. This STATIC-vs-STRICT_DNS divergence in *config shape* (not in behaviour) is the same pattern fixture 0000 already carries, and is documented in this fixture's README.
  - `expectations.yaml` — response-body byte-exact; all other dimensions not-applicable (no HTTP, no admin probe beyond the /ready harness gate, no stats, no access log).
  - `README.md` — explains the fixture's purpose (RR over 3 endpoints), the STATIC-vs-STRICT_DNS divergence, and the distribution-assertion methodology (§5.8).
  - `driver/driver.go` — fixture driver. `SubjectConfig` renders the bootstrap with injected ports. Exposes `ReferenceBootstrap() string` with the STRICT_DNS reference config (three backend port placeholders substituted by the runner). `Drive(ctx, refAddr, subjAddr)` sends 9 TCP round-trip requests against each address (`helpers.TCPRoundTrip`), capturing per-backend accept counts from a `[3]atomic.Uint64` tracker (one counter per backend, each incremented by its Accept loop). Returns the concatenated response bytes for differential comparison. The driver also exposes a post-run assertion `AssertDistribution(refCounts, subjCounts [3]uint64) error` that checks each proxy's distribution independently.
- **`test/fixtures/0001-tcp-proxy-rr/driver/driver_test.go`** — a small unit test that exercises the driver in a test-local harness (no testcontainers) to validate the distribution-assertion logic without paying differential-harness startup cost.

### 4.4 Changed documentation and state

- **`docs/envoy-go/ROADMAP.md`** — phase 02 row: `status: in-progress` during work (SPEC stage already satisfies "in-progress"), transitions to `done` at commit.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC written → state 2, PLAN written → state 3, …, phase done → active-phase advances to `03-tls`).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — add new subsection **TCP proxy** covering: (a) response-body byte-exact equivalence on tcp_proxy-terminated connections; (b) LB endpoint-selection ordering is NOT a differential dimension — it is a local correctness property; (c) half-close propagation semantics are preserved from phase 00 (no byte change).
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 02 (numbers assigned at planning/implementation time; the planner may adjust). Anticipated:
  - ADR-A: retire phase-01 first-only extractors; introduce listener/cluster managers. No prior ADR is superseded (the first-only discipline was implicit, not ADR'd). Formalises the post-phase-02 invariant "extractors operate on `*bootstrapv3.Bootstrap` via the managers, not through `internal/bootstrap.First*` helpers."
  - ADR-B: lift phase-00 `netConn`/`pump`/`halfClose` trio verbatim from `cmd/envoy-go/main.go` into `internal/filter/tcpproxy/`. No byte-level semantics change. The splice-avoidance `netConn` wrapper rationale (loopback data loss under `io.Copy` using `splice(2)`) is preserved.
  - ADR-C: per-cluster `atomic.Uint64` counter as the round-robin state scope. Rationale: LB state is per-cluster in Envoy's model; per-listener scope would fragment distribution across listeners sharing a cluster.
  - ADR-D: phase-02 filter-chain subset — exactly one filter_chain per listener, empty `filter_chain_match`, exactly one terminal filter. Supersedes nothing; defers the full match + iteration protocol to phase 07.
  - ADR-E: extend subject ready-sentinel format to one line per listener plus a terminal line. The old single-line `envoy-go ready on <addr>` format is retired. Supersedes the phase-00 sentinel contract (which was codified in `cmd/envoy-go/main.go` comments but not formally ADR'd).
  - ADR-F: fixture `0001-tcp-proxy-rr` uses STRICT_DNS on the reference side and STATIC on the subject side. Same pattern as fixture 0000; distribution of three endpoints does not change the reasoning. Documented for traceability.
  - If additional decisions emerge at plan or implementation time (e.g., a listener-bind error cascade policy, a fuzz-seed-corpus expansion rationale), they are ADR'd at that point. ADRs introduced by this phase should number sequentially from the current highest ADR number at the time of landing; ADR-0022 is the expected start based on the phase-01 ADR log, but the planner verifies at write time.

## 5. Architecture and components

### 5.1 Module graph (new / changed shape)

```
               cmd/envoy-go/main.go
              /        |        \
             /         |         \
bootstrap.Load   admin.Server   listener.Manager
                                        |
                                   filter registry (inline)
                                        |
                               internal/filter/tcpproxy.Filter
                                        |
                                cluster.Manager ──► cluster.Cluster ──► roundRobin LB
                                        ^
                                        |
                                (resolved once at build time,
                                 consulted at Handle time)
```

Imports: `cluster` depends on `github.com/envoyproxy/go-control-plane/envoy/config/{bootstrap,cluster,endpoint}/v3` (proto types only, per D-3.2). `listener` depends on `cluster`, `filter/tcpproxy`, and the proto types. `filter/tcpproxy` depends on `cluster`, `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3`, and `google.golang.org/protobuf/types/known/anypb`. `cmd/envoy-go/main.go` depends on `bootstrap`, `admin`, `listener`, `cluster`.

No package imports `main`. No cyclic imports. The filter registry is an inline map inside `internal/listener/` for phase 02; phase 07 pulls it out.

### 5.2 Listener manager — `internal/listener/`

**Responsibility:** materialise one listener per `static_resources.listeners[]` entry, wire each listener's single filter chain to its terminal filter, bind each listener's TCP socket, and run Accept loops dispatching accepted connections into the filter's `Handle`.

**Public API:**
```go
type Manager struct { /* unexported */ }

func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager) (*Manager, error)

func (m *Manager) Start(ctx context.Context) error
func (m *Manager) Stop()
func (m *Manager) Listeners() []ListenerInfo

type ListenerInfo struct {
    Name string
    Addr string // "host:port" as bound
}
```

**Build-time behaviour (`NewManager`):**

1. Iterate `bs.GetStaticResources().GetListeners()`. Empty slice → error `listener: zero listeners in bootstrap`.
2. For each listener:
   - `name` — required non-empty. Duplicate names error.
   - `address.socket_address` — required; extract `address` and `port_value`. Other address shapes (`pipe`, `envoy_internal_address`) error.
   - `filter_chains` — exactly length 1. Otherwise error `listener %q: expected exactly one filter_chain, got N`.
   - `filter_chain_match` — must be nil or the zero value. A non-zero match errors.
   - `transport_socket` — must be nil. Non-nil errors (TLS = phase 03).
   - `listener_filters` — ignored (skipped silently, per §2).
   - `filters` — exactly length 1. Otherwise error.
   - Filter resolution: look up `filters[0].typed_config.type_url` in the inline filter registry. For phase 02 exactly one URL is registered: `type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy` → `tcpproxy.NewFilter`. Unknown URL errors `listener %q: unknown filter type_url %q`.
   - Call the registered constructor with the Any and the cluster manager. Any construction error surfaces as `listener %q: %w`.
3. Return the built `*Manager` (sockets not yet bound).

**Start behaviour:**

1. For each built listener: `net.Listen("tcp", "<host>:<port>")`. On success, record the `Addr()` into `ListenerInfo.Addr`. On any bind failure, unwind (close all already-bound sockets), return the first error, and leave `Listeners()` empty.
2. Once all listeners are bound, launch one Accept-loop goroutine per listener. Each goroutine accepts until `ctx.Done()` fires (or the listener is closed by `Stop`), dispatching each accepted `net.Conn` into its listener's `filter.Handle(ctx, conn)` (typically via `go filter.Handle(...)`).
3. Return nil (non-blocking — the caller blocks on ctx).

**Stop behaviour:** close every bound listener socket; the Accept-loop goroutines observe the accept error and exit. In-flight `filter.Handle` goroutines are *not* waited on at phase 02 (drain is phase 08).

**Concurrency model:** Accept-per-listener; one goroutine per accepted connection. No connection limits, no back-pressure, no per-listener accept-rate throttle — phase 02 prioritises correctness of the core dataplane over ops-posture features.

### 5.3 Inline filter registry

Phase 02's filter registry is a simple map literal inside `internal/listener/manager.go`:

```go
type filterConstructor func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error)

type filterHandler interface {
    Handle(ctx context.Context, downstream net.Conn)
}

var filterRegistry = map[string]filterConstructor{
    "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy":
        tcpproxyConstructor,
}
```

This is intentionally minimal; phase 07 generalises it into an exported `internal/filter/registry/` package that supports registration by external packages. Phase 02 does not expose the registry — it lives entirely inside `internal/listener/`.

### 5.4 Cluster manager — `internal/cluster/`

**Responsibility:** materialise one cluster per `static_resources.clusters[]` entry and expose cluster lookup by name. Each cluster owns its endpoint list and its load balancer.

**Public API:**
```go
type Manager struct { /* unexported */ }

func NewManager(bs *bootstrapv3.Bootstrap) (*Manager, error)
func (m *Manager) Get(name string) (*Cluster, bool)

type Cluster struct { /* unexported */ }

func (c *Cluster) Name() string
func (c *Cluster) PickEndpoint() (Endpoint, error)
func (c *Cluster) ConnectTimeout() time.Duration

type Endpoint struct {
    Host string
    Port uint32
}
func (e Endpoint) Addr() string // "host:port"
```

**Build-time behaviour (`NewManager`):**

1. Iterate `bs.GetStaticResources().GetClusters()`. Empty slice → error.
2. For each cluster:
   - `name` — required non-empty. Duplicate names error.
   - `type` — must be `STATIC`. Any other value errors with a clear "phase 02 supports only STATIC clusters; got <NAME>" message. `cluster_discovery_type` (oneof for non-STATIC) errors the same way.
   - `lb_policy` — must be unset (proto default `ROUND_ROBIN`) or explicitly `ROUND_ROBIN`. Anything else errors.
   - `load_assignment.endpoints[]` — length ≥ 1 (or the whole `endpoints` slice is required to produce ≥ 1 endpoint across all locality groups).
   - For each `endpoints[i].lb_endpoints[j].endpoint.address.socket_address`: extract `(address, port_value)` as an `Endpoint`. Non-socket_address endpoints error.
   - Total endpoint count across all locality groups must be ≥ 1.
   - `connect_timeout` — parsed (`durationpb.Duration` → `time.Duration`); default 5 seconds if unset (matching Envoy v1.37.2's documented default for STATIC clusters; recorded as a phase-02 constant rather than a cross-project ADR because the choice is local to phase 02 and ADR-worthy only if a later phase needs a different default).
   - Instantiate the load balancer. Phase 02 supports only `roundRobin`. The `lb_policy` enum is the only input; the endpoint slice is closed over by the LB.
3. Return the built `*Manager`.

**Load balancer interface:**
```go
type loadBalancer interface {
    Pick() (Endpoint, error)
}

type roundRobin struct {
    endpoints []Endpoint
    counter   atomic.Uint64
}

func (rr *roundRobin) Pick() (Endpoint, error) {
    if len(rr.endpoints) == 0 {
        return Endpoint{}, errNoEndpoints
    }
    i := rr.counter.Add(1) - 1 // 0, 1, 2, …
    return rr.endpoints[int(i)%len(rr.endpoints)], nil
}
```

Note: the counter-minus-one trick makes the first pick `endpoints[0]` (sequence starts at 0). This is a nice property for unit-test readability; the project's BEHAVIOR_CONTRACT section explicitly does **not** promise this to upstream (upstream's RR is per-worker and may start elsewhere), but the unit test can pin it for the subject.

### 5.5 TCP proxy filter — `internal/filter/tcpproxy/`

**Responsibility:** parse the `TcpProxy` proto from a listener's filter `typed_config`, resolve its `cluster` reference against the cluster manager, and for each accepted downstream connection pick an endpoint, dial, and pump bytes bidirectionally.

**Public API:**
```go
type Filter struct { /* unexported */ }

func NewFilter(tc *anypb.Any, cm *cluster.Manager) (*Filter, error)

func (f *Filter) Handle(ctx context.Context, downstream net.Conn)
```

**Build-time (`NewFilter`):**

1. Validate `tc.GetTypeUrl() == "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"`. Otherwise error `tcpproxy: wrong type_url %q`.
2. `anypb.UnmarshalTo(tc, *tcpproxyv3.TcpProxy)` — error surfaces as `tcpproxy: unmarshal: %w`.
3. Read `cluster` oneof: only `cluster` (string) is supported. `weighted_clusters` errors (phase 02 does not implement weighted cluster dispatch — later phase).
4. Resolve `cluster` name against `cm.Get(clusterName)`. Missing → error `tcpproxy: cluster %q not found`.
5. Read `stat_prefix` (recorded on the filter struct; unused at phase 02).
6. All other `TcpProxy` fields are ignored per §2. This is explicit: the unmarshal succeeds, the fields are carried in the proto struct, envoy-go code simply does not read them.

**Handle behaviour:**

1. Defer-close the downstream `net.Conn`.
2. `ep, err := f.cluster.PickEndpoint()` — on error (none should occur at phase 02 because the cluster was built with ≥1 endpoints), log and return.
3. `upstream, err := net.DialTimeout("tcp", ep.Addr(), f.cluster.ConnectTimeout())` — on error, log `tcpproxy: dial %s: %v` and return (downstream closes on defer).
4. Defer-close the upstream connection.
5. Run the phase-00 pump, verbatim (ADR-B):
   ```go
   var wg sync.WaitGroup
   wg.Add(2)
   go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{downstream}); halfClose(upstream) }()
   go func() { defer wg.Done(); _, _ = io.Copy(netConn{downstream}, netConn{upstream}); halfClose(downstream) }()
   wg.Wait()
   ```
   Where `netConn` wraps `net.Conn` (preventing Linux `splice(2)` optimisation; see phase-00 pump comments) and `halfClose` performs `CloseWrite` on `*net.TCPConn`.

**Concurrency:** one goroutine per accepted connection; `Handle` is not expected to be called concurrently on the same filter instance's listener (accept loop is serial), but the filter struct itself is immutable after construction, and the cluster's `atomic.Uint64` counter makes `PickEndpoint` safe across goroutines.

### 5.6 Bootstrap loader — `internal/bootstrap/`

Unchanged API except for deletion of `FirstListenerSocket` and `FirstClusterEndpointSocket`. `Load` and `AdminSocket` are verbatim. The phase-01 typed_config Any registration (blank import of `…/tcp_proxy/v3`) is preserved; the TCP proxy filter package also imports this type directly, but the blank import in `internal/bootstrap/` costs nothing and keeps `Load` callable standalone with tcp_proxy-bearing bootstraps for future tooling.

### 5.7 `cmd/envoy-go/main.go` — rewired runtime

See §4.2 for the step-by-step flow. Key properties:

- **No direct `net.Listen` or `io.Copy` in `main`.** The dataplane lives entirely in `internal/listener/` and `internal/filter/tcpproxy/`.
- **Admin startup order:** admin starts *before* the listener manager's Accept loops, so `/ready` is reachable for health probes even while listeners are still binding. `MarkReady` fires *after* `lm.Start()` returns successfully. (This mirrors Envoy's init sequence at a gross level; precise ordering semantics are phase 08+.)
- **SIGINT handling:** a single signal handler cancels the top-level context; the listener manager's Accept loops observe the cancellation (via `listener.Close()` side-effect when `Stop` is called from `main`'s post-`<-ctx.Done` cleanup) and exit. In-flight connections are dropped.

### 5.8 Fixture `0001-tcp-proxy-rr` — driver + distribution assertion

**Topology:**

- Three host-side test backends, each a `net.Listen("tcp", "0.0.0.0:0")` + Accept loop that echoes each connection's read bytes and closes.
- Each backend holds its own `atomic.Uint64` accept counter; the driver collects them after each run.
- The reference Envoy container's bootstrap (`envoy.yaml`) declares 3 `lb_endpoints` under one STRICT_DNS cluster with `host.docker.internal` + three distinct `port_value`s. The runner substitutes the three backend ports before starting the container. `dns_lookup_family: V4_ONLY` applies per ADR-0010.
- The subject's bootstrap (`envoy-go.yaml`) declares 3 `lb_endpoints` under one STATIC cluster at `127.0.0.1` + three distinct `port_value`s. The driver's `SubjectConfig` injects these at runtime.

**Drive sequence:**

1. Reset all three backend counters to 0.
2. Drive 9 TCP round-trip requests against the reference listener address (per-request payload: `"ping-<n>-<uid>\n"`, same pattern as fixture 0000). Collect the reference-side per-backend counts `refCounts = [3]uint64{…}` from the backends (each backend saw 3 requests; counter snapshot taken once after all 9 requests complete).
3. Reset all three backend counters to 0.
4. Drive 9 TCP round-trip requests against the subject listener address. Collect `subjCounts`.
5. Concatenate the 9 reference responses and 9 subject responses into `refBytes` / `subjBytes` for the differential diff. Byte-exact equivalence holds trivially because each request is echoed identically by whichever backend serves it.

**Distribution assertion (local, per proxy):**

- For each proxy independently: `forall i: counts[i] == 3` (exact, not tolerance). The `N % 3 == 0` design makes the RR distribution exact.
- Failure reported separately from the differential diff.

**Why not differential distribution:** upstream Envoy's round-robin LB uses per-worker-thread state; the sequence of endpoints selected for N connections is not reproducible across workers or across runs. Asserting sequence-level equivalence would flake. The BEHAVIOR_CONTRACT subsection documents that LB sequence is not a differential dimension; the local distribution check (each proxy balances correctly in its own right) is what the phase commits to.

## 6. Data flow

### 6.1 Startup

```
1. main parses -c flag and opens config file.
2. main calls bootstrap.Load(file) → *bootstrapv3.Bootstrap.
3. main calls bootstrap.AdminSocket(bs) → (host, port).
4. main calls cluster.NewManager(bs) → *cluster.Manager.
   - Each STATIC cluster is materialised.
   - Each cluster owns its LB state (atomic counter at 0).
5. main calls admin.New(adminAddr); admSrv.Start() — admin server bound + serving.
6. main calls listener.NewManager(bs, cm) → *listener.Manager.
   - Each listener is wired to its terminal filter via the inline registry.
   - tcpproxy.NewFilter resolves its cluster reference against cm at build time.
   - No sockets bound yet.
7. main calls lm.Start(ctx).
   - Binds every listener.
   - Launches one Accept goroutine per listener.
   - Returns after all binds succeed (or an error on first failure).
8. main calls admSrv.MarkReady().
9. main prints per-listener ready sentinels + terminal sentinel to stdout.
10. main blocks on <-ctx.Done().
```

### 6.2 Connection

```
1. Accept goroutine for listener L receives net.Conn C.
2. Dispatches via go filter.Handle(ctx, C).
3. filter.Handle:
   - cluster.PickEndpoint() → Endpoint E (atomic.Add-indexed).
   - net.DialTimeout("tcp", E.Addr(), connectTimeout) → U (net.Conn).
   - Two goroutines pump bytes bidirectionally with CloseWrite half-close.
   - On both pumps returning, Handle closes C and U (defer).
```

### 6.3 Shutdown

Phase 02: `os.Interrupt` (SIGINT) cancels ctx; main's deferred `lm.Stop()` closes every listener socket; Accept goroutines exit on the accept error; in-flight `filter.Handle` goroutines complete or are dropped when main returns (process exits). No graceful drain. → phase 08.

## 7. Error handling and failure modes

Single rule: every error crossing a package boundary begins with `<package>: ` (e.g., `listener: `, `cluster: `, `tcpproxy: `). This matches the phase-01 convention (`bootstrap: `) and is enforceable by a lint check in phase 06+ if desired.

| Failure site | Class | Handling |
|---|---|---|
| `cluster.NewManager`: unknown `type`, unknown `lb_policy`, zero endpoints, non-socket_address endpoint, duplicate name | build-time | Return error; `main` logs and exits non-zero. |
| `listener.NewManager`: ≥2 filter_chains, non-empty match, ≥2 filters, populated transport_socket, unknown filter type_url, listener-filter construction error | build-time | Return error; `main` logs and exits non-zero. |
| `tcpproxy.NewFilter`: wrong type_url, unmarshal error, missing cluster, `weighted_clusters` set | build-time | Return error (surfaced through `listener.NewManager`). |
| `listener.Manager.Start`: bind failure | startup | Unwind (close already-bound sockets), return error; `main` logs and exits non-zero. |
| Accept goroutine: accept error | runtime | If listener is closed (expected on Stop), exit the loop cleanly. Otherwise log and continue accepting. |
| `tcpproxy.Filter.Handle`: dial failure | runtime | Log `tcpproxy: dial %s: %v`; close downstream; return. No retry. |
| `tcpproxy.Filter.Handle`: pump error (read/write) | runtime | Silently dropped by `_ = io.Copy(...)` (same as phase 00). Both halves run to completion; CloseWrite half-close is always attempted. |
| `cluster.Cluster.PickEndpoint`: zero endpoints | runtime (should never happen — build-time guarantees ≥1) | Log and return from Handle. |
| SIGINT | shutdown | Cancel ctx; lm.Stop; main returns; process exits 0. |
| Config with `dynamic_resources` or `layered_runtime` | build-time (loader) | Phase-01 behaviour preserved: bootstrap loader errors. |
| Config with TLS (`transport_socket` set) | build-time | Listener manager errors; see §2 note on why this errors rather than being ignored. |

## 8. Testing scope for phase 02

Three layers.

### 8.1 Unit tests

- `internal/listener/manager_test.go` — build-time coverage (§4.1 list), Start + bind-failure paths exercised with ephemeral ports.
- `internal/cluster/manager_test.go` — build-time coverage (§4.1 list), concurrent `Get` (N readers, no writer, no race).
- `internal/cluster/loadbalancer_test.go` — round-robin distribution + concurrency (§4.1 list).
- `internal/filter/tcpproxy/filter_test.go` — `NewFilter` happy/error paths; `Handle` with loopback echo backend; dial-failure path closes downstream.
- `internal/filter/tcpproxy/fuzz_test.go` — `FuzzTcpProxyFilter`, 30s CI budget.
- `cmd/envoy-go/main_test.go` — subject launches with a two-listener bootstrap, both ready sentinels parsed, both listeners dialable (echo round-trip against each).

### 8.2 Fixture-level (differential)

- `test/fixtures/0000-tcp-echo/` — byte-exact echo + byte-exact `/ready`. Driver updated for the new listener-name API. No other change.
- `test/fixtures/0001-tcp-proxy-rr/` — byte-exact response equivalence + local distribution assertion (3/3/3 on each proxy).

### 8.3 Conformance

None for phase 02.

## 9. Out-of-scope (explicitly deferred)

All items in §2 remain deferred. Additionally:

- **Multi-listener / multi-cluster differential fixture.** Unit-tested, not fixture-tested. Adding a fixture is a future phase's call.
- **`tcp_proxy` `idle_timeout`, `max_connect_attempts`, `hash_policy`, `tunneling_config`, `access_log`.** Ignored at phase 02; future phases either implement or error.
- **Listener filter chain (e.g., `tls_inspector`, `original_dst`).** `listener_filters` is skipped silently.
- **`weighted_clusters` in `TcpProxy`.** Build-time error at phase 02. → later phase.
- **Cluster types other than STATIC.** → later phase.
- **LB policies other than ROUND_ROBIN.** → load-balancing family.
- **Upstream TLS.** → phase 03.
- **Graceful drain, SIGTERM, hot restart.** → phase 08 (drain), runtime family (hot restart).

## 10. Deferred decisions (the planner / implementer settles these)

These are intentionally left open for the planning or implementation session to decide. None change the shape of the SPEC; all are implementation-detail choices whose outcome is recorded in PLAN.md or as PROGRESS/ADR notes.

1. **Subpackage layout inside `internal/cluster/`.** Split `loadbalancer.go` into its own subpackage (`internal/cluster/lb`) vs keep inline. Either works; the inline version is simpler for phase 02's one-policy scope. Planner decides; an ADR is not required unless the decision is surprising.
2. **`connect_timeout` default value.** Phase 02 uses 5 seconds when unset. Source: Envoy v1.37.2 documentation defaults. If the planner finds a different authoritative default, adjust and note in PLAN.md (no ADR needed unless it diverges from upstream).
3. **Whether `listener.Manager.Stop` is implemented at phase 02.** `main` may exit without calling Stop (process exit cleans the sockets). Implementing Stop is cheap and lets tests shut the manager down cleanly — expected yes, but the planner decides whether Stop is tested or stubbed.
4. **Fuzz seed corpus size beyond the three initial entries.** Adding 2–3 more seeds (e.g., a well-formed Any with a larger weighted_clusters structure that the filter rejects) is cheap coverage; optional.
5. **Whether the ready-sentinel change retains any backward-compat marker.** A simple approach: new format only; harness updates. Alternative: emit both the new per-listener lines and a legacy single-line for the first listener. The SPEC recommends the clean-break approach (pure new format), but the planner may opt for transitional compatibility if there is a reason.
6. **Fixture `0001-tcp-proxy-rr` naming.** Phase 02's `0001` is the next available fixture-id after phase-00/01's `0000`. If a later phase introduces a parallel fixture-numbering scheme, the number may change; the planner verifies at PLAN time.
7. **Per-listener vs per-Accept-loop `ctx` cancellation wiring.** Either the listener manager shares a single ctx across all Accept loops, or derives per-listener child contexts. The simpler shared-ctx approach is recommended; planner decides.
8. **Whether `stat_prefix` is stored on the Filter struct (unused) or simply discarded at parse time.** Either works; storing it is forward-looking for phase 06 (stats) and costs one string field. Recommend storing.
9. **ADR numbering.** The SPEC lists anticipated ADRs A–F with explicit purposes. At landing time, the planner assigns sequential numbers starting from the current highest ADR + 1.

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Listener bind flake on CI (ephemeral port collision). | Every fixture and test uses `port_value: 0` (OS-assigned); ready sentinels report actual bound addresses. No hard-coded ports on the subject side. Reference container uses fixed in-container ports (15000 TCP, 9901 admin) mapped to host-ephemeral by testcontainers, same pattern as phase 01. |
| RR distribution flakes due to connection coalescing / keep-alive reuse. | Each TCP round-trip request opens its own connection (explicit `net.Dial` + write + read + close in `helpers.TCPRoundTrip`). No keep-alive. The distribution is exact (3/3/3 over 9 requests), not statistical. |
| Accept goroutine leak when listener fails to bind mid-Start. | `Start` unwinds: close already-bound sockets and return; never launches Accept loops until *all* binds succeed. |
| `io.Copy` over splice silently truncating under half-close (phase-00 hazard). | Preserved mitigation: `netConn{}` wrapper + verbatim pump code. No byte-level change. |
| Concurrent `PickEndpoint` producing non-uniform distribution under high load. | The `atomic.Uint64` counter is exact; mod-indexing is deterministic. Unit test covers 100 goroutines × 30 picks each = 3000 picks, asserts distribution within ±2 of 1000 per endpoint (tight). |
| `TcpProxy` Any with adversarial bytes crashing the filter. | `FuzzTcpProxyFilter` covers the unmarshal path. Handle is not fuzzed (it's a network path; phase 07+ may add network-level fuzz). |
| Fixture 0001 distribution assertion flakes when harness run order is noisy. | Counter snapshot is taken once, after all 9 requests complete, and all 9 complete synchronously (`TCPRoundTrip` is blocking). No race between request and counter sample. |
| Ready-sentinel format change breaking forgotten call sites. | A `grep` for `envoy-go ready on` (old format) across the repo is part of the phase-02 verification step. The harness is the only known consumer. |

## 12. Acceptance checklist (for the reviewer of this phase's final state)

- [ ] `cmd/envoy-go/main.go` contains no direct `net.Listen`, `Accept`, `io.Copy`, or pump helpers. Wiring only.
- [ ] `internal/listener/manager.go` exists and builds. Unit tests pass. Build-time errors match §7.
- [ ] `internal/cluster/manager.go` + `cluster.go` + `loadbalancer.go` exist and build. Unit tests pass.
- [ ] `internal/filter/tcpproxy/filter.go` exists, pump code is verbatim from phase 00 (diff-able by inspection; ADR-B references this lift).
- [ ] `internal/filter/tcpproxy/fuzz_test.go` exists; `FuzzTcpProxyFilter` runs clean on CI short budget.
- [ ] `internal/bootstrap.FirstListenerSocket` and `FirstClusterEndpointSocket` are deleted. `AdminSocket` and `Load` unchanged. Tests for deleted functions deleted.
- [ ] `test/differential/harness.go` parses per-listener ready sentinels and exposes `ListenerAddr(name)`. Legacy single-line parser is deleted.
- [ ] Fixture `0000-tcp-echo/driver/driver.go` calls the new `ListenerAddr("l_tcp")` form.
- [ ] Fixture `0001-tcp-proxy-rr/` exists with `envoy-go.yaml`, `envoy.yaml`, `expectations.yaml`, `README.md`, and `driver/driver.go`. The fixture's differential gate is green.
- [ ] `BEHAVIOR_CONTRACT.md` contains a new TCP proxy subsection that codifies response-body byte equivalence and explicitly disclaims LB sequence as a differential dimension.
- [ ] `DECISIONS.md` contains ADRs A–F (with actual sequential numbers assigned at landing). Each ADR names doctrine citations and consequences.
- [ ] `ROADMAP.md` row for phase 02 is `status: done` at commit time.
- [ ] `STATE.md` advances to phase 03 with `lifecycle-state: 1` / `next-skill: superpowers:brainstorming` at commit time.
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all clean (captured in PROGRESS.md per §7.5(e)).
- [ ] Commit message follows §5.3 format and references the ADRs introduced or referenced.
