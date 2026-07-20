# PROGRESS 69 — the OpenTelemetry OTLP metrics stats sink (`envoy.stat_sinks.open_telemetry`)

> Live task ledger for the phase-69 IMPL. The PLAN (`PLAN.md`) is the spine; this file records what ACTUALLY happened per task — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions, and the six-gate evidence. Populated at the IMPL, not the PLAN.

## Stage pointer

- **PLAN done** (2026-07-20) — the 11-task TDD spine (T1–T11) landed; the SPEC §11 anchors RE-DERIVED at `0ea2ec20` (ZERO unacknowledged drift — every anchor exact, unlike phase 68's RD-LINES) via a read-only re-derivation agent that also extracted the exact clone skeletons (the `NewOTLPLogsClient` stanza, the `parseMetricsServiceConfig` template, the streaming-vs-unary mapping site `sink.go:198-202`, the six-field `SinkConfig` descriptor, the `otlptrace` receiver). **Adversarial verification (PLAN §1.2):** V1 (code-claims, RD-SKEW probe EXECUTED in a clone — the `DiscardUnknown:false` free-reject HELD + discriminates) → ZERO SEVERE/MODERATE, four MINOR folded; V2 (process/coverage) → ONE SEVERE folded — the SPEC §7 `TestNoNewStat_OTLPRegistrationGuard` (+0-stats one-per-sink house guard) was cited by T10 but written by no task; CORRECTED by adding T2 Step 5b + the `registration_test.go` File-structure entry. Design direction unchanged. Fresh worktree off master `0ea2ec20`, branch `phase-69-stats-sink-otlp-plan`.

## Task ledger (filled at the IMPL)

| Task | Status | Commit | Red-first / breaks / notes |
|---|---|---|---|
| T1 grpcclient OTLPMetricsClient | pending | | |
| T2 statssink OTLPMetricsSink + writer | pending | | |
| T3 bootstrap parse arm | pending | | |
| T4 main.go wiring | pending | | |
| T5 otlpmetrics receiver | pending | | |
| T6 fixture 0112 (default) | pending | | |
| T7 fixture 0113 (knobs) | pending | | |
| T8 fuzz seed + dispatch-verify | pending | | |
| T9 BEHAVIOR_CONTRACT B1–B2 | pending | | |
| T10 verify (six-gate + envelope) | pending | | |
| T11 ADR-0291 + close | pending | | |

## Findings carried from the PLAN (RE-DERIVED at `0ea2ec20`; RE-VERIFY at the IMPL tip)

- **RD-EXACT:** every SPEC §11 anchor exact at `0ea2ec20` — no line-boundary correction to carry (the SPEC's cites are adopted verbatim).
- **RD-STREAMING:** the delta+label mapping site is `run` NOT `Submit` (`sink.go:194-202`) — the OTLP writer applies `delta.apply` in the writer goroutine so an enqueue-drop never latches `deltaState`. The writer clones the UNARY otlplogs/otlptrace shape (retry-once `Export`), NOT the streaming `MetricsServiceSink` lifecycle.
- **RD-SEAM:** the package-local `otlpMetricsClient` interface seam keeps `internal/statssink` grpcclient-free (the `metricsClient` `sink.go:28-31` precedent); the cycle guard stays clean (TYPE-level).
- **RD-WRAPPER:** `emit_tags_as_attributes` / `use_tag_extracted_name` are `*wrapperspb.BoolValue`, struct-doc "Default value is true." — read `w == nil || w.GetValue()` (nil→TRUE), INVERTING the metrics_service scalar `.GetValue()` (nil→FALSE) at `bootstrap.go:600`.
- **RD-SKEW:** `DiscardUnknown:false` at `bootstrap.go:510` is the ONLY such site; `resource_detectors`/`custom_metric_conversions` are ABSENT from the pinned v1.32.4 `SinkConfig` ⇒ a config setting them boot-rejects FOR FREE once the descriptor is blank-imported (ZERO new code).
- **RD-COLLIDE:** `colmetricspb.MetricsServiceClient` (otlp collector) collides on the simple name with `metricsv3.MetricsServiceClient` (go-control-plane streaming, already wrapped) ⇒ the wrapper is `OTLPMetricsClient`.
- **RD-DELTA:** `deltaState.apply(abs) []*dto.MetricFamily` (`delta.go:39`) returns a NEW batch, never mutates input — reusable verbatim; DELTA retains `isMonotonic`, chains startTime; cumulative startTime is a fixed process-start ns constant; gauges carry no startTime.
- **RD-TAGS:** the two knobs are INDEPENDENT ⇒ `otlp.go` calls `stats.ExtractTags` DIRECTLY (same-package, as `labelMapper` does at `label.go:38`) and decides name (residual vs full-dotted) and attributes (`envoy.<tag>` KeyValue vs none) SEPARATELY; attributes gate on `emitAttrs` alone.
- **RD-VERSION:** `telemetry.sdk.name="envoy-go"`, `language="go"`, `version=admin.BuildVersionString()` — threaded into `NewOTLPMetricsSink` from `main.go` (already imports `internal/admin`, builds `minNode`), keeping `statssink` import-clean.
- **RD-FUZZLOC:** the fuzzer is `internal/bootstrap/statssink_fuzz_test.go:19`; add `const otlpType` + a seed; dispatch-verify it reaches `parseOpenTelemetrySinkConfig` (the descriptor-blank-import trap).
- **RD-RECV:** `test/helpers/otlpmetrics` clones `test/helpers/otlptrace` (unary-Export, RWMutex, hard `Close`), keyed `(name, sorted-attrs)` order-insensitive — driver-owned, NOT a BackendKind.
