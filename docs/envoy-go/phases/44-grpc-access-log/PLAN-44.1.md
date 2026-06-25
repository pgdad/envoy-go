# Phase 44.1 Implementation Plan — the core streaming gRPC Access Log Service (ALS) sink: config parse + the `ALSClient` typed wrapper + the `GrpcAccessLogSink` (10-field `HTTPAccessLogEntry` mapping) + the `access_logs.grpc_access_log.*` stats + the `0081` receiver differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** When an HCM `access_log[]` entry carries an `envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig` typed_config, envoy-go dials the configured `grpc_service.envoy_grpc.cluster_name` cluster, opens the `AccessLogService.StreamAccessLogs` client-streaming RPC, sends `identifier { node, log_name }` once per stream, then streams each access-log event as a structured `HTTPAccessLogEntry` proto built from the 10 plumbed `accesslog.Record` operators — proven cross-side EXACT (on the 7 deterministic structured fields) by the `0081-grpc-access-log` differential against `contrib-v1.37.2`.

**Architecture:** This is the project's SECOND concrete `accesslog.Sink` (alongside the phase-06.2 `AsyncFileSink`) and its SECOND `grpcclient` consumer (after ext_authz, ADR-0158). It composes on TWO already-built substrates — the phase-06.2 `Sink`/`Record` subsystem and the phase-18 `grpcclient.Dialer` — adding ZERO new Go packages in `internal/` and ZERO new go.mod modules (the receiver test-helper `test/helpers/accessloggrpc` is a NEW test-only package, the `test/helpers/extauthzgrpc` precedent). Byte-identical when no ALS `access_log` entry is configured (the file-only path is untouched; the full differential is the regression anchor). The sink mirrors `AsyncFileSink`'s bounded-channel + writer-goroutine + idempotent-`Close` shape; the `ALSClient` mirrors `AuthClient`; the parse arm extends `parseOneAccessLog`. ANCHORS ADR-0255; OPENS the Observability family (which STAYS OPEN).

**Tech Stack:** Go; the in-tree `internal/accesslog` sink subsystem; `internal/grpcclient` (the `Dialer` + the `AuthClient` typed-wrapper pattern); `internal/bootstrap` (the `access_log` parse + the ADR-0016 proto blank-import registry); the resolved `go-control-plane/envoy v1.32.4` ALS service/config/data protos; the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The access-log subsystem (`internal/accesslog`) already has ONE sink type — `AsyncFileSink`, which writes Envoy-default-format TEXT lines to a file from a bounded channel drained by a writer goroutine. You are adding a SECOND sink type — `GrpcAccessLogSink` — that streams STRUCTURED protos to a gRPC service instead. The gRPC plumbing (`internal/grpcclient`) already exists: a `Dialer` turns a cluster name into a `*grpc.ClientConn` (gated on cluster-exists + the cluster being HTTP/2), and `AuthClient` is the typed-stub-wrapper precedent you will copy for `ALSClient`. The bootstrap parser (`internal/bootstrap`) already walks each HCM's `access_log[]` and currently silently ignores everything that is not a `FileAccessLog`; you will teach it to recognise the gRPC-ALS typed_config and parse it into an `ALSConfig`.

The differential test harness boots BOTH the real reference Envoy (in a Docker container, `contrib-v1.37.2`) AND the in-process subject (envoy-go) against equivalent bootstraps, drives the same traffic at both, and asserts equivalence. For this fixture, BOTH sides stream their access-log entries to the SAME in-process gRPC receiver (started by the test driver); the driver then asserts the deterministic structured-field subset of every received entry matches cross-side. The reference (in Docker) reaches the host-bound receiver via `host.docker.internal`; the subject reaches it via `127.0.0.1`.

### Key source seams (verified at PLAN time against the tree at master `1b276cd8`; re-confirm line numbers before editing — files evolve)

- **`internal/accesslog/accesslog.go`** — `Sink interface { Submit(r any); Close() error }` (`:18`); `Record struct` (`:29`) with the 10 fields `StartTime, Method, Path, Protocol string, ResponseCode int, BytesSent int64, Duration, Authority, UserAgent, UpstreamHost`.
- **`internal/accesslog/writer.go`** — `const defaultChannelCapacity = 4096` (`:13`); `const dropLogIntervalNanos` (`:14`); `AsyncFileSink` (`:26`) — the bounded-channel `ch chan any` + `done chan struct{}` + `closeOnce sync.Once` + `lastDropLog atomic.Int64` shape; `Submit` drop-newest (`:73`); `Close` sync.Once (`:88`); `run` writer goroutine (`:101`). COPY this shape for the sink.
- **`internal/accesslog/stats.go`** — `RegisterDroppedCounter(reg) *stats.Counter` (`:14`) → `reg.NewCounter("server.accesslog_dropped")`. COPY this shape for the two new counters (a DISTINCT registration, NOT a reuse — AMEND-ALS-1).
- **`internal/filter/hcm/accesslog_emit.go`** — `emitAccessLog` H1 (`:18`) + `emitAccessLogH2` (`:43`) build a `*accesslog.Record` and `Submit` it to every sink in `f.accessLog` (`:34`/`:59`). **`Record.Path = r.URL.Path`** (`:25`, path-only — the AMEND-ALS-2 query-less constraint). NO CHANGE to this file in 44.1.
- **`internal/grpcclient/grpcclient.go`** — `Dialer` (`:78`); `New(mgr) *Dialer` (`:85`); `(*Dialer).DialContext(ctx, clusterName) (*grpc.ClientConn, error)` (`:105`) with the cluster-exists (`:113`) + `UseH2()` (`:116`) PARSE-REJECT gates; `AuthClient` (`:157`); `NewAuthClient(d, clusterName, timeout)` (`:178`) — dials via `d.DialContext(context.Background(), …)` then wraps the stub; `Close()` sync.Once-guarded (`:231`). COPY the `AuthClient`/`NewAuthClient`/`Close` shape for `ALSClient`.
- **`internal/bootstrap/bootstrap.go`** — the access-logger blank-imports (`:39`/`:40` — `file/v3` + `stream/v3`); `fileAccessLogTypeURL` const (`:145`); `AccessLogConfig struct { Path string }` (`:152`); `Bootstrap.AccessLogConfigs []AccessLogConfig` (`:177`); `parseAccessLogConfigs` (`:234`); `parseOneAccessLog(al, idx, result)` (`:265`) — the file-TypeURL branch (`:271`) + the silent-ignore. `bs.Proto.GetNode()` is available (`internal/listener/manager.go:241` already reads `bs.GetNode().GetCluster()`).
- **`cmd/envoy-go/main.go`** — `cm` cluster manager in scope (`:97`); the sink-build loop (`:106`–`:119`) — `droppedCounter := accesslog.RegisterDroppedCounter(bs.Stats)`, `sinks := make([]accesslog.Sink, 0, …)`, the per-`AccessLogConfig` `NewAsyncFileSink` loop, the defer-LIFO `Close()` (`:115`); `sinks` threads UNCHANGED into `builtins.RegisterBuiltins(… AccessLogSinks: sinks …)` (`:225`) + the listener manager (`:232`).
- **`test/helpers/extauthzgrpc/extauthzgrpc.go`** — the in-process gRPC-server test-helper precedent: `New(t)` (ephemeral port, `t.Cleanup`), `NewAtAddr(addr)` (caller-chosen port), `newServer` (`net.Listen` → `grpc.NewServer()` → `RegisterAuthorizationServer` → `go grpcSrv.Serve(lis)`), `Addr()`, `Stop()` (GracefulStop). COPY this shape for `accessloggrpc`.
- **`test/fixtures/0021-http-ext-authz-grpc/`** — the driver-owned-gRPC-server fixture precedent: `inputs/driver.go` allocates `authPort` lazily, starts `extauthzgrpc.NewAtAddr("0.0.0.0:port")`, bakes the SAME port into both YAMLs (`AuthHost=host.docker.internal` reference / `AuthHost=127.0.0.1` subject), holds `authSrv *extauthzgrpc.Server` for assertions. The gRPC cluster in `envoy.yaml` is `type: STRICT_DNS` + `typed_extension_protocol_options: HttpProtocolOptions{ explicit_http_config: { http2_protocol_options: {} } }`. COPY this layout for `0081`.
- **`test/differential/runner_test.go`** — the BackendKind dispatch switch (`:203`–`:1036`); `serveGRPCHealth` in-process h2c gRPC backend (`:3022`) — the `0.0.0.0:0` bind + go-serve precedent. **NO new BackendKind in 44.1** (see D-ALS-RECEIVER-WIRING below).
- **`test/differential/fixture/fixture.go`** — `BackendKind` enum, tail `H2GoawayResponder = 38` (`:606`). **UNCHANGED in 44.1.** The `0081` data-plane backend REUSES `HTTPFixedBody = 4` (`:156`).

