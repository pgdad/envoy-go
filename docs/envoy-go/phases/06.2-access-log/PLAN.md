# Phase 06.2 — Access log (`internal/accesslog` package, HCM emit hooks, fixture 0006) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit and at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness check — every ADR introduced or referenced is named in the phase-done commit message), §6.1 (split gate — ~1500 LoC AND <25 tasks), §7 (differential contract), §7.5 (phase-done six-gate checklist that §3 of the SPEC specialises for 06.2); `docs/envoy-go/phases/06.2-access-log/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; ~755 lines, 16 sections, read in full); `docs/envoy-go/phases/06-observability-baseline/SPEC.md` (parent master SPEC, the cross-cutting context for the 06.1 + 06.2 split); `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (the brainstorm-close artefact at master `75a6bf9` that the 06.1 + 06.2 SPECs distil from); `docs/envoy-go/phases/06.1-stats-prometheus/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 06.1 PLAN is the structural precedent — §-numbering, heredoc-style task headers, ADR-with-first-use-commit discipline, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections, TDD-step granularity); `docs/envoy-go/phases/05.2-upstream-h2/PLAN.md` (secondary structural precedent; 15 tasks, similar in shape); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0065 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-numbering rule, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0041** silent-ignore set + amendment policy, **ADR-0045** planner-time-split discipline, **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0059** internal stats Registry architecture (the architectural-shape sibling of 06.2's ADR-0066), **ADR-0061** SN1–SN8 flattening rules (SN5 anchors 06.2's ADR-0069), **ADR-0062** stats differential-equivalence shape (the equivalence-shape sibling of 06.2's ADR-0068), **ADR-0065** boundary-validation-at-user-input pattern (the pattern 06.2's ADR-0067 extends) — the tail of the verified next-free check is ADR-0065; phase 06.2's four anticipated ADRs land at ADR-0066..ADR-0069); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## Access log field mapping` placeholder at lines ~170–174, edited at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin Decision H's empirical scrape cites); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 06.2 — D-3.7 reserves pin bumps for dedicated phases).

