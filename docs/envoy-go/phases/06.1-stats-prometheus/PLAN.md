# Phase 06.1 — Stats + Prometheus admin endpoint (`internal/stats` package, `/stats/prometheus` exposition, 17 stat-emit call sites, fixture 0005) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit and at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness check — every ADR introduced or referenced is named in the phase-done commit message), §6.1 (split gate — ~1500 LoC AND <25 tasks), §7 (differential contract), §7.5 (phase-done six-gate checklist that §3 of the SPEC specialises for 06.1); `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; ~738 LOC, read in full); `docs/envoy-go/phases/06-observability-baseline/SPEC.md` (parent master SPEC, the cross-cutting context for the 06.1 + 06.2 split); `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (the brainstorm-close artefact at master `75a6bf9` that the 06.1 SPEC distils §§2–9 from); `docs/envoy-go/phases/05.2-upstream-h2/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 05.2 PLAN is the structural precedent — §-numbering, heredoc-style task headers, ADR-with-first-use-commit discipline, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections, TDD-step granularity); `docs/envoy-go/phases/05.1-downstream-h2/PLAN.md` (secondary structural precedent; 16 tasks, similar in shape); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0058 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-numbering rule, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0013** go-control-plane v1.32.4 proto-types-only pin, **ADR-0015** admin Date header rule, **ADR-0016** bootstrap loader unknown-field policy + blank-import amendment policy, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0024** per-cluster RR scope, **ADR-0027** STATIC-vs-STRICT_DNS divergence, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0035** fixture-0002 differential scope, **ADR-0042** HTTP-filter chain shape `[router]`, **ADR-0045** planner-time-split discipline (the 06.1 + 06.2 split), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0055** flow-control discipline, **ADR-0056** per-request fresh upstream H2 dial, **ADR-0057** ADR-0035 H2 leg closure, **ADR-0058** trailers observed but not forwarded — the tail of the verified next-free check); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## Stat-name mapping` placeholder at lines ~48–53 and `## Equivalence Matrix` row at line ~19, both edited at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin Rule SN4's empirical evidence cites); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 06.1 — D-3.7 reserves pin bumps for dedicated phases).

