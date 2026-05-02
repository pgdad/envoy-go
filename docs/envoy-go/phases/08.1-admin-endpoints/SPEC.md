# Phase 08.1 — Admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`)

**Phase id:** `08.1`
**Slug:** `08.1-admin-endpoints`
**Status:** `in-progress` (SPEC stage; ROADMAP row `08.1` flips `planned → in-progress` at this commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md (`docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md`) §§2–10 into formal SPEC shape, executing the seven §2.7 empirical-pin obligations against reference Envoy v1.37.2 in-session per ADR-0004)
**Depends on:** phase 07 (done at master `01abdfe` — 07.2 phase-done close, also closes parent row 07; SHA-fill follow-up at master `f3835a5`). Specifically, 07.2's listener-chain-completion landed `cmd/envoy-go/main.go:147` style `lm.Listeners()` accessor returning `[]ListenerInfo{Name, Addr}` — the 08.1 `/listeners` handler reuses it.
**Parent phase:** `08-admin-api-and-drain` — parent-master SPEC at `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md`. Per parent §5, the parent row `08` flips `in-progress → done` AT THE SAME COMMIT as 08.2's phase-done; 08.1's phase-done leaves the parent row unchanged.
**Master design document:** `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` (autonomous-brainstorm artifact per ADR-0004; this SPEC distills BRAINSTORM §§2–10 into formal contract language and executes the §2.7 empirical-pin obligations IN SESSION).
**Differential surface at end of sub-phase:** ROADMAP row `08.1` flips `in-progress → done`; parent row `08` and row `08.2` unchanged. NEW differential fixture `0009-admin-config-dump` (four-endpoint per-endpoint equivalence under a 5-request defined load against the §2.6 fixture bootstrap) is differentially green. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe`, `0008-listener-chain-match` all green. The h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS (08.1 does not touch HCM, listener filter chain, H2 codec, or any request hot path). One new fuzzer (`FuzzConfigDumpFormat`) runs clean at the 30s ADR-0018 budget. Total fuzzer count post-08.1: 10. `BEHAVIOR_CONTRACT.md ## Admin API — /ready` section restructured into `## Admin API` umbrella with five per-endpoint subsections + four new equivalence-matrix rows.

---

## 1. Purpose

Phase 08.1 lands the four read-only admin endpoints that move envoy-go from "a proxy you can route traffic through and Prometheus-scrape" to "a proxy an operator can introspect" — `GET /config_dump` (the static-config dump that lets an operator verify what Envoy parsed), `GET /clusters` (the cluster + endpoint runtime status that lets an operator see which upstreams are healthy), `GET /listeners` (the listener inventory that lets an operator see which sockets are bound), `GET /server_info` (the build / state / uptime / node summary that lets an operator know what version is running and whether it is live). All four are read-only against immutable-post-boot structures plus atomically-loaded counters; no new lifecycle state, no new mutex, no new package outside `internal/admin/`. The mutating surface (`POST /drain_listeners`) and the lifecycle-state machine (graceful drain) are 08.2's deliverables.

Concretely, 08.1 lands:

1. Four new mux registrations on the existing `*http.ServeMux` allocated by `internal/admin.Server.Start()` (current `internal/admin/admin.go:47`): `/config_dump`, `/clusters`, `/listeners`, `/server_info`. No new HTTP server, no new bind, no new transport — admin stays HTTP/1.1 plaintext per phase 01's contract.
2. A constructor-widening of `internal/admin.New` from `New(addr string, registry *stats.Registry) *Server` to `New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server`. The widening threads the additional dependencies needed by the four new handlers — explicit-threading discipline (no package-globals) per LBP-1 inherited from 06.1's `*stats.Registry` discipline and 07.1's `*HTTPRegistry` discipline.
3. Four new handler implementations in `internal/admin/`: `configdump.go`, `clusters.go`, `listeners.go`, `serverinfo.go`. Each handler is a stateless `http.HandlerFunc` walking live state at request time; no caching, no precomputation. Counter reads are `atomic.LoadInt64` per `*stats.Counter.Value()` / `*stats.Gauge.Value()` from the existing 06.1 stats Registry.
4. A new public read-only accessor `cluster.Manager.Clusters() []ClusterInfo` returning a snapshot of cluster + endpoint metadata (endpoints, hostname, weight, priority); a `ClusterInfo{Name, Endpoints []EndpointInfo}` struct introduced in `internal/cluster/manager.go`. The existing `listener.Manager.Listeners() []ListenerInfo` accessor (already wired at `cmd/envoy-go/main.go:147` style) is reused unchanged — no field extension required.
5. Differential fixture `test/differential/0009-admin-config-dump/` exercising all four endpoints under a 5-request load against the §7 fixture bootstrap; per-endpoint equivalence claims with per-endpoint tolerance discipline (see §7.1).
6. New fuzzer `internal/admin.FuzzConfigDumpFormat` (~80 LoC; 30s short-budget per ADR-0018) — fuzzes adversarial bootstrap proto values into `buildConfigDump` + `protojson.Marshal`; asserts no panic + output is valid JSON parseable by `json.Unmarshal`.
7. `BEHAVIOR_CONTRACT.md` `## Admin API — /ready` section restructured into a `## Admin API` umbrella with five per-endpoint subsections (`### /ready` verbatim-preserved + `### /stats/prometheus` short summary + `### /config_dump` + `### /clusters` + `### /listeners` + `### /server_info` newly populated). Four new equivalence-matrix rows added per §13.2. In-place edit per ADR-0052 (no new ADR required to authorize).
8. Seven new ADRs (ADR-0084..ADR-0090) per §8 + the brainstorm anticipation in BRAINSTORM §9. The seven ADRs settle: planner-time split application, admin-mux reuse pattern, `/config_dump` envelope shape + protojson MarshalOptions, `/clusters` and `/listeners` text-format shape, `/server_info` field set + state-enum coverage, deferral list per ADR-0040 format, no-ACL admin security posture.

After phase 08.1, the project has proven its ninth-leading-edge engineering claim: *envoy-go's admin API exposes the four read-only operator-introspection endpoints that match upstream Envoy v1.37.2's text-format byte layout (`/clusters`, `/listeners`) and JSON-format shape (`/config_dump`, `/server_info`) under a tolerance discipline codified in BEHAVIOR_CONTRACT.md, with the lifecycle-state coverage (`LIVE` post-MarkReady) sufficient to let 08.2's graceful-drain extensions register additional handlers and extend the existing `/ready` and `/server_info` handlers without architectural rework.*

---

## 2. Non-purposes

Per `BOOTSTRAP_PROMPT.md` §6.3 (scope-bounding) and ADR-0040 (out-of-scope deferrals format), the following are explicitly out of 08.1's scope:

### 2.1 Mutating-endpoint non-goals (per BRAINSTORM §2.1, §10)

- `POST /drain_listeners` (08.2's deliverable).
- `POST /reset_counters` (deferred to a future admin-extensions phase or family promotion).
- `POST /quitquitquit` (08.2 to evaluate; semantic overlap with SIGTERM + `/drain_listeners`; may be deferred further).
- `POST /healthcheck/fail`, `POST /healthcheck/ok` (deferred to upstream-robustness family).
- `POST /reopen_logs` (deferred to a future ops-endpoints phase).
- `POST /runtime_modify` (deferred to RTDS / runtime family).
- `POST /logging?level=...` (deferred to a future ops-endpoints phase).

08.1's four endpoints are GET-only. Method discrimination (returning 405 on POST/PUT/DELETE for read-only endpoints) is itself **not enforced** in 08.1 — see §11.8 empirical-pin finding: upstream Envoy v1.37.2 also does NOT 405 these methods; POST `/config_dump` returns 200 with the same body. envoy-go's MVP follows Envoy parity (no method check); future security-hardening phase may add 405 enforcement (see §9).

### 2.2 Read-only-endpoint surface non-goals (per BRAINSTORM §2.1, §10)

- `?format=json` query-param on `/clusters` and `/listeners` — Envoy supports both text and JSON forms (the JSON form is a separate, structurally-richer body). Shipping one cuts the design surface in half; the text form is simpler to byte-compare against Envoy and matches the canonical `curl` operator workflow. The JSON form is empirically captured at §11.9 for documentation but deferred per ADR-0040.
- `?resource=...` filtering on `/config_dump` (deferred to a future admin-extensions phase).
- `?include_eds=true` on `/config_dump` (xDS family — no static EDS in current bootstrap shape).
- `?mask=...` on `/config_dump` (deferred to a future admin-extensions phase).
- `RoutesConfigDump` envelope in `/config_dump` — routes live inside HCM `typed_config` already; Envoy emits them as a dedicated `RoutesConfigDump` envelope. The static routes ARE present in the bootstrap proto already (inlined in HCM filter configs); 08.1 does not separately enumerate them. (Deferred — follow-up minor or future admin-extensions phase.)
- `SecretsConfigDump` envelope (requires secret redaction discipline — passwords, private keys; deferred to secrets / cert-lifecycle family).
- `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes (xDS-only surfaces — deferred to xDS family).
- `/runtime`, `/certs`, `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention`, `/logging` (deferred to RTDS / cert-lifecycle / ops-endpoints families respectively).
- `/listeners/<name>/...` per-listener admin sub-routes (08.2 to evaluate; likely defer further).
- `/init_dump` (deferred — likely never; envoy-go has no init manager).
- `/`, `/help`, `/admin_summary` (the Envoy admin-home help page enumerating all endpoints; deferred to a future admin-extensions phase or never — `curl /` operator workflow is non-MVP).

### 2.3 Body-shape non-goals (per BRAINSTORM §2.3, §2.4, §2.5)

- `node.extensions[]` auto-population in `/config_dump` and `/server_info`. Per §11.4 empirical-pin finding, upstream Envoy v1.37.2 auto-populates `node.user_agent_name = "envoy"`, `node.user_agent_build_version = {...}`, and a ~3800-line `node.extensions[]` array enumerating every extension registered at boot. envoy-go has no equivalent extension-registry-to-node-proto mirroring; envoy-go's `node` field is whatever the bootstrap proto's `node:` field is (typically empty for the §7 fixture bootstrap). The differential equivalence-claim allow-lists `node.user_agent_name`, `node.user_agent_build_version`, `node.extensions` per §13.2.
- `command_line_options` full population in `/server_info`. Per §11.4 empirical-pin finding, upstream Envoy emits ~40 fields (concurrency, drain_time, parent_shutdown_time, mode, drain_strategy, hot_restart_version, etc.) all populated even with no CLI flags. envoy-go's CLI is minimal (`-c <path>`, `--allow-h2c`); envoy-go emits a partially-populated `CommandLineOptions{config_path: <path>}` proto. Differential allow-list per §13.2.
- `hot_restart_version` population in `/server_info`. Upstream Envoy emits `"11.104"` (its hot-restart-shared-mem layout version literal). envoy-go has no hot-restart machinery; envoy-go emits the literal string `"disabled"` (or omits the field — settled per §11.4 evidence and §13.2 allow-list).

### 2.4 Transport-level non-goals (per BRAINSTORM §6.3)

- HTTP/2 over admin (admin stays HTTP/1.1 — upstream Envoy's admin server is also HTTP/1.1 by default at v1.37.2; HTTP/2 over admin requires explicit configuration which is deferred).
- TLS on admin (admin stays plaintext per phase-01 contract).
- Compression on admin responses (`Content-Encoding: gzip` deferred).
- Streaming responses (each handler buffers the full body before write; `/config_dump` may be large but is bounded by the bootstrap size; large-bootstrap streaming deferred to a future admin-extensions phase).

### 2.5 Security non-goals (per BRAINSTORM §2.1 Decision G, §10)

- Authentication / ACL on the admin port. The admin port is a no-ACL plaintext HTTP/1.1 surface — operator firewall responsibility (mirrors Envoy default). Future security-hardening phase may add ACL via a separate admin-listener proto. ADR-0090 documents this disposition.
- Path normalization beyond Go stdlib `http.ServeMux`'s default (trailing slash, percent-encoding) — see §11.7 + §11.8 empirical-pin findings: envoy-go inherits ServeMux's default 404-on-trailing-slash behavior, which matches upstream Envoy's `404 invalid path` response; no explicit normalization needed.

### 2.6 Listener-side non-goals (08.2's scope)

- Drain state machine (LIVE → DRAINING → exit).
- SIGTERM-handler upgrade (current `cmd/envoy-go/main.go:152` style stays).
- Listener stop-accepting (`listener.Manager.Drain`).
- Cluster-pool drain (`cluster.Manager.Drain`).
- `/ready` DRAINING-state body (08.1 keeps phase 01's `LIVE\n` body).
- `/server_info` `state` field DRAINING transition (08.1 hardcodes `state: "LIVE"` post-MarkReady; 08.2 wires the transition).

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 08.1)

| Gate | 08.1 specialization |
|---|---|
| **(a)** `go build ./...` clean | Including the new `internal/admin/configdump.go`, `internal/admin/clusters.go`, `internal/admin/listeners.go`, `internal/admin/serverinfo.go`, the modified `internal/admin/admin.go` (constructor widening), the modified `internal/cluster/manager.go` (new `Clusters()` accessor + `ClusterInfo`/`EndpointInfo` types), and the modified `cmd/envoy-go/main.go` (new `admin.New` call site). All under `go vet ./...` clean and `golangci-lint run ./...` clean. |
| **(b)** `go test ./...` clean | New per-handler unit tests in `internal/admin/configdump_test.go`, `clusters_test.go`, `listeners_test.go`, `serverinfo_test.go`; modified `internal/admin/admin_test.go` (existing `/ready` and `/stats/prometheus` tests preserved verbatim; new tests for the four endpoints checking 200 status, header set, body well-formed). New `internal/cluster/manager_test.go` test for `Clusters()` snapshot accessor. Concurrent-scrape race-test (100 goroutines × 4 endpoints × 1s) clean under `go test -race ./...`. |
| **(c)** h2spec re-run clean (53/53 PASS at ADR-0051 pin) | 08.1 does not touch HCM, listener filter chain, H2 codec, or any request hot path. The h2spec gate at 53/53 PASS must remain green; re-running is mechanical. The ADR-0051 image pin is unchanged. |
| **(d)** new/existing fuzzers run clean for CI short-budget | Existing 9 fuzzers (`internal/bootstrap.FuzzBootstrapLoad`, `internal/filter/tcpproxy.FuzzTcpProxyFilter`, `internal/tls.FuzzTLSContextParse`, `internal/filter/hcm.FuzzHCMConfigParse`, `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode`, `internal/stats.FuzzPromTextFormat`, `internal/accesslog.FuzzDefaultFormatRender`, `internal/filter/http.FuzzFilterChainParse`) run clean at the 30s ADR-0018 budget. **NEW:** `internal/admin.FuzzConfigDumpFormat` runs clean at the same budget. Total: **10 fuzzers post-08.1**. |
| **(e)** Differential fixtures all green | All pre-existing fixtures `0000–0008` remain green. **NEW:** `0009-admin-config-dump` (`test/differential/0009-admin-config-dump/`) is green under the per-endpoint equivalence claims of §7.1. The `RequiresReference: true` flag is set in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors `0007a-cors`). |
| **(f)** `BEHAVIOR_CONTRACT.md` populated | `## Admin API — /ready` section restructured into `## Admin API` umbrella with five per-endpoint subsections (existing `### /ready` content verbatim-preserved; `### /stats/prometheus` short summary added; `### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info` newly populated per §13.1). Four new rows added to `## Equivalence Matrix` per §13.2. In-place edit per ADR-0052 — lands at the phase-done commit alongside the implementation. |

The phase-done commit message body must explicitly state that ROADMAP row `08.1` flips `in-progress → done` AT this commit, and that parent row `08` remains `in-progress` (08.2 still `planned`); per `BOOTSTRAP_PROMPT.md` §5.3 commit message format. Commit subject: `phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]`.

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 08.1)