**Goal:** Land envoy-go's access-log subsystem — an in-tree file-sink-and-default-format-formatter package backed by an async writer with bounded-channel drop-newest backpressure — and thread access-log-emit hooks through the four request-finalization sites in HCM (H1 direct_response, H1 router action, H2 direct_response, H2 router action) so a per-request access-log line is appended to a configured file sink per the Envoy default format. Records are differentially equivalent to upstream Envoy v1.37.2 under a 5-request defined load on the per-field three-tier matrix (Tier E byte-equal cross-side, Tier F format-only, Tier S subject-emits-`-`) per SPEC §1 + §6 + §7. Concretely: a NEW `internal/accesslog` package (`Sink` interface, `Record` struct, `Default()` formatter, `AsyncFileSink` with bounded-channel drop-newest backpressure, drop-counter wiring) — `Sink` interface stable, `Default()` formatter emits the literal Envoy-default-format 15-operator line shape (8 Tier-E + 3 Tier-F + 4 Tier-S), `AsyncFileSink` async-writer goroutine drains a 4096-cap channel to an `os.File` opened `O_APPEND|O_CREAT|O_WRONLY` mode 0644, drop-newest backpressure via non-blocking `select`-with-`default` increments `server.accesslog_dropped` counter and emits a 1-second-rate-limited `log.Printf` diagnostic (per ADR-0066 anticipated; per SPEC §1 #1–#3 + §5.2); 4 emit-hook deferrals at the four request-finalization sites in `internal/filter/hcm/actions.go` and `internal/filter/hcm/h2dispatch.go` — `directResponseAction.do` (H1 direct_response), `routerAction.do` (H1 router), `h2DirectResponseAdapter.WriteH2` (H2 direct_response wrapper), `routerActionH2.doH2` (H2 router) — each adds a leading `start := time.Now()` plus a trailing `defer filter.emitAccessLog(...)` capturing the four primitives (`start`, `bytesSent`, `picked cluster.Endpoint`, `statusCode`); H2 ctx-cancel returns statusForHCM=0 → deferred emit checks the zero-status sentinel and SKIPS submission per SPEC §2.1 last bullet (per SPEC §1 #6 + §5.4); option β rejection at the bootstrap parser per Decision C — `access_log[]` parsed as a list of any length (0 → no-op; N → emit to all N sinks per request, in registration order); each entry's typed-config of type `envoy.access_loggers.file` requires a non-empty `path` AND any presence of `log_format` / `format_string` / `json_format` produces a fatal parse error `unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)`; other typed-config types (`stdout` / `tcp_grpc` / `open_telemetry` / etc.) silently-ignored per ADR-0041 amendment (per ADR-0067 anticipated; per SPEC §1 #4 + §4.2); per-record per-field three-tier equivalence matrix per ADR-0068 anticipated — 8 Tier-E (byte-equal cross-side: `:METHOD`, `:PATH`, `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)` (both `-`), `USER-AGENT`, `:AUTHORITY`), 3 Tier-F (format-only: `START_TIME` RFC3339 ms-precision UTC, `DURATION` int ms ≥ 0, `UPSTREAM_HOST` `host:port` or `-`), 4 Tier-S (subject-emits-`-`: `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID`); records paired by record index; non-listed reference-side Tier-S values silently dropped (per SPEC §1 #5 + §6 + §7.4); a NEW differential fixture `test/fixtures/0006-access-log/` — second observability-surface differential and first asserting per-record field-by-field equivalence — with 5 sequential GETs per side (paths `[/health, /api/v1/foo, /api/v1/bar, /api/v1/baz, /notfound]`, total 10 records); 2 × `direct_response` (1 × 200 `OK\n` and 1 × 404 `not found\n`) + 3 × routed (200 each through STATIC cluster `c_backend` over 3 endpoints in RR; reference uses STRICT_DNS + `host.docker.internal` per ADR-0010 + `--concurrency 1` per ADR-0028); per-side log file scraped via 25ms polling-loop (no arbitrary `time.Sleep`, hard 5s deadline) per Decision G — adopts 06.1 REVIEW M-8 prophylactically; parsed via positional-15-tuple regex; per-field tier rule applied per Decision D (per SPEC §1 #9 + §7); the empirical-format-pin scrape against `envoyproxy/envoy:v1.37.2` is filled at fixture-construction time per Decision H + SPEC §11 (replaces the `<<TBD: pinned at PLAN Task N — empirical scrape>>` placeholder in SPEC §11 + the corresponding placeholder in `BEHAVIOR_CONTRACT.md ## Access log field mapping`'s in-place edit); a NEW fuzz target `internal/accesslog.FuzzAccessLogFormat` at the 30s ADR-0018 budget per Decision J — fuzzes adversarial `Record` field values (control chars, 8-bit bytes, large strings, `\n` / `\r` / `"` in headers) into `accesslog.Default()`; asserts the formatter NEVER produces a record with embedded LF (line-terminator-corruption defence) AND quoted operators escape literal `"` to `\"` per Envoy's convention; **eighth fuzzer overall** (joins the seven from 06.1) (per SPEC §1 #10 + §14.6); FOUR new ADRs ADR-0066..ADR-0069 (re-verified at Task 1 step 1 against `DECISIONS.md` tail being ADR-0065) covering access-log architecture, log_format-rejection-at-parse, three-tier differential equivalence, and `server.accesslog_dropped` counter naming; a `BEHAVIOR_CONTRACT.md ## Access log field mapping` in-place-edit population (the placeholder at lines ~170–174 fills with the 15-operator format table from SPEC §6 + the three-tier matrix from SPEC §13.1 + the empirical-pin block from SPEC §11 + the X-ENVOY-ORIGINAL-PATH?:PATH fallback note from SPEC §6.1) at the 06.2 phase-done commit per ADR-0052 (per SPEC §13 + §4.4); STATE.md / ROADMAP.md / PROGRESS.md updates with row 06.2 → `done` AND parent row 06 → `done` AT THE SAME phase-done commit (mirroring the 05/05.1/05.2 closure pattern per parent SPEC §5). After phase 06.2, the project has proven the second half of its seventh central engineering claim: envoy-go emits behaviorally-equivalent operator-grade access-log records under a defined load — visible at a configured file sink, semantically equivalent in field shape and value to upstream Envoy v1.37.2's default-format records on the per-field three-tier matrix — without coupling to any third-party access-log dependency. The parent ROADMAP row `06` (observability-baseline) closes at THIS phase's phase-done commit; the project advances to phase 07 (filter-chain framework) at lifecycle-state 1.

**Architecture:** The 06.2 surface is the additive introduction of one new package (`internal/accesslog/`) plus two new files inside the existing `internal/filter/hcm/` package (`accesslog_emit.go`, `bytecounter.go`) plus the threading of a `[]accesslog.Sink` slice through one constructor signature (`internal/filter/hcm/config.go`'s filter-chain builder, alongside the existing 06.1 `*stats.Registry` parameter) plus boot-wiring in `cmd/envoy-go/main.go` plus a small `Cluster.Dial`/`DialH2` return-tuple expansion to surface the picked endpoint to the router-action sites (per SPEC §12 #2 — option (a)). The threading mirrors 06.1's parameter-threading discipline (the same pattern that 06.1 BRAINSTORM §3 note 6 flagged for explicit enumeration so it isn't surprise scope at PLAN time, codified in 06.1 ADR-0059); SPEC §4.2's file inventory enumerates each constructor change explicitly. Concretely: `internal/accesslog/accesslog.go` (NEW; ~40 LoC) defines `type Sink interface { Submit(*Record); Close() error }` and `type Record struct { StartTime time.Time; Method string; Path string; Protocol string; ResponseCode int; BytesSent int64; Duration time.Duration; Authority string; UserAgent string; UpstreamHost string }` — 10 plumbed fields (the 10 plumbed operators from Decision A; the 5 unplumbed operators emit literal `-` without Record fields); `internal/accesslog/format.go` (NEW; ~150 LoC) defines `func Default(r *Record) []byte` emitting the literal Envoy default-format 15-operator line (terminated with single `\n`); each plumbed operator pulls from the corresponding `Record` field; each unplumbed operator emits literal `-`; quoted operators (request-line block, `USER-AGENT`, `:AUTHORITY`, `X-FORWARDED-FOR`, `X-REQUEST-ID`, `UPSTREAM_HOST`) wrap field's value in double quotes, escaping literal `"` to `\"` per Envoy convention; `START_TIME` wrapped in `[...]`; `internal/accesslog/writer.go` (NEW; ~120 LoC) defines `type AsyncFileSink struct { ch chan *Record; f *os.File; done chan struct{}; dropped *stats.Counter; lastDropLog atomic.Int64; closeOnce sync.Once }` with constructor `NewAsyncFileSink(path string, dropped *stats.Counter) (*AsyncFileSink, error)` opening file `O_APPEND|O_CREAT|O_WRONLY` mode 0644 and starting writer goroutine; `Submit(*Record)` non-blocking-sends on `ch` (cap 4096); on full-channel increments `dropped` + emits 1-second-rate-limited diagnostic via `lastDropLog` CAS-bounded deadline; writer goroutine `for r := range ch { _, _ = f.Write(Default(r)) }` (single-consumer; `os.File.Write` atomic for sub-PAGE writes under `O_APPEND` per `man 2 write`); `Close()` `sync.Once`-guarded, closes `ch`, waits for `done`, then closes fd; `internal/accesslog/stats.go` (NEW; ~10 LoC) defines `func RegisterDroppedCounter(reg *stats.Registry) *stats.Counter { return reg.NewCounter("server.accesslog_dropped") }` per ADR-0069 (SN5 mapping → `envoy_server_accesslog_dropped`; outside the 06.1 17-name allow-list); `internal/accesslog/doc.go` (REWRITE from phase-00 stub; ~30 LoC) describes the package API, lifecycle invariant ("opened in `cmd/envoy-go/main.go` between `bootstrap.Load` and `listener.Run`; closed via `defer sink.Close()` after `listener.Shutdown` returns"), and the Drain semantics out-of-scope (Phase 08); `internal/accesslog/fuzz_test.go` (NEW; ~50 LoC) carries `FuzzAccessLogFormat` per Decision J — fuzzes adversarial `Record` field values into `accesslog.Default`; asserts no embedded LF + quote-escaping; 30s ADR-0018 budget; `internal/filter/hcm/bytecounter.go` (NEW; ~10 LoC) defines `type byteCounterWriter struct { w io.Writer; n int64 }` with `Write(p []byte) (int, error)` accumulating an `int64` running total — used to capture H1 `BYTES_SENT` for `routerAction.do`; `internal/filter/hcm/accesslog_emit.go` (NEW; ~80 LoC) defines `func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time)` (H1 path) and `func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time)` (H2 path); both construct `accesslog.Record` from primitives + request fields and `Submit` to each sink in `f.accessLog`; H2 path reads pseudo-headers (`:method`, `:path`, `:authority`); both check for the H2 ctx-cancel sentinel (statusCode == 0) and skip submission per SPEC §2.1 last bullet; `internal/filter/hcm/filter.go` (MODIFIED) `Filter` struct gains an `accessLog []accesslog.Sink` field; `internal/filter/hcm/config.go` (MODIFIED) `parseFilterWithCtx` (or its caller) accepts the `[]accesslog.Sink` slice from the bootstrap (a new parameter alongside the existing 06.1 `*stats.Registry` parameter) and stores it on the constructed `*Filter`; `internal/filter/hcm/actions.go` (MODIFIED) the four request-finalization sites add deferrals — `directResponseAction.do` adds `start := time.Now()` + `defer filter.emitAccessLog(r, statusCode, int64(len(body)), cluster.Endpoint{}, start)`; `routerAction.do` adds `start := time.Now()` + `bcw := &byteCounterWriter{w: bw}` wrapping the downstream writer + captures picked endpoint via the expanded `Cluster.Dial` return-tuple + `defer filter.emitAccessLog(r, statusCode, bcw.n, picked, start)`; `routerActionH2.doH2` adds `start := time.Now()` + bytes-sent accounting via `len(resp.Body)` (per SPEC §12 #3 option (a) — H2 `BYTES_SENT` is response body bytes only) + captures picked endpoint via the expanded `Cluster.DialH2` return-tuple + `defer filter.emitAccessLogH2(req, statusForHCM, bytesSent, picked, start)` (with the zero-statusForHCM sentinel skipping emission inside `emitAccessLogH2`); `internal/filter/hcm/h2dispatch.go` (MODIFIED) `h2DirectResponseAdapter.WriteH2` adds `start := time.Now()` + `defer adapter.f.emitAccessLogH2(req, status, int64(len(body)), cluster.Endpoint{}, start)`; `internal/cluster/cluster.go` (MODIFIED) `Cluster.Dial(ctx) (net.Conn, error)` widens to `(net.Conn, Endpoint, error)` per SPEC §12 #2 option (a) — the dial method already performs `PickEndpoint()` internally, this just surfaces the picked endpoint to the caller; the post-dial-success branch unchanged otherwise; `internal/cluster/dial_h2.go` (MODIFIED) `Cluster.DialH2(ctx) (*h2.ClientConn, error)` widens to `(*h2.ClientConn, Endpoint, error)`; `internal/bootstrap/bootstrap.go` (MODIFIED) extended to parse HCM `access_log[]` field — each entry's typed-config matched against `envoy.access_loggers.file`; `path` required + non-empty; ANY presence of `log_format` / `format_string` / `json_format` fails parse; other typed-config types silently-ignored per ADR-0041 amendment; `Bootstrap` struct gains `AccessLogConfigs []AccessLogConfig` (parsed-but-not-yet-opened tuples of `(Path string)`); `cmd/envoy-go/main.go` (MODIFIED) between `bootstrap.Load(...)` and `listenerManager.Run(...)`, allocates the `server.accesslog_dropped` counter once via `accesslog.RegisterDroppedCounter(bs.Stats)`, iterates `bootstrap.AccessLogConfigs` calling `accesslog.NewAsyncFileSink(cfg.Path, droppedCounter)` for each, threads the sink slice into `internal/filter/hcm/config.go`'s filter-chain construction (joins existing 06.1 stats Registry threading), `defer`s `sink.Close()` for each in registration order after `listener.Shutdown()` returns; `internal/stats/name.go` (MODIFIED) `helpText` map gains one entry per Decision K + SPEC §12 #5 — `"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."`; `test/fixtures/0006-access-log/` (NEW directory) carries `envoy-go.yaml` (1 listener `l_h1` binding `127.0.0.1:0` plaintext, 1 HCM with `codec_type: HTTP1` + `stat_prefix: ingress_http` + `access_log[]` with one `envoy.access_loggers.file` entry whose `path` is the runner-supplied `<t.TempDir()>/subject.log`, 1 route_config with three routes (`/health` → 200 `OK\n`, `/notfound` → 404 `not found\n`, `prefix:/api/v1/` → cluster `c_backend`), 1 STATIC cluster `c_backend` with 3 endpoints), `envoy.yaml` (reference; same shape with STRICT_DNS cluster pointing at `host.docker.internal:<backend-N-port>` + `dns_lookup_family: V4_ONLY` per ADR-0010; reference is invoked with `--concurrency 1` per ADR-0028; `access_log[].path = /tmp/envoy-access.log` bind-mounted by `testcontainers-go` `Mounts` to `<t.TempDir()>/reference.log`), `expectations.yaml` (the 5-record × 15-operator three-tier matrix table from SPEC §7.4 verbatim), `README.md` (purpose + STATIC-vs-STRICT_DNS divergence + 5-request workload shape + per-side log-file mounting + polling convention + cross-reference to BEHAVIOR_CONTRACT.md), `driver/driver.go` (5 H1 GETs sequentially with target paths; polling-loop log-file readiness 25ms/5s per Decision G; positional-15-tuple regex parsing; per-field tier-rule application per ADR-0068; `:AUTHORITY` cross-side normalization per Decision J — strip-port-and-host-replace per SPEC §12 #6), `driver/driver_test.go` (parser regex unit tests; driver wiring smoke), `backends/main.go` (small Go HTTP/1.1 server returning `backend-N:v1/<n>\n` for `GET /api/v1/<n>`; body length 17 bytes byte-identical across N ∈ {0,1,2} per SPEC §7.2's BYTES_SENT-tier-E robustness invariant); `test/differential/runner.go` (MODIFIED) blank-imports the new fixture-0006 driver package and the runner's per-fixture loop calls the driver's polling-loop hooks per the in-band pattern (per Decision F + SPEC §12 #4 — in-band like the 0004/0005 drivers); `BEHAVIOR_CONTRACT.md` is edited in place at the phase-done commit per ADR-0052 — the empty `## Access log field mapping` placeholder fills with the SPEC §13.1 contents (15-operator format + three-tier matrix + empirical-pin verbatim from SPEC §11 + X-ENVOY-ORIGINAL-PATH?:PATH fallback note); the existing `## Equivalence Matrix` row at line 18 (`Access log records | Semantically equal after field-mapping`) STAYS as-is — its text is already correct; the in-place edit makes the row's reference load-bearing per SPEC §13.2; the four ADRs ADR-0066..ADR-0069 land at first-use commit ordering per the phase-04/05.1/05.2/06.1 precedent.

**Tech Stack:**
- Go 1.23 (unchanged from 06.1; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `os`, `time`, `strings`, `bytes`, `sync`, `sync/atomic`, `io`, `fmt`, `log`, `bufio`, `regexp`, `strconv`, `errors`, `context`, `net`, `net/http` — the exhaustive set the `internal/accesslog/` package and the `internal/filter/hcm/accesslog_emit.go` + `bytecounter.go` files consume.
- **NEW: no third-party access-log library.** `go.mod` MUST NOT contain `github.com/sirupsen/logrus`, `go.uber.org/zap`, `github.com/rs/zerolog`, `github.com/fluent/fluent-logger-golang`, or any other access-log / structured-logging library import. The acceptance check at Task 16 step 4 grep-verifies the absence (per ADR-0066 + SPEC §15 acceptance bullet "No third-party access-log dependency").
- `internal/stats` (06.1's deliverable) — consumed for the `server.accesslog_dropped` counter wiring per ADR-0069. The `internal/accesslog` package's external dependency surface is limited to the Go standard library plus `github.com/esalaine/envoy-go/internal/stats` (for the drop-counter Counter type) per SPEC §15 final acceptance bullet.
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged). Phase 06.2 reads HCM `access_log[]` (proto type `envoy.config.accesslog.v3.AccessLog`) + `envoy.extensions.access_loggers.file.v3.FileAccessLog` typed-config; no proto version bump.
- `google.golang.org/protobuf` (transitively; the `Bootstrap.AccessLogConfigs` field shape is in-tree, not a proto wrapper).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0006's reference (Envoy in a Docker container) — same harness as 05.1's conformance gate consumes for h2spec; phase 06.2 does not modify `test/differential/harness.go` beyond optionally adding `Mounts` plumbing for the `/tmp/envoy-access.log` bind-mount (verified at Task 15 step 1; if `Mounts` is already plumbed for fixture-0005's stats-snapshot capture, no harness change is needed).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0006's reference image AND the empirical-format-pin source at Task 3 step 4 (the verbatim default-format scrape).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 06.2 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- **Forbidden runtime imports (D-3.2 + ADR-0066):** `github.com/sirupsen/logrus/...`, `go.uber.org/zap/...`, `github.com/rs/zerolog/...`, `github.com/fluent/fluent-logger-golang/...`, `github.com/uber-go/zap/...`, ANY structured-logging library. The boundary grep at Task 16 step 4 enforces. Test-side use is also forbidden — the fixture-0006 driver parses access-log lines with a small in-fixture regex (~10 LoC), NOT via a third-party library, so the grep applies uniformly across `_test.go` and production code.
- Test-side allowed exception: the fuzz target's seed corpus may include adversarial bytes (newlines, NULs, double-quotes, etc.) but does not import any third-party fuzzer infrastructure; it consumes Go's native `testing.F`.
- `internal/accesslog/` is a NEW package introduced in 06.2 (replacing the phase-00 placeholder `doc.go`); no pre-existing imports of it exist outside that placeholder.
- `internal/filter/hcm/` extends in place; no new imports outside the standard library + `github.com/esalaine/envoy-go/internal/accesslog` + `github.com/esalaine/envoy-go/internal/cluster` (the existing 06.1 `internal/stats` import is unchanged).
- `internal/bootstrap/`, `cmd/envoy-go/` extensions add a single import path each: `github.com/esalaine/envoy-go/internal/accesslog`. The package-import-graph stays acyclic (the boundary check is grep-verifiable: no `internal/accesslog` file imports any `internal/...` other than `internal/stats`; the accesslog package is a near-leaf).

---

## Scope check — why phase 06.2 ships as one sub-phase

Net change estimate: **~1450 LoC** broken down by component (mirroring the 06.1 PLAN's component-table convention):

- `internal/accesslog/accesslog.go` ~40 + `accesslog_test.go` ~80 = ~120
- `internal/accesslog/format.go` ~150 + `format_test.go` ~150 = ~300
- `internal/accesslog/writer.go` ~120 + `writer_test.go` ~180 = ~300
- `internal/accesslog/stats.go` ~10 + `stats_test.go` ~25 = ~35
- `internal/accesslog/fuzz_test.go` ~50
- `internal/accesslog/doc.go` ~30 (rewrite from phase-00 stub)
- `internal/filter/hcm/accesslog_emit.go` ~80 + `accesslog_emit_test.go` ~150 = ~230
- `internal/filter/hcm/bytecounter.go` ~10 + `bytecounter_test.go` ~50 = ~60
- `internal/filter/hcm/filter.go` extension (struct field + plumbing) ~5 + `filter_test.go` extension ~30 = ~35
- `internal/filter/hcm/config.go` extension (parseFilterWithCtx accepts sink-slice param) ~5 + `config_test.go` extension ~20 = ~25
- `internal/filter/hcm/actions.go` extension (3 sites: directResponse + routerAction + routerActionH2) ~25 + `actions_test.go` extension ~80 = ~105
- `internal/filter/hcm/h2dispatch.go` extension (h2DirectResponseAdapter.WriteH2 emit) ~5 + `h2dispatch_test.go` extension ~30 = ~35
- `internal/cluster/cluster.go` extension (Dial return-tuple expansion) ~5 + `cluster_test.go` extension ~30 = ~35
- `internal/cluster/dial_h2.go` extension (DialH2 return-tuple expansion) ~5 + `dial_h2_test.go` extension ~30 = ~35
- `internal/bootstrap/bootstrap.go` extension (parse access_log[]; reject log_format) ~40 + `bootstrap_test.go` extension ~80 = ~120
- `internal/stats/name.go` extension (helpText entry for accesslog_dropped) ~3 + `name_test.go` extension ~10 = ~13
- `cmd/envoy-go/main.go` extension (open sinks; thread; defer Close) ~15 + `main_test.go` extension ~30 = ~45
- `test/differential/runner.go` extension (registration) ~3
- `test/fixtures/0006-access-log/` (envoy-go.yaml ~80 + envoy.yaml ~80 + expectations.yaml ~50 + README.md ~70 + driver/driver.go ~250 + driver/driver_test.go ~80 + backends/main.go ~50) = ~660
- `docs/envoy-go/DECISIONS.md` (four ADRs) ~250
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place edit) ~80
- `docs/envoy-go/ROADMAP.md` (row updates) ~3
- `docs/envoy-go/STATE.md` (lifecycle transitions) ~5
- `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` ~150

Total estimate: **~3000 LoC** (revised after counting the fixture infrastructure correctly — fixture 0006 is on the order of fixture 0005's ~650 LoC; the unit-test pairs across `internal/accesslog/` are substantial because each TDD task lands ~1.5x test-LoC vs production-LoC per the 06.1 PLAN's ratio). Task count is **16** — well below the 25-task gate (`BOOTSTRAP_PROMPT.md` §6.1's primary signal) and within SPEC §1's anticipated 12–16 range. LoC estimate is ABOVE the soft 1500-threshold OR-leg, but comparable in magnitude to 06.1 (~1700 estimated; ~3300 actual landed) and 05.2 (~1500 estimated; ~2400 actual) which both shipped as one phase. The phase-04 / 05.1 / 05.2 / 06.1 precedent is that task-count-under-25 is the load-bearing signal; the LoC OR-leg has been exceeded in three of the four prior phases without splitting, when the surface is structurally atomic.

Phase 06.2 ships as **one** sub-phase (not split into 06.2.1 + 06.2.2 — even though STATE.md flagged that natural axis as the §6.2 split candidate) for three reasons:

1. **The split-by-surface axis (e.g. 06.2.1 = `internal/accesslog` package + bootstrap parse; 06.2.2 = HCM emit hooks + fixture 0006) creates two consecutive sub-phases with vacuous gate (a) on 06.2.1.** Per BOOTSTRAP §6.3 ("do not ship incomplete stubs that conformance tests can't exercise"), a 06.2.1 carrying only the accesslog package + bootstrap parser would have no differential fixture (gate (a) vacuous; the package can be unit-tested but the differential CONTRACT — per-record three-tier equivalence between envoy-go and Envoy — needs hot-path emit-hooks firing). Splitting also leaves the HCM `Filter` struct's `accessLog []accesslog.Sink` field unconsumed in 06.2.1 production code — dead infrastructure until 06.2.2 wires up the four emit-hooks. The four emit-hook sites are the load-bearing claim that defines this sub-phase's atomic scope (mirroring 06.1's 17-stat-emit-call-sites argument).

2. **Task count is at the SPEC's recommended low end; LoC estimate is at the OR-leg with established phase-04 / 05.1 / 05.2 / 06.1 precedent.** Per phase-04 / 05.1 / 05.2 / 06.1 precedent, task-count-under-25 is the primary signal that one phase is the right shape. 06.2's 16 tasks matches SPEC §1's expected 12–16 plus one preconditions task plus one closing sweep. The ~3000-LoC OR-leg estimate is comparable to 06.1's ~3300 actual landed; the OR-leg has been exceeded in three of four prior phases without splitting, and the structural-atomicity argument (#1 above) precludes splitting on the surface axis.

3. **Fixture 0006's per-record three-tier equivalence is the load-bearing claim that defines this sub-phase's atomic scope.** Per BOOTSTRAP §6.3 + SPEC §1 #9, the project's second observability-surface differential (and the first asserting per-record field-by-field equivalence) is what makes 06.2 atomically claimable as "envoy-go emits behaviorally-equivalent operator-grade access-log records" per SPEC §1's seventh central engineering claim's second half. Removing fixture 0006 from 06.2 would leave 06.2 as a unit-test-only sub-phase — the same process smell SPEC §1 #9 specifically targets. Conversely, removing the four emit-hook sites would leave fixture 0006 with nothing to differentially compare. The four components (accesslog package, bootstrap parser, four emit-hook sites, fixture 0006) form a coherent atomic unit.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **5000** by the end of Task 14 (i.e., before fixture 0006's driver + backends tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A ~67% miss on a carefully-bounded sub-phase is a signal the plan's shape is wrong, not just that the work is large. Mid-execution split valve: `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger (any single task's sub-steps blow up past ~10 items) stays active. The two tasks most likely to blow past 10 sub-steps are Task 3 (`internal/accesslog/format.go` — the largest single-file change after fixture-0006's driver, and includes the empirical-format-pin scrape sub-step) and Task 15 (fixture 0006 driver — orchestrates 5-request workload + polling-loop + parser regex + per-field tier-rule application). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR — the natural axis remains 06.2.1 (`accesslog` package + bootstrap parse) and 06.2.2 (HCM emit hooks + fixture 0006) per STATE.md's hint.

---

## ADRs introduced by this plan

Four ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0065** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `## ADR-0065:` at the master-`9acfc0b`-then-SPEC-commit-`4062c65` baseline; the planner re-verified at PLAN-write time that ADR-0059..ADR-0065 all landed in the 06.1 phase-done + lifecycle-state-3-fix-branch + sha-fill commits per SPEC §8 anticipation; if a mid-PLAN-authoring ADR landed since the SPEC commit, re-number 06.2 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1 — the executor checks at Task 1 step 1). Per SPEC §8, phase 06.2's four ADRs land at ADR-0066..ADR-0069 in topical order. The topic-to-ADR-number map:

- **SPEC §8 ADR-0066 anticipation** (Access-log architecture: file sink + AsyncFileSink + drop-newest backpressure) → **ADR-0066** (lands Task 2, the `internal/accesslog/accesslog.go` Sink-interface + Record-struct introduction; first ADR landed of the four; the architectural shape applies to every subsequent task in the package).
- **SPEC §8 ADR-0067 anticipation** (Reject `log_format` at parse — option β; extends ADR-0065's boundary-validation pattern) → **ADR-0067** (lands Task 7, the `internal/bootstrap/bootstrap.go` `access_log[]` parser extension; first use of the option-β rejection in production code).
- **SPEC §8 ADR-0068 anticipation** (Access-log differential equivalence shape — three-tier matrix) → **ADR-0068** (lands Task 15, the fixture-0006 driver — first use of the per-record per-field three-tier equivalence assertion shape).
- **SPEC §8 ADR-0069 anticipation** (`server.accesslog_dropped` counter naming — SN5 mapping) → **ADR-0069** (lands Task 5, the `internal/accesslog/stats.go` counter-wiring file — first use of the `server.accesslog_dropped` counter name in production code; co-anchored with the helpText-map extension in `internal/stats/name.go` per Decision K).

Note: the FIRST-USE ORDERING is Tasks 2, 5, 7, 15 — i.e. ADR-0066 first (Task 2), ADR-0069 second (Task 5), ADR-0067 third (Task 7), ADR-0068 fourth (Task 15). This produces an ADR-number-vs-commit-order sequence (0066, 0069, 0067, 0068) — non-monotonic in the second half. Per SPEC §8's explicit permission ("the planner may permute commit-time landings if that reads more naturally in PLAN.md") and per the 05.2 ADR-0055..ADR-0058 + 06.1 ADR-0059..ADR-0064 precedents (both used non-monotonic commit-time orderings), the non-monotonic mapping is correct here. The contiguous-block discipline (ADR-0066..ADR-0069 inclusive, no gaps) is preserved; topical coherence drives the in-task pairing (ADR-0069 lands at Task 5 because that's where the counter is allocated; ADR-0067 lands at Task 7 because that's where the parser rejects; ADR-0068 lands at Task 15 because that's where the differential assertions fire). The PLAN documents the mapping explicitly so the executor doesn't "fix" the ordering at execution time.

Summaries:

- **ADR-0066 — Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure).** Status: Accepted. Date: task-execution date. Doctrine: D-3.2 (no third-party-runtime-import for runtime-critical surfaces) + D-3.3 (own the canonical observation surface). Decision: a thin in-tree `internal/accesslog` package (`Sink` interface, `Record` struct, `Default` formatter, `AsyncFileSink` async-writer with bounded-channel drop-newest backpressure); **no third-party access-log library** (no logrus / zap / zerolog / fluent dependency). Lock-free hot path on submit (Go's buffered-channel non-blocking `select`-with-`default` is atomic-CAS-bounded — no mutex, no syscall — when the channel has available capacity); single-consumer writer goroutine drains channel-receive into per-record `os.File.Write`, atomic for sub-PAGE writes under `O_APPEND` per `man 2 write` on Linux (no `fsync`; OS page cache is the durability ceiling — matches Envoy). Drop-newest discipline: full-channel Submit increments `server.accesslog_dropped` counter (allocated against 06.1's `*stats.Registry`) and emits a 1-second-rate-limited `log.Printf` diagnostic; no queue-depth gauge (the gauge would force an `atomic.LoadInt64` on every submit, contrary to the lock-free hot-path discipline 06.1 ADR-0059 established). Rationale (per Decision E + the parent SPEC §4.3 cross-cutting "no third-party observability dependencies"): future Observability-family phases (gRPC ALS, OTLP) need a Sink interface to hook, not a third-party file-logger; investing in our own thin shape now is the same architectural choice 06.1 made for stats, made-for-the-same-reason. Alternatives considered: (A) `logrus` / `zap` / `zerolog` directly — rejected for future-sink-coupling (binds the in-process record model to a structured-logging library's specific shape, blocking the gRPC ALS / OTLP sinks future phases will land); (B) per-record blocking `os.File.Write` on the hot path — rejected because per-request HCM finalization should not block on disk I/O; (C) unbounded channel — rejected because OOM-on-overload is worse than drop-newest. Consequences: (a) the `internal/accesslog` package's external dependencies are limited to the Go stdlib + `internal/stats`; (b) the AsyncFileSink concurrency model documented inline in `writer.go` (Submit non-blocking; writer goroutine single-consumer; Close `sync.Once`-guarded); (c) future Observability-family phases that introduce additional sinks (ALS, OTLP) extend this package by implementing the `Sink` interface — no architectural churn needed. Lands in Task 2 (the Sink-interface + Record-struct introduction; the architectural shape applies to every subsequent task in the package). Supersedes nothing.

- **ADR-0067 — Reject `log_format` at parse (option β; extends ADR-0065's boundary-validation pattern).** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale; the rejection is a contract that future bootstrap consumers MUST observe). Decision: the bootstrap parser READS HCM `access_log[]` as a list (any length: 0 → no-op; N → emit to all N sinks per request, in registration order — no artificial 1-cap); each entry's typed-config of type `envoy.access_loggers.file` MUST have `path` (required, non-empty string); ANY presence of `log_format` / `format_string` / `json_format` produces a fatal parse error: `unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)`. Other typed-config types (`envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`) remain silently-ignored per ADR-0041 amendment (Consequences (c)). Rationale (per Decision C): boundary-validation at parse-time prevents silent wrong-result behavior — a bootstrap that says "I want JSON-formatted access logs" but receives Envoy-default-formatted logs would be a silent deviation from the operator's intent; failing-loud at parse-time forces the operator to remove the field (or the project to ship the parser surface in a future phase). Extends ADR-0065's pattern (HCM `stat_prefix` regex check at the bootstrap-input boundary) to access-log config: the same shape — validate at the user-input boundary, before the assembled internal state crosses into the runtime hot path. Alternatives considered: (A) silent-ignore — rejected for the silent-wrong-result reason above; (B) honor the `log_format` field via a command-operator parser — rejected because the parser surface is ~500 LoC and a non-goal of phase 06.2 (per SPEC §2.1 first bullet). Consequences: (a) the silently-ignored field set is amended (per ADR-0041's amendment shape, mirroring the 05.1 + 05.2 + 06.1 amendments) to add `envoy.access_loggers.stdout` / `tcp_grpc` / `open_telemetry` entries (see SPEC §9); (b) parse-fail messages on `log_format` are grep-verifiable in `bootstrap_test.go`; (c) future phases that ship the format-string parser supersede this ADR. Lands in Task 7 (the bootstrap parser extension; first use of the option-β rejection in production code). Supersedes nothing; complements ADR-0065.

- **ADR-0068 — Access-log differential equivalence shape (three-tier matrix).** Status: Accepted. Date: task-execution date. Doctrine: D-3.3 (own the canonical observation surface; the equivalence claim lives here) + D-3.6 (every phase is a green build; the differential gate (a) lands non-vacuous on 06.2's observability surface — second time on the observability surface after 06.1's fixture 0005). Decision: per-record per-field three-tier equivalence — Tier E (byte-equal cross-side, 8 operators: `:METHOD`, `:PATH`, `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)` (both `-`), `USER-AGENT`, `:AUTHORITY`), Tier F (format-only — parses to expected shape on both sides; cross-side value not asserted equal, 3 operators: `START_TIME` RFC3339 ms-precision UTC, `DURATION` int ms ≥ 0, `UPSTREAM_HOST` `host:port` or `-`), Tier S (subject must emit `-`; reference unconstrained, 4 operators: `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID`). Records paired by record index (subject record N vs. reference record N, 1-indexed); non-listed reference-side Tier-S values silently dropped by the parser. Rationale (per Decision D): byte-exact whole-record equivalence is impossible on `START_TIME` (wall-clock divergence) and `DURATION` (per-request timing divergence), and fragile on `UPSTREAM_HOST` (reference container resolves `host.docker.internal` differently than subject's STATIC IPs). Pure presence-only equivalence is too loose — it would not catch a regression where the subject emits a corrupted `RESPONSE_CODE` (e.g., `5OO` instead of `500` — a hypothetical bug from a `fmt.Stringer` mistake). The three-tier matrix surfaces equivalence field-by-field: byte-equal where possible (Tier E), format-only where deterministically possible only locally (Tier F), subject-emits-`-` where the operator is not plumbed (Tier S). Each field's tier is anchored in the operator's plumbing status (Decision A) and its determinism profile. Companion to 06.1 ADR-0062 (stats-output equivalence): both decisions answer the same question — "what is the right equivalence for an observability output that has cross-proxy non-determinism in some fields?" — for their respective surfaces; the access-log answer is per-record per-field three-tier (vs. stats's per-counter delta-equality + per-gauge snapshot-equality). Specifies that the populated `## Access log field mapping` subsection's tier table IS the "Semantically equal after field-mapping" predicate from the existing `## Equivalence Matrix` row at line 18 (per SPEC §13.2 — no row text change). Alternatives considered: (A) byte-exact whole-record with known-field-replacements (substitute START_TIME/DURATION/UPSTREAM_HOST with regex-anchored placeholders pre-comparison) — rejected because the substitution layer adds ~3× driver LoC for marginal protection over what unit tests already provide; (B) field-by-field byte-exact with no Tier-F — rejected because Tier-F operators are inherently non-deterministic across proxies; the three-tier shape is the minimum loss of equivalence-strength while preserving green-gate-per-run determinism. Consequences: (a) fixture 0006's driver implements the three-tier matrix in-band per Decision F (in-band like the 0004/0005 drivers — no generic `LogFileExpectations` Driver-interface extension); (b) the `BEHAVIOR_CONTRACT.md ## Access log field mapping` subsection's in-place edit at Task 16 carries the matrix verbatim per SPEC §13.1 (no drift between SPEC §6/§7.4 and the contract addition); (c) future fixtures (06.x, 07, etc.) that exercise additional access-log operators extend the matrix, NOT the equivalence claim shape. Lands in Task 15 (the fixture-0006 driver — first use of the per-record per-field three-tier assertion shape). Supersedes nothing.

- **ADR-0069 — `server.accesslog_dropped` counter naming (SN5 mapping).** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale where context-isolation requires it; the counter naming is a contract that future stats consumers MUST observe). Decision: the drop-newest backpressure counter (per ADR-0066) is allocated against 06.1's `*stats.Registry` via `registry.NewCounter("server.accesslog_dropped")`. Per 06.1 ADR-0061 Rule SN5 (`server.<rest>` → `envoy_server_<rest>`, no labels), the Prometheus exposition name is `envoy_server_accesslog_dropped`. **Outside the 06.1 17-name allow-list** — fixture 0005's differential explicitly ignores the metric per ADR-0062's allow-list discipline (the metric is not in the 17 names; the parser drops it). Operator-visible at `/stats/prometheus` only. The counter is allocated **once total per process** (not once per sink) — the loop in `cmd/envoy-go/main.go` allocates exactly once even with N sinks, sharing the counter across all sinks; per-sink debug visibility comes through the per-sink `path` value in the rate-limited diagnostic log line. The internal-stats `helpText` map in `internal/stats/name.go` gains one entry per Decision K + SPEC §12 #5: `"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."`. Rationale (per Decision B + Decision K): naming the counter `server.accesslog_dropped` (not `accesslog.dropped` or `http.<stat_prefix>.accesslog_dropped`) anchors it on the server scope because the counter aggregates across all configured sinks (the configuration is process-global, not per-listener or per-HCM); the SN5 mapping is an existing rule (no new flattening rule needed). Alternatives considered: (A) `accesslog.dropped` — rejected because the name violates the SN1–SN5 prefix convention (would need a new SN-rule); (B) `http.<stat_prefix>.accesslog_dropped` — rejected because the per-process aggregation surface doesn't cleanly key per stat_prefix when there are multiple HCMs; (C) per-sink `accesslog.<sink_path>.dropped` — rejected because path strings are not metric-name-safe (filesystem characters fail `internal/stats.nameRE`) and the per-sink granularity is over-shaped for a backpressure indicator. Consequences: (a) the counter name is a constant in `internal/accesslog/stats.go`'s `RegisterDroppedCounter` function; (b) the `helpText` map entry follows 06.1's discipline (per Rule SN6); (c) future sink types (ALS, OTLP) introduced in later phases may add sibling counters (e.g., `server.accesslog_als_failed`) under the same SN5 mapping; (d) the metric is OUTSIDE the 06.1 17-name fixture-0005 allow-list per ADR-0062 — that fixture's parser silently drops it, no test changes needed in the 06.1 fixture. Lands in Task 5 (alongside the package skeleton; the counter wiring lives in `internal/accesslog/stats.go`; the `helpText` map extension lives in `internal/stats/name.go`). Supersedes nothing.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0070+) in the same commit as the code it decides for. If such a decision would expand phase-06.2 scope beyond SPEC §1–§13, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6 — noting that 06.2 SPEC §1's anticipated 12–16 task range is the natural axis for re-scoping (preserve the task count by absorbing the new ADR's anchoring task into an existing task; defer if the absorption would push the absorbing task past the §6.1 secondary-trigger of ~10 sub-steps).

---

## Settled SPEC §12 deferred decisions

SPEC §12 leaves eight 06.2-scoped implementation-detail choices to the planner. This PLAN settles them so the executor does not re-litigate. Only decisions with cross-phase impact are also captured as ADRs.

1. **`accesslog.Sink` interface — Submit return type.** **Decision: option (a) — `Submit(*Record)` (no return value; drop-newest is signaled via the counter only, per SPEC §12 #1's recommendation).** Rationale: the counter is the operator-visible surface; a per-call return error would force every emit site to thread the error through the deferred-emit path for no downstream consumer. Codified in Task 2 (the `Sink` interface definition). Not separately ADRd (interface-shape choice with no cross-phase impact beyond the four emit-hook sites in this PLAN).

2. **`Cluster.PickEndpoint()` accessor surfacing for the router actions.** **Decision: option (a) — `Cluster.Dial(ctx) (net.Conn, Endpoint, error)` and `Cluster.DialH2(ctx) (*h2.ClientConn, Endpoint, error)` return-tuple expansion (per SPEC §12 #2's recommendation).** Rationale: smaller surface change; the dial methods already perform `PickEndpoint()` internally (verified at PLAN-write time in `internal/cluster/cluster.go:153` — `ep, err := c.PickEndpoint()`); the API change is contained to two return-tuple expansions. The alternative (option (b) — separate `Cluster.PickEndpoint()` then `Cluster.DialEndpoint(ep)`) would split atomically-paired operations across two call sites and risk endpoint-state-divergence between the pick and the dial under concurrent use. Codified in Task 8 (`internal/cluster/cluster.go` + `dial_h2.go` extension). Not separately ADRd (mechanical refactor with no cross-phase impact beyond the routerAction sites; ADR-0017 doctrine applies — small mechanical fixes do not require ADRs).

3. **H2 path's `BYTES_SENT` accounting.** **Decision: option (a) — sum `len(resp.Body)` directly (no header bytes counted; matches Envoy's response-body-bytes-only `BYTES_SENT` semantics, per SPEC §12 #3's recommendation).** Rationale: Envoy's `BYTES_SENT` is response body bytes only per the v1.37.2 docs (verified at PLAN-write time against Envoy's source-tree well-known stats names); summing `len(resp.Body)` directly is correct and zero-overhead. The alternative (option (b) — implement a `byteCountingStreamWriter` adapter wrapping `h2.StreamWriter.WriteData`) adds ~30 LoC for no semantic gain. Codified in Task 13 step 2 (the `routerActionH2.doH2` extension). Not separately ADRd (mechanical-correctness rule with no cross-phase impact).

4. **Fixture-0006 driver pattern: in-band assertions vs. generic `LogFileExpectations` Driver-interface extension.** **Decision: in-band (per SPEC §12 #4's recommendation, mirroring the 05.2 SPEC §10 #3 + 06.1 SPEC §12 #6 in-band recommendations).** Rationale: smaller harness surface; matches the 05.2 + 06.1 in-band precedent; the per-fixture pattern is established. The driver's `DriveSubject(ctx, addr)` and `DriveReference(ctx, addr)` methods perform the full sequence (drive 5 GETs → poll log file → parse → assert per tier rule); the runner's per-fixture loop calls a fixture-specific `AssertAccessLogEquivalence(t, subjectPath, referencePath)` helper exported from the fixture-0006 driver package. Codified in Task 15. Not separately ADRd (harness-shape choice with no cross-phase impact beyond fixture conventions).

5. **`server.accesslog_dropped` HELP-text entry.** **Decision: ADD the HELP-text entry to `internal/stats/name.go`'s `helpText` map (per SPEC §12 #5's recommendation, for symmetry with 06.1's discipline per Rule SN6).** Concrete entry: `"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."`. Codified in Task 5 step 4 (alongside the `RegisterDroppedCounter` introduction). Not separately ADRd (mechanical-extension of the existing helpText map; ADR-0069 anchors the counter's naming, and the helpText extension is a Consequences (a)-anchored Lands-in-task detail; ADR-0017 doctrine applies — small mechanical fixes do not require ADRs).

6. **`:AUTHORITY` cross-side normalization.** **Decision: strip the port and assert the host part is byte-equal (per SPEC §12 #6's recommendation).** Rationale: the host part is `127.0.0.1` on both sides — the reference container resolves `host.docker.internal` from inside the container per Docker's bridge-networking convention to a host-side address; the fixture's bind-mount on `/tmp/envoy-access.log` ensures the reference's emitted authority (the port the listener bound) is the only divergence; the subject's authority is `127.0.0.1:<ephemeral>`; both sides reduce to host-only `127.0.0.1` after the port-strip pass. Concrete normalization rule: regex-replace `:\d+$` with empty string before applying Tier-E byte-equality. Codified in Task 15 step 5 (the fixture-0006 driver's parser). Not separately ADRd (driver-shape choice with no cross-phase impact).

7. **Concrete ADR numbers for ADR-0066..ADR-0069.** Per SPEC §12 #7's deferred decision: the planner re-verifies next-free at write time. **Verified at PLAN-write time:** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0065:` at the master-`9acfc0b`-then-SPEC-commit-`4062c65` baseline. Phase 06.2's four ADRs land at ADR-0066..ADR-0069. The mapping is documented in `## ADRs introduced by this plan` above; the executor re-verifies at Task 1 step 1 in case a mid-PLAN-authoring or pre-implementation ADR has landed since this PLAN was written. The non-monotonic commit-time ordering (ADR-0066 at Task 2, ADR-0069 at Task 5, ADR-0067 at Task 7, ADR-0068 at Task 15) is documented above and per the 05.2 + 06.1 precedents.

8. **Direct_response action's `start time.Time` capture.** **Decision: capture at function entry of the action's `do` method (per SPEC §12 #8's recommendation, for uniform `DURATION` semantics across all four sites).** Rationale: matching `routerAction.do` and `routerActionH2.doH2` placement gives uniform `DURATION` semantics across all four emit sites — DURATION reflects the action's wall-clock duration, regardless of whether the action does a network round-trip (router) or an in-process synth (direct_response). The alternative (capture at HCM dispatch entry, threaded through to the action) would surface different DURATION semantics across sites (downstream-request wall-clock vs. action wall-clock) without a clear gain. Codified in Task 12 step 2 (`directResponseAction.do` modification) and Task 12 step 5 (`routerAction.do` modification). Not separately ADRd (mechanical-uniformity rule with no cross-phase impact).

Three additional 06.2-internal implementation choices (not in SPEC §12 but settled here so the executor doesn't re-litigate):

9. **Empirical-format-pin scrape execution at Task 3.** **Decision: the empirical default-format scrape against reference Envoy v1.37.2 is performed AT TASK 3 (the `Default` formatter implementation task), NOT at Task 15 (the fixture construction task).** Rationale: SPEC §11 + SPEC §15 both demand the verbatim Envoy default-format string lands in the SPEC §11 placeholder block (currently `<<TBD: pinned at PLAN Task N — empirical scrape>>`) AND in `BEHAVIOR_CONTRACT.md ## Access log field mapping`'s in-place edit. The scrape's output is what the formatter implements (the operator order, the literal delimiters); deferring the scrape until Task 15 would mean Task 3 implements the formatter from the SPEC §6 catalog without empirical confirmation, then Task 15 might find a discrepancy (e.g., a delimiter difference) and force a Task 3 redo. Doing the scrape FIRST (at Task 3 step 4) anchors the formatter's correctness before the unit-test test cases are written. The SPEC §11 placeholder fill lands at the same commit as the formatter code; the BEHAVIOR_CONTRACT in-place edit lands at the phase-done commit per ADR-0052. Codified in Task 3 step 4. Not separately ADRd (procedural choice with no cross-phase impact).

10. **`AsyncFileSink.Close()` idempotency mechanism.** **Decision: `sync.Once`-guarded `closeOnce sync.Once` field + `f.closeOnce.Do(func(){ close(f.ch); <-f.done; _ = f.f.Close() })`.** Rationale: Go's `close(ch)` panics on double-close; `sync.Once` is the cleanest idempotency mechanism (matches the `connWithGauge.Close()` pattern in 06.1's `internal/cluster/cluster.go` per ADR-0063 Consequences). The alternative (atomic.Bool flag + CAS) is functionally equivalent but reads less idiomatically. Codified in Task 4 step 3. Not separately ADRd (mechanical idiom-choice with no cross-phase impact).

11. **`FuzzAccessLogFormat` fuzzer scope.** **Decision: fuzz adversarial `Record` field values (`StartTime`/`Method`/`Path`/`Protocol`/`Authority`/`UserAgent`/`UpstreamHost` strings — including control chars, 8-bit bytes, large strings, `\n`/`\r`/`"`/NUL bytes; integer fields `ResponseCode`/`BytesSent`/`Duration` from typed `Add`/`Set` patterns) into `accesslog.Default(record)`; assert (i) the formatter NEVER produces a record with embedded LF (the line terminator is `\n`; embedded LFs would corrupt the record stream by appearing as record boundaries), (ii) quoted operators escape literal `"` to `\"` per Envoy's convention, (iii) the formatter never panics.** Rationale: per Decision J + SPEC §1 #10, embedded-LF and quote-escaping bugs are the most likely class of bug in the writer; the line-stream invariant is load-bearing for fixture-0006's positional regex parser (an embedded LF would split a record into two records and skew the parser). The fuzzer's seed corpus includes adversarial entries: newlines in headers, backslashes, double-quotes, NUL bytes, control characters (0x00..0x1f range), 8-bit bytes (0x80..0xff range), very long strings (>1KiB). Per SPEC §14.6 + Decision J's "30s ADR-0018 budget"; the fuzzer is the eighth fuzzer overall (joining the seven from 06.1). Codified in Task 6. Not separately ADRd (per ADR-0042 precedent that fuzzers do not require their own ADR; ADR-0018 governs the budget).

---

## Phase-06.1 + 05.2 REVIEW carry-forward resolution matrix

SPEC §10 dispositions the carry-forwards from prior phases. Phase-06.2 disposition matrix:

| Phase-prior finding | Triage | Landing task / rationale |
|---|---|---|
| 05.2 M-4 (`readClientPreface` not ctx-aware in `internal/filter/hcm/h2/conn.go`) | DEFERRED — out of 06.2 (unchanged from 06.1's continuing-deferral) | H2 connection hardening, not access-log. Target-phase candidates: dedicated H2-hardening sub-phase / phase 07 / upstream-robustness family. Phase 06.2 does NOT touch `conn.go`. |
| 05.2 M-10 (`SETTINGS_TIMEOUT` absent in `internal/filter/hcm/h2/client.go`) | DEFERRED — out of 06.2 (unchanged) | Same reasoning as M-4. |
| 05.2 M-12 (`closedStreams` map unbounded in `internal/filter/hcm/h2/conn.go`) | DEFERRED — out of 06.2 (unchanged) | Long-lived-conn memory growth, not access-log. Target-phase candidate: upstream-robustness family. |
| 05.2 prose Minors (7 items) | DEFERRED — out of 06.2 (unchanged) | Various; carry-forward state unchanged. |
| 06.1 12 Minors (M-2..M-12 + post-phase-done reviewer-discovered) | OUT OF 06.2; lands in 06.1 review-followup batch | Separate post-phase-done branch (the established 05.1/05.2 review-followup branch pattern); NOT 06.2's responsibility. |
| **06.1 REVIEW M-8** ("hardcoded 200ms drain may flake on slow CI; recommend polling loop instead") | **ADOPTED PROPHYLACTICALLY by 06.2 design** (does NOT close M-8 against fixture 0005) | **Task 15.** Fixture 0006's driver implements the polling-loop pattern natively (Decision G drain discipline: 25ms-interval polling on log-file line count `≥ 5`, 5s hard deadline; no `time.Sleep(200 * time.Millisecond)` arbitrary sleep). This establishes the pattern for new fixtures going forward; 06.2 is the first non-vacuous instance of this pattern. M-8 itself stays open against fixture 0005 (its actual close lands in a 06.1 review-followup batch). |

The disposition table is faithful to SPEC §10's per-finding triage; the §10.2 deferred items do NOT land an ADR in 06.2 because the deferral itself is the SPEC's record (per ADR-0017 doctrine that "small mechanical fixes do not require ADRs"). The PROGRESS Task 15 entry records the M-8 prophylactic-adoption alongside the standard task entry; the PROGRESS Task 16 entry records the carry-forward triage table verbatim.

---

## Spec-review advisory responses

The SPEC's brainstorming session ran the `spec-document-reviewer` subagent loop and reached APPROVED at master `4062c65`; a follow-up reviewer-fixes commit at `7bbf4a2` filled ADR-0041 references and an H2 unit-test footnote. The SPEC at `7bbf4a2` carries no outstanding advisory items at PLAN-write time. Three planner-time advisory items, structurally akin to the 06.1 PLAN's "spec-review advisory responses" but originating from the planner's reading of the SPEC during PLAN authoring:

i. **The `Cluster.Dial`/`DialH2` return-tuple expansion** — current code is `Dial(ctx) (net.Conn, error)` and `DialH2(ctx) (*h2.ClientConn, error)`. SPEC §12 #2 anticipates an option-(a) expansion to `(net.Conn, Endpoint, error)` and `(*h2.ClientConn, Endpoint, error)`. Per `## Settled SPEC §12 deferred decisions` #2, the PLAN codifies option (a) at Task 8. The propagation surface is the existing two call-sites (`routerAction.do` and `routerActionH2.doH2`) plus their unit-test files plus any other transitive consumers; the planner verified at PLAN-write time via `grep -nR 'Dial(ctx\|DialH2(ctx' internal/ cmd/envoy-go/ --include='*.go'` that the consumer set is bounded (returns matches in `internal/filter/hcm/actions.go` and tests). Recorded here so the executor doesn't re-litigate at Task 8 execution time.

ii. **The `internal/accesslog/doc.go` rewrite** — current code is a phase-00 stub ("The real implementation lands in phase 06"). Task 2 step 8 rewrites it to the full package documentation. This is a direct rewrite (not an extension), per the same precedent 06.1 PLAN Task 2 set for `internal/stats/doc.go`. Recorded here so a reviewer reading the PLAN doesn't flag the rewrite as a SPEC-vs-PLAN divergence (SPEC §4.1 explicitly authorizes the rewrite).

iii. **The `BEHAVIOR_CONTRACT.md ## Access log field mapping` placeholder line range** — SPEC §13 cites lines 170–174 for the placeholder; the planner verified at PLAN-write time via `grep -n '^## Access log field mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md` that the heading is at line 170 and the placeholder body extends through line 174 (`grep -nA4 '^## Access log field mapping$' ...` confirms). Task 16 step 1's edit replaces lines 170–174 with the populated subsection per SPEC §13.1. If the line range shifts before Task 16 execution time (e.g., a mid-PLAN-authoring BEHAVIOR_CONTRACT edit), the executor re-locates via the same `grep` pattern at Task 16 step 1; the edit's target is the heading + placeholder, not the absolute line range.

---

## Execution preconditions (the executor checks at Task 1)

Before starting Task 2 (the first code-changing task), the executor MUST verify each of the following preconditions on the implementation worktree. If any precondition fails, the executor STOPS and follows the precondition's "if fails" guidance.

1. **Worktree branch.** The current branch is `phase/06.2-access-log-impl` per ADR-0003 + the per-phase-worktree convention. Verify with `git rev-parse --abbrev-ref HEAD`. If on a different branch, the executor invoked the wrong worktree — exit and start a new session per BOOTSTRAP §1.

2. **Branch base.** The branch was created from master tip after the PLAN.md commit's SHA-fill (i.e., master HEAD is the SHA-fill follow-up commit's SHA, not the TBD-bearing commit). Verify with `git log --oneline master | head -3` — expect the top three to be the PLAN SHA-fill, the PLAN commit, and the SPEC commit's SHA-fill in some order. If the branch base is older, the executor missed the PLAN commit's master fast-forward — exit and start a new session.

3. **PROGRESS.md absence.** `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` does NOT exist at branch creation. Task 1 creates it. If it exists, an earlier session left state behind — exit and invoke `superpowers:systematic-debugging`.

4. **Docker daemon.** `docker version` reports both client and server (the differential harness needs the daemon for fixture 0006's reference container). If the daemon is unavailable, fixture 0006's gate (a) is unrunnable; the executor still proceeds through Tasks 1–14 (which don't need the daemon), but Task 15's gate (a) sweep blocks until the daemon is up.

5. **Go toolchain.** `go version` reports `go1.23` or newer. The `go.mod` directive is `go 1.23.0`. If older, install Go 1.23+ before proceeding.

6. **golangci-lint.** `golangci-lint version` reports `1.64.8` (per ADR-0009). If older or newer, install the pinned version.

7. **Pre-existing fixtures green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005' -v` reports all PASS. If any fixture fails, a regression was introduced before 06.2 — exit and invoke `superpowers:systematic-debugging` on the regression.

8. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0065:`. If the tail is `> ADR-0065`, a mid-PLAN-authoring ADR landed; the executor re-numbers the four 06.2 ADRs sequentially from `tail + 1` and updates every task's ADR reference before starting Task 2. If the tail is `< ADR-0065`, an ADR was lost — exit and invoke `superpowers:systematic-debugging`.

9. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/SPEC.md` returns `4062c65` (the SPEC drafted commit) or `7bbf4a2` (the spec-reviewer-fixes follow-up — the current SPEC HEAD). If a newer SPEC commit lands mid-PLAN-authoring, the executor reads the new SPEC and reconciles tasks.

10. **`internal/accesslog/` is empty.** `ls internal/accesslog/` reports only `doc.go` (the phase-00 placeholder). If it contains additional files, an earlier session left state behind — exit and invoke `superpowers:systematic-debugging`.

11. **`internal/filter/hcm/` carries the four target action sites.** `grep -nE 'directResponseAction\) do|routerAction\) do|routerActionH2\) doH2|h2DirectResponseAdapter\) WriteH2' internal/filter/hcm/` returns at least four matches (one per emit-hook target site). The planner verified at PLAN-write time these are at `actions.go:95` (`directResponseAction.do`), `actions.go:128` (`routerAction.do`), `actions.go:222` (`routerActionH2.doH2`), `h2dispatch.go:89` (`h2DirectResponseAdapter.WriteH2`). If any site is missing, the upstream code drifted — exit and invoke `superpowers:systematic-debugging`.

12. **BEHAVIOR_CONTRACT placeholder present.** `grep -nA1 '^## Access log field mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns the heading at line 170 + the placeholder `_to be filled per-phase as needed._` at line ~172. If absent, BEHAVIOR_CONTRACT drifted — exit and invoke `superpowers:systematic-debugging`.

If all 12 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target.

**Precondition:** worktree exists at `phase/06.2-access-log-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up.
**Artifact:** `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (new file).
**Acceptance:** all 12 preconditions report green; PROGRESS.md preamble entry committed.
**Verification command:** `git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/06.2-access-log-impl
git log -1 --format=%H                                                # expect: same SHA as docs/envoy-go/STATE.md last-commit field (the PLAN.md SHA-fill commit)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005' -v
                                                                       # expect: every fixture PASS (pre-existing fixtures still green per gate (b))
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1                  # expect: ## ADR-0065:
git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/SPEC.md
                                                                       # expect: 4062c65 or 7bbf4a2 (the spec-reviewer-fixes follow-up); if newer, follow precondition 9 guidance
ls internal/accesslog/                                                # expect: only doc.go
grep -nE 'directResponseAction\) do|routerAction\) do|routerActionH2\) doH2|h2DirectResponseAdapter\) WriteH2' internal/filter/hcm/
                                                                       # expect: at least four matches (one per target site)
grep -nA1 '^## Access log field mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: heading + placeholder body present
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/06.2-access-log/PROGRESS.md`**

```markdown
# Phase 06.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 12 preconditions per PLAN §"Execution preconditions"; phase-06.1 close confirmed present in HEAD; SPEC at <SPEC SHA>; ADR tail at 0065 (next-free 0066); internal/accesslog/ contains only doc.go (the package implementation lands at Task 2+).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/06.2-access-log/SPEC.md
<verbatim>
$ ls internal/accesslog/
<verbatim — should report 'doc.go' only>
\`\`\`
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/06.2-access-log/PROGRESS.md
git commit -m "phase 06.2: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (per the phase-02/03/04/05.1/05.2/06.1 SHA-fill convention: a follow-up tiny commit `phase 06.2: PROGRESS SHA-fill for Task 1` lands the SHA).

*Anchored: SPEC §3, §4.4 (PROGRESS lifecycle), §15 (precondition acceptance bullet).*

---

## Task 2: `internal/accesslog/accesslog.go` — Sink interface + Record struct + doc.go rewrite [ADR-0066]

**Files:**
- Modify: `internal/accesslog/doc.go` (rewrite from phase-00 stub)
- Create: `internal/accesslog/accesslog.go`
- Create: `internal/accesslog/accesslog_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0066)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 2 entry)

The Sink interface + Record struct pair is the foundational primitive every subsequent task in the package consumes. ADR-0066 (Access-log architecture) lands at this task per the topical-co-landing decision in `## ADRs introduced by this plan` above.

**Precondition:** Task 1 done; `internal/accesslog/` contains only `doc.go`.
**Artifact:** `internal/accesslog/accesslog.go` (new); `internal/accesslog/accesslog_test.go` (new); `internal/accesslog/doc.go` (rewritten); `docs/envoy-go/DECISIONS.md` (ADR-0066 appended).
**Acceptance:** `go test ./internal/accesslog/ -count=1 -v` passes; `Sink` interface + `Record` struct compile and match the SPEC §4.1 + §5.5 contract; ADR-0066 appears in `DECISIONS.md` with full Context/Decision/Consequences sections per the ADR-0001 template.
**Verification command:** `go test ./internal/accesslog/ -count=1 -v && grep -nE '^## ADR-0066:' docs/envoy-go/DECISIONS.md`.

- [ ] **Step 1: Write the failing test for Sink interface + Record struct shape (in `accesslog_test.go`)**

```go
package accesslog

import (
	"testing"
	"time"
)

func TestRecord_AllFieldsZeroValueWellDefined(t *testing.T) {
	var r Record
	if !r.StartTime.IsZero() { t.Errorf("StartTime zero-value not zero: %v", r.StartTime) }
	if r.Method != "" { t.Errorf("Method zero-value not empty: %q", r.Method) }
	if r.Path != "" { t.Errorf("Path zero-value not empty: %q", r.Path) }
	if r.Protocol != "" { t.Errorf("Protocol zero-value not empty: %q", r.Protocol) }
	if r.ResponseCode != 0 { t.Errorf("ResponseCode zero-value not 0: %d", r.ResponseCode) }
	if r.BytesSent != 0 { t.Errorf("BytesSent zero-value not 0: %d", r.BytesSent) }
	if r.Duration != 0 { t.Errorf("Duration zero-value not 0: %v", r.Duration) }
	if r.Authority != "" { t.Errorf("Authority zero-value not empty: %q", r.Authority) }
	if r.UserAgent != "" { t.Errorf("UserAgent zero-value not empty: %q", r.UserAgent) }
	if r.UpstreamHost != "" { t.Errorf("UpstreamHost zero-value not empty: %q", r.UpstreamHost) }
}

func TestRecord_PopulatedShape(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 34, 56, 789000000, time.UTC)
	r := Record{
		StartTime: now, Method: "GET", Path: "/health", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 3, Duration: 5 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "10.0.0.1:8080",
	}
	if r.ResponseCode != 200 { t.Errorf("ResponseCode = %d, want 200", r.ResponseCode) }
	if r.Duration != 5*time.Millisecond { t.Errorf("Duration = %v, want 5ms", r.Duration) }
}

// captureSink is a test double for the Sink interface; used by Tasks 10/12/13.
type captureSink struct{ recs []*Record }
func (s *captureSink) Submit(r *Record)  { s.recs = append(s.recs, r) }
func (s *captureSink) Close() error      { return nil }

func TestSink_InterfaceImplementation(t *testing.T) {
	var s Sink = &captureSink{}
	r := &Record{Method: "GET", Path: "/x", ResponseCode: 200}
	s.Submit(r)
	if err := s.Close(); err != nil { t.Errorf("Close() error: %v", err) }
	cs := s.(*captureSink)
	if len(cs.recs) != 1 { t.Fatalf("captured %d records, want 1", len(cs.recs)) }
	if cs.recs[0].Method != "GET" { t.Errorf("captured Method = %q, want GET", cs.recs[0].Method) }
}
```

Run: `go test ./internal/accesslog/ -count=1 -v`
Expected: FAIL — `Sink`, `Record` undefined.

- [ ] **Step 2: Implement `accesslog.go` to make the tests pass**

```go
// Package accesslog provides envoy-go's access-log subsystem: an in-tree file-sink
// and Envoy-default-format formatter, plus an async-writer with bounded-channel
// drop-newest backpressure. Per ADR-0066, the package is a thin in-tree primitive
// with no third-party access-log dependency.
//
// Lifecycle: Sinks are opened in cmd/envoy-go/main.go between bootstrap.Load and
// listener.Run; threaded into the HCM filter chain via internal/filter/hcm/config.go;
// closed via defer sink.Close() after listener.Shutdown returns. SIGTERM-while-pending
// drain semantics are Phase 08's deliverable.
package accesslog

import "time"

// Sink is an access-log destination. Implementations include AsyncFileSink (the
// only sink type in 06.2; future phases may add ALS / OTLP). Submit is non-blocking
// (drop-newest backpressure on full channel; see writer.go); Close is idempotent
// and threadsafe (sync.Once-guarded; see writer.go).
type Sink interface {
	Submit(r *Record)
	Close() error
}

// Record is the per-request primitives populated by HCM at finalization-time and
// consumed by the Default formatter. Per Decision A (option B partial-with-`-`)
// the 10 fields below cover the 10 plumbed operators; the 5 unplumbed operators
// (RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
// X-FORWARDED-FOR, X-REQUEST-ID) are emitted as the literal `-` by the formatter
// without needing Record fields.
type Record struct {
	StartTime    time.Time
	Method       string
	Path         string
	Protocol     string
	ResponseCode int
	BytesSent    int64
	Duration     time.Duration
	Authority    string
	UserAgent    string
	UpstreamHost string
}
```

Run: `go test ./internal/accesslog/ -count=1 -v`
Expected: PASS.

- [ ] **Step 3: Append ADR-0066 to `DECISIONS.md`** with the full Context / Decision / Consequences sections per the ADR-0001 template — copy the summary from `## ADRs introduced by this plan` above into the proper ADR shape (Status, Date, Doctrine, Context, Decision, Alternatives Considered, Consequences, Lands-in-task: Task 2). Re-verify next-free at this step: `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` — if `> ADR-0065`, re-number all four 06.2 ADRs sequentially.

- [ ] **Step 4: Rewrite `internal/accesslog/doc.go`** from the phase-00 stub:

```go
// Package accesslog provides envoy-go's access-log subsystem.
//
// The package is documented inline in accesslog.go's package comment. See
// ADR-0066 for the architectural decision to ship a thin in-tree shape with
// no third-party access-log dependency.
//
// Sinks are opened by cmd/envoy-go/main.go between bootstrap.Load and
// listener.Run, threaded through the HCM filter chain, and closed via
// defer sink.Close() after listener.Shutdown returns. SIGTERM-while-pending
// drain semantics are Phase 08's deliverable.
package accesslog
```

- [ ] **Step 5: Append Task 2 entry to PROGRESS.md.**
- [ ] **Step 6: Commit** with subject `phase 06.2: internal/accesslog/{accesslog.go,doc.go} — Sink interface + Record struct [ADR-0066]`.

*Anchored: SPEC §1 #1, §4.1 (accesslog.go + doc.go entries), §5 (architecture), §8 (ADR-0066 anticipation).*

---

## Task 3: `internal/accesslog/format.go` — Default formatter + empirical-format-pin scrape

**Files:**
- Create: `internal/accesslog/format.go`
- Create: `internal/accesslog/format_test.go`
- Modify: `docs/envoy-go/phases/06.2-access-log/SPEC.md` (fill the §11 `<<TBD: pinned at PLAN Task N — empirical scrape>>` placeholder with the verbatim 5-record scrape from reference Envoy v1.37.2 — per `## Settled SPEC §12 deferred decisions` #9)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 3 entry; quote the empirical scrape verbatim)

The Default formatter implements the 15-operator default-format line per SPEC §6 + §11. The empirical-format-pin scrape executes at this task per `## Settled SPEC §12 deferred decisions` #9 (anchoring the formatter's correctness before unit tests are written).

**Precondition:** Task 2 done; `internal/accesslog/accesslog.go` defines `Sink`, `Record`.
**Artifact:** `internal/accesslog/format.go` (new); `internal/accesslog/format_test.go` (new); SPEC §11 placeholder filled.
**Acceptance:** `Default(record)` emits the literal Envoy default-format 15-operator line shape with all six escape rules + the SPEC §11 placeholder is filled with the verbatim 5-record empirical scrape.
**Verification command:** `go test ./internal/accesslog/ -count=1 -run TestDefault -v && ! grep -F 'TBD: pinned at PLAN Task N' docs/envoy-go/phases/06.2-access-log/SPEC.md`.

- [ ] **Step 1: Write the failing test for `Default()` happy-path + edge cases (in `format_test.go`)**

```go
package accesslog

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDefault_HappyPath_HCMDirect(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 12, 34, 56, 789000000, time.UTC),
		Method: "GET", Path: "/health", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 3, Duration: 5 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "",  // direct_response → emit literal "-"
	}
	got := Default(rec)
	if !bytes.HasSuffix(got, []byte("\n")) { t.Errorf("not LF-terminated: %q", got) }
	if bytes.Count(got, []byte("\n")) != 1 { t.Errorf("embedded LF: %q", got) }
	s := string(got)
	for _, want := range []string{
		`[2026-04-29T12:34:56.789Z]`, `"GET /health HTTP/1.1"`,
		` 200 - - 3 5 - "-" "Go-http-client/1.1" "-" "127.0.0.1:10000" "-"`,
	} {
		if !strings.Contains(s, want) { t.Errorf("Default missing %q in %q", want, s) }
	}
}

func TestDefault_RoutedPath_UpstreamHostFormatted(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/api/v1/foo", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 17, Duration: 12 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "10.0.0.1:8080",
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"10.0.0.1:8080"`) { t.Errorf("upstream host missing: %q", s) }
}

func TestDefault_QuoteEscaping(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: `/x"y`, Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 0, Duration: 0,
		Authority: `host"with-quote`, UserAgent: `agent"with-quote`,
	}
	s := string(Default(rec))
	if strings.Contains(s, `"agent"with-quote"`) { t.Errorf("unescaped quote in UA: %q", s) }
	if !strings.Contains(s, `"agent\"with-quote"`) { t.Errorf("UA quote not escaped: %q", s) }
	if !strings.Contains(s, `/x\"y`) { t.Errorf("path quote not escaped: %q", s) }
}

func TestDefault_NeverEmbedsLF(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/x\ny", Protocol: "HTTP/1.1",
		ResponseCode: 200, Authority: "h\nx", UserAgent: "ua\ny",
	}
	got := Default(rec)
	// Trim trailing LF; the rest must contain zero LFs.
	body := bytes.TrimSuffix(got, []byte{'\n'})
	if bytes.IndexByte(body, '\n') >= 0 { t.Errorf("embedded LF in body: %q", got) }
}

func TestDefault_EmptyFieldsEmitDash(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1",
		ResponseCode: 200,  // empty UserAgent, empty Authority, empty UpstreamHost → all emit `-`
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"-"`) { t.Errorf("empty fields not emitting `-`: %q", s) }
}

func TestDefault_StartTimeFormat_RFC3339Ms(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 12, 34, 56, 789012345, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
	}
	s := string(Default(rec))
	if !strings.HasPrefix(s, `[2026-04-29T12:34:56.789Z]`) {
		t.Errorf("START_TIME format wrong; got prefix %q", s[:30])
	}
}

func TestDefault_DurationMillisecondsRoundedDown(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
		Duration: 12_999_999 * time.Nanosecond, // 12.999999ms → 12
	}
	s := string(Default(rec))
	if !strings.Contains(s, " 12 ") { t.Errorf("duration not rounded down to 12: %q", s) }
}
```

Run: `go test ./internal/accesslog/ -count=1 -run TestDefault -v`
Expected: FAIL — `Default` undefined.

- [ ] **Step 2: Implement `format.go`** to make the tests pass:

```go
package accesslog

import (
	"bytes"
	"strconv"
	"strings"
)

// Default formats a Record per the Envoy v1.37.2 default access-log format
// (per ADR-0066 + SPEC §6 + §11 empirical pin). The 15 operators in identical
// positions on every record:
//
//   [%START_TIME%] "%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%" %RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION% %RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% "%REQ(X-FORWARDED-FOR)%" "%REQ(USER-AGENT)%" "%REQ(X-REQUEST-ID)%" "%REQ(:AUTHORITY)%" "%UPSTREAM_HOST%"
//
// Per Decision A's option-B partial coverage, the 5 unplumbed operators
// (RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
// X-FORWARDED-FOR, X-REQUEST-ID) emit the literal `-` (Envoy missing-value
// convention). Quoted operators escape literal `"` to `\"` per Envoy convention.
// The line is terminated with a single `\n`; embedded LFs in any field are
// stripped (replaced with `\n` literal escape) so the line-stream invariant
// load-bearing for the fixture-0006 parser holds (per SPEC §1 #10 + Decision J).
func Default(r *Record) []byte {
	var b bytes.Buffer
	b.Grow(256)
	// 1. [START_TIME]
	b.WriteByte('[')
	b.WriteString(r.StartTime.UTC().Format("2006-01-02T15:04:05.000Z"))
	b.WriteByte(']')
	// 2-4. "METHOD PATH PROTOCOL"
	b.WriteString(` "`)
	b.WriteString(escape(r.Method))
	b.WriteByte(' ')
	b.WriteString(escape(orDash(r.Path)))
	b.WriteByte(' ')
	b.WriteString(escape(r.Protocol))
	b.WriteByte('"')
	// 5. RESPONSE_CODE
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(r.ResponseCode))
	// 6-7. RESPONSE_FLAGS, BYTES_RECEIVED — both literal `-` (Tier S)
	b.WriteString(" - -")
	// 8. BYTES_SENT
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(r.BytesSent, 10))
	// 9. DURATION (int ms ≥ 0; rounded down)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(int64(r.Duration/1e6), 10))
	// 10. RESP(X-ENVOY-UPSTREAM-SERVICE-TIME) — literal `-` (Tier E both `-`)
	b.WriteString(` -`)
	// 11. "X-FORWARDED-FOR" — literal `"-"` (Tier S)
	b.WriteString(` "-"`)
	// 12. "USER-AGENT"
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.UserAgent)))
	b.WriteByte('"')
	// 13. "X-REQUEST-ID" — literal `"-"` (Tier S)
	b.WriteString(` "-"`)
	// 14. ":AUTHORITY"
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.Authority)))
	b.WriteByte('"')
	// 15. "UPSTREAM_HOST"
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.UpstreamHost)))
	b.WriteByte('"')
	b.WriteByte('\n')
	return b.Bytes()
}

