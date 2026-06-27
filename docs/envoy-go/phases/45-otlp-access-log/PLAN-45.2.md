# Phase 45.2 Implementation Plan — the OTLP access-log operator engine: lift `body` / `attributes` / `resource_attributes` from the 45.1 STRICT-REJECT to LIVE command-operator-templated content over a NEW small operator engine (`internal/accesslog/otlpformat.go`) curated to the `Record`-mapped operator subset (STRICT-REJECT every operator outside it + every non-string/kvlist/array `AnyValue` value-type at parse), extending `buildLogRecord`/`buildResource` + the sink threading, proven by the `0085-otlp-access-log-operators` differential — the FINAL leg of the by-concern 2-leg OTLP split (ANCHORS ADR-0259; flips ROADMAP row 45 → `done`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** When an HCM `access_log[]` `OpenTelemetryAccessLogConfig` carries a `body` (an `AnyValue`) and/or `attributes` (a `KeyValueList`), envoy-go substitutes the Envoy command operators (`%REQ(:METHOD)%`, `%RESPONSE_CODE%`, …) against each access-log `Record` and emits the result as `LogRecord.body` / `LogRecord.attributes`; a configured `resource_attributes` (a `KeyValueList`) is emitted as extra `Resource.attributes` (LITERAL — request operators pass through verbatim) appended after the 4 built-in labels and surviving `disable_builtin_labels` — proven cross-side EXACT on the deterministic operators (record body string/structure + attribute keys/values incl. nested `kvlist`/`array` + the merged Resource attributes) by the `0085-otlp-access-log-operators` differential against `contrib-v1.37.2`, with subject-side unit tests for the `disable_builtin_labels` survival, the unsupported-operator boot-reject, and the bad-`AnyValue`-value-type boot-reject.

**Architecture:** 45.1 shipped the LEAN built-in `LogRecord` (a bare `time_unix_nano` + the 4 `Resource` built-in labels) and STRICT-REJECTED any configured `body`/`attributes`/`resource_attributes`. 45.2 lifts that reject by adding a NEW small two-phase command-operator engine in `internal/accesslog` (`otlpformat.go`): a registry from the curated operator token → a `func(*Record) string` extractor; a **compile-at-boot** phase (`CompileOTLPTemplate` `%…%` scanner + `CompileOTLPValue` `AnyValue`-tree walker, STRICT-REJECTing unknown operators + non-string/kvlist/array value-types); and a **substitute-per-record** phase (`(*OTLPValueTemplate).Eval(rec)` emitting `stringValue` leaves, preserving `kvlist`/`array` structure). The bootstrap parse arm calls the compile helper (D-OTLP-2-COMPILE-SITE: bootstrap-compiles-at-parse — a clean `bootstrap.Load` boot error, ONE operator implementation, no import cycle), so the compiled types are EXPORTED (`OTLPTemplate`/`OTLPValueTemplate`/`OTLPAttrTemplate`) for the `OTLPConfig` fields; `buildLogRecord`/`buildResource`/`buildExportRequest` + the sink threading grow to carry them. ZERO new Go packages, ZERO new go.mod modules. Byte-identical when no OTLP entry carries `body`/`attributes`/`resource_attributes` (the 45.1 built-in path is untouched — the `0084` fixture is the regression anchor). ANCHORS ADR-0259; its IMPL flips ROADMAP row 45 → `done`; the Observability family STAYS OPEN.

**Tech Stack:** Go; the in-tree `internal/accesslog` sink subsystem (the 45.1 `OTLPAccessLogSink` + `otlpmapping.go` + the `Record` struct + the `format.go` `Default()` operator SEMANTICS source); `internal/bootstrap` (the 45.1 `parseOpenTelemetryAccessLog` arm + the ADR-0016 proto blank-import registry); the resolved `go-control-plane/envoy v1.32.4` `OpenTelemetryAccessLogConfig` config proto; the `go.opentelemetry.io/proto/otlp v1.0.0` OTLP proto module (`common/v1` `AnyValue`/`KeyValue`/`KeyValueList`/`ArrayValue` — ALREADY direct-imported since 45.1); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The access-log subsystem (`internal/accesslog`) has THREE sink types — `AsyncFileSink`, `GrpcAccessLogSink`, and (phase 45.1) `OTLPAccessLogSink`, which BATCHES OpenTelemetry `LogRecord` protos to a `LogsService` over the UNARY `Export` RPC. At 45.1 each emitted `LogRecord` carried ONLY a `time_unix_nano` timestamp plus four `Resource`-level labels (`log_name`/`zone_name`/`cluster_name`/`node_name`); the access logger's `body`, `attributes`, and `resource_attributes` config fields were STRICT-REJECTED at parse time (a boot error). Your job (45.2) is to make those three fields LIVE.

In Envoy, an OTLP access logger's `body` and `attributes` are templated with **command operators** — the same `%REQ(:METHOD)%`, `%RESPONSE_CODE%`, `%START_TIME%`, … substitution-format tokens the file access logger uses — evaluated PER access-log record. The reference Envoy was probed live (SPEC-45.2 §11, contrib-v1.37.2, 2026-06-26): `body: "%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE%"` emits `LogRecord.body.stringValue = "GET /api/widgets?x=1 HTTP/1.1 200"`; each `attributes` value resolves per-record; `resource_attributes` are built ONCE per logger so request operators in them pass through LITERALLY (`"%REQ(:AUTHORITY)%"` emitted verbatim). Envoy's operator set is vast (dozens of `StreamInfo` fields); envoy-go's proto-agnostic `Record` carries ~10 scalar fields, so 45.2 supports a CURATED subset — exactly the operators the `format.go` `Default()` file-formatter already maps to `Record` fields — and STRICT-REJECTS every operator outside it at parse (the envoy-go-strict mirror of the reference's own boot-reject of a truly-invalid operator). All substituted output is `stringValue` (no typed-int promotion); config `AnyValue` values must be `string_value`/`kvlist_value`/`array_value` (a `kvlist`/`array` is preserved structurally with each string leaf substituted) — an `int_value`/`bool_value`/`double_value`/`bytes_value` config value BOOT-REJECTS, mirroring the reference.