### Proto facts (verified at PLAN time against `go-control-plane/envoy@v1.32.4` in the module cache — re-confirm at IMPL)

Service `github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3`:
- `AccessLogServiceClient`, `func NewAccessLogServiceClient(cc grpc.ClientConnInterface) AccessLogServiceClient`.
- `AccessLogService_StreamAccessLogsClient interface { Send(*StreamAccessLogsMessage) error; CloseAndRecv() (*StreamAccessLogsResponse, error); grpc.ClientStream }`.
- `AccessLogServiceServer`, `UnimplementedAccessLogServiceServer`, `func RegisterAccessLogServiceServer(s grpc.ServiceRegistrar, srv AccessLogServiceServer)`, `AccessLogService_StreamAccessLogsServer interface { SendAndClose(*StreamAccessLogsResponse) error; Recv() (*StreamAccessLogsMessage, error); grpc.ServerStream }`.
- `StreamAccessLogsMessage{ Identifier *StreamAccessLogsMessage_Identifier; LogEntries oneof }`; `.GetHttpLogs() *StreamAccessLogsMessage_HTTPAccessLogEntries`; `.GetIdentifier()`.
- `StreamAccessLogsMessage_Identifier{ Node *corev3.Node; LogName string }`.
- `StreamAccessLogsMessage_HttpLogs{ HttpLogs *StreamAccessLogsMessage_HTTPAccessLogEntries }` (the oneof arm wrapper).
- `StreamAccessLogsMessage_HTTPAccessLogEntries{ LogEntry []*dataaccesslogv3.HTTPAccessLogEntry }` (`.GetLogEntry()`).

Config `github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3`:
- `HttpGrpcAccessLogConfig.GetCommonConfig() *CommonGrpcAccessLogConfig`.
- `CommonGrpcAccessLogConfig.GetLogName() string`, `.GetGrpcService() *corev3.GrpcService`, `.GetTransportApiVersion() corev3.ApiVersion`, `.GetBufferFlushInterval()`, `.GetBufferSizeBytes()`.

Core `github.com/envoyproxy/go-control-plane/envoy/config/core/v3`:
- `GrpcService.GetEnvoyGrpc() *GrpcService_EnvoyGrpc`, `.GetGoogleGrpc() *GrpcService_GoogleGrpc`; `GrpcService_EnvoyGrpc.GetClusterName() string`.
- `ApiVersion_AUTO = 0`, `ApiVersion_V2 = 1`, `ApiVersion_V3 = 2`.
- `Node{ Id, Cluster string }`.

Data `github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3`:
- `HTTPAccessLogEntry.GetCommonProperties() *AccessLogCommon`, `.GetProtocolVersion() HTTPAccessLogEntry_HTTPVersion`, `.GetRequest() *HTTPRequestProperties`, `.GetResponse() *HTTPResponseProperties`.
- `HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED = 0`, `_HTTP10 = 1`, `_HTTP11 = 2`, `_HTTP2 = 3`, `_HTTP3 = 4`.
- `HTTPRequestProperties{ RequestMethod corev3.RequestMethod; Path, Authority, UserAgent string }`; `HTTPResponseProperties{ ResponseCode *wrapperspb.UInt32Value; ResponseBodyBytes uint64 }`; `AccessLogCommon{ StartTime *timestamppb.Timestamp; Duration *durationpb.Duration; UpstreamRemoteAddress *corev3.Address }`.
- `request.request_method` is the `corev3.RequestMethod` ENUM (`RequestMethod_GET = 1`, etc.) — a string→enum conversion, parallel to `protocol_version`. (Confirm the enum mapping at IMPL; `corev3.RequestMethod_value[strings.ToUpper(rec.Method)]`.)

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path (this plan lives in the worktree); the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0081'`, NEVER bare `'0081'` (which matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the sink's writer goroutine is a background mutator; after it lands run the FULL `internal/accesslog` package `-race`, NOT a `-run` subset.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.

---

## D-question resolutions (the SPEC §12 PLAN pins — settled here)

**D-ALS-CONFIG-DEFER → PARSE-ACCEPT-but-INERT + parallel slices.**
- `buffer_size_bytes` / `buffer_flush_interval` (44.2) + `additional_request_headers_to_log` / `additional_response_headers_to_log` (44.3) + the `AccessLog.filter` field are PARSE-ACCEPTED-but-INERT at 44.1 (a config carrying them boots and logs, just without the deferred behavior) — matches SPEC §2/§3.1's documented phased-rollout posture. A strict-reject-until-the-leg posture is REJECTED (it would make a real upstream config that sets `buffer_size_bytes` fail to boot, a worse divergence than inert-accept).
- **Keep PARALLEL SLICES**: a NEW `Bootstrap.ALSConfigs []ALSConfig` separate from `AccessLogConfigs []AccessLogConfig`. Per-sink-type ordering within a listener is NOT load-bearing (every `Record` is `Submit`ted to every sink, `accesslog_emit.go:34`), so a unify-into-one-ordered-list refactor would churn the byte-stable file-sink path for ZERO behavioral gain. Parallel slices is the minimal, byte-stable choice.

