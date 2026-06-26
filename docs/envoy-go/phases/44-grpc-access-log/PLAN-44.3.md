# Phase 44.3 Implementation Plan — header-capture: make `additional_request_headers_to_log` / `additional_response_headers_to_log` (PARSE-ACCEPT-but-INERT through 44.1/44.2) LIVE — lift the two header-name lists into `ALSConfig` (lowercased) + a `Record` map-pair + a `captureHeaders` helper + emit-hook capture (incl. the response-header threading through the ~12 emit call sites) + the `buildHTTPAccessLogEntry` per-sink mapping + the `0083` header-capture differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** Make envoy-go's gRPC ALS sink honor the two `HttpGrpcAccessLogConfig` header-capture fields: `additional_request_headers_to_log` (field 2) and `additional_response_headers_to_log` (field 3). At 44.1/44.2 both fields parse-accept-but-do-nothing. 44.3 lifts each configured header NAME into `ALSConfig` (lowercased at parse), captures the named header VALUES at the HCM emit hooks (H1 + H2) into two new `Record` maps (`RequestHeaders` / `ResponseHeaders`, lowercase-keyed, comma-joined multi-value, absent-omitted), and maps each sink's configured subset into the entry's `request.request_headers` / `response.response_headers` `map<string,string>` proto fields — proven by the `0083-grpc-access-log-headers` differential (cross-side EXACT captured request maps + a `content-type` backend-origin response map + the absent-name OMIT proof) against `contrib-v1.37.2`.

**Architecture:** A localized extension of the phase-44.1/44.2 as-built (ADR-0255/0256): the parse arm (`internal/bootstrap`) lifts two `[]string` header-name lists into `ALSConfig` (lowercased); a shared `captureHeaders` helper + two `Record` maps (`internal/accesslog`) carry the snapshotted values; the two HCM emit hooks (`internal/filter/hcm/accesslog_emit.go`) capture the union of all sinks' configured names at request-finalization (the response headers threaded in via a NEW emit-hook parameter through the ~12 `connection.go`/`h2dispatch.go` call sites); `buildHTTPAccessLogEntry` + `NewGrpcAccessLogSink` gain the per-sink lists and filter the captured union down to each sink's subset. The capture union on the Filter is DERIVED from the already-threaded sinks (a `headerCaptureSink` interface) — ZERO new manager-chain plumbing params. ZERO new Go packages, ZERO new go.mod modules, NO new stat, NO new BackendKind, NO new fuzzer. Byte-identical when no ALS sink configures any additional header (the capture union is empty ⇒ the `Record` maps stay nil ⇒ the emitted proto is unchanged; the response param is threaded but ignored). The full differential is the regression anchor. ANCHORS ADR-0257; the 44.3 IMPL six-gate flips ROW 44 → `done` (the FINAL chartered leg; ADR-0106 per-leg, no parent rollup); the **Observability family STAYS OPEN** (OTLP / tracing / stats sinks / tap remain future rows).

**Tech Stack:** Go; the in-tree `internal/accesslog` sink subsystem (the 44.1/44.2 `GrpcAccessLogSink` + `buildHTTPAccessLogEntry` 10-field mapping); `internal/bootstrap` (the `parseGrpcAccessLog` arm + `ALSConfig`); `internal/filter/hcm` (the H1/H2 emit hooks + the `[]accesslog.Sink` plumbing + `filter_http.OrderedHeaders`); the resolved `go-control-plane/envoy v1.32.4` ALS protos (`HttpGrpcAccessLogConfig.GetAdditionalRequestHeadersToLog()` / `GetAdditionalResponseHeadersToLog()`; the entry-side `HTTPRequestProperties.request_headers` / `HTTPResponseProperties.response_headers` `map<string,string>`); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The gRPC Access Log Service (ALS) sink was BUILT at phase 44.1 (ADR-0255) and gained buffering at 44.2 (ADR-0256, squash `c4055b20`): a `GrpcAccessLogSink` (`internal/accesslog/grpcsink.go`) streams structured `HTTPAccessLogEntry` protos to an Envoy `AccessLogService.StreamAccessLogs` client-streaming RPC, accumulating entries into a buffer and flushing a batch on a size-OR-timer trigger. At request-finalization the HCM filter builds an `accesslog.Record` (`internal/accesslog/accesslog.go`) from the request/response primitives and `Submit`s it to every configured sink; the gRPC sink's writer goroutine maps the `Record` into the structured proto via `buildHTTPAccessLogEntry` (`internal/accesslog/mapping.go`, the 10-field mapping).

Two `HttpGrpcAccessLogConfig` config fields that name header keys to capture — `additional_request_headers_to_log` (field 2) and `additional_response_headers_to_log` (field 3) — are currently PARSE-ACCEPTED-but-INERT: the bootstrap parser reads the surrounding config but ignores these two lists.

Your job (phase 44.3) is to make those two fields LIVE. At parse time each configured header NAME is lifted into `ALSConfig` (lowercased — the map key is the lowercased name). At request-finalization the HCM emit hooks look up each configured header NAME (case-insensitively) in the request — AND in the response (the response headers are NOT currently threaded to the emit site, so this leg threads them in as a new emit-hook parameter) — and store `lowercase(name) → comma-joined-values` into two new `Record` maps (`RequestHeaders` / `ResponseHeaders`). The gRPC sink's `buildHTTPAccessLogEntry` then copies each of the SINK's own configured names out of those `Record` maps into the entry's `request.request_headers` / `response.response_headers` `map<string,string>` proto fields.

**The capture semantics (empirically pinned in SPEC §11 against real Envoy, 2026-06-25, `contrib-v1.37.2`):**
- **Key = LOWERCASED header name; value = verbatim** (config `X-Req-Mixed` → key `x-req-mixed`, value case-preserved) — AMEND-HDR-1.
- **Missing ⇒ OMITTED; present-but-empty ⇒ key-present, value `""`** (the discriminator is PRESENCE, not value emptiness) — AMEND-HDR-2.
- **Multi-value ⇒ COMMA-joined (no space) in WIRE ORDER** (`qv1` + `qv2` → `qv1,qv2`; reversed-order arrival → reversed join, NOT sorted) — AMEND-HDR-3.
- **Response source = the FINAL DOWNSTREAM (Envoy-mutated) response** — Envoy-synthesized headers (`server`/`date`/`x-envoy-*`) ARE captured but DIFFER side-to-side, so the differential captures BACKEND-ORIGIN headers only (`content-type`) — AMEND-HDR-4.
- **NO new stat** — the only sink stats stay `access_logs.grpc_access_log.{logs_written,logs_dropped}`; surface UNCHANGED at 1189 — AMEND-HDR-5.

The differential test harness boots BOTH the real reference Envoy (in Docker, `contrib-v1.37.2`) AND the in-process subject (envoy-go), drives the same traffic at both, and asserts equivalence. The receiver is a DRIVER-OWNED in-process `test/helpers/accessloggrpc` gRPC server that the proxy DIALS (NOT a runner BackendKind — `reference_differential_grpc_receiver_driver_owned`). The `0083` differential drives N requests carrying driver-controlled request headers (so the request-header capture is cross-side EXACT by construction) against a fixed-body backend that sets `content-type: text/plain` (so the response-header capture is cross-side EXACT for that one backend-origin header), polls the receiver to converge, and asserts the captured maps match both sides + the absent-name is OMITTED on both sides.

### Key source seams (verified at PLAN time against the tree at master `9df39735`; re-confirm line numbers before editing — files evolve)

- **`internal/bootstrap/bootstrap.go`** — the parse layer:
  - `type ALSConfig struct` (`:190`) — fields `ClusterName`, `LogName`, `BufferSizeBytes uint32`, `BufferFlushInterval time.Duration`. **ADD** `AdditionalRequestHeaders []string` + `AdditionalResponseHeaders []string` (lowercased at parse).
  - `parseGrpcAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error` (`:361`) — after the buffer reads + before the `append` at `:394`, **ADD** the two header-name reads (lowercase each name). The accessors are on the OUTER `HttpGrpcAccessLogConfig` message (fields 2/3), NOT on `common_config` / `CommonGrpcAccessLogConfig`: `cfg.GetAdditionalRequestHeadersToLog()` / `cfg.GetAdditionalResponseHeadersToLog()` (where `cfg` is the unmarshalled `*grpcalv3.HttpGrpcAccessLogConfig` — confirm the local variable name at IMPL; the buffer reads at `:380`-ish use `common := cfg.GetCommonConfig()`, so `cfg` is in scope). `strings` is needed for `strings.ToLower` — confirm it is imported (it is used elsewhere in the file; verify at IMPL).
  - The two fields are now CONSUMED (no longer parse-accept-inert); `additional_*_trailers_to_log` + `AccessLog.filter` STAY parse-accept-but-inert.
  - STRICT-REJECT roster UNCHANGED (ADR-0080) — header capture adds NO new reject (an unknown/garbage configured name simply never matches a present header ⇒ omitted; the proto `[]string` type bounds the input).