// escape replaces literal `"` with `\"` and embedded LF with literal `\n` per
// Envoy convention. The two replacements compose; backslashes themselves are
// not escaped (Envoy doesn't escape backslash in the default format).
func escape(s string) string {
	if !strings.ContainsAny(s, "\"\n\r") {
		return s
	}
	r := strings.NewReplacer(`"`, `\"`, "\n", `\n`, "\r", `\r`)
	return r.Replace(s)
}

// orDash returns "-" when the string is empty (per Envoy missing-value convention).
// Used for fields where the formatter emits a bare token (no quotes).
func orDash(s string) string {
	if s == "" { return "-" }
	return s
}

// orEmptyDash returns "-" when the string is empty; used inside quoted operators.
// The quoted operator wraps `"-"` for empty values (matching Envoy's emission).
func orEmptyDash(s string) string {
	if s == "" { return "-" }
	return s
}
```

Run: `go test ./internal/accesslog/ -count=1 -run TestDefault -v`
Expected: PASS.

- [ ] **Step 3: Run the empirical-format-pin scrape against reference Envoy v1.37.2** per Decision H + SPEC §11. Boot reference Envoy with the fixture-0006 reference bootstrap (which Task 15 will commit; for the scrape, hand-craft a minimal bootstrap mirroring SPEC §7's workload — 1 listener, 1 HCM with `access_log[].path = /tmp/envoy-access.log`, 3 STRICT_DNS endpoints to host-side echo backends bound at known ports, `dns_lookup_family: V4_ONLY` per ADR-0010, `--concurrency 1` per ADR-0028) at `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Drive the 5-request workload `[/health, /api/v1/foo, /api/v1/bar, /api/v1/baz, /notfound]`. Capture the literal 5 lines from `/tmp/envoy-access.log` (bind-mounted to a host path via `docker run -v ...`). Paste the verbatim 5 lines into `docs/envoy-go/phases/06.2-access-log/SPEC.md` §11 — replacing the `<<TBD: pinned at PLAN Task N — empirical scrape>>` placeholder with the captured block, framed as:

