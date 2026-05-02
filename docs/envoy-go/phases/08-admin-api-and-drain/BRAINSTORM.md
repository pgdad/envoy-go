# Phase 08 Brainstorm — Admin API and Graceful Drain

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-1 brainstorm session for phase 08 (`admin-api-and-drain`). The next session (lifecycle-state 1 → 2 for sub-phase 08.1, skill `superpowers:writing-plans`) authors `SPEC.md` for phase **08.1** based on this brainstorm. Phase 08.2 receives a sibling SPEC stub at the same time; its full design is deferred to the 08.2 lifecycle-state 1 brainstorm session per the 06.2 / 07.2 cascade pattern.

**Brainstorm session:** worktree `.worktrees/phase-08-admin-api-and-drain-brainstorm`, branch `phase-08-admin-api-and-drain-brainstorm`, branched from master tip `f3835a5` (phase-07.2 phase-done SHA-fill commit).

**Brainstorm mode:** autonomous per ADR-0004 (no live human-in-the-loop). Decisions are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, prior ADRs, and observed Envoy v1.37.2 contract behavior. Empirical pins requiring scrape evidence are explicitly enumerated in §2.7 and deferred to SPEC-drafting time.

---

## 1. Scope decision — planner-time split

ROADMAP row 08 bundles two architecturally distinct deliverables: **(i) a minimum read-only admin API surface (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) extending the existing admin server beyond `/ready` (phase 01) and `/stats/prometheus` (phase 06.1)**, and **(ii) graceful-drain semantics (SIGTERM-triggered drain, `POST /drain_listeners`, listener stop-accepting + in-flight-completion, `/ready` and `/server_info` state transitions)**. Per ADR-0045 (planner-time split) and the 06 → 06.1/06.2 + 07 → 07.1/07.2 precedent, phase 08 splits at brainstorm time into two sub-phases:

| ID | Title | Scope | Differential surface |
|---|---|---|---|
| **08.1** | `admin-endpoints` | Four new read-only admin handlers under the existing `internal/admin/` HTTP/1.1 mux: `GET /config_dump` (application/json via protojson), `GET /clusters` (text/plain), `GET /listeners` (text/plain), `GET /server_info` (application/json); `BEHAVIOR_CONTRACT.md` `## Admin API` umbrella section restructured with per-endpoint subsections (the existing `## Admin API — /ready` becomes `## Admin API` with `### /ready`, `### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info` subsections). | fixture `0009-admin-config-dump` (differential, scrape equivalence on the four new endpoint shapes) |
| **08.2** | `graceful-drain` | New `internal/drain/` package (drain-manager state machine: LIVE → DRAINING → exit); `cmd/envoy-go/main.go` SIGTERM handler upgrade (current SIGTERM path just cancels ctx and calls `lm.Stop`; new path threads through the drain manager); `POST /drain_listeners` admin endpoint (drain-without-exit); `/ready` extension to emit DRAINING-state body when draining (extends ADR-0015's pre-init contract); `/server_info` `state` field reflects drain transitions; `internal/listener.Manager.Drain` (stop-accepting on listening sockets); `internal/cluster.Manager.Drain` (best-effort upstream connection close after drain timeout); listener-side new-connection rejection during drain. | fixture `0010-graceful-drain` (TBD when 08.2 brainstorms — likely in-flight-request-completes-while-new-conns-rejected scenario plus `/ready` and `/server_info` state-transition observation) |

**Rationale for split:**

- 08.1 lives entirely in `internal/admin/` plus a small set of accessor methods on `internal/listener.Manager`, `internal/cluster.Manager`, and `internal/bootstrap`. Net change ~600–800 LOC + ~200–300 LOC fixture/driver. No change to the request hot path. No new package outside `internal/admin/`. Read-only surface — no mutation, no new lifecycle state.
- 08.2 introduces a new `internal/drain/` package with a state machine, mutates the request hot path (listener Accept loop must consult drain state; HCM dispatch must signal in-flight completion to the drain manager), upgrades the SIGTERM handler in `cmd/envoy-go/main.go`, extends two admin endpoints (`/ready`, `/server_info`), adds one new mutating admin endpoint (`POST /drain_listeners`), and adds new fields to `BEHAVIOR_CONTRACT.md`'s admin section. Net change ~500–800 LOC + ~200–300 LOC fixture/driver. Multiple ADRs anticipated (drain state machine, SIGTERM-vs-SIGINT semantics, `/ready` DRAINING-state contract extending ADR-0015, drain timeout default, hot-restart deferral).
- Combined: ~1100–1600 LOC production + ~400–600 LOC fixture/driver. Combined task count estimated at 28–38 across both sub-phases. **Crosses both ADR-0045 thresholds** (~25 tasks, ~1500 LOC).
- The two sub-phases share no production-code surface except the `internal/admin` HTTP/1.1 mux scaffold (the same shared scaffold that 06.1 extended via `mux.HandleFunc("/stats/prometheus", ...)` — this is mux registration, not code-sharing). 08.1 ships a stable admin-mux extension pattern that 08.2 then uses to register `POST /drain_listeners` and to extend the existing `/ready` and `/server_info` handlers.
- 08.1 and 08.2 are sequentially dependent: 08.2's `/ready` DRAINING-state extension and `/server_info` state-field transition both consume the admin-endpoint scaffold that 08.1 lands. **08.1 ships first.** ROADMAP row 08.2 is `planned` until 08.1's phase-done commit; depends-on `08.1`.
- Bundling them in one phase risks the same SPEC-bloat that drove the 06.1/06.2 and 07.1/07.2 splits. The phase-07 BRAINSTORM (§1) called out "bundling them into one phase risks bloating the SPEC" as the primary rationale; the same argument applies symmetrically here.

The phase-08 parent directory (`docs/envoy-go/phases/08-admin-api-and-drain/`) carries this BRAINSTORM.md plus a master SPEC.md (eventually) summarizing both sub-phases. Mirrors `docs/envoy-go/phases/06-observability-baseline/` and `docs/envoy-go/phases/07-filter-chain-framework/`.

---

## 2. Phase 08.1 — design decisions

### 2.1 Admin endpoint surface scope *(Decision A → ADR-NNNN)*

**Decision:** Ship exactly four new endpoints in 08.1, all read-only (`GET`-only, no mutating semantics, no query-param filtering):

| Endpoint | Method | Content-Type | Source of truth |
|---|---|---|---|
| `/config_dump` | GET | `application/json` | parsed bootstrap proto (`bs.Proto`) wrapped in `envoy.admin.v3.BootstrapConfigDump`, `envoy.admin.v3.ListenersConfigDump`, `envoy.admin.v3.ClustersConfigDump` envelopes |
| `/clusters` | GET | `text/plain; charset=UTF-8` | `internal/cluster.Manager` walked at request time |
| `/listeners` | GET | `text/plain; charset=UTF-8` | `internal/listener.Manager` walked at request time |
| `/server_info` | GET | `application/json` | composed at request time from build metadata + `internal/admin.Server` uptime + `LIVE` state + `bs.Proto.Node` |

Existing endpoints unchanged in 08.1: `/ready` (phase 01) stays at the LIVE / PRE_INITIALIZING contract; `/stats/prometheus` (phase 06.1) stays at the Prometheus text exposition contract. The `/ready` DRAINING-state extension and the `/server_info` `state`-field transition during drain are **08.2's** deliverables — the 08.1 `/server_info` `state` field is hardcoded to `"LIVE"` once `MarkReady()` has fired (mirrors the 08.1 admin server's existing readiness gate).

**Explicitly not in 08.1** (per ADR-0040 deferral discipline):

