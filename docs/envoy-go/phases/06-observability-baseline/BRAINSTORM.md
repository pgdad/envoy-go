# Phase 06 Brainstorm — Observability Baseline

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-0 brainstorm session for phase 06 (`observability-baseline`). The next session (lifecycle-state 1, skill `superpowers:writing-plans`) authors `SPEC.md` for phase **06.1** based on this brainstorm. Phase 06.2 receives a sibling SPEC stub at the same time.

**Brainstorm session:** worktree `.worktrees/phase-06-observability-baseline`, branch `phase/06-observability-baseline-brainstorm`, branched from master tip `75a6bf9` (phase-05.2 phase-done SHA-fill commit).

---

## 1. Scope decision — planner-time split

ROADMAP row 06 bundles three deliverables: **access log (file sink, Envoy default format) + stats + Prometheus admin endpoint.** Per ADR-0045 (planner-time split), and following the 05.1/05.2 precedent, phase 06 is split at brainstorm time into two sub-phases:

| ID | Title | Scope | Differential surface |
|---|---|---|---|
| **06.1** | `stats-prometheus` | `internal/stats` package, `/stats/prometheus` admin endpoint, 17 stat-emit call sites | fixture `0005-prometheus-stats` |
| **06.2** | `access-log` | `internal/accesslog` package, HCM access-log emit hooks | fixture `0006-access-log` (created when 06.2 brainstorms) |

**Rationale for split:** stats + Prometheus is one coherent unit (registry → exporter); access log is a separate filter-chain integration with its own format, async I/O, and BEHAVIOR_CONTRACT subsection. Keeping them in one phase risks bloating the SPEC and the review surface. The 05.1/05.2 cadence established that planner-time-split keeps phases reviewable.

