# Phase 06.1 — Stats + Prometheus admin endpoint (`internal/stats` package, `/stats/prometheus` exposition, 17 stat-emit call sites, fixture 0005)

**Phase id:** `06.1`
**Slug:** `06.1-stats-prometheus`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md into formal SPEC shape per ADR-0004)
**Depends on:** phase 05 (done at master `75a6bf9`)
**Parent phase:** `06-observability-baseline` (in-progress; split into `06.1` + `06.2` per ADR-0045; `06.1` does NOT close the parent — `06.2` does on its phase-done commit, mirroring the 05 / 05.1 / 05.2 closure pattern)
**Master design document:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` (this commit) and `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (brainstorm-close artifact, master `75a6bf9` parent). The BRAINSTORM is the authoritative design source; this SPEC distills BRAINSTORM §§2–9 into formal contract language.
**Differential surface at end of sub-phase:** NEW fixture `test/differential/0005-prometheus-stats/` is differentially green (gate (a) is **non-vacuous** for the first time on the observability surface): 5-request defined-load workload (`[200, 200, 404, 200, 502]`) issued sequentially at both proxies, before/after `/stats/prometheus` snapshots scraped + parsed, per-counter delta-equality and per-gauge snapshot-equality asserted across the 17 stat names enumerated in §6 below. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing` remain green (gate (b)). h2spec conformance gate (c) re-runs at the ADR-0051 pin and stays at 53/53 PASS (the 06.1 surface is observability-only and does not touch H2 wire code). Fuzz (d) re-runs the existing seven fuzzers AND adds a new `FuzzPromTextFormat` over the Prometheus exposition writer at the 30s ADR-0018 budget. Build/vet/lint/test (e) and REVIEW (f) apply normally. ROADMAP row `06.1` flips `planned → in-progress` at the SPEC commit and `→ done` at the phase-done commit; the parent row `06` stays `in-progress` (closes only at 06.2's phase-done per the 05/05.1/05.2 mirror).

---

## 1. Purpose

Phase 06.1 lands envoy-go's stats subsystem — an in-tree atomic-counter Registry backed by an admin-side Prometheus text-format exporter — and threads stat-emit call sites through the listener / HCM / cluster hot paths so a Prometheus scrape of `/stats/prometheus` returns a behaviorally-equivalent metric set to upstream Envoy v1.37.2 under a defined load. Concretely:

1. **A new `internal/stats` package** — `Registry`, `Counter`, `Gauge`, hierarchical-dotted-name helpers, and the Prometheus text-format writer that walks the Registry and emits `# HELP` / `# TYPE` / metric lines per the [Prometheus exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/) sorted alphabetically by Prometheus name (per BRAINSTORM §3 + §4.3). Registry is constructed once at boot, threaded explicitly through the existing component graph; Counter/Gauge increments are lock-free atomics; `Registry.Walk(fn)` runs under an `RWMutex` whose write lock is held only at boot-time register sites. **No third-party Prometheus library** (no `prometheus/client_golang` dependency); the writer is ~50 LoC inline. The architectural rationale is recorded in ADR-0059 anticipated (per §8 below).

2. **A `GET /stats/prometheus` admin endpoint** added alongside the phase-01 `/ready` handler — read-only against the Registry, returns `Content-Type: text/plain; version=0.0.4; charset=utf-8`, sorts metrics alphabetically by Prometheus name, emits the static-`# HELP`-text discipline from §11.2 below. Response shape: one `# HELP` + one `# TYPE` per Prometheus-name group, then one metric-line per fully-qualified label set, then a blank line between groups. Other admin endpoints (`/stats`, `/stats?format=json`, `/stats?filter=`) are explicitly out of scope per §11 below — phase 08 (admin-api-and-drain) owns the full admin surface; 06.1 ships only the alias path required by the differential fixture.

3. **17 stat-emit call sites threaded through listener / HCM / cluster.** Per BRAINSTORM §2.3 the catalog is exactly 17 names (2 listener + 5 HCM + 8 cluster + 2 server), enumerated verbatim in §6 below. Each metric is created at register-time (boot), held by reference on the relevant struct, incremented lock-free on the hot path, never re-registered. The hot-path edits total ~15 LoC across ~5 files (per BRAINSTORM §4.2); the bulk of new code is the `internal/stats` package itself.

4. **Stat-name → Prometheus-name flattening rules SN1–SN8** (per BRAINSTORM §7.1) populated into `BEHAVIOR_CONTRACT.md`'s currently-empty `## Stat-name mapping` placeholder, plus a new equivalence-matrix row for "Stats output" (per BRAINSTORM §7.3). **Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 are settled at brainstorm-close and transcribed verbatim from BRAINSTORM §7.1 into §10 below.** **Rule SN4 (status-class flattening: `_Nxx` suffix → base-name + `envoy_response_code_class` label form) was the empirical-verification gate** — its specifics were pinned at SPEC-drafting time per BRAINSTORM §2.3.1 by running reference Envoy v1.37.2 (the `ENVOY_TARGET.md` pin, server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) with a minimal HCM + 1-cluster bootstrap, scraping `/stats/prometheus`, and copying the verbatim scrape lines into §10.1 below. **Verified form:** the trailing class digit is stripped from the metric name (so `cluster.<n>.upstream_rq_2xx` flattens to base name `envoy_cluster_upstream_rq_xx` ending in literal `_xx`); the label name is `envoy_response_code_class`; the label value is the single class digit as a string (`"2"`, `"3"`, `"4"`, `"5"`) — not `"2xx"` and not the per-exact-status `"200"`. The SPEC commit lands the verified rule with verbatim evidence in §10.1.

5. **Concurrency invariant LBP-1 ("list before play")** — per BRAINSTORM §5.1: the Registry's *list of metrics* is mutable only during boot. Once `bootstrap.Load() → cluster.NewManager.Register* → listenerManager.New.Register* → admin.New` completes, the list is frozen; subsequent `NewCounter` / `NewGauge` calls panic. This is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments. A unit test in `registry_test.go` sets a "frozen" flag after boot and panics if `NewCounter` / `NewGauge` is called post-freeze (per §11.6 below).

6. **Carry-forward dispositions per BRAINSTORM §2.4:** 05.2 REVIEW Minor M-9 ("Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error") is **bundled with 06.1** because the 05.2 REVIEW explicitly deferred it to "phase-06 observability when logging/metrics surface lands"; ~5 LoC fix + ~20 LoC test. 05.2 REVIEW Minor M-4 (`readClientPreface` not ctx-aware), M-10 (`SETTINGS_TIMEOUT` absent), M-12 (`closedStreams` map unbounded) are explicitly **deferred** to a dedicated H2-hardening sub-phase or the upstream-robustness family, with target-phase candidates enumerated in §13 below.

7. **A new differential fixture `test/differential/0005-prometheus-stats/`** — the project's first observability-surface differential; first fixture asserting per-stat behavioral-delta equivalence between envoy-go and reference Envoy v1.37.2. Driver + expected-deltas table per BRAINSTORM §2.5; full driver outline in §7 below. Fixture name: `0005-prometheus-stats` (sequential after `0004-h2-routing`).

8. **A new fuzz target `internal/stats.FuzzPromTextFormat`** at the 30s ADR-0018 budget, fuzzing adversarial stat names + label values into the Prometheus text-format writer. Total fuzzer count post-06.1: 7 (the existing six from phases 01..05.2 plus this one). Rationale per BRAINSTORM §6.4: escaping bugs are the most likely class of bug in the writer, and the Prometheus parser is strict.

9. **Anticipated ADRs:** six ADRs per BRAINSTORM §8, numbered ADR-0059..ADR-0064 (next-free per the `DECISIONS.md` tail at master `75a6bf9` being ADR-0058). The planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule. Topics enumerated in §8 below.

After phase 06.1, the project has proven the first half of its seventh central engineering claim: *envoy-go emits behaviorally-equivalent counter and gauge signals under a defined load — visible at `/stats/prometheus`, byte-equal in metric name + label keys + label values + types to upstream Envoy's exposition format on the 17 enumerated names — without coupling to any third-party stats or Prometheus dependency.* The access-log half of the claim is delivered by 06.2; the parent ROADMAP row `06` flips to `done` at 06.2's phase-done.

## 2. Non-purposes

Phase 06.1 does **not** do any of the following. Most are explicit non-goals from BRAINSTORM §2.3, §5.6, §9; a few are scope-narrowings the SPEC introduces by consolidating BRAINSTORM's "deferred to phase X" annotations. Each non-purpose is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Stats coverage non-goals (per BRAINSTORM §2.3 + §9)

- **Histograms.** No `*_time_ms`, `*_request_size_bytes`, `*_response_size_bytes`, or any other histogram-shaped metric. Envoy uses circllhist (dynamic-bucket) internally and bridges to Prometheus's fixed-bucket shape via `histogram_bucket_settings`; byte-equivalent matching against the v1.37.2 reference is its own design pass and wants its own brainstorm covering circllhist→Prometheus bucket mapping. → 06.x or upstream-robustness family with own brainstorm.
- **Per-endpoint cluster stats.** `cluster.<n>.<endpoint_address>.cx_total` and the rest of Envoy's `enable_per_endpoint_stats=true` surface. 06.1 emits cluster-level only. → xDS-EDS phase revisits.
- **TLS subsystem stats.** `cluster.<n>.ssl.handshake`, `cluster.<n>.ssl.connection_error`, etc. → upstream-robustness family.
- **Runtime / admin / server stats beyond `server.live`.** Envoy emits ~30 server-scope stats (`server.uptime`, `server.memory_allocated`, `server.parent_connections`, etc.); 06.1 ships `server.live` only. `server.uptime` is named explicitly NOT-EMITTED in BRAINSTORM §2.3 because it depends on monotonic-clock + per-scrape recompute and pairs naturally with the histogram brainstorm. → phase 08 (admin API).
- **TCP-proxy filter stats.** No in-flight TCP proxy users on the differential surface; deferred until a workload demands it.
- **1xx response counters.** Envoy emits `downstream_rq_1xx`; 06.1 omits because phase 04 doesn't exercise 1xx meaningfully and the differential workload (per §7) does not include a 100 / 101 / 103 status. → deferred until exercised by a fixture.
- **`stats_config.stats_tags` regex extraction.** 06.1 hardcodes the extraction logic in `internal/stats/name.go` (per §10 Rules SN1–SN5); the bootstrap proto's `stats_config.stats_tags[]` field is silently ignored if present. → future stats-config phase or an xDS-RTDS revisit.
- **Hot reload of stats schema.** xDS-CDS would dynamically register new clusters at runtime, which would race `NewCounter` against `Walk`; explicitly out of scope for 06.1, governed by the LBP-1 invariant (§5.1 below). → xDS family.

### 2.2 Admin-endpoint coverage non-goals (per BRAINSTORM §2.5)

- **Other `/stats` paths.** `/stats` (text/plain, internal-name format), `/stats?format=json`, `/stats?filter=<regex>`, `/stats/recentlookups`, `/stats?reset` are NOT shipped. `/stats?format=prometheus` (the query-string equivalent of `/stats/prometheus`) is also NOT shipped — only the alias path. → phase 08.
- **Other admin endpoints.** `/clusters`, `/listeners`, `/config_dump`, `/server_info`, `/stats/scopes`, etc. → phase 08.

