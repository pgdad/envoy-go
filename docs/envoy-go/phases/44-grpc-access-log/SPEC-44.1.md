# Phase 44.1 SPEC — the core gRPC Access Log Service (ALS) streaming sink: config parse + the `ALSClient` typed wrapper over `grpcclient.Dialer` + the `GrpcAccessLogSink` (the 10-field structured `HTTPAccessLogEntry` mapping, identifier-once stream lifecycle, fixed simple flush) + the `access_logs.grpc_access_log.*` stats + the `0081` receiver differential — the FIRST leg of the FIRST Observability-family row (ANCHORS ADR-0255)

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-44 parent BRAINSTORM (`docs/envoy-go/phases/44-grpc-access-log/BRAINSTORM.md`, squash `81afdd59`). This SPEC charters phase **44.1** — the core streaming gRPC ALS sink: lift `envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig` out of the ADR-0041 silent-ignore set; add the `ALSClient` typed wrapper (the ADR-0158 `AuthClient` precedent) over the phase-18 `grpcclient.Dialer`; add the `GrpcAccessLogSink` (the phase-06.2 `AsyncFileSink` bounded-channel + writer-goroutine shape) streaming structured `HTTPAccessLogEntry`s over `AccessLogService.StreamAccessLogs`; register the `access_logs.grpc_access_log.*` sink stats; wire it in `cmd/envoy-go/main.go`; and prove it with a new in-process ALS-receiver BackendKind + the `0081-grpc-access-log` differential. Counts at SPEC commit UNCHANGED (stat surface **1187** [H2 cluster; non-H2 **1183**] / fixtures **82** / fuzzers **43** / BackendKind tail **38** / DECISIONS tail **ADR-0254**, next-free **ADR-0255**). The §11 D-ALS-* empirical pins were EXECUTED IN-SESSION (2026-06-25) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Deliver envoy-go's FIRST access-log sink that leaves the process: when an HCM `access_log[]` entry carries an `HttpGrpcAccessLogConfig` typed_config, envoy-go dials the configured `grpc_service.envoy_grpc.cluster_name` cluster (via the phase-18 `grpcclient.Dialer`), opens the `AccessLogService.StreamAccessLogs` client-streaming RPC, sends an `identifier { node, log_name }` once per stream, then streams each access-log event as a structured `HTTPAccessLogEntry` proto built from the 10 plumbed `accesslog.Record` operators (ADR-0067). This is the project's SECOND concrete `accesslog.Sink` (alongside the phase-06.2 file sink) and its SECOND gRPC-client consumer (after ext_authz, ADR-0158). It composes on TWO already-built substrates — the phase-06.2 `Sink`/`Record` subsystem and the phase-18 `grpcclient` layer — adding ZERO new packages and ZERO new go.mod modules. Byte-identical when no ALS `access_log` entry is configured (the file-only path is untouched; the full differential is the regression anchor). It ANCHORS ADR-0255 and OPENS the Observability family (the family STAYS OPEN — OTLP / tracing / stats sinks / tap remain future rows).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live probe (2026-06-25, contrib-v1.37.2 — an H1 downstream → a real HTTP backend, with an HCM `access_log[]` `HttpGrpcAccessLogConfig` streaming to an h2c gRPC ALS receiver, all on a shared bridge per `reference_docker_probe_bridge_network`; **decode-ran proof: `access_logs.grpc_access_log.logs_written: 13` for 13 requests + 13 captured entries**) drove these amendments:

