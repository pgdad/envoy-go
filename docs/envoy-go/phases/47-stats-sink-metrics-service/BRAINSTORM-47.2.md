# Phase 47.2 Brainstorm — `metrics_service` deltas + tags-as-labels (the SECOND-and-FINAL leg of the FOURTH Observability-family row; SUB-SPLIT by-concern into 47.2a `report_counters_as_deltas` + 47.2b `emit_tags_as_labels`)

**Status:** brainstorm complete (lifecycle-state 0 → 1 for the 47.2 leg). This document opens the SECOND-and-FINAL leg of phase 47 (`stats-sink-metrics-service`, the FOURTH row of the Observability family) and — under the ADR-0045 sub-split soft gate re-decided live in this session — SUB-SPLITS 47.2 by-concern into two independently differential-provable sub-legs: **47.2a** (`report_counters_as_deltas=true` — per-counter last-flush delta state; anticipated **ADR-0263**) then **47.2b** (`emit_tags_as_labels=true` — SN-rule tag→`LabelPair` extraction; anticipated **ADR-0264**). Each sub-leg lifts exactly ONE of the two strict-rejects the 47.1 core leg installed (`internal/bootstrap/bootstrap.go:446` / `:449`). **ROW 47 STAYS `in-progress`** — it flips `done` at the FINAL sub-leg **47.2b IMPL** per ADR-0106 + `reference_roadmap_split_phase_row_done` (three legs total now: 47.1 core, 47.2a deltas, 47.2b tags-as-labels); the Observability FAMILY STAYS OPEN.

Phase 47.2 composes entirely on the 47.1 as-built substrate (ADR-0262, squash `77f776bd`) — it invents no new framework piece, adds no new package, and adds no new go.mod module. Both sub-legs are focused modifications to the existing `internal/statssink` mapping/sink path plus a one-field `StatsSinkConfig` extension and a reject-arm lift in `internal/bootstrap`. The differential template is the landed `0089-stats-sink-metrics-service` (the two-per-side-receiver + hard-`Close()` periodic-flush correction).

The load-bearing facts that shape this brainstorm (every one citable on disk against the 47.1 as-built HEAD):

