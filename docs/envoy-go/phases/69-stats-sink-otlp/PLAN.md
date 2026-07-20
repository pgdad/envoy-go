# PLAN 69 — the OpenTelemetry OTLP metrics stats sink (`envoy.stat_sinks.open_telemetry`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-69-plan`, branch `phase-69-stats-sink-otlp-plan`, tip **`0ea2ec20`** (the phase-69 SPEC squash — master; production code byte-identical to `7a293a06` since the SPEC was docs-only), per `feedback_git_worktrees`.
>
> **Row 69 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, §10). **ADR-0291's §Context is ALREADY DRAFTED** at the SPEC squash (`grep -n '^## ADR-0291' docs/envoy-go/DECISIONS.md`, STATUS: **IN PROGRESS**); the IMPL **COMPLETES ADR-0291 IN PLACE** with §Decision + §Consequences — it does NOT append a new ADR, does NOT renumber. DECISIONS tail stays **ADR-0291**, next-free **ADR-0292** (`[RUN]`: `grep -c '^## ADR-0292' docs/envoy-go/DECISIONS.md` → 0). **This PLAN adds NO ADR content.**
>
> **Baselines RE-DERIVED at `0ea2ec20` (`[RUN]`, NOT copied):** fixtures **113** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0111-tls-cvc-empty-dynamic-fallback`, in-container port **10447** ⇒ next fixture `0112`, next port **10448**) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** · DECISIONS tail **ADR-0291** (next-free ADR-0292) · go.mod modules **2** (lineage figure; the single `go.mod` requires 67, incl. `go.opentelemetry.io/proto/otlp v1.0.0` DIRECT at `go.mod:17` — re-check `git diff go.mod` after tidy).
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 69`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **No deferred-sentence edit at ANY stage of this row** (SPEC §12); ROADMAP:198 rolls "OTLP-metrics stats sink + " OUT at the IMPL row-done edit, NOT before.
>
> **⚠️ NO PARALLEL STREAM.** Master (`0ea2ec20`) IS the SPEC squash — the ONLY delta over the BRAINSTORM tree `7a293a06` is docs (`SPEC.md` + the ADR-0291 §Context append). So the production tree is byte-identical to what the SPEC re-derived; §1 is the structural decisions the SPEC delegated to the PLAN plus a full re-verification (which found **ZERO drift** — every SPEC §11 anchor exact).
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence (phase-66's PLAN carried nine draft defects). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`).

---

## 1. Re-derivation ledger — every SPEC §11 anchor re-opened at `0ea2ec20`

**All SPEC §3/§9/§11 code anchors RE-DERIVED at `0ea2ec20` by reading `internal/statssink/{sink,delta,label,mapping,flusher}.go`, `internal/bootstrap/bootstrap.go`, `internal/bootstrap/statssink_fuzz_test.go`, `internal/grpcclient/grpcclient.go`, `cmd/envoy-go/main.go`, the pinned go-control-plane `open_telemetry/v3/open_telemetry.pb.go`, the `go.opentelemetry.io/proto/otlp@v1.0.0/collector/metrics/v1/*` protos, `test/helpers/otlptrace/otlptrace.go`, and `docs/envoy-go/BEHAVIOR_CONTRACT.md`.**

**RESULT: ZERO unacknowledged drift.** Every symbol and control-flow claim in SPEC §3/§9/§11 re-derived TRUE at its cited line; the one prior line drift (the BRAINSTORM's `StatsSinkConfig` `:357-363`) was ALREADY folded by the SPEC as C5 (`:357-372`, confirmed here). Unlike phase 68 (which found RD-LINES boundary drift), **there is no line-boundary correction to carry** — the SPEC's anchors are adopted verbatim. The findings below (RD*) are therefore the *structural decisions the SPEC delegated to the PLAN* (the exact code skeletons an implementer clones) plus the load-bearing re-confirmations; adversarial verification (§1.2) must confirm or refute each.

| # | Anchor / SPEC claim | RE-DERIVED at `0ea2ec20` | Where |
|---|---|---|---|
| **RD-EXACT** | SPEC §11 cites ~35 code anchors | **ALL EXACT.** `sink.go` `Sink`:18-21 / `metricsClient`:28-31 / cap-8 const :37 / `MetricsServiceSink` struct :71, `run` :158-209 · `delta.go` `deltaState`:18 / `apply`:39 · `label.go` `labelMapper`:22 / `ExtractTags` call :38 · `mapping.go` `snapshot`:22-47 · `flusher.go` :13-51 / no-final-flush :31-32 / `flushOnce` share :46-51 · fuzzer `internal/bootstrap/statssink_fuzz_test.go:19` · `bootstrap.go` `metricsServiceTypeURL`:224 / `parseStatsSinks`:532-569 / reject:564-565 / `parseMetricsServiceConfig`:580-604 / histogram-reject:595-597 / cluster gate:588-593 / `.GetValue()`:600 / `StatsSinkConfig`:357-372 / slices:431/442/452/464/469 / `DiscardUnknown:false`:510 · `SinkConfig` getters :96/103/110/117/124/131 · `grpcclient.go`:474/:521/roster:153-157 · `main.go` gate:206 / sub-loop:207-215 / `NewFlusher`:275 / drain:286-291 · `go.mod:17` · BC:817/:835/:836/:638. | all |
| **RD-STREAMING** | SPEC §3.7/C3: clone the UNARY OTLP-logs writer, NOT the streaming `MetricsServiceSink` | **CONFIRMED.** `MetricsServiceSink.run` (`sink.go:158-209`) is CLIENT-STREAMING (`StreamMetrics` + `sentIdentifier` once-per-stream + reopen-resend). The OTLP sink does ONE unary `Export` per flush (P-2), so its writer clones the `internal/accesslog` OTLP-logs / `internal/tracing` OTLPExporter shape: bounded channel + writer goroutine + `client.Export(ctx, req)` + retry-once + drop-newest + idempotent `Close`. **The delta+label mapping site is `run` NOT `Submit`** — re-derived at `sink.go:194-202`: the streaming sink applies `s.delta.apply(batch)` THEN `s.labels.apply(batch)` inside `run` "so that a batch dropped at enqueue never latched deltaState" (`sink.go:68-69` verbatim). The OTLP sink MIRRORS this: apply delta (if configured) in the writer, then map to OTLP — an enqueue-drop never latches `deltaState`. | T2 |
| **RD-SEAM** | SPEC §3.1: a package-local `otlpMetricsClient` interface seam keeps `internal/statssink` grpcclient-free | **CONFIRMED buildable + the pattern re-derived.** `sink.go:28-31`'s `metricsClient` seam (`StreamMetrics`/`Close`) is the exact precedent; the OTLP seam is `otlpMetricsClient interface { Export(ctx, *colmetricspb.ExportMetricsServiceRequest) error; Close() error }`. `sink.go` imports `corev3`/`metricsv3`/`dto` ONLY (re-derived `sink.go:3-12`) — the PRODUCTION sink has NO grpcclient, NO otlp-proto import. (`sink_test.go:15` DOES import grpcclient TEST-only for the compile-time seam assertion `var _ metricsClient = (*grpcclient.MetricsServiceClient)(nil)`; the OTLP test may mirror that safely — it does not affect the production dep graph.) `otlp.go` ADDS only the `colmetricspb` + `commonpb`/`metricspb`/`resourcepb` otlp proto imports — NO grpcclient import (the seam). The cycle guard `go list -deps ./internal/statssink` (PRODUCTION deps only) stays clean — re-verify at T10 (TYPE-level, `reference_xds_config_seam_transitive_cycle_guard`). | T2, T10 |
| **RD-DELTA** | SPEC §3.9: `report_counters_as_deltas` rides the landed `deltaState`; startTime chains | **CONFIRMED signature.** `deltaState.apply(abs []*dto.MetricFamily) []*dto.MetricFamily` (`delta.go:39`) rebuilds COUNTER families to per-flush deltas, shares GAUGE by pointer, **returns a NEW batch (never mutates input)** — reusable verbatim. For DELTA the OTLP `Sum.aggregationTemporality=DELTA`, `isMonotonic` RETAINED, per-window values; startTime CHAINS to the previous flush's `timeUnixNano` (the sink tracks `lastFlushNanos`). For cumulative, `Sum.aggregationTemporality=CUMULATIVE`, startTime = a FIXED process-start ns constant across flushes. Gauge datapoints carry NO startTime. envoy-go emits CORRECT ns — the reference's factor-1000 µs bug is NOT cloned (`reference_wire_format_both_sides_see_same_bytes` protects frame VALIDITY, not bug-cloning). | T2 |
| **RD-TAGS** | SPEC §3.5: `use_tag_extracted_name`/`emit_tags_as_attributes` via `ExtractTags`, projected to name/`KeyValue` | **RE-DERIVED — the two knobs are INDEPENDENT, so the OTLP sink calls `stats.ExtractTags` DIRECTLY (not `labelMapper.apply`).** `label.go:38` shows `labelMapper` calls `stats.ExtractTags(fam.GetName())` → `(residual, labels, err)` and (`:46`) forms the key `"envoy." + strings.TrimPrefix(l.Key, "envoy_")` as a `dto.LabelPair`. But `labelMapper.apply` COUPLES name-rewrite + label-emit; phase 69's two knobs are INDEPENDENT (P-5), so `otlp.go` calls `stats.ExtractTags(fam.GetName())` itself (same-package, exactly as `labelMapper` does) and decides name (residual vs full-dotted) and attributes (`KeyValue{"envoy."+TrimPrefix(key,"envoy_"), value}` vs none) SEPARATELY. Attributes are extracted regardless of `useTagExtractedName` (they gate on `emitAttrs` alone). | T2 |
| **RD-WRAPPER** | SPEC §3.5/C2: the two `*wrapperspb.BoolValue` knobs default TRUE (nil→TRUE), INVERTING the metrics_service `.GetValue()` template | **CONFIRMED against the pinned descriptor.** `SinkConfig.GetEmitTagsAsAttributes()` (`:117`) + `GetUseTagExtractedName()` (`:124`) are `*wrapperspb.BoolValue`, struct-doc "Default value is true." The metrics_service template `msc.GetReportCountersAsDeltas().GetValue()` (`bootstrap.go:600`) is ALSO a `*wrapperspb.BoolValue` read but is left nil→FALSE (`.GetValue()` on a nil wrapper is the zero value). Phase 69 INVERTS: `w := cfg.GetX(); enabled := w == nil || w.GetValue()` (nil → TRUE). The two DELTA knobs (`GetReportCountersAsDeltas()` :103, `GetReportHistogramsAsDeltas()` :110) are SCALAR `bool`; `GetPrefix()` :131 scalar string. Fixture YAML takes the BARE scalar (`use_tag_extracted_name: false`), NEVER `{value:false}` (`reference_protojson_wrapper_scalar_not_object`). | T3 |
| **RD-SKEW** | SPEC §3.3/C1: the v1.37.2-only `resource_detectors`/`custom_metric_conversions` are boot-rejected FOR FREE by `DiscardUnknown:false` | **CONFIRMED — the ONLY `DiscardUnknown` on the Load path is `false` (`bootstrap.go:510`), and the two fields are ABSENT from the pinned v1.32.4 `SinkConfig` (0 grep hits in `open_telemetry.pb.go`).** Once the `SinkConfig` descriptor is blank-imported (required for `@type` resolution + `openTelemetrySinkTypeURL`), a bootstrap setting either field hits a protojson "unknown field" error at boot — a STRUCTURAL reject, ZERO new detect-and-reject code. A SUBJECT unit test pins it (T3 Step); a named envoy-go-STRICT version-skew DEPARTURE (BC §9 B2 + ADR-0291), revisited at any go-control-plane bump. | T3, T10 |
| **RD-COLLIDE** | SPEC §3.6/C4: name the grpcclient wrapper `OTLPMetricsClient` — `colmetricspb`'s own type is `MetricsServiceClient` | **CONFIRMED — a real simple-name collision.** `go.opentelemetry.io/proto/otlp@v1.0.0/collector/metrics/v1/metrics_service_grpc.pb.go` declares `type MetricsServiceClient interface` + `func NewMetricsServiceClient(...)`; `go-control-plane/envoy@v1.32.4/service/metrics/v3/metrics_service_grpc.pb.go:28` ALSO declares `type MetricsServiceClient interface` (the streaming one, alias `metricsv3` — already used by the metrics_service sink). Distinct packages, same simple name. The grpcclient wrapper is `OTLPMetricsClient` (aliasing `colmetricspb "…/collector/metrics/v1"`); `connHolder`'s roster comment already lists the streaming `MetricsServiceClient` (`grpcclient.go:154-156`), reinforcing the need. | T1 |
| **RD-FUZZLOC** | SPEC §11: the fuzz seed lands in `FuzzStatsSinkConfigParse` at `statssink_fuzz_test.go:19` | **CONFIRMED — the file is `internal/bootstrap/statssink_fuzz_test.go` (in `bootstrap`, NOT `internal/statssink`).** SPEC cites it correctly. The per-sink `@type` consts are `const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"` (:28), `const statsdType = …` (:29); seeds interpolate them into a `f.Add([]byte(head + \`stats_sinks: … "@type": \` + msType + …))` block. Phase 69 adds `const otlpType = "type.googleapis.com/envoy.extensions.stat_sinks.open_telemetry.v3.SinkConfig"` + a seed. **The dispatch trap:** the seed reaches `parseOpenTelemetrySinkConfig` ONLY once the `SinkConfig` descriptor is blank-imported in `bootstrap.go` (else protojson errors `unable to resolve "…SinkConfig": not found` BEFORE dispatch — V1 executed this exact text) — dispatch-verify at T8. | T8 |
| **RD-VERSION** | SPEC §3.8: the `telemetry.sdk.*` triple — "the PLAN pins the exact strings from what the binary exposes" | **PINNED.** `telemetry.sdk.name = "envoy-go"` (matching `internal/tracing/exporter.go:48` `defaultScopeName = "envoy-go"`), `telemetry.sdk.language = "go"`, `telemetry.sdk.version = admin.BuildVersionString()` (the `/server_info` version string, `internal/admin/version.go:51` — `<sha7>/<go-version>/Clean/RELEASE/Go-crypto`). **THREADING (RE-DERIVED):** `main.go` ALREADY imports `internal/admin` (`main.go:27`) and builds `minNode` from the bootstrap (`main.go:164`); the version string is threaded into `NewOTLPMetricsSink` as a constructor parameter (mirroring `minNode` into `NewMetricsServiceSink`), keeping `internal/statssink` import-clean (no admin import in statssink). The `0112` differential asserts the three KEYS present per side + per-side VALUES (never cross-side equality); empty `scope` on both. | T2, T4 |
| **RD-RECV** | SPEC §5/§10 T5: `test/helpers/otlpmetrics` clones the driver-owned unary-Export receiver | **CONFIRMED absent + the template pinned.** `test/helpers/otlpmetrics` does NOT exist; `otlplogs`/`otlptrace`/`metricsservice` siblings DO. `otlptrace/otlptrace.go` is the exact template: `New(t testing.TB) *Server` / `NewAtAddr(addr)`; `Export(ctx, *ExportTraceServiceRequest) (*ExportTraceServiceResponse, error)` flattens ResourceSpans/ScopeSpans under an RWMutex; accessors `ResourceAttributes()` :165, `Reset()` :181, `Addr()` :191, `Stop()`/`Close()` (hard-stop, `sync.Once`). `otlpmetrics` clones this over `colmetricspb.MetricsServiceServer`, accumulating datapoints keyed `(name, sorted-attrs)` order-insensitive + Sum/Gauge/temporality/resource-key/startTime + delta-SUM accessors — driver-owned, NOT a BackendKind (`reference_differential_grpc_receiver_driver_owned`; tail stays 38). | T5 |

### 1.1 Structural decisions the SPEC delegated to the PLAN (each RE-DERIVED, not invented)

- **The `OTLPMetricsSink` shape + seam (T2).** `otlp.go` (NEW FILE, package `statssink`) declares the `otlpMetricsClient` seam and:
  ```go
  type OTLPMetricsSink struct {
      client  otlpMetricsClient
      ch      chan []*dto.MetricFamily
      done    chan struct{}
      cancel  context.CancelFunc
      closeOnce sync.Once

      // config (from OTLPSinkConfig; nil→TRUE wrappers resolved at parse, §3.5)
      useTagExtractedName bool
      emitAttrs           bool
      prefix              string
      delta               *deltaState // non-nil ⇒ report_counters_as_deltas (ADR-0263); nil ⇒ cumulative absolute

      // resource — the telemetry.sdk.* triple (RD-VERSION), built once
      resourceAttrs []*commonpb.KeyValue

      // startTime bookkeeping (RD-DELTA)
      startNanos    uint64 // process-start; the CUMULATIVE startTimeUnixNano (constant)
      lastFlushNanos uint64 // the previous flush's timeUnixNano — the DELTA startTime chain
      // rate-limited drop log (the accesslog lastDropLog idiom; drops LOGGED not counted, +0 stats)
  }
  func NewOTLPMetricsSink(client otlpMetricsClient, version string, reportCountersAsDeltas, useTagExtractedName, emitTagsAsAttributes bool, prefix string) *OTLPMetricsSink
  ```
  `Submit(batch)` is the non-blocking channel send (clone `MetricsServiceSink.Submit`, `sink.go:123-138`; drop-newest + rate-limited log on full). `run()` (writer goroutine) dequeues, applies `delta.apply` if configured (RD-STREAMING), maps to ONE `*colmetricspb.ExportMetricsServiceRequest`, and `client.Export(ctx, req)` with **retry-once-per-Export** (fail-open on error → rate-limited LOG, no stat). `Close()` is `sync.Once` + drain-grace + `cancel` + `client.Close()` (clone `sink.go:140-156`).
- **The mapping (T2).** `toExportRequest(batch []*dto.MetricFamily, flushNanos uint64) *colmetricspb.ExportMetricsServiceRequest`: ONE `ResourceMetrics{Resource:{Attributes: s.resourceAttrs}}` / ONE `ScopeMetrics{Scope:{}}` (empty) / one `Metric` per family. Per family: `residual, tags, err := stats.ExtractTags(fam.GetName())` (RD-TAGS; on err, fall back to the full name — mirror `labelMapper`'s err posture, re-derive `label.go`); `name := s.prefix + pick(useTagExtractedName ? residual : fam.GetName())` (prefix composes `<prefix>.<name>`, dot added — P-11; empty prefix ⇒ no dot); `attrs := emitAttrs ? kvFromTags(tags) : nil`. `MetricType_COUNTER` → `Metric_Sum{Sum:{AggregationTemporality: CUMULATIVE|DELTA, IsMonotonic: true, DataPoints:[{Attributes:attrs, StartTimeUnixNano, TimeUnixNano: flushNanos, Value: asDouble/asInt}]}}`; `MetricType_GAUGE` → `Metric_Gauge{Gauge:{DataPoints:[{Attributes:attrs, TimeUnixNano: flushNanos, Value}]}}` (NO startTime on gauge). StartTime: cumulative ⇒ `s.startNanos`; delta ⇒ `s.lastFlushNanos` (0 on the first flush is acceptable — the reference's first-window startTime is its own µs-bugged value, UNASSERTED cross-side; the subject asserts ns-magnitude + cumulative-constant only, §3.9). The mapper builds the sink's OWN protos and never mutates the shared snapshot (`flusher.go:46-51`, the ADR-0263/0264 hard constraint).
- **The parse arm (T3).** `openTelemetrySinkTypeURL` clones the `metricsServiceTypeURL` descriptor idiom (`bootstrap.go:224`): `var openTelemetrySinkTypeURL = "type.googleapis.com/" + string((&otelsinkv3.SinkConfig{}).ProtoReflect().Descriptor().FullName())` (blank/aliased import `otelsinkv3 "…/extensions/stat_sinks/open_telemetry/v3"`). `OTLPSinkConfig{ClusterName, ReportCountersAsDeltas, UseTagExtractedName, EmitTagsAsAttributes, Prefix}` (SIBLING of `StatsSinkConfig` `:357-372`). `Bootstrap.OTLPSinkConfigs []OTLPSinkConfig` (parallel to `:431/442/452/464`). `parseOpenTelemetrySinkConfig(tc *anypb.Any, i int) (OTLPSinkConfig, error)`: `UnmarshalTo` the Any → `SinkConfig`; mirror `parseMetricsServiceConfig`'s rejects (non-V3 `:585-587`, google_grpc/envoy_grpc-required `:588-591`, empty cluster_name `:592-593`); the NEW `report_histograms_as_deltas` reject (§3.4); the nil→TRUE wrapper reads (RD-WRAPPER). Fifth `parseStatsSinks` arm (`case openTelemetrySinkTypeURL:`) + the reject roster four→five (`:564-565` gains a fifth `%q` + arg).
- **The build loop (T4).** `main.go:206` gate gains `|| len(bs.OTLPSinkConfigs) > 0`; a sub-loop (clone `main.go:207-215`) `NewOTLPMetricsClient(dialer, cfg.ClusterName)` → `NewOTLPMetricsSink(client, admin.BuildVersionString(), cfg.ReportCountersAsDeltas, cfg.UseTagExtractedName, cfg.EmitTagsAsAttributes, cfg.Prefix)` → `append(statsSinks, …)`. `NewFlusher` (`:275`) + LIFO drain (`:286-291`) UNTOUCHED.
- **Task granularity.** A SINGLE FLAT ROW, 11 tasks (D-OTLPM-SPLIT resolved, §10 — the substrate is landed so the knobs are parameter-threading; the phase-48/49/57 sink precedent). **ADR-0045 escape valve ARMABLE, UNCONSUMED** (no two-package surface can strand a leg — `internal/xds`/`internal/tls`/`internal/boot`/`internal/listener` untouched). Sequencing: T1 (grpcclient client) → T2 (`internal/statssink/otlp.go`, consumes the seam) → T3 (`internal/bootstrap`, produces `OTLPSinkConfig`) → T4 (`main.go`, wires T1+T2+T3) → T5 (`otlpmetrics` receiver) → T6→T7 (fixtures `0112`/`0113`) → T8 (fuzz) → T9 (BC) → T10 (verify) → T11 (close). T1/T2/T3 are independent enough to build in any order but are sequenced so T4 has all three; T5 is independent (test-only).

### 1.2 Adversarial-pass record

**TWO independent verifiers ran against the draft in PRIVATE scratch before landing** (`reference_parallel_subagents_private_scratch`; the real repo left untouched, no worktrees registered):

- **V1 — code-claims by re-derivation + a DECISIVE by-execution probe of the load-bearing RD-SKEW claim.** V1 re-opened every §11 anchor at `0ea2ec20` (all EXACT), then `git clone --local`d into scratch and RAN the RD-SKEW probe: a real `bootstrapv3.Bootstrap` JSON with a `stats_sinks[]` `open_telemetry` entry, blank-importing the `SinkConfig` descriptor, unmarshaled with `protojson.UnmarshalOptions{DiscardUnknown:false}`. Result — `resource_detectors`/`custom_metric_conversions` each ERROR `unknown field "…"` (line 1:214, INSIDE the Any) with `DiscardUnknown:false` and are silently dropped with `true`; and WITHOUT the blank import protojson errors `unable to resolve "…SinkConfig": not found`. **So RD-SKEW HOLDS under executable probe AND discriminates (Break I fires test 4), and the T8 descriptor-blank-import dispatch trap is real.** V1 also confirmed by-execution/re-derivation: the grpcclient clone shape (`connHolder`/`dialConn`/lowercase `close()`), the `colmetricspb` symbol set + the `metricsv3.MetricsServiceClient` collision, `snapshot` emitting exactly ONE Metric + ZERO labels per family (so "one Metric/one datapoint per family" cannot be refuted), the RD-STREAMING mapping site (`sink.go:198-202`), the six-getter `SinkConfig`, the main.go wiring, the `otlptrace` receiver API + `colmetricspb.MetricsServiceServer`, and every break discriminating. **ZERO SEVERE, ZERO MODERATE; four MINOR** — all folded: (i) RD-SEAM prose sharpened to name `sink_test.go`'s TEST-only grpcclient seam-assertion import (production dep graph unaffected); (ii/iii) the RD-WRAPPER template-read + the metrics_service `.GetValue()` characterization corrected to name the `*wrapperspb.BoolValue` type; (iv) the dispatch-trap error text corrected to the executed `unable to resolve … not found`.
- **V2 — process/consistency/SPEC-coverage/adjudication.** All stage-close mechanics PASS (row 69 stays `in-progress`; the flip + sentence-narrowing correctly deferred to the IMPL row-done; the PLAN adds no ADR content and directs ADR-0291 completion IN PLACE; counts re-run mechanically — fixtures 113 / fuzzers 55 / BackendKind 38 / DECISIONS tail ADR-0291 / next-free ADR-0292 / ports 10448-10449 free / all 10 identifiers 0 hits; the sentinel re-run does NOT fire; the envelope is internally consistent across File structure / Global Constraints / T10 / Self-review / SPEC §15; the break protocol complete incl. the order-insensitive non-break control; format faithful to the phase-68 PLAN). **ONE SEVERE, ZERO MODERATE; two MINOR** — all folded: the SEVERE was the SPEC §7 `TestNoNewStat_OTLPRegistrationGuard` (the one-per-sink +0-stats regression guard, the `registration_test.go` house precedent) being cited as T10's verification mechanism while NO task wrote it — **CORRECTED by adding T2 Step 5b (write the guard, cloned from `_GraphiteRegistrationGuard`) + the `registration_test.go` File-structure entry + the Self-review mapping**; the two MINOR (this §1.2 "appended record" pointer — now resolved by this record; the T8 seed `transport_api_version` re-derivation note — added to T8 Step 2). **The design direction — a fifth `stats_sinks[]` consumer on landed substrate, single flat row, 11 tasks — is unchanged; only the dropped registration guard was a real gap, now closed.**

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-69 IMPL.
- **FOUR functionally-edited production files, ZERO new packages** (SPEC §4, §15): `internal/statssink/otlp.go` (NEW FILE, existing package) · `internal/bootstrap/bootstrap.go` · `internal/grpcclient/grpcclient.go` · `cmd/envoy-go/main.go`. **New exported symbols ONLY in the existing `internal/statssink` (`OTLPMetricsSink`/`NewOTLPMetricsSink`) + `internal/grpcclient` (`OTLPMetricsClient`/`NewOTLPMetricsClient`) rosters** — the normal growth mode for those packages; the `internal/xds` zero-new-symbol discipline (broken once, deliberately, at phase 68) is UNTOUCHED. **BYTE-UNTOUCHED:** `internal/xds`, `internal/tls`, `internal/boot`, `internal/listener`, `validate/`, and every landed `internal/statssink` file (`sink.go`/`delta.go`/`label.go`/`mapping.go`/`flusher.go` REUSED, not edited).
- **gRPC transport ONLY** (P-1). `grpc_service` REQUIRED (a reference PGV boot-reject when `protocol_specifier` is absent); envoy-go mirrors via the Dialer-cluster gate (the metrics_service precedent). A missing/non-H2 cluster `log.Fatalf`s at sink build (REFERENCE-PARITY); empty `cluster_name` / `google_grpc` / non-V3 `transport_api_version` are rejected.
- **The two BoolValue knobs default TRUE** (nil→TRUE, RD-WRAPPER) ⇒ tag-extracted residual names + tags-as-attributes are the CORE default. `report_histograms_as_deltas: true` is STRICT-REJECTED (no histograms, ADR-0060). The v1.37.2-only `resource_detectors`/`custom_metric_conversions` are boot-rejected FOR FREE (RD-SKEW).
- **Counts at the IMPL:** fixtures **113 → 115** (`0112-stats-sink-otlp`@10448 + `0113-stats-sink-otlp-knobs`@10449) · fuzzers **55 (+0, a seed only)** · stat surface **1201 (+0)** · BackendKind **38 (+0)** · go.mod **+0** (SPEC metric "2" carried; re-check `git diff go.mod` after tidy — `reference_new_subpackage_pulls_transitive_module`) · ZERO new packages · DECISIONS tail stays **ADR-0291** (completed IN PLACE; next-free ADR-0292).
- **The pinned §9 wording lands MECHANICALLY** — B1/B2 are named obligations with verbatim replacement text; never silent rewrites, never paraphrases. They land at T9, atomically with ADR-0291 completion at T11's stage-close.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md; the controller squashes at close. Locate commits by SUBJECT (`git log --grep 'phase 69'`), never by position.
- **Cluster-dialed sink accounting** (`reference_cluster_sink_dial_unaccounted`): fixtures MUST NOT cross-side-assert the sink cluster's `upstream_cx_*` (the two sides' dial accounting differs) and MUST NOT reuse `Cluster.Dial` / expect a `max_connections` permit. The sink cluster's own stats ARE themselves exported (P-15, the feedback loop) ⇒ assert APPLICATION-cluster subsets only.
- **`reference_sds_init_fetch_timeout_dial_budget_flake` / `reference_0061_ring_hash_spread_flake`** — a `TestProvider_*_Timeout` under `-race` or a `0061` spread failure is PRE-EXISTING on master (one occurrence each). Do not reflex-classify as a phase-69 regression; a SECOND occurrence justifies investigating margins.

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed.
- **`-count=1` on EVERY break** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first.
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it (the order-insensitive NON-break control, T7, is the archetype: reversing the receiver's attribute iteration must NOT fail).
- **Full selector only:** `-run 'TestDifferential/0112-stats-sink-otlp'` — never bare `0112` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).
- **A framing/count break needs its own isolating assertion** (`reference_framing_break_needs_unparsed_counter` family): the receiver's datapoint-count / dup-name keying must have a break that isolates IT.

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE repo-wide at `0ea2ec20` (`grep -rn --include='*.go'`, `.worktrees` excluded — `[RUN]`, all 0 hits):** `OTLPMetricsSink` · `NewOTLPMetricsSink` · `OTLPMetricsClient` · `NewOTLPMetricsClient` · `otlpMetricsClient` · `parseOpenTelemetrySinkConfig` · `OTLPSinkConfig` · `OTLPSinkConfigs` · `openTelemetrySinkTypeURL` · `otlpType`. **Fixtures:** `test/fixtures/0112-*` / `0113-*` do not exist; in-container ports **10448**/**10449** FREE (`grep -rn '10448\|10449' test/` → 0). **The ONE named collision (RD-COLLIDE):** `colmetricspb.MetricsServiceClient` shares a simple name with `metricsv3.MetricsServiceClient` — the wrapper is `OTLPMetricsClient` to disambiguate. **Any FURTHER name the IMPL coins (e.g. `otelsinkv3` alias, `toExportRequest`, `kvFromTags`, `TestNoNewStat_OTLPRegistrationGuard`, the fixture `package driver` helpers): grep first, record the check.**

---

## File structure

```
internal/grpcclient/grpcclient.go        [EDIT]  T1 (OTLPMetricsClient struct+ctor+Export+Close over colmetricspb; connHolder roster comment append; colmetricspb alias import)
internal/grpcclient/grpcclient_test.go   [EDIT]  T1 (bufconn fake MetricsService — Export forwards; Close idempotent)
internal/statssink/otlp.go               [ADD]   T2 (OTLPMetricsSink + otlpMetricsClient seam + toExportRequest mapping + the unary bounded-channel writer)
internal/statssink/otlp_test.go          [ADD]   T2 (mapping matrix: type/temporality/attrs/names/prefix/delta/startTime; writer drop/retry/Close; -race)
internal/statssink/registration_test.go  [EDIT]  T2 (TestNoNewStat_OTLPRegistrationGuard — the one-per-sink +0-stats guard, cloned from _GraphiteRegistrationGuard)
internal/bootstrap/bootstrap.go          [EDIT]  T3 (openTelemetrySinkTypeURL + SinkConfig blank-import; parseOpenTelemetrySinkConfig; OTLPSinkConfig; Bootstrap.OTLPSinkConfigs; the fifth parseStatsSinks arm; reject roster 4→5; nil→TRUE wrappers; histogram reject; grpc/cluster/non-V3 rejects)
internal/bootstrap/bootstrap_test.go     [EDIT]  T3 (parse-accept all six fields; each reject; the free version-skew boot-reject; the wrapper-default inversion)
cmd/envoy-go/main.go                     [EDIT]  T4 (the :206 gate clause + the OTLP build sub-loop; NewFlusher/drain UNTOUCHED)
test/helpers/otlpmetrics/otlpmetrics.go  [ADD]   T5 ((name, sorted-attrs)-keyed order-insensitive unary-Export receiver + Sum/Gauge/temporality/resource-key/startTime + delta-SUM accessors)
test/helpers/otlpmetrics/otlpmetrics_test.go [ADD] T5 (accessor/keying self-tests; order-insensitivity)
test/fixtures/0112-stats-sink-otlp/          [ADD]  T6 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md), T6 (breaks)
test/fixtures/0113-stats-sink-otlp-knobs/    [ADD]  T7 (deltas+prefix+both-false; delta-SUM stability barrier), T7 (breaks)
internal/bootstrap/statssink_fuzz_test.go [EDIT]  T8 (otlpType const + the open_telemetry seed; dispatch-verified; +0 func Fuzz)
docs/envoy-go/BEHAVIOR_CONTRACT.md        [EDIT]  T9 (B1 NEW OTLP section; B2 the BC:836 sibling-example swap OFF OTLP-metrics)
docs/envoy-go/DECISIONS.md                [EDIT]  T11 (ADR-0291 completed IN PLACE — §Decision + §Consequences)
internal/statssink/{sink,delta,label,mapping,flusher}.go · internal/xds/** · internal/tls/** · internal/boot/** · internal/listener/** · validate/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — `internal/grpcclient`: `OTLPMetricsClient` (the unary `NewOTLPLogsClient` clone over `colmetricspb`)

**Files:**
- Modify: `internal/grpcclient/grpcclient.go` (the `OTLPMetricsClient` stanza after `NewOTLPTracesClient` `:521`; the `connHolder` roster comment `:153-157`; the `colmetricspb` alias import)
- Test: `internal/grpcclient/grpcclient_test.go`

**Interfaces:**
- Produces: `type OTLPMetricsClient struct{…}` + `func NewOTLPMetricsClient(d *Dialer, clusterName string) (*OTLPMetricsClient, error)` + `func (c *OTLPMetricsClient) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error` + `func (c *OTLPMetricsClient) Close() error`. The `Export` return is NARROWED to `error` alone (discarding `*ExportMetricsServiceResponse`) so it satisfies the `otlpMetricsClient` seam (T2) — a shape clone with a narrowed return, NOT byte-for-byte (SPEC §3.6 MINOR).
- Consumes: `dialConn(d, "OTLP metrics client", clusterName)` + the embedded `connHolder`.

**Entry state:** clean `0ea2ec20`-derived branch; `go test ./internal/grpcclient/ -count=1` green.

- [ ] **Step 1 — write the failing unit test (red-first).** In `grpcclient_test.go`, model on the OTLP-logs/traces client test (find it: `grep -n 'NewOTLPLogsClient\|OTLPTracesClient' internal/grpcclient/grpcclient_test.go`; clone the bufconn + fake-server pattern). Add `TestNewOTLPMetricsClient_ExportForwards`: stand a bufconn `colmetricspb.MetricsServiceServer` whose `Export` records the received `*ExportMetricsServiceRequest` and returns an empty ack; dial it through a test `Dialer`; call `client.Export(ctx, req)` with a sentinel `ResourceMetrics`; assert `err == nil` AND the server received the sentinel. Add `TestNewOTLPMetricsClient_CloseIdempotent`: `Close()` twice → both nil. Run `go test ./internal/grpcclient/ -run 'TestNewOTLPMetricsClient' -count=1`. **Expected: FAIL** — `OTLPMetricsClient` undefined (compile error). Record the verbatim red.

- [ ] **Step 2 — add the alias import + the stanza.** Add `colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"` to the import block (near `collogspb`/`coltracepb`). Below `NewOTLPTracesClient` (`:521` region), clone the `NewOTLPLogsClient` shape (`:474`), narrowing `Export`:

```go
// OTLPMetricsClient wraps the OTLP collector MetricsService/Export unary RPC over
// a Dialer-managed *grpc.ClientConn. Named OTLPMetricsClient (not MetricsServiceClient)
// to disambiguate from colmetricspb's own MetricsServiceClient AND the go-control-plane
// streaming metricsv3.MetricsServiceClient already wrapped in this package (phase 69).
type OTLPMetricsClient struct {
	connHolder
	stub colmetricspb.MetricsServiceClient
}

func NewOTLPMetricsClient(d *Dialer, clusterName string) (*OTLPMetricsClient, error) {
	conn, err := dialConn(d, "OTLP metrics client", clusterName)
	if err != nil {
		return nil, err
	}
	return &OTLPMetricsClient{
		connHolder: connHolder{conn: conn},
		stub:       colmetricspb.NewMetricsServiceClient(conn),
	}, nil
}

// Export sends one ExportMetricsServiceRequest and narrows the unary return to
// error alone (the *ExportMetricsServiceResponse is discarded) to satisfy the
// internal/statssink otlpMetricsClient seam.
func (c *OTLPMetricsClient) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	if c == nil || c.stub == nil {
		return errors.New("grpcclient: OTLP metrics client not initialized")
	}
	_, err := c.stub.Export(ctx, req)
	return err
}

func (c *OTLPMetricsClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}
```
(Re-derive the EXACT `NewOTLPLogsClient` idiom at the tip — the nil-guard string, the `connHolder.close()` vs `.Close()` method name, the `errors` import presence — and match it; the snippet above is the shape, not a byte-pin.)

- [ ] **Step 3 — append the `connHolder` roster comment** (`:153-157`, doc-only): add `OTLPMetricsClient` to the embedded-wrappers list.

- [ ] **Step 4 — run the tests.** `go test ./internal/grpcclient/ -count=1`. **Expected: PASS** (the two new tests green; every pre-existing grpcclient test green — the addition is purely additive).

- [ ] **Step 5 — break (AFTER committing; `reference_break_protocol_commit_first`).** **Break A [Export forwards]:** make `Export` `return nil` without calling `c.stub.Export` (`_ = req; return nil`). Re-run `TestNewOTLPMetricsClient_ExportForwards` → its "server received the sentinel" assertion FIRES (the server never saw the request). Confirm WHICH assertion. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/grpcclient` silent · `go vet ./internal/grpcclient/` · `golangci-lint run ./internal/grpcclient/`.

**Commit:** `grpcclient(phase 69 T1): OTLPMetricsClient — the unary NewOTLPLogsClient clone over colmetricspb (Export narrowed to error for the statssink seam; named OTLPMetricsClient to disambiguate from colmetricspb.MetricsServiceClient / metricsv3.MetricsServiceClient); connHolder roster comment appended`

---

## Task 2 — `internal/statssink/otlp.go`: `OTLPMetricsSink` + the seam + the mapping + the unary writer

**Files:**
- Create: `internal/statssink/otlp.go`
- Test: `internal/statssink/otlp_test.go`

**Interfaces:**
- Produces: `type OTLPMetricsSink struct{…}` (implements `Sink`, `sink.go:18-21`) + `type otlpMetricsClient interface{ Export(ctx, *colmetricspb.ExportMetricsServiceRequest) error; Close() error }` (package-local seam — the production sink stays grpcclient-free, RD-SEAM) + `func NewOTLPMetricsSink(client otlpMetricsClient, version string, reportCountersAsDeltas, useTagExtractedName, emitTagsAsAttributes bool, prefix string) *OTLPMetricsSink`.
- Consumes: `deltaState`/`(*deltaState).apply` (`delta.go`), `stats.ExtractTags` (`label.go:38` precedent), `dto.MetricFamily`; the otlp protos `colmetricspb`/`metricspb`/`commonpb`/`resourcepb`.
- Reuses UNTOUCHED: `Sink`, `metricsClient` seam, `snapshot`, `Flusher`, `labelMapper` (the OTLP sink calls `stats.ExtractTags` directly, RD-TAGS).

**Entry state:** T1 landed; `go test ./internal/statssink/ -count=1` green.

**Design (RE-DERIVED; §1.1 skeleton):** the writer mirrors `MetricsServiceSink` for lifecycle (`Submit` non-blocking send `sink.go:123-138`; `Close` `sync.Once`+grace `sink.go:140-156`) but the `run` body is UNARY (retry-once `client.Export`, not a stream), and the mapping is OTLP protos. `delta.apply` runs in `run` NOT `Submit` (RD-STREAMING — enqueue-drop never latches). The resource `telemetry.sdk.*` triple is built once in the constructor (RD-VERSION). `startNanos` is captured at construction (cumulative constant); `lastFlushNanos` chains the delta startTime.

- [ ] **Step 1 — write the failing mapping tests (red-first).** In `otlp_test.go`, drive the mapping through a FAKE `otlpMetricsClient` that captures the last `*ExportMetricsServiceRequest`, feeding hand-built `[]*dto.MetricFamily` (Counter + Gauge families with dotted names carrying tags, e.g. `cluster.svc.upstream_rq_total`). Independent `Errorf` assertions (`reference_fatalf_makes_assertions_unreachable`):
  1. **Structure:** ONE `ResourceMetrics`, ONE `ScopeMetrics`, empty `Scope`; one `Metric` per family.
  2. **Type/temporality:** Counter → `Sum{IsMonotonic:true, AggregationTemporality:CUMULATIVE}`; Gauge → `Gauge`. (Default sink: cumulative.)
  3. **TRUE-default tags:** with knobs absent-→-TRUE, the metric `name` is the tag-extracted residual (`cluster.upstream_rq_total`) AND a `KeyValue` attribute `envoy.<tag>` is present with the extracted value.
  4. **nil→TRUE inversion is at PARSE, not here** — but pin the sink's behavior on the resolved bools: `useTagExtractedName=false` → full dotted name; `emitTagsAsAttributes=false` → no attributes; both-false → full dotted names, no attrs (the `0113` shape).
  5. **prefix:** `prefix="p"` → name `p.<residual>` (dot composed); empty prefix → no leading dot.
  6. **delta:** a sink built `reportCountersAsDeltas=true`, fed the SAME cumulative counter across two `Submit`s, emits `Sum{AggregationTemporality:DELTA, IsMonotonic:true}` with the SECOND flush's counter value == the per-window delta (0 if unchanged); gauge unaffected.
  7. **startTime:** cumulative → `StartTimeUnixNano` constant across two flushes AND ns-magnitude (same digit band as `TimeUnixNano`); gauge datapoints carry NO startTime.
  8. **resource:** the three `telemetry.sdk.{name,language,version}` KEYS present with the constructor's values (`envoy-go`/`go`/the passed version).
  9. **no-mutate:** the input `[]*dto.MetricFamily` is byte-unchanged after `Submit` (the shared-snapshot constraint).

  Run `go test ./internal/statssink/ -run 'TestOTLP' -count=1`. **Expected: FAIL** — `NewOTLPMetricsSink` undefined (compile error). Record the verbatim red.

- [ ] **Step 2 — write `otlp.go`** per the §1.1 skeleton: the seam, the struct, `NewOTLPMetricsSink` (build `resourceAttrs`, capture `startNanos`, allocate `delta` if `reportCountersAsDeltas`, start `run`), `Submit` (clone `sink.go:123-138`), `run` (dequeue → `if s.delta != nil { batch = s.delta.apply(batch) }` → `req := s.toExportRequest(batch, flushNanos)` → `Export` with retry-once → fail-open LOG), `toExportRequest` (the mapping, §1.1), `Close` (clone `sink.go:140-156`). Helpers `kvFromTags(tags) []*commonpb.KeyValue` and the residual/prefix name build. **Never import grpcclient** (the seam).

- [ ] **Step 3 — run the tests.** `go test ./internal/statssink/ -count=1`. **Expected: PASS** (the mapping matrix green; every pre-existing statssink test green — `otlp.go` is a new file, the reused files untouched).

- [ ] **Step 4 — breaks (AFTER committing).**
  - **Break B [type mapping]:** map Counter → `Gauge` (drop the `Sum` arm). Re-run test 2 → the "Counter→Sum" assertion FIRES. `git restore`; re-green.
  - **Break C [temporality]:** hardcode `AggregationTemporality:CUMULATIVE` even when `s.delta != nil`. Re-run test 6 → the DELTA-temporality assertion FIRES. `git restore`; re-green.
  - **Break D [tag extraction]:** when `useTagExtractedName`, use `fam.GetName()` (full dotted) instead of the residual. Re-run test 3 → the residual-name assertion FIRES. `git restore`; re-green.
  - **Break E [attributes]:** when `emitAttrs`, emit NO attributes. Re-run test 3 → the `envoy.<tag>` attribute assertion FIRES. `git restore`; re-green.
  - **Break F [no-mutate]:** in `toExportRequest`, mutate `fam.Name` in place (`fam.Name = &residual`). Re-run test 9 → the input-unchanged assertion FIRES (proving the mapper builds its OWN protos). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 5 — writer robustness + `-race`.** A drop test (fill the channel, assert Submit does not block + a drop is logged not counted — grep no new stat) + a retry test (fake `Export` fails once then succeeds → the request is delivered) + a `Close` idempotence test. `go test ./internal/statssink/ -race -count=1` (the writer goroutine is a background mutator, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 5b — the +0-stats registration guard (SPEC §7).** In `internal/statssink/registration_test.go`, add `TestNoNewStat_OTLPRegistrationGuard` cloning the existing per-sink guards (`_RegistrationGuard`/`_StatsdRegistrationGuard`/`_DogStatsdRegistrationGuard`/`_GraphiteRegistrationGuard` — all present; re-derive the exact shape at the tip): build the OTLP sink over a fake `otlpMetricsClient`, run a flush, and assert the sink CREATES no new `stats.Registry` entries (the sink EMITS stats, it does not create them; drops are LOGGED not counted — P-14, +0 stats). This is the house precedent every prior sink row added AND the concrete gate T10's "+0 stats" verification cites. **Break [guard live]:** temporarily register a counter in the sink's construction path → the guard FIRES. `git restore`; re-green.

- [ ] **Step 6 — hygiene + cycle guard + commit.** `gofmt -l internal/statssink` silent · `go vet ./internal/statssink/` · `golangci-lint run ./internal/statssink/`. **Cycle guard:** `go list -deps ./internal/statssink | grep 'envoy-go/internal'` (**no `...`**) ⇒ NO `internal/grpcclient` edge (the `otlpMetricsClient` seam holds — RD-SEAM, `reference_xds_config_seam_transitive_cycle_guard`).

**Commit:** `statssink(phase 69 T2): OTLPMetricsSink — a fifth Sink over the landed flush subsystem; per-flush ONE unary Export (Counter→monotonic Sum / Gauge→Gauge, telemetry.sdk.* resource, empty scope), tag-extracted residual names + tags-as-KeyValue-attributes (nil→TRUE knobs), prefix, report_counters_as_deltas riding deltaState (DELTA + startTime chaining), correct-ns startTime (µs bug NOT cloned); the otlpMetricsClient seam keeps statssink grpcclient-free; the UNARY bounded-channel writer (retry-once, fail-open, +0 stats + TestNoNewStat_OTLPRegistrationGuard), NOT the streaming lifecycle`

---

## Task 3 — `internal/bootstrap`: the parse arm (`openTelemetrySinkTypeURL`, `parseOpenTelemetrySinkConfig`, the fifth dispatch arm, the rejects)

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Produces: `openTelemetrySinkTypeURL` (descriptor-derived) · `OTLPSinkConfig{ClusterName, ReportCountersAsDeltas, UseTagExtractedName, EmitTagsAsAttributes, Prefix}` · `Bootstrap.OTLPSinkConfigs []OTLPSinkConfig` · `parseOpenTelemetrySinkConfig`. Consumed by `main.go` (T4).
- Reuses: the `metricsServiceTypeURL` idiom (`:224`), the `parseMetricsServiceConfig` reject template (`:585-593`, `:595-597`), the `DiscardUnknown:false` posture (`:510`).

**Entry state:** T1–T2 landed; `go test ./internal/bootstrap/ -count=1` green.

- [ ] **Step 1 — write the failing tests (red-first).** In `bootstrap_test.go`, model on the metrics_service parse tests (`grep -n 'parseMetricsServiceConfig\|MetricsServiceConfig\|StatsSinkConfigs' internal/bootstrap/bootstrap_test.go`). Add, feeding a full bootstrap YAML with a `stats_sinks[]` `open_telemetry` entry (bare-scalar wrappers, `reference_protojson_wrapper_scalar_not_object`):
  1. `TestParseStatsSinks_OpenTelemetry_AllSixFields` — accept: `grpc_service.envoy_grpc.cluster_name` + `report_counters_as_deltas:true` + `report_histograms_as_deltas:false` + `emit_tags_as_attributes:false` + `use_tag_extracted_name:false` + `prefix:"p"` → `bs.OTLPSinkConfigs[0]` == `{ClusterName:"mc", ReportCountersAsDeltas:true, UseTagExtractedName:false, EmitTagsAsAttributes:false, Prefix:"p"}`.
  2. `TestParseStatsSinks_OpenTelemetry_WrapperDefaultsTrue` — knobs ABSENT → `UseTagExtractedName==true` AND `EmitTagsAsAttributes==true` (the nil→TRUE inversion — RD-WRAPPER; the discriminating property).
  3. `TestParseStatsSinks_OpenTelemetry_HistogramReject` — `report_histograms_as_deltas:true` → boot-FAIL, substring `report_histograms_as_deltas is not supported (envoy-go has no histograms)`.
  4. `TestParseStatsSinks_OpenTelemetry_VersionSkewReject` — a `SinkConfig` with `resource_detectors: [...]` (or `custom_metric_conversions`) → boot-FAIL with a protojson "unknown field" error (RD-SKEW — the FREE strict-layer reject; assert on the RAW parse failure, NOT a hand-written reject).
  5. `TestParseStatsSinks_OpenTelemetry_TransportRejects` — subtests: missing `grpc_service` (protocol_specifier required) / empty `cluster_name` / `google_grpc` / non-V3 `transport_api_version` each boot-FAIL with the mirrored substring.
  6. `TestParseStatsSinks_SiblingRejectRosterFive` — an UNHANDLED sink TypeURL (a real-but-unhandled one, `reference_sibling_reject_test_needs_real_typeurl` — e.g. `envoy.stat_sinks.wasm` if it resolves, else the existing sibling-reject test's message) → the default reject error now NAMES FIVE supported sinks incl. `open_telemetry` (the roster grew 4→5).

  Run `go test ./internal/bootstrap/ -run 'TestParseStatsSinks_OpenTelemetry|TestParseStatsSinks_SiblingRejectRosterFive' -count=1`. **Expected: FAIL** — the arm/types don't exist. Record the verbatim red.

- [ ] **Step 2 — add the descriptor TypeURL + the blank/aliased import.** Add `otelsinkv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/stat_sinks/open_telemetry/v3"` to the import block; clone `metricsServiceTypeURL` (`:224`):
```go
var openTelemetrySinkTypeURL = "type.googleapis.com/" + string((&otelsinkv3.SinkConfig{}).ProtoReflect().Descriptor().FullName())
```
(This blank-imports the descriptor — the prerequisite for both `@type` dispatch AND the free version-skew reject, RD-SKEW.)

- [ ] **Step 3 — add `OTLPSinkConfig` + the `Bootstrap` slice.** Sibling of `StatsSinkConfig` (`:357-372`): a struct with the five projected fields; `Bootstrap.OTLPSinkConfigs []OTLPSinkConfig` parallel to `:431/442/452/464`.

- [ ] **Step 4 — add `parseOpenTelemetrySinkConfig`.** Mirror `parseMetricsServiceConfig` (`:580-604`): `UnmarshalTo` the `*anypb.Any` → `*otelsinkv3.SinkConfig`; reject non-V3 transport_api_version, google_grpc / envoy_grpc-required, empty cluster_name (the `:585-593` template); the NEW `if cfg.GetReportHistogramsAsDeltas() { return … "report_histograms_as_deltas is not supported (envoy-go has no histograms)" }`; the nil→TRUE wrapper reads (`use := cfg.GetUseTagExtractedName(); … use == nil || use.GetValue()` — RD-WRAPPER); project into `OTLPSinkConfig`.

- [ ] **Step 5 — add the fifth `parseStatsSinks` arm + grow the reject roster.** In `parseStatsSinks` (`:532-569`), add `case openTelemetrySinkTypeURL:` (append the parsed `OTLPSinkConfig` to `bs.OTLPSinkConfigs`); in the default reject (`:564-565`) add a FIFTH `%q` for `open_telemetry` and its arg (`reference_strict_reject_sibling_typeurl_gap` — BOTH the case AND the error string extend).

- [ ] **Step 6 — run the tests.** `go test ./internal/bootstrap/ -count=1`. **Expected: PASS** (all six new tests green; every pre-existing bootstrap test green — the existing four arms untouched).

- [ ] **Step 7 — breaks (AFTER committing).**
  - **Break G [wrapper default inversion]:** read the wrappers with the metrics_service `.GetValue()` template (nil→FALSE) instead of `nil || .GetValue()`. Re-run test 2 → the `UseTagExtractedName==true` / `EmitTagsAsAttributes==true` assertions FIRE (nil now reads false). Confirm WHICH. `git restore`; re-green. (Pins the INVERSION — the load-bearing C2 correction.)
  - **Break H [histogram reject]:** delete the `report_histograms_as_deltas` reject. Re-run test 3 → its boot-FAIL substring assertion FIRES (the true knob now silently accepted). `git restore`; re-green.
  - **Break I [free version-skew reject]:** flip `DiscardUnknown` to `true` at `:510` (the ONLY such site). Re-run test 4 → its "unknown field" boot-FAIL assertion FIRES (the skew field now silently dropped — proving the reject is the strict-layer's, not hand-written; RD-SKEW). `git restore`; re-green. `[the flip is one bool — pre-compilable]`.
  - **Break J [roster grew 4→5]:** revert the fifth `%q`+arg in the default reject. Re-run test 6 → its "names open_telemetry" assertion FIRES. `git restore`; re-green.

- [ ] **Step 8 — retained-reject byte-diff + hygiene + commit.** Grep each pre-existing sibling reject byte-identical + count-unchanged (`reference_lifted_reject_hidden_enforcement`): the four existing sink arms + their default-reject `%q`s untouched apart from the fifth append; the metrics_service histogram + cluster rejects unchanged. `gofmt -l internal/bootstrap` silent · `go vet ./internal/bootstrap/` · `golangci-lint run ./internal/bootstrap/`.

**Commit:** `bootstrap(phase 69 T3): the open_telemetry stats-sink parse arm — openTelemetrySinkTypeURL (descriptor-derived, SinkConfig blank-imported), parseOpenTelemetrySinkConfig projecting the six fields into OTLPSinkConfig/Bootstrap.OTLPSinkConfigs, the fifth parseStatsSinks dispatch arm (reject roster 4→5), the nil→TRUE wrapper reads (INVERTING the metrics_service .GetValue() template), the report_histograms_as_deltas reject, and the free version-skew reject via DiscardUnknown:false`

---

## Task 4 — `cmd/envoy-go/main.go`: the gate clause + the OTLP build sub-loop

**Files:**
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `bs.OTLPSinkConfigs` (T3), `grpcclient.NewOTLPMetricsClient` (T1), `statssink.NewOTLPMetricsSink` (T2), `admin.BuildVersionString()` (already imported, `main.go:27`).
- Produces: nothing exported.

**Entry state:** T1–T3 landed; `go build ./... && go vet ./...` clean.

- [ ] **Step 1 — the gate + sub-loop.** In the any-sink gate (`:206`) add `|| len(bs.OTLPSinkConfigs) > 0`. Inside the block, clone the metrics_service sub-loop (`:207-215`):
```go
		if len(bs.OTLPSinkConfigs) > 0 {
			for _, cfg := range bs.OTLPSinkConfigs {
				client, err := grpcclient.NewOTLPMetricsClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("statssink: open_telemetry client for cluster %q: %v", cfg.ClusterName, err)
				}
				statsSinks = append(statsSinks, statssink.NewOTLPMetricsSink(client, admin.BuildVersionString(), cfg.ReportCountersAsDeltas, cfg.UseTagExtractedName, cfg.EmitTagsAsAttributes, cfg.Prefix))
			}
		}
```
`NewFlusher` (`:275`) + the LIFO drain (`:286-291`) UNTOUCHED. (main.go has no unit test; correctness is proven at T6/T7 fixtures. `go build` is the gate here.)

- [ ] **Step 2 — build + vet.** `go build ./... && go vet ./cmd/envoy-go/` exit 0. **No break at this task** (a build-loop wiring arm; the fixtures T6/T7 are its liveness — a mis-wire fails the fixture, and T6 Break "resource-key drop" / the type-swap catch a broken sink build). Record that the fixture arms are T4's liveness.

- [ ] **Step 3 — hygiene + commit.** `gofmt -l cmd/envoy-go` silent · `golangci-lint run ./cmd/envoy-go/`.

**Commit:** `main(phase 69 T4): wire the open_telemetry stats sink — the :206 gate clause + a build sub-loop (NewOTLPMetricsClient → NewOTLPMetricsSink, version via admin.BuildVersionString()); NewFlusher/LIFO-drain untouched`

---

## Task 5 — `test/helpers/otlpmetrics`: the driver-owned unary-Export receiver

**Files:**
- Create: `test/helpers/otlpmetrics/otlpmetrics.go`, `test/helpers/otlpmetrics/otlpmetrics_test.go`

**Interfaces:**
- Produces: `New(t testing.TB) *Server` / `NewAtAddr(addr) (*Server, error)`; the `colmetricspb.MetricsServiceServer` `Export` accumulating datapoints keyed `(metric-name, sorted-attribute-set)` ORDER-INSENSITIVE; accessors — `SumValue(name)`/`GaugeValue(name)`/`Temporality(name)`/`IsMonotonic(name)`/`ResourceAttributes()`/`StartTime(name)`/`DeltaSum(name)` (the running sum across flushes for the `0113` barrier); `Reset()`/`Addr()`/`Stop()`/`Close()` (hard-stop, `sync.Once`). Driver-owned, NOT a BackendKind (`reference_differential_grpc_receiver_driver_owned` — BackendKind tail stays 38).

**Entry state:** T1–T4 landed; `test/helpers/otlpmetrics` does not exist (verified).

**Design (clone `test/helpers/otlptrace/otlptrace.go`, RD-RECV):** RWMutex-guarded accumulation; `Export` flattens `ResourceMetrics`→`ScopeMetrics`→`Metric`, records each datapoint under a key of `(name, sorted(attr k=v pairs))` — sorting the attributes makes the key order-insensitive (the Walk-order-vs-reference-order non-contract, §3.9). Sum datapoints record value + temporality + isMonotonic + startTime; Gauge datapoints record value. `DeltaSum(name)` sums the per-flush delta values seen so far (the `0113` stability barrier reads this). Hard `Close()` (GracefulStop deadlocks on long-lived streams — but Export is UNARY here, so `Stop()`/`Close()` mirror otlptrace).

- [ ] **Step 1 — write the receiver + its self-test (red-first on the self-test).** In `otlpmetrics_test.go`, feed two hand-built `ExportMetricsServiceRequest`s (a Sum + a Gauge, one with attributes in one order, a second with the SAME attributes REVERSED) and assert: `SumValue`/`GaugeValue` read back; the two reversed-attr datapoints resolve to the SAME key (order-insensitive); `DeltaSum` accumulates. Run `go test ./test/helpers/otlpmetrics/ -count=1`. **Expected: FAIL** (undefined). Then write `otlpmetrics.go`; re-run → PASS.

- [ ] **Step 2 — break (AFTER committing).** **Break K [order-insensitive keying]:** key on the RAW (unsorted) attribute order instead of sorted. Re-run the self-test → the "reversed-attrs resolve to the same key" assertion FIRES (they now split into two keys). `git restore`; re-green. (Proves the keying is genuinely order-insensitive — the property the `0112`/`0113` NON-break control depends on.)

- [ ] **Step 3 — hygiene + commit.** `gofmt -l test/helpers/otlpmetrics` silent · `go vet ./test/helpers/otlpmetrics/` · `golangci-lint run ./test/helpers/otlpmetrics/`.

**Commit:** `helpers(phase 69 T5): test/helpers/otlpmetrics — a driver-owned unary-Export MetricsService receiver, datapoints keyed (name, sorted-attrs) order-insensitive, with Sum/Gauge/temporality/isMonotonic/resource-key/startTime + delta-SUM accessors (the otlptrace clone; NOT a BackendKind, tail stays 38)`

---

## Task 6 — fixture `0112-stats-sink-otlp` (the DEFAULT arm) + breaks (fixtures 113 → 114)

**Files:**
- Create: `test/fixtures/0112-stats-sink-otlp/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`

**Entry state:** T1–T5 landed; `test/fixtures/0112*` does not exist (verified). Port **10448**.

**Design (SPEC §8 — the DEFAULT arm: knobs absent ⇒ cumulative + tag-extracted residual names + tags-as-attributes):**

- `tcp_proxy` (or the sink-family fixture chassis — clone the closest landed stats-sink differential, e.g. the metrics_service `0089`; `grep -rln 'stat_sinks\|metrics_service' test/fixtures/*/driver/driver.go` to find it); `BackendCount() == 1` (`reference_differential_backendcount_min_one`).
- **Both YAMLs:** a `stats_sinks[]` `open_telemetry` entry with `grpc_service.envoy_grpc.cluster_name` pointing at a sink cluster whose endpoint is templated to the per-side `otlpmetrics` receiver `Addr()`. `stats_flush_interval` short (e.g. `0.1s`, clone the landed sink fixtures). The sink cluster pins **`dns_lookup_family: V4_ONLY`** (P-16, the Docker Desktop AAAA gotcha, `reference_host_gateway_ip_docker_desktop`). NO knob fields (defaults: cumulative + tag-extracted + attributes).
- **Per-side driver-owned `otlpmetrics.Server`s** (TWO — `reference_periodic_sink_differential_two_receivers`; a shared receiver contaminates the snapshot), hard `Close()` on teardown, the served-this-arm precondition (`feedback_probe_fresh_container_per_arm` — a driver-owned SERVER needs the same discipline: assert this side's receiver actually received an Export before comparing).
- **Workload:** K=7 deterministic requests/side (or a sink-fixture-appropriate K); poll each side's receiver until a deterministic COUNTER subset converges (`reference_health_check_propagation_warmup` poll-the-gauge discipline adapted). This fixture is HTTP-stat-driven — assert via the receiver + Drive hooks, NOT `HTTPExpectations` (`reference_differential_http_expectations_tcp_only` if HTTP/non-TCP).
- **ASSERT both sides** (NAMED SUBSETS only — `reference_stats_sink_emits_used_only`; the reference emits USED-only stats and the set GROWS with use, P-4): a deterministic counter subset (`cluster.<backend>.upstream_rq_total`, `http.<stat_prefix>.downstream_rq_total`, `http.<stat_prefix>.downstream_rq_2xx`) present as monotonic-cumulative `Sum` with `value == K`, the tag-extracted residual name, the expected `KeyValue` attribute set (keyed order-insensitive, §3.9); the three `telemetry.sdk.*` resource KEYS present (per-side values, §3.8); `startTimeUnixNano` ns-magnitude + constant across cumulative flushes (subject side). **UNasserted cross-side:** the whole family set/count (surfaces differ — no envoy-go histograms), reference startTime (its µs shape), the sink cluster's `upstream_cx_*` (dial-unaccounted + the P-15 feedback loop), message framing.
- `expectations.yaml` + `README.md`: clone the closest sink fixture's shape; state the proposition (*the open_telemetry sink exports the used counter subset as monotonic cumulative Sums with value==K, tag-extracted residual names + tags-as-attributes (the TRUE default), the telemetry.sdk.* resource triple; assertions are NAMED subsets, never cross-side startTime/framing/sink-cluster stats*).
- 00xx pre-existing expectations UNCHANGED.

- [ ] **Step 1 — write `driver/driver.go`** (clone the closest landed sink differential; `fixtureName = "0112-stats-sink-otlp"`, `refListenerPort = 10448`; two per-side `otlpmetrics.Server`s; the served-this-arm assert; the named-subset comparison).
- [ ] **Step 2 — write `envoy.yaml` / `envoy-go.yaml`** (the `open_telemetry` `stats_sinks[]` entry; `V4_ONLY` sink cluster; short flush interval; NO knobs).
- [ ] **Step 3 — write `expectations.yaml` / `README.md`.**
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0112-stats-sink-otlp' -count=1`. **Expected: PASS**; fixture count 114 (`ls -d test/fixtures/[0-9]*/ | wc -l`).
- [ ] **Step 5 — hygiene + commit.** Trio on the driver package; the runner_test.go blank-import wiring line if the chassis needs it (record the chartered per-fixture wiring).

- [ ] **Step 6 — breaks (AFTER committing; the fixture is not done until its assertions are PROVEN live).**
  - **Break L [Counter→Gauge type swap]:** in `otlp.go` (temporarily) map Counter → Gauge on BOTH sides. The receiver's "monotonic cumulative Sum" assertion FIRES (the subset now arrives as Gauge). Confirm WHICH. `git restore`; re-green. (Symmetric — `CompareBytes`/whole-request blind; the named-subset structural assertion is what bites, `reference_vacuous_break_receiver_normalizes`.)
  - **Break M [tag-extraction disable]:** force `useTagExtractedName=false` on both sides. The residual-name subset poll FIRES (the counters now arrive as full-dotted names — the residual subset never converges). `git restore`; re-green.
  - **Break N [resource-key drop]:** drop one `telemetry.sdk.*` key on both sides. The resource-key-present assertion FIRES. `git restore`; re-green.
  - **NON-break control [order-insensitive]:** reverse the RECEIVER's attribute iteration order (test-side only) → the fixture MUST still PASS (proving the `(name, sorted-attrs)` keying is order-insensitive; a break that does NOT fire, recorded as the finding — §3.9). `git restore`.

**Commit:** `differential(phase 69 T6): fixture 0112-stats-sink-otlp — the DEFAULT arm (cumulative monotonic Sum / Gauge, tag-extracted residual names + tags-as-attributes, Sum value==K, the telemetry.sdk.* resource triple per-side, correct-ns startTime); two per-side driver-owned otlpmetrics receivers, named-subset assertions, V4_ONLY sink cluster (fixtures 113→114, port 10448); breaks L/M/N + the order-insensitive non-break control`

---

## Task 7 — fixture `0113-stats-sink-otlp-knobs` (the KNOB arm — deltas + prefix + both-false) + breaks (fixtures 114 → 115)

**Files:**
- Create: `test/fixtures/0113-stats-sink-otlp-knobs/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`

**Entry state:** T6 landed. Port **10449**.

**Design (SPEC §8 — the KNOB arm: `report_counters_as_deltas:true` + `prefix:<p>` + `use_tag_extracted_name:false` + `emit_tags_as_attributes:false` in ONE coherent config):**

- Both YAMLs: the same chassis as `0112`, plus the four knob fields (BARE scalars — `use_tag_extracted_name: false`, `reference_protojson_wrapper_scalar_not_object`). Port **10449**; `V4_ONLY` sink cluster; two per-side `otlpmetrics.Server`s.
- **ASSERT both sides:** the per-name RUNNING DELTA-SUM `== K` across flushes with a **POST-CONVERGENCE STABILITY BARRIER** (`reference_delta_sink_differential_stability_barrier` — after the sum reaches K, ≥2 further idle flushes confirm the SUM no longer climbs; else the first flush's delta is indistinguishable from an absolute value) on the `<prefix>.<full-dotted>` counter subset; `Sum.aggregationTemporality == DELTA`; `isMonotonic` RETAINED; NO attributes (both-false); gauges absolute. The `telemetry.sdk.*` resource triple present (per-side). This ONE fixture proves all three knobs (deltas + prefix + both-false).
- The mixed combos (deltas+attributes-on compose; one-true-one-false naming) are `Submit`-level unit tests already in T2's matrix — NOT a fixture (the D-MSL-BOTH-KNOBS compose-smoke precedent).
- `expectations.yaml` + `README.md`: state the proposition (*with the three knobs the sink emits `<prefix>.<full-dotted>` DELTA Sums whose running sum stabilizes at K with isMonotonic retained and no attributes; the barrier proves DELTA, not absolute*).

- [ ] **Step 1–3 — write driver/YAMLs/expectations** (clone `0112`; add the knobs; the delta-SUM + stability-barrier comparison; the prefix subset).
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0113-stats-sink-otlp-knobs' -count=1`. **Expected: PASS**; fixture count 115.
- [ ] **Step 5 — hygiene + commit.**

- [ ] **Step 6 — breaks (AFTER committing).**
  - **Break O [absolute-not-delta]:** on both sides, emit the counter as an ABSOLUTE cumulative value (or `AggregationTemporality:CUMULATIVE`) while the fixture expects DELTA. The post-barrier SUM OVERSHOOTS K (each flush re-adds the cumulative value, the `0090` shape) → the delta-SUM==K assertion FIRES. Confirm WHICH. `git restore`; re-green.
  - **Break P [barrier masks]:** remove the stability barrier (assert only the first flush) → confirm the barrier is load-bearing: with the barrier removed, an absolute-value impl would pass the single-flush check. Demonstrate by pairing with Break O: barrier-removed + absolute → PASS (the masking); barrier-restored + absolute → FIRES. Record both. `[NOT pre-compiled — substitution rule applies]`.
  - **Break Q [prefix drop]:** drop the `prefix` composition on both sides → the `<prefix>.<full-dotted>` subset never converges → the subset assertion FIRES. `git restore`; re-green.

**Commit:** `differential(phase 69 T7): fixture 0113-stats-sink-otlp-knobs — the KNOB arm (report_counters_as_deltas + prefix + both bool knobs false): the per-name running DELTA-SUM==K with a post-convergence stability barrier on <prefix>.<full-dotted> counters, isMonotonic retained, no attributes; breaks O/P/Q (fixtures 114→115, port 10449)`

---

## Task 8 — fuzz: the `otlpType` seed + dispatch-verification (+0 fuzzers)

**Files:**
- Modify: `internal/bootstrap/statssink_fuzz_test.go`

**Entry state:** T1–T7 landed. Fuzzer `FuzzStatsSinkConfigParse` at `:19`; consts `msType`/`statsdType` at `:28-29` (RD-FUZZLOC).

- [ ] **Step 1 — add the const + seed.** Add `const otlpType = "type.googleapis.com/envoy.extensions.stat_sinks.open_telemetry.v3.SinkConfig"` beside `msType`/`statsdType`. Add a seed `f.Add([]byte(head + \`stats_sinks:\n  - name: envoy.stat_sinks.open_telemetry\n    typed_config:\n      "@type": \` + otlpType + \`\n      grpc_service:\n        envoy_grpc:\n          cluster_name: oc\n\`))` (match the exact `f.Add` idiom re-derived at the tip).

- [ ] **Step 2 — dispatch-verify (the named trap — RD-FUZZLOC / SPEC §7).** Temporarily add a `t.Logf` (or a scratch harness) that records which `parseStatsSinks` arm the seed reaches; run `go test ./internal/bootstrap/ -run FuzzStatsSinkConfigParse -count=1 -v`; CONFIRM the seed reaches `parseOpenTelemetrySinkConfig` (NOT the protojson `unable to resolve "…SinkConfig": not found` error — which would mean the `SinkConfig` descriptor was NOT blank-imported, T3 Step 2 — and NOT the sibling-default reject). As a one-off scratch check (NOT committed): temporarily remove the `otelsinkv3` import and confirm the seed then dies at `unable to resolve … not found` BEFORE dispatch (the vacuity trap, `reference_vacuous_break_receiver_normalizes` family; V1 executed this exact failure text). Restore. Record both in PROGRESS. **Re-derivation note (V2 M2):** the metrics_service seeds carry `transport_api_version: V3`; re-derive whether the `open_telemetry` `SinkConfig`'s `grpc_service` needs it in the seed — the dispatch-verify catches a non-reaching seed either way.

- [ ] **Step 3 — reconcile the count.** `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` → **55** BEFORE and AFTER (`reference_fuzzer_count_docs_drift` — a seed is +0 `func Fuzz`). A short active-fuzz smoke (`go test -run FuzzStatsSinkConfigParse -fuzz FuzzStatsSinkConfigParse -fuzztime 10s ./internal/bootstrap/`) — no panic; NO corpus artifacts committed.

- [ ] **Step 4 — hygiene + commit.** `gofmt -l internal/bootstrap` silent · `go vet ./internal/bootstrap/` · `golangci-lint run ./internal/bootstrap/`.

**Commit:** `bootstrap(phase 69 T8): fuzz — an otlpType open_telemetry seed into FuzzStatsSinkConfigParse, dispatch-verified to reach parseOpenTelemetrySinkConfig (the descriptor-blank-import trap confirmed: without the SinkConfig import the seed dies at "unable to resolve … not found" before dispatch); +0 fuzzers, 55→55`

---

## Task 9 — BEHAVIOR_CONTRACT delta (B1–B2) — pinned VERBATIM

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

**Entry state:** T1–T8 landed. Docs-only. Anchor by SYMBOL / first-clause, not line (SPEC §9 lines are as of `0ea2ec20`; re-locate each at the IMPL tip).

- [ ] **B1 — NEW section `### Stats sinks — the OpenTelemetry (OTLP) metrics sink (per phase 69 ADR-0291)`** (SPEC §9 B1), inserted AFTER the graphite section (`### Stats sinks — the graphite_statsd UDP sink …`, BC:817) and BEFORE `### Does not yet apply to (stats sinks)` (BC:835). Modeled on the graphite section; pin (verbatim from SPEC §9 B1): the FIFTH `stats_sinks[]` consumer + the SECOND `envoy.extensions.…` typed-extension stat-sink; the six-field surface (`grpc_service` required; the two doc-default-TRUE `*wrapperspb.BoolValue` knobs — bare scalar, nil→TRUE; the two scalar delta knobs; `prefix`); ONE unary `Export`/flush over the cluster-dialed `OTLPMetricsClient` (Counter→monotonic cumulative `Sum` / Gauge→`Gauge`; ONE resourceMetrics/scopeMetrics, empty scope); the TRUE default = tag-extracted residual names + tags-as-`KeyValue`-attributes; `report_counters_as_deltas`→DELTA + `isMonotonic` retained + startTime chaining (the ADR-0263 `deltaState`); `prefix` composes; the `telemetry.sdk.*` triple (per-side values); fail-open + retry-per-flush, +0 self-stats; the reference's factor-1000 µs `startTimeUnixNano` bug NOT cloned (per-side structural startTime, never cross-side); STRICT-REJECT `report_histograms_as_deltas:true` (ADR-0060) + the version-skew `resource_detectors`/`custom_metric_conversions` boot-rejected by the strict protojson layer; byte-stable when unconfigured; the `0112`/`0113` differentials; the two inherited departures (USED-only stats ⇒ named subsets; the reference's own µs startTime bug).

- [ ] **B2 — BC:836 `### Does not yet apply to (stats sinks)` first bullet — UPDATE the sibling-reject example.** The bullet currently reads "… + a sibling sink TypeURL **(OTLP-metrics)** …". Swap **(OTLP-metrics)** for a genuinely-unsupported sink TypeURL (the IMPL picks a real-but-unhandled one — `reference_sibling_reject_test_needs_real_typeurl`; verify it resolves, e.g. `envoy.stat_sinks.wasm` / `hystrix`) and add that `open_telemetry` is now CONSUMED (the six fields, with `report_histograms_as_deltas`/version-skew rejected). The "Histograms (no envoy-go analog, ADR-0060)" + "no drain-flush at shutdown" bullets STAY, EXTENDED to name the OTLP `report_histograms_as_deltas` reject.

- [ ] **B3 — NO edit** at the OTLP ACCESS-log neighborhood (BC:638 `## Access log — OpenTelemetry …` is a DIFFERENT extension `envoy.extensions.access_loggers.open_telemetry`; the SPEC notes the name-adjacency to forestall conflation, no BC edit owed — verify byte-unchanged).

- [ ] **Verify UNCHANGED:** the graphite section (BC:817) and every other stats-sink bullet apart from the B2 first bullet.

**Commit:** `docs(phase 69 T9): BEHAVIOR_CONTRACT B1–B2 — the NEW OTLP metrics stats-sink section (six fields, nil→TRUE knobs, unary Export, DELTA/prefix, telemetry.sdk.* triple, histogram + version-skew rejects, the 0112/0113 differentials); the BC:836 sibling-reject example swapped OFF OTLP-metrics (now CONSUMED) to a genuinely-unhandled sink`

---

## Task 10 — VERIFY: the six-gate + cycle guard + full differential + `-race` + counts + envelope audit

Controller-run on the frozen pre-stage-close HEAD:

- [ ] 1. `gofmt -l internal/ test/ cmd/` — SILENT
- [ ] 2. `go vet ./...` — exit 0
- [ ] 3. `go build ./...` — exit 0
- [ ] 4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (**+0 modules** — `reference_new_subpackage_pulls_transitive_module`; `go.opentelemetry.io/proto/otlp v1.0.0` already DIRECT, `collector/metrics/v1` same module as the consumed logs/trace collectors)
- [ ] 5. `golangci-lint run ./...` — exit 0
- [ ] 6. **FULL differential:** `go test ./test/differential/ -count=1` — all **115** dirs, exit 0. The 113 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run; `reference_0061_ring_hash_spread_flake` on a second occurrence → investigate margins.

**Plus:**
- [ ] **Cycle guard:** `go list -deps ./internal/statssink | grep 'envoy-go/internal'` (**no `...`**) ⇒ NO `internal/grpcclient` edge (the `otlpMetricsClient` seam — RD-SEAM; `reference_xds_config_seam_transitive_cycle_guard`, TYPE-level).
- [ ] **`-race` on touched packages:** `go test ./internal/statssink/ ./internal/grpcclient/ ./internal/bootstrap/ ./test/helpers/otlpmetrics/ -race -count=1` — the OTLP writer goroutine is a background mutator (`reference_full_suite_race_after_background_mutator`; the FULL statssink package -race catches it).
- [ ] **Counts MECHANICAL, never copied:** fixtures **115** (tail `0113-stats-sink-otlp-knobs`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0291** · stat surface **1201** (+0 — the `TestNoNewStat_OTLPRegistrationGuard` pins it; there is NO mechanical stat command) · go.mod diff EMPTY.
- [ ] **Envelope audit:** `git diff master --stat` shows functional production = `internal/statssink/otlp.go` (new) + `internal/bootstrap/bootstrap.go` + `internal/grpcclient/grpcclient.go` + `cmd/envoy-go/main.go` ONLY; **`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener`/`validate/` ABSENT**; the landed `internal/statssink/{sink,delta,label,mapping,flusher}.go` BYTE-UNTOUCHED; `test/helpers/otlpmetrics` = the one new driver-owned receiver. **New exported symbols ONLY** in `internal/statssink` (`OTLPMetricsSink`/`NewOTLPMetricsSink`) + `internal/grpcclient` (`OTLPMetricsClient`/`NewOTLPMetricsClient`); ZERO new packages/modules/stats/BackendKinds; the `internal/xds` zero-new-symbol discipline UNTOUCHED.

*(No separate commit — T10's evidence lands in PROGRESS at T11.)*

---

## Task 11 — ADR-0291 completed IN PLACE + stage-close (controller-adjacent)

- [ ] **ADR-0291: COMPLETE IN PLACE** — append §Decision + §Consequences to the EXISTING entry (the §Context landed at the SPEC squash, STATUS: IN PROGRESS). Flip the STATUS banner to **COMPLETE**. **Do NOT append a new ADR; do NOT renumber.** Tail stays ADR-0291; next-free ADR-0292 (`grep -c '^## ADR-0292'` → 0). §Decision records the landed mechanism (the `OTLPMetricsSink` + seam + unary writer, the parse arm + nil→TRUE wrappers + the two rejects + the free version-skew reject, the `OTLPMetricsClient` disambiguation, the `otlpmetrics` receiver, the two fixtures); §Consequences records the counts, the named departures (USED-only-stats subsets, the µs-startTime non-clone, the version-skew strict boundary, the histogram coverage boundary), and the memory updates.
- [ ] **ROADMAP row 69 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **NARROW the deferred sentence NOW (and ONLY now):** roll "OTLP-metrics stats sink + " OUT of the live Observability `candidates:` sentence (ROADMAP:198, the phase-57 graphite precedent — SPEC §12; the sentence STAYS a live `candidates:` sentence afterward, the `ssl`/tracing-custom_tags candidates remain).
- [ ] **STATE.md:** edit §Current pointer IN PLACE; demote to §Recent lineage capped at five; update counts (fixtures 115).
- [ ] **PROGRESS.md:** finalize — every break's ACTUAL firing assertion (incl. the order-insensitive non-break control and the T8 dispatch-verify traps), the verbatim red-first records, any break substitutions.
- [ ] **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate by SUBJECT). Row 69 done ⇒ the sentinel's check (1) goes SILENT for row 69 ⇒ the roller SELF-PICKS the next subject at the phase-70 BRAINSTORM (the 2026-07-12 standing directive) unless the sentinel fires (it does not: checks (2)+(3) still print).
- [ ] **Sentinel re-run MECHANICALLY:** check (1) goes silent when row 69 flips (every OTHER chartered row already `done`); (2) still prints 3 via the full-phrase command (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` → 3 — the OTLP narrowing does NOT drop the whole sentence); (3) unchanged (`NEVER OPENED: gRPC/Runtime/WASM`) ⇒ does NOT fire; no `stop` file.
- [ ] **Memory updates owed (SPEC §13):** (i) the version-skew posture — a pinned dependency LACKING a newer proto field means a config setting it is boot-REJECTED FOR FREE by the `DiscardUnknown:false` protojson layer (NOT silently ignored, as an `UnmarshalTo`-only reading suggests — the whole-document protojson pass rejects the unknown field before the per-message `UnmarshalTo`); a named envoy-go-strict version-skew departure, no new code. (ii) the OTLP two-`*wrapperspb.BoolValue` knobs default TRUE ⇒ nil reads `== nil || .GetValue()`, the INVERSE of the scalar-`.GetValue()` idiom — check the proto TYPE per field, do not template blind (`reference_protojson_wrapper_scalar_not_object` extended). (iii) OPTIONAL: a "sink family fifth-consumer" note if any framework friction surfaced (else skip — the row is expected to be pure parameter-threading).
- [ ] **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 69 (stats-sink-otlp) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-69

| SPEC obligation | Where |
|---|---|
| `OTLPMetricsSink` = a fifth `Sink`, unary Export, Counter→cumulative monotonic Sum / Gauge→Gauge, empty scope (§3.1) | T2 |
| the `otlpMetricsClient` seam keeps statssink grpcclient-free (§3.1, RD-SEAM) | T2 Step 6 (cycle guard), T10 |
| the six `SinkConfig` fields; `grpc_service` required; the mirrored rejects (§3.2) | T3 Steps 4–5 |
| the version-skew fields boot-rejected FOR FREE by `DiscardUnknown:false` (§3.3, RD-SKEW) | T3 test 4, Break I, T10 |
| STRICT-REJECT `report_histograms_as_deltas:true` (§3.4) | T3 test 3, Break H |
| the two BoolValue knobs default TRUE, nil→TRUE (INVERTED template) (§3.5, RD-WRAPPER) | T3 test 2, Break G |
| `OTLPMetricsClient` disambiguation (§3.6, RD-COLLIDE) | T1 |
| the UNARY bounded-channel writer, fail-open + retry-per-flush, +0 stats (§3.7, RD-STREAMING) | T2 Steps 2/5 |
| the `telemetry.sdk.*` triple, per-side value pins (§3.8, RD-VERSION) | T2 tests 8, T4, T6 |
| correct-ns startTime (µs bug NOT cloned); DELTA temporality + isMonotonic + chaining; order-insensitive dup-name (§3.9, RD-DELTA) | T2 tests 6/7, T5, T7 |
| the RBAC RIDER stays deferred (§3.10) | (not attached — §13) |
| +0 packages / +0 modules / +0 stats / +0 fuzzers / +0 BackendKinds (§4, §7) | T2 Step 5b (`TestNoNewStat_OTLPRegistrationGuard`), T8, T10 |
| TWO fixtures `0112`/`0113` with the delta-SUM stability barrier + two per-side receivers + named subsets + V4_ONLY (§8) | T6, T7 |
| the fuzz seed + dispatch-verification trap (§7) | T8 |
| BC B1–B2 pinned wording, incl. the BC:836 sibling-example swap (§9) | T9 |
| a SINGLE FLAT ROW; ADR-0045 valve armable-but-unconsumed (§10) | §1.1, this table |
| six-gate + cycle guard + full-115-dir + -race + counts + envelope audit (§10 T10, §15) | T10 |
| ADR-0291 completed IN PLACE, no new ADR (§14) | T11 |
| Sentinel: narrow the sentence AT THE IMPL row-done, not before (§12) | T11 |
| Memory updates (§13) | T11 |

**Task count: 11** — matching the SPEC's ~11 anticipation. **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: no two-package surface can strand a leg (`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener` untouched). Sequencing: T1/T2/T3 (independent producers) → T4 (wires them) → T5 (independent receiver) → T6→T7 (fixtures) → T8 (fuzz) → T9 (BC) → T10/T11 (close).

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident (all re-derived read-only at the PLAN, §1): the ZERO-drift anchor set (RD-EXACT), the streaming-vs-unary mapping site (RD-STREAMING, `sink.go:194-202`), the nil→TRUE wrapper inversion (RD-WRAPPER, the pinned descriptor doc), and the free version-skew reject (RD-SKEW, `bootstrap.go:510` the ONLY `DiscardUnknown`).