- **AMEND-ALS-1 (the stat family — `access_logs.grpc_access_log.{logs_written,logs_dropped}`, +2 → 1189; STATIC names, NO `IsValidName` guard; NO `server.accesslog_dropped` reuse).** Live finding (D-ALS-STATS): the reference's sink-specific stats are exactly two process-global counters under the `access_logs.grpc_access_log.*` prefix — `logs_written` (== total entries handed to the sink; deterministic) and `logs_dropped` (drop accounting; stayed 0). The prefix is **NOT** the brainstorm's hypothesized `access_logs.grpc.*` and is **NOT** scoped by `log_name` — the names are fully static (no wire/config-derived segment), so the `reference_dynamic_stat_name_charset_guard` does NOT apply. The file sink's `server.accesslog_dropped` (`stats.go:14`) is NOT reused — the gRPC sink registers its own distinct `access_logs.grpc_access_log.logs_dropped`. Surface **1187 → 1189** (+2). These are NOT useH2-gated cluster stats; they are registered once per process when ≥1 gRPC ALS sink is built (the EXACT baseline + delta confirmed at IMPL via a registration test).
- **AMEND-ALS-2 (the field mapping — `request.path` carries the QUERY STRING; Duration → `common_properties.duration`).** Live finding (D-ALS-FIELDS): two corrections to the brainstorm's §load-bearing mapping table. (a) The reference's `request.path` is the full request target **including the query string** (`/some/path?x=1`), whereas envoy-go's `Record.Path` is `r.URL.Path` (path-only; `accesslog_emit.go:25`). The differential fixture MUST drive a **query-less path** so the subject's path-only mapping stays cross-side EXACT (a query-bearing request would diverge — a documented fixture-design constraint, NOT a behavior bug; envoy-go faithfully maps `Record.Path` → `request.path`). (b) The brainstorm guessed `Duration → common_properties.time_to_last_downstream_tx_byte`; the reference populates `common_properties.duration` (the total request duration) alongside the granular `time_to_*` fields. envoy-go's `Record.Duration` (`= time.Since(start)`) maps most faithfully to `common_properties.duration`. Both are non-deterministic ⇒ UNasserted in the differential regardless.
- **AMEND-ALS-3 (stream/message FRAMING legitimately varies — the wire-format-both-sides discipline applies to the `HTTPAccessLogEntry` PAYLOAD, asserted aggregated across messages, NOT to stream/message framing).** Live finding (D-ALS-LIFECYCLE): the reference does NOT emit one fixed framing — under low concurrency it opened a FRESH `StreamAccessLogs` stream per sequential request (so each first-message carried the identifier), and under burst it reused a stream (identifier-once + batched `http_logs.log_entry[]`). envoy-go's design (one lazily-established reused stream, identifier-once, fixed simple flush) is therefore a LEGITIMATE alternative framing that produces the SAME per-entry payloads. The `0081` differential consequently asserts the **deterministic per-entry field subset aggregated across all received entries**, and does NOT assert stream count, message count, or per-message batching. This is `reference_wire_format_both_sides_see_same_bytes` applied at the `HTTPAccessLogEntry` level (the shared decoded payload), not at the stream-framing level.
- **AMEND-ALS-4 (the deterministic assertable subset — 7 structured fields).** Live finding (D-ALS-FIELDS): of the fields envoy-go's 10-field `Record` mapping populates, the cross-side-deterministic subset is exactly seven: `request.request_method`, `request.path` (query-less per AMEND-ALS-2), `request.authority`, `request.user_agent`, `response.response_code`, `response.response_body_bytes` (deterministic only for a fixed-size backend body), and the top-level `protocol_version` enum (`HTTP11` = 2 for an H1 downstream). The remaining mapped fields — `common_properties.start_time`, `common_properties.duration`, `common_properties.upstream_remote_address` — are non-deterministic (timestamps, durations, host IPs / ephemeral ports) and stay UNasserted. The reference also populates many fields envoy-go's 10-field `Record` does NOT carry (`request.scheme`, `request.request_id`, `common_properties.upstream_cluster`, `access_log_type`, `response.response_code_details`, all the wire-byte counts) — these are simply absent on the subject side and are NOT asserted.

### 1.2 ADR continuity + D-disposition at SPEC commit

- **ADR-0255** (next-free) — the core gRPC ALS streaming sink; §Context drafted here (§13), §Decision/§Consequences land at the 44.1 IMPL (ADR-0044). ANCHORS the Observability family.
- D-ALS-FIELDS / D-ALS-STATS / D-ALS-LIFECYCLE / D-ALS-RECEIVER: PINNED at this SPEC (§11). D-ALS-{CONFIG-DEFER, RECEIVER-WIRING, NODE, SPLIT-FINAL, FUZZER}: PLAN/IMPL pins (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8 + the §1.1 amendments)

- **`buffer_size_bytes` / `buffer_flush_interval` honoring** — 44.1 ships a FIXED simple flush; the size/interval flush triggers are the 44.2 leg (ADR-0256). At 44.1 these two `common_config` fields are PARSE-ACCEPTED-but-INERT (§3.1 D-ALS-CONFIG-DEFER) — a documented phased-rollout departure within the parent phase (the 44.1 differential does not set them).
- **`additional_request_headers_to_log` / `additional_response_headers_to_log`** — the header-capture leg 44.3 (ADR-0257); PARSE-ACCEPTED-but-INERT at 44.1.
- **TCP ALS (`tcp_logs` / `TcpGrpcAccessLogConfig`)** — STRICT-REJECTed (ADR-0080); envoy-go's `Record` is HTTP-shaped.
- **non-V3 `transport_api_version`** — STRICT-REJECTed (ADR-0080).
- **`grpc_service.google_grpc`** (the non-Envoy-gRPC variant) — STRICT-REJECTed; only `envoy_grpc.cluster_name` is supported (the `grpcclient.Dialer` cluster-name gate).
- **The `AccessLog.filter` sub-surface** (status_code_filter / duration_filter / runtime_filter / and_filter / or_filter) — stays silently-ignored (the file-sink defers it too); a deferred gating layer.
- **`additional_request_trailers_to_log`; full `AccessLogCommon` population** (response_flags, tls_properties, metadata, the rich timing fields beyond the one mapped); the OTLP access logger; the `stdout`/`stderr` stream loggers; gRPC reconnect-backoff tuning beyond a basic reconnect — all future Observability-family rows or same-leg follow-ons.

---

## 3. The core sink — `internal/bootstrap` + `internal/grpcclient` + `internal/accesslog` + `cmd/envoy-go/main.go` (ADR-0255)

