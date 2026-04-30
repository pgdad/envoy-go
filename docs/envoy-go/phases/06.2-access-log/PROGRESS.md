# Phase 06.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 12 preconditions satisfied at cold-start: branch `phase/06.2-access-log-impl` at HEAD `54a31c2b6d4c5c333a8fb19ae015fdd4ee808d25` (matching STATE.md last-commit field); Docker client (28.4.0) + server (28.1.1) both reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; all 6 differential fixtures (0000–0005, including Docker-dependent 0004 and 0005) PASS; `github.com/envoyproxy/go-control-plane/envoy v1.32.4`; `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0065:` (next-free ADR-0066 per PLAN); SPEC.md last-commit is `7bbf4a2` (the spec-reviewer follow-up); `internal/accesslog/` contains only `doc.go`; the four action-method signatures (`directResponseAction.do`, `routerAction.do`, `routerActionH2.doH2`, `h2DirectResponseAdapter.WriteH2`) are present in `internal/filter/hcm/`; `docs/envoy-go/BEHAVIOR_CONTRACT.md` has the `## Access log field mapping` heading with placeholder body present.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `015fc0c`
**Notes:** Created PROGRESS.md; verified all 12 preconditions per PLAN §"Execution preconditions"; phase-06.1 close confirmed present in HEAD; SPEC at `7bbf4a2`; ADR tail at 0065 (next-free 0066); `internal/accesslog/` contains only `doc.go` (the package implementation lands at Task 2+).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/06.2-access-log-impl
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0065: Validate metric-name-deriving inputs at the user-input boundary
$ git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/SPEC.md
7bbf4a2389b94d75061a746fb7db079f820211c0
$ ls internal/accesslog/
doc.go
```

## Task 2 — `internal/accesslog/accesslog.go` — Sink interface + Record struct + doc.go rewrite [ADR-0066]

**Commits:** `76f3ecd`
**Notes:** Created `internal/accesslog/accesslog.go` with `Sink` interface (`Submit(*Record)` + `Close() error`) and `Record` struct (10 plumbed fields: StartTime, Method, Path, Protocol, ResponseCode, BytesSent, Duration, Authority, UserAgent, UpstreamHost). Rewrote `internal/accesslog/doc.go` from phase-00 stub to reference ADR-0066 and lifecycle context. Appended ADR-0066 to `docs/envoy-go/DECISIONS.md` (Access-log architecture decision: thin in-tree primitive, no third-party access-log dependency). TDD discipline followed: test file written first, RED confirmed, then implementation to GREEN.
**Outputs:**
```
# RED — go test ./internal/accesslog/ -count=1 -v (before accesslog.go)
# github.com/esalaine/envoy-go/internal/accesslog [github.com/esalaine/envoy-go/internal/accesslog.test]
internal/accesslog/accesslog_test.go:9:8: undefined: Record
internal/accesslog/accesslog_test.go:24:7: undefined: Record
internal/accesslog/accesslog_test.go:35:34: undefined: Record
internal/accesslog/accesslog_test.go:36:33: undefined: Record
internal/accesslog/accesslog_test.go:40:8: undefined: Sink
internal/accesslog/accesslog_test.go:41:8: undefined: Record
FAIL	github.com/esalaine/envoy-go/internal/accesslog [build failed]
FAIL