| Deferred surface | Reason | Target |
|---|---|---|
| `?format=json` query-param on `/clusters` and `/listeners` | Envoy supports both text and JSON forms; shipping one cuts the design surface in half. Text is simpler to byte-compare against Envoy and matches the canonical `curl` operator workflow. | future admin-extensions phase or family promotion |
| `?resource=...` filtering on `/config_dump` | Filtering implies query-param parsing + sub-tree extraction; defer until a consumer needs it | future admin-extensions phase |
| `?include_eds=true` on `/config_dump` | EDS configurations are xDS-family; no static EDS in current bootstrap shape | xDS family |
| `?mask=...` on `/config_dump` | Field-mask filtering; defer | future admin-extensions phase |
| `RoutesConfigDump` envelope in `/config_dump` | Routes live inside HCM `typed_config`; Envoy emits them as a dedicated `RoutesConfigDump` envelope for `/config_dump`. The static routes ARE present in the bootstrap proto already (inlined in HCM filter configs); 08.1 does not separately enumerate them. Promotion lands when the differential fixture demands it (likely a follow-up minor) | follow-up minor or future admin-extensions phase |
| `SecretsConfigDump` envelope | Requires secret redaction discipline (passwords, private keys) — non-trivial security surface | future admin-extensions or secrets-family phase |
| `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes | xDS-only surfaces | xDS family |
| `POST /reset_counters` | Mutating; not needed for read-only fixture | future admin-extensions phase |
| `/runtime`, `POST /runtime_modify` | RTDS family | RTDS / runtime family |
| `/certs` | Cert-lifecycle introspection; non-trivial | secrets / cert-lifecycle family |
| `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention` | Operational profiling endpoints; non-MVP | future ops-endpoints phase |
| `POST /quitquitquit` | Hard-shutdown trigger; semantically overlaps `/drain_listeners` + SIGTERM | 08.2 to evaluate; may be deferred further |
| `POST /healthcheck/fail`, `POST /healthcheck/ok` | Active-healthcheck-toggle endpoints; require active health checking which is a separate family | upstream-robustness family |
| `/logging`, `POST /logging?level=...` | Log-level control; non-MVP | future ops-endpoints phase |
| `POST /reopen_logs` | Log-rotation signal; non-MVP | future ops-endpoints phase |
| `/listeners/<name>/...` per-listener admin sub-routes (drain individual listeners, etc.) | Per-listener selective drain; non-MVP | 08.2 to evaluate; likely defer further |
| `/init_dump` | Envoy-init-machinery introspection; envoy-go has no equivalent init manager | likely never |

**Rationale for the four-endpoint MVP:**

- The seeded ROADMAP-row summary lists `config_dump, stats, clusters, listeners, ready, server_info`. Of those six, two are already shipped (`stats` at 06.1, `ready` at 01); the remaining four are 08.1's MVP. The summary is exhaustive — no endpoint outside the six is in scope.
- All four are read-only, which means no new state machine, no new mutation surface, no new ACL discussion (out of scope per Decision E). The mutating surface (`POST /drain_listeners`) is 08.2's deliverable.
- Each of the four has a clear data source already present in the codebase: bootstrap proto (parsed at boot), cluster.Manager (built at boot), listener.Manager (built at boot), build metadata + uptime (composable at request time). No new data plumbing required for 08.1.
- The differential fixture surface is bounded: a single multi-listener / multi-cluster bootstrap, four scrapes, four equivalence claims (with a tolerance discipline for hot-path counters in `/clusters` — see §2.6).

### 2.2 Admin server transport — reuse phase-01 HTTP/1.1 mux *(Decision B → ADR-NNNN)*

**Decision:** All four new handlers register on the existing `*http.ServeMux` allocated by `internal/admin.Server.Start()` (current line `internal/admin/admin.go:47`). No new HTTP server, no new bind, no new transport. Admin server stays HTTP/1.1 — the upstream Envoy admin server is also HTTP/1.1 by default at v1.37.2 (HTTP/2 over admin requires explicit configuration; out of scope).

The new mux registrations (08.1 lifecycle-state 3 implementation):

```go
// internal/admin/admin.go Start() body, after the existing two registrations:
mux.HandleFunc("/config_dump",  s.handleConfigDump)
mux.HandleFunc("/clusters",     s.handleClusters)
mux.HandleFunc("/listeners",    s.handleListeners)
mux.HandleFunc("/server_info",  s.handleServerInfo)
```

**Server constructor signature widens** to thread the additional dependencies needed by the new handlers:

```go
// internal/admin/admin.go — current:
func New(addr string, registry *stats.Registry) *Server

// internal/admin/admin.go — 08.1:
func New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server
```

The `*bootstrap.Bootstrap` parameter is needed for `/config_dump` (the parsed proto is the source of truth) and for `/server_info` (the `Node` field is sourced from `bs.Proto.GetNode()`). The `*cluster.Manager` is needed for `/clusters`. The `*listener.Manager` is needed for `/listeners`.

The constructor-widening pattern mirrors the 07.1 hand-off: every constructor that grows new dependencies takes them explicitly (no package globals; mirrors the LBP-1 discipline of `*stats.Registry` per phase 06.1 SPEC §5.4). The `cmd/envoy-go/main.go` call site updates with one line.

**Listener-manager and cluster-manager extension:**

```go
// internal/listener/manager.go — new:
func (m *Manager) Listeners() []ListenerInfo  // returns name + addr + state per listener
//   note: ListenerInfo{Name, Addr} already exists (see cmd/envoy-go/main.go:147 — `lm.Listeners()` is already wired);
//   08.1 may extend it with per-listener state fields (active conn count, accepted conn count) read from the
//   atomic counters already maintained by phase-06.1 stats per `listener.<addr>.downstream_cx_active`.

// internal/cluster/manager.go — new:
func (m *Manager) Clusters() []ClusterInfo  // returns name + endpoints + per-endpoint stats
//   note: cluster.Manager already exposes a `clusters map[string]*Cluster` internally;
//   08.1 adds a public read-only accessor returning a snapshot.
```

The accessor methods are read-only snapshots; no locking discipline new (the underlying maps are immutable post-boot per ADR-0045's planner-time-split + the boot-freeze pattern). Per-endpoint stats counters are read via `atomic.LoadInt64` from the existing `*stats.Counter` / `*stats.Gauge` instances allocated at phase 06.1.

**Why not a new admin-only HTTP server:** the existing one has a working bind, a working `MarkReady()` gate, working request/response timeouts (`5 * time.Second`), and is already integrated into the lifecycle (started before listeners; closed at shutdown). Splitting into a new server would duplicate all of that for zero benefit.

### 2.3 `/config_dump` body shape *(Decision C → ADR-NNNN)*

**Decision:** Body is `application/json` produced via `google.golang.org/protobuf/encoding/protojson` over a manually-assembled `*adminv3.ConfigDump` envelope containing exactly three sub-envelopes in this order: `BootstrapConfigDump`, `ListenersConfigDump`, `ClustersConfigDump`. All three are populated from static config only — no dynamic_* arrays (xDS not yet present).

**Envelope construction:**

```go
// internal/admin/configdump.go (sketch — actual code in 08.1 impl)

import (
    adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
    bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
    listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
    clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
    "google.golang.org/protobuf/encoding/protojson"
    "google.golang.org/protobuf/types/known/anypb"
)

func (s *Server) buildConfigDump() (*adminv3.ConfigDump, error) {
    bootDump := &adminv3.BootstrapConfigDump{
        Bootstrap:   s.bs.Proto,                          // the loaded *bootstrapv3.Bootstrap
        LastUpdated: timestamppb.New(s.bootTime),         // monotonic-clock at admin.New time
    }
    lisDump := &adminv3.ListenersConfigDump{
        VersionInfo:    "static",  // empirical-pin: verify Envoy emits "static" for static_resources
        StaticListeners: enumerateStaticListeners(s.bs.Proto),
    }
    cluDump := &adminv3.ClustersConfigDump{
        VersionInfo:    "static",
        StaticClusters: enumerateStaticClusters(s.bs.Proto),
    }
    bootAny, _ := anypb.New(bootDump)
    lisAny, _ := anypb.New(lisDump)
    cluAny, _ := anypb.New(cluDump)
    return &adminv3.ConfigDump{Configs: []*anypb.Any{bootAny, lisAny, cluAny}}, nil
}