- `internal/admin/configdump.go` — `handleConfigDump` http.HandlerFunc + `buildConfigDump` envelope-construction + `enumerateStaticListeners` + `enumerateStaticClusters` helpers. Reads `s.bs.Proto` directly (immutable post-boot). protojson MarshalOptions per §11.1 empirical-pin findings: `Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true`. ~150 LoC + tests.
- `internal/admin/clusters.go` — `handleClusters` http.HandlerFunc + per-cluster + per-endpoint format helpers. Reads `s.cm.Clusters()` for the cluster snapshot; reads `*stats.Counter.Value()` / `*stats.Gauge.Value()` for the live counters. Emits 28 lines per cluster: 10 cluster-level + 18 per endpoint × endpoint count, per §11.2 empirical-pin layout. ~180 LoC + tests.
- `internal/admin/listeners.go` — `handleListeners` http.HandlerFunc. Reads `s.lm.Listeners()`. Emits one line per listener: `<name>::<bind_addr>` per §11.3 empirical-pin layout. ~60 LoC + tests.
- `internal/admin/serverinfo.go` — `handleServerInfo` http.HandlerFunc + `buildServerInfo` proto-construction + `buildVersionString` build-time-baked version assembler + `deriveState` LIVE/PRE_INITIALIZING discriminator. Reads `s.bs.Proto.GetNode()` for the `node` field; reads `s.bootTime` for uptime; reads `s.ready.Load()` for state. protojson MarshalOptions identical to `/config_dump`. ~180 LoC + tests.
- `internal/admin/headers.go` — small shared helper `writeAdminHeaders(w http.ResponseWriter, contentType string)` writing the six-header set per §11.6 (`content-type`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `server: envoy`, `date: <IMF-fixdate>` is auto-added by `net/http`, `content-length` is auto-added by `net/http` when body buffering completes — see §6.6 framing-deviation note). ~30 LoC + tests.
- `internal/admin/version.go` — build-time-baked version string assembly (`var Revision = "unknown"` overridable via `-ldflags "-X internal/admin.Revision=<sha>"`); `BuildVersionString()` returns the concatenated `"envoy-go-<sha-short>/<runtime-version>/Clean/RELEASE/<crypto-backend>"` shape that mirrors Envoy's `<sha>/1.37.2/Clean/RELEASE/BoringSSL`. The exact format is settled per §12 #1 deferred decision. ~20 LoC + tests.

### 4.2 Changed production code (in 08.1)