```
Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log
under the 5-request workload from §7.2; reference image v1.37.2 at
ENVOY_TARGET.md SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd;
captured 2026-04-29 by phase 06.2 PLAN Task 3 step 3):

<line 1: GET /health → 200 direct_response>
<line 2: GET /api/v1/foo → 200 routed>
<line 3: GET /api/v1/bar → 200 routed>
<line 4: GET /api/v1/baz → 200 routed>
<line 5: GET /notfound → 404 direct_response>
```

Verify via `! grep -F 'TBD: pinned at PLAN Task N' docs/envoy-go/phases/06.2-access-log/SPEC.md` (returns no match — placeholder filled).

- [ ] **Step 4: Verify operator order matches the formatter implementation.** For each captured line, parse positionally via the same regex shape Task 15's driver will use. Confirm: 15 operators in order; literal `[`, `]`, `"`, space delimiters as the formatter emits; the 5 unplumbed operators on the reference side may carry real values (Tier S — reference unconstrained), but the order matches. If a delimiter or operator-position discrepancy surfaces, the formatter is wrong — fix `format.go` before proceeding. The §11 placeholder fill is the ground truth.

- [ ] **Step 5: Append Task 3 entry to PROGRESS.md** (quoting the empirical scrape verbatim).
- [ ] **Step 6: Commit** with subject `phase 06.2: internal/accesslog/format.go — Default formatter + SPEC §11 empirical-pin fill`.

*Anchored: SPEC §1 #1, §4.1 (format.go), §6 (15-operator catalog), §11 (empirical pin), §15 (acceptance bullet on the empirical-pin fill).*

---

## Task 4: `internal/accesslog/writer.go` — AsyncFileSink + drop-newest backpressure

**Files:**
- Create: `internal/accesslog/writer.go`
- Create: `internal/accesslog/writer_test.go`
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 4 entry)

The async writer is the package's hot-path-execution surface. Per Decision B + ADR-0066, the channel is bounded at 4096; on full-channel, drop-newest fires (counter Inc + rate-limited diagnostic). `Close()` is `sync.Once`-guarded per `## Settled SPEC §12 deferred decisions` #10.

**Precondition:** Tasks 2–3 done; `accesslog.Sink` and `accesslog.Default` are exported.
**Artifact:** `internal/accesslog/writer.go` (new); `internal/accesslog/writer_test.go` (new).
**Acceptance:** `go test -race -count=1 ./internal/accesslog/ -run TestAsyncFileSink -v` passes (happy path + race + drop-newest + rate-limit + Close-drain).
**Verification command:** `go test -race -count=1 ./internal/accesslog/ -run TestAsyncFileSink -v`.

- [ ] **Step 1: Write the failing tests** (in `writer_test.go`) — happy path; race-clean concurrent Submit; drop-newest backpressure; rate-limited diagnostic; Close-drain semantics. Use a stub `*stats.Counter` (allocate via a fresh `*stats.Registry` test-helper).

```go
package accesslog

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

func newTestRegistryAndCounter(t *testing.T) (*stats.Registry, *stats.Counter) {
	t.Helper()
	reg := stats.NewRegistry()
	c := reg.NewCounter("test.dropped")
	return reg, c
}

func TestAsyncFileSink_HappyPath_NRecordsLandNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subject.log")
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(path, c)
	if err != nil { t.Fatalf("NewAsyncFileSink: %v", err) }
	for i := 0; i < 5; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x",
			Protocol: "HTTP/1.1", ResponseCode: 200, BytesSent: 3})
	}
	if err := s.Close(); err != nil { t.Fatalf("Close: %v", err) }
	f, err := os.Open(path)
	if err != nil { t.Fatalf("Open: %v", err) }
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() { count++ }
	if count != 5 { t.Errorf("file has %d lines, want 5", count) }
	if c.Load() != 0 { t.Errorf("dropped counter = %d, want 0", c.Load()) }
}

func TestAsyncFileSink_ConcurrentSubmit_RaceClean(t *testing.T) {
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(filepath.Join(dir, "subject.log"), c)
	if err != nil { t.Fatalf("NewAsyncFileSink: %v", err) }
	const G, N = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < G; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < N; j++ {
				s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x",
					Protocol: "HTTP/1.1", ResponseCode: 200})
			}
		}()
	}
	wg.Wait()
	if err := s.Close(); err != nil { t.Fatalf("Close: %v", err) }
}

func TestAsyncFileSink_DropNewest_FullChannelIncrementsCounter(t *testing.T) {
	// Build a sink with a tiny channel by exposing a test-only constructor that
	// takes channel capacity. Implement NewAsyncFileSinkWithCapacity for this
	// purpose (test-only-friendly variant that production code does not call).
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := newAsyncFileSinkWithCapacity(filepath.Join(dir, "subject.log"), c, 1)
	if err != nil { t.Fatalf("NewAsyncFileSink: %v", err) }
	// Block the writer goroutine via a synthetic stall: fill the 1-cap channel
	// with one record, then submit several more rapidly. The writer is
	// single-consumer; under load it eventually drains, but the burst sees drops.
	rec := &Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200}
	for i := 0; i < 100; i++ { s.Submit(rec) }
	_ = s.Close()
	if c.Load() == 0 { t.Errorf("expected at least one drop; counter = 0") }
}

func TestAsyncFileSink_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(filepath.Join(dir, "x.log"), c)
	if err != nil { t.Fatal(err) }
	if err := s.Close(); err != nil { t.Errorf("first Close: %v", err) }
	if err := s.Close(); err != nil { t.Errorf("second Close: %v", err) }  // must not panic
}

func TestAsyncFileSink_Close_DrainsPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(path, c)
	if err != nil { t.Fatal(err) }
	for i := 0; i < 50; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200})
	}
	if err := s.Close(); err != nil { t.Fatal(err) }
	stat, _ := os.Stat(path)
	if stat.Size() == 0 { t.Errorf("file empty after Close; expected drained records") }
}
```

Run: `go test -race ./internal/accesslog/ -run TestAsyncFileSink -v`
Expected: FAIL — `NewAsyncFileSink`, `newAsyncFileSinkWithCapacity` undefined.

- [ ] **Step 2: Implement `writer.go`** with the AsyncFileSink + drop-newest discipline.