func (s *Server) handleConfigDump(w http.ResponseWriter, r *http.Request) {
    cd, err := s.buildConfigDump()
    if err != nil { /* synthesize 500 */ }
    body, err := protojson.MarshalOptions{
        Multiline:       true,
        Indent:          "  ",     // empirical-pin: verify Envoy uses 2-space indent
        UseProtoNames:   false,    // empirical-pin: verify camelCase vs snake_case
        EmitUnpopulated: false,    // empirical-pin: verify Envoy omits unset fields
    }.Marshal(cd)
    if err != nil { /* synthesize 500 */ }
    h := w.Header()
    h.Set("Content-Type", "application/json")
    h.Set("Cache-Control", "no-cache, max-age=0")
    h.Set("X-Content-Type-Options", "nosniff")
    h.Set("Server", "envoy")
    h.Set("Content-Length", strconv.Itoa(len(body)))
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write(body)
}
```

**Static-only:** the three envelopes' `dynamic_listeners`, `dynamic_active_clusters`, `dynamic_warming_clusters`, `dynamic_drained_clusters` arrays are left empty (zero-valued); Envoy's protojson will serialize them as `[]` only if present in the proto, and protojson's default `EmitUnpopulated: false` will omit them. Empirical-pin obligation: verify upstream Envoy at v1.37.2 with a static-only bootstrap also omits the dynamic_* arrays vs. emits empty `[]`.

**Why protojson, not custom JSON:** the Envoy admin proto types (`envoy.admin.v3.*`) are the canonical schema. Hand-rolling JSON would either duplicate the entire schema or risk drift. protojson is the standard library marshaler; its output is byte-stable per call (no map iteration). The marshaler options are tunable to match Envoy's choice (the empirical-pin obligations in §2.7 settle the option values).

**Why `MarshalOptions` and not `(*ConfigDump).MarshalJSON`:** the latter requires implementing a custom marshaler; protojson's `MarshalOptions` covers the same ground with explicit knobs.

### 2.4 `/clusters` and `/listeners` body shape *(Decision D → ADR-NNNN)*

**Decision:** Both endpoints emit `text/plain; charset=UTF-8`. Format mirrors Envoy v1.37.2's text mode (NOT the JSON mode). Each endpoint's exact byte format requires empirical-pin scrape evidence at SPEC time (§2.7 #2 + #3); the brainstorm fixes the structural shape but defers the verbatim format.

**`/clusters` structural shape (Envoy v1.37.2, text mode — verified at SPEC time):**

```
<cluster_name>::observability_name::<obs>
<cluster_name>::default_balancer_context::<ctx>
<cluster_name>::added_via_api::<bool>
<cluster_name>::eds_service_name::<eds>           (omitted if EDS not configured)
<cluster_name>::<endpoint_addr>::cx_active::<n>
<cluster_name>::<endpoint_addr>::cx_connect_fail::<n>
<cluster_name>::<endpoint_addr>::cx_total::<n>
<cluster_name>::<endpoint_addr>::rq_active::<n>
<cluster_name>::<endpoint_addr>::rq_error::<n>
<cluster_name>::<endpoint_addr>::rq_success::<n>
<cluster_name>::<endpoint_addr>::rq_timeout::<n>
<cluster_name>::<endpoint_addr>::rq_total::<n>
<cluster_name>::<endpoint_addr>::hostname::<host>
<cluster_name>::<endpoint_addr>::health_flags::<flags>
<cluster_name>::<endpoint_addr>::weight::<w>
<cluster_name>::<endpoint_addr>::region::<r>
<cluster_name>::<endpoint_addr>::zone::<z>
<cluster_name>::<endpoint_addr>::sub_zone::<sz>
<cluster_name>::<endpoint_addr>::canary::<bool>
<cluster_name>::<endpoint_addr>::priority::<p>
<cluster_name>::<endpoint_addr>::success_rate::<f>
<cluster_name>::<endpoint_addr>::local_origin_success_rate::<f>
```

The structural layout is one line per `<cluster>::<key>::<value>` triple plus one block per endpoint. Many fields envoy-go does not yet emit (active health checking is upstream-robustness family; canary / region / zone / sub_zone are LB-extensions family); 08.1 emits the structurally-required minimum and zero-fills the rest **OR** omits the unsupported lines entirely. The latter is the cleaner choice (Envoy itself omits lines whose source values are not present); the empirical-pin scrape at SPEC time settles which lines must appear unconditionally.

**MVP-emitted `/clusters` lines (08.1 minimum):** per-cluster: `observability_name`, `added_via_api: false`. Per-endpoint (one block per endpoint): `cx_active`, `cx_total`, `rq_total`, `health_flags::healthy` (hardcoded since no active health checking), `weight::1` (default), `priority::0` (default). Counters are read from existing `*stats.Counter` / `*stats.Gauge` instances per cluster (`cluster.<n>.upstream_cx_active`, `cluster.<n>.upstream_cx_total`, `cluster.<n>.upstream_rq_total` from phase 06.1's catalog).

The remainder of Envoy's text output (`region`, `zone`, `sub_zone`, `canary`, `success_rate`, `local_origin_success_rate`, `cx_connect_fail`, `rq_active`, `rq_error`, `rq_success`, `rq_timeout`, `hostname`) is **deferred** — those values either don't exist in envoy-go's current state (no active health checking, no per-endpoint locality tags, no per-endpoint outlier tracking) or are derivable but not worth emitting without a consumer. The differential equivalence claim accommodates this via a tolerant comparison (§2.6).

**`/listeners` structural shape (Envoy v1.37.2, text mode — verified at SPEC time):**

```
<listener_name>::<bind_addr>
```

One line per listener. The `<listener_name>` is the bootstrap-configured name (e.g., `l_test_a`); `<bind_addr>` is `host:port` per Envoy's normalized form. Envoy may emit additional fields (active conn count, drain state) — empirical-pin obligation at SPEC time.

**Why text mode, not JSON:** the JSON form (Envoy supports `?format=json` on both endpoints) is structurally richer but doubles the differential surface (each endpoint has two byte-distinct outputs to byte-compare against). Shipping only the text form keeps the differential fixture tractable. The JSON form is deferred per ADR-0040.

### 2.5 `/server_info` body shape *(Decision E → ADR-NNNN)*

**Decision:** Body is `application/json` produced via `protojson` over a manually-assembled `*adminv3.ServerInfo` proto. Field set MVP — only the fields envoy-go can credibly populate; the rest are zero-valued or omitted per protojson's `EmitUnpopulated: false`.

**MVP-populated fields:**

| Field | Source | Notes |
|---|---|---|
| `version` | `runtime.Version()` + envoy-go build SHA (compile-time) | format: `"envoy-go-<git-sha-short> Go-<runtime>"`; exact format is a pin obligation at SPEC time (Envoy emits a more elaborate string with build-mode + revision-hash) |
| `state` | `"LIVE"` (hardcoded post-MarkReady; PRE_INITIALIZING pre-MarkReady) | `state` enum values per Envoy v1.37.2: `LIVE`, `DRAINING`, `PRE_INITIALIZING`, `INITIALIZING`. 08.1 emits `LIVE` post-MarkReady and `PRE_INITIALIZING` pre-MarkReady. `DRAINING` is 08.2's deliverable. `INITIALIZING` is not modeled (envoy-go boot is synchronous; there is no extended init phase). |
| `uptime_current_epoch_seconds` | wall-clock time since `admin.New()` | `Duration` type per protojson; emitted as JSON number of seconds with fractional part (verify at SPEC) |
| `uptime_all_epochs_seconds` | identical to `uptime_current_epoch_seconds` (no hot restart, no parent-epoch concept) | Envoy's ServerInfo has both fields; envoy-go zero-knowledge-of-parent-epochs means both fields are equal until the runtime/hot-restart family lands |
| `node` | `bs.Proto.GetNode()` (the parsed bootstrap node) | `envoy.config.core.v3.Node` proto inlined as JSON; if `node:` is empty in the bootstrap, the field is omitted |
| `command_line_options` | `nil` / omitted | Envoy emits the parsed CLI flags here; envoy-go's CLI is minimal (`-c <path>`, `--allow-h2c`); 08.1 emits an empty `CommandLineOptions{}` proto OR omits the field entirely (empirical-pin: which does Envoy do when no CLI flags differ from defaults?) |
| `hot_restart_version` | `"disabled"` or omitted | hot restart is out of scope; emit a static placeholder string OR omit per Envoy's empirical behavior |

**Explicitly out of MVP** (fields Envoy emits that envoy-go does not populate):

- `runtime` (RuntimeOverview) — RTDS family
- Build metadata fields beyond the version string (e.g., `build_label`, `revision_status`) — could be added later
- The `command_line_options` sub-fields (`base_id`, `concurrency`, `service_cluster`, `service_node`, etc.) — most are Envoy-specific lifecycle knobs that envoy-go has no analog for

**Why protojson, not custom JSON:** same rationale as `/config_dump` (§2.3). Schema fidelity beats hand-rolling.

### 2.6 Differential fixture shape — `0009-admin-config-dump`

**Fixture goal:** a single bootstrap that exercises all four new endpoints under a controlled load, scraped from envoy-go and reference Envoy, with per-endpoint equivalence claims matched to each endpoint's tolerance profile.

**Bootstrap shape:**

```yaml
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
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: {socket_address: {address: 127.0.0.1, port_value: 18001}}
              - endpoint:
                  address: {socket_address: {address: 127.0.0.1, port_value: 18002}}
```

**Driver flow:**

1. Boot envoy-go on admin port 9901 + reference Envoy on admin port 9902 with above bootstrap (admin port differs to avoid bind conflict; backend ports identical).
2. Boot a controlled HTTP backend on ports 18001 + 18002 (returns 200 OK with static body).
3. Issue a defined load: 5 GET requests through the proxy listener (port 10000) to drive at least one connection + one request through the cluster machinery, populating `cluster.c_backend.upstream_cx_total = 1`, `cluster.c_backend.upstream_rq_total = 5`.
4. Scrape four endpoints against both proxies:
   - `GET http://127.0.0.1:9901/config_dump` vs `GET http://127.0.0.1:9902/config_dump`
   - `GET http://127.0.0.1:9901/clusters` vs `GET http://127.0.0.1:9902/clusters`
   - `GET http://127.0.0.1:9901/listeners` vs `GET http://127.0.0.1:9902/listeners`
   - `GET http://127.0.0.1:9901/server_info` vs `GET http://127.0.0.1:9902/server_info`