**Goal:** Land envoy-go's stats subsystem — an in-tree atomic-counter `*stats.Registry` backed by an admin-side Prometheus text-format exporter — and thread 17 stat-emit call sites through the listener / HCM / cluster hot paths so a Prometheus scrape of `/stats/prometheus` returns a behaviorally-equivalent metric set to upstream Envoy v1.37.2 under a defined load (per SPEC §1 #1–#3 + §6's verbatim 17-name catalog). Concretely: a NEW `internal/stats` package (`Registry`, `Counter`, `Gauge`, hierarchical-dotted-name flattening helpers per Rules SN1–SN8 from SPEC §10.1, the Prometheus text-format writer that walks the Registry and emits `# HELP` / `# TYPE` / metric lines sorted alphabetically by Prometheus name) — Registry is constructed once at boot, threaded explicitly through the existing component graph, Counter/Gauge increments are lock-free atomics, `Registry.Walk(fn)` runs under an `RWMutex` whose write lock is held only at boot-time register sites, and **no third-party Prometheus library** is imported (per ADR-0059); a NEW `GET /stats/prometheus` admin endpoint added alongside the phase-01 `/ready` handler — read-only against the Registry, returns `Content-Type: text/plain; version=0.0.4; charset=utf-8`, sorts metrics alphabetically by Prometheus name, emits the static-`# HELP`-text discipline from SPEC §11.2 (per SPEC §1 #2); 17 stat-emit call sites threaded through listener / HCM / cluster — 2 listener (`listener.<addr>.downstream_cx_total` / `_active`) + 5 HCM (`http.<stat_prefix>.downstream_rq_total` + four `_Nxx`) + 8 cluster (`cluster.<n>.upstream_rq_total` + four `_Nxx` + `upstream_cx_total` + `upstream_cx_active` + `membership_total`) + 2 server (`server.live` emitted; `server.uptime` explicitly NOT-EMITTED per SPEC §6) — each metric created at register-time (boot), held by reference on the relevant struct, incremented lock-free on the hot path, never re-registered (per SPEC §1 #3); the eight stat-name → Prometheus-name flattening rules SN1–SN8 (Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 settled at brainstorm-close per BRAINSTORM §7.1; **Rule SN4 empirically verified at SPEC-drafting time** against reference Envoy v1.37.2's `/stats/prometheus` scrape — SPEC §10.1 records the verbatim evidence — with the trailing class digit stripped from the metric name (so `cluster.<n>.upstream_rq_2xx` flattens to base name `envoy_cluster_upstream_rq_xx` ending in literal `_xx`), label name `envoy_response_code_class`, label value the single class digit as a string `"2"`/`"3"`/`"4"`/`"5"` — per ADR-0061 anchored at Task 4); the concurrency invariant LBP-1 ("list before play") — the Registry's *list of metrics* is mutable only during boot, post-`Freeze()` `NewCounter`/`NewGauge` calls panic with `stats: registry frozen: cannot register %q post-boot`, this is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments (per SPEC §5.3); the carry-forward bundle of 05.2 REVIEW Minor M-9 ("Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error") — bundled with 06.1 because the 05.2 REVIEW explicitly deferred it to "phase-06 observability when logging/metrics surface lands"; ~5 LoC fix + ~20 LoC test (per SPEC §1 #6 + §13.1); a NEW differential fixture `test/differential/0005-prometheus-stats/` — the project's first observability-surface differential and first non-vacuous gate-(a) on the observability surface — with a 5-request defined-load workload (target statuses `[200, 200, 404, 200, 502]`) issued sequentially at both proxies, before/after `/stats/prometheus` snapshots scraped + parsed, per-counter delta-equality (`delta_envoy_go == delta_envoy`) and per-gauge snapshot-equality (`after_envoy_go == after_envoy`) asserted across the 17 enumerated stat names (per SPEC §7 + ADR-0062); a NEW `internal/stats.FuzzPromTextFormat` fuzzer at the 30s ADR-0018 budget, fuzzing adversarial stat-name + label-value strings into the Prometheus text-format writer (asserts no panic; total fuzzer count post-06.1 = 7); SIX new ADRs (ADR-0059..ADR-0064 — re-verified at Task 1 step 1 against `DECISIONS.md` tail being ADR-0058) covering internal-stats-store architecture, histograms-deferred, flattening rules SN1–SN8, differential equivalence shape, per-endpoint-deferred, `stats_config.stats_tags` not honored; a `BEHAVIOR_CONTRACT.md ## Stat-name mapping` in-place-edit population (the placeholder at lines 48–53 fills with the 17-name table from SPEC §6 + the SN1–SN8 rules from SPEC §10.1) and a new equivalence-matrix row "Stats output" supersedes the seed-row at line 19 — both edits land at the 06.1 phase-done commit per ADR-0052 (per SPEC §10 + §4.4); STATE.md / ROADMAP.md / PROGRESS.md updates with row 06.1 → `done` at the phase-done commit, parent row 06 stays `in-progress` (closes only at 06.2's phase-done — per SPEC §4.4's mirror of the 05/05.1/05.2 closure pattern). After phase 06.1, the project has proven the first half of its seventh central engineering claim: envoy-go emits behaviorally-equivalent counter and gauge signals under a defined load — visible at `/stats/prometheus`, byte-equal in metric name + label keys + label values + types to upstream Envoy's exposition format on the 17 enumerated names — without coupling to any third-party stats or Prometheus dependency. The access-log half of the claim is delivered by 06.2; the project advances to phase 06.2 (access-log) at lifecycle-state 0.

**Architecture:** The 06.1 surface is the additive introduction of one new package (`internal/stats/`) plus one extended package (`internal/admin/`) plus the threading of a `*stats.Registry` parameter through five constructor signatures (`cluster.NewManager`, `cluster.NewManagerWithBaseDir`, `listener.NewManager`, `listener.NewManagerWithBaseDir`, `listener.NewManagerWithBaseDirAndAllowH2C`, `admin.New`) plus the per-HCM-instance metric allocation captured by the HCM-build closure inside the listener manager — the Registry-threading is the surface BRAINSTORM §3 note 6 flagged as "must be enumerated as part of the SPEC's Files-touched inventory so it isn't surprise scope at PLAN time" and SPEC §4.2's file inventory enumerates each constructor change explicitly. Concretely: `internal/stats/registry.go` (NEW; ~120 LoC) defines `type Registry struct { mu sync.RWMutex; metrics []Metric; byName map[string]Metric; frozen atomic.Bool }` with methods `NewRegistry() *Registry`, `(*Registry).NewCounter(name string) *Counter` (panics if frozen, panics on invalid name `^[a-zA-Z_][a-zA-Z0-9_.]*$`, panics on duplicate name), `(*Registry).NewGauge(name string) *Gauge` (same panic discipline), `(*Registry).Walk(fn func(Metric))` (RLocks, iterates `metrics` in registration order — ordering is not part of the contract; the Prometheus writer sorts post-walk), `(*Registry).Freeze()` (sets `frozen = true` via atomic CAS; called from `cmd/envoy-go/main.go` after admin server is up; idempotent); `internal/stats/counter.go` (NEW; ~40 LoC) defines `type Counter struct { name string; v atomic.Uint64 }` with `Inc()` (1-cycle `AddUint64(1)`), `Add(delta uint64)`, `Load() uint64`, `Name() string`, satisfies the `Metric` interface (Name + Type + Load-as-string); `internal/stats/gauge.go` (NEW; ~50 LoC) defines `type Gauge struct { name string; v atomic.Int64 }` with `Inc()`/`Dec()`/`Add(delta int64)`/`Set(value int64)`/`Load() int64`/`Name() string` — negative values are permitted (a `Dec` not paired with an `Inc` is defensive — gauge reflects reality per BRAINSTORM §5.2); `internal/stats/name.go` (NEW; ~150 LoC + ~50 LoC for the 13-entry static helpText map) defines `type Label struct { Key string; Value string }`, `flattenToProm(internal string) (promName string, labels []Label, err error)` (the SN1–SN8 logic — Rule SN1 strips `cluster.<n>.` prefix → label `envoy_cluster_name` + suffix prefixed with `envoy_cluster_`; Rule SN2 strips `http.<stat_prefix>.` → label `envoy_http_conn_manager_prefix` + suffix prefixed with `envoy_http_`; Rule SN3 strips `listener.<addr>.` → label `envoy_listener_address` + suffix prefixed with `envoy_listener_`; **Rule SN4** detects `_<digit>xx` suffix where digit ∈ {1..5}, strips the entire `_<digit>xx` to yield base ending `_xx`, emits label `envoy_response_code_class` with value the single digit string per the SPEC §10.1 verified form; Rule SN5 maps `server.<rest>` → `envoy_server_<rest>` no labels), `escapeLabelValue(string) string` (`\` → `\\`; `"` → `\"`; `\n` → `\\n` per Prometheus exposition format spec), `var helpText = map[string]string{...}` static map keyed by Prometheus name → human-English description (10 entries covering the 13 unique Prometheus names emitted by 06.1 — `envoy_listener_downstream_cx_total`, `envoy_listener_downstream_cx_active`, `envoy_http_downstream_rq_total`, `envoy_http_downstream_rq_xx`, `envoy_cluster_upstream_rq_total`, `envoy_cluster_upstream_rq_xx`, `envoy_cluster_upstream_cx_total`, `envoy_cluster_upstream_cx_active`, `envoy_cluster_membership_total`, `envoy_server_live`); `internal/stats/prom.go` (NEW; ~80 LoC) defines `WriteProm(w io.Writer, r *Registry) error` which walks the Registry, flattens each metric via `name.go`, groups by Prometheus name (status-class collapse joins the four `_Nxx` Prometheus names into one base-name group with four `envoy_response_code_class`-keyed lines), sorts alphabetically by Prometheus name, emits `# HELP <name> <text>` then `# TYPE <name> counter|gauge` then one metric line per fully-qualified label set, blank-line group separator between Prometheus-name groups; `internal/stats/fuzz_test.go` (NEW; ~50 LoC) carries `FuzzPromTextFormat` per BRAINSTORM §6.4 — fuzzes adversarial stat-name strings + adversarial label-value strings into `WriteProm`, asserts no panic, asserts the output round-trips through a Prometheus-format-aware parser without error; `internal/admin/prometheus.go` (NEW; ~40 LoC) defines `func handlePrometheus(r *stats.Registry) http.HandlerFunc` returning a handler that writes `Content-Type: text/plain; version=0.0.4; charset=utf-8` then calls `stats.WriteProm(w, r)` — errors from `WriteProm` are logged and otherwise ignored (no retry, no error response — too late, headers already sent per BRAINSTORM §5.3); `internal/admin/admin.go` (MODIFIED) widens `admin.New(addr string)` to `admin.New(addr string, registry *stats.Registry)`, registers `/stats/prometheus` alongside `/ready` inside `Start()`, allocates the `server.live` gauge at `New` time and Set(1)s it inside `handleReady` via `sync.Once` per SPEC §12 #3; `internal/cluster/manager.go` (MODIFIED) widens `NewManager(bs *bootstrapv3.Bootstrap)` and `NewManagerWithBaseDir(bs, baseDir)` to gain a `*stats.Registry` parameter; on each `cluster.buildCluster` call the manager creates the 8 cluster-scope metrics per cluster (`cluster.<n>.upstream_rq_total`, `cluster.<n>.upstream_rq_<2,3,4,5>xx`, `cluster.<n>.upstream_cx_total`, `cluster.<n>.upstream_cx_active`, `cluster.<n>.membership_total`) and stores the pointers on the `*Cluster` struct; `internal/cluster/cluster.go` (MODIFIED) `Cluster` struct gains 8 unexported metric-pointer fields (`upstreamRqTotal`, `upstreamRq2xx..5xx`, `upstreamCxTotal`, `upstreamCxActive`, `membershipTotal`); `Cluster.Dial(ctx)` is extended in place to `c.upstreamCxTotal.Inc()` + `c.upstreamCxActive.Inc()` on successful dial (TLS or plaintext branch), and the returned `net.Conn` is wrapped in a `connWithGauge{Conn: c, dec: c.upstreamCxActive.Dec}` that calls the gauge `Dec()` exactly once on `Close()` (the wrapper lives in `cluster.go` adjacent to `Dial`; ~25 LoC); `internal/cluster/dial_h2.go` (MODIFIED) reuses `connWithGauge` — `DialH2` now wraps the underlying `*tls.Conn` with the gauge wrapper before passing to `h2.NewClientConn` so `(*ClientConn).Close()`'s GOAWAY-then-FIN dispatch transitively triggers the gauge Dec; `internal/listener/manager.go` (MODIFIED) widens `NewManager`, `NewManagerWithBaseDir`, `NewManagerWithBaseDirAndAllowH2C` signatures with a `*stats.Registry` parameter; on each listener build the manager creates 2 listener-scope metrics per listener (`listener.<addr>.downstream_cx_total`, `listener.<addr>.downstream_cx_active`) — the `<addr>` is normalized like Envoy does (`0.0.0.0:10000` → `0.0.0.0_10000`); the HCM factory closure captures `*stats.Registry` so per-HCM-instance metric allocation works (per SPEC §5.4 + §12 #4: filter-build inside `listener.NewManager...(...)` is pre-Freeze because the listener manager's `New(...)` finishes synchronously before `admin.Listen()` returns, per the boot-order in `cmd/envoy-go/main.go`); `internal/listener/listener.go` (MODIFIED) accept-loop hot-path edits — `+2 LoC`: on each accept `cx_total.Inc()` + `cx_active.Inc()`; on accepted-conn close (driven by the connection-handler goroutine's deferred close), `cx_active.Dec()`; `internal/filter/hcm/filter.go` (MODIFIED) at `NewFilter` time the filter calls `registry.NewCounter("http.<stat_prefix>.downstream_rq_total")` and the four `_Nxx` counters, holds them by reference on the filter struct; HCM dispatch entry hot-path `+1 LoC`: `downstream_rq_total.Inc()` on first byte of request line/headers (per SPEC §12 #1's recommendation of site (a)); HCM response hook `+3-5 LoC`: on response status finalization, the integer-divide `code / 100` selects the status-class counter and `Inc()`s it; `internal/filter/hcm/actions.go` (MODIFIED) — H1 `routerAction.do` request-dispatch entry: `c.upstreamRqTotal.Inc()`; on response status finalization: `c.upstreamRq<Nxx>.Inc()` per the same `code / 100` discipline; H2 `routerActionH2.do`: same Inc pattern; `internal/filter/hcm/h2/router_action.go` (MODIFIED — M-9 carry-forward bundled per SPEC §13.1) `h2RouterActionAdapter.WriteH2` adds a `log.Printf("h2: doH2 error: %v", err)`-style log line on the `doH2` error path before returning — ~5 LoC + ~20 LoC test in `router_action_test.go` (new file per SPEC §12 #5); `internal/bootstrap/bootstrap.go` (MODIFIED) the `Bootstrap` struct gains a `Stats *stats.Registry` field constructed in `Load()` per SPEC §12 #2's recommended factoring (field-on-Bootstrap shape) — future xDS phases that add dynamic config-reload have a place to thread the Registry through a config-update path; `cmd/envoy-go/main.go` (MODIFIED) at boot threads `bs.Stats` into `cluster.NewManagerWithBaseDir`, `listener.NewManagerWithBaseDirAndAllowH2C`, `admin.New`; after the listener manager begins accepting (and after admin is listening on its port), calls `bs.Stats.Freeze()` to enforce LBP-1 — `+5-8 LoC` net; `cmd/envoy-go/main_test.go` (MODIFIED) bootstrap-variant smoke tests (pre-existing four variants from 05.2) thread a `Bootstrap.Stats` Registry through the constructor changes; `test/differential/0005-prometheus-stats/` (NEW directory) carries `envoy-go.yaml` (1 listener `l_h1` binding `127.0.0.1:0` plaintext, 1 HCM with `codec_type: HTTP1` + `stat_prefix: ingress_http`, 1 STATIC cluster `c0` pointing at the controlled backend), `envoy.yaml` (reference; same shape with STRICT_DNS cluster pointing at `host.docker.internal:<backend-port>` per ADR-0010 + `--concurrency 1` per ADR-0028), `expectations.yaml` (the 17-stat allow-list table from SPEC §7.3 verbatim), `README.md` (purpose + STATIC-vs-STRICT_DNS divergence + 5-request defined-load shape + cross-reference to BEHAVIOR_CONTRACT.md), `driver/driver.go` (5 H1 requests with target statuses `[200, 200, 404, 200, 502]`; `/` → cluster `c0`, `/missing` → HCM-direct 404, `/` with `X-Backend-Status: 502` → backend explicit 502; pre-load + post-load `/stats/prometheus` snapshots + parse + 17-name extract; per-counter delta-equality + per-gauge snapshot-equality assertions per ADR-0062), `driver/driver_test.go` (scrape-parser unit tests), `backends/main.go` (small Go program: HTTP/1.1 echo on a configurable port, reads `X-Backend-Status` header, returns the requested status with body `bad gateway\n` for 502 / `OK\n` for 200 / honors arbitrary per the header — explicit-502 path keeps the differential decoupled from dial-error→status-code mapping per BRAINSTORM §2.5); `test/differential/runner.go` (MODIFIED) blank-imports the new fixture-0005 driver package and the runner's per-fixture loop calls the driver's pre-load + post-load scrape hooks per the in-band pattern (per SPEC §12 #6 — in-band like the 0004 driver, no generic `StatsExpectations` Driver-interface extension); `BEHAVIOR_CONTRACT.md` is edited in place at the phase-done commit per ADR-0052 — the empty `## Stat-name mapping` placeholder fills with the 17-name table + SN1–SN8 rules; the empty `## Access log field mapping` placeholder stays empty (06.2's deliverable); the `## Equivalence Matrix` row 19 is superseded with the new "Stats output" row from SPEC §10.2; the six ADRs ADR-0059..ADR-0064 land at first-use commit ordering per the phase-02/03/04/05.1/05.2 precedent.

**Tech Stack:**
- Go 1.23 (unchanged from 05.2; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `sync`, `sync/atomic`, `io`, `regexp`, `sort`, `strings`, `fmt`, `net/http`, `time`, `errors`, `bytes`, `context`, `log`, `bufio` — the exhaustive set the `internal/stats/` package and the `internal/admin/prometheus.go` handler consume.
- **NEW: no third-party Prometheus library.** `go.mod` MUST NOT contain `github.com/prometheus/client_golang` or any other Prometheus library import. The acceptance check at Task 15 step 4 grep-verifies the absence (per ADR-0059 + SPEC §14 acceptance bullet "No third-party Prometheus dependency").
- **NEW: no third-party stats library either.** The `internal/stats` package's external dependencies are limited to the Go standard library. Same Task 15 step 4 grep-verifies (per SPEC §14 final acceptance bullet "No third-party stats library is imported").
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged).
- `google.golang.org/protobuf` (transitively; the `Bootstrap` struct's `Stats` field landing does not change proto consumption).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0005's reference (Envoy in a Docker container) — same harness as 05.1's conformance gate consumes for h2spec; phase 06.1 does not modify `test/differential/harness.go`.
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0005's reference image.
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 06.1 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- **Forbidden runtime imports (D-3.2 + ADR-0059):** `github.com/prometheus/client_golang/...`, `github.com/prometheus/common/...`, `github.com/prometheus/procfs/...`, ANY `github.com/*/prometheus/*` library. The boundary grep at Task 15 step 4 enforces. Test-side use is also forbidden — the fixture-0005 driver parses the Prometheus exposition with a small in-fixture parser (~30 LoC), NOT via a third-party library, so the grep applies uniformly across `_test.go` and production code.
- Test-side allowed exception: the fuzz target's seed corpus may include adversarial bytes (newlines, NULs, etc.) but does not import any third-party fuzzer infrastructure; it consumes Go's native `testing.F`.
- `internal/stats/` is a NEW package introduced in 06.1; no pre-existing imports of it exist outside of `doc.go`'s phase-00 placeholder (which Task 2 rewrites).
- `internal/admin/` extends in place; no new imports outside the standard library + `github.com/esalaine/envoy-go/internal/stats`.
- `internal/cluster/`, `internal/listener/`, `internal/filter/hcm/` extensions add a single import path each: `github.com/esalaine/envoy-go/internal/stats`. The package-import-graph stays acyclic (the boundary check is grep-verifiable: no `internal/stats` file imports any `internal/...` other than what the standard library and `go-control-plane` types already require — the stats package is a leaf).

---

## Scope check — why phase 06.1 ships as one sub-phase

Net change estimate: **~1700 LoC** broken down by component (mirroring the 05.2 PLAN's component-table convention):

- `internal/stats/registry.go` ~120 + `registry_test.go` ~150 = ~270
- `internal/stats/counter.go` ~40 + `counter_test.go` ~80 = ~120
- `internal/stats/gauge.go` ~50 + `gauge_test.go` ~80 = ~130
- `internal/stats/name.go` ~200 + `name_test.go` ~150 = ~350
- `internal/stats/prom.go` ~80 + `prom_test.go` ~120 = ~200
- `internal/stats/fuzz_test.go` ~50
- `internal/stats/doc.go` ~20 (rewrite from phase-00 stub)
- `internal/admin/prometheus.go` ~40 + `prometheus_test.go` ~100 = ~140
- `internal/admin/admin.go` extension ~20 + `admin_test.go` extension ~50 = ~70
- `internal/admin/doc.go` ~10 (rewrite to mention `/stats/prometheus`)
- `internal/cluster/manager.go` extension (Registry threading + 8-metric alloc per cluster) ~80 + `manager_test.go` extension ~80 = ~160
- `internal/cluster/cluster.go` extension (8 fields + `Dial` gauge wrapping + `connWithGauge`) ~40 + `cluster_test.go` extension ~60 = ~100
- `internal/cluster/dial_h2.go` extension (gauge-wrap reuse) ~10 + `dial_h2_test.go` extension ~30 = ~40
- `internal/listener/manager.go` extension (Registry threading + 2-metric alloc per listener; HCM-factory closure) ~50 + `manager_test.go` extension ~50 = ~100
- `internal/listener/listener.go` extension (accept-loop +2 LoC) + `listener_test.go` extension ~40 = ~50
- `internal/filter/hcm/filter.go` extension (NewFilter alloc + HCM dispatch entry + response-class dispatch) ~30 + `filter_test.go` extension ~80 = ~110
- `internal/filter/hcm/actions.go` extension (H1 + H2 router-action Inc) ~20 + `actions_test.go` extension ~80 = ~100
- `internal/filter/hcm/h2/router_action.go` M-9 ~5 + `router_action_test.go` ~30 = ~35
- `internal/bootstrap/bootstrap.go` extension (`Stats` field + `Load` alloc) ~10 + `bootstrap_test.go` extension ~30 = ~40
- `cmd/envoy-go/main.go` extension (Registry thread + Freeze) ~10 + `main_test.go` extension ~30 = ~40
- `test/differential/runner.go` extension (registration) ~3
- `test/differential/0005-prometheus-stats/` (envoy-go.yaml ~80 + envoy.yaml ~80 + expectations.yaml ~30 + README.md ~50 + driver/driver.go ~250 + driver/driver_test.go ~80 + backends/main.go ~80) = ~650
- `docs/envoy-go/DECISIONS.md` (six ADRs) ~300
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place edit) ~70
- `docs/envoy-go/ROADMAP.md` (row updates) ~3
- `docs/envoy-go/STATE.md` (lifecycle transitions) ~5
- `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` ~150

Total estimate: ~1700 LoC. The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1). Task count is **15** — well below the 25-task gate and within SPEC §1 #9's anticipated 12–16 range (Task 1 preconditions + Task 15 closing sweep envelop the SPEC's pure TDD-task count, matching the phase-04 / 05.1 / 05.2 precedent). LoC estimate is at the soft threshold; comparable in magnitude to 05.2 (~1500 estimated; ~2400 actual) and 05.1 (~2400 actual) which both shipped as one phase.

Phase 06.1 ships as **one** sub-phase (not split into 06.1.1 + 06.1.2) for three reasons:

1. **The split-by-surface axis (e.g. 06.1.1 = stats package + admin endpoint; 06.1.2 = call-site threading + fixture) creates two consecutive sub-phases with vacuous gate (a).** Per BOOTSTRAP §6.3 ("do not ship incomplete stubs that conformance tests can't exercise"), a 06.1.1 carrying only the `internal/stats` package + admin handler would have no differential fixture (gate (a) vacuous; the admin handler can be unit-tested but the differential CONTRACT — per-counter delta-equality between envoy-go and Envoy — needs hot-path call sites firing). Splitting also breaks the LBP-1 invariant: the 06.1.1 sub-phase would land a Registry that is never `Freeze()`d in production code (no listener manager threading the Registry through its construction → no boot wiring to call `Freeze()`), which is dead infrastructure until 06.1.2 wires it up. The 17 stat-emit call sites are the load-bearing claim that defines this sub-phase's atomic scope.

2. **Task count is at the SPEC's recommended low end; LoC estimate is the OR-leg with precedent.** Per phase-04 / 05.1 / 05.2 precedent, task-count-under-25 is the primary signal that one phase is the right shape. 06.1's 15 tasks matches SPEC §1 #9's expected 12–16 plus one preconditions task plus one closing sweep. The LoC estimate is at the soft threshold (~1700, with ~650 of that being fixture infrastructure); the OR-leg has the phase-04 / 05.1 / 05.2 precedent of accepted comparable one-phase shipments.

3. **The fixture 0005 differential is the load-bearing claim that defines this sub-phase's atomic scope.** Per BOOTSTRAP §6.3 + SPEC §1 #7, the project's first observability-surface differential is what makes 06.1 atomically claimable as "envoy-go emits behaviorally-equivalent counter and gauge signals" per SPEC §1's seventh central engineering claim. Removing fixture 0005 from 06.1 would leave 06.1 as a unit-test-only sub-phase — the same process smell SPEC §1 #7 specifically targets. Conversely, removing the call-site threading would leave fixture 0005 with nothing to differentially compare. The three components (stats package, call-site threading, fixture 0005) form a coherent atomic unit.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **2800** by the end of Task 11 (i.e., before fixture 0005's driver + backends tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A ~65% miss on a carefully-bounded sub-phase is a signal the plan's shape is wrong, not just that the work is large. Mid-execution split valve: `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger (any single task's sub-steps blow up past ~10 items) stays active. The two tasks most likely to blow past 10 sub-steps are Task 4 (`name.go` flattening — the largest single-file change after fixture-0005's driver) and Task 14 (fixture 0005 driver — orchestrates 5-request workload + scrape parser + 17-stat extract + delta + snapshot assertions). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR — the natural axis is split-by-fixture (06.1.1 = stats + admin + call-site threading; 06.1.2 = fixture 0005 + closing sweep).

---

## ADRs introduced by this plan

Six ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0058** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `## ADR-0058:` at the `be99b42` baseline; the planner re-verified at PLAN-write time that ADR-0055..ADR-0058 all landed in the 05.2 phase-done commit per the SPEC §8 anticipation; if a mid-PLAN-authoring ADR landed since the SPEC commit, re-number 06.1 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1 — the executor checks at Task 1 step 1). Per SPEC §8, phase 06.1's six ADRs land at ADR-0059..ADR-0064 in topical order. The topic-to-ADR-number map:

- **SPEC §8 ADR-0059 anticipation** (Internal Stats Store architecture) → **ADR-0059** (lands Task 2, the Registry-introduction task; first ADR landed of the six).
- **SPEC §8 ADR-0060 anticipation** (Histograms deferred from 06.1) → **ADR-0060** (lands Task 2 alongside ADR-0059 — the SPEC §8 entry explicitly notes "alongside the Registry; the ADR documents the deferral at Registry-introduction time"; topical-co-landing is the cleanest place for the deferral record).
- **SPEC §8 ADR-0061 anticipation** (Stat-name → Prometheus-name flattening rules SN1–SN8, with the empirically-pinned Rule SN4) → **ADR-0061** (lands Task 4, the `name.go` flattening logic — the first use of Rule SN4 in production code).
- **SPEC §8 ADR-0064 anticipation** (`stats_config.stats_tags` config not honored; extraction hardcoded) → **ADR-0064** (lands Task 4 alongside ADR-0061 — co-anchored at the `name.go` introduction since both ADRs are about name-flattening; per SPEC §8's Lands-in-task field; bundling at Task 4 keeps the ADR-pair reading naturally).
- **SPEC §8 ADR-0063 anticipation** (Per-endpoint cluster stats not emitted) → **ADR-0063** (lands Task 8, the cluster-side metric-allocation loop in `internal/cluster/manager.go` — first use of the cluster-level-only metric set; per SPEC §8's Lands-in-task field).
- **SPEC §8 ADR-0062 anticipation** (Differential equivalence shape for stats) → **ADR-0062** (lands Task 14, the fixture-0005 driver — first use of the per-counter-delta-equality + per-gauge-snapshot-equality assertion shape; the closing ADR before the BEHAVIOR_CONTRACT in-place edit at Task 15).

Note: the FIRST-USE ORDERING is Tasks 2, 4, 8, 14 — i.e. ADR-0059 + ADR-0060 first (Task 2), ADR-0061 + ADR-0064 second (Task 4), ADR-0063 third (Task 8), ADR-0062 fourth (Task 14). This produces an ADR-number-vs-commit-order sequence (0059, 0060, 0061, 0064, 0063, 0062) — non-monotonic in the second half. Per SPEC §8's explicit permission ("the planner may reorder commit-time landings if that reads more naturally in PLAN.md, in which case the actual ADR number assignments may permute (the four ADR-0055..ADR-0058 block in 05.2 used a non-monotonic commit-time ordering — this is permitted and recorded in the ADR's `Lands-in-task` field)"), the non-monotonic mapping is correct here. The contiguous-block discipline (ADR-0059..ADR-0064 inclusive, no gaps) is preserved; topical coherence drives the in-task pairing (ADR-0059 + ADR-0060 are both Registry-architecture concerns and pair at Task 2; ADR-0061 + ADR-0064 are both name-flattening concerns and pair at Task 4). The PLAN documents the mapping explicitly so the executor doesn't "fix" the ordering at execution time.

Summaries:

- **ADR-0059 — Internal Stats Store architecture.** Status: Accepted. Date: task-execution date. Doctrine: D-3.2 (no third-party-runtime-import for runtime-critical surfaces) + D-3.3 (own the canonical observation surface). Decision: a thin in-tree atomic-counter Registry (`internal/stats`) backed by a Prometheus text-format adapter; **no third-party Prometheus library** (no `prometheus/client_golang` dependency). Lock-free hot path via `atomic.{AddUint64, AddInt64, StoreInt64, LoadInt64}`; Walk-under-RLock for scrape; `RWMutex.Lock` held only at boot-time register sites. Rationale (per BRAINSTORM §2.1): future Observability-family phases (gRPC ALS, OTLP, statsd) all need to hook a registry, not a Prometheus client; investing in our own thin shape now is the same architectural choice Envoy made in `source/common/stats/`. Alternatives considered: (A) `prometheus/client_golang` directly — rejected for future-sink coupling (binds the in-process metric model to Prometheus's specific shape, blocking the gRPC ALS / OTLP / statsd sinks 06.x phases will land); (B) `expvar` + custom serializer — rejected because expvar lacks histogram support and the future histogram-bucket surface needs first-class typed-metric primitives. Consequences: (a) the `internal/stats` package's external dependencies are limited to the Go stdlib; (b) the LBP-1 invariant (this ADR's companion concept) makes the read path lock-free against hot-path increments; (c) future xDS-CDS phases that introduce dynamic cluster registration will need a copy-on-write list shape that supersedes LBP-1 — the ADR explicitly forward-flags this. Lands in Task 2 (the Registry itself). Supersedes nothing.

- **ADR-0060 — Histograms deferred from 06.1.** Status: Accepted. Date: task-execution date. Doctrine: D-3.6 (every phase is a green build — and the histogram surface is hard enough to warrant its own brainstorm pass) + D-3.4 (record durable design rationale where context-isolation requires it). Decision: 06.1 emits counters + gauges only; histograms (`upstream_rq_time`, `downstream_rq_time`, response/request size distributions) are deferred to a later sub-phase with their own brainstorm covering circllhist→Prometheus bucket mapping. Rationale (per BRAINSTORM §2.2): Envoy uses circllhist (dynamic-bucket) internally and bridges to Prometheus's fixed-bucket shape via `histogram_bucket_settings`; byte-equivalent matching against the v1.37.2 reference is hard and wants its own design pass. Bundling histograms into 06.1 would bloat the phase and leave a half-baked histogram model in tree. Carry-forward: 06.x or upstream-robustness family with own brainstorm. The deferral co-defers `server.uptime` (which depends on monotonic-clock + per-scrape recompute and pairs naturally with the histogram brainstorm). Consequences: (a) Rule SN7 of the SN1–SN8 set (per ADR-0061) reads "Histograms are not emitted by 06.1 (forward-looking)"; (b) the 17-name catalog in SPEC §6 is exhaustive; (c) the future histogram-introducing sub-phase supersedes this ADR (not by overriding the deferral but by closing it — the ADR's Status flips to "Superseded by ADR-NNNN" at that phase's writing). Lands in Task 2 (alongside ADR-0059; the ADR documents the deferral at Registry-introduction time). Supersedes nothing.

- **ADR-0061 — Stat-name → Prometheus-name flattening rules SN1–SN8 (with empirically-pinned Rule SN4).** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale where context-isolation requires it; the eight rules are the contract that future stats-emitting code consumes). Decision: the eight rules enumerated in SPEC §10.1 govern the flattening from internal hierarchical-dotted names to Prometheus-format `name{label="value"}` lines. Rules SN1, SN2, SN3, SN5, SN6, SN7, SN8 are settled at brainstorm-close (BRAINSTORM §7.1); Rule SN4 is empirically pinned at SPEC-drafting time per BRAINSTORM §2.3.1 against reference Envoy v1.37.2's default tag-extractor regex. The verbatim scrape-output evidence and the regex-source citation are pasted into ADR-0061's Context section verbatim from SPEC §10.1's evidence block (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` matching ENVOY_TARGET.md; the `RESPONSE_CODE_CLASS` tag entry in `source/common/config/well_known_names.cc` at v1.37.2). Verified Rule SN4 form: trailing class digit STRIPPED from metric name (so `cluster.<n>.upstream_rq_2xx` flattens to `envoy_cluster_upstream_rq_xx`); label name `envoy_response_code_class`; label value the single class digit as a string (`"2"`, `"3"`, `"4"`, `"5"`). Counter-examples (NOT what Envoy emits): `envoy_cluster_upstream_rq_2xx{...}` (digit suffix preserved); `envoy_cluster_upstream_rq_xx{envoy_response_code_class="2xx",...}` (label value with literal "xx"); `envoy_cluster_upstream_rq{envoy_response_code_class="2",...}` (`_xx` stripped entirely). Consequences: (a) `internal/stats/name.go`'s `flattenToProm` MUST implement Rule SN4 in this exact verified form — Task 4 step 3 codes the regex `^(.+)_([1-5])xx$` and emits the captured base + `_xx` suffix, with the digit as label value; (b) `BEHAVIOR_CONTRACT.md ## Stat-name mapping`'s in-place edit at Task 15 carries Rule SN4 in the same form as SPEC §10.1 (no drift between SPEC §10.1 and the contract addition); (c) future phases adding new stat-name patterns extend SN1–SN8 with append-only rules (SN9, SN10, ...). Lands in Task 4 (the `name.go` flattening logic; first use of all eight rules in production code). Supersedes nothing.

- **ADR-0062 — Differential equivalence shape for stats.** Status: Accepted. Date: task-execution date. Doctrine: D-3.3 (own the canonical observation surface; the equivalence claim lives here) + D-3.6 (every phase is a green build — and the differential gate (a) lands non-vacuous on 06.1's observability surface). Decision: per-stat behavioral-delta equivalence (not byte-exact whole-output); per-counter `delta_envoy_go == delta_envoy`, per-gauge `after_envoy_go == after_envoy`; HELP text ignored; non-listed Prometheus names ignored (per Rule SN6 + SPEC §7.4 allow-list discipline). Rationale (per BRAINSTORM §2.5): byte-exact full-output is fragile under minor-version Envoy bumps (a v1.37.3 bump that adds a new stat name would break the fixture without indicating any envoy-go regression); pure delta-only is permissive but constrained by the 17-name schema + unit-test name-validity coverage; layered byte-exact-schema is marginal protection over what unit tests already provide. Consequences: (a) fixture 0005's driver implements the allow-list in-band — parses the full Prometheus exposition, extracts the entries matching the 17 names, drops the rest; (b) the runner asserts only on the 17; (c) `BEHAVIOR_CONTRACT.md ## Equivalence Matrix`'s row 19 seed-row (`Names match Envoy's documented stat tree; presence required; values exact on deterministic flows`) is superseded by the new "Stats output" row that SPEC §10.2 prescribes; (d) future fixtures (06.x, 07, etc.) that exercise additional stat names extend the allow-list, NOT the equivalence claim. Lands in Task 14 (the fixture-0005 driver — first use of the per-counter-delta-equality + per-gauge-snapshot-equality assertion shape). Supersedes nothing (the seed-row supersession is a CONTRACT supersession, not an ADR supersession; the ADR establishes the shape).

- **ADR-0063 — Per-endpoint cluster stats not emitted.** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale where context-isolation requires it). Decision: 06.1 emits cluster-level metrics only (the 8 names in SPEC §6 above); per-endpoint expansion (`cluster.<n>.<endpoint_address>.cx_total`, `cluster.<n>.<endpoint_address>.rq_total`, etc. — equivalent of Envoy's `enable_per_endpoint_stats=true` mode) is deferred. Rationale (per BRAINSTORM §2.3, §9): per-endpoint expansion is dynamic in shape (endpoint set churns under xDS-EDS); statically-allocated per-endpoint metrics break LBP-1 (the post-Freeze panic discipline assumes the metric list is fixed at boot — endpoint churn would require dynamic registration); properly handling the dynamic-shape case wants xDS-EDS semantics. Carry-forward: xDS-EDS phase revisits with a copy-on-write list shape that supersedes both LBP-1 and ADR-0063. Consequences: (a) Rule SN8 of the SN1–SN8 set reads "Per-endpoint cluster stats are not emitted by 06.1 (forward-looking)"; (b) the cluster-side metric-allocation loop in Task 8 allocates exactly 8 metrics per cluster (not 8×N for N endpoints); (c) the fixture-0005 expectations table includes `cluster.c0.membership_total: 1` (the per-cluster gauge Set to len(endpoints)), but no per-endpoint rows. Lands in Task 8 (the cluster-side metric-allocation loop in `internal/cluster/manager.go`; first use of the cluster-level-only metric set). Supersedes nothing.

- **ADR-0064 — `stats_config.stats_tags` config not honored; extraction hardcoded.** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale where context-isolation requires it; the silent-ignore is a design choice, not a bug). Decision: 06.1 hardcodes the stat-name → Prometheus-name extraction logic in `internal/stats/name.go` (per Rules SN1–SN5 of the SN1–SN8 set); the bootstrap proto's `stats_config.stats_tags[]` field is silently ignored if present. Rationale (per BRAINSTORM §2.3, §5.6): the regex-driven tag-extraction surface in Envoy is complex (~50 default regexes plus user-supplied overrides) and warrants its own phase; 06.1 ships fixed extraction that matches Envoy's default tag-extractor behavior on the 17 names. The silent-ignore preserves bootstrap forward-compat — a fixture-0005 reference bootstrap with `stats_tags: []` round-trips without error. Carry-forward: future stats-config phase or an xDS-RTDS revisit. Consequences: (a) the silently-ignored field set is amended (per SPEC §9 + the 04 / 05.1 / 05.2 amendment pattern): `stats_config.stats_tags[]`, `stats_config.stats_matcher`, `stats_config.histogram_bucket_settings`, `stats_config.use_all_default_tags`, `stats_sinks[]`, HCM `stats_flush_interval`, Cluster `track_cluster_stats`, Listener `stat_prefix`; (b) the original silent-ignore ADR (ADR-N from phase 04) is amended (not superseded) per the 05.1 + 05.2 amendment shape — a single appended sub-section under that ADR's Consequences listing the newly-ignored fields; (c) future stats-config phases land their own ADR superseding ADR-0064 when honoring the proto fields becomes the contract. Lands in Task 4 (alongside ADR-0061; the `name.go` introduction is the load-bearing surface that "hardcodes" the extraction). Supersedes nothing.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0065+) in the same commit as the code it decides for. If such a decision would expand phase-06.1 scope beyond SPEC §1–§10, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6 — noting that 06.1 SPEC §1 #9's anticipated 12–16 task range is the natural axis for re-scoping (preserve the task count by absorbing the new ADR's anchoring task into an existing task; defer if the absorption would push the absorbing task past the §6.1 secondary-trigger of ~10 sub-steps).

---

## Settled SPEC §12 deferred decisions

SPEC §12 leaves eight 06.1-scoped implementation-detail choices to the planner. This PLAN settles them so the executor does not re-litigate. Only decisions with cross-phase impact are also captured as ADRs.

1. **HCM `downstream_rq_total` Inc hook location.** **Decision: site (a) — on first byte of request line/headers in `connection.go`'s read loop (per SPEC §12 #1's recommendation).** Rationale: site (a) counts requests that fail before route-match while site (b) counts only routed requests. Envoy counts at (a)-equivalent. The phase-04 HCM read loop has a clear "first byte of request line" hook (`ReadRequest` entry); Task 10 step 3 codes `f.downstreamRqTotal.Inc()` at that site. Codified in Task 10; not separately ADRd (mechanical correctness rule with no cross-phase impact).

2. **`Bootstrap` struct gains `.Stats *stats.Registry` field, or `main.go` allocates the Registry separately.** **Decision: field-on-Bootstrap shape (per SPEC §12 #2's recommendation).** Rationale: future xDS phases that add dynamic config-reload have a place to thread the Registry through a config-update path. Codified in Task 7 (`internal/bootstrap/bootstrap.go` extension). The `Bootstrap` struct's existing `Listeners` / `Clusters` / `Admin` / etc. accessor fields gain a `Stats *stats.Registry` sibling; `bootstrap.Load()` allocates `stats.NewRegistry()` and assigns it. Not separately ADRd (boot-wiring shape with no cross-phase impact beyond the next xDS phase).

3. **`server.live` set-once mechanism.** **Decision: `sync.Once` inside `handleReady` (per SPEC §12 #3's recommendation).** Rationale: defensive against future refactors that might Set(0) between transitions; cheap; well-understood. All three options (`sync.Once`, CAS-on-zero, unconditional `Set(1)` on every `/ready` 200) converge to the same exposition value, but `sync.Once` is the cleanest expression of intent. Codified in Task 6 step 3 (the `admin.New` constructor allocates the `server.live` gauge; `handleReady` consumes a `sync.Once` field on `*Server` to call `gauge.Set(1)` exactly once when the gate first flips ready). Not separately ADRd (defensive-coding choice with no cross-phase impact).

4. **HCM factory closure's per-instance metric alloc timing.** **Decision: filter-build inside `listener.NewManagerWithBaseDir...(...)` is pre-Freeze.** Rationale: the listener manager's `New(...)` finishes synchronously before `admin.Start()` returns to `main.go` and before `bs.Stats.Freeze()` is called; per-HCM filter chains are constructed inside `listener.NewManager...`'s loop. Verified at PLAN-write time by inspecting `cmd/envoy-go/main.go`'s linear boot sequence: line 52 `cluster.NewManagerWithBaseDir` → line 57 `admin.New` → line 58 `admSrv.Start()` (admin starts accepting) → line 63 `listener.NewManagerWithBaseDirAndAllowH2C` (per-HCM metric alloc happens HERE) → line 71 `lm.Start(ctx)` (listener manager begins accepting). Task 12 step 3 inserts `bs.Stats.Freeze()` AFTER line 71's `lm.Start(ctx)` and AFTER `admSrv.MarkReady()` so all NewCounter/NewGauge calls precede the freeze. The PR-relative ordering is: `cluster.NewManagerWithBaseDir` allocates 8 cluster-scope metrics per cluster; `admin.New` allocates the `server.live` gauge; `listener.NewManagerWithBaseDirAndAllowH2C` allocates 2 listener-scope metrics per listener AND triggers per-HCM metric allocation via the closure (5 HCM-scope metrics per HCM); after all three, `Freeze()` lands. Not separately ADRd (boot-order verification with no cross-phase impact).

5. **M-9 unit test file location.** **Decision: new file `internal/filter/hcm/h2/router_action_test.go` (per SPEC §12 #5's recommendation).** Rationale: clean separation; the carry-forward concern is its own surface; the file is grep-locatable independently of the existing `router_action.go` test coverage (which is sparse — Task 11 codes the M-9 test in this new file). Codified in Task 11; not separately ADRd (test-file-location choice with no cross-phase impact).

6. **Fixture-0005 driver pattern: in-band assertions vs. generic `StatsExpectations` Driver-interface extension.** **Decision: in-band (per SPEC §12 #6's recommendation, mirroring the 05.2 SPEC §10 #3 "in-band" recommendation for fixture-0004).** Rationale: smaller harness surface; matches the 05.2 precedent; the per-fixture assertion pattern is established. The driver's `DriveSubject(ctx, addr)` and `DriveReference(ctx, addr)` return `(beforeSnapshot, afterSnapshot)` tuples; the runner's per-fixture loop calls a fixture-specific `AssertStatsEquivalence(t, subject, reference)` helper exported from the fixture-0005 driver package. Codified in Task 14; not separately ADRd (harness-shape choice with no cross-phase impact beyond fixture conventions).

7. **Concrete ADR numbers for ADR-0059..ADR-0064.** Per SPEC §12 #7's deferred decision: the planner re-verifies next-free at write time. **Verified at PLAN-write time:** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0058:` at the `be99b42` baseline (the SPEC commit + SHA-fill). Phase 06.1's six ADRs land at ADR-0059..ADR-0064. The mapping is documented in `## ADRs introduced by this plan` above; the executor re-verifies at Task 1 step 1 in case a mid-PLAN-authoring or pre-implementation ADR has landed since this PLAN was written.

8. **`server.uptime` future-emission.** **Decision: NOT pre-decided; co-deferred with histograms per ADR-0060.** Per SPEC §12 #8's recommendation. Rationale: the future phase that lands histograms (per ADR-0060) is the natural home for `server.uptime` because both depend on monotonic-clock + per-scrape recompute. ADR-0060's Consequences section explicitly flags `server.uptime` as a co-deferred item. Codified in this PLAN's `## ADRs introduced by this plan` section (the ADR-0060 summary explicitly mentions the co-deferral); not separately ADRd (sub-decision of ADR-0060).

Three additional 06.1-internal implementation choices (not in SPEC §12 but settled here so the executor doesn't re-litigate):

9. **Panic-message wording for the LBP-1 violation.** **Decision: `stats: registry frozen: cannot register %q post-boot`** (the exact form in SPEC §11.6). Rationale: grep-verifiable in `registry.go`; matches the 05.1 / 05.2 panic-message convention of `<package>: <subsystem>: <reason>`. Codified in Task 2 step 3 (the `Registry.NewCounter` and `NewGauge` panic site uses `fmt.Sprintf("stats: registry frozen: cannot register %q post-boot", name)` and panics with the formatted string). Not separately ADRd (mechanical wording choice).

10. **`connWithGauge` wrapper file location.** **Decision: defined in `internal/cluster/cluster.go` adjacent to `Cluster.Dial` (NOT a new `dial.go` file).** Rationale: the 06.1 SPEC §6 inventory mentions `internal/cluster/dial.go` as a target, but the actual codebase has `Cluster.Dial` defined in `cluster.go` line 84 (no `dial.go` file exists). The wrapper is small (~25 LoC) and naturally lives next to its consumer. Task 9 step 3 codes the wrapper in `cluster.go` immediately after `Dial`'s definition. Not separately ADRd (file-organization choice).

11. **`FuzzPromTextFormat` fuzzer scope.** **Decision: fuzz adversarial stat-name strings + adversarial label-value strings into `WriteProm`; assert no panic; assert the output round-trips through a Prometheus-format-aware parser without error.** Rationale: per BRAINSTORM §6.4, escaping bugs are the most likely class of bug in the writer, and Prometheus's parser is strict. The fuzzer's seed corpus includes adversarial entries: newlines in label values, backslashes, double-quotes, NUL bytes, trailing-equals signs, very long names, names violating the regex (the fuzzer asserts `WriteProm` does NOT panic; the panic-on-invalid-name discipline lives at `Registry.NewCounter` time, not at write time, since the Registry guards entry — so the fuzzer constructs a Registry with pre-validated names and adversarial label values, NOT adversarial stat names directly). The Prometheus parser used for round-trip is a tiny in-test parser (~20 LoC) that splits lines, parses `name{labels} value` triples, asserts well-formedness — NO third-party Prometheus library import (consistent with ADR-0059 / D-3.2). Codified in Task 13; not separately ADRd (fuzzer-scope choice — per ADR-0042 precedent that fuzzers do not require their own ADR).

---

## Phase-05.2 REVIEW carry-forward resolution matrix

SPEC §13 + ADR-0058 (from 05.2) triage the four phase-05.2 REVIEW Minor findings (M-4 / M-9 / M-10 / M-12). Phase-06.1 disposition matrix:

| Phase-05.2 finding | Triage | Landing task / rationale |
|---|---|---|
| M-4 (`readClientPreface` not ctx-aware in `internal/filter/hcm/h2/conn.go`) | DEFERRED — out of 06.1 | Bundled into ADR-0058's carry-forward subsection from 05.2; H2 connection hardening, not observability. The proper fix is at the listener-manager level via uniform OS read deadlines; target-phase candidates: dedicated H2-hardening sub-phase / phase 07 / upstream-robustness family. Phase 06.1 does NOT touch `preface.go`. |
| **M-9** (Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error path) | **RESOLVED-IN-06.1** | **Task 11.** ~5 LoC fix + ~20 LoC unit test. The 05.2 REVIEW explicitly deferred this to "phase-06 observability when logging/metrics surface lands"; bundled with 06.1 because the surface (`log.Printf` to stderr) matches what 06.1 introduces. |
| M-10 (`SETTINGS_TIMEOUT` absent in `internal/filter/hcm/h2/client.go`) | DEFERRED — out of 06.1 | Bundled into ADR-0058's carry-forward subsection from 05.2; same reasoning as M-4. RFC 9113 §6.5.3's "MAY" leaves this optional; h2spec sends SETTINGS_ACK promptly so the gap is dormant. Target-phase candidates: same as M-4 (dedicated H2-hardening or phase 08). |
| M-12 (`closedStreams` map unbounded in `internal/filter/hcm/h2/conn.go`) | DEFERRED — out of 06.1 | Long-lived-conn memory growth is a hardening concern, not an observability one. Under 06.1's per-request-fresh-upstream-conn discipline (inherited from ADR-0056), the map's growth is bounded per-conn-lifetime; the issue surfaces only when conn pooling lands. Target-phase candidate: upstream-robustness family (specifically, the H2 conn-pooling sub-phase that supersedes ADR-0056). |

The disposition table is faithful to SPEC §13's per-finding triage; the §13.2 deferred items do NOT land an ADR in 06.1 because the deferral itself is the SPEC's record (per ADR-0017 doctrine that "small mechanical fixes do not require ADRs"). The PROGRESS Task 11 entry records the M-9 carry-forward landing alongside the standard task entry; the PROGRESS Task 15 entry records M-4 / M-10 / M-12 as continuing-deferred.

---

## Spec-review advisory responses

The SPEC's brainstorming session ran the `spec-document-reviewer` subagent loop and reached APPROVED. The SPEC at `be99b42` carries no outstanding advisory items at PLAN-write time. Three planner-time advisory items, structurally akin to the 05.2 PLAN's "spec-review advisory responses" but originating from the planner's reading of the SPEC during PLAN authoring:

i. **The `admin.New` signature change** — current code is `admin.New(addr string) *Server`, the SPEC §6 file inventory mentions `admin.New(ready *Ready, registry *stats.Registry) *Admin` as the target. The actual signature change in this PLAN is `admin.New(addr string, registry *stats.Registry) *Server` — preserves the existing `addr` parameter (no `*Ready` parameter exists in the current code; `MarkReady` is a method on `*Server`), adds `*stats.Registry`. The SPEC's `*Ready` reference is a SPEC-time abstraction (the implicit "ready gate" the `MarkReady` method controls); the actual struct is `atomic.Bool` inside `*Server`. The PLAN preserves this shape. Recorded here so the executor doesn't re-litigate at Task 6 execution time.

ii. **The `connWithGauge` wrapper file location** — SPEC §6's file inventory mentions `internal/cluster/dial.go` as a target, but the actual codebase has `Dial` in `cluster.go` (no `dial.go` file). The PLAN places `connWithGauge` in `cluster.go` per `## Settled SPEC §12 deferred decisions` #10. Recorded here so a reviewer reading the PLAN doesn't flag the SPEC-vs-PLAN file divergence.

iii. **The `internal/filter/hcm/h2/router_action.go` M-9 site** — SPEC §1 #6 + §13.1 mention `h2RouterActionAdapter.WriteH2` as the M-9 fix site; the planner verified at PLAN-write time that the symbol exists in the worktree (Task 11 step 1's grep `grep -nE 'h2RouterActionAdapter' internal/filter/hcm/h2/router_action.go` returns matches). If the symbol was renamed in a mid-PLAN-authoring 05.2-follow-up commit, Task 11 step 1's grep will show the renamed symbol; the executor adapts the patch site accordingly without re-litigating the M-9 fix's intent.

The planner re-verified at PLAN-write time that the SPEC at `be99b42` does not contain any internal contradiction. The `be99b42` SPEC is internally consistent: §1's seventh-claim-first-half formulation aligns with §6's 17-name catalog; §10.1's Rule SN4 verbatim evidence aligns with §6's "Verify Prometheus name" cells; §11.6's LBP-1 enforcement test description aligns with §5.3's invariant text; §12's deferred-decisions enumeration is exhaustive against the SPEC body.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/06.1-stats-prometheus-plan` (this plan's authoring branch) and NOT `phase/06.1-stats-prometheus-spec` (the SPEC's authoring branch). Recommended: `.worktrees/phase-06.1-stats-prometheus-impl` on branch `phase/06.1-stats-prometheus-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for fixture 0005's reference (Envoy in a testcontainer) AND for the unchanged 05.1 conformance gate (c) re-run during Task 15's local sweep.
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`).
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-05.2 gate (e) still holds at the 05.2 phase-done commit's tail). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. `go list -m github.com/envoyproxy/go-control-plane/envoy` resolves to `v1.32.4` (ADR-0013). If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 06.1 must not silently re-pin.
7. `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0058:` (or later if a mid-phase ADR has landed since this PLAN was written). If the tail is `ADR-0058`, the phase-06.1 ADRs are assigned 0059..0064 as in this PLAN. If higher, re-number phase-06.1 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1.
8. The phase-06.1 SPEC at `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` is at commit `be99b42` (verify with `git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md`). If the SPEC has been amended since `be99b42`, invoke `superpowers:systematic-debugging` on the divergence — the PLAN was authored against `be99b42` and silent SPEC drift voids the PLAN's traceability.
9. Phase-05.2 close is present in HEAD: `git log --oneline -- docs/envoy-go/phases/05.2-upstream-h2/REVIEW.md` shows the close commit. If missing, invoke `superpowers:systematic-debugging` on the gap.
10. The phase-00 `internal/stats/` placeholder is intact: `ls internal/stats/` returns exactly `doc.go` (the phase-00 placeholder; Task 2 rewrites it). If ANY other file is present (e.g. `registry.go` from a half-merged earlier attempt), invoke `superpowers:systematic-debugging` — 06.1's surface is being introduced incrementally and a pre-existing file voids the PLAN's traceability.
11. The current `internal/admin/admin.go` carries the `func New(addr string) *Server` signature (verify with `grep -nE 'func New\(' internal/admin/admin.go` → expect one match returning `func New(addr string) *Server`). If the signature has drifted, invoke `superpowers:systematic-debugging`.
12. The current `internal/cluster/manager.go` carries `func NewManager(bs *bootstrapv3.Bootstrap)` and `func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string)` signatures. The current `internal/listener/manager.go` carries `func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager)`, `func NewManagerWithBaseDir(...)`, `func NewManagerWithBaseDirAndAllowH2C(...)`. Verify with `grep -nE 'func New' internal/cluster/manager.go internal/listener/manager.go`. If signatures have drifted, invoke `superpowers:systematic-debugging`.
13. The `BEHAVIOR_CONTRACT.md ## Stat-name mapping` placeholder is intact (the empty-with-prose-note shape from phase 00): `grep -nE '^## Stat-name mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns at least one match; the immediately-following lines contain the `_to be filled per-phase as needed._` italic text. If the placeholder has been edited since master `be99b42`, invoke `superpowers:systematic-debugging` — the in-place edit at Task 15 must start from the placeholder shape.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/06.1-stats-prometheus-impl
git log -1 --format=%H                                                # expect: same SHA as docs/envoy-go/STATE.md last-commit field (the PLAN.md commit)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test ./...                                                         # expect: every package PASS (no FAIL, no compile error)
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1                  # expect: ## ADR-0058:
git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md
                                                                       # expect: be99b42 (or the documented SPEC commit; if newer, follow precondition 8 guidance)
git log --oneline -- docs/envoy-go/phases/05.2-upstream-h2/REVIEW.md | head -5
                                                                       # expect: at least one commit visible (the 05.2 REVIEW close)
ls internal/stats/                                                    # expect: only doc.go
grep -nE 'func New\(' internal/admin/admin.go                         # expect: line 22 matches `func New(addr string) *Server`
grep -nE 'func New' internal/cluster/manager.go internal/listener/manager.go
                                                                       # expect: cluster:NewManager(bs)+NewManagerWithBaseDir(bs,baseDir);
                                                                       #         listener:NewManager(bs,cm)+NewManagerWithBaseDir(...)+NewManagerWithBaseDirAndAllowH2C(...)
grep -nE '^## Stat-name mapping$' docs/envoy-go/BEHAVIOR_CONTRACT.md   # expect: exactly 1 match
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md`**

```markdown
# Phase 06.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-05.2 close confirmed present in HEAD; SPEC at be99b42; ADR tail at 0058 (next-free 0059); internal/stats/ contains only doc.go (registry.go etc. land at Task 2).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ golangci-lint version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md
<verbatim>
$ ls internal/stats/
<verbatim — should report 'doc.go' only>
\`\`\`
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (phase-02/03/04/05.1/05.2 precedent: a follow-up tiny commit `phase 06.1: PROGRESS SHA-fill for Task 1` lands the SHA).

*Anchored: SPEC §3, §4.4 (PROGRESS lifecycle), §14 (precondition acceptance bullet).*

---

## Task 2: `internal/stats/registry.go` + `counter.go` + LBP-1 enforcement [ADR-0059, ADR-0060]

**Files:**
- Modify: `internal/stats/doc.go` (rewrite from phase-00 stub)
- Create: `internal/stats/registry.go`
- Create: `internal/stats/registry_test.go`
- Create: `internal/stats/counter.go`
- Create: `internal/stats/counter_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0059 + ADR-0060)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 2 entry)

The Registry + Counter pair is the foundational primitive every subsequent task consumes. ADR-0059 (Internal Stats Store architecture) and ADR-0060 (Histograms deferred from 06.1) co-land at this task per the topical-co-landing decision in `## ADRs introduced by this plan` above.

- [ ] **Step 1: Write the failing test for Registry happy path + LBP-1 enforcement (in `registry_test.go`)**

```go
package stats

import (
	"strings"
	"sync"
	"testing"
)

func TestRegistry_NewCounter_HappyPath(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	if c == nil {
		t.Fatal("NewCounter returned nil")
	}
	if c.Name() != "listener.0_0_0_0_10000.downstream_cx_total" {
		t.Errorf("Counter.Name() = %q, want listener.0_0_0_0_10000.downstream_cx_total", c.Name())
	}
	if c.Load() != 0 {
		t.Errorf("freshly-allocated Counter.Load() = %d, want 0", c.Load())
	}
}

func TestRegistry_NewCounter_DuplicateNamePanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("foo")
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic on duplicate-name registration; got nil")
		}
		s, _ := got.(string)
		if !strings.Contains(s, "duplicate") {
			t.Errorf("panic message = %q, want substring 'duplicate'", s)
		}
	}()
	r.NewCounter("foo")
}

func TestRegistry_NewCounter_InvalidNamePanics(t *testing.T) {
	cases := []string{"", "1leading-digit", "with space", "with-dash", "trailing.", "with$char"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewRegistry()
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic on invalid name %q", name)
				}
			}()
			r.NewCounter(name)
		})
	}
}

func TestRegistry_Walk_RegistrationOrderInvariantNotPromised(t *testing.T) {
	r := NewRegistry()
	a := r.NewCounter("a")
	b := r.NewCounter("b")
	a.Inc()
	b.Inc()
	b.Inc()
	var seen []string
	r.Walk(func(m Metric) {
		seen = append(seen, m.Name())
	})
	if len(seen) != 2 {
		t.Fatalf("Walk visited %d metrics, want 2", len(seen))
	}
}

func TestRegistry_Freeze_PostFreezeRegisterPanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("pre.freeze")
	r.Freeze()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic on post-freeze NewCounter; got nil")
		}
		s, _ := got.(string)
		if !strings.Contains(s, "registry frozen: cannot register") {
			t.Errorf("panic message = %q, want substring 'registry frozen: cannot register'", s)
		}
	}()
	r.NewCounter("post.freeze")
}

func TestRegistry_Freeze_PostFreezeNewGaugePanics(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on post-freeze NewGauge; got nil")
		}
	}()
	r.NewGauge("post.freeze.gauge")
}

func TestRegistry_Freeze_Idempotent(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	r.Freeze() // must not panic, must not race
}

func TestRegistry_Walk_ConcurrentWithIncs_RaceClean(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("conc.counter")
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Inc()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.Walk(func(m Metric) { _ = m.Name() })
			}
		}()
	}
	wg.Wait()
	if got := c.Load(); got != 8000 {
		t.Errorf("Counter.Load() after concurrent Incs = %d, want 8000", got)
	}
}
```

Run: `go test -race ./internal/stats/ -v`
Expected: FAIL — `NewRegistry`, `NewCounter`, `Counter.Inc`, `Counter.Load`, `Counter.Name`, `Registry.Walk`, `Registry.Freeze`, `Registry.NewGauge` all undefined.

- [ ] **Step 2: Write the failing test for Counter atomicity (in `counter_test.go`)**

```go
package stats

import (
	"sync"
	"testing"
)

func TestCounter_Inc_Sequential(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("c.seq")
	if got := c.Load(); got != 0 {
		t.Fatalf("initial Load = %d, want 0", got)
	}
	c.Inc()
	c.Inc()
	c.Inc()
	if got := c.Load(); got != 3 {
		t.Errorf("Load after 3 Incs = %d, want 3", got)
	}
}

func TestCounter_Add_Sequential(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("c.add")
	c.Add(7)
	c.Add(13)
	if got := c.Load(); got != 20 {
		t.Errorf("Load after Add(7)+Add(13) = %d, want 20", got)
	}
}

func TestCounter_Inc_RaceClean(t *testing.T) {
	const N = 8
	const M = 10000
	r := NewRegistry()
	c := r.NewCounter("c.race")
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if got := c.Load(); got != N*M {
		t.Errorf("Load after %d×%d concurrent Incs = %d, want %d", N, M, got, N*M)
	}
}
```

Run: `go test -race ./internal/stats/ -run TestCounter_ -v`
Expected: FAIL — same undefined-symbol failures (the same Counter type satisfies both test files).

- [ ] **Step 3: Implement `registry.go` + `counter.go`**

`internal/stats/registry.go`:

```go
// Package stats is envoy-go's in-tree atomic-counter Registry. Per ADR-0059
// the package owns the canonical observation surface; no third-party
// Prometheus library is consumed at runtime. Per ADR-0060 histograms are
// deferred to a later sub-phase. The LBP-1 invariant ("list before play")
// makes the Walk-under-RLock-plus-atomic-Load read path lock-free against
// hot-path increments — see registry.go's Freeze documentation.
package stats

import (
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
)

// MetricType enumerates the supported metric primitives at phase 06.1.
// Histogram is reserved per ADR-0060 and not registered.
type MetricType int

const (
	MetricCounter MetricType = iota + 1
	MetricGauge
)

// Metric is the Walk-callback's view of a registered metric. Counter and Gauge
// both satisfy it; the Prometheus writer (prom.go) consumes Type to choose
// "counter" vs "gauge" in the # TYPE line.
type Metric interface {
	Name() string
	Type() MetricType
	// Format returns the metric's current value formatted as a Prometheus
	// metric-line value (the integer or non-negative integer text after the
	// labels block). Negative gauge values are permitted and rendered with a
	// minus sign per the Prometheus exposition spec.
	Format() string
}

// nameRE is the validation regex applied to every NewCounter / NewGauge name.
// Per BRAINSTORM §5.2 the form is ASCII-letter-or-underscore prefix followed
// by ASCII-alphanumerics, underscores, and dots. Dots are permitted because
// the internal hierarchical-dotted-name shape uses them as the segment separator.
var nameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

// Registry holds every metric registered at boot. The list of metrics is
// mutable only during boot; once Freeze is called, NewCounter / NewGauge
// panic. This is the LBP-1 invariant — see ADR-0059 Consequences (a) + (b).
type Registry struct {
	mu      sync.RWMutex
	metrics []Metric
	byName  map[string]Metric
	frozen  atomic.Bool
}

// NewRegistry returns a fresh registry with no metrics registered.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Metric)}
}

// NewCounter registers and returns a counter under the given hierarchical-
// dotted name. Panics if frozen, on invalid name (per nameRE), or on
// duplicate registration. The returned Counter is safe for concurrent Inc.
func (r *Registry) NewCounter(name string) *Counter {
	r.checkRegister(name)
	c := &Counter{name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		panic(fmt.Sprintf("stats: duplicate metric registration: %q", name))
	}
	r.metrics = append(r.metrics, c)
	r.byName[name] = c
	return c
}

// NewGauge registers and returns a gauge under the given name. Same panic
// discipline as NewCounter.
func (r *Registry) NewGauge(name string) *Gauge {
	r.checkRegister(name)
	g := &Gauge{name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		panic(fmt.Sprintf("stats: duplicate metric registration: %q", name))
	}
	r.metrics = append(r.metrics, g)
	r.byName[name] = g
	return g
}

// checkRegister panics if the registry is frozen or the name fails validation.
// Called from NewCounter and NewGauge before they take r.mu.Lock.
func (r *Registry) checkRegister(name string) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("stats: registry frozen: cannot register %q post-boot", name))
	}
	if !nameRE.MatchString(name) {
		panic(fmt.Sprintf("stats: invalid metric name: %q (must match %s)", name, nameRE.String()))
	}
}

// Walk invokes fn for each registered metric in registration order. The
// ordering is NOT part of the contract; the Prometheus writer (prom.go)
// sorts post-walk. Walk holds r.mu RLock for the duration of the iteration;
// concurrent Walks are permitted; concurrent NewCounter/NewGauge are NOT
// (Freeze is the discipline that prevents them).
func (r *Registry) Walk(fn func(Metric)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.metrics {
		fn(m)
	}
}

// Freeze locks the metric list. Subsequent NewCounter / NewGauge calls panic.
// Idempotent; safe for concurrent calls.
func (r *Registry) Freeze() { r.frozen.Store(true) }
```

`internal/stats/counter.go`:

```go
package stats

import (
	"strconv"
	"sync/atomic"
)

// Counter is a monotonic non-negative integer metric. The hot-path Inc and
// Add are lock-free atomics; the Walk-callback's Format reads atomically.
type Counter struct {
	name string
	v    atomic.Uint64
}

// Name returns the registered hierarchical-dotted name.
func (c *Counter) Name() string { return c.name }

// Type returns MetricCounter.
func (c *Counter) Type() MetricType { return MetricCounter }

// Inc atomically increments by 1.
func (c *Counter) Inc() { c.v.Add(1) }

// Add atomically adds delta. Caller is responsible for delta >= 0
// (the type signature uses uint64 to encode the non-negativity at compile
// time; underflow is impossible).
func (c *Counter) Add(delta uint64) { c.v.Add(delta) }

// Load returns the current value.
func (c *Counter) Load() uint64 { return c.v.Load() }

// Format implements Metric: the Prometheus value text.
func (c *Counter) Format() string { return strconv.FormatUint(c.v.Load(), 10) }
```

Rewrite `internal/stats/doc.go`:

```go
// Package stats is envoy-go's in-tree atomic-counter Registry plus the
// Prometheus text-format writer that walks the registry on /stats/prometheus
// requests. Per ADR-0059 the package owns the canonical observation surface;
// no third-party Prometheus library is consumed at runtime. Per ADR-0060
// histograms are deferred to a later sub-phase. Per ADR-0061 the
// hierarchical-dotted-name flattening rules SN1–SN8 govern the
// internal-name → Prometheus-name mapping (see name.go and BEHAVIOR_CONTRACT.md
// §Stat-name mapping). Per ADR-0064 stats_config.stats_tags is hardcoded;
// the bootstrap proto's stats_config.stats_tags[] field is silently ignored.
//
// The LBP-1 invariant ("list before play") — registry.Freeze() is called
// from cmd/envoy-go/main.go after admin server starts accepting and before
// listener manager begins accepting connections; post-Freeze NewCounter /
// NewGauge calls panic with "stats: registry frozen: cannot register %q
// post-boot". This is what makes the Walk-under-RLock-plus-atomic-Load
// read path lock-free against hot-path increments.
//
// API surface:
//   - NewRegistry() *Registry                 -- boot-time registry alloc
//   - (*Registry).NewCounter(name) *Counter   -- boot-time counter registration
//   - (*Registry).NewGauge(name) *Gauge       -- boot-time gauge registration
//   - (*Registry).Walk(fn func(Metric))       -- scrape-time iteration
//   - (*Registry).Freeze()                    -- LBP-1 boot-end seal
//   - WriteProm(w io.Writer, r *Registry)     -- Prometheus exposition writer
package stats
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test -race ./internal/stats/ -v
```

Expected: every TestRegistry_ + TestCounter_ test PASS. Race detector clean.

- [ ] **Step 5: Append ADR-0059 + ADR-0060 to `docs/envoy-go/DECISIONS.md`**

Append both ADRs verbatim from the `## ADRs introduced by this plan` summaries above. Format per ADR-0001's template (Status / Context / Decision / Consequences / Lands-in-task). Each ADR begins with `## ADR-0059: ...` / `## ADR-0060: ...` — the heading shape `^## ADR-NNNN: ` is the convention `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` consumes for next-free verification.

- [ ] **Step 6: Append Task 2 entry to PROGRESS.md**

Append the standard task entry per the phase-04 / 05.1 / 05.2 PROGRESS shape: `**Commits:**`, `**Notes:**`, `**Outputs:**` block quoting the `go test` output verbatim plus the `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` post-commit check (expect `## ADR-0060:`).

- [ ] **Step 7: Commit**

```bash
git add internal/stats/doc.go internal/stats/registry.go internal/stats/registry_test.go internal/stats/counter.go internal/stats/counter_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: stats Registry + Counter primitives + LBP-1 enforcement [ADR-0059, ADR-0060]"
```

After the commit, append a SHA-fill follow-up commit `phase 06.1: PROGRESS SHA-fill for Task 2` per the established convention.

*Anchored: SPEC §1 #1, §1 #5, §4.1 (`registry.go` / `counter.go` / `doc.go`), §5.2 (concurrency model), §5.3 (LBP-1), §8 (ADR-0059 + ADR-0060), §11.1 (registry_test + counter_test), §11.6 (LBP-1 enforcement test), §14 (acceptance bullets for the package + LBP-1 + no-third-party-Prom).*

---

## Task 3: `internal/stats/gauge.go`

**Files:**
- Create: `internal/stats/gauge.go`
- Create: `internal/stats/gauge_test.go`
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 3 entry)