- **`internal/accesslog/accesslog.go`** — `type Record struct` (`:29`) carries the 10 plumbed operators. **ADD** two maps:
  ```go
  RequestHeaders  map[string]string // captured additional_request_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
  ResponseHeaders map[string]string // captured additional_response_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
  ```
  Populated ONLY by the emit hooks when the HCM filter has a non-empty capture union; otherwise left nil (the byte-stable common path). The `Default` file formatter IGNORES these maps (ADR-0067; its 10-operator format is unchanged) — only the gRPC sink reads them.
- **`internal/accesslog/mapping.go`** — `buildHTTPAccessLogEntry(rec *Record) *dataaccesslogv3.HTTPAccessLogEntry` (`:47`, the 10-field mapping). **EXTEND** the signature to `buildHTTPAccessLogEntry(rec *Record, reqHdrNames, respHdrNames []string)` and populate `e.Request.RequestHeaders` / `e.Response.ResponseHeaders` by looking up each of the SINK's configured names in `rec.RequestHeaders` / `rec.ResponseHeaders` (absent-from-`Record` ⇒ omitted from the proto map; empty list ⇒ leave the proto map nil — the 44.1/44.2 byte-identical behavior). The proto fields: `HTTPRequestProperties.RequestHeaders` (`map[string]string`, accessor `GetRequestHeaders()`) + `HTTPResponseProperties.ResponseHeaders` (accessor `GetResponseHeaders()`).
- **`internal/accesslog/grpcsink.go`** — `type GrpcAccessLogSink struct` (`:48`) + `NewGrpcAccessLogSink(client, logName, node, written, dropped, bufferSizeBytes, bufferFlushInterval)` (`:71`) + `newGrpcSinkWithCapacity` (`:77`). **ADD** `additionalRequestHeaders []string` + `additionalResponseHeaders []string` struct fields; **EXTEND** both constructor signatures with `additionalRequestHeaders, additionalResponseHeaders []string` (threaded through); the `run()` `flush()` call to `buildHTTPAccessLogEntry(rec)` at `:230` becomes `buildHTTPAccessLogEntry(rec, s.additionalRequestHeaders, s.additionalResponseHeaders)`. **ADD** the two `headerCaptureSink` interface methods (`CaptureRequestHeaderNames() []string` / `CaptureResponseHeaderNames() []string`) returning the two lists (so the HCM filter can derive the union — see below).
- **`internal/filter/hcm/accesslog_emit.go`** — the two emit hooks `emitAccessLog` (H1, `:18`) / `emitAccessLogH2` (H2, `:43`) + `h2UserAgent` (`:67`, the case-insensitive H2-header-scan idiom to generalize). **EXTEND** both hooks with a response-header parameter (`respHeaders filter_http.OrderedHeaders`); **ADD** the union-capture (request + response) into `rec.RequestHeaders` / `rec.ResponseHeaders` when the Filter's capture union is non-empty; **ADD** a shared `captureHeaders` call. Note `package hcm` already imports `filter_http` (used in `connection.go` for `ReconcileOrderedHeaders`).
- **`internal/filter/hcm/config.go`** — `type Filter struct` (`:91`) with `accessLog []accesslog.Sink` (`:110`) + `parseFilterWithCtx(... accessLogSinks []accesslog.Sink ...)` (`:191`) setting `accessLog: accessLogSinks` (`:285`). **ADD** two Filter fields `alsReqHeaderNames []string` + `alsRespHeaderNames []string`; **ADD** the union-derivation in `parseFilterWithCtx` (iterate `accessLogSinks`, type-assert each to the `headerCaptureSink` interface, accumulate the dedup'd lowercase union). NO new param to `parseFilterWithCtx` or the manager chain — the union is DERIVED from the already-threaded sinks (D-HDR-RECORD-CAPTURE-SCOPE resolution below).
- **`internal/filter/hcm/connection.go`** — the H1 emit call sites: `:322` (404 — no response, pass `nil`), `:442` (500 — no response, pass `nil`), `:536` (local-reply, `lrHeaders` OrderedHeaders in scope), `:637` (local-reply on DecodeData, `lrHeaders` in scope), `:715` (terminal action, `resp.Headers` OrderedHeaders in scope). **THREAD** the response-header arg through each (see D-HDR-RESPONSE-THREADING).
- **`internal/filter/hcm/h2dispatch.go`** — the H2 emit call sites: `:304` (no-match synthesized action, `resp.Headers` from `c.action`), `:386` (500 — no response, pass `nil`), `:470` (local-reply, `lrHeaders` OrderedHeaders in scope), `:516`/`:522` (matched-action response, `resp.Headers` in scope), `:550` (terminal, `resp` in scope). **THREAD** the response-header arg through each.
- **`cmd/envoy-go/main.go`** — the 44.2 sink-build block builds `accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval)` per `ALSConfig` (`:138`). **EXTEND** the call with `cfg.AdditionalRequestHeaders, cfg.AdditionalResponseHeaders`.
- **`internal/bootstrap/alsconfig_fuzz_test.go`** — `FuzzParseHttpGrpcAccessLogConfig` (the EXISTING parse fuzzer; seed corpus). **ADD** header-bearing seed-corpus entries; NO new `^func Fuzz`.
- **`test/helpers/accessloggrpc/accessloggrpc.go`** — `Entries()` (`:160`, a defensive snapshot of the accumulated `*HTTPAccessLogEntry`s). **NO change** — the `0083` driver reads `e.GetRequest().GetRequestHeaders()` / `e.GetResponse().GetResponseHeaders()` off the snapshot. The receiver STAYS a driver-owned helper the proxy DIALS — NO new BackendKind.
- **`test/fixtures/0081-grpc-access-log/`** — the differential precedent to COPY for `0083` (`driver/driver.go` + `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md`). The driver registers via `fixture.RegisterFixture`, owns the receiver (`accessloggrpc.NewAtAddr`), bakes the SAME ALS port into both YAMLs (`host.docker.internal` reference / `127.0.0.1` subject), fires N query-less `GET /health` requests, polls `Count()` to converge, asserts a 7-field subset per entry + the subject `logs_written` stat. The backend is `fixture.HTTPFixedBody`.
- **`test/differential/runner_test.go`** — blank-imports each fixture's `driver` package (the auto-discovery seam). **ADD** the `0083` driver import.

### Proto facts (verified at PLAN time; re-confirm at IMPL)

- `HttpGrpcAccessLogConfig.GetAdditionalRequestHeadersToLog() []string` (field 2) + `.GetAdditionalResponseHeadersToLog() []string` (field 3) — on the OUTER `HttpGrpcAccessLogConfig` message (NOT on `common_config`). Nil-safe (return nil/empty for an absent field).
- `HTTPRequestProperties.RequestHeaders map[string]string` (accessor `GetRequestHeaders()`) + `HTTPResponseProperties.ResponseHeaders map[string]string` (accessor `GetResponseHeaders()`) — the entry-side capture targets. An empty/nil map serializes to NO bytes (byte-stable when unset).
- `filter_http.OrderedHeaders` (`internal/filter/http/types.go:109`) is `[]HeaderField`; `.Get(name) string` does a case-insensitive single-value lookup (via `http.CanonicalHeaderKey`). For MULTI-value capture (AMEND-HDR-3) the helper must SCAN the slice collecting ALL matching values (the `Get` returns only the first) — see the `captureHeaders` design (Task 3).
- `http.Header` (the H1 request headers, `r.Header`) — `.Values(name) []string` does a canonical-case lookup returning all values; nil for absent, `[""]` for present-empty. Suits the `captureHeaders` lookup directly.
- H2 request headers are `req.Headers []hpack.HeaderField` (`internal/filter/hcm/h2/client.go:43`); scan case-insensitively (the `h2UserAgent` idiom, `strings.EqualFold`).
- The H1/H2 response headers at the action + local-reply emit sites are UNIFORMLY `filter_http.OrderedHeaders` (`resp.Headers` from the shared `ActionResponse`; `lrHeaders` from `chain.LocalReplyResponse()`). The true error sites (404/500 pre-response) have NO response collection.

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path (this plan executes from the worktree at `.worktrees/phase-44.3-header-capture-impl`); pin worktree-relative paths in every dispatch; the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD).
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0083'`, NEVER bare `'0083'` (which matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the sink's writer goroutine + the 44.2 ticker goroutine are background mutators; after the capture machinery lands run the FULL `internal/accesslog` package `-race`, NOT a `-run` subset.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing` / AMEND-ALS-3): assert the per-entry PAYLOAD aggregated across messages, NEVER stream/message/batch framing cross-side.
- **Receiver is driver-owned** (`reference_differential_grpc_receiver_driver_owned`): the ALS receiver is the in-process `test/helpers/accessloggrpc` the proxy DIALS — NO helper change, NO new BackendKind.
- **Docker bridge** (`reference_docker_probe_bridge_network`): the differential harness uses the shared-bridge + `host.docker.internal` (reference) / `127.0.0.1` (subject) reachability — the `0081`/`0082` precedent, carried verbatim.
- **Backend-origin response headers only** (AMEND-HDR-4): the `0083` response capture is `content-type` ONLY (the backend-origin `text/plain`); AVOID `server`/`date`/`x-envoy-*` (side-to-side divergent).

---

## D-question resolutions (the SPEC §12 D-HDR-* PLAN pins — settled here)

**D-HDR-RESPONSE-THREADING → both emit hooks gain a `respHeaders filter_http.OrderedHeaders` param (UNIFORM type, both codecs); action + local-reply sites pass the in-scope OrderedHeaders; the 404/500 pre-response error sites pass `nil`.**
- **Finding that simplifies the SPEC default:** the response-header collection in scope at the emit sites is UNIFORMLY `filter_http.OrderedHeaders` for BOTH codecs — `resp.Headers` (from the shared `ActionResponse`, used at `connection.go:704` H1-write and `h2dispatch.go` H2-write) AND `lrHeaders` (from `chain.LocalReplyResponse()`, OrderedHeaders). There is NO separate "H2 response-header representation"; the H2 response at the dispatch layer is the same `ActionResponse.Headers` ordered carrier (the raw `[]hpack.HeaderField` lives only inside the `h2` client package, below the emit layer). So a SINGLE param type covers both hooks.
- **Per-site source/`nil` disposition:**
  - H1 (`connection.go`): `:322` (404) → `nil`; `:442` (500) → `nil`; `:536` (local-reply) → `lrHeaders`; `:637` (local-reply-on-data) → `lrHeaders`; `:715` (terminal action) → `resp.Headers`.
  - H2 (`h2dispatch.go`): `:304` (no-match synthesized action) → `resp.Headers`; `:386` (500) → `nil`; `:470` (local-reply) → `lrHeaders`; `:516`/`:522` (matched action) → `resp.Headers`; `:550` (terminal) → `resp.Headers`. (Confirm each variable name in scope at IMPL — the line numbers drift.)
  - **Pass the in-scope OrderedHeaders wherever a response exists (action AND local-reply), `nil` only at the true error sites.** This is MORE faithful than the SPEC's "nil at local-reply" sketch (the local-reply response IS a real response, even if Envoy-synthesized) and costs nothing — the in-scope value is a slice header, free to pass. The `0083` differential exercises only the terminal-action path (a 200 from the backend), so the local-reply response capture is UNIT-tested only (a coverage boundary documented in PROGRESS, NOT differential-exercised).
- **Zero-alloc / byte-stability:** passing an OrderedHeaders slice header is free (no alloc). The emit hook reads it ONLY when `len(f.alsRespHeaderNames) > 0`; otherwise it is ignored and `rec.ResponseHeaders` stays nil. So the no-capture path (every existing fixture) is byte-identical and allocation-free.
- **REJECTED alternative:** threading the raw `[]hpack.HeaderField` for H2 (a second param type + a second lookup form). Unnecessary — the ordered carrier is already the response representation at the emit layer for both codecs.

**D-HDR-RECORD-CAPTURE-SCOPE → union-on-the-Filter, DERIVED from the already-threaded sinks (a `headerCaptureSink` interface) — NOT threaded as new manager-chain params.**
- The emit hook builds ONE `Record` shared across all sinks (`for _, s := range f.accessLog { s.Submit(rec) }`), so the `Record` maps MUST carry the UNION of all ALS sinks' configured names (each sink later filters to its own subset in `buildHTTPAccessLogEntry`). The Filter therefore needs the dedup'd lowercase union.
- **The union is DERIVED from the sinks the Filter already holds**, NOT threaded as two new `[]string` params through the 4-hop main→`NewManagerWithBaseDirAndAllowH2C`→`buildListenerRuntimeWithCtx`→`parseFilterWithCtx` chain (which would churn three unrelated listener-plumbing signatures). The `GrpcAccessLogSink` exposes its own configured lists via a small interface:
  ```go
  // in package hcm (structural — methods live on *accesslog.GrpcAccessLogSink):
  type headerCaptureSink interface {
      CaptureRequestHeaderNames() []string
      CaptureResponseHeaderNames() []string
  }
  ```
  In `parseFilterWithCtx`, after `accessLog: accessLogSinks`, iterate the sinks, type-assert each to `headerCaptureSink` (the `AsyncFileSink` does NOT implement it ⇒ skipped), and accumulate a dedup'd lowercase union into `f.alsReqHeaderNames` / `f.alsRespHeaderNames`. Empty union ⇒ nil slices ⇒ the byte-stable path.
- **Rationale:** single source of truth (the sink's own list drives BOTH the emit-hook union AND the per-sink `buildHTTPAccessLogEntry` filter — they cannot desync); zero new plumbing params; the `AsyncFileSink` is correctly excluded by the type-assert. This is the SPEC's "union on the Filter" default, refined to derive-from-sinks rather than thread-as-params.
- **REJECTED alternative:** per-sink capture in the emit hook (build a separate `Record` per sink). Wasteful (re-reads headers per sink) and breaks the shared-`Record`/`Submit` shape.

**D-HDR-SINK-FILTER → `buildHTTPAccessLogEntry(rec, reqHdrNames, respHdrNames []string)` signature extension; the sink holds its own lists and filters the captured union to its subset.**
- The `GrpcAccessLogSink` holds `additionalRequestHeaders` / `additionalResponseHeaders` (lowercase, set via the extended `NewGrpcAccessLogSink`). `run()`'s `flush()` calls `buildHTTPAccessLogEntry(rec, s.additionalRequestHeaders, s.additionalResponseHeaders)`. The function looks up each name in `rec.RequestHeaders` / `rec.ResponseHeaders` (which the emit hook populated with the UNION); present ⇒ copy `name → value` into the proto map; absent ⇒ omit. Empty list ⇒ leave the proto map nil (byte-identical to 44.1/44.2).
- For the common single-ALS-sink case the sink's list == the captured union, so the filter is a pass-through. The per-sink filter exists for the multiple-ALS-sink-with-divergent-lists case (UNIT-tested in Task 5; §2 SPEC non-purpose for the differential).
- **REJECTED alternative:** a post-mapping mutation step on the built entry (re-opening `e.Request`/`e.Response`). The signature extension is cleaner — the mapping owns the entry construction in one place.

**D-HDR-FUZZER-CORPUS → extend the EXISTING `FuzzParseHttpGrpcAccessLogConfig` seed corpus; NO new `^func Fuzz`; NO `captureHeaders` fuzzer.**
- The 44.1 parse fuzzer already feeds arbitrary bytes through `parseGrpcAccessLog`, which now reads the two header-name lists — so the header-name-parse path is fuzz-covered for free. Add header-bearing seed-corpus entries: mixed-case names, duplicate names, an empty-string name, a large name list, a name with control bytes. The invariant is unchanged: `parseGrpcAccessLog` NEVER panics and returns nil or a `bootstrap:`-prefixed error. Fuzzers STAY **44** (re-verify `^func Fuzz` == 44 at the completion task per `reference_fuzzer_count_docs_drift`).
- **NO `captureHeaders` mapping fuzzer:** the helper is TOTAL (every input maps to a deterministic output; no panic surface) and is EXHAUSTIVELY table-tested in Task 3 (lowercase-key / comma-join / omit-absent / empty-for-present-empty / mixed-case-lookup). A fuzzer would add no coverage the table tests do not already give.

**D-HDR-DIFFERENTIAL-DRIVE → N=8 sequential query-less GET; request capture {present-single, absent-OMIT, multi-value}; response capture `content-type` only; cross-side EXACT map equality aggregated over entries; poll-to-converge.**
- **Drive:** copy the `0081` shape — N=8 sequential query-less `GET /health` (`DisableKeepAlives`), Host `als.example`, User-Agent `als-probe/1`. Header capture is per-entry (NOT batch-sensitive like 44.2), so the sequential drive suffices; no coalescence requirement.
- **Request capture set (`additional_request_headers_to_log`, driver-controlled ⇒ cross-side EXACT by construction):**
  - `x-req-foo` — a present single-value header the driver sets on every request (e.g. `x-req-foo: bar`). Proves the basic capture.
  - `x-req-missing` — a configured-but-ABSENT name the driver NEVER sets. Proves the OMIT behavior cross-side (AMEND-HDR-2): the key is absent from `request.request_headers` on BOTH sides.
  - `x-req-multi` — a multi-value header the driver sets twice per request (`x-req-multi: m1` + `x-req-multi: m2`). Proves the comma-join (AMEND-HDR-3): both sides yield `x-req-multi: m1,m2`. **INCLUDE in the differential** — the driver fully controls the request, both proxies join comma-no-space in wire order (AMEND-HDR-3 pinned), so it is cross-side-robust. CONTINGENCY: if the multi-value join flakes cross-side at IMPL (e.g. an HTTP/1.1 request-header folding difference), DROP `x-req-multi` from the differential capture set and rely on the Task-3 `captureHeaders` table test as the live multi-value proof (the SPEC §12 hedge); record the decision in PROGRESS.
- **Response capture set (`additional_response_headers_to_log`):** `content-type` ONLY — the `HTTPFixedBody` backend sets `content-type: text/plain` (backend-origin), which BOTH proxies pass through verbatim ⇒ cross-side EXACT. AVOID `server`/`date`/`x-envoy-*` (AMEND-HDR-4: Envoy-synthesized, side-to-side divergent). **VERIFY at IMPL** that the `HTTPFixedBody` backend actually emits `content-type: text/plain` AND that envoy-go passes it through to the downstream response at the emit site (`grep` the `fixture.HTTPFixedBody` backend handler + confirm `resp.Headers` carries `content-type` at `connection.go:715`); if envoy-go does not surface a backend `content-type` in `resp.Headers`, switch the response capture to a different backend-origin header the backend sets, or have the backend set a custom `x-backend-tag` header — record the choice in PROGRESS.
- **Cross-side assertion (EXACT, aggregated over all N entries — AMEND-ALS-3 carries):** for each received entry, `request.request_headers` matches `{"x-req-foo":"bar", "x-req-multi":"m1,m2"}` (the absent `x-req-missing` OMITTED) on BOTH sides AND `response.response_headers` matches `{"content-type":"text/plain"}` on BOTH sides. PLUS the 44.1 7-field structured subset carries (`request.{request_method=GET, path=/health[query-less], authority=als.example, user_agent=als-probe/1}` + `response.{response_code=200, response_body_bytes=17}` + `protocol_version=HTTP11`) and `logs_written == N` on the subject. NO cross-side message/stream/batch-count assertion.
- **Poll-to-converge:** poll `srv.Count()` to `>= N` (the `0081` `pollCount` shape; NEVER `time.Sleep` — `reference_concurrency_differential_release_barrier`). Default `buffer_*` (16384/1s) ⇒ converge ≤ ~1–2s/side.
- **Per-side separation:** `Reset()` between sides (the `0081` idiom).

**D-HDR-SPLIT-FINAL → one leg (re-checked, ~135 prod LoC, well under the ADR-0045 soft gate).**
- Estimated prod LoC: the two `ALSConfig` fields + the parse-arm reads/lowercase ≈ 18; the two `Record` maps + the `captureHeaders` helper (lowercase-key / comma-join / omit-absent) ≈ 38; the emit-hook capture (H1 + H2) + the response-param threading through the ~12 call sites ≈ 42; the `buildHTTPAccessLogEntry` filter + the `NewGrpcAccessLogSink`/`newGrpcSinkWithCapacity` signature + the two `headerCaptureSink` methods + the `parseFilterWithCtx` union derivation ≈ 35; `main.go` pass-through ≈ 2. Total ≈ **135 prod LoC** — comfortably under the ADR-0045 soft gate. Ships as ONE leg (the FINAL chartered leg of the by-concern 3-leg split). No further split. Task 1 records the final re-check.

---

## File structure (decomposition locked here)

**Production (touched):**
- `internal/bootstrap/bootstrap.go` — MODIFY: the two `ALSConfig` header-name fields; the lowercased reads in `parseGrpcAccessLog`.
- `internal/accesslog/accesslog.go` — MODIFY: the two `Record` maps.
- `internal/accesslog/capture.go` — CREATE: the shared `captureHeaders` helper (lowercase-key / comma-join / omit-absent). (Or fold into `mapping.go` — see Task 3; CREATE keeps it focused.)
- `internal/accesslog/mapping.go` — MODIFY: the `buildHTTPAccessLogEntry` signature + the per-sink request/response map population.
- `internal/accesslog/grpcsink.go` — MODIFY: the two sink list fields; the `NewGrpcAccessLogSink`/`newGrpcSinkWithCapacity` signature; the `buildHTTPAccessLogEntry` call; the two `CaptureRequestHeaderNames`/`CaptureResponseHeaderNames` methods.
- `internal/filter/hcm/accesslog_emit.go` — MODIFY: the response-header param on both hooks; the union-capture into `rec.RequestHeaders`/`ResponseHeaders`; the shared lookup-construction.
- `internal/filter/hcm/config.go` — MODIFY: the two Filter fields; the `headerCaptureSink` interface; the union derivation in `parseFilterWithCtx`.
- `internal/filter/hcm/connection.go` — MODIFY: thread the response-header arg through the 5 H1 emit call sites.
- `internal/filter/hcm/h2dispatch.go` — MODIFY: thread the response-header arg through the ~6 H2 emit call sites.
- `cmd/envoy-go/main.go` — MODIFY: pass `cfg.AdditionalRequestHeaders`/`cfg.AdditionalResponseHeaders` to `NewGrpcAccessLogSink`.

**Test (created / modified):**
- `internal/bootstrap/bootstrap_test.go` — MODIFY: the header-name parse table tests (lowercased / absent / duplicate / mixed-case).
- `internal/bootstrap/alsconfig_fuzz_test.go` — MODIFY: header-bearing seed corpus.
- `internal/accesslog/capture_test.go` — CREATE: the exhaustive `captureHeaders` table tests.
- `internal/accesslog/mapping_test.go` — MODIFY: the `buildHTTPAccessLogEntry` per-sink filter tests (incl. divergent multi-sink lists; empty-list byte-stable).
- `internal/accesslog/grpcsink_test.go` — MODIFY: the constructor-signature update + the `CaptureRequestHeaderNames`/`CaptureResponseHeaderNames` method tests.
- `internal/filter/hcm/accesslog_emit_test.go` (or the existing emit test file) — MODIFY: the H1/H2 capture tests (present/absent/multi-value/empty; the no-capture byte-stable path; the response-param threading).
- `internal/filter/hcm/config_test.go` — MODIFY: the union-derivation test (multi-sink dedup; file-sink excluded; empty union).
- `test/helpers/accessloggrpc/` — NO change (the driver reads the captured maps off `Entries()`).
- `test/fixtures/0083-grpc-access-log-headers/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0083` driver package.

**Docs (completion task):**
- `docs/envoy-go/DECISIONS.md` (ADR-0257 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.3.md`.