- **The 47.1 mapping is the EXACT plug-in point for both knobs — it is stateless, cumulative, and zero-labels.** `internal/statssink/mapping.go:22` `snapshot(reg *stats.Registry, nowMs int64) []*dto.MetricFamily` walks the frozen `Registry` and TYPE-SWITCHES on the concrete `*stats.Counter` / `*stats.Gauge` (`:26` / `:35`), emitting per metric one `MetricFamily` carrying the FULL dotted `Name()` (`:28` / `:37`), the absolute `float64(v.Load())` value (`:31` / `:40`), a flush-time `TimestampMs` (`:32` / `:41`), ZERO `LabelPair`s, and no `Help`. 47.2a changes the COUNTER arm's value from absolute to `current − last`; 47.2b changes BOTH arms' `Name`/`Label` split. The function is called once per tick from `internal/statssink/flusher.go:47` `flushOnce` → `snapshot(f.reg, f.nowMs())`, fanned to each sink via `s.Submit(batch)` (`:49`).
- **The flush path is STATELESS per tick — delta state is a NEW piece, and it is PER-SINK not per-Flusher.** `flusher.go:13` `Flusher{ reg, interval, sinks, nowMs }` holds NO cross-tick state; `flushOnce` (`:46`) builds one snapshot and `Submit`s the SAME batch slice to every sink (`:48`). A delta sink needs a `map[string]uint64` of last-flushed Counter values kept ACROSS flushes — and because two sinks can be configured with different `report_counters_as_deltas` values (and flush/reconnect independently), that state is logically PER-SINK, not a single Flusher-level map. The 47.2a design therefore makes the delta-aware snapshot a sink-owned (or sink-parameterized) transform, NOT a mutation of the shared `Flusher.snapshot` fan-out. (47.1 ships exactly one sink, but the per-sink framing is the correct boundary and costs nothing.)
- **Gauges have no delta — 47.2a is Counter-only.** `internal/stats/gauge.go` `Gauge.Load() int64` is a settable level, not a monotone accumulator; Envoy's `report_counters_as_deltas` (proto field 2, the name says *counters*) applies to COUNTER families only. The 47.2a delta transform touches the `*stats.Counter` arm of the type-switch (`mapping.go:26`) ONLY; the `*stats.Gauge` arm (`:35`) is UNCHANGED (absolute). This is pinned by the proto field name and confirmed at the 47.2a SPEC live probe (D-MS-DELTA-GAUGE).
- **`emit_tags_as_labels=true` is NOT the Prometheus projection — it reuses the tag EXTRACTORS, not `flattenToProm`'s output.** `internal/stats/name.go:39` `flattenToProm(internal string) (base string, labels []Label, err error)` projects an internal dotted name to the PROMETHEUS exposition form (`cluster.c_backend.upstream_rq_total` → base `envoy_cluster_upstream_rq_total` + label `{envoy_cluster_name: c_backend}`, Rule SN1 at `:43`). The metrics_service `emit_tags_as_labels` keeps the *internal* name model but strips tag VALUES into `MetricFamily.metric[].label[]` `LabelPair`s using the SAME underlying tag extractors — the emitted family NAME and the label KEY form are an empirical question (does the reference keep dots? use the `envoy.`-prefixed tag name like `envoy.cluster_name`? the Prometheus `envoy_cluster_name` form?). The 47.2b design REUSES the SN1–SN9 extraction logic (`name.go:39`–`:341` — cluster/http/listener/server + the filter-specific second-pass extractors) but the exact projected name/label-key form is a 47.2b SPEC live-probe pin (D-MS-LABEL). A design call deferred to the 47.2b SPEC: refactor `name.go` to expose a reusable `(name, labels)` extraction core consumed by BOTH `flattenToProm` and the statssink mapping, vs a parallel extractor in `internal/statssink`.
- **The two reject arms that the sub-legs lift are adjacent and explicit.** `internal/bootstrap/bootstrap.go:446` rejects `report_counters_as_deltas:true` ("47.1 emits cumulative absolute values; deferred to 47.2"); `:449` rejects `emit_tags_as_labels:true` ("47.1 emits the full dotted name with zero labels; deferred to 47.2"). 47.2a lifts `:446` + adds a `ReportCountersAsDeltas bool` to `StatsSinkConfig` (`:272`, today a one-field `{ClusterName string}` struct); 47.2b lifts `:449` + adds an `EmitTagsAsLabels bool`. The sibling-TypeURL reject (`:413`), the `stats_flush_on_admin` reject (`:405`), the non-default `histogram_emit_mode` reject (`:452`), the non-V3 transport reject (`:436`), the `google_grpc` reject (`:441`), and the empty-cluster reject (`:443`) ALL STAY (none are un-deferred by 47.2 — they remain strict per ADR-0262's deferral set + ADR-0080).
- **The sink lifecycle + the boot wiring are UNTOUCHED.** `internal/statssink/sink.go:60` `MetricsServiceSink` (the bounded-channel + writer-goroutine + identifier-once + reconnect-resend + idempotent-`Close` client-streaming template) consumes whatever `[]*dto.MetricFamily` batch it is `Submit`ted (`:101`) — it is mapping-agnostic. Both 47.2 knobs change only the CONTENT of the batch (the mapping), not the streaming. `cmd/envoy-go/main.go`'s post-`Freeze` `Flusher.Start` + LIFO-`Close` wiring is unchanged save threading the two new `StatsSinkConfig` bools into the sink/snapshotter construction.
- **The `0089` differential is the template, but the DELTA assertion model fundamentally differs.** `test/fixtures/0089-stats-sink-metrics-service` asserts the deterministic COUNTER subset (`cluster.c_backend.upstream_rq_total`, `http.hcm_local.downstream_rq_total`, `http.hcm_local.downstream_rq_2xx`) `== K=7` cross-side EXACT over TWO driver-owned per-side `metricsservice` receivers + a hard `Close()` (the periodic-flush correction; `reference_periodic_sink_differential_two_receivers`). A DELTA sink emits `current − last` per flush, so the LAST flush after convergence reads ≈ 0 — the `value==K` assertion is meaningless. 47.2a's `0090` asserts the **SUM of a counter's deltas across all received flushes == K** (poll-to-converge the running per-name sum). 47.2b's `0091` keeps the cumulative value model but asserts the `{name, sorted LabelPair[]}` split. Both inherit the two-per-side-receiver + hard-`Close()` scaffolding verbatim.
- **No new package, no new go.mod module, no new BackendKind, +0 stat surface.** 47.2 lands entirely inside `internal/statssink` + `internal/bootstrap` + `cmd/envoy-go/main.go` + two new fixture dirs + the existing driver-owned `test/helpers/metricsservice` receiver (extended with a delta-sum / label-aware assert surface). The +0-self-stats posture (D-MS-STATS-FINAL — the sink registers NO `metrics_service`-scoped stat) is PRESERVED across both legs (nothing to self-delta; no surface delta). `github.com/prometheus/client_model v0.6.1` (the `MetricFamily`/`LabelPair` protos) is already direct in go.mod (landed at 47.1); `LabelPair` is already available for 47.2b.

The next sessions author the 47.2a SPEC → PLAN → IMPL, then 47.2b's. Each SPEC executes its §10 empirical-pin obligations (D-MS-DELTA-* / D-MS-LABEL-*) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network` — a shared bridge with a metrics-receiver hostname reachable from BOTH containers; verify the receiver actually decoded ≥1 `MetricFamily` from the reference), and anchors its ADR §Context draft.

**Brainstorm session:** direct-on-master (docs-only, per the family-row BRAINSTORM precedent — phases 44/45/46 + 47.1 brainstorms committed direct on master; no worktree, no code-adjacent probe artifacts land at the brainstorm). Substantive predecessor on master: the phase-47.1 IMPL squash `77f776bd` (the metrics_service core sink; ADR-0262; row 47 `in-progress`), with the docs-only routing tip `924bfc8e` (the post-47.1-IMPL cold-start router) as the literal live tip. Counts at master tip: stat surface **1200** (H2 cluster; non-H2 **1196**), differential fixtures **91** (tail `0089-stats-sink-metrics-service`), fuzzers **50**, BackendKind tail **38** (`H2GoawayResponder`), DECISIONS tail **ADR-0262** (next-free **ADR-0263**). ALL counts stay UNCHANGED at this brainstorm (docs-only).

**Brainstorm mode:** interactive with a live human. The user re-decided the ADR-0045 sub-split via dialogue:

- **Q-split-47.2** — 47.2 is SUB-SPLIT by-concern into **47.2a (`report_counters_as_deltas`)** then **47.2b (`emit_tags_as_labels`)**, chosen over shipping the parent's chartered single combined leg. Rationale: the two knobs are genuinely INDEPENDENT (a stateful per-counter delta-bookkeeping change to the snapshot value path vs a name/label-split change to the mapping's name model — they touch disjoint code), they carry DIFFERENT risk (the delta-state change is mechanically simple; the tags-as-labels name/label form is a genuine live-probe design unknown), and they want DIFFERENT differential assertion models (delta-sum vs `LabelPair` split). Decoupling lets the simple delta leg land clean while the riskier label-form pin is isolated to its own SPEC probe + fixture, and each sub-leg ships an independently differential-provable feature (the ADR-0045 by-concern criterion). Cost: one extra full BRAINSTORM→SPEC→PLAN→IMPL cycle and row 47 flips `done` one leg later (47.2b IMPL). The FINAL split-gate (whether each sub-leg itself wants further sub-splitting) is re-checked at each sub-leg's SPEC/PLAN with real LoC counts (the 47.1 ADR-0045 re-check precedent).