The Gauge primitive parallels Counter but uses `atomic.Int64` so negative values are permitted (a `Dec` not paired with an `Inc` is defensive — gauge reflects reality per BRAINSTORM §5.2). No new ADR.

- [ ] **Step 1: Write the failing test (in `gauge_test.go`)**

```go
package stats

import (
	"sync"
	"testing"
)

func TestGauge_IncDecSet_Sequential(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.seq")
	if got := g.Load(); got != 0 {
		t.Fatalf("initial Load = %d, want 0", got)
	}
	g.Inc()
	g.Inc()
	g.Inc()
	if got := g.Load(); got != 3 {
		t.Errorf("Load after 3 Incs = %d, want 3", got)
	}
	g.Dec()
	if got := g.Load(); got != 2 {
		t.Errorf("Load after Dec = %d, want 2", got)
	}
	g.Set(100)
	if got := g.Load(); got != 100 {
		t.Errorf("Load after Set(100) = %d, want 100", got)
	}
}

func TestGauge_NegativeValueAllowed(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.neg")
	g.Dec()
	g.Dec()
	g.Dec()
	if got := g.Load(); got != -3 {
		t.Errorf("Load after 3 Decs (no Incs) = %d, want -3", got)
	}
	g.Set(-42)
	if got := g.Load(); got != -42 {
		t.Errorf("Load after Set(-42) = %d, want -42", got)
	}
}

func TestGauge_Add_PositiveAndNegative(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.add")
	g.Add(10)
	g.Add(-3)
	if got := g.Load(); got != 7 {
		t.Errorf("Load after Add(10)+Add(-3) = %d, want 7", got)
	}
}

func TestGauge_RaceClean_ConcurrentIncDecSet(t *testing.T) {
	const N = 8
	const M = 1000
	r := NewRegistry()
	g := r.NewGauge("g.race")
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				g.Inc()
				g.Dec()
			}
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				g.Set(int64(j))
			}
		}()
	}
	wg.Wait()
	// final value is non-deterministic (the Set goroutines race against each
	// other) but the race detector must report no data race.
	_ = g.Load()
}

func TestGauge_Format_NegativeRendered(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.fmt")
	g.Set(-5)
	if got := g.Format(); got != "-5" {
		t.Errorf("Format() = %q, want -5", got)
	}
	g.Set(42)
	if got := g.Format(); got != "42" {
		t.Errorf("Format() = %q, want 42", got)
	}
}
```