The differential test harness boots BOTH the real reference Envoy (in Docker, `contrib-v1.37.2`) AND the in-process subject (envoy-go) against equivalent bootstraps, drives the same traffic at both, and asserts equivalence. For this family BOTH sides export their `LogRecord`s to the SAME driver-owned in-process OTLP `LogsService` receiver (`test/helpers/otlplogs`); the driver asserts the aggregated per-record PAYLOAD (the substituted `body` + `attributes` + the merged `Resource.attributes`) cross-side — NOT `Export`-call framing, which legitimately varies (`reference_streaming_sink_differential_framing`). The reference (Docker) reaches the receiver via `host.docker.internal`; the subject via `127.0.0.1`. The `0085` fixture uses a query-less path so `%REQ(:PATH)%` is cross-side EXACT under H1 (envoy-go's H1 `Record.Path` strips the query; the reference keeps it — AMEND-OPS-6).

### Key source seams (verified at PLAN time against the tree at master `c533a9d9` — re-confirm line numbers before editing; files evolve)

- **`internal/accesslog/format.go`** — the operator SEMANTICS source (NOT a reusable engine: `Default()` is a HARD-CODED single-format function, its 15 operators inlined at fixed positions; `log_format`/`format_string` is REJECTED per ADR-0067). 45.2 REUSES its per-operator `Record`-field mapping verbatim:
  - `%START_TIME%` → `r.StartTime.UTC().Format("2006-01-02T15:04:05.000Z")` (`format.go:30`).
  - `%DURATION%` → `strconv.FormatInt(int64(r.Duration/1e6), 10)` (ms; `format.go:45`).
  - `%RESPONSE_CODE%` → `strconv.Itoa(r.ResponseCode)` (`format.go:40`); `%BYTES_SENT%` → `strconv.FormatInt(r.BytesSent, 10)` (`format.go:43`).
  - the string fields use the EXISTING `orDash(s)` / `orEmptyDash(s)` helpers (`format.go:74`/`:81` — both return `"-"` for `""`). NOTE: OTLP values are PROTO strings — there is NO CSV/quote escaping (the `format.go` `escape()` is file-sink-only; the OTLP extractors do NOT call it).
- **`internal/accesslog/accesslog.go`** — `Record struct` (`:29`): `{StartTime, Method, Path, Protocol, ResponseCode (int), BytesSent (int64), Duration (time.Duration), Authority, UserAgent, UpstreamHost}` + the two 44.3 header maps. **45.2 reads the 10 scalar fields via the operator extractors.**
- **`internal/accesslog/otlpmapping.go`** — the 45.1 PURE mapping (the functions 45.2 EXTENDS): `buildLogRecord(rec) *logspb.LogRecord` (`:20`, time_unix_nano only); `buildResource(node, logName, disableBuiltinLabels) *resourcepb.Resource` (`:28`, 4 labels or empty); `buildExportRequest(batch, node, logName, disableBuiltinLabels) *collogspb.ExportLogsServiceRequest` (`:46`). Imports already include `commonpb "go.opentelemetry.io/proto/otlp/common/v1"`.
- **`internal/accesslog/otlpsink.go`** — `OTLPAccessLogSink struct` (`:42`); `NewOTLPAccessLogSink(client, logName, node, disableBuiltinLabels, written, dropped, bufferSizeBytes, bufferFlushInterval)` (`:66`) + `newOTLPSinkWithCapacity` (`:72`); the `otlpClient` seam interface (`:23`); the writer `run`/`flush` (`:137`) — `buildExportRequest(buf, s.node, s.logName, s.disableBuiltinLabels)` at `:158`, `buildLogRecord(rec)` at `:188`, `proto.Size(lr)` size-trigger at `:190`. **The bounded-channel/buffer/export/retry loop is UNCHANGED — the operator engine is pure per-record work in the record-build step.**
- **`internal/bootstrap/bootstrap.go`** — `OTLPConfig struct` (`:217`, the 5 scalar fields 45.2 GROWS); `Bootstrap.OTLPConfigs` field decl (`:269`, doc-comment from `:259`); `parseCommonGrpcAccessLogConfig` (the shared helper ending `:467`); `parseOpenTelemetryAccessLog` (`:475`) — **the body reject `:480`, attributes reject `:483`, resource_attributes reject `:486` to LIFT; the formatters reject `:489` STAYS**; the `disable_builtin_labels: cfg.GetDisableBuiltinLabels()` read in the append block (`:502`); `stat_prefix` read-and-ignored (`:496`). Imports already include `otlpalv3` (`:15`), `grpcalv3` (`:14`), `corev3` (`:12`), `anypb`, `proto`; **`commonpb` is NOT yet imported here — Task 4 adds it.**
- **`internal/bootstrap/otlpconfig_fuzz_test.go`** — `FuzzParseOpenTelemetryAccessLogConfig` (`:24`, the 45.1 parse fuzzer; STAYS — now also exercises the lifted compile path).
- **`cmd/envoy-go/main.go`** — the OTLP sink-build loop (`if len(bs.OTLPConfigs) > 0 {`): `grpcclient.NewOTLPLogsClient(dialer, cfg.ClusterName)` → `accesslog.NewOTLPAccessLogSink(client, cfg.LogName, otlpNode, cfg.DisableBuiltinLabels, otlpWritten, otlpDropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval)`. **45.2 threads the new compiled `cfg.Body`/`cfg.Attributes`/`cfg.ResourceAttributes` into the grown signature.**
- **`internal/filter/hcm/accesslog_emit.go`** — the Record-build hooks: H1 `Path: r.URL.Path` (`:26`, query STRIPPED by `net/url`); H2 `Path: req.Path` (`:53`, `:path` pseudo, query KEPT) — AMEND-OPS-6 (the `0085` query-less path constraint).
- **`test/helpers/otlplogs/otlplogs.go`** — the driver-owned receiver: `Records() []*logspb.LogRecord` (`:144`, each carries `GetBody()`+`GetAttributes()`), `ResourceAttributes() [][]*commonpb.KeyValue` (`:165`), `Count()` (`:155`, the converge poll), `Reset()` (`:181`), `NewAtAddr`/`Stop`/`Close`. **Already exposes everything the `0085` driver needs — no change (D-OTLP-2-RECEIVER-ASSERT resolved below).**
- **`test/fixtures/0084-otlp-access-log/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`** — the differential precedent to COPY for `0085`. The driver (`driver.go`) holds `refRecords`/`subjRecords []*logspb.LogRecord` + `refResAttrs`/`subjResAttrs [][]*commonpb.KeyValue`; `BackendKind() == fixture.HTTPFixedBody`; `numRequests = 8`; the fixed `probePath="/health"` (query-less) / `probeHost="otlp.example"` / `probeUA="otlp-probe/1"`; `pollCount` poll-to-converge; `AssertStats` cross-side; `scrapeFlatStats` for the subject `logs_written`. The `assertRecords`/`assertResourceLabels` helpers are the assertion shape 45.2 EXTENDS.
- **`test/differential/runner_test.go`** — the fixture blank-import auto-discovery seam (the `0084` driver import line; add the `0085` import alongside it).

### Proto facts (verified at PLAN time against the module cache; re-confirm at IMPL)

Config `OpenTelemetryAccessLogConfig` (`…/access_loggers/open_telemetry/v3`, `otlpalv3`): `GetBody() *commonpb.AnyValue` (field 2); `GetAttributes() *commonpb.KeyValueList` (field 3); `GetResourceAttributes() *commonpb.KeyValueList` (field 4); `GetDisableBuiltinLabels() bool` (field 5); `GetFormatters() []*TypedExtensionConfig` (field 7, STAYS rejected).

OTLP `common/v1` (`commonpb "go.opentelemetry.io/proto/otlp/common/v1"`, already direct-imported by `otlpmapping.go`):
- `AnyValue struct { Value isAnyValue_Value }` with the arms `AnyValue_StringValue{ StringValue string }`, `AnyValue_KvlistValue{ KvlistValue *KeyValueList }`, `AnyValue_ArrayValue{ ArrayValue *ArrayValue }`, `AnyValue_IntValue{...}`, `AnyValue_BoolValue{...}`, `AnyValue_DoubleValue{...}`, `AnyValue_BytesValue{...}`. Getters: `GetStringValue()`, `GetKvlistValue() *KeyValueList`, `GetArrayValue() *ArrayValue`. **Type-switch on `v.GetValue()` to discriminate the arm** (a getter returns the zero value for a non-matching arm, so the getters alone cannot distinguish "string_value: \"\"" from "no value set" — use the type switch).
- `KeyValueList struct { Values []*KeyValue }` (`GetValues()`); `KeyValue struct { Key string; Value *AnyValue }` (`GetKey()`/`GetValue()`); `ArrayValue struct { Values []*AnyValue }` (`GetValues()`).

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip. The exported-returns-unexported lint nit (`feedback_pertask_gofmt_lint`) is why the compiled template types are EXPORTED.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path (this plan lives in the worktree); the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0085'`, NEVER bare `'0085'` (matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the sink's writer goroutine is a background mutator; after the sink threading lands run the FULL `internal/accesslog` package `-race`, NOT a `-run` subset.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing`): the `0085` differential asserts the aggregated per-record PAYLOAD (body + attributes + merged Resource attrs), NOT `Export`-call count / per-call batch sizes / connection count.
- **Fixture dispatch** (`reference_differential_fixture_dispatch_constraint`): one fixture dir = one runner branch — the `disable_builtin_labels`/unsupported-operator/bad-value-type cases are SUBJECT-side unit tests, NOT extra fixture dirs.

---

## D-question resolutions (the SPEC §12 D-OTLP-2-* PLAN pins — settled here)

**D-OTLP-2-COMPILE-SITE → bootstrap COMPILES at parse (bootstrap imports `internal/accesslog`); the compiled template types are EXPORTED.**
- The `internal/bootstrap` `parseOpenTelemetryAccessLog` arm calls `accesslog.CompileOTLPValue(...)` (for `body` + each `attributes` value) and `accesslog.ValidateOTLPValue(...)` (for each `resource_attributes` value — type-validate without compiling operators). A compile/validate error becomes a `bootstrap: access_log[%d]: OTLP access log …` boot error (clean `bootstrap.Load` failure; ONE operator implementation; the `reference_strict_reject_sibling_typeurl_gap` per-field discipline).
- **No import cycle:** `internal/accesslog` imports `stats` + the otlp/corev3 protos ONLY — it does NOT import `internal/bootstrap` (verified at PLAN time: `grep -rn 'internal/bootstrap' internal/accesslog/` → NONE). So the NEW edge `bootstrap → accesslog` is acyclic. (`accesslog` reaches the gRPC client through the `otlpClient` interface seam at `otlpsink.go:23`, NOT a `grpcclient` import — that seam is why `bootstrap → accesslog` does not transitively pull `grpcclient`.)
- **Exported types** (forced by bootstrap naming them in `OTLPConfig` fields + the golangci-lint exported-returns-unexported nit, `feedback_pertask_gofmt_lint`): `OTLPTemplate` (a compiled `%…%` string template), `OTLPValueTemplate` (a compiled `AnyValue` tree), `OTLPAttrTemplate` (an ordered `{Key string; Value *OTLPValueTemplate}` pair). `CompileOTLPTemplate`/`CompileOTLPValue`/`ValidateOTLPValue` return the exported types.
- **REJECTED alternative** (bootstrap stores raw `*commonpb.AnyValue` + a lightweight inline operator-validation and the sink compiles at runtime): it duplicates the operator validation across two sites and makes the strict-reject a runtime concern rather than a clean boot error. Bootstrap-compiles is the SPEC §3.2 RECOMMENDED.

**D-OTLP-2-SUPPORTED-SET-FINAL → the curated roster = the `format.go` `Default()` operators mapping 1:1 to `Record` fields, with the empty-value disposition reusing `format.go`'s `orDash`/`orEmptyDash`; confirmed at IMPL by a table-driven test.**
- The registry (the EXACT extractor semantics — reuse `format.go`'s per-field reads, MINUS the file-sink `escape()`):

  | operator token (registry key, no `%` delimiters) | extractor |
  |---|---|
  | `START_TIME` | `rec.StartTime.UTC().Format("2006-01-02T15:04:05.000Z")` |
  | `REQ(:METHOD)` | `rec.Method` (verbatim; `:method` is always present) |
  | `REQ(:PATH)` | `orDash(rec.Path)` (empty → `-`; H1 query-stripped — AMEND-OPS-6) |
  | `REQ(:AUTHORITY)` | `orEmptyDash(rec.Authority)` (empty → `-`) |
  | `REQ(USER-AGENT)` | `orEmptyDash(rec.UserAgent)` (empty → `-` — missing-value dash) |
  | `PROTOCOL` | `rec.Protocol` (verbatim) |
  | `RESPONSE_CODE` | `strconv.Itoa(rec.ResponseCode)` |
  | `BYTES_SENT` | `strconv.FormatInt(rec.BytesSent, 10)` |
  | `DURATION` | `strconv.FormatInt(int64(rec.Duration/1e6), 10)` (ms) |
  | `UPSTREAM_HOST` | `orEmptyDash(rec.UpstreamHost)` (empty → `-`) |

  `%REQ(USER-AGENT)%` is the ONLY header operator (arbitrary `%REQ(<other>)%` / `%RESP(<name>)%` STRICT-REJECT — the 44.3 capture maps hold only the configured `additional_*_headers_to_log` names; wiring the operator engine to drive header capture is a documented follow-on, SPEC §2). The empty-value dispositions above MATCH `format.go`'s file-sink behavior (the same Envoy `StreamInfoFormatter`); the IMPL confirms each via a table-driven test (the `0085` fixture uses present values, so the dashes are unit-tested, not cross-side-asserted).

**D-OTLP-2-FUZZER → land a NEW `FuzzCompileOTLPValue` over the compile path (fuzzers 45 → 46); keep the existing parse fuzzer (now also exercises the lifted compile path).**
- The operator engine is a NEW string-parsing attack surface (the `%…%` scanner + the `AnyValue`-tree walker). `FuzzCompileOTLPValue` constructs a fuzz-driven `*commonpb.AnyValue` (or feeds fuzz bytes through `proto.Unmarshal` into one) and asserts `CompileOTLPValue` never panics (returns a value or an error). The actual `^func Fuzz` count is **45** (verified at PLAN time) and the documented running total is **45** (no drift) ⇒ the new fuzzer advances both to **46**. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 46 at the completion task (`reference_fuzzer_count_docs_drift`). (Fuzzing `CompileOTLPValue` SUBSUMES `CompileOTLPTemplate`: the tree walker calls the scanner on every string leaf.)

**D-OTLP-2-RECEIVER-ASSERT → the raw `otlplogs` `Records()` + `ResourceAttributes()` surface SUFFICES; NO helper change.**
- Each `Records()` entry carries `GetBody() *AnyValue` + `GetAttributes() []*KeyValue`; `ResourceAttributes()` carries the per-`ResourceLogs` `[]*KeyValue`. The `0085` driver reads body/attributes off `Records()` and the merged resource attrs off `ResourceAttributes()` directly. No convenience accessor is added (YAGNI). The driver-local assertion helpers (`assertBody`/`assertAttributes`/`assertResourceLabels`) live in `driver.go`.

**D-OTLP-2-FIXTURE-SHAPE → the `0085` config (the deterministic-operator coverage + the structured + literal coverage); converge-poll at `numRequests = 8` (the `0084` cadence).**
- `body` (an `AnyValue` `string_value`): `"%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE% %BYTES_SENT%"` → e.g. `"GET /health HTTP/1.1 200 <n>"` (deterministic: fixed method/path/protocol/code + the `HTTPFixedBody` fixed-size body ⇒ deterministic `BYTES_SENT`).
- `attributes` (a `KeyValueList`) covering AMEND-OPS-3:
  - `op_method` → `string_value: "%REQ(:METHOD)%"` (a plain operator string leaf).
  - `nested` → `kvlist_value: { inner_code: "%RESPONSE_CODE%", inner_authority: "%REQ(:AUTHORITY)%" }` (a nested `kvlist`, each leaf substituted).
  - `arr` → `array_value: [ "%REQ(:METHOD)%", "literal-elem" ]` (an `array` mixing an operator leaf + a pure-literal leaf).
- `resource_attributes` (a `KeyValueList`) covering AMEND-OPS-1:
  - `service_name` → `string_value: "envoy-go-test"` (a static literal).
  - `authority_literal` → `string_value: "%REQ(:AUTHORITY)%"` (a request operator in a resource attr → emitted VERBATIM, NOT substituted).
- Deterministic operators asserted cross-side EXACT (fixed `Host`/query-less path/fixed-body backend): `%REQ(:METHOD)%`, `%REQ(:PATH)%`, `%REQ(:AUTHORITY)%`, `%PROTOCOL%`, `%RESPONSE_CODE%`, `%BYTES_SENT%`. EXCLUDED (non-deterministic): `%START_TIME%`, `%DURATION%`, `%UPSTREAM_HOST%`, `%REQ(USER-AGENT)%` is present-and-fixed so it MAY be asserted but the chosen config above does not template it (keeps the assertion set tight). `numRequests = 8` (the `0084` cadence; poll-to-converge, never sleep — `reference_concurrency_differential_release_barrier`).

**D-OTLP-2-SPLIT-FINAL → one leg (the FINAL chartered leg); re-checked ≈270 prod LoC, at the ADR-0045 soft gate.**
- Estimated prod LoC: `otlpformat.go` (registry ≈20 + `CompileOTLPTemplate` `%…%` scanner ≈45 + `CompileOTLPValue` tree walker ≈40 + `ValidateOTLPValue` type-only walk ≈20 + `Eval` ≈35 + the `OTLPTemplate`/`OTLPValueTemplate`/`OTLPAttrTemplate` types ≈15) ≈ **175**; the bootstrap lift (remove the 3 reject arms, add the compile/validate calls + the `commonpb` import + the 3 `OTLPConfig` fields) ≈ **45**; the `buildLogRecord`/`buildResource`/`buildExportRequest` extensions ≈ **30**; the sink + `NewOTLPAccessLogSink` signature threading ≈ **15**; main wiring ≈ **5**. Total ≈ **270 prod LoC** — within the SPEC §3.0 ≈220–320 envelope, at the ADR-0045 soft gate. Ships as ONE leg (it is the FINAL chartered leg of the 2-leg split; no further split). Row 45 flips `done` at this leg's IMPL.

---

## File structure (decomposition locked here)

**Production (touched):**
- `internal/accesslog/otlpformat.go` — CREATE: the operator engine (the registry + `OTLPTemplate`/`OTLPValueTemplate`/`OTLPAttrTemplate` types + `CompileOTLPTemplate` + `CompileOTLPValue` + `ValidateOTLPValue` + `Eval`).
- `internal/accesslog/otlpmapping.go` — MODIFY: `buildLogRecord` grows `(body *OTLPValueTemplate, attrs []OTLPAttrTemplate)`; `buildResource` grows `(resourceAttrs []*commonpb.KeyValue)`; `buildExportRequest` grows both.
- `internal/accesslog/otlpsink.go` — MODIFY: the sink carries `body`/`attrs`/`resourceAttrs`; `NewOTLPAccessLogSink` + `newOTLPSinkWithCapacity` signatures grow; the `flush`/`run` build calls thread them.
- `internal/bootstrap/bootstrap.go` — MODIFY: REMOVE the body/attributes/resource_attributes reject arms; add the `commonpb` import; compile `body`/`attributes` + validate `resource_attributes` at boot; the 3 `OTLPConfig` fields (`Body`/`Attributes`/`ResourceAttributes`).
- `cmd/envoy-go/main.go` — MODIFY: thread `cfg.Body`/`cfg.Attributes`/`cfg.ResourceAttributes` into the grown `NewOTLPAccessLogSink`.

**Test (created/modified):**
- `internal/accesslog/otlpformat_test.go` — CREATE: the table-driven engine tests (supported-set, unknown-op reject, `%%` literal, missing-value `-`, nested `kvlist`/`array`, value-type reject, the `Eval` per-record substitution).
- `internal/accesslog/otlpformat_fuzz_test.go` — CREATE: `FuzzCompileOTLPValue`.
- `internal/accesslog/otlpmapping_test.go` — MODIFY: the body/attributes substitution + the `resource_attributes` append + the AMEND-OPS-5 `disable_builtin_labels` survival cases.
- `internal/accesslog/otlpsink_test.go` — MODIFY: a body/attributes/resource_attributes end-to-end sink case + the existing cases keep their (now-extended) constructor calls; the FULL-package `-race`.
- `internal/bootstrap/bootstrap_test.go` — MODIFY: the 45.1 reject-body/reject-attributes/reject-resource_attributes cases FLIP to ACCEPT (assert the compiled fields); ADD reject-unknown-operator + reject-bad-value-type cases; the reject-formatters case STAYS.
- `test/fixtures/0085-otlp-access-log-operators/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0085` driver package.

**Docs (completion task):**
- `docs/envoy-go/DECISIONS.md` (ADR-0259 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.2.md`.

---

## Task 1: Phase scaffolding — PROGRESS-45.2.md + baselines + the final ADR-0045 split re-check (D-OTLP-2-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.2.md`

- [ ] **Step 1: Record the baseline counts**

Run and record the verbatim outputs in PROGRESS-45.2.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                       # expect 86 (tail 0084-otlp-access-log). NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS the letter-suffixed dirs — use the glob form.
grep -rh '^func Fuzz' --include='*.go' . | wc -l                        # expect 45
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go        # expect = 38 (the BackendKind tail)
grep -rn 'internal/bootstrap' internal/accesslog/ || echo NO_CYCLE      # expect NO_CYCLE (D-OTLP-2-COMPILE-SITE: bootstrap→accesslog stays acyclic)
go mod tidy -diff && echo TIDY_CLEAN                                     # expect TIDY_CLEAN (no go.mod delta anticipated this phase)
```
Baseline: stat surface **1191** (H2 cluster; non-H2 **1187**) / fixtures **86** / fuzzers **45** / BackendKind **38** / DECISIONS tail **ADR-0258** (next-free **ADR-0259**).

- [ ] **Step 2: Write the PROGRESS-45.2.md scaffold** — a header (phase 45.2 IMPL, the SPEC-45.2 reference, the worktree branch), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1191** (UNCHANGED — the operator engine adds NO stat) / fixtures **87** (`0085-otlp-access-log-operators`) / fuzzers **46** (`FuzzCompileOTLPValue`) / BackendKind **38** (UNCHANGED — driver-owned receiver) / DECISIONS **ADR-0259** / **0 new go.mod modules**.

- [ ] **Step 3: Record the D-OTLP-2-SPLIT-FINAL re-check** — note the ≈270 prod-LoC estimate (the breakdown above), confirm it sits at the ADR-0045 soft gate, and that 45.2 ships as ONE leg (the FINAL chartered leg of the 2-leg OTLP split). (Bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.2.md
git commit -m "phase 45.2 Task 1: PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-OTLP-2-SPLIT-FINAL)"
```

---

## Task 2: The operator engine `otlpformat.go` — registry + compile + validate + Eval (`internal/accesslog`) [TDD, table-driven]

**Files:**
- Create: `internal/accesslog/otlpformat.go`
- Test: `internal/accesslog/otlpformat_test.go`

These are PURE functions — no gRPC, no goroutine, no proto-config dependency beyond `commonpb` — the cleanest unit to TDD before bootstrap/sink compose them.

- [ ] **Step 1: Write the failing table tests** in `otlpformat_test.go`:
  - **`CompileOTLPTemplate` + `Eval` over a string template** (build a `*Record` with known fields, e.g. `{Method:"GET", Path:"/health", Protocol:"HTTP/1.1", ResponseCode:200, BytesSent:13, Authority:"a", UserAgent:"ua", UpstreamHost:"h"}`):
    - `"%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE%"` ⇒ `"GET /health HTTP/1.1 200"`.
    - each supported operator alone resolves to its extractor value (table row per operator — the D-OTLP-2-SUPPORTED-SET-FINAL roster); `%RESPONSE_CODE%`→`"200"`, `%BYTES_SENT%`→`"13"`, `%DURATION%`→`"0"` (zero Duration), `%START_TIME%`→ the formatted UTC string (assert via a fixed `StartTime`).
    - a pure-literal `"hello world"` ⇒ `"hello world"` (zero operators).
    - the `%%` literal: `"100%% done"` ⇒ `"100% done"`.
    - missing-value dash: an empty `UserAgent` with `"%REQ(USER-AGENT)%"` ⇒ `"-"`; an empty `Authority` with `"%REQ(:AUTHORITY)%"` ⇒ `"-"`; an empty `UpstreamHost` with `"%UPSTREAM_HOST%"` ⇒ `"-"`; an empty `Path` with `"%REQ(:PATH)%"` ⇒ `"-"`.
  - **`CompileOTLPTemplate` reject cases** (assert a non-nil error, message names the offending token): an unknown operator `"%FOOBAR%"`; a valid-Envoy-but-out-of-`Record` operator `"%UPSTREAM_CLUSTER%"`; an arbitrary header `"%REQ(X-CUSTOM)%"`; a `%RESP(...)%` operator `"%RESP(CONTENT-TYPE)%"`; an unterminated `"%REQ(:METHOD)"` (no closing `%`).
  - **`CompileOTLPValue` + `Eval` over an `AnyValue` tree** (build `*commonpb.AnyValue`s):
    - a `string_value` AnyValue ⇒ `Eval(rec)` returns an AnyValue whose `GetStringValue()` is the substituted string.
    - a `kvlist_value` `{a: "%REQ(:METHOD)%", b: "lit"}` ⇒ `Eval` returns a `kvlist_value` with `a`→`"GET"`, `b`→`"lit"` (structure preserved, each leaf a `stringValue`).
    - an `array_value` `["%RESPONSE_CODE%", "lit"]` ⇒ `Eval` returns an `array_value` `["200", "lit"]`.
    - a NESTED `kvlist_value{ outer: kvlist_value{ inner: "%REQ(:METHOD)%" } }` ⇒ the inner leaf substituted (recursion).
    - **an EMPTY `kvlist_value: {}`** ⇒ `Eval` returns an `AnyValue` whose `GetKvlistValue() != nil` with zero entries (NOT a `stringValue` — the type MUST be preserved; this is the regression guard for the kind-discriminator below). **an EMPTY `array_value: []`** ⇒ `Eval` returns an `AnyValue` whose `GetArrayValue() != nil` with zero elements.
  - **`CompileOTLPValue` value-type reject cases** (assert a non-nil error): an `int_value: 42`; a `bool_value: true`; a `double_value: 1.5`; a `bytes_value`; AND the same nested one level down (an `int_value` LEAF inside a `kvlist_value`) — the walk rejects at any depth.
  - **`ValidateOTLPValue`** (the resource_attributes type-only walk — NO operator substitution): a `string_value: "%REQ(:AUTHORITY)%"` ⇒ returns nil (an operator string is a VALID literal here — NOT scanned); a `kvlist_value{k:"v"}` ⇒ nil; an `int_value: 1` ⇒ error; a nested `int_value` leaf ⇒ error. (Proves `resource_attributes` accept request-operator strings verbatim while still type-rejecting non-string/kvlist/array.)

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestOTLPFormat -count=1`
Expected: FAIL (symbols undefined).

- [ ] **Step 3: Implement** `otlpformat.go`. Imports: `strconv`, `strings`, `commonpb "go.opentelemetry.io/proto/otlp/common/v1"`. Sketch:
```go
// otlpformat.go — phase 45.2 (OTLP access-log operator engine, ADR-0259). A small
// two-phase command-operator engine: compile-once at boot (CompileOTLPTemplate /
// CompileOTLPValue, STRICT-REJECTing unknown operators + non-string/kvlist/array
// AnyValue value-types) + substitute-per-record (Eval, emitting stringValue leaves
// and preserving kvlist/array structure). Curated to the Record-mapped operator
// subset (the format.go Default() roster) — the envoy-go-strict mirror of the
// reference's own unknown-operator boot-reject. NO existing engine to reuse:
// format.go Default() is a hard-coded single-format function. OTLP values are proto
// strings — NO CSV/quote escaping (format.go escape() is file-sink-only).

// otlpOperators maps a supported operator TOKEN (the inner text between the %
// delimiters, no %) to a func(*Record) string extractor. The empty-value dispositions
// reuse format.go's orDash/orEmptyDash (the same Envoy StreamInfoFormatter behavior).
var otlpOperators = map[string]func(*Record) string{
	"START_TIME":      func(r *Record) string { return r.StartTime.UTC().Format("2006-01-02T15:04:05.000Z") },
	"REQ(:METHOD)":    func(r *Record) string { return r.Method },
	"REQ(:PATH)":      func(r *Record) string { return orDash(r.Path) },
	"REQ(:AUTHORITY)": func(r *Record) string { return orEmptyDash(r.Authority) },
	"REQ(USER-AGENT)": func(r *Record) string { return orEmptyDash(r.UserAgent) },
	"PROTOCOL":        func(r *Record) string { return r.Protocol },
	"RESPONSE_CODE":   func(r *Record) string { return strconv.Itoa(r.ResponseCode) },
	"BYTES_SENT":      func(r *Record) string { return strconv.FormatInt(r.BytesSent, 10) },
	"DURATION":        func(r *Record) string { return strconv.FormatInt(int64(r.Duration/1e6), 10) },
	"UPSTREAM_HOST":   func(r *Record) string { return orEmptyDash(r.UpstreamHost) },
}

// otlpSegment is one compiled piece of a string template: a literal OR an operator
// extractor (exactly one of lit/op is set).
type otlpSegment struct {
	lit string
	op  func(*Record) string
}

// OTLPTemplate is a compiled %…%-operator string template (a slice of segments).
// Exported so internal/bootstrap can name it in OTLPConfig fields (D-OTLP-2-COMPILE-SITE).
type OTLPTemplate struct{ segs []otlpSegment }

// CompileOTLPTemplate scans s for %…% operators (the Envoy %-delimited grammar; %% is
// a literal %), returning a compiled template or an error naming the first unknown
// operator / unterminated %. A pure-literal string compiles to one literal segment.
func CompileOTLPTemplate(s string) (*OTLPTemplate, error) {
	var segs []otlpSegment
	var lit strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '%' {
			lit.WriteByte(s[i])
			i++
			continue
		}
		// s[i] == '%'
		if i+1 < len(s) && s[i+1] == '%' { // %% → literal %
			lit.WriteByte('%')
			i += 2
			continue
		}
		end := strings.IndexByte(s[i+1:], '%')
		if end < 0 {
			return nil, fmt.Errorf("unterminated operator at %q", s[i:])
		}
		token := s[i+1 : i+1+end]
		op, ok := otlpOperators[token]
		if !ok {
			return nil, fmt.Errorf("unsupported access-log operator %%%s%% (not in the curated Record-mapped set)", token)
		}
		if lit.Len() > 0 {
			segs = append(segs, otlpSegment{lit: lit.String()})
			lit.Reset()
		}
		segs = append(segs, otlpSegment{op: op})
		i = i + 1 + end + 1 // past the closing %
	}
	if lit.Len() > 0 {
		segs = append(segs, otlpSegment{lit: lit.String()})
	}
	return &OTLPTemplate{segs: segs}, nil
}