Self-answered (no live input required), each verified against the 47.1 as-built code on disk + the ADR-0262 deferral set:

- **Q-delta-state** — the delta state is a PER-SINK `map[string]uint64` of last-flushed Counter values keyed by full dotted name; the delta-aware snapshot is a sink-owned (or sink-parameterized) transform of the registry walk, NOT a mutation of the shared `Flusher.snapshot` fan-out (per-sink because two sinks flush/reconnect independently). Counter-only (Gauges stay absolute). First-flush + reset-on-flush + never-incremented-counter semantics are a 47.2a SPEC live-probe pin (D-MS-DELTA).
- **Q-label-model** — 47.2b REUSES the `name.go` SN1–SN9 tag-extraction logic (`flattenToProm` at `:39`), NOT a brand-new extractor table; the refactor-shared-core-vs-parallel-extractor decision + the exact emitted name/label-key form are 47.2b SPEC pins (D-MS-LABEL). `LabelPair` is already available (`prometheus/client_model v0.6.1`, direct since 47.1).
- **Q-self-stats** — the +0-self-stats posture (D-MS-STATS-FINAL) is PRESERVED across both legs; neither knob registers a new stat (the delta state is internal sink bookkeeping, not a registered metric; the label split is a mapping transform). No interaction with the self-referential-flush subtlety.
- **Q-differential-shape** — TWO new fixture dirs (`0090-stats-sink-metrics-service-deltas` at 47.2a, `0091-stats-sink-metrics-service-labels` at 47.2b), NOT prongs on `0089` — because each sub-leg needs a DIFFERENT bootstrap config (`report_counters_as_deltas:true` vs `emit_tags_as_labels:true`) and a fixture dir is ONE bootstrap pair = ONE runner branch (`reference_differential_fixture_dispatch_constraint`). Both inherit the two-per-side-receiver + hard-`Close()` periodic-flush scaffolding from `0089`.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0262 — especially ADR-0262 [the 47.1 anchor + the deferral set lifted here], ADR-0059 [the in-tree `Registry` + the `Walk` contract], ADR-0060 [histograms deferred — still moot for both legs], ADR-0061 [the SN1–SN8 Prometheus name/label rules — the 47.2b tag-extraction substrate], ADR-0080 [strict-reject — 47.2 LIFTS the two knob rejects, KEEPS the rest], ADR-0064 [the `stats_sinks[]` silent-ignore — 47.1 already lifted the metrics_service sink], ADR-0158 [the `grpcclient.Dialer` typed-wrapper], ADR-0106/0045/0044/0227), the 47.1 as-built code (`internal/statssink/{mapping,sink,flusher}.go`, `internal/bootstrap/bootstrap.go` [`StatsSinkConfig` `:272`, `parseMetricsServiceConfig` `:431`, the deltas reject `:446`, the tags reject `:449`], `internal/stats/name.go` [`flattenToProm` `:39`, SN1–SN9], `internal/stats/{counter,gauge}.go` [`Load`], `cmd/envoy-go/main.go` [the post-`Freeze` `Flusher.Start` wiring]), and `test/fixtures/0089-stats-sink-metrics-service` + `test/helpers/metricsservice` (the differential template). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to each sub-leg's SPEC-drafting time per the phase 09–47.1 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/47-stats-sink-metrics-service/BRAINSTORM.md` section-for-section, reframed for the SECOND-and-FINAL leg's sub-split (two focused mapping-path modifications on the landed 47.1 substrate — NOT a new framework piece). Per the context-isolation discipline, every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-29.

---

## 1. Mission and scope confirmation (47.2 — the sub-split + the 2 sub-legs)

### 1.1 What phase 47.2 delivers as the FINAL leg of row 47 (deltas + tags-as-labels)