Run: `go test -race ./internal/stats/ -run TestGauge_ -v`
Expected: FAIL — `NewGauge`, `Gauge.{Inc,Dec,Set,Add,Load,Format}` undefined.

- [ ] **Step 2: Implement `gauge.go`**

```go
package stats

import (
	"strconv"
	"sync/atomic"
)

// Gauge is a signed-integer metric that may rise and fall. Negative values
// are permitted (a Dec not paired with an Inc is defensive — gauge reflects
// reality per BRAINSTORM §5.2). The hot-path Inc/Dec/Set/Add are lock-free
// atomics on int64.
type Gauge struct {
	name string
	v    atomic.Int64
}

// Name returns the registered hierarchical-dotted name.
func (g *Gauge) Name() string { return g.name }

// Type returns MetricGauge.
func (g *Gauge) Type() MetricType { return MetricGauge }

// Inc atomically adds 1.
func (g *Gauge) Inc() { g.v.Add(1) }

// Dec atomically subtracts 1.
func (g *Gauge) Dec() { g.v.Add(-1) }

// Add atomically adds delta. Negative delta is permitted.
func (g *Gauge) Add(delta int64) { g.v.Add(delta) }

// Set atomically replaces the current value.
func (g *Gauge) Set(v int64) { g.v.Store(v) }

// Load returns the current value.
func (g *Gauge) Load() int64 { return g.v.Load() }

// Format implements Metric: the Prometheus value text. Negative values are
// rendered with a minus sign per the Prometheus exposition spec.
func (g *Gauge) Format() string { return strconv.FormatInt(g.v.Load(), 10) }
```

- [ ] **Step 3: Run tests to verify pass**

```bash
go test -race ./internal/stats/ -v
```

Expected: every TestGauge_ test PASS plus the existing Task-2 tests still PASS.

- [ ] **Step 4: Append Task 3 entry to PROGRESS.md**

Standard task entry plus output of `go test -race ./internal/stats/ -v`.

- [ ] **Step 5: Commit**

```bash
git add internal/stats/gauge.go internal/stats/gauge_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: stats Gauge primitive (signed-int64; negative values permitted)"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §4.1 (`gauge.go`), §5.2, §11.1 (gauge_test).*

---

## Task 4: `internal/stats/name.go` flattening rules SN1–SN8 + helpText map [ADR-0061, ADR-0064]

**Files:**
- Create: `internal/stats/name.go`
- Create: `internal/stats/name_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0061 + ADR-0064)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 4 entry)

The `name.go` flattening logic is the load-bearing contract between internal hierarchical-dotted names and the Prometheus exposition. ADR-0061 (Stat-name → Prometheus-name flattening rules SN1–SN8 with empirically-pinned Rule SN4) and ADR-0064 (`stats_config.stats_tags` config not honored) co-land at this task per the topical-co-landing decision in `## ADRs introduced by this plan` above. Rule SN4's verbatim Envoy-scrape evidence is pasted into ADR-0061's Context section from SPEC §10.1.

- [ ] **Step 1: Write the failing test (in `name_test.go`)**

```go
package stats

import (
	"reflect"
	"testing"
)

func TestFlattenToProm_Listener(t *testing.T) {
	prom, labels, err := flattenToProm("listener.0_0_0_0_10000.downstream_cx_total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_listener_downstream_cx_total" {
		t.Errorf("promName = %q, want envoy_listener_downstream_cx_total", prom)
	}
	want := []Label{{Key: "envoy_listener_address", Value: "0_0_0_0_10000"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

func TestFlattenToProm_HCM(t *testing.T) {
	prom, labels, err := flattenToProm("http.ingress_http.downstream_rq_total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_http_downstream_rq_total" {
		t.Errorf("promName = %q, want envoy_http_downstream_rq_total", prom)
	}
	want := []Label{{Key: "envoy_http_conn_manager_prefix", Value: "ingress_http"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

func TestFlattenToProm_Cluster(t *testing.T) {
	prom, labels, err := flattenToProm("cluster.c0.upstream_cx_active")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_cluster_upstream_cx_active" {
		t.Errorf("promName = %q, want envoy_cluster_upstream_cx_active", prom)
	}
	want := []Label{{Key: "envoy_cluster_name", Value: "c0"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

// Rule SN4 — the empirical-verification gate per SPEC §10.1. The trailing
// class digit is STRIPPED from the metric name (so "_2xx" → base ending
// "_xx"); label name is "envoy_response_code_class"; label value is the
// single class digit as a string.
func TestFlattenToProm_StatusClass_HCM(t *testing.T) {
	prom, labels, err := flattenToProm("http.ingress_http.downstream_rq_2xx")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_http_downstream_rq_xx" {
		t.Errorf("promName = %q, want envoy_http_downstream_rq_xx (Rule SN4: digit stripped, base ends _xx)", prom)
	}
	wantSet := map[string]string{
		"envoy_response_code_class":      "2",
		"envoy_http_conn_manager_prefix": "ingress_http",
	}
	gotSet := make(map[string]string)
	for _, l := range labels {
		gotSet[l.Key] = l.Value
	}
	if !reflect.DeepEqual(wantSet, gotSet) {
		t.Errorf("labels = %+v, want %+v", gotSet, wantSet)
	}
}

func TestFlattenToProm_StatusClass_Cluster_AllDigits(t *testing.T) {
	for digit := 1; digit <= 5; digit++ {
		internal := "cluster.c0.upstream_rq_" + string(rune('0'+digit)) + "xx"
		t.Run(internal, func(t *testing.T) {
			prom, labels, err := flattenToProm(internal)
			if err != nil {
				t.Fatalf("flattenToProm(%q): %v", internal, err)
			}
			if prom != "envoy_cluster_upstream_rq_xx" {
				t.Errorf("promName = %q, want envoy_cluster_upstream_rq_xx", prom)
			}
			wantClass := string(rune('0' + digit))
			var gotClass, gotName string
			for _, l := range labels {
				switch l.Key {
				case "envoy_response_code_class":
					gotClass = l.Value
				case "envoy_cluster_name":
					gotName = l.Value
				}
			}
			if gotClass != wantClass {
				t.Errorf("envoy_response_code_class = %q, want %q", gotClass, wantClass)
			}
			if gotName != "c0" {
				t.Errorf("envoy_cluster_name = %q, want c0", gotName)
			}
		})
	}
}

func TestFlattenToProm_Server(t *testing.T) {
	prom, labels, err := flattenToProm("server.live")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_server_live" {
		t.Errorf("promName = %q, want envoy_server_live", prom)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %+v, want empty (Rule SN5: server.<rest> has no extracted labels)", labels)
	}
}

func TestFlattenToProm_Invalid_NoMatchingRule(t *testing.T) {
	_, _, err := flattenToProm("unknown_top_segment.foo")
	if err == nil {
		t.Error("expected error for unknown top segment; got nil")
	}
}

func TestEscapeLabelValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{`with "quotes"`, `with \"quotes\"`},
		{`with\backslash`, `with\\backslash`},
		{"with\nnewline", `with\nnewline`},
		{`all "\` + "\n" + `together`, `all \"\\\ntogether`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := escapeLabelValue(tc.in)
			if got != tc.want {
				t.Errorf("escapeLabelValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHelpText_Coverage(t *testing.T) {
	wantNames := []string{
		"envoy_listener_downstream_cx_total",
		"envoy_listener_downstream_cx_active",
		"envoy_http_downstream_rq_total",
		"envoy_http_downstream_rq_xx",
		"envoy_cluster_upstream_rq_total",
		"envoy_cluster_upstream_rq_xx",
		"envoy_cluster_upstream_cx_total",
		"envoy_cluster_upstream_cx_active",
		"envoy_cluster_membership_total",
		"envoy_server_live",
	}
	for _, n := range wantNames {
		if _, ok := helpText[n]; !ok {
			t.Errorf("helpText missing entry for %q", n)
		}
	}
}
```

Run: `go test ./internal/stats/ -run 'TestFlattenToProm_|TestEscapeLabelValue|TestHelpText_' -v`
Expected: FAIL — `flattenToProm`, `Label`, `escapeLabelValue`, `helpText` all undefined.

- [ ] **Step 2: Implement `name.go`**

```go
package stats

import (
	"fmt"
	"regexp"
	"strings"
)

// Label is one Prometheus label key/value pair. Label-set ordering inside a
// single metric line is determined by the writer (prom.go) — the contract is
// stable within a Prometheus name group; the order is not asserted by tests
// at the per-label level beyond set-equality.
type Label struct {
	Key   string
	Value string
}

// statusClassRE matches the trailing _Nxx (N ∈ 1..5) per Rule SN4. Capture
// group 1 is the base (without the _Nxx); capture group 2 is the single
// class digit. The regex is anchored at end of string.
var statusClassRE = regexp.MustCompile(`^(.+)_([1-5])xx$`)

// flattenToProm transforms an internal hierarchical-dotted name to the
// Prometheus exposition form per Rules SN1–SN8 (ADR-0061; SPEC §10.1).
//
//   SN1: cluster.<n>.<rest>     → envoy_cluster_<rest> + label envoy_cluster_name=<n>
//   SN2: http.<stat_prefix>.<rest> → envoy_http_<rest> + label envoy_http_conn_manager_prefix=<stat_prefix>
//   SN3: listener.<addr>.<rest> → envoy_listener_<rest> + label envoy_listener_address=<addr>
//   SN4: <base>_Nxx             → <base>_xx + label envoy_response_code_class=N (N ∈ 1..5)
//   SN5: server.<rest>          → envoy_server_<rest> + no labels
//   SN6: HELP text best-effort English (handled by prom.go via helpText map)
//   SN7: histograms not emitted (Task-2-time NewCounter/NewGauge are the only
//        registry methods; absence is the contract)
//   SN8: per-endpoint cluster stats not emitted (similarly absent)
//
// Returns the Prometheus base name + the extracted label set + nil on success.
// Returns "", nil, error on names that match no top-level rule.
func flattenToProm(internal string) (string, []Label, error) {
	var labels []Label
	var rest, base string
	switch {
	case strings.HasPrefix(internal, "cluster."):
		// Rule SN1
		tail := strings.TrimPrefix(internal, "cluster.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches cluster.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_cluster_name", Value: tail[:dot]})
		rest = tail[dot+1:]
		base = "envoy_cluster_" + rest
	case strings.HasPrefix(internal, "http."):
		// Rule SN2
		tail := strings.TrimPrefix(internal, "http.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches http.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_http_conn_manager_prefix", Value: tail[:dot]})
		rest = tail[dot+1:]
		base = "envoy_http_" + rest
	case strings.HasPrefix(internal, "listener."):
		// Rule SN3
		tail := strings.TrimPrefix(internal, "listener.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches listener.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_listener_address", Value: tail[:dot]})
		rest = tail[dot+1:]
		base = "envoy_listener_" + rest
	case strings.HasPrefix(internal, "server."):
		// Rule SN5
		rest = strings.TrimPrefix(internal, "server.")
		base = "envoy_server_" + rest
	default:
		return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment (want cluster.|http.|listener.|server.)", internal)
	}

	// Rule SN4: detect the trailing _Nxx and split.
	if m := statusClassRE.FindStringSubmatch(base); m != nil {
		base = m[1] + "_xx"
		labels = append([]Label{{Key: "envoy_response_code_class", Value: m[2]}}, labels...)
	}

	return base, labels, nil
}

// escapeLabelValue escapes a label value per the Prometheus text-format spec:
//   \  → \\
//   "  → \"
//   \n → \n  (literal two-char backslash-n in the output)
// Other characters pass through unchanged.
func escapeLabelValue(s string) string {
	if !strings.ContainsAny(s, `\"`+"\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// helpText maps each Prometheus name emitted by 06.1 to a static English
// description per BRAINSTORM §4.5. Per Rule SN6, HELP text is NOT byte-equal
// to Envoy's HELP text — the differential equivalence claim is on values +
// label keys + types only. The 10 entries cover the 13 unique Prometheus
// names emitted by 06.1 (the four _Nxx counters per HCM and per cluster
// collapse to envoy_http_downstream_rq_xx and envoy_cluster_upstream_rq_xx
// respectively per Rule SN4).
var helpText = map[string]string{
	"envoy_listener_downstream_cx_total":  "Total connections accepted on the listener.",
	"envoy_listener_downstream_cx_active": "Active connections on the listener.",
	"envoy_http_downstream_rq_total":      "Total requests received by the HTTP connection manager.",
	"envoy_http_downstream_rq_xx":         "Requests received by the HTTP connection manager, by response code class.",
	"envoy_cluster_upstream_rq_total":     "Total requests dispatched to upstream clusters.",
	"envoy_cluster_upstream_rq_xx":        "Requests dispatched to upstream clusters, by response code class.",
	"envoy_cluster_upstream_cx_total":     "Total connections established to upstream clusters.",
	"envoy_cluster_upstream_cx_active":    "Active connections to upstream clusters.",
	"envoy_cluster_membership_total":      "Number of endpoints in the cluster.",
	"envoy_server_live":                   "1 if the server is live, 0 otherwise.",
}
```

- [ ] **Step 3: Run tests to verify pass**

```bash
go test -race ./internal/stats/ -v
```

Expected: every TestFlattenToProm_ + TestEscapeLabelValue + TestHelpText_Coverage test PASS plus the existing Task-2/3 tests still PASS.

- [ ] **Step 4: Append ADR-0061 + ADR-0064 to `docs/envoy-go/DECISIONS.md`**

Append both ADRs verbatim from the `## ADRs introduced by this plan` summaries above. ADR-0061's Context section pastes SPEC §10.1's verbatim Envoy-scrape evidence block and the negative-confirmation grep statement; the regex-source citation reads "Envoy v1.37.2 source/common/config/well_known_names.cc, the RESPONSE_CODE_CLASS tag entry. Source-tree commit pin = the v1.37.2 release tag, server-side version-string SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches ENVOY_TARGET.md)." ADR-0064's Consequences section enumerates the silently-ignored field set per the SPEC §9 + §13.2 combined list.

- [ ] **Step 5: Append Task 4 entry to PROGRESS.md**

Standard task entry plus output of `go test -race ./internal/stats/ -v` plus the post-commit `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returning `## ADR-0064:`.

- [ ] **Step 6: Commit**

```bash
git add internal/stats/name.go internal/stats/name_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: stats name flattening rules SN1-SN8 + helpText map [ADR-0061, ADR-0064]"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #4, §4.1 (`name.go`), §6 (17-name table), §8 (ADR-0061 + ADR-0064), §10.1 (Rules SN1–SN8 verbatim), §11.1 (name_test), §14 (Rule SN4 acceptance bullet — empirical evidence in ADR-0061 + the negative-confirmation grep).*

---

## Task 5: `internal/stats/prom.go` Prometheus text-format writer

**Files:**
- Create: `internal/stats/prom.go`
- Create: `internal/stats/prom_test.go`
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 5 entry)

The Prometheus writer walks the Registry, flattens each metric via `name.go`, groups by Prometheus name (status-class collapse joins the four `_Nxx` Prometheus names into one base-name group), sorts alphabetically by Prometheus name, emits `# HELP` / `# TYPE` / metric-line triples per group with a blank-line group separator. No new ADR.

- [ ] **Step 1: Write the failing test (in `prom_test.go`)**

```go
package stats

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteProm_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("WriteProm on empty registry produced %d bytes, want 0", buf.Len())
	}
}

func TestWriteProm_SingleCounter(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	c.Add(42)
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	wantHelp := "# HELP envoy_listener_downstream_cx_total Total connections accepted on the listener."
	wantType := "# TYPE envoy_listener_downstream_cx_total counter"
	wantLine := `envoy_listener_downstream_cx_total{envoy_listener_address="0_0_0_0_10000"} 42`
	for _, w := range []string{wantHelp, wantType, wantLine} {
		if !strings.Contains(out, w) {
			t.Errorf("WriteProm output missing line %q\n--- output ---\n%s", w, out)
		}
	}
}

func TestWriteProm_StatusClassCollapse(t *testing.T) {
	r := NewRegistry()
	for digit := 2; digit <= 5; digit++ {
		c := r.NewCounter("cluster.c0.upstream_rq_" + string(rune('0'+digit)) + "xx")
		c.Add(uint64(digit))
	}
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	// Exactly one HELP + one TYPE for the collapsed group.
	if got := strings.Count(out, "# HELP envoy_cluster_upstream_rq_xx"); got != 1 {
		t.Errorf("# HELP envoy_cluster_upstream_rq_xx count = %d, want 1\n--- output ---\n%s", got, out)
	}
	if got := strings.Count(out, "# TYPE envoy_cluster_upstream_rq_xx counter"); got != 1 {
		t.Errorf("# TYPE envoy_cluster_upstream_rq_xx count = %d, want 1\n--- output ---\n%s", got, out)
	}
	// Four metric lines with each class digit value.
	for digit := 2; digit <= 5; digit++ {
		want := `envoy_cluster_upstream_rq_xx{envoy_response_code_class="` + string(rune('0'+digit)) + `",envoy_cluster_name="c0"} ` + string(rune('0'+digit))
		if !strings.Contains(out, want) {
			t.Errorf("WriteProm output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestWriteProm_AlphabeticallySortedGroups(t *testing.T) {
	r := NewRegistry()
	// Register intentionally out of alphabetical order.
	r.NewCounter("cluster.c0.upstream_cx_total")
	r.NewGauge("server.live").Set(1)
	r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	clusterIdx := strings.Index(out, "# HELP envoy_cluster_upstream_cx_total")
	listenerIdx := strings.Index(out, "# HELP envoy_listener_downstream_cx_total")
	serverIdx := strings.Index(out, "# HELP envoy_server_live")
	if clusterIdx < 0 || listenerIdx < 0 || serverIdx < 0 {
		t.Fatalf("missing groups: cluster=%d listener=%d server=%d\n--- output ---\n%s", clusterIdx, listenerIdx, serverIdx, out)
	}
	if !(clusterIdx < listenerIdx && listenerIdx < serverIdx) {
		t.Errorf("groups not alphabetically sorted: cluster@%d listener@%d server@%d\n--- output ---\n%s", clusterIdx, listenerIdx, serverIdx, out)
	}
}

func TestWriteProm_GaugeRendersNegative(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("listener.0_0_0_0_10000.downstream_cx_active")
	g.Set(-3)
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	want := "} -3"
	if !strings.Contains(out, want) {
		t.Errorf("WriteProm output missing %q (negative gauge value)\n--- output ---\n%s", want, out)
	}
}

func TestWriteProm_EscapesLabelValues(t *testing.T) {
	r := NewRegistry()
	// listener.<addr> with adversarial address (synthetic; production normalisation
	// won't produce these, but the writer must not panic and must escape correctly).
	c := r.NewCounter(`listener.weird"addr.downstream_cx_total`)
	c.Inc()
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `envoy_listener_address="weird\"addr"`) {
		t.Errorf("WriteProm did not escape double-quote in label value\n--- output ---\n%s", out)
	}
}
```

NOTE: the adversarial-name test in the last case requires the registry's name regex to allow `"` and other escapable characters; the current `nameRE` REJECTS them — adjust the test to use a Registry-bypass synthetic Metric implementing the `Metric` interface directly (not through `NewCounter`) so the write-path can be exercised independently of the register-path validation. Codify that in the test:

