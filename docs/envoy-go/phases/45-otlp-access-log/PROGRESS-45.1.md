# Phase 45.1 (otlp-access-log core) IMPL — PROGRESS

**SPEC:** `SPEC-45.1.md`
**PLAN:** `PLAN-45.1.md`
**Worktree branch:** `phase-45.1-otlp-access-log-impl`

45.1 is the **CORE leg** of the OTLP access-log row (the second Observability-family row,
after the row-44 gRPC ALS opener). It ships the core OTLP access-log sink: config parse
(LIFT `OpenTelemetryAccessLogConfig` from the ADR-0041 silent-ignore set) + the
`OTLPLogsClient` UNARY typed wrapper + the `OTLPAccessLogSink` (built-in mapping = a bare
`time_unix_nano` + the 4 Resource built-in labels; the reused 44.2 buffer machinery →
per-`Export` batch) + the `access_logs.open_telemetry_access_log.*` stats + the `0084`
receiver differential.

45.1 ships as **ONE leg** (the core sink); the operator engine (the configurable
`body`/`attributes` `KeyValueList` mapping) is the separately-chartered **45.2**. The
Observability **FAMILY STAYS OPEN** (tracing / stats-sinks / tap remain future rows).

---

## Task Checklist

- [x] T1  PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-OTLP-SPLIT-FINAL)
- [x] T2  `OTLPConfig` parse arm + shared `parseCommonGrpcAccessLogConfig` helper + strict-reject [TDD]
- [x] T3  `FuzzParseOpenTelemetryAccessLogConfig` (fuzzers 44→**45** ✓ verified) [TDD] — no-panic over the OTLP parse arm; 20s fuzz, no crashers
- [x] T4  `OTLPLogsClient` UNARY typed wrapper + go.mod promotion (transitive→direct) [TDD]
- [x] T5  built-in `Record`→`LogRecord` mapping + 4-label Resource + `Export` envelope [TDD]
- [x] T6  `OTLPAccessLogSink` (lazy-establish + reused 44.2 buffer machinery → per-`Export` batch) [TDD]
- [x] T7  `RegisterOTLPSinkCounters` (+2 → 1191 ✓ verified) [TDD] — registers `access_logs.open_telemetry_access_log.{logs_written,logs_dropped}`; static names (no IsValidName guard); `TestOTLPSinkCounters` asserts non-nil distinct counters + exact +2 surface delta
- [x] T8  boot wiring (dialer hoist + OTLP sink build + full node)
- [x] T9  `test/helpers/otlplogs` receiver accumulator (driver-owned)
- [x] T10 `0084-otlp-access-log` differential fixture (fixtures 85→86)
- [x] T11 `0084` deliberate breaks (4 live assertions ✓) + 20/20 flake gate ✓ + full-package -race ✓
- [x] T12 full 86-dir differential + six-gate + docs completion bundle (ADR-0258 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile)

---

## Baseline Counts (recorded at T1, verbatim)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
85

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | tail -1
test/fixtures/0083-grpc-access-log-headers

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
44

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38

$ grep -c 'go.opentelemetry.io/proto/otlp' go.mod
0
```

Baseline summary:
- stat surface: **1189** (H2 cluster; non-H2 **1185**)
- fixtures: **85** (incl letter-suffixed `0007a`/`0007b`; tail `0083-grpc-access-log-headers`)
- fuzzers: **44**
- BackendKind tail: **38** (`H2GoawayResponder`)
- go.mod `go.opentelemetry.io/proto/otlp` modules: **0** (NOT in go.mod yet — T4 introduces it
  `// indirect` then promotes it direct)
- DECISIONS tail: **ADR-0257** (next-free **ADR-0258** — appears in DECISIONS.md only as
  forward references; no ADR-0258 section yet)

All six baseline probes match the PLAN-anticipated values EXACTLY (85 / 44 / 38 / 0 /
ADR-0257). No baseline mismatch.

NOTE: the glob form `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` is authoritative
(= 85); a `grep -cE '^[0-9]{4}-'` form UNDERCOUNTS by the letter-suffixed dirs.

---

## Anticipated EXIT Counts

