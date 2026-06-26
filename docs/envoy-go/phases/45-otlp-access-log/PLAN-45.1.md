# Phase 45.1 Implementation Plan — the core OpenTelemetry (OTLP) access-log sink: config parse (LIFT `OpenTelemetryAccessLogConfig` from the ADR-0041 silent-ignore set) + the `OTLPLogsClient` UNARY typed wrapper + the `OTLPAccessLogSink` (built-in `time_unix_nano`-only mapping + the 4 Resource built-in labels; the reused 44.2 size/interval/close buffer → per-`Export` batch) + the `access_logs.open_telemetry_access_log.*` stats + the `0084` receiver differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** When an HCM `access_log[]` entry carries an `envoy.extensions.access_loggers.open_telemetry.v3.OpenTelemetryAccessLogConfig` typed_config, envoy-go dials the configured `common_config.grpc_service.envoy_grpc.cluster_name` cluster (H2-required) and BATCHES each access-log event into an OTLP `LogRecord` (built-in mapping = `time_unix_nano` only) inside one `ExportLogsServiceRequest`, sent over the **UNARY** `opentelemetry.proto.collector.logs.v1.LogsService.Export` RPC, attaching the 4 `Resource` built-in labels `{log_name, zone_name, cluster_name, node_name}` (dropped wholesale by `disable_builtin_labels`) — proven cross-side EXACT (record count + `time_unix_nano` presence + the 4 Resource label keys + `log_name` value) by the `0084-otlp-access-log` differential against `contrib-v1.37.2`.

**Architecture:** This is the project's THIRD concrete `accesslog.Sink` (alongside the phase-06.2 `AsyncFileSink` + the phase-44 `GrpcAccessLogSink`) and its THIRD `grpcclient` consumer (after ext_authz ADR-0158 + ALS ADR-0255). It composes on THREE already-built substrates — the phase-06.2 `Sink`/`Record` subsystem, the phase-18 `grpcclient.Dialer`, and the phase-44 sink/buffer/client machinery — adding ZERO new Go packages and exactly ONE new go.mod module (`go.opentelemetry.io/proto/otlp`, promoted transitive→direct). Byte-identical when no OTLP `access_log` entry is configured (the file + gRPC-ALS paths are untouched; the full differential is the regression anchor). The sink MIRRORS `GrpcAccessLogSink`'s bounded-channel + writer-goroutine + 44.2 size/interval/close BUFFER + idempotent-`Close` shape but is a PARALLEL implementation (unary `Export` of `LogRecord` batches, no stream lifecycle, no identifier-once); the `OTLPLogsClient` mirrors `ALSClient` but UNARY; the parse arm extends `parseOneAccessLog`. ANCHORS ADR-0258; the Observability family STAYS OPEN.