5. Per-endpoint equivalence:
   - **`/config_dump`:** body byte-equal modulo (a) the `version` field in `BootstrapConfigDump.bootstrap.node.user_agent_build_version` (envoy-go-vs-Envoy build metadata divergence; documented in BEHAVIOR_CONTRACT allow-list); (b) the `last_updated` timestamp (per-boot non-deterministic); (c) hash-set ordering inside `Any.value` (the protojson output ordering is canonical — verify); (d) the `dynamic_*` arrays which both omit. Date / Server / Content-Length headers in the differential ignore-list per existing BEHAVIOR_CONTRACT discipline.
   - **`/clusters`:** structural-line set equality. Each line is parsed into `(cluster_name, key, value)` tuples; the value-set is compared with a tolerance for hot-path counters (`cx_total`, `rq_total`) which may differ by 1 due to admin-port scrape racing the request-path counter increment. The tolerance is bounded (`abs(envoy_go - envoy) <= 1`). MVP-deferred lines (locality tags, success rates) are present in Envoy's output but absent in envoy-go's; the comparator allows envoy-go's set to be a subset of Envoy's set (modulo the tolerance for the present counters).
   - **`/listeners`:** body byte-equal (single line per listener; deterministic).
   - **`/server_info`:** body byte-equal modulo (a) `version` (envoy-go-vs-Envoy build divergence; allow-listed); (b) `uptime_current_epoch_seconds` and `uptime_all_epochs_seconds` (per-boot non-deterministic; allow-listed); (c) `command_line_options` (envoy-go has fewer fields populated; tolerant comparator allows superset on Envoy side); (d) `hot_restart_version` (envoy-go emits placeholder; allow-listed).

**Equivalence claim** is registered as `RequiresReference: true` in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors 0007a-cors).

**Why a tolerant comparator:** the four endpoints span a spectrum from "deterministic config restatement" (`/config_dump`, `/listeners`) to "live-counter snapshot" (`/clusters`) to "lifecycle-state composite" (`/server_info`). A single byte-equal claim across all four would either (a) require envoy-go to emit Envoy-superset-equivalent output for fields it doesn't model (locality tags, success rates), or (b) require Envoy to be silenced for fields it emits unconditionally (uptime, build metadata). Neither is a useful definition of equivalence. The tolerance discipline is per-field, codified in BEHAVIOR_CONTRACT.md.

### 2.7 Empirical-pin obligations *(SPEC-author work)*

The brainstorm explicitly does NOT settle these; they require an empirical scrape against Envoy v1.37.2 at SPEC-drafting time per ADR-0004's hard-gate discipline (mirrors phase 07.1 BRAINSTORM §2.6 and phase 06.1 BRAINSTORM §2.3.1):

1. **`/config_dump` JSON shape:** verify protojson `MarshalOptions` settings that match Envoy's output verbatim — `Multiline: true` vs. compact one-line; `Indent: "  "` (2-space) vs. `"    "` (4-space); `UseProtoNames: false` (camelCase) vs. `true` (snake_case); `EmitUnpopulated: false` (omit zero-valued) vs. `true` (emit all). Boot Envoy with the §2.6 fixture bootstrap, scrape `/config_dump`, copy first 50 lines verbatim, pin in BEHAVIOR_CONTRACT.

2. **`/clusters` text format exact byte layout:** which lines does Envoy emit unconditionally vs. conditionally on a feature being configured. Boot Envoy with the §2.6 fixture, scrape `/clusters`, capture the full output (all `<cluster>::<key>::<value>` triples), pin verbatim. The MVP-emitted-vs-deferred line-set in §2.4 will be reconciled against this output.

3. **`/listeners` text format exact byte layout:** does Envoy emit only `<name>::<addr>` per listener, or additional fields (drain state, conn counts, ALPN config). Boot Envoy with the §2.6 fixture, scrape `/listeners`, pin verbatim.

4. **`/server_info` JSON exact field set + ordering:** which `state` enum values appear; whether `command_line_options` is emitted as `{}` when no flags set; what the `version` string looks like at v1.37.2; what `hot_restart_version` is when hot restart is not configured; what `uptime_*` units are (seconds-as-JSON-number vs. seconds-as-string). Boot Envoy with the §2.6 fixture, scrape `/server_info`, pin verbatim.

5. **`Content-Length` vs `transfer-encoding: chunked` framing on the new endpoints:** phase 01 documented that envoy-go emits `Content-Length` while upstream Envoy emits `transfer-encoding: chunked` for `/ready` (BEHAVIOR_CONTRACT.md `## Admin API — /ready`). Verify whether Envoy uses chunked framing for `/config_dump`, `/clusters`, `/listeners`, `/server_info` as well — if yes, the same framing-deviation applies and the differential harness's existing dechunk path covers it; if no, document per-endpoint framing.

6. **Header set on the new endpoints:** does Envoy emit the same header set on the new endpoints as on `/ready` (`Cache-Control: no-cache, max-age=0`, `X-Content-Type-Options: nosniff`, `Server: envoy`, `Date: <IMF-fixdate>`)? The 08.1 SPEC must pin the header set per endpoint and feed it into the differential ignore-list.

7. **`PRE_INITIALIZING` vs `INITIALIZING` `state` value:** Envoy's `ServerInfo.State` enum has both values; which does Envoy emit pre-MarkReady? envoy-go has no `INITIALIZING` phase distinct from `PRE_INITIALIZING`; the SPEC must pick a value that matches Envoy's actual behavior.

These seven empirical pins are the SPEC-time obligations carried forward from this brainstorm; all are scrape-against-v1.37.2 verifiable, and all gate the SPEC's `## Admin API` BEHAVIOR_CONTRACT additions.

### 2.8 Carry-forward dispositions

Phase 07.2 REVIEW.md identified 10 Minor findings (M-1 through M-10 per the agent-collected brief in §G of the brainstorm intelligence). None are admin/drain-specific; the closest is **M-8** (200ms drain hardcoded in the 0007b fixture driver — a test-harness observation, not a production drain bug). Phase 08.2 (graceful drain) will note M-8 as a sibling consideration (the 0010 drain fixture driver should not repeat the same hardcoded sleep pattern), but no carry-forward landing is required in 08.1's scope.

**M-1** (cluster-name validation vulnerability — `cluster.<name>` propagates into 8 metric names without `stats.IsValidName` check) is admin-tangentially-relevant: the new `/clusters` endpoint will read those same names back. If a malformed cluster name reached the metric registry, the `/clusters` text-format output would propagate the malformation. **08.1 SPEC must explicitly note** that the existing M-1 vulnerability propagates to `/clusters` output and that the M-1 fix-when-it-lands will close both surfaces simultaneously. No new fix in 08.1.

No other carry-forwards.

---

## 3. Phase 08.2 — sibling stub (full design deferred)

Phase 08.2 (`graceful-drain`) will receive its own BRAINSTORM.md at its lifecycle-state 1 brainstorm session (next session after 08.1 phase-done), per the 06.2 / 07.2 cascade pattern — phases that defer their full brainstorm rather than authoring it inline with the parent. This section captures only the forward-looking notes that 08.2's brainstorm will start from; it is NOT a 08.2 SPEC.

**Scope (per §1 split table):**