### 3.0 Split disposition — leg 44.1 of the by-concern 3-leg split; FINAL ADR-0045 re-check at PLAN

44.1 is chartered here as the core leg (config parse + `ALSClient` + `GrpcAccessLogSink` + stats + main wiring + the `0081` receiver differential). Envelope estimate: ≈300–360 prod LoC across the four touch-points + the differential backend, near the ADR-0045 soft gate; the PLAN does the final re-check with real LoC (D-ALS-SPLIT-FINAL — the 43.1/43.2a/43.2b re-check precedent), but 44.1 ships as one leg. **Row 44 flips `done` PER-LEG at each IMPL** (no parent rollup, ADR-0106); the Observability family STAYS OPEN.

### 3.1 Config parse — lift gRPC ALS from the ADR-0041 silent-ignore set (`internal/bootstrap/bootstrap.go`)

The file-only parser silently ignores every non-file `access_log` typed_config (`parseOneAccessLog`, `bootstrap.go:271` — the ADR-0041 amendment). 44.1 detects the `HttpGrpcAccessLogConfig` TypeURL and parses it into a new `ALSConfig`:

- New TypeURL constant: `httpGrpcAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig"` (alongside `fileAccessLogTypeURL`, `:145`).
- Blank-import the config proto `github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3` in `bootstrap.go` (the ADR-0016 registry discipline; alongside the existing `access_loggers/file/v3` + `access_loggers/stream/v3` imports at `:39`/`:40`).
- New parsed struct (parallel to `AccessLogConfig`, `:152`):
  ```go
  type ALSConfig struct {
      ClusterName string // common_config.grpc_service.envoy_grpc.cluster_name
      LogName     string // common_config.log_name
  }
  ```
- New `Bootstrap` field `ALSConfigs []ALSConfig` (parallel to `AccessLogConfigs`; populated in registration order across all HCM `access_log[]`). Per-sink-type ordering within a listener is NOT load-bearing — every `Record` is `Submit`ted to every sink (`accesslog_emit.go:34`), so parallel slices are correct (a unify-into-one-ordered-list refactor is a non-blocking PLAN option, D-ALS-CONFIG-DEFER).
- Parse arm in `parseOneAccessLog` (after the file-TypeURL branch): on the gRPC-ALS TypeURL, `proto.Unmarshal` the `HttpGrpcAccessLogConfig`, then:
  - **STRICT-REJECT** (ADR-0080, byte-stable error wording at PLAN): a non-`envoy_grpc` `grpc_service` (i.e. `google_grpc`); a `transport_api_version` other than V3/AUTO (V2 → reject); an empty `common_config.grpc_service.envoy_grpc.cluster_name`. The sibling `TcpGrpcAccessLogConfig` TypeURL stays in the silent-ignore set (HTTP-only).
  - **PARSE-ACCEPT-but-INERT** (the phased-rollout departure, §2): `buffer_size_bytes` / `buffer_flush_interval` (honored at 44.2); `additional_request_headers_to_log` / `additional_response_headers_to_log` (honored at 44.3); the `AccessLog.filter` field (the file-sink-parity defer). Document each as accepted-and-inert at 44.1 — a config carrying them boots and logs, just without the deferred behavior.
  - **PARSE-ACCEPT**: `common_config.log_name` (→ `ALSConfig.LogName`; empty is valid), `common_config.grpc_service.envoy_grpc.cluster_name` (→ `ALSConfig.ClusterName`).

### 3.2 The `ALSClient` typed wrapper (`internal/grpcclient/grpcclient.go` — the `AuthClient` precedent, ADR-0158)

A typed wrapper over the generated `accesslogv3.AccessLogServiceClient` stub, mirroring `AuthClient` (`:157`):

```go
type ALSClient struct {
    conn   *grpc.ClientConn
    stub   accesslogv3.AccessLogServiceClient
    target string // cluster_name — for logs/errors
    closeOnce sync.Once
    closeErr  error
}
func NewALSClient(d *Dialer, clusterName string) (*ALSClient, error) // d.DialContext → wrap (the NewAuthClient shape, :178)
func (a *ALSClient) StreamAccessLogs(ctx context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error) // a.stub.StreamAccessLogs(ctx)
func (a *ALSClient) Close() error // sync.Once-guarded, the AuthClient :231 shape
```

- Blank-import / typed-import the service proto `github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3` (`accesslogv3`) — the same module already provides `auth/v3` (`grpcclient.go:59`).
- The `Dialer.DialContext` gates (`:112`–`:118`) already enforce cluster-exists + `UseH2()` — exactly ALS's requirement (a streaming RPC needs H2). The `grpc_service.envoy_grpc.cluster_name` maps onto the cluster-name gate. NO change to `Dialer`.
- `NewALSClient` dials eagerly at construction (the `NewAuthClient` shape — `grpc.NewClient` does not block; the first real dial fires on the first stream open). Per the ADR-0158 D2 leaks-on-exit MVP, the `ClientConn` is owned by the sink and `Close()`d at sink-close (the file-sink defer-LIFO discipline at `main.go:115`).