// evalString concatenates the compiled segments against rec.
func (t *OTLPTemplate) evalString(rec *Record) string {
	if len(t.segs) == 1 && t.segs[0].op == nil {
		return t.segs[0].lit // pure literal fast-path
	}
	var b strings.Builder
	for _, sg := range t.segs {
		if sg.op != nil {
			b.WriteString(sg.op(rec))
		} else {
			b.WriteString(sg.lit)
		}
	}
	return b.String()
}
```
The `AnyValue`-tree compiled form + the walkers:
```go
// otlpValueKind discriminates the compiled AnyValue arm. An EXPLICIT kind (NOT
// slice-non-nil-ness) is load-bearing: an empty kvlist_value/array_value compiles
// to a nil slice, so switching on `kvlist != nil` would misroute an empty
// collection to the string arm — a type-changing bug (kvlist→string) that the
// cross-side differential would catch. The kind tag preserves the proto type.
type otlpValueKind uint8

const (
	otlpValueString otlpValueKind = iota
	otlpValueKvlist
	otlpValueArray
)

// OTLPValueTemplate is a compiled AnyValue tree mirroring the config AnyValue. The
// kind selects which of str/kvlist/array is meaningful (kvlist/array may be an
// empty-but-non-string collection). Exported (D-OTLP-2-COMPILE-SITE).
type OTLPValueTemplate struct {
	kind   otlpValueKind
	str    *OTLPTemplate        // kind==otlpValueString: a compiled string_value leaf
	kvlist []OTLPAttrTemplate   // kind==otlpValueKvlist: a compiled kvlist_value (ordered; may be empty)
	array  []*OTLPValueTemplate // kind==otlpValueArray: a compiled array_value (ordered; may be empty)
}