### 2.3 Process / lifecycle non-goals

- **Persistence.** Counters reset on process restart. Same as Envoy. No on-disk persistence, no graceful-drain hand-off (BRAINSTORM §5.4). → phase 08 owns drain.
- **`server.live` reset semantics.** Set to 1 the first time `admin.handleReady` returns 200; never reset by 06.1. Graceful-drain (which Envoy uses to flip `server.live` back to 0) is phase 08's deliverable. → phase 08.
- **Scrape rate-limiting / authentication.** No auth on `/stats/prometheus`, no per-source rate-limit. Envoy treats admin as trusted-loopback per its docs; 06.1 inherits. → phase 08 if ever.

### 2.4 Carry-forward non-purposes (per BRAINSTORM §2.4)

The following 05.2 REVIEW Minor findings are **explicitly deferred** OUT of 06.1 (they are NOT bundled with 06.1; the only bundled carry-forward is M-9):

- **M-4** `readClientPreface` not ctx-aware (`internal/filter/hcm/h2/conn.go`) — H2 connection hardening, not observability. → dedicated H2-hardening sub-phase or upstream-robustness family.
- **M-10** `SETTINGS_TIMEOUT` absent (`internal/filter/hcm/h2/client.go`) — same reasoning as M-4. → same target-phase candidates.
- **M-12** `closedStreams` map unbounded (`internal/filter/hcm/h2/conn.go`) — long-lived-conn memory growth is a hardening concern, not an observability one. → same target-phase candidates.

The full disposition table is recorded in §13 below for the reviewer's audit trail.

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 06.1)

Per doctrine `D-3.6`, phase 06.1 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 06.1 |
|---|---|
| (a) new/changed differential fixtures green | **Non-vacuous (first time on the observability surface).** New fixture `test/differential/0005-prometheus-stats/` passes: 5 sequential requests per proxy with target statuses `[200, 200, 404, 200, 502]` (3 × 200 routed through cluster `c0`, 1 × 404 from envoy-go HCM directly, 1 × 502 from a controlled backend that explicitly returns 502); pre-load and post-load `/stats/prometheus` snapshots scraped + parsed at both proxies; per-counter delta-equality (`delta_envoy_go == delta_envoy`) and per-gauge snapshot-equality (`after_envoy_go == after_envoy`) asserted on the 17 stat names enumerated in §6. The expected-deltas table from §7.3 is the contract; non-listed metric names in either side's exposition output are ignored by the differential (per §7.4's allow-list discipline). |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing` all pass without regression. The phase-06.1 changes are additive — `cluster.NewManager` / `cluster.NewManagerWithBaseDir` / `admin.New` signatures gain a `*stats.Registry` parameter (per §4.4 below); pre-existing fixtures' driver code threads a default-empty Registry through the constructor changes; no existing fixture's behavioral expectations change. |
| (c) conformance suites pass | `test/conformance/h2spec/` re-runs at the ADR-0051 pin (`summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`) and reports `failed == 0` over the unchanged threshold list (sections 3, 4, 5, 6 ex-6.6, 7, 8 — 53/53 PASS at the 05.1+05.2 baseline). 06.1 doesn't touch H2 wire code, so this gate is unchanged. Pin is NOT bumped (D-3.7 reserves pin bumps for dedicated phases). |
| (d) new/existing fuzzers run clean for CI short-budget | Existing six fuzzers (`internal/bootstrap.FuzzBootstrapLoad`, `internal/filter/tcpproxy.FuzzTcpProxyFilter`, `internal/tls.FuzzTLSContextParse`, `internal/filter/hcm.FuzzHCMConfigParse`, `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode`) run clean at the 30s ADR-0018 budget. **NEW:** `internal/stats.FuzzPromTextFormat` runs clean at the same budget. Total: 7 fuzzers post-06.1. |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for the new `internal/stats` package (registry / counter / gauge / name / Prometheus writer; race-clean; LBP-1 enforcement); extended tests for `internal/admin/` (`/stats/prometheus` handler — 200 response, content-type, sorted output, empty-Registry edge case, escape-of-adversarial-label-values, HELP/TYPE-line correctness); extended tests for `internal/cluster/`, `internal/listener/`, `internal/filter/hcm/` covering the 17 stat-emit call-sites and their hot-path-correctness; the M-9 carry-forward unit test for `h2RouterActionAdapter.WriteH2`. `go test -race ./...` clean — concurrent `Counter.Inc` from N goroutines, concurrent `Gauge.{Inc,Dec,Set}`, `Walk` running while `Inc/Dec/Set` running, two concurrent `Walk`s. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 06.1. The complete file inventory itemizes every constructor signature change so PLAN time has no surprise scope (per BRAINSTORM §3 note 6).

### 4.1 New production code (in 06.1)

- **`internal/stats/doc.go`** (rewrite) — describes the package API (Registry / Counter / Gauge / Walk / NewCounter / NewGauge / freeze invariant) and the LBP-1 invariant. The placeholder `doc.go` already exists in tree from a 00-bootstrap stub; this commit replaces its contents.
- **`internal/stats/registry.go`** — `type Registry struct { mu sync.RWMutex; metrics []Metric; byName map[string]Metric; frozen atomic.Bool }`. Methods: `NewRegistry() *Registry`; `(*Registry).NewCounter(name string) *Counter` (panics if frozen, panics on invalid name, panics on duplicate name); `(*Registry).NewGauge(name string) *Gauge` (same); `(*Registry).Walk(fn func(Metric))` (RLocks, iterates `metrics` in registration order, calls `fn` for each — ordering is not part of the contract; the Prometheus writer sorts post-walk); `(*Registry).Freeze()` (sets `frozen = true`; called from `cmd/envoy-go/main.go` after admin server is up; idempotent). Name validation: `^[a-zA-Z_][a-zA-Z0-9_.]*$` per BRAINSTORM §5.2.
- **`internal/stats/counter.go`** — `type Counter struct { name string; v atomic.Uint64 }`. Methods: `(*Counter).Inc()` (1-cycle `AddUint64`); `(*Counter).Add(delta uint64)`; `(*Counter).Load() uint64`; `(*Counter).Name() string`. Implements `Metric` interface (Name + Type + Load-as-string).
- **`internal/stats/gauge.go`** — `type Gauge struct { name string; v atomic.Int64 }`. Methods: `(*Gauge).Inc()`; `(*Gauge).Dec()`; `(*Gauge).Add(delta int64)`; `(*Gauge).Set(value int64)`; `(*Gauge).Load() int64`; `(*Gauge).Name() string`. Negative values are permitted (a `Dec` not paired with an `Inc` is defensive — gauge reflects reality, per BRAINSTORM §5.2).
- **`internal/stats/name.go`** — hierarchical-dotted-name helpers: `flattenToProm(internal string) (promName string, labels []Label, err error)` per the Rules SN1–SN8 enumerated in §10 below; `escapeLabelValue(string) string` per the [Prometheus text-format escape rules](https://prometheus.io/docs/instrumenting/exposition_formats/) (`\` → `\\`, `"` → `\"`, `\n` → `\\n`); `Label struct { Key string; Value string }`. The static HELP-text map (per BRAINSTORM §4.5) lives in this file as `var helpText = map[string]string{ "envoy_listener_downstream_cx_total": "Total connections accepted on the listener.", ... }` for the 13 unique Prometheus names emitted by 06.1.
- **`internal/stats/prom.go`** — the Prometheus text-format writer. `WriteProm(w io.Writer, r *Registry) error`: walks the Registry, flattens each metric via `name.go`, groups by Prometheus name (status-class collapse joins the four `_Nxx` Prometheus names into one base-name group with four `envoy_response_code_class` label-keyed lines), emits `# HELP <name> <text>` then `# TYPE <name> <type>` then one metric line per fully-qualified label set, sorted alphabetically by Prometheus name. Group separator: blank line between groups. ~50 LoC inline (per BRAINSTORM §3).
- **`internal/stats/registry_test.go`**, **`counter_test.go`**, **`gauge_test.go`**, **`name_test.go`**, **`prom_test.go`** — unit tests per BRAINSTORM §6.1 (full enumeration in §11.1 below).
- **`internal/stats/fuzz_test.go`** — `FuzzPromTextFormat` per BRAINSTORM §6.4. Fuzzes adversarial stat-name strings + adversarial label-value strings into `WriteProm`; asserts no panic; asserts output round-trips through a Prometheus-format-aware parser without error.
- **`internal/admin/prometheus.go`** — `func handlePrometheus(r *stats.Registry) http.HandlerFunc`: returns a handler that writes `Content-Type: text/plain; version=0.0.4; charset=utf-8` then calls `stats.WriteProm(w, r)`. Errors from `WriteProm` are logged and otherwise ignored (no retry, no error response — too late, headers already sent; per BRAINSTORM §5.3).
- **`internal/admin/prometheus_test.go`** — handler unit tests per BRAINSTORM §6.1 (full enumeration in §11.1 below).

### 4.2 Changed production code (in 06.1)