### 3.3 The `GrpcAccessLogSink` (`internal/accesslog` — the `AsyncFileSink` shape)

A second concrete `accesslog.Sink` (`accesslog.go:18`) alongside `AsyncFileSink`, mirroring the bounded-channel + writer-goroutine + idempotent-`Close` pattern (`writer.go:26`):

- **Fields:** a bounded `chan any` (capacity 4096, the `defaultChannelCapacity` constant); the `*ALSClient`; the parsed `LogName`; the boot `node` identifier (D-ALS-NODE, §12); the two stat counters (`logsWritten`, `logsDropped`); `done chan struct{}`; `closeOnce sync.Once`; a rate-limited drop-diagnostic (`lastDropLog`, the `writer.go:31` shape).
- **`Submit(r any)`** — NON-blocking (the `writer.go:73` drop-newest shape): `select { case ch <- r: default: logsDropped.Inc(); <rate-limited diag> }`. (The H1/H2 emit hooks `Submit` a `*accesslog.Record`; the sink ignores any non-`*Record` like the file sink does.)
- **The writer goroutine (`run`):** drains the channel; lazily establishes the `StreamAccessLogs` stream on the FIRST entry (sending the identifier-once message — see §3.5); builds the `HTTPAccessLogEntry` from the `*Record` (§3.4); sends it as a `StreamAccessLogsMessage{ http_logs: { log_entry: [entry] } }` (the FIXED simple flush — one entry per message at 44.1; 44.2 batches by size/interval); increments `logsWritten`. On a stream `Send` error: log, re-establish the stream (re-send identifier), retry once (a basic reconnect; backoff tuning deferred §2). On channel-close: `CloseAndRecv` the stream, then signal `done`.
- **`Close()`** — `sync.Once`-guarded (the `writer.go:88` shape): close the channel, await `done`, then `ALSClient.Close()`. Idempotent + threadsafe.
- **No `Formatter`** — ALS emits a structured proto, not formatted text (the `format.go` abstraction is file-sink-only).

### 3.4 The 10-field `Record` → `HTTPAccessLogEntry` structured mapping (§11 D-ALS-FIELDS, AMEND-ALS-2)

Build `*dataaccesslogv3.HTTPAccessLogEntry` (blank/typed-import `github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3`):

| `Record` field | `HTTPAccessLogEntry` path | notes |
|---|---|---|
| `Method` | `request.request_method` (`HTTPRequestProperties`, field 3) | deterministic ✓ |
| `Path` | `request.path` | deterministic ✓ — but reference carries the QUERY STRING; envoy-go maps path-only ⇒ fixture drives a **query-less** path (AMEND-ALS-2) |
| `Authority` | `request.authority` | deterministic ✓ |
| `UserAgent` | `request.user_agent` | deterministic ✓ |
| `ResponseCode` | `response.response_code` (`HTTPResponseProperties`, field 4) | `*wrapperspb.UInt32Value`; deterministic ✓ |
| `BytesSent` | `response.response_body_bytes` (uint64) | deterministic ✓ ONLY for a fixed-size backend body |
| `Protocol` | `protocol_version` (top-level field 2, the `HTTPAccessLogEntry_HTTPVersion` ENUM) | string→enum: `"HTTP/1.1"`→`HTTP11`(2), `"HTTP/2.0"`→`HTTP2`(3), else `PROTOCOL_UNSPECIFIED`(0); deterministic ✓ |
| `StartTime` | `common_properties.start_time` (`*timestamppb.Timestamp`) | non-deterministic — UNasserted |
| `Duration` | `common_properties.duration` (`*durationpb.Duration`) | non-deterministic — UNasserted (AMEND-ALS-2: NOT `time_to_last_downstream_tx_byte`) |
| `UpstreamHost` | `common_properties.upstream_remote_address.socket_address.{address,port_value}` | non-deterministic — UNasserted; split the `"host:port"` string into `core.Address{SocketAddress}` (best-effort; empty `UpstreamHost` ⇒ leave nil) |

Note `protocol_version`/`request`/`response` are TOP-LEVEL sub-messages of `HTTPAccessLogEntry` (fields 2/3/4), siblings of `common_properties` (field 1).

### 3.5 The stream lifecycle — lazy establish, identifier-once, reconnect-on-error (§11 D-ALS-LIFECYCLE)

- **Lazy establish:** the writer goroutine opens the stream on the FIRST `Record` (not at sink construction) — matching the reference and avoiding a connect attempt for a sink that never logs.
- **Identifier-once:** the FIRST `StreamAccessLogsMessage` on a stream carries `identifier { node, log_name }`; subsequent messages carry only the `http_logs` arm. The `node` is envoy-go's bootstrap node (minimal — id/cluster from `bootstrap.node` if present, else empty; node.* is non-deterministic ⇒ UNasserted, D-ALS-NODE §12). `log_name` from `ALSConfig.LogName`.
- **Reconnect-on-error:** on a `Send`/stream error, re-establish (re-send identifier) — a basic reconnect (per-attempt backoff deferred §2). envoy-go's reused-stream framing is a legitimate alternative to the reference's per-request-stream framing (AMEND-ALS-3) — the `0081` differential asserts payloads, not framing.