```go
package accesslog

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

const defaultChannelCapacity = 4096
const dropLogIntervalNanos = int64(time.Second)

// AsyncFileSink writes Default-formatted records to a file in append mode via a
// bounded-channel writer goroutine. On full channel the new record is dropped
// and the dropped counter (per ADR-0069 — server.accesslog_dropped) is Inc'd;
// a rate-limited diagnostic is emitted at most once per second.
type AsyncFileSink struct {
	ch          chan *Record
	f           *os.File
	done        chan struct{}
	dropped     *stats.Counter
	lastDropLog atomic.Int64
	closeOnce   sync.Once
	path        string
}

// NewAsyncFileSink opens path with O_APPEND|O_CREAT|O_WRONLY mode 0644 and
// starts a writer goroutine. Per ADR-0066 the channel is bounded at 4096
// records; on full channel the new record is dropped (drop-newest discipline).
func NewAsyncFileSink(path string, dropped *stats.Counter) (*AsyncFileSink, error) {
	return newAsyncFileSinkWithCapacity(path, dropped, defaultChannelCapacity)
}

// newAsyncFileSinkWithCapacity is the test-friendly variant; production callers
// use NewAsyncFileSink (capacity 4096).
func newAsyncFileSinkWithCapacity(path string, dropped *stats.Counter, cap int) (*AsyncFileSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil { return nil, err }
	s := &AsyncFileSink{
		ch:      make(chan *Record, cap),
		f:       f,
		done:    make(chan struct{}),
		dropped: dropped,
		path:    path,
	}
	go s.run()
	return s, nil
}

// Submit non-blocking-sends r on the channel. On full-channel the record is
// dropped, the counter Inc'd, and at most one diagnostic emitted per second.
func (s *AsyncFileSink) Submit(r *Record) {
	select {
	case s.ch <- r:
	default:
		s.dropped.Inc()
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("accesslog: channel full, dropping record (path=%s)", s.path)
		}
	}
}

// Close closes the channel, waits for the writer goroutine to drain, then closes
// the file descriptor. Idempotent and threadsafe via sync.Once.
func (s *AsyncFileSink) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.ch)
		<-s.done
		err = s.f.Close()
	})
	return err
}

// run is the writer goroutine: drain channel-receives into per-record file
// writes. Per `man 2 write` on Linux, O_APPEND writes are atomic for sub-PAGE
// (default 4 KiB) writes. The default-format line is well under 4 KiB; the
// per-record write is therefore atomic against any other process appending.
func (s *AsyncFileSink) run() {
	defer close(s.done)
	for r := range s.ch {
		if _, err := s.f.Write(Default(r)); err != nil {
			// Log-and-continue: a write error doesn't stop the writer; the next
			// Submit may succeed (e.g., disk full → user frees space).
			log.Printf("accesslog: file write error (path=%s): %v", s.path, err)
		}
	}
}
```

Run: `go test -race ./internal/accesslog/ -run TestAsyncFileSink -v`
Expected: PASS.

- [ ] **Step 3: Append Task 4 entry to PROGRESS.md.**
- [ ] **Step 4: Commit** with subject `phase 06.2: internal/accesslog/writer.go — AsyncFileSink + drop-newest backpressure`.

*Anchored: SPEC §1 #3, §4.1 (writer.go), §5.2 (concurrency model), §5.5 (read path), §14.1 (writer unit tests).*

---

## Task 5: `internal/accesslog/stats.go` + `internal/stats/name.go` helpText extension [ADR-0069]

**Files:**
- Create: `internal/accesslog/stats.go`
- Create: `internal/accesslog/stats_test.go`
- Modify: `internal/stats/name.go` (add `envoy_server_accesslog_dropped` helpText entry)
- Modify: `internal/stats/name_test.go` (assert the new helpText entry)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0069)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 5 entry)

The `RegisterDroppedCounter` helper anchors the counter name `server.accesslog_dropped` (per ADR-0069). The helpText extension lands here per Decision K + `## Settled SPEC §12 deferred decisions` #5 (symmetry with 06.1 Rule SN6).

**Precondition:** Tasks 2–4 done.
**Artifact:** `internal/accesslog/stats.go` (new); test pair; `internal/stats/name.go` extended with one helpText entry; ADR-0069 appended.
**Acceptance:** `go test ./internal/accesslog/ -count=1 -v && go test ./internal/stats/ -count=1 -v` passes; the helpText entry is grep-verifiable.
**Verification command:** `go test ./internal/accesslog/ ./internal/stats/ -count=1 && grep -nE 'envoy_server_accesslog_dropped' internal/stats/name.go && grep -nE '^## ADR-0069:' docs/envoy-go/DECISIONS.md`.

- [ ] **Step 1: Write the failing test for `RegisterDroppedCounter` (in `stats_test.go`)**

```go
package accesslog

import (
	"testing"
	"github.com/esalaine/envoy-go/internal/stats"
)

func TestRegisterDroppedCounter_Name(t *testing.T) {
	reg := stats.NewRegistry()
	c := RegisterDroppedCounter(reg)
	if c == nil { t.Fatal("RegisterDroppedCounter returned nil") }
	if c.Name() != "server.accesslog_dropped" {
		t.Errorf("counter name = %q, want server.accesslog_dropped", c.Name())
	}
}

func TestRegisterDroppedCounter_FlattensToPromName(t *testing.T) {
	reg := stats.NewRegistry()
	_ = RegisterDroppedCounter(reg)
	// Confirm the SN5 mapping (server.<rest> → envoy_server_<rest>) yields
	// the expected Prometheus name; reuse internal/stats's flatten helper.
	// This is a smoke against ADR-0069's "Outside the 06.1 17-name allow-list"
	// claim — the metric appears in the Registry and flattens correctly.
	var names []string
	reg.Walk(func(m stats.Metric) { names = append(names, m.Name()) })
	if len(names) != 1 || names[0] != "server.accesslog_dropped" {
		t.Errorf("Registry contents = %v, want [server.accesslog_dropped]", names)
	}
}
```

Run: `go test ./internal/accesslog/ -count=1 -run TestRegisterDroppedCounter -v`
Expected: FAIL — `RegisterDroppedCounter` undefined.

- [ ] **Step 2: Implement `internal/accesslog/stats.go`**

```go
package accesslog

import "github.com/esalaine/envoy-go/internal/stats"

// RegisterDroppedCounter allocates the `server.accesslog_dropped` counter on
// reg per ADR-0069. The counter is allocated once per process (not once per
// sink) — multiple sinks share the same counter; per-sink debug visibility is
// through the rate-limited diagnostic log line in writer.go's Submit.
//
// Per 06.1 Rule SN5 (server.<rest> → envoy_server_<rest>, no labels), the
// Prometheus name is envoy_server_accesslog_dropped. Outside the 06.1 17-name
// allow-list — fixture 0005's differential ignores the metric per ADR-0062.
// Operator-visible at /stats/prometheus only.
func RegisterDroppedCounter(reg *stats.Registry) *stats.Counter {
	return reg.NewCounter("server.accesslog_dropped")
}
```

- [ ] **Step 3: Extend `internal/stats/name.go`'s `helpText` map** with one entry:

```go
// (in helpText map literal):
"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks).",
```

Per Decision K + `## Settled SPEC §12 deferred decisions` #5, this lands here for symmetry with 06.1's discipline (per Rule SN6 — HELP-text is best-effort English). Add a corresponding test case in `internal/stats/name_test.go` asserting `helpText["envoy_server_accesslog_dropped"]` returns the documented text.

- [ ] **Step 4: Append ADR-0069 to `DECISIONS.md`** with full Context / Decision / Alternatives / Consequences sections per the ADR-0001 template (copy the summary from `## ADRs introduced by this plan`).

- [ ] **Step 5: Append Task 5 entry to PROGRESS.md.**
- [ ] **Step 6: Commit** with subject `phase 06.2: internal/accesslog/stats.go + helpText extension [ADR-0069]`.

*Anchored: SPEC §1 #3, §4.1 (stats.go), §5.6 (counter wiring), §8 (ADR-0069 anticipation), §12 #5 (helpText decision).*

---

## Task 6: `internal/accesslog/fuzz_test.go` — `FuzzAccessLogFormat` (eighth fuzzer)

**Files:**
- Create: `internal/accesslog/fuzz_test.go`
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 6 entry)

The eighth fuzzer overall (joins the seven from 06.1) per Decision J + SPEC §1 #10 + §14.6. Asserts the line-stream invariant (no embedded LF) and the quote-escaping discipline. 30s ADR-0018 budget.

**Precondition:** Tasks 2–5 done.
**Artifact:** `internal/accesslog/fuzz_test.go` (new; ~50 LoC).
**Acceptance:** `go test -fuzz=FuzzAccessLogFormat -fuzztime=30s ./internal/accesslog/` runs clean (no panics, no assertion failures, no embedded LFs).
**Verification command:** `go test -count=1 ./internal/accesslog/ -run FuzzAccessLogFormat -v` (the seed corpus runs in unit-test mode — no `-fuzz=` needed for CI).

- [ ] **Step 1: Implement `fuzz_test.go`**

```go
package accesslog

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func FuzzAccessLogFormat(f *testing.F) {
	// Seed with adversarial values: embedded LF, embedded quote, NUL bytes,
	// 8-bit characters, large strings.
	f.Add("GET", "/x", "HTTP/1.1", "host", "ua", "10.0.0.1:80")
	f.Add("\nGET", "/x\ny", "HTTP/1.1", "h\nx", "ua\n", "10.0.0.1:80")
	f.Add("GET", `/x"y`, "HTTP/1.1", `h"x`, `ua"y`, `host"port`)
	f.Add("\x00GET", "/\x00", "HTTP/1.1", "\x00", "\x00", "\x00")
	f.Add("GET", strings.Repeat("a", 2048), "HTTP/1.1", "h", "ua", "h:p")
	f.Add("\xff\x80\x81", "/\x90\x91", "HTTP/1.1", "h", "ua", "h:p")

	f.Fuzz(func(t *testing.T, method, path, proto, authority, ua, upstream string) {
		rec := &Record{
			StartTime:    time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
			Method:       method,
			Path:         path,
			Protocol:     proto,
			ResponseCode: 200,
			BytesSent:    42,
			Duration:     5 * time.Millisecond,
			Authority:    authority,
			UserAgent:    ua,
			UpstreamHost: upstream,
		}
		// (i) The formatter must not panic.
		var got []byte
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Default panicked: %v", r)
				}
			}()
			got = Default(rec)
		}()
		// (ii) The formatter MUST NOT produce a record with embedded LF beyond
		// the trailing line terminator. An embedded LF would corrupt the
		// record stream by appearing as a record boundary in the file.
		body := bytes.TrimSuffix(got, []byte{'\n'})
		if bytes.IndexByte(body, '\n') >= 0 {
			t.Fatalf("embedded LF in record body: %q (input method=%q path=%q)", got, method, path)
		}
		// (iii) Quoted operators escape literal `"` to `\"`. Approximate check:
		// count un-escaped quotes in the produced output. The format has 6
		// quote chars from the format-string itself (open/close pairs around
		// the request-line block, USER-AGENT, X-FORWARDED-FOR, X-REQUEST-ID,
		// AUTHORITY, UPSTREAM_HOST). A literal " in a field would push the
		// count to an odd number after pairing. Use a simple invariant: every
		// un-backslash-escaped " must be matched by another un-escaped ".
		quoteCount := 0
		for i := 0; i < len(got); i++ {
			if got[i] == '"' && (i == 0 || got[i-1] != '\\') {
				quoteCount++
			}
		}
		if quoteCount%2 != 0 {
			t.Fatalf("odd number of un-escaped quotes (%d): %q", quoteCount, got)
		}
	})
}
```

Run: `go test -count=1 -run FuzzAccessLogFormat ./internal/accesslog/`
Expected: PASS (the seed corpus alone passes; full fuzz at 30s comes at Task 16's gate (d) sweep).

- [ ] **Step 2: Append Task 6 entry to PROGRESS.md.**
- [ ] **Step 3: Commit** with subject `phase 06.2: internal/accesslog/fuzz_test.go — FuzzAccessLogFormat (eighth fuzzer)`.

*Anchored: SPEC §1 #10, §4.1 (fuzz_test.go), §14.6 (fuzzers gate (d)).*

---

## Task 7: `internal/bootstrap/bootstrap.go` — parse `access_log[]` + reject `log_format` [ADR-0067]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` (extend HCM parse to handle `access_log[]`)
- Modify: `internal/bootstrap/bootstrap_test.go` (parse-success / parse-fail / silent-ignore tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0067; amend ADR-0041 silently-ignored set)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 7 entry)

The bootstrap parser learns to read the HCM `access_log[]` field. Per Decision C / ADR-0067, `log_format` / `format_string` / `json_format` presence is rejected at parse-time (option β). Other typed-config types are silently-ignored per the ADR-0041 amendment.

**Precondition:** Tasks 2–6 done; the `internal/accesslog` package compiles.
**Artifact:** `internal/bootstrap/bootstrap.go` extended; test file extended; ADR-0067 appended; ADR-0041 amended.
**Acceptance:** parse-success on 0/1/N file-type entries; parse-fail with the verbatim error message on `log_format` / `format_string` / `json_format`; parse-fail on `path` empty/absent; silent-ignore on non-file typed-config types; the `Bootstrap` struct gains an `AccessLogConfigs []AccessLogConfig` field whose `Path` is read from each entry's `path`.
**Verification command:** `go test -count=1 ./internal/bootstrap/ -v` passes.

- [ ] **Step 1: Inspect the current `internal/bootstrap/bootstrap.go`** to locate the HCM-parse path and the `Bootstrap` struct. Confirm:

```bash
grep -nE 'type Bootstrap struct|func.*Load|access_log' internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
```

If the `Bootstrap` struct does not currently expose its HCM nested-config to test code, the test extension may need a small accessor; settle the shape by inspection at this step.

- [ ] **Step 2: Define `AccessLogConfig` + extend `Bootstrap`**

```go
// AccessLogConfig is the parsed-but-not-yet-opened representation of one
// envoy.access_loggers.file entry from HCM access_log[]. The sink itself is
// constructed in cmd/envoy-go/main.go after Load returns; this struct carries
// only the parse-time data.
type AccessLogConfig struct {
	Path string
}

// (in Bootstrap struct)
type Bootstrap struct {
	// ... existing fields ...
	AccessLogConfigs []AccessLogConfig
}
```

- [ ] **Step 3: Write failing tests for parse behavior** (in `bootstrap_test.go`). Sample tests:

```go
func TestBootstrap_AccessLog_FileType_PathRequired(t *testing.T) {
	bs, err := Load("testdata/06.2/access-log-file-with-path.yaml")
	if err != nil { t.Fatalf("expected parse-success, got: %v", err) }
	if len(bs.AccessLogConfigs) != 1 { t.Fatalf("got %d configs, want 1", len(bs.AccessLogConfigs)) }
	if bs.AccessLogConfigs[0].Path != "/tmp/envoy-access.log" {
		t.Errorf("Path = %q, want /tmp/envoy-access.log", bs.AccessLogConfigs[0].Path)
	}
}

func TestBootstrap_AccessLog_RejectLogFormat(t *testing.T) {
	_, err := Load("testdata/06.2/access-log-with-log-format.yaml")
	if err == nil { t.Fatal("expected parse-fail on log_format presence") }
	if !strings.Contains(err.Error(), "unsupported config: access_log[].log_format") {
		t.Errorf("error = %q, want substring 'unsupported config: access_log[].log_format'", err)
	}
}

func TestBootstrap_AccessLog_RejectJSONFormat(t *testing.T) { /* same shape, json_format */ }
func TestBootstrap_AccessLog_RejectFormatString(t *testing.T) { /* same shape, format_string */ }

func TestBootstrap_AccessLog_PathEmptyRejects(t *testing.T) {
	_, err := Load("testdata/06.2/access-log-empty-path.yaml")
	if err == nil { t.Fatal("expected parse-fail on empty path") }
}

func TestBootstrap_AccessLog_StdoutSilentlyIgnored(t *testing.T) {
	bs, err := Load("testdata/06.2/access-log-stdout-silent.yaml")
	if err != nil { t.Fatalf("expected parse-success (silent-ignore), got: %v", err) }
	if len(bs.AccessLogConfigs) != 0 { t.Errorf("expected zero AccessLogConfigs (stdout ignored), got %d", len(bs.AccessLogConfigs)) }
}

func TestBootstrap_AccessLog_NoEntriesIsValid(t *testing.T) {
	bs, err := Load("testdata/06.2/access-log-empty.yaml")
	if err != nil { t.Fatal(err) }
	if len(bs.AccessLogConfigs) != 0 { t.Errorf("expected 0 configs, got %d", len(bs.AccessLogConfigs)) }
}

func TestBootstrap_AccessLog_TwoFileEntries(t *testing.T) {
	bs, err := Load("testdata/06.2/access-log-two-files.yaml")
	if err != nil { t.Fatal(err) }
	if len(bs.AccessLogConfigs) != 2 { t.Errorf("expected 2 configs, got %d", len(bs.AccessLogConfigs)) }
}
```

Create the testdata YAML fixtures under `internal/bootstrap/testdata/06.2/` with the exact bootstrap shapes described. Each must declare a minimal valid bootstrap (1 listener + 1 HCM filter + 1 cluster) plus the targeted access-log shape.

- [ ] **Step 4: Implement the parse logic** in `bootstrap.go`. Walk each HCM filter's `access_log[]`; for each entry:
  - Type-switch the typed_config against `envoy.config.accesslog.v3.AccessLog`'s typed_config:
    - `envoy.extensions.access_loggers.file.v3.FileAccessLog` (`type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog`) — parse: read `path` (required, non-empty); reject any `access_log_format` / `log_format` / `format_string` / `json_format` field with the verbatim error message; append an `AccessLogConfig{Path: path}` to `bs.AccessLogConfigs`.
    - Any other type — silently ignored (no error, no append).
  - The error message form: `fmt.Errorf("bootstrap: unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)")`.

Run: `go test -count=1 ./internal/bootstrap/ -v`
Expected: PASS.

- [ ] **Step 5: Append ADR-0067 to `DECISIONS.md`** with full Context / Decision / Alternatives / Consequences sections (copy from `## ADRs introduced by this plan`).

- [ ] **Step 6: Amend ADR-0041's "silently-ignored set" Consequences** by appending one bulleted sub-section (per the established 05.1 + 05.2 + 06.1 amendment pattern):

```
**06.2 amendment** (per ADR-0067): the silently-ignored set is extended to include:
- `envoy.access_loggers.stdout` (typed_config of HCM `access_log[]` entries)
- `envoy.access_loggers.tcp_grpc` (gRPC ALS)
- `envoy.access_loggers.open_telemetry` (OTLP)
- HCM `access_log[].filter` field (per-record predicate filter)
- HCM `access_log_options`
- Listener-scope `access_log[]`
- Cluster-scope `access_log[]`

Rejected explicitly (NOT silently-ignored — fatal parse error per ADR-0067):
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.log_format`
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.format_string`
- `envoy.extensions.access_loggers.file.v3.FileAccessLog.json_format`
```

- [ ] **Step 7: Append Task 7 entry to PROGRESS.md.**
- [ ] **Step 8: Commit** with subject `phase 06.2: internal/bootstrap parse access_log[]; reject log_format [ADR-0067; ADR-0041 amend]`.

*Anchored: SPEC §1 #4, §4.2 (bootstrap.go extension), §8 (ADR-0067 anticipation), §9 (silent-ignore amendments).*

---

## Task 8: `Cluster.Dial` / `DialH2` return-tuple expansion (surface picked endpoint)

**Files:**
- Modify: `internal/cluster/cluster.go` (`Dial` returns `(net.Conn, Endpoint, error)`)
- Modify: `internal/cluster/dial_h2.go` (`DialH2` returns `(*h2.ClientConn, Endpoint, error)`)
- Modify: `internal/cluster/cluster_test.go` (assert the new return shape)
- Modify: `internal/cluster/dial_h2_test.go` (assert the new return shape)
- Modify: `internal/filter/hcm/actions.go` (existing call sites consume the new return tuple — both `routerAction.do` and `routerActionH2.doH2`; uses receive-but-discard pattern at this task — full consumption lands at Tasks 12/13)
- Modify: `internal/filter/hcm/actions_test.go` (callers updated for the new tuple)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 8 entry)

Per `## Settled SPEC §12 deferred decisions` #2 (option (a)). Both dial methods already perform `PickEndpoint()` internally; this task surfaces the picked endpoint to the caller for `UPSTREAM_HOST` plumbing.

**Precondition:** Tasks 2–7 done.
**Artifact:** `Dial` and `DialH2` widened return tuples; existing callers updated.
**Acceptance:** `go test -count=1 ./internal/cluster/ ./internal/filter/hcm/ -v` passes; the new return tuple is grep-verifiable.
**Verification command:** `go test -count=1 ./internal/cluster/ ./internal/filter/hcm/ -v && grep -nE 'func .*\) Dial\(.*\) \(net.Conn, Endpoint, error\)|func .*\) DialH2\(.*\) \(\*h2.ClientConn, Endpoint, error\)' internal/cluster/`.

- [ ] **Step 1: Modify `Dial`** — capture the existing local `ep` variable into the return tuple:

```go
func (c *Cluster) Dial(ctx context.Context) (net.Conn, Endpoint, error) {
	if err := ctx.Err(); err != nil { return nil, Endpoint{}, err }
	ep, err := c.PickEndpoint()
	if err != nil { return nil, Endpoint{}, err }
	// ... existing dial body, returning (final-conn, ep, nil) on success
	// or (nil, Endpoint{}, err) on any error path
	return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, ep, nil
}
```

- [ ] **Step 2: Modify `DialH2`** — same shape, returning `(cc, ep, nil)` on success.

- [ ] **Step 3: Update the existing `routerAction.do` and `routerActionH2.doH2` call sites** in `internal/filter/hcm/actions.go`. At this task they use a receive-but-discard pattern:

```go
upstream, _, err := a.cluster.Dial(ctx)  // _ is the Endpoint; consumed at Task 12
```

Tasks 12–13 will replace `_` with `picked` and pass it to `emitAccessLog`. This task ONLY widens the API and updates discarding callers so the build stays green.

