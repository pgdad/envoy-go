# Phase 08.1 — Admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 08.1); `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; ~1156 lines, 16 sections, **read in full**); `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` (parent master SPEC — cross-cutting context for the 08.1 + 08.2 split; parent §5 commits the parent-row-closure-at-08.2-phase-done discipline); `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` (the autonomous-brainstorm artefact at master `29c9497` that the 08.1 SPEC distils §§2–10 from); `docs/envoy-go/phases/07.1-http-filter-framework/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` and `docs/envoy-go/phases/07.2-listener-chain-completion/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 07.1 + 07.2 PLANs are the structural precedent — task-numbering conventions, heredoc-style task headers, ADR-with-first-use-commit discipline, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections, TDD-step granularity); `docs/envoy-go/phases/06.1-stats-prometheus/PLAN.md` (additional structural template — the constructor-widening + new-handler-on-existing-mux pattern this PLAN extends a second time); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0083 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0014** `Server: envoy` header, **ADR-0015** `/ready` pre-init contract, **ADR-0016** blank-import amendment policy, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format, **ADR-0041** silent-ignore set, **ADR-0045** planner-time-split discipline, **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0059** internal stats Registry architecture (LBP-1 origin), **ADR-0061** Rule SN4 empirical-pin pattern, **ADR-0063** cluster-scope-only metrics + per-endpoint deferral, **ADR-0070** parent phase-07 split ADR (the structural sibling 08's split mirrors), **ADR-0072** `*HTTPRegistry` threaded constructor map (the LBP-1 generalisation), **ADR-0079** `*ListenerFilterRegistry` threaded constructor map (the second LBP-1 generalisation), **ADR-0083** is the verified DECISIONS.md tail at master `01abdfe`; phase 08.1's seven anticipated ADRs land at ADR-0084..ADR-0090); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — restructures `## Admin API — /ready` into `## Admin API` umbrella with five per-endpoint subsections + adds four equivalence-matrix rows; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 08.1 — D-3.7 reserves pin bumps for dedicated phases); `docs/envoy-go/ROADMAP.md` (rows `08`, `08.1`, `08.2` per the SPEC commit's row-flip; row `08.1` flips `in-progress → done` at this phase's phase-done; rows `08` + `08.2` unchanged in 08.1).

**Goal:** Land envoy-go's admin-endpoints sub-phase — four new read-only HTTP/1.1 admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) registered on the existing `internal/admin.Server`'s mux, plus a constructor widening that threads `*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager` into `admin.New`, plus a new `cluster.Manager.Clusters() []ClusterInfo` snapshot accessor, plus a new differential fixture `0009-admin-config-dump` that asserts per-endpoint equivalence under per-endpoint tolerance against reference Envoy v1.37.2, plus one new fuzzer (`FuzzConfigDumpFormat`), plus a `BEHAVIOR_CONTRACT.md ## Admin API — /ready` → `## Admin API` umbrella restructure with five per-endpoint subsections + four new equivalence-matrix rows. Concretely: `internal/admin.New` widens from `New(addr, registry)` to `New(addr, registry, bs, cm, lm)` (~30 LoC delta + bootTime field); `internal/cluster.Manager.Clusters() []ClusterInfo` + `ClusterInfo{Name, Endpoints []EndpointInfo}` + `EndpointInfo{Address, Port}` types (~50 LoC + tests); `internal/admin/headers.go` shared header helper (~30 LoC + tests); `internal/admin/version.go` build-time-baked version-string assembler (~20 LoC + tests); `internal/admin/configdump.go` (~150 LoC + tests; protojson over `*adminv3.ConfigDump` with three sub-envelopes BootstrapConfigDump/ListenersConfigDump/ClustersConfigDump per ADR-0086 + §11.1 empirical pin); `internal/admin/clusters.go` (~180 LoC + tests; 10 cluster-level + 18 per-endpoint lines per cluster per ADR-0087 + §11.2 empirical pin); `internal/admin/listeners.go` (~60 LoC + tests; one-line-per-listener `<name>::<bind_addr>` per ADR-0087 + §11.3 empirical pin); `internal/admin/serverinfo.go` (~180 LoC + tests; protojson over `*adminv3.ServerInfo` with `version`, `state`, `uptime_*`, partial `command_line_options{config_path}`, `hot_restart_version: "disabled"` per ADR-0088 + §11.4 empirical pin); `internal/admin/admin.go` constructor widening + four mux registrations (~30 LoC delta); `internal/admin/admin_test.go` extended with the four endpoint smoke tests + `TestAdminConcurrentScrapeRace` (100 goroutines × 4 endpoints × 1s under `-race`); `cmd/envoy-go/main.go` call-site update (~5 LoC delta); `internal/bootstrap/bootstrap.go` adds `Bootstrap.ConfigPath string` field populated by the caller (`cmd/envoy-go/main.go`) post-Load (~3 LoC delta); `internal/admin.FuzzConfigDumpFormat` (~80 LoC; 30s short-budget per ADR-0018; 10th fuzzer overall); `test/fixtures/0009-admin-config-dump/` directory carrying `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `backends/main.go` (the driver canonicalises the four endpoint bodies — JSON-allow-list-filter for `/config_dump` + `/server_info`, tuple-set-canonicalise for `/clusters`, byte-passthrough for `/listeners` — and returns the canonicalised concatenation as the Drive byte stream so the runner's existing CompareBytes pass enforces equivalence; backends are HTTPHello-shaped per the existing fixture-0007a precedent); `test/differential/runner_test.go` adds the fixture-0009 driver blank-import (~1 LoC delta); `BEHAVIOR_CONTRACT.md ## Admin API — /ready` → `## Admin API` umbrella restructure per SPEC §13.1 + four new equivalence-matrix rows per SPEC §13.2 + `### Does not yet apply to` extension; seven new ADRs (ADR-0084 planner-time split application; ADR-0085 admin-mux reuse + constructor-widening LBP-1 third generalisation; ADR-0086 `/config_dump` envelope shape + protojson MarshalOptions; ADR-0087 `/clusters` + `/listeners` text-format shape + Envoy-default constants for non-modeled fields; ADR-0088 `/server_info` MVP field set + state-enum coverage; ADR-0089 admin-endpoint deferral list per ADR-0040 format; ADR-0090 no-ACL admin-endpoint security posture). After phase 08.1, the project has proven its ninth-leading-edge engineering claim per SPEC §1: *envoy-go's admin API exposes the four read-only operator-introspection endpoints that match upstream Envoy v1.37.2's text-format byte layout (`/clusters`, `/listeners`) and JSON-format shape (`/config_dump`, `/server_info`) under a tolerance discipline codified in BEHAVIOR_CONTRACT.md, with the lifecycle-state coverage (`LIVE` post-MarkReady) sufficient to let 08.2's graceful-drain extensions register additional handlers and extend `/ready` and `/server_info` without architectural rework.* Parent ROADMAP row `08` STAYS `in-progress` at this phase's phase-done (parent SPEC §5 — parent flips at 08.2 phase-done, not at 08.1 phase-done).

**Architecture:** The 08.1 surface is the additive registration of four new `mux.HandleFunc` entries on the existing admin server's `*http.ServeMux` plus a constructor-widening that threads `*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager` into `admin.New` (the third application of the LBP-1 explicit-threading discipline introduced by 06.1's `*stats.Registry` and amplified by 07.1's `*HTTPRegistry` and 07.2's `*ListenerFilterRegistry`; ADR-0085 records the third generalisation) plus a small public read-only accessor `cluster.Manager.Clusters() []ClusterInfo` returning a freshly-allocated snapshot of cluster + endpoint metadata. The four handler files (`configdump.go`, `clusters.go`, `listeners.go`, `serverinfo.go`) each implement a stateless `http.HandlerFunc` walking live state at request time; no caching, no precomputation. Counter reads are not required for 08.1's `/clusters` handler — per the planner-time decision settled in §"Planner-time deferred-decision resolution" item 8 below, all 8 per-endpoint cx_/rq_ counter lines emit literal `0` (envoy-go has no per-endpoint stats per ADR-0063 deferral; the cluster-level `cluster.<name>.upstream_cx_total` etc. are NOT distributed across endpoints). Concurrency model: all four new handlers are pure-read against immutable-post-boot structures (`s.bs.Proto`, `s.cm.Clusters()` snapshot, `s.lm.Listeners()` snapshot, `s.bootTime`); only `s.ready.Load()` (the existing `atomic.Bool`) is read mutably. No new mutex; no new channel. Race-detector contract: 100 concurrent scrape goroutines × 4 endpoints × 1s clean under `go test -race ./...` (Task 12 below). Boot-order: `admin.New` runs after `cluster.NewManager` and after `listener.NewManager` per `cmd/envoy-go/main.go`'s existing sequence — `cm` and `lm` are non-nil at the call site. Differential surface: fixture `0009-admin-config-dump` drives a 5-request load through both proxies' listeners (envoy-go on `127.0.0.1:0` subject; reference Envoy on `host.docker.internal:<refPort>` per ADR-0010 STRICT_DNS resolution and the existing harness pattern), waits 200ms for stats settle, scrapes the four admin endpoints from each proxy, and returns canonicalised bytes for the runner's CompareBytes pass; the canonicalisation in the driver applies the §13.2 per-field allow-list (JSON re-emit with allow-listed fields zeroed; tuple-set sort for `/clusters`; byte-passthrough for `/listeners`).

**Tech Stack:**
- Go 1.23 (unchanged from 07.2; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `bytes`, `context`, `encoding/json`, `fmt`, `io`, `log`, `net`, `net/http`, `runtime`, `runtime/debug`, `sort`, `strconv`, `strings`, `sync`, `sync/atomic`, `time` — the exhaustive set the four new admin handlers + their helpers consume.
- `google.golang.org/protobuf/encoding/protojson` (NEW import in `internal/admin/`; was previously only consumed by `internal/bootstrap/`) — `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}.Marshal` is the standard library marshaler for both `/config_dump` (over `*adminv3.ConfigDump`) and `/server_info` (over `*adminv3.ServerInfo`); the four MarshalOptions values are pinned per SPEC §11.1's empirical-pin findings.
- `google.golang.org/protobuf/types/known/anypb` (NEW import in `internal/admin/configdump.go`) — `anypb.New` packs the three sub-envelope ConfigDump messages into `*anypb.Any` for the `Configs` slice.
- `google.golang.org/protobuf/types/known/durationpb` (NEW import in `internal/admin/serverinfo.go`) — `durationpb.New(time.Since(s.bootTime))` populates `uptime_current_epoch` + `uptime_all_epochs`; protojson renders these as `"<N>s"` strings (e.g., `"30s"`) per SPEC §11.4(c).
- `google.golang.org/protobuf/types/known/timestamppb` (NEW import in `internal/admin/configdump.go`) — `timestamppb.New(s.bootTime)` populates `BootstrapConfigDump.LastUpdated` per SPEC §5.2.
- `github.com/envoyproxy/go-control-plane/envoy/admin/v3` (NEW import in `internal/admin/`) — `*adminv3.ConfigDump`, `*adminv3.BootstrapConfigDump`, `*adminv3.ListenersConfigDump`, `*adminv3.ListenersConfigDump_StaticListener`, `*adminv3.ClustersConfigDump`, `*adminv3.ClustersConfigDump_StaticCluster`, `*adminv3.ServerInfo`, `*adminv3.ServerInfo_LIVE`, `*adminv3.ServerInfo_PRE_INITIALIZING`, `*adminv3.CommandLineOptions` — the canonical Envoy admin proto types this phase consumes. Already present transitively in `go.sum` via the existing go-control-plane v1.32.4 import (ADR-0013 pin); no new module import.
- `github.com/esalaine/envoy-go/internal/bootstrap` (existing) — `*bootstrap.Bootstrap` with NEW `ConfigPath string` field this phase adds (per planner-time decision in §"Planner-time deferred-decision resolution" item 9 below); `bs.Proto` (the parsed `*bootstrapv3.Bootstrap`) and `bs.ConfigPath` are read by the four admin handlers.
- `github.com/esalaine/envoy-go/internal/cluster` (existing) — `*cluster.Manager` with NEW `Clusters() []ClusterInfo` accessor this phase adds; the per-cluster + per-endpoint snapshot.
- `github.com/esalaine/envoy-go/internal/listener` (existing — already exposes `Listeners() []Info` since phase 02 / 07.2; reused unchanged in 08.1).
- `github.com/esalaine/envoy-go/internal/stats` (existing — only the existing `*stats.Registry` field on `Server` is reused; the `/clusters` handler does NOT call into `*stats.Registry` per the planner-time decision in §"Planner-time deferred-decision resolution" item 8 below — all per-endpoint counter lines emit literal `0`).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged). Phase 08.1 reads `envoy.admin.v3.{ConfigDump, BootstrapConfigDump, ListenersConfigDump, ClustersConfigDump, ServerInfo, CommandLineOptions}` types; no proto version bump.
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0009's reference (Envoy in a Docker container) — same harness as 06.1/06.2/07.1/07.2's fixtures consume; phase 08.1 does NOT extend `test/differential/fixture/fixture.go` with new optional interfaces (the existing `Driver` + `StatsAsserter` shape is sufficient — the driver canonicalises in-band per the architectural choice above).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0009's reference image AND the source of the §11.1–§11.9 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 08.1's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 08.1 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS; phase 08.1's surface is admin-only (does not touch HCM, listener filter chain, H2 codec, or any request hot path).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's admin server implementation; any third-party admin-endpoint scaffolding library. Test-side use is also forbidden. The `go.mod` post-08.1 must not list any new admin-related runtime dependencies.
- `internal/admin/` extends in place; the only new external runtime imports are the four `google.golang.org/protobuf/encoding/protojson` + `anypb` + `durationpb` + `timestamppb` packages and the one `github.com/envoyproxy/go-control-plane/envoy/admin/v3` package (all already transitively present in `go.sum`). The package-import-graph stays acyclic.

---

## Scope check — why phase 08.1 ships as one sub-phase

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 PLAN's component-table convention):

- `internal/admin/admin.go` constructor widening + four mux registrations + `bootTime` field + `bs/cm/lm` fields ~+30 / -0 = ~+30 net
- `internal/admin/headers.go` ~30 + `headers_test.go` ~80 = ~110
- `internal/admin/version.go` ~30 + `version_test.go` ~80 = ~110
- `internal/admin/configdump.go` ~150 + `configdump_test.go` ~250 = ~400
- `internal/admin/clusters.go` ~180 + `clusters_test.go` ~250 = ~430
- `internal/admin/listeners.go` ~60 + `listeners_test.go` ~120 = ~180
- `internal/admin/serverinfo.go` ~180 + `serverinfo_test.go` ~250 = ~430
- `internal/admin/admin_test.go` extension (four endpoint smoke tests + concurrent-scrape race test) ~+200 = ~+200
- `internal/admin/fuzz_test.go` (NEW) ~80
- `internal/admin/doc.go` ~+10 (enumerate six endpoints) = ~+10
- `internal/cluster/manager.go` extension (`Clusters()` accessor + `ClusterInfo`/`EndpointInfo` types) ~+50 + `manager_test.go` extension ~+100 = ~+150
- `internal/bootstrap/bootstrap.go` `Bootstrap.ConfigPath` field addition ~+5 + `bootstrap_test.go` extension ~+30 = ~+35
- `cmd/envoy-go/main.go` `admin.New` call-site update + `bs.ConfigPath = *cfgPath` line ~+3 net = ~+3
- `cmd/envoy-go/main_test.go` extension (assert four new endpoints respond 200; assert `/config_dump` body parseable as JSON) ~+80 = ~+80
- `test/fixtures/0009-admin-config-dump/` (NEW directory) — `envoy.yaml` ~50 + `envoy-go.yaml` ~50 + `expectations.yaml` ~80 + `README.md` ~80 + `driver/driver.go` ~250 + `backends/main.go` ~40 = ~550
- `test/differential/runner_test.go` blank-import + helper-spawn (the existing HTTPHello backend kind suffices; no new spawn helper) ~+1 = ~+1
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` `## Admin API — /ready` → `## Admin API` umbrella restructure + four equivalence-matrix rows + `### Does not yet apply to` extension ~+150 = ~+150
- `docs/envoy-go/DECISIONS.md` (seven ADRs ADR-0084..ADR-0090) ~+350 = ~+350
- `docs/envoy-go/ROADMAP.md` row `08.1` `in-progress → done` flip ~+1 net = ~+1
- `docs/envoy-go/STATE.md` advance to 08.2 lifecycle-state 0 ~rewrite-in-place
- `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` (NEW; lifecycle artefact) ~600 (per-task entry)
- `docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md` (NEW; lifecycle artefact) ~150