### 3.6 Boot wiring (`cmd/envoy-go/main.go`)

- A `grpcclient.Dialer` is constructed from the cluster manager `cm` (`grpcclient.New(cm)`; `cm` is in scope at `main.go:97`). (ext_authz already builds a Dialer at config-load; the boot Dialer is the same `New(cm)` call.)
- Register the two sink counters once (a new `accesslog.RegisterGrpcSinkCounters(bs.Stats) (*stats.Counter, *stats.Counter)` returning `logs_written` + `logs_dropped`, the `RegisterDroppedCounter` shape at `stats.go:14`).
- For each `bs.ALSConfigs` entry: `NewALSClient(dialer, cfg.ClusterName)` → `NewGrpcAccessLogSink(client, cfg.LogName, node, logsWritten, logsDropped)`; append to the existing `sinks []accesslog.Sink` (`main.go:107`). The defer-LIFO `Close()` loop (`main.go:115`) already closes every sink — no new defer.
- The sink list threads UNCHANGED into the HCM filters (`AccessLogSinks: sinks`, `main.go:225` / `:232`) and the network builtins — no HCM/listener shape change.

### 3.7 Byte-stability

Byte-identical when no ALS `access_log` entry is configured (the file-only path + every non-access-log path untouched; the `ALSConfigs` slice is empty ⇒ no Dialer/sink/stat). The full differential (82-dir today → 83-dir with `0081`) is the regression anchor. The 2 new stats register ONLY when ≥1 gRPC ALS sink is built.

---

## 4. Framework primitives — 0 new packages + 0 new go.mod deps

- REUSED: the `accesslog.Sink` interface + `Record` struct + the `[]accesslog.Sink` HCM plumbing (`config.go:107`/`:191`/`:285`) + the H1/H2 emit hooks (`accesslog_emit.go:18`/`:43`); the `AsyncFileSink` bounded-channel + writer-goroutine + idempotent-`Close` pattern (`writer.go`); the `grpcclient.Dialer` (`:85`/`:105`) + the `AuthClient` typed-wrapper pattern (ADR-0158); the bootstrap `access_log` parse (`bootstrap.go:265`) + the proto blank-import registry (ADR-0016); the `stats.Registry` `NewCounter` discipline; the `reference_docker_probe_bridge_network` differential pattern + the ext_authz gRPC test-helper precedent (`test/helpers/extauthzgrpc`).
- NEW: the `ALSConfig` parse arm + `Bootstrap.ALSConfigs`; the `ALSClient` typed wrapper; the `GrpcAccessLogSink`; the two `access_logs.grpc_access_log.*` registrations; the main wiring; the ALS-receiver BackendKind 39 + the `0081` fixture.
- ZERO new Go packages (the sink in `internal/accesslog`, the client in `internal/grpcclient`, the parse in `internal/bootstrap`, the receiver under `test/`); ZERO new go.mod modules — the ALS service/config/data protos all resolve at the pinned `go-control-plane/envoy v1.32.4` (verified at brainstorm + this SPEC). `go mod tidy -diff` anticipated EMPTY.

---

## 5. Proto-field roster (consumed at 44.1)

From `HttpGrpcAccessLogConfig` → `CommonGrpcAccessLogConfig`: `log_name` (1) CONSUMED; `grpc_service.envoy_grpc.cluster_name` (2) CONSUMED; `transport_api_version` (6) CONSUMED-as-gate (V3/AUTO accept, else reject). `buffer_flush_interval` (3) / `buffer_size_bytes` (4) PARSE-ACCEPT-INERT (44.2). `additional_request_headers_to_log` / `additional_response_headers_to_log` PARSE-ACCEPT-INERT (44.3). From `HTTPAccessLogEntry`: the 10 fields of §3.4 POPULATED. `go mod tidy -diff` EMPTY.

## 6. PARSE-REJECT roster + fuzzer

- **PARSE-REJECT arms (ADR-0080):** `google_grpc` grpc_service; non-V3 `transport_api_version`; empty `envoy_grpc.cluster_name`. `TcpGrpcAccessLogConfig` stays silent-ignored. (The `Dialer` adds its own runtime rejects — unknown cluster / non-H2 cluster — surfaced at sink build, a `log.Fatalf` boot failure like the file-sink open error at `main.go:111`.)
- **Fuzzer (D-ALS-FUZZER, §12):** a new config-parse fuzzer over the `HttpGrpcAccessLogConfig` parse arm is the natural new attack surface (the bootstrap-parse-fuzzer precedent). Anticipated +1 ⇒ fuzzers **43 → 44**; the EXACT decision (land it at 44.1 vs defer) is a PLAN call, and landing it RECONCILES the running `^func Fuzz` total per `reference_fuzzer_count_docs_drift` (currently 43 actual).

