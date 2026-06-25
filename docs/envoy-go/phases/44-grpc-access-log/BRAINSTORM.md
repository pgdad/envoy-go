# Phase 44 Brainstorm — gRPC Access Log Service (ALS) sink (the FIRST row of the NEW Observability family; a streaming gRPC access-log sink over the phase-06.2 `Sink` abstraction, `envoy.service.accesslog.v3.AccessLogService` — split BY-CONCERN into 3 legs: 44.1 core streaming + 44.2 buffering + 44.3 header-capture)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 44 (`grpc-access-log`), the **FIRST row of the NEW Observability family** (the §9 ROADMAP family heading that opens after the Upstream-robustness family — phases 39–43 — CLOSED at the 43.2b IMPL). Per ADR-0106 a new family opens with a fresh BRAINSTORM (the 38.2→39 / 40→41 / 42→43 family-boundary precedent: a family-close ⇒ a fresh subject-selection BRAINSTORM). Phase 44 delivers a **streaming gRPC Access Log Service sink** — `envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig` driving the `envoy.service.accesslog.v3.AccessLogService.StreamAccessLogs` client-streaming RPC — as a second concrete `accesslog.Sink` alongside the phase-06.2 file sink.

Phase 44 is the natural FIRST Observability-family row: it composes on TWO already-built substrates rather than inventing either — (i) the **phase-06.2 access-log subsystem** (the `accesslog.Sink` interface `Submit(r any)`/`Close() error`, the `accesslog.Record` 10-field entry struct, the `AsyncFileSink` bounded-channel-+-writer-goroutine pattern, the HCM emit hooks, the bootstrap `access_log` parse), and (ii) the **phase-18 gRPC client layer** (`internal/grpcclient` — the cluster-name→`*grpc.ClientConn` `Dialer` + the typed `AuthClient` wrapper pattern, ADR-0158). Where phase 06.2 wrote access logs to a FILE (Envoy default text format), phase 44 STREAMS them to a gRPC service as STRUCTURED `HTTPAccessLogEntry` protos — the project's FIRST access-log sink that leaves the process, and its SECOND gRPC-client consumer after ext_authz.

The load-bearing facts that shape this brainstorm (every one citable on disk):