- `internal/admin/admin.go` — `New` signature widened from `New(addr string, registry *stats.Registry) *Server` to `New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server`. New fields on `Server`: `bs *bootstrap.Bootstrap`, `cm *cluster.Manager`, `lm *listener.Manager`, `bootTime time.Time` (set to `time.Now()` at `New` call). `Start()` body adds four `mux.HandleFunc` registrations after the existing two. `WriteTimeout` may need widening (see §12 #2 deferred decision). ~30 LoC delta.
- `internal/admin/doc.go` — package doc updated to enumerate all six endpoints (was two: `/ready`, `/stats/prometheus`).
- `internal/cluster/manager.go` — new public types `ClusterInfo struct { Name string; Endpoints []EndpointInfo }`, `EndpointInfo struct { Address string; Port uint32 }`. New public method `(m *Manager) Clusters() []ClusterInfo` returning a snapshot of the internal `clusters map[string]*Cluster`. Returned slice is freshly allocated per call (caller-mutation safe); per-cluster endpoint snapshot derived from each `Cluster.endpoints` slice (also immutable post-boot per §5.6 LBP-1 generalization). ~50 LoC delta + tests.
- `internal/cluster/manager_test.go` — new test for `Clusters()` snapshot accessor: assert returned slice is read-only (modifying the returned slice does not affect manager state); assert per-endpoint `EndpointInfo` populated; assert ordering is deterministic (alphabetical by cluster name). ~30 LoC delta.
- `internal/admin/admin_test.go` — modified: existing `/ready` and `/stats/prometheus` tests preserved verbatim. New tests added: `TestAdminConfigDumpReturns200`, `TestAdminClustersReturns200`, `TestAdminListenersReturns200`, `TestAdminServerInfoReturns200` (each: 200 status, expected `content-type`, six-header set, body well-formed per the endpoint's contract). New concurrent-scrape race-test `TestAdminConcurrentScrapeRace` (100 goroutines × 4 endpoints × 1s; race-detector clean). ~200 LoC delta.
- `cmd/envoy-go/main.go` — updated `admin.New(adminAddr, bs.Stats)` call site to `admin.New(adminAddr, bs.Stats, bs, cm, lm)` per the constructor widening. One-line semantic change. ~5 LoC delta. Per the boot ordering: `admin.New` runs after `cluster.NewManager` and after `listener.NewManager` allocates its (already-existing) state, so `cm` and `lm` are non-nil at the call site. The `*listener.Manager` is the one already constructed by phase 02; the `*cluster.Manager` is the one already constructed by phase 02; no new boot-order dependency.

### 4.3 New harness and fixture code (in 08.1)

- `test/differential/0009-admin-config-dump/README.md` — fixture overview + per-endpoint equivalence-claim narrative + 5-request load description.
- `test/differential/0009-admin-config-dump/expectations.yaml` — per-endpoint tolerance discipline encoding the §13.2 allow-list (`/config_dump` allow-listed fields, `/clusters` cx_total/rq_total ±N tolerance, `/listeners` byte-equal, `/server_info` allow-listed fields).
- `test/differential/0009-admin-config-dump/envoy.yaml` — reference Envoy bootstrap (admin port 9902 to avoid clash with envoy-go on 9901). Mirrors §7's bootstrap with the admin-port adjustment.
- `test/differential/0009-admin-config-dump/envoy-go.yaml` — envoy-go bootstrap (admin port 9901). Identical to `envoy.yaml` modulo the admin port.
- `test/differential/0009-admin-config-dump/driver/driver.go` — Go driver implementing the §7.2 driver outline: 5-request load + four-endpoint scrape + per-endpoint comparator (`compareConfigDump`, `compareClusters`, `compareListeners`, `compareServerInfo`). The comparators follow the per-endpoint tolerance discipline encoded in `expectations.yaml`. ~250 LoC.
- `test/differential/0009-admin-config-dump/backends/backend.go` — minimal Go HTTP backend bound to ports 18001 + 18002, returning `200 OK\nContent-Length: 8\n\nbackend1` (or `backend2`). ~40 LoC.
- `test/differential/runner.go` — `RegisterFixture("0009-admin-config-dump", ..., Capabilities{RequiresReference: true})` registration line added per the existing fixture-registration pattern (mirrors 0007a-cors). ~3 LoC delta.

### 4.4 Changed documentation and state (in 08.1)

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — `## Admin API — /ready` section restructured into `## Admin API` umbrella with five per-endpoint subsections; four new equivalence-matrix rows. In-place edit per ADR-0052. Lands at phase-done commit alongside impl. See §13.
- `docs/envoy-go/DECISIONS.md` — seven new ADRs (ADR-0084..ADR-0090) appended per §8. Lands incrementally per `superpowers:executing-plans` PROGRESS preamble convention (ADRs land at the task that anchors them).
- `docs/envoy-go/ROADMAP.md` — row `08.1` flips `planned → in-progress` at the SPEC commit (this commit; per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips `in-progress → done` at the 08.1 phase-done commit. Row `08` flips `planned → in-progress` at the SPEC commit (parent rollup); flips `in-progress → done` at the 08.2 phase-done commit (parent SPEC §5). Row `08.2` flips `planned → in-progress` at the 08.2 SPEC commit (later session); flips `in-progress → done` at the 08.2 phase-done commit.
- `docs/envoy-go/STATE.md` — flips `lifecycle-state: 1 → 2`, `next-skill: superpowers:writing-plans` (PLAN.md authoring for 08.1), `active-phase: 08.1-admin-endpoints`, `last-commit: <SPEC commit SHA>`, `last-updated: 2026-05-02`. SHA-fill follow-up commit per phase-04..07.2 convention.
- `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` — parent master SPEC (this commit's deliverable; companion document).
- `docs/envoy-go/phases/08.2-graceful-drain/README.md` — sibling SPEC stub (this commit's deliverable; companion document).
- `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` — this file.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 08.1)

```
cmd/envoy-go/main.go                      [modified: admin.New() call site widens]
   ↓
internal/bootstrap.Load(configPath)        [unchanged]
   ↓ → *Bootstrap (with parsed Proto)
internal/cluster.NewManager(...)           [unchanged]
   ↓ → *cluster.Manager
internal/listener.NewManager(...)          [unchanged]
   ↓ → *listener.Manager
internal/admin.New(adminAddr, bs.Stats,    [WIDENED:
                   bs, cm, lm)                bs, cm, lm threaded]
   ↓ → *admin.Server (with bs, cm, lm, bootTime)
admSrv.Start()                             [Start() body now registers
                                            six handlers; the four new
                                            handlers live in the new
                                            files configdump.go,
                                            clusters.go, listeners.go,
                                            serverinfo.go]
   ↓
... (rest of boot flow: filter registry, listener manager Start,
     MarkReady, stats.Freeze, ready sentinels, <-ctx.Done())
```

The four new admin handlers depend (read-only) on:

```
handleConfigDump  → s.bs.Proto                       (immutable post-boot)
handleClusters    → s.cm.Clusters()                  (immutable post-boot)
                  → *stats.Counter.Value()           (atomic.LoadInt64)
                  → *stats.Gauge.Value()             (atomic.LoadInt64)
handleListeners   → s.lm.Listeners()                 (immutable post-boot)
handleServerInfo  → s.ready.Load()                   (atomic.Bool)
                  → s.bootTime                       (set at New(); immutable)
                  → s.bs.Proto.GetNode()             (immutable post-boot)
                  → admin.BuildVersionString()       (build-time-baked)
```

### 5.2 Per-request flow — `/config_dump` (per BRAINSTORM §5.2)

```
client → GET /config_dump
   ↓ ServeMux dispatch → s.handleConfigDump
   ↓ method-discrimination check: Envoy parity per §11.8 — accept any method,
   ↓   no 405 enforcement (the SPEC explicitly does NOT add a method check;
   ↓   the handler runs identically for GET/POST/PUT/DELETE/HEAD)
   ↓ s.buildConfigDump():
        bootDump := &adminv3.BootstrapConfigDump{
            Bootstrap:   s.bs.Proto,
            LastUpdated: timestamppb.New(s.bootTime),
        }
        lisDump  := &adminv3.ListenersConfigDump{
            VersionInfo:     "static",          // §11.1 pinning required at impl time
            StaticListeners: enumerateStaticListeners(s.bs.Proto),
        }
        cluDump  := &adminv3.ClustersConfigDump{
            VersionInfo:    "static",
            StaticClusters: enumerateStaticClusters(s.bs.Proto),
        }
        bootAny, _ := anypb.New(bootDump)
        lisAny,  _ := anypb.New(lisDump)
        cluAny,  _ := anypb.New(cluDump)
        cd := &adminv3.ConfigDump{
            Configs: []*anypb.Any{bootAny, lisAny, cluAny},
        }
   ↓ body, _ := protojson.MarshalOptions{
        Multiline: true, Indent: " ",
        UseProtoNames: true, EmitUnpopulated: true,
     }.Marshal(cd)
   ↓ writeAdminHeaders(w, "application/json")
   ↓ w.WriteHeader(http.StatusOK)
   ↓ w.Write(body)
```

Errors from `protojson.Marshal` are recovered + logged + synthesized as `500 Internal Server Error` with body `{}` (Envoy's empirical behavior on `/config_dump` failure is a 500 with no JSON body — empirical-pin not directly captured in §11 because adversarial-input failure is hard to reproduce against running Envoy; a defensive 500-with-empty-body response matches Envoy's documented error contract).

### 5.3 Per-request flow — `/clusters` (per BRAINSTORM §5.3)

```
client → GET /clusters
   ↓ ServeMux dispatch → s.handleClusters
   ↓ infos := s.cm.Clusters()                   // snapshot of cluster + endpoint metadata
   ↓ var buf bytes.Buffer
   ↓ for each ClusterInfo (alphabetical by cluster.Name):
        // 10 cluster-level lines per §11.2 empirical-pin layout:
        buf.WriteString(c.Name + "::observability_name::" + c.Name + "\n")
        buf.WriteString(c.Name + "::default_priority::max_connections::1024\n")
        buf.WriteString(c.Name + "::default_priority::max_pending_requests::1024\n")
        buf.WriteString(c.Name + "::default_priority::max_requests::1024\n")
        buf.WriteString(c.Name + "::default_priority::max_retries::3\n")
        buf.WriteString(c.Name + "::high_priority::max_connections::1024\n")
        buf.WriteString(c.Name + "::high_priority::max_pending_requests::1024\n")
        buf.WriteString(c.Name + "::high_priority::max_requests::1024\n")
        buf.WriteString(c.Name + "::high_priority::max_retries::3\n")
        buf.WriteString(c.Name + "::added_via_api::false\n")
        // 18 per-endpoint lines per §11.2 (in declared order; in-bootstrap order):
        for each EndpointInfo e in c.Endpoints (in bootstrap-declared order):
            addr := fmt.Sprintf("%s:%d", e.Address, e.Port)
            cxActive       := s.registry.CounterValue("cluster." + c.Name + ".upstream_cx_active")
            cxConnectFail  := s.registry.CounterValue("cluster." + c.Name + ".upstream_cx_connect_fail")
            cxTotal        := s.registry.CounterValue("cluster." + c.Name + ".upstream_cx_total")
            rqActive       := s.registry.CounterValue("cluster." + c.Name + ".upstream_rq_active")
            rqError        := s.registry.CounterValue("cluster." + c.Name + ".upstream_rq_error")
            rqSuccess      := s.registry.CounterValue("cluster." + c.Name + ".upstream_rq_success")
            rqTimeout      := s.registry.CounterValue("cluster." + c.Name + ".upstream_rq_timeout")
            rqTotal        := s.registry.CounterValue("cluster." + c.Name + ".upstream_rq_total")
            buf.WriteString(c.Name + "::" + addr + "::cx_active::"             + fmt(cxActive)      + "\n")
            buf.WriteString(c.Name + "::" + addr + "::cx_connect_fail::"       + fmt(cxConnectFail) + "\n")
            buf.WriteString(c.Name + "::" + addr + "::cx_total::"              + fmt(cxTotal)       + "\n")
            buf.WriteString(c.Name + "::" + addr + "::rq_active::"             + fmt(rqActive)      + "\n")
            buf.WriteString(c.Name + "::" + addr + "::rq_error::"              + fmt(rqError)       + "\n")
            buf.WriteString(c.Name + "::" + addr + "::rq_success::"            + fmt(rqSuccess)     + "\n")
            buf.WriteString(c.Name + "::" + addr + "::rq_timeout::"            + fmt(rqTimeout)     + "\n")
            buf.WriteString(c.Name + "::" + addr + "::rq_total::"              + fmt(rqTotal)       + "\n")
            buf.WriteString(c.Name + "::" + addr + "::hostname::\n")           // empty (no DNS resolution; bootstrap addresses are literal)
            buf.WriteString(c.Name + "::" + addr + "::health_flags::healthy\n") // hardcoded — no active health checking
            buf.WriteString(c.Name + "::" + addr + "::weight::1\n")            // hardcoded — no per-endpoint weight in bootstrap
            buf.WriteString(c.Name + "::" + addr + "::region::\n")             // empty — no locality tags in bootstrap
            buf.WriteString(c.Name + "::" + addr + "::zone::\n")               // empty
            buf.WriteString(c.Name + "::" + addr + "::sub_zone::\n")           // empty
            buf.WriteString(c.Name + "::" + addr + "::canary::false\n")        // hardcoded
            buf.WriteString(c.Name + "::" + addr + "::priority::0\n")          // hardcoded — no priority in bootstrap
            buf.WriteString(c.Name + "::" + addr + "::success_rate::-1\n")     // sentinel "not measured"
            buf.WriteString(c.Name + "::" + addr + "::local_origin_success_rate::-1\n")
   ↓ writeAdminHeaders(w, "text/plain; charset=UTF-8")
   ↓ w.WriteHeader(http.StatusOK)
   ↓ w.Write(buf.Bytes())
```

The cluster-level constants `1024` and `3` are upstream Envoy's circuit-breaker proto defaults (`envoy.config.cluster.v3.CircuitBreakers.Thresholds` defaults: `max_connections=1024, max_pending_requests=1024, max_requests=1024, max_retries=3`). envoy-go has no circuit-breaker machinery (deferred to upstream-robustness family); emitting the same literals matches Envoy parity for the `/clusters` byte-format and is empirically pinned per §11.2.

The per-endpoint constants — `health_flags::healthy`, `weight::1`, `region`/`zone`/`sub_zone` empty, `canary::false`, `priority::0`, `success_rate::-1`, `local_origin_success_rate::-1` — are likewise Envoy's default-when-not-configured values per the §11.2 scrape; envoy-go emits them as constants (no per-endpoint state tracking required).

### 5.4 Per-request flow — `/listeners` (per BRAINSTORM §5.4)

```
client → GET /listeners
   ↓ ServeMux dispatch → s.handleListeners
   ↓ infos := s.lm.Listeners()                  // snapshot — already exists
   ↓ var buf bytes.Buffer
   ↓ for each ListenerInfo (alphabetical by name):
        buf.WriteString(li.Name + "::" + li.Addr + "\n")
   ↓ writeAdminHeaders(w, "text/plain; charset=UTF-8")
   ↓ w.WriteHeader(http.StatusOK)
   ↓ w.Write(buf.Bytes())
```

The `li.Addr` field on `ListenerInfo` is already populated by phase 02 / 07.2 as the listener's bind address in `host:port` form (e.g., `0.0.0.0:10000`). Per §11.3 empirical-pin layout, this is the exact format Envoy emits.

### 5.5 Per-request flow — `/server_info` (per BRAINSTORM §5.5)

```
client → GET /server_info
   ↓ ServeMux dispatch → s.handleServerInfo
   ↓ info := &adminv3.ServerInfo{
        Version:            BuildVersionString(),
        State:              s.deriveState(),
        UptimeCurrentEpoch: durationpb.New(time.Since(s.bootTime)),
        UptimeAllEpochs:    durationpb.New(time.Since(s.bootTime)),
        HotRestartVersion:  "disabled",                       // §11.4 + §13.2 allow-list
        CommandLineOptions: &adminv3.CommandLineOptions{
            ConfigPath: s.bs.ConfigPath,                      // populated; rest left zero (allow-listed per §13.2)
        },
        Node: s.bs.Proto.GetNode(),                           // bootstrap-provided; empty unless config sets it
     }
   ↓ body, _ := protojson.MarshalOptions{
        Multiline: true, Indent: " ",
        UseProtoNames: true, EmitUnpopulated: true,
     }.Marshal(info)
   ↓ writeAdminHeaders(w, "application/json")
   ↓ w.WriteHeader(http.StatusOK)
   ↓ w.Write(body)
```

`s.deriveState()` returns `adminv3.ServerInfo_LIVE` when `s.ready.Load() == true`, else `adminv3.ServerInfo_PRE_INITIALIZING`. Per §11.4 + §11.7 empirical-pin findings, the `PRE_INITIALIZING` value is forward-compatible (Envoy v1.37.2 emits no observable pre-init state for static-resources bootstraps; envoy-go's handler emits it for mathematical completeness, knowing that the differential harness can never observe it because envoy-go's admin server's response handlers are not gated by `MarkReady` — see §12 #4 deferred decision for the gating choice).

08.2 will replace the `LIVE` hardcode with a `Manager.State()` lookup that can return `DRAINING`. The `INITIALIZING` enum value is documented in the adminv3.ServerInfo_State enum but is unreachable for envoy-go's MVP scope (no xDS, no STRICT_DNS init phase that survives admin-server bind).

`BuildVersionString()` returns the build-time-baked version string (see §6.5). The exact format is settled per §12 #1 deferred decision; the §11.4 empirical reference is `"5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL"`.

### 5.6 Concurrency model (per BRAINSTORM §5.6)

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `admin.New(...)` | Once | None — single-goroutine boot |
| Boot | `admSrv.Start()` registers handlers | Once | mux is per-Server; not shared |
| Per-request | `handleConfigDump` reads `s.bs.Proto` | Per scrape | `s.bs.Proto` is immutable post-boot per §6.6 invariant; lock-free read |
| Per-request | `handleClusters` calls `s.cm.Clusters()` + counter reads | Per scrape | `cm.Clusters()` snapshots an immutable post-boot map; counter reads are `atomic.LoadInt64` per `*stats.Counter.Value()` |
| Per-request | `handleListeners` calls `s.lm.Listeners()` | Per scrape | `lm.Listeners()` snapshots an immutable post-boot list |
| Per-request | `handleServerInfo` reads `s.ready` + `s.bootTime` + `s.bs.Proto.Node` | Per scrape | `s.ready` is `atomic.Bool`; `s.bootTime` is set at `New()` and read-only thereafter; `s.bs.Proto` is immutable post-boot |

**Key invariant:** all four new handlers are **pure read** operations against immutable-post-boot structures + atomically-loaded counters. No new mutex; no new channel. The `*stats.Counter` / `*stats.Gauge` Walk-under-RLock-plus-atomic-Load discipline from phase 06.1 (LBP-1) covers the counter-read path; the bootstrap proto and the cluster/listener manager maps are immutable post-`Freeze()` per the LBP-1 generalization in phase 07.1's `*HTTPRegistry` discipline.

**Race-detector contract:** `go test -race ./...` clean for N concurrent scrapes against all four endpoints from N goroutines. The unit test `TestAdminConcurrentScrapeRace` exercises this with N=100 scrape-loop goroutines for 1 second.

---

## 6. Admin endpoint surface — interfaces, contracts (per BRAINSTORM §§2.2–2.5)

### 6.1 Constructor signature (`admin.New`)

```go
// internal/admin/admin.go — current:
func New(addr string, registry *stats.Registry) *Server

// internal/admin/admin.go — 08.1:
func New(
    addr      string,
    registry  *stats.Registry,
    bs        *bootstrap.Bootstrap,
    cm        *cluster.Manager,
    lm        *listener.Manager,
) *Server
```

The constructor widening is consistent with the LBP-1 explicit-threading discipline established by 06.1's `*stats.Registry` and amplified by 07.1's `*HTTPRegistry`. No package-globals; no `init()`-based registration; the boot graph is the single dependency injection point.

### 6.2 Cluster-manager accessor

```go
// internal/cluster/manager.go — new:
type ClusterInfo struct {
    Name      string
    Endpoints []EndpointInfo
}

type EndpointInfo struct {
    Address string  // dotted-quad or IPv6 literal
    Port    uint32
}

// Clusters returns a snapshot of all configured clusters and their endpoints.
// The returned slice is freshly allocated; mutating it does not affect the
// manager's internal state. Endpoints within each cluster are returned in the
// same order they appear in the cluster's bootstrap load_assignment.
// Counters / gauges are NOT cached in the returned struct — callers (e.g.
// the /clusters admin handler) read live values from the *stats.Registry
// per atomic.LoadInt64 at format time.
func (m *Manager) Clusters() []ClusterInfo
```

The accessor is a snapshot, not a live view; the underlying `clusters map[string]*Cluster` and per-cluster `endpoints []*Endpoint` slices are immutable post-boot per the LBP-1 generalization. The snapshot is freshly allocated to give callers safety against accidental mutation.

### 6.3 Listener-manager accessor (already exists; reused)

```go
// internal/listener/manager.go — existing (unchanged for 08.1):
type ListenerInfo struct {
    Name string
    Addr string  // host:port form
}

func (m *Manager) Listeners() []ListenerInfo
```

The existing `Listeners()` accessor is sufficient for the `/listeners` text-format requirement; no field extension needed in 08.1. (BRAINSTORM §2.2 mentions extending `ListenerInfo` with active conn count from existing 06.1 stats; per §11.3 empirical-pin scrape, upstream Envoy v1.37.2 emits ONLY `<name>::<addr>` per listener — no active conn count. Field extension deferred.)

### 6.4 Per-endpoint contract summary

| Endpoint | Method | Status | Content-Type | Body shape | Source of truth |
|---|---|---|---|---|---|
| `/config_dump` | any | 200 | `application/json` | protojson over `*adminv3.ConfigDump` with three sub-envelopes (Bootstrap, Listeners, Clusters); no `dynamic_*` arrays (no xDS) | `s.bs.Proto` (immutable post-boot) |
| `/clusters` | any | 200 | `text/plain; charset=UTF-8` | 10 cluster-level lines + 18 per-endpoint lines per cluster, alphabetical-by-name + bootstrap-declared-endpoint-order | `s.cm.Clusters()` + live counters |
| `/listeners` | any | 200 | `text/plain; charset=UTF-8` | one line per listener: `<name>::<bind_addr>`, alphabetical-by-name | `s.lm.Listeners()` |
| `/server_info` | any | 200 | `application/json` | protojson over `*adminv3.ServerInfo` with `version` + `state` + `uptime_*` + `node` + partial `command_line_options` + `hot_restart_version: "disabled"` | composed at request time |

### 6.5 Build-time version string

`internal/admin/version.go` introduces a build-time-baked version string:

```go
// internal/admin/version.go
package admin

import "runtime"

// Revision is set via -ldflags "-X github.com/esalaine/envoy-go/internal/admin.Revision=<sha>".
// Defaults to "unknown" for go-test / development builds.
var Revision = "unknown"

// BuildVersionString returns the version string emitted in /server_info's
// `version` field. Format chosen at SPEC §12 #1 — see below.
func BuildVersionString() string {
    return Revision + "/" + runtime.Version() + "/Clean/RELEASE/" + cryptoBackend
}
```

The exact format-string choice (specifically: should envoy-go emit a 5-token concatenation matching Envoy's `<sha>/<version>/Clean/RELEASE/BoringSSL`, or a simpler 3-token form like `envoy-go-<sha>/<go-version>/native-tls`?) is settled per §12 #1 deferred decision. Either form goes through the per-field allow-list in §13.2 (the differential equivalence comparator does not byte-compare `version`).

### 6.6 Framing deviation note

All four endpoints inherit phase 01's documented framing deviation: upstream Envoy emits `transfer-encoding: chunked` (per §11.5 + §11.6 empirical-pin findings); envoy-go's `net/http` server emits `Content-Length` (because each handler buffers the full body before write — `bytes.Buffer` then `w.Write(buf.Bytes())`). The differential harness's existing dechunk path covers this for the four new endpoints (mirrors `/ready`'s handling). The `BEHAVIOR_CONTRACT.md ## Admin API` umbrella section's framing-deviation paragraph is extended to enumerate all six admin endpoints; see §13.1.

---

## 7. Differential fixture `0009-admin-config-dump` (per BRAINSTORM §2.6, §7.2)

### 7.1 Equivalence claims (per BRAINSTORM §2.6, §7.2 — refined per §11 empirical findings; iter-2 refined to structural-projection assertion shape)

The fixture exercises all four new endpoints under a controlled load and asserts per-endpoint equivalence under per-endpoint tolerance. The Task 14 implementation iteration revealed that the upstream Envoy v1.37.2 `/config_dump` and `/server_info` JSON bodies emit substantially more enum default values + auto-populated build-metadata + node-extension entries than the per-field allow-list approach can manage cleanly — the recursive diff produced ~40 surface paths that would need allow-listing on first run (e.g. `Bootstrap.LayeredRuntime.layers[].rtds_layer.rtds_config.api_config_source.api_type` enum default `"AGGREGATED_GRPC"` emitted by Envoy but `"DELTA_GRPC"` by envoy-go on the same proto-default zero value, per ADR-0086 EmitUnpopulated:true consequence). Rather than allow-list a constantly-growing set of enum-default-emission divergences, the Task 14 iter-2 implementation switched to a **structural-projection assertion shape**: each per-endpoint canonicaliser extracts a narrow shape that captures the load-bearing equivalence claim and discards the surrounding noise. The reviewer's verdict (per the Task 14 review captured in PROGRESS) was "Acceptable trade-off; T15 should update SPEC §7.1 + expectations.yaml prose to match the projection-based assertion shape" — this section records that update.

- **`/config_dump`:** parse both bodies as JSON via `encoding/json.Unmarshal`. Project each body to `{configs_types: [<@type URL>...], static_listeners: [<name>...], static_clusters: [<name>...]}` — sorted; `configs_types` intersected to the three known-emitted envelope types (`BootstrapConfigDump`, `ListenersConfigDump`, `ClustersConfigDump`) so reference Envoy's additional envelopes (Routes / Secrets / Endpoints / ScopedRoutes — all deferred per ADR-0089) are filtered to the envoy-go-emitted subset. The §13.2 allow-list (`bootstrap.node.user_agent_*`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated`) is structurally satisfied because the projection drops these fields entirely; the load-bearing assertion is the three-envelope-ordering + the named static_listener/static_cluster present-and-correct on both sides. The empirical justification for the projection (rather than per-field allow-listing) is the iter-2 enum-default-emission divergence (per ADR-0086 EmitUnpopulated:true consequence) which would require ~40 allow-list entries to handle the deeply-nested proto-default enum strings.
- **`/clusters`:** parse both bodies into `(cluster_name, key, value)` tuple sets (one tuple per `<cluster>::<key>::<value>` line); compare set-equally with two normalisations. (i) The 8 per-endpoint `cx_*`/`rq_*` counter tuples are DROPPED on both sides (planner-time decision 8 — envoy-go emits `0` literal for all 8 per-endpoint counters since envoy-go has no per-endpoint stats per ADR-0063 deferral; Envoy emits per-endpoint observed values; the gap is intentional and codified). (ii) The per-endpoint address suffix is STRIPPED from the tuple key so `127.0.0.1:18001` (subj loopback bind) and `host.docker.internal:18001` (ref bridge resolution) converge to a single tuple per endpoint key, AND the `hostname` field-key is dropped on both sides because Envoy's STRICT_DNS resolution emits `host.docker.internal` while envoy-go's STATIC bind emits empty. The remaining tuple set captures the §11.2 envoy-go-emitted 10 cluster-level + 9 per-endpoint constant lines (with hostname dropped); the ±1 tolerance on hot-path counters is moot under the drop-list approach.
- **`/listeners`:** parse both bodies into a sorted name-only set (one entry per `<name>::<addr>` line, with the `<addr>` suffix stripped after the first `::`). The address strip handles cross-side address divergence (subj 127.0.0.1:<ephemeral> vs ref 0.0.0.0:<refContainerListenerPort>) without requiring a framing-aware diff; the load-bearing assertion is "each side emits the same set of listener names". The §13.2 byte-equal-after-dechunk claim from BEHAVIOR_CONTRACT is satisfied at the projection level (the projection IS byte-equal across sides); the address fields are not asserted but inferred indirectly (each side returns 200 with a non-empty body and ≥1 listener entry).
- **`/server_info`:** parse both bodies as JSON. Project each body to `{state: <enum string>}` — the load-bearing assertion is the state-enum byte-equality (`"LIVE"` on both sides post-MarkReady). The §13.2 allow-list (`version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*`, `hot_restart_version`, `node.*`) is structurally satisfied because the projection drops all but the `state` field. The empirical justification for the projection (rather than per-field allow-listing) is the same iter-2 enum-default-emission divergence as `/config_dump` — `command_line_options` alone has ~40 fields whose proto-default enum strings differ between EmitUnpopulated:true and Envoy's emission discipline.

The structural-projection approach is the same correctness discipline used by phase 06.1 for `/stats/prometheus` (twin-series filter discipline drops ~50% of Envoy-emitted counter names that envoy-go does not model; the remaining 17-name set is the load-bearing assertion). It trades narrow allow-listing for narrow projection — the projection IS the assertion shape, and the assertion shape is grep-discoverable in the driver source (`canonicaliseConfigDump`, `canonicaliseClusters`, `canonicaliseListeners`, `canonicaliseServerInfo` in `test/fixtures/0009-admin-config-dump/driver/driver.go`).

The differential equivalence claim is registered as `RequiresReference: true` in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors `0007a-cors`).

### 7.2 Driver outline

1. Boot envoy-go on admin port `9901` + listener port `10000` + reference Envoy on admin port `9902` + listener port `10001` with the §7.3 fixture bootstrap (admin + listener ports differ across the two proxies to avoid bind conflict; backend ports identical).
2. Boot two minimal Go HTTP backends on ports `18001` + `18002` (returns `200 OK` with body `backend1\n` or `backend2\n`).
3. Issue a defined load: 5 GET requests against the envoy-go listener (port 10000) and 5 GET requests against the reference Envoy listener (port 10001), each `GET / HTTP/1.1` with header `Host: test.local`. The 5 requests round-robin across the 2 endpoints, populating `cluster.c_backend.upstream_cx_total = 2 or 3 per endpoint`, `cluster.c_backend.upstream_rq_total = 2 or 3 per endpoint`.
4. Wait briefly (200ms) for stats to settle (counter writes complete after the request is acknowledged by Envoy / envoy-go).
5. Scrape the four endpoints from each proxy:
   - `GET http://127.0.0.1:9901/config_dump` vs `GET http://127.0.0.1:9902/config_dump`
   - `GET http://127.0.0.1:9901/clusters` vs `GET http://127.0.0.1:9902/clusters`
   - `GET http://127.0.0.1:9901/listeners` vs `GET http://127.0.0.1:9902/listeners`
   - `GET http://127.0.0.1:9901/server_info` vs `GET http://127.0.0.1:9902/server_info`
6. Per-endpoint comparator (per §7.1 equivalence claims) asserts the per-endpoint tolerance.

### 7.3 Fixture bootstrap (verbatim from BRAINSTORM §2.6, port-disambiguated)

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
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18002}}}
```

`envoy.yaml` is identical modulo `admin.address.socket_address.port_value: 9902` and `listeners[0].address.socket_address.port_value: 10001`.

### 7.4 Differential gate scope clarification

The differential equivalence claim is per-endpoint tolerance; it is NOT byte-equal across all four endpoints (which would require either envoy-go to emit Envoy-superset-equivalent output for fields it doesn't model — `node.extensions[]` auto-population, `command_line_options` ~40 fields, `hot_restart_version: "11.104"` — or Envoy to be silenced for fields it emits unconditionally — `uptime_*`, `version` build metadata). The tolerance discipline is per-field, codified in `BEHAVIOR_CONTRACT.md ## Equivalence Matrix` per §13.2. This is the same architectural choice 06.1 made for `/stats/prometheus` (tuple-set equality with twin-series filter discipline) and 07.1 made for `/cors` (header-set equality modulo allow-list).

---

## 8. ADRs anticipated (per BRAINSTORM §9)

The 08.1 SPEC anticipates **seven ADRs** (ADR-0084 through ADR-0090) per BRAINSTORM §9, citing `DECISIONS.md` tail SHA `01abdfe` (07.2 phase-done close) at ADR-0083 — verified at SPEC time as the next-free per ADR-0004's hard-gate discipline (§11.5). Topical-vs-commit-time ordering may permute and is recorded in each ADR's `Lands-in-task` field per the 07.1 / 07.2 PROGRESS preamble convention.

- **ADR-0084 — Phase-08 planner-time split into 08.1 + 08.2.** Status: Accepted. Doctrine: D-3.4 + D-3.5. Decision: applies ADR-0045 (planner-time-split discipline) to phase 08; documents the disjoint-scope rationale (read-only admin endpoints in 08.1 vs. mutating drain semantics + lifecycle state machine in 08.2) + 08.1-first ordering rationale (08.2 depends on 08.1's admin-mux scaffold extension). Mirrors ADR-0070 (phase-07 split application) and the 05 / 06 split-application ADRs. Rationale (per BRAINSTORM §1): combined ~1100–1600 LoC + ~28–38 tasks crosses both ADR-0045 thresholds; 08.1's read-only surface and 08.2's lifecycle-state mutation surface have disjoint risk profiles. Lands-in-task: this SPEC's commit (PROGRESS preamble).
- **ADR-0085 — Reuse phase-01 admin HTTP/1.1 mux; no new admin transport.** Status: Accepted. Doctrine: D-3.2 + D-3.4. Decision: extends the existing `internal/admin.Server` with four new `mux.HandleFunc` registrations; constructor-widening pattern threads `*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager` into `admin.New`; no package-globals; LBP-1 explicit-threading discipline carried forward from 06.1's `*stats.Registry` and 07.1's `*HTTPRegistry`. Rationale (per BRAINSTORM §2.2): the existing admin server has a working bind, working `MarkReady()` gate, working timeouts, and is integrated into the lifecycle; splitting into a new server would duplicate all of that for zero benefit. Lands-in-task: 08.1 PLAN Task wherever `internal/admin/admin.go` constructor widening lands.
- **ADR-0086 — `/config_dump` body shape: protojson over `*adminv3.ConfigDump` with three static sub-envelopes; MarshalOptions empirically pinned.** Status: Accepted. Doctrine: D-3.3 + D-3.7. Decision: body is `application/json` via `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` over a `*adminv3.ConfigDump{Configs: [BootstrapAny, ListenersAny, ClustersAny]}` envelope; static-only (no `dynamic_*` arrays since no xDS); RoutesConfigDump / SecretsConfigDump / ScopedRoutesConfigDump / EndpointsConfigDump envelopes deferred per ADR-0089. The four MarshalOptions values are fixed per §11.1 empirical-pin findings (1-space indent, snake_case, emit zero-valued fields). Rationale (per BRAINSTORM §2.3): the Envoy admin proto types are the canonical schema; protojson is the standard library marshaler; the marshaler options are tunable to match Envoy's choice empirically. Lands-in-task: 08.1 PLAN Task wherever `internal/admin/configdump.go` lands.
- **ADR-0087 — `/clusters` and `/listeners` shape: text format only; full Envoy-parity line-set; constants for fields envoy-go does not model.** Status: Accepted. Doctrine: D-3.3 + D-3.7. Decision: both endpoints emit `text/plain; charset=UTF-8`; format mirrors Envoy v1.37.2's text mode (NOT the JSON `?format=json` mode). `/clusters` emits 10 cluster-level lines + 18 per-endpoint lines per cluster (the full line set Envoy emits unconditionally per §11.2 empirical-pin); fields envoy-go does not model (active health check `health_flags`, locality `region/zone/sub_zone`, success_rate, weight, priority, canary) emit Envoy's default-when-not-configured constants (`healthy`, empty string, `-1`, `1`, `0`, `false`). `/listeners` emits one line per listener (`<name>::<bind_addr>`) per §11.3 empirical-pin. JSON form (`?format=json`) deferred per ADR-0089. Rationale (per BRAINSTORM §2.4 + §11): text mode is simpler to byte-compare and matches `curl` operator workflow; emitting Envoy's default constants for non-modeled fields gives the differential harness byte-equality (modulo the cx_total/rq_total tolerance for the round-robin-distributed counter values) without requiring envoy-go to implement features deferred to other phase families. Lands-in-task: 08.1 PLAN Task wherever `internal/admin/clusters.go` + `listeners.go` land.
- **ADR-0088 — `/server_info` MVP field set; `state` enum values `LIVE` + `PRE_INITIALIZING` only in 08.1; `INITIALIZING` is unreachable in MVP.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: body is `application/json` via the same protojson MarshalOptions as `/config_dump`; field set populates `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `node` (from bootstrap), and partially populated `command_line_options{config_path: <path>}`; `hot_restart_version: "disabled"` (sentinel). `state` enum returns `"LIVE"` post-MarkReady, `"PRE_INITIALIZING"` pre-MarkReady (mathematically complete, even though §11.4 + §11.7 empirical findings establish that the pre-init window is unobservable for static-resources bootstraps in v1.37.2; envoy-go forward-only). The `INITIALIZING` enum value is documented in `adminv3.ServerInfo_State` but not emitted by envoy-go (no xDS init, no STRICT_DNS init phase that survives admin-server bind). `DRAINING` is 08.2's deliverable (partially supersedes this ADR when 08.2 lands). Rationale (per BRAINSTORM §2.5 + §11.4): match Envoy's wire shape with empirically-pinned MarshalOptions; restrict the state-enum coverage to what envoy-go's lifecycle machinery actually models. Lands-in-task: 08.1 PLAN Task wherever `internal/admin/serverinfo.go` lands. Cross-ref: ADR-0015 (pre-init contract for `/ready` — same `PRE_INITIALIZING`-for-mathematical-completeness pattern; partially superseded by 08.2's DRAINING extension).
- **ADR-0089 — Admin-endpoint deferral list (per ADR-0040 format).** Status: Accepted. Doctrine: D-3.5. Decision: enumerates the surface deferred from 08.1's MVP per the §2.1 + §2.2 lists in this SPEC: `?format=json` query-param on `/clusters` and `/listeners`; `?resource=`, `?mask=`, `?include_eds=` filtering on `/config_dump`; `RoutesConfigDump`, `SecretsConfigDump`, `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes; `POST /reset_counters`; `/runtime`, `POST /runtime_modify`; `/certs`; `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention`; `POST /quitquitquit` (08.2 to evaluate); `POST /healthcheck/*`; `/logging`, `POST /logging?level=...`; `POST /reopen_logs`; `/listeners/<name>/...` per-listener admin sub-routes; `/init_dump`; HTTP/2 over admin transport; TLS on admin transport; compression on admin responses. Each item carries a target phase/family per the §2.1 + §2.2 enumeration. Rationale (per BRAINSTORM §2.1 Decision F): MVP scope discipline — 08.1 ships the four endpoints sufficient for the §1 mission claim; everything else has an explicit deferral target rather than a vague "TODO". Lands-in-task: 08.1 PLAN Task wherever the deferral table lands in BEHAVIOR_CONTRACT.md (`### Does not yet apply to` extension under `## Admin API`). Cross-ref: ADR-0040 (deferral format precedent).
- **ADR-0090 — No-ACL admin-endpoint security posture.** Status: Accepted. Doctrine: D-3.5. Decision: the admin port is a no-ACL plaintext HTTP/1.1 surface; operator firewall responsibility (mirrors Envoy v1.37.2's default — Envoy's admin server has no built-in ACL either, by upstream design, with upstream operator best-practice being to bind admin to localhost or an operator-only network namespace). No method discrimination on read-only endpoints (Envoy parity per §11.8 empirical-pin finding). No request-rate limiting on admin endpoints. Future security-hardening phase may add ACL via a new admin-listener proto extension; this ADR is forward-only. Rationale (per BRAINSTORM §2.1 Decision G): security-by-firewall is the established Envoy operator workflow; introducing an ACL surface in MVP would require a security-design review out of scope for 08.1. Lands-in-task: 08.1 PLAN Task wherever the security-posture paragraph lands in BEHAVIOR_CONTRACT.md (`### Does not yet apply to` extension).

**Inline supersessions** (recorded in the ADRs above, not as separate ADRs):

- ADR-0014 (`Server: envoy` header value) is referenced by all four new endpoints — they inherit. Forward-only, no amendment.
- ADR-0015 (pre-init contract for `/ready`) is **partially superseded** by ADR-0088's `state: "PRE_INITIALIZING"` extension to `/server_info`; the supersession-by-extension lands in 08.2 (when DRAINING is introduced). 08.1 is forward-only.
- ADR-0040 (out-of-scope deferrals format) is **cited as format authority** by ADR-0089; not amended.
- ADR-0041 (HCM silent-ignore set) — none of 08.1's fields are in the silent-ignore set; no amendment.
- ADR-0045 (planner-time split discipline) — ADR-0084 is its application; no separate split-discipline ADR needed.
- ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization) — §13's BEHAVIOR_CONTRACT umbrella restructure is covered by ADR-0052's existing authorization; no amendment.

(Phase 06.1 had 6 ADRs; 06.2 had 4; 07.1 had 7; 07.2 had 7. **7 sits at the high end** — appropriate for a phase that introduces four new endpoints with an empirical-pin discipline and a security-posture explicit. Planning session may consolidate ADRs 0086+0087+0088 into a single "Admin endpoint body shapes" ADR if they prove inseparable; this is recorded in the PLAN.md authoring session's deferred-decision list.)

---

## 9. Out-of-scope (explicitly deferred)

Beyond the §2 non-goals enumeration, two cross-cutting items are explicitly deferred from 08.1 and recorded here for planner / future-phase reference:

- **Method discrimination enforcement (405 on POST/PUT/DELETE for read-only endpoints).** Per §11.8 empirical-pin finding, upstream Envoy v1.37.2 does NOT 405 these methods; envoy-go matches Envoy parity in 08.1 (no method check). A future security-hardening phase may add 405 enforcement WITH an `expectations.yaml` allow-list extension to the differential harness; that work is NOT in 08.1's scope.
- **M-1 carry-forward fix (cluster-name validation per `stats.IsValidName`).** Phase 07.2 REVIEW.md M-1 identified that cluster names propagate into 8 metric names without `stats.IsValidName` check. The 08.1 `/clusters` text-format handler reads the same cluster names back into the response body; a malformed cluster name would propagate to `/clusters` output. Per §10 carry-forward disposition, 08.1 SPEC explicitly notes the propagation but does NOT bundle the fix; the M-1 fix-when-it-lands (likely a future stats-hardening phase) closes both surfaces simultaneously.

---

## 10. Carry-forward dispositions (per BRAINSTORM §2.8)

Phase 07.2 REVIEW.md identified 10 Minor findings (M-1 through M-10). 08.1's carry-forward disposition:

- **M-1 (cluster-name validation vulnerability — `cluster.<name>` propagates into 8 metric names without `stats.IsValidName` check).** Admin-tangentially-relevant: 08.1's `/clusters` text-format reads those same cluster names into the response body. **08.1 SPEC explicitly notes** (§9 + §13.1) that the M-1 vulnerability propagates to `/clusters` output — the `<cluster>::<key>::<value>` separator (`::`) is not escaped; an embedded `::` in the cluster name would corrupt the format. **No new fix in 08.1.** The 08.1 implementation treats cluster-name-with-`::` as malformed input and emits the literal name (Envoy parity — Envoy does not escape either; the security implication is on the operator). The M-1 fix-when-it-lands closes both surfaces simultaneously.
- **M-2 through M-10:** none are admin-relevant. **M-8** (200ms drain hardcoded in the 0007b fixture driver) is graceful-drain-tangentially-relevant — 08.2 (graceful drain) will note M-8 as a sibling consideration (the 0010 drain fixture driver should not repeat the same hardcoded sleep pattern), but no carry-forward landing is required in 08.1's scope.

No other 07.2-REVIEW carry-forwards are admitted into 08.1's scope.

---

## 11. Empirical-pin block (per BRAINSTORM §2.7 — all seven pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline (autonomous-brainstorm requires empirical evidence for design decisions that are not derivable from documentation alone). Mirrors 06.1's Rule SN4 empirical-pin block in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` per ADR-0061, 06.2's verbatim access-log pin per ADR-0066, and 07.1's four §11 pins.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md`). Server-build SHA confirmed by `/server_info` `version` field: `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`.

**Probe configuration:** the §7.3 fixture bootstrap (STATIC cluster `c_backend` with two endpoints `127.0.0.1:18001` + `127.0.0.1:18002`; single listener `l_main` on `0.0.0.0:10000`; admin on `0.0.0.0:9901`). Reference Envoy booted in a Docker container with `-p 9901:9901 -p 10000:10000` port forwarding. The 5-request load was driven through the listener (`curl http://127.0.0.1:10000/`) with the upstream backends UNREACHABLE inside the container — connection-fail counters are populated as a result, which exercises the cluster-counter readout in `/clusters`. Probe date: 2026-05-02. Capture script: `/tmp/envoy-empirical/run-pins.sh` (transient artifact; not committed; the verbatim outputs below are the durable evidence).

### 11.1 Empirical pin #1 — `/config_dump` JSON shape

**Verbatim Envoy `/config_dump` (first 50 lines of body, headers included):**

```
HTTP/1.1 200 OK
content-type: application/json
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

{
 "configs": [
  {
   "@type": "type.googleapis.com/envoy.admin.v3.BootstrapConfigDump",
   "bootstrap": {
    "node": {
     "user_agent_name": "envoy",
     "user_agent_build_version": {
      "version": {
       "major_number": 1,
       "minor_number": 37,
       "patch": 2
      },
      "metadata": {
       "revision.status": "Clean",
       "ssl.version": "BoringSSL",
       "revision.sha": "5afe27fb338b16d5bb06b3a7198bcd581b4e3dee",
       "build.type": "RELEASE"
      }
     },
     "extensions": [
      {
       "name": "envoy.matching.generic_proxy.input.host",
       "category": "envoy.matching.generic_proxy_request_input.input",
       "type_urls": [
        "envoy.extensions.filters.network.generic_proxy.matcher.v3.HostMatchInput"
       ]
      },
      {
       "name": "envoy.matching.generic_proxy.input.method",
       "category": "envoy.matching.generic_proxy_request_input.input",
       "type_urls": [
        "envoy.extensions.filters.network.generic_proxy.matcher.v3.MethodMatchInput"
       ]
      },
```

**Conclusions (pinned):** envoy-go's `/config_dump` handler MUST:
- (a) emit `Content-Type: application/json` (lowercase header per upstream).
- (b) marshal via `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` — verified by the body shape: 1-space indent (NOT 2 or 4), snake_case field names (`user_agent_name`, NOT `userAgentName`), zero-valued fields like `concurrency: 32` and `disabled: false` ARE emitted (i.e., `EmitUnpopulated: true`). Settles ADR-0086.
- (c) use the `*adminv3.ConfigDump{Configs: []*anypb.Any{...}}` envelope. The top-level body is `{"configs": [...]}`.
- (d) emit three sub-envelopes in this order: `BootstrapConfigDump`, `ListenersConfigDump`, `ClustersConfigDump` — each as a separate `*anypb.Any` entry in the `Configs` slice with its own `@type` discriminator.
- (e) NOT emit any `dynamic_*` arrays (no xDS) — `EmitUnpopulated: true` does NOT cause empty arrays to appear because the `oneof` discriminator at the proto level is `nil`.
- (f) Allow-list `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]` in the differential equivalence claim. envoy-go's `node` proto is the bootstrap-provided value (typically empty for the §7 fixture); upstream Envoy auto-populates these fields with build metadata + a ~3800-line extension registry enumeration. Allow-listed per §13.2.

### 11.2 Empirical pin #2 — `/clusters` text format

**Verbatim Envoy `/clusters` (full body, headers included):**

```
HTTP/1.1 200 OK
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

c_backend::observability_name::c_backend
c_backend::default_priority::max_connections::1024
c_backend::default_priority::max_pending_requests::1024
c_backend::default_priority::max_requests::1024
c_backend::default_priority::max_retries::3
c_backend::high_priority::max_connections::1024
c_backend::high_priority::max_pending_requests::1024
c_backend::high_priority::max_requests::1024
c_backend::high_priority::max_retries::3
c_backend::added_via_api::false
c_backend::127.0.0.1:18001::cx_active::0
c_backend::127.0.0.1:18001::cx_connect_fail::3
c_backend::127.0.0.1:18001::cx_total::3
c_backend::127.0.0.1:18001::rq_active::0
c_backend::127.0.0.1:18001::rq_error::3
c_backend::127.0.0.1:18001::rq_success::0
c_backend::127.0.0.1:18001::rq_timeout::0
c_backend::127.0.0.1:18001::rq_total::0
c_backend::127.0.0.1:18001::hostname::
c_backend::127.0.0.1:18001::health_flags::healthy
c_backend::127.0.0.1:18001::weight::1
c_backend::127.0.0.1:18001::region::
c_backend::127.0.0.1:18001::zone::
c_backend::127.0.0.1:18001::sub_zone::
c_backend::127.0.0.1:18001::canary::false
c_backend::127.0.0.1:18001::priority::0
c_backend::127.0.0.1:18001::success_rate::-1
c_backend::127.0.0.1:18001::local_origin_success_rate::-1
c_backend::127.0.0.1:18002::cx_active::0
c_backend::127.0.0.1:18002::cx_connect_fail::2
c_backend::127.0.0.1:18002::cx_total::2
c_backend::127.0.0.1:18002::rq_active::0
c_backend::127.0.0.1:18002::rq_error::2
c_backend::127.0.0.1:18002::rq_success::0
c_backend::127.0.0.1:18002::rq_timeout::0
c_backend::127.0.0.1:18002::rq_total::0
c_backend::127.0.0.1:18002::hostname::
c_backend::127.0.0.1:18002::health_flags::healthy
c_backend::127.0.0.1:18002::weight::1
c_backend::127.0.0.1:18002::region::
c_backend::127.0.0.1:18002::zone::
c_backend::127.0.0.1:18002::sub_zone::
c_backend::127.0.0.1:18002::canary::false
c_backend::127.0.0.1:18002::priority::0
c_backend::127.0.0.1:18002::success_rate::-1
c_backend::127.0.0.1:18002::local_origin_success_rate::-1
```

**Conclusions (pinned):** envoy-go's `/clusters` handler MUST:
- (a) emit `Content-Type: text/plain; charset=UTF-8`.
- (b) emit exactly **10 cluster-level lines** per cluster, in this exact order: `observability_name`, `default_priority::max_connections::1024`, `default_priority::max_pending_requests::1024`, `default_priority::max_requests::1024`, `default_priority::max_retries::3`, `high_priority::max_connections::1024`, `high_priority::max_pending_requests::1024`, `high_priority::max_requests::1024`, `high_priority::max_retries::3`, `added_via_api::false`. The constants `1024` and `3` are envoy.config.cluster.v3.CircuitBreakers.Thresholds proto defaults; envoy-go has no circuit-breaker machinery but emits the same constants for parity. Settles ADR-0087.
- (c) emit exactly **18 per-endpoint lines** per endpoint, in this exact order: `cx_active`, `cx_connect_fail`, `cx_total`, `rq_active`, `rq_error`, `rq_success`, `rq_timeout`, `rq_total`, `hostname::` (empty, when no DNS resolution), `health_flags::healthy` (constant, no AHC), `weight::1` (constant), `region::`, `zone::`, `sub_zone::` (all empty, no locality tags), `canary::false`, `priority::0`, `success_rate::-1`, `local_origin_success_rate::-1` (sentinel for "not measured").
- (d) the line separator between the cluster-level and per-endpoint blocks is implicit (no blank line); the ordering is cluster-level lines THEN per-endpoint lines for each endpoint in declared order.
- (e) the 5-request load drove `cluster.c_backend.upstream_cx_total = 3 + 2 = 5` and `cluster.c_backend.upstream_rq_error = 3 + 2 = 5` (round-robin LB across 2 endpoints; envoy attempted connection per request and the unreachable backend caused `cx_connect_fail` to increment too — `rq_total` stays 0 because the connection failed before any HTTP/1.1 request was sent). The ±1 tolerance applies to round-robin distribution skew. The exact split (3+2 vs 2+3 vs 1+4) is non-deterministic across runs.

### 11.3 Empirical pin #3 — `/listeners` text format

**Verbatim Envoy `/listeners` (full body, headers included):**

```
HTTP/1.1 200 OK
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

l_main::0.0.0.0:10000
```

**Conclusions (pinned):** envoy-go's `/listeners` handler MUST:
- (a) emit `Content-Type: text/plain; charset=UTF-8`.
- (b) emit exactly **one line per listener**, format `<listener_name>::<bind_addr>` where `<bind_addr>` is the listener's bind address as `host:port`. Trailing newline after the last line.
- (c) NOT emit additional fields (active conn count, drain state, ALPN config). Upstream Envoy v1.37.2's text-mode `/listeners` is exactly one line per listener — the JSON form (`?format=json`) emits richer metadata (see §11.9), but the text form is minimal. Settles ADR-0087.

### 11.4 Empirical pin #4 — `/server_info` JSON shape + state-enum value

**Verbatim Envoy `/server_info` (first 70 lines, headers included):**

```
HTTP/1.1 200 OK
content-type: application/json
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

{
 "version": "5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL",
 "state": "LIVE",
 "uptime_current_epoch": "1s",
 "uptime_all_epochs": "1s",
 "hot_restart_version": "11.104",
 "command_line_options": {
  "base_id": "0",
  "concurrency": 32,
  "config_path": "/cfg/bootstrap-static.yaml",
  "config_yaml": "",
  "allow_unknown_static_fields": false,
  "admin_address_path": "",
  "local_address_ip_version": "v4",
  "log_level": "warning",
  "component_log_level": "",
  "log_format": "[%Y-%m-%d %T.%e][%t][%l][%n] [%g:%#] %v",
  "log_path": "",
  "service_cluster": "",
  "service_node": "",
  "service_zone": "",
  "file_flush_interval": "10s",
  "drain_time": "600s",
  "parent_shutdown_time": "900s",
  "mode": "Serve",
  "disable_hot_restart": false,
  "enable_mutex_tracing": false,
  "restart_epoch": 0,
  "cpuset_threads": false,
  "reject_unknown_dynamic_fields": false,
  "log_format_escaped": false,
  "disabled_extensions": [],
  "ignore_unknown_dynamic_fields": false,
  "use_dynamic_base_id": false,
  "base_id_path": "",
  "drain_strategy": "Gradual",
  "enable_fine_grain_logging": false,
  "socket_path": "@envoy_domain_socket",
  "socket_mode": 0,
  "enable_core_dump": false,
  "stats_tag": [],
  "skip_hot_restart_on_no_parent": false,
  "skip_hot_restart_parent_stats": false,
  "skip_deprecated_logs": false,
  "file_flush_min_size": 0
 },
 "node": {
  "id": "",
  "cluster": "",
  "user_agent_name": "envoy",
  "user_agent_build_version": {
   "version": {
    "major_number": 1,
    "minor_number": 37,
    "patch": 2
   },
   "metadata": {
    "build.type": "RELEASE",
    "revision.sha": "5afe27fb338b16d5bb06b3a7198bcd581b4e3dee",
    "ssl.version": "BoringSSL",
    "revision.status": "Clean"
   }
  },
  "extensions": [
   ... (3800+ lines of extension registry enumeration follow) ...
```

**Conclusions (pinned):** envoy-go's `/server_info` handler MUST:
- (a) emit `Content-Type: application/json` with the same protojson MarshalOptions as `/config_dump`. Settles ADR-0088.
- (b) populate `state: "LIVE"` post-MarkReady. The `state` enum value is the protojson-rendered string form of `adminv3.ServerInfo_State` enum (so envoy-go just uses the proto enum value `adminv3.ServerInfo_LIVE` and protojson renders it as `"LIVE"`).
- (c) populate `uptime_current_epoch` and `uptime_all_epochs` as `durationpb.Duration` values; protojson renders these as `"<N>s"` strings (e.g., `"1s"`, `"30s"`) — NOT as numeric seconds. envoy-go uses `durationpb.New(time.Since(s.bootTime))` which protojson renders identically.
- (d) populate `version` with a build-time-baked string; the format is per §6.5 + §12 #1 deferred decision; the envoy-go format is allow-listed per §13.2 (envoy-go does not byte-compare).
- (e) populate `command_line_options` with at least `config_path`; the remaining ~40 fields are zero-valued in envoy-go (envoy-go has no analog for `concurrency`, `base_id`, `drain_time`, etc.); allow-listed per §13.2 as "subset on envoy-go side".
- (f) populate `node` from the bootstrap proto's `node:` field. When bootstrap has no `node:` field (the §7 fixture), envoy-go emits `{"id": "", "cluster": ""}` (with `EmitUnpopulated: true`). Upstream Envoy auto-populates `user_agent_name: "envoy"`, `user_agent_build_version`, and `extensions` (~3800 lines); allow-listed per §13.2.
- (g) emit `hot_restart_version: "disabled"` (the envoy-go choice) per §6.5 + §13.2; allow-listed (envoy emits `"11.104"`).
- (h) the `state` token `"LIVE"` is byte-equal across envoy-go and Envoy and IS asserted in the differential equivalence claim.

### 11.5 Empirical pin #5 — Framing across all four endpoints

All four endpoints emit `transfer-encoding: chunked` (verified by `curl -i` capturing the response line in §11.1–§11.4 above). Consistent with phase 01's `/ready` framing observation (also `transfer-encoding: chunked` upstream). envoy-go's `net/http` server emits `Content-Length` instead because each handler buffers the full body before write. The differential harness's existing dechunk path (introduced for `/ready` in phase 01, extended for `/stats/prometheus` in phase 06.1) covers all four 08.1 endpoints automatically — no new harness code required.

**Conclusion (pinned):** the framing deviation pattern (Envoy=chunked, envoy-go=Content-Length) extends from phase-01's `/ready` to all six admin endpoints in 08.1. The `BEHAVIOR_CONTRACT.md ## Admin API` umbrella section's framing-deviation paragraph (§13.1) enumerates this for all six endpoints in one place.

### 11.6 Empirical pin #6 — Header set across all four endpoints

All four endpoints (and `/ready`) emit the same six-header set, in the same lowercase order (per upstream Envoy's normalized form):

```
content-type: <varies — application/json or text/plain; charset=UTF-8>
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: <IMF-fixdate>
server: envoy
transfer-encoding: chunked          (Envoy)   |   content-length: <n>          (envoy-go)
```

The `cache-control`, `x-content-type-options`, `server`, `date` headers are constants (the `date` value varies per request but the header presence is constant). The `content-type` value varies by endpoint per the §6.4 table. The framing header (`transfer-encoding` vs `content-length`) is the documented deviation per §11.5.

**Conclusion (pinned):** envoy-go's `writeAdminHeaders` helper (`internal/admin/headers.go`) emits the six-header set. `Date` and `Content-Length` are auto-added by `net/http`; the helper sets `Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server` explicitly. The uppercase canonicalization is a `net/http` artifact; the wire-form lowercase will be emitted as `Content-Type:` etc., which Envoy's lowercase normalization renders as `content-type:`. The differential harness's existing case-insensitive header comparator (introduced for phase 01) covers this.

### 11.7 Empirical pin #7 — Pre-MarkReady `state` value

**Probe configuration:** a separate "init-blocking" bootstrap (`bootstrap-init.yaml`) with `STRICT_DNS` cluster pointing at `unresolvable.empirical-pin.invalid`. The hypothesis was that DNS resolution would block init, holding `state` at `PRE_INITIALIZING` or `INITIALIZING` for an observable window.

**Empirical finding:** Envoy v1.37.2 with `STRICT_DNS` over an unresolvable hostname STILL marks init complete and reports `state: "LIVE"` immediately after admin server bind. The Envoy log confirms: `cm init: all clusters initialized` followed by `all dependencies initialized. starting workers` followed by `starting main dispatch loop` — all within ~3ms of admin server bind. The `INITIALIZING` enum value is reachable in the proto schema but not observable for any bootstrap shape under MVP scope (no xDS, no STRICT_DNS init-blocking that survives admin-bind).

**Verbatim Envoy `/server_info` from the init-blocking bootstrap (state field excerpt):**

```
{
 "version": "5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL",
 "state": "LIVE",
 "uptime_current_epoch": "30s",
 ...
}
```

**Conclusions (pinned):** for static-resources bootstraps in v1.37.2 (and STRICT_DNS bootstraps with unresolvable hostnames), there is **NO observable pre-init window**. envoy-go's `/server_info` handler:
- (a) emits `state: "LIVE"` post-MarkReady (the asserted differential-equivalence claim).
- (b) emits `state: "PRE_INITIALIZING"` pre-MarkReady (mathematically complete; **NOT** asserted in the differential equivalence claim because the window is unobservable in upstream).
- (c) does NOT emit `INITIALIZING`. The proto enum value is documented in `adminv3.ServerInfo_State` but is unreachable for envoy-go's MVP scope (no xDS init, no STRICT_DNS init phase that survives admin-bind in upstream Envoy either).

This mirrors ADR-0015's pre-init contract for `/ready` — both endpoints have a "documented but test-irrelevant pre-init body" carry-forward; the empirical evidence stands at master `f3835a5` until the next ENVOY_TARGET pin refresh re-validates it. Settles ADR-0088 + the `state` enum coverage decision.

### 11.8 Empirical pin #8 — Method-discrimination behavior (POST/PUT on read-only endpoints)

**Verbatim Envoy `POST /config_dump` (HTTP response):**

```
HTTP/1.1 200 OK
content-type: application/json
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

{
 "configs": [
  {
   "@type": "type.googleapis.com/envoy.admin.v3.BootstrapConfigDump",
   ... (body identical to GET /config_dump) ...
```

**Verbatim Envoy `PUT /clusters` (HTTP response):**

```
HTTP/1.1 200 OK
content-type: text/plain; charset=UTF-8
... (body identical to GET /clusters) ...
```

**Conclusion (pinned):** upstream Envoy v1.37.2 does **NOT** enforce method discrimination on these read-only endpoints — POST, PUT, DELETE on `/config_dump` / `/clusters` / `/listeners` / `/server_info` all return 200 OK with the same body as GET. envoy-go's MVP follows Envoy parity (no method check; ServeMux dispatches on path only). 405 enforcement is deferred to a future security-hardening phase (per §9 + ADR-0090). The differential equivalence claim (§7.1) does not exercise non-GET methods; the §11.8 evidence is informational only.

### 11.9 Empirical pin #9 — Edge cases (HEAD, trailing slash, ?format=json)

**HEAD `/config_dump`:**

```
HTTP/1.1 200 OK
content-type: application/json
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

```

(empty body — HEAD response per RFC 9110 §9.3.2)

**`GET /config_dump/`** (trailing slash):

```
HTTP/1.1 404 Not Found
content-type: text/plain; charset=UTF-8
cache-control: no-cache, max-age=0
x-content-type-options: nosniff
date: Sat, 02 May 2026 18:08:27 GMT
server: envoy
transfer-encoding: chunked

invalid path. admin commands are:
  /: Admin home page
  /allocprofiler (POST): enable/disable the allocation profiler (if supported)
  /certs: print certs on machine
  /clusters: upstream cluster status
  /config_dump: dump current Envoy configs (experimental)
  ... (full admin help page elided) ...
```

`/clusters/`, `/listeners/`, `/server_info/` all return identical 404 responses.

**`GET /listeners?format=json`:**

```
HTTP/1.1 200 OK
content-type: application/json
...

{
 "listener_statuses": [
  {
   "name": "l_main",
   "local_address": {
    "socket_address": {
     "address": "0.0.0.0",
     "port_value": 10000
    }
   }
  }
 ]
}
```

**Conclusions (pinned):**
- (a) HEAD on all four endpoints: 200 with the same headers as GET, empty body. envoy-go's `net/http` server handles HEAD via the same handler with auto-suppression of the body — no new code path; matches Envoy parity.
- (b) Trailing slash (`/config_dump/`, `/clusters/`, `/listeners/`, `/server_info/`): 404 with the admin help page. envoy-go uses Go stdlib `http.ServeMux.HandleFunc("/config_dump", h)` which matches `/config_dump` exactly; `/config_dump/` falls through to ServeMux's default `404 page not found` handler — **divergence from Envoy's body** (envoy-go emits `404 page not found\n`; Envoy emits the admin help page). The status code matches (404); the body diverges. **Disposition:** body deviation allow-listed per §13.2 (envoy-go's body is the Go stdlib default; reproducing Envoy's admin help page is out of MVP scope per §2.2 deferral list). The differential harness does not exercise trailing-slash by default; if the planner adds such an exercise, it goes through the allow-list.
- (c) `?format=json` query-param on `/clusters` and `/listeners`: returns a JSON-form response (`{"cluster_statuses": [...]}` and `{"listener_statuses": [...]}` respectively). Out of MVP per ADR-0089. envoy-go silently ignores the query-param and returns the text form (Go stdlib `http.ServeMux` does not parse query params; the handler ignores `r.URL.RawQuery`). The differential harness does not exercise `?format=json` by default.

### 11.10 Synchronization with BEHAVIOR_CONTRACT.md

The §11.1–§11.9 verbatim blocks above are paste-verbatim-synchronized with the `BEHAVIOR_CONTRACT.md ## Admin API` per-endpoint subsections that 08.1's implementation lands in §13. No drift is permitted: future image bumps (per ADR-0008's pin-refresh procedure) require re-running the seven probes and updating both this SPEC §11 and BEHAVIOR_CONTRACT.md `## Admin API` in the same commit.

---

## 12. Deferred decisions (the planner / implementer settles these)

The following decisions are NOT settled by the BRAINSTORM nor by §11's empirical evidence; they are deferred to the planning session (`superpowers:writing-plans` for PLAN.md) or to the implementer at the corresponding task:

1. **`BuildVersionString()` exact format.** Two candidate formats (per §6.5):
   - **(A) 5-token mirror:** `<sha-short>/<go-version>/Clean/RELEASE/native-tls` — mirrors Envoy's `5afe27fb.../1.37.2/Clean/RELEASE/BoringSSL` byte layout most closely; gives differential field-comparator code a stable structure to parse.
   - **(B) 3-token simpler:** `envoy-go-<sha-short>+<go-version>` — terser; no fake `Clean/RELEASE/native-tls` tokens.
   Recommendation: **(A) for byte-shape consistency with the differential parser** (the comparator has fewer special cases), with the `native-tls` token replaced by whatever Go crypto backend envoy-go actually links against (`Go/crypto`, `BoringSSL`, etc. — picked at build time). Settle at PLAN time.
2. **`http.Server.WriteTimeout` widening.** Current is `5 * time.Second`; `/config_dump` for very large bootstraps may approach this (envoy-go's `internal/bootstrap` proto can be ~50KB at the §7 fixture; protojson rendering with `Multiline: true, Indent: " "` expands it ~3×). Recommendation: **widen to 30s**. The bootstrap-size envelope is bounded by the operator's config; 30s is generous enough for any reasonable fixture and does not weaken the admin server's resilience to slow scrape clients.
3. **`server.live` gauge interaction with ready state.** The existing `internal/admin.Server.liveOnce` allocates a `server.live` gauge at `New` time (current line `internal/admin/admin.go:33`). 08.1's constructor widening preserves this; `MarkReady` flips the gauge to 1. Recommendation: no change. (The `server.live` gauge is independent of `/server_info`'s `state` field; the field is emitted at request time, the gauge is updated at lifecycle transitions.)
4. **Pre-MarkReady scrape behavior.** Per §11.7, the admin server's response handlers are NOT gated by `MarkReady` in the current envoy-go code (only `/ready` itself checks `s.ready.Load()`). Recommendation: **do NOT gate the four new handlers on `MarkReady`**. envoy-go matches Envoy parity (the four new handlers respond 200 even pre-MarkReady; `/ready` is the dedicated lifecycle-state probe). The `state: "PRE_INITIALIZING"` value in `/server_info` IS observable in this configuration if a scrape arrives during the boot window between `admSrv.Start()` and `MarkReady`. Per §11.7 + §13.2, this is mathematically correct but not asserted differentially.
5. **`Content-Length` width and explicit `Date` header.** Per §11.6, envoy-go's `net/http` auto-adds these. Recommendation: leave it to `net/http`. (A future security-hardening phase may pin specific date formats; out of 08.1 scope.)
6. **`internal/admin.Server.bs` field type.** `*bootstrap.Bootstrap` is the brainstorm-declared type; it has methods `bs.Proto` (the parsed `*bootstrapv3.Bootstrap`) and `bs.ConfigPath` (the file path passed via `-c`). Recommendation: thread `*bootstrap.Bootstrap` directly (not just `*bootstrapv3.Bootstrap`), because `/server_info`'s `command_line_options.config_path` field needs the file path which lives on `*bootstrap.Bootstrap`, not the proto.
7. **Whether `enumerateStaticListeners` and `enumerateStaticClusters` (the helpers in `configdump.go`) walk the bootstrap proto directly or call the listener/cluster managers.** Recommendation: **walk the bootstrap proto directly.** The `*adminv3.ListenersConfigDump.StaticListeners` field's element type is `*adminv3.ListenersConfigDump_StaticListener{Listener: *anypb.Any, LastUpdated: *timestamppb.Timestamp}`. The proto Listener can be rebuilt from the bootstrap's `static_resources.listeners[]` directly — no need to round-trip through the listener manager's runtime state. Same for clusters. This keeps the `/config_dump` body deterministic and detached from any post-boot listener/cluster state changes (none in MVP, but defensive).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 `## Admin API — /ready` → `## Admin API` umbrella restructure (verbatim Markdown patch)

The existing `## Admin API — /ready` section (current line ~267 of BEHAVIOR_CONTRACT.md per the §1.1 outline) is restructured into a `## Admin API` umbrella with five per-endpoint subsections. The existing `### Ready-state response (authoritative)` and `### Pre-init response` sub-blocks are MOVED under a new `### /ready` subsection (verbatim-preserved, no content edits). Four new per-endpoint subsections are added.

```markdown
## Admin API

The envoy-go admin server is a single HTTP/1.1 plaintext bind allocated by `internal/admin.Server.Start()` (per phase 01 contract; reused unchanged in 06.1 and 08.1). Six endpoints are registered on the same `*http.ServeMux`: `/ready` (phase 01), `/stats/prometheus` (phase 06.1), `/config_dump`, `/clusters`, `/listeners`, `/server_info` (phase 08.1). 08.2 will register `POST /drain_listeners` and extend `/ready` + `/server_info` for the DRAINING state.

**Framing deviation (all six admin endpoints).** envoy-go's `net/http` server emits `Content-Length` (the body is buffered before write); upstream Envoy v1.37.2 emits `transfer-encoding: chunked`. The differential harness dechunks upstream responses before byte-comparing the body. This deviation was first documented for `/ready` at phase 01 (per ADR-0015 paragraph 3) and extends unchanged to all six endpoints. No allow-list entry; the dechunk is structural.

**Header set (all six admin endpoints, post-framing-normalization).** The lowercase wire-form header set is `content-type`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `date: <IMF-fixdate>`, `server: envoy` (per ADR-0014). All six endpoints emit this set. The differential harness uses the existing case-insensitive header comparator (introduced for phase 01).

**Method discrimination posture (all six admin endpoints).** Upstream Envoy v1.37.2 does NOT enforce method discrimination on the four 08.1 read-only endpoints (POST/PUT/DELETE return 200 with the same body as GET — empirical pin in 08.1 SPEC §11.8). envoy-go matches Envoy parity (no method check; Go stdlib `http.ServeMux` dispatches on path only). 405 enforcement is deferred to a future security-hardening phase.

### /ready
   ... (existing Ready-state response (authoritative) + Pre-init response content verbatim-preserved) ...

### /stats/prometheus
   See `## Stat-name mapping` for the body-shape contract (Prometheus text exposition format with the SN1–SN8 flattening rules per ADR-0061). Header set + framing inherit the umbrella rules above.

### /config_dump
   **Body shape.** `application/json` via `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` over `*adminv3.ConfigDump{Configs: []*anypb.Any{...}}` with three sub-envelopes in this order: `BootstrapConfigDump`, `ListenersConfigDump`, `ClustersConfigDump`. No `dynamic_*` arrays (no xDS).

   **Empirical evidence (verbatim Envoy v1.37.2 `/config_dump`, first 50 lines):** see 08.1 SPEC §11.1.

   **Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 modulo: `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated` allow-listed (envoy-go emits empty / partial values; Envoy auto-populates).

### /clusters
   **Body shape.** `text/plain; charset=UTF-8`. 10 cluster-level lines + 18 per-endpoint lines per cluster. Cluster ordering: alphabetical by cluster name. Endpoint ordering: bootstrap-declared order. envoy-go emits the same Envoy default constants (`1024`, `3`, `healthy`, empty locality, `false`, `0`, `-1`) for fields it does not model (circuit breakers, active health checking, locality tags, success rate); see 08.1 SPEC §5.3 for the verbatim line set.

   **Empirical evidence (verbatim Envoy v1.37.2 `/clusters`):** see 08.1 SPEC §11.2.

   **Equivalence claim.** Tuple-set equality on `(cluster_name, key, value)` triples. Hot-path counters `cx_total`, `cx_connect_fail`, `rq_total`, `rq_active`, `rq_error` allow ±1 tolerance (round-robin LB distribution skew across the 5-request §7.3 load).

   **M-1 carry-forward note.** Cluster-name validation is a pre-existing M-1 vulnerability identified in 07.2 REVIEW; the `<cluster>::<key>::<value>` separator is not escaped. An embedded `::` in a cluster name would corrupt the format. envoy-go matches Envoy parity (Envoy also does not escape); the M-1 fix-when-it-lands closes both surfaces simultaneously.

### /listeners
   **Body shape.** `text/plain; charset=UTF-8`. One line per listener: `<listener_name>::<bind_addr>` where `<bind_addr>` is `host:port`. Listener ordering: alphabetical by listener name. Trailing newline.

   **Empirical evidence (verbatim Envoy v1.37.2 `/listeners`):** see 08.1 SPEC §11.3.

   **Equivalence claim.** Body byte-equal (after framing dechunk). Single line per listener; no additional fields. The JSON form (`?format=json`) is structurally richer (returns `{"listener_statuses": [...]}`); deferred per ADR-0089.

### /server_info
   **Body shape.** `application/json` via the same protojson MarshalOptions as `/config_dump`. Field set populates `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `node` (from bootstrap), partial `command_line_options{config_path}`, `hot_restart_version: "disabled"`. State enum: `LIVE` post-MarkReady, `PRE_INITIALIZING` pre-MarkReady (mathematically complete but unobservable upstream — see SPEC §11.7), `DRAINING` deferred to 08.2, `INITIALIZING` not modeled.

   **Empirical evidence (verbatim Envoy v1.37.2 `/server_info`, first 70 lines):** see 08.1 SPEC §11.4.

   **Equivalence claim.** Body byte-equal modulo: `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset on envoy-go side; Envoy emits ~40 fields), `hot_restart_version`, `node.*` (same allow-list as `/config_dump`). The `state` field is byte-equal (`"LIVE"` on both sides).

### Applies to
- phase 08.1 envoy-go admin subsystem.
- all six endpoints: `/ready`, `/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`.
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to
- HTTP/2 over admin (admin stays HTTP/1.1).
- TLS on admin (admin stays plaintext).
- DRAINING-state response on `/ready` (08.2).
- DRAINING value on `/server_info` `state` field (08.2).
- Mutating endpoints — `POST /drain_listeners` is 08.2; `POST /reset_counters`, `POST /quitquitquit`, `POST /healthcheck/*`, `POST /reopen_logs`, `POST /runtime_modify`, `POST /logging` deferred per ADR-0089.
- JSON form of `/clusters` and `/listeners` — `?format=json` deferred per ADR-0089.
- Query-param filtering on `/config_dump` — `?resource=`, `?mask=`, `?include_eds=` deferred per ADR-0089.
- `RoutesConfigDump`, `SecretsConfigDump`, `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes deferred per ADR-0089.
- Other deferred admin endpoints — `/runtime`, `/certs`, `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention`, `/logging`, `/listeners/<name>/*`, `/init_dump` deferred per ADR-0089.
- ACL / authentication on admin port (no-ACL posture per ADR-0090).
- Method discrimination on read-only endpoints (Envoy parity per SPEC §11.8; 405 enforcement deferred).
- Path normalization beyond Go stdlib `http.ServeMux` (trailing-slash returns Go stdlib `404 page not found`, NOT Envoy's admin help page; allow-listed for trailing-slash behavior — envoy-go's body diverges from Envoy's body, but the status code matches).
```

### 13.2 New `## Equivalence Matrix` rows (verbatim table-row patch)

Appended to the `## Equivalence Matrix` table at the head of BEHAVIOR_CONTRACT.md:

```markdown
| Admin /config_dump          | Body byte-equal modulo build/timestamp/uptime allow-list. Three-envelope ordering: Bootstrap, Listeners, Clusters.                                                                                          | `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated` per-field allow-listed. dynamic_* arrays absent in both. |
| Admin /clusters             | Tuple-set equality on `(cluster, key, value)` triples. envoy-go emits Envoy's full unconditional 28-line-per-cluster + 18-line-per-endpoint set with default constants for non-modeled fields.                | Hot-path counters `cx_total`, `cx_connect_fail`, `rq_total`, `rq_active`, `rq_error` allow ±1 tolerance.                                                                              |
| Admin /listeners            | Body byte-equal (after framing dechunk). Single line per listener.                                                                                                                                              | None.                                                                                                                                                                                  |
| Admin /server_info          | Body byte-equal modulo build/uptime/CLI-flags/node allow-list. The `state` field IS asserted byte-equal.                                                                                                       | `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset), `hot_restart_version`, `node.user_agent_*`, `node.extensions[]` per-field allow-listed.    |
```

### 13.3 Header allow-list extensions

No new header allow-list extensions in 08.1. The phase-01 `Date` and `Server` allow-list rows (already in `## Header allow-list`) cover the four new endpoints unchanged. The `Content-Length` vs `transfer-encoding: chunked` deviation is structural (handled by the differential harness's dechunk preprocessor) — no allow-list entry; see §13.1 framing-deviation paragraph.

### 13.4 ADR-0015 forward-pointer note

The existing `### /ready` subsection's reference to ADR-0015 (pre-init contract) is preserved verbatim. ADR-0088's `state: PRE_INITIALIZING` extension to `/server_info` partially supersedes ADR-0015 by introducing a parallel pre-init contract for `/server_info`; the supersession-by-extension lands in 08.2 (when DRAINING is introduced). 08.1 leaves ADR-0015 untouched; the forward-pointer is added by 08.2.

---

## 14. Testing strategy (per BRAINSTORM §7)

### 14.1 Unit tests (`internal/admin/`)

- `configdump_test.go` — unit tests for `buildConfigDump` over a fixture bootstrap proto: assert envelope ordering (Bootstrap, Listeners, Clusters), assert all three sub-envelopes populated, assert `last_updated` is set, assert `version_info: "static"`, assert protojson output is valid JSON parseable by `json.Unmarshal`. Assert MarshalOptions resolved to the §11.1 settings.
- `clusters_test.go` — unit tests for the format function: per-cluster line ordering (alphabetical), per-endpoint line ordering (in-bootstrap-order), counter values reflected in output, empty-cluster-list emits empty body.
- `listeners_test.go` — single-line-per-listener ordering, empty-listener-list emits empty body, IPv6 bind-address `[::1]:10000` formatted correctly.
- `serverinfo_test.go` — `state: "LIVE"` post-MarkReady; `state: "PRE_INITIALIZING"` pre-MarkReady; uptime monotonically increasing across two calls separated by `time.Sleep(10ms)`; `version` field non-empty; `node` populated when bootstrap has `node:` field.
- `admin_test.go` — modified: existing tests for `/ready` + `/stats/prometheus` preserved verbatim; new tests for the four endpoints checking response status (200), Content-Type, header set, body well-formed.

### 14.2 Unit tests (`internal/cluster/`, `internal/listener/`)

- `cluster/manager_test.go` — modified: new tests for `Clusters()` snapshot accessor; assert returned slice is read-only (modification by caller does not affect manager state); assert per-endpoint `EndpointInfo` populated; assert ordering is deterministic (alphabetical by cluster name).
- `listener/manager_test.go` — unchanged in 08.1 (existing tests for `Listeners()` cover the unchanged surface).

### 14.3 Differential fixture `0009-admin-config-dump`

Per §7. The 5-request load + four-endpoint scrape + per-endpoint comparator with the §13.2 allow-list applied.

### 14.4 Race detector + lint

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean (gate (e) per `BOOTSTRAP_PROMPT.md` §7.5).

Concurrent-scrape race-test: 100 goroutines each scraping all four endpoints in a tight loop for 1s. Asserts no race-detector finding, no panic, no malformed responses. Implementation in `admin_test.go::TestAdminConcurrentScrapeRace`.

### 14.5 Fuzzers

Existing 9 fuzzers (8 from phases 02–06.2 + `FuzzFilterChainParse` from 07.1) re-run at 30s budget per ADR-0018. **NEW: `FuzzConfigDumpFormat`** (`internal/admin/`) — fuzzes adversarial bootstrap proto values into `buildConfigDump` + `protojson.Marshal`; asserts no panic + output is valid JSON parseable by `json.Unmarshal`. ~80 LoC. Total: **10 fuzzers post-08.1**.

### 14.6 h2spec re-run

Phase 08.1 does not touch HCM, listener filter chain, H2 codec, or any request hot path. The h2spec gate at 53/53 PASS must remain green; re-running is mechanical (gate (c) per ADR-0051).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Standard six-gate sweep applies; each gate's 08.1 specialization is in §3 above.

---

## 15. Acceptance checklist (for the reviewer of this sub-phase's final state)

When phase 08.1 is `done` (post-phase-done commit), the following are all true:

- [ ] **`internal/admin.New` widened** to take `*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager`. The `cmd/envoy-go/main.go` call site updated; build clean.
- [ ] **Four new mux registrations** in `internal/admin.Server.Start()`: `/config_dump`, `/clusters`, `/listeners`, `/server_info` — each registered with `mux.HandleFunc(<path>, s.handle*)`.
- [ ] **`internal/admin/configdump.go`** lands `handleConfigDump` + `buildConfigDump` + helpers. protojson MarshalOptions match §11.1: `Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true`. Three sub-envelopes (Bootstrap, Listeners, Clusters) in this order.
- [ ] **`internal/admin/clusters.go`** lands `handleClusters` + format helpers. Per-cluster: 10 cluster-level lines per §11.2. Per-endpoint: 18 lines per §11.2 with the constants `healthy`, `1`, `0`, empty, `false`, `-1` for non-modeled fields.
- [ ] **`internal/admin/listeners.go`** lands `handleListeners`. One line per listener: `<name>::<addr>`. Alphabetical-by-name ordering.
- [ ] **`internal/admin/serverinfo.go`** lands `handleServerInfo` + `buildServerInfo` + `deriveState`. State: `"LIVE"` post-MarkReady, `"PRE_INITIALIZING"` pre-MarkReady. `hot_restart_version: "disabled"`. `command_line_options.config_path` populated.
- [ ] **`internal/cluster.Manager.Clusters()`** accessor added. Returns `[]ClusterInfo{Name, Endpoints []EndpointInfo}`. Snapshot is freshly allocated per call.
- [ ] **`internal/admin/version.go`** lands `BuildVersionString()` returning the §6.5 build-time-baked string.
- [ ] **`internal/admin/headers.go`** lands `writeAdminHeaders(w, contentType)` writing the six-header set per §11.6.
- [ ] **Fixture `test/differential/0009-admin-config-dump/`** lands with `README.md`, `expectations.yaml`, `envoy.yaml`, `envoy-go.yaml`, `driver/driver.go`, `backends/backend.go`. Registered as `RequiresReference: true` in `runner.go`. Differentially green.
- [ ] **Fuzzer `internal/admin.FuzzConfigDumpFormat`** lands. Runs clean for 30s.
- [ ] **`BEHAVIOR_CONTRACT.md`** restructured per §13.1 — `## Admin API — /ready` becomes `## Admin API` umbrella with five per-endpoint subsections; existing `### /ready` content verbatim-preserved. Four new equivalence-matrix rows added per §13.2. ADR-0052 in-place edit; no new ADR for the restructure itself.
- [ ] **Seven new ADRs** (ADR-0084..ADR-0090) appended to `DECISIONS.md` per §8.
- [ ] **Concurrent-scrape race-test** `TestAdminConcurrentScrapeRace` (100 goroutines × 4 endpoints × 1s) clean under `go test -race ./...`.
- [ ] **`go vet ./...` clean**, **`golangci-lint run ./...` clean**, **`go test ./...` clean**, **`go test -race ./...` clean** (gate (e)).
- [ ] **h2spec re-run** clean at 53/53 PASS (gate (c); ADR-0051 pin unchanged).
- [ ] **Differential fixtures 0000–0008 + 0009** all green (gate (d)).
- [ ] **All 10 fuzzers** run clean at 30s budget (gate (d) extension).
- [ ] **ROADMAP row `08.1`** flips `in-progress → done` at the phase-done commit; row `08` and row `08.2` unchanged.
- [ ] **STATE.md** advanced past 08.1 phase-done; `active-phase` flips to `08.2-graceful-drain` with `lifecycle-state: 0` (or `1` if 08.2's brainstorm is the next session) and `next-skill: superpowers:brainstorming`.
- [ ] **PROGRESS.md** + **REVIEW.md** committed per phases 06.1 / 06.2 / 07.1 / 07.2 cadence.
- [ ] **Phase-done commit subject:** `phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]`. Body explicitly names the ROADMAP-row transition (`08.1 → done`) and that parent row `08` remains `in-progress`.

When all boxes above are checked, phase 08.1 is `done`, the parent row `08` stays `in-progress` (08.2 still `planned`), and the project advances to phase 08.2 (graceful-drain) at lifecycle-state 0 (full BRAINSTORM session) per `BOOTSTRAP_PROMPT.md` §5.

---

## 16. References

- **BRAINSTORM:** `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` §§2–10 (the authoritative design source; this SPEC distills §§2–10 into formal contract language and executes the §2.7 empirical-pin obligations IN-SESSION per ADR-0004).
- **Parent master SPEC:** `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` (this commit's parent SPEC — the cross-cutting discipline).
- **Sibling SPEC stub:** `docs/envoy-go/phases/08.2-graceful-drain/README.md` (this commit's 08.2 placeholder).
- **Structural precedent (sub-phase SPEC shape):** `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` and `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` — header layout, §-numbering conventions, acceptance-bullet shape, empirical-pin verbatim subsections.
- **Structural precedent (parent master SPEC shape):** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` and `docs/envoy-go/phases/06-observability-baseline/SPEC.md`.
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — sub-phase position in the lifecycle), §5.3 (Commit message format — phase-done subject), §6.2 (How to split — planner-time-split discipline; ADR-0084 applies to phase 08), §6.3 (Anti-pattern — bounding scope), §7.5 (Phase-done gate — six-gate checklist; §3 specializes), §4.1 (artifact-layout invariants — ROADMAP row flip discipline), §8 (MVP trunk — phase 08 closes the trunk; 08.2's phase-done is the trunk-close commit).
- **DECISIONS.md cross-references:**
  - **Inherited (cited, not amended):** ADR-0003 (per-phase worktree convention — each lifecycle session branches a fresh worktree), ADR-0004 (autonomous-brainstorming hard-gate — the empirical-pin discipline traces here), ADR-0008 (Envoy v1.37.2 pin — empirical-pin SHA anchor), ADR-0014 (`Server: envoy` header value — inherited by all four new endpoints), ADR-0018 (fuzzer 30s budget — `FuzzConfigDumpFormat` inherits), ADR-0040 (out-of-scope deferrals format — ADR-0089 uses), ADR-0041 (HCM silent-ignore set — not amended in 08.1), ADR-0045 (planner-time split discipline — ADR-0084 applies), ADR-0051 (h2spec pin SHA — gate (c) carry-through), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization — §13's umbrella restructure inherits), ADR-0061 (stat-name flattening rules SN1–SN8 — `/clusters` reads same `*stats.Counter`/`*stats.Gauge` instances).
  - **Partially superseded:** ADR-0015 (pre-init contract for `/ready`) — ADR-0088's `state: "PRE_INITIALIZING"` extension to `/server_info` partially supersedes by introducing a parallel pre-init contract; the supersession-by-extension lands in 08.2 (when DRAINING is introduced), not in 08.1.
  - **New (this SPEC anticipates):** ADR-0084 through ADR-0090 per §8.
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Server-build SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (per §11.4 verbatim `version` field). All seven §11 empirical pins reference this image SHA; §11.10 specifies the resync discipline on pin refresh.
- **ROADMAP.md:** rows `08`, `08.1`, `08.2` per the split landed in this commit's ROADMAP edit (08 → in-progress with sub-phases `08.1, 08.2`; 08.1 → in-progress; 08.2 → planned with depends-on `08.1`).