- [ ] **Step 4: Update tests** — every existing `cluster_test.go` / `dial_h2_test.go` / `actions_test.go` test that calls `Dial`/`DialH2` is updated for the new return tuple. The endpoint return value is asserted in at least one new test case per dial method:

```go
func TestCluster_Dial_ReturnsPickedEndpoint(t *testing.T) {
	c := buildTestCluster(...)  // existing helper
	conn, ep, err := c.Dial(ctx)
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	if ep.Host == "" { t.Errorf("ep.Host = %q, want non-empty", ep.Host) }
}
```

Run: `go test -count=1 ./internal/cluster/ ./internal/filter/hcm/ -v`
Expected: PASS.

- [ ] **Step 5: Append Task 8 entry to PROGRESS.md** + Commit with subject `phase 06.2: Cluster.Dial/DialH2 return-tuple expansion (surface picked endpoint)`.

*Anchored: SPEC §12 #2 (deferred decision), `## Settled SPEC §12 deferred decisions` #2.*

---

## Task 9: `internal/filter/hcm/bytecounter.go` — byteCounterWriter

**Files:**
- Create: `internal/filter/hcm/bytecounter.go`
- Create: `internal/filter/hcm/bytecounter_test.go`
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 9 entry)

The H1 router's `BYTES_SENT` plumbing wraps the downstream `bufio.Writer` in a counting `io.Writer`. ~10 LoC.

**Precondition:** Tasks 2–8 done.
**Artifact:** `internal/filter/hcm/bytecounter.go` (new); test pair.
**Acceptance:** `go test -count=1 ./internal/filter/hcm/ -run TestByteCounterWriter -v` passes.
**Verification command:** `go test -count=1 ./internal/filter/hcm/ -run TestByteCounterWriter -v`.

- [ ] **Step 1: Write the failing tests (in `bytecounter_test.go`)**

```go
package hcm

import (
	"bytes"
	"errors"
	"testing"
)

func TestByteCounterWriter_AccumulatesBytesWritten(t *testing.T) {
	var buf bytes.Buffer
	bcw := &byteCounterWriter{w: &buf}
	for _, p := range [][]byte{[]byte("hello "), []byte("world"), []byte("!")} {
		n, err := bcw.Write(p)
		if err != nil || n != len(p) { t.Errorf("Write(%q) = (%d, %v), want (%d, nil)", p, n, err, len(p)) }
	}
	if bcw.n != 12 { t.Errorf("bcw.n = %d, want 12", bcw.n) }
}

type shortWriter struct{ limit int }
func (sw *shortWriter) Write(p []byte) (int, error) {
	if len(p) > sw.limit { return sw.limit, errors.New("short") }
	return len(p), nil
}

func TestByteCounterWriter_ShortWriteAccountsActualBytes(t *testing.T) {
	bcw := &byteCounterWriter{w: &shortWriter{limit: 3}}
	n, err := bcw.Write([]byte("hello"))
	if err == nil { t.Fatal("expected error") }
	if n != 3 { t.Errorf("n = %d, want 3", n) }
	if bcw.n != 3 { t.Errorf("bcw.n = %d, want 3", bcw.n) }
}
```

Run: `go test -count=1 -run TestByteCounterWriter ./internal/filter/hcm/ -v`
Expected: FAIL — `byteCounterWriter` undefined.

- [ ] **Step 2: Implement `bytecounter.go`**

```go
package hcm

import "io"

// byteCounterWriter wraps an io.Writer to maintain a running int64 total of
// bytes written. Used by routerAction.do to capture BYTES_SENT for the
// access-log Record. Per SPEC §12 #3 + Decision A, the total reflects bytes
// written to the downstream (response body + status-line + headers in the H1
// path); short-writes account the actual byte count returned by the inner
// Write, not the request length.
type byteCounterWriter struct {
	w io.Writer
	n int64
}

func (bcw *byteCounterWriter) Write(p []byte) (int, error) {
	n, err := bcw.w.Write(p)
	bcw.n += int64(n)
	return n, err
}
```

Run: `go test -count=1 -run TestByteCounterWriter ./internal/filter/hcm/ -v`
Expected: PASS.

- [ ] **Step 3: Append Task 9 entry to PROGRESS.md + Commit** with subject `phase 06.2: internal/filter/hcm/bytecounter.go — byteCounterWriter`.

*Anchored: SPEC §1 #6, §4.1 (bytecounter.go), §5.4 (H1 BYTES_SENT path), §12 #3 (H2 BYTES_SENT semantics — Tasks 12-13 reuse the byteCounterWriter for H1 only).*

---

## Task 10: `internal/filter/hcm/accesslog_emit.go` — Filter.emitAccessLog (H1 + H2)

**Files:**
- Create: `internal/filter/hcm/accesslog_emit.go`
- Create: `internal/filter/hcm/accesslog_emit_test.go`
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 10 entry)

The emit-helper functions consumed by the four request-finalization sites. H1 path consumes `*http.Request` + primitives; H2 path consumes `h2.H2Request` + primitives. Both check the H2 ctx-cancel zero-status sentinel and skip submission per SPEC §2.1's last bullet.

**Precondition:** Tasks 2–9 done; the `Filter` struct has been extended in Task 11 (or is extended here in Step 1; the order is flexible — see Task 11 prerequisites).
**Artifact:** `internal/filter/hcm/accesslog_emit.go` (new) + test pair.
**Acceptance:** `go test -count=1 ./internal/filter/hcm/ -run TestEmitAccessLog -v` passes.
**Verification command:** `go test -count=1 ./internal/filter/hcm/ -run TestEmitAccessLog -v`.

- [ ] **Step 1: Decide the order of Task 10 vs Task 11.** This task introduces `Filter.emitAccessLog` as a method on `*Filter`. If Task 11 (the Filter-struct extension with the `accessLog []accesslog.Sink` field) hasn't landed, the method body's `for _, s := range f.accessLog: s.Submit(...)` loop won't compile. **Recommendation:** land Task 11's struct-field addition + sink-slice plumbing FIRST (Task 11 is a small ~30 LoC change), then come back to Task 10's emit-helper. The PLAN keeps Task 10 numbered as "the emit-helper task" for naming clarity, but the executor MAY swap Tasks 10 and 11 at execution time if cleaner.

- [ ] **Step 2: Write failing tests (in `accesslog_emit_test.go`)**

```go
package hcm

import (
	"net/http"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
)

type captureSink struct{ recs []*accesslog.Record }
func (s *captureSink) Submit(r *accesslog.Record) { s.recs = append(s.recs, r) }
func (s *captureSink) Close() error               { return nil }

func TestEmitAccessLog_H1_DirectResponseShape(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req, _ := http.NewRequest("GET", "/health", nil)
	req.Host = "127.0.0.1:10000"
	req.Header.Set("User-Agent", "Go-http-client/1.1")
	req.Proto = "HTTP/1.1"
	start := time.Now().Add(-5 * time.Millisecond)
	f.emitAccessLog(req, 200, 3, cluster.Endpoint{}, start)
	if len(cs.recs) != 1 { t.Fatalf("captured %d records, want 1", len(cs.recs)) }
	r := cs.recs[0]
	if r.Method != "GET" || r.Path != "/health" || r.Protocol != "HTTP/1.1" {
		t.Errorf("Record fields wrong: %+v", r)
	}
	if r.UpstreamHost != "" { t.Errorf("UpstreamHost should be empty for direct_response (zero Endpoint), got %q", r.UpstreamHost) }
	if r.ResponseCode != 200 || r.BytesSent != 3 { t.Errorf("status/bytes wrong: %d/%d", r.ResponseCode, r.BytesSent) }
}

func TestEmitAccessLog_H1_RoutedShape(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req, _ := http.NewRequest("GET", "/api/v1/foo", nil)
	req.Proto = "HTTP/1.1"
	picked := cluster.Endpoint{Host: "10.0.0.1", Port: 8080}
	f.emitAccessLog(req, 200, 17, picked, time.Now())
	if cs.recs[0].UpstreamHost != "10.0.0.1:8080" {
		t.Errorf("UpstreamHost = %q, want 10.0.0.1:8080", cs.recs[0].UpstreamHost)
	}
}

func TestEmitAccessLog_MultipleSinks_AllReceiveRecord(t *testing.T) {
	cs1, cs2 := &captureSink{}, &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs1, cs2}}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now())
	if len(cs1.recs) != 1 || len(cs2.recs) != 1 {
		t.Errorf("sink record counts: cs1=%d cs2=%d, want 1/1", len(cs1.recs), len(cs2.recs))
	}
}

func TestEmitAccessLog_H2_PseudoHeadersFromH2Request(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req := h2.H2Request{ /* fields populated per h2 package's H2Request shape */ }
	f.emitAccessLogH2(req, 200, 17, cluster.Endpoint{Host:"10.0.0.1", Port:8080}, time.Now())
	if len(cs.recs) != 1 { t.Fatal("expected 1 record") }
	if cs.recs[0].Protocol != "HTTP/2.0" { t.Errorf("Protocol = %q, want HTTP/2.0", cs.recs[0].Protocol) }
}

func TestEmitAccessLog_H2_StatusZeroSkipsEmission(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req := h2.H2Request{}
	f.emitAccessLogH2(req, 0, 0, cluster.Endpoint{}, time.Now())  // ctx-cancel sentinel
	if len(cs.recs) != 0 { t.Errorf("expected 0 records on status=0 ctx-cancel, got %d", len(cs.recs)) }
}

func TestEmitAccessLog_NoSinks_IsNoOp(t *testing.T) {
	f := &Filter{accessLog: nil}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now())  // must not panic
}
```

Run: `go test -count=1 -run TestEmitAccessLog ./internal/filter/hcm/ -v`
Expected: FAIL — `Filter.emitAccessLog`/`emitAccessLogH2` undefined.

- [ ] **Step 3: Implement `accesslog_emit.go`**

```go
package hcm

import (
	"net/http"
	"strconv"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode (the H1 actions return 200/404 deterministically), but the guard
// is uniform across H1+H2 callers.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       r.Method,
		Path:         r.URL.Path,
		Protocol:     r.Proto,
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    r.Host,
		UserAgent:    r.Header.Get("User-Agent"),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// emitAccessLogH2 is the H2-flavored variant; reads pseudo-headers (`:method`,
// `:path`, `:authority`) from H2Request instead of an *http.Request.
func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       req.Method(),     // accessor for :method
		Path:         req.Path(),       // accessor for :path
		Protocol:     "HTTP/2.0",
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    req.Authority(),  // accessor for :authority
		UserAgent:    req.UserAgent(),  // accessor for user-agent
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// upstreamHostString renders cluster.Endpoint as `host:port` for the access-log
// UPSTREAM_HOST operator. Zero-valued Endpoint (host == "") yields empty string;
// the formatter then emits the literal `-` per Decision A's missing-value
// convention.
func upstreamHostString(ep cluster.Endpoint) string {
	if ep.Host == "" {
		return ""
	}
	return ep.Host + ":" + strconv.Itoa(int(ep.Port))
}
```

If `h2.H2Request` lacks `Method()`/`Path()`/`Authority()`/`UserAgent()` accessors, add them at this task (or expose existing fields if the type already has them — verify at execution-time). The accessors are package-internal helpers, ~5 LoC each.

Run: `go test -count=1 -run TestEmitAccessLog ./internal/filter/hcm/ -v`
Expected: PASS.

- [ ] **Step 4: Append Task 10 entry to PROGRESS.md + Commit** with subject `phase 06.2: internal/filter/hcm/accesslog_emit.go — Filter.emitAccessLog (H1 + H2)`.

*Anchored: SPEC §1 #6, §4.1 (accesslog_emit.go), §5.4 (emit paths), §5.5 (read path), §14.2 (unit tests).*

---

## Task 11: HCM `Filter` struct extension + `parseFilterWithCtx` sink-slice plumbing

**Files:**
- Modify: `internal/filter/hcm/filter.go` (`Filter` struct gains `accessLog []accesslog.Sink` field)
- Modify: `internal/filter/hcm/config.go` (`parseFilterWithCtx` accepts and stores the sink slice)
- Modify: `internal/filter/hcm/filter_test.go` (assert the new field is plumbed)
- Modify: `internal/filter/hcm/config_test.go` (assert the new parameter is accepted)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 11 entry)

The `Filter` struct + filter-chain construction extends to carry the sink slice. Mirrors how 06.1 ADR-0059's `*stats.Registry` parameter was threaded.

**Precondition:** Tasks 2–10 done.
**Artifact:** `Filter` struct + `parseFilterWithCtx` call signature widened.
**Acceptance:** `go test -count=1 ./internal/filter/hcm/ -v` passes; the new field/parameter is grep-verifiable.
**Verification command:** `go test -count=1 ./internal/filter/hcm/ -v && grep -nE 'accessLog \[\]accesslog\.Sink' internal/filter/hcm/filter.go`.

- [ ] **Step 1: Modify `Filter` struct** in `internal/filter/hcm/filter.go`:

```go
type Filter struct {
	// ... existing fields (statPrefix, registry, downstreamRqTotal, etc.) ...
	accessLog []accesslog.Sink
}
```

- [ ] **Step 2: Modify `parseFilterWithCtx`** (or its caller in `config.go`) to accept a `[]accesslog.Sink` parameter alongside the existing `*stats.Registry` parameter, and store it on the constructed `*Filter`:

```go
func parseFilterWithCtx(..., reg *stats.Registry, accessLogSinks []accesslog.Sink) (*Filter, error) {
	// ... existing parse logic ...
	f := &Filter{
		// ... existing field initialization ...
		accessLog: accessLogSinks,
	}
	return f, nil
}
```

- [ ] **Step 3: Update all existing callers** of `parseFilterWithCtx` (or its public façade) to pass the sink slice. At this task they pass `nil` (the empty-slice case is exercised by the TestEmitAccessLog_NoSinks_IsNoOp test from Task 10); Task 14 (`cmd/envoy-go/main.go`) wires the real sinks.

- [ ] **Step 4: Run tests** — every existing HCM test must still pass with the widened signature. New tests for the field-plumbing land in `filter_test.go`:

```go
func TestFilter_AccessLogField_Plumbed(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	if len(f.accessLog) != 1 { t.Errorf("accessLog len = %d, want 1", len(f.accessLog)) }
}
```

Run: `go test -count=1 ./internal/filter/hcm/ -v`
Expected: PASS.

- [ ] **Step 5: Append Task 11 entry to PROGRESS.md + Commit** with subject `phase 06.2: HCM Filter struct + parseFilterWithCtx sink-slice plumbing`.

*Anchored: SPEC §1 #6, §4.2 (config.go + filter.go extensions), §5.1 (module graph).*

---

## Task 12: H1 emit-deferral sites (`directResponseAction.do` + `routerAction.do`)

**Files:**
- Modify: `internal/filter/hcm/actions.go` (the two H1 sites add `start := time.Now()` + `defer filter.emitAccessLog(...)`)
- Modify: `internal/filter/hcm/actions_test.go` (assert deferred emit fires with correct primitives)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 12 entry)

The two H1 finalization sites add deferrals. Per `## Settled SPEC §12 deferred decisions` #8, both capture `start` at function entry of the action's `do` method for uniform DURATION semantics. `routerAction.do` additionally wraps `bw` in a `byteCounterWriter` for `BYTES_SENT` capture and consumes the picked endpoint from Task 8's expanded `Cluster.Dial` return tuple.

**Precondition:** Tasks 2–11 done; `Filter.emitAccessLog` is exported; `byteCounterWriter` exists; `Cluster.Dial` returns the picked endpoint.
**Artifact:** the two H1 sites carry `defer filter.emitAccessLog(...)` calls.
**Acceptance:** unit tests for both sites verify the deferred emit fires with correct `start`/`bytesSent`/`picked`/`statusCode` primitives; `go test -count=1 ./internal/filter/hcm/ -v` passes.
**Verification command:** `go test -count=1 ./internal/filter/hcm/ -v && grep -cE 'defer .*\.emitAccessLog\(' internal/filter/hcm/actions.go` returns at least 2.

- [ ] **Step 1: Locate the two H1 sites** — `directResponseAction.do` (around `actions.go:95`) and `routerAction.do` (around `actions.go:128`). Verify with `grep -nE 'directResponseAction\) do|routerAction\) do' internal/filter/hcm/actions.go`.

- [ ] **Step 2: Modify `directResponseAction.do`** — add `start := time.Now()` at function entry and `defer a.filter.emitAccessLog(req, statusCode, int64(len(body)), cluster.Endpoint{}, start)` before the `return`:

```go
func (a *directResponseAction) do(_ context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	start := time.Now()
	status, _, body := a.body()
	// ... existing write logic ...
	defer a.filter.emitAccessLog(req, status, int64(len(body)), cluster.Endpoint{}, start)
	return status, nil
}
```

The action needs access to the parent `*Filter`. If it doesn't have one today, add a `filter *Filter` field to `directResponseAction` (settled in Task 11's struct extensions). Verify the `body()` method's return shape — it's `(status int, headers http.Header, body []byte)` per the existing code at `actions.go:55`.

- [ ] **Step 3: Write a failing test** asserting the deferral fires:

```go
func TestDirectResponseAction_EmitsAccessLog(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	a := &directResponseAction{filter: f, status: 200, bodyText: "OK\n"}
	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	bw := bufio.NewWriter(io.Discard)
	if _, err := a.do(context.Background(), req, bw); err != nil { t.Fatal(err) }
	bw.Flush()
	if len(cs.recs) != 1 { t.Fatalf("captured %d records, want 1", len(cs.recs)) }
	r := cs.recs[0]
	if r.ResponseCode != 200 || r.BytesSent != 3 { t.Errorf("status/bytes: %d/%d", r.ResponseCode, r.BytesSent) }
}
```

Run, expect FAIL → implement → expect PASS.

- [ ] **Step 4: Modify `routerAction.do`** — `start := time.Now()` at function entry; wrap the downstream writer in a `byteCounterWriter`; consume the picked endpoint from `Cluster.Dial`'s expanded return; `defer a.filter.emitAccessLog(req, statusCode, bcw.n, picked, start)` at exit:

```go
func (a *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	start := time.Now()
	bcw := &byteCounterWriter{w: bw}
	upstream, picked, err := a.cluster.Dial(ctx)
	if err != nil { /* existing error path; emit with statusCode = 502 + zero picked */
		defer a.filter.emitAccessLog(req, 502, bcw.n, cluster.Endpoint{}, start)
		return 502, err
	}
	defer func() { _ = upstream.Close() }()
	// ... existing body that writes through `bw`; replace `bw` with `bcw` in the
	// resp.Write(...) call so byte count flows ...
	// resp.Write(bcw)  // captures all downstream-written bytes
	defer a.filter.emitAccessLog(req, statusCode, bcw.n, picked, start)
	return statusCode, nil
}
```

If the existing `routerAction.do` writes through a path that doesn't go through `bw` for the body bytes (e.g., a `io.Copy(bw, resp.Body)` that bypasses the wrapper), thread the wrapper carefully: `io.Copy(bcw, resp.Body)` so the body bytes count. Verify at execution time by inspecting the existing write path in `actions.go:128-170`.

- [ ] **Step 5: Write a failing test** asserting the routerAction deferral:

```go
func TestRouterAction_EmitsAccessLog_RoutedShape(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	c := buildTestClusterWithEcho(t)  // existing helper (or build for this test)
	a := &routerAction{filter: f, cluster: c}
	req, _ := http.NewRequest("GET", "/api/v1/foo", nil)
	bw := bufio.NewWriter(io.Discard)
	_, _ = a.do(context.Background(), req, bw)
	if len(cs.recs) != 1 { t.Fatal("expected 1 record") }
	if cs.recs[0].UpstreamHost == "" { t.Errorf("UpstreamHost empty; expected host:port") }
	if cs.recs[0].BytesSent <= 0 { t.Errorf("BytesSent <= 0; expected positive count") }
}
```

Run, expect PASS after step 4's implementation.

- [ ] **Step 6: Append Task 12 entry to PROGRESS.md + Commit** with subject `phase 06.2: HCM H1 emit-deferral sites (directResponseAction + routerAction)`.

*Anchored: SPEC §1 #6, §4.2 (actions.go extension), §5.4 (H1 emit paths).*

---

## Task 13: H2 emit-deferral sites (`h2DirectResponseAdapter.WriteH2` + `routerActionH2.doH2`)

**Files:**
- Modify: `internal/filter/hcm/h2dispatch.go` (`h2DirectResponseAdapter.WriteH2` adds `defer adapter.f.emitAccessLogH2(...)`)
- Modify: `internal/filter/hcm/actions.go` (`routerActionH2.doH2` adds `start := time.Now()` + bytes-sent accounting + defer)
- Modify: `internal/filter/hcm/h2dispatch_test.go` + `actions_test.go` (assert the H2 deferrals fire)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 13 entry)

The two H2 finalization sites add deferrals. The H2 ctx-cancel path in `routerActionH2.doH2` returns `(0, h2.NewStreamError(...))`; the deferred emit's zero-status sentinel skips submission per SPEC §2.1.

**Precondition:** Tasks 2–12 done; `Filter.emitAccessLogH2` is exported.
**Artifact:** the two H2 sites carry deferrals.
**Acceptance:** unit tests verify H2 deferrals fire with correct primitives + ctx-cancel skips emission.
**Verification command:** `go test -count=1 ./internal/filter/hcm/ -v && grep -cE 'defer .*\.emitAccessLogH2\(' internal/filter/hcm/h2dispatch.go internal/filter/hcm/actions.go` returns at least 2.

- [ ] **Step 1: Modify `h2DirectResponseAdapter.WriteH2`** in `h2dispatch.go` (around `h2dispatch.go:89`):

```go
func (a *h2DirectResponseAdapter) WriteH2(_ context.Context, req h2.H2Request, sw h2.StreamWriter) error {
	start := time.Now()
	status, _, body := a.a.body()
	defer a.f.emitAccessLogH2(req, status, int64(len(body)), cluster.Endpoint{}, start)
	// ... existing writeH2 body logic ...
	return a.a.writeH2(sw)
}
```