```go
type synthMetric struct {
	name   string
	mtype  MetricType
	format string
}

func (s *synthMetric) Name() string       { return s.name }
func (s *synthMetric) Type() MetricType   { return s.mtype }
func (s *synthMetric) Format() string     { return s.format }

// Replace TestWriteProm_EscapesLabelValues body:
func TestWriteProm_EscapesLabelValues(t *testing.T) {
	r := NewRegistry()
	r.metrics = append(r.metrics, &synthMetric{name: `listener.weird"addr.downstream_cx_total`, mtype: MetricCounter, format: "1"})
	r.byName[`listener.weird"addr.downstream_cx_total`] = r.metrics[0]
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	if !strings.Contains(buf.String(), `envoy_listener_address="weird\"addr"`) {
		t.Errorf("WriteProm did not escape double-quote in label value\n--- output ---\n%s", buf.String())
	}
}
```

(The synthetic-Metric injection is a test-only mechanism that lets the writer's escaping be tested independently of the registry's validation. Production code never reaches the writer with invalid characters because `NewCounter`/`NewGauge` reject them.)

Run: `go test ./internal/stats/ -run TestWriteProm_ -v`
Expected: FAIL — `WriteProm` undefined.

- [ ] **Step 2: Implement `prom.go`**

```go
package stats

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteProm walks the registry, flattens each metric via name.go's
// flattenToProm, groups by Prometheus name (status-class collapse joins the
// four _Nxx Prometheus names into one base-name group with four
// envoy_response_code_class-keyed lines per Rule SN4), sorts alphabetically by
// Prometheus name, and emits one # HELP + one # TYPE per group followed by
// one metric line per fully-qualified label set. Group separator is a blank
// line. Returns nil on success or the first io.Writer error encountered.
//
// On a flattenToProm error for any single metric (which should not happen if
// NewCounter/NewGauge validation held), the metric is silently skipped and
// the writer continues — log+ignore matches the BRAINSTORM §5.3 rationale
// that "errors from WriteProm are logged and otherwise ignored (no retry,
// no error response — too late, headers already sent)."
func WriteProm(w io.Writer, r *Registry) error {
	type promLine struct {
		labels []Label
		value  string
	}
	type promGroup struct {
		name    string
		mtype   MetricType
		help    string
		entries []promLine
	}
	groups := make(map[string]*promGroup)
	var keys []string

	r.Walk(func(m Metric) {
		base, labels, err := flattenToProm(m.Name())
		if err != nil {
			return // skip malformed names (defence-in-depth; should not occur)
		}
		g, ok := groups[base]
		if !ok {
			g = &promGroup{
				name:  base,
				mtype: m.Type(),
				help:  helpText[base], // empty string if absent; emitted as "" then
			}
			groups[base] = g
			keys = append(keys, base)
		}
		g.entries = append(g.entries, promLine{labels: labels, value: m.Format()})
	})
	sort.Strings(keys)

	for i, k := range keys {
		g := groups[k]
		help := g.help
		if help == "" {
			help = g.name // fall back to the name as a no-op help when missing
		}
		typeStr := "counter"
		if g.mtype == MetricGauge {
			typeStr = "gauge"
		}
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", g.name, help, g.name, typeStr); err != nil {
			return err
		}
		for _, e := range g.entries {
			if err := writeMetricLine(w, g.name, e.labels, e.value); err != nil {
				return err
			}
		}
		if i < len(keys)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeMetricLine emits one Prometheus metric line:
//
//   <name>{<key>="<escaped_value>",...} <value>\n
//
// When labels is empty, the {} block is OMITTED:
//
//   <name> <value>\n
func writeMetricLine(w io.Writer, name string, labels []Label, value string) error {
	if len(labels) == 0 {
		_, err := fmt.Fprintf(w, "%s %s\n", name, value)
		return err
	}
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, l := range labels {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(l.Key)
		sb.WriteString(`="`)
		sb.WriteString(escapeLabelValue(l.Value))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	sb.WriteByte(' ')
	sb.WriteString(value)
	sb.WriteByte('\n')
	_, err := io.WriteString(w, sb.String())
	return err
}
```

- [ ] **Step 3: Run tests to verify pass**

```bash
go test -race ./internal/stats/ -v
```

Expected: every TestWriteProm_ test PASS plus the existing Task-2/3/4 tests still PASS.

- [ ] **Step 4: Append Task 5 entry to PROGRESS.md**

Standard task entry plus output of `go test -race ./internal/stats/ -v`.

- [ ] **Step 5: Commit**

```bash
git add internal/stats/prom.go internal/stats/prom_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: stats Prometheus text-format writer (alphabetic sort + status-class collapse)"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #2, §4.1 (`prom.go`), §5.6 (read-path shape), §11.1 (prom_test).*

---

## Task 6: `internal/admin/prometheus.go` handler + `admin.New` signature widening

**Files:**
- Create: `internal/admin/prometheus.go`
- Create: `internal/admin/prometheus_test.go`
- Modify: `internal/admin/admin.go` (signature change + `/stats/prometheus` route + `server.live` gauge alloc + `sync.Once`)
- Modify: `internal/admin/admin_test.go` (call-sites + `/stats/prometheus` route tests)
- Modify: `internal/admin/doc.go` (mention `/stats/prometheus`)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 6 entry)

Adds the admin endpoint that the differential fixture scrapes. `admin.New(addr string)` widens to `admin.New(addr string, registry *stats.Registry) *Server` (per advisory item i in `## Spec-review advisory responses`); the `Start()` method registers `/stats/prometheus` alongside `/ready`; `handleReady`'s ready-flip `Set(1)`s the `server.live` gauge inside a `sync.Once` per SPEC §12 #3.

- [ ] **Step 1: Write the failing test (in `prometheus_test.go`)**

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestHandlePrometheus_ContentType(t *testing.T) {
	r := stats.NewRegistry()
	c := r.NewCounter("server.live")
	c.Inc()
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/plain; version=0.0.4; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func TestHandlePrometheus_EmptyRegistry(t *testing.T) {
	r := stats.NewRegistry()
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	if got := rec.Code; got != http.StatusOK {
		t.Errorf("status = %d, want 200 (empty registry → empty body, still 200)", got)
	}
	if got := rec.Body.String(); got != "" {
		t.Errorf("body = %q, want empty", got)
	}
}

func TestHandlePrometheus_RoundTrip(t *testing.T) {
	r := stats.NewRegistry()
	r.NewGauge("server.live").Set(1)
	r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total").Add(7)
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE envoy_server_live gauge",
		"envoy_server_live 1",
		"# TYPE envoy_listener_downstream_cx_total counter",
		`envoy_listener_downstream_cx_total{envoy_listener_address="0_0_0_0_10000"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n--- body ---\n%s", want, body)
		}
	}
}
```

Run: `go test ./internal/admin/ -run TestHandlePrometheus_ -v`
Expected: FAIL — `handlePrometheus` undefined; `stats` import in admin currently absent.

- [ ] **Step 2: Implement `internal/admin/prometheus.go`**

```go
package admin

import (
	"log"
	"net/http"

	"github.com/esalaine/envoy-go/internal/stats"
)

// handlePrometheus returns an HTTP handler that serves the
// Prometheus text-format exposition by walking the given Registry.
//
// Per SPEC §11.2 + ADR-0059: the response Content-Type is
// "text/plain; version=0.0.4; charset=utf-8" matching the Prometheus
// exposition spec; the body is the alphabetically-sorted-by-Prom-name
// exposition; HELP-text is the static map keyed by Prometheus name;
// errors from stats.WriteProm are logged and otherwise ignored (per
// BRAINSTORM §5.3: too late to retry — headers already sent).
func handlePrometheus(r *stats.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := stats.WriteProm(w, r); err != nil {
			log.Printf("admin: /stats/prometheus: write: %v", err)
		}
	}
}
```

- [ ] **Step 3: Modify `internal/admin/admin.go`** — widen `New` signature, register the new route, add `server.live` gauge with `sync.Once`

```go
package admin

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// Server is the admin HTTP/1.1 server. /ready and /stats/prometheus are
// implemented in phase 06.1; other admin endpoints land in phase 08.
type Server struct {
	addr      string
	ln        net.Listener
	httpSrv   *http.Server
	ready     atomic.Bool
	registry  *stats.Registry
	liveGauge *stats.Gauge
	liveOnce  sync.Once
}

// New returns an admin server targeting addr. The server is not running yet;
// call Start. The /ready gate is initially closed (MarkReady flips it). The
// registry parameter is the boot-time Registry threaded by main.go; it MUST
// NOT be Frozen yet (admin allocates the server.live gauge at New time per
// SPEC §5.4 + §12 #3).
func New(addr string, registry *stats.Registry) *Server {
	return &Server{
		addr:      addr,
		registry:  registry,
		liveGauge: registry.NewGauge("server.live"),
	}
}

// Start binds and begins serving in a background goroutine. Returns the bound
// address (useful when addr had port 0). Error only if bind fails.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/stats/prometheus", handlePrometheus(s.registry))
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// MarkReady flips /ready into the ready state.
func (s *Server) MarkReady() { s.ready.Store(true) }

// Close performs best-effort shutdown. Idempotent. No graceful drain (phase 08).
func (s *Server) Close() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=UTF-8")
	h.Set("Cache-Control", "no-cache, max-age=0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Server", "envoy")

	if !s.ready.Load() {
		body := []byte("PRE_INITIALIZING\n")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
		return
	}
	// LIVE-path: the first time this branch executes, Set(1) the
	// server.live gauge per SPEC §12 #3. Subsequent calls are no-ops.
	s.liveOnce.Do(func() { s.liveGauge.Set(1) })
	body := []byte("LIVE\n")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

- [ ] **Step 4: Modify `internal/admin/admin_test.go`** — update every `admin.New(addr)` call site to `admin.New(addr, stats.NewRegistry())`; add a test for the `/stats/prometheus` route (sanity check: `Start` registers it; a GET returns 200 with the Prom Content-Type) and a test for the `server.live` gauge transition (initial Load == 0; after one `MarkReady()` + GET `/ready` returning 200, Load == 1; after a second GET `/ready`, Load still == 1 — the `sync.Once` discipline).

```go
// Excerpt — full file rewrites every admin.New call:
import "github.com/esalaine/envoy-go/internal/stats"

func TestServer_StatsPrometheusRouteRegistered(t *testing.T) {
	r := stats.NewRegistry()
	srv := New("127.0.0.1:0", r)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()
	resp, err := http.Get("http://" + addr + "/stats/prometheus")
	if err != nil {
		t.Fatalf("GET /stats/prometheus: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestServer_LiveGaugeSetOnceFlippedAtFirstReady200(t *testing.T) {
	r := stats.NewRegistry()
	srv := New("127.0.0.1:0", r)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()
	// Initially: server.live == 0, /ready returns 503.
	resp503, _ := http.Get("http://" + addr + "/ready")
	_ = resp503.Body.Close()
	if got := srv.liveGauge.Load(); got != 0 {
		t.Errorf("server.live before MarkReady = %d, want 0", got)
	}
	srv.MarkReady()
	for i := 0; i < 3; i++ {
		resp, _ := http.Get("http://" + addr + "/ready")
		_ = resp.Body.Close()
	}
	if got := srv.liveGauge.Load(); got != 1 {
		t.Errorf("server.live after MarkReady + 3× /ready = %d, want 1", got)
	}
}
```

- [ ] **Step 5: Modify `internal/admin/doc.go`** — extend the package doc to mention `/stats/prometheus` alongside `/ready`. Single-paragraph addition.

- [ ] **Step 6: Run tests to verify pass**

```bash
go test -race ./internal/stats/ ./internal/admin/ -v
```

Expected: every TestHandlePrometheus_ + TestServer_ test PASS.

- [ ] **Step 7: Append Task 6 entry to PROGRESS.md and Commit**

```bash
git add internal/admin/prometheus.go internal/admin/prometheus_test.go internal/admin/admin.go internal/admin/admin_test.go internal/admin/doc.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: admin /stats/prometheus endpoint + server.live gauge with sync.Once"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #2, §4.1 (`prometheus.go`), §4.2 (`admin.go` extension), §5.7 (`server.live`), §11.2 (prometheus_test), §12 #3 (`sync.Once`).*

---

## Task 7: `internal/bootstrap/bootstrap.go` `.Stats` field + `Load` Registry alloc

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` (add `Stats *stats.Registry` field; allocate in `Load`)
- Modify: `internal/bootstrap/bootstrap_test.go` (assert `Stats` is non-nil after `Load`)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 7 entry)

Per `## Settled SPEC §12 deferred decisions` #2: the `Bootstrap` struct gains a `Stats *stats.Registry` field (field-on-Bootstrap shape, not a free-standing alloc in `main.go`). Future xDS phases that add dynamic config-reload have a place to thread the Registry through a config-update path.

- [ ] **Step 1: Write the failing test (in `bootstrap_test.go`)**

```go
func TestLoad_AllocatesStatsRegistry(t *testing.T) {
	bs, err := Load(strings.NewReader(minimalBootstrapYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bs.Stats == nil {
		t.Fatal("Bootstrap.Stats is nil; expected an allocated *stats.Registry")
	}
	// The Registry MUST NOT be Frozen yet (downstream constructors register).
	c := bs.Stats.NewCounter("test.field-not-frozen")
	if c == nil {
		t.Fatal("NewCounter on Bootstrap.Stats returned nil")
	}
}
```

(`minimalBootstrapYAML` is an existing test fixture; if the test package doesn't already define a minimal YAML fixture, copy the smallest one from the existing tests.)

Run: `go test ./internal/bootstrap/ -run TestLoad_AllocatesStatsRegistry -v`
Expected: FAIL — `Bootstrap.Stats` field undefined.

- [ ] **Step 2: Modify `internal/bootstrap/bootstrap.go`**

Add an import of `github.com/esalaine/envoy-go/internal/stats`. Add a `Stats *stats.Registry` field to the `Bootstrap` struct (or whatever the struct is named — verify via `grep -nE 'type.*struct' internal/bootstrap/bootstrap.go`; the loader's return value is what gets a `.Stats` field). At the construction site inside `Load(r io.Reader)`, after the proto unmarshal succeeds, allocate `bs.Stats = stats.NewRegistry()` before returning.

If `Load` currently returns `(*bootstrapv3.Bootstrap, error)` directly (the proto type, not a wrapping struct), introduce a thin wrapper:

```go
type Bootstrap struct {
	Proto *bootstrapv3.Bootstrap
	Stats *stats.Registry
}

func Load(r io.Reader) (*Bootstrap, error) {
	// existing proto-unmarshal logic, returning the *bootstrapv3.Bootstrap value as `proto`
	return &Bootstrap{Proto: proto, Stats: stats.NewRegistry()}, nil
}
```

— and propagate the call-site changes through `cmd/envoy-go/main.go` and `internal/bootstrap/bootstrap_test.go` accessors. The propagation may touch a few call sites in `internal/cluster/manager.go` / `internal/listener/manager.go` / `internal/admin/admin_test.go` (the existing places that consume the proto). The planner picks the wrapper-vs-direct-field shape based on what `bootstrap.Load`'s current return type is at Task 7 execution time — verify with `grep -nE 'func Load' internal/bootstrap/bootstrap.go` at Step 1's precondition stage.

If the existing return type is already a wrapping struct (e.g. `*Bootstrap` with sub-fields like `Proto`, `BaseDir`, `AdminSocket`), simply add the `Stats *stats.Registry` field alongside the existing fields and allocate in `Load`.

- [ ] **Step 3: Run tests to verify pass**

```bash
go test ./internal/bootstrap/ -v
```

Expected: every existing test PLUS TestLoad_AllocatesStatsRegistry PASS.

- [ ] **Step 4: Append Task 7 entry to PROGRESS.md and Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: Bootstrap.Stats field + Load allocates *stats.Registry"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §4.2 (`bootstrap.go` extension), §5.4 (boot wiring sequence), §12 #2 (settled).*

---

## Task 8: `internal/cluster/manager.go` Registry threading + 8-metric per-cluster alloc + `Cluster` struct fields [ADR-0063]

**Files:**
- Modify: `internal/cluster/cluster.go` (add 8 metric-pointer fields to `Cluster`)
- Modify: `internal/cluster/manager.go` (widen `NewManager` and `NewManagerWithBaseDir` signatures with `*stats.Registry`; allocate 8 metrics per cluster at build time)
- Modify: `internal/cluster/cluster_test.go` (call sites + new tests for register-time metric alloc)
- Modify: `internal/cluster/manager_test.go` (call sites)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0063)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 8 entry)

The cluster-side metric-allocation loop is the first use of the cluster-level-only metric set; ADR-0063 (Per-endpoint cluster stats not emitted) lands here.

- [ ] **Step 1: Write the failing test (in `cluster_test.go` / `manager_test.go`)**

```go
func TestNewManager_AllocatesEightMetricsPerCluster(t *testing.T) {
	bs := minimalBootstrapWithSingleCluster(t, "c0", "127.0.0.1:9001")
	r := stats.NewRegistry()
	m, err := NewManager(bs, r)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	c, ok := m.LookupByName("c0")
	if !ok {
		t.Fatal("cluster c0 not found")
	}
	// Each metric pointer must be non-nil.
	if c.upstreamRqTotal == nil ||
		c.upstreamRq2xx == nil || c.upstreamRq3xx == nil ||
		c.upstreamRq4xx == nil || c.upstreamRq5xx == nil ||
		c.upstreamCxTotal == nil ||
		c.upstreamCxActive == nil ||
		c.membershipTotal == nil {
		t.Errorf("expected all 8 metric pointers non-nil; got: %+v", c)
	}
	// membership_total Set to 1 at register time (single endpoint).
	if got := c.membershipTotal.Load(); got != 1 {
		t.Errorf("membershipTotal = %d, want 1", got)
	}
	// Walk: 8 metrics must be visible under cluster.c0.* names.
	var seen []string
	r.Walk(func(m stats.Metric) {
		seen = append(seen, m.Name())
	})
	wantNames := map[string]bool{
		"cluster.c0.upstream_rq_total":  true,
		"cluster.c0.upstream_rq_2xx":    true,
		"cluster.c0.upstream_rq_3xx":    true,
		"cluster.c0.upstream_rq_4xx":    true,
		"cluster.c0.upstream_rq_5xx":    true,
		"cluster.c0.upstream_cx_total":  true,
		"cluster.c0.upstream_cx_active": true,
		"cluster.c0.membership_total":   true,
	}
	for _, n := range seen {
		delete(wantNames, n)
	}
	if len(wantNames) != 0 {
		t.Errorf("missing cluster metrics: %v", wantNames)
	}
}
```

(The `minimalBootstrapWithSingleCluster` helper is a small test factory; the cluster_test.go file already has fixture-bootstrap helpers — adapt the smallest existing helper.)

Run: `go test ./internal/cluster/ -run TestNewManager_AllocatesEightMetricsPerCluster -v`
Expected: FAIL — the metric fields don't exist on `*Cluster`; `NewManager` doesn't accept a Registry.

- [ ] **Step 2: Modify `internal/cluster/cluster.go`** — add 8 metric-pointer fields to `Cluster`

```go
import (
	// existing imports
	"github.com/esalaine/envoy-go/internal/stats"
)

type Cluster struct {
	name           string
	endpoints      []Endpoint
	connectTimeout time.Duration
	lb             loadBalancer
	upstreamCfg    *stdtls.Config
	useH2          bool
	// 06.1 metric fields (per ADR-0063 — cluster-level only; per-endpoint
	// expansion deferred). All fields are non-nil after Manager.buildCluster
	// completes; all are concurrent-safe (atomic primitives).
	upstreamRqTotal  *stats.Counter
	upstreamRq2xx    *stats.Counter
	upstreamRq3xx    *stats.Counter
	upstreamRq4xx    *stats.Counter
	upstreamRq5xx    *stats.Counter
	upstreamCxTotal  *stats.Counter
	upstreamCxActive *stats.Gauge
	membershipTotal  *stats.Gauge
}
```

Add an unexported method `(*Cluster).statusClassCounter(code int) *stats.Counter` that returns the right `_Nxx` counter for an HTTP status code (used by Task 10 in `actions.go`):

```go
// statusClassCounter returns the upstream_rq_<Nxx> counter for the given
// HTTP status code per the integer-divide code/100 discipline. Returns nil
// for codes outside [100, 599].
func (c *Cluster) statusClassCounter(code int) *stats.Counter {
	switch code / 100 {
	case 2:
		return c.upstreamRq2xx
	case 3:
		return c.upstreamRq3xx
	case 4:
		return c.upstreamRq4xx
	case 5:
		return c.upstreamRq5xx
	default:
		return nil
	}
}
```

- [ ] **Step 3: Modify `internal/cluster/manager.go`** — widen the two constructor signatures and allocate the 8 metrics per cluster at build time

```go
import (
	// existing
	"github.com/esalaine/envoy-go/internal/stats"
)

func NewManager(bs *bootstrapv3.Bootstrap, registry *stats.Registry) (*Manager, error) {
	return NewManagerWithBaseDir(bs, "", registry)
}

func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string, registry *stats.Registry) (*Manager, error) {
	// ... existing parse logic ...
	for _, cProto := range bs.GetStaticResources().GetClusters() {
		c, err := buildCluster(cProto, baseDir)
		if err != nil {
			return nil, err
		}
		registerClusterMetrics(registry, c)
		// existing append/index work
	}
	// ...
}

// registerClusterMetrics allocates the 8 cluster-scope metrics per ADR-0063
// and stores the pointers on c. Called once per cluster at Manager build time;
// pre-Freeze (the listener manager and admin server precede the
// registry.Freeze() call in cmd/envoy-go/main.go).
func registerClusterMetrics(r *stats.Registry, c *Cluster) {
	prefix := "cluster." + c.name + "."
	c.upstreamRqTotal = r.NewCounter(prefix + "upstream_rq_total")
	c.upstreamRq2xx = r.NewCounter(prefix + "upstream_rq_2xx")
	c.upstreamRq3xx = r.NewCounter(prefix + "upstream_rq_3xx")
	c.upstreamRq4xx = r.NewCounter(prefix + "upstream_rq_4xx")
	c.upstreamRq5xx = r.NewCounter(prefix + "upstream_rq_5xx")
	c.upstreamCxTotal = r.NewCounter(prefix + "upstream_cx_total")
	c.upstreamCxActive = r.NewGauge(prefix + "upstream_cx_active")
	c.membershipTotal = r.NewGauge(prefix + "membership_total")
	c.membershipTotal.Set(int64(len(c.endpoints))) // SPEC §6: Set once at register, equals N endpoints
}
```

- [ ] **Step 4: Update existing call sites in `cluster_test.go` and `manager_test.go`** — every `NewManager(bs)` becomes `NewManager(bs, stats.NewRegistry())`; every `NewManagerWithBaseDir(bs, baseDir)` becomes `NewManagerWithBaseDir(bs, baseDir, stats.NewRegistry())`. The propagated changes should also surface in any caller files outside `internal/cluster/` — Task 12 will catch those in `cmd/envoy-go/main.go`.

- [ ] **Step 5: Run tests to verify pass**

```bash
go test -race ./internal/cluster/ -v
```

Expected: every test PASS, including the new `TestNewManager_AllocatesEightMetricsPerCluster`.

- [ ] **Step 6: Append ADR-0063 to `docs/envoy-go/DECISIONS.md`**

Per the summary in `## ADRs introduced by this plan`. Status / Context / Decision / Consequences / Lands-in-task per ADR-0001 template.

- [ ] **Step 7: Append Task 8 entry to PROGRESS.md and Commit**

```bash
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/cluster_test.go internal/cluster/manager_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: cluster Registry threading + 8 metrics per cluster + Cluster fields [ADR-0063]"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #3, §4.2 (`cluster.go`/`manager.go` extensions), §5.4 (boot wiring), §6 (8 cluster names), §8 (ADR-0063), §11.3 (cluster_test extension).*

---

## Task 9: Cluster `Dial` + `DialH2` upstream-cx metric wiring (`connWithGauge` wrapper)

**Files:**
- Modify: `internal/cluster/cluster.go` (extend `Dial` to Inc both upstream_cx counters; wrap returned conn in `connWithGauge`)
- Modify: `internal/cluster/dial_h2.go` (reuse `connWithGauge` on the TLS conn before passing to `h2.NewClientConn`)
- Modify: `internal/cluster/cluster_test.go` (assert Inc + wrapper Dec on Close)
- Modify: `internal/cluster/dial_h2_test.go` (assert the H2 path also Incs/Decs)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 9 entry)

Cluster-side hot-path edits per SPEC §5.5: on successful dial `c.upstreamCxTotal.Inc()` + `c.upstreamCxActive.Inc()`; the returned `net.Conn` is wrapped in a `connWithGauge` that calls the gauge `Dec()` exactly once on `Close()`. No new ADR.

- [ ] **Step 1: Write the failing test (in `cluster_test.go`)**

```go
func TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose(t *testing.T) {
	// Spin up a no-op TCP backend so Dial succeeds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _, _ = io.Copy(io.Discard, c) }(c)
		}
	}()
	bs := bootstrapWithCluster(t, "c0", ln.Addr().String())
	r := stats.NewRegistry()
	m, err := NewManager(bs, r)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	c, _ := m.LookupByName("c0")
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("pre-Dial upstreamCxTotal = %d, want 0", got)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("pre-Dial upstreamCxActive = %d, want 0", got)
	}
	conn, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Errorf("post-Dial upstreamCxTotal = %d, want 1", got)
	}
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Errorf("post-Dial upstreamCxActive = %d, want 1", got)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("post-Close upstreamCxActive = %d, want 0", got)
	}
	// Total stays at 1 (counter, never decrements).
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Errorf("post-Close upstreamCxTotal = %d, want 1 (counter)", got)
	}
}