**D-ALS-RECEIVER-WIRING → a driver-owned `test/helpers/accessloggrpc` helper; NO new BackendKind.**
- The receiver is a NEW shared test-only package `test/helpers/accessloggrpc` (the `test/helpers/extauthzgrpc` precedent), started BY THE DRIVER (not the runner's BackendKind dispatch). Rationale: the ext_authz gRPC fixture (`0021`) — the directly analogous "the proxy dials an in-process gRPC server whose accumulated state the driver asserts" shape — owns its gRPC server in the driver (`extauthzgrpc.NewAtAddr` at `driver.go`), NOT via a BackendKind, precisely because the driver must (a) hold the server reference to read accumulated entries for assertions, and (b) bake the SAME pre-allocated port into BOTH bootstrap YAMLs. The ALS receiver has identical requirements ⇒ driver-owned.
- **CONSEQUENCE — NO new BackendKind (BackendKind stays 38, revising the SPEC §8.2/§14 anticipated 39).** The `0081` data-plane backend (the route target) REUSES the existing `HTTPFixedBody = 4` BackendKind (a fixed-size body ⇒ deterministic `response.response_body_bytes`, which AMEND-ALS-4 requires). This is a clean PLAN-time refinement of a SPEC anticipation, exactly what D-ALS-RECEIVER-WIRING exists to resolve. Final IMPL counts: BackendKind **38** (UNCHANGED), NOT 39.

**D-ALS-NODE → source `identifier.node` from `bs.Proto.GetNode()` (id + cluster); UNasserted.**
- Build a minimal `*corev3.Node{ Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster() }` at boot (both getters are nil-safe; empty when the bootstrap has no `node`). Thread it from `main.go` into `NewGrpcAccessLogSink`. `node.*` is non-deterministic cross-side ⇒ UNasserted in the `0081` differential regardless (AMEND-ALS-4). Confirmed available: `internal/listener/manager.go:241` already reads `bs.GetNode().GetCluster()`.

**D-ALS-SPLIT-FINAL → one leg (re-checked, ~300–360 prod LoC, at the ADR-0045 soft gate).**
- Estimated prod LoC: bootstrap parse arm + `ALSConfig` + STRICT-REJECT arms ≈ 70; `ALSClient` typed wrapper ≈ 45; `GrpcAccessLogSink` (channel + Submit + writer goroutine + lazy/identifier/reconnect + Close) ≈ 140; `Record`→`HTTPAccessLogEntry` mapping + the two enum converters ≈ 75; stats registration ≈ 12; main wiring ≈ 22. Total ≈ **365 prod LoC** — right at the ADR-0045 soft gate. Ships as ONE leg (the SPEC chartered 44.1 as the core leg; buffering 44.2 + header-capture 44.3 are separately chartered). No further split.

**D-ALS-FUZZER → land the parse fuzzer at 44.1 (fuzzers 43 → 44).**
- Land a `FuzzParseHttpGrpcAccessLogConfig` no-panic fuzzer over the new parse arm (the natural new attack surface; the bootstrap-parse-fuzzer precedent). The documented running total is reconciled at **43** (43.2b D-H2B-FUZZER-RECONCILE; actual `^func Fuzz` = 43, verified at PLAN time) ⇒ the new fuzzer advances both the doc figure AND the actual count to **44** with no drift to absorb. Re-verify `grep -rc '^func Fuzz' --include='*.go' | awk -F: '{s+=$2} END{print s}'` == 44 at the completion task (`reference_fuzzer_count_docs_drift`).

---

## File structure (decomposition locked here)

**Production (touched):**
- `internal/bootstrap/bootstrap.go` — MODIFY: add the gRPC-ALS blank-import; the `httpGrpcAccessLogTypeURL` const; `ALSConfig` struct; `Bootstrap.ALSConfigs` field; the parse arm + STRICT-REJECT in `parseOneAccessLog`.
- `internal/bootstrap/alsconfig_fuzz_test.go` — CREATE: the parse fuzzer.
- `internal/grpcclient/grpcclient.go` — MODIFY: add the `accesslogv3` import + the `ALSClient` type + `NewALSClient` + `StreamAccessLogs` + `Close`.
- `internal/accesslog/grpcsink.go` — CREATE: `GrpcAccessLogSink` + the `alsClient` seam interface + `NewGrpcAccessLogSink`.
- `internal/accesslog/mapping.go` — CREATE: `buildHTTPAccessLogEntry` + `protocolVersionEnum` + `requestMethodEnum`.
- `internal/accesslog/stats.go` — MODIFY: add `RegisterGrpcSinkCounters`.
- `cmd/envoy-go/main.go` — MODIFY: the boot wiring (Dialer + per-`ALSConfig` sink build + counter registration + node).

**Test (created):**
- `internal/bootstrap/bootstrap_test.go` — MODIFY: the parse-accept + STRICT-REJECT + accept-inert table tests.
- `internal/grpcclient/grpcclient_test.go` — MODIFY: the `ALSClient` tests.
- `internal/accesslog/mapping_test.go` — CREATE: the table-driven mapping + enum tests.
- `internal/accesslog/grpcsink_test.go` — CREATE: the sink channel/drop/identifier-once/Close + `-race` tests (against a fake `alsClient`).
- `internal/accesslog/stats_test.go` — MODIFY/CREATE: the registration test (surface 1187 → 1189).
- `test/helpers/accessloggrpc/accessloggrpc.go` (+ `_test.go`) — CREATE: the in-process ALS receiver accumulator.
- `test/fixtures/0081-grpc-access-log/{inputs/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0081` inputs package (the fixture auto-discovery seam).

**Docs (completion task):**
- `docs/envoy-go/DECISIONS.md` (ADR-0255 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.1.md`.

---

## Task 1: Phase scaffolding — PROGRESS-44.1.md + baselines + the final ADR-0045 split re-check (D-ALS-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.1.md`

- [ ] **Step 1: Record the baseline counts**

Run and record the verbatim outputs in PROGRESS-44.1.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                     # expect 82 (incl the letter-suffixed 0007a-cors + 0007b-iteration-probe; tail 0080-h2-goaway-rotation). NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS by 2 (it skips 0007a/0007b) — use the glob form for the canonical 82.
grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'   # expect 43
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go      # expect = 38 (the BackendKind tail)
```
Baseline: stat surface **1187** (H2 cluster; non-H2 **1183**) / fixtures **82** / fuzzers **43** / BackendKind **38** / DECISIONS tail **ADR-0254** (next-free **ADR-0255**).

- [ ] **Step 2: Write the PROGRESS-44.1.md scaffold** — a header (phase 44.1 IMPL, the SPEC reference, the worktree branch), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1189** / fixtures **83** / fuzzers **44** / BackendKind **38** (UNCHANGED — see D-ALS-RECEIVER-WIRING) / DECISIONS **ADR-0255**.

- [ ] **Step 3: Record the D-ALS-SPLIT-FINAL re-check** — note the ~365 prod-LoC estimate (the breakdown above), confirm it sits at the ADR-0045 soft gate, and that 44.1 ships as ONE leg. (This is a bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.1.md
git commit -m "phase 44.1 Task 1: PROGRESS scaffold + baselines + the final ADR-0045 split re-check"
```

---

## Task 2: The `ALSConfig` parse arm + `Bootstrap.ALSConfigs` + STRICT-REJECT (`internal/bootstrap`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (a table driven over bootstrap YAML strings, the existing `Load(strings.NewReader(yaml))` test shape):
  - **accept-minimal**: an HCM `access_log[]` with `HttpGrpcAccessLogConfig { common_config: { log_name: "mylog", grpc_service: { envoy_grpc: { cluster_name: "als_cluster" } } } }` ⇒ `Load` succeeds, `len(bs.ALSConfigs) == 1`, `bs.ALSConfigs[0] == ALSConfig{ClusterName: "als_cluster", LogName: "mylog"}`.
  - **accept-empty-log_name**: `log_name` omitted ⇒ `ALSConfig{ClusterName: "als_cluster", LogName: ""}` (empty is valid).
  - **accept-inert-buffer**: a config additionally setting `buffer_size_bytes: 16384` + `buffer_flush_interval: 1s` ⇒ boots, `len(bs.ALSConfigs) == 1` (the fields are accepted-but-inert at 44.1).
  - **accept-inert-headers**: a config additionally setting `additional_request_headers_to_log: [x-foo]` ⇒ boots, `len(bs.ALSConfigs) == 1`.
  - **accept-transport-V3**: `transport_api_version: V3` ⇒ boots. **accept-transport-AUTO**: omitted (AUTO=0) ⇒ boots.
  - **reject-google_grpc**: `grpc_service: { google_grpc: { … } }` ⇒ `Load` errors with a `bootstrap:`-prefixed message naming `google_grpc`.
  - **reject-transport-V2**: `transport_api_version: V2` ⇒ errors.
  - **reject-empty-cluster**: `grpc_service: { envoy_grpc: { cluster_name: "" } }` ⇒ errors.
  - **coexist**: a file `access_log` + a gRPC `access_log` in the same HCM ⇒ `len(bs.AccessLogConfigs) == 1` AND `len(bs.ALSConfigs) == 1` (the parallel-slices decision).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: FAIL (`ALSConfigs` undefined / gRPC configs silently ignored so the slice is empty).

- [ ] **Step 3: Implement** in `bootstrap.go`:

Add the blank-import alongside `:39`/`:40` (ADR-0016):
```go
// Phase 44.1 registers the gRPC-ALS access-logger extension proto so protojson
// round-trips bootstraps carrying HCM access_log[] HttpGrpcAccessLogConfig
// entries (ADR-0255). Per ADR-0016 amendment policy documented in PROGRESS.
_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
```
Add a typed import for the parse (alias to avoid the existing `accesslogv3` = `config/accesslog/v3` collision):
```go
grpcalv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
```
Add the TypeURL const alongside `fileAccessLogTypeURL` (`:145`):
```go
// httpGrpcAccessLogTypeURL is the TypeURL for the gRPC HTTP access logger
// (envoy.access_loggers.http_grpc). Lifted from the ADR-0041 silent-ignore set
// at phase 44.1 (ADR-0255).
httpGrpcAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig"
```
Add the struct + field (parallel to `AccessLogConfig` `:152` / `AccessLogConfigs` `:177`):
```go
// ALSConfig is the parsed gRPC Access Log Service sink config from one HCM
// access_log[] HttpGrpcAccessLogConfig entry (ADR-0255). The sink is built in
// cmd/envoy-go/main.go after Load returns. buffer_*/additional_*_headers are
// PARSE-ACCEPTED-but-INERT at 44.1 (honored at 44.2/44.3); only the two fields
// below are consumed.
type ALSConfig struct {
    ClusterName string // common_config.grpc_service.envoy_grpc.cluster_name
    LogName     string // common_config.log_name (empty is valid)
}
```
Add `ALSConfigs []ALSConfig` to `Bootstrap`. Extend `parseOneAccessLog` (after the `fileAccessLogTypeURL` branch, replacing the unconditional silent-ignore `return nil` at `:274` with a gRPC-ALS branch then the silent-ignore):
```go
if tc.GetTypeUrl() == httpGrpcAccessLogTypeURL {
    return parseGrpcAccessLog(tc, idx, result)
}
// Other non-file typed_config (stdout, tcp_grpc, open_telemetry) — silently
// ignored per ADR-0041 amendment.
return nil
```
And the helper:
```go
func parseGrpcAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error {
    cfg := &grpcalv3.HttpGrpcAccessLogConfig{}
    if err := proto.Unmarshal(tc.GetValue(), cfg); err != nil {
        return fmt.Errorf("bootstrap: access_log[%d] grpc unmarshal: %w", idx, err)
    }
    common := cfg.GetCommonConfig()
    // STRICT-REJECT (ADR-0080): transport_api_version V2.
    if v := common.GetTransportApiVersion(); v == corev3.ApiVersion_V2 {
        return fmt.Errorf("bootstrap: access_log[%d]: grpc ALS transport_api_version V2 is not supported (envoy-go is V3-only)", idx)
    }
    gs := common.GetGrpcService()
    eg := gs.GetEnvoyGrpc()
    // STRICT-REJECT (ADR-0080): google_grpc, or grpc_service absent.
    if eg == nil {
        return fmt.Errorf("bootstrap: access_log[%d]: grpc ALS requires grpc_service.envoy_grpc (google_grpc is not supported)", idx)
    }
    // STRICT-REJECT (ADR-0080): empty cluster_name.
    if eg.GetClusterName() == "" {
        return fmt.Errorf("bootstrap: access_log[%d]: grpc ALS grpc_service.envoy_grpc.cluster_name is required (must be non-empty)", idx)
    }
    result.ALSConfigs = append(result.ALSConfigs, ALSConfig{
        ClusterName: eg.GetClusterName(),
        LogName:     common.GetLogName(),
    })
    return nil
}
```
(Add the `anypb` import if not present — `google.golang.org/protobuf/types/known/anypb`; check the existing imports first.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/
git commit -m "phase 44.1 Task 2: parse HttpGrpcAccessLogConfig → ALSConfig + STRICT-REJECT arms (ADR-0080); buffer_*/headers PARSE-ACCEPT-INERT"
```

---

## Task 3: The `HttpGrpcAccessLogConfig` parse fuzzer (D-ALS-FUZZER; fuzzers 43 → 44) [fuzz]

**Files:**
- Create: `internal/bootstrap/alsconfig_fuzz_test.go`

- [ ] **Step 1: Write the fuzzer** — a no-panic fuzzer that wraps arbitrary bytes as a `HttpGrpcAccessLogConfig` `typed_config` inside a minimal bootstrap and feeds it through `Load` (or directly through `parseGrpcAccessLog` with a synthesised `*anypb.Any`). The invariant: `Load` / the parse arm NEVER panics (it returns a value or a `bootstrap:`-prefixed error) for any input. Model it on the existing bootstrap-parse fuzzer (find it with `grep -rln '^func Fuzz' internal/bootstrap/`); reuse its seed-corpus + harness shape.
```go
func FuzzParseHttpGrpcAccessLogConfig(f *testing.F) {
    f.Add([]byte{})                          // empty
    f.Add([]byte("\x0a\x00"))                // a truncated CommonGrpcAccessLogConfig
    f.Fuzz(func(t *testing.T, data []byte) {
        any := &anypb.Any{TypeUrl: httpGrpcAccessLogTypeURL, Value: data}
        result := &Bootstrap{}
        _ = parseGrpcAccessLog(any, 0, result) // must not panic
    })
}
```

- [ ] **Step 2: Run the fuzzer briefly to confirm it executes**

Run: `go test ./internal/bootstrap/ -run 'FuzzParseHttpGrpcAccessLogConfig' -count=1` then `go test ./internal/bootstrap/ -fuzz 'FuzzParseHttpGrpcAccessLogConfig' -fuzztime 20s`
Expected: PASS / no crashers.

- [ ] **Step 3: Confirm the count advanced**

Run: `grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'`
Expected: **44** (was 43). Record in PROGRESS-44.1.md.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/alsconfig_fuzz_test.go
git commit -m "phase 44.1 Task 3: FuzzParseHttpGrpcAccessLogConfig (no-panic); fuzzers 43 → 44 (D-ALS-FUZZER)"
```

---

## Task 4: The `ALSClient` typed wrapper (`internal/grpcclient` — the `AuthClient` precedent, ADR-0158) [TDD]

**Files:**
- Modify: `internal/grpcclient/grpcclient.go`
- Test: `internal/grpcclient/grpcclient_test.go`

- [ ] **Step 1: Write the failing tests** (mirror the existing `AuthClient` tests — find them in `grpcclient_test.go`):
  - **nil-dialer**: `NewALSClient(nil, "c")` ⇒ `(nil, err)` naming the cluster.
  - **unknown-cluster**: a `Dialer` over a manager with no `c` ⇒ `NewALSClient(d, "c")` errors `unknown cluster` (the `DialContext` gate).
  - **non-H2-cluster**: a cluster without `http2_protocol_options{}` ⇒ errors `HTTP/2 framing` (the `UseH2()` gate). (Reuse the test-cluster-manager builders the `AuthClient` tests use.)
  - **Close-idempotent**: against a valid H2 cluster, `c.Close()` twice returns the same (nil) error — no panic.
  - **StreamAccessLogs-returns-stream**: against an in-process `accessloggrpc` receiver (this can defer to Task 8's helper if ordering requires — otherwise stand up a bare `grpc.NewServer()` with `RegisterAccessLogServiceServer` of a no-op stub in the test), `c.StreamAccessLogs(ctx)` returns a non-nil stream whose `Send` succeeds. (If Task 8 is not yet landed, assert only construction + a `Send` against a throwaway in-test server.)

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/grpcclient/ -run TestALS -count=1`
Expected: FAIL (`ALSClient` / `NewALSClient` undefined).

- [ ] **Step 3: Implement** in `grpcclient.go` (add `accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"` to the imports; the module already provides `auth/v3`):
```go
// ----------------------------------------------------------------------------
// ALSClient — the typed AccessLogService/StreamAccessLogs wrapper (ADR-0255).
// ----------------------------------------------------------------------------

// ALSClient wraps a *grpc.ClientConn with the typed
// envoy.service.accesslog.v3.AccessLogServiceClient stub. One *ALSClient per
// gRPC-ALS sink (cluster_name), owned by the GrpcAccessLogSink and Close()d at
// sink close. The AuthClient precedent (ADR-0158); StreamAccessLogs opens the
// client-streaming RPC the sink's writer goroutine drives.
type ALSClient struct {
    conn   *grpc.ClientConn
    stub   accesslogv3.AccessLogServiceClient
    target string // cluster_name — for logs/errors

    closeOnce sync.Once
    closeErr  error
}

// NewALSClient dials the named cluster via d.DialContext and wraps the resulting
// *grpc.ClientConn. On dial error returns (nil, err) verbatim (already
// cluster-named via DialContext's wrapping). The NewAuthClient shape (:178).
func NewALSClient(d *Dialer, clusterName string) (*ALSClient, error) {
    if d == nil {
        return nil, fmt.Errorf("grpcclient: new ALS client %q: dialer is nil", clusterName)
    }
    conn, err := d.DialContext(context.Background(), clusterName)
    if err != nil {
        return nil, err
    }
    return &ALSClient{
        conn:   conn,
        stub:   accesslogv3.NewAccessLogServiceClient(conn),
        target: clusterName,
    }, nil
}

// StreamAccessLogs opens the AccessLogService/StreamAccessLogs client-streaming
// RPC. The caller (the sink writer goroutine) sends the identifier-once message
// then each entry, and CloseAndRecv's at drain.
func (a *ALSClient) StreamAccessLogs(ctx context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error) {
    if a == nil || a.stub == nil {
        return nil, errors.New("grpcclient: StreamAccessLogs: nil ALSClient / stub")
    }
    return a.stub.StreamAccessLogs(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (sync.Once), the
// AuthClient.Close shape (:231).
func (a *ALSClient) Close() error {
    if a == nil {
        return nil
    }
    a.closeOnce.Do(func() {
        if a.conn != nil {
            a.closeErr = a.conn.Close()
        }
    })
    return a.closeErr
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/grpcclient/ -run TestALS -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/grpcclient/ && golangci-lint run ./internal/grpcclient/... && go vet ./internal/grpcclient/... && go build ./...
git add internal/grpcclient/
git commit -m "phase 44.1 Task 4: ALSClient typed wrapper over the grpcclient.Dialer (the AuthClient precedent, ADR-0158)"
```

---

## Task 5: The `Record` → `HTTPAccessLogEntry` mapping + the two enum converters (`internal/accesslog/mapping.go`) [TDD, table-driven]

**Files:**
- Create: `internal/accesslog/mapping.go`
- Test: `internal/accesslog/mapping_test.go`

This is a PURE function — no gRPC, no goroutine — so it is the cleanest unit to TDD before the sink composes it.

- [ ] **Step 1: Write the failing table tests** in `mapping_test.go`:
  - `protocolVersionEnum`: `"HTTP/1.1"` → `HTTPAccessLogEntry_HTTP11`(2); `"HTTP/2.0"` → `_HTTP2`(3); `""`/`"HTTP/3"`/garbage → `_PROTOCOL_UNSPECIFIED`(0).
  - `requestMethodEnum`: `"GET"` → `RequestMethod_GET`; `"POST"` → `RequestMethod_POST`; `""`/garbage → `RequestMethod_METHOD_UNSPECIFIED`(0). (Use `corev3.RequestMethod_value[strings.ToUpper(m)]` with a fallback.)
  - `buildHTTPAccessLogEntry(rec)` over a fully-populated `*Record` ⇒ assert the 7 DETERMINISTIC fields (NOTE: the function takes ONLY `*Record` — `node`/`log_name` are NOT per-entry fields; the sink attaches them to the first `StreamAccessLogsMessage` identifier, Task 6): `GetRequest().GetRequestMethod()==GET`, `GetRequest().GetPath()=="/foo"`, `GetRequest().GetAuthority()=="example.com"`, `GetRequest().GetUserAgent()=="agent/1"`, `GetResponse().GetResponseCode().GetValue()==200`, `GetResponse().GetResponseBodyBytes()==13`, `GetProtocolVersion()==HTTP11`. And assert the non-deterministic fields are POPULATED (non-nil) but leave their values unasserted: `GetCommonProperties().GetStartTime()!=nil`, `GetCommonProperties().GetDuration()!=nil`, and for a `UpstreamHost=="10.0.0.1:8080"` that `GetCommonProperties().GetUpstreamRemoteAddress().GetSocketAddress().GetAddress()=="10.0.0.1"` + `GetPortValue()==8080`.
  - **empty UpstreamHost** ⇒ `GetCommonProperties().GetUpstreamRemoteAddress()==nil` (leave nil; do not synthesise a zero address).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestMapping -count=1`
Expected: FAIL (functions undefined).

- [ ] **Step 3: Implement** `mapping.go`. Imports: `dataaccesslogv3 "…/data/accesslog/v3"`, `corev3 "…/config/core/v3"`, `wrapperspb`, `timestamppb`, `durationpb`, `net`, `strconv`, `strings`. Sketch:
```go
func protocolVersionEnum(proto string) dataaccesslogv3.HTTPAccessLogEntry_HTTPVersion {
    switch proto {
    case "HTTP/1.1": return dataaccesslogv3.HTTPAccessLogEntry_HTTP11
    case "HTTP/2.0": return dataaccesslogv3.HTTPAccessLogEntry_HTTP2
    case "HTTP/1.0": return dataaccesslogv3.HTTPAccessLogEntry_HTTP10
    default:         return dataaccesslogv3.HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED
    }
}

func requestMethodEnum(m string) corev3.RequestMethod {
    if v, ok := corev3.RequestMethod_value[strings.ToUpper(m)]; ok {
        return corev3.RequestMethod(v)
    }
    return corev3.RequestMethod_METHOD_UNSPECIFIED
}

// buildHTTPAccessLogEntry maps the 10-field Record into the structured proto
// (SPEC §3.4 / AMEND-ALS-2/4). The 3 non-deterministic fields (start_time,
// duration, upstream_remote_address) are populated but UNasserted cross-side.
func buildHTTPAccessLogEntry(rec *Record) *dataaccesslogv3.HTTPAccessLogEntry {
    e := &dataaccesslogv3.HTTPAccessLogEntry{
        ProtocolVersion: protocolVersionEnum(rec.Protocol),
        Request: &dataaccesslogv3.HTTPRequestProperties{
            RequestMethod: requestMethodEnum(rec.Method),
            Path:          rec.Path, // path-only (AMEND-ALS-2); reference carries the query string
            Authority:     rec.Authority,
            UserAgent:     rec.UserAgent,
        },
        Response: &dataaccesslogv3.HTTPResponseProperties{
            ResponseCode:      wrapperspb.UInt32(uint32(rec.ResponseCode)),
            ResponseBodyBytes: uint64(rec.BytesSent),
        },
        CommonProperties: &dataaccesslogv3.AccessLogCommon{
            StartTime: timestamppb.New(rec.StartTime),
            Duration:  durationpb.New(rec.Duration),
        },
    }
    if addr := socketAddress(rec.UpstreamHost); addr != nil {
        e.CommonProperties.UpstreamRemoteAddress = addr
    }
    return e
}

// socketAddress splits "host:port" → core.Address{SocketAddress}; nil on empty
// or unparseable input (best-effort; UNasserted cross-side).
func socketAddress(hostPort string) *corev3.Address {
    if hostPort == "" {
        return nil
    }
    host, portStr, err := net.SplitHostPort(hostPort)
    if err != nil {
        return nil
    }
    port, err := strconv.ParseUint(portStr, 10, 32)
    if err != nil {
        return nil
    }
    return &corev3.Address{Address: &corev3.Address_SocketAddress{SocketAddress: &corev3.SocketAddress{
        Address:       host,
        PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(port)},
    }}}
}
```
(`identifier.node`/`log_name` are NOT part of the per-entry mapping — they go on the FIRST `StreamAccessLogsMessage`, built in the sink, Task 6.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/accesslog/ -run TestMapping -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/mapping.go internal/accesslog/mapping_test.go
git commit -m "phase 44.1 Task 5: Record → HTTPAccessLogEntry structured mapping + protocol_version/request_method enum converters (AMEND-ALS-2/4)"
```

---

## Task 6: The `GrpcAccessLogSink` (`internal/accesslog/grpcsink.go`) [TDD, `-race`]

**Files:**
- Create: `internal/accesslog/grpcsink.go`
- Test: `internal/accesslog/grpcsink_test.go`

The sink mirrors `AsyncFileSink` (bounded channel + writer goroutine + idempotent Close). To unit-test WITHOUT a real gRPC server, the sink depends on a minimal interface `alsClient` (satisfied by `*grpcclient.ALSClient`); tests inject a fake.

- [ ] **Step 1: Write the failing tests** in `grpcsink_test.go` against a fake `alsClient` (a struct recording every `Send`ed `*StreamAccessLogsMessage` + a configurable `Send`-error to force the reconnect path):
  - **submit-streams-entry**: `Submit(rec)` ⇒ the fake's stream receives ONE message whose `GetHttpLogs().GetLogEntry()` has one entry with the mapped method/path; `logsWritten == 1` after drain.
  - **identifier-once**: `Submit` 3 records ⇒ exactly the FIRST `Send`'s message carries a non-nil `GetIdentifier()` (with `LogName` set + the node), subsequent messages carry `GetIdentifier()==nil` + only `GetHttpLogs()`.
  - **drop-newest**: a capacity-1 sink (test constructor) with a blocked writer ⇒ submitting past capacity increments `logsDropped`, never blocks `Submit`.
  - **reconnect-on-send-error**: the fake returns an error on the first `Send`, success after ⇒ the sink re-opens the stream (re-sends identifier) and the entry still lands; `logsWritten` counts the eventually-sent entry.
  - **close-idempotent**: `Close()` twice ⇒ no panic, the channel drains, `ALSClient.Close()` called once.
  - **non-Record-ignored**: `Submit("garbage")` ⇒ no panic, no entry, no `logsWritten`.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestGrpcSink -count=1`
Expected: FAIL (`GrpcAccessLogSink` / `NewGrpcAccessLogSink` undefined).

- [ ] **Step 3: Implement** `grpcsink.go`. Define the seam + the sink:
```go
// alsClient is the minimal sink-facing seam over *grpcclient.ALSClient
// (test-fakeable). *grpcclient.ALSClient satisfies it.
type alsClient interface {
    StreamAccessLogs(ctx context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error)
    Close() error
}

// GrpcAccessLogSink streams structured HTTPAccessLogEntry protos to an Envoy
// AccessLogService over a lazily-established, identifier-once, reused
// client-streaming RPC (ADR-0255). It mirrors AsyncFileSink's bounded-channel +
// writer-goroutine + idempotent-Close shape (writer.go). Fixed simple flush
// (one entry per message) at 44.1; buffering is 44.2.
type GrpcAccessLogSink struct {
    ch          chan any
    client      alsClient
    logName     string
    node        *corev3.Node
    logsWritten *stats.Counter
    logsDropped *stats.Counter
    done        chan struct{}
    closeOnce   sync.Once
    lastDropLog atomic.Int64
}

func NewGrpcAccessLogSink(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter) *GrpcAccessLogSink {
    return newGrpcSinkWithCapacity(client, logName, node, written, dropped, defaultChannelCapacity)
}
// newGrpcSinkWithCapacity is the test-friendly variant (the AsyncFileSink
// newAsyncFileSinkWithCapacity precedent).
func newGrpcSinkWithCapacity(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, capacity int) *GrpcAccessLogSink {
    s := &GrpcAccessLogSink{
        ch: make(chan any, capacity), client: client, logName: logName, node: node,
        logsWritten: written, logsDropped: dropped, done: make(chan struct{}),
    }
    go s.run()
    return s
}
```
- **`Submit`** — the `writer.go:73` drop-newest shape exactly (select-default → `logsDropped.Inc()` + the rate-limited diag).
- **`run`** — the writer goroutine: maintain a `stream` + a `sentIdentifier bool`; for each `r` from the channel, if `rec, ok := r.(*Record)`; lazily open the stream (`s.client.StreamAccessLogs(context.Background())`) if nil; build `msg := &StreamAccessLogsMessage{ LogEntries: &StreamAccessLogsMessage_HttpLogs{ HttpLogs: &StreamAccessLogsMessage_HTTPAccessLogEntries{ LogEntry: []*HTTPAccessLogEntry{ buildHTTPAccessLogEntry(rec) } } } }`; if `!sentIdentifier` set `msg.Identifier = &StreamAccessLogsMessage_Identifier{ Node: s.node, LogName: s.logName }` and `sentIdentifier = true`; `stream.Send(msg)`; on error log + reset `stream=nil, sentIdentifier=false` and retry ONCE (re-open + re-send, which re-attaches the identifier); on success `logsWritten.Inc()`. On channel-close: `stream.CloseAndRecv()` if non-nil, then `close(s.done)`.
- **`Close`** — `sync.Once`: `close(s.ch)`; `<-s.done`; `s.client.Close()`. (The `writer.go:88` shape.)

Imports: `context`, `log`, `sync`, `sync/atomic`, `time`, `accesslogv3`, `corev3`, `internal/stats`.

- [ ] **Step 4: Run to verify they pass + the FULL-package race**

Run: `go test ./internal/accesslog/ -run TestGrpcSink -count=1`
Expected: PASS.
Then the FULL package `-race` (the writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`):
Run: `go test ./internal/accesslog/ -race -count=1`
Expected: PASS, no race.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/grpcsink.go internal/accesslog/grpcsink_test.go
git commit -m "phase 44.1 Task 6: GrpcAccessLogSink — bounded channel + writer goroutine + lazy-establish/identifier-once/reconnect/idempotent-Close (ADR-0255)"
```

---

## Task 7: The two `access_logs.grpc_access_log.*` stat registrations + a registration test (1187 → 1189) [TDD]

**Files:**
- Modify: `internal/accesslog/stats.go`
- Test: `internal/accesslog/stats_test.go`

- [ ] **Step 1: Write the failing registration test** — `RegisterGrpcSinkCounters(reg)` returns two non-nil distinct `*stats.Counter`; the registry then carries `access_logs.grpc_access_log.logs_written` + `access_logs.grpc_access_log.logs_dropped` (assert via the registry's name-lookup / snapshot helper the existing stats tests use). Surface delta: registering them adds exactly **2** counters (the 1187 → 1189 arithmetic — assert the count delta, not the absolute 1189, to stay robust). Confirm the names are STATIC (no `IsValidName` guard needed — AMEND-ALS-1).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/accesslog/ -run TestGrpcSinkCounters -count=1`
Expected: FAIL (`RegisterGrpcSinkCounters` undefined).

- [ ] **Step 3: Implement** in `stats.go` (the `RegisterDroppedCounter` shape; a DISTINCT registration, NOT reusing `server.accesslog_dropped` — AMEND-ALS-1):
```go
// RegisterGrpcSinkCounters allocates the two process-global gRPC-ALS sink
// counters (ADR-0255 / AMEND-ALS-1). Registered once per process when ≥1 gRPC
// ALS sink is built. STATIC names (no IsValidName guard — not wire/config
// derived). NOT a reuse of server.accesslog_dropped — the gRPC sink owns its
// own logs_dropped.
func RegisterGrpcSinkCounters(reg *stats.Registry) (written, dropped *stats.Counter) {
    return reg.NewCounter("access_logs.grpc_access_log.logs_written"),
        reg.NewCounter("access_logs.grpc_access_log.logs_dropped")
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/accesslog/ -run TestGrpcSinkCounters -count=1`
Expected: PASS. Record the +2 surface delta (1187 → 1189) in PROGRESS-44.1.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go build ./...
git add internal/accesslog/stats.go internal/accesslog/stats_test.go
git commit -m "phase 44.1 Task 7: register access_logs.grpc_access_log.{logs_written,logs_dropped} (+2 → 1189; AMEND-ALS-1)"
```

---

## Task 8: Boot wiring (`cmd/envoy-go/main.go`) — Dialer + per-`ALSConfig` sink build + counters + node

**Files:**
- Modify: `cmd/envoy-go/main.go`

main.go is not unit-tested in isolation (the differential is its behavioral proof); the gate here is build + boot-smoke, with Task 11's `0081` fixture as the real test.

- [ ] **Step 1: Implement** — extend the sink-build block (`:106`–`:119`). After the file-sink loop, before the defer-LIFO `Close()`:
```go
// Phase 44.1 (ADR-0255): build one GrpcAccessLogSink per gRPC-ALS access_log[]
// entry. The two sink counters register once iff ≥1 ALS sink exists. The
// shared grpcclient.Dialer over cm; the node sourced from bootstrap.node
// (D-ALS-NODE; UNasserted). Appended to the same sinks slice → the defer-LIFO
// Close() below already closes them (each ALSClient is Close()d at sink close).
if len(bs.ALSConfigs) > 0 {
    dialer := grpcclient.New(cm)
    written, dropped := accesslog.RegisterGrpcSinkCounters(bs.Stats)
    node := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
    for _, cfg := range bs.ALSConfigs {
        client, err := grpcclient.NewALSClient(dialer, cfg.ClusterName)
        if err != nil {
            log.Fatalf("accesslog: gRPC ALS client for cluster %q: %v", cfg.ClusterName, err)
        }
        sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped))
    }
}
```
(Add imports: `grpcclient`, `corev3`. `sinks` was `make([]accesslog.Sink, 0, len(bs.AccessLogConfigs))` — leave the cap as-is, append grows it.)

- [ ] **Step 2: Build + boot-smoke**

Run: `go build ./... && echo BUILD_OK`
Then a manual boot-smoke against a hand-written bootstrap with a gRPC-ALS `access_log` pointing at a non-existent cluster ⇒ confirm the `log.Fatalf` fires (the Dialer's unknown-cluster gate surfaces at sink build). And against a valid H2 cluster ⇒ boots clean.

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 44.1 Task 8: boot wiring — grpcclient.Dialer + per-ALSConfig GrpcAccessLogSink build + counter registration + bootstrap node (D-ALS-NODE)"
```