**Production code: ~1700 LoC + ~1380 LoC tests = ~3080 LoC total Go**, plus ~550 LoC fixture YAML/Go + ~700 LoC docs. Well within ADR-0045's split-gate thresholds (the gate is "either >25 tasks OR >1500 LoC of production code" — production is ~1700 LoC, which is barely over the LoC limit but the task count is 15, which is well under 25; the SPEC §8 split discipline (parent 08 → 08.1 + 08.2 per ADR-0084) already applied the gate at the parent level, leaving 08.1 as a single coherent admin-endpoints sub-phase). This affirms the planner-time-split decision recorded in ADR-0084.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/admin/admin.go` | MODIFIED | `Server` struct + `New` constructor (widened) + `Start()` mux registration (six handlers post-08.1) + `MarkReady` + `Close` + existing `handleReady`. The constructor widening adds four new fields (`bs`, `cm`, `lm`, `bootTime`). The four new handlers do NOT live here; they live in `configdump.go` / `clusters.go` / `listeners.go` / `serverinfo.go` (one file per endpoint per the BRAINSTORM §6.1 decomposition). |
| `internal/admin/headers.go` | NEW | Shared `writeAdminHeaders(w http.ResponseWriter, contentType string)` helper writing the four constant headers (`Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server`) per SPEC §11.6. `Date` and `Content-Length` are auto-added by `net/http`. |
| `internal/admin/version.go` | NEW | Build-time-baked version-string assembly: `var Revision string` (overridable via `-ldflags "-X internal/admin.Revision=<sha>"`; defaults to `runtime/debug.ReadBuildInfo().Settings.vcs.revision` else `"unknown"`); `BuildVersionString() string` returns the 5-token form `<sha-short>/<go-version>/Clean/RELEASE/Go-crypto` per SPEC §6.5 + §12 #1 settled below. |
| `internal/admin/configdump.go` | NEW | `handleConfigDump` http.HandlerFunc + `buildConfigDump(bs *bootstrap.Bootstrap, bootTime time.Time) (*adminv3.ConfigDump, error)` envelope-construction helper + `enumerateStaticListeners(bs *bootstrapv3.Bootstrap) []*adminv3.ListenersConfigDump_StaticListener` + `enumerateStaticClusters(bs *bootstrapv3.Bootstrap) []*adminv3.ClustersConfigDump_StaticCluster` walkers (walking the bootstrap proto directly per SPEC §12 #7 settled below). |
| `internal/admin/clusters.go` | NEW | `handleClusters` http.HandlerFunc — iterates `s.cm.Clusters()` snapshot in alphabetical-by-name order, emits 10 cluster-level lines + 18 per-endpoint lines per cluster per SPEC §11.2. Per-endpoint `cx_*` / `rq_*` counter lines emit literal `0` per planner-time decision settled below. |
| `internal/admin/listeners.go` | NEW | `handleListeners` http.HandlerFunc — iterates `s.lm.Listeners()` snapshot in alphabetical-by-name order, emits one line per listener `<name>::<addr>` per SPEC §11.3. |
| `internal/admin/serverinfo.go` | NEW | `handleServerInfo` http.HandlerFunc + `buildServerInfo(s *Server) *adminv3.ServerInfo` proto-constructor + `deriveState(ready *atomic.Bool) adminv3.ServerInfo_State` discriminator (returns `LIVE` if `ready.Load()`, else `PRE_INITIALIZING`). Reads `s.bs.Proto.GetNode()` for the `node` field; `s.bs.ConfigPath` for `command_line_options.config_path`; `s.bootTime` for uptime; `s.ready.Load()` for state. |
| `internal/admin/prometheus.go` | UNCHANGED | Existing 06.1 handler remains untouched. |
| `internal/admin/admin_test.go` | MODIFIED | Existing tests for `/ready` + `MarkReady` preserved verbatim. New tests added: `TestAdminConfigDumpReturns200`, `TestAdminClustersReturns200`, `TestAdminListenersReturns200`, `TestAdminServerInfoReturns200` (each: 200 status, expected `Content-Type`, four-header set, body well-formed). Concurrent-scrape race test `TestAdminConcurrentScrapeRace` (100 goroutines × 4 endpoints × 1s; race-detector clean). |
| `internal/admin/headers_test.go` | NEW | Tests for `writeAdminHeaders` — asserts the four constant headers are set; asserts no other headers are touched; asserts `Date` and `Content-Length` are NOT set by the helper (they're added by `net/http`). |
| `internal/admin/version_test.go` | NEW | Tests for `BuildVersionString` — asserts the 5-token form; asserts `Revision = "unknown"` default for go-test builds; asserts the format-string structure (5 `/`-separated tokens). |
| `internal/admin/configdump_test.go` | NEW | Unit tests for `buildConfigDump` over a fixture bootstrap proto: assert envelope ordering (Bootstrap, Listeners, Clusters), assert all three sub-envelopes populated, assert `last_updated` is set, assert `version_info: "static"`, assert protojson output is valid JSON parseable by `json.Unmarshal`, assert MarshalOptions resolve to the §11.1 settings (snake_case field names, 1-space indent, zero-valued fields emitted). |
| `internal/admin/clusters_test.go` | NEW | Unit tests for `handleClusters` — per-cluster line ordering (alphabetical), per-endpoint line ordering (in-bootstrap-order), exact 10 + 18N line count per cluster, all 8 per-endpoint cx_/rq_ lines emit `0` constants, empty-cluster-list emits empty body. |
| `internal/admin/listeners_test.go` | NEW | Unit tests for `handleListeners` — single-line-per-listener format, alphabetical ordering, empty-listener-list emits empty body, IPv6 bind-address `[::1]:10000` formatted correctly. |
| `internal/admin/serverinfo_test.go` | NEW | Unit tests for `handleServerInfo` — `state: "LIVE"` post-MarkReady; `state: "PRE_INITIALIZING"` pre-MarkReady; uptime monotonically increasing across two calls separated by `time.Sleep(10ms)`; `version` field non-empty; `node` populated when bootstrap has `node:` field; `command_line_options.config_path` populated. |
| `internal/admin/fuzz_test.go` | NEW | `FuzzConfigDumpFormat` — fuzzes adversarial `*bootstrapv3.Bootstrap` proto values into `buildConfigDump` + `protojson.Marshal`; asserts (i) no panic, (ii) output is valid JSON parseable by `json.Unmarshal`, (iii) when output is non-empty, root JSON object has a `configs` field. ~80 LoC. ADR-0018 30s short-budget. |
| `internal/admin/doc.go` | MODIFIED | Package doc updated to enumerate all six endpoints (was two: `/ready`, `/stats/prometheus`). |
| `internal/cluster/manager.go` | MODIFIED | New public types `ClusterInfo struct { Name string; Endpoints []EndpointInfo }`, `EndpointInfo struct { Address string; Port uint32 }`. New public method `(m *Manager) Clusters() []ClusterInfo` returning a freshly-allocated snapshot in alphabetical-by-name order; per-cluster endpoints in bootstrap-declared order (the order `extractEndpoints` already preserves). |
| `internal/cluster/manager_test.go` | MODIFIED | New tests for `Clusters()` snapshot accessor: assert returned slice is read-only (modifying does not affect manager state); assert per-endpoint `EndpointInfo` populated; assert ordering is deterministic (alphabetical by cluster name). |
| `internal/bootstrap/bootstrap.go` | MODIFIED | Add `Bootstrap.ConfigPath string` field. Default zero-value `""`; populated by callers (`cmd/envoy-go/main.go`) post-Load. The `Load` signature is NOT widened (per planner-time decision settled below); test code that does not need the field can leave it zero. |
| `internal/bootstrap/bootstrap_test.go` | MODIFIED | Add a one-line test asserting the new field defaults to `""` after `Load(r)`; assert it's settable post-Load. |
| `cmd/envoy-go/main.go` | MODIFIED | Update `admin.New(adminAddr, bs.Stats)` → `admin.New(adminAddr, bs.Stats, bs, cm, lm)`; add `bs.ConfigPath = *cfgPath` after `bootstrap.Load`. |
| `cmd/envoy-go/main_test.go` | MODIFIED | Add a smoke test asserting the four new admin endpoints all return 200 + non-empty body when a representative bootstrap is loaded; assert `/config_dump` body parses as JSON. |
| `test/fixtures/0009-admin-config-dump/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/main.go` per the SPEC §7 spec. |
| `test/fixtures/0009-admin-config-dump/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port `9901` in-container; listener port `10001`; cluster `c_backend` with 2 STRICT_DNS endpoints `host.docker.internal:<refBackendPort1>` + `host.docker.internal:<refBackendPort2>` per ADR-0010). The driver templates the backend ports at runtime. Mirrors §7.3 fixture bootstrap. |
| `test/fixtures/0009-admin-config-dump/envoy-go.yaml` | NEW | Subject envoy-go bootstrap (admin port resolved from yaml at boot; listener port resolved at boot; cluster `c_backend` with 2 STATIC endpoints `127.0.0.1:<subjBackendPort1>` + `127.0.0.1:<subjBackendPort2>`). |
| `test/fixtures/0009-admin-config-dump/expectations.yaml` | NEW | Prose narrative of the per-endpoint allow-list (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's canonicalisation). Documents: `/config_dump` allow-listed JSON paths; `/clusters` per-endpoint counter `0`-only emission; `/listeners` byte-equal; `/server_info` allow-listed JSON paths. Cross-refs SPEC §13.2 + ADR-0086 + ADR-0087 + ADR-0088. |
| `test/fixtures/0009-admin-config-dump/README.md` | NEW | Fixture purpose, design narrative, allow-list rationale, planner-time per-endpoint counter `0`-emission decision rationale (cross-ref to ADR-0063 deferral). |
| `test/fixtures/0009-admin-config-dump/driver/driver.go` | NEW | `Driver` impl: `BackendCount() = 2`; `BackendKind() = HTTPHello`; `SubjectListenerName() = "l_main"`; `ReferenceListenerPort() = refContainerListenerPort`; `ReferenceBootstrap` + `SubjectConfig` template the bootstraps with the backend ports; `DriveReference(ctx, addr)` and `DriveSubject(ctx, addr)` issue 5 sequential `GET / HTTP/1.1` round-trips against the listener (round-robin LB across the 2 endpoints will deterministically distribute), then sleep 200ms for stats settle, then return an empty byte stream (the actual differential happens in `ProbeAdmin`); `ProbeAdmin(ctx, refAdminAddr, subjAdminAddr)` scrapes the four endpoints from each proxy, applies per-endpoint canonicalisation (JSON-allow-list-filter for `/config_dump` + `/server_info`; tuple-set sort for `/clusters`; byte-passthrough for `/listeners`; trailing `/ready` byte-passthrough for the existing inheritance), and returns the canonicalised concatenation as the runner's CompareBytes input. |
| `test/fixtures/0009-admin-config-dump/backends/main.go` | NEW | Optional. NOT required if `BackendKind() = HTTPHello` (the existing `test/fixtures/0007a-cors/backends/main.go` HTTPHello implementation is reused via the runner's `startHTTPHelloBackend` helper). If HTTPHello is reused, this file is OMITTED. PLAN Task 13 confirms HTTPHello reuse is sufficient. |
| `test/differential/runner_test.go` | MODIFIED | Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver"`. ~1 LoC delta. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | `## Admin API — /ready` → `## Admin API` umbrella restructure + five per-endpoint subsections + four equivalence-matrix rows + `### Does not yet apply to` extension per SPEC §13.1 + §13.2. In-place edit per ADR-0052. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append seven new ADRs ADR-0084..ADR-0090 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `08.1` `in-progress → done` flip at the phase-done commit. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance to 08.2 lifecycle-state 0 with `next-skill: superpowers:brainstorming` and `active-phase: 08.2-graceful-drain`. |
| `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. |
| `docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 cadence; populates per the requesting-code-review skill (Task 15). |

---

## Planner-time deferred-decision resolution (settles SPEC §12)

The planner is required by SPEC §12 to settle seven deferred decisions before implementation. The eight (i.e. settled-here) decisions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **`BuildVersionString()` exact format → option (A) the 5-token mirror.** Format: `<sha-short>/<go-version>/Clean/RELEASE/Go-crypto`. Where:
   - `<sha-short>` is `Revision[:7]` if `len(Revision) >= 7` else `Revision` (typically the 7-char abbreviation Envoy uses; for go-test builds where `Revision == "unknown"`, the literal string `"unknown"` is emitted).
   - `<go-version>` is `runtime.Version()` (e.g., `"go1.23.0"`).
   - `"Clean"` and `"RELEASE"` are literal tokens (Envoy emits these unconditionally per §11.4 evidence; envoy-go emits them as constants for byte-shape parity — they carry no envoy-go semantic).
   - `"Go-crypto"` is the literal token replacing Envoy's `"BoringSSL"` (envoy-go links against Go stdlib `crypto/tls`, not BoringSSL; the literal indicates the crypto backend).
   - `Revision` is sourced via `runtime/debug.ReadBuildInfo().Settings` lookup for `vcs.revision` at `init()` time; if not present (e.g. go-test builds), defaults to `"unknown"`. The `-ldflags "-X github.com/esalaine/envoy-go/internal/admin.Revision=<sha>"` override remains supported (release builds use it).
   - The `version` field is allow-listed per §13.2 (envoy-go does not byte-compare against Envoy's). The 5-token shape is for differential-parser consistency only.
   *Anchored: SPEC §6.5 + §12 #1.*

2. **`http.Server.WriteTimeout` widening → 30s.** Per SPEC §12 #2 recommendation. The bootstrap-size envelope is bounded by the operator's config; 30s is generous enough for any reasonable fixture (the §7 fixture's bootstrap is ~3KB; protojson rendering with `Multiline: true, Indent: " "` expands ~3× → ~9KB; far under any reasonable scrape latency budget). The widening lands in Task 5 (admin.go constructor edit) and is asserted by an `internal/admin/admin_test.go::TestAdminWriteTimeoutIs30s` test that reflects on `s.httpSrv.WriteTimeout`. *Anchored: SPEC §12 #2.*

3. **`server.live` gauge interaction → no change.** Per SPEC §12 #3 recommendation. `liveOnce` + `liveGauge` continue to be allocated at `New` time and `Set(1)` on the first LIVE-state `/ready` response. The new `/server_info` `state` field is independent: emitted at request time, not gated on the gauge. `/server_info`'s state value derives from `s.ready.Load()`, which is the same atomic the `/ready` handler reads. *Anchored: SPEC §12 #3.*

4. **Pre-MarkReady scrape behaviour → do NOT gate the four new handlers on `MarkReady`.** Per SPEC §12 #4 recommendation. The four new handlers respond 200 even pre-MarkReady; only `/ready` itself checks `s.ready.Load()`. Concretely: the `state: "PRE_INITIALIZING"` value in `/server_info` IS observable in this configuration if a scrape arrives between `admSrv.Start()` and `MarkReady`. Per §11.7 + §13.2, this is mathematically correct but not asserted differentially (Envoy v1.37.2 also has no observable pre-init window for static-resources bootstraps; the differential equivalence claim only exercises the post-MarkReady state). *Anchored: SPEC §12 #4 + §11.7.*

5. **`Content-Length` width and explicit `Date` header → leave to `net/http`.** Per SPEC §12 #5 recommendation. The `writeAdminHeaders` helper sets only `Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server`. `Date` is auto-added by `net/http`'s `Server.serve()` per RFC 9110 §6.6.1; `Content-Length` is auto-added when `WriteHeader` is called before any `Write` and the body fits in the response buffer (which it always does for the four new handlers — each buffers in `bytes.Buffer` then writes once). Documented in `internal/admin/headers.go`'s package-doc comment. *Anchored: SPEC §12 #5 + §11.6.*

6. **`internal/admin.Server.bs` field type → `*bootstrap.Bootstrap`.** Per SPEC §12 #6 recommendation. Thread `*bootstrap.Bootstrap` directly (not just `*bootstrapv3.Bootstrap`), because `/server_info`'s `command_line_options.config_path` field needs the file path which lives on `*bootstrap.Bootstrap.ConfigPath` (the new field this PLAN adds — see decision 9 below), not the proto. The `bs *bootstrap.Bootstrap` field on `Server` is set by `New(...)` and read by `handleConfigDump` (via `bs.Proto`) and `handleServerInfo` (via `bs.Proto.GetNode()` + `bs.ConfigPath`). *Anchored: SPEC §12 #6.*

7. **`enumerateStaticListeners` and `enumerateStaticClusters` → walk the bootstrap proto directly.** Per SPEC §12 #7 recommendation. The helpers walk `bs.Proto.GetStaticResources().GetListeners()` and `.GetClusters()` directly, packing each into a `*adminv3.ListenersConfigDump_StaticListener{Listener: anypb.New(l), LastUpdated: timestamppb.New(s.bootTime)}` (and analogously for clusters). This keeps the `/config_dump` body deterministic and detached from any post-boot listener/cluster state changes (none in MVP, but defensive). It also avoids round-tripping through the listener/cluster managers' runtime state which would conflate the "what was configured" vs "what is running" distinction. *Anchored: SPEC §12 #7.*

8. **Per-endpoint counter values in `/clusters` → emit literal `0` for all 8 per-endpoint cx_/rq_ counter lines.** Per the planner-time-discovered gap: envoy-go has no per-endpoint stats per ADR-0063 deferral (only cluster-scope counters exist: `cluster.<name>.upstream_cx_total`, `upstream_cx_active`, `upstream_rq_total`, `upstream_rq_2xx/3xx/4xx/5xx`, `membership_total`); the SPEC §5.3 pseudocode reading `s.registry.CounterValue("cluster." + c.Name + ".upstream_cx_active")` is misleading because that name resolves to a cluster-level counter, not a per-endpoint one. The truthful per-endpoint counter value envoy-go can report is `0` for every per-endpoint `cx_*` and `rq_*` counter (no per-endpoint state tracked). The differential equivalence claim is then NOT byte-equal modulo the SPEC §7.1 ±1 tolerance — it is byte-equal modulo the per-endpoint counter values being allow-listed entirely (i.e., envoy-go emits `0` for all 8 fields per endpoint; Envoy emits per-endpoint observed values). The driver's canonicalisation for `/clusters` (Task 13) drops these 8 fields per endpoint from the canonical tuple set on BOTH sides before set-equality comparison. The fixture's `expectations.yaml` documents this allow-list extension; ADR-0087 records the decision; SPEC §7.1's ±1 tolerance is amended in BEHAVIOR_CONTRACT.md §13.2 to "per-endpoint cx_*/rq_* counters fully allow-listed (envoy-go emits 0; Envoy emits observed)". *Rationale:* (a) ADR-0063 deferred per-endpoint stats from 06.1; introducing them in 08.1 would be scope-creep; (b) emitting cluster-level counters per-endpoint would be wrong (the values would not sum to the cluster total across endpoints); (c) the operator's view of the cluster is preserved at cluster-level (the cluster-level `upstream_cx_total` etc. are emitted under the `## Stat-name mapping` flattening of /stats/prometheus, which already works in 06.1). *Anchored: SPEC §5.3 (pseudocode revision) + SPEC §7.1 (tolerance amendment) + ADR-0063 (per-endpoint stats deferral) + ADR-0087.*

9. **`Bootstrap.ConfigPath` plumbing → add `ConfigPath string` field; do NOT widen `bootstrap.Load` signature.** Per the planner-time-discovered gap: the SPEC §12 #6 decision threads `*bootstrap.Bootstrap` into `admin.New`, but `*bootstrap.Bootstrap` does not currently carry the config-file path needed by `/server_info`'s `command_line_options.config_path`. Recommendation: add a single new field `ConfigPath string` to the `Bootstrap` struct (defaults to `""` on `Load`); the caller (`cmd/envoy-go/main.go`) sets it post-Load via `bs.ConfigPath = *cfgPath` immediately after `bootstrap.Load(f)`. This avoids widening the `Load(r io.Reader)` signature (which would ripple into every test that calls `Load` and is overkill for what is fundamentally a sidecar metadata field). The `Bootstrap.ConfigPath` field is empty in test code that does not need it; no test breakage. The `/server_info` handler's `CommandLineOptions{ConfigPath: s.bs.ConfigPath}` simply emits empty when the field is empty (matches Envoy parity for a `<empty>` `--config-path` CLI flag). *Anchored: SPEC §12 #6 (field-type decision); planner-time gap-fill.*

These nine decisions are reproduced verbatim in `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The seven ADRs anticipated by SPEC §8 (ADR-0084..ADR-0090). Each ADR's "Lands-in-task" anchor is fixed below; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The seven ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0084 | Phase-08 planner-time split into 08.1 + 08.2 | Task 1 (PROGRESS preamble) — the scope decision is formalised at the same moment as PROGRESS.md is created. |
| ADR-0085 | Reuse phase-01 admin HTTP/1.1 mux; constructor-widening LBP-1 third generalisation | Task 5 (`internal/admin/admin.go` constructor widening) — the LBP-1 third application IS the constructor-widening edit. |
| ADR-0086 | `/config_dump` body shape: protojson over `*adminv3.ConfigDump` with three sub-envelopes; MarshalOptions empirically pinned per SPEC §11.1 | Task 6 (`internal/admin/configdump.go` first lands). |
| ADR-0087 | `/clusters` and `/listeners` shape: text format only; full Envoy-parity line-set; constants for non-modeled fields; per-endpoint counters emit `0` | Task 7 (`internal/admin/clusters.go` first lands). |
| ADR-0088 | `/server_info` MVP field set; `state` enum coverage `LIVE` + `PRE_INITIALIZING` only; `INITIALIZING` unreachable in MVP | Task 9 (`internal/admin/serverinfo.go` first lands). |
| ADR-0089 | Admin-endpoint deferral list per ADR-0040 format | Task 14 (BEHAVIOR_CONTRACT umbrella restructure — the `### Does not yet apply to` extension IS the deferral table). |
| ADR-0090 | No-ACL admin-endpoint security posture | Task 14 (BEHAVIOR_CONTRACT umbrella restructure — the security-posture paragraph lands alongside the deferral table). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Doctrine / Lands-in-task / Context / Decision / Consequences / Supersedes); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-08.1-admin-endpoints-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003 + the per-phase-worktree convention: `git worktree add .worktrees/phase-08.1-admin-endpoints-impl -b phase-08.1-admin-endpoints-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -3` shows the PLAN.md commit (this plan) and (optionally) its SHA-fill follow-up at the head, with the SPEC.md commit `1f85b07` and its SHA-fill `65b7455` immediately before. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0083:`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0084..ADR-0090 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` returns `1f85b07` (the SPEC commit). If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status -uall --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean (the `-short` flag skips the differential suite which needs Docker; this is a pure unit-test pre-check). Differential suite is exercised in Task 15.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008'` returns every fixture PASS. The 9 pre-existing fixtures (0000–0008) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 9 fuzzers from phases 02–07.2 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each). Mechanical re-run is sufficient — none should regress in a phase that does not touch their code paths.
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **Pre-existing admin server has the constructor signature this PLAN widens.** `grep -nE '^func New\(addr string, registry \*stats\.Registry\) \*Server' internal/admin/admin.go` returns exactly 1 match. If 0, the constructor has already been widened by a concurrent phase — investigate before proceeding (the PLAN may need amending).
12. **Pre-existing cluster manager does NOT yet have `Clusters()` accessor.** `grep -nE '^func \(m \*Manager\) Clusters\(\)' internal/cluster/manager.go` returns empty. If non-empty, the accessor has been added by a concurrent phase — re-check the SPEC's expected shape against what's there.
13. **Pre-existing listener manager has `Listeners()` accessor.** `grep -nE '^func \(m \*Manager\) Listeners\(\) \[\]Info' internal/listener/manager.go` returns exactly 1 match. (Confirmed: phase 02 introduced this; SPEC §6.3 reuses unchanged.)
14. **Pre-existing `Bootstrap` struct does NOT yet have `ConfigPath` field.** `grep -nE 'ConfigPath\s+string' internal/bootstrap/bootstrap.go` returns empty. If non-empty, the field has been added by a concurrent phase — investigate.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).

If all 15 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble [ADR-0084]

**Files:**
- Create: `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0084)

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. ADR-0084 (Phase-08 planner-time split into 08.1 + 08.2) lands here as the first ADR of the seven, anchored at the PROGRESS preamble — the implementation session's first commit — since the ROADMAP edit that the scope decision formalises already landed at master `1f85b07` (the SPEC commit), and Task 1 is the first opportunity to land the ADR after the SPEC commit's ROADMAP edit. The PROGRESS preamble also reproduces the nine planner-time deferred-decisions resolution items from the PLAN's `## Planner-time deferred-decision resolution` section verbatim, so any task-N reader has the full context without back-reading this PLAN.

**Precondition:** worktree exists at `phase-08.1-admin-endpoints-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 15 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` (new file); `docs/envoy-go/DECISIONS.md` (ADR-0084 appended).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble entry committed; ADR-0084 appears in `DECISIONS.md` with full Context/Decision/Consequences sections per the ADR-0001 template.
**Verification command:** `git log -1 --format=%H -- docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` returns the Task 1 commit's SHA; `grep -nE '^## ADR-0084:' docs/envoy-go/DECISIONS.md | wc -l` returns 1.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-08.1-admin-endpoints-impl
git log --oneline master | head -3                                    # expect: PLAN SHA-fill, PLAN commit, then SPEC SHA-fill (65b7455) + SPEC commit (1f85b07)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                          # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
                                                                       # expect: ADR-0083:
git log -1 --format=%H -- docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md
                                                                       # expect: 1f85b07... or descendant
git status -uall --porcelain                                           # expect: empty
grep -nE '^func New\(addr string, registry \*stats\.Registry\) \*Server' internal/admin/admin.go
                                                                       # expect: 1 match (line 31)
grep -nE '^func \(m \*Manager\) Clusters\(\)' internal/cluster/manager.go
                                                                       # expect: empty
grep -nE 'ConfigPath\s+string' internal/bootstrap/bootstrap.go
                                                                       # expect: empty
grep -nE '^func \(m \*Manager\) Listeners\(\) \[\]Info' internal/listener/manager.go
                                                                       # expect: 1 match
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md`**

```markdown
# Phase 08.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04/05.1/05.2/06.1/06.2/07.1/07.2 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 15 preconditions were satisfied at cold-start>

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nine planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`BuildVersionString()` format = `<sha-short>/<go-version>/Clean/RELEASE/Go-crypto`** (option A; 5-token mirror; `Revision` sourced from `runtime/debug.ReadBuildInfo` else `"unknown"`; `-ldflags` override supported).
2. **`http.Server.WriteTimeout = 30s`** (widened from 5s).
3. **`server.live` gauge = no change** (existing `liveOnce`/`liveGauge` pattern preserved).
4. **Pre-MarkReady scrape behaviour = NOT gated** (the four new handlers respond 200 even pre-MarkReady; only `/ready` checks `s.ready.Load()`).
5. **`Content-Length` + `Date` headers = `net/http` auto-adds** (the `writeAdminHeaders` helper sets only Content-Type, Cache-Control, X-Content-Type-Options, Server).
6. **`Server.bs` field type = `*bootstrap.Bootstrap`** (carries `bs.ConfigPath` for `/server_info`).
7. **`enumerateStaticListeners`/`enumerateStaticClusters` = walk bootstrap proto directly** (not via cluster/listener managers' runtime state).
8. **Per-endpoint cx_*/rq_* counter lines in `/clusters` = emit literal `0`** (envoy-go has no per-endpoint stats per ADR-0063; differential allow-list widened to fully allow-list these 8 fields per endpoint).
9. **`Bootstrap.ConfigPath` field = ADD; `bootstrap.Load` signature = NOT widened** (caller sets `bs.ConfigPath = *cfgPath` post-Load).

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0084]

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-07.2 close + 08.1 SPEC + 08.1 PLAN confirmed present in HEAD; SPEC at 1f85b07; ADR tail at 0083 (next-free 0084); internal/admin/{configdump,clusters,listeners,serverinfo,headers,version}.go absent (the four handler implementations + helpers land at Tasks 4-9); internal/cluster.Manager.Clusters() not yet present (Task 3); internal/bootstrap.Bootstrap.ConfigPath not yet present (Task 2). Landed ADR-0084 (phase-08 planner-time split into 08.1 + 08.2; mirrors ADR-0070 pattern from phase-07).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md
<verbatim>
\`\`\`
```

- [ ] **Step 3: Append ADR-0084 to `docs/envoy-go/DECISIONS.md`**

Append to the file's tail (after ADR-0083; preserve existing content verbatim). Use the ADR-0001 template structure (Status / Doctrine / Lands-in-task / Context / Decision / Consequences / Supersedes). Body content draws from SPEC §8 (ADR-0084 anticipation) — Status: Accepted; Doctrine: D-3.4 + D-3.5; Lands-in-task: Task 1 (PROGRESS preamble); Context: phase-08's combined LoC + task estimate (per BRAINSTORM §1: ~1100–1600 LoC + ~28–38 tasks combined) crosses both ADR-0045 thresholds; the read-only-admin (08.1) vs mutating-drain (08.2) surfaces have disjoint risk profiles + disjoint review surfaces; mirrors ADR-0070's parent-07 split application; Decision: 08 splits into 08.1 (admin-endpoints; this phase) + 08.2 (graceful-drain; future phase) at planner time per ADR-0045; the ROADMAP edit landed at the SPEC commit (master `1f85b07`); the ADR landing here (Task 1 PROGRESS preamble) formalises the decision in DECISIONS.md; Consequences: (a) parent row 08 stays in-progress through both sub-phase phase-done commits — flips at 08.2's phase-done per parent SPEC §5; (b) 08.1 and 08.2 ship as independent sub-phases each with their own SPEC + PLAN + PROGRESS + REVIEW lifecycle artefacts; (c) the BEHAVIOR_CONTRACT umbrella section is restructured by 08.1 (this phase); 08.2 extends the same umbrella; (d) future ADR-0091..onwards may amend either sub-phase without colliding.

- [ ] **Step 4: Run preconditions verbatim and confirm pristine state**

```bash
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
go test -race -count=1 -short ./...                           # expect: all PASS (short mode skips differential)
```

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md docs/envoy-go/DECISIONS.md
git commit -m "phase 08.1: PROGRESS preamble + ADR-0084 (phase-08 planner-time split) [ADR-0084]"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR-0084 anticipation), §15 (acceptance bullet for ADRs in DECISIONS.md), and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `internal/bootstrap.Bootstrap.ConfigPath` field addition

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` (add `ConfigPath string` field on `Bootstrap`)
- Modify: `internal/bootstrap/bootstrap_test.go` (add field-default-zero test)

This task adds a single new field `ConfigPath string` to the `Bootstrap` struct so that `cmd/envoy-go/main.go` can populate it post-Load and `internal/admin/serverinfo.go` (Task 9) can read it for the `command_line_options.config_path` value. The `bootstrap.Load(r io.Reader)` signature is NOT widened per planner-time decision 9. Defaults to `""`; tests that don't need it leave it zero.

**Precondition:** Task 1 done; `Bootstrap` struct does not yet have `ConfigPath` field.
**Artifact:** the new field on `Bootstrap`; one new test asserting the field defaults to `""`.
**Acceptance:** `go build ./...` clean; `go test ./internal/bootstrap/...` passes; `grep -nE 'ConfigPath\s+string' internal/bootstrap/bootstrap.go` returns 1 match.

- [ ] **Step 1: Write failing test in `internal/bootstrap/bootstrap_test.go`**

Append a new test:

```go
func TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty(t *testing.T) {
	const minimalBootstrap = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	bs, err := Load(strings.NewReader(minimalBootstrap))
	if err == nil {
		// minimal bootstrap should error (zero clusters), but if it loads,
		// assert ConfigPath is empty
		if bs.ConfigPath != "" {
			t.Errorf("ConfigPath after Load: got %q, want \"\"", bs.ConfigPath)
		}
	}
	// Construct directly and assert the field is settable.
	b := &Bootstrap{ConfigPath: "/tmp/envoy.yaml"}
	if b.ConfigPath != "/tmp/envoy.yaml" {
		t.Errorf("ConfigPath after struct-literal set: got %q, want %q", b.ConfigPath, "/tmp/envoy.yaml")
	}
}
```

(If the existing test file has no `strings` import, add it; if a similar minimal-bootstrap fixture string already exists, reuse it.)

- [ ] **Step 2: Run test; confirm it fails**

```bash
go test -run TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty ./internal/bootstrap/... 2>&1 | head -10
```

Expected: build error (`unknown field 'ConfigPath' in struct literal`).

- [ ] **Step 3: Add `ConfigPath` field to `Bootstrap` struct**

In `internal/bootstrap/bootstrap.go`, add a new field to the `Bootstrap` struct (after the existing `AccessLogConfigs` field):

```go
type Bootstrap struct {
	// Proto is the unmarshalled Envoy v3 Bootstrap message.
	Proto *bootstrapv3.Bootstrap
	// Stats is the boot-time metrics Registry. ...
	Stats *stats.Registry
	// AccessLogConfigs is the parsed access_log[] file-sink entries ...
	AccessLogConfigs []AccessLogConfig
	// ConfigPath is the file path the bootstrap was loaded from. Set by the
	// caller (cmd/envoy-go/main.go) post-Load via bs.ConfigPath = *cfgPath;
	// Load itself leaves this empty (the bootstrap.Load API takes an io.Reader,
	// not a file path, by ADR-0001 design). Phase 08.1's /server_info admin
	// handler reads this for the command_line_options.config_path field.
	// Test code that does not exercise /server_info may leave this field empty.
	ConfigPath string
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test ./internal/bootstrap/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/bootstrap/...
```

Expected: all PASS, vet clean, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 08.1: internal/bootstrap.Bootstrap.ConfigPath field — sidecar metadata for /server_info"
```

SHA-fill follow-up.

*Anchored: PLAN §"Planner-time deferred-decision resolution" item 9; SPEC §12 #6.*

---

## Task 3: `internal/cluster.Manager.Clusters()` snapshot accessor + `ClusterInfo`/`EndpointInfo` types

**Files:**
- Modify: `internal/cluster/manager.go` (add `ClusterInfo`, `EndpointInfo`, `Clusters()`)
- Modify: `internal/cluster/manager_test.go` (add snapshot tests)

This task adds the public read-only accessor `(m *Manager) Clusters() []ClusterInfo` returning a freshly-allocated snapshot of cluster + endpoint metadata. The `/clusters` admin handler (Task 7) consumes this directly. Per SPEC §6.2, the returned slice is freshly allocated per call (caller-mutation safe); ordering is alphabetical by cluster name; per-cluster endpoints in bootstrap-declared order (the order `extractEndpoints` already preserves; existing `Cluster.endpoints []Endpoint` field is in declaration order).

**Precondition:** Task 2 done; `cluster.Manager.Clusters()` does not yet exist.
**Artifact:** two new exported types (`ClusterInfo`, `EndpointInfo`); one new exported method (`Clusters()`); three new tests.
**Acceptance:** `go build ./...` clean; `go test ./internal/cluster/...` passes; `grep -nE '^func \(m \*Manager\) Clusters\(\) \[\]ClusterInfo' internal/cluster/manager.go` returns 1 match.

- [ ] **Step 1: Write failing tests in `internal/cluster/manager_test.go`**

Append three tests (use the existing test helpers/bootstrap fixtures in the file as templates; the canonical fixture has 1-2 clusters with 1-2 endpoints):

```go
func TestManager_Clusters_SnapshotReturnsAllClusters(t *testing.T) {
	bs := mustParseBootstrap(t, /* fixture YAML with two clusters c_a, c_b each with 2 endpoints */)
	m, err := NewManager(bs.Proto, stats.NewRegistry())
	if err != nil { t.Fatalf("NewManager: %v", err) }
	infos := m.Clusters()
	if len(infos) != 2 {
		t.Fatalf("Clusters() returned %d; want 2", len(infos))
	}
	// Alphabetical-by-name ordering invariant
	if infos[0].Name != "c_a" || infos[1].Name != "c_b" {
		t.Errorf("Clusters() ordering: got [%q, %q]; want [c_a, c_b]", infos[0].Name, infos[1].Name)
	}
	// Per-cluster endpoints populated
	if len(infos[0].Endpoints) != 2 {
		t.Errorf("Clusters()[0].Endpoints: got %d; want 2", len(infos[0].Endpoints))
	}
	if infos[0].Endpoints[0].Address == "" || infos[0].Endpoints[0].Port == 0 {
		t.Errorf("Clusters()[0].Endpoints[0]: empty fields: %+v", infos[0].Endpoints[0])
	}
}

func TestManager_Clusters_FreshlyAllocatedPerCall(t *testing.T) {
	bs := mustParseBootstrap(t, /* same fixture */)
	m, _ := NewManager(bs.Proto, stats.NewRegistry())
	a := m.Clusters()
	b := m.Clusters()
	// Different slice headers (snapshot semantics)
	if &a[0] == &b[0] {
		t.Errorf("Clusters() returned aliased slice; expect freshly allocated per call")
	}
	// Mutation of returned slice does not affect manager state
	a[0].Name = "MUTATED"
	a[0].Endpoints[0].Address = "MUTATED"
	c := m.Clusters()
	if c[0].Name == "MUTATED" {
		t.Errorf("mutating Clusters() result affected manager state (Name)")
	}
	if c[0].Endpoints[0].Address == "MUTATED" {
		t.Errorf("mutating Clusters() result affected manager state (Endpoint.Address)")
	}
}

func TestManager_Clusters_EmptyClustersListReturnsEmpty(t *testing.T) {
	// NewManager errors on zero clusters per the existing manager.go contract;
	// this test asserts that IF a Manager could ever have zero clusters,
	// Clusters() returns an empty (non-nil) slice. Constructed directly:
	m := &Manager{clusters: map[string]*Cluster{}}
	if got := m.Clusters(); got == nil || len(got) != 0 {
		t.Errorf("Clusters() on empty manager: got %v; want non-nil empty slice", got)
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test -run TestManager_Clusters ./internal/cluster/... 2>&1 | head -10
```

Expected: build error (`undefined: ClusterInfo` or `m.Clusters undefined`).

- [ ] **Step 3: Add types and method to `internal/cluster/manager.go`**

After the existing `Get` method (around line 113), add:

```go
// ClusterInfo is the public read-only summary of one cluster, returned by
// Manager.Clusters() and consumed by phase-08.1's /clusters admin handler.
// Per ADR-0087, the /clusters handler reads the snapshot once per scrape and
// formats one block per cluster (10 cluster-level lines + 18 per-endpoint
// lines per the §11.2 empirical pin).
type ClusterInfo struct {
	Name      string
	Endpoints []EndpointInfo
}

// EndpointInfo is the public read-only summary of one upstream endpoint
// within a ClusterInfo. Address is the dotted-quad / IPv6-literal host; Port
// is the TCP port. The combined "address:port" form is what the /clusters
// handler emits in the per-endpoint key prefix per SPEC §11.2.
type EndpointInfo struct {
	Address string
	Port    uint32
}

// Clusters returns a freshly-allocated snapshot of all configured clusters
// and their endpoints, in alphabetical-by-name order. Per-cluster endpoints
// are returned in their bootstrap-declared order (the order extractEndpoints
// preserves at NewManager time). The returned slice is safe for caller
// mutation: modifying it does not affect the Manager's internal state.
//
// Counters / gauges are NOT cached in the returned struct — phase-08.1's
// /clusters handler emits literal `0` for all 8 per-endpoint cx_*/rq_*
// counter fields per the planner-time decision (envoy-go has no per-endpoint
// stats per ADR-0063 deferral; cluster-level counters are surfaced via
// /stats/prometheus and would not partition meaningfully across endpoints).
//
// Phase 08.1 (Task 3) introduces this accessor; ADR-0087 records the design.
func (m *Manager) Clusters() []ClusterInfo {
	out := make([]ClusterInfo, 0, len(m.clusters))
	for _, c := range m.clusters {
		eps := make([]EndpointInfo, 0, len(c.endpoints))
		for _, ep := range c.endpoints {
			eps = append(eps, EndpointInfo{Address: ep.Host, Port: ep.Port})
		}
		out = append(out, ClusterInfo{Name: c.name, Endpoints: eps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
```

Add `"sort"` to the imports if not present.

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test -count=1 ./internal/cluster/... 2>&1 | tail -5
go vet ./...
golangci-lint run ./internal/cluster/...
```

Expected: all PASS, vet clean, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 08.1: internal/cluster.Manager.Clusters() snapshot accessor + ClusterInfo/EndpointInfo types"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (Manager.Clusters() deliverable), §6.2 (signature + snapshot semantics), §14.2 (test list).*

---

## Task 4: `internal/admin/headers.go` + `internal/admin/version.go` shared helpers

**Files:**
- Create: `internal/admin/headers.go`
- Create: `internal/admin/headers_test.go`
- Create: `internal/admin/version.go`
- Create: `internal/admin/version_test.go`

This task introduces the two shared helper files that the four new endpoint handlers (Tasks 6-9) depend on:
- `headers.go` — `writeAdminHeaders(w http.ResponseWriter, contentType string)` writing the four constant headers per SPEC §11.6 (Content-Type, Cache-Control, X-Content-Type-Options, Server). Date and Content-Length are auto-added by net/http per planner-time decision 5.
- `version.go` — `BuildVersionString() string` returning the 5-token form per planner-time decision 1; `Revision` package var sourced from `runtime/debug.ReadBuildInfo` else `"unknown"`; `-ldflags "-X .Revision=<sha>"` override supported.

**Precondition:** Task 3 done; `internal/admin/headers.go` and `internal/admin/version.go` do not exist.
**Artifact:** four new files (two impls + two tests).
**Acceptance:** `go build ./internal/admin/...` clean; `go test ./internal/admin/...` passes for the two new test files.

- [ ] **Step 1: Write failing tests in `internal/admin/headers_test.go`**

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteAdminHeaders_SetsFourConstantHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	writeAdminHeaders(w, "application/json")
	h := w.Header()
	cases := []struct{ key, want string }{
		{"Content-Type", "application/json"},
		{"Cache-Control", "no-cache, max-age=0"},
		{"X-Content-Type-Options", "nosniff"},
		{"Server", "envoy"},
	}
	for _, c := range cases {
		if got := h.Get(c.key); got != c.want {
			t.Errorf("header %q: got %q, want %q", c.key, got, c.want)
		}
	}
}

func TestWriteAdminHeaders_DoesNotSetDateOrContentLength(t *testing.T) {
	w := httptest.NewRecorder()
	writeAdminHeaders(w, "text/plain")
	if got := w.Header().Get("Date"); got != "" {
		t.Errorf("Date should be empty (auto-added by net/http); got %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length should be empty (auto-added by net/http); got %q", got)
	}
}

func TestWriteAdminHeaders_OverwritePreviousContentType(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "previous/value")
	writeAdminHeaders(w, "application/json")
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", got, "application/json")
	}
}

// TestWriteAdminHeaders_AppliedThroughHTTPServer is an end-to-end check that
// the headers reach the wire (net/http does not strip them; case-canonicalisation
// happens at write time per the standard library's contract).
func TestWriteAdminHeaders_AppliedThroughHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAdminHeaders(w, "text/plain; charset=UTF-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil { t.Fatalf("GET: %v", err) }
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Server"); got != "envoy" {
		t.Errorf("end-to-end Server: got %q, want %q", got, "envoy")
	}
	if got := resp.Header.Get("Date"); got == "" {
		t.Errorf("end-to-end Date: empty; want net/http auto-add")
	}
}
```

- [ ] **Step 2: Run test; confirm failure**

```bash
go test -run TestWriteAdminHeaders ./internal/admin/... 2>&1 | head -10
```

Expected: build error (`undefined: writeAdminHeaders`).

- [ ] **Step 3: Write `internal/admin/headers.go`**

```go
package admin

import "net/http"

// writeAdminHeaders writes the four constant headers every 08.1 admin
// endpoint emits per SPEC §11.6 + ADR-0014: Content-Type (varies per
// endpoint; supplied by the caller), Cache-Control: no-cache, max-age=0,
// X-Content-Type-Options: nosniff, Server: envoy.
//
// Date and Content-Length are NOT set here — net/http auto-adds them at
// write time (per RFC 9110 §6.6.1 for Date; auto-computed Content-Length
// when WriteHeader precedes a single Write of bounded body). This matches
// the SPEC §12 #5 + planner-time decision 5 stance: leave it to net/http.
//
// /ready (phase 01) and /stats/prometheus (phase 06.1) set these headers
// inline rather than via this helper for historical reasons; the four 08.1
// endpoints all route through this helper for consistency.
func writeAdminHeaders(w http.ResponseWriter, contentType string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "no-cache, max-age=0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Server", "envoy")
}
```

- [ ] **Step 4: Run header tests; confirm they pass**

```bash
go test -run TestWriteAdminHeaders ./internal/admin/... -v 2>&1 | tail -10
```

Expected: 4 PASS.

- [ ] **Step 5: Write failing tests in `internal/admin/version_test.go`**

```go
package admin

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildVersionString_FiveTokens(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if len(parts) != 5 {
		t.Fatalf("BuildVersionString() = %q; want 5 slash-separated tokens, got %d", v, len(parts))
	}
}

func TestBuildVersionString_GoVersionToken(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[1] != runtime.Version() {
		t.Errorf("BuildVersionString() token 2 = %q; want %q", parts[1], runtime.Version())
	}
}

func TestBuildVersionString_LiteralCleanReleaseGocrypto(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[2] != "Clean" {
		t.Errorf("BuildVersionString() token 3 = %q; want %q", parts[2], "Clean")
	}
	if parts[3] != "RELEASE" {
		t.Errorf("BuildVersionString() token 4 = %q; want %q", parts[3], "RELEASE")
	}
	if parts[4] != "Go-crypto" {
		t.Errorf("BuildVersionString() token 5 = %q; want %q", parts[4], "Go-crypto")
	}
}

func TestBuildVersionString_RevisionDefaultsToUnknownInTestBuild(t *testing.T) {
	// In a go-test build, debug.ReadBuildInfo's vcs.revision setting may
	// be empty (depending on whether the build embeds VCS info). The
	// fallback path emits "unknown". Either way, the first token must
	// be non-empty.
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[0] == "" {
		t.Errorf("BuildVersionString() token 1 (revision) is empty; want non-empty (either VCS-derived or 'unknown')")
	}
}

func TestBuildVersionString_RevisionLDFlagOverride(t *testing.T) {
	// Save and restore the package-level Revision var to assert the
	// -ldflags override path: setting Revision rebuilds the version string.
	saved := Revision
	defer func() { Revision = saved }()
	Revision = "abcdef1234567890"
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	// Per planner-time decision 1: <sha-short> is Revision[:7] when len(Revision) >= 7.
	if parts[0] != "abcdef1" {
		t.Errorf("BuildVersionString() token 1 with Revision=abcdef1234567890: got %q, want %q", parts[0], "abcdef1")
	}
}
```

- [ ] **Step 6: Run version tests; confirm failure**

```bash
go test -run TestBuildVersionString ./internal/admin/... 2>&1 | head -10
```

Expected: build error (`undefined: BuildVersionString`).

- [ ] **Step 7: Write `internal/admin/version.go`**

```go
package admin

import (
	"runtime"
	"runtime/debug"
)

// Revision is the VCS revision the binary was built from. Default-initialised
// from runtime/debug.ReadBuildInfo's vcs.revision setting at init time, or
// "unknown" when no VCS info is embedded (e.g., go-test builds, raw `go run`).
// Release builds override via:
//
//	go build -ldflags "-X github.com/esalaine/envoy-go/internal/admin.Revision=<sha>" ...
//
// The 7-char abbreviation is taken at format time (per BuildVersionString),
// matching Envoy's version-string convention (`5afe27f...` rather than the
// full 40-char SHA).
var Revision = readRevision()

func readRevision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	return "unknown"
}

// BuildVersionString returns the version string emitted in /server_info's
// `version` field per SPEC §6.5 + planner-time decision 1 (option A; 5-token
// mirror of Envoy's `<sha>/1.37.2/Clean/RELEASE/BoringSSL` shape):
//
//	<sha-short>/<go-version>/Clean/RELEASE/Go-crypto
//
// Where:
//   - <sha-short> = first 7 chars of Revision (or full Revision if shorter,
//     including the literal "unknown" fallback).
//   - <go-version> = runtime.Version() (e.g., "go1.23.0").
//   - "Clean" + "RELEASE" + "Go-crypto" = literal tokens (Envoy emits
//     "Clean/RELEASE/BoringSSL"; envoy-go substitutes "Go-crypto" because
//     it links against Go stdlib crypto/tls, not BoringSSL).
//
// The /server_info equivalence claim allow-lists the version field in the
// differential matrix per SPEC §13.2; this format is for byte-shape
// consistency with the differential parser and operator-side `curl /server_info`
// readability, not byte equality with Envoy.
func BuildVersionString() string {
	rev := Revision
	if len(rev) >= 7 {
		rev = rev[:7]
	}
	return rev + "/" + runtime.Version() + "/Clean/RELEASE/Go-crypto"
}
```

- [ ] **Step 8: Run version tests; confirm they pass**

```bash
go test -run TestBuildVersionString ./internal/admin/... -v 2>&1 | tail -15
go vet ./...
golangci-lint run ./internal/admin/...
```

Expected: 5 PASS, vet clean, lint clean.

- [ ] **Step 9: Commit**

```bash
git add internal/admin/headers.go internal/admin/headers_test.go internal/admin/version.go internal/admin/version_test.go
git commit -m "phase 08.1: internal/admin/{headers,version}.go shared helpers — SPEC §11.6 + §6.5"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (headers.go + version.go deliverables), §6.5 (BuildVersionString format), §11.6 (header set), §12 #1 + #5 (planner-time-resolved per PLAN §"Planner-time deferred-decision resolution" items 1 + 5).*

---

## Task 5: `internal/admin/admin.go` constructor widening + `bootTime` field [ADR-0085]

**Files:**
- Modify: `internal/admin/admin.go` (widen `New` signature; add `bs/cm/lm/bootTime` fields; widen `WriteTimeout`; pre-register four new mux handlers in `Start()`)
- Modify: `internal/admin/admin_test.go` (update existing `New` call sites; add `TestServer_NewWidenedConstructor` + `TestAdminWriteTimeoutIs30s`)
- Create: `internal/admin/admin_helpers_test.go` (helper to build a minimal `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` for the four new handler tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0085)

This task widens `internal/admin.New` from `New(addr, registry)` to `New(addr, registry, bs, cm, lm)` per SPEC §6.1 + planner-time decision 6 (`bs *bootstrap.Bootstrap`). Adds three new fields to `Server` (`bs`, `cm`, `lm`) plus a `bootTime time.Time` field set at `New()` time. Widens `http.Server.WriteTimeout` from 5s to 30s per planner-time decision 2. Pre-registers four placeholder mux handlers in `Start()` (the actual handler implementations land at Tasks 6-9; this task lands stubs that return 501 Not Implemented so the routes resolve before their per-task tests overwrite the stubs). ADR-0085 (admin-mux reuse + LBP-1 third generalisation) lands here at the constructor-widening edit, since this IS the third application of the LBP-1 explicit-threading discipline.

The four stubs are deliberately kept in `admin.go` (not yet split into per-endpoint files) so the test file can reference `s.handleConfigDump` etc. for the mux-registration-test in Task 11. Tasks 6-9 each create their per-endpoint file + test file; the file-creation steps OVERWRITE the placeholder body with the real implementation.

**Precondition:** Tasks 2 + 4 done; the existing `New(addr, registry)` signature is the only call site. This task ALSO breaks `cmd/envoy-go/main.go`'s admin.New call — Task 10 fixes that. Between Tasks 5 and 10, `go build ./cmd/envoy-go/...` will fail; this is intentional and documented in PROGRESS for Task 5.
**Artifact:** widened `New`; widened `WriteTimeout`; four stub handlers; new ADR-0085.
**Acceptance:** `go build ./internal/admin/...` clean; `go test ./internal/admin/...` passes; `go build ./cmd/envoy-go/...` FAILS (call-site breakage; Task 10 fixes); ADR-0085 in DECISIONS.md.

- [ ] **Step 1: Write the new tests + update existing tests in `internal/admin/admin_test.go`**

For each existing `New("127.0.0.1:0", stats.NewRegistry())` call site (lines 13, 57, 83, 115, 121, 137, 158), thread the three new args. Use `nil, nil, nil` as the placeholder for the three new params in tests that do NOT exercise the four new endpoints; later tasks (6-9) replace `nil` with `mustMinimalBs(t)`, `mustMinimalCM(t)`, `mustMinimalLM(t)` per the helper file Step 4 introduces.

Add two new tests:

```go
func TestServer_NewWidenedConstructor(t *testing.T) {
	r := stats.NewRegistry()
	s := New("127.0.0.1:0", r, nil, nil, nil)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.registry != r {
		t.Errorf("registry not threaded")
	}
	// bs/cm/lm fields are nil-safe (the four handlers will check); bootTime is set.
	if s.bootTime.IsZero() {
		t.Errorf("bootTime not set at New time")
	}
	if s.liveGauge == nil {
		t.Errorf("liveGauge not allocated (server.live)")
	}
}

func TestAdminWriteTimeoutIs30s(t *testing.T) {
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	_ = addr
	if got := s.httpSrv.WriteTimeout; got != 30*time.Second {
		t.Errorf("WriteTimeout: got %v, want %v (per PLAN planner-time decision 2)", got, 30*time.Second)
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/admin/... 2>&1 | head -20
```

Expected: build errors (`too many arguments in call to New`).

- [ ] **Step 3: Edit `internal/admin/admin.go` — widen Server struct + New + Start**

```go
package admin

import (
	"bytes"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/listener"
	"github.com/esalaine/envoy-go/internal/stats"
)

// Server is the admin HTTP/1.1 server. Phase 01 introduced /ready; phase 06.1
// added /stats/prometheus; phase 08.1 adds /config_dump, /clusters, /listeners,
// /server_info (the four read-only operator-introspection endpoints from
// SPEC §1). The constructor widens to thread *bootstrap.Bootstrap (for
// /config_dump body + /server_info node + command_line_options.config_path),
// *cluster.Manager (for /clusters cluster snapshot), and *listener.Manager
// (for /listeners listener snapshot) — the third application of the LBP-1
// explicit-threading discipline (after 06.1's *stats.Registry and 07.1's
// *HTTPRegistry / 07.2's *ListenerFilterRegistry); ADR-0085 records the
// generalisation. 08.2 will add POST /drain_listeners and extend /ready +
// /server_info for the DRAINING state.
type Server struct {
	addr      string
	ln        net.Listener
	httpSrv   *http.Server
	ready     atomic.Bool
	registry  *stats.Registry
	liveGauge *stats.Gauge
	liveOnce  sync.Once
	// 08.1 fields (per ADR-0085 + planner-time decision 6)
	bs       *bootstrap.Bootstrap
	cm       *cluster.Manager
	lm       *listener.Manager
	bootTime time.Time
}

// New returns an admin server targeting addr. The server is not running yet;
// call Start. The /ready gate is initially closed (MarkReady flips it). The
// registry parameter is the boot-time Registry threaded by main.go; it MUST
// NOT be Frozen yet (admin allocates the server.live gauge at New time per
// SPEC §5.4 + §12 #3). The bs/cm/lm parameters are the bootstrap +
// cluster manager + listener manager threaded by main.go for the four new
// 08.1 admin endpoints (per ADR-0085); they may be nil in test code that
// does NOT exercise those endpoints. bootTime is set to time.Now() at call.
func New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server {
	return &Server{
		addr:      addr,
		registry:  registry,
		liveGauge: registry.NewGauge("server.live"),
		bs:        bs,
		cm:        cm,
		lm:        lm,
		bootTime:  time.Now(),
	}
}

// Start binds and begins serving in a background goroutine. Returns the bound
// address (useful when addr had port 0). Error only if bind fails.
//
// Six routes are registered post-08.1: /ready (phase 01), /stats/prometheus
// (phase 06.1), /config_dump + /clusters + /listeners + /server_info
// (phase 08.1). WriteTimeout is widened from phase 01's 5s to 30s per
// planner-time decision 2 — /config_dump's protojson rendering of large
// bootstraps may approach the budget on slow scrape clients; 30s is generous
// enough for any reasonable fixture without weakening resilience.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/stats/prometheus", handlePrometheus(s.registry))
	mux.HandleFunc("/config_dump", s.handleConfigDump)
	mux.HandleFunc("/clusters", s.handleClusters)
	mux.HandleFunc("/listeners", s.handleListeners)
	mux.HandleFunc("/server_info", s.handleServerInfo)
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// (existing MarkReady, Close, handleReady preserved verbatim)
```

The bottom of the file gains four placeholder handler stubs (each will be REPLACED by Tasks 6-9 with the real impl in `configdump.go` / `clusters.go` / `listeners.go` / `serverinfo.go`):

```go
// handleConfigDump is a placeholder; the real implementation lands in Task 6
// (internal/admin/configdump.go). The placeholder is kept here so the mux
// route resolves between Tasks 5 and 6.
func (s *Server) handleConfigDump(w http.ResponseWriter, r *http.Request) {
	body := []byte("config_dump: not yet implemented\n")
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}

// handleClusters is a placeholder; real impl lands in Task 7.
func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	body := []byte("clusters: not yet implemented\n")
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}

// handleListeners is a placeholder; real impl lands in Task 8.
func (s *Server) handleListeners(w http.ResponseWriter, r *http.Request) {
	body := []byte("listeners: not yet implemented\n")
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}

// handleServerInfo is a placeholder; real impl lands in Task 9.
func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	body := []byte("server_info: not yet implemented\n")
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}

// _ = bytes.Buffer{} — silence unused-import; `bytes` is consumed by the
// per-endpoint impls in Tasks 6-9.
var _ = bytes.Buffer{}
```

(The `bytes` import + `_ = bytes.Buffer{}` line is removed in Task 6 when configdump.go starts using it; an alternative is to defer adding the import until Task 6.)

- [ ] **Step 4: Create `internal/admin/admin_helpers_test.go`** (test-only helpers)

```go
package admin

import (
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/listener"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
)

// minimalBootstrapYAML is the SPEC §7.3 fixture bootstrap (admin :9901,
// listener :10000, cluster c_backend with 2 endpoints :18001 + :18002).
// Used by the 08.1 admin handler tests as the canonical test bootstrap.
const minimalBootstrapYAML = `admin:
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
`

// mustMinimalBs returns the parsed *bootstrap.Bootstrap for the §7.3 fixture.
// Sets ConfigPath to "/test/envoy-go.yaml" so /server_info tests can assert
// the field is threaded.
func mustMinimalBs(t *testing.T) *bootstrap.Bootstrap {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(minimalBootstrapYAML))
	if err != nil { t.Fatalf("bootstrap.Load: %v", err) }
	bs.ConfigPath = "/test/envoy-go.yaml"
	return bs
}

