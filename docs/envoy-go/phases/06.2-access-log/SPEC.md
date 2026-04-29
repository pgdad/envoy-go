# Phase 06.2 — Access log (`internal/accesslog` package, HCM emit hooks, fixture 0006)

**Phase id:** `06.2`
**Slug:** `06.2-access-log`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close artifact into formal SPEC shape per ADR-0004)
**Depends on:** phase 06.1 (done at master `9acfc0b` via 06.1's phase-done commit `ae8276b` and master fast-forward; the `internal/stats` Registry is the optional consumption surface for the drop-counter wiring per Decision B)
**Parent phase:** `06-observability-baseline` (in-progress; closes at THIS phase's phase-done commit, mirroring the 05 / 05.1 / 05.2 closure pattern recorded in `STATE.md` at master `75a6bf9` and in the parent SPEC §5)
**Master design document:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` and `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (brainstorm-close artifact, master `75a6bf9` parent). The BRAINSTORM is the upstream design source; this SPEC distills the access-log decisions A–M from the lifecycle-state-0 brainstorm session that immediately precedes this commit (held against master `9acfc0b`) into formal contract language. The phase-06 BRAINSTORM §1 split-table fixed the per-sub-phase scope; the phase-06 BRAINSTORM defers ALL access-log architecture decisions to this sub-phase — those decisions are now settled and are this SPEC's contract.
**Differential surface at end of sub-phase:** NEW fixture `test/fixtures/0006-access-log/` is differentially green (gate (a) is **non-vacuous** for the second time on the observability surface — 06.1's was the first): 5 sequential GET requests per side (10 records total) issued through both proxies; per-side Envoy-default-format access-log file is scraped and parsed into a positional 15-tuple per record; per-field equivalence is asserted under the three-tier matrix (Tier E byte-equal, Tier F format-only, Tier S subject-emits-`-`) per Decision D below. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats` remain green (gate (b)). h2spec conformance gate (c) re-runs at the ADR-0051 pin and stays at 53/53 PASS (the 06.2 surface is observability-only and does not touch H2 wire code; the change is purely additive). Fuzz (d) re-runs the existing seven fuzzers AND adds a new `FuzzAccessLogFormat` over the access-log default-format writer at the 30s ADR-0018 budget. Build/vet/lint/test (e) and REVIEW (f) apply normally. ROADMAP rows `06.2` AND parent `06` flip `in-progress → done` AT THE SAME phase-done commit per the parent SPEC §5 / 05.2 closure precedent.

---

## 1. Purpose

Phase 06.2 lands envoy-go's access-log subsystem — an in-tree file-sink-and-default-format-formatter package backed by an async writer with bounded-channel backpressure — and threads access-log-emit hooks through the four request-finalization sites in HCM (H1 direct_response, H1 router action, H2 direct_response, H2 router action) so a per-request access-log line is appended to a configured file sink per the Envoy default format. The records are differentially equivalent to upstream Envoy v1.37.2 under a defined load on the per-field three-tier matrix from Decision D below. Concretely:

1. **A new `internal/accesslog` package** — `Sink` interface, `Record` struct (the per-request primitives populated by HCM at finalization-time), `Default()` formatter that emits the literal Envoy-default-format 15-operator line shape, and `AsyncFileSink` (a goroutine-backed writer that drains a bounded channel of records to an `os.File` opened `O_APPEND|O_CREAT|O_WRONLY` mode 0644). The placeholder `internal/accesslog/doc.go` (a phase-00 stub that currently reads "The real implementation lands in phase 06") is replaced by full package documentation describing the API and lifecycle. **No third-party access-log library** (no logrus, zerolog, or fluentd-style dependency); the formatter and writer are ~250 LoC in-tree. The architectural rationale is recorded in ADR-0066 anticipated (per §8 below) and mirrors 06.1 ADR-0059's same-shape rationale for the stats subsystem.

2. **Field coverage: option B (partial-with-`-`-for-unsupported)** per Decision A. The 15-operator default-format line is emitted in identical positions on every record. **Operators 06.2 plumbs (10):** `START_TIME`, `:METHOD`, `:PATH` (via the `?:` fallback when `X-ENVOY-ORIGINAL-PATH` is absent — which it is on every fixture-0006 record on both sides; see §6.1 below), `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT` (response body bytes), `DURATION` (downstream-request wall-clock ms), `:AUTHORITY`, `USER-AGENT`, `UPSTREAM_HOST` (resolved endpoint `host:port`; literal `-` for `direct_response` paths). **Operators 06.2 emits as the literal `-` Envoy-missing-value convention (5):** `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)`, `X-FORWARDED-FOR`, `X-REQUEST-ID`. The 10 + 5 = 15 operator count is the Envoy default format's full operator count (per the empirical pin scrape in §11 below); the SPEC ships the line *shape* exactly, with five fields explicitly documented as not-plumbed-in-06.2.

3. **Async-writer backpressure: drop-newest with bounded channel** per Decision B. `AsyncFileSink`'s submit channel has capacity 4096 records (well above the ~5-record fixture workload per side; the differential never exercises the drop path). On a full channel, the non-blocking `select`-with-`default` send fails: the new record is dropped, the `server.accesslog_dropped` counter (allocated against 06.1's `*stats.Registry` per Decision I's ADR-0069 + SN5 mapping) is `Inc`'d, and a throttled `log.Printf` emits at most one diagnostic line per second (rate-limited by a `sync.Once`-style deadline-tracking field on the sink). **No queue-depth gauge** — adding one would force an `atomic.LoadInt64` on every submit, contrary to the lock-free hot-path discipline 06.1 ADR-0059 established.

4. **Format-config rejection: option β** per Decision C. The bootstrap parser READS the HCM `access_log[]` field as a list of any length (0 → no-op; N → emit to all N sinks per request, in registration order — no artificial 1-cap). Each entry must carry a typed-config of type `envoy.access_loggers.file` with: `path` (required, non-empty string) AND no `log_format` field (any presence — `log_format`, `format_string`, `json_format` — produces a parse-time fatal error: `unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)`). Other typed-config types (`envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`, etc.) remain silently-ignored as before, per ADR-0067 anticipated. This is the boundary-validation pattern from 06.1 ADR-0065 (`stat_prefix` regex check at the bootstrap-input boundary) extended to access-log config: validate at parse-time so that fail-loud fires before any per-request writer is constructed.

5. **Per-field equivalence: 3-tier matrix** per Decision D, codified in the populated `BEHAVIOR_CONTRACT.md ## Access log field mapping` subsection per §13 below. **Tier E (exact byte-equal cross-side) — 8 operators:** `:METHOD`, `:PATH`, `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)` (both `-`), `USER-AGENT`, `:AUTHORITY`. **Tier F (format-only — parses to expected shape on both sides; cross-side value not asserted equal) — 3 operators:** `START_TIME` (RFC3339 ms-precision UTC, within the workload's wall-clock window), `DURATION` (int ms ≥ 0), `UPSTREAM_HOST` (`host:port` for routed; `-` for direct_response). **Tier S (subject MUST emit `-`; reference unconstrained) — 4 operators:** `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID`. Counts: 8 + 3 + 4 = 15 = the operator count in the format. The 3-tier shape is the access-log analog of 06.1 ADR-0062's stats-output behavioral-delta shape — semantically equivalent without requiring byte-exact whole-record equality, and codified in ADR-0068 anticipated.

6. **Architecture (package shape) per Decision E.** New package `internal/accesslog/` ships `accesslog.go` (Sink interface + Record struct), `format.go` (the `Default()` formatter), `writer.go` (`AsyncFileSink`), `stats.go` (the `server.accesslog_dropped` counter wiring), `doc.go` (rewritten from the phase-00 placeholder), and full unit + fuzz test coverage. New file `internal/filter/hcm/accesslog_emit.go` carries `Filter.emitAccessLog(...)` which constructs the `Record` from the four primitives captured by each finalization site (`start time.Time`, `bytes int64`, `picked cluster.Endpoint` zero-valued for direct-response paths, the request + response status code) and submits to each `Sink` in `filter.accessLog`. New file `internal/filter/hcm/bytecounter.go` carries the ~10-LoC `byteCounterWriter` (an `io.Writer` wrapper that maintains an `int64` running total of bytes written). The four finalization sites in `internal/filter/hcm/actions.go` and `internal/filter/hcm/h2dispatch.go` add `defer filter.emitAccessLog(...)` calls — see §5.4 below for exact site enumeration. **Differential coverage split:** the two H1 sites (`directResponseAction.do`, `routerAction.do`) are exercised **differentially** by fixture 0006 (per §7.2 below; the fixture declares `codec_type: HTTP1`). The two H2 sites (`h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`) are exercised by **unit tests** in `internal/filter/hcm/accesslog_emit_test.go` (per §14.2). This mirrors 06.1's H1-only fixture-0005 precedent (06.1's stat-emit hot path is exercised differentially through the H1 codec; H2 hot-path emission is unit-tested under fixture 0004's separate H2-routing differential surface). A future observability sub-phase may add an H2 leg to fixture 0006 (or a sibling H2-bearing fixture) once H2-specific access-log behavior becomes differentially-distinguishable.

7. **Sink lifecycle.** Sinks are opened in `cmd/envoy-go/main.go` between `bootstrap.Load(...)` and `listener.Run(...)`, threaded into the filter chain via the existing `internal/filter/hcm/config.go` plumbing (mirrors how the 06.1 Stats Registry was threaded through `cluster.NewManager` / `listenerManager.New` / `admin.New`). On `defer sink.Close()` after `listener.Shutdown()` returns (process-exit teardown), the channel drains and the file-descriptor closes. **Drain semantics for graceful shutdown (request-in-flight at SIGTERM; observe-the-pending-record-then-close vs. drop-on-cancel) are PHASE 08's deliverable;** 06.2 closes at process exit only. State this explicitly per Decision E to keep the phase boundary clean.

8. **Carry-forward dispositions** per Decision M. The 05.2 carry-forwards (M-4 `readClientPreface` ctx-awareness, M-10 `SETTINGS_TIMEOUT` absent, M-12 `closedStreams` unbounded, the seven 05.2 prose Minors): unchanged carry-forward state — 06.2 doesn't address them; the tag continues `phase-07-or-later-must-consider`. The 06.1 12 Minors (M-2..M-12 plus the post-phase-done reviewer-discovered Minor): 06.1's L4 review-followup batch responsibility (separate post-phase-done branch — established 05.1/05.2 pattern), NOT 06.2's. **EXCEPTION:** 06.1 REVIEW M-8 ("hardcoded 200ms drain may flake on slow CI; recommend polling loop instead") — 06.2's fixture-0006 driver adopts the polling-loop pattern natively (Decision G drain discipline). This **does NOT close M-8 itself** (which targets fixture 0005's existing driver), but establishes the pattern for new fixtures going forward. Tag: "adopted prophylactically by 06.2 design".

9. **A new differential fixture `test/fixtures/0006-access-log/`** — the project's second observability-surface differential fixture and the first asserting per-record field-by-field equivalence between subject and reference access-log files. Driver mounts a per-side log file (subject opens `<t.TempDir()>/subject.log` directly; reference container bind-mounts `<t.TempDir()>/reference.log` to `/tmp/envoy-access.log` via `testcontainers-go` `Mounts`), drives 5 sequential GETs per side (10 records total), polls both files at 25ms intervals until each reaches `≥ 5` lines (hard deadline 5s), parses each line into a positional 15-tuple via a regex anchored on the format's literal delimiters (`[`, `]`, `"`, space), and asserts per the three-tier matrix. Fixture detail in §7 below.

10. **A new fuzz target `internal/accesslog.FuzzAccessLogFormat`** at the 30s ADR-0018 budget per Decision J. Random `Record` field values include control characters, 8-bit bytes, large strings, `\n` / `\r` / `"` in headers. The fuzzer asserts: (i) the formatter NEVER produces a record with embedded LF (the access-log line terminator IS `\n`; embedded LFs would corrupt the record stream by appearing as record boundaries) — escaping discipline; (ii) quoted operators escape literal `"` to `\"` per Envoy's convention. **Eighth fuzzer overall** (joins the seven from 06.1).

11. **Anticipated ADRs:** four ADRs per Decision I, numbered ADR-0066..ADR-0069 (next-free per the `DECISIONS.md` tail at master `9acfc0b` being ADR-0065 — re-verified at SPEC-write time per ADR-0004's autonomous-numbering rule; brainstorm-time also recorded ADR-0066 as next-free). Topics enumerated in §8 below.

After phase 06.2, the project has proven the second half of its seventh central engineering claim: *envoy-go emits behaviorally-equivalent operator-grade access-log records under a defined load — visible at a configured file sink, semantically equivalent in field shape and value to upstream Envoy v1.37.2's default-format records on the per-field three-tier matrix — without coupling to any third-party access-log dependency.* The parent ROADMAP row `06` flips to `done` at THIS phase's phase-done commit per the parent SPEC §5 closure pattern.

## 2. Non-purposes

Phase 06.2 does **not** do any of the following. Most are explicit non-goals from the parent BRAINSTORM §1's split table or the lifecycle-state-0 brainstorm session that produced this SPEC; a few are scope-narrowings the SPEC introduces by consolidating brainstorm-time deferrals. Each non-purpose is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Access-log coverage non-goals (per Decision A's option-B partial-coverage choice + parent README §"OUT of 06.2")

- **Custom format strings.** The bootstrap proto's `log_format` / `format_string` / `json_format` typed-config fields are explicitly **rejected at parse time** with a fatal error per Decision C / ADR-0067. No `text_format`-style command-operator parser ships in 06.2. → A future phase (or an Observability-family extension) ships the format-string parser; 06.2's design assumption is that the implicit default format is sufficient for the differential surface.
- **Operators not plumbed (5 of 15).** `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)`, `X-FORWARDED-FOR`, `X-REQUEST-ID` are emitted as the literal `-` (Envoy missing-value convention) per Decision A. Plumbing them would require: (a) a response-flags state machine threaded through the action-result codes (router/dial/codec failure classes); (b) downstream-request body-size accounting through the H1/H2 read-side; (c) an upstream-injected response-header observer; (d) request-header injection of `X-Forwarded-For` / `X-Request-ID`. Each of these is a ~50-200 LoC surface change in its own right. → A future observability-extension phase plumbs each operator individually.
- **Other access-log sinks** beyond `envoy.access_loggers.file`. `envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`, gRPC ALS — all silent-ignored at the bootstrap parser per Decision C. → Observability-family deliverables.
- **Per-route access-log filters.** The bootstrap proto's `access_log[].filter` field (per-record predicate filtering — only emit records matching a status-code range, header pattern, etc.) is silently ignored. The 06.2 surface emits unconditionally per request finalization. → Phase 07's filter-chain framework.
- **Log rotation.** Size-based / time-based / signal-driven (SIGHUP) log-file rotation is out of scope. The file is opened once at boot, appended to per request, closed at process exit. → Future phase or operational tooling (logrotate).
- **`fsync` / durability ceiling.** No per-record `fsync`. The OS page cache is the durability ceiling. Matches Envoy. → If a future workload demands stronger durability, an opt-in fsync-on-close discipline lands behind a config flag in a later phase.
- **Trailers in access logs.** Per ADR-0058 carry-forward, trailers are observed but not forwarded; their presence does not surface in any access-log operator. → gRPC family.
- **Access-log records for incomplete (no-response) requests.** A request that is half-read when downstream RST/FIN-cancels (the H2 ctx-cancel path in `routerActionH2.doH2`) does not produce an access-log record in 06.2 — the deferred `emitAccessLog` runs only after the action returns a finalized status code. The H2 ctx-cancel path returns `(0, h2.NewStreamError(...))` per `actions.go:240-247`; the deferred emit checks for the zero-status sentinel and skips emission. → A future observability-extension phase may surface ctx-cancel as a synthetic `0` status with an explicit response-flag.

### 2.2 Format-fidelity non-goals (per Decision D's three-tier choice)

- **Cross-side `START_TIME` byte-equality.** Two proxies will not produce the same wall-clock timestamp; the equivalence claim relaxes to format-only (Tier F) — the field parses to RFC3339 ms-precision UTC on both sides AND falls within the workload wall-clock window.
- **Cross-side `DURATION` byte-equality.** Per-request wall-clock is non-deterministic; the equivalence claim relaxes to format-only (Tier F) — the field parses to a non-negative integer (ms).
- **Cross-side `UPSTREAM_HOST` byte-equality.** Reference Envoy may resolve `host.docker.internal` to a different IPv4 address than the subject's STATIC endpoint resolves to (per ADR-0010); the equivalence claim relaxes to format-only (Tier F) — the field parses to either `<host>:<port>` (routed) or the literal `-` (direct_response) on both sides; cross-side address byte-equality is not asserted.
- **Tier-S fields.** `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID` — subject must emit `-` per Decision A; reference is unconstrained (Envoy emits these on a real workload). The differential parser ignores the reference's value on these four fields.
- **`X-ENVOY-ORIGINAL-PATH` fallback half of the `:PATH` operator.** Operator #3 in the format is `%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%` — emit the original-path header if present, else fall through to `:PATH`. **Neither side emits `X-ENVOY-ORIGINAL-PATH` on fixture 0006's workload** (envoy-go does not inject it; reference Envoy doesn't either, because the request never traverses a route with `path_rewrite`); both sides therefore emit `:PATH`'s value via the fallback. The SPEC documents this explicitly so that a future phase that introduces path-rewriting (and thus exercises the `X-ENVOY-ORIGINAL-PATH` half) does NOT accidentally regress fixture 0006 — when that surface lands, fixture 0006's expectations need a Tier-E/F re-evaluation under the new behavior.

### 2.3 Process / lifecycle non-goals

- **Graceful drain of pending records.** SIGTERM-while-record-pending semantics (drop, drain-and-close-with-bounded-deadline, block) is PHASE 08's deliverable. 06.2 closes the sink at process-exit only; in-flight records may be dropped if the channel is non-empty at `Close()` time. → Phase 08 owns drain.
- **Authentication / access-control on the file sink.** The file is owned by the process user; standard POSIX file-permissions (mode 0644 default) apply. No ACL, no SELinux integration, no audit-log mode. → Operational tooling.
- **Concurrency model for two-or-more sinks.** The SPEC supports `access_log[]` of any length per Decision C, but each sink runs an independent goroutine. Cross-sink ordering is per-sink only; no global record-ordering invariant across sinks. (Single-sink intra-record ordering matches the per-request finalization sequence.) → Forward-looking; matches Envoy.

### 2.4 Carry-forward non-purposes (per Decision M)

The following 05.2 + 06.1 REVIEW Minor findings are **explicitly deferred** OUT of 06.2 (they are NOT bundled with 06.2):

- **05.2 M-4** `readClientPreface` not ctx-aware — H2 connection hardening, not access-log. → dedicated H2-hardening sub-phase or upstream-robustness family.
- **05.2 M-10** `SETTINGS_TIMEOUT` absent — same reasoning. → same target-phase candidates.
- **05.2 M-12** `closedStreams` map unbounded — long-lived-conn memory growth, not access-log. → upstream-robustness family.
- **05.2 prose Minors (7 items)** — unchanged carry-forward state.
- **06.1 12 Minors (M-2..M-12 plus reviewer-discovered)** — separate 06.1 post-phase-done batch (the established 05.1/05.2 review-followup branch pattern); NOT 06.2's responsibility.

The full disposition table is recorded in §13 below for the reviewer's audit trail. The one EXCEPTION (06.1 REVIEW M-8 — drain-loop polling) is **adopted prophylactically by 06.2's fixture-0006 driver design** (Decision G) but does not close M-8 itself; M-8 stays open against fixture 0005.

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 06.2)

Per doctrine `D-3.6`, phase 06.2 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 06.2 |
|---|---|
| (a) new/changed differential fixtures green | **Non-vacuous (second time on the observability surface, after 06.1's fixture 0005).** New fixture `test/fixtures/0006-access-log/` passes: 5 sequential GET requests per proxy with target paths `[/health, /api/v1/foo, /api/v1/bar, /api/v1/baz, /notfound]` (2 × `direct_response`: 1 × 200 `OK\n` and 1 × 404; 3 × routed: 200 each through cluster `c_backend` over 3 endpoints in RR); per-side access-log file scraped + polled-until-`≥ 5`-lines (hard deadline 5s) + parsed into a positional 15-tuple per record + asserted under the three-tier matrix from Decision D. The expected-records table from §7.4 is the contract; non-listed operators (none — every record has all 15 operators per the format) are not skipped. |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats` all pass without regression. The phase-06.2 changes are additive — `cluster.NewManager` / `listenerManager.New` / `admin.New` / `bootstrap.Load` signatures are unchanged from 06.1's; the new wiring is `cmd/envoy-go/main.go` allocating the sink-slice from bootstrap and threading it into the HCM filter chain via the existing `internal/filter/hcm/config.go` plumbing. Pre-existing fixtures' driver code is unchanged because 06.2's bootstrap parser silently no-ops when `access_log[]` is empty / absent. |
| (c) conformance suites pass | `test/conformance/h2spec/` re-runs at the ADR-0051 pin (`summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md`) and reports `failed == 0` over the unchanged threshold list (sections 3, 4, 5, 6 ex-6.6, 7, 8 — 53/53 PASS at the 05.1+05.2+06.1 baseline). 06.2 doesn't touch H2 wire code, so this gate is unchanged. Pin is NOT bumped (D-3.7 reserves pin bumps for dedicated phases). |
| (d) new/existing fuzzers run clean for CI short-budget | Existing seven fuzzers (`internal/bootstrap.FuzzBootstrapLoad`, `internal/filter/tcpproxy.FuzzTcpProxyFilter`, `internal/tls.FuzzTLSContextParse`, `internal/filter/hcm.FuzzHCMConfigParse`, `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode`, `internal/stats.FuzzPromTextFormat`) run clean at the 30s ADR-0018 budget. **NEW:** `internal/accesslog.FuzzAccessLogFormat` runs clean at the same budget. Total: 8 fuzzers post-06.2. |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for the new `internal/accesslog` package (sink interface + record / format / writer / drop-counter wiring; race-clean; all-15-operators-emitted invariant); extended tests for `internal/bootstrap/` (parse + reject `log_format`; silent-ignore non-file types; 0-or-N sink list); extended tests for `internal/filter/hcm/` (the four finalization-site `defer emitAccessLog(...)` calls with byteCounter / endpoint / start-time capture); the byteCounterWriter unit test. `go test -race -count=1 ./...` clean — concurrent `Sink.Submit` from N goroutines, concurrent submit + writer-goroutine consumption, drop-newest backpressure under sustained-overload synthesis, sink `Close()` while submit is in-flight (drop-or-deliver semantics asserted). |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

The phase-done commit subject (per `BOOTSTRAP_PROMPT.md` §5.3) is: `phase 06.2: phase-done — access-log lands; ROADMAP rows 06.2 + 06 → done [ADR-0066, ADR-0067, ADR-0068, ADR-0069]`. The body explicitly names both ROADMAP-row transitions per parent SPEC §5.

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 06.2. The complete file inventory itemizes every signature change so PLAN time has no surprise scope (the same discipline 06.1 honored per its BRAINSTORM §3 note 6).

### 4.1 New production code (in 06.2)

- **`internal/accesslog/doc.go`** (rewrite) — describes the package API (Sink interface, Record struct, Default formatter, AsyncFileSink lifecycle, Close-drain semantics, the `server.accesslog_dropped` counter wiring) and the lifecycle invariant ("opened in `cmd/envoy-go/main.go` between `bootstrap.Load` and `listener.Run`; closed via `defer sink.Close()` after `listener.Shutdown` returns"). The placeholder `doc.go` already exists from a phase-00 stub (currently reads "The real implementation lands in phase 06"); this commit replaces its contents.
- **`internal/accesslog/accesslog.go`** — `type Sink interface { Submit(*Record); Close() error }` and `type Record struct { StartTime time.Time; Method string; Path string; Protocol string; ResponseCode int; BytesSent int64; Duration time.Duration; Authority string; UserAgent string; UpstreamHost string }` (10 fields — the 10 plumbed operators from Decision A; the 5 unplumbed operators are emitted as the literal `-` by the formatter without needing Record fields). The `Sink` interface is small and stable so future sinks (ALS, OTLP) can implement it without churn.
- **`internal/accesslog/format.go`** — `func Default(r *Record) []byte`: emits the literal Envoy default-format line shape (15 operators in identical positions, terminated with a single `\n`). Each plumbed operator pulls from the corresponding `Record` field; each unplumbed operator emits the literal `-`. Quoted operators (the request-line quoted block, `USER-AGENT`, `:AUTHORITY` — see §6 for the exact format) escape literal `"` to `\"` per Envoy convention. The empirical pin block in §11 below pins the operator order.
- **`internal/accesslog/writer.go`** — `type AsyncFileSink struct { ch chan *Record; f *os.File; done chan struct{}; dropped *stats.Counter; lastDropLog atomic.Int64 }`. Constructor `NewAsyncFileSink(path string, dropped *stats.Counter) (*AsyncFileSink, error)` opens the file `O_APPEND|O_CREAT|O_WRONLY` mode 0644 and starts the writer goroutine. `Submit(*Record)` non-blocking-sends on `ch` (capacity 4096); on full-channel, increments `dropped` and (rate-limited via `lastDropLog` with a 1 second interval) emits `log.Printf("accesslog: channel full, dropping record (path=%s)", path)`. The writer goroutine `for r := range ch { _, _ = f.Write(Default(r)) }` (single-consumer; per-record `os.File.Write` is atomic for sub-PAGE writes under `O_APPEND`). `Close()` closes `ch`, waits for `done`, then calls `f.Close()`.
- **`internal/accesslog/stats.go`** — small wiring file: `func RegisterDroppedCounter(reg *stats.Registry) *stats.Counter { return reg.NewCounter("server.accesslog_dropped") }` per Decision I's ADR-0069 (the SN5 server-scope mapping). Outside the 06.1 17-name allow-list, so the differential ignores the metric name; operator-visible only.
- **`internal/accesslog/accesslog_test.go`** — Sink interface unit tests; Record-construction shape; size of Record vs. allocation count.
- **`internal/accesslog/format_test.go`** — `Default()` happy-path emits the literal 15-operator line; quoted-operator escaping (literal `"` → `\"`); never produces an embedded LF; the 5 unplumbed operators emit `-` verbatim; empty-string fields emit `-` (per Envoy convention) where the operator semantically demands it (e.g., `USER-AGENT` empty → `-`); `UPSTREAM_HOST` empty (zero-value `Endpoint`) → `-`.
- **`internal/accesslog/writer_test.go`** — happy path: submit N records → file contains N lines; race-clean concurrent `Submit` from M goroutines; drop-newest backpressure under synthesized overload (channel pre-filled, the M+1th Submit increments the dropped counter); rate-limited diagnostic log (the second drop within 1 second does NOT emit a second log line); `Close()` after pending records: the writer drains the channel and writes the pending records before closing the fd.
- **`internal/accesslog/fuzz_test.go`** — `FuzzAccessLogFormat` per Decision J. Asserts: (i) the formatter NEVER produces a record with embedded LF (the access-log line terminator is `\n`; embedded LFs would corrupt the record stream); (ii) quoted operators escape literal `"` to `\"`. Random `Record` field values include control chars, 8-bit bytes, large strings, `\n` / `\r` / `"` in headers. 30s budget per ADR-0018. **Eighth fuzzer overall.**
- **`internal/filter/hcm/accesslog_emit.go`** — `func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time)`: constructs an `accesslog.Record` from the four primitives + the request fields (`r.Method`, `r.URL.Path`, `r.Proto`, `r.Host`, `r.Header.Get("User-Agent")`); `UpstreamHost` is the zero-value `Endpoint`'s `host:port` (empty string → formatter emits `-`) for direct_response paths or the picked endpoint's `host:port` for routed paths; iterates `f.accessLog` and `Submit`s the record to each sink. The H2-flavored variant accepts an `h2.H2Request` and reads the pseudo-headers (`:method`, `:path`, `:scheme`, `:authority`) instead of the H1 `*http.Request`.
- **`internal/filter/hcm/accesslog_emit_test.go`** — `emitAccessLog` unit tests: H1 + H2 record-shape correctness; status-code-zero (ctx-cancel) skips emission; `picked.Host == ""` (direct_response) renders as `UpstreamHost = ""` → formatter emits `-`; multiple sinks all receive the same record.
- **`internal/filter/hcm/bytecounter.go`** — `type byteCounterWriter struct { w io.Writer; n int64 }` with `Write(p []byte) (int, error) { n, err := bcw.w.Write(p); bcw.n += int64(n); return n, err }`. ~10 LoC. The writer wraps the per-action downstream writer (`bufio.Writer` for H1, `h2.StreamWriter` for H2 — H2 needs a separate adapter because its `WriteData` doesn't conform to `io.Writer`; the H2 emit path tracks bytes via the action's known-body-size sum).
- **`internal/filter/hcm/bytecounter_test.go`** — happy path; `n` accumulates correctly across multiple `Write`s; partial-write with `n < len(p)` (writer returns short — `bcw.n` reflects the short count).

### 4.2 Changed production code (in 06.2)

- **`internal/bootstrap/bootstrap.go`** — extended to parse the HCM `access_log[]` field. Each entry's typed-config is matched against `envoy.access_loggers.file`: if matched, `path` is read (required, non-empty string); if `log_format` / `format_string` / `json_format` is present, parse fails with `unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)` per Decision C / ADR-0067. Other typed-config types are silently-ignored (the existing silent-ignore set, amended). The `Bootstrap` struct gains an `AccessLog []accesslog.Sink` field threaded into the HCM filter chain at construction-time. **Note:** the sink construction itself happens in `cmd/envoy-go/main.go` (which has access to the file system); the bootstrap parser captures the parsed config (paths + drop-counter Registry handle) and `main.go` opens the sinks. This factoring keeps `bootstrap.Load` testable without filesystem side effects.
- **`internal/bootstrap/bootstrap_test.go`** — extended: parse-success on a config with one `access_log[]` entry of type `envoy.access_loggers.file`; parse-success on 0 entries (no-op); parse-success on N entries (all opened); **parse-fail** on `log_format` field present (per Decision C / ADR-0067); parse-fail on `path` field absent or empty; silent-ignore on `envoy.access_loggers.stdout` / `envoy.access_loggers.tcp_grpc` / `envoy.access_loggers.open_telemetry`.
- **`internal/filter/hcm/filter.go`** — `Filter` struct gains an `accessLog []accesslog.Sink` field; the `emitAccessLog` method is added (in the new file `accesslog_emit.go` per §4.1). The HCM filter chain construction (per `internal/filter/hcm/config.go`) propagates the sink slice from the bootstrap into each per-listener filter; mirrors how the 06.1 Stats Registry was threaded in 06.1 Task 7.
- **`internal/filter/hcm/config.go`** — extended: `parseFilterWithCtx` (or its caller) accepts the `[]accesslog.Sink` slice from the bootstrap and stores it on the constructed `*Filter`. The signature change is a single additional parameter (tag-along with the existing 06.1 `*stats.Registry` parameter) — minimal surface area.
- **`internal/filter/hcm/actions.go`** — extended for the four request-finalization sites. The exact sites (per the `do` and `doH2` method names in the actual code, NOT the brainstorm-dispatch's "Run" abstract):
  - **`(*directResponseAction).do`** (H1 codec-neutral; the `directResponseAction` struct is shared between H1 and H2 actions via the `directResponseAction` value embedded in `h2DirectResponseAdapter` per `h2dispatch.go:55-58`) — the H1 path adds a `defer filter.emitAccessLog(...)`.
  - **`(*routerAction).do`** (H1 router action) — adds `start := time.Now()` at action entry; wraps the `bw` `bufio.Writer` in a `byteCounterWriter` to capture `BYTES_SENT`; captures the picked endpoint via `c.PickEndpoint()` (already called inside `Cluster.Dial`; the action receives a value via a small refactor — see §12 #2 deferred decision); `defer filter.emitAccessLog(r, statusCode, bcw.n, picked, start)` at action exit.
  - **`(*routerActionH2).doH2`** (H2 router action) — same primitive-capture pattern. The H2-specific deferral surfaces the H2-pseudo-header-bearing `req h2.H2Request` value to the emit hook (the hook inspects `:method` / `:path` / `:scheme` / `:authority` rather than `*http.Request`).
- **`internal/filter/hcm/h2dispatch.go`** — extended for the H2 direct_response site. `h2DirectResponseAdapter.WriteH2` (per `h2dispatch.go:89-95`) adds a `defer filter.emitAccessLog(...)` mirroring the H1 direct_response-path emit. The `h2RouterActionAdapter.WriteH2` (per `h2dispatch.go:117`) does NOT add an emit deferral here — the underlying `*routerActionH2.doH2` carries the deferral, and the adapter is just a wire-shape wrapper. (If a future refactor moves the deferral up to the adapter, that is a future-phase concern; 06.2 keeps the deferral at the action level for symmetry with H1.)
- **`cmd/envoy-go/main.go`** — extended: between `bootstrap.Load(...)` and `listenerManager.Run(...)`, iterate `bootstrap.AccessLogConfigs` (the parsed-but-not-yet-opened list of `(path, droppedCounter)` tuples); call `accesslog.NewAsyncFileSink(path, droppedCounter)` for each; collect the sink slice; thread the slice into `internal/filter/hcm/config.go`'s filter-chain construction. After `listener.Shutdown()` returns (process-exit teardown), `defer` each `sink.Close()` in registration order.
- **`internal/filter/hcm/filter_test.go`** — extended: new fixtures for the four-finalization-sites emit-deferral; mock-sink that captures records; assertions on Record shape per H1/H2 and direct_response/routed.

### 4.3 New harness and fixture code (in 06.2)

- **`test/fixtures/0006-access-log/`** — new fixture directory (under `test/fixtures/`, mirroring 0005's location convention; the brainstorm-dispatch's "test/fixtures/0006-access-log" path is honored). Contents:
  - **`envoy-go.yaml`** — subject bootstrap. 1 listener (`l_h1`) binding `127.0.0.1:0` plaintext (no TLS — 06.2 is observability-only). 1 filter chain with empty `filter_chain_match`. 1 HCM network filter with `codec_type: HTTP1`, `stat_prefix: ingress_http`, and an `access_log[]` list with one entry of type `envoy.access_loggers.file` whose `path` is the runner-supplied `<t.TempDir()>/subject.log`. 1 route_config with one `*` vhost holding three routes: `path: /health` → `direct_response 200 body: OK\n`; `path: /notfound` → `direct_response 404 body: not found\n`; `prefix: /api/v1/` → cluster `c_backend`. 1 STATIC cluster `c_backend` with 3 endpoints pointing at the controlled backends' ports.
  - **`envoy.yaml`** — reference bootstrap. Same listener / HCM / route_config shape. 1 STRICT_DNS cluster `c_backend` pointing at `host.docker.internal:<backend-N-port>` for N ∈ {0,1,2} with `dns_lookup_family: V4_ONLY` per ADR-0010; same `stat_prefix: ingress_http`. The `access_log[]` field has one entry of type `envoy.access_loggers.file` whose `path` is `/tmp/envoy-access.log` (bind-mounted by the runner to `<t.TempDir()>/reference.log` via `testcontainers-go` `Mounts`). The reference is invoked with `--concurrency 1` per ADR-0028.
  - **`expectations.yaml`** — prose description of the 5-request workload + the 15-operator three-tier matrix table (§7.4 below). Tier-E rows specify byte-equal cross-side; Tier-F rows specify the parser predicate; Tier-S rows specify subject-must-emit-`-`. The doc is the contract; the driver implements it.
  - **`README.md`** — explains the fixture's purpose (per-record three-tier field equivalence on the 15 operators; second observability-surface differential), the STATIC-vs-STRICT_DNS divergence, the 5-request workload shape, the per-side log-file mounting + polling convention (Decision G drain discipline), the cross-reference to `BEHAVIOR_CONTRACT.md ## Access log field mapping`'s 15-operator table + the three tiers.
  - **`driver/driver.go`** — `BackendCount() = 3`. `SubjectListenerName() = "l_h1"`. `ReferenceListenerPort() = 15006`. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 5 H1 GETs sequentially with paths `[/health, /api/v1/foo, /api/v1/bar, /api/v1/baz, /notfound]`. After the 5 responses are received, the driver polls the per-side log file at 25ms intervals (no arbitrary `time.Sleep`) until the file has `≥ 5` lines OR a 5s hard deadline is reached; on deadline-trip, the driver fails with a diagnostic naming the side and the observed line count. Once both files are full, parse each into a positional 15-tuple per record via a regex anchored on `[`, `]`, `"`, space delimiters. Apply the per-field tier rule: Tier-E byte-equal; Tier-F parser-predicate (RFC3339 / int / `host:port` shape); Tier-S subject-emits-`-` (reference value ignored). Failure prints `record N / field K NAME: subject=<...> reference=<...>` to facilitate debugging.
  - **`driver/driver_test.go`** — the parser regex unit tests (15-tuple extraction round-trip on hand-authored sample lines from §7.4); driver wiring smoke test.
  - **`backends/main.go`** — small Go program that starts an HTTP/1.1 server on a configurable port. Each backend instance N ∈ {0,1,2} responds to any GET with `200 OK` and body `backend-N:v1/<n>\n` (where `<n>` is the trailing path segment); body is byte-identical regardless of which backend serves the request because all three return the same prefix-then-suffix shape (e.g., `/api/v1/foo` → `backend-0:v1/foo` from backend 0; backend 1 sees `/api/v1/bar` → `backend-1:v1/bar`; etc.). **Critically**, the body length (and thus `BYTES_SENT`) is byte-identical across backends because `len("backend-0:v1/foo\n") = len("backend-1:v1/bar\n") = len("backend-2:v1/baz\n") = 17` (the path segments `foo`/`bar`/`baz` are all 3 bytes; the prefix `backend-N:v1/` is also a fixed 13 bytes regardless of N). This is what makes Tier-E `BYTES_SENT` byte-equality robust against RR endpoint-selection divergence between the two proxies.
- **`test/differential/runner.go`** (extended) — registration update: blank-import the new fixture-0006 driver package; the runner's per-fixture loop calls the driver's `DriveSubject` / `DriveReference` per the existing pattern. The polling-loop file-readiness pattern is fixture-0006-specific; the planner picks at PLAN time whether to surface it as a generic `LogFileExpectations` Driver-interface extension or to keep it in-band like the 0004/0005 drivers do (per §12 #4). **Recommendation: in-band** — matches the 05.2 + 06.1 in-band precedent; the per-fixture pattern is established and the polling-loop is a 30-LoC inline routine.

### 4.4 Changed documentation and state (in 06.2)

- **`docs/envoy-go/ROADMAP.md`** — row `06.2`: `status: planned → in-progress` flipped at the SPEC commit (per the corrected pattern from phase 05/05.1/05.2/06.1's SPEC commits, recorded in `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); transitions to `done` at the 06.2 phase-done commit. Row `06` (parent): `in-progress → done` AT THE SAME phase-done commit per parent SPEC §5. The §5.1 phase-done commit's commit-message body must explicitly name BOTH ROADMAP-row transitions (`06.2 → done` AND `06 → done`).
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → at phase-done, parent row `06` closes). Updated by the parent session, NOT by the SPEC-drafting subagent.
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (extended in-place per ADR-0052's authorization, mirroring the 05.1 / 05.2 / 06.1 in-place-edit pattern) — the empty `## Access log field mapping` placeholder at lines 170–174 (currently reading `_to be filled per-phase as needed._`) is filled with the 15-operator format table from §6 below, the three-tier matrix from §13, and the X-ENVOY-ORIGINAL-PATH fallback note from §6.1. The existing `## Equivalence Matrix` row at line 18 (`Access log records | Semantically equal after field-mapping`) STAYS as-is (its text is already correct); the in-place edit makes the row's `## Access log field mapping` reference load-bearing — that subsection is now populated per Decision D's three-tier matrix, and the row's "Semantically equal" predicate IS the three-tier matrix. The in-place edit lands at the 06.2 phase-done commit, NOT at the SPEC commit (per ADR-0052 discipline).
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — UNCHANGED in 06.2 (no pin bump; D-3.7 reserves pin bumps for dedicated phases).
- **`docs/envoy-go/DECISIONS.md`** — four new ADRs introduced by phase 06.2, numbered ADR-0066..ADR-0069 (next-free per the `DECISIONS.md` tail at master `9acfc0b` being ADR-0065; the planner re-verifies next-free at write time per ADR-0004's autonomous-numbering rule). Topics enumerated in §8 below; the ADRs themselves are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).
- **`docs/envoy-go/phases/06-observability-baseline/SPEC.md`** — UNCHANGED in 06.2 (the parent master SPEC is read-only history once drafted at 06.1's SPEC commit).
- **`docs/envoy-go/phases/06.2-access-log/README.md`** — superseded by THIS SPEC.md at the SPEC commit. The README content is preserved as historical context (as 06.1's README was preserved); the dispatcher handles the README-vs-SPEC transition.

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 06.2)

Phase 06.2 adds one new package (`internal/accesslog/`), extends one existing package (`internal/filter/hcm/`) with two new files (`accesslog_emit.go`, `bytecounter.go`), and threads a `[]accesslog.Sink` slice through one constructor signature (`internal/filter/hcm/config.go`'s filter-chain builder) plus the boot wiring in `cmd/envoy-go/main.go`. The threading is the surface that mirrors 06.1's parameter-threading discipline established by ADR-0059 (the same pattern that 06.1 BRAINSTORM §3 note 6 flagged for explicit enumeration so it isn't surprise scope at PLAN time).

```
cmd/envoy-go/main.go                 (MODIFIED: open AsyncFileSinks between bootstrap.Load
                                        and listener.Run; thread slice into HCM filter chain;
                                        defer sink.Close() after listener.Shutdown)
internal/bootstrap/bootstrap.go      (MODIFIED: parse access_log[]; reject log_format;
                                        silent-ignore non-file types; Bootstrap struct gains
                                        AccessLogConfigs []AccessLogConfig field)
internal/bootstrap/bootstrap_test.go (MODIFIED: parse-success / parse-fail / silent-ignore tests)
internal/filter/hcm/filter.go        (MODIFIED: Filter struct gains accessLog []accesslog.Sink)
internal/filter/hcm/config.go        (MODIFIED: parseFilterWithCtx accepts and stores the sink slice)
internal/filter/hcm/actions.go       (MODIFIED: directResponseAction.do +1 LoC defer emit;
                                        routerAction.do +5 LoC for start/bcw/picked capture +
                                        defer emit; routerActionH2.doH2 same shape)
internal/filter/hcm/h2dispatch.go    (MODIFIED: h2DirectResponseAdapter.WriteH2 +1 LoC defer emit)
internal/filter/hcm/accesslog_emit.go     (NEW: Filter.emitAccessLog (H1 + H2 variants))
internal/filter/hcm/accesslog_emit_test.go (NEW)
internal/filter/hcm/bytecounter.go        (NEW: byteCounterWriter ~10 LoC)
internal/filter/hcm/bytecounter_test.go   (NEW)
internal/filter/hcm/filter_test.go        (MODIFIED: emit-deferral tests)

internal/accesslog/                  (NEW package — replaces phase-00 placeholder doc.go)
   doc.go                            (REWRITE: API + lifecycle + drop-counter contract)
   accesslog.go                      (NEW: Sink interface, Record struct)
   format.go                         (NEW: Default formatter)
   writer.go                         (NEW: AsyncFileSink)
   stats.go                          (NEW: server.accesslog_dropped counter wiring)
   accesslog_test.go                 (NEW)
   format_test.go                    (NEW)
   writer_test.go                    (NEW)
   fuzz_test.go                      (NEW: FuzzAccessLogFormat — eighth fuzzer)

test/differential/runner.go          (MODIFIED: blank-import for fixture 0006;
                                        the runner's per-fixture loop calls the driver's
                                        polling-loop log-file-readiness pattern in-band)

test/fixtures/0006-access-log/       (NEW fixture directory)
   envoy.yaml, envoy-go.yaml, expectations.yaml, README.md
   driver/driver.go, driver/driver_test.go
   backends/main.go

internal/stats/                      (UNCHANGED — but the server.accesslog_dropped counter
                                        is allocated against the existing Registry per
                                        Decision I + ADR-0069; the counter name is outside
                                        the 06.1 17-name allow-list so the 0005 differential
                                        ignores it)
internal/cluster/                    (UNCHANGED at the type-shape level; routerAction.do +
                                        routerActionH2.doH2 capture the picked endpoint via
                                        an existing-or-new cluster.Endpoint accessor — the
                                        details are in §12 #2 below)
internal/listener/                   (UNCHANGED)
internal/admin/                      (UNCHANGED — no admin endpoint changes)
internal/filter/hcm/h2/              (UNCHANGED on the codec primitives)
internal/filter/tcpproxy/            (UNCHANGED)
internal/tls/                        (UNCHANGED)

test/conformance/h2spec/             (UNCHANGED — pin and threshold list stay at 05.1+05.2 baseline)
test/fixtures/0000-tcp-echo/         (UNCHANGED)
test/fixtures/0001-tcp-proxy-rr/     (UNCHANGED)
test/fixtures/0002-tls-tcp/          (UNCHANGED)
test/fixtures/0003-http11-routing/   (UNCHANGED)
test/fixtures/0004-h2-routing/       (UNCHANGED)
test/fixtures/0005-prometheus-stats/ (UNCHANGED)

docs/envoy-go/BEHAVIOR_CONTRACT.md   (MODIFIED at phase-done commit, NOT SPEC commit:
                                        ## Access log field mapping populated; existing
                                        ## Equivalence Matrix row's "Semantically equal"
                                        predicate concretized via the populated subsection)
docs/envoy-go/CONFORMANCE_PINS.md    (UNCHANGED)
docs/envoy-go/DECISIONS.md           (APPENDED at impl-time per ADR-by-ADR commits:
                                        ADR-0066..ADR-0069 — four ADRs; planner verifies
                                        next-free at write time)
docs/envoy-go/ROADMAP.md             (MODIFIED at SPEC commit: row 06.2 planned → in-progress.
                                        At phase-done: row 06.2 → done; row 06 (parent) →
                                        done AT THE SAME COMMIT)
docs/envoy-go/STATE.md               (MODIFIED at each lifecycle transition by parent session)
docs/envoy-go/phases/06-observability-baseline/SPEC.md   (UNCHANGED — parent master SPEC, read-only)
docs/envoy-go/phases/06.2-access-log/README.md   (SUPERSEDED by THIS SPEC.md at SPEC commit)
docs/envoy-go/phases/06.2-access-log/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
```

### 5.2 Async-writer concurrency model (per Decision B)

| Actor | Operation | Frequency | Synchronization |
|---|---|---|---|
| Hot path | `Sink.Submit(*Record)` | Per request finalization (~1× per HCM request lifecycle) | Non-blocking channel send: `select { case ch <- r: default: dropped.Inc(); ... }` |
| Writer goroutine | `for r := range ch { f.Write(Default(r)) }` | One goroutine per `AsyncFileSink`; consumes records as fast as the OS lets the file accept them | Channel receive (blocks on empty); single-consumer of `ch` |
| Drop path | `dropped.Inc()` + rate-limited `log.Printf` | Only on full channel (cap 4096); fixture-0006 never exercises this | atomic.Int64 CAS for the rate-limit deadline; counter Inc is lock-free per 06.1 ADR-0059 |
| Boot | `NewAsyncFileSink(path, dropped)` | Once per sink at process start | Single-threaded boot; no contention |
| Shutdown | `Close()` (drains channel + closes fd) | Once per sink at process exit | `close(ch)` then wait on `done` channel; no in-flight Submit observable past Close (callers stop calling Submit before Close per the boot/shutdown ordering) |

The hot-path `Submit` is **lock-free in the common case** (channel non-full): Go's channel send on a buffered channel with available capacity is a single atomic-CAS-bounded operation — no mutex, no syscall. On a full channel, the `default` branch fires synchronously without blocking the request handler. This satisfies the same lock-free-hot-path discipline 06.1 ADR-0059 established for the stats subsystem.

The writer-goroutine's `f.Write(Default(r))` on a `os.File` opened `O_APPEND` is **atomic for sub-PAGE writes** under POSIX (per `man 2 write` on Linux: "If the O_APPEND flag of the file status flags is set, the file offset shall be set to the end of the file prior to each write and no intervening file modification operation shall occur between changing the file offset and the write operation"). The default-format line for the 5-request fixture-0006 workload is well under 4 KiB per record; the contract holds. NO `fsync`. Matches Envoy. The OS page-cache is the durability ceiling.

### 5.3 Boot wiring sequence (per Decision E)

```
cmd/envoy-go/main.go
   ↓
bootstrap.Load(configPath) → returns *Bootstrap (now has .AccessLogConfigs []AccessLogConfig
                              for parsed-but-unopened sink configs)
   ↓
   (06.1 wiring: stats Registry, cluster Manager, listener Manager, admin)
   ↓
sinks := []accesslog.Sink{}
for _, cfg := range bootstrap.AccessLogConfigs:
    droppedCounter := registry.NewCounter("server.accesslog_dropped")
    // ^ allocated once total per process — re-getting on subsequent sinks
    // is forbidden by 06.1's LBP-1 invariant; the loop allocates exactly once
    // even with N sinks, sharing the counter across all sinks. The counter
    // increment-on-drop is correctly attributed because the drop-event is
    // per-Submit and the call site can identify the sink (in the diagnostic
    // log line); the counter aggregates across all sinks.
    sink, err := accesslog.NewAsyncFileSink(cfg.Path, droppedCounter)
    if err != nil { log.Fatalf("accesslog: open %q: %v", cfg.Path, err) }
    sinks = append(sinks, sink)
   ↓
   thread `sinks` into the HCM filter chain via the existing internal/filter/hcm/config.go
   plumbing (the same parameter-threading shape that 06.1 ADR-0059 introduced for the
   *stats.Registry parameter — extended here with the additional []accesslog.Sink
   parameter to parseFilterWithCtx)
   ↓
   registry.Freeze()  (per 06.1 LBP-1)
   ↓
   listenerManager.Run()  (begins accepting connections)
   ↓
   ... (request handling — Submit fires per request finalization)
   ↓
SIGTERM / process exit
   ↓
   listener.Shutdown()  (returns when accept loop exits; in-flight requests still draining)
   ↓
   for _, s := range sinks: defer s.Close()  (in registration order)
   ↓
   process exits
```

The drop-counter is allocated **exactly once total** (not once per sink) — the counter aggregates drops across all sinks. The diagnostic log line distinguishes which sink dropped via the per-sink `path` value. This keeps 06.1's LBP-1 invariant intact (no post-Freeze `NewCounter` calls) while providing per-sink debug visibility through the log channel.

The `defer sink.Close()` ordering is per-registration; the `Close()` method is idempotent and threadsafe (the underlying `close(ch)` panics on double-close per Go's channel-close semantics, so the implementation guards with a `sync.Once`).

### 5.4 Emit paths (per-request hot path, per Decision F)

| File | Hot-path edits |
|---|---|
| `internal/filter/hcm/actions.go` (`directResponseAction.do`) | `+1 LoC` — `defer filter.emitAccessLog(r, statusCode, int64(len(body)), cluster.Endpoint{}, start)`. The `start` is captured via a small refactor: the `do` method gains a leading `start := time.Now()` line. `BYTES_SENT` is the body length (already known statically — `len(body)`) per the direct_response action's deterministic body. `picked` is the zero-value `Endpoint{}` → formatter emits `-`. |
| `internal/filter/hcm/actions.go` (`routerAction.do`, H1 router) | `+5-7 LoC` — `start := time.Now()` at action entry; `bcw := &byteCounterWriter{w: bw}` wrapping the downstream `bufio.Writer`; the action writes through `bcw` for `resp.Write(bcw)`; `picked` is captured via the `Cluster.PickEndpoint()` call (which is currently inside `Cluster.Dial`; the small refactor in §12 #2 surfaces the pick to the caller); `defer filter.emitAccessLog(r, statusCode, bcw.n, picked, start)` at action exit. The `BYTES_SENT` reflects the bytes written through `bcw` to the downstream — including the resp body bytes. |
| `internal/filter/hcm/h2dispatch.go` (`h2DirectResponseAdapter.WriteH2`) | `+1 LoC` — same shape as H1 direct_response. The H2 codec's `StreamWriter.WriteData(body, true)` writes a known-length body; `BYTES_SENT` is `int64(len(body))`. |
| `internal/filter/hcm/actions.go` (`routerActionH2.doH2`, H2 router) | `+5-7 LoC` — `start := time.Now()` at action entry; the H2 stream writer's bytes-written count is tracked by summing `len(resp.Headers)`-encoded-bytes + `len(resp.Body)` (the H2 path doesn't have an `io.Writer`-shaped wrapper because `StreamWriter.WriteData` takes a body slice; a separate counter sums these). The ctx-cancel branch (`return 0, h2.NewStreamError(...)`) returns statusForHCM = 0; the deferred emit checks for the zero-status sentinel and SKIPS emission (the request did not produce a finalized response status; matches Envoy's "no record on cancel" behavior). The H2-path `picked` plumbing mirrors H1. |

**Total request-path edits: ~12-16 LoC across 2 files (`actions.go`, `h2dispatch.go`).** The `internal/accesslog` package + `internal/filter/hcm/accesslog_emit.go` + `internal/filter/hcm/bytecounter.go` are the bulk of new code.

**Differential vs unit-test coverage of the four sites:** the two H1 sites are exercised differentially by fixture 0006 (per §7.2). The two H2 sites are exercised by unit tests in `internal/filter/hcm/accesslog_emit_test.go` (per §14.2 — H1 + H2 record-shape correctness, status-code-zero ctx-cancel skips emission, multiple-sinks delivery). The H2 codec wire surface is unchanged from 05.2 + 06.1; the 06.2 changes at the H2 sites are the additive `defer filter.emitAccessLog(...)` lines above. This split mirrors 06.1's H1-only fixture-0005 precedent and aligns with the project's longer-running pattern of one differential fixture per codec (fixture 0003 = H1 routing; fixture 0004 = H2 routing; fixture 0006 = H1 access-log).

### 5.5 Read path (file sink)

```
HCM request finalization
   ↓
filter.emitAccessLog(r, statusCode, bytesSent, picked, start)
   ↓
construct accesslog.Record{
    StartTime:    start,
    Method:       r.Method,
    Path:         r.URL.Path,
    Protocol:     r.Proto,        (or "HTTP/2.0" for H2)
    ResponseCode: statusCode,
    BytesSent:    bytesSent,
    Duration:     time.Since(start),
    Authority:    r.Host,
    UserAgent:    r.Header.Get("User-Agent"),
    UpstreamHost: picked.Host + ":" + strconv.Itoa(int(picked.Port)),  (or "" for direct_response)
}
   ↓
for _, sink := range filter.accessLog: sink.Submit(record)
   ↓
sink.Submit:
   select {
   case ch <- record: // common case: buffered write, no blocking
   default:           // rare: channel full
       dropped.Inc()
       (rate-limited log)
   }
   ↓ (writer goroutine)
record arrives on ch
   ↓
line := accesslog.Default(record)   (~ 200 bytes, ends in '\n')
   ↓
f.Write(line)                       (atomic for sub-PAGE writes under O_APPEND)
   ↓
ON SHUTDOWN: close(ch); writer drains; f.Close()
```

The read path (the writer goroutine + file write) is decoupled from the hot path. The hot path's only synchronization with the writer is the channel; on full-channel drops, the hot path is still lock-free.

### 5.6 The `server.accesslog_dropped` counter (per Decision I + ADR-0069)

Allocated once at boot (per §5.3) via `registry.NewCounter("server.accesslog_dropped")`. The internal name flattens to Prometheus name `envoy_server_accesslog_dropped` per Rule SN5 (`server.<rest>` → `envoy_server_<rest>`, no labels). **Outside the 06.1 17-name allow-list** — the fixture-0005 differential ignores it; operator-visible at `/stats/prometheus` only. The counter is `Inc`'d once per dropped record.

The counter's read path is unchanged from 06.1: `Walk` iterates the Registry, the writer renders one line `envoy_server_accesslog_dropped <value>`. No HELP-text entry is added in 06.2 (the entry would be added in `internal/stats/name.go`'s `helpText` map; if the planner chooses to add it for completeness, it lands at PLAN time per §12 #5 deferred decision; recommendation: add it for symmetry with 06.1's HELP-text discipline).

## 6. Access-log default-format catalog — the 15 operators (per Decision A, transcribed from the empirical pin in §11)

The Envoy default access-log format is the literal command-operator string at upstream Envoy v1.37.2 (per the empirical pin in §11; placeholder until PLAN execution lands the verbatim scrape). The 15 operators in identical positions:

| # | Operator | Plumbed in 06.2 | Tier (per Decision D / §13) | Source field on the Record |
|---|---|---|---|---|
| 1 | `[%START_TIME%]` | yes | F (format-only) | `Record.StartTime` formatted as `2006-01-02T15:04:05.000Z` (RFC3339 ms-precision UTC) |
| 2 | `"%REQ(:METHOD)%` (open-quoted block) | yes | E (byte-equal) | `Record.Method` |
| 3 | `%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%` | yes (via `:PATH` fallback — see §6.1) | E (byte-equal) | `Record.Path` |
| 4 | `%PROTOCOL%"` (close-quoted block) | yes | E (byte-equal) | `Record.Protocol` |
| 5 | `%RESPONSE_CODE%` | yes | E (byte-equal) | `Record.ResponseCode` |
| 6 | `%RESPONSE_FLAGS%` | NO (literal `-`) | S (subject-emits-`-`) | — |
| 7 | `%BYTES_RECEIVED%` | NO (literal `-`) | S (subject-emits-`-`) | — |
| 8 | `%BYTES_SENT%` | yes | E (byte-equal) | `Record.BytesSent` (decimal int) |
| 9 | `%DURATION%` | yes | F (format-only — int ms ≥ 0) | `time.Since(Record.StartTime)` rounded down to ms |
| 10 | `%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%` | NO (literal `-`) | E (byte-equal — both `-`) | — |
| 11 | `"%REQ(X-FORWARDED-FOR)%"` (quoted) | NO (literal `"-"`) | S (subject-emits-`-`) | — |
| 12 | `"%REQ(USER-AGENT)%"` (quoted) | yes | E (byte-equal) | `Record.UserAgent` |
| 13 | `"%REQ(X-REQUEST-ID)%"` (quoted) | NO (literal `"-"`) | S (subject-emits-`-`) | — |
| 14 | `"%REQ(:AUTHORITY)%"` (quoted) | yes | E (byte-equal) | `Record.Authority` |
| 15 | `"%UPSTREAM_HOST%"` (quoted) | yes | F (format-only — `host:port` or `-`) | `Record.UpstreamHost` |

**Total: 15 operators.** 8 Tier-E + 3 Tier-F + 4 Tier-S = 15 (= the operator count in the format). The format string itself, with the literal delimiters `[`, `]`, `"`, space, is what the §11 empirical pin block records verbatim.

Quoted operators (`USER-AGENT`, `:AUTHORITY`, `X-FORWARDED-FOR`, `X-REQUEST-ID`, `UPSTREAM_HOST`, plus the request-line block enclosing `:METHOD` / `:PATH` / `PROTOCOL`) wrap the field's value in double quotes; literal `"` in the value escapes to `\"` per Envoy convention. `START_TIME` is wrapped in `[...]` per the format-string literal.

### 6.1 The `X-ENVOY-ORIGINAL-PATH?:PATH` fallback (per Decision A, operator #3)

The format-string operator `%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%` is the conditional-fallback shape: emit the value of header `X-ENVOY-ORIGINAL-PATH` if present on the request, else fall through to `:PATH`. **Neither side emits `X-ENVOY-ORIGINAL-PATH` on fixture 0006's workload:**

- Subject (envoy-go): does not inject the header (the path-rewriting surface that produces it is a future-phase concern); on every fixture-0006 request, the request lacks the header so the formatter emits `Record.Path` via the fallback.
- Reference (Envoy v1.37.2): does not emit the header either, because fixture 0006 has no `path_rewrite`-bearing route; reference Envoy emits `:PATH` via the fallback.

Both sides therefore emit the original `:PATH` on every record. Tier-E byte-equality holds. The SPEC documents this explicitly so that a future phase that introduces path-rewriting (and thus exercises the `X-ENVOY-ORIGINAL-PATH` half) does NOT silently regress fixture 0006 — when that surface lands, fixture 0006's expectations need a Tier-E/F re-evaluation under the new behavior.

## 7. Differential fixture `0006-access-log` (per Decision G)

### 7.1 Equivalence claim shape

Per Decision D, the differential equivalence claim is **per-field three-tier** (not byte-exact whole-record): drive a defined load, parse each access-log file into positional 15-tuples, apply the per-field tier rule to each record. Records on the two sides are paired by record index (record 1 of subject vs. record 1 of reference, record 2 vs. record 2, ...). This is a stronger equivalence than 06.1's per-counter-delta-equality (which is whole-set with allow-list), and is the access-log analog of 06.1 ADR-0062's stats-output behavioral-equivalence shape.

Rationale (per Decision D's brainstorm settlement):
- Pure byte-exact whole-record equivalence is impossible on `START_TIME` (wall-clock divergence) and `DURATION` (per-request timing divergence). It is also fragile on `UPSTREAM_HOST` because the reference container resolves `host.docker.internal` while the subject uses STATIC IPs.
- Pure presence-only equivalence is too loose: it would not catch a regression where the subject emits a corrupted `RESPONSE_CODE` (e.g., `5OO` instead of `500` — a hypothetical bug from a `fmt.Stringer` mistake).
- The three-tier matrix surfaces field-by-field: byte-equal where possible (Tier E), format-only where deterministically possible only locally (Tier F), subject-emits-`-` where the operator is not plumbed (Tier S). Each field's tier is anchored in the operator's plumbing status (Decision A) and its determinism profile.

### 7.2 Workload (per Decision G)

5 sequential GETs per side, total 10 records:

| # | Path | Action | Cluster | Expected status | Expected `BYTES_SENT` |
|---|---|---|---|---|---|
| 1 | `GET /health` | `direct_response` | (none — HCM-direct) | 200 | 3 (`OK\n`) |
| 2 | `GET /api/v1/foo` | `routerAction` | `c_backend` (3 endpoints, RR) | 200 | 17 (`backend-N:v1/foo\n`) |
| 3 | `GET /api/v1/bar` | `routerAction` | `c_backend` (RR) | 200 | 17 (`backend-N:v1/bar\n`) |
| 4 | `GET /api/v1/baz` | `routerAction` | `c_backend` (RR) | 200 | 17 (`backend-N:v1/baz\n`) |
| 5 | `GET /notfound` | `direct_response` | (none — HCM-direct) | 404 | 10 (`not found\n`) |

The 17-byte `BYTES_SENT` for the routed responses holds across all three RR-selected backends because every backend emits `backend-N:v1/<n>\n` with N a single digit and `<n>` a 3-byte path segment (`foo`, `bar`, `baz`). Byte-identical body length across endpoints is what makes Tier-E `BYTES_SENT` byte-equality robust against RR endpoint-selection divergence between subject and reference.

**Codec scope (per §1 item 6 / §5.4):** fixture 0006's bootstrap declares `codec_type: HTTP1` on both sides. Only the two H1 emit-hook sites (`directResponseAction.do`, `routerAction.do`) are exercised differentially here; the two H2 sites (`h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`) are exercised by unit tests in `internal/filter/hcm/accesslog_emit_test.go` per §14.2. This aligns with the project's per-codec fixture pattern (0003 = H1 routing; 0004 = H2 routing; 0006 = H1 access-log) and with 06.1's H1-only fixture-0005 precedent.

### 7.3 Driver outline (per Decision G)

1. Boot envoy-go on port P1 + reference Envoy on port P2 with identical (modulo the cluster STATIC-vs-STRICT_DNS divergence) bootstraps.
2. Boot 3 controlled backends on ports P3 / P4 / P5; each emits `backend-N:v1/<path-segment>\n` for `GET /api/v1/<path-segment>`.
3. Both subjects' `access_log[]` is configured with one `envoy.access_loggers.file` entry: subject's `path` is `<t.TempDir()>/subject.log`; reference's `path` is `/tmp/envoy-access.log` (bind-mounted by `testcontainers-go` `Mounts` to `<t.TempDir()>/reference.log`).
4. Send 5 sequential GETs per side with paths `[/health, /api/v1/foo, /api/v1/bar, /api/v1/baz, /notfound]`.
5. **Drain discipline (Decision G — polling loop, no arbitrary sleep)**: after the 5th response is received, poll the per-side log file at 25ms intervals until each side's file has `≥ 5` lines OR a 5s hard deadline trips; on deadline-trip, fail with a diagnostic naming the side and the observed line count. (This adopts 06.1 REVIEW M-8's recommendation prophylactically per Decision M's exception.)
6. Parse each line into a positional 15-tuple via a regex anchored on `[`, `]`, `"`, space.
7. Pair subject's record N with reference's record N (1-indexed).
8. For each operator position 1..15, apply the tier rule (Tier E byte-equal; Tier F parser predicate; Tier S subject-emits-`-`-and-reference-ignored). On failure, print `record N / field K NAME: subject=<...> reference=<...>` to facilitate debugging.

### 7.4 Expected records (`expectations.yaml` shape, three-tier matrix transcribed from Decision D)

For each of the 5 record indices on each side, the 15-tuple under the format is:

```
record 1 (GET /health → 200 direct_response):
  field  1 START_TIME           = [<RFC3339-ms-UTC>]            tier F  (parser: regex ^\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z\]$)
  field  2 :METHOD              = "GET                          tier E  (subject == reference == "GET)
  field  3 :PATH                = /health                       tier E  (subject == reference == /health)
  field  4 PROTOCOL             = HTTP/1.1"                     tier E  (subject == reference == HTTP/1.1")
  field  5 RESPONSE_CODE        = 200                           tier E  (subject == reference == 200)
  field  6 RESPONSE_FLAGS       = -                             tier S  (subject == "-"; reference unconstrained)
  field  7 BYTES_RECEIVED       = -                             tier S  (subject == "-"; reference unconstrained)
  field  8 BYTES_SENT           = 3                             tier E  (subject == reference == 3)
  field  9 DURATION             = <int-ms>                      tier F  (parser: int >= 0)
  field 10 RESP-SVC-TIME        = -                             tier E  (subject == reference == "-")
  field 11 X-FORWARDED-FOR      = "-"                           tier S
  field 12 USER-AGENT           = "Go-http-client/1.1"          tier E  (subject == reference; both Go-http-client/1.1 from the driver's net/http client)
  field 13 X-REQUEST-ID         = "-"                           tier S
  field 14 :AUTHORITY           = "127.0.0.1:<port>"            tier E  (modulo port — driver normalizes both to "<authority>" for cross-side compare; see §7.5)
  field 15 UPSTREAM_HOST        = "-"                           tier F  (direct_response → "-" on both sides)

record 2 (GET /api/v1/foo → 200 routed via c_backend):
  field  1 START_TIME           = [<RFC3339-ms-UTC>]            tier F
  field  2 :METHOD              = "GET                          tier E
  field  3 :PATH                = /api/v1/foo                   tier E
  field  4 PROTOCOL             = HTTP/1.1"                     tier E
  field  5 RESPONSE_CODE        = 200                           tier E
  field  6 RESPONSE_FLAGS       = -                             tier S
  field  7 BYTES_RECEIVED       = -                             tier S
  field  8 BYTES_SENT           = 17                            tier E
  field  9 DURATION             = <int-ms>                      tier F
  field 10 RESP-SVC-TIME        = -                             tier E (both "-")
  field 11 X-FORWARDED-FOR      = "-"                           tier S
  field 12 USER-AGENT           = "Go-http-client/1.1"          tier E
  field 13 X-REQUEST-ID         = "-"                           tier S
  field 14 :AUTHORITY           = "<normalized-authority>"      tier E
  field 15 UPSTREAM_HOST        = "<host>:<port>"               tier F  (parser: regex ^"[^"]+:\d+"$)

record 3 (GET /api/v1/bar → 200 routed): same shape as record 2; :PATH = /api/v1/bar; BYTES_SENT = 17
record 4 (GET /api/v1/baz → 200 routed): same shape as record 2; :PATH = /api/v1/baz; BYTES_SENT = 17

record 5 (GET /notfound → 404 direct_response):
  field  1 START_TIME           = [<RFC3339-ms-UTC>]            tier F
  field  2 :METHOD              = "GET                          tier E
  field  3 :PATH                = /notfound                     tier E
  field  4 PROTOCOL             = HTTP/1.1"                     tier E
  field  5 RESPONSE_CODE        = 404                           tier E
  field  6 RESPONSE_FLAGS       = -                             tier S
  field  7 BYTES_RECEIVED       = -                             tier S
  field  8 BYTES_SENT           = 10                            tier E
  field  9 DURATION             = <int-ms>                      tier F
  field 10 RESP-SVC-TIME        = -                             tier E (both "-")
  field 11 X-FORWARDED-FOR      = "-"                           tier S
  field 12 USER-AGENT           = "Go-http-client/1.1"          tier E
  field 13 X-REQUEST-ID         = "-"                           tier S
  field 14 :AUTHORITY           = "<normalized-authority>"      tier E
  field 15 UPSTREAM_HOST        = "-"                           tier F  (direct_response → "-" on both sides)
```

### 7.5 Authority normalization

The `:AUTHORITY` field's value differs between subject and reference because each binds a different listener port (subject uses `:0` ephemeral; reference uses port 15006). The driver normalizes both sides' authorities to a canonical form (`<authority-without-port>` or `host.docker.internal:<KNOWN>` per the runner's port mapping) before applying Tier-E byte-equality. The exact normalization rule lands at PLAN time per §12 #6; recommendation: strip the port and assert the host part is byte-equal (`127.0.0.1`/`host.docker.internal` are both rendered as `127.0.0.1` on the reference inside the container per Docker's bridge-networking convention; the runner applies a string-substitution normalization).

## 8. ADRs anticipated

Per Decision I, four ADRs are anticipated for 06.2, numbered ADR-0066..ADR-0069 (next-free per the `DECISIONS.md` tail at master `9acfc0b` being ADR-0065). The planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule. The ADRs are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).

The four anticipated ADRs mirror the four shape-of-decision groups from 06.1's six ADRs: architecture-shape (ADR-0066 ↔ 06.1 ADR-0059), boundary-validation (ADR-0067 ↔ 06.1 ADR-0065), differential-equivalence (ADR-0068 ↔ 06.1 ADR-0062), and stat-name extension (ADR-0069 ↔ 06.1 ADR-0061's SN5).

- **ADR-0066 — Access-log architecture (file sink + AsyncFileSink + drop-newest backpressure).** Status: Accepted. Doctrine: D-3.2 + D-3.3. Decision: a thin in-tree `internal/accesslog` package (`Sink` interface, `Record` struct, `Default` formatter, `AsyncFileSink` async-writer with bounded-channel drop-newest backpressure); **no third-party access-log library**. Lock-free hot path on submit (Go's buffered-channel non-blocking send is atomic-CAS-bounded); writer goroutine consumes single-channel-receive; per-record `os.File.Write` is atomic for sub-PAGE writes under `O_APPEND` (no `fsync`; OS buffering is the durability ceiling — matches Envoy). Rationale (per Decision E + the parent SPEC §4.3 cross-cutting "no third-party observability dependencies" decision): future Observability-family phases (gRPC ALS, OTLP) need a Sink interface to hook, not a third-party file-logger; investing in our own thin shape now is the same architectural choice 06.1 made for stats, made-for-the-same-reason. Documents the no-third-party-library decision per doctrine D-3.2 and the AsyncFileSink concurrency model. Lands-in-task: 06.2 Task 1 (the `internal/accesslog` package skeleton).

- **ADR-0067 — Reject `log_format` at parse (option β; extends ADR-0065's boundary-validation pattern).** Status: Accepted. Doctrine: D-3.4. Decision: the bootstrap parser READS HCM `access_log[]` as a list; each entry's typed-config of type `envoy.access_loggers.file` MUST have `path` (required, non-empty string); ANY presence of `log_format` / `format_string` / `json_format` produces a fatal parse error: `unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)`. Rationale (per Decision C): boundary-validation at parse-time prevents silent wrong-result behavior — a bootstrap that says "I want JSON-formatted access logs" but receives Envoy-default-formatted logs would be a silent deviation from the operator's intent; failing-loud at parse-time forces the operator to remove the field (or the project to ship the parser surface in a future phase). Extends ADR-0065's pattern (HCM `stat_prefix` regex check at the bootstrap-input boundary) to access-log config. Lands-in-task: 06.2 Task 4 (the bootstrap parser extension).

- **ADR-0068 — Access-log differential equivalence shape (3-tier matrix).** Status: Accepted. Doctrine: D-3.3 + D-3.6. Decision: per-record per-field three-tier equivalence — Tier E (byte-equal cross-side, 8 operators), Tier F (format-only — parses to expected shape on both sides; cross-side value not asserted equal, 3 operators), Tier S (subject must emit `-`; reference unconstrained, 4 operators). Records are paired by record-index; non-listed reference fields (the Tier-S reference values) are silently dropped by the parser. Rationale (per Decision D): byte-exact whole-record equivalence is impossible on timestamp / duration / upstream-host fields under cross-proxy divergence; a layered byte-exact-record-with-known-field-replacements approach was considered but adds ~3× driver LoC for marginal protection over what unit tests already provide; the three-tier matrix surfaces the equivalence claim field-by-field at the cost of ~50 LoC of parser regex and tier-application logic. Companion to 06.1 ADR-0062 (stats-output equivalence): both decisions answer the same question — "what is the right equivalence for an observability output that has cross-proxy non-determinism in some fields?" — for their respective surfaces. Specifies that the populated `## Access log field mapping` subsection's tier table IS the "Semantically equal after field-mapping" predicate from the existing `## Equivalence Matrix` row. Lands-in-task: 06.2 Task 13 (the differential fixture's tier-application logic in `driver/driver.go`).

- **ADR-0069 — `server.accesslog_dropped` counter naming (SN5 mapping).** Status: Accepted. Doctrine: D-3.4. Decision: the drop-newest backpressure counter (Decision B) is allocated against 06.1's `*stats.Registry` via `registry.NewCounter("server.accesslog_dropped")`. Per 06.1 ADR-0061 Rule SN5 (`server.<rest>` → `envoy_server_<rest>`, no labels), the Prometheus name is `envoy_server_accesslog_dropped`. **Outside the 06.1 17-name allow-list** — fixture 0005's differential explicitly ignores the metric per ADR-0062's allow-list discipline. Operator-visible at `/stats/prometheus` only. Rationale (per Decision B): naming the counter `server.accesslog_dropped` (not `accesslog.dropped` or `http.<stat_prefix>.accesslog_dropped`) anchors it on the server scope because the counter aggregates across all configured sinks (the configuration is process-global, not per-listener or per-HCM). Lands-in-task: 06.2 Task 1 (alongside the package skeleton; the counter wiring lives in `internal/accesslog/stats.go`).

The four ADRs are anticipated as ADR-0066..ADR-0069 in topical order. The planner may permute commit-time landings if that reads more naturally in PLAN.md (the four ADR-0055..ADR-0058 block in 05.2 used a non-monotonic commit-time ordering, and the four ADR-0059..ADR-0064 block in 06.1 followed the same pattern — both permitted and recorded in each ADR's `Lands-in-task` field).

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 06.2 silently ignores the following at parse time (no error, no honored behavior):

- HCM `access_log[].filter` field (per-record predicate filter) — silently ignored. → Phase 07's filter-chain framework.
- HCM `access_log[]` entries of type `envoy.access_loggers.stdout` — silently ignored. → Observability-family.
- HCM `access_log[]` entries of type `envoy.access_loggers.tcp_grpc` (gRPC ALS) — silently ignored. → Observability-family (gRPC ALS sub-phase).
- HCM `access_log[]` entries of type `envoy.access_loggers.open_telemetry` — silently ignored. → Observability-family (OTel sub-phase).
- HCM `access_log_options` — silently ignored.
- Listener-scope `access_log[]` (the listener-level access-log surface in upstream Envoy, distinct from the HCM-level surface) — silently ignored. → Future phase if a fixture demands it.
- Cluster-scope `access_log[]` — silently ignored. → Same.

The full silently-ignored set is the union of phases 04 / 05.1 / 05.2 / 06.1's silently-ignored sets plus 06.2's amendment above. The phase-04 / 05.1 / 05.2 / 06.1 ignored sets are NOT amended by this list — only extended. ADR-0041 (the original silent-ignore ADR — phase 04, already amended by 05.1's `http2_protocol_options` and 05.2 + 06.1's same-shape extensions) is amended (not superseded) to record the 06.2 additions; the amendment shape mirrors the 05.1 + 05.2 + 06.1 amendments (a single appended sub-section under ADR-0041's Consequences, listing the newly-ignored fields).

## 10. Carry-forward dispositions (per Decision M)

Phase-06.2 disposition of carry-forwards from prior phases:

### 10.1 Bundled with 06.2

**None.** Unlike 06.1 (which bundled 05.2 REVIEW M-9 — the explicit 05.2-REVIEW-deferred-to-06 line), no prior carry-forward has an explicit "lands in 06.2" annotation. The only EXCEPTION is 06.1 REVIEW M-8 (drain-loop polling), which is **not bundled** but is **adopted prophylactically by 06.2's fixture-0006 driver design** (Decision G) — see §10.3 below.

### 10.2 Carried forward (unchanged disposition from 06.1)

- **05.2 M-4** `readClientPreface` not ctx-aware (`internal/filter/hcm/h2/conn.go`). *Deferred.* Tag: `phase-07-or-later-must-consider`.
- **05.2 M-10** `SETTINGS_TIMEOUT` absent (`internal/filter/hcm/h2/client.go`). *Deferred.* Same tag.
- **05.2 M-12** `closedStreams` map unbounded (`internal/filter/hcm/h2/conn.go`). *Deferred.* Tag: `upstream-robustness-must-consider`.
- **05.2 prose Minors (7 items)** — unchanged carry-forward state.
- **06.1 12 Minors (M-2..M-12 + reviewer-discovered post-phase-done minor)** — separate 06.1 post-phase-done batch (the established 05.1/05.2 review-followup branch pattern); not 06.2's responsibility.

### 10.3 Adopted-prophylactically EXCEPTION

- **06.1 REVIEW M-8** ("hardcoded 200ms drain may flake on slow CI; recommend polling loop instead") — fixture 0006's driver adopts the polling-loop pattern natively (Decision G drain discipline: poll the per-side log file at 25ms intervals until each side has `≥ 5` lines, hard deadline 5s; no `time.Sleep(200 * time.Millisecond)` arbitrary sleep). This **does NOT close M-8 itself** (which targets fixture 0005's existing driver — that driver continues to flake until M-8's actual fix lands in a 06.1 review-followup batch); it establishes the pattern for new fixtures going forward. 06.2 is the first non-vacuous instance of this pattern.

The dispositions table for the reviewer's audit trail:

| Finding | Disposition | Target-phase candidates |
|---|---|---|
| 05.2 M-4 (readClientPreface ctx-unaware) | Deferred — out of 06.2 (unchanged from 06.1) | dedicated H2-hardening sub-phase / phase 07 / upstream-robustness family |
| 05.2 M-10 (SETTINGS_TIMEOUT absent) | Deferred — out of 06.2 (unchanged) | dedicated H2-hardening sub-phase / phase 08 |
| 05.2 M-12 (closedStreams map unbounded) | Deferred — out of 06.2 (unchanged) | upstream-robustness family (H2 conn-pooling sub-phase) |
| 05.2 prose Minors (7) | Deferred — out of 06.2 (unchanged) | various |
| 06.1 12 Minors (M-2..M-12 + reviewer-discovered) | Out of 06.2; lands in 06.1 review-followup batch | 06.1 post-phase-done branch |
| 06.1 REVIEW M-8 (drain-loop polling) | **Adopted prophylactically by 06.2 design** (does NOT close M-8 against fixture 0005) | (06.1 review-followup branch closes M-8 itself) |

ADR authoring at 06.2 phase-done: no carry-forward ADR is needed because the 06.2 SPEC itself records the dispositions; per ADR-0017 doctrine ("small mechanical fixes do not require ADRs"), the deferred items land as plain task entries in future PLAN.md when their target phase enters lifecycle-state 1. The adopted-prophylactically EXCEPTION is recorded inline in the fixture-0006 driver code as a comment cross-reference to 06.1 REVIEW M-8.

## 11. Empirical default-format pin (per Decision H, reserved for PLAN execution)

Mirrors 06.1's Rule SN4 empirical-pin block in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` (per ADR-0061). The SPEC reserves a placeholder identical in shape to 06.1 Rule SN4's empirical-evidence block; the actual pin is filled at PLAN execution time (the fixture-construction task — "PLAN Task N", numbered when PLAN.md drafts).

**Pin procedure (executed at PLAN Task N — the fixture-0006 construction task):** boot reference Envoy v1.37.2 (image pinned at `ENVOY_TARGET.md`'s SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`) with the fixture-0006 reference bootstrap (`envoy.yaml`); drive the 5-request workload from §7.2; capture the literal 5 emitted lines from `/tmp/envoy-access.log` (the path the `access_log[]` entry configures); paste them verbatim into the empirical pin block below; the same lines also land in `BEHAVIOR_CONTRACT.md ## Access log field mapping` (see §13). The pin is what the SPEC asserts about the Envoy default format; future image bumps (per `ENVOY_TARGET.md` refresh procedure) that change the format would fail loud at fixture run.

```
Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log
under the 5-request workload from §7.2; reference image v1.37.2 at
ENVOY_TARGET.md SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd):

<<TBD: pinned at PLAN Task N — empirical scrape>>
```

The pin block above is what `BEHAVIOR_CONTRACT.md ## Access log field mapping`'s in-place edit will ALSO carry verbatim (per §13 below). The §13 block + the §11 block are synchronized (no drift).

The format-string literal that the empirical pin records (the operator string the formatter must emit) is what `internal/accesslog/format.go`'s `Default()` implements; the fixture-0006 driver's positional regex anchors on the literal `[`, `]`, `"`, space delimiters from this pin. A future image bump that changes the operator order or the delimiter shape would produce a fixture-run regression on the parser regex first, then on the per-field tier rule — both fail loud.

## 12. Deferred decisions (the planner / implementer settles these)

Items the SPEC names but does not finalize; the planner closes them in PLAN.md or the implementer closes them at task time per the SPEC's recommendation.

1. **`accesslog.Sink` interface — Submit return type.** Two viable shapes: (a) `Submit(*Record)` (no return; drop-newest is signaled via the counter only); (b) `Submit(*Record) error` (drop-newest returns a non-nil sentinel error so the caller can branch). **Recommendation: (a)** — the counter is the operator-visible surface; the per-call return would force every emit site to thread the error through the deferred-emit path for no downstream consumer. Planner records in PLAN.md.

2. **`Cluster.PickEndpoint()` accessor surfacing for the router actions.** `routerAction.do` and `routerActionH2.doH2` need the resolved endpoint for `UPSTREAM_HOST`; today the pick happens inside `Cluster.Dial` and `Cluster.DialH2` without surfacing. Two viable shapes: (a) the dial methods return `(net.Conn, Endpoint, error)` / `(*ClientConn, Endpoint, error)` instead of `(net.Conn, error)` / `(*ClientConn, error)`; (b) the action calls `c.PickEndpoint()` separately and then `c.DialEndpoint(ep)` (a new constructor on `*Cluster` that takes a pre-picked endpoint). **Recommendation: (a)** (smaller surface change; the dial methods already perform the pick internally and just need to return it; the API change is contained to two return-tuple expansions). Planner records in PLAN.md.

3. **H2 path's `BYTES_SENT` accounting.** The H1 path uses `byteCounterWriter` wrapping the `bufio.Writer`; the H2 path's `StreamWriter.WriteData` does NOT have an `io.Writer` shape. Two viable shapes: (a) sum `len(resp.Body)` directly (no header bytes counted; close-but-not-equal to Envoy's `BYTES_SENT` semantics — Envoy counts response *body* bytes, not header bytes; this is correct); (b) implement a thin `byteCountingStreamWriter` adapter with a separate `WriteData` overlay. **Recommendation: (a)** — Envoy's `BYTES_SENT` is response body bytes only per the v1.37.2 docs; summing `len(resp.Body)` directly is correct and zero-overhead. Planner records in PLAN.md.

4. **Fixture-0006 driver pattern: in-band assertions vs. generic `LogFileExpectations` Driver-interface extension.** Decision G outlines a driver-side polling-and-tier-applying pattern; the runner could surface this as a generic Driver-interface extension or keep it in-band like the 0004/0005 drivers do. **Recommendation: in-band** — matches the 05.2 + 06.1 in-band precedent; the per-fixture pattern is established. Planner records in PLAN.md.

5. **`server.accesslog_dropped` HELP-text entry.** 06.1's stats writer emits HELP-text lines per ADR-0061 Rule SN6; the planner picks at PLAN time whether to add a HELP-text entry for `envoy_server_accesslog_dropped` to `internal/stats/name.go`'s `helpText` map. **Recommendation: add it** for symmetry with 06.1's discipline (`"envoy_server_accesslog_dropped": "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."`). Planner records in PLAN.md.

6. **`:AUTHORITY` cross-side normalization.** §7.5 documents the divergence (subject ephemeral port vs. reference fixed port); the driver normalizes both to a canonical form before applying Tier-E byte-equality. The exact normalization rule (strip-port-and-host-replace vs. host:port substitution) lands at PLAN time. **Recommendation: strip the port** and assert the host part is byte-equal; the host part is `127.0.0.1` on both sides (the reference container resolves `host.docker.internal` from inside the container, and the fixture's Mounts ensure both sides write to the same observation surface). Planner records in PLAN.md.

7. **Concrete ADR numbers for ADR-0066..ADR-0069.** Per `DECISIONS.md` tail at master `9acfc0b` being ADR-0065, the next-free is ADR-0066; 06.2's four ADRs land at ADR-0066..ADR-0069. The planner re-verifies next-free at write time (per ADR-0004's autonomous-numbering rule) and assigns the four anticipated topics to the four numbers in the order they're authored in PLAN.md. The topical ordering above (architecture / boundary-validation / equivalence-shape / counter-naming) is the suggested authoring order; the planner may permute.

8. **Direct_response action's `start time.Time` capture.** `directResponseAction.do` is currently a synchronous-write function with no start-of-action timing; the planner adds a leading `start := time.Now()` line and threads it into the `defer emit`. The recommendation is at the function entry; alternative placements (e.g., at the HCM dispatch entry, threaded through to the action) would surface a different `DURATION` semantics (downstream-request wall-clock vs. action wall-clock). **Recommendation: at function entry of the action's `do`** to match `routerAction.do` and `routerActionH2.doH2` — uniform `DURATION` semantics across all four sites. Planner records in PLAN.md.

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 `## Access log field mapping` subsection (full population)

The `## Access log field mapping` placeholder at `BEHAVIOR_CONTRACT.md` lines 170–174 is currently empty (reads `_to be filled per-phase as needed._`). Phase 06.2 fills it with the 15-operator format table from §6 above PLUS the three-tier matrix from §7.4 PLUS the empirical-pin block from §11 PLUS the `X-ENVOY-ORIGINAL-PATH?:PATH` fallback note from §6.1.

```
The access-log field mapping enumerates every operator in the Envoy default
access-log format (15 operators in identical positions on every record) per
ADR-0066, the per-operator equivalence tier per ADR-0068's three-tier matrix,
and the empirical-pin block recording the verbatim format-string shape from
reference Envoy v1.37.2. The differential equivalence claim (the
"Semantically equal after field-mapping" predicate from the Equivalence
Matrix row above) IS the three-tier matrix below.

15-operator default format (per ADR-0066; empirical-pin in §11 of the 06.2 SPEC):

[<START_TIME>] "<:METHOD> <:PATH> <PROTOCOL>" <RESPONSE_CODE> <RESPONSE_FLAGS>
<BYTES_RECEIVED> <BYTES_SENT> <DURATION> <RESP-SVC-TIME> "<X-FORWARDED-FOR>"
"<USER-AGENT>" "<X-REQUEST-ID>" "<:AUTHORITY>" "<UPSTREAM_HOST>"

Three-tier matrix (per ADR-0068):

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

X-ENVOY-ORIGINAL-PATH?:PATH fallback note (per 06.2 SPEC §6.1):
  Operator #3 in the format is %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% — emit the
  original-path header if present, else fall through to :PATH. Neither side
  emits X-ENVOY-ORIGINAL-PATH on fixture 0006's workload (envoy-go does not
  inject it; reference Envoy doesn't either, because fixture 0006 has no
  path_rewrite-bearing route); both sides emit :PATH via the fallback. A
  future phase introducing path-rewriting must re-evaluate fixture 0006's
  Tier-E/F expectations under the new behavior.

Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log
under the 5-request workload from 06.2 SPEC §7.2; reference image v1.37.2 at
ENVOY_TARGET.md SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd):

<<TBD: pinned at PLAN Task N — empirical scrape>>
```

### 13.2 `## Equivalence Matrix` row (existing row, definition concretized)

The `## Equivalence Matrix` row at line 18 (`Access log records | Semantically equal after field-mapping`) STAYS as-is — its text is already correct as a forward-looking placeholder. The 06.2 phase-done commit's in-place edit makes the row's `## Access log field mapping` reference **load-bearing** — the populated subsection (per §13.1 above) IS the "Semantically equal after field-mapping" predicate. No row text change is needed.

This is unlike 06.1's matrix-row addition (where the new "Stats output" row was added because no row existed); 06.2's matrix surface is already shaped, and 06.2 just concretizes its definition. Per Decision Z of the brainstorm dispatch's "anti-patterns to avoid" list (no new row added; existing row text unchanged), the in-place edit lands only on the `## Access log field mapping` subsection itself.

## 14. Testing strategy (per Decision E + Decision G + Decision J)

### 14.1 Unit tests (`internal/accesslog/`)

- **`accesslog_test.go`** — Sink interface surface; `Record` struct field accessors; size-of-Record allocations; `Sink` interface implements the documented contract (Submit + Close).
- **`format_test.go`** — `Default()` happy-path: emits the literal 15-operator format line; the line ends in a single `\n` (no embedded LF anywhere else); quoted-operator escaping (literal `"` → `\"`); the 5 unplumbed operators emit `-` verbatim (literal positions 6, 7, 10, 11, 13 — RESPONSE_FLAGS / BYTES_RECEIVED / RESP-SVC-TIME / X-FORWARDED-FOR / X-REQUEST-ID); empty-string fields (e.g., `Record.UserAgent == ""`) emit `-` per Envoy's missing-value convention; `UpstreamHost == ""` (zero-value `Endpoint`) emits `-` per Tier-F direct_response handling.
- **`writer_test.go`** — happy path (submit N → file has N lines; race-clean concurrent Submit from M goroutines, file has exactly M*N lines); drop-newest backpressure (channel pre-filled to capacity 4096; the 4097th Submit increments the dropped counter, the rate-limited log fires once); rate-limited diagnostic (the 4098th and 4099th Submits within 1 second do NOT emit a second log line); `Close()` after pending records (the writer drains the channel before closing the fd).

### 14.2 Unit tests (`internal/filter/hcm/`)

- **`accesslog_emit_test.go`** — `Filter.emitAccessLog` for H1 + H2 record-shape correctness; status-code-zero (the H2 ctx-cancel sentinel) skips emission (no Submit fired); `picked.Host == ""` (direct_response) renders as `Record.UpstreamHost == ""` → formatter emits `-`; multiple sinks all receive the same record; mock-sink that captures records.
- **`bytecounter_test.go`** — `byteCounterWriter` happy path; `n` accumulates correctly across multiple `Write`s; partial-write with `n < len(p)`.

### 14.3 Unit tests (existing-package extensions)

`internal/bootstrap/`:
- New unit tests for the `access_log[]` parser: parse-success on 0/1/N entries; parse-fail on `log_format` present per Decision C / ADR-0067; parse-fail on `path` empty/absent; silent-ignore on non-file typed-config types.

`internal/filter/hcm/`:
- Extended `actions_test.go` / `h2dispatch_test.go`: each of the four finalization sites' deferred emit fires with the correct primitives (start, bytes, picked, statusCode) on an end-to-end mock-sink test.

### 14.4 Differential fixture `0006-access-log` (per §7 above)

The 5-request workload + per-record three-tier matrix per Decision D + the polling-loop drain discipline per Decision G.

### 14.5 h2spec re-run (gate (c))

Per Decision K (gate (c)), phase 06.2 doesn't touch H2 wire code. h2spec gates remain at 53/53 PASS — already-pinned at the ADR-0051 SHA. Gate (c) re-runs unchanged.

### 14.6 Fuzzers (gate (d))

Existing seven fuzzers re-run at the 30s ADR-0018 budget:
- `internal/bootstrap.FuzzBootstrapLoad`
- `internal/filter/tcpproxy.FuzzTcpProxyFilter`
- `internal/tls.FuzzTLSContextParse`
- `internal/filter/hcm.FuzzHCMConfigParse`
- `internal/filter/hcm/h2.FuzzFrameStream`
- `internal/filter/hcm/h2.FuzzHPACKDecode`
- `internal/stats.FuzzPromTextFormat` (06.1)

**NEW: `internal/accesslog.FuzzAccessLogFormat`** — fuzzes adversarial `Record` field values (control chars, 8-bit bytes, large strings, `\n` / `\r` / `"` in headers) into `accesslog.Default`. Asserts: (i) the formatter NEVER produces a record with embedded LF (the line terminator is `\n`; embedded LFs would corrupt the record stream by appearing as record boundaries); (ii) quoted operators escape literal `"` to `\"` per Envoy's convention. 30s budget per ADR-0018.

Total fuzzer count post-06.2: **8** (the eighth fuzzer overall, joining the seven from 06.1).

### 14.7 Race detector + lint (gate (e))

`go vet ./... && golangci-lint run ./... && go test -race -count=1 ./...` clean. Race-detector specifically exercises:
- Concurrent `Sink.Submit` from N goroutines.
- Concurrent submit + writer-goroutine consumption (the channel-receive vs. channel-send race the Go runtime guards).
- Drop-newest backpressure under sustained-overload synthesis.
- Sink `Close()` while submit is in-flight (drop-or-deliver semantics asserted by the test; `Close()` is the boundary).

## 15. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 06.2's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) **non-vacuous** (fixture 0006 differential green; second non-vacuous gate-(a) on the observability surface).
- [ ] `internal/accesslog/` package exists; `Sink` interface + `Record` struct + `Default` formatter + `AsyncFileSink` writer + `RegisterDroppedCounter` wiring all implemented; the placeholder phase-00 `doc.go` is replaced.
- [ ] **No third-party access-log dependency.** `go.mod` does not contain a logging library import (`github.com/sirupsen/logrus`, `go.uber.org/zap`, etc.); grep-verifiable.
- [ ] The 15-operator default format is implemented per §6 (operator order matches the §11 empirical pin verbatim); Tier-E/F/S tier assignments match Decision D's matrix.
- [ ] AsyncFileSink's drop-newest discipline is enforced: full-channel Submit increments `server.accesslog_dropped` and emits a rate-limited diagnostic; no queue-depth gauge.
- [ ] Bootstrap parser rejects `log_format` / `format_string` / `json_format` per Decision C / ADR-0067 with the verbatim error message; silently-ignores non-file typed-config types; accepts 0-or-N file-type entries with required `path`.
- [ ] All four finalization sites in `internal/filter/hcm/actions.go` and `internal/filter/hcm/h2dispatch.go` carry `defer filter.emitAccessLog(...)` per Decision F: `directResponseAction.do` (H1), `routerAction.do` (H1), `h2DirectResponseAdapter.WriteH2` (H2 direct_response wrapper), and `routerActionH2.doH2` (H2 router). H2 ctx-cancel skips emission per the zero-status sentinel.
- [ ] `BEHAVIOR_CONTRACT.md ## Access log field mapping` is populated with the 15-operator format table from §6 + the three-tier matrix from §13.1 + the empirical-pin block (verbatim from §11 once PLAN Task N's scrape lands) + the `X-ENVOY-ORIGINAL-PATH?:PATH` fallback note from §6.1; the in-place edit lands at the phase-done commit (NOT the SPEC commit) per ADR-0052's discipline. The existing `## Equivalence Matrix` row at line 18 is unchanged in text; its definition is now load-bearing per §13.2.
- [ ] All four 06.2 ADRs (ADR-0066 architecture / ADR-0067 reject-log_format / ADR-0068 differential-equivalence-shape / ADR-0069 counter-naming) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. The ADR-numbering-shift discipline from ADR-0045 + ADR-0004 is honored (the planner verified next-free at write time and the four numbers are contiguous; topical-vs-commit-order non-monotonicity is permitted and recorded in each ADR's `Lands-in-task` field per the 05.2 ADR-0055..ADR-0058 + 06.1 ADR-0059..ADR-0064 precedents).
- [ ] Fixture `0006-access-log/` is committed in full: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The 5-request workload + polling-loop drain discipline (Decision G) + per-record three-tier matrix (Decision D) is implemented in `driver/driver.go`; the `--concurrency 1` reference invocation is honored.
- [ ] **The empirical-format pin is filled.** §11's `<<TBD: pinned at PLAN Task N — empirical scrape>>` placeholder is replaced with the verbatim 5-record scrape from reference Envoy v1.37.2 at the `ENVOY_TARGET.md` SHA. The `BEHAVIOR_CONTRACT.md ## Access log field mapping` subsection's empirical-pin block carries the same verbatim scrape (no drift between SPEC §11 and the contract addition).
- [ ] `test/conformance/h2spec/` is UNCHANGED; pin still at the ADR-0051 SHA; 53/53 PASS.
- [ ] No phase-04 / 05.1 / 05.2 / 06.1 fixture (`0000`/`0001`/`0002`/`0003`/`0004`/`0005`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `STATE.md` is at lifecycle-state 6 for 06.2; `ROADMAP.md` row `06.2` is `done`; row `06` (parent) flips `in-progress → done` AT THE SAME phase-done commit. The §5.1 phase-done commit's commit-message body explicitly names BOTH ROADMAP-row transitions per parent SPEC §5.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the §5.3 verification protocol; SHA-fill for each task entry per the phase-04 / 05.1 / 05.2 / 06.1 convention.
- [ ] The carry-forward dispositions table (§10) is faithfully recorded: NO carry-forwards bundled into 06.2; the 06.1 REVIEW M-8 polling-loop adoption is prophylactic only (does NOT close M-8 against fixture 0005); 05.2 M-4 / M-10 / M-12 + 05.2 prose Minors + 06.1 12 Minors are all unchanged in disposition.
- [ ] **`FuzzAccessLogFormat` is committed** under `internal/accesslog/fuzz_test.go`; runs clean at the 30s ADR-0018 budget; total fuzzer count post-06.2 is 8.
- [ ] No third-party access-log library is imported. The `internal/accesslog` package's external dependencies are limited to the Go standard library (`os`, `time`, `strings`, `bytes`, `sync`, `sync/atomic`, `io`, `fmt`, `log`) plus `internal/stats` (for the drop-counter Counter type).

When all boxes above are checked, phase 06.2 is `done`, the parent row `06` flips `in-progress → done` AT THE SAME COMMIT (mirroring the 05 / 05.1 / 05.2 closure pattern), and the project advances to phase 07 (filter-chain framework) at lifecycle-state 1.

## 16. References

- **Brainstorm-close artifact (this SPEC's design source):** the lifecycle-state-0 brainstorm session held at master `9acfc0b` produced Decisions A–M (encoded as the SPEC's contract per the dispatch's "design decisions to encode" enumeration). The decisions are recorded inline in this SPEC's body (§§1–13) rather than in a separate BRAINSTORM.md (the brainstorm dispatcher inlined them per the lifecycle-state-1 routing); this SPEC IS the brainstorm-close artifact.
- **Parent BRAINSTORM:** `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` — the upstream design source for the parent observability phase; defers all access-log architecture decisions to this sub-phase per §1 split table. Cite §1 (split table) for the parent-SPEC-level scope split.
- **Parent master SPEC:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` — phase-06 parent; carries the cross-cutting decisions that apply to BOTH 06.1 and 06.2 (no third-party observability dependencies, BEHAVIOR_CONTRACT placeholder discipline, parent-rollup phase-done at 06.2's phase-done).
- **Sibling SPEC (preceding sub-phase):** `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` — the structural template this SPEC mirrors (§-numbering, section ordering, header tone, acceptance-bullet format, doctrine-citation discipline). 06.2 IS a sibling document.
- **Placeholder README being superseded:** `docs/envoy-go/phases/06.2-access-log/README.md` — the placeholder this SPEC supersedes at the SPEC commit.
- **Structural precedents (sub-phase SPEC shape):** `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md`, `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md`, `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md`.
- **BEHAVIOR_CONTRACT.md:** `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the contract this SPEC's §13 extensions land in (in-place edit at phase-done per ADR-0052). The empty `## Access log field mapping` placeholder at lines 170–174 is what this phase populates; the existing `## Equivalence Matrix` row at line 18 (`Access log records | Semantically equal after field-mapping`) is concretized by the population without a row text change.
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Cited in §11's empirical-pin gate (the format-string pin's verbatim scrape uses this SHA against the fixture-0006 reference bootstrap).
- **DECISIONS.md:** `docs/envoy-go/DECISIONS.md` — ADR-0001 (template), ADR-0004 (autonomous-numbering rule), ADR-0008 (Envoy pin, referenced via ENVOY_TARGET.md), ADR-0010 (`dns_lookup_family: V4_ONLY` for STRICT_DNS reference clusters), ADR-0017 (small-mechanical-fixes do not require ADRs), ADR-0018 (fuzzer 30s short-budget policy), ADR-0028 (`--concurrency 1` reference invocation), ADR-0045 (planner-time-split discipline), ADR-0051 (h2spec pin SHA), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization), ADR-0058 (last 05.2 ADR; trailers observed-but-not-forwarded), ADR-0059 (06.1 stats architecture; the architectural-shape sibling of 06.2's ADR-0066), ADR-0061 (06.1 SN1–SN8; SN5 anchors 06.2's ADR-0069 counter naming), ADR-0062 (06.1 stats differential-equivalence; the differential-equivalence sibling of 06.2's ADR-0068), ADR-0065 (06.1 boundary-validation at user-input; the boundary-validation sibling of 06.2's ADR-0067; **last extant ADR at master `9acfc0b`** — the 06.2 ADRs start at ADR-0066).
- **BOOTSTRAP_PROMPT cross-references:**
  - **§5** (Phase Lifecycle State Machine) — the lifecycle states 1 (SPEC drafting; this commit's deliverable) → 6 (REVIEW approved + phase-done) that 06.2 traverses.
  - **§5.3** (Commit message format) — the phase-done commit message format `phase 06.2: phase-done — access-log lands; ROADMAP rows 06.2 + 06 → done [ADR-0066, ADR-0067, ADR-0068, ADR-0069]` plus differential-surface + conformance summary.
  - **§6.2** (How to split — planner-time-split discipline) — the discipline ADR-0045 invokes for the 06.1 + 06.2 split; this SPEC honors §6.2 by being one of two sibling sub-phase SPECs under the parent.
  - **§7.5** (Phase-done gate — six-gate checklist) — the gate set §3 specializes for 06.2.
  - **§4.1** (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit) — the row-flip discipline §4.4 honors.
- **ROADMAP.md:** `docs/envoy-go/ROADMAP.md` — rows `06`, `06.1`, `06.2` per the split landed in 06.1's SPEC commit (rows in their current shape at master `9acfc0b`).
- **PROGRESS-style precedents:** `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`, `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md`, `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` — the SHA-fill convention 06.2's PROGRESS.md mirrors.
- **Master commit anchors:** master HEAD at SPEC commit-to-be: `9acfc0b` (parent of THIS SPEC commit; the worktree was branched from this SHA per the brainstorm dispatch). 06.1 phase-done: `ae8276b`. 05.2 phase-done: `0c01ed6`. 75a6bf9: prior master before fast-forward (cited in the parent SPEC's "depends on" anchor — phase 05's phase-done state recorded in `STATE.md`).