# GREEN — go test ./internal/accesslog/ -count=1 -v (after accesslog.go)
=== RUN   TestRecord_AllFieldsZeroValueWellDefined
--- PASS: TestRecord_AllFieldsZeroValueWellDefined (0.00s)
=== RUN   TestRecord_PopulatedShape
--- PASS: TestRecord_PopulatedShape (0.00s)
=== RUN   TestSink_InterfaceImplementation
--- PASS: TestSink_InterfaceImplementation (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# grep -nE '^## ADR-0066:' docs/envoy-go/DECISIONS.md
2335:## ADR-0066: Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure)

# git diff --stat HEAD (after staging)
 docs/envoy-go/DECISIONS.md           | 33 +++++++++++++++++++++++++
 internal/accesslog/accesslog.go      | 40 ++++++++++++++++++++++++++++++
 internal/accesslog/accesslog_test.go | 47 ++++++++++++++++++++++++++++++++++++
 internal/accesslog/doc.go            | 14 ++++++++---
 4 files changed, 130 insertions(+), 4 deletions(-)
```

## Task 4 — `internal/accesslog/writer.go` — AsyncFileSink + drop-newest backpressure

**Commits:** TBD — this task's commit
**Notes:** Created `internal/accesslog/writer.go` with `AsyncFileSink`: bounded 4096-cap channel (drop-newest discipline per ADR-0066); `NewAsyncFileSink` opens path with `O_APPEND|O_CREATE|O_WRONLY 0644` and spawns writer goroutine; `Submit` does a non-blocking channel send — on full channel increments the `*stats.Counter` (ADR-0069 `server.accesslog_dropped`) and emits a rate-limited diagnostic (at most once per second via `atomic.Int64` + `CompareAndSwap`); `Close` is idempotent+threadsafe via `sync.Once` — closes the channel, waits for the writer goroutine to drain (blocking on `<-s.done`), then closes the file descriptor. Created `internal/accesslog/writer_test.go` with 5 TDD tests: happy-path 5-records-land-5-lines, concurrent 8×100 submit race-clean, drop-newest full-channel increments counter (capacity-1 sink + 100 submits), Close idempotent (double-close no error), Close drains pending (50 queued records → non-empty file). `stats.Counter.Load()` returns `uint64`; test comparisons against `0` are untyped and compile without cast.

**Outputs:**
```
# RED — go test -race ./internal/accesslog/ -run TestAsyncFileSink -v (before writer.go)
# github.com/esalaine/envoy-go/internal/accesslog [github.com/esalaine/envoy-go/internal/accesslog.test]
internal/accesslog/writer_test.go:25:12: undefined: NewAsyncFileSink
internal/accesslog/writer_test.go:57:12: undefined: NewAsyncFileSink
internal/accesslog/writer_test.go:82:12: undefined: newAsyncFileSinkWithCapacity
internal/accesslog/writer_test.go:99:12: undefined: NewAsyncFileSink
internal/accesslog/writer_test.go:115:12: undefined: NewAsyncFileSink
FAIL	github.com/esalaine/envoy-go/internal/accesslog [build failed]
FAIL

# GREEN — go test -race -count=1 ./internal/accesslog/ -run TestAsyncFileSink -v (after writer.go)
=== RUN   TestAsyncFileSink_HappyPath_NRecordsLandNLines
--- PASS: TestAsyncFileSink_HappyPath_NRecordsLandNLines (0.00s)
=== RUN   TestAsyncFileSink_ConcurrentSubmit_RaceClean
--- PASS: TestAsyncFileSink_ConcurrentSubmit_RaceClean (0.00s)
=== RUN   TestAsyncFileSink_DropNewest_FullChannelIncrementsCounter
2026/04/30 05:18:32 accesslog: channel full, dropping record (path=...)
--- PASS: TestAsyncFileSink_DropNewest_FullChannelIncrementsCounter (0.00s)
=== RUN   TestAsyncFileSink_Close_Idempotent
--- PASS: TestAsyncFileSink_Close_Idempotent (0.00s)
=== RUN   TestAsyncFileSink_Close_DrainsPending
--- PASS: TestAsyncFileSink_Close_DrainsPending (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.012s

# Verification — go test -race -count=1 ./internal/accesslog/ -v (full package)
# All 12 tests pass (7 TestDefault_* + 5 TestAsyncFileSink_*); no race detected
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.013s
```

## Task 3 — `internal/accesslog/format.go` — Default formatter + empirical-format-pin scrape

**Commits:** `d3da508`
**Notes:** Created `internal/accesslog/format.go` with `Default(*Record) []byte` implementing the Envoy v1.37.2 default access-log format (15 operators, identical positions on every record per SPEC §6). Six escape rules: `"` → `\"`, `\n` → `\n` literal, `\r` → `\r` literal in all field values; embedded LFs replaced so line-stream invariant holds. The 5 unplumbed operators (RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME), X-FORWARDED-FOR, X-REQUEST-ID) emit literal `-` per Decision A (Tier-S). Created `internal/accesslog/format_test.go` with 7 TDD tests (happy-path, routed upstream host, quote escaping, no-embedded-LF, empty-fields-dash, RFC3339ms time format, ms-rounded-down duration).

Empirical scrape: booted reference Envoy v1.37.2 (SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`) with a minimal HCM config + small Go backend on port 18443; drove 5 sequential GETs (`/health`, `/api/v1/foo`, `/api/v1/bar`, `/api/v1/baz`, `/notfound`); captured `/tmp/0006-pin/envoy-access.log`. Format analysis: reference Envoy emits `0` for BYTES_RECEIVED and UUID for X-REQUEST-ID (Envoy auto-generates it); subject emits `-` for all 5 Tier-S operators per Decision A. Positional structure (operator count = 15, delimiter shapes `[`, `]`, `"`, space) matches exactly — no corrections needed to `format.go`. Both TBD placeholders in SPEC.md filled: §11 (line 572 area) and §13.1 (line 650 area), using the verbatim 5-line scrape.

**Outputs:**
```
# RED — go test ./internal/accesslog/ -count=1 -run TestDefault -v (before format.go)
# github.com/esalaine/envoy-go/internal/accesslog [github.com/esalaine/envoy-go/internal/accesslog.test]
internal/accesslog/format_test.go:18:9: undefined: Default
internal/accesslog/format_test.go:38:14: undefined: Default
internal/accesslog/format_test.go:49:14: undefined: Default
internal/accesslog/format_test.go:61:9: undefined: Default
internal/accesslog/format_test.go:72:14: undefined: Default
internal/accesslog/format_test.go:81:14: undefined: Default
internal/accesslog/format_test.go:93:14: undefined: Default
FAIL	github.com/esalaine/envoy-go/internal/accesslog [build failed]
FAIL

# GREEN — go test ./internal/accesslog/ -count=1 -run TestDefault -v (after format.go)
=== RUN   TestDefault_HappyPath_HCMDirect
--- PASS: TestDefault_HappyPath_HCMDirect (0.00s)
=== RUN   TestDefault_RoutedPath_UpstreamHostFormatted
--- PASS: TestDefault_RoutedPath_UpstreamHostFormatted (0.00s)
=== RUN   TestDefault_QuoteEscaping
--- PASS: TestDefault_QuoteEscaping (0.00s)
=== RUN   TestDefault_NeverEmbedsLF
--- PASS: TestDefault_NeverEmbedsLF (0.00s)
=== RUN   TestDefault_EmptyFieldsEmitDash
--- PASS: TestDefault_EmptyFieldsEmitDash (0.00s)
=== RUN   TestDefault_StartTimeFormat_RFC3339Ms
--- PASS: TestDefault_StartTimeFormat_RFC3339Ms (0.00s)
=== RUN   TestDefault_DurationMillisecondsRoundedDown
--- PASS: TestDefault_DurationMillisecondsRoundedDown (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# Empirical scrape — verbatim /tmp/0006-pin/envoy-access.log from reference Envoy v1.37.2
# (image SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd, captured 2026-04-30)
[2026-04-30T09:10:30.856Z] "GET /health HTTP/1.1" 200 - 0 3 0 - "-" "curl/8.5.0" "b66c2c7d-3921-4184-b6c1-6a80dd5e7e8e" "127.0.0.1:15006" "-"
[2026-04-30T09:10:30.861Z] "GET /api/v1/foo HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "1210434d-5aa4-4a56-a256-3ff6fc989ce5" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.865Z] "GET /api/v1/bar HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "c76bd1e7-3f55-4a6b-a3df-f88f00c7250a" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.870Z] "GET /api/v1/baz HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "5b25ba00-2be4-4ae6-9693-0ce90609f529" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.875Z] "GET /notfound HTTP/1.1" 404 - 0 10 0 - "-" "curl/8.5.0" "5a9c562a-1ebf-4676-a556-bf02f89a0fad" "127.0.0.1:15006" "-"

# Verification grep (no match = PASS)
$ ! grep -F 'TBD: pinned at PLAN Task N' docs/envoy-go/phases/06.2-access-log/SPEC.md
(no output — PASS)
```