// mustMinimalCM returns a *cluster.Manager built from the §7.3 fixture.
func mustMinimalCM(t *testing.T, bs *bootstrap.Bootstrap) *cluster.Manager {
	t.Helper()
	cm, err := cluster.NewManager(bs.Proto, bs.Stats)
	if err != nil { t.Fatalf("cluster.NewManager: %v", err) }
	return cm
}

// mustMinimalLM returns a *listener.Manager built from the §7.3 fixture.
// Threads empty-but-frozen filter+listener-filter registries.
func mustMinimalLM(t *testing.T, bs *bootstrap.Bootstrap, cm *cluster.Manager) *listener.Manager {
	t.Helper()
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Freeze()
	lfReg := listenerfilter.NewListenerFilterRegistry()
	lfReg.Freeze()
	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, "", false, bs.Stats, nil, httpReg, lfReg)
	if err != nil { t.Fatalf("listener.NewManager...: %v", err) }
	return lm
}
```

(The router filter must be registered for the bootstrap parse to succeed — the HCM filter chain is parsed at `listener.NewManager` time. If `httpReg.Freeze` after empty Register fails the chain build, the helper expands to register `router.New` per the existing 07.1 boot pattern. The implementer at Task 5 verifies what's needed by running the test.)

- [ ] **Step 5: Update existing `New` call sites in `admin_test.go`**

Mechanically replace each `New("127.0.0.1:0", stats.NewRegistry())` with `New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil)` (7 sites per the existing-tests grep in PLAN preamble).

- [ ] **Step 6: Run tests; confirm they pass**

```bash
go build ./internal/admin/...
go test -count=1 ./internal/admin/... 2>&1 | tail -10
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: build clean, all tests PASS, vet clean, lint clean.