| Metric | Baseline | Exit | Delta | Note |
|---|---|---|---|---|
| stat surface | 1189 | 1191 | +2 | `access_logs.open_telemetry_access_log.{logs_written,logs_dropped}` |
| fixtures | 85 | 86 | +1 | `0084-otlp-access-log` |
| fuzzers | 44 | 45 | +1 | `FuzzParseOpenTelemetryAccessLogConfig` |
| BackendKind tail | 38 | 38 | 0 | UNCHANGED — the OTLP logs receiver is driver-owned (D-ALS-RECEIVER-WIRING analog) |
| DECISIONS tail | ADR-0257 | ADR-0258 | +1 | OTLP access-log core ADR |
| go.mod modules | — | +1 | +1 | `go.opentelemetry.io/proto/otlp` transitive→direct |

---

## D-OTLP-SPLIT-FINAL Re-check (ADR-0045 soft gate)

Estimated production LoC breakdown (~305 prod LoC):

| Component | Est. LoC |
|---|---|
| bootstrap (`OTLPConfig` parse arm + shared `parseCommonGrpcAccessLogConfig` helper + strict-reject) | ~80 |
| `OTLPLogsClient` UNARY typed wrapper | ~40 |
| `OTLPAccessLogSink` (lazy-establish + reused 44.2 buffer machinery → per-`Export` batch) | ~120 |
| built-in `Record`→`LogRecord` mapping + 4-label Resource + `Export` envelope | ~45 |
| stats (`RegisterOTLPSinkCounters`, +2) | ~6 |
| `main.go` wiring (dialer hoist + OTLP sink build) | ~20 |
| **Total** | **~305** |