Phase 47.2 completes the `metrics_service` stats sink by lifting the two knobs the 47.1 core leg strict-rejected, each as an independent sub-leg:

- **47.2a (`report_counters_as_deltas=true`)** — when the metrics_service `MetricsServiceConfig.report_counters_as_deltas` is true, each COUNTER family carries the per-flush DELTA (`current − last_flushed`) instead of the cumulative absolute value; a per-sink last-value map keyed by full dotted name holds the cross-flush state. Gauges stay absolute. (anticipated ADR-0263)
- **47.2b (`emit_tags_as_labels=true`)** — when `emit_tags_as_labels` is true, each family's dotted-name tag SEGMENTS are extracted into `MetricFamily.metric[].label[]` `LabelPair`s (reusing the `name.go` SN-rule extractors) instead of being inlined into the family name. (anticipated ADR-0264)

When both knobs are absent/false, the export is byte-identical to the 47.1 cumulative/no-labels path (the regression anchor: the full differential, including `0089`, is untouched).

### 1.2 What phase 47.2 does NOT deliver (forward to §8)

statsd / dog_statsd / graphite_statsd / hystrix / wasm / `open_telemetry` metrics sinks (sibling future Observability rows — STAY explicit boot-rejects); histograms + `histogram_emit_mode` (none exist — ADR-0060; STAYS rejected); `stats_flush_on_admin` (the flush-on-scrape oneof variant — STAYS rejected); the `grpc_service.google_grpc` variant (STAYS rejected); non-V3 `transport_api_version` (STAYS rejected); `stats_config` / stats-matcher inclusion-exclusion scoping (a separate surface); the tap filter (a sibling future Observability row); the combined deltas-AND-tags-as-labels interaction as a dedicated fixture (each sub-leg's fixture exercises its OWN knob; the both-true config is accepted once both legs land but is not separately differential-pinned unless the 47.2b SPEC finds a reason).

### 1.3 Phase-done as the FINAL leg of the FOURTH Observability-family row (family STAYS OPEN)

Row 47 (`stats-sink-metrics-service`) registers `in-progress` since the phase-47 brainstorm; it flips `done` at the FINAL sub-leg **47.2b IMPL** (per ADR-0106 + `reference_roadmap_split_phase_row_done` — a fully-consumed split phase flips its row `done` once ALL legs land; the 47.1 IMPL did NOT flip it). The Observability FAMILY STAYS OPEN — statsd / dog_statsd / OTLP metrics / the tap filter remain future rows. There is NO family-close at phase 47.

### 1.4 ADR-0045 split readiness — the sub-split is decided; per-sub-leg FINAL gate re-checked at SPEC/PLAN

The sub-split is decided at this brainstorm (Q-split-47.2 → 47.2a deltas / 47.2b tags-as-labels). The FINAL split gate (whether either sub-leg itself warrants further sub-splitting) is RE-CHECKED at each sub-leg's SPEC/PLAN with real LoC counts (the 47.1 ADR-0045 re-check precedent). Neither sub-leg is expected to need further splitting (each is a single focused mapping-path change + one reject lift + one fixture), but that is a per-sub-leg PLAN call.

### 1.5 Seed-stub alignment + package placement

NO new package. Both sub-legs land inside the EXISTING `internal/statssink/` (the `mapping.go` snapshot transform — stateful for 47.2a, name/label-split for 47.2b — and, for 47.2a, a per-sink last-value map that may live in `sink.go` or a new small `delta.go` within the package, a 47.2a PLAN call) + `internal/bootstrap/bootstrap.go` (the `StatsSinkConfig` two-bool extension + the reject-arm lifts) + `cmd/envoy-go/main.go` (thread the two bools into sink construction). The differential receiver stays the existing `test/helpers/metricsservice` (extended with a delta-sum and a label-aware assert surface). No config-proto blank-import change (the metrics_service proto is already registered at 47.1, ADR-0016).

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for phase 47.2 (unlike the phase-11 local_ratelimit notes).

### 1.7 Phase 47.2's relationship to the existing seams (two focused mapping-path modifications)

Phase 47.2 REUSES everything 47.1 built: the `internal/statssink` `Flusher` + `MetricsServiceSink` + `snapshot` mapping; the `MetricsServiceClient` typed wrapper; the `grpcclient.Dialer`; the `StatsSinkConfig` parse + the post-`Freeze` boot wiring; the `name.go` SN-rule tag extractors (for 47.2b); the two-per-side-receiver + hard-`Close()` `0089` differential template + the `test/helpers/metricsservice` receiver. It ADDS: a per-sink last-value delta map + a Counter-only delta transform (47.2a); a name/label-split mapping path reusing the SN-rule extractors (47.2b); two `StatsSinkConfig` bools + the two reject-arm lifts; two new fixture dirs (`0090`, `0091`); a delta-sum + a label-aware receiver assert surface. It INVENTS no new framework piece, package, go.mod module, or BackendKind.

## 2. Design decisions

### 2.1 Sub-split confirmation: 47.2a deltas → 47.2b tags-as-labels *(Q-split-47.2 → ADR-0263 + ADR-0264)*

By-concern sub-split over the parent's single combined leg. The deltas knob (a stateful per-counter value transform) and the tags-as-labels knob (a name/label-split mapping transform) are independent, differently-risked, and want different differential assertion models. Each sub-leg lifts ONE reject and ships an independently differential-provable feature.

### 2.2 47.2a delta semantics: per-sink last-value map, Counter-only, current−last *(self-answered; pinned at 47.2a SPEC, D-MS-DELTA)*

A per-sink `map[string]uint64` of last-flushed Counter values keyed by full dotted name. Each flush computes `delta = current − last` per Counter, emits the delta as the COUNTER family value, then stores `current`. Gauges stay absolute (no delta — the proto field applies to counters only). The state is sink-owned (two sinks flush/reconnect independently), so the delta-aware snapshot is a sink-parameterized transform of the registry walk, NOT a mutation of the shared `Flusher.snapshot` fan-out. First-flush behavior (emit absolute, or zero-delta, or `current−0`), reset-on-flush vs monotonic accounting, and whether a never-incremented counter is emitted are 47.2a SPEC live-probe pins (D-MS-DELTA). Under a u64 counter (monotone), `current ≥ last` always holds, so the delta is non-negative — no underflow handling needed (the registry counters never decrement; confirm at SPEC).

### 2.3 47.2b label semantics: SN-rule tag extraction → LabelPair, reusing name.go *(self-answered; pinned at 47.2b SPEC, D-MS-LABEL)*

`emit_tags_as_labels=true` extracts the dotted-name tag SEGMENTS into `MetricFamily.metric[].label[]` `LabelPair`s, reusing the `name.go:39` SN1–SN9 tag extractors (the same logic that produces the Prometheus labels). The exact emitted family-NAME form (keep dots? `envoy.`-prefixed tag-name key like `envoy.cluster_name`, or the Prometheus `envoy_cluster_name` key?) and which segments become labels are 47.2b SPEC live-probe pins (D-MS-LABEL). A design call deferred to the 47.2b SPEC: refactor `name.go` to expose a reusable `(name, []Label)` extraction core consumed by BOTH `flattenToProm` and the statssink label mapping, vs a parallel extractor in `internal/statssink`. Labels are emitted in SORTED key order (the mongo `name.go:285` sorted-label precedent; `reference_streaming_sink_differential_framing` asserts the label SET, ordering normalized).

### 2.4 The sink lifecycle + boot wiring are UNTOUCHED *(self-answered)*

`MetricsServiceSink` (`sink.go:60`) is mapping-agnostic — it streams whatever batch it is `Submit`ted. Both knobs change only the batch CONTENT (the mapping), not the streaming. The `cmd/envoy-go/main.go` post-`Freeze` `Flusher.Start` + LIFO-`Close` wiring is unchanged save threading the two new `StatsSinkConfig` bools into the sink/snapshotter construction.

### 2.5 Deferred-policy posture: lift exactly the two knob rejects; KEEP all other rejects *(self-answered; pinned at SPEC, D-MS-REJECT)*

47.2a lifts `bootstrap.go:446` (`report_counters_as_deltas:true`); 47.2b lifts `:449` (`emit_tags_as_labels:true`). EVERY other reject installed at 47.1 STAYS strict (the sibling-TypeURL `:413`, `stats_flush_on_admin` `:405`, non-default `histogram_emit_mode` `:452`, non-V3 transport `:436`, `google_grpc` `:441`, empty cluster `:443`) — per ADR-0262's deferral set + ADR-0080. Each lift is proven by a parse-accepts unit test (the formerly-rejected config now parses to a `StatsSinkConfig` with the bool set) replacing the existing reject unit test.

### 2.6 Stat surface: +0 across both legs (D-MS-STATS-FINAL preserved) *(self-answered; SPEC re-confirms)*

The +0-self-stats posture is PRESERVED: neither knob registers a new stat (the delta map is internal sink bookkeeping; the label split is a mapping transform). No surface delta. `TestNoNewStat_RegistrationGuard` (the 47.1 guard) stays green across both legs. Re-confirmed at each SPEC live probe (the reference emits no delta/label-specific self-stat).

## 3. Framework-survey result — NO new framework, NO new package, NO new go.mod module (47.2)

### 3.1 Framework: two focused transforms on the landed `internal/statssink` mapping path *(per §1.7)*

47.2a adds a per-sink last-value delta transform; 47.2b adds a name/label-split transform reusing the `name.go` SN-rule extractors. No new subsystem.

### 3.2 NEW packages: NONE. Both sub-legs land inside `internal/statssink` + `internal/bootstrap` + `cmd/envoy-go/main.go`.

### 3.3 go.mod modules: NONE new. `github.com/prometheus/client_model v0.6.1` (carrying `MetricFamily`/`Metric`/`Counter`/`Gauge`/`LabelPair`) is already direct (landed at 47.1); `LabelPair` is already available for 47.2b. `go mod tidy -diff` stays EMPTY.

### 3.4 REUSES

The `internal/statssink` `Flusher` + `MetricsServiceSink` + `snapshot` mapping (ADR-0262); the `MetricsServiceClient` + `grpcclient.Dialer` (ADR-0158); the `name.go` SN-rule tag extractors (ADR-0061, for 47.2b); the `stats.Registry.Walk` + `Counter`/`Gauge` `Load` (ADR-0059); the `StatsSinkConfig` parse + the post-`Freeze` boot wiring; the two-per-side-receiver + hard-`Close()` + poll-to-converge + wire-format-both-sides `0089` differential model + the `test/helpers/metricsservice` receiver.

## 4. Bootstrap-level applicability — the `StatsSinkConfig` two-bool extension (NOT a new surface)

The metrics_service sink stays the same TOP-LEVEL `stats_sinks[]` surface 47.1 lifted. 47.2 extends the parsed `StatsSinkConfig` (`bootstrap.go:272`, today `{ClusterName string}`) with `ReportCountersAsDeltas bool` (47.2a) + `EmitTagsAsLabels bool` (47.2b), set from `MetricsServiceConfig.report_counters_as_deltas` / `.emit_tags_as_labels` in `parseMetricsServiceConfig` (`:431`) once the corresponding reject arm is lifted. `cmd/envoy-go/main.go` threads each bool into the per-config sink/snapshotter construction. No change to the `stats_flush_interval` parse or the single-Flusher fan-out.

## 5. Stat surface hypothesis — +0 across both legs (47.2)

### 5.1 Stat names (SPEC re-confirms, D-MS-STATS-FINAL)

NO new stat. The delta map is internal sink bookkeeping; the label split is a mapping transform. The metrics-service cluster's incidental `upstream_cx_*`/`upstream_rq_*` are already-registered (no surface delta). +0 preserved across both legs.

### 5.2 envoy-go-strict departure flags

None beyond the documented deferrals (§8) and the surviving strict-reject arms (§2.5). Both knobs are now ACCEPTED (the formerly-strict rejects lifted) — moving them from envoy-go-strict-reject to reference-parity-accept.

### 5.3 Anticipated surface arithmetic

stat surface **1200** UNCHANGED at 47.2a AND 47.2b (non-H2 **1196** unchanged); re-confirmed at each SPEC live probe.

## 6. Differential fixture envelope — TWO new directories (one per sub-leg)

### 6.1 Fixtures

- **`0090-stats-sink-metrics-service-deltas`** (47.2a; fixtures **91 → 92**): the `0089` topology + workload (K deterministic requests through a basic HCM listener, an h2c metrics cluster, two per-side receivers, hard `Close()`), with `report_counters_as_deltas:true`. Asserts the **SUM of each deterministic counter's deltas across all received flushes == K** cross-side EXACT (poll-to-converge the running per-name sum — a release barrier, never a sleep). The delta model means a single flush's value is partial; the sum is the invariant.
- **`0091-stats-sink-metrics-service-labels`** (47.2b; fixtures **92 → 93**): the `0089` topology + workload, with `emit_tags_as_labels:true`. Asserts the deterministic counter subset's `{family-name, sorted LabelPair[]}` split cross-side EXACT (e.g. the `cluster.c_backend.upstream_rq_total` family's extracted cluster-name label) PLUS the cumulative value `== K`. The exact asserted name/label-key form is pinned at the 47.2b SPEC live probe (D-MS-LABEL).