---

## Task 1: Phase scaffolding — PROGRESS-44.3.md + baselines + the final ADR-0045 split re-check (D-HDR-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.3.md`

- [ ] **Step 1: Record the baseline counts.** Run and record the verbatim outputs in PROGRESS-44.3.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                          # expect 84 (incl letter-suffixed 0007a/0007b; tail 0082-grpc-access-log-buffering). NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS by 2 — use the glob form.
grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'   # expect 44
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go            # the BackendKind tail == 38
```
Baseline: stat surface **1189** (H2 cluster; non-H2 **1185**) / fixtures **84** / fuzzers **44** / BackendKind **38** / DECISIONS tail **ADR-0256** (next-free **ADR-0257**).

- [ ] **Step 2: Write the PROGRESS-44.3.md scaffold** — a header (phase 44.3 IMPL, the SPEC-44.3 reference, the worktree branch `phase-44.3-header-capture-impl`), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1189** (UNCHANGED — NO new header-capture stat, AMEND-HDR-5) / fixtures **85** (`0083-grpc-access-log-headers`) / fuzzers **44** (UNCHANGED) / BackendKind **38** (UNCHANGED — the receiver is driver-owned, REUSES `HTTPFixedBody`) / DECISIONS **ADR-0257**.

- [ ] **Step 3: Record the D-HDR-SPLIT-FINAL re-check** — note the ~135 prod-LoC estimate (the breakdown in the D-question section), confirm it sits well under the ADR-0045 soft gate, and that 44.3 ships as ONE leg (the FINAL chartered leg of the by-concern 3-leg split; the 44.3 IMPL six-gate flips ROW 44 → `done`, the Observability FAMILY STAYS OPEN). (Bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.3.md
git commit -m "phase 44.3 Task 1: PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-HDR-SPLIT-FINAL)"
```