---

## Task 9: The `test/helpers/accessloggrpc` receiver accumulator (D-ALS-RECEIVER-WIRING) [TDD]

**Files:**
- Create: `test/helpers/accessloggrpc/accessloggrpc.go`, `test/helpers/accessloggrpc/accessloggrpc_test.go`

The in-process ALS receiver — the `extauthzgrpc` precedent, but accumulating `HTTPAccessLogEntry`s across messages + streams (AMEND-ALS-3) and exposing a thread-safe poll surface for the driver.

- [ ] **Step 1: Write the failing test** — start a `New(t)`, dial it as a client, open `StreamAccessLogs`, `Send` two messages (the first with `identifier` + one entry, the second with two batched entries), `CloseAndRecv`; then assert `Server.Entries()` returns all 3 accumulated `*HTTPAccessLogEntry`s (order-preserving) and `Count() == 3`. A second stream's entries accumulate onto the same slice.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/helpers/accessloggrpc/ -count=1`
Expected: FAIL (package/symbols undefined).

- [ ] **Step 3: Implement** — mirror `extauthzgrpc.go`:
```go
type Server struct {
    accesslogv3.UnimplementedAccessLogServiceServer
    addr     string
    lis      net.Listener
    grpcSrv  *grpc.Server
    mu       sync.RWMutex
    entries  []*dataaccesslogv3.HTTPAccessLogEntry
    stopOnce sync.Once
}
func New(t testing.TB) *Server { s, err := newServer("127.0.0.1:0"); if err != nil { t.Fatalf(...) }; t.Cleanup(s.Stop); return s }
func NewAtAddr(addr string) (*Server, error) { return newServer(addr) }     // 0.0.0.0:port for Docker reachability
func newServer(addr string) (*Server, error) {
    lis, err := net.Listen("tcp", addr); if err != nil { return nil, err }
    s := &Server{addr: lis.Addr().String(), lis: lis, grpcSrv: grpc.NewServer()}
    accesslogv3.RegisterAccessLogServiceServer(s.grpcSrv, s)
    go func() { _ = s.grpcSrv.Serve(lis) }()
    return s, nil
}
func (s *Server) StreamAccessLogs(stream accesslogv3.AccessLogService_StreamAccessLogsServer) error {
    for {
        msg, err := stream.Recv()
        if err == io.EOF { return stream.SendAndClose(&accesslogv3.StreamAccessLogsResponse{}) }
        if err != nil { return err }
        if http := msg.GetHttpLogs(); http != nil {
            s.mu.Lock(); s.entries = append(s.entries, http.GetLogEntry()...); s.mu.Unlock()
        }
    }
}
func (s *Server) Entries() []*dataaccesslogv3.HTTPAccessLogEntry { s.mu.RLock(); defer s.mu.RUnlock(); out := make([]*dataaccesslogv3.HTTPAccessLogEntry, len(s.entries)); copy(out, s.entries); return out }
func (s *Server) Count() int { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.entries) }
func (s *Server) Addr() string { return s.addr }
func (s *Server) Stop() { s.stopOnce.Do(s.grpcSrv.GracefulStop) }
```

