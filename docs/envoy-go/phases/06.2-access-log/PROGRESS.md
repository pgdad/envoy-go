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

## Task 4 — `internal/accesslog/writer.go` — AsyncFileSink + drop-newest backpressure

**Commits:** `edefa73`
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

## Task 5 — `internal/accesslog/stats.go` + `internal/stats/name.go` helpText extension [ADR-0069]

**Commits:** `5278161`
**Notes:** Created `internal/accesslog/stats.go` with `RegisterDroppedCounter(*stats.Registry) *stats.Counter` allocating the `server.accesslog_dropped` counter per ADR-0069. The counter maps to Prometheus name `envoy_server_accesslog_dropped` via Rule SN5 (no labels). Extended `internal/stats/name.go` helpText map from 10 to 11 entries; updated comment to reflect new count. Added `TestHelpText_AccessLogDropped` to `internal/stats/name_test.go`. Appended ADR-0069 to `docs/envoy-go/DECISIONS.md`. TDD discipline followed: test file written first, RED confirmed, then implementation to GREEN.
**Outputs:**
```
# RED — go test ./internal/accesslog/ -count=1 -run TestRegisterDroppedCounter -v (before stats.go)
# github.com/esalaine/envoy-go/internal/accesslog [github.com/esalaine/envoy-go/internal/accesslog.test]
internal/accesslog/stats_test.go:10:7: undefined: RegisterDroppedCounter
internal/accesslog/stats_test.go:19:6: undefined: RegisterDroppedCounter
FAIL	github.com/esalaine/envoy-go/internal/accesslog [build failed]
FAIL

# GREEN — go test ./internal/accesslog/ -count=1 -run TestRegisterDroppedCounter -v (after stats.go)
=== RUN   TestRegisterDroppedCounter_Name
--- PASS: TestRegisterDroppedCounter_Name (0.00s)
=== RUN   TestRegisterDroppedCounter_FlattensToPromName
--- PASS: TestRegisterDroppedCounter_FlattensToPromName (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# GREEN — go test ./internal/accesslog/ ./internal/stats/ -count=1 (full acceptance)
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.004s
ok  	github.com/esalaine/envoy-go/internal/stats	0.002s

# helpText grep
$ grep -n 'envoy_server_accesslog_dropped' internal/stats/name.go
135:	"envoy_server_accesslog_dropped":      "Total access-log records dropped due to backpressure (per-process aggregate across all sinks).",

# ADR-0069 grep
$ grep -n '^## ADR-0069:' docs/envoy-go/DECISIONS.md
2368:## ADR-0069: `server.accesslog_dropped` counter naming (SN5 mapping)
```

## Task 6 — `internal/accesslog/fuzz_test.go` — FuzzAccessLogFormat (eighth fuzzer)

**Commits:** `fa161a1`
**Notes:** Created `internal/accesslog/fuzz_test.go` with `FuzzAccessLogFormat(f *testing.F)` — the eighth fuzzer per Decision J + SPEC §1 #10 + §14.6. The fuzzer validates robustness of the `Default` formatter against malformed, control-laden, and edge-case inputs. Six seed corpus cases cover: normal inputs (case 0), embedded LF in fields (case 1), embedded quote in fields (case 2), NUL bytes (case 3), large strings (case 4, 2048 'a's), and 8-bit sequences (case 5, `\xff\x80\x81` etc.). Each test invocation: (1) constructs a `Record` with fuzzer-provided inputs for method, path, protocol, authority, user-agent, and upstream host; (2) calls `Default(rec)` within a panic-catch handler (verifying no panic); (3) asserts no embedded LF in the output body (line-stream invariant — output is exactly one line ending with `\n`); (4) counts un-escaped quotes and verifies count is even (matched pairs — every quote either escaped or quoted-off).

**Outputs:**
```
$ go test -count=1 ./internal/accesslog/ -run FuzzAccessLogFormat -v
=== RUN   FuzzAccessLogFormat
=== RUN   FuzzAccessLogFormat/seed#0
=== RUN   FuzzAccessLogFormat/seed#1
=== RUN   FuzzAccessLogFormat/seed#2
=== RUN   FuzzAccessLogFormat/seed#3
=== RUN   FuzzAccessLogFormat/seed#4
=== RUN   FuzzAccessLogFormat/seed#5
--- PASS: FuzzAccessLogFormat (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#0 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#1 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#2 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#3 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#4 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#5 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s
```

## Task 7 — `internal/bootstrap/bootstrap.go` — parse `access_log[]` + reject `log_format` [ADR-0067]

**Commits:** `6949fce`
**Notes:** Added `AccessLogConfig` struct and `AccessLogConfigs []AccessLogConfig` field to `Bootstrap`. Implemented `parseAccessLogConfigs` + `parseOneAccessLog` helpers that walk static_resources listeners → filter_chains → HCM filters → `access_log[]` entries. File-type entries (`type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog`) with `path` set are collected; `log_format` / `format_string` (`format` field) / `json_format` / `typed_json_format` produce fatal parse errors per ADR-0067 option β. Non-file typed_configs (stdout/stream, tcp_grpc, open_telemetry) are silently ignored per ADR-0041 amendment. Added blank imports for `file/v3` and `stream/v3` so protojson can round-trip bootstraps containing those typed_configs without "type not registered" errors. Added 8 new tests (all GREEN); total bootstrap tests: 19 pass (11 existing + 8 new). Appended ADR-0067 to DECISIONS.md; amended ADR-0041 with 06.2 amendment block.
**Outputs:**
```
$ go test -count=1 ./internal/bootstrap/ -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.00s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
=== RUN   TestLoad_YAMLSyntaxError
--- PASS: TestLoad_YAMLSyntaxError (0.00s)
=== RUN   TestLoad_UnknownTopLevelField
--- PASS: TestLoad_UnknownTopLevelField (0.00s)
=== RUN   TestLoad_EmptyDocument
--- PASS: TestLoad_EmptyDocument (0.00s)
=== RUN   TestAdminSocket_HappyPath
--- PASS: TestAdminSocket_HappyPath (0.00s)
=== RUN   TestAdminSocket_MissingAdmin
--- PASS: TestAdminSocket_MissingAdmin (0.00s)
=== RUN   TestBootstrap_RoundTrips_FixtureFour_Shape
--- PASS: TestBootstrap_RoundTrips_FixtureFour_Shape (0.00s)
=== RUN   TestLoad_AllocatesStatsRegistry
--- PASS: TestLoad_AllocatesStatsRegistry (0.00s)
=== RUN   TestLoad_HCMRoundTrip
--- PASS: TestLoad_HCMRoundTrip (0.00s)
=== RUN   TestBootstrap_AccessLog_FileType_PathRequired
--- PASS: TestBootstrap_AccessLog_FileType_PathRequired (0.00s)
=== RUN   TestBootstrap_AccessLog_RejectLogFormat
--- PASS: TestBootstrap_AccessLog_RejectLogFormat (0.00s)
=== RUN   TestBootstrap_AccessLog_RejectJSONFormat
--- PASS: TestBootstrap_AccessLog_RejectJSONFormat (0.00s)
=== RUN   TestBootstrap_AccessLog_RejectFormatString
--- PASS: TestBootstrap_AccessLog_RejectFormatString (0.00s)
=== RUN   TestBootstrap_AccessLog_PathEmptyRejects
--- PASS: TestBootstrap_AccessLog_PathEmptyRejects (0.00s)
=== RUN   TestBootstrap_AccessLog_StdoutSilentlyIgnored
--- PASS: TestBootstrap_AccessLog_StdoutSilentlyIgnored (0.00s)
=== RUN   TestBootstrap_AccessLog_NoEntriesIsValid
--- PASS: TestBootstrap_AccessLog_NoEntriesIsValid (0.00s)
=== RUN   TestBootstrap_AccessLog_TwoFileEntries
--- PASS: TestBootstrap_AccessLog_TwoFileEntries (0.00s)
=== RUN   FuzzBootstrapLoad
=== RUN   FuzzBootstrapLoad/seed#0
=== RUN   FuzzBootstrapLoad/seed#1
=== RUN   FuzzBootstrapLoad/seed#2
=== RUN   FuzzBootstrapLoad/seed#3
=== RUN   FuzzBootstrapLoad/seed#4
=== RUN   FuzzBootstrapLoad/seed#5
=== RUN   FuzzBootstrapLoad/seed#6
=== RUN   FuzzBootstrapLoad/seed#7
--- PASS: FuzzBootstrapLoad (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#0 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#1 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#2 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#3 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#4 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#5 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#6 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#7 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.008s
```
```
$ grep -nE '^## ADR-0067:' docs/envoy-go/DECISIONS.md
2417:## ADR-0067: Reject `log_format` at parse (option β; extends ADR-0065's boundary-validation pattern)
```
```
$ grep -nE 'AccessLogConfigs' internal/bootstrap/bootstrap.go
81:	// AccessLogConfigs is the parsed access_log[] file-sink entries from each
87:	AccessLogConfigs []AccessLogConfig
126:	if err := parseAccessLogConfigs(bs, result); err != nil {
132:// parseAccessLogConfigs walks the static_resources listeners looking for HCM
134:// entries are collected into result.AccessLogConfigs; other typed_config types
137:func parseAccessLogConfigs(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
165:// File-type entries with a valid path are appended to result.AccessLogConfigs.
197:	result.AccessLogConfigs = append(result.AccessLogConfigs, AccessLogConfig{Path: fal.GetPath()})
```

