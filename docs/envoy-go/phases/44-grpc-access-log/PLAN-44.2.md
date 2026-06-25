# Phase 44.2 Implementation Plan — buffering: lift `buffer_size_bytes` / `buffer_flush_interval` into `ALSConfig` + replace the 44.1 fixed simple flush with an accumulate-and-batch buffer (size-OR-timer trigger) in `GrpcAccessLogSink.run()` + the receiver per-message batch exposure + the `0082` buffering differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** Make envoy-go's gRPC ALS sink honor the two `CommonGrpcAccessLogConfig` flush-policy fields: `buffer_size_bytes` (field 4, default 16384) and `buffer_flush_interval` (field 3, default 1s). The 44.1 writer goroutine Sends one `HTTPAccessLogEntry` per `StreamAccessLogsMessage` (a fixed simple flush); 44.2 introduces a BUFFER inside that goroutine that accumulates entries and flushes a BATCH (all accumulated entries in one message) on EITHER the accumulated serialized-entry-byte sum reaching `buffer_size_bytes` OR a `buffer_flush_interval` timer elapsing, whichever fires first — proven by the `0082-grpc-access-log-buffering` differential (cross-side aggregated 7-field payload + a SUBJECT-side `maxBatchSize >= 2` proof) against `contrib-v1.37.2`.

**Architecture:** This is a localized extension of the phase-44.1 as-built (ADR-0255): the parse arm (`internal/bootstrap`) lifts two more fields into `ALSConfig`; the sink (`internal/accesslog/grpcsink.go`) replaces its `for r := range s.ch` body with a `select` over the channel AND a `time.Ticker`, plus a `buf`/`bufBytes`/`flush()` closure and a flush-on-channel-close step; the `NewGrpcAccessLogSink` signature gains two params; `main.go` passes them through; the driver-owned `test/helpers/accessloggrpc` receiver gains per-message batch exposure. ZERO new Go packages, ZERO new go.mod modules, NO new stat, NO new BackendKind, NO new fuzzer (the existing parse fuzzer covers the new fields). Byte-identical when no ALS `access_log` entry is configured (the file-only path is untouched; the full differential is the regression anchor). ANCHORS ADR-0256; row 44 STAYS `in-progress` (44.3 header-capture remains); the Observability family STAYS OPEN.

**Tech Stack:** Go; the in-tree `internal/accesslog` sink subsystem (the 44.1 `GrpcAccessLogSink` writer-goroutine shape); `internal/bootstrap` (the `parseGrpcAccessLog` arm + `ALSConfig`); `google.golang.org/protobuf/proto` (`proto.Size` for the byte accounting — already a direct module dep) + `time` (the ticker); the resolved `go-control-plane/envoy v1.32.4` ALS protos (the `CommonGrpcAccessLogConfig.GetBufferSizeBytes()`/`GetBufferFlushInterval()` getters at `als.pb.go:253`/`:260`); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The gRPC Access Log Service (ALS) sink was BUILT at phase 44.1 (squash `ec05a421`, ADR-0255): a `GrpcAccessLogSink` (in `internal/accesslog/grpcsink.go`) streams structured `HTTPAccessLogEntry` protos to an Envoy `AccessLogService.StreamAccessLogs` client-streaming RPC. At 44.1 the sink's writer goroutine drains a bounded channel one `Record` at a time and Sends ONE entry per `StreamAccessLogsMessage` — a "fixed simple flush". Two config fields that control batching — `buffer_size_bytes` and `buffer_flush_interval` — are currently PARSE-ACCEPTED-but-INERT (the bootstrap parser reads the surrounding config but ignores these two).