func TestDial_CloseIdempotent(t *testing.T) {
	// connWithGauge.Close must Dec exactly once even if called multiple times.
	// Setup as above; conn.Close() then conn.Close() again — Active still 0,
	// not -1.
	// (Implementation detail: connWithGauge uses a sync.Once to guard the Dec.)
}
```

Run: `go test -race ./internal/cluster/ -run TestDial_ -v`
Expected: FAIL — `Dial` doesn't Inc the metrics; the wrapper doesn't exist.

- [ ] **Step 2: Implement `connWithGauge` + extend `Dial` in `cluster.go`**

```go
import (
	// existing
	"sync"
)

// connWithGauge wraps a net.Conn so the cluster's upstream_cx_active gauge
// decrements exactly once when Close is first called. Per ADR-0063 the
// gauge is per-cluster (not per-endpoint).
type connWithGauge struct {
	net.Conn
	dec  func()
	once sync.Once
}

func (c *connWithGauge) Close() error {
	c.once.Do(c.dec)
	return c.Conn.Close()
}

// Dial — extend the existing body. After the successful raw dial / TLS
// handshake (whichever returns), Inc both cx counters and wrap the conn:
func (c *Cluster) Dial(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	final := raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, nil
}
```

- [ ] **Step 3: Modify `internal/cluster/dial_h2.go`** — wrap the TLS conn with `connWithGauge` before handing to `h2.NewClientConn`

The existing `DialH2` calls `c.Dial(ctx)` (which already returns a `*connWithGauge` after Step 2's edit) and type-asserts the underlying conn to `*stdtls.Conn`. The type-assertion now fails because the value is `*connWithGauge`, not `*stdtls.Conn` directly. Fix: type-assert to `*connWithGauge` first, then unwrap the `Conn` field, then assert to `*stdtls.Conn`. The gauge Dec lives on the `connWithGauge`'s `Close()`; when `(*ClientConn).Close()` calls the underlying conn's `Close()`, the gauge fires.

```go
func (c *Cluster) DialH2(ctx context.Context) (*h2.ClientConn, error) {
	dc, err := c.Dial(ctx)
	if err != nil {
		return nil, err
	}
	wrapped, ok := dc.(*connWithGauge)
	if !ok {
		_ = dc.Close()
		return nil, fmt.Errorf("cluster: dial h2: expected *connWithGauge, got %T", dc)
	}
	tlsConn, ok := wrapped.Conn.(*stdtls.Conn)
	if !ok {
		_ = wrapped.Close()
		return nil, fmt.Errorf("cluster: dial h2: not a TLS conn (got %T)", wrapped.Conn)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = wrapped.Close()
		return nil, fmt.Errorf("cluster: dial h2: handshake: %w", err)
	}
	if got := tlsConn.ConnectionState().NegotiatedProtocol; got != "h2" {
		_ = wrapped.Close()
		return nil, fmt.Errorf("cluster: dial h2: alpn negotiated %q, want \"h2\"", got)
	}
	cc, err := h2.NewClientConn(ctx, wrapped) // pass the wrapper, not the raw tlsConn — so Close() Decs the gauge
	if err != nil {
		_ = wrapped.Close()
		return nil, fmt.Errorf("cluster: dial h2: client conn: %w", err)
	}
	return cc, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test -race ./internal/cluster/ -v
```

Expected: every existing test PLUS the new `TestDial_` tests PASS.

- [ ] **Step 5: Append Task 9 entry to PROGRESS.md and Commit**

```bash
git add internal/cluster/cluster.go internal/cluster/dial_h2.go internal/cluster/cluster_test.go internal/cluster/dial_h2_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: cluster Dial + DialH2 upstream-cx metric wiring (connWithGauge wrapper)"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §4.2 (`dial.go`/`dial_h2.go` extensions per the SPEC; `connWithGauge` lives in `cluster.go` per `## Settled SPEC §12 deferred decisions` #10), §5.5 (Increment paths table), §11.3 (cluster_test extension).*

---

## Task 10: Listener-side Registry threading + 2-metric per-listener alloc + accept-loop hot path

**Files:**
- Modify: `internal/listener/manager.go` (widen `NewManager`, `NewManagerWithBaseDir`, `NewManagerWithBaseDirAndAllowH2C` with `*stats.Registry`; allocate 2 metrics per listener; capture Registry in HCM-factory closure for downstream HCM allocations)
- Modify: `internal/listener/listener.go` (accept-loop +2 LoC: `cx_total.Inc()` + `cx_active.Inc()`; defer `cx_active.Dec()` on conn close)
- Modify: `internal/listener/manager_test.go` (call sites + 2-metric alloc test)
- Modify: `internal/listener/listener_test.go` (accept-loop Inc/Dec assertions)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 10 entry)

Listener-side hot-path edits per SPEC §5.5: per-listener `downstream_cx_total` (counter) + `downstream_cx_active` (gauge). Registry threads to the listener-internal HCM-factory closure so per-HCM metric allocation works at filter-build time (Task 11). No new ADR.

- [ ] **Step 1: Write the failing test (in `listener_test.go` / `manager_test.go`)**

```go
func TestListenerManager_AllocatesTwoMetricsPerListener(t *testing.T) {
	bs := bootstrapWithTcpListener(t, "l_h1", "127.0.0.1:0")
	r := stats.NewRegistry()
	cm, _ := cluster.NewManager(bs, r)
	lm, err := NewManager(bs, cm, r)
	if err != nil {
		t.Fatalf("listener.NewManager: %v", err)
	}
	defer lm.Stop()
	// Walk the registry; the listener.<addr>.downstream_cx_{total,active}
	// names must be present.
	var seen []string
	r.Walk(func(m stats.Metric) { seen = append(seen, m.Name()) })
	wantSubstr := []string{".downstream_cx_total", ".downstream_cx_active"}
	for _, w := range wantSubstr {
		found := false
		for _, n := range seen {
			if strings.HasPrefix(n, "listener.") && strings.HasSuffix(n, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing listener.<addr>%s metric (seen=%v)", w, seen)
		}
	}
}

func TestListener_AcceptLoop_IncsCxTotalAndCxActive(t *testing.T) {
	// Spin up listener; dial a single conn; assert cx_total Inc'd to 1 and
	// cx_active Inc'd to 1 (and Dec'd to 0 after conn close).
	// Implementation note: the listener's accept loop must Inc within ~10ms
	// of the accept; the test polls.
}
```

Run: `go test -race ./internal/listener/ -run 'TestListenerManager_|TestListener_AcceptLoop_' -v`
Expected: FAIL — listener-side metrics not allocated; accept-loop doesn't Inc.

- [ ] **Step 2: Modify `internal/listener/manager.go`** — widen the three `NewManager*` signatures and allocate 2 metrics per listener at build time

```go
import (
	// existing
	"strings"

	"github.com/esalaine/envoy-go/internal/stats"
)

func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, registry *stats.Registry) (*Manager, error) {
	return NewManagerWithBaseDir(bs, cm, "", registry)
}

func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, registry *stats.Registry) (*Manager, error) {
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, false, registry)
}

func NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry) (*Manager, error) {
	// existing parse logic ...
	for _, lProto := range bs.GetStaticResources().GetListeners() {
		l, err := buildListener(lProto, cm, baseDir, allowH2C, registry)
		if err != nil {
			return nil, err
		}
		registerListenerMetrics(registry, l)
		// existing append/index work
	}
	// ...
}

// normalizeAddr turns a net.Addr.String() into the Envoy-style form used as
// the listener-scope label and stat-name segment: 0.0.0.0:10000 → 0.0.0.0_10000.
func normalizeAddr(addr string) string {
	return strings.NewReplacer(":", "_", ".", "_").Replace(addr)
}

// registerListenerMetrics allocates the 2 listener-scope metrics per SPEC §6.
// Stores the pointers on l for the accept loop's hot path.
func registerListenerMetrics(r *stats.Registry, l *Listener) {
	prefix := "listener." + normalizeAddr(l.boundAddr) + "."
	l.downstreamCxTotal = r.NewCounter(prefix + "downstream_cx_total")
	l.downstreamCxActive = r.NewGauge(prefix + "downstream_cx_active")
}
```

The HCM factory closure (which constructs `internal/filter/hcm` filters at filter-chain-build time) MUST capture `registry` so per-HCM-instance metric allocation works at Task 11. The exact closure shape is what `buildListener` already uses for its filter-chain construction; pass `registry` through that closure parameter list.

- [ ] **Step 3: Modify `internal/listener/listener.go`** — extend the accept loop and the conn-handler shutdown

```go
type Listener struct {
	// existing fields
	downstreamCxTotal  *stats.Counter
	downstreamCxActive *stats.Gauge
}

// In the accept loop (or the per-conn handler kickoff):
for {
	conn, err := l.ln.Accept()
	if err != nil {
		// existing error handling
	}
	l.downstreamCxTotal.Inc()
	l.downstreamCxActive.Inc()
	go func(c net.Conn) {
		defer l.downstreamCxActive.Dec()
		defer c.Close()
		l.handleConn(ctx, c) // existing handler
	}(conn)
}
```

(The exact accept-loop site is whatever the existing listener uses; the planner picks the smallest-delta location at Task 10 step 3 execution time. The Inc/Dec discipline is: Inc on accept, Dec on conn close (deferred) — exactly once per conn.)

- [ ] **Step 4: Update existing call sites** — in `manager_test.go` and any other test/main file that constructs the listener manager: every `NewManager(bs, cm)` becomes `NewManager(bs, cm, stats.NewRegistry())`; ditto the BaseDir variants.

- [ ] **Step 5: Run tests to verify pass**

```bash
go test -race ./internal/listener/ ./internal/stats/ ./internal/admin/ ./internal/cluster/ -v
```

Expected: every test PASS.

- [ ] **Step 6: Append Task 10 entry to PROGRESS.md and Commit**

```bash
git add internal/listener/manager.go internal/listener/listener.go internal/listener/manager_test.go internal/listener/listener_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: listener Registry threading + 2 metrics per listener + accept-loop hot path"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #3, §4.2 (`listener/manager.go` + `listener.go` extensions), §5.4 (boot wiring), §5.5 (accept-loop hot path), §6 (2 listener names), §11.3 (listener_test extension).*

---

## Task 11: HCM-side per-instance metric alloc + dispatch-entry/response-class hot path + M-9 carry-forward log line

**Files:**
- Modify: `internal/filter/hcm/filter.go` (Filter struct gains 5 metric-pointer fields; `NewFilter` allocates from the listener-captured Registry; HCM dispatch entry Inc; response-finalization status-class dispatch)
- Modify: `internal/filter/hcm/actions.go` (H1 + H2 router-action: `c.upstreamRqTotal.Inc()` on dispatch entry; `c.statusClassCounter(code).Inc()` on response status finalization)
- Modify: `internal/filter/hcm/h2/router_action.go` (M-9 carry-forward: `log.Printf("h2: doH2 error: %v", err)` on the `doH2` error path before returning)
- Create: `internal/filter/hcm/h2/router_action_test.go` (M-9 unit test per SPEC §11.4 + §12 #5)
- Modify: `internal/filter/hcm/filter_test.go` (extend hot-path Inc/dispatch tests)
- Modify: `internal/filter/hcm/actions_test.go` (extend H1+H2 router-action hot-path Inc tests)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 11 entry)

HCM-side hot-path edits per SPEC §5.5; the M-9 carry-forward bundle from 05.2 lands in this same task because the surface (`log.Printf` on the H2 router-action error path) lives adjacent to the Inc-on-response-class wiring. No new ADR.

- [ ] **Step 1: Write the failing tests**

In `filter_test.go`:

```go
func TestNewFilter_Allocates5HCMMetrics(t *testing.T) {
	// Build an HCM with stat_prefix=ingress_http; assert the 5 names are
	// present in the Registry.
	r := stats.NewRegistry()
	f := newFilterForTest(t, "ingress_http", r)
	wantNames := []string{
		"http.ingress_http.downstream_rq_total",
		"http.ingress_http.downstream_rq_2xx",
		"http.ingress_http.downstream_rq_3xx",
		"http.ingress_http.downstream_rq_4xx",
		"http.ingress_http.downstream_rq_5xx",
	}
	var seen []string
	r.Walk(func(m stats.Metric) { seen = append(seen, m.Name()) })
	for _, w := range wantNames {
		found := false
		for _, n := range seen {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing HCM metric %q (seen=%v)", w, seen)
		}
	}
	_ = f
}

func TestFilter_RequestEntry_IncsDownstreamRqTotal(t *testing.T) {
	// Drive a single HTTP/1.1 GET / through the filter; assert
	// downstream_rq_total Inc'd by 1.
}

func TestFilter_ResponseFinalization_IncsStatusClassCounter(t *testing.T) {
	// Drive a request that produces a 200, 404, 500 in turn; assert each
	// _Nxx counter Inc'd appropriately.
}
```

In `actions_test.go`:

```go
func TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass(t *testing.T) {
	// Drive routerAction.do against a backend returning 200; assert
	// c.upstreamRqTotal Inc'd by 1 and c.upstreamRq2xx Inc'd by 1.
}

func TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass(t *testing.T) {
	// Same shape on the H2 path.
}
```

In `internal/filter/hcm/h2/router_action_test.go` (new file):

```go
package h2

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

// TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error covers the M-9
// carry-forward from 05.2 REVIEW: when h2RouterActionAdapter.WriteH2
// invokes doH2 and the call returns an error, a log line is emitted to
// the standard logger before the function returns.
func TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	adapter := &h2RouterActionAdapter{
		// Inject a fake action whose doH2 returns a sentinel error.
		// (Use the smallest in-package shim; the precise factoring
		// depends on the existing adapter struct's fields — Task 11
		// step 4 picks the shim shape based on what's already there.)
	}
	wantErr := errors.New("synthetic doH2 failure")
	// Drive WriteH2 with a fake StreamWriter that records the error path.
	// (The fake's exact shape is established in the test file's helper
	// section; ~30 LoC.)

	got := adapter.WriteH2(/* args per the existing interface */)
	if !errors.Is(got, wantErr) {
		t.Fatalf("WriteH2 returned %v, want %v", got, wantErr)
	}
	if !strings.Contains(buf.String(), "h2: doH2 error:") {
		t.Errorf("log output missing 'h2: doH2 error:' prefix; got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), wantErr.Error()) {
		t.Errorf("log output missing the underlying error string; got: %q", buf.String())
	}
}
```

NOTE: the M-9 test's test peer (the fake `doH2`-failure-injecting adapter) requires a small refactor friendliness in `h2RouterActionAdapter` — if `doH2` is currently a method on the adapter (not a function-typed field), the test substitutes via a build-time test hook. The simplest substitution shape: extract `doH2` as an unexported method `(*h2RouterActionAdapter).doH2(...)` and override via a function-typed field `doH2Fn func(...) error` defaulting to the method-bound version, with the test file constructing an adapter whose `doH2Fn` is the sentinel-failing function. Codify in Task 11 step 3.

Run: `go test ./internal/filter/hcm/ ./internal/filter/hcm/h2/ -v`
Expected: FAIL — the new tests reference symbols that don't exist yet.

- [ ] **Step 2: Modify `internal/filter/hcm/filter.go`** — Filter struct gains 5 metric fields; `NewFilter` allocates from the listener-captured Registry

```go
import (
	// existing
	"github.com/esalaine/envoy-go/internal/stats"
)

type Filter struct {
	// existing fields
	downstreamRqTotal *stats.Counter
	downstreamRq2xx   *stats.Counter
	downstreamRq3xx   *stats.Counter
	downstreamRq4xx   *stats.Counter
	downstreamRq5xx   *stats.Counter
}

func NewFilter(cfg *Config, clusters *cluster.Manager, registry *stats.Registry) (*Filter, error) {
	// existing parse + validation
	prefix := "http." + cfg.StatPrefix + "."
	f := &Filter{
		// existing field assigns
		downstreamRqTotal: registry.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   registry.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   registry.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   registry.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   registry.NewCounter(prefix + "downstream_rq_5xx"),
	}
	return f, nil
}

// downstreamStatusClassCounter selects the right _Nxx counter for an HTTP
// status code per the integer-divide code/100 discipline.
func (f *Filter) downstreamStatusClassCounter(code int) *stats.Counter {
	switch code / 100 {
	case 2:
		return f.downstreamRq2xx
	case 3:
		return f.downstreamRq3xx
	case 4:
		return f.downstreamRq4xx
	case 5:
		return f.downstreamRq5xx
	default:
		return nil // 1xx codes or out-of-range — see SPEC §2.1 (1xx omitted) + §11
	}
}
```

The HCM dispatch entry — the site BRAINSTORM §4.2 + SPEC §5.5 + `## Settled SPEC §12 deferred decisions` #1 settle as "site (a): on first byte of request line/headers in `connection.go`'s read loop" — adds `f.downstreamRqTotal.Inc()` immediately after a successful `ReadRequest` returns (or the equivalent first-byte hook in the connection-read loop). The HCM response hook adds (at the site where the response status code is finalized, before bytes hit the wire):

