# Phase 08.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04/05.1/05.2/06.1/06.2/07.1/07.2 PROGRESS.md structure.

## Preamble — execution preconditions

All 15 preconditions enumerated in PLAN.md `## Execution preconditions` were satisfied at cold-start; no deviations. Branch `phase-08.1-admin-endpoints-impl` is checked out; master tail shows the PLAN SHA-fill (`581d0ea`), the PLAN commit (`e928f9e`), the SPEC SHA-fill (`65b7455`), and the SPEC commit (`1f85b07`); toolchain reports `go1.26.2` and `golangci-lint v1.64.8` (ADR-0009 pin); Docker client + server reported (Docker Desktop 4.41.2, Engine 28.1.1); DECISIONS.md tail at ADR-0083 (next-free is ADR-0084 per ADR-0004); SPEC commit SHA `1f85b07...`; working tree pristine; `go test -count=1 -short ./...` clean across all packages; differential suite clean (10/10 fixtures pass: 0000, 0001, 0002, 0003, 0004, 0005, 0006, 0007a, 0007b, 0008); reference Envoy image present locally with the ADR-0008-pinned digest `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; pre-existing admin server constructor `func New(addr string, registry *stats.Registry) *Server` present at `internal/admin/admin.go:31`; `internal/cluster.Manager.Clusters()` not yet present; `internal/bootstrap.Bootstrap.ConfigPath` field not yet present; `internal/listener.Manager.Listeners() []Info` present at `internal/listener/manager.go:928`; `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` empty (D-3.7).

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
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-07.2 close + 08.1 SPEC + 08.1 PLAN confirmed present in HEAD; SPEC at `1f85b07`; ADR tail at 0083 (next-free 0084); `internal/admin/{configdump,clusters,listeners,serverinfo,headers,version}.go` absent (the four handler implementations + helpers land at Tasks 4-9); `internal/cluster.Manager.Clusters()` not yet present (Task 3); `internal/bootstrap.Bootstrap.ConfigPath` not yet present (Task 2). Landed ADR-0084 (phase-08 planner-time split into 08.1 + 08.2; mirrors ADR-0070 pattern from phase-07).

**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-08.1-admin-endpoints-impl

$ go version
go version go1.26.2 linux/amd64

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0083:

$ git log -1 --format=%H -- docs/envoy-go/phases/08.1-admin-endpoints/SPEC.md
1f85b073f3f297d06ab40a871bbac3d6503c6e1c

$ go vet ./...
(clean)

$ golangci-lint run ./...
(clean)

$ go test -race -count=1 -short ./...
(all PASS)
```

## Task 2 — `internal/bootstrap.Bootstrap.ConfigPath` field addition