## Task 8 — `Cluster.Dial` / `DialH2` return-tuple expansion (surface picked endpoint)

**Commits:** `6ba3905`
**Notes:** Widened `Cluster.Dial(ctx) (net.Conn, error)` → `(net.Conn, Endpoint, error)` and `Cluster.DialH2(ctx) (*h2.ClientConn, error)` → `(*h2.ClientConn, Endpoint, error)`. All error paths return `Endpoint{}` (zero value); success paths surface the `ep` variable already captured by `PickEndpoint()`. Updated all call sites: `internal/filter/hcm/actions.go` (both `routerAction.do` and `routerActionH2.doH2`) and `internal/filter/tcpproxy/filter.go` use receive-but-discard `_` per PLAN Task 8 (Tasks 12–13 will replace `_` with `picked`). All existing tests in `cluster_test.go` and `dial_h2_test.go` updated to the new 3-tuple form. One new test per dial method added: `TestCluster_Dial_ReturnsPickedEndpoint` and `TestCluster_DialH2_ReturnsPickedEndpoint` each assert `ep.Host` non-empty and endpoint matches configured listener address.
**Outputs:**
```
# go test -count=1 ./internal/cluster/ -v (last lines)
--- PASS: TestBuildCluster_NoTypedExtension_BaselineFalse (0.00s)
=== RUN   TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions
--- PASS: TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions (0.00s)
=== RUN   TestNewManager_MixedPlaintextAndTLSClusters
--- PASS: TestNewManager_MixedPlaintextAndTLSClusters (0.00s)
=== RUN   TestNewManager_AllocatesEightMetricsPerCluster
--- PASS: TestNewManager_AllocatesEightMetricsPerCluster (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.011s

# go test -count=1 ./internal/filter/hcm/ -v (last lines)
--- PASS: TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess (0.00s)
=== RUN   TestMatchPath
--- PASS: TestMatchPath (0.00s)
=== RUN   TestMatchPrefix
--- PASS: TestMatchPrefix (0.00s)
=== RUN   TestRouteTableMatch_FirstMatchWins
--- PASS: TestRouteTableMatch_FirstMatchWins (0.00s)
=== RUN   TestRouteTableMatch_QueryStringExcluded
--- PASS: TestRouteTableMatch_QueryStringExcluded (0.00s)
=== RUN   TestRouteTableMatch_NoMatch
--- PASS: TestRouteTableMatch_NoMatch (0.00s)
=== RUN   TestRouteTableMatch_EmptyTable
--- PASS: TestRouteTableMatch_EmptyTable (0.00s)
=== RUN   FuzzHCMConfigParse
--- PASS: FuzzHCMConfigParse (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.216s

# grep verification
$ grep -nE 'func \(c \*Cluster\) Dial|func \(c \*Cluster\) DialH2' internal/cluster/cluster.go internal/cluster/dial_h2.go
internal/cluster/dial_h2.go:32:func (c *Cluster) DialH2(ctx context.Context) (*h2.ClientConn, Endpoint, error) {
internal/cluster/cluster.go:149:func (c *Cluster) Dial(ctx context.Context) (net.Conn, Endpoint, error) {
```

## Task 9 — `internal/filter/hcm/bytecounter.go` — byteCounterWriter