- [ ] **Step 4: Run to verify it passes (with `-race` — accumulation is concurrent)**

Run: `go test ./test/helpers/accessloggrpc/ -race -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/accessloggrpc/ && golangci-lint run ./test/helpers/accessloggrpc/... && go build ./...
git add test/helpers/accessloggrpc/
git commit -m "phase 44.1 Task 9: test/helpers/accessloggrpc — in-process AccessLogService receiver accumulator (the extauthzgrpc precedent; D-ALS-RECEIVER-WIRING)"
```

---

## Task 10: The `0081-grpc-access-log` differential fixture (cross-side EXACT, poll-to-converge, query-less path)

**Files:**
- Create: `test/fixtures/0081-grpc-access-log/{inputs/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0081` inputs package)

Model the whole directory on `test/fixtures/0021-http-ext-authz-grpc/` (driver-owned gRPC server + dual-YAML port baking). The data-plane backend REUSES `HTTPFixedBody = 4` (deterministic `response_body_bytes`).

- [ ] **Step 1: Author the bootstraps.** Both: an **H1** downstream listener → a route to the `HTTPFixedBody` backend; an HCM `access_log[]` `HttpGrpcAccessLogConfig { common_config: { log_name: "0081", grpc_service: { envoy_grpc: { cluster_name: "c_als" } } } }`; a `c_als` cluster `type: STRICT_DNS` + `HttpProtocolOptions{ explicit_http_config: { http2_protocol_options: {} } }` (h2c, no TLS — D-ALS-RECEIVER) pointing at `{{.ALSHost}}:{{.ALSPort}}`. `envoy.yaml` (reference): `ALSHost = host.docker.internal`. `envoy-go.yaml` (subject): `ALSHost = 127.0.0.1`. (Copy the templating + admin/listener-port placeholders from `0021`.)