---

## Task 2: Lift the two header-name lists into `ALSConfig` (lowercased) (`internal/bootstrap`) [TDD] + header-bearing fuzzer seed corpus (D-HDR-FUZZER-CORPUS)

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`, `internal/bootstrap/alsconfig_fuzz_test.go`

- [ ] **Step 1: Write the failing parse tests** in `bootstrap_test.go` (extend the existing grpc-ALS parse table — the `Load(strings.NewReader(yaml))` shape). Assert on `bs.ALSConfigs[0].AdditionalRequestHeaders` + `.AdditionalResponseHeaders` (`[]string`):
  - **both-present-lowercased**: a config with `additional_request_headers_to_log: ["X-Req-Foo", "x-req-multi"]` + `additional_response_headers_to_log: ["Content-Type"]` ⇒ `AdditionalRequestHeaders == []string{"x-req-foo", "x-req-multi"}` AND `AdditionalResponseHeaders == []string{"content-type"}` (lowercased at parse — AMEND-HDR-1).
  - **absent-both**: a config with neither field ⇒ both slices nil/empty (the no-capture path).
  - **mixed-case + duplicate preserved-order**: `additional_request_headers_to_log: ["X-A", "x-a", "X-B"]` ⇒ `[]string{"x-a", "x-a", "x-b"}` (lowercased; duplicates NOT de-duped at parse — the map write is idempotent, and the union-dedup happens at the Filter; order preserved).
  - **header-with-strict-reject**: a config with `additional_request_headers_to_log` set AND `google_grpc` ⇒ STILL errors on `google_grpc` (the reject arms run BEFORE the header reads; the header fields add no accept-path that bypasses a reject).

- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/bootstrap/ -run TestLoad -count=1` → Expected: FAIL (`ALSConfig` has no `AdditionalRequestHeaders`/`AdditionalResponseHeaders` field).

