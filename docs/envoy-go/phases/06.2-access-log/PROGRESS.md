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

**Commits:** TBD — this task's commit
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
