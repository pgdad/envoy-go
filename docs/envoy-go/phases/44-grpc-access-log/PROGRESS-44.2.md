# Phase 44.2 (grpc-access-log buffering) IMPL — PROGRESS

**SPEC:** `SPEC-44.2.md`
**PLAN:** `PLAN-44.2.md`
**Worktree branch:** `phase-44.2-buffering-plan`

---

## Task Checklist

- [x] T1  PROGRESS scaffold + baselines + the final ADR-0045 re-check (D-BUF-SPLIT-FINAL)
- [x] T2  lift buffer_size_bytes/buffer_flush_interval into ALSConfig with 16384/1s defaults [TDD]
- [x] T3  buffer-bearing seed corpus for the EXISTING FuzzParseHttpGrpcAccessLogConfig (NO new func Fuzz) [fuzz]
- [x] T4  the buffer machinery in GrpcAccessLogSink.run() (buf/bufBytes/time.Ticker/flush() size-OR-timer + flush-on-close; logsWritten.Add(len(buf)) batch-invariant; signature extension) [TDD, full-package -race]
- [x] T5  the main.go pass-through
- [x] T6  the accessloggrpc per-message batch exposure (BatchSizes/MaxBatchSize/MessageCount + Reset clears) [TDD]
- [x] T7  the 0082-grpc-access-log-buffering differential (cross-side aggregated 7-field payload + subject maxBatchSize >= 2 proof; large size [dormant] + 1s timer + N=16 CONCURRENT burst)
- [x] T8  0082 deliberate breaks (-count=1, incl revert-to-fixed-flush collapsing maxBatchSize to 1) + 20/20 flake + full-package -race
- [x] T9  full 84-dir differential + six-gate + ADR-0256 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 44 leg 44.2 → done; family STAYS OPEN)

---

## Baseline Counts (recorded at T1)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
83

$ grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'
44

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38
```

Baseline summary:
- stat surface: **1189** (H2 cluster; non-H2 **1185**)
- fixtures: **83**
- fuzzers: **44**
- BackendKind tail: **38** (`H2GoawayResponder`)
- DECISIONS tail: **ADR-0255** (next-free **ADR-0256**)

---

## Anticipated EXIT Counts

| Metric | Baseline | Exit | Delta | Note |
|---|---|---|---|---|
| stat surface | 1189 | 1189 | 0 | UNCHANGED — NO new buffering stat (AMEND-BUF-4) |
| fixtures | 83 | 84 | +1 | `0082-grpc-access-log-buffering` |
| fuzzers | 44 | 44 | 0 | UNCHANGED — existing FuzzParseHttpGrpcAccessLogConfig covers the buffer fields (T3 adds seed corpus only) |
| BackendKind tail | 38 | 38 | 0 | ALS receiver is driver-owned (D-ALS-RECEIVER-WIRING), NOT a new BackendKind |
| DECISIONS tail | ADR-0255 | ADR-0256 | +1 | gRPC ALS buffering ADR |

---

## Task 3 — Buffer-bearing seed corpus (D-BUF-FUZZER-CORPUS)

6 new `f.Add(...)` seeds added to `FuzzParseHttpGrpcAccessLogConfig` in
`internal/bootstrap/alsconfig_fuzz_test.go` (raw proto wire bytes; no new
imports; no new `func Fuzz`):

| Seed | Description |
|---|---|
| `\x0a\x04\x22\x02\x08\x01` | `common_config.buffer_size_bytes = 1` (tiny) |
| `\x0a\x02\x22\x00` | `common_config.buffer_size_bytes = 0` (explicit zero; wrapper present, value default-not-emitted; flush-every-entry path) |
| `\x0a\x08\x22\x06\x08\xff\xff\xff\xff\x0f` | `common_config.buffer_size_bytes = 0xffffffff` (uint32 max) |
| `\x0a\x04\x1a\x02\x10\x01` | `common_config.buffer_flush_interval = 1ns` (sub-millisecond) |
| `\x0a\x06\x1a\x04\x08\x80\xa3\x05` | `common_config.buffer_flush_interval = 86400s` (24 h, very long) |
| `\x0a\x0a\x1a\x02\x08\x01\x22\x04\x08\x80\x80\x01` | both fields: `buffer_size_bytes=16384`, `buffer_flush_interval=1s` (explicit defaults) |

Absent `buffer_flush_interval` (→ 1s coercion) is covered by seeds 1–3 and
the pre-existing seeds (none of which set field 3).

Fuzzer runs:
- `-run 'FuzzParseHttpGrpcAccessLogConfig' -count=1`: **PASS** (0.003 s)
- `-fuzz 'FuzzParseHttpGrpcAccessLogConfig' -fuzztime 20s`: **PASS**, no crashers
  (1.28 M execs, 34 new interesting inputs found beyond baseline 230)

`^func Fuzz` count: **44** (UNCHANGED — no new fuzzer added).

Gates: `gofmt -l` printed nothing; `golangci-lint run` clean; `go build ./...` clean.

---

## D-BUF-SPLIT-FINAL Re-check (ADR-0045 soft gate)

Estimated production LoC breakdown:

| Component | Est. LoC |
|---|---|
| ALSConfig two fields + parse-arm reads/defaults | ~18 |
| buffer machinery in grpcsink.go (buf/bufBytes/Ticker/flush/close) | ~90 |
| accessloggrpc receiver batchSizes field + 3 accessors + Reset clear | ~22 |
| main.go pass-through | ~2 |
| **Total** | **~132** |

~132 prod LoC — well under the ADR-0045 soft gate. **44.2 ships as ONE leg.** (Bookkeeping re-check only; no code change.)

---

## Task 8 — `0082` deliberate-break proofs + flake gate + full-package `-race`

VERIFICATION-ONLY: no permanent production change. Each break broke ONE
production line, was confirmed to make `0082` FAIL (proving the assertion is
LIVE), then `git restore`d and confirmed to PASS again. Every run used
`-count=1` (reference_differential_break_protocol_count1 — defeat go-test
caching). Baseline `0082` was green before any break.

### Step 1 — deliberate-break live-assertion proofs

**(a) Buffering coalescence proof — `maxBatchSize >= 2`** *(the load-bearing 44.2 break)*
- **Broke:** `internal/accesslog/grpcsink.go` `run()` — replaced the size-trigger
  guard `if bufBytes >= s.bufferSizeBytes { flush() }` with `if true { flush() }`
  (force a flush after EVERY append → batches collapse to size 1; reverts the
  sink to the 44.1 fixed one-entry-per-message flush).
- **FAIL observed:** `runner_test.go:1280: subject max batch size = 1, want >= 2`
  `(buffering did not coalesce; subjBatchSizes=[1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1])`
- **Restore:** `git restore internal/accesslog/grpcsink.go` → `git diff --exit-code`
  clean → `0082` **PASS**. ⇒ assertion LIVE.

**(b) Batch-invariant entry counter — `logs_written == 16`**
- **Broke:** `internal/accesslog/grpcsink.go` `flush()` —
  `s.logsWritten.Add(uint64(len(buf)))` → `s.logsWritten.Inc()` (count messages
  not entries).
- **FAIL observed:** `runner_test.go:1280: subject access_logs.grpc_access_log.logs_written: got 1, want 16`
  (all 16 entries coalesced into one message ⇒ one Inc).
- **Restore:** `git restore` → `git diff --exit-code` clean → `0082` **PASS**.
  ⇒ assertion LIVE.

**(c) Aggregated cross-side payload — `user_agent`**
- **Broke:** `internal/accesslog/mapping.go` `buildHTTPAccessLogEntry` — dropped
  the `UserAgent: rec.UserAgent` field from the `HTTPRequestProperties` literal.
- **FAIL observed:** `runner_test.go:1280: subject entry 0: request.user_agent = "", want "als-probe/1"`
- **Restore:** `git restore internal/accesslog/mapping.go` → `git diff --exit-code`
  clean → `0082` **PASS**. ⇒ assertion LIVE.

**(d) Flush-on-close (AMEND-BUF-5) — DOCUMENTED COVERAGE BOUNDARY (did NOT bite)**
- **Broke:** `internal/accesslog/grpcsink.go` `run()` channel-closed path —
  removed the final `flush()` before `CloseAndRecv` (the `if !ok { flush(); ... }`
  drain).
- **Observed:** `0082` **PASSED** on 3/3 consecutive `-count=1` runs — the break
  did NOT reliably bite. Reason (as the plan anticipated): the 1s
  `buffer_flush_interval` timer flushes the whole buffer well before the driver's
  converge-poll completes and the sink is Closed, so by the time the channel
  closes the buffer is already empty and the missing close-flush drains nothing.
  Forcing this break would require a fragile timing arrangement; per the plan
  (cf. the 43.2b watcher-idle-close coverage boundary) it is recorded as a
  COVERAGE BOUNDARY rather than forced.
- **Live proof instead:** the Task 4 unit test
  `internal/accesslog/grpcsink_test.go::TestGrpcSink_FlushOnClose` directly
  exercises the close-flush path (no timer flush in between) and **PASSES** —
  that is the live assertion for AMEND-BUF-5.
- **Restore:** `git restore internal/accesslog/grpcsink.go` → `git diff --exit-code`
  clean.

After every restore: `git status` clean, branch confirmed `phase-44.2-buffering-plan`,
production file byte-identical to committed (`git diff --exit-code` returned clean).

### Step 2 — flake gate (coalescence-determinism, SPEC §8.1)

`for i in 1..20: go test ./test/differential/ -run 'TestDifferential/0082' -count=1`
⇒ **20/20 PASS**. No flakes; no `subject ready: EOF` startup-race reruns needed
(subjBatchSizes consistently coalesced with a wide margin, as Task 7 observed).

### Step 3 — full-package `-race`

`go test ./internal/accesslog/ -race -count=1` ⇒ **PASS** (1.077 s), no data race
(the sink writer goroutine + the new flush-interval ticker goroutine are clean
under the race detector — reference_full_suite_race_after_background_mutator).

### Tree state

`git diff --stat` after all breaks reverted: ONLY this PROGRESS doc modified; all
production files (`internal/accesslog/grpcsink.go`, `internal/accesslog/mapping.go`)
clean. Final `0082` run GREEN. Worktree stayed on branch
`phase-44.2-buffering-plan`; the main checkout was never touched.

---

## Task 9 — full 84-dir differential + six-gate + docs completion

### The six-gate (all GREEN)

| Gate | Result |
|---|---|
| `gofmt -l .` | **0** files (clean) |
| `golangci-lint run ./...` | clean |
| `go vet ./...` | clean |
| `go build ./...` | ok |
| `go test ./... -count=1` | **PASS** — `ok ... test/differential 255.491s` (the full **84-dir** differential) + all unit packages green |
| `go test ./internal/accesslog/ -race -count=1` | **PASS** (1.075 s) — writer-goroutine + ticker, race-clean |

The full `go test ./...` exited 0 with NO `FAIL`/`panic`/`EOF` anywhere — so all 84
differential subtests passed, **including `0081-grpc-access-log` (STAYS green — 44.1
fixed-flush is now the `buffer_size_bytes: 0` degenerate path, but `0081` sets no
buffer fields ⇒ inherits the 16384/1s defaults; its N=8 sequential entries flush on
the 1s timer in singletons/small batches; its assertions are payload + `logs_written
== 8`, batch-count-agnostic, so they hold regardless) and `0082-grpc-access-log-
buffering`**. No non-ALS fixture moved (the byte-stability regression anchor held); no
`subject ready: EOF` startup-race reruns were needed.

### Count confirmations

- `stats_test.go` no-delta: `TestGrpcSinkCounters` **PASS** — stat surface **1189** UNCHANGED (non-H2 **1185**).
- `go mod tidy -diff`: **EMPTY** (no go.mod/go.sum movement; ZERO new module).
- fixtures: **84** (`ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` = 84; tail `0082-grpc-access-log-buffering`).
- fuzzers: **44** (`grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'` = 44 — UNCHANGED; `reference_fuzzer_count_docs_drift` reconciled: 44 consistent across STATE / BEHAVIOR_CONTRACT / ROADMAP / DECISIONS / PROGRESS-44.2).
- BackendKind tail: **38** UNCHANGED (driver-owned `accessloggrpc` receiver).
- DECISIONS tail: **ADR-0255 → ADR-0256** (next-free **ADR-0257**).

### Documentation landed

- **DECISIONS.md** — ADR-0256 §Decision + §Consequences landed beneath the §Context promoted from the SPEC-44.2 §13 draft (status PROPOSED → ACCEPTED per ADR-0044).
- **BEHAVIOR_CONTRACT.md** — the ALS streaming-sink block gained "The buffer machinery (44.2)"; the buffer fields MOVED out of the parse-accept-but-inert set into supported; `logs_written` noted batch-invariant; the "Phase 44.2 — 1189 → 1189 (+0)" stat-surface entry added.
- **STATE.md** — new `phase 44.2 (grpc-access-log) IMPL done` active-phase bullet prepended (prior PLAN-done bullet demoted); counts stat 1189 / fixtures 84 / fuzzers 44 / BackendKind 38 / DECISIONS ADR-0256; recorded NEXT → the 44.3 (header-capture) SPEC (ADR-0257).
- **ROADMAP.md** — row 44 leg 44.2 → `done` (per-leg, ADR-0106; row 44 STAYS `in-progress` — 44.3 remains; the Observability family STAYS OPEN).

### Tree state

Worktree stayed on branch `phase-44.2-buffering-plan`; the main checkout was never
touched (the completion bundle is committed LOCALLY only — NEVER pushed).