- [ ] **Step 3: Implement** in `bootstrap.go`. Extend the struct (`:190`):
```go
type ALSConfig struct {
	ClusterName               string
	LogName                   string
	BufferSizeBytes           uint32
	BufferFlushInterval       time.Duration
	AdditionalRequestHeaders  []string // additional_request_headers_to_log (field 2), lowercased at parse — AMEND-HDR-1
	AdditionalResponseHeaders []string // additional_response_headers_to_log (field 3), lowercased at parse — AMEND-HDR-1
}
```
In `parseGrpcAccessLog` (`:361`), after the buffer reads and before the `append` (`:394`), build the two lowercased lists (confirm the local var holding the unmarshalled `*grpcalv3.HttpGrpcAccessLogConfig` — the buffer reads use `common := cfg.GetCommonConfig()`, so `cfg` is in scope; `strings` import confirmed present):
```go
// additional_{request,response}_headers_to_log (fields 2/3, on the OUTER
// HttpGrpcAccessLogConfig — NOT common_config): the configured names are
// lowercased once at parse so the capture lookup + the map key are lowercase
// (AMEND-HDR-1). An absent/empty list ⇒ nil (the no-capture path). Duplicates
// are NOT de-duped here (idempotent map write; the Filter dedups the union).
reqHdrs := lowerAll(cfg.GetAdditionalRequestHeadersToLog())
respHdrs := lowerAll(cfg.GetAdditionalResponseHeadersToLog())
result.ALSConfigs = append(result.ALSConfigs, ALSConfig{
	ClusterName:               eg.GetClusterName(),
	LogName:                   common.GetLogName(),
	BufferSizeBytes:           bufferSizeBytes,
	BufferFlushInterval:       bufferFlushInterval,
	AdditionalRequestHeaders:  reqHdrs,
	AdditionalResponseHeaders: respHdrs,
})
```
Add the small helper (or inline a loop if the file convention prefers it):
```go
// lowerAll returns a new slice with each element lowercased, or nil for an
// empty/nil input (so the no-capture path stays nil — byte-stable).
func lowerAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}
```
Update the `ALSConfig` doc comment + the `Bootstrap.ALSConfigs` doc comment to note the two header-name fields are now CONSUMED (no longer parse-accept-but-inert); `additional_*_trailers_to_log` + `AccessLog.filter` STAY inert (deferred).

- [ ] **Step 4: Run to verify they pass.** Run: `go test ./internal/bootstrap/ -run TestLoad -count=1` → Expected: PASS.

- [ ] **Step 5: Header-bearing fuzzer seed corpus (D-HDR-FUZZER-CORPUS)** — in `alsconfig_fuzz_test.go`, add `f.Add(...)` seeds covering a `HttpGrpcAccessLogConfig` carrying `additional_request_headers_to_log` / `additional_response_headers_to_log` with: mixed-case names, duplicate names, an empty-string name, a large name list, a name with control bytes. (Marshal real `*grpcalv3.HttpGrpcAccessLogConfig` messages via the existing test helper if present, else raw wire bytes.) Add a comment noting the header-name lists are now part of the fuzzed surface (44.3). Then run:
```bash
go test ./internal/bootstrap/ -run 'FuzzParseHttpGrpcAccessLogConfig' -count=1
go test ./internal/bootstrap/ -fuzz 'FuzzParseHttpGrpcAccessLogConfig' -fuzztime 20s
grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'   # expect 44 (UNCHANGED)
```
Expected: PASS / no crashers; fuzzer count UNCHANGED at **44**. Record in PROGRESS-44.3.md.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/
git commit -m "phase 44.3 Task 2: lift additional_{request,response}_headers_to_log into ALSConfig (lowercased; AMEND-HDR-1) + header-bearing fuzzer seed corpus (D-HDR-FUZZER-CORPUS; fuzzers stay 44)"
```

---

## Task 3: The `Record` two-map extension + the shared `captureHeaders` helper (`internal/accesslog`) [TDD]

**Files:**
- Modify: `internal/accesslog/accesslog.go`
- Create: `internal/accesslog/capture.go`
- Test: `internal/accesslog/capture_test.go`

The `CaptureHeaders` helper is the single source of the AMEND-HDR-1/2/3 semantics (lowercase-key, comma-join, omit-absent, empty-for-present-empty), called from both H1 + H2 for both request + response. It takes a `lookup` closure so it is codec-agnostic. It is EXPORTED from `package accesslog` from the start (`CaptureHeaders`, not `captureHeaders`) because the HCM emit hook (`package hcm`, Task 4) calls it across the package boundary AND the gRPC sink may use it — one shared helper, no duplication. Do NOT define it unexported; the Task-4 cross-package call depends on the exported name.

- [ ] **Step 1: Write the failing exhaustive table tests** in `capture_test.go`. The helper signature (EXPORTED):
```go
func CaptureHeaders(names []string, lookup func(name string) ([]string, bool)) map[string]string
```
Table cases (a fake `lookup` backed by a `map[string][]string`):
  - **present single** — `names=["x-req-foo"]`, lookup returns `(["bar"], true)` ⇒ `{"x-req-foo":"bar"}`.
  - **absent omitted** — `names=["x-req-missing"]`, lookup returns `(nil, false)` ⇒ `{}` (empty map; key omitted — AMEND-HDR-2).
  - **present-empty kept** — `names=["x-req-foo"]`, lookup returns `([""], true)` ⇒ `{"x-req-foo":""}` (key present, empty value — AMEND-HDR-2).
  - **multi-value comma-joined wire-order** — `names=["x-req-multi"]`, lookup returns `(["m1","m2"], true)` ⇒ `{"x-req-multi":"m1,m2"}`; reversed `["ZZZ","AAA"]` ⇒ `{"x-req-multi":"ZZZ,AAA"}` (NOT sorted — AMEND-HDR-3).
  - **name already lowercase, value verbatim** — value case preserved (`"BarVal"` stays `"BarVal"`).
  - **empty names ⇒ nil map** — `names=nil` (or `[]`) ⇒ returns nil (the byte-stable no-capture sentinel; assert `== nil`, NOT just empty).
  - **mixed set** — `names=["x-a","x-missing","x-b"]` with `x-a` present, `x-missing` absent, `x-b` present-multi ⇒ `{"x-a":..., "x-b":"...,..."}` (two keys; the absent one omitted).
  - Assert the KEY is the name verbatim from `names` (the caller passes already-lowercased names — the helper does NOT re-lowercase; the lowercasing happened at parse, Task 2). (If the design prefers the helper to lowercase defensively, add a lowercased-key assertion and lowercase inside — but keep it consistent with Task 2's parse-time lowercasing; do NOT double-lowercase the value.)
  - The tests call the EXPORTED `accesslog.CaptureHeaders` (same package, so unqualified `CaptureHeaders` in `capture_test.go`).

- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/accesslog/ -run TestCaptureHeaders -count=1` → Expected: FAIL (undefined `CaptureHeaders`).

- [ ] **Step 3: Implement** `internal/accesslog/capture.go`:
```go
package accesslog

import "strings"

// CaptureHeaders builds the lowercase-keyed captured-header map for the named
// headers per AMEND-HDR-1/2/3: for each configured name, lookup returns the
// header's values and whether it is PRESENT. A present header (even with an
// empty value) becomes a map entry with the comma-joined values (no space, in
// the lookup's order — wire order). An absent header is OMITTED (the
// discriminator is presence, not value emptiness). The caller passes
// already-lowercased names (lowercased once at parse time). Returns nil when
// names is empty (the byte-stable no-capture sentinel — keeps Record maps nil).
func CaptureHeaders(names []string, lookup func(name string) ([]string, bool)) map[string]string {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		if vals, ok := lookup(name); ok {
			out[name] = strings.Join(vals, ",")
		}
	}
	return out
}
```
Extend `Record` (`accesslog.go:29`) with the two maps (the struct comment block from the Orientation seam):
```go
	RequestHeaders  map[string]string // captured additional_request_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
	ResponseHeaders map[string]string // captured additional_response_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
```