**Tech Stack:** Go; the in-tree `internal/accesslog` sink subsystem; `internal/grpcclient` (the `Dialer` + the `ALSClient` typed-wrapper pattern); `internal/bootstrap` (the `access_log` parse + the ADR-0016 proto blank-import registry); the resolved `go-control-plane/envoy v1.32.4` `OpenTelemetryAccessLogConfig` config proto; the `go.opentelemetry.io/proto/otlp v1.0.0` OTLP proto module (collector/logs/v1 + logs/v1 + common/v1 + resource/v1), PROMOTED transitive→direct; the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ONE new go.mod module (`go mod tidy -diff` anticipated to show ONLY the require-block promotion).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The access-log subsystem (`internal/accesslog`) already has TWO sink types — `AsyncFileSink` (Envoy-default-format TEXT lines to a file) and `GrpcAccessLogSink` (STRUCTURED `HTTPAccessLogEntry` protos client-STREAMED to an Envoy `AccessLogService` over `StreamAccessLogs`). You are adding a THIRD sink type — `OTLPAccessLogSink` — that BATCHES `LogRecord` protos to an OpenTelemetry `LogsService` over the **UNARY** `Export` RPC instead. The gRPC plumbing (`internal/grpcclient`) already exists: a `Dialer` turns a cluster name into a `*grpc.ClientConn` (gated on cluster-exists + the cluster being HTTP/2), and `ALSClient` is the typed-stub-wrapper precedent you will copy for `OTLPLogsClient` (but the ALS client wraps a client-streaming RPC; OTLP's `Export` is a plain unary call, so `OTLPLogsClient` is STRICTLY SIMPLER — no `StreamAccessLogs`, no stream lifecycle). The bootstrap parser (`internal/bootstrap`) already walks each HCM's `access_log[]`, recognises `FileAccessLog` + `HttpGrpcAccessLogConfig`, strict-rejects `TcpGrpcAccessLogConfig`, and silently ignores everything else (the inline comment at `bootstrap.go:336` names `open_telemetry` as silently-ignored); you will teach it to recognise the OTLP typed_config and parse it into an `OTLPConfig`.

The built-in OTLP record is FAR LEANER than a `HTTPAccessLogEntry`. Per the SPEC §11 live probe (contrib-v1.37.2, 2026-06-26, decode-ran proof `access_logs.open_telemetry_access_log.logs_written: 13`), the pure built-in `LogRecord` carries ONLY `time_unix_nano` — no `observed_time_unix_nano`, no `severity_*`, no `body`, no `LogRecord.attributes`. The only "built-in" content beyond the timestamp is FOUR `Resource.attributes` key/values (`log_name`/`zone_name`/`cluster_name`/`node_name`, always all 4 keys, empty when the source is unset), dropped wholesale by `disable_builtin_labels: true`. So 45.1's mapping is exactly `Record.StartTime → LogRecord.time_unix_nano` PLUS the 4-label `Resource`. The 9 other scalar `Record` fields + the two 44.3 header maps are NOT consumed at 45.1 (they have no built-in `LogRecord` home — they become 45.2 operator-templated `attributes`/`body`).

The differential test harness boots BOTH the real reference Envoy (in a Docker container, `contrib-v1.37.2`) AND the in-process subject (envoy-go) against equivalent bootstraps, drives the same traffic at both, and asserts equivalence. For this fixture, BOTH sides export their `LogRecord`s to the SAME in-process OTLP `LogsService` receiver (started by the test driver); the driver then asserts the aggregated per-record payload (count + `time_unix_nano` presence + the 4 Resource label keys + `log_name` value) matches cross-side — NOT `Export`-call count / per-call batch sizes / connection count, all of which legitimately vary (the unary `Export` BATCHES; framing is side-to-side variable per `reference_streaming_sink_differential_framing`). The reference (in Docker) reaches the host-bound receiver via `host.docker.internal`; the subject reaches it via `127.0.0.1`.

### Key source seams (verified at PLAN time against the tree at master `9989a7a4`; re-confirm line numbers before editing — files evolve)

- **`internal/accesslog/accesslog.go`** — `Sink interface { Submit(r any); Close() error }` (`:18`); `Record struct` (`:29`) with `StartTime time.Time` (`:30`) + the 9 other scalars + the two 44.3 header maps `RequestHeaders`/`ResponseHeaders` (`:41`/`:42`). **45.1 reads ONLY `Record.StartTime`.**
- **`internal/accesslog/grpcsink.go`** — the SHAPE you mirror (PARALLEL, not literal reuse): `const closeDrainGrace = 5 * time.Second` (`:31`); `GrpcAccessLogSink struct` (`:48`) with `ch chan any` / `client` / `logName` / `node *corev3.Node` / `logsWritten`+`logsDropped *stats.Counter` / `done` / `closeOnce` / `closeErr` / `lastDropLog atomic.Int64` / `bufferSizeBytes int` / `bufferFlushInterval time.Duration` / `ctx`+`cancel`; `NewGrpcAccessLogSink` (`:80`) + `newGrpcSinkWithCapacity` (`:86`, the test-friendly capacity variant); `Submit` drop-newest (`:121`); `Close` sync.Once + drain-grace + cancel (`:145`); `run` writer goroutine with the `flush` closure (size/timer/close triggers, `proto.Size` byte-sum at `:256`, `logsWritten.Add(uint64(len(buf)))` at `:224`, retry-once-then-drop) (`:166`). The OTLP sink's `run`/`flush` is SIMPLER — no `stream`/`sentIdentifier`/`CloseAndRecv` (unary `Export`).
- **`internal/accesslog/stats.go`** — `RegisterGrpcSinkCounters(reg) (written, dropped *stats.Counter)` (`:23`) → `reg.NewCounter("access_logs.grpc_access_log.{logs_written,logs_dropped}")`. COPY this shape for `RegisterOTLPSinkCounters` (STATIC names, no `IsValidName` guard).
- **`internal/accesslog/mapping.go`** — `func buildHTTPAccessLogEntry(rec *Record, reqHdrNames, respHdrNames []string) *dataaccesslogv3.HTTPAccessLogEntry` (`:54`). The 45.1 analogue is a MUCH simpler `buildLogRecord(rec *Record) *logspb.LogRecord` (time_unix_nano only) + a `buildResource` + a `buildExportRequest` — in a NEW `otlpmapping.go`.
- **`internal/grpcclient/grpcclient.go`** — `Dialer` + `New(mgr) *Dialer` + `(*Dialer).DialContext(ctx, clusterName)` with the cluster-exists + `UseH2()` PARSE-REJECT gates (unchanged); the `ALSClient` block (`:244`–`:315`): `ALSClient struct { conn; stub; target; closeOnce; closeErr }` (`:259`), `NewALSClient(d, clusterName)` (`:277`, dials via `d.DialContext(context.Background(), …)` then wraps the stub), `Close()` sync.Once-guarded (`:305`). COPY the `NewALSClient`/`Close` shape for `OTLPLogsClient`; replace `StreamAccessLogs` with a unary `Export`.
- **`internal/bootstrap/bootstrap.go`** — the access-logger blank-imports (alongside `file/v3` + the `grpc/v3` import); the TypeURL consts block (`:150`–`:176`: `fileAccessLogTypeURL` `:158`, `httpGrpcAccessLogTypeURL` `:163`, `tcpGrpcAccessLogTypeURL` `:169`, `alsDefaultBufferSizeBytes uint32 = 16384` `:175`); `ALSConfig struct` (`:193`); `Bootstrap.ALSConfigs` (`:235`); `parseOneAccessLog(al, idx, result)` (`:323`) — the gRPC-ALS branch (`:329`), the TCP-gRPC strict-reject (`:332`), the silent-ignore fallthrough (`:335`–`:339`, comment NAMES `open_telemetry`); `parseGrpcAccessLog(tc, idx, result)` (`:371`) — the V2/google_grpc/empty-cluster STRICT-REJECT arms (`:377`/`:382`/`:385`) + the `buffer_size_bytes` default-16384 (`:391`) + the `buffer_flush_interval` default-1s panic-guard (`:400`) + `lowerAll` for the header lists; `lowerAll` helper (`:424`). `bs.Proto.GetNode()` carries Id/Cluster/**Locality** (the full node).
- **`cmd/envoy-go/main.go`** — `cm` cluster manager (`:100`); the sink-build block (`:105`–`:145`): `droppedCounter := accesslog.RegisterDroppedCounter(bs.Stats)` (`:109`), `sinks := make([]accesslog.Sink, 0, len(bs.AccessLogConfigs))` (`:110`), the file-sink loop (`:111`–`:117`), **the ALS block GUARDED by `if len(bs.ALSConfigs) > 0 {` (`:129`)** — `dialer := grpcclient.New(cm)` (`:130`), `RegisterGrpcSinkCounters` (`:131`), `node := &corev3.Node{Id, Cluster}` (`:132`, the MINIMAL node), the per-`ALSConfig` `NewALSClient`→`NewGrpcAccessLogSink` loop (`:133`–`:139`); the defer-LIFO `Close()` (`:141`–`:145`). `corev3` + `grpcclient` are ALREADY imported. **Hoist the `dialer := grpcclient.New(cm)` out of the ALS `if` so an OTLP-only boot builds it (Task 8).**
- **`test/helpers/accessloggrpc/accessloggrpc.go`** — the driver-owned in-process gRPC-receiver-accumulator precedent: `Server` embeds `UnimplementedAccessLogServiceServer`, `New(t testing.TB)` (ephemeral `127.0.0.1:0` + `t.Cleanup`), `NewAtAddr(addr)` (caller-chosen `0.0.0.0:port` for Docker reachability), `newServer` (`net.Listen` → `grpc.NewServer()` → `RegisterAccessLogServiceServer` → `go grpcSrv.Serve(lis)`), the `StreamAccessLogs` accumulate-under-RWMutex, `Entries()`/`Count()`/`Reset()`/`Addr()`/`Stop()`/`Close()`. COPY this shape for `otlplogs` (an `Export`-unary server accumulating `LogRecord`s + per-`ResourceLogs` `Resource.attributes`).
- **`test/fixtures/0081-grpc-access-log/`** — the driver-owned-gRPC-receiver fixture precedent: `driver/driver.go` (package `driver`, `func init() { fixture.RegisterFixture(...) }`), `allocateALSPort` (lazy `Listen+Close`), `ensureServer` (`accessloggrpc.NewAtAddr("0.0.0.0:port")`), `ReferenceBootstrap`/`SubjectConfig` baking the SAME port into both YAMLs (`ALSHost=host.docker.internal` reference / `127.0.0.1` subject), `driveSide` (Reset → fire N query-less requests → `pollCount` to N → snapshot), `AssertStats` (per-entry subset on BOTH sides + the subject `/stats` `logs_written == N`). `BackendKind() == fixture.HTTPFixedBody`. COPY this whole directory layout for `0084`.
- **`test/differential/runner_test.go:108`** — `_ "github.com/esalaine/envoy-go/test/fixtures/0081-grpc-access-log/driver"` (the fixture blank-import auto-discovery seam). Add the `0084` driver import here.
- **`test/differential/fixture/fixture.go`** — `BackendKind` enum, tail `H2GoawayResponder = 38`. **UNCHANGED in 45.1** (the OTLP receiver is driver-owned). The `0084` data-plane backend REUSES `HTTPFixedBody = 4`.

### Proto facts (verified at PLAN time against `go-control-plane/envoy@v1.32.4` + `go.opentelemetry.io/proto/otlp@v1.0.0` in the module cache — re-confirm at IMPL)

Config `github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3` (the `OpenTelemetryAccessLogConfig` lives in `logs_service.pb.go`):
- `OpenTelemetryAccessLogConfig` fields by tag: `CommonConfig *v3.CommonGrpcAccessLogConfig` (field **1** — the SAME `CommonGrpcAccessLogConfig` `HttpGrpcAccessLogConfig` carries, `v3` = `access_loggers/grpc/v3`); `Body *v1.AnyValue` (field **2**); `Attributes *v1.KeyValueList` (field **3**); `ResourceAttributes *v1.KeyValueList` (field **4**); `DisableBuiltinLabels bool` (field **5**); `StatPrefix string` (field **6**); `Formatters []*v31.TypedExtensionConfig` (field **7**). (`v1` = otlp `common/v1`, pulled transitively by the field types.)
- Getters: `.GetCommonConfig()`, `.GetBody()`, `.GetAttributes()`, `.GetResourceAttributes()`, `.GetDisableBuiltinLabels()`, `.GetStatPrefix()`, `.GetFormatters()`.

OTLP module `go.opentelemetry.io/proto/otlp@v1.0.0` (NOT yet in go.mod — zero entries; the two `go.sum:171-172` lines are ORPHANS a `go mod tidy` would currently remove. It enters the build graph as an `// indirect` dep the moment Task 2's `open_telemetry/v3` blank-import lands (that package imports `go.opentelemetry.io/proto/otlp/common/v1`), then is PROMOTED to a DIRECT require at Task 4's first direct otlp import):
- `collector/logs/v1` (`collogspb`): `LogsServiceClient` interface with the UNARY `Export(ctx, *ExportLogsServiceRequest, opts ...grpc.CallOption) (*ExportLogsServiceResponse, error)` (`logs_service_grpc.pb.go:27`); `NewLogsServiceClient(cc grpc.ClientConnInterface)`; `LogsServiceServer` interface + `UnimplementedLogsServiceServer` + `RegisterLogsServiceServer(s grpc.ServiceRegistrar, srv LogsServiceServer)` (`:50`/`:58`/`:73`); `ExportLogsServiceRequest{ ResourceLogs []*logspb.ResourceLogs }` (field 1); `ExportLogsServiceResponse`.
- `logs/v1` (`logspb`): `LogRecord{ TimeUnixNano uint64 (fixed64, field 1); ObservedTimeUnixNano uint64 (field 11) }` — 45.1 sets ONLY `TimeUnixNano`; `ResourceLogs{ Resource *resourcepb.Resource (field 1); ScopeLogs []*ScopeLogs (field 2) }`; `ScopeLogs{ Scope *commonpb.InstrumentationScope (field 1, LEFT NIL); LogRecords []*LogRecord (field 2) }`.
- `common/v1` (`commonpb`): `KeyValue{ Key string; Value *AnyValue }`; `AnyValue{ Value isAnyValue_Value }` + `AnyValue_StringValue{ StringValue string }` (the string-typed arm).
- `resource/v1` (`resourcepb`): `Resource{ Attributes []*commonpb.KeyValue (field 1) }`.

Core `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (`corev3`, already used by the ALS path):
- `Node.GetId() string`, `.GetCluster() string`, `.GetLocality() *Locality`; `Locality.GetZone() string`. (All nil-safe — empty when the bootstrap has no `node` / no `locality`.)
- `GrpcService.GetEnvoyGrpc()`/`.GetGoogleGrpc()`; `GrpcService_EnvoyGrpc.GetClusterName()`; `ApiVersion_V2 = 1`.

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path (this plan lives in the worktree); the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0084'`, NEVER bare `'0084'` (which matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the sink's writer goroutine is a background mutator; after it lands run the FULL `internal/accesslog` package `-race`, NOT a `-run` subset.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing`): the `0084` differential asserts the aggregated per-record PAYLOAD, NOT `Export`-call count / per-call batch sizes / connection count (which legitimately vary side-to-side).

---

## D-question resolutions (the SPEC §12 D-OTLP-* PLAN pins — settled here)

**D-OTLP-CONFIG-DEFER → FACTOR a shared `parseCommonGrpcAccessLogConfig` helper consumed by BOTH parse arms (DRY); preserve `parseGrpcAccessLog`'s byte-stable error wording via a `sinkLabel` param.**
- The V2-reject / google_grpc-vs-envoy_grpc-reject / empty-cluster-reject / `buffer_size_bytes`-default-16384 / `buffer_flush_interval`-default-1s logic in `parseGrpcAccessLog` (`bootstrap.go:377`–`:403`) reads a `*grpcalv3.CommonGrpcAccessLogConfig`. `OpenTelemetryAccessLogConfig.GetCommonConfig()` returns the IDENTICAL type. ⇒ Extract `parseCommonGrpcAccessLogConfig(common *grpcalv3.CommonGrpcAccessLogConfig, idx int, sinkLabel string) (clusterName, logName string, bufBytes uint32, flush time.Duration, err error)`. The reject-error format strings interpolate `sinkLabel` (`"grpc ALS"` for the existing arm, `"OTLP access log"` for the OTLP arm) so the existing 44.x golden wording is byte-IDENTICAL (`sinkLabel="grpc ALS"` reproduces the exact existing strings) — the existing `bootstrap_test.go` reject assertions are the GREEN guard that the refactor preserved behavior. Factoring is DRY-preferred (SPEC §3.1 reuse note); the duplicate-the-~25-lines alternative is REJECTED (it would let the two arms drift).
- **Keep PARALLEL SLICES**: a NEW `Bootstrap.OTLPConfigs []OTLPConfig` separate from `ALSConfigs`/`AccessLogConfigs`. Per-sink-type ordering is NOT load-bearing (every `Record` is `Submit`ted to every sink), so a unify-into-one-list refactor would churn the byte-stable file/gRPC paths for zero gain.

**D-OTLP-STATS-PREFIX → `stat_prefix` PARSE-ACCEPT-but-INERT at 45.1 (fixed names).**
- 45.1 emits the two FIXED names `access_logs.open_telemetry_access_log.{logs_written,logs_dropped}` regardless of `stat_prefix`. Honoring the `…open_telemetry_access_log.<stat_prefix>.…` infix (which needs the `reference_dynamic_stat_name_charset_guard` `IsValidName` guard + per-`stat_prefix` registration) is a documented follow-on. Rationale: the `0084` differential sets no `stat_prefix` ⇒ behavior matches; honoring it now buys ZERO differential coverage at the cost of the dynamic-stat-name machinery. A config carrying `stat_prefix` boots + logs under the fixed names — a documented envoy-go-strict departure, NOT a behavior bug. STATIC names ⇒ NO `IsValidName` guard at 45.1.

**D-OTLP-RECEIVER-WIRING → a driver-owned `test/helpers/otlplogs` accumulator; NO new BackendKind.**
- The receiver is a NEW shared test-only package `test/helpers/otlplogs` (the `test/helpers/accessloggrpc` precedent), started BY THE DRIVER (lazily-allocated port baked into BOTH YAMLs), NOT a runner `BackendKind` (`reference_differential_grpc_receiver_driver_owned` — a gRPC service the proxy DIALS is driver-owned; the 44.1 D-ALS-RECEIVER-WIRING precedent that REVISED the anticipated 39 to 38). The receiver's `Export` accumulates `LogRecord`s across `Export` calls + the `ResourceLogs`/`ScopeLogs` nesting AND records each `ResourceLogs.Resource.Attributes` set. The OTLP cluster is h2c (`HttpProtocolOptions{ explicit_http_config: { http2_protocol_options: {} } }`, no TLS — SPEC D-OTLP-RECEIVER). **CONSEQUENCE — BackendKind stays 38**; the `0084` data-plane backend REUSES `HTTPFixedBody = 4`.

**D-OTLP-NODE → the OTLP sink reads the FULL bootstrap node `bs.Proto.GetNode()` (Id/Cluster/Locality.Zone); the ALS minimal node is UNCHANGED.**
- The OTLP `Resource` labels need `zone_name` (← `node.locality.zone`), which the ALS path's minimal `&corev3.Node{Id, Cluster}` (`main.go:132`) lacks. Resolution: the OTLP wiring passes the FULL `bs.Proto.GetNode()` (carries Id/Cluster/Locality) into `NewOTLPAccessLogSink`; the sink reads `GetId()`/`GetCluster()`/`GetLocality().GetZone()` at flush (all nil-safe; `zone_name` empty when no locality). Do NOT widen the ALS minimal node (keep the 44.x byte-stable path untouched). Under the `0084` no-node/no-locality config all three node-derived labels are empty on BOTH sides (the differential asserts the 4 KEYS present + `log_name` value).

**D-OTLP-SPLIT-FINAL → one leg (re-checked, ~305 prod LoC, at the ADR-0045 soft gate).**
- Estimated prod LoC: bootstrap (the shared `parseCommonGrpcAccessLogConfig` helper extraction is net-neutral; `OTLPConfig` + `OTLPConfigs` + blank-import + TypeURL const + `parseOpenTelemetryAccessLog` arm = the V2/cluster/buffer reads via the helper + the 4 per-sibling rejects + `disable_builtin_labels` + `stat_prefix`-inert) ≈ **80**; `OTLPLogsClient` typed wrapper (struct + New + Export + Close) ≈ **40**; `OTLPAccessLogSink` (channel + Submit + writer goroutine + size/interval/close buffer + unary Export flush + retry-once + idempotent Close + ctx/cancel) ≈ **120**; the built-in `buildLogRecord` + `buildResource` (4 labels + disable gate) + `buildExportRequest` envelope ≈ **45**; stats registration ≈ **6**; main wiring (dialer hoist + per-`OTLPConfig` sink build + counters + node) ≈ **20**. Total ≈ **305 prod LoC** — right at the ADR-0045 soft gate (matching the SPEC §3.0 ≈260–320 anticipation). Ships as ONE leg (the SPEC chartered 45.1 as the core leg; the operator engine is the separately-chartered 45.2). No further split.

**D-OTLP-FUZZER → land the parse fuzzer at 45.1 (fuzzers 44 → 45).**
- Land a `FuzzParseOpenTelemetryAccessLogConfig` no-panic fuzzer over the new `parseOpenTelemetryAccessLog` arm (the natural new attack surface; the `FuzzParseHttpGrpcAccessLogConfig` 44.1 precedent). The actual `^func Fuzz` count is **44** (verified at PLAN time) and the documented running total is **44** (no drift to absorb) ⇒ the new fuzzer advances both to **45**. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 45 at the completion task (`reference_fuzzer_count_docs_drift`).

---

## File structure (decomposition locked here)

**Production (touched):**
- `go.mod` / `go.sum` — MODIFY: Task 2's `open_telemetry/v3` blank-import introduces `go.opentelemetry.io/proto/otlp v1.0.0` as an `// indirect` dep (`go mod tidy` adds it + reconciles the pre-existing unstaged `go.sum` lines); Task 4's first DIRECT otlp import (`collogspb`) promotes it `// indirect`→direct (`go mod tidy` again).
- `internal/bootstrap/bootstrap.go` — MODIFY: the open_telemetry/v3 blank-import; the `otlpAccessLogTypeURL` const; the `OTLPConfig` struct; `Bootstrap.OTLPConfigs` field; the `parseOpenTelemetryAccessLog` arm + the shared `parseCommonGrpcAccessLogConfig` helper (refactor `parseGrpcAccessLog` to use it).
- `internal/bootstrap/otlpconfig_fuzz_test.go` — CREATE: the parse fuzzer.
- `internal/grpcclient/grpcclient.go` — MODIFY: the `collogspb` import + the `OTLPLogsClient` type + `NewOTLPLogsClient` + `Export` + `Close`.
- `internal/accesslog/otlpmapping.go` — CREATE: `buildLogRecord` + `buildResource` + `buildExportRequest`.
- `internal/accesslog/otlpsink.go` — CREATE: `OTLPAccessLogSink` + the `otlpClient` seam interface + `NewOTLPAccessLogSink` + `newOTLPSinkWithCapacity`.
- `internal/accesslog/stats.go` — MODIFY: add `RegisterOTLPSinkCounters`.
- `cmd/envoy-go/main.go` — MODIFY: hoist the `grpcclient.New(cm)` dialer; the per-`OTLPConfig` sink build + counter registration + the full bootstrap node.

**Test (created):**
- `internal/bootstrap/bootstrap_test.go` — MODIFY: the OTLP parse-accept + STRICT-REJECT + accept-inert table tests + the shared-helper-preserves-grpc-wording guard (existing grpc reject tests stay green).
- `internal/grpcclient/grpcclient_test.go` — MODIFY: the `OTLPLogsClient` tests.
- `internal/accesslog/otlpmapping_test.go` — CREATE: the table-driven `buildLogRecord`/`buildResource`/`buildExportRequest` + `disable_builtin_labels` tests.
- `internal/accesslog/otlpsink_test.go` — CREATE: the sink channel/drop/buffer-flush/retry-once/Close + `-race` tests (against a fake `otlpClient`).
- `internal/accesslog/stats_test.go` — MODIFY: the registration test (surface 1189 → 1191).
- `test/helpers/otlplogs/otlplogs.go` (+ `_test.go`) — CREATE: the in-process OTLP `LogsService` receiver accumulator.
- `test/fixtures/0084-otlp-access-log/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0084` driver package.

**Docs (completion task):**
- `docs/envoy-go/DECISIONS.md` (ADR-0258 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md`.

---

## Task 1: Phase scaffolding — PROGRESS-45.1.md + baselines + the final ADR-0045 split re-check (D-OTLP-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md`

- [ ] **Step 1: Record the baseline counts**

Run and record the verbatim outputs in PROGRESS-45.1.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                       # expect 85 (tail 0083-grpc-access-log-headers). NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS (it skips the letter-suffixed 0007a/0007b) — use the glob form.
grep -rh '^func Fuzz' --include='*.go' . | wc -l                        # expect 44
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go        # expect = 38 (the BackendKind tail)
grep -c 'go.opentelemetry.io/proto/otlp' go.mod                          # expect 0 (NOT in go.mod yet — Task 2 introduces it `// indirect`, Task 4 promotes it direct)
```
Baseline: stat surface **1189** (H2 cluster; non-H2 **1185**) / fixtures **85** / fuzzers **44** / BackendKind **38** / DECISIONS tail **ADR-0257** (next-free **ADR-0258**).

- [ ] **Step 2: Write the PROGRESS-45.1.md scaffold** — a header (phase 45.1 IMPL, the SPEC-45.1 reference, the worktree branch), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1191** (+2 — `access_logs.open_telemetry_access_log.{logs_written,logs_dropped}`) / fixtures **86** (`0084-otlp-access-log`) / fuzzers **45** (`FuzzParseOpenTelemetryAccessLogConfig`) / BackendKind **38** (UNCHANGED — driver-owned receiver) / DECISIONS **ADR-0258** / **+1 go.mod module** (`go.opentelemetry.io/proto/otlp` transitive→direct).

- [ ] **Step 3: Record the D-OTLP-SPLIT-FINAL re-check** — note the ~305 prod-LoC estimate (the D-OTLP-SPLIT-FINAL breakdown above), confirm it sits at the ADR-0045 soft gate, and that 45.1 ships as ONE leg (the operator engine is the separately-chartered 45.2). (Bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md
git commit -m "phase 45.1 Task 1: PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-OTLP-SPLIT-FINAL)"
```

---

## Task 2: The `OTLPConfig` parse arm + the shared `parseCommonGrpcAccessLogConfig` helper + STRICT-REJECT (`internal/bootstrap`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (table-driven over bootstrap YAML strings via the existing `Load(strings.NewReader(yaml))` shape; model on the existing gRPC-ALS parse tests):
  - **accept-minimal**: an HCM `access_log[]` with `OpenTelemetryAccessLogConfig { common_config: { log_name: "otel", grpc_service: { envoy_grpc: { cluster_name: "otlp_cluster" } } } }` ⇒ `Load` succeeds, `len(bs.OTLPConfigs) == 1`, `bs.OTLPConfigs[0] == OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second, DisableBuiltinLabels: false}`.
  - **accept-empty-log_name**: `log_name` omitted ⇒ `LogName: ""` (valid).
  - **accept-disable_builtin_labels**: `disable_builtin_labels: true` ⇒ `DisableBuiltinLabels: true`.
  - **accept-buffer-fields**: `buffer_size_bytes: 8192` + `buffer_flush_interval: 2s` ⇒ `BufferSizeBytes: 8192`, `BufferFlushInterval: 2*time.Second`. **accept-buffer-zero**: `buffer_size_bytes: 0` (explicit) ⇒ `BufferSizeBytes: 0` (flush-every-entry, NOT coerced to 16384). **accept-flush-default**: `buffer_flush_interval` omitted ⇒ `time.Second`.
  - **accept-stat_prefix-inert**: `stat_prefix: myprefix` ⇒ boots, `len(bs.OTLPConfigs) == 1` (parse-accept-but-inert; the struct has no StatPrefix field — read-and-ignore).
  - **accept-transport-V3/AUTO**: `transport_api_version: V3` ⇒ boots; omitted (AUTO) ⇒ boots.
  - **reject-google_grpc**: `grpc_service: { google_grpc: { … } }` ⇒ `Load` errors, message names `google_grpc` + `OTLP access log` + `access_log[%d]`.
  - **reject-transport-V2**: `transport_api_version: V2` ⇒ errors (`OTLP access log` + `V2`).
  - **reject-empty-cluster**: `envoy_grpc: { cluster_name: "" }` ⇒ errors.
  - **reject-body**: a non-nil `body` ⇒ errors naming `body` (deferred to 45.2). **reject-attributes**: a non-empty `attributes` ⇒ errors naming `attributes`. **reject-resource_attributes**: a non-empty `resource_attributes` ⇒ errors naming `resource_attributes`. **reject-formatters**: a non-empty `formatters` ⇒ errors naming `formatters`.
  - **coexist**: a file + a gRPC + an OTLP `access_log` in the same HCM ⇒ `len(bs.AccessLogConfigs)==1 && len(bs.ALSConfigs)==1 && len(bs.OTLPConfigs)==1` (parallel slices).
  - **grpc-wording-unchanged (the refactor guard)**: keep/confirm the EXISTING gRPC-ALS reject tests (`reject-google_grpc`, `reject-transport-V2`, `reject-empty-cluster` for `HttpGrpcAccessLogConfig`) assert the SAME byte-stable messages they assert today (`"grpc ALS …"`) — proving the shared-helper extraction preserved behavior.

- [ ] **Step 2: Run to verify the NEW tests fail (and the existing grpc tests still pass)**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: FAIL on the new OTLP cases (`OTLPConfigs` undefined / OTLP configs silently ignored). The existing gRPC-ALS cases still PASS (no refactor yet).

- [ ] **Step 3: Implement** in `bootstrap.go`:

Add the blank-import alongside the existing access-logger imports (ADR-0016):
```go
// Phase 45.1 registers the OTLP access-logger extension proto so protojson
// round-trips bootstraps carrying HCM access_log[] OpenTelemetryAccessLogConfig
// entries (ADR-0258). Its body/attributes field types transitively pull
// go.opentelemetry.io/proto/otlp common/v1.
_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
```
Add a typed import for the parse (the config proto):
```go
otlpalv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
```
Add the TypeURL const alongside `tcpGrpcAccessLogTypeURL` (`:169`):
```go
// otlpAccessLogTypeURL is the TypeURL for the OpenTelemetry (OTLP) access logger
// (envoy.access_loggers.open_telemetry). Lifted from the ADR-0041 silent-ignore
// set at phase 45.1 (ADR-0258).
otlpAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.open_telemetry.v3.OpenTelemetryAccessLogConfig"
```
Add the struct + the `Bootstrap.OTLPConfigs` field (parallel to `ALSConfig`/`ALSConfigs`):
```go
// OTLPConfig is the parsed OpenTelemetry (OTLP) access-log sink config from one
// HCM access_log[] OpenTelemetryAccessLogConfig entry (ADR-0258). The sink is
// built in cmd/envoy-go/main.go after Load returns. body/attributes/
// resource_attributes/formatters are STRICT-REJECTED at 45.1 (45.2 / always-out-
// of-scope); stat_prefix is PARSE-ACCEPT-but-INERT (the fixed stat names regardless).
type OTLPConfig struct {
    ClusterName          string        // common_config.grpc_service.envoy_grpc.cluster_name
    LogName              string        // common_config.log_name (empty is valid; → the log_name Resource label)
    BufferSizeBytes      uint32        // common_config.buffer_size_bytes (default 16384 when wrapper absent; explicit 0 ⇒ flush-every-entry)
    BufferFlushInterval  time.Duration // common_config.buffer_flush_interval (default 1s when absent/zero — a NewTicker(<=0) panic-guard)
    DisableBuiltinLabels bool          // disable_builtin_labels (AMEND-OTLP-DISABLE-BUILTIN; drops all 4 Resource labels wholesale)
}
```
Refactor `parseGrpcAccessLog` (`:371`) to call a NEW shared helper, preserving the existing wording via `sinkLabel="grpc ALS"`:
```go
// parseCommonGrpcAccessLogConfig reads the CommonGrpcAccessLogConfig shared by
// HttpGrpcAccessLogConfig (phase 44) and OpenTelemetryAccessLogConfig (phase 45):
// it STRICT-REJECTS transport_api_version V2, a google_grpc (non-envoy_grpc)
// grpc_service, and an empty cluster_name (ADR-0080), and applies the
// buffer_size_bytes default (16384 when the wrapper is absent; explicit 0 honored
// as flush-every-entry) + the buffer_flush_interval default (1s when absent/zero —
// the time.NewTicker(<=0) panic-guard). sinkLabel ("grpc ALS" / "OTLP access log")
// is interpolated into the reject messages so each arm keeps its own wording.
func parseCommonGrpcAccessLogConfig(common *grpcalv3.CommonGrpcAccessLogConfig, idx int, sinkLabel string) (clusterName, logName string, bufBytes uint32, flush time.Duration, err error) {
    if v := common.GetTransportApiVersion(); v == corev3.ApiVersion_V2 { //nolint:staticcheck // SA1019: this arm EXISTS to PARSE-REJECT the deprecated V2 transport (ADR-0080; envoy-go is V3-only).
        return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s transport_api_version V2 is not supported (envoy-go is V3-only)", idx, sinkLabel)
    }
    eg := common.GetGrpcService().GetEnvoyGrpc()
    if eg == nil {
        return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s requires grpc_service.envoy_grpc (google_grpc is not supported)", idx, sinkLabel)
    }
    if eg.GetClusterName() == "" {
        return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s grpc_service.envoy_grpc.cluster_name is required (must be non-empty)", idx, sinkLabel)
    }
    bufBytes = alsDefaultBufferSizeBytes
    if w := common.GetBufferSizeBytes(); w != nil {
        bufBytes = w.GetValue()
    }
    flush = common.GetBufferFlushInterval().AsDuration()
    if flush <= 0 {
        flush = time.Second
    }
    return eg.GetClusterName(), common.GetLogName(), bufBytes, flush, nil
}
```
> **Wording-preservation check:** with `sinkLabel="grpc ALS"` the three reject strings are byte-IDENTICAL to the current `parseGrpcAccessLog` strings (`:378`/`:383`/`:386`). Confirm against the existing test assertions — if any existing test pins a slightly different string, MATCH the existing string (do not "improve" it).

Rewrite `parseGrpcAccessLog`'s body to delegate (keeping the header reads + the append):
```go
clusterName, logName, bufBytes, flush, err := parseCommonGrpcAccessLogConfig(cfg.GetCommonConfig(), idx, "grpc ALS")
if err != nil {
    return err
}
reqHdrs := lowerAll(cfg.GetAdditionalRequestHeadersToLog())
respHdrs := lowerAll(cfg.GetAdditionalResponseHeadersToLog())
result.ALSConfigs = append(result.ALSConfigs, ALSConfig{
    ClusterName: clusterName, LogName: logName, BufferSizeBytes: bufBytes,
    BufferFlushInterval: flush, AdditionalRequestHeaders: reqHdrs, AdditionalResponseHeaders: respHdrs,
})
return nil
```
Add the OTLP dispatch arm in `parseOneAccessLog` (after the `tcpGrpcAccessLogTypeURL` reject `:334`, before the silent-ignore fallthrough; UPDATE the fallthrough comment to drop `open_telemetry` from the named-silently-ignored list):
```go
if tc.GetTypeUrl() == otlpAccessLogTypeURL {
    return parseOpenTelemetryAccessLog(tc, idx, result)
}
```
And the OTLP parse helper:
```go
// parseOpenTelemetryAccessLog processes a single OpenTelemetryAccessLogConfig
// access_log entry and appends an OTLPConfig to result.OTLPConfigs (ADR-0258).
// It STRICT-REJECTS (ADR-0080 / reference_strict_reject_sibling_typeurl_gap): the
// common_config V2/google_grpc/empty-cluster (via the shared helper); a non-nil
// body / non-empty attributes / non-empty resource_attributes (deferred to 45.2);
// a non-empty formatters (always out of scope). disable_builtin_labels is CONSUMED;
// stat_prefix is read-and-ignored (PARSE-ACCEPT-but-INERT — the fixed stat names).
func parseOpenTelemetryAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error {
    cfg := &otlpalv3.OpenTelemetryAccessLogConfig{}
    if err := proto.Unmarshal(tc.GetValue(), cfg); err != nil {
        return fmt.Errorf("bootstrap: access_log[%d] otlp unmarshal: %w", idx, err)
    }
    // Per-sibling STRICT-REJECTS (each its own arm — reference_strict_reject_sibling_typeurl_gap).
    if cfg.GetBody() != nil {
        return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log body is not supported at phase 45.1 (operator templating is phase 45.2)", idx)
    }
    if len(cfg.GetAttributes().GetValues()) > 0 {
        return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log attributes is not supported at phase 45.1 (operator templating is phase 45.2)", idx)
    }
    if len(cfg.GetResourceAttributes().GetValues()) > 0 {
        return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log resource_attributes is not supported at phase 45.1 (operator templating is phase 45.2)", idx)
    }
    if len(cfg.GetFormatters()) > 0 {
        return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log formatters (custom formatter extensions) are not supported", idx)
    }
    clusterName, logName, bufBytes, flush, err := parseCommonGrpcAccessLogConfig(cfg.GetCommonConfig(), idx, "OTLP access log")
    if err != nil {
        return err
    }
    // stat_prefix (field 6) is read-and-ignored (PARSE-ACCEPT-but-INERT, AMEND-OTLP-STATS):
    // 45.1 emits the fixed access_logs.open_telemetry_access_log.* names regardless.
    result.OTLPConfigs = append(result.OTLPConfigs, OTLPConfig{
        ClusterName: clusterName, LogName: logName, BufferSizeBytes: bufBytes,
        BufferFlushInterval: flush, DisableBuiltinLabels: cfg.GetDisableBuiltinLabels(),
    })
    return nil
}
```
(Confirm `anypb`/`proto`/`corev3`/`grpcalv3` are already imported — they are, from the 44.x ALS arm. `KeyValueList.GetValues()` is the otlp common/v1 accessor for the `attributes`/`resource_attributes` repeated field — verify the exact accessor name at IMPL; an empty/nil `KeyValueList` ⇒ `len(...) == 0` ⇒ accept.)

- [ ] **Step 4: Reconcile go.mod (the blank-import's FIRST go.mod touch) + run the tests**

The `open_telemetry/v3` blank-import pulls `go.opentelemetry.io/proto/otlp` into the build graph (its config-proto field types import otlp `common/v1`), and go.mod has ZERO otlp entries today — so a plain `go build`/`go test` FAILS with `updates to go.mod needed, disabled by -mod=readonly` until go.mod is reconciled. Run `go mod tidy` FIRST:
```bash
go mod tidy
grep 'go.opentelemetry.io/proto/otlp' go.mod          # expect it now present, marked `// indirect` (no DIRECT otlp import yet — Task 4 promotes it)
go mod tidy -diff                                      # expect EMPTY (clean)
go test ./internal/bootstrap/ -run TestLoad -count=1   # expect PASS (new OTLP cases + the unchanged gRPC-ALS reject-wording cases)
```
Expected: `go mod tidy` adds `go.opentelemetry.io/proto/otlp v1.0.0 // indirect` + retains/reconciles the pre-existing unstaged `go.sum` otlp lines; the tests PASS.

- [ ] **Step 5: Per-task gates + commit** (stage go.mod + go.sum — the commit MUST build from a fresh checkout)
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/ go.mod go.sum
git commit -m "phase 45.1 Task 2: parse OpenTelemetryAccessLogConfig → OTLPConfig + shared parseCommonGrpcAccessLogConfig helper + STRICT-REJECT body/attributes/resource_attributes/formatters (ADR-0080) — disable_builtin_labels CONSUMED, stat_prefix INERT; otlp enters go.mod (// indirect)"
```

---

## Task 3: The `OpenTelemetryAccessLogConfig` parse fuzzer (D-OTLP-FUZZER; fuzzers 44 → 45) [fuzz]

**Files:**
- Create: `internal/bootstrap/otlpconfig_fuzz_test.go`

- [ ] **Step 1: Write the fuzzer** — a no-panic fuzzer over `parseOpenTelemetryAccessLog`. The invariant: the parse arm NEVER panics (it returns a value or a `bootstrap:`-prefixed error) for any input. Model on `FuzzParseHttpGrpcAccessLogConfig` (find it: `grep -rln 'FuzzParseHttpGrpcAccessLogConfig' internal/bootstrap/`); reuse its seed-corpus + harness shape.
```go
func FuzzParseOpenTelemetryAccessLogConfig(f *testing.F) {
    f.Add([]byte{})                          // empty
    f.Add([]byte("\x0a\x00"))                // a truncated common_config
    f.Fuzz(func(t *testing.T, data []byte) {
        any := &anypb.Any{TypeUrl: otlpAccessLogTypeURL, Value: data}
        result := &Bootstrap{}
        _ = parseOpenTelemetryAccessLog(any, 0, result) // must not panic
    })
}
```

- [ ] **Step 2: Run the fuzzer briefly**

Run: `go test ./internal/bootstrap/ -run 'FuzzParseOpenTelemetryAccessLogConfig' -count=1` then `go test ./internal/bootstrap/ -fuzz 'FuzzParseOpenTelemetryAccessLogConfig' -fuzztime 20s`
Expected: PASS / no crashers.

- [ ] **Step 3: Confirm the count advanced**

Run: `grep -rh '^func Fuzz' --include='*.go' . | wc -l`
Expected: **45** (was 44). Record in PROGRESS-45.1.md.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/otlpconfig_fuzz_test.go
git commit -m "phase 45.1 Task 3: FuzzParseOpenTelemetryAccessLogConfig (no-panic); fuzzers 44 → 45 (D-OTLP-FUZZER)"
```

---

## Task 4: The `OTLPLogsClient` UNARY typed wrapper (`internal/grpcclient`) + the go.mod promotion [TDD]

**Files:**
- Modify: `internal/grpcclient/grpcclient.go`
- Modify: `go.mod` / `go.sum` (the `// indirect`→direct promotion — this is the FIRST direct otlp import; Task 2 already introduced otlp as `// indirect`)
- Test: `internal/grpcclient/grpcclient_test.go`

- [ ] **Step 1: Write the failing tests** (mirror the existing `ALSClient` tests — find them in `grpcclient_test.go`):
  - **nil-dialer**: `NewOTLPLogsClient(nil, "c")` ⇒ `(nil, err)` naming the cluster.
  - **unknown-cluster**: a `Dialer` over a manager with no `c` ⇒ `NewOTLPLogsClient(d, "c")` errors `unknown cluster` (the `DialContext` gate).
  - **non-H2-cluster**: a cluster without `http2_protocol_options{}` ⇒ errors `HTTP/2 framing` (the `UseH2()` gate). (Reuse the test-cluster-manager builders the `ALSClient` tests use.)
  - **Close-idempotent**: against a valid H2 cluster, `c.Close()` twice returns the same (nil) error — no panic.
  - **Export-roundtrips**: against an in-process `otlplogs` receiver (this can defer to Task 9's helper if ordering requires — otherwise stand up a bare `grpc.NewServer()` + `RegisterLogsServiceServer` of a no-op `UnimplementedLogsServiceServer`-embedding stub in the test), `c.Export(ctx, &collogspb.ExportLogsServiceRequest{})` returns a non-nil response + nil error.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/grpcclient/ -run TestOTLP -count=1`
Expected: FAIL (`OTLPLogsClient` / `NewOTLPLogsClient` undefined).

- [ ] **Step 3: Implement** in `grpcclient.go` (add `collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"` to the imports — the FIRST direct otlp import):
```go
// ----------------------------------------------------------------------------
// OTLPLogsClient — the typed LogsService/Export UNARY wrapper (ADR-0258).
// ----------------------------------------------------------------------------

// OTLPLogsClient wraps a *grpc.ClientConn with the typed
// go.opentelemetry.io/proto/otlp collector LogsServiceClient stub. One
// *OTLPLogsClient per OTLP access-log sink (cluster_name), owned by the
// OTLPAccessLogSink and Close()d at sink close. The ALSClient precedent
// (ADR-0255) but UNARY — Export is a plain unary RPC (no stream lifecycle).
type OTLPLogsClient struct {
    conn   *grpc.ClientConn
    stub   collogspb.LogsServiceClient
    target string // cluster_name — for logs/errors

    closeOnce sync.Once
    closeErr  error
}

// NewOTLPLogsClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed OTLPLogsClient. On dial error returns
// (nil, err) verbatim (already cluster-named via DialContext's wrapping). The
// NewALSClient shape (no per-call timeout — the sink bounds Export via its ctx).
func NewOTLPLogsClient(d *Dialer, clusterName string) (*OTLPLogsClient, error) {
    if d == nil {
        return nil, fmt.Errorf("grpcclient: new OTLP logs client %q: dialer is nil", clusterName)
    }
    conn, err := d.DialContext(context.Background(), clusterName)
    if err != nil {
        return nil, err
    }
    return &OTLPLogsClient{
        conn:   conn,
        stub:   collogspb.NewLogsServiceClient(conn),
        target: clusterName,
    }, nil
}

// Export sends one ExportLogsServiceRequest over the unary LogsService/Export
// RPC. The sink's writer goroutine bounds ctx; on error the sink retries once
// (the *grpc.ClientConn self-heals — no stream to re-open).
func (c *OTLPLogsClient) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
    if c == nil || c.stub == nil {
        return nil, errors.New("grpcclient: Export: nil OTLPLogsClient / stub")
    }
    return c.stub.Export(ctx, req)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (sync.Once), the
// ALSClient.Close shape.
func (c *OTLPLogsClient) Close() error {
    if c == nil {
        return nil
    }
    c.closeOnce.Do(func() {
        if c.conn != nil {
            c.closeErr = c.conn.Close()
        }
    })
    return c.closeErr
}
```

- [ ] **Step 4: Promote the go.mod module `// indirect`→direct + run the tests**

The new `collogspb` import is the FIRST DIRECT otlp import (Task 2 introduced otlp as `// indirect`), so `go mod tidy` now moves it OUT of the `// indirect` annotation into the direct require block:
```bash
go mod tidy
grep 'go.opentelemetry.io/proto/otlp' go.mod          # expect it now in the DIRECT require block (NO `// indirect` suffix)
go mod tidy -diff                                      # expect EMPTY (clean)
go test ./internal/grpcclient/ -run TestOTLP -count=1  # expect PASS
```
Expected: `go mod tidy` promotes `go.opentelemetry.io/proto/otlp v1.0.0` to the direct `require` block; the tests PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/grpcclient/ && golangci-lint run ./internal/grpcclient/... && go vet ./internal/grpcclient/... && go build ./...
git add internal/grpcclient/ go.mod go.sum
git commit -m "phase 45.1 Task 4: OTLPLogsClient UNARY typed wrapper over the grpcclient.Dialer (the ALSClient precedent, ADR-0258) + promote go.opentelemetry.io/proto/otlp // indirect→direct"
```

---

## Task 5: The built-in `Record → LogRecord` mapping + the 4-label `Resource` + the Export envelope (`internal/accesslog/otlpmapping.go`) [TDD, table-driven]

**Files:**
- Create: `internal/accesslog/otlpmapping.go`
- Test: `internal/accesslog/otlpmapping_test.go`

These are PURE functions — no gRPC, no goroutine — the cleanest unit to TDD before the sink composes them.

- [ ] **Step 1: Write the failing table tests** in `otlpmapping_test.go`:
  - `buildLogRecord(rec)` over a `*Record{StartTime: someTime}` ⇒ `GetTimeUnixNano() == uint64(someTime.UnixNano())`; ALL of `GetObservedTimeUnixNano()`/`GetSeverityNumber()`/`GetSeverityText()`/`GetBody()`/`GetAttributes()` are zero/nil (the LEAN built-in record — AMEND-OTLP-BUILTINS/SEVERITY).
  - `buildResource(node, logName, disableBuiltinLabels=false)` over a node `{Id: "n", Cluster: "c", Locality{Zone: "z"}}` + `logName="L"` ⇒ `GetAttributes()` has exactly 4 KeyValues in order `[log_name=L, zone_name=z, cluster_name=c, node_name=n]` (assert key + `GetValue().GetStringValue()`).
  - `buildResource` with an EMPTY node + empty logName ⇒ still 4 keys `{log_name, zone_name, cluster_name, node_name}`, each `stringValue == ""` (always-all-4 — AMEND-OTLP-BUILTINS).
  - `buildResource(..., disableBuiltinLabels=true)` ⇒ `GetAttributes()` is EMPTY (nil/len 0) — the all-or-nothing drop (AMEND-OTLP-DISABLE-BUILTIN).
  - `buildExportRequest(batch, node, logName, disableBuiltinLabels)` over a 2-record batch ⇒ exactly one `ResourceLogs`; its `Resource` == `buildResource(...)`; exactly one `ScopeLogs` with `GetScope() == nil` (absent — AMEND-OTLP-EXPORT-SHAPE) and `GetLogRecords()` == the 2 records in order.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestOTLPMapping -count=1`
Expected: FAIL (functions undefined).

- [ ] **Step 3: Implement** `otlpmapping.go`. Imports: `collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"`, `logspb "go.opentelemetry.io/proto/otlp/logs/v1"`, `commonpb "go.opentelemetry.io/proto/otlp/common/v1"`, `resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"`, `corev3 "…/config/core/v3"`. Sketch:
```go
// buildLogRecord maps a Record into the LEAN built-in OTLP LogRecord: ONLY
// time_unix_nano is set (AMEND-OTLP-BUILTINS) — no observed_time, severity, body,
// or LogRecord.attributes (those are 45.2 operator templating). The time VALUE is
// non-deterministic; only PRESENCE is asserted cross-side.
func buildLogRecord(rec *Record) *logspb.LogRecord {
    return &logspb.LogRecord{
        TimeUnixNano: uint64(rec.StartTime.UnixNano()),
    }
}

// builtinLabel keys, always emitted in this order (AMEND-OTLP-BUILTINS).
func buildResource(node *corev3.Node, logName string, disableBuiltinLabels bool) *resourcepb.Resource {
    if disableBuiltinLabels {
        return &resourcepb.Resource{} // empty Resource — drops all 4 wholesale (AMEND-OTLP-DISABLE-BUILTIN)
    }
    kv := func(k, v string) *commonpb.KeyValue {
        return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
    }
    return &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
        kv("log_name", logName),
        kv("zone_name", node.GetLocality().GetZone()),
        kv("cluster_name", node.GetCluster()),
        kv("node_name", node.GetId()),
    }}
}

// buildExportRequest wraps a batch of LogRecords into one ExportLogsServiceRequest:
// one ResourceLogs (Resource = the built-in labels) → one ScopeLogs (Scope ABSENT)
// → the batch (AMEND-OTLP-EXPORT-SHAPE).
func buildExportRequest(batch []*logspb.LogRecord, node *corev3.Node, logName string, disableBuiltinLabels bool) *collogspb.ExportLogsServiceRequest {
    return &collogspb.ExportLogsServiceRequest{
        ResourceLogs: []*logspb.ResourceLogs{{
            Resource:  buildResource(node, logName, disableBuiltinLabels),
            ScopeLogs: []*logspb.ScopeLogs{{LogRecords: batch}},
        }},
    }
}
```
(`node.GetLocality().GetZone()` / `GetCluster()` / `GetId()` are all nil-safe — a nil `node` yields empty strings, matching the reference's empty labels.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/accesslog/ -run TestOTLPMapping -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpmapping.go internal/accesslog/otlpmapping_test.go
git commit -m "phase 45.1 Task 5: built-in Record→LogRecord (time_unix_nano only) + 4-label Resource + disable_builtin_labels + Export envelope (AMEND-OTLP-BUILTINS/EXPORT-SHAPE)"
```

---

## Task 6: The `OTLPAccessLogSink` (`internal/accesslog/otlpsink.go`) [TDD, `-race`]

**Files:**
- Create: `internal/accesslog/otlpsink.go`
- Test: `internal/accesslog/otlpsink_test.go`

The sink MIRRORS `GrpcAccessLogSink`'s bounded-channel + writer-goroutine + 44.2 size/interval/close BUFFER + idempotent-Close shape, but flushes via the UNARY `Export` of a `LogRecord` batch (no stream / no identifier / no `CloseAndRecv`). To unit-test WITHOUT a real gRPC server the sink depends on a minimal `otlpClient` interface (satisfied by `*grpcclient.OTLPLogsClient`); tests inject a fake.

- [ ] **Step 1: Write the failing tests** in `otlpsink_test.go` against a fake `otlpClient` (a struct recording every `Export`ed `*ExportLogsServiceRequest` + a configurable `Export`-error to force the retry path; the fake's `Export` takes a DEFENSIVE COPY of `req.ResourceLogs` / the LogRecords since the sink reuses its buffer — the 44.2 fake precedent):
  - **submit-exports-record**: a `bufferSizeBytes=0` sink (flush-every-entry); `Submit(rec)` ⇒ the fake receives ONE `Export` whose single `ResourceLogs.ScopeLogs[0].LogRecords` has one record with `TimeUnixNano == uint64(rec.StartTime.UnixNano())`; `logsWritten == 1` after drain.
  - **builtin-labels**: the exported `ResourceLogs[0].Resource.Attributes` carry the 4 keys with `log_name == <configured>` (the node-derived three from the test node).
  - **disable-builtin-labels**: a sink built with `disableBuiltinLabels=true` ⇒ the exported `Resource.Attributes` is EMPTY; `TimeUnixNano` still present.
  - **batch-on-size**: a sink with a `bufferSizeBytes` large enough to hold 3 records; `Submit` 3 records then `Close` ⇒ the fake received ONE `Export` carrying all 3 records (the close-drain flush — AMEND-OTLP-EXPORT-SHAPE); `logsWritten == 3`.
  - **drop-newest**: a capacity-1 sink (the `newOTLPSinkWithCapacity` test constructor) with a blocked writer ⇒ submitting past capacity increments `logsDropped`, never blocks `Submit`.
  - **retry-once-on-export-error**: the fake returns an error on the first `Export`, success after ⇒ the batch is re-`Export`ed and lands; `logsWritten` counts the eventually-sent batch. A SECOND consecutive failure ⇒ the batch is dropped (logged-not-counted): `logsWritten` unchanged, no panic.
  - **close-idempotent**: `Close()` twice ⇒ no panic, the channel drains, `OTLPLogsClient.Close()` called once.
  - **non-Record-ignored**: `Submit("garbage")` ⇒ no panic, no export, no `logsWritten`.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestOTLPSink -count=1`
Expected: FAIL (`OTLPAccessLogSink` / `NewOTLPAccessLogSink` undefined).

- [ ] **Step 3: Implement** `otlpsink.go`. Define the seam + the sink (mirror `grpcsink.go` but unary):
```go
// otlpClient is the minimal sink-facing seam over *grpcclient.OTLPLogsClient
// (test-fakeable). *grpcclient.OTLPLogsClient satisfies it structurally, so the
// sink need not import grpcclient (no import cycle; main wiring passes the
// concrete client).
type otlpClient interface {
    Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error)
    Close() error
}

// OTLPAccessLogSink is the project's THIRD accesslog.Sink: it BATCHES built-in
// OTLP LogRecords (time_unix_nano + the 4 Resource labels) and Exports a batch as
// one ExportLogsServiceRequest over the UNARY LogsService/Export on the FIRST of
// three triggers (the reused 44.2 machinery): the accumulated serialized bytes
// reaching bufferSizeBytes (0 ⇒ flush-every-entry), the bufferFlushInterval timer,
// or Close draining the pending buffer. It mirrors GrpcAccessLogSink's bounded-
// channel + writer-goroutine + idempotent-Close shape but is UNARY (no stream / no
// identifier / no CloseAndRecv). On a full channel the record is dropped (drop-
// newest), logsDropped Inc'd. On an Export error the batch is retried ONCE (the
// ClientConn self-heals); a second failure drops the batch logged-not-counted
// (memory stays bounded). logsWritten counts ENTRIES (batch-invariant). (ADR-0258)
type OTLPAccessLogSink struct {
    ch                   chan any
    client               otlpClient
    logName              string
    node                 *corev3.Node
    disableBuiltinLabels bool
    logsWritten          *stats.Counter
    logsDropped          *stats.Counter
    done                 chan struct{}
    closeOnce            sync.Once
    closeErr             error
    lastDropLog          atomic.Int64
    bufferSizeBytes      int
    bufferFlushInterval  time.Duration
    ctx                  context.Context
    cancel               context.CancelFunc
}

func NewOTLPAccessLogSink(client otlpClient, logName string, node *corev3.Node, disableBuiltinLabels bool, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *OTLPAccessLogSink {
    return newOTLPSinkWithCapacity(client, logName, node, disableBuiltinLabels, written, dropped, bufferSizeBytes, bufferFlushInterval, defaultChannelCapacity)
}
// newOTLPSinkWithCapacity is the test-friendly capacity variant.
func newOTLPSinkWithCapacity(client otlpClient, logName string, node *corev3.Node, disableBuiltinLabels bool, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, capacity int) *OTLPAccessLogSink {
    s := &OTLPAccessLogSink{
        ch: make(chan any, capacity), client: client, logName: logName, node: node,
        disableBuiltinLabels: disableBuiltinLabels, logsWritten: written, logsDropped: dropped,
        bufferSizeBytes: bufferSizeBytes, bufferFlushInterval: bufferFlushInterval, done: make(chan struct{}),
    }
    s.ctx, s.cancel = context.WithCancel(context.Background())
    go s.run()
    return s
}
```
- **`Submit`** — the `grpcsink.go:121` drop-newest shape exactly (select-default → `logsDropped.Inc()` + the rate-limited diag; log_name in the message).
- **`Close`** — the `grpcsink.go:145` shape: `sync.Once`: `close(s.ch)`; `select { <-s.done | <-time.After(closeDrainGrace): s.cancel(); <-s.done }`; `s.cancel()` (idempotent); `s.closeErr = s.client.Close()`.
- **`run`** — the writer goroutine: `defer close(s.done)`; a `flush` closure + the size/timer/close select loop (the `grpcsink.go:166` shape but SIMPLER — no `stream`/`sentIdentifier`):
```go
func (s *OTLPAccessLogSink) run() {
    defer close(s.done)
    var buf []*logspb.LogRecord
    bufBytes := 0
    flush := func() {
        if len(buf) == 0 {
            return
        }
        req := buildExportRequest(buf, s.node, s.logName, s.disableBuiltinLabels)
        var err error
        for attempt := 0; attempt < 2; attempt++ {
            if _, err = s.client.Export(s.ctx, req); err == nil {
                s.logsWritten.Add(uint64(len(buf)))
                break
            }
            log.Printf("accesslog: OTLP export (log_name=%s, attempt=%d): %v", s.logName, attempt+1, err)
        }
        // On a second failure the batch is dropped (logged, not counted) — bounds
        // memory under a sustained outage (logs_dropped stays channel-full-only).
        buf = buf[:0]
        bufBytes = 0
    }
    ticker := time.NewTicker(s.bufferFlushInterval)
    defer ticker.Stop()
    for {
        select {
        case r, ok := <-s.ch:
            if !ok {
                flush() // drain the pending buffer on close
                return
            }
            rec, ok := r.(*Record)
            if !ok {
                log.Printf("accesslog: OTLP sink got non-*Record %T (log_name=%s); dropping", r, s.logName)
                continue
            }
            lr := buildLogRecord(rec)
            buf = append(buf, lr)
            bufBytes += proto.Size(lr)
            if bufBytes >= s.bufferSizeBytes { // SIZE trigger; 0 ⇒ every entry flushes
                flush()
            }
        case <-ticker.C:
            flush() // TIMER trigger (no-op if buf empty)
        }
    }
}
```
> **buf-reuse contract (the 44.2 note):** the real gRPC `Export` serializes the request synchronously before returning, so the buffer bytes are captured before `buf[:0]` reuse (zero extra allocation in production). The test fake records the request pointer, so it MUST take a defensive copy of the LogRecords in its `Export` (mirror the 44.2 grpcsink fake).

Imports: `context`, `log`, `sync`, `sync/atomic`, `time`, `proto`, `corev3`, `collogspb`, `logspb`, `internal/stats`.

- [ ] **Step 4: Run to verify they pass + the FULL-package race**

Run: `go test ./internal/accesslog/ -run TestOTLPSink -count=1`
Expected: PASS.
Then the FULL package `-race` (the writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`):
Run: `go test ./internal/accesslog/ -race -count=1`
Expected: PASS, no race.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpsink.go internal/accesslog/otlpsink_test.go
git commit -m "phase 45.1 Task 6: OTLPAccessLogSink — bounded channel + writer goroutine + size/interval/close buffer + unary Export flush + retry-once + idempotent Close (ADR-0258)"
```

---

## Task 7: The two `access_logs.open_telemetry_access_log.*` stat registrations + a registration test (1189 → 1191) [TDD]

**Files:**
- Modify: `internal/accesslog/stats.go`
- Test: `internal/accesslog/stats_test.go`

- [ ] **Step 1: Write the failing registration test** — `RegisterOTLPSinkCounters(reg)` returns two non-nil distinct `*stats.Counter`; the registry then carries `access_logs.open_telemetry_access_log.logs_written` + `access_logs.open_telemetry_access_log.logs_dropped`. Surface delta: registering them adds exactly **2** counters (assert the count DELTA, robust to the absolute 1191). STATIC names (no `IsValidName` guard — AMEND-OTLP-STATS). Model on the existing `RegisterGrpcSinkCounters` test.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/accesslog/ -run TestOTLPSinkCounters -count=1`
Expected: FAIL (`RegisterOTLPSinkCounters` undefined).

- [ ] **Step 3: Implement** in `stats.go` (the `RegisterGrpcSinkCounters:23` shape):
```go
// RegisterOTLPSinkCounters allocates the two process-global OTLP access-log sink
// counters (ADR-0258 / AMEND-OTLP-STATS). Registered once per process when ≥1
// OTLP sink is built. STATIC names (no IsValidName guard — not wire/config
// derived; stat_prefix honoring is deferred). The OTLP sink owns its own
// logs_dropped (NOT a reuse of server.accesslog_dropped or the gRPC-ALS counter).
func RegisterOTLPSinkCounters(reg *stats.Registry) (written, dropped *stats.Counter) {
    return reg.NewCounter("access_logs.open_telemetry_access_log.logs_written"),
        reg.NewCounter("access_logs.open_telemetry_access_log.logs_dropped")
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/accesslog/ -run TestOTLPSinkCounters -count=1`
Expected: PASS. Record the +2 surface delta (1189 → 1191) in PROGRESS-45.1.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go build ./...
git add internal/accesslog/stats.go internal/accesslog/stats_test.go
git commit -m "phase 45.1 Task 7: register access_logs.open_telemetry_access_log.{logs_written,logs_dropped} (+2 → 1191; AMEND-OTLP-STATS)"
```

---

## Task 8: Boot wiring (`cmd/envoy-go/main.go`) — dialer HOIST + per-`OTLPConfig` sink build + counters + full node

**Files:**
- Modify: `cmd/envoy-go/main.go`

main.go is not unit-tested in isolation (the differential is its behavioral proof); the gate here is build + boot-smoke, with Task 10's `0084` fixture as the real test.

- [ ] **Step 1: Implement** — HOIST the dialer out of the ALS-only `if` (`:129`–`:140`) so an OTLP-only boot constructs it, then add the OTLP sink-build block. Restructure to:
```go
// Phase 44.1 (ADR-0255) + phase 45.1 (ADR-0258): build the gRPC-ALS and OTLP
// access-log sinks. The shared grpcclient.Dialer (one Dialer serves both passes)
// is HOISTED to fire when EITHER family is configured (an OTLP-only boot must
// still build it). Each sink is appended to the same `sinks` slice BEFORE the
// defer-LIFO Close() below, which already covers them.
if len(bs.ALSConfigs) > 0 || len(bs.OTLPConfigs) > 0 {
    dialer := grpcclient.New(cm)
    if len(bs.ALSConfigs) > 0 {
        written, dropped := accesslog.RegisterGrpcSinkCounters(bs.Stats)
        alsNode := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
        for _, cfg := range bs.ALSConfigs {
            client, err := grpcclient.NewALSClient(dialer, cfg.ClusterName)
            if err != nil {
                log.Fatalf("accesslog: gRPC ALS client for cluster %q: %v", cfg.ClusterName, err)
            }
            sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, alsNode, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval, cfg.AdditionalRequestHeaders, cfg.AdditionalResponseHeaders))
        }
    }
    if len(bs.OTLPConfigs) > 0 {
        otlpWritten, otlpDropped := accesslog.RegisterOTLPSinkCounters(bs.Stats)
        // The OTLP Resource labels need node.locality.zone, so source the FULL
        // bootstrap node (Id/Cluster/Locality) — NOT the ALS minimal node (D-OTLP-NODE).
        otlpNode := bs.Proto.GetNode()
        for _, cfg := range bs.OTLPConfigs {
            client, err := grpcclient.NewOTLPLogsClient(dialer, cfg.ClusterName)
            if err != nil {
                log.Fatalf("accesslog: OTLP logs client for cluster %q: %v", cfg.ClusterName, err)
            }
            sinks = append(sinks, accesslog.NewOTLPAccessLogSink(client, cfg.LogName, otlpNode, cfg.DisableBuiltinLabels, otlpWritten, otlpDropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval))
        }
    }
}
```
(`corev3` + `grpcclient` already imported. `bs.Proto.GetNode()` is nil-safe — a nil node yields empty labels.)

- [ ] **Step 2: Build + boot-smoke**

Run: `go build ./... && echo BUILD_OK`
Then a manual boot-smoke against a hand-written bootstrap with an OTLP `access_log` pointing at a non-existent cluster ⇒ confirm the `log.Fatalf` fires (the Dialer's unknown-cluster gate at sink build). And against a valid H2 cluster (no ALS config — proves the HOIST) ⇒ boots clean.

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 45.1 Task 8: boot wiring — HOIST grpcclient.Dialer (OTLP-only boots) + per-OTLPConfig OTLPAccessLogSink build + counter registration + full bootstrap node (D-OTLP-NODE)"
```

---

## Task 9: The `test/helpers/otlplogs` receiver accumulator (D-OTLP-RECEIVER-WIRING) [TDD]

**Files:**
- Create: `test/helpers/otlplogs/otlplogs.go`, `test/helpers/otlplogs/otlplogs_test.go`

The in-process OTLP `LogsService` receiver — the `accessloggrpc` precedent, but a UNARY `Export` server accumulating `*LogRecord`s across calls + the `ResourceLogs`/`ScopeLogs` nesting AND recording each `ResourceLogs.Resource.Attributes` set, exposing a thread-safe poll surface.

- [ ] **Step 1: Write the failing test** — `New(t)`, dial it, `Export` two requests (the first one `ResourceLogs` with a `Resource{log_name=L,…}` + 1 `LogRecord`, the second with 2 records), then assert `Server.Records()` returns all 3 accumulated `*LogRecord`s (order-preserving), `Count() == 3`, and `ResourceAttributes()` (the most-recent or aggregated per-`ResourceLogs` `[]*KeyValue`) carries the `log_name` key. A second `Export` accumulates onto the same slices. `-race` (accumulation is concurrent).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/helpers/otlplogs/ -count=1`
Expected: FAIL (package/symbols undefined).

- [ ] **Step 3: Implement** — mirror `accessloggrpc.go` (embed `collogspb.UnimplementedLogsServiceServer`; `RegisterLogsServiceServer`; plain h2c `grpc.NewServer()`):
```go
type Server struct {
    collogspb.UnimplementedLogsServiceServer
    addr     string
    lis      net.Listener
    grpcSrv  *grpc.Server
    mu       sync.RWMutex
    records  []*logspb.LogRecord
    resAttrs [][]*commonpb.KeyValue // per-ResourceLogs Resource.attributes, in arrival order
    stopOnce sync.Once
}
func New(t testing.TB) *Server { /* newServer("127.0.0.1:0") + t.Cleanup(Stop) */ }
func NewAtAddr(addr string) (*Server, error) { return newServer(addr) }   // "0.0.0.0:port" for Docker reachability
func newServer(addr string) (*Server, error) {
    lis, err := net.Listen("tcp", addr); if err != nil { return nil, fmt.Errorf("listen %s: %w", addr, err) }
    s := &Server{addr: lis.Addr().String(), lis: lis, grpcSrv: grpc.NewServer()}
    collogspb.RegisterLogsServiceServer(s.grpcSrv, s)
    go func() { _ = s.grpcSrv.Serve(lis) }()
    return s, nil
}
// Export implements collogspb.LogsServiceServer — accumulate every LogRecord across
// the ResourceLogs/ScopeLogs nesting + record each Resource.attributes set.
func (s *Server) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
    s.mu.Lock()
    for _, rl := range req.GetResourceLogs() {
        s.resAttrs = append(s.resAttrs, rl.GetResource().GetAttributes())
        for _, sl := range rl.GetScopeLogs() {
            s.records = append(s.records, sl.GetLogRecords()...)
        }
    }
    s.mu.Unlock()
    return &collogspb.ExportLogsServiceResponse{}, nil
}
func (s *Server) Records() []*logspb.LogRecord { /* RLock defensive snapshot copy */ }
func (s *Server) Count() int { /* RLock len(records) — the converge poll */ }
func (s *Server) ResourceAttributes() [][]*commonpb.KeyValue { /* RLock defensive snapshot */ }
func (s *Server) Reset() { /* Lock; records=nil; resAttrs=nil */ }
func (s *Server) Addr() string { return s.addr }
func (s *Server) Stop()  { s.stopOnce.Do(s.grpcSrv.GracefulStop) }
func (s *Server) Close() { s.stopOnce.Do(s.grpcSrv.Stop) }   // immediate hard-stop for the driver
```
(Imports: `context`, `fmt`, `net`, `sync`, `testing`, `collogspb`, `logspb`, `commonpb`, `grpc`. `Records`/`ResourceAttributes`/`Count`/`Reset`/`Stop`/`Close` mirror the accessloggrpc accessors verbatim.)

- [ ] **Step 4: Run to verify it passes (with `-race`)**

Run: `go test ./test/helpers/otlplogs/ -race -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/otlplogs/ && golangci-lint run ./test/helpers/otlplogs/... && go build ./...
git add test/helpers/otlplogs/
git commit -m "phase 45.1 Task 9: test/helpers/otlplogs — in-process OTLP LogsService receiver accumulator (the accessloggrpc precedent; D-OTLP-RECEIVER-WIRING)"
```

---

## Task 10: The `0084-otlp-access-log` differential fixture + the `disable_builtin_labels` subject unit test

**Files:**
- Create: `test/fixtures/0084-otlp-access-log/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0084` driver package)
- (the `disable_builtin_labels` unit prong lands in `internal/accesslog/otlpsink_test.go` — Task 6 already covers it; confirm it asserts the empty-Resource path)

Model the whole directory on `test/fixtures/0081-grpc-access-log/`. The data-plane backend REUSES `HTTPFixedBody = 4` (BackendKind stays 38).

- [ ] **Step 1: Author the bootstraps.** Both: an **H1** downstream listener → a route to the `HTTPFixedBody` backend; an HCM `access_log[]` `OpenTelemetryAccessLogConfig { common_config: { log_name: "0084", grpc_service: { envoy_grpc: { cluster_name: "c_otlp" } } } }` (NO `body`/`attributes`/`disable_builtin_labels` — the pure built-in record); a `c_otlp` cluster `type: STRICT_DNS` + `typed_extension_protocol_options: HttpProtocolOptions{ explicit_http_config: { http2_protocol_options: {} } }` (h2c, no TLS — D-OTLP-RECEIVER) pointing at `{{.OTLPHost}}:{{.OTLPPort}}`. `envoy.yaml` (reference): `OTLPHost = host.docker.internal`. `envoy-go.yaml` (subject): `OTLPHost = 127.0.0.1`. (Copy the listener/admin/backend templating from `0081`; SWAP the access-logger typed_config + the cluster name.)

- [ ] **Step 2: Author `driver/driver.go`.** Copy `0081/driver/driver.go` and adapt: `package driver`, `fixtureName = "0084-otlp-access-log"`, `func init() { fixture.RegisterFixture(fixtureName, &otlpDriver{}) }`; the receiver is `otlplogs.NewAtAddr(...)` accumulating `*logspb.LogRecord`s; the driver holds `refRecords`/`subjRecords []*logspb.LogRecord` + the per-side `Resource.attributes` snapshots; `allocateOTLPPort`/`ensureServer` mirror `allocateALSPort`/`ensureServer`; `BackendKind() == fixture.HTTPFixedBody`; the template keys are `OTLPHost`/`OTLPPort`. Drive: fire **N** (e.g. 8) requests with a FIXED `Host`/`User-Agent` and a **query-less** path against the data-plane listener, then `pollCount(srv, N)` (poll-to-converge, NEVER `time.Sleep` — `reference_concurrency_differential_release_barrier`). `Reset()`/fresh-server per side for clean separation; hard-`Close()` after the subject snapshot.

- [ ] **Step 3: Author the assertions (`expectations.yaml` + the driver's `AssertStats`).** Cross-side EXACT, aggregated over all N received records (AMEND-OTLP-BUILTINS/EXPORT-SHAPE): both sides `len(records) == N` (a zero-record "pass" is vacuous — prove decode ran on BOTH sides); for EVERY record `GetTimeUnixNano() != 0` (PRESENCE, not value); for every per-`ResourceLogs` `Resource.attributes` snapshot, the 4 keys `{log_name, zone_name, cluster_name, node_name}` are ALL present and `log_name == "0084"` (the node-derived three may be empty both sides under the no-node config — assert KEYS present + the `log_name` value). Plus the SUBJECT-side `access_logs.open_telemetry_access_log.logs_written == N` (scrape `/stats` via the `scrapeFlatStats` helper — copy it from `0081`). **UNasserted (per `reference_streaming_sink_differential_framing`):** the `time_unix_nano` VALUE; `Export`-call count / per-call batch sizes / upstream connection count; `severity_*`/`body`/`LogRecord.attributes` (absent both sides); the OTLP cluster's incidental upstream stats.

- [ ] **Step 4: Author `README.md`** — the fixture's purpose, the driver-owned-receiver lifecycle (the `0081` README shape), the query-less-path constraint, the framing-not-asserted note (AMEND-OTLP-EXPORT-SHAPE), the `disable_builtin_labels`-covered-by-unit-test note (one fixture dir = one runner branch — `reference_differential_fixture_dispatch_constraint`), and the host-reachability table (`host.docker.internal` reference / `127.0.0.1` subject).

- [ ] **Step 5: Register + run the fixture isolated.**

Add `_ "github.com/esalaine/envoy-go/test/fixtures/0084-otlp-access-log/driver"` to `runner_test.go`'s fixture blank-imports (alongside `:108`).
Run (the correct selector — `reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0084' -count=1`
Expected: PASS (both sides export the same N records with the 4 Resource label keys + `log_name=0084`; `logs_written == N`).
Confirm fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **86**.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0084-otlp-access-log/ test/differential/runner_test.go
git commit -m "phase 45.1 Task 10: 0084-otlp-access-log differential — cross-side EXACT on record count + time_unix_nano presence + the 4 Resource label keys + log_name, poll-to-converge, query-less path (fixtures 85 → 86)"
```

---

## Task 11: `0084` deliberate-break proofs + flake gate + the FULL-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0084` FAILS (the assertion is live), then `git restore` it:
  - (a) Break `buildLogRecord` to NOT set `TimeUnixNano` (leave it 0) ⇒ the `time_unix_nano != 0` assertion must FAIL.
  - (b) Break `buildResource` to emit `log_name == ""` (or drop the `log_name` key) ⇒ the `log_name == "0084"` / 4-keys assertion must FAIL.
  - (c) Break the `logsWritten.Add(...)` site (skip it) ⇒ the subject `logs_written == N` assertion must FAIL.
  - (d) Break the OTLP parse arm so the sink never builds (e.g. early-return before the append) ⇒ the both-sides `len(records) == N` / subject-side assertion must FAIL (proving the subject actually exports).
  Run each: `go test ./test/differential/ -run 'TestDifferential/0084' -count=1` ⇒ expect FAIL, then restore ⇒ expect PASS. Record each break+restore in PROGRESS-45.1.md.

