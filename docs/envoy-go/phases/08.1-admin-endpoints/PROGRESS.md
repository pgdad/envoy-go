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