Two separate dirs (NOT prongs on `0089`) because each needs a DIFFERENT bootstrap config and a fixture dir is ONE bootstrap pair = ONE runner branch (`reference_differential_fixture_dispatch_constraint`).

**Scaffolding-clone note (47.2a SPEC):** `0090`/`0091` clone the AUTHORITATIVE `0089/driver/driver.go` (two per-side receivers + hard `Close()`), NOT the STALE `0089/README.md` (which still describes the pre-correction single-shared-receiver + per-side `Reset()` model — a known artifact defect; the driver code is the truth, `reference_periodic_sink_differential_two_receivers`). Reconcile the `0089` README at the 47.2a SPEC/IMPL when the scaffolding is cloned.

### 6.2 Total

fixtures **91 → 92** at 47.2a (`0090`) **→ 93** at 47.2b (`0091`).

### 6.3 New BackendKind: NONE (the driver-owned receiver stays; BackendKind 38)

The differential receiver stays the driver-owned `test/helpers/metricsservice` `MetricsServiceServer` (extended with a delta-sum accumulator + a label-aware assert surface) — NOT a runner `BackendKind` (`reference_differential_grpc_receiver_driver_owned`). BackendKind tail stays **38**.

### 6.4 New fuzzer: NONE anticipated

`FuzzStatsSinkConfigParse` (landed at 47.1) already fuzzes the `stats_sinks[]`/`MetricsServiceConfig` parse INCLUDING the two knob fields (they are parsed today — into the reject arms). Lifting the rejects keeps them inside the same fuzzed parse path (now accepted instead of rejected), so no new fuzzer is needed. Reconcile the running `^func Fuzz` total (currently 50 actual) per `reference_fuzzer_count_docs_drift` if that changes at a SPEC/PLAN call.