- [ ] **Step 4: Run to verify they pass.** Run: `go test ./internal/accesslog/ -run TestCaptureHeaders -count=1` → Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/accesslog.go internal/accesslog/capture.go internal/accesslog/capture_test.go
git commit -m "phase 44.3 Task 3: Record RequestHeaders/ResponseHeaders maps + the shared exported CaptureHeaders helper (lowercase-key/comma-join/omit-absent — AMEND-HDR-1/2/3)"
```

---

## Task 4: The emit-hook capture + the response-header threading + the union on the Filter (`internal/filter/hcm`) [TDD; the no-capture path BYTE-STABLE]

**Files:**
- Modify: `internal/filter/hcm/accesslog_emit.go`, `internal/filter/hcm/config.go`, `internal/filter/hcm/connection.go`, `internal/filter/hcm/h2dispatch.go`
- Test: `internal/filter/hcm/accesslog_emit_test.go` (or the existing emit test file), `internal/filter/hcm/config_test.go`

This task makes the capture LIVE at the emit hooks. The Filter gains the capture union (two `[]string` fields, set directly in unit tests here; wired from the sinks in Task 5). The emit hooks gain a response-header param and the union-capture. The capture is gated on a non-empty union ⇒ the no-capture path is byte-stable.

- [ ] **Step 1: Write the failing emit-capture tests** in the hcm emit test file. Construct a `Filter` with `alsReqHeaderNames` / `alsRespHeaderNames` set directly + a fake sink that records the submitted `*accesslog.Record`. Drive `emitAccessLog` (H1) and `emitAccessLogH2` (H2) and assert on the recorded `Record.RequestHeaders` / `Record.ResponseHeaders`:
  - **H1 request capture** — an `*http.Request` with `X-Req-Foo: bar` + two `X-Req-Multi` values; `alsReqHeaderNames=["x-req-foo","x-req-missing","x-req-multi"]` ⇒ `rec.RequestHeaders == {"x-req-foo":"bar","x-req-multi":"m1,m2"}` (absent omitted).
  - **H1 response capture** — pass an `OrderedHeaders` response carrying `Content-Type: text/plain`; `alsRespHeaderNames=["content-type"]` ⇒ `rec.ResponseHeaders == {"content-type":"text/plain"}`.
  - **H1 nil response (error site)** — pass `nil` response headers with `alsRespHeaderNames` non-empty ⇒ `rec.ResponseHeaders == nil` (or empty — assert it does not panic and omits all).
  - **H2 request capture** — an `h2.H2Request` whose `Headers` slice carries the configured names (the `h2UserAgent` scan generalized) ⇒ same shape as H1.
  - **H2 response capture** — pass an `OrderedHeaders` response ⇒ same shape.
  - **no-capture byte-stable** — `alsReqHeaderNames` AND `alsRespHeaderNames` both empty ⇒ `rec.RequestHeaders == nil` AND `rec.ResponseHeaders == nil` (assert `== nil`, the byte-stable sentinel; the response param is passed but ignored).
  - **present-empty** — a header present with empty value ⇒ key present, value `""`.

- [ ] **Step 2: Write the failing union-derivation test** in `config_test.go`: build two fake `headerCaptureSink` sinks with overlapping+divergent lists (e.g. sink A req `["x-a","x-b"]`, sink B req `["x-b","x-c"]`) + one `AsyncFileSink` (no capture methods), pass them as `accessLogSinks` to `parseFilterWithCtx` (or the union-derivation helper directly), and assert `f.alsReqHeaderNames` is the dedup'd union `{x-a,x-b,x-c}` (order-insensitive — compare as a set) and the file sink is excluded; an all-file-sink set ⇒ nil union.

- [ ] **Step 3: Run to verify they fail.** Run: `go test ./internal/filter/hcm/ -run 'TestEmitAccessLog|TestACaptureUnion' -count=1` → Expected: FAIL (signature mismatch + no capture).

- [ ] **Step 4: Implement.**
  - **`config.go`** — add to `Filter` (`:91`): `alsReqHeaderNames []string` + `alsRespHeaderNames []string` (the dedup'd lowercase capture union). Add the interface + the derivation:
```go
// headerCaptureSink is the optional capability a Sink advertises when it
// captures additional request/response headers (the gRPC ALS sink). The Filter
// derives the dedup'd union of all sinks' configured names so the emit hooks
// capture each named header once into the shared Record (each sink later
// filters to its own subset in buildHTTPAccessLogEntry). The file sink does
// not implement this, so it is excluded by the type-assert.
type headerCaptureSink interface {
	CaptureRequestHeaderNames() []string
	CaptureResponseHeaderNames() []string
}

// alsHeaderCaptureUnion returns the dedup'd lowercase union of the configured
// request/response header names across all header-capturing sinks. Both results
// are nil when no sink captures (the byte-stable no-capture path).
func alsHeaderCaptureUnion(sinks []accesslog.Sink) (req, resp []string) {
	var reqSet, respSet map[string]struct{}
	for _, s := range sinks {
		hc, ok := s.(headerCaptureSink)
		if !ok {
			continue
		}
		reqSet = addAll(reqSet, hc.CaptureRequestHeaderNames())
		respSet = addAll(respSet, hc.CaptureResponseHeaderNames())
	}
	return setKeys(reqSet), setKeys(respSet)
}
```
    (with tiny `addAll`/`setKeys` helpers — a set keyed by the already-lowercased name; nil when empty). In `parseFilterWithCtx`, after `accessLog: accessLogSinks`, set `f.alsReqHeaderNames, f.alsRespHeaderNames = alsHeaderCaptureUnion(accessLogSinks)`.
  - **`accesslog_emit.go`** — extend both hooks with `respHeaders filter_http.OrderedHeaders` (import `filter_http`):
```go
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{ /* the existing 10 fields ... */ }
	f.captureRecordHeaders(rec, reqHeaderLookupH1(r), respHeaderLookup(respHeaders))
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}
```
    Add a small `captureRecordHeaders` Filter method that only allocates when the union is non-empty (keeps the no-capture path nil/byte-stable):
```go
func (f *Filter) captureRecordHeaders(rec *accesslog.Record, reqLookup, respLookup func(string) ([]string, bool)) {
	if len(f.alsReqHeaderNames) > 0 {
		rec.RequestHeaders = accesslog.CaptureHeaders(f.alsReqHeaderNames, reqLookup)
	}
	if len(f.alsRespHeaderNames) > 0 && respLookup != nil {
		rec.ResponseHeaders = accesslog.CaptureHeaders(f.alsRespHeaderNames, respLookup)
	}
}
```
    **NOTE on the helper's package boundary:** `accesslog.CaptureHeaders` was EXPORTED at Task 3 precisely for this cross-package call (`package hcm` → `package accesslog`). No rename or cross-package edit is needed here — this task touches only `internal/filter/hcm/*` files. The lookups (the H2 + response scans MUST collect ALL matching fields in wire order — NOT first-match — to honor the AMEND-HDR-3 comma-join; the `h2UserAgent` idiom returns the first match, so generalize it to ACCUMULATE):
```go
// reqHeaderLookupH1 adapts http.Header to the captureHeaders lookup (canonical-
// case Values; absent ⇒ (nil,false); present-empty ⇒ ([""],true)).
func reqHeaderLookupH1(r *http.Request) func(string) ([]string, bool) {
	return func(name string) ([]string, bool) { v := r.Header.Values(name); return v, v != nil }
}
// reqHeaderLookupH2 scans the H2Request.Headers slice case-insensitively,
// collecting all matching values in wire order (generalizes h2UserAgent).
func reqHeaderLookupH2(req h2.H2Request) func(string) ([]string, bool) { /* EqualFold scan-collect */ }
// respHeaderLookup scans an OrderedHeaders carrier case-insensitively (Get
// returns only the first value; we need all for the comma-join). nil carrier ⇒
// a lookup that always returns (nil,false). Used for BOTH H1 and H2 responses.
func respHeaderLookup(oh filter_http.OrderedHeaders) func(string) ([]string, bool) { /* nil-safe EqualFold scan-collect */ }
```
    `emitAccessLogH2` mirrors this with `reqHeaderLookupH2(req)` + the same `respHeaderLookup`.
  - **`connection.go` / `h2dispatch.go`** — thread the response-header arg through each emit call site per D-HDR-RESPONSE-THREADING: action + local-reply sites pass the in-scope OrderedHeaders (`resp.Headers` / `lrHeaders`); the 404/500 error sites pass `nil`. (5 H1 sites + ~6 H2 sites — confirm each variable in scope.)

- [ ] **Step 5: Run to verify they pass + the no-capture byte-stability.** Run: `go test ./internal/filter/hcm/ -count=1` → Expected: PASS (the existing emit tests that pass `nil`/empty union still see nil `Record` maps — byte-stable). Confirm the FULL hcm package compiles + the existing non-ALS access-log tests are unaffected.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/
git commit -m "phase 44.3 Task 4: emit-hook header capture (H1+H2) + response-header threading through the ~12 emit sites + the capture union on the Filter (D-HDR-RESPONSE-THREADING/RECORD-CAPTURE-SCOPE); no-capture path byte-stable"
```

---

## Task 5: The sink mapping — `buildHTTPAccessLogEntry` per-sink filter + `NewGrpcAccessLogSink` signature + the `headerCaptureSink` methods (`internal/accesslog`) [TDD, full-package `-race`]

**Files:**
- Modify: `internal/accesslog/mapping.go`, `internal/accesslog/grpcsink.go`
- Test: `internal/accesslog/mapping_test.go`, `internal/accesslog/grpcsink_test.go`