// OTLPAttrTemplate is one ordered key→compiled-value pair (a KeyValue in a
// kvlist_value OR a top-level attributes entry). Exported (D-OTLP-2-COMPILE-SITE).
type OTLPAttrTemplate struct {
	Key   string
	Value *OTLPValueTemplate
}

// CompileOTLPValue walks a config AnyValue, compiling each string_value leaf
// (CompileOTLPTemplate — operators substituted at Eval) and recursing kvlist/array;
// it returns a type error for any int/bool/double/bytes value (AMEND-OPS-2) at any
// depth. A nil AnyValue compiles to a nil template (the no-body case).
func CompileOTLPValue(v *commonpb.AnyValue) (*OTLPValueTemplate, error) {
	if v == nil {
		return nil, nil
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		t, err := CompileOTLPTemplate(v.GetStringValue())
		if err != nil {
			return nil, err
		}
		return &OTLPValueTemplate{kind: otlpValueString, str: t}, nil
	case *commonpb.AnyValue_KvlistValue:
		kvs := make([]OTLPAttrTemplate, 0, len(v.GetKvlistValue().GetValues()))
		for _, kv := range v.GetKvlistValue().GetValues() {
			child, err := CompileOTLPValue(kv.GetValue())
			if err != nil {
				return nil, err
			}
			kvs = append(kvs, OTLPAttrTemplate{Key: kv.GetKey(), Value: child})
		}
		return &OTLPValueTemplate{kind: otlpValueKvlist, kvlist: kvs}, nil
	case *commonpb.AnyValue_ArrayValue:
		arr := make([]*OTLPValueTemplate, 0, len(v.GetArrayValue().GetValues()))
		for _, elem := range v.GetArrayValue().GetValues() {
			child, err := CompileOTLPValue(elem)
			if err != nil {
				return nil, err
			}
			arr = append(arr, child)
		}
		return &OTLPValueTemplate{kind: otlpValueArray, array: arr}, nil
	default:
		// A non-nil AnyValue with no oneof arm set (e.g. body: {}) yields a nil
		// v.GetValue() interface and lands here too — boot-rejected as unsupported.
		return nil, fmt.Errorf("unsupported OTLP value type %T (only string, kvlist, array are supported)", v.GetValue())
	}
}