- [ ] **Step 2: Modify `routerActionH2.doH2`** in `actions.go` (around `actions.go:222`):

```go
func (r *routerActionH2) doH2(ctx context.Context, req h2.H2Request, w h2.StreamWriter) (int, error) {
	start := time.Now()
	cc, picked, err := r.cluster.DialH2(ctx)
	if err != nil {
		defer r.filter.emitAccessLogH2(req, 502, 0, cluster.Endpoint{}, start)
		return 502, err
	}
	defer func() { _ = cc.Close() }()
	// ... existing round-trip logic; on success, capture bytesSent = len(resp.Body) ...
	// per `## Settled SPEC §12 deferred decisions` #3 (option (a) — sum body bytes only).
	bytesSent := int64(len(respBody))
	defer r.filter.emitAccessLogH2(req, statusForHCM, bytesSent, picked, start)
	return statusForHCM, nil
	// Note: when statusForHCM == 0 (ctx-cancel sentinel per actions.go:240-247),
	// the deferred emitAccessLogH2 sees statusCode == 0 and skips Submit per
	// SPEC §2.1's "no record on ctx-cancel" semantics.
}
```

- [ ] **Step 3: Write failing tests** for both H2 sites:

```go
func TestH2DirectResponseAdapter_EmitsAccessLog(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	a := &h2DirectResponseAdapter{
		a: &directResponseAction{filter: f, status: 200, bodyText: "OK\n"},
		f: f,
	}
	// ... invoke WriteH2 with a mock h2.StreamWriter ...
	if len(cs.recs) != 1 { t.Fatal("expected 1 record") }
}

func TestRouterActionH2_DoH2_StatusZeroSkipsEmit(t *testing.T) {
	cs := &captureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	r := &routerActionH2{ /* setup such that doH2 returns (0, ctxCancelErr) */ }
	r.filter = f
	_, _ = r.doH2(canceledCtx, h2.H2Request{}, mockStreamWriter{})
	if len(cs.recs) != 0 { t.Errorf("expected 0 records on ctx-cancel, got %d", len(cs.recs)) }
}
```

Run: `go test -count=1 ./internal/filter/hcm/ -v` expect PASS.

- [ ] **Step 4: Append Task 13 entry to PROGRESS.md + Commit** with subject `phase 06.2: HCM H2 emit-deferral sites (h2DirectResponseAdapter + routerActionH2)`.

*Anchored: SPEC §1 #6, §4.2 (actions.go + h2dispatch.go extensions), §5.4 (H2 emit paths), §14.2 (H2 unit-test split).*

---

## Task 14: `cmd/envoy-go/main.go` — open AsyncFileSinks + thread + defer Close

**Files:**
- Modify: `cmd/envoy-go/main.go` (boot wiring per SPEC §5.3)
- Modify: `cmd/envoy-go/main_test.go` (extend bootstrap-variant smoke tests for the access-log path)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 14 entry)

Boot wiring per SPEC §5.3: between `bootstrap.Load(...)` and `listenerManager.Run(...)`, allocate the `server.accesslog_dropped` counter once via `accesslog.RegisterDroppedCounter(bs.Stats)`; iterate `bs.AccessLogConfigs` calling `accesslog.NewAsyncFileSink(cfg.Path, droppedCounter)` for each; collect the slice; thread into `internal/filter/hcm/config.go`'s filter-chain construction; `defer sink.Close()` for each in registration order after `listener.Shutdown()` returns.

**Precondition:** Tasks 2–13 done.
**Artifact:** `cmd/envoy-go/main.go` + `main_test.go` extended.
**Acceptance:** `go test -count=1 ./cmd/envoy-go/ -v && go vet ./...` passes; manual smoke test (boot envoy-go with a single `access_log[]` entry → file is created and receives records on requests).
**Verification command:** `go test -count=1 ./cmd/envoy-go/ -v && go vet ./...`.

- [ ] **Step 1: Modify `cmd/envoy-go/main.go`** per SPEC §5.3's boot wiring sequence:

```go
// (after bootstrap.Load returns)
droppedCounter := accesslog.RegisterDroppedCounter(bs.Stats)
sinks := make([]accesslog.Sink, 0, len(bs.AccessLogConfigs))
for _, cfg := range bs.AccessLogConfigs {
	sink, err := accesslog.NewAsyncFileSink(cfg.Path, droppedCounter)
	if err != nil {
		log.Fatalf("accesslog: open %q: %v", cfg.Path, err)
	}
	sinks = append(sinks, sink)
}
// (defer sink.Close after listener.Shutdown — registration order)
defer func() {
	for _, s := range sinks {
		_ = s.Close()
	}
}()

// (thread sinks into HCM filter-chain construction; the existing 06.1 plumbing
// already passes bs.Stats; this extension passes sinks alongside per Task 11's
// parseFilterWithCtx widened signature)
lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs, cm, bs.Stats, baseDir, allowH2C, sinks)
```

The exact threading shape depends on the listener manager's existing constructor — verify at execution time. Pass `sinks` either through `listener.NewManager...` or through a separate hcm-config-builder call site, whichever is the smaller-diff option.

- [ ] **Step 2: Extend `main_test.go`** — at least one new test case asserting that:
  - A bootstrap with one `access_log[]` entry produces a non-empty `bs.AccessLogConfigs`.
  - The sink is opened (file exists; smoke after one request, verify the file has at least one line).
  - SIGTERM (or context-cancel teardown) leaves the file with exactly the records emitted (drain semantics: pending records flushed before `f.Close()`).

- [ ] **Step 3: Run `go test -count=1 ./cmd/envoy-go/ -v && go vet ./...`** expect PASS.

- [ ] **Step 4: Append Task 14 entry to PROGRESS.md + Commit** with subject `phase 06.2: cmd/envoy-go/main.go — open AsyncFileSinks + thread + defer Close`.

*Anchored: SPEC §1 #7, §4.2 (main.go extension), §5.3 (boot wiring sequence).*

---

## Task 15: Differential fixture `test/fixtures/0006-access-log/` + runner registration [ADR-0068]

**Files:**
- Create: `test/fixtures/0006-access-log/envoy-go.yaml`
- Create: `test/fixtures/0006-access-log/envoy.yaml`
- Create: `test/fixtures/0006-access-log/expectations.yaml`
- Create: `test/fixtures/0006-access-log/README.md`
- Create: `test/fixtures/0006-access-log/driver/driver.go`
- Create: `test/fixtures/0006-access-log/driver/driver_test.go`
- Create: `test/fixtures/0006-access-log/backends/main.go`
- Modify: `test/differential/runner.go` (blank-import the new fixture-0006 driver package; register)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0068)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (append Task 15 entry)

The differential fixture per SPEC §7. ADR-0068 (three-tier matrix) lands here per the first-use commit-time ordering. The polling-loop drain pattern from Decision G (adopting 06.1 REVIEW M-8 prophylactically) is implemented in the driver.

**Precondition:** Tasks 2–14 done; the empirical-format pin from Task 3 step 3 is committed.
**Artifact:** complete fixture directory + driver + backends + runner registration; ADR-0068 appended.
**Acceptance:** `go test -count=1 ./test/differential/ -run Test.*0006 -v` passes (gate (a) non-vacuous green); pre-existing fixtures still green (gate (b)).
**Verification command:** `go test -count=1 ./test/differential/ -v`.

- [ ] **Step 1: Verify `test/differential/harness.go` Mounts plumbing**. The reference container needs to bind-mount `<t.TempDir()>/reference.log` to `/tmp/envoy-access.log`. Verify `testcontainers-go` `Mounts` is wired:

```bash
grep -nE 'Mounts|HostConfigModifier' test/differential/harness.go
```

If absent, add minimal Mounts plumbing at this step (small ~15 LoC change, separate from the fixture itself).

- [ ] **Step 2: Author `envoy-go.yaml`** — 1 listener `l_h1` binding `127.0.0.1:0` plaintext; 1 HCM with `codec_type: HTTP1`, `stat_prefix: ingress_http`, `access_log[]` with one `envoy.access_loggers.file` entry whose `path` is configured at runtime by the runner (see Step 6); 1 route_config with three routes (`/health` → 200 `OK\n`; `/notfound` → 404 `not found\n`; `prefix:/api/v1/` → cluster `c_backend`); 1 STATIC cluster `c_backend` with 3 endpoints. Use the same shapes as fixture-0005's `envoy-go.yaml`.

- [ ] **Step 3: Author `envoy.yaml`** — same shape with STRICT_DNS cluster `c_backend` pointing at `host.docker.internal:<backend-N-port>` for N ∈ {0,1,2} with `dns_lookup_family: V4_ONLY` per ADR-0010; reference is invoked with `--concurrency 1` per ADR-0028 (the existing harness machinery sets this); `access_log[].path = /tmp/envoy-access.log` (bind-mounted to `<t.TempDir()>/reference.log` per Step 1's harness plumbing).

- [ ] **Step 4: Author `expectations.yaml`** — transcribe SPEC §7.4 verbatim: the 5-record × 15-operator three-tier matrix. The driver implements the predicates; this file is the contract.

- [ ] **Step 5: Author `README.md`** — purpose (per-record three-tier field equivalence; second observability-surface differential), STATIC-vs-STRICT_DNS divergence, 5-request workload, log-file mounting + polling convention, cross-reference to BEHAVIOR_CONTRACT.md.

- [ ] **Step 6: Author `backends/main.go`** — small Go HTTP/1.1 server. Each backend N ∈ {0,1,2} serves any GET with `200 OK` and body `backend-N:v1/<path-segment>\n` where `<path-segment>` is the trailing path component. Body length 17 bytes byte-identical across N for foo/bar/baz per SPEC §7.2 invariant.

- [ ] **Step 7: Author `driver/driver.go`** — implement `BackendCount() int = 3`, `SubjectListenerName() = "l_h1"`, `ReferenceListenerPort() = 15006`, `DriveSubject(ctx, addr)` and `DriveReference(ctx, addr)`:

```go
func (d *Driver) DriveSubject(ctx context.Context, addr string) error {
	return drive(ctx, addr, d.SubjectLogPath)
}
func (d *Driver) DriveReference(ctx context.Context, addr string) error {
	return drive(ctx, addr, d.ReferenceLogPath)
}