- **The `accesslog.Sink` interface is the exact seam a streaming sink slots into — no HCM/bootstrap shape change.** `internal/accesslog/accesslog.go:18` defines `Sink { Submit(r any); Close() error }` (a minimal 2-method contract; the file sink's `Submit` is non-blocking with drop-newest backpressure). `internal/filter/hcm/config.go:107` carries `accessLog []accesslog.Sink` on the Filter, threaded via `parseFilterWithCtx(... accessLogSinks ...)` (`:191`) and set at `:285`; the HCM emit hooks `emitAccessLog` (H1, `internal/filter/hcm/accesslog_emit.go:18`) + `emitAccessLogH2` (H2, `:43`) build a `Record` and `Submit` it to each sink. A gRPC ALS sink is a NEW `Sink` impl that joins the file sink in the `[]accesslog.Sink` — the emit paths, the sink list, and `cmd/envoy-go/main.go`'s sink loop are UNCHANGED in shape (the file sink is `accesslog.NewAsyncFileSink(path, dropped)`; the ALS sink is built analogously and appended).
- **The `accesslog.Record` 10-field struct maps cleanly into `HTTPAccessLogEntry`.** `internal/accesslog/accesslog.go:29` defines `Record { StartTime time.Time; Method, Path, Protocol string; ResponseCode int; BytesSent int64; Duration time.Duration; Authority, UserAgent, UpstreamHost string }` (the 10 plumbed operators per ADR-0067 Option B; 5 unplumbed operators emit `-` in the file formatter). These 10 fields map to the structured `envoy.data.accesslog.v3.HTTPAccessLogEntry`: `StartTime→common_properties.start_time`, `Method→request.request_method`, `Path→request.path`, `Protocol→protocol_version`, `ResponseCode→response.response_code`, `BytesSent→response.response_body_bytes`, `Authority→request.authority`, `UserAgent→request.user_agent`, `UpstreamHost→common_properties.upstream_remote_address`, `Duration→common_properties.time_to_last_downstream_tx_byte`. The gRPC sink does NOT use a `Formatter` (the file-sink text-format abstraction at `internal/accesslog/format.go:26`) — ALS emits a structured proto, not formatted text.
- **The gRPC client layer already exists — phase 44 composes a typed `ALSClient`, it does NOT build a gRPC client.** `internal/grpcclient/grpcclient.go:85` is `New(mgr *cluster.Manager) *Dialer`; `(*Dialer).DialContext(ctx, clusterName)` (`:105`) returns a `*grpc.ClientConn` dialed through the cluster manager's endpoint selection + TLS, with two PARSE-REJECT gates — the cluster must EXIST and `UseH2()` must be true (`:116`, "gRPC requires HTTP/2 framing"). `AuthClient` (`:157`, `NewAuthClient` `:178`) is the typed-wrapper precedent (ADR-0158 §Consequences explicitly names "future ProcessorClient, RateLimitClient" layering on the Dialer). Phase 44 adds a typed `ALSClient` wrapping the generated `accesslogv3.AccessLogServiceClient` stub over `Dialer.DialContext` — the exact AuthClient shape. The `grpc_service.envoy_grpc.cluster_name` config field maps onto the Dialer's cluster-name gate, and the Dialer's H2 requirement is exactly ALS's (a streaming RPC needs H2).
- **The ALS protos resolve at the project's pinned `go-control-plane/envoy v1.32.4` — ZERO new go.mod modules.** Verified in-session against the resolved module dir: the service proto `github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3` provides `StreamAccessLogsMessage`, `StreamAccessLogsResponse`, and the `AccessLogService` gRPC stub (`als.pb.go` + `als_grpc.pb.go`); the config proto `github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3` provides `HttpGrpcAccessLogConfig` + `CommonGrpcAccessLogConfig` + `TcpGrpcAccessLogConfig`; the entry proto is `github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3` (`HTTPAccessLogEntry`). `CommonGrpcAccessLogConfig` fields (pinned by proto tag): `log_name` (1), `grpc_service` (2), `buffer_flush_interval` (3, `*durationpb.Duration`), `buffer_size_bytes` (4, `*wrapperspb.UInt32Value`), `transport_api_version` (6, `ApiVersion` enum). The config-proto extension is registered by a blank-import in `internal/bootstrap/bootstrap.go` alongside the existing `access_loggers/file/v3` + `access_loggers/stream/v3` blank-imports (the file/stream imports already there).
- **The bootstrap `access_log` parser is the extension point — currently file-only, silently ignoring non-file types.** `internal/bootstrap/bootstrap.go:265` `parseOneAccessLog` checks `tc.GetTypeUrl() != fileAccessLogTypeURL` (`:271`, the file TypeURL pinned at `:145`) and silently ignores everything else per the ADR-0041 amendment (`:273`); the file path lands in `AccessLogConfig { Path string }` (`:152`). Phase 44 EXTENDS this: detect the `HttpGrpcAccessLogConfig` TypeURL → a new `ALSConfig` (cluster name + log_name + the 44.2/44.3 fields), keeping the file path silently distinct.
- **The streaming protocol is identifier-once client-streaming.** `AccessLogService.StreamAccessLogs(stream StreamAccessLogsMessage) returns (StreamAccessLogsResponse)` is client-streaming: the FIRST `StreamAccessLogsMessage` on a stream carries `identifier { node, log_name }`; subsequent messages carry only the `http_logs` oneof arm (`HTTPAccessLogsEntries { log_entry []HTTPAccessLogEntry }`). This is a wire-format pin both sides share (`reference_wire_format_both_sides_see_same_bytes`): the reference and subject both stream the SAME `StreamAccessLogsMessage` framing to the SAME receiver — the differential receiver decodes the identical proto from either side.
- **Determinism is per-entry STRUCTURED-FIELD, asserted at a receiver backend.** The differential is NOT stats-driven — it is a new BackendKind (an ALS gRPC receiver) that accumulates the streamed `HTTPAccessLogEntry`s; the driver polls-to-converge until N entries arrive (a release barrier, never a sleep — `reference_concurrency_differential_release_barrier`), then asserts the DETERMINISTIC structured-field subset cross-side EXACT (method/path/authority/protocol/response_code/user_agent, + the additional-headers at 44.3). The non-deterministic `AccessLogCommon` fields (timing, addresses, response_flags) are NOT asserted — the SPEC live-probe (D-ALS-FIELDS) pins exactly which fields BOTH sides populate deterministically (the `0006-access-log` field-discipline precedent: assert what is stable, ignore environment-specific operators).

The next sessions author the 44.1 SPEC then PLAN then IMPL, then 44.2's and 44.3's. Each SPEC executes its §10 empirical-pin obligations (D-ALS-*) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network` — a shared bridge with a receiver hostname reachable from BOTH containers; verify decode ran), and anchors its ADR §Context draft.

**Brainstorm session:** worktree `phase-44-brainstorm` off master (per `feedback_git_worktrees`). Substantive predecessor on master: the phase-43.2b IMPL squash `676e17cf` (the graceful GOAWAY-driven H2 upstream connection rotation; ADR-0254; row 43 done ⇒ Upstream-robustness family CLOSED), with the docs-only routing tip `492b8827` (the in-flight next-prompt tip-stamp fix) as the literal live tip. Counts at master tip: stat surface **1187** (H2 cluster; non-H2 **1183**), differential fixtures **82** (tail `0080-h2-goaway-rotation`), fuzzers **43**, BackendKind tail **38** (`H2GoawayResponder`), DECISIONS tail **ADR-0254** (next-free **ADR-0255**). ALL counts stay UNCHANGED at this brainstorm (docs-only).

**Brainstorm mode:** interactive with a live human. The user picked the family + subject + envelope + split via dialogue:

- **Q0-family** — the next family is **Observability** (the §9 heading), chosen over HTTP/3+QUIC (the most directly-composing follow-on to the 43.2 connection-pool seam, but introduces a heavy QUIC transport — likely the first new go.mod module in a long stretch), gRPC, and xDS (the largest structural lift; unblocks the most downstream work but a control-plane rewrite). Observability composes on the phase-06 stats + access-log baseline at the lowest risk.
- **Q0-subject** — the FIRST Observability-family row is **gRPC ALS** (the `AccessLogService` streaming sink), chosen over stats sinks (statsd/metrics_service), tracing (the largest — span model + propagation + sampling + a flake-prone span differential), and the tap filter. gRPC ALS is the cleanest extension of an abstraction the project ALREADY owns (the `accesslog.Sink`), it is canonical Envoy, and its differential is the most deterministic of the five (a structured-proto receiver, not a span/timing comparison).
- **Q1-envelope** — **Minimal + header-capture + buffering**: accept `log_name` + `grpc_service.envoy_grpc.cluster_name`; map the existing 10 `Record` fields into `HTTPAccessLogEntry`; PLUS `additional_request_headers_to_log` / `additional_response_headers_to_log` (extending `Record` + the H1/H2 emit hooks to capture arbitrary configured header values); PLUS honoring `buffer_size_bytes` + `buffer_flush_interval`. Chosen over the bare-minimal (10-field-map-only) interpretation. HTTP-logs-only (the `Record` is HTTP-shaped; `tcp_logs`/`TcpGrpcAccessLogConfig` strict-rejected/deferred) and transport-V3-only stay fixed by the project shape.
- **Q-split** — a **BY-CONCERN 3-leg split** under the ADR-0045 gate: **44.1 (core)** = config parse (log_name + envoy_grpc.cluster_name) + the `ALSClient` typed wrapper + the `GrpcAccessLogSink` (stream lifecycle + the 10-field structured mapping + a FIXED simple flush) + stats + main wiring + the differential receiver backend (ADR-0255); **44.2 (buffering)** = honor `buffer_size_bytes` + `buffer_flush_interval` (replace the fixed flush with size+interval triggers) + an extended differential prong (ADR-0256); **44.3 (header-capture)** = `additional_request_headers_to_log`/`additional_response_headers_to_log` via a `Record` extension + emit-hook capture, mapped into `request.request_headers`/`response.response_headers` + an extended differential prong (ADR-0257). Chosen over shipping flat (~350–370 prod LoC — right at the ADR-0045 gate) and over a 2-leg split (which would couple buffering with the core sink). Buffering is split out as its OWN leg because it is an independent flush-policy concern; header-capture is split out because it touches a DIFFERENT subsystem (the `Record` + the emit hooks, not the sink). Each leg lands an independently differential-provable feature.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0254 — especially ADR-0067 [the access-log 10-operator Record shape — the EXACT fields phase 44 maps], ADR-0158 [the `grpcclient.Dialer` + typed-wrapper layer — the ALSClient precedent], ADR-0041 [the silent-ignore set for unsupported `access_log` typed_config — phase 44 LIFTS gRPC ALS out of it], ADR-0016 [the proto blank-import registry discipline], ADR-0106/0045/0044/0080/0227), the as-built `internal/accesslog` package (`accesslog.go` [the `Sink` interface `:18`, the `Record` struct `:29`], `writer.go` [the `AsyncFileSink` bounded-channel + writer-goroutine `:26`, the constructors `:40`/`:47`], `format.go` [the Default formatter `:26`], `stats.go` [`RegisterDroppedCounter` → `server.accesslog_dropped` `:14`]), `internal/filter/hcm` (`config.go` [`accessLog` field `:107`, the threading `:191`/`:285`], `accesslog_emit.go` [`emitAccessLog` `:18` / `emitAccessLogH2` `:43`]), `internal/bootstrap/bootstrap.go` (`Load` `:195`, `parseAccessLogConfigs` `:234`, `parseOneAccessLog` `:265`, the file TypeURL `:145`, the silent-ignore `:271`, `AccessLogConfig` `:152`, the access-logger blank-imports `:34`), `internal/grpcclient` (`grpcclient.go` [`New` `:85`, `DialContext` `:105`, the `UseH2` gate `:116`, `AuthClient` `:157`, `NewAuthClient` `:178`], `doc.go` [the ADR-0158 cross-phase-reuse design]), `internal/stats/registry.go` (the Freeze discipline; `NewCounter`; `NewCounterIfAbsent` `:157`; `IsValidName`), `test/differential/fixture/fixture.go` (the `BackendKind` enum tail `H2GoawayResponder = 38` `:606`), and the resolved `go-control-plane/envoy v1.32.4` module (the ALS service/config/data protos). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–43 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/43-connection-pooling/BRAINSTORM.md` section-for-section, reframed for the FIRST row of a NEW family: a streaming `Sink` impl EXTENDING the phase-06.2 access-log abstraction (NOT a new logging subsystem), a typed `ALSClient` over the phase-18 `grpcclient.Dialer` (NOT a new gRPC client), a structured-proto differential against a receiver backend (NOT a stats/span comparison), a by-concern 3-leg split, and a release-barrier (sleepless) cross-side differential per leg. Per the context-isolation discipline, every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-25.

---

## 1. Mission and scope confirmation (44 — the parent + the 3 legs)

### 1.1 What phase 44 delivers as a self-contained whole (envelope: a streaming gRPC ALS sink + header-capture + buffering)

Phase 44 delivers a working `envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig` access logger: when an HCM `access_log[]` entry carries the ALS typed_config, envoy-go dials the configured `envoy_grpc.cluster_name` cluster (via `grpcclient.Dialer`), opens the `AccessLogService.StreamAccessLogs` client-streaming RPC, and streams each access-log event as a structured `HTTPAccessLogEntry` (the first message carrying `identifier { node, log_name }`). The 10 plumbed `Record` operators map into the structured proto; `additional_request_headers_to_log`/`additional_response_headers_to_log` capture arbitrary configured headers (44.3); `buffer_size_bytes`/`buffer_flush_interval` control flush batching (44.2). Byte-identical when no ALS `access_log` entry is configured (the file-only path is untouched — the full differential is the regression anchor).

### 1.2 What phase 44 does NOT deliver (forward to §8)

TCP ALS (`tcp_logs` / `TcpGrpcAccessLogConfig`) — strict-rejected, HTTP-only; `additional_request_trailers_to_log`; the `AccessLog.filter` sub-surface (status_code_filter / duration_filter / etc. — an access-log gating layer the file sink also defers); non-V3 `transport_api_version`; full `AccessLogCommon` population (response_flags, tls_properties, metadata, upstream_cluster, the rich timing fields beyond the one mapped); the OTLP access logger (`open_telemetry` — a sibling future Observability-family row); the `stdout`/`stderr` stream loggers; gRPC-side retry/reconnect-backoff tuning beyond a basic reconnect.

### 1.3 Phase-done as the FIRST Observability-family row landing (family OPENS, stays open)

Phase 44 is the FIRST row of the Observability family; landing it OPENS the family (the row registers `in-progress` at this brainstorm, flips `done` per-leg at each IMPL per ADR-0106). The family STAYS OPEN — gRPC ALS is one of several Observability surfaces (OTLP access log, tracing, stats sinks, the tap filter remain as future rows). There is NO family-close at phase 44.

### 1.4 ADR-0045 split readiness — a pre-authorized by-concern 3-leg split

The split is decided at this brainstorm (Q-split → 3 legs): 44.1 core / 44.2 buffering / 44.3 header-capture. The FINAL split gate is RE-CHECKED at each SPEC/PLAN with real LoC counts (the ADR-0045 re-check precedent); if 44.2 or 44.3 prove trivially small they may fold, but the by-concern boundaries are the planning default.

### 1.5 Seed-stub alignment + package placement

NO new package anticipated: the `GrpcAccessLogSink` lives in `internal/accesslog/` (alongside `AsyncFileSink`); the `ALSClient` typed wrapper lives in `internal/grpcclient/` (alongside `AuthClient`); the `ALSConfig` parse extends `internal/bootstrap/bootstrap.go`; the differential receiver backend lives under `test/`. The config-proto extension is registered via a blank-import in `bootstrap.go`.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for phase 44 (unlike the phase-11 local_ratelimit notes).

### 1.7 Phase 44's relationship to the existing seams (a `Sink` impl + a typed gRPC client + a bootstrap parse extension)

Phase 44 REUSES: the `accesslog.Sink` interface + the `[]accesslog.Sink` HCM plumbing (`config.go:107`/`:191`/`:285`) + the `Record` struct + the H1/H2 emit hooks (`accesslog_emit.go:18`/`:43`); the `grpcclient.Dialer` cluster-name→ClientConn layer + the AuthClient typed-wrapper pattern (`grpcclient.go:85`/`:105`/`:157`); the bootstrap `access_log` parse (`bootstrap.go:265`) + the proto blank-import registry (`:34`); the `stats.Registry` Freeze/`NewCounter` discipline (`registry.go`). It ADDS: the `GrpcAccessLogSink` (the streaming sink), the `ALSClient` (the typed stub wrapper), the `ALSConfig` parse arm, the ALS receiver BackendKind, and the `0081` fixture.

## 2. Design decisions

### 2.1 Family + subject confirmation: the Observability family opens with gRPC ALS *(Q0 → phase 44 row registered)*

The Observability family opens (over HTTP/3+QUIC, gRPC, xDS); the first row is gRPC ALS (over stats sinks, tracing, tap). Rationale: it composes on the phase-06.2 `Sink` abstraction + the phase-18 gRPC client at the lowest risk, it is canonical Envoy, and its structured-proto differential is the most deterministic of the candidate surfaces.

### 2.2 Envelope: Minimal + header-capture + buffering *(Q1 → the full accept set)*

Accept `log_name` + `grpc_service.envoy_grpc.cluster_name` + `additional_request_headers_to_log`/`additional_response_headers_to_log` + `buffer_size_bytes`/`buffer_flush_interval`; map the 10 Record fields; HTTP-only; transport-V3-only; strict-reject `tcp_logs`/non-V3.

### 2.3 Split axis: by-concern, 44.1 (core) → 44.2 (buffering) → 44.3 (header-capture) *(Q-split → ADR-0255 + ADR-0256 + ADR-0257)*

By-concern over flat or 2-leg. Buffering and header-capture are independent concerns layered onto the streaming core: buffering is a flush-policy concern internal to the sink; header-capture touches the `Record` + the emit hooks (a different subsystem). Each leg lands a working, differential-provable sink.

### 2.4 The sink lifecycle: lazy stream establishment, identifier-once, reconnect-on-error *(self-answered; pinned at SPEC, D-ALS-LIFECYCLE)*

The `GrpcAccessLogSink` mirrors `AsyncFileSink`'s bounded-channel + writer-goroutine shape (`writer.go:26`): `Submit` is non-blocking (drop-newest on channel-full, incrementing a dropped counter); the writer goroutine drains the channel, lazily establishes the `StreamAccessLogs` stream on first entry (sending the `identifier` once), batches entries into `StreamAccessLogsMessage.http_logs`, and on a stream error re-establishes (re-sending the identifier). `Close` is idempotent (`sync.Once`, the `AsyncFileSink` precedent).

### 2.5 The structured mapping: the 10 Record operators → HTTPAccessLogEntry *(self-answered; pinned at SPEC, D-ALS-FIELDS)*

The 10-field map (§ load-bearing facts). The SPEC live-probe pins which fields the reference populates from the same inputs (and the exact sub-message nesting — `common_properties` vs `request` vs `response`), and which non-deterministic fields to leave unasserted in the differential.

### 2.6 Deferred-policy posture: additive config; gRPC ALS LIFTED from the ADR-0041 silent-ignore set *(self-answered; pinned at SPEC, D-ALS-REJECT)*

The ALS typed_config moves from "silently ignored" (`bootstrap.go:273`) to "parsed + enforced"; `tcp_logs`/`TcpGrpcAccessLogConfig` + non-V3 `transport_api_version` are strict-rejected (ADR-0080); the `AccessLog.filter` field stays silently ignored (a deferred sub-surface, §8).

### 2.7 Stat surface hypothesis: a small `access_logs.grpc.*` counter set + incidental ALS-cluster upstream stats *(self-answered; SPEC pins, D-ALS-STATS)*

The ALS cluster is dialed as a normal cluster, so it incidentally emits the standard `upstream_cx_*`/`upstream_rq_*` cluster stats (a long-lived streaming connection). The sink-specific stats (dropped / stream-active / reconnect) are a small set under a `access_logs.grpc.*` or `server.access_logs.grpc.*` prefix (the EXACT names + scope SPEC-pinned against the reference). `server.accesslog_dropped` (`stats.go:14`) already exists for the file sink — whether the gRPC sink shares it or registers a distinct dropped counter is a SPEC pin.

## 3. Framework-survey result — a `Sink` impl + a typed gRPC client over existing seams + 0 new packages + 0 new go.mod modules (44 anticipated)

### 3.1 Framework: the `GrpcAccessLogSink` (new, in `internal/accesslog`) + the `ALSClient` (new, in `internal/grpcclient`) over existing seams *(per §1.7)*

### 3.2 NEW packages: NONE anticipated (the sink + client live in existing packages; the receiver backend lives under `test/`).

### 3.3 go.mod modules: anticipated ZERO new (44) — the ALS service/config/data protos resolve at the pinned `go-control-plane/envoy v1.32.4` (verified at brainstorm; re-pinned at each SPEC).

### 3.4 REUSES

The `accesslog.Sink` interface + `[]accesslog.Sink` HCM plumbing; the `Record` struct + the H1/H2 emit hooks; the `AsyncFileSink` bounded-channel + writer-goroutine + idempotent-`Close` pattern; the `grpcclient.Dialer` + the `AuthClient` typed-wrapper pattern (ADR-0158); the bootstrap `access_log` parse + the proto blank-import registry (ADR-0016); the `stats.Registry` Freeze/`NewCounter` discipline; the differential release-barrier + poll-the-gauge + wire-format-both-sides model.

## 4. Per-listener applicability — the EXISTING HCM `access_log[]` surface

The ALS sink is configured exactly where the file sink is: an HCM `access_log[]` entry (parsed at `bootstrap.go:234`/`:265`). A listener may carry BOTH a file sink AND a gRPC ALS sink (multiple `access_log[]` entries → multiple `Sink`s in the `[]accesslog.Sink`); each `Record` is `Submit`ted to every sink. The ALS sink additionally requires the `grpc_service` cluster to exist + be H2 (the Dialer gate) — a boot-time reject if absent.

## 5. Stat surface hypothesis — a small `access_logs.grpc.*` set + incidental ALS-cluster upstream stats (44.1)

### 5.1 Stat names (SPEC pins, D-ALS-STATS)

Candidate sink stats: a dropped counter (shared `server.accesslog_dropped` or a distinct `access_logs.grpc.*` name), a stream-active gauge, a reconnect/failure counter. The ALS cluster's incidental `upstream_cx_*`/`upstream_rq_*` are already-registered cluster stats (no surface delta from them). The EXACT new-name set + the `+N` surface delta are SPEC-pinned against `contrib-v1.37.2`.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

None anticipated beyond the documented deferrals (§8). The hard-cap/soft-cap distinctions of the Upstream-robustness family do not apply here.

### 5.3 Anticipated surface arithmetic

stat surface **1187 → 1187+N** at 44.1 (N = the new sink-specific counter/gauge names, SPEC-pinned — likely small, 1–3); UNCHANGED at 44.2/44.3 unless a buffering/header stat is reference-confirmed.

## 6. Differential fixture envelope — anticipated ONE directory `0081-grpc-access-log` (extended per leg)

### 6.1 Fixtures

`0081-grpc-access-log` lands at 44.1 (fixtures **82 → 83**); 44.2 (buffering) and 44.3 (header-capture) likely EXTEND `0081` with additional prongs/assertions rather than adding new directories (a SPEC/PLAN call per leg — the single-dir-with-prongs vs new-dir decision; note `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch, so a cross-side prong and a boot-reject prong cannot share a dir).

### 6.2 Total

fixtures **82 → 83** (anticipated; possibly **84**/**85** if 44.2/44.3 warrant their own dirs — SPEC-pinned per leg).

### 6.3 New BackendKind: anticipated ONE

A gRPC ALS receiver backend (anticipated BackendKind **39**) — an in-process `AccessLogServiceServer` that accepts `StreamAccessLogs`, accumulates the received `HTTPAccessLogEntry`s, and exposes them (count + fields) for the driver to poll/assert. The reference Envoy AND the subject both stream to this SAME receiver (the wire-format-both-sides discipline). Whether the receiver is in-process or out-of-process is a PLAN call (the ext_authz gRPC test-helper precedent — `test/helpers/extauthzgrpc`).

### 6.4 New fuzzer: anticipated NONE-to-ONE

A possible `HttpGrpcAccessLogConfig` parse fuzzer or a `Record→HTTPAccessLogEntry` mapping fuzzer (a SPEC/PLAN call). Reconcile the documented-vs-actual `^func Fuzz` count per `reference_fuzzer_count_docs_drift` if a fuzzer lands (the running total is currently 43 actual).

## 7. Anticipated ADRs — 3: ADR-0255 (44.1 core) + ADR-0256 (44.2 buffering) + ADR-0257 (44.3 header-capture)

- **ADR-0255** (44.1) — the gRPC ALS streaming sink: the `GrpcAccessLogSink` over the phase-06.2 `Sink` abstraction; the `ALSClient` typed wrapper over the `grpcclient.Dialer` (ADR-0158); the identifier-once `StreamAccessLogs` lifecycle + reconnect; the 10-field `HTTPAccessLogEntry` mapping; the ALS-config parse arm LIFTING gRPC ALS from the ADR-0041 silent-ignore set; the receiver BackendKind + `0081`. ANCHORS the Observability family.
- **ADR-0256** (44.2) — buffering: `buffer_size_bytes` + `buffer_flush_interval` flush triggers replacing the 44.1 fixed flush.
- **ADR-0257** (44.3) — header-capture: `additional_request_headers_to_log` / `additional_response_headers_to_log` via a `Record` extension + emit-hook capture → `request.request_headers` / `response.response_headers`.

## 8. Deferred items

TCP ALS (`tcp_logs` / `TcpGrpcAccessLogConfig`) — HTTP-only (strict-reject); `additional_request_trailers_to_log`; the `AccessLog.filter` sub-surface (status_code/duration/runtime/and_filter/or_filter); non-V3 `transport_api_version`; full `AccessLogCommon` population (response_flags, tls_properties, metadata, upstream_cluster, rich timing); the OTLP access logger (`open_telemetry`); the `stdout`/`stderr` stream loggers; gRPC reconnect-backoff tuning; the `grpc_service.google_grpc` (non-Envoy-gRPC) variant. Each is a future Observability-family row or a same-leg follow-on.

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 06.2 (access-log) deferred all non-file `access_log` sinks to the silent-ignore set (ADR-0041); phase 44.1 PICKS UP the gRPC ALS sink from that set. ADR-0158 (phase 18) explicitly named "future ProcessorClient, RateLimitClient" typed wrappers over the Dialer; the `ALSClient` is another such pickup. No other open deferral is discharged by phase 44.

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

Executed IN-SESSION at each leg's SPEC against `envoyproxy/envoy:contrib-v1.37.2` via `reference_docker_probe_bridge_network` (a shared bridge; an ALS receiver hostname reachable from BOTH containers; verify decode ran — `downstream_cx_rx_bytes_total > 0` analogue, i.e. the receiver actually received ≥1 entry from the reference):

- **D-ALS-FIELDS** (44.1) — which `HTTPAccessLogEntry` fields the reference populates from a basic request (the exact `common_properties`/`request`/`response` sub-message nesting for the 10 mapped operators); which fields are non-deterministic (timing, addresses, response_flags) and must stay unasserted; the `identifier { node, log_name }` content the reference sends (node id shape). NOTE: `protocol_version` is the `HTTPAccessLogEntry_HTTPVersion` ENUM, not a string — the `Record.Protocol` string ("HTTP/1.1"/"HTTP/2.0") needs a string→enum conversion (the 44.1 SPEC must pin the enum mapping, NOT assume a passthrough string).
- **D-ALS-STATS** (44.1) — the exact sink-specific stat names + scope (`access_logs.grpc.*` vs `server.*`) + the `+N` surface delta; whether the ALS cluster's incidental `upstream_cx_*`/`upstream_rq_*` move under a streaming RPC; whether the file-sink `server.accesslog_dropped` is shared or a distinct ALS dropped counter is registered.
- **D-ALS-LIFECYCLE** (44.1) — the `StreamAccessLogs` framing the reference emits (identifier on the first message only; entries batched into `http_logs`); reconnect behavior on receiver close; whether the reference sends a final flush on drain.
- **D-ALS-RECEIVER** (44.1) — the receiver BackendKind shape (in-process `AccessLogServiceServer` accumulating entries + a poll/assert surface); confirm the reference streams to a plain h2c gRPC receiver (no TLS) for the differential, or whether the cluster needs TLS.
- **D-ALS-BUFFER** (44.2) — the reference's `buffer_size_bytes`/`buffer_flush_interval` flush-trigger behavior (does it flush on EITHER threshold; the default values 16384 bytes / 1s); how to drive a deterministic flush in the differential (a small interval / a size threshold reached by K entries) such that the receiver converges.
- **D-ALS-HEADERS** (44.3) — the `additional_request_headers_to_log`/`additional_response_headers_to_log` semantics (case-insensitivity, missing-header behavior — omitted vs empty, multi-value joining) and the exact `HTTPRequestProperties.request_headers`/`HTTPResponseProperties.response_headers` map population.
- **D-ALS-SPLIT** (per leg) — the FINAL ADR-0045 split-gate re-check with real LoC counts (fold 44.2/44.3 if trivial); the single-dir-with-prongs vs new-dir fixture decision per leg.
- **D-ALS-FUZZER** (per leg) — whether a config-parse or mapping fuzzer lands; reconcile the documented-vs-actual `^func Fuzz` count.

## 11. Prior-phase lessons applied

- `reference_wire_format_both_sides_see_same_bytes` — the `StreamAccessLogsMessage` framing is shared; the receiver decodes the identical proto from reference + subject; adopt the reference framing verbatim (identifier-once).
- `reference_docker_probe_bridge_network` — the SPEC live probe + the differential use a shared bridge with a receiver hostname reachable from BOTH containers; verify the receiver actually received ≥1 entry from the reference (decode-ran proof).
- `reference_concurrency_differential_release_barrier` — the differential polls-to-converge until N entries arrive (a release barrier), never a `time.Sleep`.
- `reference_differential_run_selector` — `-run 'TestDifferential/0081'`, NOT a bare `'0081'`.
- `reference_differential_break_protocol_count1` — `-count=1` on every deliberate-break AND every `-race` run.
- `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch; a cross-side prong and a boot-reject prong need separate dirs.
- `reference_full_suite_race_after_background_mutator` — the sink's writer goroutine is a background mutator; after it lands, re-run the FULL package `-race`, NOT a `-run` subset.
- `reference_dynamic_stat_name_charset_guard` — if any stat segment is wire/config-derived (e.g. a log_name-keyed stat), guard it through `stats.IsValidName` before `NewCounterIfAbsent` (which panics on invalid names).
- `reference_fuzzer_count_docs_drift` — reconcile the running fuzzer total if a fuzzer lands.
- `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_push_to_origin` / `feedback_execution_style` / `feedback_pertask_gofmt_lint` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting` — the stage discipline for the SPEC/PLAN/IMPL sessions.

## 12. Section closeout

Phase 44 opens the Observability family with a streaming gRPC ALS sink, split by-concern into 44.1 (core streaming + the 10-field mapping + the receiver differential; ADR-0255), 44.2 (buffering; ADR-0256), and 44.3 (header-capture; ADR-0257). It composes on the phase-06.2 `Sink` abstraction + the phase-18 `grpcclient.Dialer` + the resolved `go-control-plane/envoy v1.32.4` ALS protos — ZERO new packages, ZERO new go.mod modules anticipated. Counts UNCHANGED at this brainstorm (stat **1187** / fixtures **82** / fuzzers **43** / BackendKind **38** / DECISIONS **ADR-0254**, next-free **ADR-0255**). Anticipated at the 44.1 IMPL: stat **1187+N** (small, SPEC-pinned) / fixtures **83** (`0081-grpc-access-log`) / fuzzers **43** (or +1) / BackendKind **39** (the ALS receiver) / DECISIONS **ADR-0255**. The row-44 Observability-family row registers `in-progress`; it flips `done` per-leg at each IMPL and the family STAYS OPEN.

**NEXT → the 44.1 (core) SPEC** (`superpowers:writing-plans` predecessor is the SPEC stage: execute the §10 D-ALS-{FIELDS,STATS,LIFECYCLE,RECEIVER} pins live against `contrib-v1.37.2` per `reference_docker_probe_bridge_network`; anchor the ADR-0255 §Context draft; a docs-only commit; a fresh worktree off master per `feedback_git_worktrees` if any code-adjacent probe artifacts land — the doc itself commits direct on master). The BRAINSTORM predecessor — `phase 43.2b (h2-connection-pool) IMPL done` (squash `676e17cf`; the Upstream-robustness family CLOSED).