- [ ] **Step 2: Flake gate** — 20 consecutive green runs:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0084' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. (A transient `subject ready: EOF` is the startup-race flake — `reference_differential_fullsuite_startup_flake` — isolate-re-run that single run; NOT a 0084 regression.)

- [ ] **Step 3: FULL `internal/accesslog` package `-race`** (the sink writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`):
```bash
go test ./internal/accesslog/ -race -count=1
```
Expected: PASS, no race.

- [ ] **Step 4: Commit the PROGRESS update**
```bash
git add docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md
git commit -m "phase 45.1 Task 11: 0084 deliberate-break proofs (4 live assertions) + 20/20 flake + full-package -race"
```

---

## Task 12: Full 86-dir differential + six-gate + ADR-0258 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 45 STAYS in-progress; family STAYS OPEN)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                 # clean
go build ./...                               # ok
go test ./... -count=1                       # full unit + the 86-dir differential
go test ./internal/accesslog/ -race -count=1 # the background-mutator race gate
```
Expected: all green. (The full differential is the byte-stability regression anchor — no non-OTLP fixture should move.) Also confirm `go mod tidy -diff` is EMPTY (the otlp promotion is the only go.mod delta).

- [ ] **Step 2: ADR-0258 §Decision/§Consequences** — land them in DECISIONS.md beneath the §Context already drafted at SPEC §13 (ADR-0044). §Decision: the `OTLPAccessLogSink` over the `Sink` abstraction; the `OTLPLogsClient` UNARY wrapper over the `Dialer`; the built-in mapping (`time_unix_nano` only + the 4-label `Resource` + `disable_builtin_labels` all-or-nothing); the reused 44.2 size/interval/close buffer → per-`Export` batch + retry-once; the parse arm lifting OTLP from the ADR-0041 silent-ignore set via the shared `parseCommonGrpcAccessLogConfig` helper; STRICT-REJECT `google_grpc`/non-V3/empty-cluster + `body`/`attributes`/`resource_attributes` (45.2) + `formatters`; `stat_prefix` PARSE-ACCEPT-but-INERT; the two static `access_logs.open_telemetry_access_log.*` counters; the driver-owned `otlplogs` receiver (NO new BackendKind); the `go.opentelemetry.io/proto/otlp` transitive→direct promotion. §Consequences: `body`/`attributes`/`resource_attributes` (45.2, ADR-0259) + `stat_prefix` honoring deferred; the Observability family STAYS OPEN; row 45 STAYS `in-progress` (45.1 is NOT the final leg).

- [ ] **Step 3: BEHAVIOR_CONTRACT.md** — add the `### Access log — OpenTelemetry (OTLP) access-log sink` subsection (SPEC §9) + advance the stat-surface block 1189 → 1191.

- [ ] **Step 4: STATE.md + ROADMAP.md** — STATE active-phase → `phase 45.1 (otlp-access-log) IMPL done`; the count figures → stat **1191** / fixtures **86** / fuzzers **45** / BackendKind **38** / DECISIONS **ADR-0258** / +1 go.mod module. ROADMAP row 45 STAYS **`in-progress`** (per-leg ADR-0106 + `reference_roadmap_split_phase_row_done`; 45.1 is NOT the final leg — row 45 flips `done` at the 45.2 IMPL; the Observability family STAYS OPEN). Set the next action → the 45.2 (operator engine) SPEC.

- [ ] **Step 5: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == **45** and advance the documented running total 44 → 45 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md / PROGRESS-45.1.md consistently.

- [ ] **Step 6: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 45.1 (otlp-access-log core) IMPL: ADR-0258 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 45 STAYS in-progress; Observability family STAYS OPEN); stat 1191 / fixtures 86 / fuzzers 45 / BackendKind 38 / +1 go.mod module"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 45.1 (otlp-access-log) IMPL: the core OTLP access-log sink — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 45.1 IMPL squash and route the next session to the **45.2 (operator engine) SPEC**.
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1191** (H2 cluster; non-H2 **1187**) / fixtures **86** (tail `0084-otlp-access-log`) / fuzzers **45** / BackendKind **38** (UNCHANGED — the OTLP receiver is driver-owned) / DECISIONS **ADR-0258**. +1 go.mod module (`go.opentelemetry.io/proto/otlp` transitive→direct; `go mod tidy -diff` EMPTY afterward). ZERO new `internal/` packages (`otlplogs` is test-only).

> **NOTE on the stat surface non-H2 figure:** the SPEC §14 records non-H2 **1187** at IMPL (the +2 OTLP counters register only when ≥1 OTLP sink is built; the H2-cluster figure is 1191). Confirm the exact figures via the registration test (Task 7) + the six-gate (Task 12) — assert the +2 DELTA, not a brittle absolute.