**ROADMAP row 06 split rationale (for the eventual SPEC.md):** the two sub-phases share no code surface (stats package vs. access-log package) and differ in dependency profile (06.2 depends on 06.1 only insofar as it may emit metrics about its own buffer pressure, but that's optional). 06.1 ships first.

The phase-06 parent (`docs/envoy-go/phases/06-observability-baseline/`) carries this BRAINSTORM.md plus a master SPEC.md (eventually) that summarizes both sub-phases. Mirrors `docs/envoy-go/phases/05-http-2/`.

---

## 2. Phase 06.1 — design decisions

### 2.1 Stats backend architecture *(Q2 outcome → ADR-NNNN)*

**Decision:** Thin internal stats package backed by atomic counters, with a Prometheus adapter that walks the registry and writes the [Prometheus text exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/) directly. **No third-party Prometheus library** (no `prometheus/client_golang` dependency).

Rationale:
- Future Observability-family phases (gRPC ALS, OTLP, statsd) all need to hook a registry, not a Prometheus client. Investing in our own thin registry now is the same shape Envoy chose (`source/common/stats/` in upstream Envoy) for the same reason.
- Stat-name → Prometheus-name flattening (e.g., `cluster.<n>.upstream_rq_total` → `envoy_cluster_upstream_rq_total{envoy_cluster_name="<n>"}`) is a translation layer either way; doing it explicitly in our adapter is clearer than working around `client_golang`'s namespace conventions.
- Prometheus text format is well-specified and stable; ~50 LOC for the writer, including label-value escaping rules.

Alternatives considered: (A) `prometheus/client_golang` directly — rejected for future-sink coupling; (C) `expvar` + custom serializer — rejected because expvar lacks histogram support (and we'd recreate half the registry anyway).

### 2.2 Stats surface scope *(Q3 outcome → ADR-NNNN)*

**Decision:** Counters + gauges only in 06.1, ~17 names. **Histograms ADR-deferred** to a later phase (06.x or upstream-robustness family) with their own brainstorm covering circllhist→Prometheus bucket mapping.

Rationale:
- Gauges cost almost nothing once the registry exists and make the differential fixture meaningful (`upstream_cx_active`, `membership_total` are real signals).
- Histograms are a research project: Envoy uses circllhist (dynamic-bucket) internally and bridges to Prometheus's fixed-bucket shape via `histogram_bucket_settings`. Byte-equivalent matching against the v1.37.2 reference is hard and wants its own design pass.
- Bundling histograms here would bloat the phase and leave a half-baked histogram model in tree.

Alternatives considered: (A) counters only — too thin; (C) full histograms now — too big; (D) match Envoy's full v1.37.2 stat tree — clearly out of scope.

### 2.3 Stats catalog (the 17 names)

`<stat_prefix>` is read from HCM config (already plumbed from phase 04). `<addr>` is the listener bind address normalized like Envoy does (`0.0.0.0_10000`).

**Listener — 2 names:**
| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `listener.<addr>.downstream_cx_total` | counter | `envoy_listener_downstream_cx_total{envoy_listener_address="<addr>"}` |
| `listener.<addr>.downstream_cx_active` | gauge | `envoy_listener_downstream_cx_active{envoy_listener_address="<addr>"}` |

**HCM — 5 names:**
| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `http.<stat_prefix>.downstream_rq_total` | counter | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_2xx` | counter | base-name + `envoy_response_code_class` label per Envoy's default tag extractor (exact form TBD — see §2.3.1 below) |
| `http.<stat_prefix>.downstream_rq_3xx` | counter | (same shape, class 3) |
| `http.<stat_prefix>.downstream_rq_4xx` | counter | (same shape, class 4) |
| `http.<stat_prefix>.downstream_rq_5xx` | counter | (same shape, class 5) |

**Cluster — 8 names:**
| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `cluster.<n>.upstream_rq_total` | counter | `envoy_cluster_upstream_rq_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_2xx` | counter | base-name + `envoy_response_code_class` label per Envoy's default tag extractor (exact form TBD — see §2.3.1) |
| `cluster.<n>.upstream_rq_3xx` | counter | (same shape, class 3) |
| `cluster.<n>.upstream_rq_4xx` | counter | (same shape, class 4) |
| `cluster.<n>.upstream_rq_5xx` | counter | (same shape, class 5) |
| `cluster.<n>.upstream_cx_total` | counter | `envoy_cluster_upstream_cx_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_active` | gauge | `envoy_cluster_upstream_cx_active{envoy_cluster_name="<n>"}` |
| `cluster.<n>.membership_total` | gauge | `envoy_cluster_membership_total{envoy_cluster_name="<n>"}` (set once at register, equals N endpoints) |

**Server — 2 names:**
| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `server.live` | gauge | `envoy_server_live` (set to 1 once admin `/ready` returns 200; never reset by 06.1) |
| (NOT EMITTED) `server.uptime` | — | depends on monotonic-clock + per-scrape recompute; defer with histograms |

**Total: 17 internal names.** The four `downstream_rq_Nxx` (and four `upstream_rq_Nxx`) Prometheus exposition forms depend on Envoy's `stats_tags` default tag-extractor regex behavior — see §2.3.1.

#### 2.3.1 Empirical-verification gate for status-class stats *(SPEC-author obligation)*

Envoy v1.37.2's default tag-extractor regex governs how `_2xx`/`_3xx`/`_4xx`/`_5xx` suffixes flatten to a Prometheus metric name + `envoy_response_code_class` label. Two ambiguities the brainstorm explicitly does NOT resolve:

1. **Label value form:** `"2"` (single digit) or `"2xx"` (full token)?
2. **Metric-name collapse:** does the suffix get removed entirely (resulting base name `envoy_cluster_upstream_rq`) or preserved as a literal `_xx` (resulting base name `envoy_cluster_upstream_rq_xx`)?

Two reasonable-looking sources (Istio Insider docs vs. envoyproxy/envoy issue #2141) gave contradictory answers in our brainstorm review. The brainstorm-review reviewer recommended C1+C2 fixes based on one of those readings, but the contradiction means **the only reliable answer is empirical**: at SPEC-drafting time, run reference Envoy v1.37.2 (image pinned in `docs/envoy-go/ENVOY_TARGET.md`) with a minimal HCM + 1-cluster config, send a request that produces a 2xx response, scrape `/stats/prometheus`, and copy the exact name/label/value verbatim. The SPEC must record:
- the verbatim scrape output for these stats
- the exact tag-extractor regex behavior (cite Envoy v1.37.2 `source/common/config/well_known_names.cc` SHA)
- the four flattening rules in BEHAVIOR_CONTRACT.md `## Stat-name mapping` based on what the scrape showed

**The differential equivalence claim (§2.5, §7.3) requires byte-equality on metric name + label keys + label values + types, which means SPEC-drafting MUST verify these empirically before fixing the rules.** Brainstorm-time guesses are insufficient.

**Explicit non-goals (06.1 does NOT emit):** all histograms; per-endpoint cluster stats (`cluster.<n>.<endpoint_address>.cx_total`) — equivalent of Envoy's `enable_per_endpoint_stats=false`; TLS subsystem stats (`cluster.<n>.ssl.*`); runtime / admin / server stats beyond `server.live`; TCP-proxy filter stats; 1xx response counters.

### 2.4 Carry-forward dispositions *(Q4 outcome)*

**Decision: address M-9 only.** M-4 / M-10 / M-12 deferred to a dedicated H2-hardening phase (06.x or upstream-robustness family).

| Item | Disposition |
|---|---|
| **M-9** Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error | **Bundled with 06.1** — the 05.2 REVIEW explicitly deferred this to "phase-06 observability when logging/metrics surface lands." ~5 LOC + ~20 LOC test. Lands in same SPEC under "Carry-forward dispositions." |
| **M-4** `readClientPreface` not ctx-aware (`internal/filter/hcm/h2/conn.go`) | Deferred. H2 connection hardening, not observability. Fits a future H2-hardening sub-phase or upstream-robustness family. |
| **M-10** `SETTINGS_TIMEOUT` absent (`internal/filter/hcm/h2/client.go`) | Deferred. Same reasoning as M-4. |
| **M-12** `closedStreams` map unbounded (`internal/filter/hcm/h2/conn.go`) | Deferred. Long-lived-conn memory growth is a hardening concern, not an observability one. |

Rationale: M-9 is the only carry-forward with an explicit cross-reference saying it should land here; honoring the 05.2 REVIEW disposition is straightforward. M-4/M-10/M-12 are H2 spec corners (timeouts, bounding behavior, edge-case tests) that want their own brainstorm — bundling them into a stats/Prometheus phase forces an "and also some H2 hardening" appendix that won't review well.

### 2.5 Differential fixture shape *(Q5 outcome)*

**Decision — admin endpoint coverage:** ship `/stats/prometheus` only (alias path; not the `?format=prometheus` query form). Phase 08 owns full admin API; 06.1 must not creep into surface that isn't strictly required.

**Decision — equivalence claim:** **behavioral-delta assertion** (Q5 = B). No byte-exact whole-output comparison. Drive a defined load, snapshot the 17 stats before/after on both proxies, assert per-stat delta-equality between envoy-go and Envoy. Gauges are snapshot-equal after drain.

Rationale:
- Pure byte-exact full-output is fragile — a minor-version Envoy bump that adds a new stat would break the fixture without indicating any envoy-go regression.
- Pure delta-only is somewhat permissive (typo in stat name would still pass with delta=0=0), but the schema is constrained by the fixture's stat-name allow-list (the 17 names) and the unit-test layer catches name validity at register time.
- Layered (delta + byte-exact-schema) was considered but adds ~2× fixture LOC for marginal protection over what unit tests already provide.

**Driver outline:**
1. Boot envoy-go on port P1 + reference Envoy on port P2 with identical bootstrap (one HTTP/1.1 listener, one cluster `c0` with 1 endpoint, `stat_prefix: ingress_http`).
2. Boot a controlled backend on port P3 that returns whatever status the driver requests (default 200, configurable per request via header). Backend is an explicit-502-returning shape for the 502 test point — not a dial-failure path (avoids dependency on dial-error→status mapping which is a separate concern).
3. Scrape both `/stats/prometheus` → parse → snapshot 17 stats as `before`.
4. Send 5 sequential requests with target statuses `[200, 200, 404, 200, 502]` (404 = no-route from envoy-go's HCM; 502 = backend explicit return).
5. Scrape again → `after`.
6. Assert per-counter `delta_envoy_go == delta_envoy`; per-gauge `after_envoy_go == after_envoy`.

**Expected deltas (`expectations.yaml` shape):**
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

Connection-count rows use `≥ 1` because both proxies may use HTTP keepalive and collapse multiple requests onto one upstream connection; the differential equality holds regardless of magnitude.

---

## 3. Architecture & package layout

```
internal/stats/                  -- new package (thin Stats Store)
  doc.go                         -- already exists; rewrite to describe the API
  registry.go                    -- type Registry; methods NewCounter, NewGauge, Walk
  counter.go                     -- type Counter (atomic.Uint64 backed)
  gauge.go                       -- type Gauge (atomic.Int64 backed; supports Inc/Dec/Set)
  name.go                        -- hierarchical-dotted-name helpers + flatten-to-Prom-name
  registry_test.go, *_test.go    -- unit tests

internal/admin/                  -- existing package; extended
  prometheus.go                  -- new: GET /stats/prometheus handler
  prometheus_test.go             -- new
  admin.go                       -- modified: route registration adds /stats/prometheus
  doc.go                         -- updated to mention the new endpoint

internal/cluster/                -- existing; modified to call into stats
  manager.go                     -- on cluster register: NewCounter("cluster.<n>.upstream_rq_total"), etc.
  dial.go / dial_h2.go           -- inc upstream_cx_total, upstream_cx_active gauge

internal/filter/hcm/             -- existing; modified to call into stats
  filter.go (or HCM dispatch)    -- on req entry: inc downstream_rq_total
                                 -- on resp: inc downstream_rq_<status_class>
  h2/router_action.go            -- M-9: log line on doH2 error path

internal/listener/               -- existing; modified
  listener.go                    -- inc downstream_cx_total per accept

cmd/envoy-go/main.go             -- modified: thread stats.Registry from boot
                                    through manager → admin handler
internal/bootstrap/              -- modified: construct + share Registry singleton

test/differential/0005-prometheus-stats/    -- new fixture directory
  README.md, expectations.yaml,
  envoy.yaml, envoy-go.yaml, driver.go
test/differential/runner.go      -- registration update for fixture 0005
```

**Key shape notes:**

1. **Registry is constructed once at boot, threaded explicitly.** Allocated in `bootstrap.Load()`, passed to `cluster.Manager`, listener manager, HCM filter chain, and admin server. No package-global. (Mirrors how `internal/cluster.Manager` is already threaded.)
2. **Every metric is created at register time, not at increment time.** Counter/Gauge are tiny structs (one atomic word + a name string), held by reference on the Cluster / HCM / Listener struct.
3. **No third-party Prometheus library.** The text-format writer is ~50 LOC inline.
4. **`internal/accesslog/` is not touched in 06.1** (placeholder doc.go remains). Comes alive in 06.2.
5. **M-9 piggy-back lives entirely in `internal/filter/hcm/h2/router_action.go`** with its own unit test.
6. **Existing `cluster.NewManager` / `cluster.NewManagerWithBaseDir` signatures change** to accept a `*stats.Registry`. Both constructors must be extended (not replaced); `cmd/envoy-go/main.go` and any test fixtures that call these constructors will need a one-line update. Likewise `admin.New(ready)` becomes `admin.New(ready, *stats.Registry)`. The SPEC must enumerate the call-site updates as part of its "Files touched" inventory so it isn't surprise scope at PLAN time.

---

## 4. Data flow & wiring

### 4.1 Boot wiring (one-time, at process start)

```
cmd/envoy-go/main.go
   ↓
bootstrap.Load(configPath) → returns *Bootstrap (now has .Stats *stats.Registry)
   ↓
   ├─→ admin.New(ready, stats)              -- admin server gets a Registry handle
   │      ↓ registers /ready + /stats/prometheus handlers
   │
   ├─→ cluster.NewManager(clusters, stats)  -- manager threads stats into each Cluster on register
   │      ↓ on cluster.Register():
   │           NewCounter("cluster.<n>.upstream_rq_total")
   │           NewCounter("cluster.<n>.upstream_rq_2xx".."5xx")
   │           NewCounter("cluster.<n>.upstream_cx_total")
   │           NewGauge("cluster.<n>.upstream_cx_active")
   │           NewGauge("cluster.<n>.membership_total")  (Set to len(endpoints))
   │
   └─→ listenerManager.New(listeners, stats, hcmFactory)
          ↓ on listener.Register():
               NewCounter("listener.<addr>.downstream_cx_total")
               NewGauge("listener.<addr>.downstream_cx_active")
          ↓ HCM factory captures stats; per-HCM-instance allocates:
               NewCounter("http.<stat_prefix>.downstream_rq_total")
               NewCounter("http.<stat_prefix>.downstream_rq_2xx".."5xx")
```

### 4.2 Increment paths (per-request hot path)

| File | Hot-path edits |
|---|---|
| `internal/listener/listener.go` (Accept loop) | `+2 LOC` — `cx_total.Inc()` + `cx_active.Inc()` on accept; `defer cx_active.Dec()` on close |
| `internal/filter/hcm/filter.go` (HCM dispatch entry) | `+1 LOC` — `downstream_rq_total.Inc()` on first byte of request line/headers |
| HCM response-side hook | `+3-5 LOC` — switch on response status class → `downstream_rq_<Nxx>.Inc()` once per response. Lives where the response status code is finalized, before bytes hit the wire. |
| `internal/cluster/dial.go` + `dial_h2.go` | `+2 LOC` each — `cx_total.Inc()` + `cx_active.Inc()` on successful dial; `defer cx_active.Dec()` on conn close |
| `routerActionH2.do` + H1 router action | `+1 LOC` — `upstream_rq_total.Inc()` at request dispatch; `+3-5 LOC` for `upstream_rq_<Nxx>.Inc()` on response status |

**Total request-path edits: ~15 LOC across ~5 files.** Stats package itself is the bulk of new code.

### 4.3 Read path (Prometheus scrape)

```
GET /stats/prometheus
   ↓
admin.handlePrometheus(w, r):
   1. registry.Walk(func(metric stats.Metric) { ... })
   2. For each metric:
      a. flatten internal name → prom name + label set (name.go)
      b. group by prom name (status-class collapse)
      c. emit "# HELP <prom_name> ..."   (HELP text from a static map)
      d. emit "# TYPE <prom_name> <type>"
      e. emit "<prom_name>{label=\"value\",...} <value>\n"  (escape per Prom spec)
   3. Output is sorted alphabetically by Prom name (matches Envoy).
```

The handler is read-only against the Registry. Reads use `atomic.LoadUint64` / `LoadInt64` — no lock contention with hot-path increments. Walk over the Registry uses an `RWMutex` only for the metric *list* (mutated only at register time, never per-request).

### 4.4 `server.live` gauge

Set to 1 inside `admin.handleReady` the first time it returns 200. Never reset. (06.1 has no graceful-drain; phase 08 owns that.)

### 4.5 HELP text source

Static map in `name.go` keyed by Prometheus name → human-English description. Authored once for the 13 unique Prometheus names. **Not byte-equal to Envoy's HELP text** (Rule SN6 below). Envoy's HELP is not semantically meaningful per the Prometheus exposition spec, and the differential fixture (Q5 = B) checks values + label keys + types only — not HELP strings.

---

## 5. Error handling, edge cases, concurrency

### 5.1 Concurrency model

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `Registry.NewCounter`, `NewGauge` | Once per metric, at process start | `Registry.mu` Lock |
| Hot path | `Counter.Inc()`, `Gauge.{Inc,Dec,Set}` | Per request / connection / accept | **Lock-free** — `atomic.{AddUint64, AddInt64, StoreInt64, LoadInt64}` |
| Scrape | `Registry.Walk(fn)` | Per `/stats/prometheus` request (rare; Prom default scrape interval is 15s) | `Registry.mu` RLock |

**Invariant:** the Registry's *list of metrics* is mutable only during boot. Once boot completes, the list is fixed. After boot, scrapes RLock the list (cheap) and read each metric's atomic value with no contention against hot-path increments. Hot-path Inc/Dec/Set never touch `Registry.mu`.

**SPEC-mandated invariant (LBP-1, "list before play"):** The SPEC must explicitly forbid `NewCounter`/`NewGauge` calls after the listener manager begins accepting connections. All metric registration completes during the boot phase (`bootstrap.Load` → `cluster.NewManager.Register*` → `listenerManager.New.Register*` → `admin.New`); no lazy/on-demand registration. This invariant is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments. A unit test in `registry_test.go` should set a "frozen" flag after boot and panic if `NewCounter`/`NewGauge` is called post-freeze.

If a future xDS-CDS phase introduces dynamic cluster registration, `NewCounter` would race against `Walk` — explicitly out of scope for 06.1, documented as such in the SPEC.

### 5.2 Edge cases

- **Status code 0 / no response sent:** `downstream_rq_total` increments but no `downstream_rq_<Nxx>` does. Matches Envoy (totals don't reconcile against status-class sums by design; Envoy has `downstream_rq_completed` for that — not shipped in 06.1).
- **1xx responses:** Envoy emits a separate `downstream_rq_1xx`. 06.1 omits it (phase 04 doesn't exercise 1xx meaningfully).
- **Status class arithmetic (`code / 100`):** matches Envoy. `499` → 4xx. Out-of-range codes are unreachable from envoy-go's response path.
- **HCM with no `stat_prefix`:** phase 04 already requires it; fatal config error if absent.
- **Cluster with 0 endpoints:** `membership_total = 0`, accurate. Differential fixture uses 1-endpoint clusters.
- **Cluster name / stat_prefix / listener address with characters needing escape (`\`, `"`, `\n`):** label-value escaper in `prometheus.go`. Unit test with adversarial input mandated.
- **Concurrent scrapes:** both RLock; both walk; both write their own response. No interaction. Snapshot consistency is per-walk: a given walk reads each metric atomically once; different metrics within one walk may have been updated between reads. Matches Envoy.
- **Counter overflow:** `atomic.Uint64` wraps at 2^64 (584,942 years at 1M req/s). Non-issue.
- **Negative gauge:** if `Dec` runs unmatched with `Inc`, gauge reads negative. Defensive design — gauge reflects reality. Test catches paired-Inc/Dec bugs at CI time.
- **Boot-time duplicate registration:** fatal panic. Matches Envoy. Catches "two clusters with the same name" class of bug.
- **Boot-time invalid name:** name must match `^[a-zA-Z_][a-zA-Z0-9_.]*$`; panic on fail. Unit test asserts.

### 5.3 Error paths

- **Prometheus serialization errors:** none possible at runtime — we control the writer, all values are `uint64`/`int64`, names+labels validated at register time.
- **Client disconnect mid-response:** `w.Write` returns an error, logged and otherwise ignored (no retry, no error response — too late, headers already sent).

### 5.4 Persistence

**None.** Counters reset on process restart. Same as Envoy. No on-disk persistence, no graceful-drain hand-off.

### 5.5 Race-detector contract

`go test -race ./...` clean across:
- Concurrent `Counter.Inc` from N goroutines
- Concurrent `Gauge.{Inc,Dec,Set}` from N goroutines
- `Walk` running while `Inc/Dec/Set` are running (the post-boot read-only-list assumption)
- Two concurrent `Walk`s

Unit tests in `registry_test.go` exercise each. Differential fixture indirectly stresses #3 (driver scrapes mid-load).

### 5.6 Things 06.1 does NOT handle

- Hot reload of stats schema (xDS family)
- Per-endpoint stats expansion (xDS-EDS revisits)
- Stat tag extraction via `stats_config.stats_tags` regex (06.1 hardcodes extraction in `name.go`; ADR-worthy)
- Histograms (own brainstorm)

---

## 6. Testing strategy

### 6.1 Unit tests (per-package)

`internal/stats/`:
- `registry_test.go` — register / lookup / walk; duplicate-name panic; invalid-name panic; concurrent walks; race-clean concurrent Inc + Walk
- `counter_test.go` — Inc semantics; race-clean concurrent Inc from N goroutines; overflow not testable (just document)
- `gauge_test.go` — Inc/Dec/Set; negative gauge after unmatched Dec; race-clean
- `name_test.go` — flatten happy paths; status-class collapse (4 internal names → 1 Prom name); label-value escaping with adversarial inputs; invalid-name validator

`internal/admin/prometheus_test.go`:
- HTTP handler returns 200 + correct content-type (`text/plain; version=0.0.4; charset=utf-8`)
- Output is alphabetically sorted by Prometheus name
- Empty registry → handler still writes the (empty) response cleanly
- Counter + gauge values render correctly (including negative gauge)
- HELP / TYPE lines present and correctly placed

`internal/cluster/`, `internal/listener/`, `internal/filter/hcm/`:
- New unit tests asserting hot-path increments fire on the right edges. Use real `stats.Registry` (not mocks) — fast, deterministic, exercises the integration end-to-end.

**M-9 unit test:** `routerActionH2.doH2` returns an error → log line is captured (test logger sink) → assert error was logged before the function returned. ~20 LOC.

### 6.2 Differential fixture `0005-prometheus-stats`

(See §2.5 above for driver flow + expected deltas.)

### 6.3 h2spec re-run

Phase 06.1 doesn't touch H2 wire code. h2spec gates remain at 53/53 — already-pinned at the ADR-0051 SHA. Gate (c) re-runs unchanged.

### 6.4 Fuzzers

Existing six fuzzers re-run at 30s budget. **New: `FuzzPromTextFormat`** — fuzzes adversarial stat names / label values into the Prometheus writer. Cheap (~50 LOC); escaping bugs are the most likely class of bug in the writer, and Prometheus's parser is strict. Total: 7 fuzzers post-06.1.

### 6.5 Race detector + lint

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean (gate (e)).

---

## 7. BEHAVIOR_CONTRACT.md additions

### 7.1 `## Stat-name mapping` subsection (full population)

Currently empty placeholder. Phase 06.1 fills it with the 17-name table from §2.3 + the following flattening rules. Rules SN1-SN3, SN5-SN8 are settled at brainstorm time. **Rule SN4 is empirically pinned at SPEC-drafting time per §2.3.1**:

```
Rule SN1: Name segments matching `cluster.<n>.<rest>` extract `<n>` as label
          `envoy_cluster_name` and prefix `<rest>` with `envoy_cluster_`.
Rule SN2: Name segments matching `http.<stat_prefix>.<rest>` extract <stat_prefix>
          as label `envoy_http_conn_manager_prefix` and prefix <rest> with `envoy_http_`.
Rule SN3: Name segments matching `listener.<addr>.<rest>` extract <addr> as label
          `envoy_listener_address` and prefix <rest> with `envoy_listener_`.
Rule SN4: TBD AT SPEC TIME — names ending `_Nxx` where N ∈ {1..5} flatten to a
          base name + `envoy_response_code_class` label per Envoy v1.37.2's default
          tag-extractor regex. The exact base-name shape (`_xx` preserved vs. removed)
          and label-value form (`"2"` vs `"2xx"`) are determined empirically against
          a reference Envoy scrape; see §2.3.1.
Rule SN5: Server-scope names (`server.<rest>`) flatten to `envoy_server_<rest>`
          with no extracted labels.
Rule SN6: HELP text is best-effort English, NOT byte-equal to Envoy's HELP. The
          differential equivalence claim is on values + label keys + types only.
Rule SN7: Histograms are not emitted by 06.1. (Forward-looking.)
Rule SN8: Per-endpoint cluster stats are not emitted by 06.1. (Forward-looking.)
```

### 7.2 `## Access log field mapping` subsection

Stays as the existing placeholder. Forward-looking note added: "Populated in 06.2."

### 7.3 New equivalence-matrix row

```
| Dimension       | Equivalence claim                                        | Allow-list / tolerance               |
|-----------------|----------------------------------------------------------|--------------------------------------|
| Stats output    | Per-stat behavioral delta after defined load is equal    | 17 stats listed in §Stat-name        |
|                 | between envoy-go and reference Envoy. Gauges are        | mapping. All other Envoy stat names  |
|                 | snapshot-equal after drain. Names + label keys + types  | in /stats/prometheus output are      |
|                 | byte-equal; HELP text ignored.                          | ignored by the differential.         |
```

---

## 8. ADRs anticipated

The planning session (`superpowers:writing-plans` for SPEC.md, then PLAN.md) finalizes count + numbering. Six ADRs anticipated for 06.1:

1. **Internal Stats Store architecture** — atomic-counter Registry, no third-party Prometheus dep, lock-free hot path. (Q2 outcome.)
2. **Histograms deferred** from 06.1 to a later sub-phase with own brainstorm + bucket-mapping ADR. (Q3 outcome.)
3. **Stat-name → Prometheus-name flattening rules** (SN1–SN8 in §7.1).
4. **Differential equivalence shape for stats** — behavioral delta, not byte-exact whole-output. (Q5 outcome.)
5. **Per-endpoint cluster stats not emitted** — 06.1 emits cluster-level only; future xDS-EDS phase revisits.
6. **`stats_config.stats_tags` config not honored** — 06.1 hardcodes extraction in `name.go`; future phase may parse the field.

(05.2 had 4 ADRs; 05.1 had 9. 6 sits in between — typical for a baseline-infrastructure phase.)

---

## 9. Out-of-scope items deferred to later phases

| Item | Deferred to |
|---|---|
| Histograms (`upstream_rq_time`, `downstream_rq_time`, etc.) | 06.x or upstream-robustness family — own brainstorm |
| Per-endpoint cluster stats (`cluster.<n>.<endpoint_address>.cx_total`) | xDS-EDS phase |
| TLS subsystem stats (`cluster.<n>.ssl.*`) | upstream-robustness family |
| Runtime / admin / server stats beyond `server.live` | phase 08 (admin API) |
| TCP-proxy filter stats | (TBD; no in-flight TCP proxy users) |
| 1xx response counters | (deferred; not exercised by current fixtures) |
| `stats_config.stats_tags` regex extraction | future stats-config phase |
| Hot reload of stats schema | xDS family |
| Other admin endpoints (`/stats`, `?format=json`, `?filter=`) | phase 08 |
| Access log file sink, format, async writer | **06.2** |
| H2 hardening: M-4 (ctx-aware preface), M-10 (SETTINGS_TIMEOUT), M-12 (closedStreams cap) | dedicated H2-hardening sub-phase or upstream-robustness family |

---

## 10. Hand-off to writing-plans

Next session (lifecycle-state 1, skill `superpowers:writing-plans`) authors:

- `docs/envoy-go/phases/06-observability-baseline/SPEC.md` — master design summarizing 06.1 + 06.2 scope (mirrors `docs/envoy-go/phases/05-http-2/SPEC.md`).
- `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` — sub-phase SPEC for 06.1 derived from §§2-9 of this document.
- `docs/envoy-go/phases/06.2-access-log/README.md` — sibling SPEC stub, citing the master SPEC + this BRAINSTORM § "06.2 sub-phase" forward-looking notes.
- ROADMAP.md split: row `06 | observability-baseline | 05 | planned` becomes parent `06 | observability-baseline | 05 | in-progress | 06.1, 06.2 | ...` with rows `06.1 | stats-prometheus | 05 | planned | | ...` and `06.2 | access-log | 06.1 | planned | | ...`.
- After SPEC, lifecycle-state 1 → 2 with `next-skill: superpowers:writing-plans` (PLAN.md authoring) and `active-phase: 06.1-stats-prometheus`.

This BRAINSTORM.md is committed as the brainstorm-close artifact and is read-only history once the next session starts. Future sessions consult it as the authoritative record of the design decisions made here.
