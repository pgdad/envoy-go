# Phase 47.2b IMPL — Progress: `emit_tags_as_labels=true` (the SN-rule tag→`LabelPair` extraction)

**SPEC:** `docs/envoy-go/phases/47-stats-sink-metrics-service/SPEC-47.2b.md`
**PLAN:** `docs/envoy-go/phases/47-stats-sink-metrics-service/PLAN-47.2b.md`
**Worktree branch:** `phase-47.2b-impl` (off master, isolated worktree)

This leg is the **SECOND-and-FINAL sub-leg** of the FOURTH Observability-family row. It lifts the
`internal/bootstrap/bootstrap.go:454` `emit_tags_as_labels:true` strict-reject (reference-parity-accept),
adds a `StatsSinkConfig.EmitTagsAsLabels bool` (scalar — NOT a `*BoolValue`), refactors `internal/stats/name.go`
`flattenToProm` into a shared exported `ExtractTags` SN-rule core (D-MS-LABEL-REUSE option (a) — `flattenToProm`
becomes a thin `"envoy_" + ReplaceAll(residual,".","_")` Prometheus projection, byte-identical), adds a
sink-owned `labelMapper` (`internal/statssink/label.go`) applied in `MetricsServiceSink.Submit` PARALLEL to the
47.2a `deltaState` (compose order delta-THEN-labels; dotted residual name + dotted `envoy.` keys + sorted
`LabelPair`s; BOTH Counter and Gauge; SN4 multi-tag; no shared-slice mutation), threads the 4th bool through
`cmd/envoy-go/main.go`, extends the driver-owned `test/helpers/metricsservice` receiver with a label-aware
`FamilyWithLabels` surface, and proves it with the `0091-stats-sink-metrics-service-labels` cross-side EXACT
`{residual-name, sorted labels}` cumulative `value==K` differential. It **ANCHORS ADR-0264**
(§Decision/§Consequences body lands atomically at the completion task per ADR-0044). **ROADMAP row 47
(`stats-sink-metrics-service`) FLIPS `done` at THIS IMPL** — 47.2b is the FINAL sub-leg (ADR-0106 +
`reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

---

## D-MS-SPLIT-2 — FINAL ADR-0045 re-check: NO sub-split (§3.0 / Task 1 Step 2)

The realized 47.2b scope is ~150 LoC of new logic: bootstrap ~5 lines (lift one reject arm + one bool field +
set-on-append + three doc-comment edits); the `ExtractTags` refactor is a mechanical PURE-EXTRACTION of an
existing function (no new behavior — gated byte-identical by `name_test.go`'s 53 funcs + `prom_test.go` + the
full prom differential); the new `label.go` ~55 lines (`labelMapper` + `apply`); `sink.go` ~6 lines (a
`labels *labelMapper` field + a 4th bool ctor param + the `Submit` branch after `delta`); `main.go` ~1 line
(the thread); the receiver ~25 lines (a `byKey map[string]familyValue` + `FamilyWithLabels` + `labelKey` +
the `StreamMetrics`/`Reset` updates); the `0091` fixture a mechanical clone of the authoritative
`0090/driver/driver.go` with the delta-SUM stability barrier DROPPED (cumulative `value==K` model) and the
keyed `{residual-name, sorted labels}` lookups. This is a single focused change — a 47.2b-i/47.2b-ii sub-split
would be over-decomposition (SPEC §3.0). **D-MS-SPLIT-2 RESOLVED: NO further split.** **47.2b is the FINAL
sub-leg — its IMPL flips ROW 47 `done`** (ADR-0106 + `reference_roadmap_split_phase_row_done`).

---

## Baselines (verbatim outputs — recorded 2026-06-29, this worktree HEAD `a4c6d2ef`)

```
$ ls -d test/fixtures/*/ | wc -l                                   # expect 92 (tail 0090-stats-sink-metrics-service-deltas)
92

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 50
50

$ grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1    # tail ADR-0263 (next-free ADR-0264)
## ADR-0263

$ go build ./... && go test ./internal/stats/ ./internal/statssink/ ./internal/bootstrap/ ./test/helpers/metricsservice/ -count=1
ok  	github.com/esalaine/envoy-go/internal/stats
ok  	github.com/esalaine/envoy-go/internal/statssink
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/test/helpers/metricsservice
```

**Baseline summary:** stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **92** /
fuzzers **50** / BackendKind **38** / DECISIONS tail **ADR-0263** (next-free **ADR-0264**) /
`client_model v0.6.1` already direct / build OK / the four touched packages green.

**Anticipated at IMPL exit:** stat **1200 UNCHANGED** (+0) / fixtures **93** (`0091-stats-sink-metrics-service-labels`) /
fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0264** / NO new package / NO new go.mod module / `go mod tidy -diff` EMPTY.

---

## Task ledger

- [x] **Task 1** — Baselines + PROGRESS scaffold + FINAL ADR-0045 re-check (D-MS-SPLIT-2: no sub-split). (controller)
- [x] **Task 2** — Lift the `emit_tags_as_labels:true` reject + add `StatsSinkConfig.EmitTagsAsLabels` (scalar bool).
- [x] **Task 3** — The shared `ExtractTags` SN-rule core refactor (`internal/stats/name.go`; byte-identical — `name_test.go` pure-addition, prom green).
- [x] **Task 4** — The sink-owned `labelMapper` tag→`LabelPair` transform (`internal/statssink/label.go`).
- [x] **Task 5** — Thread the 4th bool into `MetricsServiceSink` + main + both-knobs compose smoke (delta-then-labels: msg0=7, msg1=3 with labels).
- [x] **Task 6** — The receiver label-aware `FamilyWithLabels` surface (`test/helpers/metricsservice`; additive, 0089/0090 non-regress).
- [x] **Task 7** — The `0091-stats-sink-metrics-service-labels` cross-side EXACT cumulative-value fixture (registered in runner_test.go; 2xx two-label SN4 split matched first try).
- [x] **Task 8** — `0091` deliberate breaks (a/b/c behavioral, d on both projections) + 20/20 flake-soak + full-package `-race` clean on internal/statssink AND internal/stats (controller).
- [x] **Task 9** — Full 93-dir differential (`ok 283s`) + six-gate GREEN + ADR-0264 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (ROW 47 FLIPS `done`).
