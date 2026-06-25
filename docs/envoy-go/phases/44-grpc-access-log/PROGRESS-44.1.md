# Phase 44.1 (grpc-access-log core) IMPL — PROGRESS

**SPEC:** `SPEC-44.1.md`
**PLAN:** `PLAN-44.1.md`
**Worktree branch:** `phase-44.1-grpc-access-log-impl`

---

## Task Checklist

- [x] T1  PROGRESS scaffold + baselines + ADR-0045 split re-check
- [x] T2  ALSConfig parse arm + Bootstrap.ALSConfigs + STRICT-REJECT [TDD]
- [x] T3  FuzzParseHttpGrpcAccessLogConfig [fuzz]
- [x] T4  ALSClient typed wrapper [TDD]
- [x] T5  Record→HTTPAccessLogEntry mapping + enum converters [TDD]
- [x] T6  GrpcAccessLogSink [TDD, full-package -race]
- [x] T7  2× access_logs.grpc_access_log.* stat registrations [TDD]
- [x] T8  main.go boot wiring
- [x] T9  test/helpers/accessloggrpc receiver [TDD]
- [x] T10 0081-grpc-access-log differential
- [x] T11 deliberate-break proofs + flake + race
- [x] T12 ADR-0255 + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile

---

## Baseline Counts (recorded at T1)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
82

$ grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'
43

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38
```

Baseline summary:
- stat surface: **1187** (H2 cluster; non-H2 **1183**)
- fixtures: **82**
- fuzzers: **43**
- BackendKind tail: **38** (`H2GoawayResponder`)
- DECISIONS tail: **ADR-0254** (next-free **ADR-0255**)

---

## Anticipated EXIT Counts

| Metric | Baseline | Exit | Delta | Note |
|---|---|---|---|---|
| stat surface | 1187 | 1189 | +2 | `access_logs.grpc_access_log.{logs_written,logs_dropped}` |
| fixtures | 82 | 83 | +1 | `0081-grpc-access-log` |
| fuzzers | 43 | 44 | +1 | `FuzzParseHttpGrpcAccessLogConfig` |
| BackendKind tail | 38 | 38 | 0 | ALS receiver is driver-owned (D-ALS-RECEIVER-WIRING), NOT a new BackendKind |
| DECISIONS tail | ADR-0254 | ADR-0255 | +1 | gRPC ALS core ADR |

---

## D-ALS-SPLIT-FINAL Re-check (ADR-0045 soft gate)

Estimated production LoC breakdown:

| Component | Est. LoC |
|---|---|
| bootstrap parse arm + ALSConfig + STRICT-REJECT | ~70 |
| ALSClient typed wrapper | ~45 |
| GrpcAccessLogSink (channel + Submit + writer goroutine + lazy/identifier/reconnect + Close) | ~140 |
| Record→HTTPAccessLogEntry mapping + 2 enum converters | ~75 |
| stats registration | ~12 |
| main wiring | ~22 |
| **Total** | **~365** |

~365 prod LoC — right at the ADR-0045 soft gate. **44.1 ships as ONE leg.** (Bookkeeping re-check only; no code change.)

---

## T11 — 0081 deliberate-break liveness proofs + flake gate + full-package -race

VERIFICATION ONLY — no production change survives. Each break: edit → run → confirm FAIL on
the expected assertion → `git restore` → re-run → confirm PASS. Selector always
`go test ./test/differential/ -run 'TestDifferential/0081' -count=1` (`-count=1` defeats
go-test PASS caching; the `TestDifferential/0081` prefix avoids the bare-`0081` vacuous-green
selector footgun).

Baseline (pre-break): `ok ... test/differential 2.380s`.

### Break (a) — protocol_version
- **Edit:** `internal/accesslog/mapping.go` `protocolVersionEnum`, `case "HTTP/1.1":` returns
  `HTTPAccessLogEntry_HTTP2` (instead of `_HTTP11`).
- **Assertion that failed:** `protocol_version == HTTP11`.
- **Verbatim FAIL:**
  ```
  --- FAIL: TestDifferential/0081-grpc-access-log (3.43s)
      runner_test.go:1279: subject entry 0: protocol_version = HTTP2, want HTTP11
  ```
- **Restore-PASS:** `git restore internal/accesslog/mapping.go` → `ok ... 2.220s`.

### Break (b) — request.user_agent
- **Edit:** `internal/accesslog/mapping.go` `buildHTTPAccessLogEntry`, `UserAgent: ""` (instead
  of `rec.UserAgent`).
- **Assertion that failed:** `request.user_agent == "als-probe/1"`.
- **Verbatim FAIL:**
  ```
  --- FAIL: TestDifferential/0081-grpc-access-log (3.14s)
      runner_test.go:1279: subject entry 0: request.user_agent = "", want "als-probe/1"
  ```
- **Restore-PASS:** `git restore internal/accesslog/mapping.go` → `ok ... 3.368s`.

### Break (c) — logs_written stat (per-entry assertions stay live)
- **Edit:** `internal/accesslog/grpcsink.go` `run()`, comment out `s.logsWritten.Inc()`
  (entries still stream, so per-entry assertions still PASS — ONLY the stat fails).
- **Assertion that failed:** `access_logs.grpc_access_log.logs_written == 8`.
- **Verbatim FAIL:**
  ```
  --- FAIL: TestDifferential/0081-grpc-access-log (3.25s)
      runner_test.go:1279: subject access_logs.grpc_access_log.logs_written: got 0, want 8
  ```
  (Confirmed: the per-entry field assertions did NOT fire — only the stat assertion bit, exactly as designed.)
- **Restore-PASS:** `git restore internal/accesslog/grpcsink.go` → `ok ... 2.686s`.

### Break (d) — request.path mapping
- **Edit:** `internal/accesslog/mapping.go` `buildHTTPAccessLogEntry`, `Path: rec.Path + "X"`.
- **Assertion that failed:** `request.path == "/health"`.
- **Verbatim FAIL:**
  ```
  --- FAIL: TestDifferential/0081-grpc-access-log (3.51s)
      runner_test.go:1279: subject entry 0: request.path = "/healthX", want "/health"
  ```
- **Restore-PASS:** `git restore internal/accesslog/mapping.go` → `ok ... 2.702s`; `git status` clean.

All four assertions are LIVE. After the four restores `git status` showed no production diff.

### Flake gate — 20 consecutive runs
`for i in 1..20: go test ./test/differential/ -run 'TestDifferential/0081' -count=1` →
**20/20 PASS**, zero flakes (no transient `subject ready: EOF` startup race observed).

### Full-package -race
`go test ./internal/accesslog/ -race -count=1` (FULL package, not a -run subset; the sink
writer goroutine is a background mutator) → `ok ... internal/accesslog 1.019s`, no race.

---

## T12 — full six-gate + the completion documentation bundle

**The SIX-GATE (all GREEN, captured 2026-06-25):**
- `gofmt -l .` → **0** files.
- `golangci-lint run ./...` → clean (exit 0).
- `go vet ./...` → clean (exit 0).
- `go build ./...` → ok (exit 0).
- `go test ./... -count=1` → **PASS**, exit 0; `ok ... test/differential 252.451s` (the full **83-dir differential** — the byte-stability anchor), **0 FAIL** across all packages, no `subject ready: EOF` startup flake. NO non-ALS fixture regressed.
- `go test ./internal/accesslog/ -race -count=1` → `ok ... internal/accesslog 1.020s`, no race (the writer-goroutine background-mutator gate).

**Fuzzer-count reconcile (`reference_fuzzer_count_docs_drift`):**
`grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'` → **44** (43 → 44; `FuzzParseHttpGrpcAccessLogConfig`). The documented running total advanced 43 → 44 across STATE / BEHAVIOR_CONTRACT / ROADMAP / DECISIONS / this PROGRESS.

**The completion documentation bundle (docs-only — all production landed in T1–T11):**
- `DECISIONS.md` — ADR-0255 §Context (promoted from SPEC §13 DRAFT) + §Decision + §Consequences landed IN-PLACE per ADR-0044; tail ADR-0254 → **ADR-0255** (next-free **ADR-0256**).
- `BEHAVIOR_CONTRACT.md` — the `### Access log — gRPC Access Log Service (ALS) streaming sink` subsection + the stat-surface phase-44.1 entry (1187 → **1189**; non-H2 1183 → **1185**).
- `STATE.md` — the new `active-phase` IMPL-done entry prepended; the PLAN-done entry demoted to `prior active-phase`.
- `ROADMAP.md` — row 44 leg 44.1 → `done` (row STAYS `in-progress`; the Observability family STAYS OPEN); Next → the 44.2 (buffering) SPEC.
- `PROGRESS-44.1.md` — this file; all T1–T12 boxes checked.

**As-built EXIT counts (consistent across all 5 docs):** stat surface **1189** (H2; non-H2 **1185**) / fixtures **83** / fuzzers **44** / BackendKind tail **38** (UNCHANGED — driver-owned receiver) / DECISIONS tail **ADR-0255** (next-free **ADR-0256**). The completion commit touches ONLY `docs/`.