- **`internal/admin/admin.go`** — `admin.New(ready *Ready, registry *stats.Registry) *Admin` signature change (was `admin.New(ready *Ready) *Admin`). The constructor registers `/stats/prometheus` alongside `/ready`. The `*stats.Registry` is held by reference; the Prometheus handler is constructed via `handlePrometheus(registry)` per §4.1 above.
- **`internal/admin/admin_test.go`** — extended: `admin.New(ready, registry)` call sites updated; new test coverage for `/stats/prometheus` route registration; the existing `/ready` tests pass unchanged.
- **`internal/admin/doc.go`** — updated to describe the new `/stats/prometheus` endpoint alongside `/ready`.
- **`internal/cluster/manager.go`** — `cluster.NewManager(clusters []*Cluster, registry *stats.Registry) *Manager` signature change (was `cluster.NewManager(clusters []*Cluster) *Manager`). On `cluster.Register()`, the manager calls into the Registry to create the 8 cluster-scope metrics per cluster: `cluster.<n>.upstream_rq_total` (counter), `cluster.<n>.upstream_rq_2xx`/`_3xx`/`_4xx`/`_5xx` (counters), `cluster.<n>.upstream_cx_total` (counter), `cluster.<n>.upstream_cx_active` (gauge), `cluster.<n>.membership_total` (gauge — Set to `len(endpoints)` once at register-time, per BRAINSTORM §2.3). The metrics are stored by reference on the `*Cluster` struct; runtime callers access them as `c.upstreamRqTotal.Inc()`, etc.
- **`internal/cluster/manager.go`** — `cluster.NewManagerWithBaseDir(clusters, baseDir, registry)` signature change (extends the existing `NewManagerWithBaseDir(clusters, baseDir)` — both constructors must be extended in the same commit; the test-fixtures-only constructor matches the production constructor's signature). Per BRAINSTORM §3 note 6, this is enumerated explicitly so it is not surprise scope at PLAN time.
- **`internal/cluster/cluster.go`** — `Cluster` struct gains 8 unexported metric-pointer fields (`upstreamRqTotal *stats.Counter`, etc.). The accessor `Cluster.Dial(ctx)` and `Cluster.DialH2(ctx)` are unchanged; per-dial metric updates land in the call sites in `dial.go` and `dial_h2.go` (per §4.2 below).
- **`internal/cluster/dial.go`** (extended) — on successful dial: `c.upstreamCxTotal.Inc()`, `c.upstreamCxActive.Inc()`, the returned `net.Conn` is wrapped in a `connWithGauge` that calls `c.upstreamCxActive.Dec()` exactly once on `Close()`. Hot-path edits: `+2 LoC` plus the `connWithGauge` wrapper (~10 LoC).
- **`internal/cluster/dial_h2.go`** (extended) — same wrapping pattern as `dial.go`. The `*h2.ClientConn` returned by `h2.NewClientConn` already wraps the underlying `*tls.Conn`; the gauge-wrapping happens at the `*tls.Conn` layer below the H2 client conn so `(*ClientConn).Close()`'s GOAWAY-then-FIN dispatch transitively triggers the gauge Dec. Hot-path edits: `+2 LoC` plus reuse of `connWithGauge`.
- **`internal/listener/manager.go`** (extended) — `listenerManager.New(listeners []Listener, registry *stats.Registry, hcmFactory ...) *Manager` signature change. On `listener.Register()`, the manager creates 2 listener-scope metrics per listener: `listener.<addr>.downstream_cx_total` (counter), `listener.<addr>.downstream_cx_active` (gauge). The address `<addr>` is normalized like Envoy does (`0.0.0.0:10000` → `0.0.0.0_10000`) per BRAINSTORM §2.3. The HCM factory captures `registry` so per-HCM-instance metric allocation works; concretely, the factory closure threads the Registry into each `hcm.NewFilter` invocation.
- **`internal/listener/listener.go`** (extended) — accept loop hot-path: on each accept, `cx_total.Inc()` + `cx_active.Inc()`; on accepted-conn close (driven by the connection-handler goroutine's deferred close), `cx_active.Dec()`. Hot-path edits: `+2 LoC`.
- **`internal/filter/hcm/filter.go`** (extended) — HCM dispatch entry: `+1 LoC` `downstream_rq_total.Inc()` on first byte of request line/headers (or, equivalently, when the route table is consulted — the planner picks the precise hook at PLAN time per §12 #1). Per-HCM-instance metric allocation per BRAINSTORM §4.1: at `NewFilter` time, the filter calls `registry.NewCounter("http.<stat_prefix>.downstream_rq_total")` and the four `_Nxx` counters; held by reference on the filter struct. The HCM `stat_prefix` is read from config (already plumbed from phase 04); fatal config error if absent (per BRAINSTORM §5.2).
- **`internal/filter/hcm/filter.go`** (extended) — HCM response hook: on response status finalization (before bytes hit the wire), the integer-divide `code / 100` selects the status-class counter and `Inc()`s it. `+3-5 LoC`. Out-of-range codes are unreachable from envoy-go's response path (per BRAINSTORM §5.2).
- **`internal/filter/hcm/actions.go`** (extended — H1 router action) — on `routerAction.do` request-dispatch entry: `c.upstreamRqTotal.Inc()`. On response status finalization: `c.upstreamRq<Nxx>.Inc()` per the same `code / 100` discipline as the HCM-side downstream counters. `+1 LoC` + `+3-5 LoC`.
- **`internal/filter/hcm/actions.go`** (extended — H2 router action) — same Inc pattern in `routerActionH2.do`. `+1 LoC` + `+3-5 LoC`.
- **`internal/filter/hcm/h2/router_action.go`** (extended — M-9 carry-forward bundled per BRAINSTORM §2.4) — `h2RouterActionAdapter.WriteH2` adds a `log.Printf("h2: doH2 error: %v", err)`-style log line on the `doH2` error path before returning. ~5 LoC + ~20 LoC test in a new `router_action_test.go` (or appended to the existing test file — planner's choice per §12 #5). The structured-logging discipline for 06.1 is plain `log.Printf` to stderr; phase 06.2 / phase 08 may upgrade to a structured logger.
- **`cmd/envoy-go/main.go`** (extended) — at boot: `registry := stats.NewRegistry()`; threads `registry` into `cluster.NewManager`, `listenerManager.New`, `admin.New`; after the listener manager begins accepting (and after admin is listening on its port), calls `registry.Freeze()` to enforce LBP-1. Adjusts the existing two `cluster.NewManager(clusters)` / `admin.New(ready)` call sites to the new signatures. `+5-8 LoC` net.
- **`cmd/envoy-go/main_test.go`** (extended) — bootstrap-variant smoke tests (pre-existing four variants from 05.2) thread a `stats.NewRegistry()` through the constructor changes; no new bootstrap variant added — the differential fixture 0005 covers the integration check.
- **`internal/bootstrap/bootstrap.go`** (extended) — the `Bootstrap` struct gains a `Stats *stats.Registry` field constructed in `Load()`; threaded into `cluster.NewManager` / `listenerManager.New` / `admin.New` at the call sites in `main.go`. **Or**, alternatively, the planner may keep `bootstrap.Load()`'s return shape unchanged and have `main.go` allocate the Registry separately (BRAINSTORM §4.1 shows the field-on-Bootstrap shape; PLAN time picks the final factoring per §12 #2). Default recommendation: field-on-Bootstrap so future xDS phases that add dynamic config-reload have a place to thread the Registry through a config-update path.

### 4.3 New harness and fixture code (in 06.1)

- **`test/differential/0005-prometheus-stats/`** — new fixture directory. Contents:
  - **`envoy-go.yaml`** — subject bootstrap. 1 listener (`l_h1`) binding `127.0.0.1:0` plaintext (no TLS — the fixture is observability-only; reusing fixture 0003's HCM shape minimizes harness churn). 1 filter chain with empty `filter_chain_match`. 1 HCM network filter with `codec_type: HTTP1` and `stat_prefix: ingress_http`. 1 route_config with one `*` vhost holding three routes: `path: /` → cluster `c0` (the routed-to-upstream path); `path: /missing` → `direct_response 404 not found\n` (the HCM-direct-404 path that produces the 404 expected delta); `*` → cluster `c0` (catch-all). 1 STATIC cluster `c0` with 1 endpoint pointing at the controlled backend's port. The controlled backend is launched by the runner per BRAINSTORM §2.5 step 2 — explicit-status returner driven by a per-request header.
  - **`envoy.yaml`** — reference bootstrap. Same listener / HCM / route_config shape. 1 STRICT_DNS cluster `c0` pointing at `host.docker.internal:<backend-port>` with `dns_lookup_family: V4_ONLY` per ADR-0010; same `stat_prefix: ingress_http`. The reference is invoked with `--concurrency 1` per ADR-0028.
  - **`expectations.yaml`** — prose description of the 5-request workload + the 17-stat allow-list table (§7.3 below). Allow-list lines for non-listed stats: any Prometheus metric NAME not in the 17-name allow-list is ignored by the differential. HELP-text values are ignored (per Rule SN6). Connection-count rows use `≥ 1` because both proxies may use HTTP keepalive and collapse multiple requests onto one upstream connection (per BRAINSTORM §2.5).
  - **`README.md`** — explains the fixture's purpose (differential per-stat behavioral-delta equivalence on the 17 names; first observability-surface differential), the STATIC-vs-STRICT_DNS divergence (same as 0001/0002/0003/0004), the 5-request defined-load shape (`[200, 200, 404, 200, 502]`), the per-counter-delta-equality + per-gauge-snapshot-equality assertion shape, the cross-reference to `BEHAVIOR_CONTRACT.md ## Stat-name mapping`'s 17 names + Rules SN1–SN8.
  - **`driver/driver.go`** — `BackendCount() = 1`. `SubjectListenerName() = "l_h1"`. `ReferenceListenerPort() = 15005`. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 5 H1 requests with target statuses `[200, 200, 404, 200, 502]`. The 200 / 502 requests target `path: /` (routed to cluster `c0`); 200 sends no special header; 502 sends `X-Backend-Status: 502` which the controlled backend honors via explicit return. The 404 request targets `path: /missing` (HCM-direct 404, never reaches the cluster). Before the 5 requests: the driver scrapes both `/stats/prometheus` admin endpoints and parses into a 17-name snapshot. After the 5 requests: same scrape + parse → after-snapshot. The driver returns `(beforeSubject, afterSubject, beforeRef, afterRef)` to the runner; the runner asserts per-counter delta-equality and per-gauge snapshot-equality.
  - **`driver/driver_test.go`** — distribution-assertion / scrape-parser unit tests (mirror of fixture 0003's pattern).
  - **`backends/main.go`** — small Go program that starts an HTTP/1.1 echo server on a configurable port; reads a per-request `X-Backend-Status` header; returns the requested status code with a body of `bad gateway\n` (for 502) or `OK\n` (for 200) or whatever the header specifies. Used by the runner to drive the explicit-502 path without coupling to a dial-failure path (per BRAINSTORM §2.5 step 2's "explicit-502-returning shape" rationale — keeps the differential decoupled from dial-error→status-code mapping).
- **`test/differential/runner.go`** (extended) — registration update: blank-import the new fixture-0005 driver package; the runner's per-fixture loop calls the driver's pre-load + post-load hooks per the new "scrape-around-load" pattern. The pattern is fixture-0005-specific; the planner picks at PLAN time whether to surface it as a generic `StatsExpectations` Driver-interface extension or to keep it in-band like the 0004 driver does (per §12 #6 and the fixture-0004 precedent — recommendation: in-band).

### 4.4 Changed documentation and state (in 06.1)

- **`docs/envoy-go/ROADMAP.md`** — row `06.1`: `status: planned → in-progress` flipped at the SPEC commit (per the corrected pattern from phase 05/05.1/05.2's SPEC commits, recorded in `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); transitions to `done` at the 06.1 phase-done commit. Row `06` (parent): stays `in-progress` at the 06.1 phase-done commit (the parent only flips to `done` at 06.2's phase-done — see parent SPEC §5). Row `06.2`: stays `planned` until 06.2's SPEC drafts. The split landed in this commit's ROADMAP edit (per the SPEC-drafting subagent's deliverable list).
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → 06.2 entry at lifecycle-state 1). Updated by the parent session, NOT by the SPEC-drafting subagent.
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (extended in-place per ADR-0052's authorization, mirroring the 05.1 / 05.2 in-place-edit pattern) — the empty `## Stat-name mapping` placeholder at lines 48–53 is filled with the 17-name table from §6 below, the SN1–SN8 rules from §10 below, and the equivalence-matrix new row from §10.2 below. The empty `## Access log field mapping` placeholder at lines 56–61 stays empty in 06.1 (06.2 fills it). The in-place edit lands at the 06.1 phase-done commit, NOT at the SPEC commit.
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — UNCHANGED in 06.1 (no pin bump; D-3.7 reserves pin bumps for dedicated phases).
- **`docs/envoy-go/DECISIONS.md`** — six new ADRs introduced by phase 06.1, numbered ADR-0059..ADR-0064 (next-free per the `DECISIONS.md` tail at master `75a6bf9` being ADR-0058; the planner re-verifies next-free at write time per ADR-0004's autonomous-numbering rule). Topics enumerated in §8 below; the ADRs themselves are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).
- **`docs/envoy-go/phases/06-observability-baseline/SPEC.md`** — UNCHANGED in 06.1 (the parent master SPEC is read-only history once drafted).

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 06.1)

Phase 06.1 adds one new package (`internal/stats/`), extends one existing package (`internal/admin/`), and threads a `*stats.Registry` parameter through three constructor signatures (`cluster.NewManager`, `cluster.NewManagerWithBaseDir`, `admin.New`) plus one constructor that gains the parameter implicitly via the listener-manager's signature change (`listenerManager.New`). The parameter-threading is the surface that BRAINSTORM §3 note 6 flagged as "must be enumerated as part of the SPEC's Files-touched inventory so it isn't surprise scope at PLAN time" — the §4.2 file inventory above honors that note.

```
cmd/envoy-go/main.go                 (MODIFIED: alloc Registry, thread through all constructors,
                                       call Registry.Freeze() after admin starts accepting)
cmd/envoy-go/main_test.go            (MODIFIED: bootstrap variants thread a Registry; no new variant)
internal/bootstrap/bootstrap.go      (MODIFIED: Bootstrap struct gains .Stats *stats.Registry;
                                       Load() allocates and assigns — planner picks final factoring
                                       per §12 #2 above)
internal/listener/manager.go         (MODIFIED: NewManager signature gains *stats.Registry;
                                       on listener.Register: create 2 listener-scope metrics)
internal/listener/listener.go        (MODIFIED: accept loop +2 LoC for cx_total.Inc + cx_active.Inc/Dec)
internal/listener/manager_test.go    (MODIFIED: call sites updated)
internal/cluster/cluster.go          (MODIFIED: Cluster struct gains 8 metric-pointer fields)
internal/cluster/manager.go          (MODIFIED: NewManager + NewManagerWithBaseDir gain *stats.Registry;
                                       on cluster.Register: create 8 cluster-scope metrics)
internal/cluster/dial.go             (MODIFIED: +2 LoC for upstream_cx_total.Inc + upstream_cx_active
                                       Inc/Dec via connWithGauge wrapper)
internal/cluster/dial_h2.go          (MODIFIED: same +2 LoC pattern; reuses connWithGauge)
internal/cluster/cluster_test.go     (MODIFIED: call sites + new tests for register-time metric alloc)
internal/cluster/manager_test.go     (MODIFIED: call sites)
internal/filter/hcm/filter.go        (MODIFIED: at NewFilter alloc 5 HCM-scope metrics; on req entry +1 LoC;
                                       on resp: +3-5 LoC for status-class dispatch)
internal/filter/hcm/actions.go       (MODIFIED: routerAction.do +1 + +3-5 LoC; routerActionH2.do same)
internal/filter/hcm/h2/router_action.go  (MODIFIED: M-9 carry-forward log line on doH2 error path)
internal/filter/hcm/h2/router_action_test.go  (NEW or appended: M-9 unit test)

internal/admin/admin.go              (MODIFIED: New signature gains *stats.Registry;
                                       registers /stats/prometheus alongside /ready)
internal/admin/admin_test.go         (MODIFIED: call sites + /stats/prometheus tests)
internal/admin/prometheus.go         (NEW: handlePrometheus(registry) http.HandlerFunc)
internal/admin/prometheus_test.go    (NEW)
internal/admin/doc.go                (MODIFIED: mention /stats/prometheus)

internal/stats/                      (NEW package)
   doc.go                            (REWRITE: API + LBP-1 invariant)
   registry.go                       (NEW)
   counter.go                        (NEW)
   gauge.go                          (NEW)
   name.go                           (NEW: flatten + escape + helpText map)
   prom.go                           (NEW: WriteProm)
   registry_test.go                  (NEW)
   counter_test.go                   (NEW)
   gauge_test.go                     (NEW)
   name_test.go                      (NEW)
   prom_test.go                      (NEW)
   fuzz_test.go                      (NEW: FuzzPromTextFormat)

test/differential/runner.go          (MODIFIED: blank-import for fixture 0005;
                                       the runner's per-fixture loop calls the driver's
                                       pre-load + post-load scrape hooks per the in-band
                                       pattern — no new generic interface extension)

test/differential/0005-prometheus-stats/  (NEW fixture directory)
   envoy.yaml, envoy-go.yaml, expectations.yaml, README.md
   driver/driver.go, driver/driver_test.go
   backends/main.go

internal/accesslog/                  (UNCHANGED — placeholder doc.go remains; comes alive in 06.2)
internal/filter/tcpproxy/            (UNCHANGED)
internal/tls/                        (UNCHANGED)
internal/filter/hcm/h2/              (UNCHANGED on the codec primitives — only router_action.go gets the M-9 fix)
internal/filter/hcm/connection.go    (UNCHANGED)
internal/filter/hcm/config.go        (UNCHANGED — codec_type/HCM-side fields are 05.1's surface)

test/conformance/h2spec/             (UNCHANGED — pin and threshold list stay at 05.1 baseline)
test/fixtures/0000-tcp-echo/         (UNCHANGED)
test/fixtures/0001-tcp-proxy-rr/     (UNCHANGED)
test/fixtures/0002-tls-tcp/          (UNCHANGED)
test/fixtures/0003-http11-routing/   (UNCHANGED)
test/fixtures/0004-h2-routing/       (UNCHANGED)

docs/envoy-go/BEHAVIOR_CONTRACT.md   (MODIFIED at phase-done commit, NOT SPEC commit:
                                       ## Stat-name mapping populated; new equivalence-matrix row)
docs/envoy-go/CONFORMANCE_PINS.md    (UNCHANGED)
docs/envoy-go/DECISIONS.md           (APPENDED at impl-time per ADR-by-ADR commits:
                                       ADR-0059..ADR-0064 — six ADRs; planner verifies next-free
                                       at write time)
docs/envoy-go/ROADMAP.md             (MODIFIED at SPEC commit: row 06.1 planned → in-progress;
                                       row 06.2 added; row 06 parent gains sub-phases column.
                                       At phase-done: row 06.1 → done; row 06 stays in-progress)
docs/envoy-go/STATE.md               (MODIFIED at each lifecycle transition by parent session)
docs/envoy-go/phases/06-observability-baseline/SPEC.md   (UNCHANGED — parent master SPEC, read-only)
docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
docs/envoy-go/phases/06.2-access-log/README.md   (UNCHANGED — sibling SPEC stub)
```

### 5.2 Stats Registry concurrency model (per BRAINSTORM §5.1)

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `Registry.NewCounter`, `NewGauge` | Once per metric, at process start (~25 calls total: 2 listener + 5 HCM + 8 cluster + 1 server.live + ~9 from M-9 / future-scope) | `Registry.mu` Lock |
| Boot-end | `Registry.Freeze()` | Once, after admin server is accepting | atomic CAS on `frozen` bool |
| Hot path | `Counter.Inc()`, `Gauge.{Inc,Dec,Set}` | Per request / connection / accept | **Lock-free** — `atomic.{AddUint64, AddInt64, StoreInt64, LoadInt64}` only |
| Scrape | `Registry.Walk(fn)` | Per `/stats/prometheus` request (rare; Prom default scrape interval is 15s) | `Registry.mu` RLock |
| Post-freeze register attempt | `Registry.NewCounter`, `NewGauge` | Forbidden by LBP-1 | Panics on `frozen == true` (see §11.6 below) |

The Registry's *list of metrics* is mutable only during boot. Once `Freeze()` is called, the list is fixed. After Freeze, scrapes RLock the list (cheap) and read each metric's atomic value with no contention against hot-path increments. Hot-path Inc/Dec/Set never touch `Registry.mu`. Two concurrent scrapes both RLock; both walk; both write their own response — no interaction. Snapshot consistency is per-walk: a given walk reads each metric atomically once; different metrics within one walk may have been updated between reads. Matches Envoy.

### 5.3 The LBP-1 invariant (per BRAINSTORM §5.1, formalized for SPEC binding)

**LBP-1 ("list before play"):** All `*stats.Registry` `NewCounter` / `NewGauge` calls MUST complete before the listener manager begins accepting connections. The boot-time order is:

```
bootstrap.Load(configPath)                     // alloc *stats.Registry
   ↓
cluster.NewManager(clusters, registry)         // .Register() per cluster, allocs 8 metrics × N
   ↓
listenerManager.New(listeners, registry, ...)  // .Register() per listener, allocs 2 metrics × N
                                               // HCM factory captures registry for per-HCM-instance
                                               // metric alloc on first HCM construct
   ↓
admin.New(ready, registry)                     // alloc 1 metric (server.live); register routes
   ↓
admin.Listen()                                 // admin starts accepting
   ↓
registry.Freeze()                              // all NewCounter/NewGauge after this point panic
   ↓
listenerManager.Run()                          // the listener manager begins accepting connections
```

After `Freeze()`, any `NewCounter` / `NewGauge` call panics with `stats: registry frozen: cannot register %q post-boot` (the diagnostic is grep-verifiable in `registry.go`). This invariant is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments. A unit test in `registry_test.go` asserts the panic on a post-freeze `NewCounter` call (per §11.1 below).

If a future xDS-CDS phase introduces dynamic cluster registration, `NewCounter` would race against `Walk` — explicitly out of scope for 06.1, governed by this invariant. The xDS-CDS-introduces-dynamic-stats problem is solved at that phase's own brainstorm time, likely with a copy-on-write list shape; LBP-1 is the simpler shape that 06.1's surface needs.

### 5.4 Boot wiring sequence (per BRAINSTORM §4.1)

```
cmd/envoy-go/main.go
   ↓
bootstrap.Load(configPath) → returns *Bootstrap (now has .Stats *stats.Registry)
   ↓
   ├─→ admin.New(ready, stats)               -- admin server gets a Registry handle
   │      ↓
   │      registers /ready + /stats/prometheus handlers
   │      allocs the server.live gauge
   │
   ├─→ cluster.NewManager(clusters, stats)   -- manager threads stats into each Cluster on register
   │      ↓ on cluster.Register():
   │           NewCounter("cluster.<n>.upstream_rq_total")
   │           NewCounter("cluster.<n>.upstream_rq_2xx".."5xx")
   │           NewCounter("cluster.<n>.upstream_cx_total")
   │           NewGauge("cluster.<n>.upstream_cx_active")
   │           NewGauge("cluster.<n>.membership_total")  -- Set() to len(endpoints) at register
   │
   └─→ listenerManager.New(listeners, stats, hcmFactory)
          ↓ on listener.Register():
               NewCounter("listener.<addr>.downstream_cx_total")
               NewGauge("listener.<addr>.downstream_cx_active")
          ↓ HCM factory captures stats; per-HCM-instance allocates:
               NewCounter("http.<stat_prefix>.downstream_rq_total")
               NewCounter("http.<stat_prefix>.downstream_rq_2xx".."5xx")
   ↓
admin.Listen()       -- admin starts accepting on its bind port
   ↓
registry.Freeze()
   ↓
listenerManager.Run()  -- listener manager begins accepting connections
```

The HCM factory closure's capture of `*stats.Registry` is the load-bearing wiring that lets per-HCM-instance metric alloc happen at filter-build time (not boot time strictly). Filter-build time is still pre-Freeze because the listener manager's `New(...)` finishes synchronously before `admin.Listen()` returns; per-HCM filter chains are constructed inside `listenerManager.New`'s loop. The planner verifies this ordering at PLAN write time (per §12 #4).

### 5.5 Increment paths (per-request hot path, per BRAINSTORM §4.2)

| File | Hot-path edits |
|---|---|
| `internal/listener/listener.go` (Accept loop) | `+2 LoC` — `cx_total.Inc()` + `cx_active.Inc()` on accept; `defer cx_active.Dec()` on conn close |
| `internal/filter/hcm/filter.go` (HCM dispatch entry) | `+1 LoC` — `downstream_rq_total.Inc()` on first byte of request line/headers |
| `internal/filter/hcm/filter.go` (HCM response hook) | `+3-5 LoC` — switch on response status class → `downstream_rq_<Nxx>.Inc()` once per response. Lives where the response status code is finalized, before bytes hit the wire. |
| `internal/cluster/dial.go` | `+2 LoC` + ~10 LoC `connWithGauge` wrapper — `cx_total.Inc()` + `cx_active.Inc()` on successful dial; `cx_active.Dec()` on conn close |
| `internal/cluster/dial_h2.go` | `+2 LoC` — same wrapping pattern, reuses `connWithGauge` |
| `internal/filter/hcm/actions.go` (`routerAction.do`, H1) | `+1 LoC` — `upstream_rq_total.Inc()` at request dispatch; `+3-5 LoC` for `upstream_rq_<Nxx>.Inc()` on response status |
| `internal/filter/hcm/actions.go` (`routerActionH2.do`, H2) | `+1 LoC` + `+3-5 LoC` — same pattern |

**Total request-path edits: ~15 LoC across ~5 files.** The `internal/stats` package itself is the bulk of new code.

### 5.6 Read path (Prometheus scrape)

```
GET /stats/prometheus
   ↓
admin.handlePrometheus(w, r):
   1. registry.Walk(func(metric stats.Metric) { ... })
   2. For each metric:
      a. flatten internal name → prom name + label set (name.go)
      b. group by prom name (status-class collapse joins the four `_Nxx` → one base-name group)
      c. emit "# HELP <prom_name> <text>"   (HELP text from helpText map)
      d. emit "# TYPE <prom_name> counter|gauge"
      e. emit "<prom_name>{label=\"value\",...} <value>\n"  (escape per Prom spec)
   3. Output is sorted alphabetically by Prom name (matches Envoy).
```

The handler is read-only against the Registry. Reads use `atomic.LoadUint64` / `LoadInt64` — no lock contention with hot-path increments. Walk over the Registry uses an `RWMutex` only for the metric *list* (mutated only at register time, never per-request). Concurrent scrapes both RLock; both walk; both write their own response — no interaction.

Snapshot consistency is per-walk: a given walk reads each metric atomically once; different metrics within one walk may have been updated between reads. Matches Envoy.

### 5.7 The `server.live` gauge (per BRAINSTORM §4.4)

Set to 1 inside `admin.handleReady` the first time it returns 200. Never reset by 06.1. (No graceful-drain in 06.1; phase 08 owns that.) Allocated at `admin.New` time. The set-once discipline uses a `sync.Once` inside `handleReady` to ensure the gauge transitions exactly once — a defensive measure since the gauge would otherwise be Set to 1 on every `/ready` 200 response (Set(1) is idempotent, so the defense is cosmetic, but the `sync.Once` prevents a future refactor from accidentally Set-ing to 0 between transitions).

### 5.8 HELP text discipline (per BRAINSTORM §4.5)

Static map in `internal/stats/name.go` keyed by Prometheus name → human-English description. Authored once for the 13 unique Prometheus names emitted by 06.1. **Not byte-equal to Envoy's HELP text** (Rule SN6 below). Envoy's HELP is not semantically meaningful per the Prometheus exposition spec, and the differential fixture (Q5 = B per BRAINSTORM §2.5) checks values + label keys + types only — not HELP strings.

Example entries:

```
"envoy_listener_downstream_cx_total":   "Total connections accepted on the listener.",
"envoy_listener_downstream_cx_active":  "Active connections on the listener.",
"envoy_http_downstream_rq_total":       "Total requests received by the HTTP connection manager.",
"envoy_http_downstream_rq":             "Requests received by the HTTP connection manager, by response code class.",
"envoy_cluster_upstream_rq_total":      "Total requests dispatched to upstream clusters.",
"envoy_cluster_upstream_rq":            "Requests dispatched to upstream clusters, by response code class.",
"envoy_cluster_upstream_cx_total":      "Total connections established to upstream clusters.",
"envoy_cluster_upstream_cx_active":     "Active connections to upstream clusters.",
"envoy_cluster_membership_total":       "Number of endpoints in the cluster.",
"envoy_server_live":                    "1 if the server is live, 0 otherwise.",
```

(Note: the entries marked with the "by response code class" suffix are the Rule SN4 collapsed forms; the exact base-name shape is pinned at the Rule SN4 empirical-verification gate per §10 below. The HELP text examples above use a hypothetical `_xx`-removed form; the parent session updates the examples after Rule SN4 is empirically verified.)

## 6. Stats catalog — the 17 internal names (per BRAINSTORM §2.3, transcribed verbatim)

`<stat_prefix>` is read from HCM config (already plumbed from phase 04). `<addr>` is the listener bind address normalized like Envoy does (e.g., `0.0.0.0:10000` → `0.0.0.0_10000`). `<n>` is the cluster name as configured in the bootstrap.

**Listener — 2 names:**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `listener.<addr>.downstream_cx_total` | counter | `envoy_listener_downstream_cx_total{envoy_listener_address="<addr>"}` |
| `listener.<addr>.downstream_cx_active` | gauge | `envoy_listener_downstream_cx_active{envoy_listener_address="<addr>"}` |

**HCM — 5 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `http.<stat_prefix>.downstream_rq_total` | counter | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_2xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_3xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_4xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_5xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="<stat_prefix>"}` |

**Cluster — 8 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `cluster.<n>.upstream_rq_total` | counter | `envoy_cluster_upstream_rq_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_2xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_3xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="3",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_4xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_5xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_total` | counter | `envoy_cluster_upstream_cx_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_active` | gauge | `envoy_cluster_upstream_cx_active{envoy_cluster_name="<n>"}` |
| `cluster.<n>.membership_total` | gauge | `envoy_cluster_membership_total{envoy_cluster_name="<n>"}` (Set once at register, equals N endpoints) |

> **Twin-series filter discipline (per empirical-verification scrape):** Envoy v1.37.2 ALSO emits two twin metric families that envoy-go does NOT emit and the differential fixture (§7) MUST filter out before per-counter delta comparison: (a) `envoy_cluster_external_upstream_rq_xx` (the "external" upstream-rq twin Envoy uses to split internal vs external traffic via `internal_traffic` config); (b) `envoy_listener_http_downstream_rq_xx` (a listener-scoped HCM-rq twin keyed by both listener address and HCM stat_prefix); plus the per-exact-status family `envoy_cluster_upstream_rq{envoy_response_code="200"}` (a separate metric family with `envoy_response_code` label, distinct from `envoy_cluster_upstream_rq_xx`'s `envoy_response_code_class` label). The fixture's allow-list per §7 enumerates exactly the 13 unique Prometheus names this SPEC ships; everything else in the Envoy scrape is ignored.

**Server — 2 names (one EMITTED, one explicitly NOT-EMITTED):**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `server.live` | gauge | `envoy_server_live` (Set to 1 once admin `/ready` returns 200; never reset by 06.1) |
| `server.uptime` | — | **NOT EMITTED** — depends on monotonic-clock + per-scrape recompute; deferred with histograms (see §2.1 above) |

**Total: 17 internal names.** The four `downstream_rq_Nxx` and four `upstream_rq_Nxx` Prometheus exposition forms collapse to two base-name groups (one HCM, one cluster) per the Rule SN4 status-class flattening discipline (§10).

## 7. Differential fixture `0005-prometheus-stats` (per BRAINSTORM §2.5)

### 7.1 Equivalence claim shape

Per BRAINSTORM §2.5, the differential equivalence claim is **behavioral-delta** (not byte-exact whole-output): drive a defined load, snapshot the 17 stats before/after on both proxies, assert per-stat delta-equality between envoy-go and Envoy. Gauges are snapshot-equal after drain.

Rationale (per BRAINSTORM §2.5):
- Pure byte-exact full-output is fragile — a minor-version Envoy bump that adds a new stat would break the fixture without indicating any envoy-go regression.
- Pure delta-only is somewhat permissive (typo in stat name would still pass with delta=0=0), but the schema is constrained by the fixture's stat-name allow-list (the 17 names) and the unit-test layer catches name validity at register time.
- Layered (delta + byte-exact-schema) was considered but adds ~2× fixture LoC for marginal protection over what unit tests already provide.

### 7.2 Driver outline (per BRAINSTORM §2.5)

1. Boot envoy-go on port P1 + reference Envoy on port P2 with identical bootstraps (one HTTP/1.1 listener, one cluster `c0` with 1 endpoint, `stat_prefix: ingress_http`).
2. Boot a controlled backend on port P3 that returns whatever status the driver requests via the `X-Backend-Status` request header (default 200, configurable per request via header). The backend is an explicit-502-returning shape for the 502 test point — NOT a dial-failure path (avoids dependency on dial-error→status mapping which is a separate concern).
3. Scrape both `/stats/prometheus` → parse → snapshot 17 stats as `before`.
4. Send 5 sequential requests with target statuses `[200, 200, 404, 200, 502]` (404 = no-route from envoy-go's HCM via `path: /missing`; 502 = backend explicit return via `X-Backend-Status: 502`).
5. Scrape again → `after`.
6. Assert per-counter `delta_envoy_go == delta_envoy`; per-gauge `after_envoy_go == after_envoy`.

### 7.3 Expected deltas (`expectations.yaml` shape, transcribed verbatim from BRAINSTORM §2.5)

```
listener.<addr>.downstream_cx_total: ≥ 1   (keepalive may collapse 5 reqs into fewer cx)
listener.<addr>.downstream_cx_active: 0    (gauge: 0 after drain)
http.ingress_http.downstream_rq_total: 5
http.ingress_http.downstream_rq_2xx: 3
http.ingress_http.downstream_rq_3xx: 0
http.ingress_http.downstream_rq_4xx: 1
http.ingress_http.downstream_rq_5xx: 1
cluster.c0.upstream_rq_total: 4            (404 doesn't reach cluster — handled by HCM directly)
cluster.c0.upstream_rq_2xx: 3
cluster.c0.upstream_rq_3xx: 0
cluster.c0.upstream_rq_4xx: 0
cluster.c0.upstream_rq_5xx: 1
cluster.c0.upstream_cx_total: ≥ 1
cluster.c0.upstream_cx_active: 0           (gauge)
cluster.c0.membership_total: 1             (gauge)
server.live: 1                             (gauge)
```

Connection-count rows use `≥ 1` because both proxies may use HTTP keepalive and collapse multiple requests onto one upstream connection; the differential equality holds regardless of magnitude (subject and reference both apply the same `≥ 1` rule).

### 7.4 Allow-list discipline

Any Prometheus metric name in either side's `/stats/prometheus` exposition output that is NOT in the 17-name table above is **ignored** by the differential. This is the equivalence-matrix new row's "Allow-list / tolerance" column (per §10.2 below). Reference Envoy v1.37.2 emits ~150 metric names by default; the 17-name allow-list is a strict subset. HELP-text values are also ignored (per Rule SN6).

The driver's scrape-parser implements the allow-list in-band: parse the full exposition, extract the entries matching the 17 names, drop the rest. The runner asserts only on the 17.

## 8. ADRs anticipated

Per BRAINSTORM §8, six ADRs are anticipated for 06.1, numbered ADR-0059..ADR-0064 (next-free per the `DECISIONS.md` tail at master `75a6bf9` being ADR-0058). The planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule. The ADRs are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).

The numbering below is the expected mapping based on topical ordering; the planner may reorder commit-time landings if that reads more naturally in PLAN.md, in which case the actual ADR number assignments may permute (the four ADR-0055..ADR-0058 block in 05.2 used a non-monotonic commit-time ordering — this is permitted and recorded in the ADR's `Lands-in-task` field).

- **ADR-0059 — Internal Stats Store architecture.** Status: Accepted. Doctrine: D-3.2 + D-3.3. Decision: a thin in-tree atomic-counter Registry (`internal/stats`) backed by a Prometheus text-format adapter; **no third-party Prometheus library** (no `prometheus/client_golang` dependency). Lock-free hot path via `atomic.{AddUint64, AddInt64, StoreInt64, LoadInt64}`; Walk-under-RLock for scrape; `RWMutex.Lock` held only at boot-time register sites. Rationale (per BRAINSTORM §2.1): future Observability-family phases (gRPC ALS, OTLP, statsd) all need to hook a registry, not a Prometheus client; investing in our own thin shape now is the same architectural choice Envoy made in `source/common/stats/`. Alternatives considered: (A) `prometheus/client_golang` directly — rejected for future-sink coupling; (C) `expvar` + custom serializer — rejected because expvar lacks histogram support. Lands-in-task: 06.1 Task 1 (the Registry itself).

- **ADR-0060 — Histograms deferred from 06.1.** Status: Accepted. Doctrine: D-3.6 + D-3.4. Decision: 06.1 emits counters + gauges only; histograms (`upstream_rq_time`, `downstream_rq_time`, response/request size distributions) are deferred to a later sub-phase with their own brainstorm covering circllhist→Prometheus bucket mapping. Rationale (per BRAINSTORM §2.2): Envoy uses circllhist (dynamic-bucket) internally and bridges to Prometheus's fixed-bucket shape via `histogram_bucket_settings`; byte-equivalent matching against the v1.37.2 reference is hard and wants its own design pass. Bundling histograms into 06.1 would bloat the phase and leave a half-baked histogram model in tree. Carry-forward: 06.x or upstream-robustness family with own brainstorm. Lands-in-task: 06.1 Task 1 (alongside the Registry; the ADR documents the deferral at Registry-introduction time).

- **ADR-0061 — Stat-name → Prometheus-name flattening rules SN1–SN8.** Status: Accepted. Doctrine: D-3.4. Decision: the eight rules enumerated in §10 below govern the flattening from internal hierarchical-dotted names to Prometheus-format `name{label="value"}` lines. Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 are settled at brainstorm-close (BRAINSTORM §7.1); Rule SN4 is empirically pinned at SPEC-drafting time per BRAINSTORM §2.3.1 against reference Envoy v1.37.2's default tag-extractor regex. The verbatim scrape-output evidence and the regex-source SHA from `source/common/config/well_known_names.cc` at v1.37.2 are pasted into ADR-0061's Context section. Lands-in-task: 06.1 Task wherever the flattening logic in `name.go` first lands (the planner picks).

- **ADR-0062 — Differential equivalence shape for stats.** Status: Accepted. Doctrine: D-3.3 + D-3.6. Decision: per-stat behavioral-delta equivalence (not byte-exact whole-output); per-counter `delta_envoy_go == delta_envoy`, per-gauge `after_envoy_go == after_envoy`; HELP text ignored; non-listed Prometheus names ignored (per Rule SN6 + §7.4 above). Rationale (per BRAINSTORM §2.5): byte-exact full-output is fragile under minor-version Envoy bumps; pure delta-only is permissive but constrained by the 17-name schema + unit-test name-validity coverage; layered byte-exact-schema is marginal protection. Lands-in-task: 06.1 Task wherever the differential runner hooks the scrape-and-diff pattern (the planner picks).

- **ADR-0063 — Per-endpoint cluster stats not emitted.** Status: Accepted. Doctrine: D-3.4. Decision: 06.1 emits cluster-level metrics only (the 8 names in §6 above); per-endpoint expansion (`cluster.<n>.<endpoint_address>.cx_total`, `cluster.<n>.<endpoint_address>.rq_total`, etc. — equivalent of Envoy's `enable_per_endpoint_stats=true` mode) is deferred. Rationale (per BRAINSTORM §2.3, §9): per-endpoint expansion is dynamic in shape (endpoint set churns under xDS-EDS); statically-allocated per-endpoint metrics break LBP-1; properly handling the dynamic-shape case wants xDS-EDS semantics. Carry-forward: xDS-EDS phase revisits. Lands-in-task: 06.1 Task wherever the cluster-side metric-allocation loop in `internal/cluster/manager.go` first lands.

- **ADR-0064 — `stats_config.stats_tags` config not honored; extraction hardcoded.** Status: Accepted. Doctrine: D-3.4. Decision: 06.1 hardcodes the stat-name → Prometheus-name extraction logic in `internal/stats/name.go` (per Rules SN1–SN5); the bootstrap proto's `stats_config.stats_tags[]` field is silently ignored if present. Rationale (per BRAINSTORM §2.3, §5.6): the regex-driven tag-extraction surface in Envoy is complex and warrants its own phase; 06.1 ships fixed extraction that matches Envoy's default tag-extractor behavior on the 17 names. Carry-forward: future stats-config phase or an xDS-RTDS revisit. Lands-in-task: 06.1 Task wherever the flattening logic in `name.go` first lands (alongside ADR-0061).

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 06.1 silently ignores the following at parse time (no error, no honored behavior):

- Bootstrap proto's `stats_config.stats_tags[]` field — entire array silently ignored per ADR-0064.
- Bootstrap proto's `stats_sinks[]` field — every entry silently ignored. Phase 06.1 ships only the in-process Prometheus exporter; gRPC ALS / statsd / OTel / etc. are Observability-family deliverables.
- Bootstrap proto's `stats_config.stats_matcher` / `stats_config.histogram_bucket_settings` / `stats_config.use_all_default_tags` fields — all silently ignored (the histogram-related ones because histograms are deferred per ADR-0060).
- HCM `stats_flush_interval` — silently ignored.
- Cluster `track_cluster_stats` field — silently ignored (the eight cluster-scope metrics are emitted unconditionally for every registered cluster).
- Listener `stat_prefix` field — silently ignored (the listener-scope metrics use the bind-address-derived `<addr>` form per §6 above; Envoy supports `stat_prefix` for listener-side stat-name override but 06.1 does not).

The full silently-ignored set is the union of phases 04 / 05.1 / 05.2's silently-ignored sets plus 06.1's amendment above. The phase-04 / 05.1 / 05.2 ignored sets are NOT amended by this list — only extended. ADR-N (the original silent-ignore ADR) is amended (not superseded) to record the 06.1 additions; the amendment shape mirrors the 05.1 + 05.2 amendments (a single appended sub-section under ADR-N's Consequences, listing the newly-ignored fields).

## 10. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 10.1 `## Stat-name mapping` subsection (full population)

The `## Stat-name mapping` placeholder at `BEHAVIOR_CONTRACT.md` lines 48–53 is currently empty. Phase 06.1 fills it with the 17-name table from §6 above PLUS the eight flattening rules below. **Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 are settled at brainstorm-close and transcribed verbatim from BRAINSTORM §7.1. Rule SN4 was empirically pinned at SPEC-drafting time** per BRAINSTORM §2.3.1; the verbatim scrape evidence and the regex source citation are inline in Rule SN4 below.

```
Rule SN1: Name segments matching `cluster.<n>.<rest>` extract `<n>` as label
          `envoy_cluster_name` and prefix `<rest>` with `envoy_cluster_`.

Rule SN2: Name segments matching `http.<stat_prefix>.<rest>` extract <stat_prefix>
          as label `envoy_http_conn_manager_prefix` and prefix <rest> with `envoy_http_`.

Rule SN3: Name segments matching `listener.<addr>.<rest>` extract <addr> as label
          `envoy_listener_address` and prefix <rest> with `envoy_listener_`.

Rule SN4: Names ending `_Nxx` where N ∈ {1..5} flatten to a base name with the
          trailing class digit STRIPPED (so the metric name ends in literal `_xx`),
          plus a label `envoy_response_code_class` whose value is the single class
          digit as a string (`"1"`, `"2"`, `"3"`, `"4"`, `"5"`). Empirically verified
          against reference Envoy v1.37.2 at the `ENVOY_TARGET.md`-pinned image
          (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) on 2026-04-27; see
          empirical evidence block below.

          Examples (canonical):
              cluster.foo.upstream_rq_2xx
                → envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="foo"}
              http.ingress_http.downstream_rq_5xx
                → envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"}

          Counter-examples (NOT what Envoy emits):
              ✗ envoy_cluster_upstream_rq_2xx{...}            -- digit suffix preserved (wrong)
              ✗ ...{envoy_response_code_class="2xx",...}       -- label value with literal "xx" (wrong)
              ✗ envoy_cluster_upstream_rq{envoy_response_code_class="2",...}  -- _xx stripped entirely (wrong)

          Empirical evidence (verbatim excerpt from reference-Envoy /stats/prometheus
          scrape under a 5-request load with statuses [200,200,404,200,500] through
          HCM stat_prefix=ingress_http to cluster c_backend):

              # TYPE envoy_cluster_upstream_rq_xx counter
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c_backend"} 3
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="c_backend"} 1
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="c_backend"} 1
              # TYPE envoy_http_downstream_rq_xx counter
              envoy_http_downstream_rq_xx{envoy_response_code_class="1",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_http"} 3
              envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_http"} 1
              envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"} 1

          Negative-confirmation grep (entire 1181-line scrape, no matches):
              grep -E 'envoy_[a-z_]*_(1xx|2xx|3xx|4xx|5xx)' /stats/prometheus  # -> 0 matches

          Tag-extractor regex source: Envoy v1.37.2
          source/common/config/well_known_names.cc, the `RESPONSE_CODE_CLASS`
          tag entry. Source-tree commit pin = the v1.37.2 release tag, server-side
          version-string SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches
          ENVOY_TARGET.md). The regex captures the inner `\dxx` token from the
          stat suffix `_<class>xx`, removes the entire `_<class>xx` from the stat
          name (yielding the base name ending `_xx` after the standard rename), and
          emits the captured digit as the `response_code_class` tag value.

Rule SN5: Server-scope names (`server.<rest>`) flatten to `envoy_server_<rest>`
          with no extracted labels.

Rule SN6: HELP text is best-effort English, NOT byte-equal to Envoy's HELP. The
          differential equivalence claim is on values + label keys + types only.

Rule SN7: Histograms are not emitted by 06.1. (Forward-looking.)

Rule SN8: Per-endpoint cluster stats are not emitted by 06.1. (Forward-looking.)
```

The 17-name table from §6 above is also transcribed verbatim into the `## Stat-name mapping` subsection below the rules.

### 10.2 New equivalence-matrix row (transcribed from BRAINSTORM §7.3)

The `## Equivalence Matrix` subsection at `BEHAVIOR_CONTRACT.md` lines 9–23 gains a new row:

```
| Dimension       | Equivalence claim                                        | Allow-list / tolerance               |
|-----------------|----------------------------------------------------------|--------------------------------------|
| Stats output    | Per-stat behavioral delta after defined load is equal    | 17 stats listed in § Stat-name       |
|                 | between envoy-go and reference Envoy. Gauges are         | mapping. All other Envoy stat names  |
|                 | snapshot-equal after drain. Names + label keys + types   | in /stats/prometheus output are      |
|                 | byte-equal; HELP text ignored.                           | ignored by the differential.         |
```

This row supersedes the existing seed-row at line 19 (`| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |`); the seed-row is the matrix's pre-06.1 forward-looking placeholder, and 06.1's row is its concrete settlement on the 17-name surface.

## 11. Testing strategy (per BRAINSTORM §6)

### 11.1 Unit tests (`internal/stats/`)

- **`registry_test.go`** — register / lookup / walk happy paths; duplicate-name panic at register; invalid-name panic at register (regex `^[a-zA-Z_][a-zA-Z0-9_.]*$` per §4.1); concurrent walks (two goroutines RLock-iterate, no interaction); race-clean concurrent `Inc` + `Walk` (under `-race`); `Freeze()` once-only behavior; **post-Freeze `NewCounter` / `NewGauge` panic** (LBP-1 enforcement test, per §5.3).
- **`counter_test.go`** — `Inc` semantics (1 → 2 → 3 sequential); `Add(delta)` sequential; race-clean concurrent `Inc` from N goroutines (under `-race`, N=8, 10000 inc each, asserts final value == 8 × 10000); overflow not testable at 1M req/s for ~584,000 years per BRAINSTORM §5.2 — just documented.
- **`gauge_test.go`** — `Inc` / `Dec` / `Set` happy paths; negative gauge after unmatched `Dec` (defensive — gauge reflects reality per BRAINSTORM §5.2); race-clean concurrent `Inc` / `Dec` / `Set`.
- **`name_test.go`** — flatten happy paths for all 13 unique Prometheus names + the four-fold status-class collapse (4 internal `_Nxx` names → 1 Prometheus name); label-value escaping with adversarial inputs (newlines, backslashes, double-quotes); invalid-name validator rejects `_internal name`, `name with space`, etc.
- **`prom_test.go`** — `WriteProm` happy path against a small Registry; alphabetically-sorted output; empty Registry → handler still writes the (empty) response cleanly; Counter + Gauge values render correctly (including negative gauge); HELP / TYPE lines present and correctly placed; group separator (blank line between Prometheus-name groups); status-class collapse renders four metric-lines under one HELP/TYPE pair.

### 11.2 Unit tests (`internal/admin/`)

- **`prometheus_test.go`** — HTTP handler returns 200 + correct content-type (`text/plain; version=0.0.4; charset=utf-8`); output is alphabetically sorted by Prometheus name; empty Registry → handler still writes the (empty) response cleanly; Counter + Gauge values render correctly; HELP / TYPE lines present.

### 11.3 Unit tests (existing-package extensions)

`internal/cluster/`, `internal/listener/`, `internal/filter/hcm/`:
- New unit tests asserting hot-path increments fire on the right edges. Use real `stats.Registry` (not mocks) — fast, deterministic, exercises the integration end-to-end.
- `internal/cluster/cluster_test.go` extended: at `cluster.Register()` time, the 8 cluster-scope metrics are allocated; runtime accessors return the right pointers.
- `internal/cluster/dial_test.go` / `dial_h2_test.go` extended: on successful dial, `upstream_cx_total` increments by 1 and `upstream_cx_active` increments by 1; on conn close, `upstream_cx_active` decrements by 1.
- `internal/listener/listener_test.go` extended: on accept, `downstream_cx_total` increments by 1 and `downstream_cx_active` increments by 1; on conn close, `downstream_cx_active` decrements by 1.
- `internal/filter/hcm/filter_test.go` extended: on request-line entry, `downstream_rq_total` increments; on response status finalization, `downstream_rq_<Nxx>` increments per the integer-divide `code / 100` discipline.
- `internal/filter/hcm/actions_test.go` extended: same shape on `routerAction.do` (H1) and `routerActionH2.do` (H2).

### 11.4 M-9 carry-forward unit test (per BRAINSTORM §6.1)

`internal/filter/hcm/h2/router_action_test.go` (new or appended to existing test file): `h2RouterActionAdapter.WriteH2` invokes `doH2` which returns an error → log line is captured (test logger sink) → assert error was logged before the function returned. ~20 LoC.

### 11.5 Differential fixture `0005-prometheus-stats` (per §7 above)

The 5-request workload + per-counter delta-equality + per-gauge snapshot-equality assertion shape per BRAINSTORM §2.5.

### 11.6 LBP-1 enforcement test

`internal/stats/registry_test.go`: post-Freeze `NewCounter("foo")` panics with the diagnostic `stats: registry frozen: cannot register %q post-boot`; post-Freeze `NewGauge("bar")` panics with the same; pre-Freeze the same calls succeed.

### 11.7 h2spec re-run (gate (c))

Per BRAINSTORM §6.3, phase 06.1 doesn't touch H2 wire code. h2spec gates remain at 53/53 PASS — already-pinned at the ADR-0051 SHA. Gate (c) re-runs unchanged.

### 11.8 Fuzzers (gate (d))

Existing six fuzzers re-run at the 30s ADR-0018 budget:
- `internal/bootstrap.FuzzBootstrapLoad`
- `internal/filter/tcpproxy.FuzzTcpProxyFilter`
- `internal/tls.FuzzTLSContextParse`
- `internal/filter/hcm.FuzzHCMConfigParse`
- `internal/filter/hcm/h2.FuzzFrameStream`
- `internal/filter/hcm/h2.FuzzHPACKDecode`

**NEW: `internal/stats.FuzzPromTextFormat`** — fuzzes adversarial stat names + label values into `stats.WriteProm`. Cheap (~50 LoC); escaping bugs are the most likely class of bug in the writer, and Prometheus's parser is strict. The fuzzer's seed corpus includes adversarial entries: newlines in label values, backslashes, double-quotes, NUL bytes, trailing-equals signs, very long names, names violating the regex (the fuzzer asserts `WriteProm` does NOT panic; the panic-on-invalid-name discipline lives at `Registry.NewCounter` time, not at write time, since the Registry guards entry).

Total fuzzer count post-06.1: **7**.

### 11.9 Race detector + lint (gate (e))

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean. Race-detector specifically exercises (per BRAINSTORM §5.5):
- Concurrent `Counter.Inc` from N goroutines.
- Concurrent `Gauge.{Inc, Dec, Set}` from N goroutines.
- `Walk` running while `Inc/Dec/Set` are running (the post-boot read-only-list assumption).
- Two concurrent `Walk`s.

## 12. Deferred decisions (the planner / implementer settles these)

Items the SPEC names but does not finalize; the planner closes them in PLAN.md or the implementer closes them at task time per the SPEC's recommendation.

1. **HCM `downstream_rq_total` Inc hook location.** Two viable sites: (a) on first byte of request line/headers in `connection.go`'s read loop; (b) when the route table is consulted in `filter.go`. Both are pre-response-finalization, both are once-per-request, but (a) counts requests that fail before route-match while (b) counts only routed requests. Envoy counts at (a)-equivalent. **Recommendation: (a).** Planner records in PLAN.md.

2. **`Bootstrap` struct gains `.Stats` field, or `main.go` allocates the Registry separately.** BRAINSTORM §4.1 shows the field-on-Bootstrap shape; the alternative is a free-standing alloc in `main.go` with the Registry threaded into the constructors directly. **Recommendation: field-on-Bootstrap** so future xDS phases that add dynamic config-reload have a place to thread the Registry through a config-update path. Planner records in PLAN.md.

3. **`server.live` set-once mechanism.** `sync.Once` inside `handleReady` vs. CAS-on-zero vs. unconditional `Set(1)` on every `/ready` 200. All three converge to the same exposition value. **Recommendation: `sync.Once`** (defensive against future refactors that might Set(0) between transitions; cheap; well-understood). Planner records in PLAN.md.

4. **HCM factory closure's per-instance metric alloc timing.** Filter-build inside `listenerManager.New(...)` is pre-Freeze (the listener manager's New finishes synchronously before `admin.Listen()` returns); per-HCM filter chains are constructed inside `listenerManager.New`'s loop. The planner verifies this ordering at PLAN write time and asserts no per-request `NewCounter` on the hot path (LBP-1 is the contract).

5. **M-9 unit test file location.** `internal/filter/hcm/h2/router_action_test.go` (new file) or appended to an existing test file. **Recommendation: new file** (clean separation; the carry-forward concern is its own surface). Planner picks at PLAN time.

6. **Fixture-0005 driver pattern: in-band assertions vs. generic `StatsExpectations` Driver-interface extension.** BRAINSTORM §2.5 outlines a driver-side scrape-and-diff pattern; the runner could surface this as a generic Driver-interface extension (mirroring the optional `H2Expectations` extension flagged in the master 05 SPEC §10 #3) or keep it in-band like the 0004 driver does (per the 05.2 SPEC §10 #3 "in-band" recommendation). **Recommendation: in-band** (smaller harness surface; matches the 05.2 precedent; the per-fixture assertion pattern is established). Planner records in PLAN.md.

7. **Concrete ADR numbers for ADR-0059..ADR-0064.** Per `DECISIONS.md` tail at master `75a6bf9` being ADR-0058, the next-free is ADR-0059; 06.1's six ADRs land at ADR-0059..ADR-0064. The planner re-verifies next-free at write time (per ADR-0004's autonomous-numbering rule) and assigns the six anticipated topics to the six numbers in the order they're authored in PLAN.md. The topical ordering above (architecture / histograms-deferred / flattening / equivalence / per-endpoint-deferred / stats_tags-not-honored) is the suggested authoring order; the planner may permute.

8. **`server.uptime` future-emission.** Currently NOT-EMITTED per BRAINSTORM §2.3. The future phase that lands histograms (per ADR-0060) is the natural home for `server.uptime` because both depend on monotonic-clock + per-scrape recompute. The planner does NOT pre-decide; ADR-0060's Consequences section flags `server.uptime` as a co-deferred item.

## 13. Phase-05.2 REVIEW carry-forward triage (per BRAINSTORM §2.4)

Phase-05.2 closed with REVIEW Minor findings M-4 / M-9 / M-10 / M-12 carrying forward. Phase-06.1 disposition:

### 13.1 Bundled with 06.1 — ADR-0061 or a small dedicated landing task

- **M-9 — Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error path.** *Bundled with 06.1.* The 05.2 REVIEW explicitly deferred this to "phase-06 observability when logging/metrics surface lands." The fix is ~5 LoC + ~20 LoC test, mechanical, and the surface (`log.Printf` to stderr) matches what 06.1 introduces. Lands as a small task in PLAN.md alongside the M-9 unit test file (per §12 #5).

### 13.2 Carried forward to later phases (per-finding disposition)

- **M-4 — `readClientPreface` not ctx-aware (`internal/filter/hcm/h2/conn.go`).** *Deferred.* H2 connection hardening, not observability. Fits a future H2-hardening sub-phase or the upstream-robustness family. The 05.2 SPEC §12.2 already noted "Phase-04 H1 has the same shape on the H1 read path (no regression); the proper fix is at the listener-manager level via uniform OS read deadlines" — that home is a phase 06.x or 07 concern. **Target-phase candidates: a dedicated H2-hardening sub-phase (06.x) OR fold into phase 07's filter-chain framework OR the upstream-robustness family.** The carry-forward is tagged "phase-06.x-or-07-must-consider" in the dispositions table.

- **M-10 — `SETTINGS_TIMEOUT` absent (`internal/filter/hcm/h2/client.go`).** *Deferred.* Same reasoning as M-4. RFC 9113 §6.5.3's "MAY" leaves this optional; h2spec sends SETTINGS_ACK promptly so the gap is dormant. The proper fix lands with the listener-manager's per-conn timeout policies. **Target-phase candidates: same as M-4.** Tagged "phase-06.x-or-08-must-consider".

- **M-12 — `closedStreams` map unbounded (`internal/filter/hcm/h2/conn.go`).** *Deferred.* Long-lived-conn memory growth is a hardening concern, not an observability one. Under 06.1's per-request-fresh-upstream-conn discipline (inherited from ADR-0056), the map's growth is bounded per-conn-lifetime; the issue surfaces only when conn pooling lands. **Target-phase candidate: the upstream-robustness family (specifically, the H2 conn-pooling sub-phase that supersedes ADR-0056).** Tagged "upstream-robustness-must-consider".

The dispositions table for the reviewer's audit trail:

| Finding | Disposition | Target-phase candidates |
|---|---|---|
| M-4 (readClientPreface ctx-unaware) | Deferred — out of 06.1 | dedicated H2-hardening sub-phase / phase 07 / upstream-robustness family |
| M-9 (h2RouterActionAdapter.WriteH2 missing log line) | **Bundled with 06.1** | (this phase) |
| M-10 (SETTINGS_TIMEOUT absent) | Deferred — out of 06.1 | dedicated H2-hardening sub-phase / phase 08 |
| M-12 (closedStreams map unbounded) | Deferred — out of 06.1 | upstream-robustness family (H2 conn-pooling sub-phase) |

ADR-0061 (or a separate carry-forward ADR — planner picks at PLAN write time per §12 #7's flexibility) is the formal landing for the §13.1 M-9 bundle. The §13.2 deferred items land as plain task entries in a future PLAN.md (not 06.1's); no ADR is authored in 06.1 for the deferred items because the deferral itself is the SPEC's record (per ADR-0017 doctrine that "small mechanical fixes do not require ADRs").

## 14. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 06.1's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) **non-vacuous** (fixture 0005 differential green; first non-vacuous gate-(a) on the observability surface).
- [ ] `internal/stats/` package exists; `Registry` + `Counter` + `Gauge` + `Walk` + `Freeze` implemented; `WriteProm` writer in `prom.go`; the 17-name flattening logic in `name.go`; the static HELP-text map in `name.go` covers the 13 unique Prometheus names emitted by 06.1.
- [ ] **No third-party Prometheus dependency.** `go.mod` does not contain `github.com/prometheus/client_golang` or any other Prometheus library import (grep-verifiable).
- [ ] LBP-1 invariant enforced: `Registry.Freeze()` is called from `cmd/envoy-go/main.go` after admin server starts accepting and before listener manager begins accepting connections; post-Freeze `NewCounter` / `NewGauge` calls panic with the diagnostic `stats: registry frozen: cannot register %q post-boot`. Unit test in `registry_test.go` asserts the panic.
- [ ] **Rule SN4 has been empirically verified against a v1.37.2 reference Envoy scrape** — the SPEC commit lands the verified rule with verbatim scrape evidence in §10.1 (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`; pinned image per `ENVOY_TARGET.md`) and a citation of the `RESPONSE_CODE_CLASS` tag entry in `source/common/config/well_known_names.cc` at v1.37.2. Reviewer at REVIEW time grep-checks: (a) the `# TYPE envoy_cluster_upstream_rq_xx counter` and `# TYPE envoy_http_downstream_rq_xx counter` blocks are present in §10.1's evidence block; (b) the negative-confirmation grep statement (`grep -E 'envoy_[a-z_]*_(1xx|2xx|3xx|4xx|5xx)'` returning 0 matches over the full scrape) is present; (c) the 06.1 phase-done commit's in-place edit of `BEHAVIOR_CONTRACT.md ## Stat-name mapping` carries Rule SN4 in the same form as §10.1 (no drift between SPEC §10.1 and the contract addition).
- [ ] `internal/admin/prometheus.go` exists; `handlePrometheus(registry)` returns a `http.HandlerFunc` that writes `Content-Type: text/plain; version=0.0.4; charset=utf-8` and the alphabetically-sorted-by-Prom-name exposition; routed at `GET /stats/prometheus` by `admin.New`.
- [ ] `cluster.NewManager`, `cluster.NewManagerWithBaseDir`, `admin.New`, and `listenerManager.New` signatures all gain a `*stats.Registry` parameter; all call sites updated (grep-verifiable: `cluster.NewManager(` / `admin.New(` / `listenerManager.New(` show the new signatures uniformly).
- [ ] All 17 stat-emit call sites are grep-verifiable: 2 in `internal/listener/listener.go`'s accept loop, 5 in `internal/filter/hcm/filter.go` + `actions.go`'s HCM dispatch, 8 in `internal/cluster/`'s manager/dial paths, 1 in `internal/admin/admin.go`'s `handleReady`, plus the implicit "NOT-EMITTED" `server.uptime` enforced by absence in the codebase.
- [ ] `BEHAVIOR_CONTRACT.md ## Stat-name mapping` is populated with the 17-name table from §6 and the SN1–SN8 rules from §10.1; `## Equivalence Matrix` has the new "Stats output" row from §10.2; the in-place edits land at the phase-done commit (NOT the SPEC commit) per ADR-0052's discipline.
- [ ] `## Access log field mapping` placeholder remains empty (06.2's deliverable); the BEHAVIOR_CONTRACT in-place edit at the 06.1 phase-done commit is grep-verifiable to the `## Stat-name mapping` and `## Equivalence Matrix` subsections only.
- [ ] All six 06.1 ADRs (the planner-assigned ADR-0059..ADR-0064 mapping to architecture / histograms-deferred / flattening rules / differential equivalence shape / per-endpoint-deferred / stats_tags-not-honored) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. The ADR-numbering-shift discipline from ADR-0045 + ADR-0004 is honored (the planner verified next-free at write time and the six numbers are contiguous; topical-vs-commit-order non-monotonicity is permitted and recorded in each ADR's `Lands-in-task` field per the 05.2 ADR-0055..ADR-0058 precedent).
- [ ] Fixture `0005-prometheus-stats/` is committed in full: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The 5-request workload + per-counter delta-equality + per-gauge snapshot-equality assertion shape is implemented in `driver/driver.go`; the `--concurrency 1` reference invocation is honored.
- [ ] `test/conformance/h2spec/` is UNCHANGED; pin still at the ADR-0051 SHA; 53/53 PASS.
- [ ] No phase-04 / 05.1 / 05.2 fixture (`0000`/`0001`/`0002`/`0003`/`0004`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `STATE.md` is at lifecycle-state 6 for 06.1; `ROADMAP.md` row `06.1` is `done`; row `06` (parent) stays `in-progress`; row `06.2` is `planned`. The §5.1 phase-done commit's message names every ADR introduced or referenced.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the §5.3 verification protocol; SHA-fill for each task entry per the phase-04 / 05.1 / 05.2 convention.
- [ ] The phase-05.2 REVIEW carry-forward triage (§13) is faithfully recorded: M-9 is bundled into 06.1 (the log-line + unit test land in `internal/filter/hcm/h2/router_action.go` + `router_action_test.go`); M-4 / M-10 / M-12 are noted as deferred in the SPEC's §13.2 dispositions table and are NOT implemented in 06.1.
- [ ] **`FuzzPromTextFormat` is committed** under `internal/stats/fuzz_test.go`; runs clean at the 30s ADR-0018 budget; total fuzzer count post-06.1 is 7.
- [ ] No third-party stats library is imported. The `internal/stats` package's external dependencies are limited to the Go standard library (`sync`, `sync/atomic`, `io`, `regexp`, `sort`, `strings`, `fmt`).

When all boxes above are checked, phase 06.1 is `done`, the parent row `06` stays `in-progress` (closes only at 06.2's phase-done), and the project advances to phase 06.2 (access-log) at lifecycle-state 1.

## 15. References

- **BRAINSTORM:** `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` — the authoritative design source; this SPEC distills BRAINSTORM §§2–9 into formal SPEC shape. Every decision in this SPEC traces back to BRAINSTORM.
- **Parent master SPEC:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` — phase-06 parent; carries the cross-cutting decisions that apply to BOTH 06.1 and 06.2.
- **Sibling SPEC stub:** `docs/envoy-go/phases/06.2-access-log/README.md` — placeholder for 06.2; will be superseded by the 06.2 SPEC at lifecycle-state 1 of that sub-phase.
- **Structural precedent (sub-phase SPEC shape):** `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md` and `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` — the §-numbering, header tone, acceptance-bullet format, and overall shape this SPEC mirrors.
- **Structural precedent (parent master SPEC shape):** `docs/envoy-go/phases/05-http-2/SPEC.md`.
- **BEHAVIOR_CONTRACT.md:** `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the contract this SPEC's §10 extensions land in (in-place edit at phase-done per ADR-0052).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Cited in Rule SN4's empirical-verification gate (§10.1 + §14 acceptance bullet). The Rule SN4 verification scrape uses the image at this SHA against a minimal HCM + 1-cluster bootstrap.
- **DECISIONS.md:** `docs/envoy-go/DECISIONS.md` — ADR-0001 (template), ADR-0004 (autonomous-numbering rule), ADR-0008 (Envoy pin, referenced via ENVOY_TARGET.md), ADR-0010 (`dns_lookup_family: V4_ONLY` for STRICT_DNS reference clusters), ADR-0017 (small-mechanical-fixes do not require ADRs), ADR-0018 (fuzzer 30s short-budget policy), ADR-0028 (`--concurrency 1` reference invocation), ADR-0045 (planner-time-split discipline), ADR-0051 (h2spec pin SHA), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization), ADR-0058 (last extant ADR at master `75a6bf9`; the 06.1 ADRs start at ADR-0059).
- **BOOTSTRAP_PROMPT cross-references:**
  - **§5** (Phase Lifecycle State Machine) — the lifecycle states 1 (SPEC drafting; this commit's deliverable) → 6 (REVIEW approved + phase-done) that 06.1 traverses.
  - **§5.3** (Commit message format) — the phase-done commit message format `phase 06.1: stats-prometheus [ADR-0059, ADR-0060, ..., ADR-0064]` plus differential-surface + conformance summary.
  - **§6.2** (How to split — planner-time-split discipline) — the discipline ADR-0045 invokes for the 06.1 + 06.2 split; this SPEC honors §6.2 by being one of two sibling sub-phase SPECs under the parent.
  - **§7.5** (Phase-done gate — six-gate checklist) — the gate set §3 specializes for 06.1.
  - **§4.1** (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit) — the row-flip discipline §4.4 honors.
- **ROADMAP.md:** `docs/envoy-go/ROADMAP.md` — rows `06`, `06.1`, `06.2` per the split landed in this commit's ROADMAP edit.
- **PROGRESS-style precedents:** `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`, `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — the SHA-fill convention 06.1's PROGRESS.md mirrors.