## 7. Anticipated ADRs — ADR-0263 (47.2a deltas) + ADR-0264 (47.2b tags-as-labels)

- **ADR-0263** (47.2a, §Context DRAFT anchored here) — `report_counters_as_deltas=true`: the per-sink last-flush Counter delta state (`map[string]uint64` keyed by full dotted name; `current−last` per flush; Counter-only, Gauges stay absolute); the sink-owned delta transform (NOT a `Flusher.snapshot` mutation); the `bootstrap.go:446` reject lift + the `StatsSinkConfig.ReportCountersAsDeltas` field; the `0090` delta-sum differential. CONTINUES the Observability family; row 47 STAYS `in-progress`.
- **ADR-0264** (47.2b, anticipated) — `emit_tags_as_labels=true`: the SN-rule tag→`LabelPair` extraction (reusing `name.go`'s SN1–SN9 extractors; the exact name/label-key form pinned at the 47.2b SPEC); the `bootstrap.go:449` reject lift + the `StatsSinkConfig.EmitTagsAsLabels` field; the `0091` LabelPair differential; **flips ROW 47 `done`** (the FINAL leg); the Observability family STAYS OPEN.

Per ADR-0044, each ADR's §Decision+§Consequences body lands at its sub-leg's IMPL; the §Context draft is anchored at the sub-leg's SPEC (ADR-0263 §Context is drafted at the 47.2a SPEC, promoted from SPEC-47.2a §13). This brainstorm anchors the ADR-0263 §Context as a forward-looking DRAFT only.

## 8. Deferred items

statsd / dog_statsd / graphite_statsd / hystrix / wasm / `open_telemetry` metrics sinks (sibling future Observability rows — STAY explicit boot-rejects); `stats_flush_on_admin` (STAYS rejected); histograms + `histogram_emit_mode` (none exist — ADR-0060; STAYS rejected); the `grpc_service.google_grpc` variant (STAYS rejected); non-V3 `transport_api_version` (STAYS rejected); `stats_config` / stats-matcher inclusion-exclusion scoping (a separate surface); gRPC reconnect-backoff tuning beyond the basic 47.1 reconnect; the tap filter (a sibling future Observability row); a dedicated both-knobs-true (deltas AND tags-as-labels) interaction fixture (not separately pinned unless the 47.2b SPEC finds a reason). Each is a future row or stays a strict-reject.

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 47.1 (ADR-0262 §Consequences) deferred `report_counters_as_deltas:true`, `emit_tags_as_labels:true`, non-default `histogram_emit_mode`, `stats_flush_on_admin`, and the sibling sink TypeURLs to "47.2 (anticipated ADR-0263, chartered by its own brainstorm)". Phase 47.2 PICKS UP exactly the first two (the deltas + tags-as-labels knobs); the remaining three (histogram_emit_mode, stats_flush_on_admin, sibling TypeURLs) STAY deferred/strict-rejected (NOT un-deferred by 47.2). ADR-0061 (phase 06.1/09) established the SN1–SN9 tag extractors — 47.2b PICKS UP that extraction logic for the `LabelPair` mapping (a reuse, not a re-implementation). No other open deferral is discharged by phase 47.2.

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

Executed IN-SESSION at each sub-leg's SPEC against `envoyproxy/envoy:contrib-v1.37.2` via `reference_docker_probe_bridge_network` (a shared bridge; a metrics-receiver hostname reachable from BOTH containers; verify the receiver actually decoded ≥1 `MetricFamily` from the reference):

- **D-MS-DELTA** (47.2a) — the exact delta semantics the reference emits under `report_counters_as_deltas:true`: first-flush behavior (does flush #1 emit the absolute value, a zero delta, or `current−0`?); reset-on-flush vs monotonic accounting; whether a never-incremented (zero) counter is emitted at all under deltas; whether the delta is computed against the previous FLUSH or the previous SEND; the type enum stays `COUNTER`. Confirm the registry's u64 counters never decrement (non-negative deltas, no underflow).
- **D-MS-DELTA-GAUGE** (47.2a) — confirm the reference leaves GAUGE families absolute (un-delta'd) under `report_counters_as_deltas:true` (the proto field name says *counters*).
- **D-MS-DELTA-SUBJECT** (47.2a) — the deterministic counter subset whose per-flush deltas SUM to K cross-side EXACT; confirm both sides' running sums converge to the same value (the `0090` release barrier).
- **D-MS-LABEL** (47.2b) — the exact `MetricFamily` shape the reference emits under `emit_tags_as_labels:true`: the family-NAME form (does it keep dots? strip the tag value? `envoy_`-flattened or internal-dotted?); the `LabelPair` KEY form (`envoy.cluster_name` vs `envoy_cluster_name` vs `cluster_name`); which dotted-name segments become labels (cluster name, HCM stat_prefix, listener address, response-code class?); the label ORDER (asserted as a set). Confirm the chosen deterministic subset's label split is cross-side stable.
- **D-MS-LABEL-REUSE** (47.2b) — the design call: refactor `name.go` to expose a shared `(name, []Label)` extraction core consumed by both `flattenToProm` and the statssink label mapping, vs a parallel extractor in `internal/statssink`; pinned once D-MS-LABEL fixes the target form.
- **D-MS-STATS-FINAL-2** (per sub-leg) — re-confirm +0 self-stats (the reference emits no delta/label-specific self-stat); the `TestNoNewStat_RegistrationGuard` stays green.
- **D-MS-SPLIT-2** (per sub-leg) — the FINAL ADR-0045 split-gate re-check with real LoC counts at each sub-leg's SPEC/PLAN (expected: no further sub-split).
- **D-MS-REJECT-2** (per sub-leg) — confirm the reference ACCEPTS the lifted knob (it boots `report_counters_as_deltas:true` / `emit_tags_as_labels:true`), making the lift reference-parity; the other rejects STAY (re-verify the reference's stance on each is unchanged).

## 11. Prior-phase lessons applied

- `reference_periodic_sink_differential_two_receivers` — both `0090` + `0091` inherit the TWO driver-owned per-side receivers + the hard `Close()` (NOT a single shared receiver + `GracefulStop`); the periodic flush means the reference streams the whole test, so a shared accumulator contaminates the subject snapshot and renders subject-side deliberate breaks vacuous.
- `reference_wire_format_both_sides_see_same_bytes` — the `StreamMetricsMessage` framing is shared; the receiver decodes the identical proto from reference + subject; adopt the reference's delta/label wire form verbatim.
- `reference_docker_probe_bridge_network` — the SPEC live probes + the differentials use a shared bridge with a receiver hostname reachable from BOTH containers; verify the receiver actually decoded ≥1 `MetricFamily` from the reference (decode-ran proof).
- `reference_concurrency_differential_release_barrier` — `0090` polls-to-converge the running delta SUM; `0091` polls the cumulative subset; never a `time.Sleep`.
- `reference_streaming_sink_differential_framing` — assert the aggregated payload (the delta-sum / the `{name, label-set}` split), NOT stream/message framing nor per-message family count (which vary side-to-side); the label SET, ordering normalized.
- `reference_differential_grpc_receiver_driver_owned` — the metrics receiver stays a driver-owned `test/helpers/metricsservice` server, NOT a runner `BackendKind` (BackendKind stays 38).
- `reference_differential_fixture_dispatch_constraint` — each sub-leg gets its OWN fixture dir (different bootstrap config = different runner branch); `0090` and `0091` cannot share a dir.
- `reference_differential_run_selector` — `-run 'TestDifferential/0090'` / `'TestDifferential/0091'`, NEVER a bare `'0090'`.
- `reference_differential_break_protocol_count1` — `-count=1` on every deliberate-break AND every `-race` run.
- `reference_full_suite_race_after_background_mutator` — the per-sink delta map is a NEW mutable state read by the writer-goroutine path; after 47.2a lands, re-run the FULL `internal/statssink` package `-race`, NOT a `-run` subset.
- `reference_dynamic_stat_name_charset_guard` — if 47.2b's label extraction derives a config/wire-derived label segment, guard it through `stats.IsValidName` semantics where applicable (the SN-rule extractors already operate on already-valid registered names, so this is likely moot — confirm at the 47.2b SPEC).
- `reference_fuzzer_count_docs_drift` — reconcile the running fuzzer total (50 actual) if a fuzzer lands; none anticipated.
- `reference_roadmap_split_phase_row_done` — row 47 flips `done` only at the FINAL sub-leg 47.2b IMPL (ALL three legs landed).
- `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_push_to_origin` / `feedback_execution_style` / `feedback_pertask_gofmt_lint` / `feedback_subagent_autocommit_claudemd` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting` — the stage discipline for the SPEC/PLAN/IMPL sessions.

## 12. Section closeout

Phase 47.2 completes the Observability family's FOURTH row with the two deferred `metrics_service` knobs, SUB-SPLIT by-concern into 47.2a (`report_counters_as_deltas` — a per-sink last-flush Counter delta state, Counter-only; anticipated ADR-0263; `0090` delta-sum differential) and 47.2b (`emit_tags_as_labels` — SN-rule tag→`LabelPair` extraction reusing `name.go`; anticipated ADR-0264; `0091` LabelPair differential). Both compose entirely on the landed 47.1 substrate (ADR-0262) — NO new package, NO new go.mod module, NO new BackendKind, +0 stat surface — lifting exactly the two knob rejects (`bootstrap.go:446` / `:449`) while KEEPING every other 47.1 strict-reject. Counts UNCHANGED at this brainstorm (stat **1200** / fixtures **91** / fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0262**, next-free **ADR-0263**). Anticipated at the 47.2a IMPL: stat **1200** / fixtures **92** (`0090`) / fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0263**; at the 47.2b IMPL: stat **1200** / fixtures **93** (`0091`) / fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0264**. Row 47 STAYS `in-progress` (flips `done` at the FINAL leg 47.2b IMPL); the Observability FAMILY STAYS OPEN.

**NEXT → the 47.2a (`report_counters_as_deltas`) SPEC** (the SPEC stage: execute the §10 D-MS-DELTA{,-GAUGE,-SUBJECT} + D-MS-STATS-FINAL-2 + D-MS-REJECT-2 pins live against `contrib-v1.37.2` per `reference_docker_probe_bridge_network`; anchor the ADR-0263 §Context draft; a docs-only commit direct on master; a fresh worktree off master per `feedback_git_worktrees` if any code-adjacent probe artifacts land). The BRAINSTORM predecessor — `phase 47.1 (stats-sink metrics_service) IMPL done` (squash `77f776bd`; ADR-0262; row 47 `in-progress`; the Observability family STAYS OPEN).