// drive issues the 5 sequential GETs then polls the log file at 25ms intervals
// until ≥ 5 lines or 5s deadline (Decision G — polling-loop, no arbitrary
// time.Sleep; adopts 06.1 REVIEW M-8 prophylactically).
func drive(ctx context.Context, addr, logPath string) error {
	paths := []string{"/health", "/api/v1/foo", "/api/v1/bar", "/api/v1/baz", "/notfound"}
	c := &http.Client{Timeout: 2 * time.Second}
	for _, p := range paths {
		resp, err := c.Get("http://" + addr + p)
		if err != nil { return fmt.Errorf("GET %s: %w", p, err) }
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	// Poll the log file at 25ms intervals.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logPath)
		if err == nil && bytes.Count(data, []byte{'\n'}) >= 5 { return nil }
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("polling deadline (5s) exceeded for %s; log file not yet ≥ 5 lines", logPath)
}

// AssertAccessLogEquivalence parses both log files into 15-tuples and applies
// the three-tier matrix per ADR-0068.
func (d *Driver) AssertAccessLogEquivalence(t fixture.TB) {
	subject := parseLogFile(t, d.SubjectLogPath)
	reference := parseLogFile(t, d.ReferenceLogPath)
	if len(subject) != 5 || len(reference) != 5 {
		t.Fatalf("record counts: subject=%d reference=%d, want 5/5", len(subject), len(reference))
	}
	expectedPaths := []string{"/health", "/api/v1/foo", "/api/v1/bar", "/api/v1/baz", "/notfound"}
	expectedStatus := []string{"200", "200", "200", "200", "404"}
	expectedBytes := []string{"3", "17", "17", "17", "10"}
	for i := 0; i < 5; i++ {
		applyTierMatrix(t, i+1, subject[i], reference[i], expectedPaths[i], expectedStatus[i], expectedBytes[i])
	}
}
```

The `parseLogFile` helper applies the positional regex anchored on `[`, `]`, `"`, space delimiters:

```go
// recordRE captures the 15 positional operators per the SPEC §6 catalog.
var recordRE = regexp.MustCompile(
	`^\[([^\]]+)\] "([^ ]+) ([^ ]+) ([^"]+)" (\d+) (\S+) (\S+) (\d+) (\d+) (\S+) "([^"]*)" "([^"]*)" "([^"]*)" "([^"]*)" "([^"]*)"$`,
)
```

The `applyTierMatrix` helper applies the per-field tier rule per ADR-0068:

```go
func applyTierMatrix(t fixture.TB, recIdx int, subj, ref [15]string, expectedPath, expectedStatus, expectedBytes string) {
	// Tier E (8): :METHOD, :PATH, PROTOCOL, RESPONSE_CODE, BYTES_SENT,
	//             RESP-SVC-TIME (both `-`), USER-AGENT, :AUTHORITY
	// Tier F (3): START_TIME, DURATION, UPSTREAM_HOST
	// Tier S (4): RESPONSE_FLAGS, BYTES_RECEIVED, X-FORWARDED-FOR, X-REQUEST-ID

	// Tier E byte-equal where the values are deterministic and known
	// independently — :METHOD, :PATH, PROTOCOL, RESPONSE_CODE, BYTES_SENT.
	if subj[1] != "GET" { t.Errorf("rec %d :METHOD subject = %q, want GET", recIdx, subj[1]) }
	if ref[1] != "GET" { t.Errorf("rec %d :METHOD reference = %q, want GET", recIdx, ref[1]) }
	if subj[2] != expectedPath { t.Errorf("rec %d :PATH subject = %q, want %q", recIdx, subj[2], expectedPath) }
	if ref[2] != expectedPath { t.Errorf("rec %d :PATH reference = %q, want %q", recIdx, ref[2], expectedPath) }
	// ... and so on for fields 4 (PROTOCOL), 5 (RESPONSE_CODE), 8 (BYTES_SENT)
	// Field 10 (RESP-SVC-TIME) — both must equal `-`
	if subj[9] != "-" || ref[9] != "-" {
		t.Errorf("rec %d RESP-SVC-TIME subject=%q reference=%q; both must be `-`", recIdx, subj[9], ref[9])
	}
	// Field 12 (USER-AGENT) — byte-equal cross-side (both Go-http-client/1.1 from the driver)
	if subj[11] != ref[11] {
		t.Errorf("rec %d USER-AGENT subject=%q reference=%q; not equal", recIdx, subj[11], ref[11])
	}
	// Field 14 (:AUTHORITY) — port-strip per Decision J + `## Settled SPEC §12` #6
	if stripPort(subj[13]) != stripPort(ref[13]) {
		t.Errorf("rec %d :AUTHORITY (port-stripped) subject=%q reference=%q", recIdx, stripPort(subj[13]), stripPort(ref[13]))
	}

	// Tier F: parser-predicate only. No cross-side equality assertion.
	if !startTimeRE.MatchString("[" + subj[0] + "]") { t.Errorf("rec %d START_TIME subject malformed: %q", recIdx, subj[0]) }
	if !startTimeRE.MatchString("[" + ref[0] + "]") { t.Errorf("rec %d START_TIME reference malformed: %q", recIdx, ref[0]) }
	if duration, err := strconv.Atoi(subj[8]); err != nil || duration < 0 {
		t.Errorf("rec %d DURATION subject = %q, want non-negative int", recIdx, subj[8])
	}
	// UPSTREAM_HOST: subject + reference must match a `host:port` regex OR be literal `-`.
	if !upstreamHostRE.MatchString(subj[14]) { t.Errorf("rec %d UPSTREAM_HOST subject malformed: %q", recIdx, subj[14]) }

	// Tier S: subject must be exactly `-` for fields 6,7 (un-quoted) and `"-"` for 11,13 (quoted).
	if subj[5] != "-" { t.Errorf("rec %d RESPONSE_FLAGS subject = %q, want `-`", recIdx, subj[5]) }
	if subj[6] != "-" { t.Errorf("rec %d BYTES_RECEIVED subject = %q, want `-`", recIdx, subj[6]) }
	if subj[10] != "-" { t.Errorf("rec %d X-FORWARDED-FOR subject = %q, want `-`", recIdx, subj[10]) }
	if subj[12] != "-" { t.Errorf("rec %d X-REQUEST-ID subject = %q, want `-`", recIdx, subj[12]) }
	// Reference values for Tier S fields are NOT asserted (Envoy emits real values; we ignore them).
}
```

- [ ] **Step 8: Author `driver/driver_test.go`** — parser regex unit tests (round-trip the SPEC §11 empirical scrape lines through the regex; assert the 15-tuple extraction matches the field-by-field expected values).

- [ ] **Step 9: Modify `test/differential/runner.go`** — blank-import the fixture-0006 driver package and register the fixture in the runner's per-fixture loop, mirroring the fixture-0005 registration pattern.

- [ ] **Step 10: Append ADR-0068 to `DECISIONS.md`** with full Context / Decision / Alternatives / Consequences sections.

- [ ] **Step 11: Run `go test -count=1 ./test/differential/ -run Test.*0006 -v`** expect PASS. If failing, inspect the parser regex, the tier-matrix application, the polling-loop deadline, and the empirical-pin discrepancy first (debugging order matches fixture-0005's playbook in 06.1 PROGRESS).

- [ ] **Step 12: Append Task 15 entry to PROGRESS.md + Commit** with subject `phase 06.2: differential fixture 0006-access-log + runner [ADR-0068]`.

*Anchored: SPEC §1 #9, §4.3 (fixture inventory), §7 (full fixture spec), §8 (ADR-0068 anticipation), §13.2 (Equivalence Matrix row anchor).*

---

## Task 16: BEHAVIOR_CONTRACT in-place edit + ROADMAP/STATE updates + closing all-gates sweep

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place: `## Access log field mapping` populated per SPEC §13.1)
- Modify: `docs/envoy-go/ROADMAP.md` (anticipated row updates — applied at the phase-done commit per BOOTSTRAP §5 step 6)
- Modify: `docs/envoy-go/STATE.md` (lifecycle-state 4)
- Modify: `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` (closing entry with all-gates output)

The closing sweep. The BEHAVIOR_CONTRACT in-place edit lands at this commit per ADR-0052's authorisation (per SPEC §13 + §4.4); ROADMAP row updates anticipate the REVIEW session's phase-done commit (per the 05.2 / 06.1 PLAN's Task 15 NOTE convention; phase 06.2 is special because the parent row 06 ALSO flips at the phase-done — both updates land at the REVIEW session's commit, not at this Task 16 commit).

**Precondition:** Tasks 1–15 done; all four ADRs (ADR-0066..ADR-0069) landed; the empirical-format pin filled at Task 3.
**Artifact:** BEHAVIOR_CONTRACT in-place edit; STATE.md → state 4; PROGRESS closing entry with all six gates (a/b/c/d/e green; f deferred to REVIEW session).
**Acceptance:** all gates green per BOOTSTRAP §7.5 / SPEC §3; the BEHAVIOR_CONTRACT edit is grep-verifiable.
**Verification command:** the gate sweep at Step 3 below + `grep -A20 '^## Access log field mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md` shows the populated subsection.

- [ ] **Step 1: Edit `BEHAVIOR_CONTRACT.md ## Access log field mapping`** in place. The existing placeholder (lines ~170–174) is:

```
## Access log field mapping

_to be filled per-phase as needed._

The access-log field mapping enumerates every field that must appear (and the field it maps to on upstream Envoy), the ignore-list for values that are inherently non-deterministic (timestamps, connection ids, durations), and the format normalization rules used by the differential harness before comparison.
```

Replace with the populated subsection per SPEC §13.1:

```
## Access log field mapping

*Introduced by phase 06.2. Justified by ADR-0066 (architecture: in-tree file sink + AsyncFileSink + drop-newest backpressure), ADR-0067 (reject log_format at parse), ADR-0068 (this subsection — three-tier per-record per-field equivalence matrix), ADR-0069 (server.accesslog_dropped counter naming).*

The access-log field mapping enumerates every operator in the Envoy default
access-log format (15 operators in identical positions on every record) per
ADR-0066, the per-operator equivalence tier per ADR-0068's three-tier matrix,
and the empirical-pin block recording the verbatim format-string shape from
reference Envoy v1.37.2. The differential equivalence claim (the
"Semantically equal after field-mapping" predicate from the Equivalence
Matrix row at line 18) IS the three-tier matrix below.

### 15-operator default format (per ADR-0066; empirical-pin in §11 of the 06.2 SPEC)

[<START_TIME>] "<:METHOD> <:PATH> <PROTOCOL>" <RESPONSE_CODE> <RESPONSE_FLAGS>
<BYTES_RECEIVED> <BYTES_SENT> <DURATION> <RESP-SVC-TIME> "<X-FORWARDED-FOR>"
"<USER-AGENT>" "<X-REQUEST-ID>" "<:AUTHORITY>" "<UPSTREAM_HOST>"

### Three-tier matrix (per ADR-0068)

Tier E (exact byte-equal cross-side; 8 operators):
  :METHOD, :PATH, PROTOCOL, RESPONSE_CODE, BYTES_SENT,
  RESP(X-ENVOY-UPSTREAM-SERVICE-TIME) (both `-`), USER-AGENT, :AUTHORITY

Tier F (format-only — parses to expected shape on both sides; cross-side value
not asserted equal; 3 operators):
  START_TIME (RFC3339 ms-precision UTC, within workload wall-clock window)
  DURATION (int ms ≥ 0)
  UPSTREAM_HOST (`<host>:<port>` for routed; `-` for direct_response)

Tier S (subject must emit `-`; reference unconstrained; 4 operators):
  RESPONSE_FLAGS, BYTES_RECEIVED, X-FORWARDED-FOR, X-REQUEST-ID

Counts: 8 + 3 + 4 = 15 (= the operator count in the format).

### X-ENVOY-ORIGINAL-PATH?:PATH fallback note (per 06.2 SPEC §6.1)

Operator #3 in the format is %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% — emit the
original-path header if present, else fall through to :PATH. Neither side
emits X-ENVOY-ORIGINAL-PATH on fixture 0006's workload (envoy-go does not
inject it; reference Envoy doesn't either, because fixture 0006 has no
path_rewrite-bearing route); both sides emit :PATH via the fallback. A
future phase introducing path-rewriting must re-evaluate fixture 0006's
Tier-E/F expectations under the new behavior.

### Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log)

[Verbatim 5 lines from the 06.2 SPEC §11 empirical-pin block — copied at this
commit; SPEC §11 is the ground truth, this subsection mirrors it.]

### Applies to

- Phase-06.2 envoy-go `internal/accesslog` package + the four HCM emit-deferral sites (`directResponseAction.do`, `routerAction.do`, `h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`), exercised via fixture `0006-access-log` (H1 differential) + `internal/filter/hcm/accesslog_emit_test.go` (H2 unit tests).
- The 15-operator Envoy default format only. Custom format strings (the `log_format`/`format_string`/`json_format` typed-config fields) are rejected at parse-time per ADR-0067.

### Does not yet apply to

- Operators not plumbed in 06.2 (5 of 15: RESPONSE_FLAGS, BYTES_RECEIVED, RESP-SVC-TIME, X-FORWARDED-FOR, X-REQUEST-ID — Tier S subject-emits-`-`).
- Sinks other than `envoy.access_loggers.file` (stdout / tcp_grpc (gRPC ALS) / open_telemetry — silently-ignored per ADR-0041 06.2 amendment).
- Per-route access-log filters (`access_log[].filter` — silently-ignored).
- Log rotation, fsync, durability ceilings (out of scope per SPEC §2.1).
- Trailers in access logs (deferred to gRPC family per ADR-0058).
- Access-log records for ctx-cancelled requests (skipped per the H2 zero-status sentinel, SPEC §2.1).
- SIGTERM-while-record-pending drain semantics (Phase 08's deliverable).
```

- [ ] **Step 2: Confirm SPEC §11 ↔ BEHAVIOR_CONTRACT mirror.** Both should carry the verbatim 5-line empirical scrape. Verify with `diff` of the two block excerpts.

- [ ] **Step 3: Six-gate local sweep** — run all gates per BOOTSTRAP §7.5 and SPEC §3:

```bash
# Gate (a): new fixture green
go test -count=1 ./test/differential/ -run Test.*0006 -v

# Gate (b): pre-existing fixtures still green
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005' -v

# Gate (c): conformance suites pass (h2spec at the ADR-0051 pin — UNCHANGED in 06.2)
go test -count=1 ./test/conformance/h2spec/ -v

# Gate (d): fuzzers run clean for CI short-budget (the eight, one of which is new)
go test -race ./internal/bootstrap/        -fuzz=FuzzBootstrapLoad     -fuzztime=30s
go test -race ./internal/filter/tcpproxy/  -fuzz=FuzzTcpProxyFilter    -fuzztime=30s
go test -race ./internal/tls/              -fuzz=FuzzTLSContextParse   -fuzztime=30s
go test -race ./internal/filter/hcm/       -fuzz=FuzzHCMConfigParse    -fuzztime=30s
go test -race ./internal/filter/hcm/h2/    -fuzz=FuzzFrameStream       -fuzztime=30s
go test -race ./internal/filter/hcm/h2/    -fuzz=FuzzHPACKDecode       -fuzztime=30s
go test -race ./internal/stats/            -fuzz=FuzzPromTextFormat    -fuzztime=30s
go test -race ./internal/accesslog/        -fuzz=FuzzAccessLogFormat   -fuzztime=30s   # NEW

# Gate (e): vet/lint/test
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./...
```

Expected: every gate green (a/b/c/d/e). Gate (f) — REVIEW.md approval — is the next session's responsibility, deferred per BOOTSTRAP §5 step 6.

- [ ] **Step 4: ADR-0066 boundary grep + no-third-party-access-log-library check**

```bash
# Per ADR-0066 + SPEC §15 acceptance bullet "No third-party access-log dependency".
grep -nE 'github.com/sirupsen/logrus|go.uber.org/zap|github.com/rs/zerolog|github.com/fluent/fluent-logger-golang' go.mod go.sum
```

Expected: NO matches in go.mod (matches in go.sum may be transitive — verify each is NOT pulled by an `internal/...` import path via `go mod why`).

```bash
# Per SPEC §15 final acceptance bullet "No third-party access-log library is imported".
grep -nR '"github.com/.*\(logrus\|zap\|zerolog\|fluent\)' internal/ cmd/envoy-go/ --include='*.go' | grep -v '_test.go'
```

Expected: empty output.

- [ ] **Step 5: Four-emit-hook-site grep** per SPEC §15 acceptance bullet:

```bash
grep -cE 'defer .*\.emitAccessLog\(' internal/filter/hcm/actions.go
# expect: at least 2 (directResponseAction.do + routerAction.do)
grep -cE 'defer .*\.emitAccessLogH2\(' internal/filter/hcm/h2dispatch.go internal/filter/hcm/actions.go
# expect: at least 2 (h2DirectResponseAdapter.WriteH2 + routerActionH2.doH2)
```

Expected: 2 + 2 = 4 distinct deferral sites.

- [ ] **Step 6: 15-operator-format grep** — confirm `accesslog.Default` emits all 15 operator positions:

```bash
go test -count=1 -v -run TestDefault ./internal/accesslog/
```

Expected: every test case PASS, including the SPEC §11 empirical-pin round-trip (each captured line round-trips through Default → parse → re-emit → matches).

- [ ] **Step 7: Update STATE.md to lifecycle-state 4 (verification-pending)**

```yaml
- active-phase: 06.2-access-log
- phase-directory: docs/envoy-go/phases/06.2-access-log/
- lifecycle-state: 4   # implementation complete; verification not yet run
- next-skill: superpowers:verification-before-completion
- next-skill-scope: <verify all six gates per BOOTSTRAP §7.5 / SPEC §3>
- last-commit: <Task 16 commit SHA>
- last-updated: <date>
```

- [ ] **Step 8: Anticipated ROADMAP row updates (lands at the phase-done commit, NOT at Task 16)**

Per BOOTSTRAP §5 step 6: the phase-done commit (lifecycle-state 6) is owned by the REVIEW session, NOT by Task 16. Task 16 advances STATE to lifecycle-state 4. The verification session re-runs the gates (state 5) and the REVIEW session writes REVIEW.md (state 6 — phase-done). The anticipated ROADMAP-row text (to land at the phase-done commit per SPEC §4.4):

```markdown
| 06   | observability-baseline | 05 | done         | 06.1, 06.2 | Sub-phases 06.1 (stats) + 06.2 (access-log). Closes at 06.2's phase-done commit per parent SPEC §5 closure pattern. |
| 06.1 | stats-prometheus       | 05 | done         |  | (unchanged) |
| 06.2 | access-log             | 06.1 | done       |  | Internal `accesslog` package (file sink + Default formatter + AsyncFileSink) + 4 HCM emit-deferral sites + fixture 0006 (per-record three-tier equivalence; 5-request workload, 10 records); BEHAVIOR_CONTRACT.md ## Access log field mapping populated; closes parent row 06 at this commit. ADR-0066..ADR-0069. |
```

The PROGRESS Task 16 entry records "ROADMAP rows 06.2 + 06 still in their pre-phase-done state at this commit; the phase-done commit at lifecycle-state 6 will flip BOTH (06.2 → done; 06 → done) per parent SPEC §5." Refinement: Task 16 advances STATE to lifecycle-state 4 only.

- [ ] **Step 9: Append Task 16 closing entry to PROGRESS.md** (with verification block).

The PROGRESS Task 16 entry is the session's "verification proof" — `superpowers:verification-before-completion` reads it when phase 06.2 moves to lifecycle-state 5. Keep every last-30-lines block verbatim. Mirror the phase-04 / 05.1 / 05.2 / 06.1 PROGRESS Task-N closing entry shape.

The entry includes:
- Each gate's command + last-30-lines output verbatim.
- ADR-0066 boundary grep result (no third-party access-log library).
- Four-emit-hook-site grep results.
- The carry-forward triage log: 06.1 REVIEW M-8 polling-loop ADOPTED-PROPHYLACTICALLY (cite Task 15 commit) — does NOT close M-8 against fixture 0005; 05.2 M-4 / M-10 / M-12 + 05.2 prose Minors + 06.1 12 Minors all unchanged in disposition.
- Four-ADR landing summary: ADR-0066..ADR-0069 anchoring tasks (Task 2 / Task 5 / Task 7 / Task 15) + commit SHAs.

- [ ] **Step 10: Commit**

The phase-done commit message per BOOTSTRAP §5.3 names every ADR introduced or referenced. THIS commit is the implementation session's last commit (Task 16); the phase-done commit is the REVIEW session's commit (which carries `phase 06.2: phase-done — access-log lands; ROADMAP rows 06.2 + 06 → done [ADR-0066, ADR-0067, ADR-0068, ADR-0069]` per SPEC §3). Task 16's commit message:

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/phases/06.2-access-log/PROGRESS.md
git commit -m "phase 06.2: BEHAVIOR_CONTRACT in-place edit + all-gates green local sweep (a/b/c/d/e green; f deferred to REVIEW) [ADR-0066, ADR-0067, ADR-0068, ADR-0069]"
```

(The commit-message-completeness check from BOOTSTRAP §5.3 is satisfied by the bracketed ADR list. Both ROADMAP.md updates AND the parent ROADMAP row 06 update are deliberately NOT included in this commit's `git add` — those updates land at the REVIEW session's phase-done commit per parent SPEC §5.)

- [ ] **Step 11: Confirm phase-06.2 readiness for state-5 transition (do NOT advance STATE — that's the verification session per BOOTSTRAP §5)**

The implementation session ends with Task 16 committed on `phase/06.2-access-log-impl`. STATE advancement through 5 → 6 is per-session work, not this task's responsibility.

*Anchored: SPEC §1 #5, §3 (six-gate phase-done), §4.4 (ROADMAP/STATE/PROGRESS lifecycle), §13 (BEHAVIOR_CONTRACT additions), §15 (full acceptance checklist), and BOOTSTRAP §5.3 (commit-message-completeness), §7.5 (six-gate sweep).*

---

## Refinement

This section absorbs the conventions that the 06.1 PLAN's Refinement section codified for execution-time consistency. Every item below applies to phase 06.2 unless explicitly noted.

**SHA-fill follow-up convention (per phase-02 / 03 / 04 / 05.1 / 05.2 / 06.1 precedent).** Every task's commit lands the task's main change; immediately after, a follow-up tiny commit `phase 06.2: PROGRESS SHA-fill for Task N` updates that task's PROGRESS.md `**Commits:**` line with the just-landed short SHA. The follow-up commit's body is empty; its title is the only line. Two commits per task; the executor MUST NOT skip the follow-up.

**BEHAVIOR_CONTRACT in-place edit lands at the Task 16 commit (per ADR-0052).** The `## Access log field mapping` placeholder population lands at Task 16's commit, NOT at any earlier task's commit. Per ADR-0052 the in-place edit is authorised; per SPEC §4.4 the timing is "at the phase-done commit" — but per BOOTSTRAP §5 step 6 the phase-done commit is the REVIEW session's, NOT the implementation session's; the BEHAVIOR_CONTRACT edit anticipates the REVIEW session by landing at Task 16 (the implementation session's last commit) so the verification session can grep-check the edit before REVIEW runs. Mirrors the 06.1 PLAN's identical convention.

**Empirical-format-pin scrape lands at Task 3 (NOT at Task 15).** Per `## Settled SPEC §12 deferred decisions` #9, the empirical scrape against reference Envoy v1.37.2 executes at Task 3 step 3 (the formatter implementation task), filling the SPEC §11 placeholder. This anchors the formatter's correctness BEFORE the unit tests are written; deferring to Task 15 would risk a Task-3-redo if a delimiter discrepancy surfaces during fixture construction.

**M-8 prophylactic-adoption rationale.** The 06.1 REVIEW Minor M-8 carry-forward (drain-loop polling vs. arbitrary `time.Sleep`) is ADOPTED PROPHYLACTICALLY by fixture 0006's driver design (Task 15 step 7's polling-loop pattern). This does NOT close M-8 against fixture 0005 (its actual fix lands in a 06.1 review-followup batch); it establishes the pattern for new fixtures going forward. The bundled-adoption is documented in the Task 15 PROGRESS entry's M-8 cross-reference and in the Task 16 carry-forward triage log.

**ROADMAP row 06.2 → in-progress at the SPEC commit (already landed); → done at the phase-done commit (with parent row 06 closing AT THE SAME COMMIT).** Per BOOTSTRAP §4.1 invariant 3: at the SPEC commit (already landed at master `4062c65`, before this PLAN commit), row 06.2 flipped `planned → in-progress` — the SPEC-authoring session did this. Per SPEC §4.4: at the phase-done commit (the REVIEW session's lifecycle-state-6 commit, NOT Task 16), row 06.2 flips `in-progress → done` AND parent row 06 ALSO flips `in-progress → done` AT THE SAME COMMIT (mirroring the 05/05.1/05.2 closure pattern). Task 16's commit deliberately does NOT touch ROADMAP.md; the anticipated text is recorded in the PROGRESS Task 16 entry but lands at the REVIEW session's phase-done commit. The phase-done commit-message body explicitly names BOTH ROADMAP-row transitions per SPEC §3's commit-subject template.

**ADR-numbering monotonicity discipline (ADR-0066..ADR-0069 contiguous).** Per ADR-0004's autonomous-numbering rule, the planner verified at PLAN-write time that the DECISIONS.md tail is `ADR-0065`; phase 06.2's four ADRs land at ADR-0066..ADR-0069 (contiguous block). Per `## ADRs introduced by this plan` above, the commit-time ordering (Task 2 / Task 5 / Task 7 / Task 15) produces non-monotonic ADR-number-vs-commit-order (0066, 0069, 0067, 0068), permitted per SPEC §8 and the 05.2 + 06.1 precedents. The contiguous-block discipline is preserved (no gaps); each ADR's `Lands-in-task` field records the in-task anchoring. The Task 1 step 1 precondition re-verifies the tail; if ADR-0065 has been superseded by a mid-PLAN-authoring ADR, every task's ADR reference shifts uniformly (planner verified at PLAN-write time that no such ADR exists; the precondition re-check is defence-in-depth).

**Commit-message-completeness check (per BOOTSTRAP §5.3).** Each task's commit message names the ADR(s) introduced in that task (in `[ADR-NNNN]` square-bracket form per the phase-04/05.1/05.2/06.1 convention). The Task 16 closing commit (per Step 10) names ALL FOUR ADRs in the bracketed list — `[ADR-0066, ADR-0067, ADR-0068, ADR-0069]` — so a `git log --grep='ADR-006[6-9]'` query surfaces every authoring task plus the closing task. The phase-done commit (REVIEW session's) carries the same bracketed list per SPEC §3.

**Six-gate local sweep at Task 16 (per BOOTSTRAP §7.5; SPEC §3).** Gates (a) / (b) / (c) / (d) / (e) all run at Task 16; gate (f) defers to REVIEW. The PROGRESS Task 16 entry quotes each gate's last-30-lines output verbatim. The Task-16 step 4 boundary grep + the step 5 four-emit-hook-site grep are SPEC §15-anchored acceptance bullets that the verification session re-runs.

**H2 ctx-cancel sentinel discipline.** The H2 ctx-cancel path in `routerActionH2.doH2` returns `(0, h2.NewStreamError(...))`. The deferred `emitAccessLogH2` checks the zero-status sentinel and SKIPS submission per SPEC §2.1's "no record on ctx-cancel" semantics. This matches Envoy's behavior (no access-log record for cancelled requests) and the discipline is preserved by Task 13's emit-deferral implementation. The PROGRESS Task 13 entry explicitly verifies the discipline via the unit test `TestRouterActionH2_DoH2_StatusZeroSkipsEmit`.

**No-third-party-access-log-library acceptance (per ADR-0066 + SPEC §15).** Task 16 step 4's grep is the gate; the executor CONFIRMS no `logrus`/`zap`/`zerolog`/`fluent` import lands in any production-code path. Test-side use is also forbidden — the fixture-0006 driver parses access-log lines with a small in-fixture regex, NOT via a third-party library. The grep applies uniformly across `_test.go` and production code.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 16 on `phase/06.2-access-log-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/06.2-access-log-impl
   ```
2. **The verification session** (next-fresh from the implementation session) re-runs all six gates per BOOTSTRAP §7.5 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. Verification commits `phase 06.2: STATE.md → lifecycle-state 5` on master.
3. **The REVIEW session** (next-fresh from verification) writes `docs/envoy-go/phases/06.2-access-log/REVIEW.md` per BOOTSTRAP §5 state 5 → 6. The REVIEW session's phase-done commit (per SPEC §3 commit-subject template):
   - Flips ROADMAP row 06.2 → `done`.
   - Flips ROADMAP parent row 06 → `done` AT THE SAME COMMIT (mirroring the 05 / 05.1 / 05.2 closure pattern per parent SPEC §5).
   - Lands the BEHAVIOR_CONTRACT verification block (a re-grep that the Task 16 edit landed correctly).
   - Advances STATE to phase 07 (`active-phase: 07-filter-chain-framework`; `lifecycle-state: 0` or `1` per the §5 state machine; `next-skill: superpowers:brainstorming`) at the SAME phase-done commit.
   - Commit message: `phase 06.2: phase-done — access-log lands; ROADMAP rows 06.2 + 06 → done [ADR-0066, ADR-0067, ADR-0068, ADR-0069]`.

**No part of this section is done by Task 16.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/06.2-access-log-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` (per ADR-0005 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md), `next-skill-scope: <execute PLAN.md per the 16-task sequence; create worktree .worktrees/phase-06.2-access-log-impl per ADR-0003>`, `last-commit: TBD` (the SHA-fill follow-up commit lands the actual SHA per the phase-02..06.1 SHA-fill precedent).
3. Fast-forward `master` to `phase/06.2-access-log-plan` per ADR-0003 (parent session's responsibility).
4. Worktree for the next session: `.worktrees/phase-06.2-access-log-impl` on branch `phase/06.2-access-log-impl` (recommended per `## Execution preconditions` #1).
5. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on `phase/06.2-access-log-plan`). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (`internal/accesslog/{accesslog,format,writer,stats,doc,fuzz_test}.go` and the test pairs; `internal/filter/hcm/{accesslog_emit,bytecounter}.go` and the test pairs; the four emit-hook deferrals across `actions.go` + `h2dispatch.go`; the bootstrap parser extension; the `Cluster.Dial`/`DialH2` return-tuple expansion; the `cmd/envoy-go/main.go` boot wiring; the `internal/stats/name.go` helpText extension; fixture 0006 in full; runner registration; four ADRs ADR-0066..ADR-0069; `BEHAVIOR_CONTRACT.md ## Access log field mapping` in-place edit.)
- Does the PLAN settle every 06.2-scoped SPEC §12 deferred decision? (8 items — see `## Settled SPEC §12 deferred decisions`.)
- Does the PLAN mitigate every SPEC §14 testing-strategy item with a task-level step? (14.1 unit tests for `internal/accesslog/` → Tasks 2-5; 14.2 unit tests for `internal/filter/hcm/` → Tasks 9-13; 14.3 unit tests for existing-package extensions → Tasks 7/8; 14.4 differential fixture 0006 → Task 15; 14.5 h2spec re-run → Task 16 step 3; 14.6 fuzzers → Tasks 6/16; 14.7 race detector + lint → Task 16 step 3.)
- Does the PLAN preserve the empirical-format-pin discipline? (Task 3 step 3 fills SPEC §11 + Task 16 step 1 mirrors into BEHAVIOR_CONTRACT; the two blocks are synchronized — no drift.)
- Does the PLAN preserve the H2 ctx-cancel zero-status sentinel? (Task 10 step 3 codes the guard; Task 13 step 3 verifies via TestRouterActionH2_DoH2_StatusZeroSkipsEmit.)
- Does the PLAN preserve the carry-forward triage from SPEC §10? (06.1 REVIEW M-8 ADOPTED-PROPHYLACTICALLY at Task 15 — does NOT close M-8; 05.2 + 06.1 minors all unchanged in disposition.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 3 format + empirical-pin scrape, Task 12-13 emit-hook deferrals, Task 15 fixture infrastructure, Task 16 final sweep)?
- Does the ADR number sequence match the verified DECISIONS.md tail? (ADR-0065 → ADR-0066..0069; non-monotonic mapping by topic-vs-first-use-order documented above.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~3000 LoC, 16 tasks, no further coherent split axis exists; per phase-04 / 05.1 / 05.2 / 06.1 precedent, one-sub-phase shipment is correct.)
- Does the import topology stay clean? (`internal/accesslog/` is a near-leaf importing only stdlib + `internal/stats`; `internal/filter/hcm/`, `internal/bootstrap/`, `cmd/envoy-go/` import `internal/accesslog/`; no third-party access-log library; the boundary grep at Task 16 step 4 enforces.)
- Are spec-review advisory items addressed? (Three planner-time items in `## Spec-review advisory responses`.)
- Are the four ADRs internally consistent? (ADR-0066's no-third-party-library decision matches Task 16 step 4's grep; ADR-0067's reject-log_format matches Task 7's parse fail-paths; ADR-0068's three-tier matrix matches Task 15's driver assertions; ADR-0069's `server.accesslog_dropped` SN5 mapping matches the `internal/stats/name.go` helpText entry at Task 5.)

