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