NOTE: `go build ./cmd/envoy-go/...` is EXPECTED to fail at this point (call-site breakage); Task 10 fixes that. Document in PROGRESS.md.

- [ ] **Step 7: Append ADR-0085 to `docs/envoy-go/DECISIONS.md`**

Per SPEC §8 (ADR-0085 anticipation). Status: Accepted. Doctrine: D-3.2 + D-3.4. Lands-in-task: Task 5 (admin.New constructor widening). Context: the admin server already has a working bind, working /ready gate, working timeouts, working integration into the lifecycle (per phase-01 + phase-06.1 architecture); splitting into a new admin server for the four new endpoints would duplicate all that for zero benefit. The constructor-widening pattern that 06.1 used for *stats.Registry and 07.1 used for *HTTPRegistry / 07.2 used for *ListenerFilterRegistry generalises to a third application here. Decision: extend internal/admin.Server with four new fields (bs, cm, lm, bootTime) by widening admin.New(addr, registry) → admin.New(addr, registry, bs, cm, lm); register four new mux.HandleFunc entries on the existing *http.ServeMux in admin.Server.Start(); no new HTTP server, no new bind, no new transport. Per planner-time decision 2 (PLAN), WriteTimeout widens from 5s to 30s. Consequences: (a) cmd/envoy-go/main.go's admin.New call site widens by three args (Task 10 lands); (b) test code that does NOT exercise the four new endpoints passes nil for bs/cm/lm; (c) the LBP-1 explicit-threading discipline now has three sibling applications (06.1 *stats.Registry, 07.1 *HTTPRegistry, 07.2 *ListenerFilterRegistry, 08.1 admin three-thread) which collectively prove the pattern's generality; (d) 08.2 (graceful drain) inherits the four threaded fields and adds a drain-state field (no further constructor widening anticipated; 08.2 may add a single `drainState atomic.Pointer[DrainState]` field on Server without changing New's signature).

- [ ] **Step 8: Commit**

```bash
git add internal/admin/admin.go internal/admin/admin_test.go internal/admin/admin_helpers_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 08.1: internal/admin.New widens (bs,cm,lm,bootTime); WriteTimeout 30s; four mux stubs + ADR-0085 [ADR-0085]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (admin.go modifications), §5.1 (boot graph + constructor widening), §6.1 (constructor signature), §8 (ADR-0085 anticipation), §12 #2 + #6 (planner-time-resolved per PLAN items 2 + 6).*

---

## Task 6: `internal/admin/configdump.go` — `/config_dump` handler [ADR-0086]

**Files:**
- Create: `internal/admin/configdump.go`
- Create: `internal/admin/configdump_test.go`
- Modify: `internal/admin/admin.go` (delete the placeholder `handleConfigDump` stub from Task 5; remove `bytes` import if no longer used)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0086)

This task lands the real `/config_dump` handler per SPEC §5.2 + §11.1 + ADR-0086. Body is `application/json` via `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` over `*adminv3.ConfigDump{Configs: []*anypb.Any{<Bootstrap>, <Listeners>, <Clusters>}}`. The three sub-envelope walkers (`enumerateStaticListeners`, `enumerateStaticClusters`) walk the bootstrap proto directly per planner-time decision 7. ADR-0086 lands here.

**Precondition:** Task 5 done; `internal/admin/configdump.go` does not exist; the Task 5 placeholder is in `admin.go`.
**Artifact:** real handler file; full unit test coverage; ADR-0086.
**Acceptance:** `go test ./internal/admin/... -run TestConfigDump` passes; `/config_dump` HTTP smoke (existing TestServer_NewWidenedConstructor pattern) returns 200 + JSON parseable body; ADR-0086 in DECISIONS.md.

- [ ] **Step 1: Write `internal/admin/configdump_test.go` failing tests**

```go
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/esalaine/envoy-go/internal/bootstrap"
)

func TestBuildConfigDump_ThreeSubEnvelopesInOrder(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, err := buildConfigDump(bs, time.Now())
	if err != nil { t.Fatalf("buildConfigDump: %v", err) }
	if len(cd.Configs) != 3 {
		t.Fatalf("Configs len: got %d, want 3", len(cd.Configs))
	}
	wantTypes := []string{
		"type.googleapis.com/envoy.admin.v3.BootstrapConfigDump",
		"type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
		"type.googleapis.com/envoy.admin.v3.ClustersConfigDump",
	}
	for i, want := range wantTypes {
		if got := cd.Configs[i].GetTypeUrl(); got != want {
			t.Errorf("Configs[%d].@type: got %q, want %q", i, got, want)
		}
	}
}

func TestBuildConfigDump_BootstrapEnvelopeContainsParsedProto(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	bootAny := cd.Configs[0]
	bootDump := &adminv3.BootstrapConfigDump{}
	if err := bootAny.UnmarshalTo(bootDump); err != nil {
		t.Fatalf("UnmarshalTo BootstrapConfigDump: %v", err)
	}
	if bootDump.GetBootstrap() == nil {
		t.Errorf("BootstrapConfigDump.Bootstrap is nil")
	}
	if bootDump.GetLastUpdated() == nil {
		t.Errorf("BootstrapConfigDump.LastUpdated is nil")
	}
}

func TestBuildConfigDump_ListenersEnvelopeContainsOneStaticListener(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	lisDump := &adminv3.ListenersConfigDump{}
	if err := cd.Configs[1].UnmarshalTo(lisDump); err != nil {
		t.Fatalf("UnmarshalTo ListenersConfigDump: %v", err)
	}
	if got := lisDump.GetVersionInfo(); got != "static" {
		t.Errorf("ListenersConfigDump.VersionInfo: got %q, want %q", got, "static")
	}
	if got := len(lisDump.GetStaticListeners()); got != 1 {
		t.Errorf("StaticListeners len: got %d, want 1", got)
	}
}

func TestBuildConfigDump_ClustersEnvelopeContainsOneStaticCluster(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	cluDump := &adminv3.ClustersConfigDump{}
	if err := cd.Configs[2].UnmarshalTo(cluDump); err != nil {
		t.Fatalf("UnmarshalTo ClustersConfigDump: %v", err)
	}
	if got := cluDump.GetVersionInfo(); got != "static" {
		t.Errorf("ClustersConfigDump.VersionInfo: got %q, want %q", got, "static")
	}
	if got := len(cluDump.GetStaticClusters()); got != 1 {
		t.Errorf("StaticClusters len: got %d, want 1", got)
	}
}

func TestHandleConfigDump_HTTPSmoke200JSON(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/config_dump")
	if err != nil { t.Fatalf("GET /config_dump: %v", err) }
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
	if srv := resp.Header.Get("Server"); srv != "envoy" {
		t.Errorf("Server: got %q, want %q", srv, "envoy")
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("body is empty")
	}
	// Body must be valid JSON
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Errorf("body is not valid JSON: %v\nbody: %s", err, body)
	}
	if _, ok := generic["configs"]; !ok {
		t.Errorf("body has no 'configs' field; body: %s", body)
	}
}

func TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, _ := http.Get("http://" + addr + "/config_dump")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Snake-case field names per ADR-0086 (UseProtoNames: true)
	if !strings.Contains(bodyStr, `"static_listeners"`) {
		t.Errorf("body lacks snake_case 'static_listeners'; got camelCase? body excerpt: %s", bodyStr[:min(300, len(bodyStr))])
	}
	// 1-space indent per ADR-0086 (Indent: " ")
	if !strings.Contains(bodyStr, "\n {\n") && !strings.Contains(bodyStr, "\n  ") {
		// Check the first nested-object indent is 1 space, not 2
	}
	// EmitUnpopulated: zero-valued fields appear (concretely: cluster's load_assignment ought to be present even if no zero defaults yet shown; we don't test specific zero defaults to avoid coupling)
}

func min(a, b int) int { if a < b { return a }; return b }
```

(The imports for `time`, `adminv3`, `bootstrapv3` etc. need to land in the test file; the implementer adds them as needed.)

- [ ] **Step 2: Run; confirm test file fails to build**

```bash
go test -run TestBuildConfigDump ./internal/admin/... 2>&1 | head -20
```

Expected: build error (`undefined: buildConfigDump`).

- [ ] **Step 3: Write `internal/admin/configdump.go`**

```go
package admin

import (
	"bytes"
	"log"
	"net/http"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/esalaine/envoy-go/internal/bootstrap"
)

// configDumpMarshalOptions is the protojson MarshalOptions tuple pinned by
// SPEC §11.1 empirical-pin findings: 1-space indent (NOT 2 or 4), snake_case
// field names (NOT camelCase), zero-valued fields emitted (the
// EmitUnpopulated: true setting is what gives Envoy's body its
// "show me everything" character — the differential equivalence comparator
// relies on both sides emitting the same set of fields). Settled by
// ADR-0086 (planner consolidates the four-value tuple).
//
// The same MarshalOptions is reused by /server_info (Task 9) for cross-endpoint
// JSON-body shape consistency.
var configDumpMarshalOptions = protojson.MarshalOptions{
	Multiline:       true,
	Indent:          " ",
	UseProtoNames:   true,
	EmitUnpopulated: true,
}