~305 prod LoC — sits **at the ADR-0045 soft gate**. **45.1 ships as ONE leg** (the CORE
sink). The configurable operator engine (`body`/`attributes` `KeyValueList`) is the
separately-chartered **45.2**. The 45.1 IMPL six-gate flips ROADMAP leg 45.1 → `done`
(per-leg, ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability
**FAMILY STAYS OPEN**. (Bookkeeping re-check only; no code change.)

---

## T11 — `0084` deliberate-break liveness proofs + flake gate + race (VERIFICATION-ONLY)

Baseline: `go test ./test/differential/ -run 'TestDifferential/0084' -count=1` ⇒ `ok` (3.1s).

Every run used `-count=1` (defeats go-test result caching, which serves a stale PASS after
a production break) and the FULL subtest selector `-run 'TestDifferential/0084'` (a bare
`0084` matches ZERO subtests → vacuous green). Each break was reverted with
`git restore <file>` and `git diff --stat` confirmed clean before the next break.

### Break (a) — `time_unix_nano != 0` assertion

- **Broke:** `internal/accesslog/otlpmapping.go` `buildLogRecord` — commented out the
  `TimeUnixNano: uint64(rec.StartTime.UnixNano())` field so it stays 0.
- **Observed FAIL:** `runner_test.go:1282: subject record 0: time_unix_nano = 0, want non-zero (presence)`
  (the targeted `assertRecords` time-presence assertion).
- **Post-restore:** `ok` PASS.

### Break (b) — `log_name == "0084"` / 4-keys assertion

- **Broke:** `internal/accesslog/otlpmapping.go` `buildResource` — emitted `kv("log_name", "")`
  (empty log_name value).
- **Observed FAIL:** `runner_test.go:1282: subject ResourceLogs 0: log_name = "", want "0084"`
  (the targeted `assertResourceLabels` log_name-value assertion).
- **Post-restore:** `ok` PASS.

### Break (c) — subject `logs_written == 8` assertion

- **Broke:** `internal/accesslog/otlpsink.go` `run`/`flush` — commented out the
  `s.logsWritten.Add(uint64(len(buf)))` on the Export-success arm.
- **Observed FAIL:** `runner_test.go:1282: subject access_logs.open_telemetry_access_log.logs_written: got 0, want 8`
  (the targeted subject-side stat assertion).
- **Post-restore:** `ok` PASS.

### Break (d) — subject-side `len(records) == 8` (no sink built)

- **Broke:** `internal/bootstrap/bootstrap.go` `parseOpenTelemetryAccessLog` — inserted an
  early `return nil` before the `append` to `result.OTLPConfigs`, so no `OTLPConfig` is
  produced ⇒ the subject builds no OTLP sink ⇒ exports nothing.
- **Observed FAIL (SUBJECT side, as expected):**
  `runner_test.go:1208: subj drive: OTLP receiver: timed out waiting for 8 records (got 0)`
  — the subject never exports; the reference still exported 8 (only envoy-go's parse was
  broken), so the failure is correctly isolated to the subject's record-count path.
- **Post-restore:** `ok` PASS.

### Flake gate — 20/20 PASS

`for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0084' -count=1 ...` ⇒
runs 1–20 all PASS (no `subject ready: EOF` startup flake observed).

### Full-package race

`go test ./internal/accesslog/ -race -count=1` ⇒ `ok  github.com/esalaine/envoy-go/internal/accesslog  1.104s` — no race (the sink writer goroutine is a background mutator; the FULL package, not a `-run` subset).

### Tree hygiene

All 4 breaks reverted; `git diff --stat` after restore was clean each time. The only
committed change at T11 is this PROGRESS update (no production-code change).

---

## T12 — full 86-dir differential + six-gate + docs completion bundle

ALL 12 tasks complete. The completion commit is **docs-only** (`git add docs/`); the
six-gate (below) is a VERIFICATION run, not a code change.

### The six-gate (the house completion gate) — ALL GREEN

```
$ gofmt -l . | wc -l
0

$ golangci-lint run ./...                      # clean (exit 0)

$ go vet ./...                                 # clean (exit 0)

$ go build ./...                               # ok (exit 0)

$ go test ./... -count=1                       # ALL GREEN — full units + the 86-dir differential
ok  github.com/esalaine/envoy-go/test/differential   259.074s   (TestDifferential — 86 subtests)
... (all packages ok; no FAIL)

$ go test ./internal/accesslog/ -race -count=1
ok  github.com/esalaine/envoy-go/internal/accesslog   1.105s    (the background-mutator race gate)

$ go mod tidy -diff                            # EMPTY (the otlp transitive→direct promotion is the only go.mod delta)
```

No non-OTLP fixture moved — the full 86-dir differential is the byte-stability regression
anchor and held (the 85 prior fixtures byte-identical; `0084-otlp-access-log` cross-side
EXACT). No `subject ready: EOF` startup flake observed in the completion run.

### Final EXIT counts (each VERIFIED)

| Metric | Baseline | Exit | Note |
|---|---|---|---|
| stat surface | 1189 | **1191** | +2 `access_logs.open_telemetry_access_log.{logs_written,logs_dropped}` (process-global STATIC; non-H2 1185 → **1187**) |
| fixtures | 85 | **86** | tail `0084-otlp-access-log` (`ls -d test/fixtures/[0-9][0-9][0-9][0-9]* \| wc -l` == 86) |
| fuzzers | 44 | **45** | `FuzzParseOpenTelemetryAccessLogConfig` (`grep -rh '^func Fuzz' --include='*.go' . \| wc -l` == 45) |
| BackendKind tail | 38 | **38** | UNCHANGED — the OTLP logs receiver is driver-owned (`test/helpers/otlplogs`) |
| DECISIONS tail | ADR-0257 | **ADR-0258** | the OTLP access-log core ADR (§Decision/§Consequences landed at this IMPL; next-free ADR-0259) |
| go.mod modules | — | **+1** | `go.opentelemetry.io/proto/otlp` transitive→direct |

### Docs landed in the completion bundle

- `docs/envoy-go/DECISIONS.md` — ADR-0258 §Decision + §Consequences landed beneath the
  SPEC-anchored §Context (status PROPOSED → ACCEPTED per ADR-0044); tail ADR-0257 → ADR-0258.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new `### Access log — OpenTelemetry (OTLP)
  access-log sink` subsection + the silent-ignore-set note + the stat-surface changelog row
  (Phase 45.1 — 1189 → 1191).
- `docs/envoy-go/STATE.md` — active-phase header → `phase 45.1 (otlp-access-log) IMPL done`;
  recorded NEXT → the 45.2 (operator engine) SPEC; current counts in the narrative.
- `docs/envoy-go/ROADMAP.md` — row 45 IMPL-DONE annotation (row 45 STAYS `in-progress` —
  45.2 is the final leg; the Observability family STAYS OPEN).
- `docs/envoy-go/phases/45-otlp-access-log/PROGRESS-45.1.md` — this file (all 12 tasks
  complete + the final counts + the six-gate results).

Fuzzer-count reconcile (`reference_fuzzer_count_docs_drift`): the documented running total
advanced 44 → 45 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / DECISIONS.md (the
new ADR-0258) / this PROGRESS — verified `^func Fuzz` == 45.