// ValidateOTLPValue walks a config AnyValue, returning a type error for any
// int/bool/double/bytes value at any depth — WITHOUT compiling operators (resource_
// attributes are literal pass-through, AMEND-OPS-1: a string_value "%REQ(...)%" is a
// VALID literal here, emitted verbatim). Used for resource_attributes.
func ValidateOTLPValue(v *commonpb.AnyValue) error {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return nil
	case *commonpb.AnyValue_KvlistValue:
		for _, kv := range v.GetKvlistValue().GetValues() {
			if err := ValidateOTLPValue(kv.GetValue()); err != nil {
				return err
			}
		}
		return nil
	case *commonpb.AnyValue_ArrayValue:
		for _, elem := range v.GetArrayValue().GetValues() {
			if err := ValidateOTLPValue(elem); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported OTLP value type %T (only string, kvlist, array are supported)", v.GetValue())
	}
}

// Eval substitutes the compiled tree against rec, returning a fresh *commonpb.AnyValue
// with every string leaf a stringValue (the substituted text) and the kvlist/array
// structure preserved. A nil receiver returns nil (the no-body case).
func (t *OTLPValueTemplate) Eval(rec *Record) *commonpb.AnyValue {
	if t == nil {
		return nil
	}
	switch t.kind {
	case otlpValueKvlist:
		kvs := make([]*commonpb.KeyValue, len(t.kvlist))
		for i, at := range t.kvlist {
			kvs[i] = &commonpb.KeyValue{Key: at.Key, Value: at.Value.Eval(rec)}
		}
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: kvs}}}
	case otlpValueArray:
		elems := make([]*commonpb.AnyValue, len(t.array))
		for i, vt := range t.array {
			elems[i] = vt.Eval(rec)
		}
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: elems}}}
	default: // otlpValueString — an empty string_value leaf compiles to a zero-segment OTLPTemplate (evalString → "")
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: t.str.evalString(rec)}}
	}
}
```
(Add `fmt` to the imports. `orDash`/`orEmptyDash` are already in the package from `format.go` — reuse them, do NOT redefine. A `string_value: ""` config leaf compiles to a zero-segment `OTLPTemplate` and `evalString` returns `""` — confirm the test covers the empty-string leaf so the `len(t.segs)==0` path is exercised.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/accesslog/ -run TestOTLPFormat -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpformat.go internal/accesslog/otlpformat_test.go
git commit -m "phase 45.2 Task 2: the OTLP operator engine (otlpformat.go) — registry + CompileOTLPTemplate/CompileOTLPValue/ValidateOTLPValue + Eval; curated Record-mapped set, STRICT-REJECT unknown operators + non-string/kvlist/array value-types (ADR-0259, AMEND-OPS-2/3/4)"
```

---

## Task 3: The operator-engine fuzzer `FuzzCompileOTLPValue` (D-OTLP-2-FUZZER; fuzzers 45 → 46) [fuzz]

**Files:**
- Create: `internal/accesslog/otlpformat_fuzz_test.go`

- [ ] **Step 1: Write the fuzzer** — a no-panic fuzzer over `CompileOTLPValue` (the tree walker, which calls `CompileOTLPTemplate` on every string leaf — the richest new surface). Feed fuzz bytes as a `string_value` template AND, separately, as marshalled-proto into an `AnyValue` so both the scanner and the tree walk are exercised. The invariant: `CompileOTLPValue` / `CompileOTLPTemplate` NEVER panic (return a value or an error).
```go
func FuzzCompileOTLPValue(f *testing.F) {
	f.Add("")                                       // empty
	f.Add("%REQ(:METHOD)% %RESPONSE_CODE%")         // valid operators
	f.Add("100%% literal")                          // %% literal
	f.Add("%FOOBAR%")                               // unknown operator
	f.Add("%REQ(:METHOD)")                          // unterminated
	f.Fuzz(func(t *testing.T, s string) {
		// the scanner directly
		_, _ = CompileOTLPTemplate(s) // must not panic
		// the tree walker over a string_value leaf carrying the fuzz string
		v := &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
		_, _ = CompileOTLPValue(v)    // must not panic
		_ = ValidateOTLPValue(v)      // must not panic
	})
}
```

- [ ] **Step 2: Run the fuzzer briefly**

Run: `go test ./internal/accesslog/ -run 'FuzzCompileOTLPValue' -count=1` then `go test ./internal/accesslog/ -fuzz 'FuzzCompileOTLPValue' -fuzztime 20s`
Expected: PASS / no crashers.

- [ ] **Step 3: Confirm the count advanced**

Run: `grep -rh '^func Fuzz' --include='*.go' . | wc -l`
Expected: **46** (was 45). Record in PROGRESS-45.2.md.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpformat_fuzz_test.go
git commit -m "phase 45.2 Task 3: FuzzCompileOTLPValue (no-panic over the operator scanner + the AnyValue-tree walker); fuzzers 45 → 46 (D-OTLP-2-FUZZER)"
```

---

## Task 4: The config-parse LIFT (`internal/bootstrap`) — remove the body/attributes/resource_attributes rejects + compile-at-boot + the value-type reject + the `OTLPConfig` fields [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Update the tests** in `bootstrap_test.go` (the 45.1 OTLP parse table — find the OTLP cases: `grep -n 'OTLP\|open_telemetry\|OTLPConfig' internal/bootstrap/bootstrap_test.go`):
  - **FLIP** the 45.1 `reject-body` / `reject-attributes` / `reject-resource_attributes` cases to ACCEPT: a `body: { string_value: "%REQ(:METHOD)% %RESPONSE_CODE%" }` ⇒ `Load` succeeds, `bs.OTLPConfigs[0].Body != nil` (a compiled `*accesslog.OTLPValueTemplate`); an `attributes: { values: [ {key: "m", value: {string_value: "%REQ(:METHOD)%"}} ] }` ⇒ `len(bs.OTLPConfigs[0].Attributes) == 1 && bs.OTLPConfigs[0].Attributes[0].Key == "m"`; a `resource_attributes: { values: [ {key: "svc", value: {string_value: "envoy-go"}} ] }` ⇒ `len(bs.OTLPConfigs[0].ResourceAttributes) == 1`.
  - **accept-no-templating** (the 45.1 built-in regression anchor): the minimal `OpenTelemetryAccessLogConfig` (no body/attributes/resource_attributes) ⇒ `Body == nil && len(Attributes) == 0 && len(ResourceAttributes) == 0` (the byte-stable built-in path).
  - **accept-structured**: a `body: { kvlist_value: {...} }` + an `attributes` value `{ array_value: [...] }` ⇒ compile succeeds (the nested-structure path).
  - **accept-resource-literal-operator**: a `resource_attributes` value `{ string_value: "%REQ(:AUTHORITY)%" }` ⇒ ACCEPTS (literal pass-through; NOT operator-scanned — AMEND-OPS-1).
  - **reject-unknown-operator**: a `body: { string_value: "%FOOBAR%" }` ⇒ `Load` errors, message names the operator + `OTLP access log` + `access_log[%d]`.
  - **reject-bad-value-type**: a `body: { int_value: 42 }` ⇒ errors (the AMEND-OPS-2 type reject); an `attributes` value `{ bool_value: true }` ⇒ errors; a `resource_attributes` value `{ int_value: 1 }` ⇒ errors (resource_attributes are type-validated too).
  - **reject-formatters STAYS**: a non-empty `formatters` ⇒ errors naming `formatters` (UNCHANGED).
  - (Keep the existing accept-minimal / accept-disable_builtin_labels / accept-buffer-fields / reject-google_grpc / reject-V2 / reject-empty-cluster / coexist cases — they are the byte-stable regression guard.)

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: FAIL on the new/flipped cases (`Body`/`Attributes`/`ResourceAttributes` undefined; the body/attributes/resource_attributes still rejected).

- [ ] **Step 3: Implement** in `bootstrap.go`:

Add the `commonpb` import alongside the existing imports:
```go
commonpb "go.opentelemetry.io/proto/otlp/common/v1"
```
And the `internal/accesslog` import (the NEW acyclic edge — D-OTLP-2-COMPILE-SITE):
```go
"github.com/esalaine/envoy-go/internal/accesslog"
```
Grow `OTLPConfig` (`:217`) with the 3 compiled/validated fields:
```go
	// Body is the compiled body AnyValue template (operator-substituted per record
	// → LogRecord.body); nil when no body is configured (the 45.1 built-in path).
	Body *accesslog.OTLPValueTemplate
	// Attributes are the compiled attributes templates (ordered key→template,
	// operator-substituted per record → LogRecord.attributes); empty when none.
	Attributes []accesslog.OTLPAttrTemplate
	// ResourceAttributes are the LITERAL resource_attributes (validated string/
	// kvlist/array; request operators pass through verbatim — AMEND-OPS-1), emitted
	// as extra Resource.attributes after the 4 built-in labels (surviving
	// disable_builtin_labels — AMEND-OPS-5); nil when none.
	ResourceAttributes []*commonpb.KeyValue