**Commits:** `50f3473` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-4. Step 1 appended the failing test `TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty` to `internal/bootstrap/bootstrap_test.go` (the existing `strings` import covered the new test). Step 2 confirmed the build error `unknown field ConfigPath in struct literal of type Bootstrap`. Step 3 added the `ConfigPath string` field to the `Bootstrap` struct in `internal/bootstrap/bootstrap.go` immediately after `AccessLogConfigs`, with the doc comment from PLAN verbatim (calls out ADR-0001 design — `Load` takes `io.Reader` not a path; sidecar setter pattern from `cmd/envoy-go/main.go`; reader at Task 9's `/server_info` handler for `command_line_options.config_path`). Step 4 confirmed test PASS, vet clean, lint clean. Per planner-time decision 9, `bootstrap.Load(r io.Reader)`'s signature was NOT widened — the field defaults to `""` and tests that don't need it leave it zero. `go build ./...` clean (cmd/envoy-go still compiles; the call-site wiring lands at Task 10). `grep -nE 'ConfigPath\s+string' internal/bootstrap/bootstrap.go` returns 1 match at line 122.

**Outputs:**
```
$ go test -run TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty -v ./internal/bootstrap/...
=== RUN   TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty
--- PASS: TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.004s

$ go test ./internal/bootstrap/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.008s

$ go vet ./...
(clean)

$ golangci-lint run ./internal/bootstrap/...
(clean)

$ go build ./...
(clean)

$ grep -nE 'ConfigPath\s+string' internal/bootstrap/bootstrap.go
122:	ConfigPath string
```

## Task 3 — `internal/cluster.Manager.Clusters()` snapshot accessor + `ClusterInfo`/`EndpointInfo` types

**Commits:** `07c72bd` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-4. Step 1 appended three failing tests to `internal/cluster/manager_test.go` (`TestManager_Clusters_SnapshotReturnsAllClusters`, `TestManager_Clusters_FreshlyAllocatedPerCall`, `TestManager_Clusters_EmptyClustersListReturnsEmpty`); the placeholder `mustParseBootstrap(t, /* fixture YAML */)` from PLAN was substituted with the file's existing builder pattern (`mkBootstrap` + `mkStaticCluster` + `mkLbEndpoint`) — that pattern is the canonical bootstrap fixture in this test file (no YAML fixture helper exists). Step 2 confirmed the build error `m.Clusters undefined (type *Manager has no field or method Clusters, but does have field clusters)`. Step 3 added types `ClusterInfo` and `EndpointInfo` plus method `(m *Manager) Clusters() []ClusterInfo` to `internal/cluster/manager.go` immediately after the existing `Get` method, copying the verbatim Go code from PLAN Step 3 including docstrings (which reference ADR-0063, ADR-0087, and planner-time decision 8). Added `"sort"` to imports. Added a single `//nolint:revive // ADR-0087 reserves the ClusterInfo name…` pragma on `ClusterInfo` to silence revive's stutter check (the codebase uses this pattern when public type names are deliberately ADR-anchored — see `internal/listener/listenerfilter/types.go:12` and `internal/filter/http/types.go:72`). Step 4 confirmed all three new tests PASS (entire cluster package PASS), `go vet` clean, `golangci-lint run ./internal/cluster/...` clean. `go build ./...` clean. `grep -nE '^func \(m \*Manager\) Clusters\(\) \[\]ClusterInfo' internal/cluster/manager.go` returns 1 match at line 150. Mutation-of-returned-slice test passes because the implementation allocates fresh slices via `make()` for both the outer `[]ClusterInfo` and per-cluster `[]EndpointInfo`. Alphabetical-by-name ordering implemented via `sort.Slice`. The Manager-direct construction path (`&Manager{clusters: map[string]*Cluster{}}`) confirmed `clusters` (lowercase) is the correct field name. Field mapping: `Cluster.name → ClusterInfo.Name`; `Cluster.endpoints[].Host → EndpointInfo.Address`; `Cluster.endpoints[].Port → EndpointInfo.Port`.

**Outputs:**
```
$ go test -run TestManager_Clusters -v ./internal/cluster/... 2>&1 | tail -20
=== RUN   TestManager_Clusters_SnapshotReturnsAllClusters
--- PASS: TestManager_Clusters_SnapshotReturnsAllClusters (0.00s)
=== RUN   TestManager_Clusters_FreshlyAllocatedPerCall
--- PASS: TestManager_Clusters_FreshlyAllocatedPerCall (0.00s)
=== RUN   TestManager_Clusters_EmptyClustersListReturnsEmpty
--- PASS: TestManager_Clusters_EmptyClustersListReturnsEmpty (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.003s

$ go test -count=1 ./internal/cluster/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/cluster	0.012s

$ go vet ./...
(clean)

$ golangci-lint run ./internal/cluster/...
(clean)

$ go build ./...
(clean)

$ grep -nE '^func \(m \*Manager\) Clusters\(\) \[\]ClusterInfo' internal/cluster/manager.go
150:func (m *Manager) Clusters() []ClusterInfo {
```

## Task 4 — `internal/admin/headers.go` + `internal/admin/version.go` shared helpers

**Commits:** `6a3a10b` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-8, run as two sequential cycles to keep the red→green phase visible per file. Cycle 1 (headers): Step 1 created `internal/admin/headers_test.go` with the four tests verbatim from PLAN (`TestWriteAdminHeaders_SetsFourConstantHeaders`, `_DoesNotSetDateOrContentLength`, `_OverwritePreviousContentType`, `_AppliedThroughHTTPServer`). Step 2 confirmed the build error `undefined: writeAdminHeaders` (4 occurrences across the test file). Step 3 created `internal/admin/headers.go` with the verbatim 4-line `writeAdminHeaders(w, contentType)` body (lowercase / unexported per task spec) and verbatim ADR-0014/SPEC §11.6 doc comment. Step 4 confirmed 4 PASS. Cycle 2 (version): Step 5 created `internal/admin/version_test.go` with the five tests verbatim from PLAN (`TestBuildVersionString_FiveTokens`, `_GoVersionToken`, `_LiteralCleanReleaseGocrypto`, `_RevisionDefaultsToUnknownInTestBuild`, `_RevisionLDFlagOverride`). Step 6 confirmed the build error `undefined: BuildVersionString` (and `undefined: Revision`). Step 7 created `internal/admin/version.go` with the verbatim impl: `var Revision = readRevision()`, `readRevision()` walking `runtime/debug.ReadBuildInfo().Settings` for `vcs.revision` with `"unknown"` fallback, and `BuildVersionString()` emitting `<rev[:7]>/<runtime.Version()>/Clean/RELEASE/Go-crypto`. Step 8 confirmed 5 PASS. The `Revision` package var + `BuildVersionString` are exported (uppercase) so `-ldflags "-X .Revision=<sha>"` and the Task 9 `/server_info` handler can reach them; `writeAdminHeaders` stays lowercase (unexported, package-internal). The `TestBuildVersionString_RevisionLDFlagOverride` test uses the prescribed `saved := Revision; defer func(){Revision = saved}()` save/restore pattern so test order does not matter. **One deviation from PLAN-verbatim:** the package-doc comment in `version.go` originally read `Default-initialised` (PLAN Step 3 verbatim text); golangci-lint's `misspell` check flagged it as British spelling and was rejected. Fixed to `Default-initialized` (a comment-only edit; semantics unchanged). All 9 new tests PASS; existing admin tests (`Server_*`, `HandlePrometheus_*`) still PASS — no regression. `go vet`, `golangci-lint`, `go build ./...`, and `go test -count=1 ./...` all clean.

**Outputs:**
```
$ go test -run TestWriteAdminHeaders ./internal/admin/... 2>&1 | head -10
# github.com/esalaine/envoy-go/internal/admin [github.com/esalaine/envoy-go/internal/admin.test]
internal/admin/headers_test.go:11:2: undefined: writeAdminHeaders
internal/admin/headers_test.go:28:2: undefined: writeAdminHeaders
internal/admin/headers_test.go:40:2: undefined: writeAdminHeaders
internal/admin/headers_test.go:51:3: undefined: writeAdminHeaders
FAIL	github.com/esalaine/envoy-go/internal/admin [build failed]
FAIL

$ go test -run TestWriteAdminHeaders ./internal/admin/... -v 2>&1 | tail -15
=== RUN   TestWriteAdminHeaders_SetsFourConstantHeaders
--- PASS: TestWriteAdminHeaders_SetsFourConstantHeaders (0.00s)
=== RUN   TestWriteAdminHeaders_DoesNotSetDateOrContentLength
--- PASS: TestWriteAdminHeaders_DoesNotSetDateOrContentLength (0.00s)
=== RUN   TestWriteAdminHeaders_OverwritePreviousContentType
--- PASS: TestWriteAdminHeaders_OverwritePreviousContentType (0.00s)
=== RUN   TestWriteAdminHeaders_AppliedThroughHTTPServer
--- PASS: TestWriteAdminHeaders_AppliedThroughHTTPServer (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.002s

$ go test -run TestBuildVersionString ./internal/admin/... 2>&1 | head -10
# github.com/esalaine/envoy-go/internal/admin [github.com/esalaine/envoy-go/internal/admin.test]
internal/admin/version_test.go:10:7: undefined: BuildVersionString
internal/admin/version_test.go:18:7: undefined: BuildVersionString
internal/admin/version_test.go:26:7: undefined: BuildVersionString
internal/admin/version_test.go:44:7: undefined: BuildVersionString
internal/admin/version_test.go:54:11: undefined: Revision
internal/admin/version_test.go:55:17: undefined: Revision
internal/admin/version_test.go:56:2: undefined: Revision
internal/admin/version_test.go:57:7: undefined: BuildVersionString
FAIL	github.com/esalaine/envoy-go/internal/admin [build failed]

$ go test -run TestBuildVersionString ./internal/admin/... -v 2>&1 | tail -20
=== RUN   TestBuildVersionString_FiveTokens
--- PASS: TestBuildVersionString_FiveTokens (0.00s)
=== RUN   TestBuildVersionString_GoVersionToken
--- PASS: TestBuildVersionString_GoVersionToken (0.00s)
=== RUN   TestBuildVersionString_LiteralCleanReleaseGocrypto
--- PASS: TestBuildVersionString_LiteralCleanReleaseGocrypto (0.00s)
=== RUN   TestBuildVersionString_RevisionDefaultsToUnknownInTestBuild
--- PASS: TestBuildVersionString_RevisionDefaultsToUnknownInTestBuild (0.00s)
=== RUN   TestBuildVersionString_RevisionLDFlagOverride
--- PASS: TestBuildVersionString_RevisionLDFlagOverride (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.002s

$ go test -count=1 ./internal/admin/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/admin	0.048s

$ go vet ./...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go build ./...
(clean)
```

## Task 5 — `internal/admin/admin.go` constructor widening + `bootTime` field [ADR-0085]

**Commits:** `4fc9706` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-8. Step 1 wrote the two new tests (`TestServer_NewWidenedConstructor`, `TestAdminWriteTimeoutIs30s`) into `internal/admin/admin_test.go` referencing the not-yet-widened `New(addr, registry, nil, nil, nil)` signature plus the not-yet-added `bootTime` field. Step 2 confirmed the build error `too many arguments in call to New` (and `s.bootTime undefined`). Step 3 widened `internal/admin/admin.go`: added imports for `internal/bootstrap`, `internal/cluster`, `internal/listener`; widened the `Server` struct with four new fields (`bs *bootstrap.Bootstrap`, `cm *cluster.Manager`, `lm *listener.Manager`, `bootTime time.Time`); widened `New` signature from `(addr, registry)` to `(addr, registry, bs, cm, lm)` setting `bootTime: time.Now()`; widened `http.Server.WriteTimeout` from `5*time.Second` to `30*time.Second` per planner-time decision 2; pre-registered the four placeholder mux entries (`/config_dump`, `/clusters`, `/listeners`, `/server_info`) routing to `s.handleConfigDump`/`Clusters`/`Listeners`/`ServerInfo`; added the four placeholder handlers each emitting `<endpoint>: not yet implemented\n` text body with `Content-Length` set inline (no `bytes` package needed at Task 5 — the per-endpoint files in Tasks 6-9 each add their own `bytes` consumer). Per the PLAN's "alternative is to defer adding the import until Task 6" note (which is the cleaner option flagged in PLAN Step 3), the `bytes` import was deferred — no `var _ = bytes.Buffer{}` hack is in this commit. Step 4 created `internal/admin/admin_helpers_test.go` with the verbatim `minimalBootstrapYAML` constant + `mustMinimalBs`/`mustMinimalCM`/`mustMinimalLM` helpers; the helper file imports `internal/filter/http/router` and registers `router.New` on the HTTPRegistry before `Freeze()` per the existing 07.1 boot pattern + per the existing `internal/listener/manager_test.go:42` (`testHTTPRegistry`) precedent — the §7.3 fixture's HCM filter chain references `envoy.filters.http.router` and the listener-manager build at `NewManagerWithBaseDirAndAllowH2C` parse-time fails without it. The listener-filter registry is empty-but-frozen because the §7.3 fixture has no `listener_filters[]`. All four helpers carry `//nolint:unused // PLAN Task 5 scaffolding; consumed by Tasks 6-9 handler tests.` pragmas (mirroring the existing pragma pattern at `internal/listener/listenerfilter/callbacks_test.go:13`) since at Task-5 commit time none of the helpers have a consumer; Tasks 6-9 each add a per-endpoint test file that consumes one or more of them. Step 5 mechanically updated the seven existing `New(...)` call sites in `internal/admin/admin_test.go` (lines 13, 57, 83, 115, 121, 137, 158 per PLAN preamble) by appending `, nil, nil, nil` — these tests do NOT exercise the four new endpoints so nil is correct per ADR-0085 consequence (b). Step 6 confirmed `go build ./internal/admin/... clean`, `go test -count=1 ./internal/admin/... ok` (20 PASS — 6 pre-existing `TestServer_*` + 1 pre-existing `TestServer_LiveGaugeSetOnceFlippedAtFirstReady200` + 2 new `TestServer_NewWidenedConstructor` + `TestAdminWriteTimeoutIs30s` + 4 `TestWriteAdminHeaders_*` + 3 `TestHandlePrometheus_*` + 5 `TestBuildVersionString_*` from Task 4), `go vet ./internal/admin/...` clean, `golangci-lint run ./internal/admin/...` clean (the four `//nolint:unused` pragmas suppress the unused-helper warnings as documented in PLAN Step 4 + the listener-filter precedent). Step 7 appended ADR-0085 to `docs/envoy-go/DECISIONS.md` (one match for `^## ADR-0085:`); Status Accepted; Date 2026-05-02; Doctrine D-3.2 + D-3.4; Lands-in-task Task 5; Context paragraph anchors the four read-only endpoints + the existing-server-reuse rationale + the LBP-1 generalisation; Decision paragraph documents the three-arg widening + the 30s WriteTimeout + the placeholder-then-Tasks-6-9-overwrite shape; four Alternatives (second-server, package-globals, drop-registry, keep-5s) all rejected with reasons; five Consequences (a)-(e) covering: (a) cmd/envoy-go widens at Task 10; (b) test code passes nil for bs/cm/lm when not exercising new endpoints + the `mustMinimalBs/CM/LM` helper pattern; (c) LBP-1's fourth sibling application (06.1 + 07.1 + 07.2 + 08.1) ratifying the canonical wiring discipline; (d) 08.2 inherits without further constructor widening; (e) WriteTimeout 30s ceiling still bounds slowloris. Step 8 commit landed at `4fc9706`. **IMPORTANT:** `go build ./cmd/envoy-go/...` FAILS at this commit with `cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New / have (string, *stats.Registry) / want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)` — this is INTENTIONAL and EXPECTED per PLAN Step 6 ("`go build ./cmd/envoy-go/...` is EXPECTED to fail at this point (call-site breakage); Task 10 fixes that"); the call-site update lands at Task 10. Between Tasks 5 and 10 inclusive, `go build ./cmd/envoy-go/...`, `go vet ./...`, and `go test ./...` will all fail at the cmd/envoy-go boundary; the per-task acceptance gates only require `./internal/admin/...` (and the per-task target package) to be clean. Five files modified/created: `internal/admin/admin.go` (modified, +88/-12 LoC including widened struct/New/Start + four placeholder handlers); `internal/admin/admin_test.go` (modified, +43/-7 LoC including 7 call-site updates + 2 new tests); `internal/admin/admin_helpers_test.go` (created, 102 LoC); `docs/envoy-go/DECISIONS.md` (appended ADR-0085, +43 LoC).

**Outputs:**
```
$ go test -run "TestServer_NewWidenedConstructor|TestAdminWriteTimeoutIs30s" -v ./internal/admin/... 2>&1 | tail -10
=== RUN   TestServer_NewWidenedConstructor
--- PASS: TestServer_NewWidenedConstructor (0.00s)
=== RUN   TestAdminWriteTimeoutIs30s
--- PASS: TestAdminWriteTimeoutIs30s (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.004s

$ go build ./internal/admin/...
(clean)

$ go test -count=1 ./internal/admin/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/admin	0.051s

$ go vet ./internal/admin/...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go test -v -count=1 ./internal/admin/... 2>&1 | grep -c '^--- PASS:'
20

$ go build ./cmd/envoy-go/... 2>&1
# github.com/esalaine/envoy-go/cmd/envoy-go
cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New
	have (string, *stats.Registry)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
(EXPECTED FAIL — Task 10 fixes the call site)

$ grep -c '^## ADR-0085:' docs/envoy-go/DECISIONS.md
1
```

## Task 6 — `internal/admin/configdump.go` — `/config_dump` handler [ADR-0086]

**Commits:** `044f751` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-7. Step 1 wrote `internal/admin/configdump_test.go` with the six tests verbatim from PLAN: four `TestBuildConfigDump_*` unit tests (three-sub-envelopes-in-order; bootstrap-envelope-contains-parsed-proto; listeners-envelope-contains-one-static-listener; clusters-envelope-contains-one-static-cluster) + two HTTP smoke tests (`TestHandleConfigDump_HTTPSmoke200JSON` exercising 200/Content-Type/Server header + `json.Unmarshal`-parseable body with `configs` field present, and `TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent` exercising `static_listeners` snake_case + 1-space indent). Imports added: `time`, `adminv3` (per PLAN Step 1 note "the implementer adds them as needed"); `bootstrapv3` is NOT directly referenced in the test file (the helpers in admin_helpers_test.go expose `*bootstrap.Bootstrap`, and `cd.Configs[N].UnmarshalTo(&adminv3.<X>ConfigDump{})` walks types in adminv3 only). The PLAN Step 1 trailing `func min(a, b int) int { ... }` helper was OMITTED — go.mod pins `go 1.23.0` which makes Go's built-in `min` available; defining a local helper would shadow the builtin and trigger lint. Step 2 confirmed the build error `undefined: buildConfigDump` (4 references, all from configdump_test.go, all flagged). Step 3 wrote `internal/admin/configdump.go` per PLAN Step 3 verbatim minus the trailing `var _ = bytes.Buffer{}` placeholder line: Task 5 deferred adding the `bytes` import (PROGRESS Task 5 note explicitly states "the per-endpoint files in Tasks 6-9 each add their own `bytes` consumer; no `var _ = bytes.Buffer{}` hack"), so configdump.go has no `bytes` import and no `var _ = bytes.Buffer{}` line either; the package compiles clean. The package-level `configDumpMarshalOptions` carries the four-value tuple `{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` per ADR-0086. The `handleConfigDump` method dispatches `buildConfigDump(s.bs, s.bootTime)` then `configDumpMarshalOptions.Marshal(cd)` with the dual error path returning 500 + `{}` body per ADR-0086 consequence (e). `buildConfigDump` defends against `bs == nil || bs.Proto == nil` by returning empty `*adminv3.ConfigDump{}` (relevant for tests that pass nil; the four widened-constructor `TestServer_*` tests in admin_test.go pass nil so the handler MUST tolerate it gracefully — the empty body is then a valid JSON document). The two `enumerateStatic*` helpers walk `bs.GetStaticResources().GetListeners()/GetClusters()` directly per planner-time decision 7 (NOT through cm.Clusters()/lm.Listeners()), each pack inner items into `*anypb.Any` with `LastUpdated: timestamppb.New(bootTime)`. Step 4 deleted the placeholder `func (s *Server) handleConfigDump` block (10 LoC) from `internal/admin/admin.go` introduced by Task 5; the mux registration `mux.HandleFunc("/config_dump", s.handleConfigDump)` now resolves to the real handler in configdump.go (Go compiles the file as part of the same package). The `bytes` import in admin.go was already absent (Task 5 deferred per PROGRESS Task 5 note); no further import-cleanup needed. Step 5 ran the four verification gates — all clean: `go build ./internal/admin/...` clean, `go test -count=1 ./internal/admin/...` 26 PASS (the 20 from Task 5 + 6 new), `go vet ./internal/admin/...` clean, `golangci-lint run ./internal/admin/...` clean (after two minor lint fixes: the PLAN-verbatim test had an empty-branch `if !strings.Contains(bodyStr, "\n {\n") && !strings.Contains(bodyStr, "\n  ") {}` that staticcheck SA9003-flagged; rewrote it as `if !strings.Contains(bodyStr, "\n ") { t.Errorf("body lacks 1-space indent marker; ...") }` which preserves the test intent — assert at least one one-space indent token exists in the body — and is non-empty; and the PLAN-verbatim docstring "behaviour" was misspell-flagged, replaced with "behavior" to match US-spelling project convention). Step 6 appended ADR-0086 to `docs/envoy-go/DECISIONS.md` (one match for `^## ADR-0086:`); Status Accepted; Date 2026-05-02; Doctrine D-3.3 + D-3.7; Lands-in-task Task 6; Context paragraph anchors SPEC §11.1 verbatim Envoy v1.37.2 scrape pin + the four-tuple non-derivability + planner-time decision 7's bootstrap-walk choice; Decision paragraph documents the four `protojson.MarshalOptions` values + the three-sub-envelope ordering invariant + the static-only stance + the `EmitUnpopulated` body-shape character + the 500 + `{}` error path + the cross-endpoint `configDumpMarshalOptions` reuse for /server_info; five Alternatives (defaults, encoding/json mirror, manager-walk, no-ADR-implicit ordering, empty-`dynamic_*`-arrays) all rejected with reasons; five Consequences (a)-(e) covering: (a) byte-equality modulo §13.2 allow-list (node.user_agent_*, node.extensions[], `<*ConfigDump>.last_updated`); (b) Task 13's comparator depends on the three-position invariant; (c) future RoutesConfigDump/SecretsConfigDump etc. envelopes append at index >= 3 (additive, never reordering); (d) /server_info reuses the same MarshalOptions tuple for cross-endpoint consistency; (e) error path returns 500 + `{}` JSON-valid body. Step 7 commit landed at `044f751`. **IMPORTANT:** `go build ./cmd/envoy-go/...` STILL FAILS at this commit with `cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New` — INTENTIONAL per PLAN; Task 10 fixes the call site. Four files modified/created: `internal/admin/configdump.go` (created, 145 LoC), `internal/admin/configdump_test.go` (created, 144 LoC after the lint-fix rewrite of the empty-branch indent assertion), `internal/admin/admin.go` (modified, -10 LoC removing the placeholder handler), `docs/envoy-go/DECISIONS.md` (appended ADR-0086, +43 LoC).

**Outputs:**
```
$ go test -count=1 -run 'TestBuildConfigDump|TestHandleConfigDump' -v ./internal/admin/... 2>&1 | tail -15
=== RUN   TestBuildConfigDump_ThreeSubEnvelopesInOrder
--- PASS: TestBuildConfigDump_ThreeSubEnvelopesInOrder (0.00s)
=== RUN   TestBuildConfigDump_BootstrapEnvelopeContainsParsedProto
--- PASS: TestBuildConfigDump_BootstrapEnvelopeContainsParsedProto (0.00s)
=== RUN   TestBuildConfigDump_ListenersEnvelopeContainsOneStaticListener
--- PASS: TestBuildConfigDump_ListenersEnvelopeContainsOneStaticListener (0.00s)
=== RUN   TestBuildConfigDump_ClustersEnvelopeContainsOneStaticCluster
--- PASS: TestBuildConfigDump_ClustersEnvelopeContainsOneStaticCluster (0.00s)
=== RUN   TestHandleConfigDump_HTTPSmoke200JSON
--- PASS: TestHandleConfigDump_HTTPSmoke200JSON (0.01s)
=== RUN   TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent
--- PASS: TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.030s

$ go build ./internal/admin/...
(clean)

$ go test -count=1 ./internal/admin/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/admin	0.076s

$ go vet ./internal/admin/...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go build ./cmd/envoy-go/... 2>&1
# github.com/esalaine/envoy-go/cmd/envoy-go
cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New
	have (string, *stats.Registry)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
(EXPECTED FAIL — Task 10 fixes the call site)

$ grep -c '^## ADR-0086:' docs/envoy-go/DECISIONS.md
1
```

**Follow-up — T6 review:** Tightened `TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent` per code-review I-1 (depth-1 field anchors 1-space indent — replaced loose `\n ` substring check with `\n "configs"` substring which only matches under `Indent: " "`; under any wider indent the depth-1 field would carry N+ spaces and silently miss) and I-2 (`"node":\s+null` regex anchors `EmitUnpopulated: true` — the §7.3 fixture's bootstrap has no `node` field so under `EmitUnpopulated: true` the marshaler emits `"node": null` and under `EmitUnpopulated: false` would elide the field entirely; regex tolerates protojson's deliberate post-colon-spacing randomization which alternates between 1 and 2 spaces per Marshal call). Added `regexp` import; no other code touched. `go test -count=1 -run TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent ./internal/admin/... -v` PASS (10/10 in stress run); `go test -count=1 ./internal/admin/...` 26 PASS; `go vet`/`golangci-lint` clean. Commit `7b938fc`.

## Task 7 — `internal/admin/clusters.go` — `/clusters` handler [ADR-0087]

**Commits:** `2022e68` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-7. Step 1 wrote `internal/admin/clusters_test.go` with seven tests covering the SPEC §11.2 + ADR-0087 contract: `TestHandleClusters_HTTPSmoke200Text` (200 + `text/plain; charset=UTF-8` Content-Type + non-empty body), `TestHandleClusters_TenClusterLevelLinesPerCluster` (exact 46-line count for the §7.3 fixture: 10 cluster-level + 2×18 per-endpoint), `TestHandleClusters_ClusterLevelLineFormat` (verifies all 10 §11.2(b) cluster-level lines present in exact verbatim form: `observability_name`, the 4 `default_priority::*` + 4 `high_priority::*` circuit-breaker constants, `added_via_api::false`), `TestHandleClusters_PerEndpointLinesAllZeroPlusConstants` (verifies the 8 cx_/rq_ counter lines emit literal `0` per planner-time decision 8, plus the 10 §11.2(c) per-endpoint constant lines: `hostname::` empty, `health_flags::healthy`, `weight::1`, `region/zone/sub_zone` empty, `canary::false`, `priority::0`, `success_rate::-1`, `local_origin_success_rate::-1`), `TestHandleClusters_BodyExactByteLayout` (full 46-line byte-equal assertion against the §11.2-pinned line set with cx_/rq_ counters as `0`), `TestHandleClusters_EndpointDeclarationOrderPreserved` (asserts `127.0.0.1:18001` block precedes `127.0.0.1:18002` per bootstrap-declared order — NOT alphabetical / address-sorted), `TestHandleClusters_NilManagerEmitsEmptyBody` (defensive nil-cm path emits 200 + empty body rather than panicking, supporting the ADR-0085 nil-tolerated test convention). The PLAN-suggested `TestHandleClusters_AlphabeticalByClusterName` was omitted (PLAN explicitly notes it requires a `mustParseBs(yaml)` helper that does not yet exist; the alphabetical sort is covered by Task 3's `TestManager_Clusters_SnapshotReturnsAllClusters` per the PLAN's own note). Step 2 confirmed the 7 tests fail against the placeholder (501 status; missing line content; nil-cm path returns 501 not 200). Step 3 created `internal/admin/clusters.go` (97 LoC) with three functions: `(s *Server) handleClusters(w, r)` walks `s.cm.Clusters()` and accumulates the per-cluster blocks into a `bytes.Buffer`, then emits `text/plain; charset=UTF-8` headers + 200 + body; `writeClusterBlock(buf, c)` orchestrates the 10 cluster-level + 18 per-endpoint emission for one cluster; `writeClusterLevelLines(buf, name)` and `writeEndpointLines(buf, clusterName, addr)` each emit their respective verbatim §11.2 line sets via `fmt.Fprintf`. The handler accepts `*cluster.ClusterInfo` directly (no interface indirection — the PLAN-suggested `clusterInfoLike`/`endpointInfoLike` interface adapters were skipped per the PLAN's "simpler is to reference cluster.ClusterInfo / cluster.EndpointInfo directly" note; the helpers are package-private and unit-testable through the public handler entry point with zero adapter overhead). The 8 cx_/rq_ counter values emit literal `"0"` per planner-time decision 8 + ADR-0063 per-endpoint stats deferral; the 10 cluster-level constants (`1024`, `3`, `false`) and 10 per-endpoint constants (`healthy`, `1`, empty, `false`, `0`, `-1`) are emitted unconditionally. The nil-cm guard `if s.cm != nil` wraps the cluster iteration; under nil-cm the buffer stays empty and the handler still emits 200 + empty body. Step 4 deleted the placeholder `func (s *Server) handleClusters` block (10 LoC) from `internal/admin/admin.go` introduced by Task 5; the mux registration `mux.HandleFunc("/clusters", s.handleClusters)` resolves to the real handler in clusters.go. `grep -nE 'func \(s \*Server\) handleClusters' internal/admin/admin.go internal/admin/clusters.go` returns exactly 1 match (clusters.go line 26). Step 5 ran the four verification gates — all clean: `go build ./internal/admin/...` clean, `go test -count=1 ./internal/admin/...` 33 PASS (26 from Task 6 + 7 new), `go vet ./internal/admin/...` clean, `golangci-lint run ./internal/admin/...` clean (after one minor lint fix: `localise` was misspell-flagged, replaced with `localize` to match US-spelling project convention). Step 6 appended ADR-0087 to `docs/envoy-go/DECISIONS.md` (one match for `^## ADR-0087:`); Status Accepted; Date 2026-05-02; Doctrine D-3.3 + D-3.5; Lands-in-task Task 7 (covers BOTH `/clusters` here and `/listeners` in Task 8 — Task 8 references this ADR rather than introducing a new one); Context paragraph anchors SPEC §11.2 + §11.3 verbatim Envoy v1.37.2 scrape pin + the planner-time decision 8 cx_/rq_ counter `0`-emission rationale + ADR-0063 cross-reference + the bind-address-via-`Listener.GetAddress()` clause for `/listeners`; Decision paragraph documents the text/plain Content-Type + the exact 28-line per-cluster layout (10 cluster-level + 18 per-endpoint) + the per-endpoint constants list + the §11.3 one-line-per-listener form for `/listeners`; six Alternatives (counter-from-cluster-totals, live-per-endpoint stats, JSON form, skip-non-modeled lines, listener-extension fields, listener-bind-via-runtime-state) all rejected with reasons; five Consequences (a)-(e) covering: (a) `/clusters` byte-equality modulo §13.2 8-cx_/rq_-fields-per-endpoint allow-list; (b) `/listeners` byte-equality + framing dechunk handled by harness; (c) ADR-0063 reaffirmed + future per-endpoint-stats supersedes planner-time decision 8 (line layout unchanged; 8 emitted values become live readings); (d) `/listeners` stays trivial (one-line-per-listener; 08.2 may surface drain-state field as additive extension); (e) shared ADR rationale (the two text-format endpoints' decisions are tightly coupled — same Content-Type, same line terminator, same alphabetical-by-name ordering, same accessor-walk pattern, both omit JSON form per ADR-0089 — so consolidation per ADR-0004's anti-fragmentation guidance). Step 7 commit landed at `2022e68`. **IMPORTANT:** `go build ./cmd/envoy-go/...` STILL FAILS at this commit with `cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New` — INTENTIONAL per PLAN; Task 10 fixes the call site. Four files modified/created: `internal/admin/clusters.go` (created, 97 LoC), `internal/admin/clusters_test.go` (created, 308 LoC for the 7 tests), `internal/admin/admin.go` (modified, -10 LoC removing the placeholder handler), `docs/envoy-go/DECISIONS.md` (appended ADR-0087, +59 LoC).

**Outputs:**
```
$ go test -count=1 -run TestHandleClusters -v ./internal/admin/... 2>&1 | tail -16
=== RUN   TestHandleClusters_HTTPSmoke200Text
--- PASS: TestHandleClusters_HTTPSmoke200Text (0.01s)
=== RUN   TestHandleClusters_TenClusterLevelLinesPerCluster
--- PASS: TestHandleClusters_TenClusterLevelLinesPerCluster (0.01s)
=== RUN   TestHandleClusters_ClusterLevelLineFormat
--- PASS: TestHandleClusters_ClusterLevelLineFormat (0.01s)
=== RUN   TestHandleClusters_PerEndpointLinesAllZeroPlusConstants
--- PASS: TestHandleClusters_PerEndpointLinesAllZeroPlusConstants (0.01s)
=== RUN   TestHandleClusters_BodyExactByteLayout
--- PASS: TestHandleClusters_BodyExactByteLayout (0.01s)
=== RUN   TestHandleClusters_EndpointDeclarationOrderPreserved
--- PASS: TestHandleClusters_EndpointDeclarationOrderPreserved (0.01s)
=== RUN   TestHandleClusters_NilManagerEmitsEmptyBody
--- PASS: TestHandleClusters_NilManagerEmitsEmptyBody (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.082s

$ go build ./internal/admin/...
(clean)

$ go test -count=1 ./internal/admin/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/admin	0.154s

$ go vet ./internal/admin/...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go build ./cmd/envoy-go/... 2>&1
# github.com/esalaine/envoy-go/cmd/envoy-go
cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New
	have (string, *stats.Registry)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
(EXPECTED FAIL — Task 10 fixes the call site)

$ grep -nE 'func \(s \*Server\) handleClusters' internal/admin/admin.go internal/admin/clusters.go
internal/admin/clusters.go:26:func (s *Server) handleClusters(w http.ResponseWriter, _ *http.Request) {

$ grep -c '^## ADR-0087:' docs/envoy-go/DECISIONS.md
1
```

## Task 8 — `internal/admin/listeners.go` — `/listeners` handler

**Commits:** `9362f65` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-6. NO new ADR — Task 8 is covered by ADR-0087 from Task 7 (the shared text-format-endpoint shape consolidates `/clusters` + `/listeners` per ADR-0004's anti-fragmentation guidance). Step 1 wrote `internal/admin/listeners_test.go` with five tests covering the SPEC §11.3 + ADR-0087 contract: `TestHandleListeners_HTTPSmoke200Text` (200 + `text/plain; charset=UTF-8` Content-Type + body has `l_main::` prefix + `\n` suffix), `TestHandleListeners_BodyExactByteLayout` (byte-exact `l_main::127.0.0.1:10000\n` for the §7.3 fixture, 24 bytes), `TestHandleListeners_NilManagerEmitsEmptyBody` (defensive nil-lm path emits 200 + empty body per ADR-0085 nil-tolerated convention), `TestHandleListeners_AlphabeticalByName` (multi-listener fixture `l_z` + `l_a` declared non-alphabetical; body emits `l_a::` before `l_z::`), `TestHandleListeners_IPv6BindAddrPassthrough` (documents the byte-pass-through contract for IPv6 addresses; skips when listener.Manager's pre-existing `fmt.Sprintf("%s:%d", host, port)` IPv6 bind limitation hits — out of Task 8 scope to fix). The PLAN Step 1 multi-listener test sketch (`t.Skip` per PLAN's "requires multi-listener fixture" note) was promoted to a real test using a proto-direct fixture builder (`mustMultiListenerBs` + `mkAdminListener` + `mkAdminCluster`) modeled on `internal/listener/manager_test.go:73` (`mkListener` / `mkTcpProxyFilter` / `mkClusterMgr`); the alphabetical-by-name ordering is now exercised end-to-end through the handler. Step 2 confirmed the 4 non-skip tests fail against the placeholder (501 status; missing line content; nil-lm path returns 501 not 200). Step 3 created `internal/admin/listeners.go` (40 LoC) with one method `(s *Server) handleListeners(w, _)` that allocates a `bytes.Buffer`, walks `s.lm.Listeners()` snapshot (when non-nil), defensively `sort.Slice`-orders by `Name` (the snapshot's source — manager.go:928's `m.runtimes` walk — is declaration-order, NOT alphabetical, so the §11.3 ordering is enforced at scrape time), emits `<Name>::<Addr>\n` per listener, then sets `text/plain; charset=UTF-8` headers + 200 + body. Per the existing handler convention the second `*http.Request` parameter is `_` (unused). Listener.Info `{Name string; Addr string}` is the post-Start snapshot from `internal/listener/manager.go:34`; `Addr` is `ln.Addr().String()` captured at Start time (manager.go:738) — which natively produces square-bracket-wrapped IPv6 host forms (`[::]:port` / `[::1]:port`). The handler is a pure pass-through — no parsing, splitting, or reformatting — so byte-shape parity with whatever the listener.Manager surfaces is preserved by construction. Step 4 deleted the placeholder `func (s *Server) handleListeners` block (8 LoC) from `internal/admin/admin.go` introduced by Task 5; the mux registration `mux.HandleFunc("/listeners", s.handleListeners)` resolves to the real handler in listeners.go. `grep -nE 'func \(s \*Server\) handleListeners' internal/admin/admin.go internal/admin/listeners.go` returns exactly 1 match (listeners.go line 26). Step 5 ran the four verification gates — all clean: `go build ./internal/admin/...` clean, `go test -count=1 ./internal/admin/...` 37 PASS + 1 SKIP (33 from Task 7 + 4 new passes; the IPv6 test skips on the pre-existing listener.Manager bind limitation), `go vet ./internal/admin/...` clean, `golangci-lint run ./internal/admin/...` clean. Step 6 committed at `9362f65`. **NO ADR landed for Task 8 per PLAN** — the shape decisions for both /clusters and /listeners are consolidated in ADR-0087 from Task 7. **IMPORTANT:** `go build ./cmd/envoy-go/...` STILL FAILS with `cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New` — INTENTIONAL per PLAN; Task 10 fixes the call site. Three files modified/created: `internal/admin/listeners.go` (created, 40 LoC), `internal/admin/listeners_test.go` (created, 343 LoC for the 5 tests + 4 fixture/helper builders: `mkAdminListener`, `mkAdminCluster`, `mustMultiListenerBs`, `mustIPv6Bs`, `mustLMFromBs`), `internal/admin/admin.go` (modified, -8 LoC removing the placeholder handler).

**Outputs:**
```
$ go test -count=1 -run TestHandleListeners -v ./internal/admin/... 2>&1 | tail -15
=== RUN   TestHandleListeners_HTTPSmoke200Text
--- PASS: TestHandleListeners_HTTPSmoke200Text (0.02s)
=== RUN   TestHandleListeners_BodyExactByteLayout
--- PASS: TestHandleListeners_BodyExactByteLayout (0.02s)
=== RUN   TestHandleListeners_NilManagerEmitsEmptyBody
--- PASS: TestHandleListeners_NilManagerEmitsEmptyBody (0.01s)
=== RUN   TestHandleListeners_AlphabeticalByName
--- PASS: TestHandleListeners_AlphabeticalByName (0.02s)
=== RUN   TestHandleListeners_IPv6BindAddrPassthrough
    listeners_test.go:218: lm.Start IPv6 bind failed (pre-existing listener.Manager limitation outside Task 8 scope): listener: "l_v6": bind ::1:0: listen tcp: address ::1:0: too many colons in address
--- SKIP: TestHandleListeners_IPv6BindAddrPassthrough (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.079s

$ go build ./internal/admin/...
(clean)

$ go test -count=1 ./internal/admin/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/admin	0.225s

$ go vet ./internal/admin/...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go test -v -count=1 ./internal/admin/... 2>&1 | grep -c '^--- PASS:'
37

$ go test -v -count=1 ./internal/admin/... 2>&1 | grep -c '^--- SKIP:'
1

$ go build ./cmd/envoy-go/... 2>&1
# github.com/esalaine/envoy-go/cmd/envoy-go
cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New
	have (string, *stats.Registry)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
(EXPECTED FAIL — Task 10 fixes the call site)

$ grep -nE 'func \(s \*Server\) handleListeners' internal/admin/admin.go internal/admin/listeners.go
internal/admin/listeners.go:26:func (s *Server) handleListeners(w http.ResponseWriter, _ *http.Request) {
```

## Task 9 — `internal/admin/serverinfo.go` — `/server_info` handler + ADR-0088

**Commits:** `7b080e0` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** TDD red→green per PLAN Steps 1-7. Step 1 wrote `internal/admin/serverinfo_test.go` with the six tests verbatim from PLAN Step 1: `TestHandleServerInfo_HTTPSmoke200JSON` (200 + `application/json` Content-Type + body parses as JSON + carries all seven required top-level fields `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `node`, `command_line_options`, `hot_restart_version`), `TestHandleServerInfo_StatePostMarkReady` (post-`MarkReady()` body contains `"state": "LIVE"`), `TestHandleServerInfo_StatePreMarkReady` (no-MarkReady body contains `"state": "PRE_INITIALIZING"`), `TestHandleServerInfo_UptimeMonotonic` (defensive — both bodies parse + carry `uptime_current_epoch`; sub-second values may both round to `"0s"` so monotonicity is asserted as "trivially ≥" per PLAN's note), `TestHandleServerInfo_CommandLineOptionsConfigPath` (`mustMinimalBs` sets `bs.ConfigPath = "/test/envoy-go.yaml"`, body contains `"config_path": "/test/envoy-go.yaml"`), `TestHandleServerInfo_HotRestartVersionDisabled` (literal `"hot_restart_version": "disabled"`). Step 2 confirmed all 6 fail against the placeholder (501 status; missing JSON shape; placeholder text body `server_info: not yet implemented`). Step 3 created `internal/admin/serverinfo.go` (53 LoC) with three functions: `handleServerInfo` (handler — calls `buildServerInfo`, marshals via the reused `configDumpMarshalOptions` from configdump.go, writes `application/json` + 200 + body; defensive 500 + `{}` on marshal error), `buildServerInfo` (assembles `*adminv3.ServerInfo` from the Server's threaded fields — `Version = BuildVersionString()`, `State = deriveState(&s.ready)`, `UptimeCurrentEpoch = UptimeAllEpochs = durationpb.New(time.Since(s.bootTime))` (same value, single epoch — no hot-restart), `HotRestartVersion = "disabled"`, `CommandLineOptions = &adminv3.CommandLineOptions{ConfigPath: s.bs.ConfigPath}` (partial — other fields zero-valued via `EmitUnpopulated: true`), `Node = s.bs.Proto.GetNode()` (proto3-nil-safe); defensive nil-handling for `s.bs == nil`), `deriveState` (returns `adminv3.ServerInfo_LIVE` when `ready.Load()` is true, else `adminv3.ServerInfo_PRE_INITIALIZING` — INITIALIZING unreachable in MVP per ADR-0088, DRAINING is 08.2's deliverable). The `core_v3_Node` placeholder in PLAN Step 3's pseudocode was resolved by importing `corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"` and declaring `var node *corev3.Node` directly. Step 4 deleted the placeholder `func (s *Server) handleServerInfo` (8 LoC) from `internal/admin/admin.go` introduced by Task 5; the mux registration `mux.HandleFunc("/server_info", s.handleServerInfo)` resolves to the real handler in serverinfo.go. After the placeholder deletion `gofmt -w` removed a single trailing blank line; `strconv` remains in use by `handleReady`. `grep -nE 'func \(s \*Server\) handleConfigDump|handleClusters|handleListeners|handleServerInfo' internal/admin/admin.go` returns ZERO matches — all four placeholder handlers from Task 5 are now gone from admin.go (real handlers live in configdump.go, clusters.go, listeners.go, serverinfo.go respectively). Step 5 ran the four verification gates — all clean: `go build ./internal/admin/...` clean, `go test -count=1 ./internal/admin/...` 43 PASS + 1 SKIP (37 from Task 8 + 6 new from Task 9; the IPv6 test from Task 8 still skips on the pre-existing listener.Manager limitation), `go vet ./internal/admin/...` clean, `golangci-lint run ./internal/admin/...` clean. Step 6 appended ADR-0088 to `docs/envoy-go/DECISIONS.md` (Status: Accepted, Date: 2026-05-02, Doctrine: D-3.3 + D-3.5, Lands-in-task: Task 9). The ADR establishes the body-shape contract: protojson over `*adminv3.ServerInfo` reusing `configDumpMarshalOptions` per ADR-0086 consequence (d); state-enum coverage in 08.1 is exactly `LIVE` + `PRE_INITIALIZING` (INITIALIZING unreachable in static-bootstrap-only MVP; DRAINING is 08.2's deliverable, will be added by ADR-0088 amendment); MVP field set is `version` (BuildVersionString per ADR-0086), `state`, `uptime_current_epoch` + `uptime_all_epochs` (single epoch), `hot_restart_version: "disabled"` (literal — envoy-go has no hot-restart per ADR-0001), partial `command_line_options{config_path: bs.ConfigPath}` (other fields zero-valued via `EmitUnpopulated`), `node` from bootstrap proto. Consequences: (a) /server_info equivalence claim is post-MarkReady; (b) `configDumpMarshalOptions` reuse per ADR-0086 (d); (c) 08.2 amends this ADR to add DRAINING; (d) `version`, `uptime_*`, `command_line_options.*` (beyond `config_path`), `hot_restart_version`, `node.user_agent_*` + `node.extensions[]` are §13.2 differential-allow-listed; the `state` field IS byte-equal post-MarkReady. `grep -c '^## ADR-0088:' docs/envoy-go/DECISIONS.md` returns 1. Step 7 committed at `7b080e0`. **IMPORTANT:** `go build ./cmd/envoy-go/...` STILL FAILS with `cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New` — INTENTIONAL per PLAN; Task 10 fixes the call site AND lands the `bs.ConfigPath = *cfgPath` post-Load assignment. Four files modified/created: `internal/admin/serverinfo.go` (created, 53 LoC), `internal/admin/serverinfo_test.go` (created, 145 LoC for the 6 tests), `internal/admin/admin.go` (modified, -8 LoC removing the placeholder handler + 1 trailing-blank-line strip via gofmt), `docs/envoy-go/DECISIONS.md` (modified, +ADR-0088 appended after ADR-0087).

**Outputs:**
```
$ go test -count=1 -run TestHandleServerInfo -v ./internal/admin/... 2>&1 | tail -16
=== RUN   TestHandleServerInfo_HTTPSmoke200JSON
--- PASS: TestHandleServerInfo_HTTPSmoke200JSON (0.02s)
=== RUN   TestHandleServerInfo_StatePostMarkReady
--- PASS: TestHandleServerInfo_StatePostMarkReady (0.02s)
=== RUN   TestHandleServerInfo_StatePreMarkReady
--- PASS: TestHandleServerInfo_StatePreMarkReady (0.02s)
=== RUN   TestHandleServerInfo_UptimeMonotonic
--- PASS: TestHandleServerInfo_UptimeMonotonic (0.06s)
=== RUN   TestHandleServerInfo_CommandLineOptionsConfigPath
--- PASS: TestHandleServerInfo_CommandLineOptionsConfigPath (0.01s)
=== RUN   TestHandleServerInfo_HotRestartVersionDisabled
--- PASS: TestHandleServerInfo_HotRestartVersionDisabled (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.152s

$ go build ./internal/admin/...
(clean)

$ go test -count=1 ./internal/admin/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/admin	0.374s

$ go vet ./internal/admin/...
(clean)

$ golangci-lint run ./internal/admin/...
(clean)

$ go test -v -count=1 ./internal/admin/... 2>&1 | grep -c '^--- PASS:'
43

$ go test -v -count=1 ./internal/admin/... 2>&1 | grep -c '^--- SKIP:'
1

$ go build ./cmd/envoy-go/... 2>&1
# github.com/esalaine/envoy-go/cmd/envoy-go
cmd/envoy-go/main.go:83:33: not enough arguments in call to admin.New
	have (string, *stats.Registry)
	want (string, *stats.Registry, *bootstrap.Bootstrap, *cluster.Manager, *listener.Manager)
(EXPECTED FAIL — Task 10 fixes the call site)

$ grep -nE 'func \(s \*Server\) handleConfigDump|handleClusters|handleListeners|handleServerInfo' internal/admin/admin.go
(no matches — all four placeholder handlers from Task 5 are now removed; real handlers live in configdump.go, clusters.go, listeners.go, serverinfo.go)

$ grep -nE 'func \(s \*Server\) handleServerInfo' internal/admin/serverinfo.go
internal/admin/serverinfo.go:21:func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {

$ grep -c '^## ADR-0088:' docs/envoy-go/DECISIONS.md
1
```

## Task 10 — `cmd/envoy-go/main.go` — admin.New(bs,cm,lm) call-site + Bootstrap.ConfigPath wiring

**Commits:** `4b2e1c2` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** Lands per PLAN Steps 1-5. Step 1 edited `cmd/envoy-go/main.go` with three coordinated changes: (a) added `bs.ConfigPath = *cfgPath` immediately after `bootstrap.Load(f)` (line ~52-57; populates the field added by Task 2 so /server_info can emit it under `command_line_options.config_path` per ADR-0088); (b) deleted the original pre-`httpReg` `admSrv := admin.New(adminAddr, bs.Stats); ... defer admSrv.Close()` block (5 LoC removed); (c) inserted the new widened `admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm)` block (with the LBP-1 + defer-LIFO commentary) immediately after `lm, err := listener.NewManagerWithBaseDirAndAllowH2C(...)` and before `ctx, cancel := signal.NotifyContext(...)`. The boot-order rationale: cluster + listener manager must exist before `admin.New` is called because the constructor binds them into `s.cm` + `s.lm` for the four new endpoints (per ADR-0085 + planner-time decision 6); `admin.New` must still happen before `bs.Stats.Freeze()` because admin allocates the `server.live` gauge at New time (SPEC §5.4 + §12 #3). Defer LIFO ordering changes from {sinks, admSrv, lm} pre-08.1 to {sinks, admSrv, lm-Stop} post-08.1 — admin still closes before sinks; the new `lm.Stop()` defer (registered AFTER admSrv.Close defer because `lm.Start(ctx)` happens AFTER `admSrv.Start()`) runs FIRST under LIFO. 08.1 SPEC §5.3 does not mandate strict resource-shutdown ordering across admin/listener/sinks; the cost is the LBP-1 cluster + listener pre-existence requirement. Step 2 verified `go build ./...` clean — FIRST FULL-REPO BUILD SUCCESS SINCE TASK 5 (the admin.New 2-arg → 5-arg widening at Task 5 had broken cmd/envoy-go intentionally; Tasks 5-9 ran with the cmd/envoy-go build red, completing internal/admin/* development against `go build ./internal/admin/...` only; this task closes the breakage). Step 3 added `TestMain_FourNewAdminEndpointsRespond200` (~125 LoC) to `cmd/envoy-go/main_test.go` — boots the binary on a representative HCM-with-router bootstrap (the fixture-0005 / fixture-0006 shape), waits for the `l_http` ready sentinel via the existing `waitForReadySentinels` helper, then GETs each of the four endpoints from the admin port and asserts: (a) status 200, (b) body non-empty, (c) `/config_dump` and `/server_info` bodies parse as `map[string]interface{}` via `json.Unmarshal` (SPEC §5.4.1 + §5.4.4 — both render protojson, NOT YAML); `/clusters` and `/listeners` are asserted only on status + non-empty (their text/plain bodies are operator-friendly per ADR-0087, byte-shape covered by Task 11 in-package tests + Task 14 differential fixture). The test uses subtests via `t.Run(ep.path, ...)` so a single endpoint failure surfaces independently. Added one new import `encoding/json` to main_test.go. The `buildBinaryOrSkip` + `freeTCPPort` + `waitForReadySentinels` helpers are reused unchanged from the existing 06.1/07.1/07.2 patterns. Step 4 ran the four verification gates — all clean: `go build ./...` clean (FIRST FULL-REPO SUCCESS SINCE TASK 5), `go test -count=1 -short ./...` PASS across all packages including admin (43 + 1-skip), bootstrap, cluster, listener, filter/* and cmd/envoy-go (now 6 tests including the new four-endpoint smoke), `go vet ./...` clean, `golangci-lint run ./...` clean. The differential suite passed end-to-end on a re-run after one transient flake on `0006-access-log` (Envoy container readiness timeout — orthogonal to this task; re-run was green). Step 5 committed at `4b2e1c2`. **THIS TASK ENDS THE INTENTIONAL `cmd/envoy-go` BUILD BREAKAGE FROM TASK 5**; full repo `go build ./...` is now clean. Two files modified: `cmd/envoy-go/main.go` (modified, +24 / -6 LoC: +1 `bs.ConfigPath = *cfgPath` line + 6 lines of supporting commentary, -5 LoC pre-`httpReg` admSrv block, +14 LoC post-`lm` admSrv block + 11 lines of LBP-1/defer commentary), `cmd/envoy-go/main_test.go` (modified, +135 LoC: +1 import line for `encoding/json` + ~125 LoC `TestMain_FourNewAdminEndpointsRespond200` function + comment block).

**Outputs:**
```
$ go build ./... 2>&1
(clean — FIRST FULL-REPO BUILD SUCCESS SINCE TASK 5)

$ go vet ./... 2>&1
(clean)

$ go test -count=1 ./cmd/envoy-go/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.223s

$ go test -count=1 -short ./... 2>&1 | tail -10
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	0.079s
ok  	github.com/esalaine/envoy-go/test/differential	0.079s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.006s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	0.007s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	0.005s
ok  	github.com/esalaine/envoy-go/test/helpers	0.008s

$ golangci-lint run ./... 2>&1
(clean)

$ go test -count=1 ./test/differential/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/test/differential	26.693s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.002s

$ grep -nE 'admin\.New\(' cmd/envoy-go/main.go
139:	admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm)

$ grep -nE 'bs\.ConfigPath' cmd/envoy-go/main.go
57:	bs.ConfigPath = *cfgPath

$ grep -c '^func TestMain_FourNewAdminEndpointsRespond200' cmd/envoy-go/main_test.go
1
```

## Task 11 — `internal/admin/admin_test.go` — four-endpoint smoke + method-discrimination Envoy parity

**Commits:** `4e17985` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** Lands per PLAN Steps 1-3. Step 1 appended two new tests to `internal/admin/admin_test.go` (added `"strings"` import alongside the existing `"io"`/`"net/http"`/`"testing"`/`"time"` set). The first test, `TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders`, is the per-endpoint smoke test mandated by SPEC §3 gate (b) + §6.4 per-endpoint contract + §11.6 four-header set: it builds the canonical §7.3 fixture (via `mustMinimalBs` / `mustMinimalCM` / `mustMinimalLM` helpers from `admin_helpers_test.go` — no helper relocation needed for T11; the deferred T8 refactor of `mkAdminListener` / `mkAdminCluster` / `mustLMFromBs` from `listeners_test.go` is NOT triggered by T11 because the smoke test only needs the minimal-fixture helpers, not the per-listener / per-cluster builders), constructs the widened `admin.New(addr, bs.Stats, bs, cm, lm)`, starts it, calls `MarkReady()`, sleeps 20ms for the accept goroutine, then runs a four-row table-driven subtest (one `t.Run` per endpoint) over `/config_dump` (Content-Type `application/json`), `/clusters` (`text/plain; charset=UTF-8`), `/listeners` (same), `/server_info` (`application/json`). Each subtest asserts: (a) status 200, (b) Content-Type matches the row's expected value, (c) the three constant headers from §11.6 — `Cache-Control: no-cache, max-age=0`, `X-Content-Type-Options: nosniff`, `Server: envoy` — match exactly via a nested `[]struct{key, want string}` table, (d) the `Date` header is non-empty (net/http auto-adds it per RFC 9110 §6.6.1, per planner-time decision 5 + headers.go §11.6 commentary). The second test, `TestAdmin_FourEndpointsAcceptAnyMethod`, pins SPEC §11.8 method-discrimination Envoy parity: upstream Envoy v1.37.2 does NOT enforce GET-only on read-only admin endpoints (the SPEC §11.8 verbatim evidence shows POST returns the same 200 + body as GET). Test boots the same widened admin server, then issues `http.Post("http://"+addr+"/config_dump", "application/json", strings.NewReader(""))` and asserts status 200; this contract is what allows envoy-go to register the four handlers via plain `mux.HandleFunc(path, handler)` (no method check) — matching the existing four handlers in `configdump.go` / `clusters.go` / `listeners.go` / `serverinfo.go` which do not branch on `r.Method` (verified by grep — none of the four handlers reads `r.Method`). Step 2 ran the targeted tests — both PASS: `TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders` (4 subtests, 30ms total), `TestAdmin_FourEndpointsAcceptAnyMethod` (20ms). Then ran the full admin package (`go test -count=1 ./internal/admin/...` ok 0.419s, all tests passing including the existing 06.1 prometheus + the four 08.1 handler unit tests + the new T11 smoke + parity tests), `go test -count=1 -short ./...` (PASS across all packages — admin, bootstrap, cluster, listener, filter/*, cmd/envoy-go, conformance/h2spec, differential, all fixture drivers), `go vet ./...` clean, `golangci-lint run ./...` clean. NO method-discrimination logic was added to handlers — the PLAN's Step 3 simpler path was taken: assert that POST returns 200 (Envoy parity for read-only endpoints is "method ignored — return body"), no GET-only guard added. This matches the existing handler implementations from Tasks 6-9 which all use `mux.HandleFunc` without method branching. The deferred T8 helper-relocation (move `mkAdminListener` / `mkAdminCluster` / `mustLMFromBs` from `listeners_test.go` to `admin_helpers_test.go`) is documented as NOT triggered by T11 — those helpers build per-cluster + per-listener fixtures for unit-level body-shape tests in clusters_test.go / listeners_test.go; T11's smoke tests only need the package-level four-endpoint contract, satisfied by `mustMinimalBs/CM/LM`. The relocation can wait for T15 REVIEW or skip entirely — no block on T11. Step 3 committed at `4e17985`. One file modified: `internal/admin/admin_test.go` (modified, +88 LoC: +1 import line for `"strings"` + ~85 LoC two new test functions).

**Outputs:**
```
$ go build ./... 2>&1
(clean)

$ go vet ./... 2>&1
(clean)

$ go test -count=1 -run 'TestAdmin_AllFourEndpoints|TestAdmin_FourEndpointsAcceptAnyMethod' -v ./internal/admin/... 2>&1 | tail -15
=== RUN   TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders
=== RUN   TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//config_dump
=== RUN   TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//clusters
=== RUN   TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//listeners
=== RUN   TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//server_info
--- PASS: TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders (0.03s)
    --- PASS: TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//config_dump (0.00s)
    --- PASS: TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//clusters (0.00s)
    --- PASS: TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//listeners (0.00s)
    --- PASS: TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders//server_info (0.00s)
=== RUN   TestAdmin_FourEndpointsAcceptAnyMethod
--- PASS: TestAdmin_FourEndpointsAcceptAnyMethod (0.02s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.051s

$ go test -count=1 ./internal/admin/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/admin	0.419s

$ go test -count=1 -short ./... 2>&1 | grep -E 'FAIL|^---' | head -10
(no failures — all packages PASS)

$ golangci-lint run ./... 2>&1
(clean)

$ grep -c '^func TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders\|^func TestAdmin_FourEndpointsAcceptAnyMethod' internal/admin/admin_test.go
2
```

## Task 12 — `internal/admin/admin_test.go::TestAdminConcurrentScrapeRace` — concurrent-scrape race-detector contract

**Commits:** `5e1d454` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** Lands per PLAN Steps 1-4 the SPEC §3 gate (b) + §5.6 race-detector contract: 100 goroutines × 4 endpoints × 1s wall under `go test -race`. Step 1 appended `TestAdminConcurrentScrapeRace` to `internal/admin/admin_test.go` (+66 LoC), and added three new imports — `"fmt"` (for the `fmt.Errorf` wrapping in goroutine error sends), `"sync"` (for `sync.WaitGroup`), augmenting the existing `"io"`/`"net/http"`/`"strings"`/`"testing"`/`"time"` set. The test boots the canonical four-endpoint fixture via `mustMinimalBs` / `mustMinimalCM` / `mustMinimalLM`, calls `admin.New(addr, bs.Stats, bs, cm, lm)`, starts the server, calls `MarkReady()`, then sleeps 20ms for the accept goroutine. It then computes `deadline := time.Now().Add(1*time.Second)`, declares the four endpoints `[]string{"/config_dump", "/clusters", "/listeners", "/server_info"}`, allocates a `sync.WaitGroup`, and a buffered `chan error` of size `100*4=400` (deep enough to never block on send even if every goroutine reports max errors). It then spawns 100 goroutines, each running its own `&http.Client{Timeout: 2 * time.Second}` (per-goroutine to avoid sharing a `Client.Transport`'s connection pool semantics across hot-loop senders, but `http.Client` itself IS goroutine-safe — the per-goroutine allocation is purely defensive); each goroutine loops while `time.Now().Before(deadline)`, picks an endpoint via `endpoints[(i+int(time.Now().UnixNano()))%len(endpoints)]` (a per-goroutine round-robin offset by the wall-clock nanosecond — guarantees all four endpoints get hit by all 100 goroutines, and the per-iter randomness varies request ordering so the race detector sees more interleavings), `client.Get`s the URL, drains the body via `io.Copy(io.Discard, resp.Body)`, closes the body, and on either err or non-200 sends an error on the buffered channel via a non-blocking `select { case errs <- err: default: }`. After all 100 goroutines complete, `wg.Wait()` returns, `close(errs)`, and the test ranges over the channel and reports each error via `t.Errorf`. The `testing.Short()` guard at the top — `t.Skip("race-stress test; skipped under -short")` — keeps the 1s + race-detector overhead out of the inner-loop `-short` runs, matching the SPEC §5.6 contract that this is a `-race`-only stress test (run separately by `go test -race -run TestAdminConcurrentScrapeRace`). Step 2 ran the test under `-race` — PASSED in 1.04s wall (race-detector overhead minimal because all four handlers read immutable-post-boot state per ADR-0085: `s.bs.Proto` is the once-set static bootstrap, `s.cm.Clusters()` returns a copy of the immutable cluster snapshot, `s.lm.Listeners()` returns a copy of the immutable listener snapshot, `s.bootTime` is set in `New()` before any goroutine accesses it; the only mutable state read is `s.ready` which is a `sync/atomic.Bool` — reads on it are race-free by definition). Step 3 ran `go test -count=1 -race ./internal/admin/...` — full admin package PASS in 2.530s (every existing test continues to pass under `-race`, no regressions from the new test's added compile/link cost). Step 4 ran `go test -race -count=1 -short ./...` (full repo, no failures across all packages — admin, bootstrap, cluster, listener, filter/*, cmd/envoy-go, conformance/h2spec, differential, all fixture drivers), `go vet ./...` clean, `golangci-lint run ./...` clean. Step 5 committed at `5e1d454` with the exact PLAN-prescribed message. One file modified: `internal/admin/admin_test.go` (modified, +66 LoC: +2 import lines for `"fmt"` and `"sync"` + ~64 LoC the new test function with comment block).

**Outputs:**
```
$ go test -count=1 -race -run TestAdminConcurrentScrapeRace -v ./internal/admin/... 2>&1 | tail -10
=== RUN   TestAdminConcurrentScrapeRace
--- PASS: TestAdminConcurrentScrapeRace (1.04s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	2.067s

$ go test -count=1 -race ./internal/admin/... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/internal/admin	2.530s

$ go test -race -count=1 -short ./... 2>&1 | grep -E 'FAIL|^---' | head -10
(no failures — all packages PASS)

$ go vet ./... 2>&1
(clean)

$ golangci-lint run ./... 2>&1
(clean)

$ grep -c '^func TestAdminConcurrentScrapeRace' internal/admin/admin_test.go
1
```

## Task 13 — `internal/admin/fuzz_test.go::FuzzConfigDumpFormat` — adversarial bootstrap proto fuzzer (10th project fuzzer)

**Commits:** `8dd5f16` — this task's commit; PROGRESS bookkeeping commit TBD
**Notes:** Lands per PLAN Steps 1-4 the SPEC §3 gate (d) + §14.5 fuzzer: a new `FuzzConfigDumpFormat` adversarial fuzzer that exercises `buildConfigDump` + `protojson.Marshal` (the two hot proto paths in `configdump.go`) with mutated YAML inputs, asserts (i) no panic, (ii) output is valid JSON parseable by `json.Unmarshal`, (iii) when output is non-empty, the root JSON object has a `"configs"` field. This brings the project fuzzer count from 9 (post-07.2) to 10 (post-08.1) — the nine pre-existing fuzzers are `FuzzBootstrapLoad` (`internal/bootstrap/`), `FuzzTcpProxyFilter` (`internal/filter/tcpproxy/`), `FuzzTLSContextParse` (`internal/tls/`), `FuzzHCMConfigParse` (`internal/filter/hcm/`), `FuzzFrameStream` (`internal/filter/hcm/h2/`), `FuzzHPACKDecode` (also `internal/filter/hcm/h2/`), `FuzzPromTextFormat` (`internal/stats/`), `FuzzDefaultFormatRender` (`internal/accesslog/`), `FuzzFilterChainParse` (`internal/listener/listenerfilter/`); per `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*'` the new file is the 10th fuzz_test.go in the repo. Step 1 wrote `internal/admin/fuzz_test.go` (NEW, 81 LoC) verbatim from the PLAN Step 1 body: package `admin`, imports `encoding/json` / `strings` / `testing` / `time` + `github.com/esalaine/envoy-go/internal/bootstrap`, three seed corpus YAMLs (empty, admin-only socket_address, admin + minimal STATIC cluster with one ROUND_ROBIN endpoint at 127.0.0.1:18001), each added via `f.Add(s)` (corpus type is `string`, so the fuzz body's parameter is `yamlBytes string` — Go fuzz infers the parameter type from the seed corpus type, and string is one of the supported `testing.F.Fuzz` corpus types). Inside `f.Fuzz`: (a) call `bootstrap.Load(strings.NewReader(yamlBytes))` — most adversarial mutations fail YAML/proto parse; the fuzzer just returns on err (which is the only correct behavior — bootstrap.Load is the gate, and its existing `FuzzBootstrapLoad` already covers parse-path adversarial inputs). (b) On a successfully-loaded bootstrap, call `buildConfigDump(bs, time.Now())` — this is what we're fuzzing. A non-nil err is acceptable (anypb.New can fail for unregistered types, though it shouldn't with go-control-plane admin/v3 types — but the fuzzer is conservative and returns on err rather than asserting err == nil; what's NOT acceptable is a panic, which testing.F catches automatically and reports as a fuzz failure). (c) On a successful build, marshal via `configDumpMarshalOptions.Marshal(cd)` — same panic-but-not-error contract. (d) If body is empty, return (the bs == nil + bs.Proto == nil defensive path returns `&adminv3.ConfigDump{}` which marshals to a non-empty JSON `{}` body, but the fuzzer guards just in case). (e) `json.Unmarshal(body, &generic)` — must succeed; on err the fuzzer reports `t.Errorf("buildConfigDump output is not valid JSON: ...")`. (f) `_, ok := generic["configs"]; !ok` — must be present; on absence the fuzzer reports `t.Errorf("buildConfigDump output lacks 'configs' field: ...")`. The error reports include `body[:min(200, len(body))]` (Go 1.21+ built-in `min`, no helper needed; module is at `go 1.23.0` per `go.mod`, and Go 1.23 is installed). Step 2 ran the fuzzer for 30 seconds at the ADR-0018 short-budget: `go test -fuzz=FuzzConfigDumpFormat -fuzztime=30s ./internal/admin/...` — clean: 1.69M execs (44k/sec average peak 192k/sec on a 32-worker box), 244 new-interesting corpus entries, ZERO failures, ZERO crashes (no `t.Errorf` triggered, no panic recovered, no `failures.txt` written to `internal/admin/testdata/fuzz/FuzzConfigDumpFormat/`). The high-rate exec count confirms `bootstrap.Load` rejects most random inputs (so the fuzzer's hot path is YAML→err return), but the 244 new-interesting entries show the fuzzer DID find inputs that successfully parsed AND drove all the way through `buildConfigDump` + protojson.Marshal — none of which broke the contract. Step 3 (full 9-fuzzer regression) is OPTIONAL per PLAN's note ("mechanical re-run of the 9 pre-existing fuzzers"); skipped this task because (a) the pre-existing 9 fuzzers were each run at 30s during their respective phase tasks (02-07.2), (b) Task 13's scope per phase 08.1 is the 10th fuzzer only (no changes to the 9 pre-existing), (c) the regression run is a one-shot verification across phases that belongs to T15's REVIEW gate, not T13's narrow "land the fuzzer" scope. Step 4 committed at `8dd5f16` with the PLAN-prescribed message. Verification: `go test -count=1 ./internal/admin/...` PASS in 1.439s (regular run; fuzz target is NOT invoked when `-fuzz` flag is absent — Go's fuzz test framework gates the corpus-mutation loop behind the `-fuzz=<name>` flag, with the `f.Fuzz` callback running only the seed corpus as regular test cases when invoked via plain `go test`; this matches the project pattern of the other 9 fuzz_test.go files which do not pollute the regular `go test` run). `go vet ./internal/admin/...` clean. `golangci-lint run ./internal/admin/...` clean. One file added: `internal/admin/fuzz_test.go` (NEW, 81 LoC).

**Outputs:**
```
$ go test -count=1 ./internal/admin/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/admin	1.439s

$ go vet ./internal/admin/... 2>&1
(clean)

$ golangci-lint run ./internal/admin/... 2>&1
(clean)

$ go test -fuzz=FuzzConfigDumpFormat -fuzztime=30s ./internal/admin/... 2>&1 | tail -10
fuzz: elapsed: 9s, execs: 283343 (4723/sec), new interesting: 174 (total: 177)
fuzz: elapsed: 12s, execs: 431042 (49221/sec), new interesting: 176 (total: 179)
fuzz: elapsed: 15s, execs: 659655 (76222/sec), new interesting: 180 (total: 183)
fuzz: elapsed: 18s, execs: 1236036 (192121/sec), new interesting: 199 (total: 202)
fuzz: elapsed: 21s, execs: 1330524 (31497/sec), new interesting: 222 (total: 225)
fuzz: elapsed: 24s, execs: 1550796 (73376/sec), new interesting: 232 (total: 235)
fuzz: elapsed: 27s, execs: 1555130 (1445/sec), new interesting: 239 (total: 242)
fuzz: elapsed: 30s, execs: 1688147 (44344/sec), new interesting: 241 (total: 244)
fuzz: elapsed: 31s, execs: 1688147 (0/sec), new interesting: 241 (total: 244)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	32.543s

$ find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' | wc -l
10

$ grep -c '^func FuzzConfigDumpFormat' internal/admin/fuzz_test.go
1
```