## 7. Stat surface — add 2 (1187 → 1189) (per §11 D-ALS-STATS + AMEND-ALS-1)

- `access_logs.grpc_access_log.logs_written` — counter, ++ per `HTTPAccessLogEntry` handed to the stream (deterministic == total entries; the clean assertable sink counter).
- `access_logs.grpc_access_log.logs_dropped` — counter, ++ per `Submit` drop-newest (channel-full).
- Process-global (registered when ≥1 gRPC ALS sink is built), NOT log_name-scoped, NOT useH2-gated, STATIC names (no `IsValidName` guard). NO `server.accesslog_dropped` reuse (AMEND-ALS-1). The ALS cluster's incidental `upstream_cx_http2_total` / `upstream_cx_total` / `upstream_cx_active` / `upstream_rq_total` / `upstream_rq_active` / `http2.streams_active` / membership_* are ALREADY-registered cluster stat shapes (no surface delta; note `upstream_rq_total` counts STREAMS opened, not log messages — NOT an entry proxy). Surface **1187 → 1189** (+2); EXACT figure confirmed at IMPL via a registration test.

## 8. Differential fixture taxonomy (+1: `0081` cross-side EXACT on the per-entry subset)

### 8.1 `0081-grpc-access-log` (cross-side EXACT — poll-to-converge then aggregated per-entry assertion)
An **H1** downstream listener (the access log fires regardless of downstream codec — no H2-downstream requirement; the H2 is the ALS cluster→receiver leg) → a route to a fixed-body backend, with an HCM `access_log[]` `HttpGrpcAccessLogConfig` streaming to an ALS receiver, on BOTH subject (in-process envoy-go) and reference (`contrib-v1.37.2`). Both sides stream the SAME `HTTPAccessLogEntry` payloads to the SAME receiver (the wire-format-both-sides discipline at the entry level, AMEND-ALS-3). SLEEPLESS: poll-to-converge until N entries arrive (a release barrier, NEVER a `time.Sleep` — `reference_concurrency_differential_release_barrier`), driven by firing N requests with a fixed `User-Agent` + `Host` + a **query-less** path (AMEND-ALS-2) against a fixed-size backend body.

- **Assertions (cross-side EXACT, aggregated over all received entries — AMEND-ALS-3/4):** for each of the N entries, the deterministic 7-field subset matches both sides: `request.request_method`, `request.path` (query-less), `request.authority`, `request.user_agent`, `response.response_code`, `response.response_body_bytes`, `protocol_version` (== `HTTP11`). Plus `logs_written == N` on the subject (the clean sink counter).
- **UNasserted:** `common_properties.{start_time,duration,upstream_remote_address}`, `request.{scheme,request_id}`, `common_properties.upstream_cluster`, `access_log_type`, `response.response_code_details`, all wire-byte counts, the `identifier.node` content, and stream/message/batch framing (D-ALS-FIELDS non-deterministic set + the subject-absent fields).
- **Reachability (D-ALS-RECEIVER-WIRING, §12):** the in-process receiver must be reachable from BOTH the subject (host) AND the reference (Docker container) — a shared bridge + a receiver hostname reachable from the reference container, per `reference_docker_probe_bridge_network` (the 0080 backend / ext_authz gRPC test-helper precedent). The ALS cluster is h2c (no TLS) per D-ALS-RECEIVER. Exact wiring (in-process accumulator + how the reference container reaches the host-bound receiver) is a PLAN/IMPL deliverable.

### 8.2 New BackendKind 39 (the ALS gRPC receiver)
An in-process `accesslogv3.AccessLogServiceServer` (embed `UnimplementedAccessLogServiceServer`; `RegisterAccessLogServiceServer`) whose `StreamAccessLogs` loops `stream.Recv()`, accumulates every received `HTTPAccessLogEntry` (across messages + streams — AMEND-ALS-3), and exposes a poll/assert surface (entry count + per-entry fields) for the driver; `SendAndClose(&StreamAccessLogsResponse{})` on stream end. Plain h2c gRPC server (no TLS, D-ALS-RECEIVER). BackendKind tail **38 → 39**. Whether it's a fixture-local backend or a shared `test/helpers` package is a PLAN call (the ext_authz gRPC helper precedent).

## 9. Behavior-contract delta (the 44.1 bundle; ADR-0255 atomic landing)

Add a `### Access log — gRPC Access Log Service (ALS) streaming sink` subsection: an HCM `access_log[]` `HttpGrpcAccessLogConfig` dials `grpc_service.envoy_grpc.cluster_name` (H2-required), opens `AccessLogService.StreamAccessLogs`, sends `identifier { node, log_name }` once per stream, then streams each event as a structured `HTTPAccessLogEntry` (the 10-field mapping; `protocol_version` enum); a fixed simple flush (one entry per message at 44.1); reconnect-on-error; drop-newest backpressure into `access_logs.grpc_access_log.logs_dropped`; `logs_written` per entry. STRICT-REJECT `tcp_logs` / non-V3 / `google_grpc` / empty cluster_name. The stat-surface block advances 1187 → 1189. ADR-0255 lands atomically with this contract delta at the 44.1 IMPL.