```go
if c := f.downstreamStatusClassCounter(resp.StatusCode); c != nil {
	c.Inc()
}
```

- [ ] **Step 3: Modify `internal/filter/hcm/actions.go`** — H1 + H2 router-action Inc

```go
// In routerAction.do (H1):
func (r *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) error {
	r.cluster.upstreamRqTotal.Inc()
	// existing dispatch logic that obtains `resp` from the upstream
	// ...
	if c := r.cluster.statusClassCounter(resp.StatusCode); c != nil {
		c.Inc()
	}
	// existing response writeback
}

// In routerActionH2.do (H2):
func (r *routerActionH2) do(ctx context.Context, req H2Request, w h2.StreamWriter) error {
	r.cluster.upstreamRqTotal.Inc()
	// existing DialH2 + RoundTrip logic
	// ...
	if c := r.cluster.statusClassCounter(resp.Status); c != nil {
		c.Inc()
	}
	// existing response writeback
}
```

(For the H1 502 path on dial failure, the cluster-side `statusClassCounter(502)` is consulted — the 5xx Inc lands on the dial-failure local-reply path too, since the cluster-scope counter reflects "what status-class came out of THIS cluster's dispatch".)

- [ ] **Step 4: Modify `internal/filter/hcm/h2/router_action.go`** — M-9 carry-forward log line

```go
// In h2RouterActionAdapter.WriteH2 (or whatever the existing symbol is named):
err := a.doH2(ctx, req, sw) // existing call
if err != nil {
	log.Printf("h2: doH2 error: %v", err)
	return err
}
return nil
```

Refactor friendliness for the M-9 test: extract `doH2` as a function-typed field `doH2Fn func(...) error` with default-binding to the existing method, so the test can substitute. ~5 LoC of struct-field plumbing.

- [ ] **Step 5: Run tests to verify pass**

```bash
go test -race ./internal/filter/hcm/ ./internal/filter/hcm/h2/ -v
go test -race ./... -count=1 -short
```

Expected: every existing test PLUS the new hot-path tests + the M-9 test PASS.

- [ ] **Step 6: Append Task 11 entry to PROGRESS.md and Commit**

The PROGRESS Task 11 entry records the M-9 carry-forward landing alongside the standard HCM-Inc-wiring notes; cite the 05.2 REVIEW M-9 finding as the SOURCE of the carry-forward.

```bash
git add internal/filter/hcm/filter.go internal/filter/hcm/actions.go internal/filter/hcm/h2/router_action.go internal/filter/hcm/h2/router_action_test.go internal/filter/hcm/filter_test.go internal/filter/hcm/actions_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: HCM Inc wiring (downstream_rq + upstream_rq status-class) + M-9 log line carry-forward"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #3, §1 #6, §4.2 (filter.go + actions.go + router_action.go extensions), §5.5 (Increment paths table — HCM dispatch entry / response hook / H1+H2 router-action), §6 (5 HCM names + 4 of the 8 cluster names), §11.3 (filter_test + actions_test extensions), §11.4 (M-9 unit test), §12 #1 (HCM Inc hook site (a)), §12 #5 (M-9 test file location), §13.1 (M-9 carry-forward bundle).*

---

## Task 12: `cmd/envoy-go/main.go` Registry threading + `Freeze()` boot ordering

**Files:**
- Modify: `cmd/envoy-go/main.go` (thread `bs.Stats` into all three managers + admin; call `bs.Stats.Freeze()` after admin starts AND after listener manager begins accepting)
- Modify: `cmd/envoy-go/main_test.go` (bootstrap-variant smoke tests thread the Registry)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 12 entry)

The boot wiring per SPEC §5.4 + `## Settled SPEC §12 deferred decisions` #4. The `Freeze()` call lands AFTER `lm.Start(ctx)` returns and AFTER `admSrv.MarkReady()` so all NewCounter/NewGauge calls precede the freeze.

- [ ] **Step 1: Write the failing test (in `main_test.go`)**

The existing `cmd/envoy-go/main_test.go` smoke tests boot the binary against several bootstrap fixtures. Extend the smallest one to additionally GET `/stats/prometheus` after readiness and assert the body contains `# HELP envoy_server_live` (verifying that the Registry was threaded through all the way).

```go
func TestMain_StatsPrometheusEndpointResponds(t *testing.T) {
	// existing per-bootstrap setup
	// ...
	resp, err := http.Get("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		t.Fatalf("GET /stats/prometheus: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("# HELP envoy_server_live")) {
		t.Errorf("body missing # HELP envoy_server_live\n--- body ---\n%s", body)
	}
}
```

Run: `go build ./cmd/envoy-go && go test ./cmd/envoy-go/ -v`
Expected: FAIL — main.go doesn't yet thread the Registry; previous-task call-site widenings have left main.go uncompilable until this task.

- [ ] **Step 2: Modify `cmd/envoy-go/main.go`** — thread `bs.Stats` into every manager + admin; call `Freeze()`

```go
func main() {
	// existing flag + config-load
	// bs is now *bootstrap.Bootstrap (or whatever Task 7 settled the wrapper as);
	// bs.Stats is the *stats.Registry pre-allocated by bootstrap.Load.

	adminHost, adminPort, err := bootstrap.AdminSocket(bs.Proto)
	if err != nil {
		log.Fatalf("extract admin: %v", err)
	}
	adminAddr := fmt.Sprintf("%s:%d", adminHost, adminPort)

	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath), bs.Stats)
	if err != nil {
		log.Fatalf("cluster manager: %v", err)
	}

	admSrv := admin.New(adminAddr, bs.Stats)
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats)
	if err != nil {
		log.Fatalf("listener manager: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := lm.Start(ctx); err != nil {
		log.Fatalf("listener start: %v", err)
	}
	defer lm.Stop()

	admSrv.MarkReady()

	// LBP-1: all NewCounter / NewGauge calls have completed (admin allocated
	// server.live; cluster manager allocated 8×N cluster metrics; listener
	// manager allocated 2×M listener metrics; HCM filter-build allocated
	// 5×K HCM metrics inside listener.NewManagerWithBaseDirAndAllowH2C).
	// Freeze the registry — post-Freeze NewCounter/NewGauge calls panic.
	bs.Stats.Freeze()

	for _, info := range lm.Listeners() {
		_, _ = fmt.Fprintf(os.Stdout, "envoy-go listener %s ready on %s\n", info.Name, info.Addr)
	}
	_, _ = fmt.Fprintln(os.Stdout, "envoy-go ready")

	<-ctx.Done()
}
```

(Adapt the `bs.Proto` accessor to whatever shape Task 7 settled `bootstrap.Load`'s return type into. If Task 7 kept `Load` returning `*bootstrapv3.Bootstrap` directly with a separate `*stats.Registry` allocated next to it, threading is `cluster.NewManagerWithBaseDir(bs, dir, registry)` etc.)

- [ ] **Step 3: Update `main_test.go` bootstrap variants** — every previously-existing variant's call site widens to thread the Registry; the new TestMain_StatsPrometheusEndpointResponds case is added on the smallest bootstrap variant.

- [ ] **Step 4: Run tests to verify pass**

```bash
go build ./cmd/envoy-go
go test -race ./... -count=1
```

Expected: every package compiles and tests PASS, including the new main test and the cumulative cross-package integration.

- [ ] **Step 5: Append Task 12 entry to PROGRESS.md and Commit**

```bash
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: main.go boot wiring (Registry thread + Freeze post-listener-Start)"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §4.2 (`main.go` extension), §5.3 (LBP-1), §5.4 (boot wiring sequence — verbatim ordering), §12 #4 (filter-build pre-Freeze verification).*

---

## Task 13: `internal/stats/fuzz_test.go` `FuzzPromTextFormat` at 30s ADR-0018 budget

**Files:**
- Create: `internal/stats/fuzz_test.go`
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 13 entry)

Per SPEC §11.8 + BRAINSTORM §6.4 + `## Settled SPEC §12 deferred decisions` #11. Fuzzes adversarial label-value strings (the Registry's name regex prevents adversarial stat names from reaching `WriteProm`, so the fuzzer constructs synthetic Metrics with valid names but adversarial label values via the synthetic-Metric injection technique from Task 5's prom_test). No new ADR — per ADR-0042 precedent that fuzzers do not require their own ADR.

- [ ] **Step 1: Write `fuzz_test.go`**

```go
package stats

import (
	"bytes"
	"strings"
	"testing"
)

// FuzzPromTextFormat fuzzes the Prometheus text-format writer with adversarial
// label values. Per BRAINSTORM §6.4, escaping bugs are the most likely class
// of bug in the writer, and Prometheus's parser is strict. The fuzzer asserts
// no panic AND that the output round-trips through a tiny in-test Prometheus
// parser without error (no third-party Prom library — keeps the fuzzer
// consistent with ADR-0059 / D-3.2).
func FuzzPromTextFormat(f *testing.F) {
	seeds := []string{
		"plain",
		`with "quotes"`,
		`with\backslash`,
		"with\nnewline",
		"\x00null-byte",
		"trailing=equals",
		"x" + strings.Repeat("y", 4096), // very long
		"unicode-é-snowman-☃",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, labelValue string) {
		r := NewRegistry()
		// Use the synthetic-Metric injection from prom_test.go; the regular
		// NewCounter/NewGauge paths reject adversarial names but the writer
		// must not panic on adversarial label values arriving via the
		// (in production) listener.<addr> tag-extraction path.
		r.metrics = append(r.metrics, &synthMetric{
			name:   "listener.adv_addr.downstream_cx_total",
			mtype:  MetricCounter,
			format: "1",
		})
		r.byName["listener.adv_addr.downstream_cx_total"] = r.metrics[0]

		var buf bytes.Buffer
		if err := WriteProm(&buf, r); err != nil {
			t.Fatalf("WriteProm panic-or-error on label %q: %v", labelValue, err)
		}
		// Round-trip parse: every non-empty non-comment line must match
		//   <name>{<key>="<value>",...} <value>
		// or
		//   <name> <value>
		for _, line := range strings.Split(buf.String(), "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !validatePromLine(line) {
				t.Errorf("malformed Prom line %q from label-value seed %q", line, labelValue)
			}
		}
	})
}

// validatePromLine implements a minimal Prometheus-format check. ~30 LoC.
// Returns true if line matches the Prometheus exposition grammar.
func validatePromLine(line string) bool {
	// Trim trailing space (none expected after the value).
	// Find the last space — it separates the labels-block (or name) from
	// the value. Verify the value is a valid integer/float.
	idx := strings.LastIndex(line, " ")
	if idx < 0 {
		return false
	}
	head := line[:idx]
	value := line[idx+1:]
	// Value must be parseable as int or float (we only emit ints in 06.1
	// but the parser is generous).
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return false
	}
	// Head is either <name> or <name>{<labels>}.
	if !strings.Contains(head, "{") {
		return nameRE.MatchString(head)
	}
	open := strings.Index(head, "{")
	close := strings.LastIndex(head, "}")
	if close < open {
		return false
	}
	name := head[:open]
	labels := head[open+1 : close]
	if !nameRE.MatchString(name) {
		return false
	}
	// labels: comma-separated key="value" pairs (escaped).
	for _, pair := range splitLabelsRespectingEscapes(labels) {
		eq := strings.Index(pair, "=")
		if eq < 0 {
			return false
		}
		k := pair[:eq]
		v := pair[eq+1:]
		if !nameRE.MatchString(k) {
			return false
		}
		if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) {
			return false
		}
	}
	return true
}

// splitLabelsRespectingEscapes splits "a=\"x\",b=\"y\"" into ["a=\"x\"","b=\"y\""],
// respecting backslash-escapes inside quoted values.
func splitLabelsRespectingEscapes(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	prevBackslash := false
	for _, r := range s {
		switch {
		case r == '\\' && !prevBackslash:
			prevBackslash = true
			cur.WriteRune(r)
			continue
		case r == '"' && !prevBackslash:
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
		prevBackslash = false
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
```

(Add `strconv` import for the float-parse.)

- [ ] **Step 2: Run the fuzzer at the 30s ADR-0018 budget**

```bash
go test -race ./internal/stats/ -fuzz=FuzzPromTextFormat -fuzztime=30s
```

Expected: no failures; the seed corpus exercises every known escape path; new corpus entries that the fuzzer discovers are kept under `internal/stats/testdata/fuzz/FuzzPromTextFormat/` per Go's native fuzzer convention.

- [ ] **Step 3: Run all existing fuzzers to confirm no regressions**

Per SPEC §11.8 the existing six fuzzers re-run at the 30s budget:

```bash
go test -race ./internal/bootstrap/      -fuzz=FuzzBootstrapLoad     -fuzztime=30s
go test -race ./internal/filter/tcpproxy/ -fuzz=FuzzTcpProxyFilter    -fuzztime=30s
go test -race ./internal/tls/             -fuzz=FuzzTLSContextParse   -fuzztime=30s
go test -race ./internal/filter/hcm/      -fuzz=FuzzHCMConfigParse    -fuzztime=30s
go test -race ./internal/filter/hcm/h2/   -fuzz=FuzzFrameStream       -fuzztime=30s
go test -race ./internal/filter/hcm/h2/   -fuzz=FuzzHPACKDecode       -fuzztime=30s
go test -race ./internal/stats/           -fuzz=FuzzPromTextFormat    -fuzztime=30s
```

Total fuzzer count post-06.1: 7. Expected: every fuzzer runs clean.

- [ ] **Step 4: Append Task 13 entry to PROGRESS.md and Commit**

The PROGRESS Task 13 entry includes the verbatim outputs of all seven fuzzer runs (last 5 lines each).

```bash
git add internal/stats/fuzz_test.go docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: FuzzPromTextFormat (adversarial label-value escaping; 30s budget)"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §11.8 (fuzzer enumeration + total post-06.1 = 7), §12 #11 (`## Settled SPEC §12 deferred decisions` #11 fuzzer scope), §14 (FuzzPromTextFormat acceptance bullet).*

---

## Task 14: Differential fixture `test/differential/0005-prometheus-stats/` + runner registration [ADR-0062]

**Files:**
- Create: `test/differential/0005-prometheus-stats/envoy-go.yaml`
- Create: `test/differential/0005-prometheus-stats/envoy.yaml`
- Create: `test/differential/0005-prometheus-stats/expectations.yaml`
- Create: `test/differential/0005-prometheus-stats/README.md`
- Create: `test/differential/0005-prometheus-stats/driver/driver.go`
- Create: `test/differential/0005-prometheus-stats/driver/driver_test.go`
- Create: `test/differential/0005-prometheus-stats/backends/main.go`
- Modify: `test/differential/runner.go` (blank-import the driver package; per-fixture loop calls the in-band scrape hooks)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0062)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (append Task 14 entry)

The project's first observability-surface differential and first non-vacuous gate-(a) on the observability surface. ADR-0062 (Differential equivalence shape for stats) lands here.

- [ ] **Step 1: Author `envoy-go.yaml`** — subject bootstrap

```yaml
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_h1
      address: { socket_address: { address: 127.0.0.1, port_value: 0 } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { path: /missing }
                          direct_response: { status: 404, body: { inline_string: "not found\n" } }
                        - match: { prefix: / }
                          route: { cluster: c0 }
  clusters:
    - name: c0
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c0
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: 127.0.0.1, port_value: 0 } }  # filled by harness
```

- [ ] **Step 2: Author `envoy.yaml`** — reference bootstrap (STRICT_DNS per ADR-0010 + `--concurrency 1` per ADR-0028)

Same listener / HCM / route_config shape; cluster `c0` is `type: STRICT_DNS` pointing at `host.docker.internal:<backend-port>` with `dns_lookup_family: V4_ONLY`. The runner invokes Envoy with `--concurrency 1`.

- [ ] **Step 3: Author `expectations.yaml`** (verbatim from SPEC §7.3)

```yaml
# Phase 06.1 expectations: per-counter delta-equality, per-gauge snapshot-equality
# under the 5-request workload [200, 200, 404, 200, 502].
counters:
  - name: http.ingress_http.downstream_rq_total
    delta: 5
  - name: http.ingress_http.downstream_rq_2xx
    delta: 3
  - name: http.ingress_http.downstream_rq_3xx
    delta: 0
  - name: http.ingress_http.downstream_rq_4xx
    delta: 1
  - name: http.ingress_http.downstream_rq_5xx
    delta: 1
  - name: cluster.c0.upstream_rq_total
    delta: 4   # 404 is HCM-direct, never reaches cluster
  - name: cluster.c0.upstream_rq_2xx
    delta: 3
  - name: cluster.c0.upstream_rq_3xx
    delta: 0
  - name: cluster.c0.upstream_rq_4xx
    delta: 0
  - name: cluster.c0.upstream_rq_5xx
    delta: 1
  - name: listener.<addr>.downstream_cx_total
    delta_min: 1   # keepalive may collapse
  - name: cluster.c0.upstream_cx_total
    delta_min: 1
gauges:
  - name: listener.<addr>.downstream_cx_active
    after: 0
  - name: cluster.c0.upstream_cx_active
    after: 0
  - name: cluster.c0.membership_total
    after: 1
  - name: server.live
    after: 1
allow_list_discipline: |
  Any Prometheus metric NAME not in this 17-name table is ignored by the
  differential. HELP-text values are ignored (Rule SN6).
```

- [ ] **Step 4: Author `README.md`**

Single page: purpose (first observability-surface differential; first non-vacuous gate-(a) on observability), STATIC-vs-STRICT_DNS divergence (same as 0001/0002/0003/0004), 5-request defined-load shape, per-counter-delta-equality + per-gauge-snapshot-equality assertion shape, cross-reference to `BEHAVIOR_CONTRACT.md ## Stat-name mapping`'s 17 names + Rules SN1–SN8.

- [ ] **Step 5: Author `backends/main.go`**

Small Go program: HTTP/1.1 server on a configurable port; reads `X-Backend-Status` header; returns the requested status code with body `bad gateway\n` for 502 / `OK\n` for 200 / honors arbitrary status per the header. ~80 LoC.

- [ ] **Step 6: Author `driver/driver.go`**

```go
package driver

import (
	"context"
	"net/http"
	// ... per the existing fixture-0003/0004 driver shape

	"github.com/esalaine/envoy-go/test/differential/runner"
)

// Driver implements the runner.Driver interface for fixture 0005.
type Driver struct{}

func init() { runner.Register("0005-prometheus-stats", &Driver{}) }

func (d *Driver) BackendCount() int { return 1 }
func (d *Driver) SubjectListenerName() string { return "l_h1" }

// DriveSubject and DriveReference are SYMMETRIC: both invoke driveOne which
// (a) scrapes /stats/prometheus and parses the 17-name allow-list,
// (b) sends 5 H1 requests with target statuses [200,200,404,200,502],
// (c) scrapes again and parses, (d) returns (before, after) snapshots.
func (d *Driver) DriveSubject(ctx context.Context, addr string, adminAddr string) (DriverResult, error) {
	return driveOne(ctx, addr, adminAddr)
}
func (d *Driver) DriveReference(ctx context.Context, addr string, adminAddr string) (DriverResult, error) {
	return driveOne(ctx, addr, adminAddr)
}

type DriverResult struct {
	Before, After Snapshot
}

type Snapshot struct {
	// 17-name allow-listed values (counters as uint64; gauges as int64).
	HCMRqTotal, HCMRq2xx, HCMRq3xx, HCMRq4xx, HCMRq5xx uint64
	ClusterRqTotal, ClusterRq2xx, ClusterRq3xx, ClusterRq4xx, ClusterRq5xx uint64
	ListenerCxTotal, ClusterCxTotal uint64
	ListenerCxActive, ClusterCxActive, ClusterMembership, ServerLive int64
}

func driveOne(ctx context.Context, addr, adminAddr string) (DriverResult, error) {
	before, err := scrapeAndParse(ctx, adminAddr)
	if err != nil { return DriverResult{}, err }

	// 5 sequential H1 requests
	for _, req := range fiveRequestPlan() {
		_, err := http.DefaultClient.Do(req)
		if err != nil { return DriverResult{}, err }
	}

	after, err := scrapeAndParse(ctx, adminAddr)
	if err != nil { return DriverResult{}, err }
	return DriverResult{Before: before, After: after}, nil
}

func fiveRequestPlan() []*http.Request {
	// Build 5 requests:
	//   GET / (200 from cluster c0 backend)
	//   GET / (200)
	//   GET /missing (404 HCM-direct)
	//   GET / (200)
	//   GET / with X-Backend-Status: 502 (502 explicit from backend)
	// The driver knows the listener bind addr and the X-Backend-Status header.
}

func scrapeAndParse(ctx context.Context, adminAddr string) (Snapshot, error) {
	// HTTP GET /stats/prometheus; parse the body line-by-line; extract the 17 names; populate Snapshot.
}

// AssertStatsEquivalence is the in-band assertion entry-point the runner
// invokes per `## Settled SPEC §12 deferred decisions` #6. Per ADR-0062:
// per-counter delta_envoy_go == delta_envoy; per-gauge after_envoy_go ==
// after_envoy. HELP text ignored; non-listed names ignored.
func AssertStatsEquivalence(t Testing, subject, reference DriverResult) {
	// Counter delta-equality on the 17 names (cx_total / rq_total uses delta_min ≥ 1)
	// Gauge snapshot-equality
	// Failure messages name the diverging metric and quote both sides' values.
}

type Testing interface { Errorf(format string, args ...any); Fatalf(format string, args ...any) }
```

(The exact integration with the existing runner — interface methods, init pattern, AssertStatsEquivalence signature — mirrors the fixture-0004 in-band pattern from 05.2; the planner inspects `test/differential/runner.go`'s existing fixture-0004 registration at Task 14 step 7 execution time and matches the shape.)

- [ ] **Step 7: Author `driver/driver_test.go`**

Unit tests for `scrapeAndParse` against canned exposition strings; unit tests for `AssertStatsEquivalence` with synthetic snapshots that pass / fail / report-good-error-message. ~80 LoC.

- [ ] **Step 8: Modify `test/differential/runner.go`** — blank-import the new fixture-0005 driver package

Single-line addition; the driver's `init()` calls `runner.Register("0005-prometheus-stats", &Driver{})` so the runner's per-fixture loop discovers the new fixture automatically. Per `## Settled SPEC §12 deferred decisions` #6 (in-band, no generic `StatsExpectations` Driver-interface extension): the runner's per-fixture loop calls a fixture-specific `AssertStatsEquivalence(t, subject, reference)` exported from the driver package after the standard differential checks complete.

- [ ] **Step 9: Run the fixture**

```bash
go test ./test/differential/ -run Test.*0005 -v
```

Expected: PASS. The 5-request workload runs against both proxies; per-counter delta-equality and per-gauge snapshot-equality hold across the 17 names.

- [ ] **Step 10: Append ADR-0062 to `docs/envoy-go/DECISIONS.md`**

Per the summary in `## ADRs introduced by this plan`. ADR-0062's Consequences (c) explicitly notes that `BEHAVIOR_CONTRACT.md ## Equivalence Matrix`'s row 19 seed-row is superseded by the new "Stats output" row at Task 15.

- [ ] **Step 11: Append Task 14 entry to PROGRESS.md and Commit**