- [ ] **Step 1: Write the failing tests.**
  - In `mapping_test.go` (the `buildHTTPAccessLogEntry` extension): a `Record` with `RequestHeaders={"x-a":"1","x-b":"2"}` + `ResponseHeaders={"content-type":"text/plain"}`:
    - sink names `req=["x-a","x-b"]`, `resp=["content-type"]` ⇒ `e.Request.RequestHeaders == {"x-a":"1","x-b":"2"}` AND `e.Response.ResponseHeaders == {"content-type":"text/plain"}`.
    - DIVERGENT sink subset `req=["x-a"]` (only x-a) ⇒ `e.Request.RequestHeaders == {"x-a":"1"}` (x-b filtered out — the per-sink filter; the multi-sink divergent case).
    - sink name configured but absent from the `Record` (`req=["x-a","x-zzz"]`, `Record` has only `x-a`) ⇒ `{"x-a":"1"}` (x-zzz omitted — never captured).
    - empty sink lists (`req=nil, resp=nil`) ⇒ `e.Request.RequestHeaders == nil` AND `e.Response.ResponseHeaders == nil` (byte-identical to 44.1/44.2 — assert nil).
    - nil `Record` maps (the no-capture path) + non-empty sink lists ⇒ nil proto maps (nothing to copy).
  - In `grpcsink_test.go`: the constructor-signature update (`NewGrpcAccessLogSink`/`newGrpcSinkWithCapacity` now take `additionalRequestHeaders, additionalResponseHeaders []string` — update existing call sites) + `CaptureRequestHeaderNames()`/`CaptureResponseHeaderNames()` return the configured lists; an end-to-end sink test that `Submit`s a `Record` with captured maps and asserts the flushed `HTTPAccessLogEntry` carries the filtered `request_headers`/`response_headers`.

- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/accesslog/ -run 'TestBuildHTTPAccessLogEntry|TestGrpcSink' -count=1` → Expected: FAIL (signature mismatch).

- [ ] **Step 3: Implement.**
  - **`mapping.go`** — extend the signature + add the per-sink population (a small `applyCapturedHeaders` so the lookup-and-copy is shared between request + response):
```go
func buildHTTPAccessLogEntry(rec *Record, reqHdrNames, respHdrNames []string) *dataaccesslogv3.HTTPAccessLogEntry {
	e := &dataaccesslogv3.HTTPAccessLogEntry{ /* the existing 10-field mapping ... */ }
	if m := filterCaptured(rec.RequestHeaders, reqHdrNames); m != nil {
		e.Request.RequestHeaders = m
	}
	if m := filterCaptured(rec.ResponseHeaders, respHdrNames); m != nil {
		e.Response.ResponseHeaders = m
	}
	// ... the existing upstream-address tail ...
	return e
}