```
In `parseOpenTelemetryAccessLog` (`:475`): REMOVE the body reject arm (`:480-482`), the attributes reject arm (`:483-485`), the resource_attributes reject arm (`:486-488`); KEEP the formatters reject arm (`:489-491`). After the `parseCommonGrpcAccessLogConfig` call succeeds, compile/validate and populate the new fields:
```go
	// Compile body + attributes (operators substituted per record — AMEND-OPS-2/3/4);
	// the strict-reject (unknown operator / non-string-kvlist-array value-type) is a
	// clean boot error here (D-OTLP-2-COMPILE-SITE).
	bodyTmpl, err := accesslog.CompileOTLPValue(cfg.GetBody())
	if err != nil {
		return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log body: %w", idx, err)
	}
	var attrs []accesslog.OTLPAttrTemplate
	for _, kv := range cfg.GetAttributes().GetValues() {
		vt, err := accesslog.CompileOTLPValue(kv.GetValue())
		if err != nil {
			return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log attributes[%q]: %w", idx, kv.GetKey(), err)
		}
		attrs = append(attrs, accesslog.OTLPAttrTemplate{Key: kv.GetKey(), Value: vt})
	}
	// resource_attributes are LITERAL (no operator substitution — AMEND-OPS-1) but
	// still type-validated (string/kvlist/array only — AMEND-OPS-2).
	resAttrs := cfg.GetResourceAttributes().GetValues()
	for _, kv := range resAttrs {
		if err := accesslog.ValidateOTLPValue(kv.GetValue()); err != nil {
			return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log resource_attributes[%q]: %w", idx, kv.GetKey(), err)
		}
	}
```
Then thread them into the appended `OTLPConfig`:
```go
	result.OTLPConfigs = append(result.OTLPConfigs, OTLPConfig{
		ClusterName:          clusterName,
		LogName:              logName,
		BufferSizeBytes:      bufBytes,
		BufferFlushInterval:  flush,
		DisableBuiltinLabels: cfg.GetDisableBuiltinLabels(),
		Body:                 bodyTmpl,
		Attributes:           attrs,
		ResourceAttributes:   resAttrs,
	})
```
(`resAttrs` is `[]*commonpb.KeyValue` — the proto KeyValues emitted verbatim. Confirm `cfg.GetAttributes().GetValues()` / `cfg.GetResourceAttributes().GetValues()` are the correct accessors — a nil `KeyValueList` ⇒ `len == 0` ⇒ the loops are no-ops ⇒ nil fields. Update the `parseOpenTelemetryAccessLog` doc-comment to reflect the LIFT: body/attributes are now compiled, resource_attributes validated; only formatters STAYS rejected.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: PASS (the flipped accept cases + the new reject-unknown-operator/reject-bad-value-type + the unchanged grpc-ALS + the formatters reject). Confirm `go mod tidy -diff` is EMPTY (no new module — `commonpb`/`accesslog` are already in the build graph).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/
git commit -m "phase 45.2 Task 4: LIFT body/attributes/resource_attributes from STRICT-REJECT to compiled-at-boot (bootstrap→accesslog, D-OTLP-2-COMPILE-SITE); OTLPConfig.{Body,Attributes,ResourceAttributes}; reject unknown-operator + bad-value-type; formatters STAYS rejected"
```

---

## Task 5: The `buildLogRecord` / `buildResource` / `buildExportRequest` extensions (`internal/accesslog/otlpmapping.go`) [TDD, table-driven]

**Files:**
- Modify: `internal/accesslog/otlpmapping.go`
- Test: `internal/accesslog/otlpmapping_test.go`

- [ ] **Step 1: Write the failing table tests** (extend `otlpmapping_test.go`; keep the 45.1 built-in cases passing with the new nil/empty args):
  - **`buildLogRecord(rec, nil, nil)`** ⇒ the 45.1 built-in record: `GetTimeUnixNano()` set, `GetBody() == nil`, `GetAttributes()` empty (byte-identical to 45.1).
  - **`buildLogRecord(rec, bodyTmpl, attrTmpls)`** with a compiled `bodyTmpl` (`"%REQ(:METHOD)% %RESPONSE_CODE%"`) + one attr (`{Key:"m", Value: compiled "%REQ(:METHOD)%"}`) ⇒ `GetBody().GetStringValue() == "GET 200"`; `GetAttributes()` has one `KeyValue{Key:"m", Value.stringValue:"GET"}`; `GetTimeUnixNano()` still set.
  - **nested** body (`kvlist_value{a:"%REQ(:METHOD)%"}`) ⇒ `GetBody().GetKvlistValue()` with `a`→`"GET"`.
  - **`buildResource(node, "L", false, nil)`** ⇒ the 45.1 4 built-in labels only.
  - **`buildResource(node, "L", false, resAttrs)`** with `resAttrs = [{svc:"x"}, {auth:"%REQ(:AUTHORITY)%"}]` ⇒ `GetAttributes()` = the 4 built-ins THEN `svc`→`"x"`, `auth`→`"%REQ(:AUTHORITY)%"` (VERBATIM, in that order — AMEND-OPS-1/5).
  - **`buildResource(node, "L", true, resAttrs)`** (disable_builtin_labels) ⇒ `GetAttributes()` = JUST the resource attrs `[svc, auth]` (the 4 built-ins dropped, resource attrs SURVIVE — AMEND-OPS-5).
  - **`buildResource(node, "L", true, nil)`** ⇒ EMPTY `Resource` (the 45.1 disable path with no resource attrs).
  - **`buildExportRequest(batch, node, "L", false, bodyTmpl, attrTmpls, resAttrs)`** over a 2-record batch ⇒ one `ResourceLogs` whose `Resource` == `buildResource(...)` and whose `ScopeLogs[0].LogRecords` == the 2 records each built via `buildLogRecord(rec, bodyTmpl, attrTmpls)`.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestOTLPMapping -count=1`
Expected: FAIL (signature mismatch / undefined behavior).

- [ ] **Step 3: Implement** the signature + body changes in `otlpmapping.go`:
```go
// buildLogRecord maps a Record into an OTLP LogRecord: always time_unix_nano; when
// body != nil sets LogRecord.body = body.Eval(rec); when attrs is non-empty sets
// LogRecord.attributes = [{Key, Value: a.Value.Eval(rec)} …]. The 45.1 built-in path
// is the body==nil && len(attrs)==0 case — byte-identical.
func buildLogRecord(rec *Record, body *OTLPValueTemplate, attrs []OTLPAttrTemplate) *logspb.LogRecord {
	lr := &logspb.LogRecord{TimeUnixNano: uint64(rec.StartTime.UnixNano())}
	if body != nil {
		lr.Body = body.Eval(rec)
	}
	if len(attrs) > 0 {
		kvs := make([]*commonpb.KeyValue, len(attrs))
		for i, a := range attrs {
			kvs[i] = &commonpb.KeyValue{Key: a.Key, Value: a.Value.Eval(rec)}
		}
		lr.Attributes = kvs
	}
	return lr
}

// buildResource emits the 4 built-in Resource labels UNLESS disableBuiltinLabels,
// then ALWAYS appends the literal resourceAttrs (they survive disableBuiltinLabels —
// AMEND-OPS-5). An empty Resource only when disableBuiltinLabels && len(resourceAttrs)==0.
func buildResource(node *corev3.Node, logName string, disableBuiltinLabels bool, resourceAttrs []*commonpb.KeyValue) *resourcepb.Resource {
	var attrs []*commonpb.KeyValue
	if !disableBuiltinLabels {
		kv := func(k, v string) *commonpb.KeyValue {
			return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
		}
		attrs = []*commonpb.KeyValue{
			kv("log_name", logName),
			kv("zone_name", node.GetLocality().GetZone()),
			kv("cluster_name", node.GetCluster()),
			kv("node_name", node.GetId()),
		}
	}
	attrs = append(attrs, resourceAttrs...)
	return &resourcepb.Resource{Attributes: attrs}
}