## 10. Per-task structure (~10–12 tasks; PLAN decomposes)

PROGRESS + baselines + the final ADR-0045 re-check (D-ALS-SPLIT-FINAL); the `ALSConfig` parse arm + `Bootstrap.ALSConfigs` + the blank-import + the STRICT-REJECT arms (+ the parse fuzzer, D-ALS-FUZZER) [TDD]; the `ALSClient` typed wrapper [TDD, the AuthClient test shape]; the `GrpcAccessLogSink` (bounded channel + writer goroutine + lazy-establish/identifier-once/reconnect + idempotent Close) [TDD, `-race`]; the 10-field `Record`→`HTTPAccessLogEntry` mapping + the `protocol_version` enum [TDD, table-driven]; the two `access_logs.grpc_access_log.*` registrations (1187→1189) + a registration test; the main wiring (Dialer + per-`ALSConfig` sink build + counter registration); the ALS-receiver BackendKind 39 (D-ALS-RECEIVER-WIRING); the `0081` cross-side EXACT fixture (poll-to-converge, query-less path, aggregated per-entry subset); `0081` deliberate breaks (`-count=1`, `reference_differential_break_protocol_count1`) + 20/20 flake + the FULL-package `-race` (the sink's writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`); full 83-dir differential + six-gate; ADR-0255 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + the fuzzer-count reconcile (`reference_fuzzer_count_docs_drift`); completion bundle (ROADMAP row 44 leg 44.1 → done; the Observability family STAYS OPEN).

## 11. SPEC-time empirical-pin block (D-ALS-* — executed IN-SESSION 2026-06-25)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge `alsprobe-net`; a single throwaway "probe" container serving an HTTP backend on :9000 + an h2c ALS gRPC receiver on :8080, both reachable from the reference container by the hostname `probe`; an H1 downstream → backend route with an HCM `access_log[]` `HttpGrpcAccessLogConfig` → ALS cluster `als_cluster` [STRICT_DNS, h2c, `typed_extension_protocol_options` HttpProtocolOptions `explicit_http_config.http2_protocol_options{}`]; **decode-ran proof: `access_logs.grpc_access_log.logs_written: 13` for 13 requests + 13 captured `HTTPAccessLogEntry`s in the receiver stdout**).

| Pin | Result |
|-----|--------|
| **D-ALS-FIELDS** | PINNED (AMEND-ALS-2/4). The 10-field mapping is §3.4. Deterministic-assertable subset = 7 fields (method/path[query-less]/authority/user_agent/response_code/response_body_bytes/protocol_version). `protocol_version` = `HTTP11`(2) for H1 (a top-level enum, NOT under common_properties). `request.path` carries the QUERY STRING (envoy-go maps path-only ⇒ query-less fixture). Duration → `common_properties.duration` (NOT `time_to_last_downstream_tx_byte`). Non-deterministic (UNasserted): start_time, duration, all addresses/ports, stream_id/request_id, node build/extensions, wire-byte counts. `request`/`response`/`protocol_version` are top-level siblings of `common_properties`. |
| **D-ALS-STATS** | PINNED (AMEND-ALS-1). Sink stats = `access_logs.grpc_access_log.logs_written` + `access_logs.grpc_access_log.logs_dropped` (process-global, STATIC names, NOT log_name-scoped). NO `server.accesslog_dropped` reuse. Surface **1187 → 1189** (+2). ALS cluster's incidental `upstream_cx_*`/`upstream_rq_*`/`http2.streams_active` are already-registered shapes (no delta; `upstream_rq_total` counts streams, not messages — NOT an entry proxy). |
| **D-ALS-LIFECYCLE** | PINNED (AMEND-ALS-3). Identifier-once-per-stream confirmed (a captured message with `identifier` ABSENT carried 2 batched entries). Entries batch into `http_logs.log_entry[]`. Framing VARIES: sequential requests opened fresh streams (identifier each); concurrent reused a stream (identifier-once + batch). NO special final-flush message on drain (graceful stop → streams terminate `Canceled`; eager flush leaves nothing). ⇒ assert per-entry payload aggregated across messages/streams, NOT framing. |
| **D-ALS-RECEIVER** | PINNED. A plain h2c (no-TLS) `grpc.NewServer()` + `RegisterAccessLogServiceServer` on a bare port works — the reference dialed cleartext-h2 and streamed (13 written, 0 dropped). The ALS cluster needs `typed_extension_protocol_options` HttpProtocolOptions `explicit_http_config.http2_protocol_options{}` (booted first try on contrib-v1.37.2); without an H2 options block the gRPC dial fails. The receiver accumulates entries across messages + streams; the Go accessor for the batched entries is `GetLogEntry()` (the `log_entry` repeated field). |

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-ALS-CONFIG-DEFER** — confirm the PARSE-ACCEPT-but-INERT disposition for `buffer_*` (44.2) + `additional_*_headers` (44.3) vs a STRICT-REJECT-until-the-leg-lands posture; whether to unify `AccessLogConfigs` + `ALSConfigs` into one ordered list or keep parallel slices (§3.1).
- **D-ALS-RECEIVER-WIRING** — the `0081` receiver reachability: in-process accumulator + how the reference Docker container reaches the host-bound receiver (shared bridge + hostname, the 0080 backend / ext_authz gRPC helper precedent); fixture-local backend vs a `test/helpers` package (§8.2).
- **D-ALS-NODE** — the `identifier.node` content envoy-go sends (minimal id/cluster from bootstrap.node vs empty; node.* is UNasserted regardless) — confirm envoy-go has a bootstrap node to source (§3.5).
- **D-ALS-SPLIT-FINAL** — the final ADR-0045 re-check at PLAN with real LoC (44.1 anticipated ≈300–360 prod LoC, one leg; §3.0).
- **D-ALS-FUZZER** — land the `HttpGrpcAccessLogConfig` parse fuzzer at 44.1 (fuzzers 43 → 44) vs defer; reconcile the documented-vs-actual `^func Fuzz` count regardless (`reference_fuzzer_count_docs_drift`).

## 13. ADR continuity — the ADR-0255 §Context DRAFT (anchored here; full entry lands at the 44.1 IMPL)

**ADR-0255 §Context (draft):** The phase-06.2 access-log subsystem (ADR-0066/0067) shipped a single sink type — a file sink (`AsyncFileSink`) writing Envoy-default-format text over the 10-field `Record`; every non-file `access_log` typed_config was silently ignored (ADR-0041). The phase-18 gRPC client layer (ADR-0158) shipped a cluster-name→`*grpc.ClientConn` `Dialer` + a typed `AuthClient` wrapper, explicitly naming future typed wrappers (ProcessorClient, RateLimitClient) as the layering pattern. Phase 44 opens the Observability family with a streaming gRPC Access Log Service sink — `envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig` driving `envoy.service.accesslog.v3.AccessLogService.StreamAccessLogs` — composing on BOTH substrates: a second `accesslog.Sink` (the `AsyncFileSink` shape) over a typed `ALSClient` (the `AuthClient` shape) over the `grpcclient.Dialer`. The 2026-06-25 live probe (D-ALS-FIELDS/STATS/LIFECYCLE/RECEIVER, contrib-v1.37.2) pinned: the structured `HTTPAccessLogEntry` field population (the 10-field mapping; `protocol_version` is a top-level enum, `request.path` carries the query string, Duration → `common_properties.duration`); the two process-global sink stats `access_logs.grpc_access_log.{logs_written,logs_dropped}` (+2 → 1189; NOT the brainstorm's `access_logs.grpc.*`); the identifier-once stream framing (with the finding that framing legitimately varies — fresh-stream-per-request vs reused-stream-with-batching — so a differential asserts per-entry payload, not framing); and that a plain h2c gRPC receiver works. The 44.1 (core) leg lifts gRPC ALS from the silent-ignore set (parse `HttpGrpcAccessLogConfig` → `ALSConfig`; STRICT-REJECT `tcp_logs`/non-V3/`google_grpc`/empty-cluster), adds the `ALSClient` typed wrapper + the `GrpcAccessLogSink` (lazy-establish / identifier-once / reconnect / fixed simple flush / the 10-field structured mapping), the two sink stats, the main wiring, and the `0081` receiver differential (BackendKind 39). `buffer_size_bytes`/`buffer_flush_interval` (44.2, ADR-0256) and `additional_{request,response}_headers_to_log` (44.3, ADR-0257) are PARSE-ACCEPT-but-INERT at 44.1. §Decision/§Consequences land at the 44.1 IMPL. ANCHORS ADR-0255 + the Observability family (which STAYS OPEN).

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat **1187** (H2 cluster; non-H2 **1183**) / fixtures **82** / fuzzers **43** / BackendKind **38** / DECISIONS **ADR-0254** (next-free **ADR-0255**). Anticipated at the 44.1 IMPL: stat **1189** (+2 — `access_logs.grpc_access_log.logs_written` + `logs_dropped`; AMEND-ALS-1) / fixtures **83** (`0081-grpc-access-log`) / fuzzers **44** (or 43 — D-ALS-FUZZER) / BackendKind **39** (the ALS receiver) / DECISIONS **ADR-0255** (ANCHORS the Observability family). ZERO new packages + ZERO new go.mod modules. ROADMAP row 44 leg 44.1 flips **`done`** at the 44.1 IMPL (per-leg, ADR-0106); the Observability family STAYS OPEN. Next → the 44.1 PLAN (`superpowers:writing-plans` — the §12 D-ALS-* PLAN questions; a fresh worktree off master per `feedback_git_worktrees`), then the 44.1 IMPL (subagent-driven per `feedback_execution_style`).