// filterCaptured copies the sink's configured names out of the captured map
// (the emit-hook UNION) into a fresh map, omitting names the request/response
// did not carry. Returns nil when names is empty or nothing matched (so the
// proto map stays unset — byte-identical to the no-capture path).
func filterCaptured(captured map[string]string, names []string) map[string]string {
	if len(names) == 0 || len(captured) == 0 {
		return nil
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		if v, ok := captured[n]; ok {
			out[n] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
```
  - **`grpcsink.go`** — add the two struct fields; extend `NewGrpcAccessLogSink` + `newGrpcSinkWithCapacity` signatures (thread the two lists into the struct literal); change the `flush()` call `buildHTTPAccessLogEntry(rec)` → `buildHTTPAccessLogEntry(rec, s.additionalRequestHeaders, s.additionalResponseHeaders)`; add the two methods:
```go
func (s *GrpcAccessLogSink) CaptureRequestHeaderNames() []string  { return s.additionalRequestHeaders }
func (s *GrpcAccessLogSink) CaptureResponseHeaderNames() []string { return s.additionalResponseHeaders }
```

- [ ] **Step 4: Run to verify they pass + the FULL-package `-race`.** Run: `go test ./internal/accesslog/ -run 'TestBuildHTTPAccessLogEntry|TestGrpcSink' -count=1` → PASS. Then the FULL package race (the writer goroutine + the 44.2 ticker goroutine are background mutators — `reference_full_suite_race_after_background_mutator`): `go test ./internal/accesslog/ -race -count=1` → PASS, no race.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/mapping.go internal/accesslog/grpcsink.go internal/accesslog/mapping_test.go internal/accesslog/grpcsink_test.go
git commit -m "phase 44.3 Task 5: buildHTTPAccessLogEntry per-sink request/response header filter + NewGrpcAccessLogSink signature + CaptureRequest/ResponseHeaderNames (D-HDR-SINK-FILTER)"
```

---

## Task 6: Boot wiring pass-through (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go`

main.go is not unit-tested in isolation (the differential is its behavioral proof); the gate here is build + the `0083` fixture (Task 7).

- [ ] **Step 1: Implement** — extend the `NewGrpcAccessLogSink` call (`:138`) to pass the two new `ALSConfig` fields:
```go
sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval, cfg.AdditionalRequestHeaders, cfg.AdditionalResponseHeaders))
```
(No new import; the `ALSConfig` fields are `[]string`.) The HCM-filter union is DERIVED from the sinks in `parseFilterWithCtx` (Task 4), so NO additional main.go threading is needed — the sinks already flow into the listener manager via `AccessLogSinks: sinks` (`:251`).

- [ ] **Step 2: Build + boot-smoke.** Run: `go build ./... && echo BUILD_OK`. Then a manual boot-smoke against a hand-written bootstrap with a gRPC-ALS `access_log` carrying `additional_request_headers_to_log`/`additional_response_headers_to_log` pointing at a valid H2 cluster ⇒ boots clean (the Filter derives the union; no panic).

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 44.3 Task 6: boot wiring — pass cfg.AdditionalRequestHeaders/AdditionalResponseHeaders through to NewGrpcAccessLogSink"
```

---

## Task 7: The `0083-grpc-access-log-headers` differential fixture (cross-side EXACT captured request maps + the `content-type` response map + the absent-name OMIT proof; D-HDR-DIFFERENTIAL-DRIVE)

**Files:**
- Create: `test/fixtures/0083-grpc-access-log-headers/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0083` driver package)

Copy the WHOLE `0081-grpc-access-log/` directory as the starting point, then add the header-capture config + drive. The data-plane backend REUSES `HTTPFixedBody`. Same driver-owned-receiver lifecycle.

- [ ] **Step 1: VERIFY the backend-origin `content-type` (D-HDR-DIFFERENTIAL-DRIVE / AMEND-HDR-4).** Confirm the `fixture.HTTPFixedBody` backend emits `content-type: text/plain` (grep the backend handler in `test/differential/fixture/`) AND that envoy-go surfaces it in the downstream response at the emit site (a backend-origin response header copied through the router action into `resp.Headers`). If the backend does NOT set `content-type`, or envoy-go does not pass it through, switch the response capture to a header the backend definitely sets (or add a custom `x-backend-tag` to the backend / use a backend kind that sets one). Record the verified header in PROGRESS-44.3.md. (This de-risks the cross-side response-map assertion before authoring it.)

- [ ] **Step 2: Author the bootstraps** (copy `0081` envoy.yaml/envoy-go.yaml). The ONLY functional change vs `0081`: add the two header-capture fields to the `HttpGrpcAccessLogConfig` (the OUTER message, alongside `common_config` — NOT inside `common_config`) in BOTH YAMLs, and set `log_name: "0083"`:
```yaml
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig
                common_config:
                  log_name: "0083"
                  grpc_service:
                    envoy_grpc:
                      cluster_name: c_als
                additional_request_headers_to_log: ["x-req-foo", "x-req-missing", "x-req-multi"]
                additional_response_headers_to_log: ["content-type"]
```
(Keep the listener/route/cluster shape identical to `0081`; only `log_name` + the two header lists differ. Confirm the exact YAML indentation/structure against `0081`'s actual `typed_config` block.)

- [ ] **Step 3: Author `driver/driver.go`** (copy `0081`'s, rename `fixtureName = "0083-grpc-access-log-headers"`, `refListenerPort = 10083`, keep `numRequests = 8`). The KEY changes from `0081`:
  - **Request headers on every probe:** in `fireProbe`, after `req.Header.Set("User-Agent", probeUA)`, add `req.Header.Set("X-Req-Foo", "bar")` + `req.Header.Add("X-Req-Multi", "m1")` + `req.Header.Add("X-Req-Multi", "m2")` (do NOT set `X-Req-Missing` — it is the absent-OMIT proof). Keep the query-less `GET /health`, Host `als.example`.
  - **Capture-map assertions:** extend the per-entry assertion (the `0081` `assertEntries` 7-field subset, kept verbatim) with, for each entry on BOTH sides:
    - `e.GetRequest().GetRequestHeaders()` deep-equals `{"x-req-foo":"bar", "x-req-multi":"m1,m2"}` (the absent `x-req-missing` key NOT present — assert `_, ok := m["x-req-missing"]; !ok`).
    - `e.GetResponse().GetResponseHeaders()` deep-equals `{"content-type":"<verified value from Step 1>"}` (e.g. `text/plain`).
  - Keep `ensureServer`/`allocateALSPort`/`pollCount`/`scrapeFlatStats`/the template helpers verbatim. `BackendKind()` stays `fixture.HTTPFixedBody`. Keep the subject `logs_written == numRequests` stat assertion.
  - Add a `FIXTURE_0083_DUMP` env-gated diagnostic dumping the per-entry request/response header maps both sides (the `0081`/`0082` dump idiom) for debugging.

- [ ] **Step 4: Author `expectations.yaml` + `README.md`.** `expectations.yaml`: mirror `0081`'s (the cross-side byte-equal data-plane output — N×`status=200`). `README.md`: copy `0081`'s, then update — the header-capture purpose (the two header lists LIVE; lowercase key / verbatim value / comma-join / omit-absent); the D-HDR-DIFFERENTIAL-DRIVE drive (N=8 sequential, the `x-req-foo`/`x-req-multi`/`x-req-missing` request set + `content-type` response); the AMEND-HDR-4 "backend-origin response headers only — avoid `server`/`date`/`x-envoy-*`" note; the host-reachability table (`host.docker.internal` reference / `127.0.0.1` subject).

- [ ] **Step 5: Register + run the fixture isolated.** Add the `0083` driver blank-import to `runner_test.go` (the `0081`/`0082` import lines are the template). Run (the correct selector — `reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0083' -count=1` → Expected: PASS (both sides stream the same captured maps; `x-req-missing` omitted both sides; `logs_written == N`). Confirm fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **85** (the glob form). If the `x-req-multi` cross-side join flakes, apply the D-HDR-DIFFERENTIAL-DRIVE contingency (drop `x-req-multi` from the differential, keep it as the Task-3 unit proof) and record it.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0083-grpc-access-log-headers/ test/differential/runner_test.go
git commit -m "phase 44.3 Task 7: 0083-grpc-access-log-headers differential — cross-side EXACT captured request maps (x-req-foo/x-req-multi, x-req-missing OMITTED) + content-type response map (fixtures 84 → 85)"
```

---

## Task 8: `0083` deliberate-break proofs + flake gate + the FULL-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0083` FAILS (proving the assertion is live), then `git restore` it:
  - (a) **The capture bites (THE load-bearing 44.3 break)** — in `accesslog_emit.go`, make `captureRecordHeaders` a no-op (skip populating `rec.RequestHeaders`/`ResponseHeaders`), OR in `mapping.go` make `filterCaptured` always return nil. ⇒ the cross-side `request.request_headers` / `response.response_headers` map assertions must FAIL (the maps go empty).
  - (b) **The lowercase-key bite** — in Task 2's `lowerAll` (or the capture), skip the `strings.ToLower`. ⇒ the captured key becomes the mixed-case wire name on the subject (e.g. `X-Req-Foo`) while the reference lowercases ⇒ the cross-side map-key assertion must FAIL. (If the driver sets already-lowercase request header names, force a mixed-case config name in the YAML for this break to bite — or rely on the `content-type` config name `Content-Type` if you author it mixed-case; document which.)
  - (c) **The comma-join bite** — in `captureHeaders`, change `strings.Join(vals, ",")` to `strings.Join(vals, ";")`. ⇒ `x-req-multi` becomes `m1;m2` on the subject ⇒ the cross-side assertion must FAIL. (Skip/adapt if `x-req-multi` was dropped per the Task-7 contingency.)
  - (d) **The omit-absent bite** — in `captureHeaders`, store absent names as `""` instead of omitting. ⇒ `x-req-missing` appears as `{"x-req-missing":""}` on the subject ⇒ the cross-side "key absent both sides" assertion must FAIL.
  - (e) **The aggregated 7-field payload still bites** — break `buildHTTPAccessLogEntry` to drop `UserAgent`. ⇒ the cross-side `user_agent` assertion must FAIL (proves the 44.1 subset still live alongside the new maps).
  Run each: `go test ./test/differential/ -run 'TestDifferential/0083' -count=1` ⇒ expect FAIL, then restore ⇒ expect PASS. Record each break+restore in PROGRESS-44.3.md (the live-assertion proof).

- [ ] **Step 2: Flake gate** — 20 consecutive green runs:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0083' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. A transient `subject ready: EOF` is the startup-race flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run that single run; NOT a `0083` regression.

- [ ] **Step 3: FULL `internal/accesslog` package `-race`** (the sink writer goroutine + the 44.2 ticker goroutine — `reference_full_suite_race_after_background_mutator`): `go test ./internal/accesslog/ -race -count=1` → Expected: PASS, no race.

- [ ] **Step 4: Commit the PROGRESS update** (break-proofs + flake + race recorded)
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.3.md
git commit -m "phase 44.3 Task 8: 0083 deliberate-break proofs (disable-capture/lowercase/comma-join/omit-absent/payload) + 20/20 flake + full-package -race"
```

---

## Task 9: Full 85-dir differential + six-gate + ADR-0257 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 44 → done; Observability family STAYS OPEN)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.3.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                 # clean
go build ./...                               # ok
go test ./... -count=1                       # full unit + the 85-dir differential
go test ./internal/accesslog/ -race -count=1 # the background-mutator race gate
```
Expected: all green. The full differential is the byte-stability regression anchor — no non-ALS fixture should move; `0081`/`0082` stay green (they configure NO additional headers ⇒ the capture union is empty ⇒ their `Record` maps stay nil ⇒ their emitted protos are byte-identical). CONFIRM this explicitly.

- [ ] **Step 2: ADR-0257 §Decision/§Consequences** — land them in DECISIONS.md beneath the §Context already drafted at SPEC §13 (ADR-0044). §Decision: lift `additional_{request,response}_headers_to_log` into `ALSConfig` (lowercased); the `Record` two-map extension + the `captureHeaders` helper (lowercase-key / comma-join / omit-absent / empty-for-present-empty — AMEND-HDR-1/2/3); the emit-hook union capture + the response-header threading through the ~12 emit sites (uniform `OrderedHeaders` param, nil at error sites); the capture union DERIVED from the sinks (the `headerCaptureSink` interface — no new manager-chain params); `buildHTTPAccessLogEntry` per-sink filter; the `0083` differential. §Consequences: NO new stat/BackendKind/fuzzer/package/module; the no-capture path byte-stable; `additional_*_trailers_to_log` + `AccessLog.filter` STAY parse-accept-inert; the response capture reads the final downstream response (backend-origin headers cross-side-assertable, Envoy-synthesized not — AMEND-HDR-4); row 44 → `done` (FINAL leg); the Observability family STAYS OPEN.

- [ ] **Step 3: BEHAVIOR_CONTRACT.md** — update the `### Access log — gRPC Access Log Service (ALS) streaming sink` block per SPEC §9: ADD the header-capture paragraph (the sink captures the configured request/response header VALUES at the HCM emit hooks into `request.request_headers`/`response.response_headers`; lowercase key / verbatim value; missing ⇒ OMITTED, present-empty ⇒ empty-string; multi-value ⇒ comma-joined wire-order; the response capture reads the final downstream response so the differential captures backend-origin headers only — AMEND-HDR-1..4). MOVE `additional_{request,response}_headers_to_log` from the parse-accept-but-inert list into the supported set (leaving `additional_*_trailers_to_log` + `AccessLog.filter` deferred). The stat-surface block STAYS 1189 (AMEND-HDR-5).

- [ ] **Step 4: STATE.md + ROADMAP.md** — STATE active-phase → `phase 44.3 (grpc-access-log) IMPL done`; the count figures → stat **1189** / fixtures **85** / fuzzers **44** / BackendKind **38** / DECISIONS **ADR-0257**. ROADMAP row 44 → **done** (the FINAL chartered leg lands; per-leg, ADR-0106, `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN (OTLP / tracing / stats sinks / tap remain future rows). Set the next action → the next Observability-family row's BRAINSTORM/SPEC (or whatever STATE's roadmap sequence dictates next).

- [ ] **Step 5: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'` == **44** (UNCHANGED) and that the documented running total stays 44 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md / PROGRESS-44.3.md consistently.

- [ ] **Step 6: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 44.3 (grpc-access-log header-capture) IMPL: ADR-0257 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 44 done; Observability family STAYS OPEN); stat 1189 / fixtures 85 / fuzzers 44 / BackendKind 38"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 44.3 (grpc-access-log header-capture) IMPL: make additional_{request,response}_headers_to_log LIVE — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 44.3 IMPL squash and route the next session to the next Observability-family step (per STATE's roadmap sequence).
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1189** (H2 cluster; non-H2 **1185**) — UNCHANGED (NO new header-capture stat, AMEND-HDR-5) / fixtures **85** (tail `0083-grpc-access-log-headers`) / fuzzers **44** (UNCHANGED) / BackendKind **38** (UNCHANGED — the ALS receiver is driver-owned, REUSES `HTTPFixedBody`) / DECISIONS **ADR-0257**. ZERO new go.mod modules (`go mod tidy -diff` EMPTY). ZERO new `internal/` packages (the `capture.go` file lives in the existing `accesslog` package). ROADMAP row 44 → `done`; the Observability FAMILY STAYS OPEN.