- New `internal/drain/` package owning the drain-state machine (`Manager` with states `LIVE`, `DRAINING`, exit) + drain-completion signaling.
- `cmd/envoy-go/main.go` SIGTERM-handler upgrade: current `<-ctx.Done()` + deferred `lm.Stop()` (line 152) becomes a flow that signals the drain manager, waits for drain-completion (or timeout), then exits cleanly.
- `internal/listener.Manager.Drain()`: stop-accepting on listening sockets (close the `net.Listener` returned by `net.Listen`; the Accept loop unblocks with `net.ErrClosed`).
- `internal/cluster.Manager.Drain()`: best-effort upstream connection close after drain timeout (or earlier if in-flight count reaches zero).
- `POST /drain_listeners` admin endpoint: 200 OK with body `OK\n` (empirical-pin obligation); triggers `Manager.Drain()` without process exit.
- `/ready` extension: when `Manager.State() == DRAINING`, return 503 with body `DRAINING\n` (or whatever Envoy emits — empirical-pin obligation; this extends ADR-0015's pre-init contract with a new state-specific body).
- `/server_info` `state` field: returns `"DRAINING"` when drain is in progress.
- New `BEHAVIOR_CONTRACT.md ## Admin API ### /drain_listeners` subsection + `### /ready` extension for the DRAINING-state body + `## Graceful drain` umbrella section.

**Anticipated 08.2 ADRs (5–7, settled at 08.2 brainstorm time):**

- Drain state machine shape (LIVE / DRAINING / exit; no INITIALIZING per §2.5 decision)
- SIGTERM-vs-SIGINT semantics (SIGTERM = drain-then-exit; SIGINT = drain-then-exit OR immediate-exit — matches Envoy's behavior)
- `/ready` DRAINING-state body extension (extends ADR-0015 — partial supersession)
- `/drain_listeners` POST contract (response body, idempotency, multi-call behavior)
- `/server_info` state transitions during drain (LIVE → DRAINING — and PRE_DRAINING if Envoy has it as a distinct intermediate state)
- Drain timeout default (Envoy default is 600s; envoy-go MVP likely 30s with operator-knob deferred)
- Hot-restart deferral (ADR'd "hot restart is out of scope; runtime/hot-restart family will land it; envoy-go-go's drain is one-process scope only")

**Out of 08.2 scope (deferred per ADR-0040):**

- Hot restart / parent-child handoff (requires socket-passing, shared-memory state, runtime/hot-restart family)
- `POST /quitquitquit` endpoint (semantic overlap with SIGTERM + `/drain_listeners`; defer evaluation)
- Per-listener selective drain (`/listeners/<name>/drain`)
- `drain_strategy` per-listener (default GRADUAL only; IMMEDIATE strategy deferred)
- Configurable `drain_time_s` via bootstrap or admin POST
- Connection-level drain windows configurability
- Drain manager interaction with xDS (no xDS yet)

**Differential fixture (08.2):** `0010-graceful-drain` (TBD shape; likely an in-flight-request-completes-while-new-conns-rejected scenario plus `/ready` and `/server_info` state-transition observation). Specification deferred to 08.2 BRAINSTORM.

The 08.2 BRAINSTORM will start from this stub; the §3 forward-looking notes here are the seed.

---

## 4. Architecture & package layout (08.1 only)

```
internal/admin/                                  -- existing; expanded
  admin.go                                       -- modified: New() signature widens to take
                                                 --   *bootstrap.Bootstrap, *cluster.Manager,
                                                 --   *listener.Manager. Start() registers four
                                                 --   new mux handlers.
  configdump.go                                  -- NEW: handleConfigDump + buildConfigDump +
                                                 --   enumerateStaticListeners + enumerateStaticClusters
                                                 --   ~150 LOC + tests
  clusters.go                                    -- NEW: handleClusters + format helpers
                                                 --   ~120 LOC + tests
  listeners.go                                   -- NEW: handleListeners + format helpers
                                                 --   ~80 LOC + tests
  serverinfo.go                                  -- NEW: handleServerInfo + buildServerInfo +
                                                 --   uptime tracking
                                                 --   ~150 LOC + tests
  prometheus.go                                  -- existing, untouched
  doc.go                                         -- update package doc to enumerate the six
                                                 --   endpoints (was four)
  *_test.go                                      -- per-handler unit tests

internal/listener/                               -- existing; small accessor extension
  manager.go                                     -- NEW: ClustersInfo() / Listeners() snapshot
                                                 --   methods returning read-only structs.
                                                 --   note: ListenerInfo{Name, Addr} already exists;
                                                 --   may be extended with stats fields.
                                                 --   ~30 LOC

internal/cluster/                                -- existing; small accessor extension
  manager.go                                     -- NEW: Clusters() snapshot method returning
                                                 --   ClusterInfo{Name, Endpoints []EndpointInfo, ...}
                                                 --   ~50 LOC

cmd/envoy-go/main.go                             -- modified: admin.New() call site widens to
                                                 --   thread bs + cm + lm. One-line change.
                                                 --   ~5 LOC

test/differential/0009-admin-config-dump/        -- new fixture
  README.md
  expectations.yaml                              -- per-endpoint tolerance discipline
  envoy.yaml                                     -- reference Envoy bootstrap (port 9902 admin)
  envoy-go.yaml                                  -- envoy-go bootstrap (port 9901 admin)
  driver/driver.go                               -- 5-request load + 4-endpoint scrape +
                                                 --   per-endpoint comparator
  backends/backend.go                            -- ports 18001 + 18002 static-200 backend

test/differential/runner.go                      -- registration update for fixture 0009
                                                 --   (RequiresReference: true)

test/fuzz/                                       -- one new fuzzer
  configdump_fuzz.go                             -- NEW: FuzzConfigDumpFormat — fuzzes
                                                 --   adversarial bootstrap protos into
                                                 --   buildConfigDump + protojson.Marshal;
                                                 --   asserts no panic / invalid JSON output
                                                 --   ~80 LOC
```

**Key shape notes:**

1. **No new lifecycle state, no new persistence.** All four handlers are stateless beyond the per-`Server` references to `*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager`. Each handler walks live state at request time.

2. **Constructor-widening pattern.** `admin.New()` grows three parameters; the call site in `cmd/envoy-go/main.go` updates with one line. No package-globals introduced. Mirrors the 07.1 hand-off: every new dependency is threaded explicitly (LBP-1 discipline from 06.1).

3. **`ListenerInfo` struct extension.** The existing `cmd/envoy-go/main.go:147` already calls `lm.Listeners()` returning `[]ListenerInfo{Name, Addr}`. 08.1 may extend the struct with read-only state fields (active conn count from the existing `listener.<addr>.downstream_cx_active` gauge). The extension is additive — existing call sites unchanged.

4. **`ClusterInfo` struct introduction.** `cluster.Manager` does not currently have a public read-only accessor for its cluster list (the underlying `clusters map[string]*Cluster` is internal). 08.1 introduces `ClusterInfo{Name string, Endpoints []EndpointInfo}` and a `Clusters()` accessor. Counters are NOT cached in the struct — they are read live from the `*stats.Counter` / `*stats.Gauge` instances at format time per endpoint scrape (`atomic.LoadInt64`).

5. **No filter-chain hot-path mutation.** `/config_dump`, `/clusters`, `/listeners`, `/server_info` all read snapshots; none mutate the running listener / cluster / filter state. This is a clean boundary distinguishing 08.1 from 08.2.

6. **One new fuzzer.** `FuzzConfigDumpFormat` is the obvious adversarial-config bug vector (the parsed bootstrap proto goes through protojson serialization; pathological proto values may produce invalid JSON or panic the marshaler). 80 LOC + 30s budget per ADR-0018.

---

## 5. Data flow & wiring (08.1)

### 5.1 Boot wiring

```
cmd/envoy-go/main.go
   ↓
bootstrap.Load(configPath) → *Bootstrap          [unchanged]
   ↓
cluster.NewManager(...)                          [unchanged]
   ↓
admin.New(adminAddr, bs.Stats, bs, cm, lm)       [WIDENED — bs, cm, lm threaded]
   ↓ admin.Server holds: bootTime, bs, cm, lm + the existing fields
   ↓
admSrv.Start()                                   [unchanged outer; Start() now registers
                                                  six handlers including the four new ones]
   ↓
... (rest of boot flow: filter registry, listener manager, ctx + signal, lm.Start, MarkReady,
     stats.Freeze, ready sentinels, <-ctx.Done())
```

The `bootTime` is captured at `admin.New()` time (currently `time.Now()` at that line). It serves as the source for `/server_info`'s `uptime_current_epoch_seconds` and `/config_dump`'s `BootstrapConfigDump.last_updated`.

### 5.2 Per-request flow — `/config_dump`

```
client → GET /config_dump
   ↓ ServeMux dispatch → s.handleConfigDump
   ↓ s.buildConfigDump():
        bootDump := *adminv3.BootstrapConfigDump{Bootstrap: s.bs.Proto, LastUpdated: timestamppb.New(s.bootTime)}
        lisDump  := *adminv3.ListenersConfigDump{StaticListeners: enumerateStaticListeners(s.bs.Proto), VersionInfo: "static"}
        cluDump  := *adminv3.ClustersConfigDump{StaticClusters: enumerateStaticClusters(s.bs.Proto), VersionInfo: "static"}
        cd := &adminv3.ConfigDump{Configs: []*anypb.Any{anypb.New(bootDump), anypb.New(lisDump), anypb.New(cluDump)}}
   ↓ body, _ := protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: <pin>, EmitUnpopulated: <pin>}.Marshal(cd)
   ↓ write status + headers (Content-Type: application/json, Cache-Control, X-Content-Type-Options, Server, Date) + body
```

Errors from `protojson.Marshal` are logged and synthesized as `500 Internal Server Error` with body `{"error": "marshal failure"}`. Empirical-pin obligation: verify Envoy's error-response shape on `/config_dump` failure (likely just an HTTP 500 with no JSON body).

### 5.3 Per-request flow — `/clusters`

```
client → GET /clusters
   ↓ ServeMux dispatch → s.handleClusters
   ↓ infos := s.cm.Clusters()  -- []ClusterInfo snapshot
   ↓ var buf bytes.Buffer
   ↓ for each cluster info:
        buf.WriteString(cluster.Name + "::observability_name::" + cluster.Name + "\n")
        buf.WriteString(cluster.Name + "::added_via_api::false\n")
        for each endpoint:
             cxActive := s.cm.CounterValue("cluster." + cluster.Name + ".upstream_cx_active")  -- via *stats.Gauge.Value()
             cxTotal  := s.cm.CounterValue("cluster." + cluster.Name + ".upstream_cx_total")
             rqTotal  := s.cm.CounterValue("cluster." + cluster.Name + ".upstream_rq_total")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::cx_active::" + fmt(cxActive) + "\n")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::cx_total::"  + fmt(cxTotal)  + "\n")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::rq_total::"  + fmt(rqTotal)  + "\n")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::health_flags::healthy\n")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::weight::1\n")
             buf.WriteString(cluster.Name + "::" + endpoint.Addr + "::priority::0\n")
   ↓ write status + headers (Content-Type: text/plain; charset=UTF-8, ...) + buf.Bytes()
```

Per-cluster line ordering: alphabetical by cluster name, then in-bootstrap-declared endpoint order (preserves bootstrap deterministic order). Empirical-pin obligation: verify Envoy's ordering matches.

### 5.4 Per-request flow — `/listeners`

```
client → GET /listeners
   ↓ ServeMux dispatch → s.handleListeners
   ↓ infos := s.lm.Listeners()  -- []ListenerInfo snapshot
   ↓ var buf bytes.Buffer
   ↓ for each listener info (alphabetical by name, deterministic):
        buf.WriteString(listener.Name + "::" + listener.Addr + "\n")
   ↓ write status + headers + buf.Bytes()
```

### 5.5 Per-request flow — `/server_info`

```
client → GET /server_info
   ↓ ServeMux dispatch → s.handleServerInfo
   ↓ info := &adminv3.ServerInfo{
        Version: buildVersionString(),
        State:   s.deriveState(),  -- LIVE post-MarkReady; PRE_INITIALIZING pre-MarkReady
        UptimeCurrentEpoch: durationpb.New(time.Since(s.bootTime)),
        UptimeAllEpochs:    durationpb.New(time.Since(s.bootTime)),
        Node:    s.bs.Proto.GetNode(),
     }
   ↓ body, _ := protojson.MarshalOptions{Multiline: true, Indent: "  ", ...}.Marshal(info)
   ↓ write status + headers + body
```

The `s.deriveState()` method returns `"LIVE"` when `s.ready.Load()` is true, `"PRE_INITIALIZING"` otherwise. 08.2 will extend this to return `"DRAINING"` when the drain manager is active.

`buildVersionString()` returns a build-time-baked string. The exact format pin obligation: see §2.7 #4.

### 5.6 Concurrency model

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `admin.New(...)` | Once | None — single-goroutine boot |
| Boot | `admSrv.Start()` registers handlers | Once | mux is per-Server; not shared |
| Per-request | `handleConfigDump` reads `s.bs.Proto` | Per scrape | `s.bs.Proto` is immutable post-boot; lock-free read |
| Per-request | `handleClusters` calls `s.cm.Clusters()` + counter reads | Per scrape | `cm.Clusters()` snapshots an immutable post-boot map; counter reads are `atomic.LoadInt64` per `*stats.Counter.Value()` |
| Per-request | `handleListeners` calls `s.lm.Listeners()` | Per scrape | `lm.Listeners()` snapshots an immutable post-boot list |
| Per-request | `handleServerInfo` reads `s.ready` + `s.bootTime` + `s.bs.Proto.Node` | Per scrape | `s.ready` is `atomic.Bool`; `s.bootTime` is set at `New()` and read-only; `s.bs.Proto` is immutable post-boot |

**Key invariant:** all four new handlers are **pure read** operations against immutable-post-boot structures + atomically-loaded counters. No new mutex; no new channel. The `*stats.Counter` / `*stats.Gauge` Walk-under-RLock-plus-atomic-Load discipline from phase 06.1 (LBP-1) covers the counter-read path; the bootstrap proto and the cluster/listener manager maps are immutable post-`Freeze()` per the LBP-1 generalization in phase 07.

**Race-detector contract:** `go test -race ./...` clean for N concurrent scrapes against all four endpoints from N goroutines. Unit tests in `*_test.go` exercise.

---

## 6. Error handling, edge cases (08.1)

### 6.1 Edge cases

- **Empty cluster list** (`bootstrap.static_resources.clusters: []`): `/clusters` body is empty (`""`). `/config_dump` `ClustersConfigDump.static_clusters: []`. Empirical-pin obligation: verify Envoy emits empty body vs. `# No clusters\n` or similar.
- **Empty listener list:** `/listeners` body empty. `/config_dump` `ListenersConfigDump.static_listeners: []`. Same pin obligation.
- **Listener with no name** (Envoy permits this — the listener gets an auto-generated name): `/listeners` emits the auto-generated name. envoy-go's listener manager already validates that listener names are non-empty (per phase 02 SPEC); this case is unreachable in current envoy-go but the format must handle it for robustness.
- **Cluster with cluster-name containing `::`:** the text format separator is `::`; an embedded `::` in the cluster name would corrupt the format. M-1 carry-forward (cluster-name validation): the existing M-1 vulnerability is that cluster names are not validated; 08.1 SPEC documents the propagation but does not fix M-1. The 08.1 implementation treats cluster-name-with-`::` as malformed input and emits the literal name (Envoy parity — Envoy does not escape either; the security implication is on the operator).
- **Bootstrap with no `node` field:** `/server_info` `node` field omitted (per protojson `EmitUnpopulated: false`).
- **`/server_info` requested pre-MarkReady:** `state: "PRE_INITIALIZING"`, uptime fields populated relative to `admin.New` time, version + node populated. Body is well-formed — no special-casing.
- **Concurrent scrape during a config reload:** envoy-go does not support config reload (no xDS, no SIGHUP-triggered reload). The bootstrap proto is genuinely immutable post-boot. No race possible. (xDS family will revisit.)
- **Scrape body exceeds `WriteTimeout` (5s):** the `*http.Server`'s `WriteTimeout: 5 * time.Second` (current `internal/admin/admin.go:53`) bounds total response time. `/config_dump` for very large bootstraps may approach this; the SPEC may need to widen the timeout (e.g., 30s) or accept that pathologically-large bootstraps fail. Empirical-pin: verify Envoy's admin server timeout behavior.
- **`HEAD /config_dump`:** Go's `http.ServeMux` + `*http.Server` handles HEAD via the same handler with auto-suppression of the body. The status + headers should match GET. Empirical-pin: verify Envoy's behavior on HEAD.
- **Trailing slash (`/config_dump/`):** ServeMux dispatch behavior on `path/` vs. `path` — Go's default redirects `path/` to `path` if `path` is registered. Empirical-pin: verify Envoy's behavior; the SPEC may need an explicit rejection of trailing slash.
- **Method other than GET (POST, PUT, DELETE) on read-only endpoints:** envoy-go's handlers do not currently method-check. Envoy returns 405 Method Not Allowed for non-GET on read-only endpoints (verify). The SPEC must add explicit method checks.

### 6.2 Error paths

| Source | Failure mode | Behavior |
|---|---|---|
| `protojson.Marshal` panic / error | Adversarial proto value | Recover panic; synthesize 500 with body `{"error": "..."}` (or empty body — pin); log |
| `bs.Proto` nil (impossible in practice; bootstrap.Load guarantees non-nil on success) | Defensive | Synthesize 500; log |
| `cm` / `lm` nil | Constructor misuse (impossible if main.go threads correctly) | Panic at `admin.New()` time, not at request time |
| Missing counter (cluster.Manager has a cluster but no `*stats.Counter` for `upstream_rq_total`) | Stats registry desync | Read returns 0; format proceeds; log a one-time warning |

### 6.3 Things 08.1 does NOT handle

- Method discrimination (handlers accept any method; SPEC may add 405 enforcement)
- Path normalization beyond ServeMux's default (trailing slash, percent-encoding)
- Authentication / ACL on the admin port (out of scope per Decision E)
- Body filtering / query-param parsing (`?format=`, `?resource=`, `?mask=`, `?include_eds=`)
- Streaming responses (each handler buffers the full body before write; `/config_dump` may be large but bounded by the bootstrap size)
- HTTP/2 over admin (admin stays HTTP/1.1)
- TLS on admin (admin is plaintext per phase-01 contract)
- Compression (no `Content-Encoding: gzip`)

---

## 7. Testing strategy (08.1)

### 7.1 Unit tests

`internal/admin/`:

- `configdump_test.go` — unit tests for `buildConfigDump` over a fixture bootstrap proto: assert envelope ordering (Bootstrap, Listeners, Clusters), assert all three sub-envelopes populated, assert `last_updated` is set, assert `version_info` is `"static"`, assert protojson output is valid JSON parseable by `json.Unmarshal`.
- `clusters_test.go` — unit tests for the format function: per-cluster line ordering (alphabetical), per-endpoint line ordering (in-bootstrap order), counter values reflected in output, empty-cluster-list emits empty body.
- `listeners_test.go` — single-line-per-listener ordering, empty-listener-list emits empty body.
- `serverinfo_test.go` — `state: LIVE` post-MarkReady; `state: PRE_INITIALIZING` pre-MarkReady; uptime monotonically increasing across two calls separated by `time.Sleep`.
- `admin_test.go` — modified: existing tests for `/ready` + `/stats/prometheus` preserved verbatim; new tests for the four endpoints checking response status (200), Content-Type, header set, body well-formed.

`internal/cluster/`:

- `manager_test.go` — modified: new tests for `Clusters()` snapshot accessor; assert returned slice is read-only (modification by caller does not affect manager state); assert per-endpoint EndpointInfo populated.

`internal/listener/`:

- `manager_test.go` — modified: tests for `Listeners()` (already exists) extended for any new fields added to `ListenerInfo`.

### 7.2 Differential fixture `0009-admin-config-dump`

(See §2.6 above for matrix + per-endpoint equivalence claim.)

**Driver outline:**

1. Boot envoy-go on admin:9901 + reference Envoy on admin:9902 + backends:18001+18002 with the §2.6 bootstrap (admin port differs to avoid bind conflict).
2. Issue 5 `GET / HTTP/1.1` requests through proxy:10000 (driving cluster + endpoint counters).
3. Wait briefly for stats to settle (counter writes complete after the request is acknowledged).
4. Scrape the four endpoints from each proxy.
5. Per-endpoint comparator (see §2.6 for the tolerance discipline):
   - `/config_dump`: parse both as JSON, compare structurally with field-tolerance for `version`, `last_updated`, `node.user_agent_build_version`.
   - `/clusters`: parse both into `(cluster, key, value)` tuple sets, compare with tolerance for hot-path counters.
   - `/listeners`: byte-equal.
   - `/server_info`: parse both as JSON, compare structurally with field-tolerance for `version`, `uptime_*`, `command_line_options`, `hot_restart_version`.

### 7.3 Race detector + lint

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean (gate (e)).

Concurrent-scrape race-test: 100 goroutines each scraping all four endpoints in a tight loop for 1s. Asserts no race-detector finding, no panic, no malformed responses.

### 7.4 Fuzzers

Existing 9 fuzzers (8 from phases 02–06 + `FuzzFilterChainParse` from 07.1) re-run at 30s budget per ADR-0018. **New: `FuzzConfigDumpFormat`** — fuzzes adversarial bootstrap proto values into `buildConfigDump` + `protojson.Marshal`; asserts no panic + output is valid JSON parseable by `json.Unmarshal`. ~80 LOC. Total: 10 fuzzers post-08.1.

### 7.5 h2spec re-run

Phase 08.1 does not touch HCM, listener filter chain, H2 codec, or any request hot path. The h2spec gate at 53/53 must remain green; re-running is mechanical (gate (c) per ADR-0051).

### 7.6 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Standard six-gate sweep applies:

- (a) `go build ./...` clean
- (b) `go test ./...` clean (existing + new unit tests)
- (c) h2spec re-run clean (53/53; ADR-0051 pin unchanged)
- (d) Differential fixtures 0000–0008 + new 0009 all green
- (e) `go vet ./... && golangci-lint run ./... && go test -race ./...` clean
- (f) `BEHAVIOR_CONTRACT.md` `## Admin API` umbrella section restructured + populated for the four new endpoints

---

## 8. BEHAVIOR_CONTRACT.md additions (08.1)

Phase 08.1 restructures the existing `## Admin API — /ready` section into a `## Admin API` umbrella with per-endpoint subsections. The structure mirrors the existing `## HTTP/1.1`, `## HTTP/2`, `## TCP proxy`, `## HTTP filter chain` (07.1) sections.

### 8.1 Section restructure

Before (current `BEHAVIOR_CONTRACT.md` line ~267):

```
## Admin API — /ready

### Ready-state response (authoritative)
### Pre-init response
### Applies to
### Does not yet apply to
```

After (post-08.1):

```
## Admin API

### /ready                                       -- existing content, verbatim-preserved
    Ready-state response (authoritative)
    Pre-init response
### /stats/prometheus                            -- short summary; full pin in phase 06.1's section
### /config_dump                                 -- NEW (08.1)
    Body shape (envelope ordering, protojson options, indentation)
    Header set (Content-Type, Cache-Control, X-Content-Type-Options, Server, Date)
    Empirical-pin block (verbatim Envoy v1.37.2 scrape — first 50 lines)
    Equivalence claim (byte-equal modulo build/timestamp/uptime allow-list)
### /clusters                                    -- NEW (08.1)
    Body shape (text format, line ordering, separator)
    Header set
    Empirical-pin block (verbatim Envoy v1.37.2 scrape)
    Equivalence claim (tuple-set equality with hot-path counter tolerance)
    MVP-emitted vs. deferred line-set
### /listeners                                   -- NEW (08.1)
    Body shape (single line per listener)
    Header set
    Empirical-pin block
    Equivalence claim (byte-equal)
### /server_info                                 -- NEW (08.1)
    Body shape (JSON field set, state enum, uptime units)
    Header set
    Empirical-pin block (verbatim Envoy v1.37.2 scrape)
    Equivalence claim (byte-equal modulo build/uptime/CLI-flags allow-list)

### Applies to
    - phase-08.1 envoy-go admin subsystem.
    - all six endpoints: /ready, /stats/prometheus, /config_dump, /clusters, /listeners, /server_info.

### Does not yet apply to
    - HTTP/2 over admin (admin stays HTTP/1.1)
    - TLS on admin (admin stays plaintext)
    - DRAINING-state response on /ready (08.2)
    - DRAINING value on /server_info `state` field (08.2)
    - Mutating endpoints (POST /drain_listeners is 08.2)
    - JSON form of /clusters and /listeners (deferred per ADR-0040)
    - Query-param filtering on /config_dump (deferred per ADR-0040)
    - RoutesConfigDump, SecretsConfigDump, ScopedRoutesConfigDump, EndpointsConfigDump envelopes
    - All deferred admin endpoints listed in 08.1 SPEC §[ADR for deferral list]
    - ACL / authentication on admin port (operator firewall responsibility)
    - Method discrimination on read-only endpoints (POST/PUT/DELETE behavior pinned but not enforced beyond Envoy parity)
```

The existing `### /ready` content is verbatim-preserved (no re-derivation; the 01 + ADR-0015 evidence stands).

### 8.2 Empirical-pin verbatim subsections

Following the 06.1 / 06.2 / 07.1 pattern, four short verbatim-evidence blocks live under `### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info` — each is a 5–30-line Envoy-scrape excerpt scraped at SPEC time and pinned at the ENVOY_TARGET.md SHA.

### 8.3 New equivalence-matrix rows

Appended to the `## Equivalence Matrix` table:

```
| Dimension                   | Equivalence claim                                          | Allow-list / tolerance                                  |
|-----------------------------|-----------------------------------------------------------|---------------------------------------------------------|
| Admin /config_dump          | Body byte-equal to reference Envoy v1.37.2 modulo        | `version`, `last_updated`, `node.user_agent_build_*`   |
|                             | build/timestamp/uptime allow-list. Three-envelope        | per-field allow-listed. dynamic_* arrays absent in     |
|                             | ordering: Bootstrap, Listeners, Clusters.                | both.                                                   |
| Admin /clusters             | Tuple-set equality on `(cluster, key, value)` triples.   | Hot-path counters `cx_total`, `rq_total` allow ±1      |
|                             | envoy-go MVP-emitted line set is a subset of Envoy's     | tolerance. envoy-go-deferred lines (locality tags,     |
|                             | unconditionally-emitted line set.                        | success rates) absent on envoy-go side.                 |
| Admin /listeners            | Body byte-equal.                                         | None.                                                   |
| Admin /server_info          | Body byte-equal modulo build/uptime/CLI-flags allow-list.| `version`, `uptime_current_epoch_seconds`,             |
|                             |                                                          | `uptime_all_epochs_seconds`, `command_line_options`,    |
|                             |                                                          | `hot_restart_version` per-field allow-listed.           |
```

### 8.4 ADR-0015 forward pointer

The existing `### /ready` subsection mentions ADR-0015 (pre-init contract). 08.2's DRAINING-state extension will partially supersede ADR-0015 by adding a DRAINING-state body. 08.1 leaves ADR-0015 untouched; the forward pointer is added by 08.2.

---

## 9. ADRs anticipated (08.1)

The planning session (`superpowers:writing-plans` for SPEC.md, then PLAN.md) finalizes count + numbering. **Seven ADRs anticipated for 08.1**, starting at ADR-0084 (next-free per `STATE.md` last-commit `01abdfe` = phase-07.2 closing at ADR-0083):

1. **Phase-08 planner-time split (08.1 + 08.2)** — mirrors ADR-0045 (planner-time split) and the 06 → 06.1/06.2 + 07 → 07.1/07.2 precedents. Documents the disjoint-scope rationale (read-only admin endpoints vs. mutating drain semantics + lifecycle state machine) + 08.1-first ordering rationale (08.2 depends on 08.1's admin-mux scaffold extension). (Decision §1.)

2. **Reuse phase-01 admin HTTP/1.1 mux; no new admin transport** — extends the existing `internal/admin.Server` with four new handlers; constructor-widening pattern for new dependencies; no package-globals; LBP-1 discipline carried forward. (Decision §2.2.)

3. **`/config_dump` shape: protojson over `*adminv3.ConfigDump` envelope; static-only ConfigDump sub-envelopes (Bootstrap + Listeners + Clusters); no Routes/Secrets/Endpoints/ScopedRoutes/EDS envelopes** — settles the wire format choice (protojson with empirical-pinned MarshalOptions); deferral list for the missing envelopes (per ADR-0040). (Decision §2.3.)

4. **`/clusters` and `/listeners` shape: text format only (no JSON form); MVP-emitted line set is a structurally-required subset of Envoy's unconditional output; deferred lines listed per ADR-0040** — settles the format choice + the partial-coverage discipline. (Decision §2.4.)

5. **`/server_info` MVP field set; `state` enum values `LIVE` + `PRE_INITIALIZING` only in 08.1 (DRAINING + INITIALIZING deferred)** — settles the field set + the lifecycle-state coverage; documents that `INITIALIZING` is not modeled (envoy-go has no extended init phase distinct from PRE_INITIALIZING). (Decision §2.5.)

6. **Admin endpoint deferral list** — enumerates the deferred surface per ADR-0040 format (`?format=json` query-param, `?resource=` filtering, RoutesConfigDump, SecretsConfigDump, ScopedRoutesConfigDump, EndpointsConfigDump, POST /reset_counters, /runtime, /certs, /memory, /heap_dump, POST /quitquitquit (08.2 to evaluate), POST /healthcheck/*, /logging, POST /reopen_logs, /listeners/<name>/* sub-routes, /init_dump). Reference-point for future admin-extensions phases. (Decision §2.1 deferral table.)

7. **No-ACL admin-endpoint security posture** — operator firewall responsibility; admin port plaintext; mirrors Envoy default. Documented carry-forward consideration: future security-hardening phase may add ACL via a separate admin proto. (Decision §2.5 amplification.)

**Inline supersession candidates (consolidate at SPEC time if appropriate):**

- ADR-0014 (Server header value) is referenced by the `/ready` subsection but is not modified by 08.1. Forward-only.
- ADR-0015 (pre-init contract for `/ready`) is referenced; 08.2 will partially supersede when adding the DRAINING-state body. 08.1 forward-only.
- ADR-0040 (out-of-scope deferrals format) governs ADRs 4 + 6 above; both ADRs cite it as the format authority.
- ADR-0041 (silent-ignore set) — none of the 08.1 fields are in the silent-ignore set; no amendment.
- ADR-0045 (planner-time split) — ADR 1 is its application; no separate split-ADR needed.

(Phase 06.1 had 6 ADRs; 06.2 had 4; 07.1 had 7; 07.2 had 7. 7 sits at the high end — appropriate for a phase that introduces four new endpoints with an empirical-pin discipline. Planning session may consolidate ADRs 3+4+5 into a single "Admin endpoint body shapes" ADR if they prove inseparable.)

---

## 10. Out-of-scope items deferred to later phases

| Item | Deferred to |
|---|---|
| Graceful drain (drain manager + SIGTERM-triggered drain + `POST /drain_listeners` + `/ready` DRAINING-state body + `/server_info` DRAINING state + listener.Manager.Drain + cluster.Manager.Drain + connection-level drain windows) | **08.2** |
| `?format=json` query-param on `/clusters` and `/listeners` | future admin-extensions phase |
| `?resource=` / `?mask=` / `?include_eds=` filtering on `/config_dump` | future admin-extensions phase |
| `RoutesConfigDump` envelope in `/config_dump` | follow-up minor or future admin-extensions phase |
| `SecretsConfigDump` envelope (with redaction discipline) | future secrets/cert-lifecycle family |
| `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes | xDS family |
| `POST /reset_counters` | future admin-extensions phase |
| `/runtime`, `POST /runtime_modify` | RTDS family |
| `/certs` | secrets / cert-lifecycle family |
| `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention` | future ops-endpoints phase |
| `POST /quitquitquit` | 08.2 to evaluate; may be deferred further |
| `POST /healthcheck/fail`, `POST /healthcheck/ok` | upstream-robustness family (active health checking) |
| `/logging`, `POST /logging?level=...` | future ops-endpoints phase |
| `POST /reopen_logs` | future ops-endpoints phase |
| `/listeners/<name>/...` per-listener admin sub-routes | 08.2 to evaluate; likely defer further |
| `/init_dump` | likely never (envoy-go has no init manager) |
| Hot restart / parent-child handoff | runtime / hot-restart family |
| HTTP/2 over admin transport | future admin-extensions phase or admin-h2 upgrade |
| TLS on admin transport | future admin-extensions phase |
| Compression on admin responses (`Content-Encoding: gzip`) | future admin-extensions phase |
| Authentication / ACL on admin port | future security-hardening phase |
| Method discrimination on read-only endpoints (405 enforcement) | 08.1 SPEC may include or defer based on Envoy parity scrape |
| Streaming `/config_dump` for very large bootstraps | future admin-extensions phase |
| M-1 (cluster-name validation) carry-forward fix | future stats-hardening phase or 08.1 SPEC may bundle if cheap |

---

## 11. Hand-off to writing-plans

Next session (lifecycle-state 1 → 2 for phase 08.1, skill `superpowers:writing-plans`) authors:

- `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` — master design summarizing 08.1 + 08.2 scope (mirrors `docs/envoy-go/phases/06-observability-baseline/SPEC.md` and the existing `phases/05-http-2/SPEC.md` + `phases/07-filter-chain-framework/SPEC.md` once written).
- `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` — sub-phase SPEC for 08.1 derived from §§2-10 of this document, including the §2.7 empirical-pin obligations executed at SPEC-drafting time (verbatim Envoy v1.37.2 scrape evidence pinned for `/config_dump` JSON shape, `/clusters` text format, `/listeners` text format, `/server_info` JSON shape, header sets across all four, framing for each endpoint, and `state`-enum value in pre-MarkReady response).
- `docs/envoy-go/phases/08.2-graceful-drain/README.md` — sibling SPEC stub citing the master SPEC + this BRAINSTORM § "08.2 sub-phase" forward-looking notes (the `## 1. Scope decision` table + § 3 stub is sufficient for the placeholder).
- **ROADMAP.md split:** row `08 | admin-api-and-drain | 07 | planned` becomes parent `08 | admin-api-and-drain | 07 | in-progress | 08.1, 08.2 | ...` with rows `08.1 | admin-endpoints | 07 | planned | | ...` and `08.2 | graceful-drain | 08.1 | planned | | ...`.
- After SPEC, lifecycle-state 1 → 2 with `next-skill: superpowers:writing-plans` (PLAN.md authoring) and `active-phase: 08.1-admin-endpoints`.

This BRAINSTORM.md is committed as the brainstorm-close artifact and is read-only history once the next session starts. Future sessions consult it as the authoritative record of the design decisions made here.