- [ ] **Step 2: Author `inputs/driver.go`.** `func init() { fixture.RegisterFixture("0081-grpc-access-log", &alsDriver{}) }`. Allocate `alsPort` lazily (the `0021` `allocateAuthPort` shape); start the receiver `accessloggrpc.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.alsPort))`, hold `d.alsSrv`; bake `alsPort` into BOTH bootstraps via the template data. Drive: fire **N** (e.g. 8) requests with a FIXED `User-Agent: als-probe/1`, `Host: als.example`, and a **query-less** path `/health` (AMEND-ALS-2 — a query string would diverge cross-side) against the data-plane listener. Then **poll-to-converge** (a release barrier, NEVER `time.Sleep` — `reference_concurrency_differential_release_barrier`): poll `d.alsSrv.Count()` until `>= N` or a 30s deadline (the `0066` `pollMembershipHealthy` 200ms-poll shape). **Per-side separation:** the receiver accumulates from BOTH sides; reset/snapshot between the reference run and the subject run (a `Reset()` accessor on the helper, OR a fresh receiver per side — prefer a fresh `accessloggrpc` server per side to keep the entry sets cleanly separated). Return the snapshot as the driver output for the asserter.

- [ ] **Step 3: Author the assertions (`expectations.yaml` + the driver's asserter).** Cross-side EXACT, aggregated over all N received entries (AMEND-ALS-3/4) — for each entry assert the 7-field subset: `request.request_method == GET`, `request.path == "/health"`, `request.authority == "als.example"`, `request.user_agent == "als-probe/1"`, `response.response_code == 200`, `response.response_body_bytes == <fixed body len>`, `protocol_version == HTTP11`. Plus the SUBJECT-side `access_logs.grpc_access_log.logs_written == N` (scrape `/stats`). **UNasserted:** `common_properties.*`, `identifier.node`, stream/message/batch framing, and every subject-absent field (`request.scheme`, `request_id`, `upstream_cluster`, `access_log_type`, `response_code_details`, wire-byte counts).

- [ ] **Step 4: Author `README.md`** — the fixture's purpose, the driver-owned-receiver lifecycle (the `0021` README §Auth-server-lifecycle shape), the AMEND-ALS-2 query-less constraint, the AMEND-ALS-3 framing-not-asserted note, and the host-reachability table (`host.docker.internal` reference / `127.0.0.1` subject).

- [ ] **Step 5: Register + run the fixture isolated.**

Add `_ "github.com/esalaine/envoy-go/test/fixtures/0081-grpc-access-log/inputs"` to `runner_test.go`'s fixture blank-imports.
Run (the correct selector — `reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0081' -count=1`
Expected: PASS (both sides stream the same 7-field subset; `logs_written == N`).
Confirm fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **83** (the glob form — NOT `grep -cE '^[0-9]{4}-'`, which undercounts by 2).

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0081-grpc-access-log/ test/differential/runner_test.go
git commit -m "phase 44.1 Task 10: 0081-grpc-access-log differential — cross-side EXACT on the 7-field subset, poll-to-converge, query-less path (fixtures 82 → 83)"
```

---

## Task 11: `0081` deliberate-break proofs + flake gate + the FULL-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0081` FAILS (proving the assertion is live), then `git restore` it:
  - (a) Break `protocolVersionEnum` to always return `HTTP2` ⇒ the `protocol_version == HTTP11` assertion must FAIL.
  - (b) Break `buildHTTPAccessLogEntry` to drop `UserAgent` ⇒ the `request.user_agent` assertion must FAIL.
  - (c) Break the `logsWritten.Inc()` site (skip it) ⇒ the `logs_written == N` subject assertion must FAIL.
  - (d) Break the path mapping to append a constant suffix ⇒ the `request.path == "/health"` assertion must FAIL.
  Run each: `go test ./test/differential/ -run 'TestDifferential/0081' -count=1` ⇒ expect FAIL, then restore ⇒ expect PASS. Record each break+restore in PROGRESS-44.1.md (the live-assertion proof).

- [ ] **Step 2: Flake gate** — 20 consecutive green runs:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0081' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. (A transient `subject ready: EOF` is the startup-race flake — `reference_differential_fullsuite_startup_flake` — isolate-re-run that single run; it is NOT a 0081 regression.)

- [ ] **Step 3: FULL `internal/accesslog` package `-race`** (the sink writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`):
```bash
go test ./internal/accesslog/ -race -count=1
```
Expected: PASS, no race.

- [ ] **Step 4: Commit the PROGRESS update** (break-proofs + flake + race recorded)
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.1.md
git commit -m "phase 44.1 Task 11: 0081 deliberate-break proofs (4 live assertions) + 20/20 flake + full-package -race"
```

---

## Task 12: Full 83-dir differential + six-gate + ADR-0255 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 44 leg 44.1 → done; family STAYS OPEN)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.1.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                 # clean
go build ./...                               # ok
go test ./... -count=1                       # full unit + the 83-dir differential
go test ./internal/accesslog/ -race -count=1 # the background-mutator race gate
```
Expected: all green. (The full differential is the byte-stability regression anchor — no non-ALS fixture should move.)

- [ ] **Step 2: ADR-0255 §Decision/§Consequences** — land them in DECISIONS.md beneath the §Context already drafted at SPEC §13 (ADR-0044). §Decision: the `GrpcAccessLogSink` over the `Sink` abstraction; the `ALSClient` over the `Dialer`; lazy-establish/identifier-once/reconnect/fixed-simple-flush; the 10-field mapping (`protocol_version`/`request_method` enums; `request.path` path-only per AMEND-ALS-2); the parse arm lifting gRPC ALS from the ADR-0041 silent-ignore set; STRICT-REJECT `google_grpc`/non-V3/empty-cluster; the two static `access_logs.grpc_access_log.*` counters; the driver-owned `accessloggrpc` receiver (NO new BackendKind — D-ALS-RECEIVER-WIRING). §Consequences: `buffer_*` (44.2) + `additional_*_headers` (44.3) PARSE-ACCEPT-INERT; the Observability family STAYS OPEN.

- [ ] **Step 3: BEHAVIOR_CONTRACT.md** — add the `### Access log — gRPC Access Log Service (ALS) streaming sink` subsection (SPEC §9) + advance the stat-surface block 1187 → 1189.

- [ ] **Step 4: STATE.md + ROADMAP.md** — STATE active-phase → `phase 44.1 (grpc-access-log) IMPL done`; the count figures → stat **1189** / fixtures **83** / fuzzers **44** / BackendKind **38** / DECISIONS **ADR-0255**. ROADMAP row 44 leg 44.1 → **done** (per-leg, ADR-0106; the Observability family STAYS OPEN — 44.2/44.3 + OTLP/tracing/stats-sinks/tap remain). Set the next action → the 44.2 (buffering) SPEC.

- [ ] **Step 5: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'` == **44** and advance the documented running total 43 → 44 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md / PROGRESS-44.1.md consistently.

- [ ] **Step 6: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 44.1 (grpc-access-log core) IMPL: ADR-0255 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 44 leg 44.1 done; Observability family STAYS OPEN); stat 1189 / fixtures 83 / fuzzers 44 / BackendKind 38"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 44.1 (grpc-access-log) IMPL: the core gRPC ALS streaming sink — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 44.1 IMPL squash and route the next session to the **44.2 (buffering) SPEC**.
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1189** (H2 cluster; non-H2 **1185**) / fixtures **83** (tail `0081-grpc-access-log`) / fuzzers **44** / BackendKind **38** (UNCHANGED — the ALS receiver is driver-owned) / DECISIONS **ADR-0255**. ZERO new go.mod modules (`go mod tidy -diff` EMPTY). ZERO new `internal/` packages (`accessloggrpc` is test-only).

> **NOTE for the executor on the SPEC's anticipated BackendKind 39:** the SPEC §8.2/§14 anticipated a new BackendKind 39 for the ALS receiver; this PLAN's D-ALS-RECEIVER-WIRING resolution supersedes that — the receiver is driver-owned (the `0021` ext_authz precedent), so BackendKind stays **38** and the data-plane backend reuses `HTTPFixedBody = 4`. If, at IMPL, a driver-owned receiver proves infeasible (e.g. the differential harness cannot bake a driver-allocated port into both YAMLs for this fixture shape — it can, per `0021`), fall back to a new BackendKind 39 and update these counts. Surface any such deviation to the controller before proceeding.