```bash
git add test/differential/0005-prometheus-stats/ test/differential/runner.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: fixture 0005-prometheus-stats + runner registration [ADR-0062]"
```

SHA-fill follow-up commit per the established convention.

*Anchored: SPEC §1 #7, §3 (gate (a) non-vacuous), §4.3 (fixture inventory), §6 (17-name allow-list source-of-truth), §7 (fixture spec), §8 (ADR-0062), §11.5 (driver test coverage), §12 #6 (in-band pattern), §14 (fixture acceptance bullet).*

---

## Task 15: BEHAVIOR_CONTRACT in-place edit + ROADMAP/STATE updates + closing all-gates sweep

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place: `## Stat-name mapping` populated; `## Equivalence Matrix` row added)
- Modify: `docs/envoy-go/ROADMAP.md` (row 06.1 → done; rows 06 + 06.2 status confirmation)
- Modify: `docs/envoy-go/STATE.md` (lifecycle-state 4)
- Modify: `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md` (closing entry with all-gates output)

The closing sweep. The BEHAVIOR_CONTRACT in-place edit lands at this commit per ADR-0052's authorisation (per SPEC §10 + §4.4); ROADMAP row updates anticipate the REVIEW session's phase-done commit (per the 05.2 PLAN's Task 15 NOTE convention).

- [ ] **Step 1: Edit `BEHAVIOR_CONTRACT.md ## Stat-name mapping`** in place

The existing placeholder (lines ~48–53) is:

```
## Stat-name mapping

_to be filled per-phase as needed._

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.
```

Replace with the populated subsection: the introductory paragraph (kept verbatim), then the eight rules SN1–SN8 verbatim from SPEC §10.1 (including Rule SN4's verbatim Envoy-scrape evidence block + the negative-confirmation grep statement + the regex-source citation), then the 17-name table from SPEC §6 verbatim:

```
## Stat-name mapping

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.

### Flattening rules SN1–SN8 (per ADR-0061; introduced by phase 06.1)

[insert SPEC §10.1's verbatim block — SN1 through SN8 with the Rule SN4 evidence section]

### 17-name table (introduced by phase 06.1)

[insert SPEC §6's verbatim table — 2 listener + 5 HCM + 8 cluster + 2 server (one EMITTED, one explicitly NOT-EMITTED)]

### Twin-series filter discipline (per empirical-verification scrape)

[insert SPEC §6's twin-series filter footnote]
```

- [ ] **Step 2: Edit `BEHAVIOR_CONTRACT.md ## Equivalence Matrix`** in place

The existing matrix at lines 9–23 has a row at line 19:

```
| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |
```

Per SPEC §10.2 + ADR-0062 Consequences (c), supersede this row with:

```
| Stats output | Per-stat behavioral delta after defined load is equal between envoy-go and reference Envoy. Gauges are snapshot-equal after drain. Names + label keys + types byte-equal; HELP text ignored. Allow-list: 17 stats listed in § Stat-name mapping. All other Envoy stat names in /stats/prometheus output are ignored by the differential. |
```

(The supersession is in-place; the new row replaces the old at the same line index. The `## Equivalence Matrix` table-header is unchanged.)

- [ ] **Step 3: Six-gate local sweep** — run all gates per BOOTSTRAP §7.5 and SPEC §3

```bash
# Gate (a): new fixture green
go test ./test/differential/ -run Test.*0005 -v

# Gate (b): pre-existing fixtures still green
go test ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004' -v

# Gate (c): conformance suites pass (h2spec at the ADR-0051 pin)
go test ./test/conformance/h2spec/ -v

# Gate (d): fuzzers run clean for CI short-budget (the seven, one of which is new)
go test -race ./internal/bootstrap/      -fuzz=FuzzBootstrapLoad     -fuzztime=30s
go test -race ./internal/filter/tcpproxy/ -fuzz=FuzzTcpProxyFilter    -fuzztime=30s
go test -race ./internal/tls/             -fuzz=FuzzTLSContextParse   -fuzztime=30s
go test -race ./internal/filter/hcm/      -fuzz=FuzzHCMConfigParse    -fuzztime=30s
go test -race ./internal/filter/hcm/h2/   -fuzz=FuzzFrameStream       -fuzztime=30s
go test -race ./internal/filter/hcm/h2/   -fuzz=FuzzHPACKDecode       -fuzztime=30s
go test -race ./internal/stats/           -fuzz=FuzzPromTextFormat    -fuzztime=30s

# Gate (e): vet/lint/test
go vet ./...
golangci-lint run ./...
go test -race ./...
```

Expected: every gate green (a/b/c/d/e). Gate (f) — REVIEW.md approval — is the next session's responsibility, deferred per BOOTSTRAP §5 step 6.

- [ ] **Step 4: ADR-0059 boundary grep + no-third-party-Prom-or-stats check**

```bash
# Per ADR-0059 + SPEC §14 acceptance bullet "No third-party Prometheus dependency".
grep -nE 'github.com/prometheus|github.com/[^/]+/prometheus' go.mod go.sum
```

Expected: NO matches in go.mod (matches in go.sum may be transitive — verify each is NOT pulled by an `internal/...` import path via `go mod why`).

```bash
# Per SPEC §14 final acceptance bullet "No third-party stats library is imported".
grep -nR '"github.com/[^"]*\(prometheus\|stats\|metrics\|expvar\)' internal/ cmd/envoy-go/ --include='*.go' | grep -v '_test.go' | grep -v 'github.com/esalaine/envoy-go/internal/stats'
```

Expected: empty output.

- [ ] **Step 5: All-17-stat-emit-call-sites grep**

Per SPEC §14 acceptance bullet:

```bash
# 2 listener
grep -nE 'downstreamCxTotal\.Inc|downstreamCxActive\.(Inc|Dec)' internal/listener/
# 5 HCM
grep -nE 'downstreamRqTotal\.Inc|downstreamRq[2-5]xx\.Inc' internal/filter/hcm/
# 8 cluster
grep -nE 'upstreamRqTotal\.Inc|upstreamRq[2-5]xx\.Inc|upstreamCxTotal\.Inc|upstreamCxActive\.(Inc|Dec)|membershipTotal\.Set' internal/cluster/
# 1 server.live
grep -nE 'liveGauge\.Set' internal/admin/
```

Expected: exactly 2 / 5 / 8 / 1 distinct call sites (per SPEC §14 bullet "All 17 stat-emit call sites are grep-verifiable: 2 in `internal/listener/listener.go`'s accept loop, 5 in `internal/filter/hcm/filter.go` + `actions.go`, 8 in `internal/cluster/`'s manager/dial paths, 1 in `internal/admin/admin.go`'s `handleReady`, plus the implicit "NOT-EMITTED" `server.uptime` enforced by absence in the codebase").

- [ ] **Step 6: Update STATE.md to lifecycle-state 4 (verification-pending)**

```yaml
- active-phase: 06.1-stats-prometheus
- phase-directory: docs/envoy-go/phases/06.1-stats-prometheus/
- lifecycle-state: 4   # implementation complete; verification not yet run
- next-skill: superpowers:verification-before-completion
- next-skill-scope: <verify all six gates per BOOTSTRAP §7.5 / SPEC §3>
- last-commit: <Task 15 commit SHA>
- last-updated: <date>
```

- [ ] **Step 7: Anticipated ROADMAP row updates (lands at the phase-done commit, NOT at Task 15)**

Per BOOTSTRAP §5 step 6: the phase-done commit (lifecycle-state 6) is owned by the REVIEW session, NOT by Task 15. Task 15 advances STATE to lifecycle-state 4 (implementation complete; verification pending). The verification session re-runs the gates (state 5) and the REVIEW session writes REVIEW.md (state 6 — phase-done). The anticipated ROADMAP-row text (to land at the phase-done commit per SPEC §4.4):

```markdown
| 06   | observability-baseline | 05 | in-progress |  | Sub-phases 06.1 (stats) + 06.2 (access-log). Closes only at 06.2's phase-done. |
| 06.1 | stats-prometheus | 05  | done         |  | In-tree stats Registry + /stats/prometheus exposition + 17 call sites; first non-vacuous observability-surface differential (fixture 0005). LBP-1 invariant; no third-party Prometheus dependency. ADR-0059..ADR-0064. |
| 06.2 | access-log       | 06.1 | planned     |  | Access-log subsystem; closes parent 06 at its phase-done. |
```

The PROGRESS Task 15 entry records "ROADMAP rows 06.1 still `in-progress` at this commit; the phase-done commit at lifecycle-state 6 will flip it to `done` and confirm row 06 stays `in-progress` and row 06.2 stays `planned`." Refinement: Task 15 advances STATE to lifecycle-state 4 only.

- [ ] **Step 8: Append Task 15 closing entry to PROGRESS.md (with verification block)**

The PROGRESS Task 15 entry is the session's "verification proof" — `superpowers:verification-before-completion` reads it when phase 06.1 moves to lifecycle-state 5. Keep every last-30-lines block verbatim. Mirror the phase-04 / 05.1 / 05.2 PROGRESS Task-N closing entry shape.

The entry includes:
- Each gate's command + last-30-lines output verbatim.
- ADR-0059 boundary grep result (no third-party Prom or stats library).
- 17-call-site grep results.
- The carry-forward triage log: M-9 RESOLVED-IN-06.1 (cite Task 11 commit); M-4 / M-10 / M-12 continue-DEFERRED per the matrix in this PLAN's `## Phase-05.2 REVIEW carry-forward resolution matrix`.
- Six-ADR landing summary: ADR-0059..ADR-0064 anchoring tasks (Task 2 / Task 2 / Task 4 / Task 4 / Task 8 / Task 14) + commit SHAs.

- [ ] **Step 9: Commit**

The phase-done commit message per BOOTSTRAP §5.3 names every ADR introduced or referenced:

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md
git commit -m "phase 06.1: BEHAVIOR_CONTRACT in-place edit + all-gates green local sweep (a/b/c/d/e green; f deferred to REVIEW) [ADR-0059, ADR-0060, ADR-0061, ADR-0062, ADR-0063, ADR-0064]"
```

(The commit-message-completeness check from BOOTSTRAP §5.3 is satisfied by the bracketed ADR list. Both ROADMAP.md and the parent ROADMAP row are deliberately NOT included in this commit's `git add` — those updates land at the REVIEW session's phase-done commit.)

- [ ] **Step 10: Confirm phase-06.1 readiness for state-5 transition (do NOT advance STATE — that's the verification session per BOOTSTRAP §5)**

The implementation session ends with Task 15 committed on `phase/06.1-stats-prometheus-impl`. STATE advancement through 5 → 6 is per-session work, not this task's responsibility.

*Anchored: SPEC §1 #4 (BEHAVIOR_CONTRACT in-place edit), §3 (six-gate phase-done), §4.4 (ROADMAP/STATE/PROGRESS lifecycle), §10 (BEHAVIOR_CONTRACT additions), §14 (full acceptance checklist), and BOOTSTRAP §5.3 (commit-message-completeness), §7.5 (six-gate sweep).*

---

## Refinement

This section absorbs the conventions that the 05.2 PLAN's Refinement section codified for execution-time consistency. Every item below applies to phase 06.1 unless explicitly noted.

**SHA-fill follow-up convention (per phase-02 / 03 / 04 / 05.1 / 05.2 precedent).** Every task's commit lands the task's main change; immediately after, a follow-up tiny commit `phase 06.1: PROGRESS SHA-fill for Task N` updates that task's PROGRESS.md `**Commits:**` line with the just-landed short SHA. The follow-up commit's body is empty; its title is the only line. Two commits per task; the executor MUST NOT skip the follow-up. (The 05.2 PROGRESS at `bd75c88` shows the convention applied 15 times across Tasks 1–15.)

**BEHAVIOR_CONTRACT in-place edit lands at the phase-done commit (per ADR-0052).** The `## Stat-name mapping` placeholder population + the `## Equivalence Matrix` row supersession both land at Task 15's commit, NOT at any earlier task's commit. Per ADR-0052 the in-place edit is authorised; per SPEC §4.4 the timing is "at the phase-done commit". The PROGRESS Task 15 entry quotes the resulting `git diff` for the BEHAVIOR_CONTRACT.md edit verbatim so the verification session can grep-check the edit landed at the right commit. (Per the 05.1 + 05.2 PLANs' identical convention.)

**M-9 piggy-back rationale.** The 05.2 REVIEW Minor M-9 carry-forward (`h2RouterActionAdapter.WriteH2` missing log line on `doH2` error path) lands in Task 11 alongside the HCM-Inc-wiring even though M-9 is mechanically unrelated to the stats subsystem. Rationale per SPEC §13.1 + BRAINSTORM §2.4: the surface (`log.Printf` to stderr) matches what 06.1 introduces (the same `log.Printf` pattern is used by `internal/admin/prometheus.go`'s `WriteProm` error path); the 05.2 REVIEW explicitly deferred M-9 to "phase-06 observability when logging/metrics surface lands"; bundling at Task 11 keeps the carry-forward log auditable in one place (the PROGRESS Task 11 entry cites the 05.2 REVIEW M-9 finding as the source). The bundled landing is documented in the Task 15 PROGRESS closing entry's carry-forward triage log.

**ROADMAP row 06.1 → in-progress at the SPEC commit (already landed); → done at the phase-done commit.** Per BOOTSTRAP §4.1 invariant 3: at the SPEC commit (already landed at `be99b42`, before this PLAN commit), row 06.1 flipped `planned → in-progress` — the SPEC-authoring session did this. Per SPEC §4.4: at the phase-done commit (the REVIEW session's lifecycle-state-6 commit, NOT Task 15), row 06.1 flips `in-progress → done`. Row 06 (parent) stays `in-progress` after 06.1's phase-done — the parent only flips to `done` at 06.2's phase-done (mirroring the 05/05.1/05.2 closure pattern per SPEC §4.4). Row 06.2 stays `planned` until 06.2's SPEC drafts. Task 15's commit deliberately does NOT touch ROADMAP.md; the anticipated text is recorded in the PROGRESS Task 15 entry but lands at the REVIEW session's phase-done commit. (Mirrors the 05.2 PLAN's Task 15 NOTE.)

**ADR-numbering monotonicity discipline (ADR-0059..ADR-0064 contiguous).** Per ADR-0004's autonomous-numbering rule, the planner verified at PLAN-write time that the DECISIONS.md tail is `ADR-0058`; phase 06.1's six ADRs land at ADR-0059..ADR-0064 (contiguous block). Per `## ADRs introduced by this plan` above, the commit-time ordering (Task 2 / Task 4 / Task 8 / Task 14) produces the in-task pairing (ADR-0059 + ADR-0060 at Task 2; ADR-0061 + ADR-0064 at Task 4; ADR-0063 at Task 8; ADR-0062 at Task 14) — a non-monotonic ADR-number-vs-commit-order in the second half (0059, 0060, 0061, 0064, 0063, 0062), permitted per SPEC §8 and the 05.2 ADR-0055..ADR-0058 precedent. The contiguous-block discipline is preserved (no gaps); each ADR's `Lands-in-task` field records the in-task anchoring. The Task 1 step 1 precondition re-verifies the tail; if ADR-0058 has been superseded by a mid-PLAN-authoring ADR, every task's ADR reference shifts uniformly (planner verified at PLAN-write time that no such ADR exists; the precondition re-check is defence-in-depth).

**Commit-message-completeness check (per BOOTSTRAP §5.3).** Each task's commit message names the ADR(s) introduced in that task (in `[ADR-NNNN]` square-bracket form per the phase-04/05.1/05.2 convention). The Task 15 phase-done commit (per Step 9) names ALL SIX ADRs in the bracketed list — `[ADR-0059, ADR-0060, ADR-0061, ADR-0062, ADR-0063, ADR-0064]` — so a `git log --grep='ADR-006[0-4]'` query surfaces every authoring task plus the closing task. The PROGRESS Task 15 entry quotes the phase-done commit message verbatim so the verification session can grep-confirm.

**Six-gate local sweep at Task 15 (per BOOTSTRAP §7.5; SPEC §3).** Gates (a) / (b) / (c) / (d) / (e) all run at Task 15; gate (f) defers to REVIEW. The PROGRESS Task 15 entry quotes each gate's last-30-lines output verbatim. The Task-15 step 4 boundary grep + the step 5 17-call-site grep are SPEC §14-anchored acceptance bullets that the verification session re-runs. (Mirrors the 05.2 PLAN's Task-15 closing-sweep shape.)

**Synthetic-Metric-injection technique for the writer's escape tests.** The fuzz target (Task 13) and the prom_test escape test (Task 5) construct a Registry with synthetic Metric implementations (NOT through the regular `NewCounter`/`NewGauge` paths, since those reject adversarial names by design) — see `## Settled SPEC §12 deferred decisions` #11. The synthetic-injection technique is test-only and does not touch production-code escape paths; production code reaches the writer only via valid-name-validated Counter / Gauge instances. Recorded here so a reviewer reading the PLAN doesn't flag the test-only Registry-bypass as a contract violation.

**Threading the Bootstrap.Stats wrapper through call sites.** Task 7 introduces `Bootstrap.Stats *stats.Registry`. If Task 7 settles `bootstrap.Load`'s return type as `*Bootstrap` (a wrapping struct over `*bootstrapv3.Bootstrap`), then every existing call site that currently passes `bs *bootstrapv3.Bootstrap` to a manager constructor needs an accessor change (`bs.Proto`). The propagation surface is small — `cmd/envoy-go/main.go` (Task 12) plus the existing manager-test files. The planner inspects at Task 7 step 2 execution time whether the existing `Load` already returns a wrapper or returns the proto directly; the simpler-diff option is preferred (if the existing `Load` returns the proto directly, introduce the smallest-possible wrapper at Task 7 and propagate accessors at Task 12; if the existing `Load` already returns a wrapper, just add the `.Stats` field).

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 15 on `phase/06.1-stats-prometheus-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/06.1-stats-prometheus-impl
   ```
2. **The verification session** (next-fresh from the implementation session) re-runs all six gates per BOOTSTRAP §7.5 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. Verification commits `phase 06.1: STATE.md → lifecycle-state 5` on master.
3. **The REVIEW session** (next-fresh from verification) writes `docs/envoy-go/phases/06.1-stats-prometheus/REVIEW.md` per BOOTSTRAP §5 state 5 → 6. The REVIEW session's phase-done commit:
   - Flips ROADMAP row 06.1 → `done`.
   - Confirms ROADMAP row 06 stays `in-progress` (closes only at 06.2's phase-done — mirroring the 05 / 05.1 / 05.2 closure pattern per SPEC §4.4).
   - Confirms ROADMAP row 06.2 stays `planned`.
   - Lands the BEHAVIOR_CONTRACT in-place edit verification block (a re-grep that the Task 15 edit landed correctly).
   - Advances STATE to phase 06.2 (`active-phase: 06.2-access-log`; `lifecycle-state: 1`; `next-skill: superpowers:brainstorming`) at the SAME phase-done commit.

**No part of this section is done by Task 15.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/06.1-stats-prometheus-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` (per ADR-0005 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md), `next-skill-scope: <execute PLAN.md>`, `last-commit: <PLAN.md commit SHA>`.
3. Fast-forward `master` to `phase/06.1-stats-prometheus-plan` per ADR-0003.
4. Worktree for the next session: `.worktrees/phase-06.1-stats-prometheus-impl` on branch `phase/06.1-stats-prometheus-impl` (recommended per `## Execution preconditions` #1).
5. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on master). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (`internal/stats/{registry,counter,gauge,name,prom,doc,fuzz_test}.go` and the test pairs; `internal/admin/{prometheus,admin,doc}.go` and the test pairs; the cluster + listener + HCM + h2-router-action call-site changes; the `cmd/envoy-go/main.go` Registry threading + `Freeze()` boot-order; the `internal/bootstrap/bootstrap.go` `.Stats` field + `Load` allocator; the M-9 carry-forward fix + test; fixture 0005 in full; runner registration; six ADRs ADR-0059..ADR-0064; `BEHAVIOR_CONTRACT.md ## Stat-name mapping` + `## Equivalence Matrix` in-place edits; phase-05.2 REVIEW carry-forward triage.)
- Does the PLAN settle every 06.1-scoped SPEC §12 deferred decision? (8 items — see `## Settled SPEC §12 deferred decisions`.)
- Does the PLAN mitigate every SPEC §11 risk with a task-level step or an ADR? (11.1 unit tests for `internal/stats/` → Tasks 2-5; 11.2 unit tests for `internal/admin/` → Task 6; 11.3 unit tests for existing-package extensions → Tasks 8/10/11; 11.4 M-9 carry-forward unit test → Task 11; 11.5 differential fixture 0005 → Task 14; 11.6 LBP-1 enforcement test → Task 2; 11.7 h2spec re-run → Task 15; 11.8 fuzzers → Tasks 13/15; 11.9 race detector + lint → Task 15.)
- Does the PLAN resolve phase-05.2 REVIEW Minor findings triaged in SPEC §13? (M-9 RESOLVED-IN-06.1 at Task 11; M-4 / M-10 / M-12 continue-DEFERRED per the matrix.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 4 name flattening, Task 11 HCM hot-path + M-9, Task 14 fixture infrastructure, Task 15 final sweep)?
- Does the ADR number sequence match the verified DECISIONS.md tail? (ADR-0058 → ADR-0059..0064; non-monotonic mapping by topic-vs-first-use-order documented above.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~1700 LoC, 15 tasks, no further coherent split axis exists; per phase-04 / 05.1 / 05.2 precedent, one-sub-phase shipment is correct.)
- Are spec-review advisory items addressed? (Three planner-time items in `## Spec-review advisory responses`.)
- Does the import topology stay clean? (`internal/stats/` is a leaf; `internal/admin/`, `internal/cluster/`, `internal/listener/`, `internal/filter/hcm/`, `internal/bootstrap/`, `cmd/envoy-go/` import `internal/stats/`; no third-party Prom or stats library; the boundary grep at Task 15 step 4 enforces.)
- Does the PLAN preserve the LBP-1 invariant? (Task 2 codes the panic on post-Freeze register; Task 12 codes the `Freeze()` call after admin starts AND after listener manager begins accepting; Task 15 step 4's boundary grep verifies the Freeze call site.)
- Does the PLAN preserve Rule SN4's empirically-verified form? (Task 4 codes `^(.+)_([1-5])xx$` regex; Task 4 unit test asserts the canonical SN4 form for digits 1-5; ADR-0061 Context section pastes SPEC §10.1's verbatim Envoy-scrape evidence; the BEHAVIOR_CONTRACT in-place edit at Task 15 carries Rule SN4 in the same form.)
- Are the six ADRs internally consistent? (ADR-0059's no-third-party-library decision matches Task 15 step 4's grep; ADR-0060's histograms-deferred matches the absence of Histogram type in Task 2's primitives + the SPEC §6 NOT-EMITTED `server.uptime`; ADR-0061's Rule SN4 form matches Task 4's regex; ADR-0062's per-counter-delta-equality matches Task 14's driver assertions; ADR-0063's per-endpoint-deferred matches Task 8's 8-metrics-per-cluster (not 8×N) loop; ADR-0064's silent-ignore matches the SPEC §9 amendment to ADR-N.)