Your job (phase 44.2) is to make those two fields LIVE. The writer goroutine must accumulate entries into a buffer and flush the whole buffer as ONE batched message when EITHER (a) the running sum of serialized entry bytes reaches `buffer_size_bytes` (the SIZE trigger) OR (b) a flush-interval timer fires (the TIMER trigger). The defaults are `buffer_size_bytes`=16384 and `buffer_flush_interval`=1s (empirically pinned against real Envoy in the SPEC's §11 live probe). An explicit `buffer_size_bytes: 0` degenerates to flush-every-entry; the interval default is a HARD panic-guard (`time.NewTicker(d)` panics for `d <= 0`, so the parse layer MUST coerce nil/zero → 1s).

The differential test harness boots BOTH the real reference Envoy (in Docker, `contrib-v1.37.2`) AND the in-process subject (envoy-go), drives the same traffic at both, and asserts equivalence. The reference buffers PER-WORKER-THREAD (un-pinned worker count) while envoy-go uses ONE process-global buffer — so cross-side batch/message COUNTS are infeasible to compare. The `0082` differential therefore asserts (1) the per-entry PAYLOAD aggregated across all received entries cross-side (the 44.1 AMEND-ALS-3 discipline) and (2) a SUBJECT-side-only proof that envoy-go produced at least one message carrying ≥2 entries (`maxBatchSize >= 2`) — which BITES against a regression to the 44.1 one-entry-per-message flush.

### Key source seams (verified at PLAN time against the tree at master `52912c5e`; re-confirm line numbers before editing — files evolve)

- **`internal/accesslog/grpcsink.go`** — the 44.1 as-built this leg modifies:
  - `const closeDrainGrace = 5 * time.Second` (`:30`) — the bounded-Close grace. UNCHANGED.
  - `type GrpcAccessLogSink struct` (`:40`) — fields `ch chan any`, `client alsClient`, `logName`, `node *corev3.Node`, `logsWritten`/`logsDropped *stats.Counter`, `done`, `closeOnce`, `closeErr`, `lastDropLog`, `ctx`/`cancel`. **ADD** `bufferSizeBytes int` + `bufferFlushInterval time.Duration`.
  - `NewGrpcAccessLogSink(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter) *GrpcAccessLogSink` (`:60`) — **EXTEND** the signature with `bufferSizeBytes int, bufferFlushInterval time.Duration` (and thread through `newGrpcSinkWithCapacity` at `:66`).
  - `Submit` (`:84`) — UNCHANGED (drop-newest, `logsDropped.Inc()`).
  - `Close` (`:108`) — UNCHANGED in shape (`close(s.ch)`; await `done` up to grace; `cancel()`; `client.Close()`). The drain CHANGE lives in `run()`'s channel-closed path, not here.
  - `run()` (`:127`) — the `for r := range s.ch { … Send one entry … }` loop body + the trailing `CloseAndRecv`. **THIS IS THE CORE REWRITE** (§ Task 4). The 44.1 per-record up-to-two-attempts reconnect loop (`:138`–`:171`) is preserved but lifted into the `flush()` closure (resend the WHOLE batch once on a Send error). The `s.logsWritten.Inc()` at `:169` becomes `s.logsWritten.Add(uint64(len(buf)))` in `flush()`.
- **`internal/accesslog/mapping.go`** — `buildHTTPAccessLogEntry(rec *Record) *dataaccesslogv3.HTTPAccessLogEntry` (the 10-field mapping). UNCHANGED — the buffer accumulates whatever it returns; `proto.Size(entry)` is computed on the returned message.
- **`internal/accesslog/stats.go`** — `RegisterGrpcSinkCounters(reg) (written, dropped *stats.Counter)`. UNCHANGED — NO new stat (AMEND-BUF-4). The registration test (`stats_test.go`) still asserts the +0 delta (surface stays 1189).
- **`internal/stats/counter.go`** — `func (c *Counter) Inc()` (`:22`) + **`func (c *Counter) Add(delta uint64)` (`:27`)** — the batch-invariant counter bump uses `Add`.
- **`internal/bootstrap/bootstrap.go`** —
  - `httpGrpcAccessLogTypeURL` const (`:161`). UNCHANGED.
  - `type ALSConfig struct { ClusterName, LogName string }` (`:183`). **ADD** `BufferSizeBytes uint32` + `BufferFlushInterval time.Duration`.
  - `parseGrpcAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error` (`:349`) — after the STRICT-REJECT arms + before the `append`, **ADD** the two buffer reads with defaults. The `proto.Unmarshal` import (`"google.golang.org/protobuf/proto"` at `:141`) is present; **ADD** `"time"` to the imports (NOT currently imported — verified).
  - `corev3` is already imported (used in the V2 reject at `:355`); the buffer getters return `*durationpb.Duration` / `*wrapperspb.UInt32Value` but we call `.AsDuration()` / `.GetValue()` on the getter results, so NO new `durationpb`/`wrapperspb` import is needed (only the methods, not the named types).
- **`cmd/envoy-go/main.go`** — the 44.1 sink-build block builds `accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped)` per `ALSConfig`. **EXTEND** the call with `cfg.BufferSizeBytes, cfg.BufferFlushInterval`. (Find it with `grep -n 'NewGrpcAccessLogSink' cmd/envoy-go/main.go`.)
- **`test/helpers/accessloggrpc/accessloggrpc.go`** — `Server` with `entries []*HTTPAccessLogEntry` + RWMutex + `StreamAccessLogs` (appends `httpLogs.GetLogEntry()...` per message, `:140`) + `Entries()`/`Count()`/`Reset()`/`Addr()`/`Stop()`/`Close()`. **ADD** `batchSizes []int` + per-message append of `len(httpLogs.GetLogEntry())` under the same mutex + `BatchSizes() []int` / `MaxBatchSize() int` / `MessageCount() int` accessors; `Reset()` must ALSO clear `batchSizes`.
- **`internal/bootstrap/alsconfig_fuzz_test.go`** — `FuzzParseHttpGrpcAccessLogConfig` (the EXISTING parse fuzzer; seed corpus at `:16`–`:21`). **ADD** buffer-bearing seed-corpus entries; NO new `^func Fuzz`.
- **`test/fixtures/0081-grpc-access-log/`** — the differential precedent to COPY for `0082`. Note the directory uses `driver/driver.go` (NOT `inputs/`). The driver registers via `fixture.RegisterFixture`, owns the receiver (`accessloggrpc.NewAtAddr("0.0.0.0:<port>")`), bakes the SAME ALS port into both YAMLs (`host.docker.internal` reference / `127.0.0.1` subject), fires N requests, polls `Count()` to converge, and asserts the 7-field subset per entry + the subject `logs_written` stat. The data-plane backend is `fixture.HTTPFixedBody` (17-byte body).
- **`test/differential/runner_test.go`** — blank-imports each fixture's `driver` package (the auto-discovery seam). **ADD** the `0082` driver import.

### Proto facts (verified at PLAN time against `go-control-plane/envoy@v1.32.4` in the module cache — re-confirm at IMPL)

- `CommonGrpcAccessLogConfig.GetBufferFlushInterval() *durationpb.Duration` (`als.pb.go:253`) — field 3. `.AsDuration()` → `time.Duration`; nil-safe (returns the zero `*durationpb.Duration` → `0s`).
- `CommonGrpcAccessLogConfig.GetBufferSizeBytes() *wrapperspb.UInt32Value` (`als.pb.go:260`) — field 4. Returns the WRAPPER POINTER (nil when the field is absent); `.GetValue()` → `uint32`. Use the nil-check on the getter result to distinguish absent (→ default 16384) from explicit-0 (→ flush-every-entry).
- `proto.Size(m proto.Message) int` (`google.golang.org/protobuf/proto`) == the C++ `ByteSizeLong()` — the serialized byte size of one `*HTTPAccessLogEntry`. Already a direct module dep (`proto.Unmarshal` in `bootstrap.go`); needs a NEW import line only in `grpcsink.go`.

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path (this plan lives in the worktree at `.worktrees/phase-44.2-buffering-plan`); the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0082'`, NEVER bare `'0082'` (which matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the sink's writer goroutine is a background mutator, and 44.2 ADDS a `time.Ticker` (a second goroutine writing `ticker.C`) inside it; after the buffer machinery lands run the FULL `internal/accesslog` package `-race`, NOT a `-run` subset.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing` / AMEND-ALS-3 REINFORCED): assert the per-entry PAYLOAD aggregated across messages, NEVER stream/message/batch framing cross-side. The subject batching proof is SUBJECT-side ONLY.
- **Coalescence determinism** (SPEC §8.1 caveat): the subject `maxBatchSize >= 2` proof only holds if ≥2 entries land in one flush interval — the drive MUST guarantee coalescence (fire concurrently / rapid-fire) + a generous interval, verified non-flaky over 20/20.

---

## D-question resolutions (the SPEC §12 D-BUF-* PLAN pins — settled here)

**D-BUF-TIMER-MECHANISM → `time.NewTicker` (fixed period; empty-buffer no-op tick).**
- The writer goroutine holds `ticker := time.NewTicker(s.bufferFlushInterval); defer ticker.Stop()` and `select`s `<-ticker.C` → `flush()`. When the buffer is empty the tick is a cheap no-op branch (`if len(buf)==0 { return }`). This is the SPEC §3.2 default. A reset-on-flush `time.Timer` (the "interval since last flush" semantics, marginally closer to the reference's per-flush timer) is REJECTED: it adds Stop/Reset/drain-the-channel state for ZERO observable behavioral gain — the differential asserts aggregated payload + a `maxBatchSize >= 2` lower bound, neither of which distinguishes a fixed-period ticker from a reset timer. The empty-buffer no-op tick is acceptable (a single length check per idle interval; the sink is not a hot path).
- **PANIC-GUARD invariant:** `time.NewTicker(d)` PANICS for `d <= 0`. The parse layer (Task 2) coerces nil/zero `buffer_flush_interval` → 1s, so the value reaching `NewTicker` is ALWAYS positive. Task 4's test suite includes a construction test that a sink built with the parse-layer default never panics; the sink does NOT re-guard (the invariant is the parse layer's, documented at both ends).

**D-BUF-FUZZER-CORPUS → extend the EXISTING `FuzzParseHttpGrpcAccessLogConfig` seed corpus; NO new `^func Fuzz`.**
- The 44.1 fuzzer already feeds arbitrary bytes through `parseGrpcAccessLog`, which now reads the two buffer fields — so the buffer-parse path is fuzz-covered for free. Add buffer-bearing seed-corpus entries (a `common_config` carrying `buffer_size_bytes` and/or `buffer_flush_interval`: a tiny size, an explicit 0, a huge value, a sub-millisecond interval, a very-long interval) to exercise the default/edge arms with realistic structure. Fuzzers STAY **44** (re-verify `^func Fuzz` == 44 at the completion task per `reference_fuzzer_count_docs_drift`). The invariant is unchanged: `parseGrpcAccessLog` NEVER panics and returns nil or a `bootstrap:`-prefixed error.

**D-BUF-RECEIVER-BATCH-API → `BatchSizes() []int` + `MaxBatchSize() int` + `MessageCount() int`; `Reset()` clears `batchSizes`; the existing RWMutex.**
- Add a `batchSizes []int` field to `accessloggrpc.Server`, appended `len(httpLogs.GetLogEntry())` per message INSIDE the existing `s.mu.Lock()` block in `StreamAccessLogs` (alongside the entry append — one lock acquisition, no new lock). Expose all three: `BatchSizes()` (a defensive copy, like `Entries()`), `MaxBatchSize()` (the max element, 0 for empty), `MessageCount()` (`len(batchSizes)`). `Reset()` sets BOTH `entries = nil` AND `batchSizes = nil` under the lock. Rationale for all three: `MaxBatchSize() >= 2` is the assertion that bites; `MessageCount()` + the entry count gives `messageCount < N` as an equivalent cross-check + a useful diagnostic; `BatchSizes()` is the raw dump for `FIXTURE_0082_DUMP` debugging. The mutex discipline matches the 44.1 `entries` field exactly (the `-race` gate covers it).

**D-BUF-DIFFERENTIAL-DRIVE → large `buffer_size_bytes` (1048576, never hit) + a wide `buffer_flush_interval` (1s) + N=16 fired CONCURRENTLY; subject proof `MaxBatchSize() >= 2`, verified 20/20.**
- **Buffer settings (both YAMLs):** `buffer_size_bytes: 1048576` (1 MiB — N small entries never reach it, so the SIZE trigger never fires; the byte-accounting-fragile size-cap path is deliberately AVOIDED cross-side per SPEC §8.1) + `buffer_flush_interval: 1s` (the TIMER is the deterministic flush lever).
- **N = 16** requests (double the 0081 N=8 — a comfortable margin for `maxBatchSize >= 2` while keeping the suite fast).
- **Burst pattern:** fire all N CONCURRENTLY via a `sync.WaitGroup` of goroutines (a SHARED keep-alive-disabled client is fine — the point is that the N `Submit`s queue into envoy-go's single process-global buffer within the 1s interval, FASTER than the timer elapses, so the next tick flushes ≥2 of them as one batch). This REPLACES the 0081 sequential `DisableKeepAlives` loop. The concurrent fan-out + the 1s interval keeps a wide margin over the per-request round-trip (sub-millisecond loopback), so ≥2 entries reliably coalesce. (Contrast: the 0081 sequential-with-keepalives-disabled drive would let entries flush singly under a tight interval — a false-negative `maxBatchSize == 1` flake; concurrency is the coalescence guarantee.)
- **Poll-to-converge:** poll `srv.Count()` to `>= N` (the 0081 `pollCount` shape; NEVER `time.Sleep` — `reference_concurrency_differential_release_barrier`). With a 1s interval the converge takes ≤~1–2s per side.
- **Cross-side assertion:** the 7-field subset per entry, aggregated over all N entries, BOTH sides (the 0081 `assertEntries` verbatim) + subject `logs_written == N`.
- **Subject batching proof:** `subjBatchSizes`'s max `>= 2` (snapshot `srv.BatchSizes()` per side alongside the entries, like `refEntries`/`subjEntries`). Asserted SUBJECT-side ONLY. **Non-flaky over 20/20** (Task 7 gate) — if it flakes, widen the interval to 2s and/or raise N before any other change.
- **Per-side separation:** `Reset()` between sides (clears entries AND batchSizes), the 0081 idiom.

**D-BUF-SPLIT-FINAL → one leg (re-checked, ~110–150 prod LoC, well under the ADR-0045 soft gate).**
- Estimated prod LoC: the `ALSConfig` two fields + the parse-arm reads/defaults ≈ 18; the buffer machinery in `grpcsink.go` (the two struct fields + the signature/constructor threading + the `buf`/`bufBytes`/ticker/`flush()` rewrite of `run()` + the flush-on-close) ≈ 90; the receiver `batchSizes` field + 3 accessors + the `Reset` clear ≈ 22; `main.go` pass-through ≈ 2. Total ≈ **132 prod LoC** — comfortably under the ADR-0045 soft gate. Ships as ONE leg (the SPEC chartered 44.2 as the buffering leg; header-capture 44.3 is separately chartered). No further split.

---

## File structure (decomposition locked here)

**Production (touched):**
- `internal/bootstrap/bootstrap.go` — MODIFY: add `"time"` import; the two `ALSConfig` buffer fields; the buffer reads + defaults in `parseGrpcAccessLog`.
- `internal/accesslog/grpcsink.go` — MODIFY: add `"google.golang.org/protobuf/proto"` import; the two sink fields; the `NewGrpcAccessLogSink`/`newGrpcSinkWithCapacity` signature extension; the `run()` buffer rewrite (`buf`/`bufBytes`/ticker/`flush()` + flush-on-close).
- `cmd/envoy-go/main.go` — MODIFY: pass `cfg.BufferSizeBytes`/`cfg.BufferFlushInterval` to `NewGrpcAccessLogSink`.
- `test/helpers/accessloggrpc/accessloggrpc.go` — MODIFY: the `batchSizes` field + 3 accessors + the `Reset` clear.

**Test (created / modified):**
- `internal/bootstrap/bootstrap_test.go` — MODIFY: the buffer-field parse table tests (default / wrapper-absent / explicit-0 / explicit-value / interval default-coerce).
- `internal/bootstrap/alsconfig_fuzz_test.go` — MODIFY: buffer-bearing seed corpus.
- `internal/accesslog/grpcsink_test.go` — MODIFY: the buffer-machinery tests (size-trigger batch / timer-trigger batch / flush-on-close / `logs_written` batch-invariant / no-panic-with-default-interval) against the fake `alsClient`.
- `test/helpers/accessloggrpc/accessloggrpc_test.go` — MODIFY: the batch-exposure tests (per-message batch recording / `MaxBatchSize` / `MessageCount` / `Reset` clears).
- `test/fixtures/0082-grpc-access-log-buffering/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0082` driver package.

**Docs (completion task):**
- `docs/envoy-go/DECISIONS.md` (ADR-0256 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.2.md`.

---

## Task 1: Phase scaffolding — PROGRESS-44.2.md + baselines + the final ADR-0045 split re-check (D-BUF-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.2.md`

- [ ] **Step 1: Record the baseline counts**

Run and record the verbatim outputs in PROGRESS-44.2.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                          # expect 83 (incl the letter-suffixed 0007a/0007b; tail 0081-grpc-access-log). NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS by 2 — use the glob form.
grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'   # expect 44
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go            # expect = 38 (the BackendKind tail)
```
Baseline: stat surface **1189** (H2 cluster; non-H2 **1185**) / fixtures **83** / fuzzers **44** / BackendKind **38** / DECISIONS tail **ADR-0255** (next-free **ADR-0256**).

- [ ] **Step 2: Write the PROGRESS-44.2.md scaffold** — a header (phase 44.2 IMPL, the SPEC-44.2 reference, the worktree branch `phase-44.2-buffering-plan`), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1189** (UNCHANGED — NO new buffering stat, AMEND-BUF-4) / fixtures **84** (`0082-grpc-access-log-buffering`) / fuzzers **44** (UNCHANGED — the existing parse fuzzer covers the buffer fields) / BackendKind **38** (UNCHANGED — the receiver is driver-owned) / DECISIONS **ADR-0256**.

- [ ] **Step 3: Record the D-BUF-SPLIT-FINAL re-check** — note the ~132 prod-LoC estimate (the breakdown in the D-question section above), confirm it sits well under the ADR-0045 soft gate, and that 44.2 ships as ONE leg. (Bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.2.md
git commit -m "phase 44.2 Task 1: PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-BUF-SPLIT-FINAL)"
```

---

## Task 2: Lift the two buffer fields into `ALSConfig` with the reference defaults (`internal/bootstrap`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (extend the existing grpc-ALS parse table — the `Load(strings.NewReader(yaml))` shape). Assert on `bs.ALSConfigs[0].BufferSizeBytes` (uint32) + `.BufferFlushInterval` (time.Duration):
  - **default-both (wrapper + duration ABSENT)**: a `common_config` with only `log_name` + `grpc_service` ⇒ `BufferSizeBytes == 16384` AND `BufferFlushInterval == 1 * time.Second` (AMEND-BUF-2 defaults).
  - **explicit-size**: `buffer_size_bytes: 4096` ⇒ `BufferSizeBytes == 4096`.
  - **explicit-zero-size**: `buffer_size_bytes: 0` (wrapper PRESENT, value 0) ⇒ `BufferSizeBytes == 0` (honored — flush-every-entry; NOT coerced to the default). NOTE: in protojson/YAML a `UInt32Value` of 0 still renders the wrapper present — confirm the test YAML produces a present-but-zero wrapper (if the YAML round-trip elides it, assert via a directly-constructed `*anypb.Any` through `parseGrpcAccessLog` instead, the fuzzer's `anypb.Any` idiom).
  - **explicit-interval-subsecond**: `buffer_flush_interval: 0.2s` ⇒ `BufferFlushInterval == 200 * time.Millisecond` (positive sub-second honored verbatim).
  - **interval-zero-coerced**: `buffer_flush_interval: 0s` (duration present, zero) ⇒ `BufferFlushInterval == 1 * time.Second` (the PANIC-GUARD coercion — a 0 interval would panic `NewTicker`).
  - **interval-absent-coerced**: duration omitted ⇒ `BufferFlushInterval == 1 * time.Second`.
  - **buffer-with-strict-reject**: a config with `buffer_size_bytes` set AND `google_grpc` ⇒ STILL errors on `google_grpc` (the reject arms run BEFORE the buffer reads; the buffer fields add no new accept-path that bypasses a reject).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: FAIL (`ALSConfig` has no `BufferSizeBytes`/`BufferFlushInterval` field).

- [ ] **Step 3: Implement** in `bootstrap.go`:

Add `"time"` to the import block (verify it is not already present — it is NOT at PLAN time).

Extend the struct (`:183`):
```go
type ALSConfig struct {
	ClusterName         string        // common_config.grpc_service.envoy_grpc.cluster_name
	LogName             string        // common_config.log_name (empty is valid)
	BufferSizeBytes     uint32        // common_config.buffer_size_bytes (default 16384 when wrapper absent; explicit 0 ⇒ flush-every-entry) — AMEND-BUF-2
	BufferFlushInterval time.Duration // common_config.buffer_flush_interval (default 1s when absent/zero — a NewTicker(<=0) panic-guard) — AMEND-BUF-2
}
```
In `parseGrpcAccessLog` (`:349`), after the empty-cluster reject (`:365`) and before the `append` (`:366`), read the two fields with defaults:
```go
// buffer_size_bytes (field 4): the WRAPPER pointer is nil when the field is
// absent ⇒ default 16384 (AMEND-BUF-2); an explicit present-but-0 value is
// honored as flush-every-entry (the size threshold sum >= 0 always fires).
bufferSizeBytes := uint32(16384)
if w := common.GetBufferSizeBytes(); w != nil {
	bufferSizeBytes = w.GetValue()
}
// buffer_flush_interval (field 3): absent/zero ⇒ default 1s. This is a HARD
// panic-guard, not merely a fidelity default — time.NewTicker(d) panics for
// d <= 0, so a non-positive interval MUST be coerced before it reaches the
// sink's ticker (§3.2). Any POSITIVE explicit value (incl. sub-second) is
// honored verbatim.
bufferFlushInterval := common.GetBufferFlushInterval().AsDuration()
if bufferFlushInterval <= 0 {
	bufferFlushInterval = time.Second
}
result.ALSConfigs = append(result.ALSConfigs, ALSConfig{
	ClusterName:         eg.GetClusterName(),
	LogName:             common.GetLogName(),
	BufferSizeBytes:     bufferSizeBytes,
	BufferFlushInterval: bufferFlushInterval,
})
```
Update the `ALSConfig` doc comment (`:178`) + the `Bootstrap.ALSConfigs` doc comment (`:210`) to note the two buffer fields are now CONSUMED (no longer parse-accept-but-inert); `additional_*_headers` + `AccessLog.filter` STAY inert (44.3).

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/bootstrap/ -run TestLoad -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/
git commit -m "phase 44.2 Task 2: lift buffer_size_bytes/buffer_flush_interval into ALSConfig with 16384/1s defaults (AMEND-BUF-2; the 1s interval is a NewTicker panic-guard)"
```

---

## Task 3: Buffer-bearing fuzzer seed corpus (D-BUF-FUZZER-CORPUS; NO new `^func Fuzz`) [fuzz]

**Files:**
- Modify: `internal/bootstrap/alsconfig_fuzz_test.go`

- [ ] **Step 1: Add buffer-bearing seed entries** to the existing `FuzzParseHttpGrpcAccessLogConfig` `f.Add(...)` block. The cleanest way to produce valid buffer-bearing seeds is to marshal real `*grpcalv3.HttpGrpcAccessLogConfig` messages (a small helper in the test) — but to keep the fuzzer dependency-light, raw protobuf wire bytes for the nested `common_config` are acceptable too. Add seeds covering: a `common_config` with `buffer_size_bytes` tiny (e.g. 1), explicit 0, huge (e.g. `0xffffffff`); `buffer_flush_interval` sub-millisecond, very-long, and absent. If using marshalled messages, add a `//nolint` if the linter flags the test-only marshal helper. The invariant is UNCHANGED: `parseGrpcAccessLog` NEVER panics, returns nil or a `bootstrap:`-prefixed error. Add a comment noting the buffer fields are now part of the fuzzed surface (44.2).

- [ ] **Step 2: Run the fuzzer briefly to confirm it executes**

Run: `go test ./internal/bootstrap/ -run 'FuzzParseHttpGrpcAccessLogConfig' -count=1` then `go test ./internal/bootstrap/ -fuzz 'FuzzParseHttpGrpcAccessLogConfig' -fuzztime 20s`
Expected: PASS / no crashers.

- [ ] **Step 3: Confirm the count is UNCHANGED at 44**

Run: `grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'`
Expected: **44** (UNCHANGED — no new fuzzer; only seed corpus added). Record in PROGRESS-44.2.md.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/alsconfig_fuzz_test.go
git commit -m "phase 44.2 Task 3: buffer-bearing seed corpus for FuzzParseHttpGrpcAccessLogConfig (D-BUF-FUZZER-CORPUS; fuzzers stay 44)"
```

---

## Task 4: The buffer machinery in `GrpcAccessLogSink` — accumulate + size-trigger + interval-timer + flush-on-close (`internal/accesslog/grpcsink.go`) [TDD, `-race`]

**Files:**
- Modify: `internal/accesslog/grpcsink.go`
- Test: `internal/accesslog/grpcsink_test.go`

This is the CORE rewrite. The 44.1 `run()` loop Sends one entry per record; 44.2 accumulates into `buf` and Sends a BATCH on size-OR-timer-OR-close. The fixed-flush behavior is the `buffer_size_bytes: 0` degenerate case (every entry crosses `sum >= 0` immediately).

- [ ] **Step 1: Write the failing tests** in `grpcsink_test.go` against the existing fake `alsClient` (which records every `Send`ed `*StreamAccessLogsMessage`). The existing fake records messages; assert on `len(msg.GetHttpLogs().GetLogEntry())` per message to see batching. The test constructor `newGrpcSinkWithCapacity` now takes the two buffer params — update existing call sites + add:
  - **size-trigger batches**: build a sink with a `bufferSizeBytes` small enough that ~3 entries cross it and a LONG `bufferFlushInterval` (e.g. 10s, so the timer never fires during the test); `Submit` 6 records; after Close, assert ≥1 message carries ≥2 entries (the size trigger batched), and the total entry count across all messages == 6, and `logsWritten == 6`.
  - **timer-trigger batches**: build a sink with a HUGE `bufferSizeBytes` (never hit) and a SHORT `bufferFlushInterval` (e.g. 50ms); `Submit` several records rapidly (faster than the interval), then wait for ≥1 tick (poll the fake's received-message count to converge, NEVER a bare sleep-then-assert — poll with a deadline); assert the entries arrived in a batch (≥1 message with ≥2 entries) via the timer; `logsWritten` == the count.
  - **flush-on-close (AMEND-BUF-5)**: build a sink with a HUGE size + LONG interval (neither trigger fires); `Submit` 3 records; `Close()`; assert all 3 entries were flushed in the final batched message before `CloseAndRecv` (none lost), `logsWritten == 3`.
  - **logs_written batch-invariant**: across any of the above, `logsWritten` counts ENTRIES not messages (assert `logsWritten == totalEntries`, `messageCount < totalEntries` in the batched case).
  - **explicit-zero-size = flush-every-entry**: `bufferSizeBytes == 0` + LONG interval; `Submit` 4 records; assert 4 messages each with exactly 1 entry (the degenerate fixed-flush — `sum >= 0` always crosses); `logsWritten == 4`.
  - **identifier-once carries**: the FIRST flushed message carries a non-nil `GetIdentifier()` (node + log_name); subsequent flushed messages carry `GetIdentifier() == nil` (re-armed only across a reconnect).
  - **reconnect-resends-whole-batch**: the fake errors on the first `Send`, succeeds after; `Submit` enough to form a batch + flush; assert the sink re-opens the stream (re-attaches the identifier) and resends the WHOLE batch; the entries still land; `logsWritten` counts the eventually-sent batch ONCE (not double).
  - **no-panic-with-parse-default-interval**: construct a sink with `bufferFlushInterval = 1 * time.Second` (the parse-layer default) ⇒ no panic (the `NewTicker` guard invariant; a regression that passed 0 here would panic).
  - **non-Record-ignored**: `Submit("garbage")` ⇒ no panic, not buffered, not counted.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/accesslog/ -run TestGrpcSink -count=1`
Expected: FAIL (the constructor signature mismatch + no batching behavior).

- [ ] **Step 3: Implement** in `grpcsink.go`:

Add `"google.golang.org/protobuf/proto"` to the imports. Add two struct fields to `GrpcAccessLogSink`:
```go
	bufferSizeBytes     int           // accumulated-serialized-byte flush threshold (AMEND-BUF-1); 0 ⇒ flush-every-entry
	bufferFlushInterval time.Duration // flush-interval timer period (AMEND-BUF-2; guaranteed > 0 by the parse layer)
```
Extend `NewGrpcAccessLogSink` + `newGrpcSinkWithCapacity` signatures (and set the fields in the struct literal):
```go
func NewGrpcAccessLogSink(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *GrpcAccessLogSink {
	return newGrpcSinkWithCapacity(client, logName, node, written, dropped, bufferSizeBytes, bufferFlushInterval, defaultChannelCapacity)
}

func newGrpcSinkWithCapacity(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, capacity int) *GrpcAccessLogSink {
	s := &GrpcAccessLogSink{
		ch:                  make(chan any, capacity),
		client:              client,
		logName:             logName,
		node:                node,
		logsWritten:         written,
		logsDropped:         dropped,
		bufferSizeBytes:     bufferSizeBytes,
		bufferFlushInterval: bufferFlushInterval,
		done:                make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}
```
Rewrite `run()` (replace the `for r := range s.ch { … }` body + the trailing `CloseAndRecv`). The `flush()` closure owns the up-to-two-attempts reconnect-and-resend-the-WHOLE-batch logic (lifted from the 44.1 per-record loop), `logsWritten.Add(len(buf))` on success, and resets `buf`/`bufBytes`:
```go
func (s *GrpcAccessLogSink) run() {
	defer close(s.done)

	var stream accesslogv3.AccessLogService_StreamAccessLogsClient
	sentIdentifier := false
	var buf []*dataaccesslogv3.HTTPAccessLogEntry
	bufBytes := 0

	// flush Sends the accumulated batch as ONE StreamAccessLogsMessage, with the
	// identifier on the first successful flush of the stream's life (re-armed
	// across a reconnect). Up to two attempts: the initial Send plus one
	// reconnect-and-resend-the-whole-batch. On success logsWritten += len(buf)
	// (batch-invariant — AMEND-BUF-4); on a second failure the batch is dropped
	// (logged, not counted). Empty buf is a no-op (the timer's idle tick).
	flush := func() {
		if len(buf) == 0 {
			return
		}
		for attempt := 0; attempt < 2; attempt++ {
			if stream == nil {
				st, err := s.client.StreamAccessLogs(s.ctx)
				if err != nil {
					log.Printf("accesslog: gRPC ALS open stream (log_name=%s): %v", s.logName, err)
					return // leave stream nil; the buffer is kept for the next flush attempt
				}
				stream = st
				sentIdentifier = false
			}
			msg := &accesslogv3.StreamAccessLogsMessage{
				LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: buf,
					},
				},
			}
			if !sentIdentifier {
				msg.Identifier = &accesslogv3.StreamAccessLogsMessage_Identifier{
					Node:    s.node,
					LogName: s.logName,
				}
			}
			if err := stream.Send(msg); err != nil {
				log.Printf("accesslog: gRPC ALS send (log_name=%s): %v", s.logName, err)
				stream = nil
				sentIdentifier = false
				continue // reconnect-and-resend the whole batch once
			}
			sentIdentifier = true
			s.logsWritten.Add(uint64(len(buf)))
			break
		}
		// Reset the buffer whether the batch was sent or dropped (a dropped batch
		// is logged-not-counted, matching the 44.1 one-shot-reconnect drop policy).
		buf = buf[:0]
		bufBytes = 0
	}

	ticker := time.NewTicker(s.bufferFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-s.ch:
			if !ok {
				flush() // AMEND-BUF-5: drain the pending buffer before CloseAndRecv
				if stream != nil {
					if _, err := stream.CloseAndRecv(); err != nil {
						log.Printf("accesslog: gRPC ALS close-and-recv (log_name=%s): %v", s.logName, err)
					}
				}
				return
			}
			rec, ok := r.(*Record)
			if !ok {
				log.Printf("accesslog: gRPC ALS sink got non-*Record %T (log_name=%s); dropping", r, s.logName)
				continue
			}
			entry := buildHTTPAccessLogEntry(rec)
			buf = append(buf, entry)
			bufBytes += proto.Size(entry)
			if bufBytes >= s.bufferSizeBytes { // SIZE trigger (AMEND-BUF-1); 0 ⇒ every entry flushes
				flush()
			}
		case <-ticker.C:
			flush() // TIMER trigger (no-op if buf empty)
		}
	}
}
```
**Note on the `buf` reset:** `buf = buf[:0]` reuses the backing array — but `flush()` sets `msg.HttpLogs.LogEntry = buf`, and a reused backing array would be overwritten by the next batch's `append` BEFORE the fake/real Send has serialized it. The real gRPC `Send` serializes synchronously before returning, so the bytes are captured before `buf[:0]` reuse — SAFE for production. HOWEVER the test fake records the `*StreamAccessLogsMessage` POINTER (whose `LogEntry` slice header still points at the shared backing array) — a reused backing array would corrupt earlier recorded messages. **Two options, pick at IMPL:** (a) the fake takes a defensive copy of `GetLogEntry()` when recording (the cleaner fix — the fake is test-only); OR (b) `run()` allocates a fresh slice per flush (`buf = nil` instead of `buf[:0]`) — marginally more GC but trivially correct for both fake and real. **Prefer (a)** (the fake copies on record) to keep production zero-extra-allocation; document the reuse contract in a `flush()` comment. If (a) proves awkward, fall back to (b) and note the allocation in PROGRESS.

- [ ] **Step 4: Run to verify they pass + the FULL-package race**

Run: `go test ./internal/accesslog/ -run TestGrpcSink -count=1`
Expected: PASS.
Then the FULL package `-race` (the writer goroutine + the NEW ticker goroutine are background mutators — `reference_full_suite_race_after_background_mutator`):
Run: `go test ./internal/accesslog/ -race -count=1`
Expected: PASS, no race.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/accesslog/ && golangci-lint run ./internal/accesslog/... && go vet ./internal/accesslog/... && go build ./...
git add internal/accesslog/grpcsink.go internal/accesslog/grpcsink_test.go
git commit -m "phase 44.2 Task 4: GrpcAccessLogSink buffer machinery — accumulate + size-OR-timer flush + flush-on-close (AMEND-BUF-1/4/5); logs_written batch-invariant"
```

---

## Task 5: Boot wiring pass-through (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go`

main.go is not unit-tested in isolation (the differential is its behavioral proof); the gate here is build + the `0082` fixture (Task 7).

- [ ] **Step 1: Implement** — extend the `NewGrpcAccessLogSink` call in the 44.1 ALS sink-build block (find it with `grep -n 'NewGrpcAccessLogSink' cmd/envoy-go/main.go`) to pass the two new `ALSConfig` fields:
```go
sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval))
```
(`cfg.BufferSizeBytes` is `uint32` → cast to `int` for the sink field; `cfg.BufferFlushInterval` is already `time.Duration`. No new import.)

- [ ] **Step 2: Build + boot-smoke**

Run: `go build ./... && echo BUILD_OK`
Then a manual boot-smoke against a hand-written bootstrap with a gRPC-ALS `access_log` carrying `buffer_size_bytes`/`buffer_flush_interval` pointing at a valid H2 cluster ⇒ boots clean (the sink's ticker starts; no panic).

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 44.2 Task 5: boot wiring — pass cfg.BufferSizeBytes/BufferFlushInterval through to NewGrpcAccessLogSink"
```

---

## Task 6: The `accessloggrpc` receiver per-message batch exposure (D-BUF-RECEIVER-BATCH-API) [TDD]

**Files:**
- Modify: `test/helpers/accessloggrpc/accessloggrpc.go`
- Test: `test/helpers/accessloggrpc/accessloggrpc_test.go`

The receiver accumulates a FLAT `[]*HTTPAccessLogEntry` (44.1); 44.2 ADDS a per-message batch-size record so the `0082` driver can prove the subject batched (`MaxBatchSize >= 2`).

- [ ] **Step 1: Write the failing tests** in `accessloggrpc_test.go` (extend the existing tests): start a `New(t)`, dial it, open `StreamAccessLogs`, `Send` THREE messages with entry counts {1, 3, 2} (the second + third batched), `CloseAndRecv`; then assert:
  - `Count() == 6` (the flat entry total, UNCHANGED behavior).
  - `MessageCount() == 3`.
  - `BatchSizes()` deep-equals `[]int{1, 3, 2}` (arrival order).
  - `MaxBatchSize() == 3`.
  - After `Reset()`: `Count() == 0` AND `MessageCount() == 0` AND `MaxBatchSize() == 0` AND `BatchSizes()` is empty (the Reset clears BOTH).
  - `MaxBatchSize()` on a fresh server (no messages) == 0 (no panic on empty).
  Keep/extend the existing `-race` concurrency test so the new field is covered under `-race`.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./test/helpers/accessloggrpc/ -count=1`
Expected: FAIL (`BatchSizes`/`MaxBatchSize`/`MessageCount` undefined).

- [ ] **Step 3: Implement** in `accessloggrpc.go`:
- Add the field to `Server` (next to `entries`): `batchSizes []int`.
- In `StreamAccessLogs`, inside the existing `if httpLogs := msg.GetHttpLogs(); httpLogs != nil { s.mu.Lock(); … s.mu.Unlock() }` block, ALSO append the batch size (one lock, both appends):
```go
		if httpLogs := msg.GetHttpLogs(); httpLogs != nil {
			entries := httpLogs.GetLogEntry()
			s.mu.Lock()
			s.entries = append(s.entries, entries...)
			s.batchSizes = append(s.batchSizes, len(entries))
			s.mu.Unlock()
		}
```
- Add the three accessors (the `Entries()`/`Count()` mutex idiom):
```go
// BatchSizes returns a defensive snapshot copy of the per-message entry counts
// in arrival order (one int per received StreamAccessLogsMessage carrying
// http_logs). The differential driver reads this to prove the subject batched
// (MaxBatchSize >= 2). 44.2 (D-BUF-RECEIVER-BATCH-API).
func (s *Server) BatchSizes() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]int, len(s.batchSizes))
	copy(out, s.batchSizes)
	return out
}

// MaxBatchSize returns the largest per-message entry count seen (0 if no
// messages). The subject-side batching proof: a buffered sink yields >= 2;
// a one-entry-per-message fixed flush yields 1.
func (s *Server) MaxBatchSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	max := 0
	for _, n := range s.batchSizes {
		if n > max {
			max = n
		}
	}
	return max
}

// MessageCount returns the number of received messages carrying http_logs
// (== len(BatchSizes())). A batched run has MessageCount < total entries.
func (s *Server) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.batchSizes)
}
```
- Update `Reset()` to clear BOTH:
```go
func (s *Server) Reset() {
	s.mu.Lock()
	s.entries = nil
	s.batchSizes = nil
	s.mu.Unlock()
}
```
- Update the `Server` doc comment to mention the `batchSizes` accumulator + the three accessors.
(Avoid `max` as a variable name if the linter/Go-version flags the builtin shadow — rename to `m` if `golangci-lint` complains under the toolchain's Go version.)

- [ ] **Step 4: Run to verify they pass (with `-race`)**

Run: `go test ./test/helpers/accessloggrpc/ -race -count=1`
Expected: PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/accessloggrpc/ && golangci-lint run ./test/helpers/accessloggrpc/... && go build ./...
git add test/helpers/accessloggrpc/
git commit -m "phase 44.2 Task 6: accessloggrpc per-message batch exposure — BatchSizes/MaxBatchSize/MessageCount + Reset clears (D-BUF-RECEIVER-BATCH-API)"
```

---

## Task 7: The `0082-grpc-access-log-buffering` differential fixture (cross-side aggregated payload + the SUBJECT-side `maxBatchSize >= 2` proof; D-BUF-DIFFERENTIAL-DRIVE)

**Files:**
- Create: `test/fixtures/0082-grpc-access-log-buffering/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0082` driver package)

Copy the WHOLE `0081-grpc-access-log/` directory as the starting point, then make the buffering changes. The data-plane backend REUSES `HTTPFixedBody` (deterministic `response_body_bytes`). Same driver-owned-receiver lifecycle.

- [ ] **Step 1: Author the bootstraps** (copy `0081` envoy.yaml/envoy-go.yaml). The ONLY functional change vs `0081`: add the two buffer fields to the `common_config` block in BOTH YAMLs:
```yaml
                      common_config:
                        log_name: "0082"
                        grpc_service:
                          envoy_grpc:
                            cluster_name: c_als
                        buffer_size_bytes: 1048576      # 1 MiB — never hit by N small entries (the SIZE trigger stays dormant; the TIMER is the flush lever — D-BUF-DIFFERENTIAL-DRIVE / SPEC §8.1)
                        buffer_flush_interval: 1s       # the deterministic flush cadence; wide vs the concurrent burst so >= 2 entries coalesce
```
(Keep the listener/route/cluster shape identical to `0081`; only `log_name` + the two buffer fields differ.)

- [ ] **Step 2: Author `driver/driver.go`** (copy `0081`'s, rename `fixtureName = "0082-grpc-access-log-buffering"`, `refListenerPort = 10082`, bump `numRequests = 16`). The KEY changes from `0081`:
  - **Concurrent burst (coalescence guarantee):** replace `driveSide`'s sequential `for i := 0; i < numRequests; i++` loop with a CONCURRENT fan-out — fire all N via a `sync.WaitGroup` of goroutines against the same listener `addr`, collecting per-request status codes (guard the status collection with a mutex or per-index slice). The point: the N records queue into envoy-go's single process-global buffer within the 1s interval, so the next tick flushes ≥2 as one batch. (A `http.Client` with `MaxIdleConnsPerHost` raised, or per-goroutine clients, is fine; the existing `DisableKeepAlives` transport works with concurrent goroutines too.) Keep the per-request byte-stream output deterministic (sort the collected statuses or emit a fixed `status=200` line per request — the cross-side `CompareBytes` only needs the multiset to match; N×`status=200` is identical both sides).
  - **Snapshot batch sizes per side:** add `refBatchSizes []int` + `subjBatchSizes []int` driver fields; in `driveSide` return `srv.BatchSizes()` alongside `srv.Entries()`; store per side in `DriveReference`/`DriveSubject` (mirror `refEntries`/`subjEntries`).
  - Keep `ensureServer`/`allocateALSPort`/`pollCount`/`fireProbe`/`scrapeFlatStats`/the template helpers verbatim. `BackendKind()` stays `fixture.HTTPFixedBody`.

- [ ] **Step 3: Author the assertions (`AssertStats` + `expectations.yaml`).** Cross-side EXACT, aggregated over all N entries (the `0081` `assertEntries` verbatim — 7-field subset: method=GET, path=/health, authority=als.example, user_agent=als-probe/1, response_code=200, response_body_bytes=17, protocol_version=HTTP11) for BOTH sides + subject `logs_written == N`. THEN the **subject-side batching proof**:
```go
	// SUBJECT-side batching proof (D-BUF-DIFFERENTIAL-DRIVE / AMEND-BUF-3): the
	// buffered sink coalesced >= 2 entries into at least one StreamAccessLogsMessage.
	// This BITES against a regression to the 44.1 one-entry-per-message fixed flush
	// (which would make every batch size 1). SUBJECT-side ONLY — the reference's
	// per-worker batching is its own un-pinned business (cross-side batch counts
	// are infeasible).
	maxSubj := 0
	for _, n := range d.subjBatchSizes {
		if n > maxSubj {
			maxSubj = n
		}
	}
	if maxSubj < 2 {
		t.Fatalf("subject max batch size = %d, want >= 2 (buffering did not coalesce; subjBatchSizes=%v)", maxSubj, d.subjBatchSizes)
	}
```
Add a `FIXTURE_0082_DUMP` env-gated diagnostic dumping `refBatchSizes`/`subjBatchSizes` + entry counts (the `0081` `FIXTURE_0081_DUMP` idiom). UNasserted: `common_properties.*`, `identifier.node`, all reference-side framing/batch counts, and every subject-absent reference field.

- [ ] **Step 4: Author `README.md`** — copy `0081`'s, then update: the buffering purpose (the two buffer fields LIVE; the size-OR-timer flush); the D-BUF-DIFFERENTIAL-DRIVE drive (N=16 concurrent burst + 1MiB size [dormant] + 1s timer flush; the coalescence-determinism caveat from SPEC §8.1); the AMEND-BUF-3 "cross-side batch counts infeasible — aggregated payload + a subject-side `maxBatchSize >= 2` proof" note; the host-reachability table (`host.docker.internal` reference / `127.0.0.1` subject).

- [ ] **Step 5: Register + run the fixture isolated.**

Add the `0082` driver blank-import to `runner_test.go` (the `0081` import line is the template — `_ "github.com/esalaine/envoy-go/test/fixtures/0082-grpc-access-log-buffering/driver"`).
Run (the correct selector — `reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0082' -count=1`
Expected: PASS (both sides stream the same 7-field subset; `logs_written == N`; subject `maxBatchSize >= 2`).
Confirm fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **84** (the glob form — NOT `grep -cE '^[0-9]{4}-'`).

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0082-grpc-access-log-buffering/ test/differential/runner_test.go
git commit -m "phase 44.2 Task 7: 0082-grpc-access-log-buffering differential — cross-side aggregated 7-field payload + subject maxBatchSize >= 2 proof, concurrent burst + timer flush (fixtures 83 → 84)"
```

---

## Task 8: `0082` deliberate-break proofs + flake gate + the FULL-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0082` FAILS (proving the assertion is live), then `git restore` it:
  - (a) **The buffering proof bites** — revert the sink to the 44.1 fixed flush: in `grpcsink.go` `flush()`, make it Send one entry at a time (or force `bufferSizeBytes` effectively 0 by flushing after every append). ⇒ the subject `maxBatchSize >= 2` assertion must FAIL (collapses to 1). THIS is the load-bearing 44.2 break.
  - (b) **The batch-invariant counter** — break `s.logsWritten.Add(uint64(len(buf)))` to `s.logsWritten.Inc()` (count messages not entries). ⇒ the subject `logs_written == N` assertion must FAIL (would read messageCount < N).
  - (c) **The aggregated payload still bites** — break `buildHTTPAccessLogEntry` to drop `UserAgent` (or `protocolVersionEnum` to return HTTP2). ⇒ the cross-side `user_agent` (or `protocol_version`) assertion must FAIL.
  - (d) **Flush-on-close (AMEND-BUF-5)** — break the channel-closed path to `return` WITHOUT the final `flush()`. ⇒ with the timer wide (1s) and the converge poll, some entries may remain unflushed at Close ⇒ the entry-count / `logs_written == N` assertion must FAIL. (If this proves racy because the 1s timer flushes everything before Close anyway, document it as a coverage boundary like the 43.2b watcher-idle-close break, and rely on the Task 4 unit `flush-on-close` test as the live proof instead.)
  Run each: `go test ./test/differential/ -run 'TestDifferential/0082' -count=1` ⇒ expect FAIL, then restore ⇒ expect PASS. Record each break+restore in PROGRESS-44.2.md (the live-assertion proof).

- [ ] **Step 2: Flake gate** — 20 consecutive green runs (the coalescence-determinism gate — SPEC §8.1):
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0082' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. If `maxBatchSize >= 2` flakes, widen `buffer_flush_interval` to 2s and/or raise N before any other change (the coalescence margin). A transient `subject ready: EOF` is the startup-race flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run that single run; NOT a 0082 regression.

- [ ] **Step 3: FULL `internal/accesslog` package `-race`** (the sink writer goroutine + the NEW ticker goroutine — `reference_full_suite_race_after_background_mutator`):
```bash
go test ./internal/accesslog/ -race -count=1
```
Expected: PASS, no race.

- [ ] **Step 4: Commit the PROGRESS update** (break-proofs + flake + race recorded)
```bash
git add docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.2.md
git commit -m "phase 44.2 Task 8: 0082 deliberate-break proofs (incl revert-to-fixed-flush collapses maxBatchSize) + 20/20 flake + full-package -race"
```

---

## Task 9: Full 84-dir differential + six-gate + ADR-0256 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 44 leg 44.2 → done; family STAYS OPEN)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/44-grpc-access-log/PROGRESS-44.2.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                 # clean
go build ./...                               # ok
go test ./... -count=1                       # full unit + the 84-dir differential
go test ./internal/accesslog/ -race -count=1 # the background-mutator race gate
```
Expected: all green. (The full differential is the byte-stability regression anchor — no non-ALS fixture should move; `0081` stays green — the 44.1 fixed-flush is now the `buffer_size_bytes: 0` degenerate path, but `0081` sets no buffer fields so it gets the 16384/1s defaults; its N=8 sequential entries each flush on the 1s timer in singletons or small batches — its assertions are payload + `logs_written == 8`, batch-count-agnostic, so it stays green regardless. CONFIRM this explicitly.)

- [ ] **Step 2: ADR-0256 §Decision/§Consequences** — land them in DECISIONS.md beneath the §Context already drafted at SPEC §13 (ADR-0044). §Decision: lift `buffer_size_bytes`/`buffer_flush_interval` into `ALSConfig` (defaults 16384/1s; explicit-0 size ⇒ flush-every-entry; the 1s interval is a `NewTicker` panic-guard); the buffer machinery in `run()` (`buf`/`bufBytes`/`time.Ticker`/`flush()` size-OR-timer trigger + flush-on-close); `logs_written` batch-invariant (`Add(len(buf))`); the receiver batch exposure; the `0082` differential (aggregated payload + subject `maxBatchSize >= 2`). §Consequences: NO new stat/BackendKind/fuzzer/package/module; `additional_*_headers` (44.3) STAY parse-accept-inert; the per-worker-vs-process-global framing divergence is documented-faithful; the Observability family STAYS OPEN.

- [ ] **Step 3: BEHAVIOR_CONTRACT.md** — update the `### Access log — gRPC Access Log Service (ALS) streaming sink` block per SPEC §9: replace the "fixed simple flush (one entry per message)" wording with the buffer machinery (accumulate + size-OR-timer flush, defaults 16384/1s, `logs_written` ++ per entry batch-invariant, flush-on-Close before CloseAndRecv); MOVE `buffer_size_bytes`/`buffer_flush_interval` from the parse-accept-but-inert list into the supported set. The stat-surface block STAYS 1189 (AMEND-BUF-4).

- [ ] **Step 4: STATE.md + ROADMAP.md** — STATE active-phase → `phase 44.2 (grpc-access-log) IMPL done`; the count figures → stat **1189** / fixtures **84** / fuzzers **44** / BackendKind **38** / DECISIONS **ADR-0256**. ROADMAP row 44 leg 44.2 → **done** (per-leg, ADR-0106; the Observability family STAYS OPEN — 44.3 header-capture + OTLP/tracing/stats-sinks/tap remain). Set the next action → the **44.3 (header-capture) SPEC** (ADR-0257).

- [ ] **Step 5: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'` == **44** (UNCHANGED) and that the documented running total stays 44 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md / PROGRESS-44.2.md consistently.

- [ ] **Step 6: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 44.2 (grpc-access-log buffering) IMPL: ADR-0256 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 44 leg 44.2 done; Observability family STAYS OPEN); stat 1189 / fixtures 84 / fuzzers 44 / BackendKind 38"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 44.2 (grpc-access-log buffering) IMPL: the buffer_size_bytes/buffer_flush_interval flush triggers replacing the 44.1 fixed flush — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 44.2 IMPL squash and route the next session to the **44.3 (header-capture) SPEC**.
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1189** (H2 cluster; non-H2 **1185**) — UNCHANGED (NO new buffering stat, AMEND-BUF-4) / fixtures **84** (tail `0082-grpc-access-log-buffering`) / fuzzers **44** (UNCHANGED) / BackendKind **38** (UNCHANGED — the ALS receiver is driver-owned) / DECISIONS **ADR-0256**. ZERO new go.mod modules (`go mod tidy -diff` EMPTY). ZERO new `internal/` packages.