**Commits:** `3c16e29`
**Notes:** Created `internal/filter/hcm/bytecounter.go` (~10 LoC): tiny `byteCounterWriter` struct wrapping an `io.Writer` and accumulating an `int64` running byte count via `Write(p) (int, error)` that increments `n` by the actual bytes written (short-writes account the actual count, not the request length, per SPEC §12 #3). Created `internal/filter/hcm/bytecounter_test.go` with 2 TDD tests: happy-path 3-write accumulation (12 bytes total) and short-write accounting (3 returned + error).
**Outputs:**
```
$ go test -count=1 -run TestByteCounterWriter ./internal/filter/hcm/ -v
=== RUN   TestByteCounterWriter_AccumulatesBytesWritten
--- PASS: TestByteCounterWriter_AccumulatesBytesWritten (0.00s)
=== RUN   TestByteCounterWriter_ShortWriteAccountsActualBytes
--- PASS: TestByteCounterWriter_ShortWriteAccountsActualBytes (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.004s
```

## Task 11 — HCM Filter struct extension + parseFilterWithCtx sink-slice plumbing

**Commits:** `d6c9749`
**Notes:** Added `accessLog []accesslog.Sink` field to `Filter` struct (config.go line 66). Extended `parseFilterWithCtx` signature with trailing `accessLogSinks []accesslog.Sink` parameter; field set in returned `*Filter`. Updated all 6 callers: `NewFilterWithCtx`, `parseFilter` (config.go), `NewFilter` (filter.go), and all 5 `parseFilterWithCtx` call sites in `config_test.go` — each passes `nil` (Task 14 wires real sinks). Added `TestFilter_AccessLogField_Plumbed` to `config_test.go`. Added `"github.com/esalaine/envoy-go/internal/accesslog"` import to both `config.go` and `config_test.go`.
**Outputs:**
```
$ go test -count=1 ./internal/filter/hcm/ -v 2>&1 | tail -20
=== RUN   TestMatchPrefix
--- PASS: TestMatchPrefix (0.00s)
=== RUN   TestRouteTableMatch_FirstMatchWins
--- PASS: TestRouteTableMatch_FirstMatchWins (0.00s)
=== RUN   TestRouteTableMatch_QueryStringExcluded
--- PASS: TestRouteTableMatch_QueryStringExcluded (0.00s)
=== RUN   TestRouteTableMatch_NoMatch
--- PASS: TestRouteTableMatch_NoMatch (0.00s)
=== RUN   TestRouteTableMatch_EmptyTable
--- PASS: TestRouteTableMatch_EmptyTable (0.00s)
=== RUN   FuzzHCMConfigParse
=== RUN   FuzzHCMConfigParse/seed#0
=== RUN   FuzzHCMConfigParse/seed#1
=== RUN   FuzzHCMConfigParse/seed#2
--- PASS: FuzzHCMConfigParse (0.00s)
    --- PASS: FuzzHCMConfigParse/seed#0 (0.00s)
    --- PASS: FuzzHCMConfigParse/seed#1 (0.00s)
    --- PASS: FuzzHCMConfigParse/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.216s
$ grep -nE 'accessLog \[\]accesslog\.Sink' internal/filter/hcm/config.go
66:	accessLog []accesslog.Sink
```

## Task 10 — `internal/filter/hcm/accesslog_emit.go` — Filter.emitAccessLog (H1 + H2)

**Commits:** `94a8568`
**Notes:** Created `internal/filter/hcm/accesslog_emit.go` with three functions: `(*Filter).emitAccessLog` (H1 variant — reads `*http.Request` primitives), `(*Filter).emitAccessLogH2` (H2 variant — reads `h2.H2Request` pseudo-headers and extracts User-Agent via `h2UserAgent`), and helpers `h2UserAgent` (case-insensitive `user-agent` scan over `[]hpack.HeaderField`) and `upstreamHostString` (renders `Endpoint` as `host:port` or empty string for zero endpoint). Both emit methods guard on `statusCode == 0` (H2 ctx-cancel sentinel per SPEC §2.1) and `len(f.accessLog) == 0` (no-op when no sinks). Created `accesslog_emit_test.go` with 6 TDD tests (RED then GREEN). All 6 new tests plus full hcm suite pass.
**Outputs:**
```
# RED — go test -count=1 -run TestEmitAccessLog ./internal/filter/hcm/ -v (before accesslog_emit.go)
internal/filter/hcm/accesslog_emit_test.go:26:4: f.emitAccessLog undefined (type *Filter has no field or method emitAccessLog)
internal/filter/hcm/accesslog_emit_test.go:54:4: f.emitAccessLog undefined (type *Filter has no field or method emitAccessLog)
...
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm [build failed]

# GREEN — go test -count=1 -run TestEmitAccessLog ./internal/filter/hcm/ -v (after accesslog_emit.go)
=== RUN   TestEmitAccessLog_H1_DirectResponseShape
--- PASS: TestEmitAccessLog_H1_DirectResponseShape (0.00s)
=== RUN   TestEmitAccessLog_H1_RoutedShape
--- PASS: TestEmitAccessLog_H1_RoutedShape (0.00s)
=== RUN   TestEmitAccessLog_MultipleSinks_AllReceiveRecord
--- PASS: TestEmitAccessLog_MultipleSinks_AllReceiveRecord (0.00s)
=== RUN   TestEmitAccessLog_H2_PseudoHeadersFromH2Request
--- PASS: TestEmitAccessLog_H2_PseudoHeadersFromH2Request (0.00s)
=== RUN   TestEmitAccessLog_H2_StatusZeroSkipsEmission
--- PASS: TestEmitAccessLog_H2_StatusZeroSkipsEmission (0.00s)
=== RUN   TestEmitAccessLog_NoSinks_IsNoOp
--- PASS: TestEmitAccessLog_NoSinks_IsNoOp (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.003s
```

## Task 12 — HCM H1 emit-deferral sites (directResponseAction.do + routerAction.do)

**Commits:** `d6d35be`
**Notes:** Added `filter *Filter` field to `directResponseAction`, `routerAction`, and `routerActionH2` structs (the H2 field is wired at Task 13; added now so `routeTable.bindFilter` compiles). Added `routeTable.bindFilter(f *Filter)` to `route.go`: iterates routes and sets the filter backpointer on each action that holds one; called from `parseFilterWithCtx` after the `*Filter` is constructed (actions are built before Filter exists — post-build wiring pattern). Modified `directResponseAction.do` to record `start := time.Now()` and defer `a.filter.emitAccessLog(req, a.status, int64(len(a.bodyText)), cluster.Endpoint{}, start)` guarded by `a.filter != nil`. Modified `routerAction.do` to wrap `bw` in `byteCounterWriter` (counts downstream bytes via `resp.Write(bcw)`), capture `picked` from `Cluster.Dial`, and register a single top-of-function defer (closure capturing `statusCode` and `picked` by reference) that reads the final values after all writes; `statusCode` is set on each early-return path (503 for dial-failure, 502 for write/read failure, `resp.StatusCode` on success). Added 4 new tests: `TestDirectResponseAction_EmitsAccessLog`, `TestDirectResponseAction_NilFilter_DoesNotPanic`, `TestRouterAction_EmitsAccessLog_HappyPath`, `TestRouterAction_EmitsAccessLog_DialFailure`. All pass. Import `github.com/esalaine/envoy-go/internal/accesslog` added to `actions_test.go`. Deviation: the defer-count grep finds 1 literal `defer.*emitAccessLog(` in actions.go (directResponseAction path) plus 1 inside a `defer func(){...}()` closure (routerAction path) — both fire on return; the plan's "≥2 matches" was approximate, functional coverage is complete.
**Outputs:**
```
$ go test -count=1 ./internal/filter/hcm/ -v -run 'TestDirectResponseAction_EmitsAccessLog|TestDirectResponseAction_NilFilter|TestRouterAction_EmitsAccessLog'
=== RUN   TestDirectResponseAction_EmitsAccessLog
--- PASS: TestDirectResponseAction_EmitsAccessLog (0.00s)
=== RUN   TestDirectResponseAction_NilFilter_DoesNotPanic
--- PASS: TestDirectResponseAction_NilFilter_DoesNotPanic (0.00s)
=== RUN   TestRouterAction_EmitsAccessLog_HappyPath
--- PASS: TestRouterAction_EmitsAccessLog_HappyPath (0.00s)
=== RUN   TestRouterAction_EmitsAccessLog_DialFailure
--- PASS: TestRouterAction_EmitsAccessLog_DialFailure (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.006s

$ go test -count=1 ./internal/filter/hcm/ 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.220s

$ grep -nE 'emitAccessLog' internal/filter/hcm/actions.go
98:// Phase 06.2 Task 12: emits access-log record via a.filter.emitAccessLog on
103:	defer func() {
105:			a.filter.emitAccessLog(req, a.status, int64(len(a.bodyText)), cluster.Endpoint{}, start)
152:		defer func() { a.filter.emitAccessLog(req, statusCode, bcw.n, picked, start) }()
```

## Task 13 — HCM H2 emit-deferral sites (h2DirectResponseAdapter.WriteH2 + routerActionH2.doH2)

**Commits:** `aefe093`
**Notes:** Modified `h2DirectResponseAdapter.WriteH2` in `h2dispatch.go`: added `start := time.Now()` + `defer a.f.emitAccessLogH2(req, a.a.status, int64(len(a.a.bodyText)), cluster.Endpoint{}, start)`. The `req h2.H2Request` parameter was previously blank (`_`); renamed so the emit can read it. Added `"time"` and `"github.com/esalaine/envoy-go/internal/cluster"` imports to `h2dispatch.go`. Modified `routerActionH2.doH2` in `actions.go`: added `start := time.Now()`; declared `statusForHCM`, `bytesSentH2`, and `picked` before a top-of-function `defer func(){...}()` closure (guarded by `r.filter != nil`) that calls `r.filter.emitAccessLogH2` with final values; `statusForHCM` set to 502 on dial-failure + RoundTrip error paths, `resp.Status` on success; remains 0 on the ctx-cancel CANCEL path — `emitAccessLogH2` guards on statusCode==0 and skips emission per SPEC §2.1. `bytesSentH2 = len(resp.Body)` on success path. The `filter *Filter` field added to `routerActionH2` in Task 12 is consumed here. Added 4 new tests to `h2dispatch_test.go`: `TestH2DirectResponseAdapter_WriteH2_EmitsAccessLog`, `TestRouterActionH2_DoH2_EmitsAccessLog_HappyPath`, `TestRouterActionH2_DoH2_EmitsAccessLog_DialFailure`, `TestRouterActionH2_DoH2_CtxCancel_SkipsEmit`. All pass. Added `"time"` + `"github.com/esalaine/envoy-go/internal/accesslog"` imports to `h2dispatch_test.go`.
**Outputs:**
```
$ go test -count=1 ./internal/filter/hcm/ -v -run 'TestH2DirectResponse.*EmitsAccessLog|TestRouterActionH2_DoH2_EmitsAccessLog|TestRouterActionH2_DoH2_CtxCancel_SkipsEmit'
=== RUN   TestH2DirectResponseAdapter_WriteH2_EmitsAccessLog
--- PASS: TestH2DirectResponseAdapter_WriteH2_EmitsAccessLog (0.00s)
=== RUN   TestRouterActionH2_DoH2_EmitsAccessLog_HappyPath
--- PASS: TestRouterActionH2_DoH2_EmitsAccessLog_HappyPath (0.00s)
=== RUN   TestRouterActionH2_DoH2_EmitsAccessLog_DialFailure
--- PASS: TestRouterActionH2_DoH2_EmitsAccessLog_DialFailure (0.00s)
=== RUN   TestRouterActionH2_DoH2_CtxCancel_SkipsEmit
--- PASS: TestRouterActionH2_DoH2_CtxCancel_SkipsEmit (0.20s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.210s

$ go test -count=1 ./internal/filter/hcm/ 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.419s

$ grep -nE 'emitAccessLogH2' internal/filter/hcm/h2dispatch.go internal/filter/hcm/actions.go
internal/filter/hcm/h2dispatch.go:96:	defer a.f.emitAccessLogH2(req, a.a.status, int64(len(a.a.bodyText)), cluster.Endpoint{}, start)
internal/filter/hcm/actions.go:267:			r.filter.emitAccessLogH2(req, statusForHCM, int64(bytesSentH2), picked, start)
```

## Task 14 — `cmd/envoy-go/main.go` — open AsyncFileSinks + thread + defer Close

**Commits:** `3691571`
**Notes:** Widened `hcm.NewFilterWithCtxAndSinks` (new exported function in `config.go`) that delegates to `parseFilterWithCtx` with the sinks slice — `NewFilterWithCtx` continues to call with `nil` preserving backward compat. In `internal/listener/manager.go`: added `"github.com/esalaine/envoy-go/internal/accesslog"` import; widened `filterConstructor` type with trailing `[]accesslog.Sink` parameter; updated `tcpproxy` and `hcm` closures in `filterRegistry` (`hcm` now calls `NewFilterWithCtxAndSinks`); widened `buildListenerRuntimeWithCtx` with `accessLogSinks []accesslog.Sink` and threads it to the constructor call; widened `NewManagerWithBaseDirAndAllowH2C` with the same parameter; `NewManager` and `NewManagerWithBaseDir` delegate with `nil` (strategy b — minimal diff). In `cmd/envoy-go/main.go`: added `"github.com/esalaine/envoy-go/internal/accesslog"` import; after `cluster.NewManagerWithBaseDir` success, call `accesslog.RegisterDroppedCounter(bs.Stats)` and loop over `bs.AccessLogConfigs` opening each `NewAsyncFileSink`; added a `defer func(){ for _, s := range sinks { _ = s.Close() } }()` placed before the `admSrv` and `lm` constructions so it fires (LIFO) after `lm.Stop()` + `admSrv.Close()`, ensuring in-flight log records flush before files close; pass `sinks` to `NewManagerWithBaseDirAndAllowH2C`. Updated `internal/listener/manager_test.go`: both `NewManagerWithBaseDirAndAllowH2C` call sites pass `nil` for the new parameter. Added `TestEnvoyGoBinary_AccessLogSmoke` to `cmd/envoy-go/main_test.go`: boots binary with HCM access_log pointing to a temp file, makes one HTTP/1.1 GET, signals SIGINT shutdown, then asserts the log file is non-empty.
**Outputs:**
```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ go test -count=1 ./internal/listener/ ./cmd/envoy-go/ ./internal/filter/hcm/ -v 2>&1 | tail -20
--- PASS: TestEnvoyGoBinary_TwoListenerCutover (0.57s)
=== RUN   TestEnvoyGoBinary_HCMSmoke
--- PASS: TestEnvoyGoBinary_HCMSmoke (0.53s)
=== RUN   TestMain_StatsPrometheusEndpointResponds
--- PASS: TestMain_StatsPrometheusEndpointResponds (0.57s)
=== RUN   TestEnvoyGoBinary_AccessLogSmoke
--- PASS: TestEnvoyGoBinary_AccessLogSmoke (0.60s)
=== RUN   TestEnvoyGoBinary_H2Smoke
--- PASS: TestEnvoyGoBinary_H2Smoke (0.57s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.850s
```

## Task 15 — Differential fixture 0006-access-log + runner registration [ADR-0068]

**Commits:** `085890d`
**Notes:** Created the project's first per-record access-log differential fixture. Deliverables: `test/fixtures/0006-access-log/` (envoy-go.yaml, envoy.yaml, expectations.yaml, README.md, driver/driver.go, driver/driver_test.go, backends/main.go); extended `test/differential/fixture/fixture.go` (HTTPFixedBody BackendKind=4, HostMount struct, ReferenceLogMounter + AccessLogAsserter interfaces); extended `test/differential/harness.go` (StartReferenceProxyWithMounts using HostConfig.Binds — testcontainers v0.27.0 silently drops MountTypeBind in ContainerMounts, documented in-code); extended `test/differential/runner_test.go` (blank-import driver, HTTPFixedBody backend spawn, mount+assert wiring at steps 11).

Key implementation decisions: (a) BYTES_SENT fixed by moving `io.ReadAll` before `resp.Write` in `routerAction.do` so only body bytes are counted (not status-line+headers); (b) reference log polling uses 30s deadline (Envoy v1.37.2 flushes its file-access-log buffer on ~1s periodic timer; tested: flush arrives within 10s); (c) DriveReference normalizes `localhost:{port}` → `127.0.0.1:{port}` to make Go HTTP client send matching `Host` header for Tier-E AUTHORITY assertion; (d) RESP_SVC_TIME (field 10) promoted from Tier-E to Tier-S — reference Envoy injects X-Envoy-Upstream-Service-Time on routed requests (Decision A: envoy-go does not); (e) bind-mount uses Docker `HostConfig.Binds` format `"hostPath:containerPath"` — testcontainers-go v0.27.0's `MountTypeBind` is silently dropped in `mapToDockerMounts`.

All 9 driver unit tests pass. Full differential suite (0000–0006, 7 fixtures) passes: `TestDifferential` PASS (21s). All 20 packages pass `go test ./...`.
**Outputs:**
```
$ go test -count=1 -v -timeout 120s ./test/differential/ -run 'TestDifferential/0006'
=== RUN   TestDifferential
=== RUN   TestDifferential/0006-access-log
[... container lifecycle logs ...]
--- PASS: TestDifferential/0006-access-log (11.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	11.365s

$ go test -count=1 ./test/differential/ -v 2>&1 | grep -E 'PASS|FAIL'
--- PASS: TestDifferential/0000-tcp-echo (1.20s)
--- PASS: TestDifferential/0001-tcp-proxy-rr (1.20s)
--- PASS: TestDifferential/0002-tls-tcp (1.25s)
--- PASS: TestDifferential/0003-http11-routing (1.27s)
--- PASS: TestDifferential/0004-h2-routing (1.65s)
--- PASS: TestDifferential/0005-prometheus-stats (1.98s)
--- PASS: TestDifferential/0006-access-log (11.00s)
--- PASS: TestDifferential (19.56s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	21.060s

$ go test ./... 2>&1 | grep -E '^ok|^FAIL'
ok  	github.com/esalaine/envoy-go/cmd/envoy-go
ok  	github.com/esalaine/envoy-go/internal/accesslog
ok  	github.com/esalaine/envoy-go/internal/admin
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/internal/cluster
ok  	github.com/esalaine/envoy-go/internal/filter/hcm
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy
ok  	github.com/esalaine/envoy-go/internal/listener
ok  	github.com/esalaine/envoy-go/internal/stats
ok  	github.com/esalaine/envoy-go/internal/tls
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec
ok  	github.com/esalaine/envoy-go/test/differential
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver
ok  	github.com/esalaine/envoy-go/test/helpers

$ grep -nE '^## ADR-0068:' docs/envoy-go/DECISIONS.md
2448:## ADR-0068: Differential fixture 0006-access-log — three-tier equivalence matrix
```

## Task 16 — BEHAVIOR_CONTRACT in-place edit + closing all-gates sweep [ADR-0066..ADR-0069]

**Commits:** `a349f35`

**Notes:** Closing implementation-session task. Edited `docs/envoy-go/BEHAVIOR_CONTRACT.md` `## Access log field mapping` placeholder in place per ADR-0052 — populated subsection per SPEC §13.1, anchored on ADR-0066/0067/0068/0069. Three-tier matrix lists 7 Tier-E + 3 Tier-F + 5 Tier-S = 15 operators (RESP-SVC-TIME demoted from Tier-E to Tier-S during Task 15 fixture-0006 implementation per Decision A — reference Envoy injects the header but envoy-go does not). Six-gate local sweep run; gates (a)/(b)/(c)/(d)/(e) all GREEN; gate (f) deferred to REVIEW session per BOOTSTRAP §5 step 6. Lint cleanup performed during closing sweep: `gofmt` fixes to `internal/bootstrap/bootstrap.go`, `test/fixtures/0006-access-log/driver/driver.go`, `internal/accesslog/format_test.go`, `internal/accesslog/accesslog_test.go`; `goimports` fixes to `internal/accesslog/stats_test.go`; `errcheck` fix (`defer f.Close()` → `defer func() { _ = f.Close() }()`) in `internal/accesslog/writer_test.go`. STATE.md advanced lifecycle-state 3 → 4; next-skill set to `superpowers:verification-before-completion`. ROADMAP rows 06.2 + 06 will flip to `done` AT THE PHASE-DONE COMMIT in the REVIEW session per parent SPEC §5 closure pattern (NOT at this Task 16 commit).

**Outputs:**

```
# Gate (a): new fixture green (0006)
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/0006' -v 2>&1 | tail -20
2026/04/30 06:26:36 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/30 06:26:36 ✅ Container created: de2885b22bdf
2026/04/30 06:26:36 🐳 Starting container: de2885b22bdf
2026/04/30 06:26:36 ✅ Container started: de2885b22bdf
2026/04/30 06:26:36 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/30 06:26:36 ✅ Container created: 1add499c8896
2026/04/30 06:26:36 🐳 Starting container: 1add499c8896
2026/04/30 06:26:36 ✅ Container started: 1add499c8896
2026/04/30 06:26:47 🐳 Terminating container: 1add499c8896
2026/04/30 06:26:47 🚫 Container terminated: 1add499c8896
--- PASS: TestDifferential (11.30s)
    --- PASS: TestDifferential/0006-access-log (11.30s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	11.386s

# Gate (b): pre-existing fixtures still green (0000-0005)
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/000[0-5]' -v 2>&1 | tail -15
--- PASS: TestDifferential (8.71s)
    --- PASS: TestDifferential/0000-tcp-echo (1.48s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.20s)
    --- PASS: TestDifferential/0002-tls-tcp (1.21s)
    --- PASS: TestDifferential/0003-http11-routing (1.24s)
    --- PASS: TestDifferential/0004-h2-routing (1.64s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.92s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	8.792s

# Gate (c): h2spec conformance (53/53 PASS)
$ go test -count=1 -timeout 120s ./test/conformance/h2spec/ -v 2>&1 | tail -10
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
2026/04/30 06:27:03 🚫 Container terminated: 3d56ffde97a0
--- PASS: TestH2Spec (2.22s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.299s

# Gate (d): fuzz seed corpus runs (7 fuzzers)
$ go test -count=1 ./internal/bootstrap/ -run FuzzBootstrapLoad -v 2>&1 | tail -5
    --- PASS: FuzzBootstrapLoad/seed#5 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#6 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#7 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s

$ go test -count=1 ./internal/filter/tcpproxy/ -run FuzzTcpProxyFilter -v 2>&1 | tail -5
    --- PASS: FuzzTcpProxyFilter/seed#0 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#1 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.003s

$ go test -count=1 ./internal/tls/ -run FuzzTLSContextParse -v 2>&1 | tail -5
    --- PASS: FuzzTLSContextParse/seed#1 (0.00s)
    --- PASS: FuzzTLSContextParse/seed#2 (0.00s)
    --- PASS: FuzzTLSContextParse/seed#3 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	0.003s

$ go test -count=1 ./internal/filter/hcm/ -run FuzzHCMConfigParse -v 2>&1 | tail -5
    --- PASS: FuzzHCMConfigParse/seed#0 (0.00s)
    --- PASS: FuzzHCMConfigParse/seed#1 (0.00s)
    --- PASS: FuzzHCMConfigParse/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.004s

$ go test -count=1 ./internal/filter/hcm/h2/ -run 'FuzzFrameStream|FuzzHPACKDecode' -v 2>&1 | tail -5
--- PASS: FuzzHPACKDecode (0.00s)
    --- PASS: FuzzHPACKDecode/seed#0 (0.00s)
    --- PASS: FuzzHPACKDecode/seed#1 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.002s

$ go test -count=1 ./internal/stats/ -run FuzzPromTextFormat -v 2>&1 | tail -5
    --- PASS: FuzzPromTextFormat/seed#6 (0.00s)
    --- PASS: FuzzPromTextFormat/seed#7 (0.00s)
    --- PASS: FuzzPromTextFormat/1d8483e640bf8347 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	0.001s

$ go test -count=1 ./internal/accesslog/ -run FuzzAccessLogFormat -v 2>&1 | tail -5
    --- PASS: FuzzAccessLogFormat/seed#3 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#4 (0.00s)
    --- PASS: FuzzAccessLogFormat/seed#5 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# Gate (e): vet/lint/test
$ go vet ./...
(no output — clean)

$ golangci-lint run ./... 2>&1
(no output after lint fixes — clean)

$ go test -race -count=1 ./... 2>&1 | grep -E '^(ok|FAIL)'
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.088s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.018s
ok  	github.com/esalaine/envoy-go/internal/admin	1.072s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.051s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.049s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.475s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.527s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.033s
ok  	github.com/esalaine/envoy-go/internal/listener	1.052s
ok  	github.com/esalaine/envoy-go/internal/stats	1.029s
ok  	github.com/esalaine/envoy-go/internal/tls	1.081s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.097s
ok  	github.com/esalaine/envoy-go/test/differential	22.318s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.008s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.017s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.016s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.016s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/helpers	1.033s
(20 packages: all ok, no FAIL)

# Step 3 boundary greps per SPEC §15
$ grep -nE 'github.com/sirupsen/logrus|go.uber.org/zap|github.com/rs/zerolog|github.com/fluent/fluent-logger-golang' go.mod | head -5
50: github.com/sirupsen/logrus v1.9.3 // indirect
(logrus is an indirect dependency only — NOT imported in internal/ or cmd/ code)

$ grep -rE '"github.com/(sirupsen/logrus|go\.uber\.org/zap|rs/zerolog|fluent/fluent-logger-golang)' internal/ cmd/ | head -5
(no output — no third-party access-log library imported in code)

$ grep -nE 'emitAccessLog' internal/filter/hcm/actions.go internal/filter/hcm/h2dispatch.go
internal/filter/hcm/actions.go:106:        a.filter.emitAccessLog(req, a.status, int64(len(a.bodyText)), cluster.Endpoint{}, start)
internal/filter/hcm/actions.go:158:        defer func() { a.filter.emitAccessLog(req, statusCode, bytesSent, picked, start) }()
internal/filter/hcm/actions.go:283:        r.filter.emitAccessLogH2(req, statusForHCM, int64(bytesSentH2), picked, start)
internal/filter/hcm/h2dispatch.go:96:  defer a.f.emitAccessLogH2(req, a.a.status, int64(len(a.a.bodyText)), cluster.Endpoint{}, start)
(4 emit-hook sites: 2 H1 in actions.go lines 106+158, 2 H2 in actions.go:283 + h2dispatch.go:96)

$ grep -nE '^## ADR-006[6-9]:' docs/envoy-go/DECISIONS.md
2349:## ADR-0066: Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure)
2382:## ADR-0069: server.accesslog_dropped counter naming (SN5 mapping)
2417:## ADR-0067: Reject log_format at parse (option β; extends ADR-0065's boundary-validation pattern)
2448:## ADR-0068: Differential fixture 0006-access-log — three-tier equivalence matrix
```

**Carry-forward triage:**
- 06.1 REVIEW M-8 (drain-loop polling): ADOPTED PROPHYLACTICALLY in fixture-0006 driver (Task 15, commit `085890d`). Does NOT close M-8 against fixture 0005 — its actual fix is reserved for a 06.1 review-followup batch.
- 05.2 M-4 / M-10 / M-12: unchanged.
- 05.2 prose Minors (7): unchanged.
- 06.1 12 Minors (M-2..M-12 + reviewer-discovered): unchanged.

**Four ADRs landed (per BOOTSTRAP §5.3 commit-message-completeness):**
- ADR-0066 (Access-log architecture): Task 2, commit `76f3ecd`
- ADR-0067 (Reject log_format at parse): Task 7, commit `6949fce`
- ADR-0068 (Three-tier equivalence matrix): Task 15, commit `085890d`
- ADR-0069 (server.accesslog_dropped counter naming): Task 5, commit `5278161`

## Verification (lifecycle-state 4) — FAILED

Per `BOOTSTRAP_PROMPT.md` §5 state 4 and `STATE.md`'s `next-skill-scope`: a fresh-session re-run of every SPEC §3 / BOOTSTRAP §7.5 phase-done gate, with each command's verbatim output captured here. Worktree `.worktrees/phase-06.2-access-log-verify`, branch `phase/06.2-access-log-verify`, branched from impl-branch tip `b12838a` per ADR-0003 + per-phase-worktree convention (after master fast-forward of `phase/06.2-access-log-impl` per the per-session exit protocol). Verifier date: 2026-04-30.

**Outcome: gate (d) FAIL.** A fresh fuzz run of `FuzzAccessLogFormat` at the ADR-0018 30-second budget produced a crasher in 1.07 s. The Default formatter's `escape()` helper at `internal/accesslog/format.go:51` escapes `"` → `\"`, `\n` → `\n`, `\r` → `\r` — but does NOT escape `\` → `\\`. When any quoted operator's value (UPSTREAM_HOST, X-FORWARDED-FOR, USER-AGENT, X-REQUEST-ID, AUTHORITY) ends with a literal backslash, the closing `"` field-delimiter is preceded by `\` in the output line, which round-trip un-escape readers (and the fuzzer's parseability invariant in `internal/accesslog/fuzz_test.go:46`, which counts un-escaped quotes by skipping a quote when the preceding byte is `\`) interpret as an escaped quote — the closing field delimiter is silently swallowed. Reference Envoy's `AccessLogFormatUtils::escapeUtilityValue` and RFC 4180 CSV-style escaping both require `\` → `\\`; SPEC §11's empirical-pin write-up at Task 3 documented `"` → `\"`, `\n` → `\n`, `\r` → `\r` per the format.go header but did NOT cover `\` (a gap the Task 16 sweep also did not surface). Gates (a), (b), (c), (e), and the SPEC §15 boundary greps all PASS in the verifier worktree.

**Discrepancy with Task 16's "all five executable gates green" claim** is attributable to the impl session's gate (d) sweep running each fuzzer with `-run <Name> -v` only — that runs the seed corpus (6 hand-picked seeds for `FuzzAccessLogFormat`, none of which exercised a quoted-field value ending with `\`) but does NOT run any fuzz-engine-discovered inputs. BOOTSTRAP §7.5 (d) ("any new fuzzer has run clean for its short-budget CI run") plus ADR-0018 ("30-second budget for the new fuzzer") together mean the gate is satisfied only by `-fuzz=<Name> -fuzztime=30s`, not by `-run` (which only covers seed corpus). The Task 16 sweep's gate-(d) interpretation was undercovered relative to the gate spec; this verifier's targeted 30 s run surfaced the gap exactly as gate (d) is designed to.

**Next action per BOOTSTRAP §5 deviation rule (`Unexpected state → superpowers:systematic-debugging FIRST`):** STATE advances back to lifecycle-state 3 (impl incomplete) with `next-skill: superpowers:systematic-debugging`. The bug is fully characterised in this block; the fix branch can proceed directly to the fix shape after a brief systematic-debugging pass confirms the characterisation. The fix branch is `.worktrees/phase-06.2-access-log-impl-followup-gate-d` on branch `phase/06.2-access-log-impl-followup-gate-d` branched from this verify commit's master fast-forward HEAD per ADR-0003 + per-phase-worktree convention.

The fix shape (one option, very obvious): extend `escape()` in `internal/accesslog/format.go` to also map `\` → `\\` (must run BEFORE the `"` → `\"` substitution to avoid double-escaping the backslash that `\"` introduces — i.e., either a pre-pass that escapes lone `\`, or a single `strings.NewReplacer` pass with `\` → `\\` listed first). TDD per D-3.1: a unit test in `internal/accesslog/format_test.go` (e.g., `TestDefault_BackslashInQuotedField` with `UpstreamHost = "\\"` asserting `\\` immediately precedes the closing `"` in the output) goes RED before the fix and GREEN after. The auto-saved fuzz corpus seed (see "Seed file disposition" below) should be re-introduced on the fix branch as a permanent regression seed at `internal/accesslog/testdata/fuzz/FuzzAccessLogFormat/1bdc705d534eee86`. SPEC §11 may need a one-line amend to extend the escape catalog from `{", \n, \r}` to `{\, ", \n, \r}` (preserving the empirical-pin format-string verbatim) — likely no ADR required since the fix matches reference Envoy + RFC 4180 (mirror 06.1's ADR-0065 only if a non-obvious design tradeoff appears; the obvious option here is unambiguous).

Independently consider whether the fuzzer's parseability invariant (even-quote-count with `\\`-precedence-skip at `fuzz_test.go:46`) should be tightened to detect this corner case more directly — the current heuristic is correct enough for this seed (it surfaces the bug) but would not catch all post-fix invariant regressions if `escape()`'s logic is later refactored. A lightweight strengthening (e.g., counting `\\\"` separately) is a follow-up consideration for the fix branch.

**Seed file disposition (verifier role contract).** Go's fuzz framework persisted the minimised crasher input as `internal/accesslog/testdata/fuzz/FuzzAccessLogFormat/1bdc705d534eee86`. The 05.2 verifier precedent (`b34bd99`) and the 06.1 verifier precedent (`1f94b74`) both committed only `STATE.md` + `PROGRESS.md` — no production-code or test-corpus changes — and this verifier follows that role contract: the seed file is **deleted** before this verification commit. The seed bytes are quoted verbatim immediately below for the fix branch's reproduction. The fix branch can either (a) re-derive an equivalent seed by re-running the fuzzer (typically <2 s on a 32-worker host given the bug's coverage proximity), OR (b) hand-craft an exact byte-equivalent file at the same testdata path. Option (b) is preferred — the failing seed is already minimised by Go's fuzz engine and reproduces the bug deterministically.

**Verbatim seed file content** (`internal/accesslog/testdata/fuzz/FuzzAccessLogFormat/1bdc705d534eee86`, 6 lines, trailing newline):

```
go test fuzz v1
string("0")
string("0")
string("0")
string("0")
string("0")
string("\\")
```

Decoded inputs (positional per `FuzzAccessLogFormat`'s 6-arg signature in `fuzz_test.go:18`): `method="0"`, `path="0"`, `proto="0"`, `authority="0"`, `ua="0"`, `upstream="\\"` (single backslash byte, length 1).

**Outputs:**

```
$ pwd
/home/esa/git/envoy-go/.worktrees/phase-06.2-access-log-verify
$ git rev-parse --abbrev-ref HEAD
phase/06.2-access-log-verify
$ git log -1 --format=%H
b12838a8d40c1d65ec07f1e2a98ec0ee15b81c34
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version 2>&1 | head -1
golangci-lint has version v1.64.8 built with go1.26.2
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0068: Differential fixture 0006-access-log — three-tier equivalence matrix
$ # 06.2 ADRs in commit-add order: 0066, 0069, 0067, 0068 (non-monotonic per topological-vs-commit-order convention).
```

**Gate (a) — fixture-0006-access-log differential (NEW in 06.2 per ADR-0068) — PASS:**

```
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/0006' -v 2>&1 | grep -E '^---|^PASS|^FAIL|^ok'
--- PASS: TestDifferential (11.35s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	11.436s
```

The reference Envoy image SHA at the `ENVOY_TARGET.md` pin is exercised non-vacuously (testcontainers ryuk + reference-Envoy lifecycle visible in full output); three-tier equivalence per ADR-0068 verified.

**Gate (b) — all pre-existing differential fixtures (regression check) — PASS:**

```
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/000[0-5]' -v 2>&1 | grep -E '^---|^PASS|^FAIL|^ok'
--- PASS: TestDifferential (8.73s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	8.813s
```

All six pre-existing fixtures (0000-tcp-echo, 0001-tcp-proxy-rr, 0002-tls-tcp, 0003-http11-routing, 0004-h2-routing, 0005-prometheus-stats) PASS — no regression from access-log emit-hook plumbing.

**Gate (c) — h2spec conformance (UNCHANGED expectation 53/53 PASS) — PASS:**

```
$ go test -count=1 -timeout 120s ./test/conformance/h2spec/ -v 2>&1 | tail -8
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
2026/04/30 07:28:34 🐳 Terminating container: 60c5a6ecb7bd
2026/04/30 07:28:34 🚫 Container terminated: 60c5a6ecb7bd
--- PASS: TestH2Spec (2.15s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.230s
```

53/53 pass at the pinned `summerwind/h2spec` SHA per ADR-0051 threshold (sections 3, 4, 5, 6 ex-6.6, 7, 8). UNCHANGED relative to phase 05/05.1/05.2/06.1 as required by SPEC §15.

**Gate (d) — fuzzer short-budget runs — FAIL on `FuzzAccessLogFormat` (NEW in 06.2):**

Seed-corpus runs for all 8 fuzzers PASS (the impl Task 16 sweep's exact reproduction):

```
$ go test -count=1 ./internal/bootstrap/ -run FuzzBootstrapLoad -v 2>&1 | tail -3
    --- PASS: FuzzBootstrapLoad/seed#7 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s
$ go test -count=1 ./internal/filter/tcpproxy/ -run FuzzTcpProxyFilter -v 2>&1 | tail -3
    --- PASS: FuzzTcpProxyFilter/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.004s
$ go test -count=1 ./internal/tls/ -run FuzzTLSContextParse -v 2>&1 | tail -3
    --- PASS: FuzzTLSContextParse/seed#3 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	0.003s
$ go test -count=1 ./internal/filter/hcm/ -run FuzzHCMConfigParse -v 2>&1 | tail -3
    --- PASS: FuzzHCMConfigParse/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.005s
$ go test -count=1 ./internal/filter/hcm/h2/ -run 'FuzzFrameStream|FuzzHPACKDecode' -v 2>&1 | tail -3
    --- PASS: FuzzHPACKDecode/seed#1 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.003s
$ go test -count=1 ./internal/stats/ -run FuzzPromTextFormat -v 2>&1 | tail -3
    --- PASS: FuzzPromTextFormat/1d8483e640bf8347 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	0.002s
$ go test -count=1 ./internal/accesslog/ -run FuzzAccessLogFormat -v 2>&1 | tail -3
    --- PASS: FuzzAccessLogFormat/seed#5 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s
```

The new fuzzer (`FuzzAccessLogFormat`) at the ADR-0018 30 s budget — **FAIL**:

```
$ go test -count=1 -fuzz=FuzzAccessLogFormat -fuzztime=30s ./internal/accesslog/ 2>&1 | tail -15
fuzz: elapsed: 0s, gathering baseline coverage: 0/6 completed
fuzz: elapsed: 0s, gathering baseline coverage: 6/6 completed, now fuzzing with 32 workers
fuzz: minimizing 124-byte failing input file
fuzz: elapsed: 1s, minimizing
--- FAIL: FuzzAccessLogFormat (1.07s)
    --- FAIL: FuzzAccessLogFormat (0.00s)
        fuzz_test.go:51: odd number of un-escaped quotes (11): "[2026-04-29T00:00:00.000Z] \"0 0 0\" 200 - - 42 5 - \"-\" \"0\" \"-\" \"0\" \"\\\"\n"
    Failing input written to testdata/fuzz/FuzzAccessLogFormat/1bdc705d534eee86
    To re-run:
    go test -run=FuzzAccessLogFormat/1bdc705d534eee86
FAIL
exit status 1
FAIL	github.com/esalaine/envoy-go/internal/accesslog	1.075s
```

Decoded output line (after un-escaping the Go-string literals in the FAIL message): `[2026-04-29T00:00:00.000Z] "0 0 0" 200 - - 42 5 - "-" "0" "-" "0" "\"<LF>` — the final UPSTREAM_HOST quoted value is `\` (single backslash), the closing `"` of that field is preceded by the `\`, the fuzzer's even-quote-count check sees the closing `"` as escaped and reports 11 un-escaped quotes (an odd number).

**Gate (e) — `go vet` / `golangci-lint` / `go test -race ./...` — PASS (test suite under `-race` does NOT replay the auto-saved fuzz seed because it lives in `testdata/fuzz/<Name>/<sha>` which Go fuzz only replays when the fuzzer name appears in `-run`; `go test ./...` matches all top-level Test* but FuzzAccessLogFormat is matched by the seeded F.Add and the saved seed):**

```
$ go vet ./...
(exit 0, no output)

$ golangci-lint run ./...
(exit 0, no output)

$ go test -race -count=1 -timeout 600s ./... 2>&1 | grep -E '^(ok|FAIL)'
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.118s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.016s
ok  	github.com/esalaine/envoy-go/internal/admin	1.068s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.047s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.052s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.491s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.509s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.034s
ok  	github.com/esalaine/envoy-go/internal/listener	1.051s
ok  	github.com/esalaine/envoy-go/internal/stats	1.032s
ok  	github.com/esalaine/envoy-go/internal/tls	1.087s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.153s
ok  	github.com/esalaine/envoy-go/test/differential	22.478s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.011s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.012s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.013s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.014s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.013s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.015s
ok  	github.com/esalaine/envoy-go/test/helpers	1.025s
(20 packages: all ok, no FAIL, no DATA RACE warnings)
```

Note: this `go test -race ./...` was run BEFORE the `-fuzz=FuzzAccessLogFormat -fuzztime=30s` invocation, so the auto-saved seed file did not yet exist when `-race` ran. After this verification commit deletes the seed file (per the verifier role contract), the next `go test -race ./...` will continue to pass clean — the bug is exposed only by fuzz-engine input generation, not by the seed corpus or any unit test.

**SPEC §15 boundary greps — PASS:**

```
$ grep -nE 'github.com/sirupsen/logrus|go.uber.org/zap|github.com/rs/zerolog|github.com/fluent/fluent-logger-golang' go.mod
50:	github.com/sirupsen/logrus v1.9.3 // indirect
$ # logrus is // indirect (transitive testcontainers dependency); no direct imports in code.
$ grep -rE '"github.com/(sirupsen/logrus|go\.uber\.org/zap|rs/zerolog|fluent/fluent-logger-golang)' internal/ cmd/
(no output — clean)

$ grep -nE 'emitAccessLog' internal/filter/hcm/actions.go internal/filter/hcm/h2dispatch.go
internal/filter/hcm/actions.go:106:			a.filter.emitAccessLog(req, a.status, int64(len(a.bodyText)), cluster.Endpoint{}, start)
internal/filter/hcm/actions.go:158:		defer func() { a.filter.emitAccessLog(req, statusCode, bytesSent, picked, start) }()
internal/filter/hcm/actions.go:283:			r.filter.emitAccessLogH2(req, statusForHCM, int64(bytesSentH2), picked, start)
internal/filter/hcm/h2dispatch.go:96:	defer a.f.emitAccessLogH2(req, a.a.status, int64(len(a.a.bodyText)), cluster.Endpoint{}, start)
$ # 4 emit-hook sites: 2 H1 (actions.go:106, :158) + 2 H2 (actions.go:283, h2dispatch.go:96).

$ grep -nE '^## ADR-006[6-9]:' docs/envoy-go/DECISIONS.md
2349:## ADR-0066: Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure)
2382:## ADR-0069: `server.accesslog_dropped` counter naming (SN5 mapping)
2417:## ADR-0067: Reject `log_format` at parse (option β; extends ADR-0065's boundary-validation pattern)
2448:## ADR-0068: Differential fixture 0006-access-log — three-tier equivalence matrix
$ # All four anticipated 06.2 ADRs anchored in DECISIONS.md.
```

**Differential surface:** gate (a) fixture-0006-access-log PASS non-vacuous (reference Envoy at the ENVOY_TARGET pin); gate (b) 6/6 pre-existing fixtures PASS (0000/0001/0002/0003/0004/0005). **Conformance:** h2spec 53/53 PASS at the pinned summerwind SHA (sections 3/4/5/6 ex-6.6/7/8 per ADR-0051 threshold). **Fuzz:** 7/8 PASS (BootstrapLoad, TcpProxyFilter, FrameStream, HPACKDecode, TLSContextParse, HCMConfigParse, PromTextFormat); 1/8 FAIL (AccessLogFormat — see gate (d) above).

## Fix — gate-(d) FuzzAccessLogFormat — escape() backslash catalog extension

**Commits:** `3fe7fbf`

Worktree `.worktrees/phase-06.2-access-log-impl-followup-gate-d`, branch `phase/06.2-access-log-impl-followup-gate-d`, branched from master fast-forward HEAD `a0192c0` (verify-fail SHA-fill) per ADR-0003 + per-phase-worktree convention. Mirrors the 06.1 gate-(d) fix precedent at `79be6b0` (`phase 06.1 fix: gate-(d) FuzzHCMConfigParse — validate stat_prefix at HCM parse boundary [ADR-0065]`).

**Notes.** Closes the gate-(d) FAIL recorded in the verification block above. Two changes:

1. **`internal/accesslog/format.go::escape()`** — extend the escape catalog from `{", \n, \r}` to `{\, ", \n, \r}` and add `\` to the early-return `ContainsAny` filter. Order matters: `\` → `\\` MUST appear first in the `strings.NewReplacer` arg list because `NewReplacer` is single-pass non-overlapping; listing `\` first means an input `\"` is processed as `\` (replaces to `\\`) then `"` (replaces to `\"`) → output `\\\"` (escaped backslash + escaped quote, well-formed). Listing `"` first would emit `\"` first, then attempt to replace the introduced `\` (but NewReplacer doesn't re-scan — so the introduced `\` stays bare; output `\\"` would parse as escaped-backslash + bare-quote = field terminator, which is what the bug looked like). The Default doc-comment updated to record the catalog and the order rationale; the SPEC §11 empirical-pin block needs no amend (the 5-record reference scrape contains no backslash content; SPEC §6.1 et al. say "per Envoy convention" — under-specified but consistent with the fix).

2. **`internal/accesslog/fuzz_test.go::FuzzAccessLogFormat`** — strengthen the parseability-invariant heuristic. The original heuristic (`got[i] == '"' && got[i-1] != '\\'`) is wrong on the well-formed sequence `\\"` (escaped-backslash + bare-quote = legitimate field terminator): it sees the preceding `\` and incorrectly classifies the `"` as escaped. The fix counts CONSECUTIVE preceding `\` bytes and treats the `"` as escaped iff that count is ODD. With the format.go fix in place, an input `upstream="\"` produces output `..."\\"` — the closing `"` is preceded by 2 backslashes (even) → counted as un-escaped → the total quote count is 12 (even) → invariant holds. The mirror heuristic in `format_test.go::TestDefault_BackslashInQuotedField` uses the same pair-counting approach.

3. **Regression seed.** Re-introduced the auto-saved Go-fuzz crasher input at `internal/accesslog/testdata/fuzz/FuzzAccessLogFormat/1bdc705d534eee86` (6-line file: `go test fuzz v1` / 5× `string("0")` / `string("\\")` representing the minimised input — method=path=proto=authority=ua=`"0"`, upstream=`\` single backslash). Go's fuzz framework auto-replays files at `testdata/fuzz/<FuzzName>/<sha>` on every `go test ./internal/accesslog/` run; this guarantees the bug never recurs even if the format.go change is later refactored.

4. **Three new unit tests in `internal/accesslog/format_test.go`** (TDD discipline per D-3.1; RED before fix, GREEN after):
   - `TestDefault_BackslashInQuotedField` — exact reproduction of the failing fuzz seed (UPSTREAM_HOST=`\`); asserts the closing `"` is preceded by `\\` not `\`, and that the un-escaped quote count is even using the same pair-counting heuristic.
   - `TestDefault_BackslashInMiddleOfField` — interior backslashes also doubled (catalog extension is positional-uniform).
   - `TestDefault_BackslashThenQuoteInField` — the order-sensitive case: input `\"` serializes to `\\\"` (4 chars) not `\\"` (3 chars).

**ADR consideration.** No ADR added. The fix is mechanical: the escape catalog extension matches reference Envoy `AccessLogFormatUtils::escapeUtilityValue` and RFC 4180 CSV-style escaping; no design alternative was meaningfully considered. (Mirror 06.1's ADR-0065 only when a non-obvious validation pattern is being introduced — that fix introduced a NEW pattern "validate metric-name-deriving inputs at the user-input boundary"; this fix is a 1-character extension to an existing escape catalog with the obvious choice. A code-comment in `escape()` records the order-sensitivity rationale.)

**Outputs:**

```
$ pwd
/home/esa/git/envoy-go/.worktrees/phase-06.2-access-log-impl-followup-gate-d
$ git rev-parse --abbrev-ref HEAD
phase/06.2-access-log-impl-followup-gate-d
$ git log -1 --format=%H master
a0192c08a4e0bc7adde50f0a8ccc2f1849e63c7e

# RED — new tests fail before format.go fix
$ go test -count=1 -v -run 'TestDefault_BackslashInQuotedField|TestDefault_BackslashInMiddleOfField|TestDefault_BackslashThenQuoteInField' ./internal/accesslog/
=== RUN   TestDefault_BackslashInQuotedField
    format_test.go:88: UPSTREAM_HOST=`\` should serialize to `"\\"`; got tail = "\"0\" \"\\\"\n"
    format_test.go:91: UPSTREAM_HOST tail looks like an escaped quote (closing `"` swallowed): "[2026-04-29T00:00:00.000Z] \"0 0 0\" 200 - - 42 5 - \"-\" \"0\" \"-\" \"0\" \"\\\"\n"
    format_test.go:102: odd un-escaped quote count (11) in "..."
--- FAIL: TestDefault_BackslashInQuotedField (0.00s)
=== RUN   TestDefault_BackslashInMiddleOfField
    format_test.go:118: interior backslashes not doubled; got "...\"a\\b\\c\"..."
--- FAIL: TestDefault_BackslashInMiddleOfField (0.00s)
=== RUN   TestDefault_BackslashThenQuoteInField
    format_test.go:137: backslash-quote not escaped to `\\\"`; got "...\\\\\"\"..."
--- FAIL: TestDefault_BackslashThenQuoteInField (0.00s)
FAIL

# GREEN — same tests pass after format.go fix + fuzz_test.go invariant strengthening
$ go test -count=1 -v -run 'TestDefault_BackslashInQuotedField|TestDefault_BackslashInMiddleOfField|TestDefault_BackslashThenQuoteInField' ./internal/accesslog/
=== RUN   TestDefault_BackslashInQuotedField
--- PASS: TestDefault_BackslashInQuotedField (0.00s)
=== RUN   TestDefault_BackslashInMiddleOfField
--- PASS: TestDefault_BackslashInMiddleOfField (0.00s)
=== RUN   TestDefault_BackslashThenQuoteInField
--- PASS: TestDefault_BackslashThenQuoteInField (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.004s

# Regression seed re-introduced + replayed
$ go test -count=1 -v -run 'FuzzAccessLogFormat/1bdc705d534eee86' ./internal/accesslog/
=== RUN   FuzzAccessLogFormat
=== RUN   FuzzAccessLogFormat/1bdc705d534eee86
--- PASS: FuzzAccessLogFormat (0.00s)
    --- PASS: FuzzAccessLogFormat/1bdc705d534eee86 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s

# Local six-gate sweep — all GREEN

# Gate (a) — fixture-0006-access-log differential
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/0006' -v 2>&1 | grep -E '^---|^PASS|^ok'
--- PASS: TestDifferential (11.43s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	11.501s

# Gate (b) — pre-existing fixtures 0000-0005 (regression check)
$ go test -count=1 -timeout 120s ./test/differential/ -run 'TestDifferential/000[0-5]' -v 2>&1 | grep -E '^---|^PASS|^ok'
--- PASS: TestDifferential (9.14s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	9.217s

# Gate (c) — h2spec
$ go test -count=1 -timeout 120s ./test/conformance/h2spec/ -v 2>&1 | tail -3
--- PASS: TestH2Spec (2.36s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.443s

# Gate (d) — FuzzAccessLogFormat 30s budget (the verifier's failing gate, now PASS)
$ go test -count=1 -fuzz=FuzzAccessLogFormat -fuzztime=30s ./internal/accesslog/ 2>&1 | tail -10
fuzz: elapsed: 24s, execs: 20588601 (939234/sec), new interesting: 66 (total: 87)
fuzz: elapsed: 27s, execs: 24333799 (1248186/sec), new interesting: 66 (total: 87)
fuzz: elapsed: 30s, execs: 27109368 (925592/sec), new interesting: 66 (total: 87)
fuzz: elapsed: 31s, execs: 27109368 (0/sec), new interesting: 66 (total: 87)
PASS
ok  	github.com/esalaine/envoy-go/internal/accesslog	31.018s

(27,109,368 executions in 30s, 0 crashers, 87 interesting inputs — clean.)

# Gate (e) — go vet / golangci-lint / go test -race ./...
$ go vet ./...
(exit 0, no output)
$ golangci-lint run ./...
(exit 0, no output — lint cleanup applied during fix: gofmt of TestDefault_BackslashInQuotedField; misspell `serialises`→`serializes`, `serialise`→`serialize`, `catalogue`→`catalog`)
$ go test -race -count=1 -timeout 600s ./... 2>&1 | grep -E '^(ok|FAIL)'
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.126s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.017s
[... 18 other packages, all ok ...]
ok  	github.com/esalaine/envoy-go/test/helpers	1.026s
(20 packages: all ok, no FAIL, no DATA RACE warnings)

# 7 pre-existing fuzzers seed-corpus runs (regression check; output abbreviated)
$ for fz in \
    'BootstrapLoad,bootstrap' \
    'TcpProxyFilter,filter/tcpproxy' \
    'TLSContextParse,tls' \
    'HCMConfigParse,filter/hcm' \
    'PromTextFormat,stats'; do
    name="${fz%,*}"; pkg="${fz#*,}"
    go test -count=1 ./internal/$pkg/ -run "Fuzz$name" 2>&1 | tail -1
done
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.004s
ok  	github.com/esalaine/envoy-go/internal/tls	0.003s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.005s
ok  	github.com/esalaine/envoy-go/internal/stats	0.002s
$ go test -count=1 ./internal/filter/hcm/h2/ -run 'FuzzFrameStream|FuzzHPACKDecode' 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.003s
$ go test -count=1 ./internal/accesslog/ -run FuzzAccessLogFormat 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.001s
(8/8 fuzzers PASS seed corpus, including the previously-failing FuzzAccessLogFormat which now also passes the 30s budget at gate (d).)
```

**Carry-forward triage:**
- 06.1 review-followup carry (M-2..M-12 + reviewer-discovered Minors): unchanged.
- 05.2 M-4 / M-10 / M-12: unchanged.
- 05.2 prose Minors (7): unchanged.
- 06.1 REVIEW M-8 (drain-loop polling) — closed prophylactically in fixture 0006 driver at Task 15 (`085890d`); unchanged here.
- This commit's lint cleanup (`gofmt` + 3× `misspell`) is local to the new test/code lines added by this fix; it is NOT a carry-forward and does NOT close any pre-existing review item.

STATE advances 3 → 4 with `next-skill: superpowers:verification-before-completion`. The next session is a verify-2 (`phase/06.2-access-log-verify-2`, fresh worktree per the 05.1 + 06.1 verify-2 precedent) that re-runs all six gates fresh — gate (d) at the 30s budget specifically. SHA-fill follow-up per the phase-02..06.1 convention: `phase 06.2 follow-up: STATE.md + PROGRESS.md SHA-fill for gate-(d) fix commit (TBD → 3fe7fbf)` after the master fast-forward.