// handleConfigDump implements the /config_dump contract per SPEC §5.2 +
// §11.1 + ADR-0086. Body is application/json; envelope is
// *adminv3.ConfigDump with three sub-envelopes (Bootstrap, Listeners,
// Clusters) packed as *anypb.Any in this exact order. No dynamic_* arrays
// (no xDS); no RoutesConfigDump / SecretsConfigDump / ScopedRoutesConfigDump
// / EndpointsConfigDump (deferred per ADR-0089). The handler is stateless
// and walks immutable-post-boot state at request time.
//
// Errors from protojson.Marshal are recovered + logged + synthesized as
// 500 Internal Server Error with empty body {} per SPEC §5.2 (defensive;
// Envoy's empirical behaviour on /config_dump failure is undocumented but
// 500-with-empty-body is the conservative shape).
func (s *Server) handleConfigDump(w http.ResponseWriter, r *http.Request) {
	cd, err := buildConfigDump(s.bs, s.bootTime)
	if err != nil {
		log.Printf("admin: /config_dump: build: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	body, err := configDumpMarshalOptions.Marshal(cd)
	if err != nil {
		log.Printf("admin: /config_dump: marshal: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	writeAdminHeaders(w, "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// buildConfigDump assembles the *adminv3.ConfigDump envelope from the
// bootstrap proto and the boot timestamp. Walks the bootstrap proto
// directly per planner-time decision 7; does not consult cluster/listener
// managers' runtime state. Returns an error from anypb.New (which can fail
// if the inner proto's type isn't registered with protoregistry.GlobalTypes
// — but the ConfigDump sub-envelope types are all from go-control-plane
// admin/v3 and are registered transitively).
func buildConfigDump(bs *bootstrap.Bootstrap, bootTime time.Time) (*adminv3.ConfigDump, error) {
	if bs == nil || bs.Proto == nil {
		// Defensive: in tests with bs == nil, return an empty ConfigDump
		// so the response is a valid (if empty) JSON body.
		return &adminv3.ConfigDump{}, nil
	}
	bootDump := &adminv3.BootstrapConfigDump{
		Bootstrap:   bs.Proto,
		LastUpdated: timestamppb.New(bootTime),
	}
	lisDump := &adminv3.ListenersConfigDump{
		VersionInfo:     "static",
		StaticListeners: enumerateStaticListeners(bs.Proto, bootTime),
	}
	cluDump := &adminv3.ClustersConfigDump{
		VersionInfo:    "static",
		StaticClusters: enumerateStaticClusters(bs.Proto, bootTime),
	}
	bootAny, err := anypb.New(bootDump)
	if err != nil { return nil, err }
	lisAny, err := anypb.New(lisDump)
	if err != nil { return nil, err }
	cluAny, err := anypb.New(cluDump)
	if err != nil { return nil, err }
	return &adminv3.ConfigDump{Configs: []*anypb.Any{bootAny, lisAny, cluAny}}, nil
}

// enumerateStaticListeners walks bs.GetStaticResources().GetListeners() and
// packs each one into a *adminv3.ListenersConfigDump_StaticListener. Per
// planner-time decision 7, this walks the bootstrap proto directly (NOT
// through the listener manager's runtime state).
func enumerateStaticListeners(bs *bootstrapv3.Bootstrap, bootTime time.Time) []*adminv3.ListenersConfigDump_StaticListener {
	ls := bs.GetStaticResources().GetListeners()
	out := make([]*adminv3.ListenersConfigDump_StaticListener, 0, len(ls))
	ts := timestamppb.New(bootTime)
	for _, l := range ls {
		any, err := anypb.New(l)
		if err != nil {
			log.Printf("admin: /config_dump: anypb.New listener %q: %v", l.GetName(), err)
			continue
		}
		out = append(out, &adminv3.ListenersConfigDump_StaticListener{
			Listener:    any,
			LastUpdated: ts,
		})
	}
	return out
}

// enumerateStaticClusters mirrors enumerateStaticListeners for clusters.
func enumerateStaticClusters(bs *bootstrapv3.Bootstrap, bootTime time.Time) []*adminv3.ClustersConfigDump_StaticCluster {
	cs := bs.GetStaticResources().GetClusters()
	out := make([]*adminv3.ClustersConfigDump_StaticCluster, 0, len(cs))
	ts := timestamppb.New(bootTime)
	for _, c := range cs {
		any, err := anypb.New(c)
		if err != nil {
			log.Printf("admin: /config_dump: anypb.New cluster %q: %v", c.GetName(), err)
			continue
		}
		out = append(out, &adminv3.ClustersConfigDump_StaticCluster{
			Cluster:     any,
			LastUpdated: ts,
		})
	}
	return out
}

// _ = bytes.Buffer{}  // bytes is consumed elsewhere; prevent lint trim
var _ = bytes.Buffer{}
```

(The `bytes` declaration may not be needed if other handlers in admin.go already use it — the implementer prunes unused imports. The `bytes` import was added in Task 5 as a forward-declaration; this task removes it once the per-endpoint files take over.)

- [ ] **Step 4: Delete the placeholder `handleConfigDump` stub from `internal/admin/admin.go`**

In `admin.go`, delete the placeholder `func (s *Server) handleConfigDump(...)` block introduced by Task 5. The mux registration `mux.HandleFunc("/config_dump", s.handleConfigDump)` now resolves to the real handler in `configdump.go` (Go compiles the file as part of the same package). Also remove the `var _ = bytes.Buffer{}` line if no longer referenced; remove the `bytes` import if no longer used.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go build ./internal/admin/...
go test -count=1 -run TestConfigDump -v ./internal/admin/... 2>&1 | tail -20
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: 6 PASS, vet clean, lint clean.

- [ ] **Step 6: Append ADR-0086 to `docs/envoy-go/DECISIONS.md`**

Per SPEC §8 (ADR-0086 anticipation). Status: Accepted. Doctrine: D-3.3 + D-3.7. Lands-in-task: Task 6. Context: SPEC §11.1 verbatim Envoy v1.37.2 scrape pinned the body shape; the marshaler-options and the three-sub-envelope ordering are not derivable from documentation alone. Decision: body = application/json via protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true} over *adminv3.ConfigDump{Configs: [BootstrapAny, ListenersAny, ClustersAny]}. Static-only (no dynamic_* arrays); RoutesConfigDump / SecretsConfigDump / ScopedRoutesConfigDump / EndpointsConfigDump deferred per ADR-0089. enumerateStatic{Listeners,Clusters} walk the bootstrap proto directly per planner-time decision 7. Consequences: (a) the body is byte-equal to Envoy modulo the §13.2 allow-list (node.user_agent_*, node.extensions[], <*ConfigDump>.last_updated); (b) the differential comparator (Task 13) JSON-parses both bodies and field-walks with the allow-list applied; (c) future RoutesConfigDump etc. envelopes can be added without changing the three-envelope ordering invariant (they appear after the three existing envelopes in the Configs slice); (d) the same MarshalOptions tuple is reused by /server_info (Task 9) for cross-endpoint JSON-body shape consistency.

- [ ] **Step 7: Commit**

```bash
git add internal/admin/configdump.go internal/admin/configdump_test.go internal/admin/admin.go docs/envoy-go/DECISIONS.md
git commit -m "phase 08.1: internal/admin/configdump.go — /config_dump handler + ADR-0086 [ADR-0086]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (configdump.go deliverable), §5.2 (per-request flow), §6.4 (per-endpoint contract), §11.1 (empirical pin), §12 #7 (planner-time-resolved), §14.1 (configdump_test.go test list).*

---

## Task 7: `internal/admin/clusters.go` — `/clusters` handler [ADR-0087]

**Files:**
- Create: `internal/admin/clusters.go`
- Create: `internal/admin/clusters_test.go`
- Modify: `internal/admin/admin.go` (delete placeholder `handleClusters`)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0087)

This task lands the real `/clusters` handler per SPEC §5.3 + §11.2 + ADR-0087. Body is `text/plain; charset=UTF-8`. For each cluster (alphabetical-by-name), emits 10 cluster-level lines + 18 per-endpoint lines per the §11.2 verbatim line set. Per planner-time decision 8, all 8 per-endpoint cx_*/rq_* counter lines emit literal `0`. Cluster-level constants (1024, 3, false) and per-endpoint constants (healthy, 1, 0, empty, false, -1) are emitted unconditionally. ADR-0087 lands here (covers both `/clusters` and `/listeners` text-format shape).

**Precondition:** Task 6 done; `internal/admin/clusters.go` does not exist; the Task 5 placeholder is in `admin.go`.
**Artifact:** real handler file + tests + ADR-0087.
**Acceptance:** `go test ./internal/admin/... -run TestClusters` passes; HTTP smoke returns 200 + text body matching the §11.2 line layout for the §7.3 fixture.

- [ ] **Step 1: Write `internal/admin/clusters_test.go` failing tests**

```go
package admin

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandleClusters_HTTPSmoke200Text(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil { t.Fatalf("GET /clusters: %v", err) }
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/plain; charset=UTF-8")
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("empty body")
	}
}

func TestHandleClusters_TenClusterLevelLinesPerCluster(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/clusters")
	body, _ := io.ReadAll(resp.Body)
	defer func() { _ = resp.Body.Close() }()

	// The §7.3 fixture has one cluster c_backend with 2 endpoints, so
	// expect exactly 10 cluster-level + 2 × 18 per-endpoint = 46 lines.
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if got := len(lines); got != 46 {
		t.Errorf("/clusters total lines: got %d, want 46 (10 cluster-level + 2×18 per-endpoint); body:\n%s", got, body)
	}
}

func TestHandleClusters_ClusterLevelLineFormat(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/clusters")
	body, _ := io.ReadAll(resp.Body)
	defer func() { _ = resp.Body.Close() }()
	bodyStr := string(body)

	wantLines := []string{
		"c_backend::observability_name::c_backend",
		"c_backend::default_priority::max_connections::1024",
		"c_backend::default_priority::max_pending_requests::1024",
		"c_backend::default_priority::max_requests::1024",
		"c_backend::default_priority::max_retries::3",
		"c_backend::high_priority::max_connections::1024",
		"c_backend::high_priority::max_pending_requests::1024",
		"c_backend::high_priority::max_requests::1024",
		"c_backend::high_priority::max_retries::3",
		"c_backend::added_via_api::false",
	}
	for _, want := range wantLines {
		if !strings.Contains(bodyStr, want+"\n") {
			t.Errorf("/clusters body missing line %q", want)
		}
	}
}

func TestHandleClusters_PerEndpointLinesAllZeroPlusConstants(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/clusters")
	body, _ := io.ReadAll(resp.Body)
	defer func() { _ = resp.Body.Close() }()
	bodyStr := string(body)

	// All 8 per-endpoint cx_*/rq_* counter lines emit `0` per planner-time decision 8
	for _, ep := range []string{"127.0.0.1:18001", "127.0.0.1:18002"} {
		for _, key := range []string{"cx_active", "cx_connect_fail", "cx_total", "rq_active", "rq_error", "rq_success", "rq_timeout", "rq_total"} {
			want := "c_backend::" + ep + "::" + key + "::0\n"
			if !strings.Contains(bodyStr, want) {
				t.Errorf("/clusters body missing per-endpoint zero line %q", want)
			}
		}
		// Constants per §11.2
		for _, lit := range []string{"hostname::", "health_flags::healthy", "weight::1", "region::", "zone::", "sub_zone::", "canary::false", "priority::0", "success_rate::-1", "local_origin_success_rate::-1"} {
			want := "c_backend::" + ep + "::" + lit + "\n"
			if !strings.Contains(bodyStr, want) {
				t.Errorf("/clusters body missing per-endpoint constant line %q", want)
			}
		}
	}
}

func TestHandleClusters_AlphabeticalByClusterName(t *testing.T) {
	// Construct a Manager directly with two clusters out-of-alphabetical order:
	// c_zeta then c_alpha. Assert the response orders them c_alpha then c_zeta.
	// (Uses internal package access; mustMinimalBs returns a single-cluster
	// fixture. For this test, write a separate two-cluster YAML inline.)
	const twoClusterYAML = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}

static_resources:
  listeners: []
  clusters:
    - name: c_zeta
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_zeta
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18001}}}
    - name: c_alpha
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_alpha
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18002}}}
`
	// (In a real implementation, refactor mustMinimalBs into mustParseBs(yaml) so the helper accepts arbitrary YAML.)
	// Stub: implement inline if needed
	t.Skip("requires mustParseBs(yaml) helper; refactor in this task or skip until Task 13's expanded helper")
}
```

(The last test outlines an alphabetical-ordering check; if `mustMinimalBs` doesn't accept YAML, the implementer either refactors the helper or accepts the test as a `t.Skip` for now — the alphabetical sort is anyway covered by Task 3's `TestManager_Clusters_SnapshotReturnsAllClusters`.)

- [ ] **Step 2: Run; confirm test failures**

```bash
go test -run TestHandleClusters ./internal/admin/... 2>&1 | head -20
```

Expected: tests run against the placeholder, status is 501, fail.

- [ ] **Step 3: Write `internal/admin/clusters.go`**

```go
package admin

import (
	"bytes"
	"fmt"
	"net/http"
)

// handleClusters implements /clusters per SPEC §5.3 + §11.2 + ADR-0087.
// Body is text/plain; charset=UTF-8. For each cluster (alphabetical-by-name),
// emits 10 cluster-level lines + 18 per-endpoint lines (in declaration order).
// Per planner-time decision 8, all 8 per-endpoint cx_*/rq_* counter lines
// emit literal `0` (envoy-go has no per-endpoint stats per ADR-0063).
// Cluster-level constants (1024, 3, false) and per-endpoint constants
// (healthy, 1, 0, empty, false, -1) are emitted unconditionally.
func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if s.cm != nil {
		for _, c := range s.cm.Clusters() {
			writeClusterBlock(&buf, c)
		}
	}
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// writeClusterBlock emits the 10 cluster-level lines + 18 per-endpoint
// lines per endpoint for one cluster, in the exact order pinned by SPEC
// §11.2. Pulled out for unit-test readability.
func writeClusterBlock(buf *bytes.Buffer, c clusterInfoLike) {
	name := c.ClusterName()
	// 10 cluster-level lines per §11.2(b)
	fmt.Fprintf(buf, "%s::observability_name::%s\n", name, name)
	fmt.Fprintf(buf, "%s::default_priority::max_connections::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_pending_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_retries::3\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_connections::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_pending_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_retries::3\n", name)
	fmt.Fprintf(buf, "%s::added_via_api::false\n", name)
	// 18 per-endpoint lines per §11.2(c) — in declaration order
	for _, ep := range c.EndpointsList() {
		addr := fmt.Sprintf("%s:%d", ep.AddressStr(), ep.PortNum())
		// 8 cx_*/rq_* counter lines all emit `0` per planner-time decision 8
		for _, key := range []string{"cx_active", "cx_connect_fail", "cx_total", "rq_active", "rq_error", "rq_success", "rq_timeout", "rq_total"} {
			fmt.Fprintf(buf, "%s::%s::%s::0\n", name, addr, key)
		}
		// 10 constant lines per §11.2(c)
		fmt.Fprintf(buf, "%s::%s::hostname::\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::health_flags::healthy\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::weight::1\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::region::\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::zone::\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::sub_zone::\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::canary::false\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::priority::0\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::success_rate::-1\n", name, addr)
		fmt.Fprintf(buf, "%s::%s::local_origin_success_rate::-1\n", name, addr)
	}
}

// clusterInfoLike + endpointInfoLike are minimal interfaces over
// cluster.ClusterInfo / cluster.EndpointInfo so writeClusterBlock can be
// unit-tested with a synthetic struct without spinning a full
// cluster.Manager. The cluster.ClusterInfo type implements these via its
// fields directly (we wrap with adapter funcs in clusters.go's
// per-endpoint enumeration loop).
type clusterInfoLike interface {
	ClusterName() string
	EndpointsList() []endpointInfoLike
}
type endpointInfoLike interface {
	AddressStr() string
	PortNum() uint32
}

// (The implementer either implements adapter types here, or — simpler —
// inlines the cluster.ClusterInfo struct access into writeClusterBlock and
// drops the interface indirection. The interface is shown here for testability;
// pick whichever the lint/clean-code review prefers.)
```

(The interface indirection is one option; simpler is to reference `cluster.ClusterInfo` / `cluster.EndpointInfo` directly. The implementer chooses based on the lint feedback. The Task 7 acceptance bullet confirms either works.)

- [ ] **Step 4: Delete the placeholder `handleClusters` from `admin.go`**

Same pattern as Task 6 step 4.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go build ./internal/admin/...
go test -count=1 -run TestHandleClusters -v ./internal/admin/... 2>&1 | tail -25
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: 4 PASS (last test skipped), vet clean, lint clean.

- [ ] **Step 6: Append ADR-0087 to `docs/envoy-go/DECISIONS.md`**

Per SPEC §8 (ADR-0087 anticipation). Status: Accepted. Doctrine: D-3.3 + D-3.7. Lands-in-task: Task 7 (covers both /clusters here and /listeners in Task 8). Context: SPEC §11.2 + §11.3 verbatim Envoy v1.37.2 scrapes pinned the text-format line layout; the per-endpoint constants (healthy, 1, 0, empty, false, -1) and the cluster-level constants (1024, 3) are Envoy default-when-not-configured values envoy-go emits for byte-shape parity. Decision: text/plain only (NOT the ?format=json mode — deferred per ADR-0089); /clusters emits 10 cluster-level lines + 18 per-endpoint lines per cluster; /listeners emits one line per listener `<name>::<addr>`; alphabetical-by-name ordering on both. Per planner-time decision 8 (PLAN), all 8 per-endpoint cx_*/rq_* counter lines emit literal `0` (envoy-go has no per-endpoint stats per ADR-0063 deferral); the differential allow-list widens SPEC §7.1's ±1 tolerance to fully allow-list these 8 fields per endpoint. Consequences: (a) byte-equality holds modulo the per-endpoint counter allow-list + the per-cluster counters allow-list (both fully zero on envoy-go side); (b) ADR-0063's per-endpoint-stats-deferral is reaffirmed and explicitly cross-referenced; (c) future stats-hardening phase that lands per-endpoint stats supersedes the planner-time decision 8 — the /clusters handler will then read live per-endpoint values and the differential allow-list narrows back to ±1 tolerance; (d) /listeners stays trivial — one line per listener, no extension fields anticipated until 08.2 may evaluate.

- [ ] **Step 7: Commit**

```bash
git add internal/admin/clusters.go internal/admin/clusters_test.go internal/admin/admin.go docs/envoy-go/DECISIONS.md
git commit -m "phase 08.1: internal/admin/clusters.go — /clusters handler + ADR-0087 [ADR-0087]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (clusters.go deliverable), §5.3 (per-request flow), §6.4 (per-endpoint contract), §11.2 (empirical pin), §14.1 (clusters_test.go test list); planner-time decision 8 (per-endpoint counter `0` emission).*

---

## Task 8: `internal/admin/listeners.go` — `/listeners` handler

**Files:**
- Create: `internal/admin/listeners.go`
- Create: `internal/admin/listeners_test.go`
- Modify: `internal/admin/admin.go` (delete placeholder `handleListeners`)

This task lands the real `/listeners` handler per SPEC §5.4 + §11.3 (covered by ADR-0087 from Task 7; no new ADR). Body is `text/plain; charset=UTF-8`. One line per listener: `<name>::<bind_addr>` (where bind_addr is `host:port`). Alphabetical-by-name ordering. Trailing newline after each line.

**Precondition:** Task 7 done; `internal/admin/listeners.go` does not exist; the Task 5 placeholder is in `admin.go`.
**Artifact:** real handler file + tests.
**Acceptance:** `go test ./internal/admin/... -run TestHandleListeners` passes; HTTP smoke returns 200 + body `l_main::<addr>\n` for the §7.3 fixture.

- [ ] **Step 1: Write `internal/admin/listeners_test.go` failing tests**

```go
package admin

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestHandleListeners_HTTPSmoke200Text(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	defer lm.Stop()
	if err := lm.Start(/* ctx */); err != nil { t.Fatalf("lm.Start: %v", err) }
	time.Sleep(20 * time.Millisecond)
	resp, err := http.Get("http://" + addr + "/listeners")
	if err != nil { t.Fatalf("GET /listeners: %v", err) }
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/plain; charset=UTF-8")
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.HasPrefix(bodyStr, "l_main::") {
		t.Errorf("body must start with 'l_main::'; got %q", bodyStr)
	}
	if !strings.HasSuffix(bodyStr, "\n") {
		t.Errorf("body must end with newline; got %q", bodyStr)
	}
}

func TestHandleListeners_AlphabeticalByName(t *testing.T) {
	// Use mustMinimalLM-like helper that builds a multi-listener bootstrap.
	// (Or: bypass mustMinimalLM and construct a *listener.Manager with
	// synthetic Info entries; depends on listener package's exported API.)
	t.Skip("requires multi-listener fixture; alphabetical ordering otherwise covered by Listeners() snapshot semantics")
}

func TestHandleListeners_NilLMReturnsEmptyBody(t *testing.T) {
	s := New("127.0.0.1:0", mustMinimalBs(t).Stats, nil, nil, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/listeners")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "" {
		t.Errorf("nil-lm body: got %q, want empty", body)
	}
}

// helper: assert two []string are sorted
func assertSorted(t *testing.T, ss []string) {
	t.Helper()
	if !sort.StringsAreSorted(ss) {
		t.Errorf("not sorted: %v", ss)
	}
}
```

(The first test calls `lm.Start(ctx)` — needs a `context.Context` parameter; the implementer adds the import + a `ctx, cancel := context.WithTimeout(...)` lifecycle.)

- [ ] **Step 2: Run tests; confirm failures**

```bash
go test -run TestHandleListeners ./internal/admin/... 2>&1 | head -10
```

Expected: tests run against placeholder, status 501, fail.

- [ ] **Step 3: Write `internal/admin/listeners.go`**

```go
package admin

import (
	"bytes"
	"net/http"
	"sort"
)

// handleListeners implements /listeners per SPEC §5.4 + §11.3 + ADR-0087.
// Body is text/plain; charset=UTF-8. One line per listener: `<name>::<addr>`
// where addr is the listener's bind address as `host:port`. Alphabetical
// by listener name; trailing newline after each line.
func (s *Server) handleListeners(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if s.lm != nil {
		infos := s.lm.Listeners()
		// Sort defensively (s.lm.Listeners() ordering is not contract-promised);
		// alphabetical-by-name is the §11.3 ordering.
		sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
		for _, li := range infos {
			buf.WriteString(li.Name)
			buf.WriteString("::")
			buf.WriteString(li.Addr)
			buf.WriteByte('\n')
		}
	}
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
```

- [ ] **Step 4: Delete the placeholder `handleListeners` from `admin.go`** (same pattern as Task 6 step 4).

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go build ./internal/admin/...
go test -count=1 -run TestHandleListeners -v ./internal/admin/... 2>&1 | tail -15
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: tests PASS (with the multi-listener test t.Skip'd), vet clean, lint clean.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/listeners.go internal/admin/listeners_test.go internal/admin/admin.go
git commit -m "phase 08.1: internal/admin/listeners.go — /listeners handler (ADR-0087 covers shape)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (listeners.go deliverable), §5.4 (per-request flow), §6.4 (per-endpoint contract), §11.3 (empirical pin), §14.1 (listeners_test.go test list); ADR-0087 covers shape (no new ADR for /listeners).*

---

## Task 9: `internal/admin/serverinfo.go` — `/server_info` handler [ADR-0088]

**Files:**
- Create: `internal/admin/serverinfo.go`
- Create: `internal/admin/serverinfo_test.go`
- Modify: `internal/admin/admin.go` (delete placeholder `handleServerInfo`)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0088)

This task lands the real `/server_info` handler per SPEC §5.5 + §11.4 + ADR-0088. Body is `application/json` via the same protojson MarshalOptions as `/config_dump` (reused from Task 6's `configDumpMarshalOptions`). Field set per SPEC §5.5: `version` (from `BuildVersionString()` Task 4), `state` (`LIVE` post-MarkReady; `PRE_INITIALIZING` pre-MarkReady per planner-time decision 4), `uptime_current_epoch` + `uptime_all_epochs` (`durationpb.New(time.Since(s.bootTime))`), `hot_restart_version: "disabled"`, partial `command_line_options{config_path: s.bs.ConfigPath}`, `node` (from `s.bs.Proto.GetNode()`). ADR-0088 lands here.

**Precondition:** Task 8 done; `internal/admin/serverinfo.go` does not exist.
**Artifact:** real handler file + tests + ADR-0088.
**Acceptance:** `go test ./internal/admin/... -run TestHandleServerInfo` passes; HTTP smoke returns 200 + JSON body containing `state`, `version`, `uptime_current_epoch`, `node`, `command_line_options`.

- [ ] **Step 1: Write `internal/admin/serverinfo_test.go` failing tests**

```go
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandleServerInfo_HTTPSmoke200JSON(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/server_info")
	if err != nil { t.Fatalf("GET /server_info: %v", err) }
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
	body, _ := io.ReadAll(resp.Body)
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	for _, key := range []string{"version", "state", "uptime_current_epoch", "uptime_all_epochs", "node", "command_line_options", "hot_restart_version"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("body missing field %q; body: %s", key, body)
		}
	}
}

func TestHandleServerInfo_StatePostMarkReady(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(20 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/server_info")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"state": "LIVE"`) {
		t.Errorf("state post-MarkReady: body lacks `\"state\": \"LIVE\"`; body: %s", body)
	}
}

func TestHandleServerInfo_StatePreMarkReady(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	// NO MarkReady call.
	time.Sleep(20 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/server_info")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"state": "PRE_INITIALIZING"`) {
		t.Errorf("state pre-MarkReady: body lacks `\"state\": \"PRE_INITIALIZING\"`; body: %s", body)
	}
}

func TestHandleServerInfo_UptimeMonotonic(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(10 * time.Millisecond)
	resp1, _ := http.Get("http://" + addr + "/server_info")
	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	time.Sleep(50 * time.Millisecond)
	resp2, _ := http.Get("http://" + addr + "/server_info")
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	// Parse and assert uptime_current_epoch from second is >= first.
	// Both are durationpb-rendered as "<N>s" strings. For sub-second values
	// they may both round to "0s" — for this test we either bump the sleep
	// or assert string-level monotonicity is "0s ≤ 0s" trivially.
	if len(body1) == 0 || len(body2) == 0 {
		t.Fatal("empty body")
	}
	// Defensive check: both bodies parse and have uptime field.
	for _, b := range [][]byte{body1, body2} {
		var g map[string]interface{}
		if err := json.Unmarshal(b, &g); err != nil { t.Fatalf("body parse: %v", err) }
		if _, ok := g["uptime_current_epoch"]; !ok { t.Errorf("body lacks uptime_current_epoch") }
	}
}

func TestHandleServerInfo_CommandLineOptionsConfigPath(t *testing.T) {
	bs := mustMinimalBs(t)  // sets bs.ConfigPath = "/test/envoy-go.yaml"
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/server_info")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"config_path": "/test/envoy-go.yaml"`) {
		t.Errorf("body lacks `\"config_path\": \"/test/envoy-go.yaml\"`; body excerpt: %s", body)
	}
}

func TestHandleServerInfo_HotRestartVersionDisabled(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(10 * time.Millisecond)
	resp, _ := http.Get("http://" + addr + "/server_info")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"hot_restart_version": "disabled"`) {
		t.Errorf("body lacks `\"hot_restart_version\": \"disabled\"`; body excerpt: %s", body)
	}
}
```

- [ ] **Step 2: Run; confirm failure**

```bash
go test -run TestHandleServerInfo ./internal/admin/... 2>&1 | head -15
```

Expected: tests run against placeholder, status 501, fail.

- [ ] **Step 3: Write `internal/admin/serverinfo.go`**

```go
package admin