// buildExportRequest wraps a batch into one ExportLogsServiceRequest, threading the
// body/attributes templates + the literal resourceAttrs.
func buildExportRequest(batch []*logspb.LogRecord, node *corev3.Node, logName string, disableBuiltinLabels bool, resourceAttrs []*commonpb.KeyValue) *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource:  buildResource(node, logName, disableBuiltinLabels, resourceAttrs),
			ScopeLogs: []*logspb.ScopeLogs{{LogRecords: batch}},
		}},
	}
}
```
> **DESIGN NOTE — where the per-record body/attributes Eval happens:** `buildExportRequest` receives an ALREADY-BUILT `batch []*logspb.LogRecord`. The body/attributes substitution happens in `buildLogRecord` at the moment each record is appended to the writer's buffer (Task 6's `run` loop calls `buildLogRecord(rec, s.body, s.attrs)`), NOT in `buildExportRequest`. So `buildExportRequest` does NOT take the body/attr templates — only `resourceAttrs` (resource-scoped, built once per Export). This keeps the per-record Eval at buffer-append time (matching the 45.1 `proto.Size(lr)` size-accounting which must reflect the templated record) and the resource build at flush time. (Confirm the Task-5 `buildExportRequest` test passes the pre-built batch + `resourceAttrs`, NOT the templates.)
> **buildResource nil-vs-empty:** when `disableBuiltinLabels && len(resourceAttrs)==0`, `attrs` stays nil and `Resource{Attributes: nil}` ≡ the 45.1 empty Resource — byte-identical. `append(nil, resourceAttrs...)` with an empty `resourceAttrs` is nil (Go), preserving the 45.1 path.

(Confirm `commonpb` is already imported in `otlpmapping.go` — it is, from 45.1.)

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/accesslog/ -run TestOTLPMapping -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpmapping.go internal/accesslog/otlpmapping_test.go
git commit -m "phase 45.2 Task 5: extend buildLogRecord (body/attributes substitution) + buildResource (resource_attributes append surviving disable_builtin_labels — AMEND-OPS-5) + buildExportRequest threading"
```

---

## Task 6: The sink template threading + the `NewOTLPAccessLogSink` signature (`internal/accesslog/otlpsink.go`) [TDD, full-package `-race`]

**Files:**
- Modify: `internal/accesslog/otlpsink.go`
- Test: `internal/accesslog/otlpsink_test.go`

- [ ] **Step 1: Write the failing test** (extend `otlpsink_test.go`; the existing cases adopt the new constructor args — pass `nil, nil, nil` for the 45.1 built-in cases so they stay byte-identical):
  - **body+attributes+resource_attributes end-to-end**: a `bufferSizeBytes=0` sink (flush-every-entry) built with a compiled `body` (`"%REQ(:METHOD)% %RESPONSE_CODE%"`), one attr (`{Key:"m", Value: compiled "%REQ(:METHOD)%"}`), and a literal `resourceAttrs` (`[{svc:"x"}]`); `Submit(rec)` ⇒ the fake `otlpClient` receives ONE `Export` whose `ResourceLogs[0]`:
    - `.ScopeLogs[0].LogRecords[0].GetBody().GetStringValue() == "GET 200"` + `.GetAttributes()` has `{m:"GET"}` + `GetTimeUnixNano()` set.
    - `.Resource.GetAttributes()` = the 4 built-ins + `{svc:"x"}` (resource append).
  - **disable_builtin_labels survival**: a sink with `disableBuiltinLabels=true` + a `resourceAttrs` `[{svc:"x"}]` ⇒ the exported `Resource.GetAttributes()` = JUST `{svc:"x"}` (AMEND-OPS-5) + the body/attributes still present.
  - **the 45.1 built-in cases** (submit-exports-record / builtin-labels / batch-on-size / drop-newest / retry-once / close-idempotent / non-Record-ignored): UNCHANGED behavior with `body=nil, attrs=nil, resourceAttrs=nil` — confirm they still pass (the byte-stable built-in regression).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestOTLPSink -count=1`
Expected: FAIL (constructor arity / the new body/attributes not threaded).

- [ ] **Step 3: Implement** in `otlpsink.go`:
  - Add the fields to `OTLPAccessLogSink` (`:42`): `body *OTLPValueTemplate`, `attrs []OTLPAttrTemplate`, `resourceAttrs []*commonpb.KeyValue` (add the `commonpb` import).
  - Grow `NewOTLPAccessLogSink` + `newOTLPSinkWithCapacity` signatures to take `body *OTLPValueTemplate, attrs []OTLPAttrTemplate, resourceAttrs []*commonpb.KeyValue` (place them after `disableBuiltinLabels`, before the counters — keep the param order stable + documented) and store them on the struct.
  - In `run`'s record-append: `lr := buildLogRecord(rec, s.body, s.attrs)` (was `buildLogRecord(rec)`). The `proto.Size(lr)` size-accounting (`:190`) now reflects the templated record — correct (bigger records flush sooner; no logic change).
  - In `flush`: `req := buildExportRequest(buf, s.node, s.logName, s.disableBuiltinLabels, s.resourceAttrs)` (the new `resourceAttrs` arg).

- [ ] **Step 4: Run to verify they pass + the FULL-package `-race`**

Run: `go test ./internal/accesslog/ -run TestOTLPSink -count=1`
Expected: PASS.
Then the FULL package `-race` (`reference_full_suite_race_after_background_mutator`):
Run: `go test ./internal/accesslog/ -race -count=1`
Expected: PASS, no race.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/otlpsink.go internal/accesslog/otlpsink_test.go
git commit -m "phase 45.2 Task 6: thread the compiled body/attributes + literal resource_attributes through the OTLPAccessLogSink (NewOTLPAccessLogSink signature grows); the per-record Eval at buffer-append, the resource build at flush (ADR-0259)"
```

---

## Task 7: Boot wiring (`cmd/envoy-go/main.go`) — thread the compiled `OTLPConfig` fields

**Files:**
- Modify: `cmd/envoy-go/main.go`

main.go is not unit-tested in isolation (the `0085` differential is its behavioral proof); the gate here is build + boot-smoke.

- [ ] **Step 1: Implement** — in the `if len(bs.OTLPConfigs) > 0 {` loop, grow the `NewOTLPAccessLogSink` call to pass the compiled fields:
```go
			sinks = append(sinks, accesslog.NewOTLPAccessLogSink(client, cfg.LogName, otlpNode, cfg.DisableBuiltinLabels, cfg.Body, cfg.Attributes, cfg.ResourceAttributes, otlpWritten, otlpDropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval))
```
(Match the Task-6 param order exactly — `body, attrs, resourceAttrs` after `disableBuiltinLabels`. No new import — `cfg.Body`/`cfg.Attributes`/`cfg.ResourceAttributes` are `accesslog`/`commonpb` types already in the build graph.)

- [ ] **Step 2: Build + boot-smoke**