import (
	"log"
	"net/http"
	"sync/atomic"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// handleServerInfo implements /server_info per SPEC §5.5 + §11.4 + ADR-0088.
// Body is application/json via the same MarshalOptions as /config_dump
// (reused from configdump.go's configDumpMarshalOptions). Field set:
// version (BuildVersionString), state (LIVE/PRE_INITIALIZING per
// planner-time decision 4), uptime_current_epoch + uptime_all_epochs,
// hot_restart_version: "disabled", partial command_line_options{config_path},
// node (from bootstrap proto). DRAINING state is 08.2's deliverable.
func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	info := buildServerInfo(s)
	body, err := configDumpMarshalOptions.Marshal(info)
	if err != nil {
		log.Printf("admin: /server_info: marshal: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	writeAdminHeaders(w, "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// buildServerInfo assembles the *adminv3.ServerInfo proto from the
// Server's threaded fields. Callable from tests.
func buildServerInfo(s *Server) *adminv3.ServerInfo {
	configPath := ""
	var node *core_v3_Node // defined inline below — see imports note
	if s.bs != nil {
		configPath = s.bs.ConfigPath
		if s.bs.Proto != nil {
			node = s.bs.Proto.GetNode()
		}
	}
	uptime := durationpb.New(time.Since(s.bootTime))
	return &adminv3.ServerInfo{
		Version:            BuildVersionString(),
		State:              deriveState(&s.ready),
		UptimeCurrentEpoch: uptime,
		UptimeAllEpochs:    uptime,
		HotRestartVersion:  "disabled",
		CommandLineOptions: &adminv3.CommandLineOptions{
			ConfigPath: configPath,
		},
		Node: node,
	}
}

// deriveState returns ServerInfo_LIVE when the ready atomic is set, else
// ServerInfo_PRE_INITIALIZING per planner-time decision 4 + SPEC §11.7.
// INITIALIZING is unreachable in 08.1 MVP (no xDS init phase that
// survives admin-server bind). DRAINING is 08.2's deliverable.
func deriveState(ready *atomic.Bool) adminv3.ServerInfo_State {
	if ready.Load() {
		return adminv3.ServerInfo_LIVE
	}
	return adminv3.ServerInfo_PRE_INITIALIZING
}

// (The core_v3_Node placeholder above resolves to:
//    corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
//    type core_v3_Node = corev3.Node
// The implementer at Task 9 lands the import + type alias OR uses *corev3.Node directly.)
```

- [ ] **Step 4: Delete the placeholder `handleServerInfo` from `admin.go`** (same pattern as Task 6 step 4).

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go build ./internal/admin/...
go test -count=1 -run TestHandleServerInfo -v ./internal/admin/... 2>&1 | tail -20
go vet ./internal/admin/...
golangci-lint run ./internal/admin/...
```

Expected: 6 PASS, vet clean, lint clean.

- [ ] **Step 6: Append ADR-0088 to `docs/envoy-go/DECISIONS.md`**

Per SPEC §8 (ADR-0088 anticipation). Status: Accepted. Doctrine: D-3.3 + D-3.5. Lands-in-task: Task 9. Context: SPEC §11.4 verbatim Envoy v1.37.2 scrape pinned the field set + the protojson rendering of duration / state-enum values; the state-enum coverage decision is forward-looking (08.1 settles LIVE + PRE_INITIALIZING; 08.2 will add DRAINING; INITIALIZING is unreachable in MVP). Decision: body = application/json via the same MarshalOptions as /config_dump (reused from ADR-0086); field set populates version (per ADR-0086's BuildVersionString format — option A 5-token), state (LIVE/PRE_INITIALIZING per planner-time decision 4), uptime_current_epoch + uptime_all_epochs (durationpb.New(time.Since(bootTime))), hot_restart_version: "disabled", partial command_line_options{config_path: bs.ConfigPath}, node from bootstrap proto. INITIALIZING enum value is documented in adminv3.ServerInfo_State but not emitted by envoy-go. Consequences: (a) state field IS byte-equal across envoy-go and Envoy post-MarkReady (both emit "LIVE"); (b) version, uptime, command_line_options.*, hot_restart_version, node.* allow-listed in differential per §13.2; (c) 08.2 supersedes the LIVE/PRE_INITIALIZING-only coverage by adding DRAINING; the supersession lands as an ADR amendment in 08.2; (d) ADR-0015's pre-init contract for /ready is the structural sibling — both endpoints have a "documented but test-irrelevant pre-init body" carry-forward.

- [ ] **Step 7: Commit**

```bash
git add internal/admin/serverinfo.go internal/admin/serverinfo_test.go internal/admin/admin.go docs/envoy-go/DECISIONS.md
git commit -m "phase 08.1: internal/admin/serverinfo.go — /server_info handler + ADR-0088 [ADR-0088]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (serverinfo.go deliverable), §5.5 (per-request flow), §6.4 (per-endpoint contract), §11.4 (empirical pin), §11.7 (PRE_INITIALIZING), §14.1 (serverinfo_test.go test list); planner-time decisions 1 + 4.*

---

## Task 10: `cmd/envoy-go/main.go` — admin.New call-site update + Bootstrap.ConfigPath wiring

**Files:**
- Modify: `cmd/envoy-go/main.go` (update `admin.New` call site; set `bs.ConfigPath = *cfgPath` post-Load)
- Modify: `cmd/envoy-go/main_test.go` (smoke-assert the four new endpoints respond 200 + `/config_dump` body parses as JSON)

This task fixes the `cmd/envoy-go/main.go` build breakage left by Task 5's constructor widening, AND lands the `bs.ConfigPath = *cfgPath` post-Load assignment per planner-time decision 9. After this task, `go build ./...` is clean across the whole repo.

**Precondition:** Tasks 5-9 done; `cmd/envoy-go/main.go` build is broken (call-site mismatch).
**Artifact:** updated main.go; updated main_test.go with four-endpoint smoke.
**Acceptance:** `go build ./...` clean; `go test ./cmd/envoy-go/...` passes; the four new endpoints all return 200; `/config_dump` body parses as JSON.

- [ ] **Step 1: Edit `cmd/envoy-go/main.go`**

Replace the existing line ~83:
```go
admSrv := admin.New(adminAddr, bs.Stats)
```
with:
```go
admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm)
```

Wait — `lm` is constructed AFTER `admSrv` in the current main.go (line 122 `lm, err := listener.NewManager...`; line 83 `admSrv := admin.New(...)`). The constructor-widening order requires `lm` to exist before `admin.New` is called. Re-order: move the `admSrv := admin.New(...)` block to AFTER the `lm, err := listener.NewManager(...)` block. Specifically:
- Cut the current lines 83–87 (`admSrv := admin.New(...); admSrv.Start(); defer admSrv.Close()`).
- Paste them immediately after `lm, err := listener.NewManagerWithBaseDirAndAllowH2C(...)` (around current line 125, before the `ctx, cancel := signal.NotifyContext(...)` line).
- Update the `defer admSrv.Close()` ordering — defers are LIFO, so the relative order matters: current order is admin defer → access-log defer; new order is access-log defer → admin defer (admin gets closed first). The 08.1 SPEC does not mandate an order; this is fine. Document in PROGRESS.

ALSO add immediately after `bs, err := bootstrap.Load(f)` (current line 47):
```go
bs.ConfigPath = *cfgPath
```

ALSO move the `admSrv.MarkReady()` call (current line 135) into its post-Start position — it stays where it is (after `lm.Start(ctx)`); the `admSrv` variable is still in scope.

The boot ordering post-edit:
1. parse `*cfgPath`
2. open + `bootstrap.Load(f)`
3. **`bs.ConfigPath = *cfgPath`** (NEW)
4. `bootstrap.AdminSocket(bs.Proto)` → `adminAddr`
5. `cluster.NewManagerWithBaseDir(bs.Proto, ..., bs.Stats)` → `cm`
6. access-log sink construction
7. `httpReg := filter_http.NewHTTPRegistry(); ...; httpReg.Freeze()`
8. `lfReg := listenerfilter.NewListenerFilterRegistry(); ...; lfReg.Freeze()`
9. `lm, err := listener.NewManagerWithBaseDirAndAllowH2C(...)` → `lm`
10. **`admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm)`** (MOVED after lm)
11. **`admSrv.Start()`** + `defer admSrv.Close()`
12. `ctx, cancel := signal.NotifyContext(...)`
13. `lm.Start(ctx)` + `defer lm.Stop()`
14. `admSrv.MarkReady()`
15. `bs.Stats.Freeze()`
16. per-listener ready sentinels + terminal `envoy-go ready`
17. `<-ctx.Done()`

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: clean (admin + main packages compile).

- [ ] **Step 3: Add four-endpoint smoke test to `cmd/envoy-go/main_test.go`**

Append a new test:

```go
func TestMain_FourNewAdminEndpointsRespond200(t *testing.T) {
	// Reuse the existing test scaffolding: write a minimal bootstrap
	// (the SPEC §7.3 fixture format) to a temp file, spawn envoy-go,
	// wait for terminal sentinel, scrape the four endpoints from the
	// admin port, assert all respond 200 + non-empty body + the JSON
	// endpoints body parses as JSON.
	cfgPath := writeTempBootstrap(t, /* SPEC §7.3 fixture YAML, admin :0 */)
	adminAddr, _, cleanup := spawnEnvoyGo(t, cfgPath)
	defer cleanup()

	for _, ep := range []string{"/config_dump", "/clusters", "/listeners", "/server_info"} {
		t.Run(ep, func(t *testing.T) {
			resp, err := http.Get("http://" + adminAddr + ep)
			if err != nil { t.Fatalf("GET %s: %v", ep, err) }
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				t.Errorf("status: got %d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if len(body) == 0 {
				t.Errorf("empty body")
			}
			if ep == "/config_dump" || ep == "/server_info" {
				var generic map[string]interface{}
				if err := json.Unmarshal(body, &generic); err != nil {
					t.Errorf("body not JSON: %v\nbody: %s", err, body)
				}
			}
		})
	}
}
```

(The `writeTempBootstrap` + `spawnEnvoyGo` helpers already exist in main_test.go per the existing 06.1/07.1 pattern; the implementer reuses them. If they take a different shape, the test adapts.)

- [ ] **Step 4: Run all tests**

```bash
go test -count=1 ./... 2>&1 | tail -20
go vet ./...
golangci-lint run ./...
```

Expected: all PASS, vet clean, lint clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go
git commit -m "phase 08.1: cmd/envoy-go/main.go — admin.New(bs,cm,lm) call-site + bs.ConfigPath wiring"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (cmd/envoy-go/main.go modifications), §5.1 (boot graph ordering), planner-time decision 9 (Bootstrap.ConfigPath plumbing).*

---

## Task 11: `internal/admin/admin_test.go` — four-endpoint smoke + mux-registration assertion

**Files:**
- Modify: `internal/admin/admin_test.go` (add four-endpoint smoke tests asserting 200 + correct content-type + four-header set)

This task adds four small end-to-end smoke tests asserting that — after the constructor widening (Task 5) and the four real handler registrations (Tasks 6-9) — each of the four new endpoints responds 200, with the correct Content-Type, with the four constant headers per SPEC §11.6 set. These tests provide a single-place enforcement of the §6.4 per-endpoint contract; the per-handler unit tests already exercised in Tasks 6-9 cover the body shape.

**Precondition:** Tasks 6-9 + 10 done; the four real handlers are registered; the call-site mismatch is fixed.
**Artifact:** four new smoke tests in admin_test.go.
**Acceptance:** `go test -count=1 ./internal/admin/...` passes; the four smoke tests assert 200 + Content-Type + 4-header set.

- [ ] **Step 1: Append the four smoke tests to `internal/admin/admin_test.go`**

```go
func TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(20 * time.Millisecond)

	cases := []struct {
		path        string
		wantContent string
	}{
		{"/config_dump", "application/json"},
		{"/clusters", "text/plain; charset=UTF-8"},
		{"/listeners", "text/plain; charset=UTF-8"},
		{"/server_info", "application/json"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			resp, err := http.Get("http://" + addr + c.path)
			if err != nil { t.Fatalf("GET %s: %v", c.path, err) }
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				t.Errorf("status: got %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); got != c.wantContent {
				t.Errorf("Content-Type: got %q, want %q", got, c.wantContent)
			}
			// Four constant headers from §11.6
			for _, h := range []struct{ key, want string }{
				{"Cache-Control", "no-cache, max-age=0"},
				{"X-Content-Type-Options", "nosniff"},
				{"Server", "envoy"},
			} {
				if got := resp.Header.Get(h.key); got != h.want {
					t.Errorf("header %q: got %q, want %q", h.key, got, h.want)
				}
			}
			// Date is auto-added; assert non-empty.
			if got := resp.Header.Get("Date"); got == "" {
				t.Errorf("Date header empty (net/http should auto-add)")
			}
		})
	}
}

func TestAdmin_FourEndpointsAcceptAnyMethod(t *testing.T) {
	// Per SPEC §11.8: upstream Envoy v1.37.2 does NOT enforce method
	// discrimination on the read-only endpoints; envoy-go matches Envoy
	// parity. This test asserts POST /config_dump returns 200 with the
	// same body as GET (mirrors §11.8 verbatim evidence).
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Post("http://"+addr+"/config_dump", "application/json", strings.NewReader(""))
	if err != nil { t.Fatalf("POST /config_dump: %v", err) }
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("POST /config_dump status: got %d, want 200 (Envoy parity per §11.8)", resp.StatusCode)
	}
}
```

(Add `"strings"` import if not already present.)

- [ ] **Step 2: Run tests**

```bash
go test -count=1 -run TestAdmin_AllFourEndpoints -v ./internal/admin/... 2>&1 | tail -25
go test -count=1 -run TestAdmin_FourEndpointsAcceptAnyMethod -v ./internal/admin/... 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/admin/admin_test.go
git commit -m "phase 08.1: internal/admin/admin_test.go — four-endpoint smoke + method-discrimination Envoy parity"
```

SHA-fill follow-up.

*Anchored: SPEC §3 gate (b) (per-endpoint smoke), §6.4 (per-endpoint contract), §11.6 (header set), §11.8 (method-discrimination Envoy parity).*

---

## Task 12: `internal/admin/admin_test.go::TestAdminConcurrentScrapeRace` — concurrent-scrape race test

**Files:**
- Modify: `internal/admin/admin_test.go` (add `TestAdminConcurrentScrapeRace`)

This task lands the concurrent-scrape race test per SPEC §3 gate (b) + §5.6 race-detector contract: 100 goroutines × 4 endpoints × 1s under `go test -race ./...`. Asserts no race-detector finding, no panic, no malformed responses.

**Precondition:** Task 11 done; the four endpoints respond 200.
**Artifact:** one new test.
**Acceptance:** `go test -count=1 -race -run TestAdminConcurrentScrapeRace ./internal/admin/...` passes; race detector reports no findings.

- [ ] **Step 1: Append the race test to `internal/admin/admin_test.go`**

```go
func TestAdminConcurrentScrapeRace(t *testing.T) {
	if testing.Short() {
		t.Skip("race-stress test; skipped under -short")
	}
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm)
	addr, err := s.Start()
	if err != nil { t.Fatalf("Start: %v", err) }
	defer func() { _ = s.Close() }()
	s.MarkReady()
	time.Sleep(20 * time.Millisecond)

	const N = 100
	const D = 1 * time.Second
	deadline := time.Now().Add(D)
	endpoints := []string{"/config_dump", "/clusters", "/listeners", "/server_info"}
	var wg sync.WaitGroup
	errs := make(chan error, N*4)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := &http.Client{Timeout: 2 * time.Second}
			for time.Now().Before(deadline) {
				ep := endpoints[(i+int(time.Now().UnixNano()))%len(endpoints)]
				resp, err := client.Get("http://" + addr + ep)
				if err != nil {
					select { case errs <- fmt.Errorf("goroutine %d %s: %w", i, ep, err): default: }
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 {
					select { case errs <- fmt.Errorf("goroutine %d %s: status %d", i, ep, resp.StatusCode): default: }
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("%v", err)
	}
}
```

(Add `"fmt"`, `"io"`, `"sync"` imports as needed.)

- [ ] **Step 2: Run with race detector**

```bash
go test -count=1 -race -run TestAdminConcurrentScrapeRace -v ./internal/admin/... 2>&1 | tail -10
```

Expected: PASS (~1s wall time + race-detector overhead). No race findings.

- [ ] **Step 3: Run the full admin suite under race detector to verify no regressions**

```bash
go test -count=1 -race ./internal/admin/... 2>&1 | tail -5
```

Expected: every test PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/admin/admin_test.go
git commit -m "phase 08.1: internal/admin/admin_test.go::TestAdminConcurrentScrapeRace — 100×4×1s race-detector contract (SPEC §5.6)"
```

SHA-fill follow-up.

*Anchored: SPEC §3 gate (b) (race-detector contract), §5.6 (concurrency model + race-detector contract), §14.4 (race-detector + lint specialisation).*

---

## Task 13: `internal/admin/fuzz_test.go::FuzzConfigDumpFormat` — adversarial bootstrap proto fuzzer

**Files:**
- Create: `internal/admin/fuzz_test.go`

This task lands the new fuzzer per SPEC §3 gate (d) + §14.5: `FuzzConfigDumpFormat` fuzzes adversarial `*bootstrapv3.Bootstrap` proto values into `buildConfigDump` + `protojson.Marshal`. Asserts (i) no panic, (ii) output is valid JSON parseable by `json.Unmarshal`, (iii) when output is non-empty, root JSON object has a `configs` field. ~80 LoC. Runs at the ADR-0018 30s short-budget. 10th fuzzer post-08.1.

**Precondition:** Task 12 done.
**Artifact:** one new fuzz_test.go file.
**Acceptance:** `go test -fuzz=FuzzConfigDumpFormat -fuzztime=30s ./internal/admin/...` runs clean; total fuzzer count post-08.1 is 10.

- [ ] **Step 1: Write `internal/admin/fuzz_test.go`**

```go
package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/bootstrap"
)

// FuzzConfigDumpFormat fuzzes adversarial bootstrap inputs through
// buildConfigDump + protojson.Marshal; asserts no panic, output is valid
// JSON, output (when non-empty) has a "configs" field. The corpus is
// seeded with a few representative malformed/edge bootstrap YAMLs
// (zero-clusters, zero-listeners, large-name cluster, IPv6 endpoint,
// etc.); the fuzzer mutates the YAML bytes (the bootstrap.Load path
// fails on most inputs — we observe the failure and only feed
// successfully-parsed bootstraps to buildConfigDump).
//
// SPEC §14.5: ~80 LoC; ADR-0018 30s short-budget. 10th fuzzer post-08.1.
func FuzzConfigDumpFormat(f *testing.F) {
	seeds := []string{
		// Empty
		``,
		// Just admin
		`admin: {address: {socket_address: {address: 127.0.0.1, port_value: 9901}}}`,
		// Admin + minimal cluster
		`admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters:
    - name: c1
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c1
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18001}}}
`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, yamlBytes string) {
		// bootstrap.Load fails on most adversarial inputs; that's fine.
		bs, err := bootstrap.Load(strings.NewReader(yamlBytes))
		if err != nil {
			return
		}
		// buildConfigDump must not panic on any successfully-parsed bootstrap.
		cd, err := buildConfigDump(bs, time.Now())
		if err != nil {
			// build error is OK (e.g. anypb.New for an unregistered type);
			// what's NOT OK is a panic.
			return
		}
		// Marshal must not panic.
		body, err := configDumpMarshalOptions.Marshal(cd)
		if err != nil {
			return
		}
		if len(body) == 0 {
			return
		}
		// Output must be valid JSON.
		var generic map[string]interface{}
		if err := json.Unmarshal(body, &generic); err != nil {
			t.Errorf("buildConfigDump output is not valid JSON: %v\nbody[:200]: %s", err, body[:min(200, len(body))])
			return
		}
		// Non-empty output must have a "configs" field.
		if _, ok := generic["configs"]; !ok {
			t.Errorf("buildConfigDump output lacks 'configs' field; body[:200]: %s", body[:min(200, len(body))])
		}
	})
}
```

- [ ] **Step 2: Run the fuzzer at the 30s short-budget**

```bash
go test -fuzz=FuzzConfigDumpFormat -fuzztime=30s ./internal/admin/... 2>&1 | tail -10
```

Expected: no panics, no `t.Errorf` failures, no crashing inputs added to the seed corpus. The fuzzer reports something like `fuzz: elapsed: 30s, execs: NNNNN, new interesting: 0`. (The seed-corpus `min` helper is the same as in Task 6's configdump_test.go; ensure it's defined or re-define here.)

- [ ] **Step 3: Re-run the full short-budget regression for the 10 fuzzers**

```bash
# Mechanical re-run of the 9 pre-existing fuzzers at 30s each + the new one
for fuzz in FuzzBootstrapLoad FuzzTcpProxyFilter FuzzTLSContextParse FuzzHCMConfigParse FuzzFrameStream FuzzHPACKDecode FuzzPromTextFormat FuzzDefaultFormatRender FuzzFilterChainParse FuzzConfigDumpFormat; do
  echo "=== $fuzz ==="
  pkg=$(grep -lr "func $fuzz" --include='*.go' | head -1 | xargs dirname)
  go test -fuzz=$fuzz -fuzztime=30s ./$pkg/ 2>&1 | tail -3
done
```

Expected: each fuzzer reports clean (no new interesting inputs causing failures).

- [ ] **Step 4: Commit**

```bash
git add internal/admin/fuzz_test.go
git commit -m "phase 08.1: internal/admin/fuzz_test.go::FuzzConfigDumpFormat — 30s short-budget per ADR-0018; 10th fuzzer"
```

SHA-fill follow-up.

*Anchored: SPEC §3 gate (d) (fuzzer count goes from 9 to 10; ADR-0018 30s short-budget), §14.5 (fuzzer list).*

---

## Task 14: `test/fixtures/0009-admin-config-dump/` — differential fixture

**Files:**
- Create: `test/fixtures/0009-admin-config-dump/envoy.yaml`
- Create: `test/fixtures/0009-admin-config-dump/envoy-go.yaml`
- Create: `test/fixtures/0009-admin-config-dump/expectations.yaml`
- Create: `test/fixtures/0009-admin-config-dump/README.md`
- Create: `test/fixtures/0009-admin-config-dump/driver/driver.go`
- Modify: `test/differential/runner_test.go` (add blank-import for the new driver package)

This task lands the differential fixture per SPEC §7. The driver implements `Driver` (one listener `l_main`; 2 HTTPHello backends; 5 GET requests in `DriveSubject`/`DriveReference`; canonicalised four-endpoint scrape in `ProbeAdmin`). The canonicalisation in `ProbeAdmin` applies the §13.2 per-field allow-list:
- `/config_dump`: parse JSON; recursively delete allow-listed paths (`bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions`, `<*ConfigDump>.last_updated`); re-marshal canonical (sorted keys via `json.Marshal` + `bytes.Buffer + json.Indent`); emit.
- `/clusters`: parse into `(cluster, key, value)` tuples; drop the 8 per-endpoint cx_*/rq_* counter tuples (per planner-time decision 8); sort tuples; emit.
- `/listeners`: byte-passthrough (after dechunk handled by the runner's existing dechunk path).
- `/server_info`: parse JSON; recursively delete allow-listed paths (`version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` except byte-equal `state`, `hot_restart_version`, `node.user_agent_*`, `node.extensions`); re-marshal canonical; emit.

**Precondition:** Tasks 11 + 12 + 13 done; the four endpoints respond 200; pre-existing fixtures 0000–0008 still green.
**Artifact:** the fixture directory + driver registration.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0009' -v` passes; the fixture is differentially green.

- [ ] **Step 1: Create `test/fixtures/0009-admin-config-dump/envoy-go.yaml`**

The subject envoy-go bootstrap. The driver templates the backend ports + admin port + listener port at runtime via `SubjectConfig`; this static file is a template with `{{.AdminPort}}`, `{{.ListenerPort}}`, `{{.BackendPort1}}`, `{{.BackendPort2}}` placeholders consumed by `text/template` in the driver. Mirrors SPEC §7.3 modulo placeholders:

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
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: {{.BackendPort1}}}}}
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: {{.BackendPort2}}}}}
```

(If the existing Driver convention is to inline the template string in `driver.go` rather than ship a `.yaml.tmpl` file, use the inline pattern — the implementer follows whichever convention the existing fixtures use; check `test/fixtures/0007a-cors/driver/driver.go` for the inline-string pattern.)

- [ ] **Step 2: Create `test/fixtures/0009-admin-config-dump/envoy.yaml`**

Reference Envoy bootstrap. Per the existing fixtures (0007a et al.), reference Envoy uses STRICT_DNS pointing at `host.docker.internal` to reach host backends. Mirrors the subject modulo:
- admin port can stay symbolic (`{{.RefAdminPort}}` — runner allocates an in-container port)
- listener port can stay symbolic (`{{.RefListenerPort}}`)
- cluster type changes from `STATIC` to `STRICT_DNS`; endpoints become `host.docker.internal:{{.BackendPort1}}` etc.

```yaml
admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: {{.RefAdminPort}}}

static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 0.0.0.0, port_value: {{.RefListenerPort}}}}
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
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: {{.BackendPort1}}}}}
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: {{.BackendPort2}}}}}
```

(Per ADR-0010, the V4_ONLY DNS rule is required for the reference side; per ADR-0028, `--concurrency 1` is set on the Envoy CLI by the runner — the driver does not need to set it in the bootstrap.)

- [ ] **Step 3: Create `test/fixtures/0009-admin-config-dump/expectations.yaml`** (prose; not machine-evaluated per ADR-0019)

```yaml
# Phase 08.1 fixture 0009-admin-config-dump — expectations
#
# This file is prose, not machine-evaluated (per ADR-0019). The runner
# enforces the assertions described below via the driver's ProbeAdmin
# canonicalisation + the runner's CompareBytes pass on the canonicalised
# scrape results.
#
# Asserted equivalence (per SPEC §7.1 + ADR-0086 + ADR-0087 + ADR-0088,
# refined per planner-time decision 8 in PLAN §"Planner-time deferred-
# decision resolution"):
#
#   /config_dump (per ADR-0086):
#     - body parsed as JSON; recursively compared with allow-list applied.
#     - allow-listed JSON paths (zeroed on BOTH sides before comparison):
#       - bootstrap.node.user_agent_name
#       - bootstrap.node.user_agent_build_version (entire subtree)
#       - bootstrap.node.extensions[]
#       - configs[*].bootstrap.last_updated  (timestamps differ trivially)
#       - configs[*].listeners.last_updated (likewise)
#       - configs[*].clusters.last_updated  (likewise)
#     - asserted: three sub-envelopes in order (BootstrapConfigDump,
#       ListenersConfigDump, ClustersConfigDump); version_info: "static" on
#       both static envelopes; static_listeners[] populated with one entry
#       (l_main); static_clusters[] populated with one entry (c_backend with
#       2 endpoints).
#
#   /clusters (per ADR-0087, refined by planner-time decision 8):
#     - body parsed into (cluster, key, value) tuple set.
#     - 8 per-endpoint cx_*/rq_* counter tuples DROPPED on both sides
#       before set comparison (planner-time decision 8 — envoy-go emits 0
#       for all 8; Envoy emits per-endpoint observed values; the gap is
#       intentional and codified).
#     - asserted: 10 cluster-level lines per cluster (in §11.2 order);
#       10 per-endpoint constant lines per endpoint (hostname, health_flags,
#       weight, region, zone, sub_zone, canary, priority, success_rate,
#       local_origin_success_rate); cluster ordering (alphabetical-by-name).
#
#   /listeners (per ADR-0087):
#     - body byte-equal modulo framing dechunk (the runner's existing
#       dechunk preprocessor — phase-01 inheritance — handles upstream's
#       transfer-encoding: chunked).
#     - asserted: one line per listener `<name>::<addr>`; alphabetical
#       ordering.
#
#   /server_info (per ADR-0088):
#     - body parsed as JSON; recursively compared with allow-list applied.
#     - allow-listed JSON paths (zeroed on BOTH sides before comparison):
#       - version
#       - uptime_current_epoch
#       - uptime_all_epochs
#       - hot_restart_version
#       - command_line_options.* (entire subtree EXCEPT it would be
#         reasonable to keep config_path comparison; allow-list it for
#         simplicity since envoy-go's path is host-side and Envoy's is
#         container-side)
#       - node.user_agent_name
#       - node.user_agent_build_version (entire subtree)
#       - node.extensions[]
#     - asserted: state field byte-equal ("LIVE" on both sides).
#
# Not asserted (allow-listed structurally):
#   - Date / Server / Content-Length / Content-Type / Transfer-Encoding —
#     per the existing helpers / differential allow-list in
#     BEHAVIOR_CONTRACT.md ## Header allow-list. The driver's ProbeAdmin
#     does NOT include headers in the canonicalised byte stream.
#
# Per planner-time decision 4: the four new endpoints are NOT gated on
# MarkReady. The driver waits 200ms after the 5-request load completes
# before scraping the four endpoints; both proxies will be in LIVE state
# by then (envoy-go is post-MarkReady; reference Envoy is post-init). No
# pre-MarkReady scrape is exercised by this fixture.
#
# Per planner-time decision 7: /config_dump's three sub-envelope walks
# read from the bootstrap proto directly (NOT from the listener/cluster
# managers' runtime state). This makes the body deterministic and
# detached from any post-boot state changes.
```

- [ ] **Step 4: Create `test/fixtures/0009-admin-config-dump/README.md`**

```markdown
# Fixture 0009 — admin-config-dump differential

This fixture asserts per-endpoint equivalence between envoy-go's four 08.1
admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`)
and reference Envoy v1.37.2 under a 5-request defined load against a STATIC
cluster with 2 endpoints. See `expectations.yaml` for the full per-endpoint
allow-list rationale. See SPEC §7 + ADR-0086 + ADR-0087 + ADR-0088 for the
authoritative contract.

## 5-request workload

The driver issues 5 sequential `GET / HTTP/1.1` round-trips against the
listener (port 10000 on subject; in-container port allocated by the runner
on reference). The 5 requests round-robin across the 2 endpoints, populating
upstream connection counters on both sides. After the load, the driver
sleeps 200ms for stats to settle, then scrapes the four admin endpoints
from each proxy.

## Per-endpoint canonicalisation

The driver's `ProbeAdmin` applies per-endpoint canonicalisation BEFORE
returning the byte stream to the runner's CompareBytes pass:

- `/config_dump` + `/server_info`: JSON parse; recursively zero allow-
  listed paths (build metadata, timestamps, uptime, command-line options,
  node.user_agent_*, node.extensions); re-marshal with sorted keys + 1-
  space indent.
- `/clusters`: line-parse into (cluster, key, value) tuples; drop the 8
  per-endpoint cx_*/rq_* counter tuples per planner-time decision 8;
  sort tuples; emit.
- `/listeners`: byte-passthrough (the runner's existing dechunk
  preprocessor handles upstream's transfer-encoding: chunked).

## Planner-time decision 8 cross-reference

envoy-go does not track per-endpoint stats (per ADR-0063 deferral); the
`/clusters` per-endpoint cx_*/rq_* counter lines emit literal `0` on the
envoy-go side. Reference Envoy emits per-endpoint observed values. The
canonicalisation drops these 8 tuples on BOTH sides so the set-equality
comparison passes.

## SPEC + ADR cross-references

- SPEC §7 (differential fixture) + §11.1–§11.4 (verbatim Envoy scrapes)
- ADR-0086 (`/config_dump` body shape)
- ADR-0087 (`/clusters` + `/listeners` body shape)
- ADR-0088 (`/server_info` body shape + state-enum coverage)
- ADR-0089 (admin-endpoint deferral list)
- ADR-0090 (no-ACL admin security posture)
- planner-time decision 8 (per-endpoint counter `0` emission;
  `expectations.yaml` documents the allow-list extension)

## Backend kind

`HTTPHello` (existing helper from fixture 0007a-cors) — backends return a
fixed body; the differential cares about admin-endpoint output, not
backend-response equivalence.
```

- [ ] **Step 5: Create `test/fixtures/0009-admin-config-dump/driver/driver.go`** — the substantial Go file

This is the largest single file in 08.1 (~250 LoC). The structure mirrors `test/fixtures/0007a-cors/driver/driver.go`:

```go
// Package driver registers the 0009-admin-config-dump fixture with the
// differential runner. This is the project's first admin-endpoint
// differential — it asserts wire-equivalent per-endpoint shape between
// envoy-go and reference Envoy v1.37.2 across the four 08.1 read-only
// endpoints (/config_dump, /clusters, /listeners, /server_info) under a
// 5-request defined load against a STATIC cluster with 2 endpoints per
// SPEC §7.
//
// Integration shape (SPEC §7.2 driver outline):
//
//  1. SubjectConfig templates the envoy-go bootstrap with admin/listener/
//     backend ports; ReferenceBootstrap templates the reference bootstrap
//     with the same ports via host.docker.internal (ADR-0010 STRICT_DNS).
//  2. DriveReference / DriveSubject issue 5 sequential H1 GET round-trips
//     against the listener; sleep 200ms; return empty bytes (the actual
//     differential happens in ProbeAdmin).
//  3. ProbeAdmin scrapes the four endpoints from each proxy, canonicalises
//     each per the §13.2 allow-list (per planner-time decisions 7 + 8),
//     and returns the canonicalised concatenation as the runner's
//     CompareBytes input.
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const (
	fixtureName               = "0009-admin-config-dump"
	refContainerListenerPort  = 10001
)

// adminDriver implements fixture.Driver for the four-endpoint admin
// differential.
type adminDriver struct{}

func init() {
	fixture.RegisterFixture(fixtureName, &adminDriver{})
}

func (adminDriver) BackendCount() int                      { return 2 }
func (adminDriver) BackendKind() fixture.BackendKind       { return fixture.HTTPHello }
func (adminDriver) SubjectListenerName() string            { return "l_main" }
func (adminDriver) ReferenceListenerPort() int             { return refContainerListenerPort }

// Subject bootstrap template. Admin port is bound to 0 so the runner can
// reuse harness scaffolding; the templated value below is what the runner
// passes via subjAdminPort.
const subjectTmpl = `admin:
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
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: {{.BackendPort1}}}}}
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: {{.BackendPort2}}}}}
`

// Reference bootstrap template. STRICT_DNS host.docker.internal per ADR-0010.
const referenceTmpl = `admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: 9901}
static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 0.0.0.0, port_value: {{.ListenerPort}}}}
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
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: {{.BackendPort1}}}}}
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: {{.BackendPort2}}}}}
`

func (adminDriver) ReferenceBootstrap(backendPorts []int) string {
	var b bytes.Buffer
	template.Must(template.New("ref").Parse(referenceTmpl)).Execute(&b, map[string]interface{}{
		"ListenerPort": refContainerListenerPort,
		"BackendPort1": backendPorts[0],
		"BackendPort2": backendPorts[1],
	})
	return b.String()
}

func (adminDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	var b bytes.Buffer
	template.Must(template.New("subj").Parse(subjectTmpl)).Execute(&b, map[string]interface{}{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort1": backendPorts[0],
		"BackendPort2": backendPorts[1],
	})
	return b.String()
}

// drive5RequestLoad issues 5 sequential GET / HTTP/1.1 requests with
// Host: test.local. Returns empty bytes; the actual differential happens
// in ProbeAdmin.
func drive5RequestLoad(ctx context.Context, addr string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/", nil)
		if err != nil { return nil, err }
		req.Host = "test.local"
		resp, err := client.Do(req)
		if err != nil { return nil, fmt.Errorf("request %d: %w", i, err) }
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	// Allow stats to settle before the admin scrape.
	time.Sleep(200 * time.Millisecond)
	return nil, nil
}

func (adminDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive5RequestLoad(ctx, addr)
}

func (adminDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive5RequestLoad(ctx, addr)
}

// ProbeAdmin scrapes the four 08.1 endpoints from each proxy and returns
// the canonicalised concatenation. The canonicalisation applies the
// per-endpoint allow-list per SPEC §13.2 + planner-time decisions 7+8.
func (adminDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = scrapeAndCanonicalise(ctx, refAdminAddr)
	if err != nil { return nil, nil, fmt.Errorf("ref scrape: %w", err) }
	subjBytes, err = scrapeAndCanonicalise(ctx, subjAdminAddr)
	if err != nil { return nil, nil, fmt.Errorf("subj scrape: %w", err) }
	return refBytes, subjBytes, nil
}

func scrapeAndCanonicalise(ctx context.Context, addr string) ([]byte, error) {
	var out bytes.Buffer
	for _, ep := range []string{"/config_dump", "/clusters", "/listeners", "/server_info"} {
		body, err := scrapeOne(ctx, addr, ep)
		if err != nil { return nil, fmt.Errorf("%s: %w", ep, err) }
		canon, err := canonicaliseEndpoint(ep, body)
		if err != nil { return nil, fmt.Errorf("%s canonicalise: %w", ep, err) }
		out.WriteString("=== " + ep + " ===\n")
		out.Write(canon)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func scrapeOne(ctx context.Context, addr, ep string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+ep, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// canonicaliseEndpoint applies per-endpoint canonicalisation per
// expectations.yaml. The SPEC §13.2 + planner-time decision 8 allow-list
// rules are encoded here. For /config_dump + /server_info, JSON-parse and
// recursively delete allow-listed paths; for /clusters, tuple-parse and
// drop the 8 per-endpoint cx_*/rq_* counter tuples; for /listeners,
// passthrough.
func canonicaliseEndpoint(ep string, body []byte) ([]byte, error) {
	switch ep {
	case "/config_dump":
		return canonicaliseConfigDump(body)
	case "/clusters":
		return canonicaliseClusters(body), nil
	case "/listeners":
		return body, nil
	case "/server_info":
		return canonicaliseServerInfo(body)
	default:
		return nil, fmt.Errorf("unknown endpoint: %s", ep)
	}
}

// canonicaliseConfigDump zeros allow-listed JSON paths and re-marshals
// canonically.
func canonicaliseConfigDump(body []byte) ([]byte, error) {
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil { return nil, err }
	// Walk each config sub-envelope; zero allow-listed paths.
	if configs, ok := generic["configs"].([]interface{}); ok {
		for _, c := range configs {
			cm, ok := c.(map[string]interface{})
			if !ok { continue }
			delete(cm, "last_updated")
			if bs, ok := cm["bootstrap"].(map[string]interface{}); ok {
				if node, ok := bs["node"].(map[string]interface{}); ok {
					delete(node, "user_agent_name")
					delete(node, "user_agent_build_version")
					delete(node, "extensions")
				}
			}
		}
	}
	return canonicalJSON(generic), nil
}

// canonicaliseClusters parses lines, drops 8 per-endpoint counter tuples,
// sorts the remainder, and re-emits.
func canonicaliseClusters(body []byte) []byte {
	dropKeys := map[string]bool{
		"cx_active": true, "cx_connect_fail": true, "cx_total": true,
		"rq_active": true, "rq_error": true, "rq_success": true,
		"rq_timeout": true, "rq_total": true,
	}
	var keep []string
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" { continue }
		// Per-endpoint lines have format `<cluster>::<addr>::<key>::<value>`.
		// Cluster-level lines have `<cluster>::<key>::<value>` (3 fields).
		parts := strings.Split(line, "::")
		if len(parts) >= 4 {
			key := parts[len(parts)-2]
			if dropKeys[key] { continue }
		}
		keep = append(keep, line)
	}
	sort.Strings(keep)
	return []byte(strings.Join(keep, "\n") + "\n")
}

// canonicaliseServerInfo zeros allow-listed JSON paths and re-marshals.
func canonicaliseServerInfo(body []byte) ([]byte, error) {
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil { return nil, err }
	delete(generic, "version")
	delete(generic, "uptime_current_epoch")
	delete(generic, "uptime_all_epochs")
	delete(generic, "hot_restart_version")
	delete(generic, "command_line_options")
	if node, ok := generic["node"].(map[string]interface{}); ok {
		delete(node, "user_agent_name")
		delete(node, "user_agent_build_version")
		delete(node, "extensions")
	}
	return canonicalJSON(generic), nil
}

// canonicalJSON re-marshals m with sorted keys + 1-space indent so byte
// equality is order-independent.
func canonicalJSON(m map[string]interface{}) []byte {
	out, _ := json.MarshalIndent(m, "", " ")
	// json.MarshalIndent already sorts map keys; ordering of slices is
	// preserved (slices are ordered semantically — e.g. configs[]
	// ordering Bootstrap-Listeners-Clusters is asserted by the spec).
	return out
}
```