Run: `go build ./... && echo BUILD_OK`
Then a manual boot-smoke against a hand-written bootstrap with an OTLP `access_log` carrying a `body: { string_value: "%REQ(:METHOD)%" }` pointing at a valid H2 cluster ⇒ confirm it boots clean; and one carrying `body: { string_value: "%FOOBAR%" }` ⇒ confirm `bootstrap.Load` boot-rejects (the parse-layer strict-reject, before sink build).

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 45.2 Task 7: boot wiring — thread cfg.Body/Attributes/ResourceAttributes into NewOTLPAccessLogSink"
```

---

## Task 8: The `0085-otlp-access-log-operators` differential fixture + the subject unit tests

**Files:**
- Create: `test/fixtures/0085-otlp-access-log-operators/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0085` driver package)
- (the `disable_builtin_labels` survival + the unsupported-operator/bad-value-type rejects are SUBJECT-side unit tests — Tasks 4 & 6 already cover them; confirm coverage, add any gap here)

Model the whole directory on `test/fixtures/0084-otlp-access-log/`. The data-plane backend REUSES `HTTPFixedBody = 4` (BackendKind stays 38).

- [ ] **Step 1: Author the bootstraps.** Copy `0084`'s `envoy.yaml`/`envoy-go.yaml` and SWAP the access-logger `OpenTelemetryAccessLogConfig` to carry the D-OTLP-2-FIXTURE-SHAPE config: `log_name: "0085"`; a `body: { string_value: "%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE% %BYTES_SENT%" }`; an `attributes` `KeyValueList` with `op_method`→`%REQ(:METHOD)%`, `nested`→`kvlist_value{inner_code:"%RESPONSE_CODE%", inner_authority:"%REQ(:AUTHORITY)%"}`, `arr`→`array_value:["%REQ(:METHOD)%","literal-elem"]`; a `resource_attributes` `KeyValueList` with `service_name`→`"envoy-go-test"` (static) + `authority_literal`→`"%REQ(:AUTHORITY)%"` (literal pass-through). The `c_otlp` h2c cluster + the H1 downstream listener are UNCHANGED from `0084`. `OTLPHost = host.docker.internal` (reference) / `127.0.0.1` (subject).

- [ ] **Step 2: Author `driver/driver.go`.** Copy `0084/driver/driver.go` and adapt: `fixtureName = "0085-otlp-access-log-operators"`; `wantLogName = "0085"`; the same `numRequests = 8` / `probePath="/health"` (query-less — AMEND-OPS-6) / `probeHost="otlp.example"` / `probeUA="otlp-probe/1"`; the same allocateOTLPPort/ensureServer/driveSide/pollCount/fireProbe/scrapeFlatStats machinery. EXTEND `AssertStats` (+ new helpers `assertBody`/`assertAttributes`) to assert cross-side EXACT, aggregated over all N records:
  - every record's `GetBody().GetStringValue()` == `"GET /health HTTP/1.1 200 <bytesSent>"` AND matches cross-side (compute the expected body from the known fixed request + the fixed-body backend size; OR assert ref==subj per-record-index without hardcoding the byte count — see note).
  - every record's `GetAttributes()` carries `op_method:"GET"`, `nested` (a `kvlist` with `inner_code:"200"`, `inner_authority:"otlp.example"`), `arr` (an `array` `["GET","literal-elem"]`) — assert the substituted leaves AND cross-side equality.
  - the `ResourceAttributes()` snapshots carry the 4 built-ins + `service_name:"envoy-go-test"` + `authority_literal:"%REQ(:AUTHORITY)%"` (the LITERAL un-substituted string — AMEND-OPS-1) and match cross-side.
  - the subject-side `access_logs.open_telemetry_access_log.logs_written == N` (UNCHANGED `scrapeFlatStats`).
  > **NOTE on `%BYTES_SENT%`:** assert ref==subj per-record (cross-side EXACT) rather than hardcoding the fixed-body byte count — the `HTTPFixedBody` body size is the same both sides, so cross-side equality is the robust assertion (avoids coupling the fixture to the backend's exact byte count). If cross-side `%BYTES_SENT%` proves flaky, drop it from the `body` template (keep method/path/protocol/code) — it is the one operator to remove first.
  - **UNasserted** (per `reference_streaming_sink_differential_framing`): `%START_TIME%`/`%DURATION%`/`%UPSTREAM_HOST%` (excluded from the config); `time_unix_nano` VALUE; `Export`-call framing.

- [ ] **Step 3: Author `expectations.yaml` + `README.md`.** `expectations.yaml`: the standard admin-diff + the StatsAsserter dispatch (copy `0084`'s). `README.md`: the fixture's purpose (operator-templated body/attributes/resource_attributes), the query-less-path constraint (AMEND-OPS-6), the deterministic-operators-only cross-side note (START_TIME/DURATION/UPSTREAM_HOST excluded), the resource_attributes-literal-pass-through note (AMEND-OPS-1), the `disable_builtin_labels`/unsupported-operator/bad-value-type-are-unit-tests note (`reference_differential_fixture_dispatch_constraint`), and the host-reachability table.

- [ ] **Step 4: Confirm the subject-side unit coverage.** Verify (do NOT duplicate as a fixture): the `disable_builtin_labels` + resource_attributes survival is covered by Task 6's `otlpsink_test.go` disable-builtin case (AMEND-OPS-5); the unsupported-operator + bad-value-type boot-rejects are covered by Task 4's `bootstrap_test.go` reject cases. If any is missing, add it to the respective `_test.go` (NOT here).

- [ ] **Step 5: Register + run the fixture isolated.**

Add `_ "github.com/esalaine/envoy-go/test/fixtures/0085-otlp-access-log-operators/driver"` to `runner_test.go`'s fixture blank-imports (alongside the `0084` line).
Run (the correct selector — `reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0085' -count=1`
Expected: PASS (both sides export the same substituted body + attributes + the merged Resource attributes; `logs_written == N`).
Confirm fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **87**.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0085-otlp-access-log-operators/ test/differential/runner_test.go
git commit -m "phase 45.2 Task 8: 0085-otlp-access-log-operators differential — cross-side EXACT on templated body + attributes (incl. nested kvlist/array) + the literal resource_attributes pass-through, poll-to-converge, query-less path (fixtures 86 → 87)"
```

---

## Task 9: `0085` deliberate-break proofs + flake gate + the FULL-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0085` FAILS (the assertion is live), then `git restore` it:
  - (a) Break the operator engine so `%REQ(:METHOD)%` resolves to a wrong constant (e.g. return `"X"` from the `REQ(:METHOD)` extractor) ⇒ the body/attributes substitution assertion must FAIL.
  - (b) Break `CompileOTLPValue` to drop the `kvlist` recursion (return an empty kvlist) ⇒ the `nested` attribute assertion must FAIL.
  - (c) Break `buildResource` to NOT append `resourceAttrs` ⇒ the `service_name`/`authority_literal` Resource-attribute assertion must FAIL.
  - (d) Break the resource_attributes path to operator-SUBSTITUTE `authority_literal` (e.g. run it through `CompileOTLPValue`+`Eval` instead of verbatim) ⇒ the `authority_literal == "%REQ(:AUTHORITY)%"` (LITERAL) assertion must FAIL — proving the AMEND-OPS-1 pass-through is live.
  Run each: `go test ./test/differential/ -run 'TestDifferential/0085' -count=1` ⇒ expect FAIL, then restore ⇒ expect PASS. Record each break+restore in PROGRESS-45.2.md.

- [ ] **Step 2: Flake gate** — 20 consecutive green runs:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0085' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. (A transient `subject ready: EOF` is the startup-race flake — `reference_differential_fullsuite_startup_flake` — isolate-re-run that single run; NOT a 0085 regression.)

- [ ] **Step 3: FULL `internal/accesslog` package `-race`** (`reference_full_suite_race_after_background_mutator`):
```bash
go test ./internal/accesslog/ -race -count=1
```
Expected: PASS, no race.

- [ ] **Step 4: Commit the PROGRESS update**
```bash
git add docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.2.md
git commit -m "phase 45.2 Task 9: 0085 deliberate-break proofs (4 live assertions incl. the AMEND-OPS-1 literal pass-through) + 20/20 flake + full-package -race"
```

---

## Task 10: Full 87-dir differential + six-gate + ADR-0259 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 45 → `done`) + fuzzer reconcile

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.2.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                 # clean
go build ./...                               # ok
go test ./... -count=1                       # full unit + the 87-dir differential
go test ./internal/accesslog/ -race -count=1 # the background-mutator race gate
```
Expected: all green. (The full differential is the byte-stability regression anchor — the `0084` built-in fixture + every non-OTLP fixture MUST stay unmoved.) Also confirm `go mod tidy -diff` is EMPTY (zero go.mod delta this phase).

- [ ] **Step 2: ADR-0259 §Decision/§Consequences** — land them in DECISIONS.md beneath the §Context drafted at SPEC-45.2 §13 (ADR-0044; promote status PROPOSED → ACCEPTED). §Decision: the `internal/accesslog/otlpformat.go` operator engine (registry + `CompileOTLPTemplate`/`CompileOTLPValue`/`ValidateOTLPValue` + `Eval`); the curated `Record`-mapped operator set + STRICT-REJECT of unknown operators + non-string/kvlist/array value-types at parse (bootstrap-compiles-at-parse, D-OTLP-2-COMPILE-SITE; exported `OTLPTemplate`/`OTLPValueTemplate`/`OTLPAttrTemplate`); the `buildLogRecord`/`buildResource` extensions (body/attributes substitution + the resource_attributes append surviving `disable_builtin_labels` — AMEND-OPS-5); the literal `resource_attributes` pass-through (AMEND-OPS-1); all-string output (AMEND-OPS-2); the `FuzzCompileOTLPValue`; the `0085` differential. §Consequences: `formatters` STAYS rejected; arbitrary `%REQ()%`/`%RESP()%` header operators + operators outside the curated set STRICT-REJECT (a documented departure — some valid-Envoy operators boot-reject in envoy-go); `stat_prefix` STAYS PARSE-ACCEPT-but-INERT; the stat surface is UNCHANGED (1191); **row 45 flips `done`** (45.2 is the FINAL leg); the Observability family STAYS OPEN (tracing / stats sinks / tap remain future rows).

- [ ] **Step 3: BEHAVIOR_CONTRACT.md** — extend the `### Access log — OpenTelemetry (OTLP) access-log sink` subsection (SPEC-45.2 §9): body/attributes are per-record command-operator-templated (the curated set); string/`kvlist`/`array` config supported (each string leaf substituted; `int`/`bool`/`double`/`bytes` STRICT-REJECT); operators outside the curated set STRICT-REJECT; resource_attributes are literal extra Resource.attributes (surviving `disable_builtin_labels`); `formatters` STAYS rejected; the stat surface UNCHANGED (1191). (No stat-surface number change — the operator engine adds no stat.)

- [ ] **Step 4: STATE.md + ROADMAP.md** — STATE active-phase → `phase 45.2 (otlp-access-log operator engine) IMPL done`; the count figures → stat **1191** (UNCHANGED) / fixtures **87** / fuzzers **46** / BackendKind **38** / DECISIONS **ADR-0259** / 0 new go.mod modules. **ROADMAP row 45 (`otlp-access-log`) FLIPS `done`** (the FINAL leg, per-leg ADR-0106 + `reference_roadmap_split_phase_row_done`; the Observability FAMILY STAYS OPEN — tracing / stats sinks / tap remain rows). Set the next action → the next Observability-family BRAINSTORM (the next family row).

- [ ] **Step 5: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == **46** and advance the documented running total 45 → 46 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md / PROGRESS-45.2.md consistently.

- [ ] **Step 6: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 45.2 (otlp-access-log operator engine) IMPL: ADR-0259 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 45 → done; Observability family STAYS OPEN); stat 1191 (UNCHANGED) / fixtures 87 / fuzzers 46 / BackendKind 38 / 0 new go.mod modules"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 45.2 (otlp-access-log operator engine) IMPL: the OTLP access-log operator engine — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 45.2 IMPL squash and route the next session to the next Observability-family BRAINSTORM (row 45 is now `done`; the family stays open).
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1191** (H2 cluster; non-H2 **1187**) UNCHANGED / fixtures **87** (tail `0085-otlp-access-log-operators`) / fuzzers **46** (`FuzzCompileOTLPValue`) / BackendKind **38** (UNCHANGED — driver-owned receiver) / DECISIONS **ADR-0259**. ZERO new go.mod modules (`go mod tidy -diff` EMPTY). ZERO new `internal/` packages (the engine is `internal/accesslog/otlpformat.go`). **ROADMAP row 45 FLIPS `done`** (the FINAL leg); the Observability family STAYS OPEN.

> **NOTE on the stat surface non-H2 figure:** the operator engine adds NO stat — the H2-cluster figure stays **1191**, non-H2 **1187** (UNCHANGED from 45.1). Confirm via the six-gate (no registration-test delta this phase).