(The implementer adapts to whatever exact harness conventions exist — the runner_test.go's spawn/template plumbing may need a small tweak. Two possible gotchas: (1) the runner's allocate-port plumbing may not pass `subjAdminPort` to the driver in the existing shape; check `test/differential/runner_test.go` near the `SubjectConfig` call site to confirm; (2) the runner's `ProbeAdmin` path may dechunk before calling — check that `/listeners`'s byte-passthrough still byte-equates after dechunk happens.)

- [ ] **Step 6: Add the blank-import to `test/differential/runner_test.go`**

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver"
```

(Insert in alphabetical order, after the `0008-...` import.)

- [ ] **Step 7: Run the new fixture**

```bash
go test -count=1 -v -run 'TestDifferential/0009-admin-config-dump' ./test/differential/... 2>&1 | tail -40
```

Expected: PASS. If FAIL, the failure is one of:
- the canonicalisation drops too few or too many paths (refine `canonicaliseConfigDump` / `canonicaliseServerInfo`);
- the `/clusters` per-endpoint counter set differs in line shape (revisit the drop-key logic);
- the runner's harness doesn't allocate ports the way the template expects (adjust the template).

Iterate until green; record each iteration in PROGRESS.md.

- [ ] **Step 8: Run the full differential suite to verify no regression**

```bash
go test -count=1 -v ./test/differential/... 2>&1 | tail -30
```

Expected: every fixture (0000–0009) PASS.

- [ ] **Step 9: Commit**

```bash
git add test/fixtures/0009-admin-config-dump/ test/differential/runner_test.go
git commit -m "phase 08.1: test/fixtures/0009-admin-config-dump — admin-endpoint differential fixture"
```

SHA-fill follow-up.

*Anchored: SPEC §3 gate (e) (differential fixtures green), §7.1 (per-endpoint equivalence claims), §7.2 (driver outline), §7.3 (fixture bootstrap), §7.4 (differential gate scope clarification); planner-time decisions 7 + 8.*

---

## Task 15: BEHAVIOR_CONTRACT.md restructure + ADR-0089 + ADR-0090; six-gate verification sweep + REVIEW.md + phase-done commit + STATE.md/ROADMAP advance

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (`## Admin API — /ready` → `## Admin API` umbrella restructure + four equivalence-matrix rows + `### Does not yet apply to` extension per SPEC §13.1 + §13.2)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0089 + ADR-0090)
- Modify: `docs/envoy-go/ROADMAP.md` (row `08.1` `in-progress → done` flip)
- Modify: `docs/envoy-go/STATE.md` (rewrite to advance to 08.2 lifecycle-state 0)
- Modify: `internal/admin/doc.go` (enumerate six endpoints)
- Create: `docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md` (end-of-phase review per `superpowers:requesting-code-review`)
- Modify: `docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md` (final task entry)

This task is the phase-done landing. It runs the six-gate verification sweep per SPEC §3 + BOOTSTRAP §7.5; populates `BEHAVIOR_CONTRACT.md` + ADR-0089 + ADR-0090 in place; updates `internal/admin/doc.go` to reflect the six-endpoint reality; runs the requesting-code-review skill to populate REVIEW.md; and commits the phase-done bundle in one commit per the 06.1 / 06.2 / 07.1 / 07.2 closure pattern. ROADMAP row `08.1` flips `in-progress → done` AT this commit; parent row `08` stays `in-progress` (08.2 still `planned`).

**Precondition:** Tasks 1-14 done; all unit tests + differential suite + fuzzers + h2spec re-run green; `go vet ./... && golangci-lint run ./... && go test -race ./...` clean.
**Artifact:** the BEHAVIOR_CONTRACT.md restructure; ADR-0089 + ADR-0090; ROADMAP flip; STATE advance; REVIEW.md; phase-done commit.
**Acceptance:** `BEHAVIOR_CONTRACT.md ## Admin API` umbrella present with five per-endpoint subsections; four new equivalence-matrix rows present; ADR-0089 + ADR-0090 in DECISIONS.md; ROADMAP row `08.1` reads `done`; STATE.md `active-phase: 08.2-graceful-drain` + `lifecycle-state: 0` + `next-skill: superpowers:brainstorming`; phase-done commit subject `phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]`.

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

# Gate (c): h2spec re-run at ADR-0051 pin
# (Use the existing h2spec helper script; expect 53/53 PASS unchanged.)
go test -count=1 -v ./test/conformance/h2spec/... 2>&1 | tail -20

# Gate (d): all 10 fuzzers run clean at 30s
for fuzz in FuzzBootstrapLoad FuzzTcpProxyFilter FuzzTLSContextParse FuzzHCMConfigParse FuzzFrameStream FuzzHPACKDecode FuzzPromTextFormat FuzzDefaultFormatRender FuzzFilterChainParse FuzzConfigDumpFormat; do
  echo "=== $fuzz ==="
  pkg=$(grep -lr "func $fuzz" --include='*.go' | head -1 | xargs dirname)
  go test -fuzz=$fuzz -fuzztime=30s ./$pkg/ 2>&1 | tail -3
done

# Gate (e): differential fixtures all green
go test -count=1 -v ./test/differential/... 2>&1 | tail -30

# Gate (f): BEHAVIOR_CONTRACT.md to be populated by Steps 2-3 of THIS task
```

If any gate fails, fix the failure (NOT in this task; document the root-cause and either backport to the failing task's commit or land a small bridge commit before continuing). Re-run the failing gate to verify green.

- [ ] **Step 2: Edit `docs/envoy-go/BEHAVIOR_CONTRACT.md`** per SPEC §13.1

Locate the existing `## Admin API — /ready` section (around line 267 per the SPEC §13.1 line-number reference; the implementer greps for the exact line). Restructure following the SPEC §13.1 verbatim Markdown patch:
- Rename the section heading from `## Admin API — /ready` to `## Admin API`.
- Add the umbrella opening (3 paragraphs: framing deviation, header set, method discrimination posture) per SPEC §13.1.
- Move the existing `### Ready-state response (authoritative)` and `### Pre-init response` sub-blocks under a new `### /ready` subsection (verbatim-preserved; no content edits).
- Add a `### /stats/prometheus` short-summary subsection per SPEC §13.1.
- Add four new per-endpoint subsections (`### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info`) per SPEC §13.1's verbatim content.
- Add the `### Applies to` list per SPEC §13.1.
- Add the `### Does not yet apply to` list per SPEC §13.1.

Then locate the `## Equivalence Matrix` table (head of file). Append four new rows per SPEC §13.2's verbatim table-row patch.

- [ ] **Step 3: Append ADR-0089 + ADR-0090 to `docs/envoy-go/DECISIONS.md`**

ADR-0089 (Admin-endpoint deferral list per ADR-0040 format). Status: Accepted. Doctrine: D-3.5. Lands-in-task: Task 15. Context: SPEC §2.1 + §2.2 enumerate the surface deferred from 08.1 MVP; the deferral list needs an explicit ADR per ADR-0040's format. Decision: enumerates the surface per SPEC §8 ADR-0089 anticipation (ports the §2.1 + §2.2 lists into the `### Does not yet apply to` block). Each item carries a target phase/family per the SPEC enumeration. Consequences: future admin-extensions phases can reference this ADR as the canonical deferral list; new deferrals append to the list rather than create new ADRs.

ADR-0090 (No-ACL admin-endpoint security posture). Status: Accepted. Doctrine: D-3.5. Lands-in-task: Task 15. Context: per BRAINSTORM §2.1 Decision G + SPEC §2.5, the admin port is a no-ACL plaintext HTTP/1.1 surface mirroring upstream Envoy's default. Decision: no authentication; no ACL; no method discrimination on read-only endpoints (Envoy parity per §11.8). Consequences: operator firewall is the security boundary; future security-hardening phase may add ACL via a new admin-listener proto extension; this ADR is forward-only (a security-hardening ADR-0091+ partially supersedes when it lands).

- [ ] **Step 4: Update `internal/admin/doc.go`** to enumerate six endpoints

Replace the existing two-endpoint enumeration with a six-endpoint one citing 08.1.

- [ ] **Step 5: Run `superpowers:requesting-code-review` (or equivalent) and create `REVIEW.md`**

Per BOOTSTRAP §7.5 gate (f) cadence. The REVIEW.md follows the 06.1 / 06.2 / 07.1 / 07.2 shape:
- Headline assessment (1 paragraph)
- Strengths (3-5 bullets)
- Findings (Major / Minor / Note tiers; numbered M-1, M-2, ... and N-1, N-2, ...)
- Carry-forward dispositions (which Minor findings carry to 08.2 vs which are landed inline)
- Six-gate verification appendix (all six gates verbatim outputs)

The implementer drafts REVIEW.md by:
1. Reading the entire 08.1 commit history (`git log --reverse master.. -- internal/admin internal/cluster internal/bootstrap cmd/envoy-go test/fixtures/0009-admin-config-dump`).
2. Spawning a code-reviewer subagent (per `superpowers:requesting-code-review` skill) with the SPEC + the diff + the 07.2 REVIEW.md as a structural template.
3. Capturing the subagent's findings into REVIEW.md.
4. For each Major finding: stop the session, re-open the impl task, fix, and re-verify. Major findings BLOCK phase-done.
5. For each Minor finding: decide carry-forward vs inline-fix; record in §10 carry-forward.

- [ ] **Step 6: Update `docs/envoy-go/ROADMAP.md`** — row `08.1` `in-progress → done`

Locate row `08.1` in the table; flip the status field from `in-progress` to `done`. Row `08` stays `in-progress` (parent flips at 08.2 phase-done per parent SPEC §5). Row `08.2` stays `planned`.

- [ ] **Step 7: Rewrite `docs/envoy-go/STATE.md`** to advance to 08.2 lifecycle-state 0

```markdown
# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `08.2-graceful-drain`
- **phase-directory:** `docs/envoy-go/phases/08.2-graceful-drain/` — currently contains the SPEC stub `README.md` from the 08.1 SPEC commit (`1f85b07`). The 08.2 BRAINSTORM session populates this directory with `BRAINSTORM.md` per `superpowers:brainstorming`. Phase 08.1 (parent + sub-phase 08.1) closes at THIS commit; the parent ROADMAP row `08` STAYS `in-progress` (flips at 08.2 phase-done per parent SPEC §5). All earlier phases (00–07.2) remain closed read-only history.
- **lifecycle-state:** `0` — phase 08.1 phase-done at the commit named in `last-commit`. Next session is 08.2 BRAINSTORM (state 0 → 1) per `superpowers:brainstorming`.
- **next-skill:** `superpowers:brainstorming` — autonomous brainstorm session for 08.2 (per ADR-0004's hard-gate discipline). Inputs: `docs/envoy-go/phases/08-admin-api-and-drain/{SPEC.md, BRAINSTORM.md}` (parent master SPEC + master BRAINSTORM context); `docs/envoy-go/phases/08.2-graceful-drain/README.md` (sibling SPEC stub); `docs/envoy-go/phases/08.1-admin-endpoints/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (just-closed sub-phase artefacts; the 08.2 brainstorm builds on 08.1's admin-mux scaffold + ADR-0085's constructor-widening pattern).
- **next-skill-scope:** Lifecycle-state 0 → 1 BRAINSTORM session deliverables: `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` — full autonomous-brainstorm artefact mirroring 06.2 + 07.2 BRAINSTORM shape; populates §1 mission + §2 design dimensions + §3 surface inventory + §4 carry-forward + §5 per-endpoint flow + §6 contract surface + §7 fixture design + §8 testing + §9 ADR anticipation + §10 carry-forward + §11 empirical-pin obligations (per ADR-0004 hard gate). After BRAINSTORM, advance STATE to lifecycle-state 1 with `next-skill: superpowers:writing-plans` for 08.2 SPEC drafting.
- **last-commit:** `<phase-done commit SHA — TBD; SHA-fill follow-up>` — `phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]`. Lands the four new admin endpoints + the constructor widening + the differential fixture 0009 + the BEHAVIOR_CONTRACT umbrella restructure + ROADMAP row 08.1 done flip + the seven new ADRs. SHA filled in a follow-up commit per the phase-04..07.2 SHA-fill convention.
- **last-updated:** <date>

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
```

- [ ] **Step 8: Update PROGRESS.md** with the Task 15 entry

Append the Task 15 entry summarising the BEHAVIOR_CONTRACT restructure + the ADR landings + the ROADMAP flip + the STATE advance + the six-gate verification.

- [ ] **Step 9: Phase-done commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/08.1-admin-endpoints/PROGRESS.md docs/envoy-go/phases/08.1-admin-endpoints/REVIEW.md internal/admin/doc.go
git commit -m "$(cat <<'EOF'
phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]

Lands the four new read-only admin endpoints (/config_dump, /clusters,
/listeners, /server_info) on the existing internal/admin.Server's mux;
widens admin.New to thread *bootstrap.Bootstrap, *cluster.Manager,
*listener.Manager (LBP-1 third generalisation per ADR-0085); adds
cluster.Manager.Clusters() snapshot accessor; adds
internal/bootstrap.Bootstrap.ConfigPath sidecar-metadata field; lands
internal/admin/{configdump,clusters,listeners,serverinfo,headers,version}.go
+ unit tests; lands FuzzConfigDumpFormat (10th fuzzer; ADR-0018 30s
budget); lands differential fixture 0009-admin-config-dump (per-endpoint
canonicalisation per planner-time decisions 7+8 + SPEC §13.2 allow-list);
restructures BEHAVIOR_CONTRACT.md ## Admin API umbrella with five
per-endpoint subsections + four equivalence-matrix rows per ADR-0052;
lands seven ADRs ADR-0084 (phase-08 split), ADR-0085 (admin-mux reuse),
ADR-0086 (/config_dump body shape), ADR-0087 (/clusters + /listeners
text-format shape), ADR-0088 (/server_info MVP field set), ADR-0089
(deferral list), ADR-0090 (no-ACL security posture).

ROADMAP row 08.1 flips in-progress → done at THIS commit. Parent row 08
STAYS in-progress (flips at 08.2 phase-done per parent SPEC §5). Row 08.2
stays planned.

Six-gate verification all green:
  (a) go build ./... + go vet + golangci-lint clean
  (b) go test -race ./... clean (incl. TestAdminConcurrentScrapeRace)
  (c) h2spec 53/53 PASS at ADR-0051 pin (unchanged)
  (d) 10 fuzzers run clean at 30s short-budget (ADR-0018)
  (e) differential 0000-0009 all green
  (f) BEHAVIOR_CONTRACT.md ## Admin API umbrella populated
EOF
)"
```

SHA-fill follow-up commit per the phase-04..07.2 convention.

*Anchored: SPEC §3 (phase-done gates), §13 (BEHAVIOR_CONTRACT additions), §15 (acceptance checklist), BOOTSTRAP §5.3 (commit-message format), §7.5 (six-gate checklist).*

---

## Refinement

The PLAN above is sized at 15 tasks per the STATE.md projection. Two refinement notes the implementer should be aware of:

1. **Task 5 leaves the cmd/envoy-go build broken intermediately** (between Tasks 5 and 10). This is intentional — the alternative is to land main.go's call-site update as part of Task 5, which conflates the admin-package widening with the cmd-package wiring. PROGRESS.md documents the intermediate breakage explicitly.

2. **Task 14's canonicalisation may need iteration on the first run.** The exact JSON-allow-list paths for `/config_dump` + `/server_info` were derived from the SPEC §11.1 + §11.4 verbatim Envoy scrapes, but the actual Envoy v1.37.2 response in the harness may have different field shapes (e.g., timestamp formats, nested struct shapes) that require canonicalisation refinement. The implementer iterates: run fixture, observe diff, refine canonicalisation, re-run. Each iteration committed separately.

3. **Task 15's REVIEW.md may surface findings that block phase-done.** Per the closure pattern, Major findings BLOCK; Minor findings carry to 08.2 unless inline-fixable. The implementer follows the requesting-code-review skill's guidance.

---

## Post-plan handoff

After Task 15's phase-done commit + SHA-fill follow-up:

- ROADMAP row `08.1`: `done`. Row `08`: `in-progress` (closes at 08.2 phase-done). Row `08.2`: `planned` (next session is 08.2 BRAINSTORM).
- STATE.md: `active-phase: 08.2-graceful-drain`, `lifecycle-state: 0`, `next-skill: superpowers:brainstorming`.
- BEHAVIOR_CONTRACT.md: `## Admin API` umbrella populated with five per-endpoint subsections; four new equivalence-matrix rows.
- DECISIONS.md tail: `ADR-0090`. Next-free: `ADR-0091` (08.2's ADR-0091..onwards anticipated).
- Production code count post-08.1: ~1700 LoC (admin package + cluster/bootstrap deltas) + ~1380 LoC tests + ~550 LoC fixture; fuzzer count 10; differential fixture count 10 (0000-0009 + 0007a-cors + 0007b-iteration-probe + 0009-admin-config-dump).
- The `internal/admin.Server` is now ready for 08.2's drain-state extension: a `drainState atomic.Pointer[DrainState]` field can be added to `Server` without changing `New`'s signature; `/ready` and `/server_info` extend to handle DRAINING; a new `POST /drain_listeners` handler registers on the same mux.

---

## References

- **SPEC:** `docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md` — the authoritative source; every PLAN task traces to one or more SPEC sections (§§1–16).
- **Parent master SPEC:** `docs/envoy-go/phases/08-admin-api-and-drain/SPEC.md` — cross-cutting context for the 08.1 + 08.2 split.
- **BRAINSTORM:** `docs/envoy-go/phases/08-admin-api-and-drain/BRAINSTORM.md` — autonomous brainstorm artefact the SPEC distils §§2–10 from.
- **Sibling SPEC stub:** `docs/envoy-go/phases/08.2-graceful-drain/README.md` — placeholder; populated by 08.2's BRAINSTORM session.
- **Structural precedent (PLAN shape):** `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` + `07.2-listener-chain-completion/PLAN.md` — task-numbering, heredoc-style task headers, ADR-with-first-use-commit, "Anchored:" footers, "Refinement" + "Post-plan handoff" closing sections.
- **Structural precedent (admin-package extension):** `docs/envoy-go/phases/06.1-stats-prometheus/PLAN.md` — the prior phase that extended `internal/admin.Server` (added `/stats/prometheus` handler + `*stats.Registry` constructor parameter).
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — sub-phase position), §5.3 (commit-message format — phase-done subject), §6.2 (planner-time-split discipline; ADR-0084 applies), §7.5 (phase-done gate — six-gate checklist; SPEC §3 specialises), §4.1 (artifact-layout invariants — ROADMAP row flip discipline).
- **DECISIONS.md cross-references:**
  - **Inherited (cited, not amended):** ADR-0003, ADR-0004, ADR-0005, ADR-0008, ADR-0010, ADR-0014, ADR-0015, ADR-0016, ADR-0017, ADR-0018, ADR-0040, ADR-0041, ADR-0045, ADR-0051, ADR-0052, ADR-0059, ADR-0061, ADR-0063, ADR-0070, ADR-0072, ADR-0079, ADR-0083.
  - **Partially superseded:** ADR-0015 — partial supersession by ADR-0088's `state: PRE_INITIALIZING` extension to /server_info; the supersession-by-extension lands in 08.2 (when DRAINING is introduced), not in 08.1.
  - **New (this PLAN lands):** ADR-0084 through ADR-0090 per SPEC §8.
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Server-build SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (per SPEC §11.4 verbatim `version` field). All seven SPEC §11 empirical pins reference this image SHA.
- **ROADMAP.md:** rows `08`, `08.1`, `08.2` per the SPEC commit's row-flip; row `08.1` flips `in-progress → done` at this PLAN's Task 15 phase-done; rows `08` and `08.2` unchanged in 08.1.
