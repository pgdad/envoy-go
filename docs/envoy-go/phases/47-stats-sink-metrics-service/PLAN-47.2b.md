# Phase 47.2b Implementation Plan — `emit_tags_as_labels=true`: the SN-rule tag→`LabelPair` extraction on the landed 47.1/47.2a `metrics_service` sink

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a bootstrap `stats_sinks[]` metrics_service entry sets `emit_tags_as_labels: true`, envoy-go emits, per metric family (BOTH Counter and Gauge), the family's tag SEGMENTS as `MetricFamily.metric[].label[]` `LabelPair`s — the tag VALUE stripped from the family `name` (the residual dotted name kept, dots collapsed), emitted as a label keyed by the Envoy DOTTED tag-name (`envoy.cluster_name`, `envoy.http_conn_manager_prefix`, `envoy.response_code_class`, ...). The metric `type` and `value` are unchanged (cumulative-absolute, orthogonal to `report_counters_as_deltas`). Absent/false is byte-identical to the 47.1/47.2a no-labels path.

**Architecture:** The `internal/stats/name.go` SN1–SN9 tag-extraction logic is refactored into a shared, exported `ExtractTags(internal) (residualDotted, []Label, err)` core (resolving **D-MS-LABEL-REUSE** in favor of the shared core — option (a)). `flattenToProm` becomes a thin Prometheus projection over it (`"envoy_" + ReplaceAll(residual, ".", "_")`); a new sink-owned `labelMapper` in `internal/statssink/label.go` projects the SAME core output to the metrics_service dotted form (residual name + `"envoy." + TrimPrefix(key, "envoy_")` keys + sorted `LabelPair`s). The mapper is applied in `MetricsServiceSink.Submit` PARALLEL to the 47.2a `deltaState` (compose order: delta-then-labels), building the sink's OWN families — NEVER mutating the shared `flusher.go:47` snapshot slice. The `bootstrap.go:454` `emit_tags_as_labels:true` strict-reject is LIFTED (reference-parity-accept); an `EmitTagsAsLabels bool` is added to `StatsSinkConfig` (scalar `GetEmitTagsAsLabels()`, NOT a `*BoolValue`) and threaded through `cmd/envoy-go/main.go`. The `0091-stats-sink-metrics-service-labels` cross-side EXACT differential asserts the deterministic counter subset's `{residual-name, sorted LabelPair[]}` split + cumulative `value == K` via a new additive `FamilyWithLabels` surface on the driver-owned `test/helpers/metricsservice` receiver.

**Tech Stack:** Go; `github.com/prometheus/client_model v0.6.1` (`dto`, `dto.LabelPair`); `google.golang.org/protobuf/proto`; `go-control-plane/envoy v1.32.4` (`MetricsServiceConfig`); the differential runner (`test/differential`); Docker reference `envoyproxy/envoy:contrib-v1.37.2`.

**Charter:** `docs/envoy-go/phases/47-stats-sink-metrics-service/SPEC-47.2b.md` (read §3 the transform design, §8 the `0091` differential + the receiver surface, §10 the task sketch, §11 the executed live-probe pins, §12 the D-questions, §13 the ADR-0264 §Context draft).

**Execution mode (per the router + memories):** DOCS-ONLY direct-on-master is the PLAN's own landing; the IMPL THIS PLAN DESCRIBES is subagent-driven (`feedback_execution_style` · `superpowers:subagent-driven-development`) in a FRESH worktree off master (`feedback_git_worktrees`). Subagents commit LOCALLY only (`feedback_subagents_no_push`); the CONTROLLER verifies each commit, cleans leak files, re-runs the six-gate + the FULL-package `internal/statssink` AND `internal/stats` `-race`, does the `0091` deliberate breaks ITSELF, squashes, and pushes at stage-close (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`). Per-task: `gofmt -l` + `golangci-lint` on touched packages, not just `go vet` (`feedback_pertask_gofmt_lint`). Worktree hygiene: `git restore` to undo deliberate breaks (NO checkout-sha/amend — `feedback_subagent_worktree_detach`); pin the canonical worktree-relative paths + controller-verify the main checkout stays clean (`feedback_subagent_worktree_path_targeting`).

**Anchors:** ADR-0264 (§Decision/§Consequences body lands at this IMPL per ADR-0044; §Context drafted at SPEC §13) · ADR-0080 (strict-reject — LIFT only `:454`, keep the rest) · ADR-0061 (the SN1–SN9 name/label rules — the tag-extraction substrate the label mapper REUSES via the shared `ExtractTags` core) · ADR-0045 (sub-split — 47.2b is the FINAL no-split sub-leg; D-MS-SPLIT-2 re-check Task 1) · ADR-0106 + `reference_roadmap_split_phase_row_done` (row 47 FLIPS `done` at THIS — the FINAL sub-leg's — IMPL; the Observability family STAYS OPEN) · ADR-0059 (frozen Registry) · ADR-0060 (no histograms — moot) · ADR-0262 (the 47.1 substrate) · ADR-0263 (the 47.2a deltas substrate the labels transform composes ON).

**Anticipated counts at IMPL exit:** stat **1200** (non-H2 **1196**, +0 — the label transform is a sink-owned mapping, D-MS-STATS-FINAL-2) / fixtures **92 → 93** (`0091-stats-sink-metrics-service-labels`) / fuzzers **50** (`FuzzStatsSinkConfigParse` already covers the now-accepted field) / BackendKind **38** (driver-owned receiver) / DECISIONS **ADR-0263 → ADR-0264**. NO new package, NO new go.mod module, NO new BackendKind (`client_model v0.6.1` already direct — `LabelPair` resolves; `go mod tidy -diff` stays EMPTY). The shared `ExtractTags` refactor stays inside `internal/stats` (no new package).

---

## File map

**Create:**
- `internal/statssink/label.go` — the sink-owned `labelMapper` (`apply(in) []*dto.MetricFamily`) projecting `stats.ExtractTags` to the metrics_service dotted form.
- `internal/statssink/label_test.go` — table-driven transform unit tests against the §11 pinned forms (residual name + dotted `envoy.` keys + SN4 multi-tag + both types + no-input-mutation).
- `test/fixtures/0091-stats-sink-metrics-service-labels/driver/driver.go` — cloned from the AUTHORITATIVE 0090 driver; keyed `{residual-name, sorted labels}` cumulative `value == K` poll-to-converge (NO delta-SUM stability barrier).
- `test/fixtures/0091-stats-sink-metrics-service-labels/envoy.yaml` — reference bootstrap (clone of 0090 + `emit_tags_as_labels: true` REPLACING `report_counters_as_deltas: true`).
- `test/fixtures/0091-stats-sink-metrics-service-labels/envoy-go.yaml` — subject bootstrap (clone + `emit_tags_as_labels: true`).
- `test/fixtures/0091-stats-sink-metrics-service-labels/expectations.yaml` — the labels-split + cumulative-value prose.
- `test/fixtures/0091-stats-sink-metrics-service-labels/README.md` — two-receiver + cumulative-value + labels-split README.

**Modify:**
- `internal/stats/name.go:39`–`:352` (extract the shared `ExtractTags` core; `flattenToProm` becomes the thin Prometheus projection).
- `internal/stats/name_test.go` (NO expectation changes — it is the byte-identity guard; OPTIONALLY add a focused `TestExtractTags` for the dotted residual + the `envoy_` keys).
- `internal/bootstrap/bootstrap.go:273` (add `EmitTagsAsLabels bool`), `:454`–`:456` (lift the reject), `:460`–`:463` (set the field on append), `:269`–`:272` + `:331`–`:334` + `:434`–`:436` (doc comments drop `emit_tags_as_labels` from the strict-reject lists).
- `internal/bootstrap/bootstrap_test.go:1809`–`:1821` (drop the `emit_tags_as_labels` reject case from `TestStatsSink_Rejects`; KEEP `histogram_emit_mode` + all others), `:1912`–`:1946` (add a sibling `TestStatsSink_AcceptEmitTagsAsLabels` parse-accept test).
- `internal/statssink/sink.go:64`–`:101`, `:109`–`:112` (add `labels *labelMapper` field; 4th bool param on `NewMetricsServiceSink` + `newSinkWithCapacity`; apply in `Submit` after `delta`).
- `internal/statssink/sink_test.go:156,198,236,252,272` (add the 4th `false`/`true` arg; add a labels-mode + a both-knobs compose Submit test), `internal/statssink/registration_test.go:33` (4th `false` arg).
- `cmd/envoy-go/main.go:199` (pass `cfg.EmitTagsAsLabels` as the 4th arg).
- `test/differential/runner_test.go:117`–`:118` (add the `0091` driver blank-import after the `0090` one — WITHOUT this the fixture is unregistered and every `0091` run is a vacuous green).
- `test/helpers/metricsservice/metricsservice.go:60`–`:74`, `:115`–`:127`, `:145`–`:175`, `:228`–`:243` (add a `byKey map[string]familyValue` accumulator + `FamilyWithLabels(name, labels)` accessor + the `labelKey` helper; update `StreamMetrics` + `Reset` + `newServer`).
- `test/helpers/metricsservice/metricsservice_test.go` (if present — add a `FamilyWithLabels` test; else add a focused unit test to the package).
- `docs/envoy-go/DECISIONS.md` (ADR-0264 §Decision/§Consequences body), `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the labels contract delta), `docs/envoy-go/STATE.md` + `docs/envoy-go/ROADMAP.md` (row 47 FLIPS `done`).

---

## Design decisions resolved in this PLAN (the §12 D-questions)

- **D-MS-SPLIT-2 (the FINAL ADR-0045 re-check):** NO further sub-split. Realized scope is ~150 LoC of new logic (bootstrap ~5 lines; the `ExtractTags` refactor is a mechanical pure-extraction of an existing function; `label.go` ~55; `sink.go` ~6; `main.go` ~1; receiver ~25; the `0091` fixture a mechanical 0090 clone). A 47.2b-i/47.2b-ii split would be over-decomposition. Recorded in Task 1.
- **D-MS-LABEL-REUSE → SHARED CORE (option (a)).** Refactor `name.go` to expose `ExtractTags(internal) (residualDotted string, labels []Label, err error)` — the single SN1–SN9 + SN4 matcher — consumed by BOTH `flattenToProm` (the Prometheus projection) AND the statssink `labelMapper` (the dotted projection). Chosen over the parallel-extractor (option (b)) because: (1) `internal/stats/name_test.go` has **53 test functions** covering every SN-rule arm + `prom_test.go` + the full prom differential — a strong byte-identity guard that makes the refactor safe; (2) `flattenToProm` has exactly ONE production caller (`prom.go:38`) — a clean refactor surface; (3) a parallel extractor would DUPLICATE 13 SN-rule arms (cluster/http/listener/server/wasm/lrl/bandwidth/rbac/zookeeper/mongo/kafka/redis/thrift), a severe future-drift hazard (every new filter's tag extractor would need adding in two places — the codebase's many `KEEP IN SYNC` comments show this pain is real). The metrics_service labels then match `flattenToProm`'s extraction EXACTLY by construction. **HARD GATE:** the refactor MUST keep `flattenToProm`'s output byte-identical — `go test ./internal/stats/ -count=1` green with `name_test.go` expectations UNCHANGED + the full prom differential green. If any arm's residual projection is not byte-identical, that arm's base-building is preserved per-arm (the fallback).
- **D-MSL-PLACEMENT → sink-owned `labelMapper`** applied in `Submit` PARALLEL to the 47.2a `deltaState.apply` (`sink.go:110`–`:111`): a `labels *labelMapper` field on `MetricsServiceSink` (non-nil ⇒ `emit_tags_as_labels`). **HARD CONSTRAINT:** NO in-place mutation of the shared `flusher.go:47` snapshot slice — `apply` builds the sink's OWN families (new `Name`, new `Metric`/`Label`; the `Counter`/`Gauge` value pointer shared read-only). **Compose order:** `delta` (rewrites the Counter VALUE) THEN `labels` (rewrites the NAME + LABEL) — orthogonal (disjoint fields), pinned delta-then-labels for determinism.
- **D-MSL-RECEIVER-KEY → a composite `name|sorted-labels` key.** Add a `byKey map[string]familyValue` accumulator (last-seen value+type) + a `FamilyWithLabels(name string, labels []*dto.LabelPair)` accessor that computes the same key. ADDITIVE — `Family`/`FamilySum`/`Messages`/`Count` STAY (0089/0090 non-regress; those families have empty labels so the name-only surface is unaffected).
- **D-MSL-SUBSET-2XX → INCLUDE the `downstream_rq_2xx` two-label split** (`name="http.downstream_rq_xx"` + `{envoy.http_conn_manager_prefix, envoy.response_code_class="2"}`) — envoy-go's SN4 (`name.go:346`) reproduces it; it proves the multi-tag cross-side correctness. FALLBACK to the two single-label splits (cluster_name, http_conn_manager_prefix) only if a residual mismatch surfaces at IMPL.
- **D-MSL-BOTH-KNOBS → accepted-and-composed, unit/smoke-tested, NOT a separate fixture.** A both-knobs-true Submit test confirms delta-then-labels composition (delta on the Counter VALUE, labels on the NAME+LABEL — orthogonal). `0091` exercises the labels knob ALONE (cumulative values).

---

## Task 1: Baselines, PROGRESS scaffold, FINAL ADR-0045 re-check (D-MS-SPLIT-2)

**Files:**
- Create: `docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.2b.md` (controller-local; match the 47.2a IMPL precedent — do NOT commit if the phase convention keeps PROGRESS out of git).

- [ ] **Step 1: Capture baselines** (the worktree is fresh off master at the post-SPEC tip — HEAD is the phase-47.2b SPEC docs-only commit `470966e3`)

Run and record:
```bash
ls -d test/fixtures/*/ | wc -l                          # expect 92
grep -rh '^func Fuzz' --include='*.go' . | wc -l        # expect 50
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1   # tail ADR-0263 (next-free 0264)
go build ./... && go test ./internal/stats/ ./internal/statssink/ ./internal/bootstrap/ ./test/helpers/metricsservice/ -count=1
```
Expected: build clean; the four packages green; counts as noted.

- [ ] **Step 2: Record the D-MS-SPLIT-2 disposition in PROGRESS**

The realized scope is a single focused change (one `StatsSinkConfig` bool + one reject lift + a pure-extraction `ExtractTags` refactor + a sink-owned label transform + the main thread + one fixture + a label-aware receiver surface) — NO 47.2b-i/47.2b-ii sub-split (ADR-0045 final gate; SPEC §3.0). Note it; this resolves D-MS-SPLIT-2. Note also: **47.2b is the FINAL sub-leg — its IMPL flips ROW 47 `done`** (ADR-0106 + `reference_roadmap_split_phase_row_done`).

- [ ] **Step 3: Commit the PROGRESS scaffold** (if the phase commits PROGRESS; otherwise skip)

```bash
git add docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.2b.md
git commit -m "phase 47.2b Task 1: baselines + PROGRESS + D-MS-SPLIT-2 final no-split re-check"
```

---

## Task 2: Config — lift the `emit_tags_as_labels:true` reject + add `EmitTagsAsLabels bool`

**Files:**
- Modify: `internal/bootstrap/bootstrap.go:273` (field), `:454`–`:456` (lift), `:460`–`:463` (append), `:269`–`:272` + `:331`–`:334` + `:434`–`:436` (doc comments).
- Test: `internal/bootstrap/bootstrap_test.go:1809`–`:1821` (drop the reject case), `:1912`–`:1946` (add the accept test).

- [ ] **Step 1: Write the failing test** — add `TestStatsSink_AcceptEmitTagsAsLabels` (a sibling of `TestStatsSink_AcceptReportCountersDeltas` at `:1912`) asserting BOTH the `false` and `true` config parse-accept AND that `true` sets the field:

```go
// TestStatsSink_AcceptEmitTagsAsLabels: emit_tags_as_labels false OR true both
// parse-accept; true records EmitTagsAsLabels on the config (the strict-reject was
// lifted at 47.2b — reference-parity-accept, ADR-0264). emit_tags_as_labels is a
// scalar bool (NOT a *BoolValue — contrast report_counters_as_deltas).
func TestStatsSink_AcceptEmitTagsAsLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  string
		want bool
	}{
		{"false", "false", false},
		{"true", "true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + metricsServiceType + `
      emit_tags_as_labels: ` + tc.val + `
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`)
			bs, err := Load(strings.NewReader(src))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := len(bs.StatsSinkConfigs); got != 1 {
				t.Fatalf("StatsSinkConfigs: got %d, want 1", got)
			}
			if got := bs.StatsSinkConfigs[0].EmitTagsAsLabels; got != tc.want {
				t.Errorf("EmitTagsAsLabels = %v, want %v", got, tc.want)
			}
		})
	}
}
```
Also DELETE the `emit_tags_as_labels` case (`:1809`–`:1821`) from `TestStatsSink_Rejects`. KEEP the `histogram_emit_mode`, `transport_api_version_V2`, `google_grpc`, `empty_cluster_name`, `sibling_statsd_sink`, and `stats_flush_on_admin` cases unchanged.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bootstrap/ -run 'TestStatsSink_AcceptEmitTagsAsLabels' -count=1 -v`
Expected: FAIL — `EmitTagsAsLabels` is not a field of `StatsSinkConfig` (compile error), or (if you add the field first) the `true` subtest fails because the reject still fires.

- [ ] **Step 3: Add the field**

`internal/bootstrap/bootstrap.go:273` (extend the existing struct, mirroring the `ReportCountersAsDeltas` doc shape at `:275`–`:279`):
```go
type StatsSinkConfig struct {
	ClusterName string // MetricsServiceConfig.grpc_service.envoy_grpc.cluster_name
	// ReportCountersAsDeltas is MetricsServiceConfig.report_counters_as_deltas
	// (ADR-0263): true ⇒ each COUNTER family carries the per-flush delta
	// (current − last_flushed) instead of the cumulative absolute value; GAUGES
	// stay absolute. Absent/false ⇒ the 47.1 cumulative path.
	ReportCountersAsDeltas bool
	// EmitTagsAsLabels is MetricsServiceConfig.emit_tags_as_labels (ADR-0264):
	// true ⇒ each metric family's tag SEGMENTS are stripped from the dotted name
	// into metric[].label[] LabelPairs (keyed by the Envoy dotted tag-name
	// envoy.<tag>); BOTH Counter and Gauge. The metric value is unchanged
	// (cumulative-absolute, orthogonal to ReportCountersAsDeltas). Absent/false ⇒
	// the 47.1 full-dotted-name/zero-labels path. emit_tags_as_labels is a scalar
	// bool (NOT a *wrapperspb.BoolValue — contrast ReportCountersAsDeltas).
	EmitTagsAsLabels bool
}
```

- [ ] **Step 4: Lift the reject + set the field**

`internal/bootstrap/bootstrap.go` — DELETE the `:454`–`:456` arm:
```go
	if msc.GetEmitTagsAsLabels() {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service emit_tags_as_labels:true is not supported (47.1 emits the full dotted name with zero labels; deferred to 47.2)", idx)
	}
```
and set the field on the append (`:460`–`:463`):
```go
	result.StatsSinkConfigs = append(result.StatsSinkConfigs, StatsSinkConfig{
		ClusterName:            eg.GetClusterName(),
		ReportCountersAsDeltas: msc.GetReportCountersAsDeltas().GetValue(),
		EmitTagsAsLabels:       msc.GetEmitTagsAsLabels(),
	})
```
Note the asymmetry: `GetReportCountersAsDeltas()` returns `*wrapperspb.BoolValue` (read via `.GetValue()`); `GetEmitTagsAsLabels()` returns a plain `bool` (read directly — NO `.GetValue()`). Update THREE doc comments to drop `emit_tags_as_labels` from the strict-reject lists (it is now consumed; `histogram_emit_mode` + the rest STAY rejected): the `StatsSinkConfig` TYPE-level comment (`:269`–`:272`, the "the remaining strict-reject arms (emit_tags_as_labels / non-default histogram_emit_mode / ...)" line at `:270`), the `StatsSinkConfigs` field doc (`:331`–`:334`, the "emit_tags_as_labels:true, a non-default histogram_emit_mode, ..." line at `:332`), and the `parseMetricsServiceConfig` doc comment (`:434`–`:436`, the "emit_tags_as_labels:true (47.1 emits the full dotted name ...)" line at `:434`). Both knobs are now CONSUMED; note `emit_tags_as_labels` is CONSUMED into `StatsSinkConfig.EmitTagsAsLabels` (ADR-0264).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/bootstrap/ -run 'TestStatsSink' -count=1 -v`
Expected: PASS — `TestStatsSink_AcceptEmitTagsAsLabels` both subtests green; `TestStatsSink_Rejects` green (the `histogram_emit_mode` case + the rest still reject; the `emit_tags_as_labels` case is gone). Then `gofmt -l internal/bootstrap/ && go vet ./internal/bootstrap/ && golangci-lint run ./internal/bootstrap/...` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 47.2b Task 2: lift the emit_tags_as_labels:true reject + add StatsSinkConfig.EmitTagsAsLabels (scalar bool)"
```

---

## Task 3: The shared `ExtractTags` core refactor (`internal/stats/name.go`) — D-MS-LABEL-REUSE option (a)

**Files:**
- Modify: `internal/stats/name.go:39`–`:352` (extract the core; `flattenToProm` becomes the projection).
- Test: `internal/stats/name_test.go` (NO expectation changes — the byte-identity guard; OPTIONALLY add a `TestExtractTags`).

**The pure-extraction refactor.** `flattenToProm` today (`:39`) builds the Prometheus base directly per arm. Refactor so a new exported `ExtractTags` does the SN-rule matching and produces the DOTTED residual + the `envoy_`-keyed labels, and `flattenToProm` is the thin projection. The residual-to-base projection is the uniform `"envoy_" + strings.ReplaceAll(residual, ".", "_")` — verified byte-identical for every arm (a valid Prometheus name has no dots; the SN1/SN3/SN5 arms that concatenate `rest` without dot-replacement only do so because their real rests are dot-free, so the uniform `ReplaceAll` is harmless there; `name_test.go`'s 53 tests + `prom_test.go` + the full prom differential are the hard byte-identity gate).

- [ ] **Step 1: Confirm the byte-identity guard is GREEN before touching anything**

Run: `go test ./internal/stats/ -count=1`
Expected: PASS (this is the pre-refactor baseline; `name_test.go`'s 53 functions + `prom_test.go` cover every SN-rule arm).

- [ ] **Step 2: (Optional) Write a focused `TestExtractTags`** — pin the DOTTED residual + the `envoy_` keys for the SN1/SN2/SN3/SN4 arms the `0091` subset exercises (the metrics_service projection is downstream of these):

```go
func TestExtractTags(t *testing.T) {
	cases := []struct {
		in           string
		wantResidual string
		wantLabels   []Label
	}{
		{"cluster.c_backend.upstream_rq_total", "cluster.upstream_rq_total",
			[]Label{{Key: "envoy_cluster_name", Value: "c_backend"}}},
		{"http.hcm_local.downstream_rq_total", "http.downstream_rq_total",
			[]Label{{Key: "envoy_http_conn_manager_prefix", Value: "hcm_local"}}},
		{"http.hcm_local.downstream_rq_2xx", "http.downstream_rq_xx",
			// SN4 prepends response_code_class; the statssink mapper sorts by key.
			[]Label{{Key: "envoy_response_code_class", Value: "2"}, {Key: "envoy_http_conn_manager_prefix", Value: "hcm_local"}}},
		{"server.live", "server.live", nil}, // SN5: residual == name, no tags
	}
	for _, c := range cases {
		residual, labels, err := ExtractTags(c.in)
		if err != nil {
			t.Fatalf("ExtractTags(%q): %v", c.in, err)
		}
		if residual != c.wantResidual {
			t.Errorf("ExtractTags(%q) residual = %q, want %q", c.in, residual, c.wantResidual)
		}
		// compare labels as a set (SN4 prepend order is the raw order; the mapper sorts)
		if !sameLabelSet(labels, c.wantLabels) {
			t.Errorf("ExtractTags(%q) labels = %+v, want %+v", c.in, labels, c.wantLabels)
		}
	}
	// no-match keeps an error (untagged-name passthrough is the mapper's job)
	if _, _, err := ExtractTags("listener_manager.listener_create_success"); err == nil {
		t.Error("ExtractTags(listener_manager.*): want error (no top-level rule), got nil")
	}
}
```
(Add a small `sameLabelSet` helper or reuse an existing one in the test file.)

- [ ] **Step 3: Refactor `name.go`** — rename the existing `flattenToProm` body to `ExtractTags`, converting each `base = "envoy_..."` assignment to the dotted `residual` form and each early `return base, labels, nil` to `return residual, labels, nil`, with the SN4 collapse operating on the residual; then add the thin `flattenToProm` projection. Mechanically, per arm:
  - **SN1 cluster** (`:50`–`:52`): `residual = "cluster." + rest` (rest = `tail[dot+1:]`, unchanged); label key stays `envoy_cluster_name`.
  - **SN2 http** (`:60`,`:73`–`:74`): `residual = "http." + rest` where `rest` keeps the existing internal-dot→underscore on the `tail[dot+1:]` segment (SN2's `ReplaceAll` stays — it is part of the residual for http). Label key `envoy_http_conn_manager_prefix`.
  - **SN3 listener** (`:82`–`:84`): `residual = "listener." + rest`.
  - **SN5 server** (`:87`–`:88`): `residual = "server." + rest` (rest = `TrimPrefix(internal,"server.")`).
  - **wasm** (`:118`–`:123`): `residual = "wasm." + tail`; `return residual, nil, nil`.
  - **SN9 lrl** (`:150`–`:156`): `residual = "http_local_rate_limit." + counter`; label key `envoy_local_http_ratelimit_prefix`; `return residual, labels, nil`.
  - **bandwidth** (`:182`–`:196`): `residual = internal` (whole; inline-prefix, no label); `return residual, nil, nil`.
  - **rbac** (`:227`–`:242`): `residual = rest` (`rest = internal[idx+1:]`, i.e. `rbac.<...>`); label key `envoy_rbac_prefix`; `return residual, labels, nil`.
  - **zookeeper** (`:256`–`:262`): `residual = internal` (whole); `return residual, nil, nil`.
  - **mongo** (`:278`–`:288`): `residual = "mongo." + tail` (after `hoistMongoDynamicSegments`); labels sorted; `return residual, labels, nil`.
  - **kafka** (`:298`–`:301`): `residual = "kafka." + rest`; `return residual, nil, nil`.
  - **redis** (`:315`–`:323`): `residual = "redis." + tail`; label key `envoy_redis_prefix`; `return residual, labels, nil`.
  - **thrift** (`:332`–`:341`): `residual = "thrift." + tail`; label key `envoy_thrift_prefix`; `return residual, labels, nil`.
  - **default no-match** (`:342`): `return "", nil, fmt.Errorf(...)` unchanged.
  - **SN4** (`:346`–`:349`): operate on `residual` — `if m := statusClassRE.FindStringSubmatch(residual); m != nil { residual = m[1] + "_xx"; labels = append([]Label{{Key: "envoy_response_code_class", Value: m[2]}}, labels...) }`. Then `return residual, labels, nil`.

  Then:
```go
// ExtractTags splits an internal hierarchical-dotted name into the residual
// dotted name (tag-value segments removed, dots preserved) + the extracted tag
// set (keys in the envoy_ Prometheus underscore form, values the dynamic tokens),
// applying the SN1–SN9 prefix rules + the SN4 status-class _Nxx→_xx collapse
// (ADR-0061). It is the SHARED matcher consumed by BOTH flattenToProm (the
// Prometheus projection) AND the internal/statssink labelMapper (the
// metrics_service dotted projection, ADR-0264). Returns "", nil, error on a name
// matching no top-level/filter rule. (Phase 47.2b extracted this from
// flattenToProm; the projection below preserves flattenToProm's output verbatim —
// name_test.go is the byte-identity guard.)
func ExtractTags(internal string) (residual string, labels []Label, err error) {
	// ... the former flattenToProm body, producing the dotted residual ...
}

// flattenToProm transforms an internal hierarchical-dotted name to the Prometheus
// exposition form (envoy_-prefixed, dots→underscores) per Rules SN1–SN9. It is a
// thin projection over ExtractTags: the residual's dots become underscores and an
// envoy_ prefix is prepended. Returns "", nil, error on a name matching no rule.
func flattenToProm(internal string) (string, []Label, error) {
	residual, labels, err := ExtractTags(internal)
	if err != nil {
		return "", nil, err
	}
	return "envoy_" + strings.ReplaceAll(residual, ".", "_"), labels, nil
}
```
Keep the existing doc block (`:24`–`:38`) attached to whichever function it best describes; move the SN-rule summary onto `ExtractTags` and leave a short note on `flattenToProm`.

- [ ] **Step 4: Run the byte-identity gate** (the load-bearing guard)

Run: `go test ./internal/stats/ -count=1 -v`
Expected: PASS — every `name_test.go` function GREEN with NO expectation edits (proves `flattenToProm` output is byte-identical), `prom_test.go` GREEN, the new `TestExtractTags` (if added) GREEN, the `internal/stats` fuzzers' seed corpus still passes. If ANY `name_test.go` test fails, an arm's residual projection is not byte-identical — fix that arm's residual (the fallback: preserve that arm's exact base-building) until green; do NOT edit the test expectation.

- [ ] **Step 5: Lint + commit**

Run: `gofmt -l internal/stats/ && go vet ./internal/stats/ && golangci-lint run ./internal/stats/...` clean.
```bash
git add internal/stats/name.go internal/stats/name_test.go
git commit -m "phase 47.2b Task 3: extract the shared ExtractTags SN-rule core (flattenToProm now a thin Prometheus projection; byte-identical, name_test.go unchanged)"
```

---

## Task 4: The sink-owned label transform (`internal/statssink/label.go`)

**Files:**
- Create: `internal/statssink/label.go`, `internal/statssink/label_test.go`.

- [ ] **Step 1: Write the failing table-driven test** (`label_test.go`)

Cover, against the §11 pinned forms: a single-tag Counter (`cluster.c_backend.upstream_rq_total` → `name="cluster.upstream_rq_total"` + `{envoy.cluster_name=c_backend}`); a single-tag Gauge (`cluster.c0.membership_total` → `name="cluster.membership_total"` + `{envoy.cluster_name=c0}` — BOTH types, AMEND-MSL-LABELS-ON-BOTH-TYPES); the SN4 MULTI-tag (`http.hcm_local.downstream_rq_2xx` → `name="http.downstream_rq_xx"` + sorted `[{envoy.http_conn_manager_prefix=hcm_local},{envoy.response_code_class=2}]`); an untagged name passthrough (`listener_manager.listener_create_success` → full name, empty labels, shared by pointer); the value is UNCHANGED; the input batch is NOT mutated.

```go
package statssink

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

// (counterFam/gaugeFam already exist in delta_test.go — same package; reuse them.)

func labelOf(m *dto.Metric) []*dto.LabelPair { return m.GetLabel() }

func TestLabelMapper_Apply(t *testing.T) {
	lm := newLabelMapper()

	out := lm.apply([]*dto.MetricFamily{
		counterFam("cluster.c_backend.upstream_rq_total", 7),
		gaugeFam("cluster.c0.membership_total", 1),
		counterFam("http.hcm_local.downstream_rq_2xx", 7),
		counterFam("listener_manager.listener_create_success", 1),
	})

	// 1: single-tag Counter — residual name + one label; value unchanged.
	if got := out[0].GetName(); got != "cluster.upstream_rq_total" {
		t.Errorf("counter residual name = %q, want cluster.upstream_rq_total", got)
	}
	if got := out[0].GetMetric()[0].GetCounter().GetValue(); got != 7 {
		t.Errorf("counter value = %v, want 7 (unchanged)", got)
	}
	assertLabels(t, out[0].GetMetric()[0].GetLabel(), [][2]string{{"envoy.cluster_name", "c_backend"}})

	// 2: single-tag Gauge — labels on BOTH types (AMEND-MSL-LABELS-ON-BOTH-TYPES).
	if got := out[1].GetName(); got != "cluster.membership_total" {
		t.Errorf("gauge residual name = %q, want cluster.membership_total", got)
	}
	assertLabels(t, out[1].GetMetric()[0].GetLabel(), [][2]string{{"envoy.cluster_name", "c0"}})

	// 3: SN4 multi-tag — _2xx→_xx + two SORTED labels.
	if got := out[2].GetName(); got != "http.downstream_rq_xx" {
		t.Errorf("2xx residual name = %q, want http.downstream_rq_xx", got)
	}
	assertLabels(t, out[2].GetMetric()[0].GetLabel(), [][2]string{
		{"envoy.http_conn_manager_prefix", "hcm_local"},
		{"envoy.response_code_class", "2"},
	})

	// 4: untagged → full name + empty labels (shared by pointer).
	if got := out[3].GetName(); got != "listener_manager.listener_create_success" {
		t.Errorf("untagged name = %q, want full name unchanged", got)
	}
	if len(out[3].GetMetric()[0].GetLabel()) != 0 {
		t.Errorf("untagged labels = %v, want empty", out[3].GetMetric()[0].GetLabel())
	}
}

func TestLabelMapper_Apply_DoesNotMutateInput(t *testing.T) {
	lm := newLabelMapper()
	in := []*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)}
	_ = lm.apply(in)
	if got := in[0].GetName(); got != "cluster.c_backend.upstream_rq_total" {
		t.Fatalf("input family name mutated: %q", got)
	}
	if n := len(in[0].GetMetric()[0].GetLabel()); n != 0 {
		t.Fatalf("input metric labels mutated: %d labels", n)
	}
}

// assertLabels checks the LabelPair slice equals want (in order — the mapper sorts by key).
func assertLabels(t *testing.T, got []*dto.LabelPair, want [][2]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].GetName() != w[0] || got[i].GetValue() != w[1] {
			t.Errorf("label[%d] = {%q,%q}, want {%q,%q}", i, got[i].GetName(), got[i].GetValue(), w[0], w[1])
		}
	}
}

var _ = proto.String // keep the import if otherwise unused after edits
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/statssink/ -run 'TestLabelMapper' -count=1 -v`
Expected: FAIL — `newLabelMapper`/`apply` undefined.

- [ ] **Step 3: Implement `label.go`**

```go
package statssink

import (
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/esalaine/envoy-go/internal/stats"
)

// labelMapper rewrites each family's full dotted Name into the metrics_service
// tag-split form (emit_tags_as_labels=true, ADR-0264): the residual dotted name
// (tag-value segments removed, dots collapsed) + the extracted tags as
// metric[].label[] LabelPairs keyed by the Envoy DOTTED tag-name (envoy.<tag>,
// derived from stats.ExtractTags's envoy_ key via "envoy."+TrimPrefix). It reuses
// stats.ExtractTags — the SAME SN1–SN9 + SN4 matcher flattenToProm uses — so the
// metrics_service labels match the Prometheus extraction exactly. Labels apply to
// BOTH Counter and Gauge families (the knob is name-model, not value-model —
// CONTRAST the 47.2a Counter-only deltas). Stateless; one per emit_tags sink.
type labelMapper struct{}

func newLabelMapper() *labelMapper { return &labelMapper{} }

// apply returns a NEW batch with each family's Name rewritten to the dotted
// residual and metric[].label[] set to the SORTED LabelPairs. A name matching no
// SN-rule (ExtractTags error) — or one with no extracted tags — is shared by
// pointer unchanged (its full dotted name + empty labels; AMEND-MSL untagged
// passthrough). Like deltaState.apply it MUST NOT mutate the shared snapshot slice
// the Flusher fans to every sink (flusher.go:47): tagged families are rebuilt (new
// MetricFamily/Metric, sorted new LabelPairs), the Counter/Gauge value pointer
// shared read-only. The metric value is UNCHANGED (cumulative-absolute, unless a
// preceding deltaState.apply already rewrote the Counter VALUE — orthogonal: this
// touches only Name + Label).
func (lm *labelMapper) apply(in []*dto.MetricFamily) []*dto.MetricFamily {
	out := make([]*dto.MetricFamily, 0, len(in))
	for _, fam := range in {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil || len(labels) == 0 {
			out = append(out, fam) // untagged: full name + empty labels, shared by pointer
			continue
		}
		pairs := make([]*dto.LabelPair, 0, len(labels))
		for _, l := range labels {
			pairs = append(pairs, &dto.LabelPair{
				Name:  proto.String("envoy." + strings.TrimPrefix(l.Key, "envoy_")),
				Value: proto.String(l.Value),
			})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].GetName() < pairs[j].GetName() })
		ms := make([]*dto.Metric, 0, len(fam.GetMetric()))
		for _, m := range fam.GetMetric() {
			ms = append(ms, &dto.Metric{
				Label:       pairs,
				Counter:     m.Counter, // shared read-only (nil for gauges)
				Gauge:       m.Gauge,   // shared read-only (nil for counters)
				TimestampMs: m.TimestampMs,
			})
		}
		out = append(out, &dto.MetricFamily{Name: proto.String(residual), Type: fam.Type, Metric: ms})
	}
	return out
}
```
Note: the `len(labels) == 0` branch covers BOTH the no-match (`err != nil`) and the matched-but-no-tag cases (SN5 `server.*`, wasm/kafka/zookeeper/bandwidth inline arms — their residual equals the original name, so passthrough is identical). The acyclic-import check holds (`internal/statssink` already imports `internal/stats`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/statssink/ -run 'TestLabelMapper' -count=1 -v`
Expected: PASS (both). Then `gofmt -l internal/statssink/ && go vet ./internal/statssink/ && golangci-lint run ./internal/statssink/...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/statssink/label.go internal/statssink/label_test.go
git commit -m "phase 47.2b Task 4: sink-owned labelMapper tag->LabelPair transform (dotted residual + dotted envoy. keys; both types; SN4 multi-tag; no input mutation)"
```

---

## Task 5: Wire the transform into the sink + main + the both-knobs compose smoke

**Files:**
- Modify: `internal/statssink/sink.go:64`–`:101`, `:109`–`:112`.
- Modify: `cmd/envoy-go/main.go:199`.
- Test: `internal/statssink/sink_test.go:156,198,236,252,272` + a labels-mode and a both-knobs Submit test; `internal/statssink/registration_test.go:33`.

- [ ] **Step 1: Write the failing tests** — add a labels-mode Submit test AND a both-knobs (delta+labels) compose test to `sink_test.go`, reusing the existing `fakeMetricsClient`/`fakeMetricsStream` + `counterFam` (from `delta_test.go`) + `testNode()`. Sketch (adapt to the file's existing fake names + the `messages()` poll idiom at `:155`,`:197`,`:272`):

```go
func TestSink_LabelsMode_RewritesNameAndLabels(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), false /*deltas*/, true /*emitTagsAsLabels*/, 8)
	defer s.Close()

	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)})
	// poll stream.messages() until 1 message captured, then assert:
	//   msg.EnvoyMetrics[0].name == "cluster.upstream_rq_total"
	//   msg.EnvoyMetrics[0].metric[0].label == [{envoy.cluster_name, c_backend}]
	//   msg.EnvoyMetrics[0].metric[0].counter.value == 7 (cumulative — unchanged)
}

func TestSink_BothKnobs_DeltaThenLabels(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), true /*deltas*/, true /*labels*/, 8)
	defer s.Close()

	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)})
	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 10)})
	// poll for 2 messages; assert:
	//   both carry name=="cluster.upstream_rq_total" + label {envoy.cluster_name,c_backend}
	//   msg0 counter.value == 7 (delta first=absolute), msg1 counter.value == 3 (delta) — labels on the DELTA value
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/statssink/ -run 'TestSink_LabelsMode|TestSink_BothKnobs' -count=1 -v`
Expected: FAIL — `newSinkWithCapacity` has 4 args, not 5 (compile error).

- [ ] **Step 3: Add the field + param + Submit branch**

`sink.go` — add the field (`:64`–`:78`, next to `delta`):
```go
type MetricsServiceSink struct {
	ch          chan []*dto.MetricFamily
	client      metricsClient
	node        *corev3.Node
	delta       *deltaState  // non-nil ⇒ report_counters_as_deltas (ADR-0263); nil ⇒ absolute
	labels      *labelMapper // non-nil ⇒ emit_tags_as_labels (ADR-0264); nil ⇒ full dotted name, no labels
	...
}
```
Thread the 4th bool through both constructors (`:80`–`:101`):
```go
func NewMetricsServiceSink(client metricsClient, node *corev3.Node, reportCountersAsDeltas, emitTagsAsLabels bool) *MetricsServiceSink {
	return newSinkWithCapacity(client, node, reportCountersAsDeltas, emitTagsAsLabels, defaultChannelCapacity)
}

func newSinkWithCapacity(client metricsClient, node *corev3.Node, reportCountersAsDeltas, emitTagsAsLabels bool, capacity int) *MetricsServiceSink {
	s := &MetricsServiceSink{
		ch:     make(chan []*dto.MetricFamily, capacity),
		client: client,
		node:   node,
		done:   make(chan struct{}),
	}
	if reportCountersAsDeltas {
		s.delta = newDeltaState()
	}
	if emitTagsAsLabels {
		s.labels = newLabelMapper()
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}
```
Apply in `Submit` (`:109`–`:112`) — delta FIRST (rewrites the Counter VALUE), labels SECOND (rewrites Name + Label); both build the sink's OWN batch, never mutating the shared slice:
```go
func (s *MetricsServiceSink) Submit(batch []*dto.MetricFamily) {
	if s.delta != nil {
		batch = s.delta.apply(batch) // build the sink's OWN delta batch; never mutate the shared slice
	}
	if s.labels != nil {
		batch = s.labels.apply(batch) // rewrite Name+Label; orthogonal to delta's VALUE rewrite
	}
	select {
	case s.ch <- batch:
	default:
		// ... unchanged drop-newest + rate-limited diagnostic ...
	}
}
```
Update the `MetricsServiceSink` doc comment (`:61`–`:63`) to note the optional labels mode (parallel to the deltas note).

- [ ] **Step 4: Fix the other call sites** (compile) — add the 4th arg (`false` = the 47.1/47.2a no-labels path):
- `internal/statssink/registration_test.go:33`: `NewMetricsServiceSink(client, testNode(), false, false)`
- `internal/statssink/sink_test.go:156,198,252`: `NewMetricsServiceSink(client, testNode(), false, false)`
- `internal/statssink/sink_test.go:236`: `newSinkWithCapacity(client, testNode(), false, false, 1)`
- `internal/statssink/sink_test.go:272` (the existing 47.2a delta-mode test): `newSinkWithCapacity(client, testNode(), true /*deltas*/, false /*labels*/, 8)`

- [ ] **Step 5: Thread the bool through main** — `cmd/envoy-go/main.go:199`:
```go
			statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, node, cfg.ReportCountersAsDeltas, cfg.EmitTagsAsLabels))
```
(`cfg` is the loop var over `bs.StatsSinkConfigs` at `:194`.) No other change — the Flusher/Freeze ordering, the Dialer hoist, and the LIFO-drain Close are untouched (the label mapper is stateless in-sink; the writer goroutine + ticker + the 47.2a delta map remain the only background mutators).

- [ ] **Step 6: Run tests + build to verify they pass**

Run: `go test ./internal/statssink/ -count=1` then `go build ./...`
Expected: PASS — the labels-mode + both-knobs tests green; all existing absolute/delta tests green (value-stability: `false,false` ⇒ both nil ⇒ Submit byte-identical to 47.1); build clean. Then `gofmt -l && go vet && golangci-lint run` on `internal/statssink/` + `cmd/envoy-go/`, clean.

- [ ] **Step 7: Commit**

```bash
git add internal/statssink/sink.go internal/statssink/sink_test.go internal/statssink/registration_test.go cmd/envoy-go/main.go
git commit -m "phase 47.2b Task 5: thread emit_tags_as_labels into MetricsServiceSink (apply labelMapper after delta in Submit; main wiring; both-knobs compose; false ⇒ 47.1-identical)"
```

---

## Task 6: The receiver label-aware surface (`test/helpers/metricsservice`)

**Files:**
- Modify: `test/helpers/metricsservice/metricsservice.go:60`–`:74` (the `byKey` field), `:115`–`:127` (init in `newServer`), `:145`–`:175` (`StreamMetrics` accumulate), `:228`–`:243` (clear in `Reset`).
- Test: `test/helpers/metricsservice/metricsservice_test.go` (create if absent).

- [ ] **Step 1: Write the failing test** — assert `FamilyWithLabels` keys by `{name, sorted-labels}` while `Family`/`FamilySum` stay name-only (the 0089/0090 non-regress guard). Two families share a residual NAME but differ by labels; the keyed lookup separates them. If no in-process test client harness exists, exercise `StreamMetrics` through the same loopback dial the differential uses, or add a focused unit test feeding two `StreamMetricsMessage`s:

```go
// after streaming a family name="cluster.upstream_rq_total" with label
// {envoy.cluster_name=c_backend} value 7, AND another with {envoy.cluster_name=c_metrics} value 3:
lbA := []*dto.LabelPair{{Name: proto.String("envoy.cluster_name"), Value: proto.String("c_backend")}}
if v, typ, ok := srv.FamilyWithLabels("cluster.upstream_rq_total", lbA); !ok || v != 7 || typ != dto.MetricType_COUNTER {
	t.Fatalf("FamilyWithLabels(c_backend) = %v,%v,%v want 7,COUNTER,true", v, typ, ok)
}
lbB := []*dto.LabelPair{{Name: proto.String("envoy.cluster_name"), Value: proto.String("c_metrics")}}
if v, _, ok := srv.FamilyWithLabels("cluster.upstream_rq_total", lbB); !ok || v != 3 {
	t.Fatalf("FamilyWithLabels(c_metrics) = %v,%v want 3,true (label-keyed separation)", v, ok)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/helpers/metricsservice/ -count=1 -v`
Expected: FAIL — `FamilyWithLabels` undefined.

- [ ] **Step 3: Implement the additive surface**

Add the composite-key accumulator to `Server` (`:60`–`:74`, alongside `fams`/`sums`):
```go
	mu       sync.RWMutex
	fams     map[string]familyValue
	sums     map[string]float64
	byKey    map[string]familyValue // keyed by labelKey(name, sorted labels) — emit_tags_as_labels (ADR-0264)
	node     *corev3.Node
	msgCount int
```
Init in `newServer` (`:121`–`:127`): `byKey: make(map[string]familyValue),`.
Accumulate in `StreamMetrics` (`:160`–`:172`). The existing code declares `ms` INSIDE the `if ms := f.GetMetric(); len(ms) > 0` value-extraction block at `:162`, so `ms` is NOT in scope at the `s.fams`/`s.sums` store site — HOIST it out of the `if` first so the byKey store can read `ms[0].GetLabel()`:
```go
		for _, f := range msg.GetEnvoyMetrics() {
			var value float64
			ms := f.GetMetric() // hoisted out of the former `if ms := ...` init
			if len(ms) > 0 {
				switch f.GetType() {
				case dto.MetricType_GAUGE:
					value = ms[0].GetGauge().GetValue()
				default:
					value = ms[0].GetCounter().GetValue()
				}
			}
			s.fams[f.GetName()] = familyValue{value: value, typ: f.GetType()}
			s.sums[f.GetName()] += value
			if len(ms) > 0 {
				s.byKey[labelKey(f.GetName(), ms[0].GetLabel())] = familyValue{value: value, typ: f.GetType()}
			}
		}
```
Add the helper + accessor (next to `FamilySum`):
```go
// labelKey is the composite accumulator key under emit_tags_as_labels: the family
// residual name plus its label set rendered in SORTED key order, so families
// sharing a residual name but differing by labels (cluster.upstream_rq_total for
// both c_backend AND c_metrics) accumulate under distinct keys. The label slice is
// sorted by name into a copy (the input is not mutated).
func labelKey(name string, labels []*dto.LabelPair) string {
	cp := append([]*dto.LabelPair(nil), labels...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].GetName() < cp[j].GetName() })
	var b strings.Builder
	b.WriteString(name)
	for _, l := range cp {
		b.WriteByte('|')
		b.WriteString(l.GetName())
		b.WriteByte('=')
		b.WriteString(l.GetValue())
	}
	return b.String()
}

// FamilyWithLabels returns the last-seen (value, type, ok) for the accumulated
// MetricFamily whose residual name is `name` and whose label set equals `labels`
// (compared in sorted-key order — order-insensitive). ok is false if none was
// received. Used by the 0091 emit_tags_as_labels differential: multiple families
// share a residual name (cluster.upstream_rq_total for c_backend AND c_metrics) so
// the name-only Family() is ambiguous under labels. Additive to Family()/FamilySum()
// (name-only), which 0089/0090 keep (their families have empty labels).
func (s *Server) FamilyWithLabels(name string, labels []*dto.LabelPair) (value float64, typ dto.MetricType, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fv, ok := s.byKey[labelKey(name, labels)]
	return fv.value, fv.typ, ok
}
```
Add the imports `"sort"` and `"strings"` to the file. Clear in `Reset` (`:236`–`:243`): add `s.byKey = make(map[string]familyValue)`. Update the `Server` doc block (`:46`–`:55`) to mention `FamilyWithLabels`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/helpers/metricsservice/ -count=1 -v`
Expected: PASS. Then `gofmt -l && go vet && golangci-lint run` on the package, clean. Re-run `go test ./internal/statssink/ ./internal/bootstrap/ ./internal/stats/ -count=1` to confirm no cross-package regression.

- [ ] **Step 5: Commit**

```bash
git add test/helpers/metricsservice/
git commit -m "phase 47.2b Task 6: add the label-aware FamilyWithLabels surface (composite {name,sorted-labels} key) to the metricsservice receiver (additive; 0089/0090 non-regress)"
```

---

## Task 7: The `0091-stats-sink-metrics-service-labels` cross-side EXACT fixture

**Files:**
- Create: `test/fixtures/0091-stats-sink-metrics-service-labels/{driver/driver.go,envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`.
- Modify: `test/differential/runner_test.go:117`–`:118` (register the driver).

- [ ] **Step 1: Clone the AUTHORITATIVE 0090 driver** (the FRESHEST two-receiver template — `reference_periodic_sink_differential_two_receivers`)

```bash
mkdir -p test/fixtures/0091-stats-sink-metrics-service-labels/driver
cp test/fixtures/0090-stats-sink-metrics-service-deltas/driver/driver.go test/fixtures/0091-stats-sink-metrics-service-labels/driver/driver.go
cp test/fixtures/0090-stats-sink-metrics-service-deltas/envoy.yaml      test/fixtures/0091-stats-sink-metrics-service-labels/envoy.yaml
cp test/fixtures/0090-stats-sink-metrics-service-deltas/envoy-go.yaml   test/fixtures/0091-stats-sink-metrics-service-labels/envoy-go.yaml
```

- [ ] **Step 2: Adapt the driver** — the changes from 0090:
  - `fixtureName = "0091-stats-sink-metrics-service-labels"`; `refListenerPort = 10091`; `wantNodeID = "envoy-go-subject-0091"` (cluster `wantNodeCluster` unchanged is fine).
  - **DROP the delta-SUM model + the stability barrier.** Replace `subsetNames []string` with a `subset` of `{residualName string, labels []*dto.LabelPair}` entries (the keyed lookup targets — D-MSL-SUBSET-2XX includes the 2xx two-label split):
    ```go
    type subsetEntry struct {
    	residual string
    	labels   []*dto.LabelPair
    }
    var subset = []subsetEntry{
    	{"cluster.upstream_rq_total", lp("envoy.cluster_name", backendName)},
    	{"http.downstream_rq_total", lp("envoy.http_conn_manager_prefix", statPrefix)},
    	{"http.downstream_rq_xx", lp2("envoy.http_conn_manager_prefix", statPrefix, "envoy.response_code_class", "2")},
    }
    // lp/lp2 build sorted []*dto.LabelPair (proto.String).
    ```
  - `familyReading` carries the LAST-SEEN cumulative value + type (NOT a sum): `type familyReading struct { value float64; typ dto.MetricType; ok bool }`.
  - `driveSide` snapshot loop reads `FamilyWithLabels`:
    ```go
    snap := sideSnapshot{fams: make(map[string]familyReading, len(subset))}
    for _, e := range subset {
    	v, typ, ok := srv.FamilyWithLabels(e.residual, e.labels)
    	snap.fams[keyOf(e)] = familyReading{value: v, typ: typ, ok: ok}
    }
    ```
    (`keyOf(e)` = the labelKey rendering, so the snapshot map separates the 3 entries; or index by `e.residual` since the three residuals are distinct.)
  - **DELETE `awaitFurtherFlushes` + its call** (cumulative value model — NO delta-SUM stability barrier; `reference_delta_sink_differential_stability_barrier` is a 47.2a/0090-only concern; AMEND-MSL-VALUE-CUMULATIVE). Keep `pollSubset` but converge on the VALUE:
    ```go
    func subsetConverged(srv *metricsservice.Server) bool {
    	for _, e := range subset {
    		v, _, ok := srv.FamilyWithLabels(e.residual, e.labels)
    		if !ok || v != float64(numReq) {
    			return false
    		}
    	}
    	return true
    }
    ```
    and `describeSubset` reads `FamilyWithLabels`.
  - `assertSide` asserts `fr.value == float64(numReq)` (cumulative last-seen == K, the 0089 model) and `fr.typ == COUNTER`; update `FIXTURE_0090_DUMP` → `FIXTURE_0091_DUMP` + the dump labels.
  - The package doc comment: rewrite to the labels-split + CUMULATIVE-value model (each family carries the residual dotted name + the tag labels; the Counter value is cumulative-absolute == K after K 2xx; NO delta-SUM barrier; the 2xx two-label SN4 split; gauges labeled but unasserted).
  - Add a `proto` import (`google.golang.org/protobuf/proto`) for the `lp`/`lp2` builders.

- [ ] **Step 3: Adapt the YAMLs** — in BOTH `envoy.yaml` and `envoy-go.yaml`, REPLACE `report_counters_as_deltas: true` with `emit_tags_as_labels: true` in the metrics_service `typed_config` block (the 0090 yamls carry `transport_api_version: V3` + `report_counters_as_deltas: true`; swap the second knob):
```yaml
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
      transport_api_version: V3            # keep
      emit_tags_as_labels: true            # NEW for 0091 (REPLACES report_counters_as_deltas)
      grpc_service:
        envoy_grpc:
          cluster_name: c_metrics
```
Update the YAML header comments (the fixture name + the knob description: labels-split, cumulative value) — DROP the delta prose. Keep everything else (clusters, listener, node, `stats_flush_interval: 0.5s`) verbatim.

- [ ] **Step 4: Write `expectations.yaml` + `README.md`** — describe the labels-split + cumulative-value model (each subset counter's residual name + its sorted tag labels; cumulative last-seen value == K; the 2xx SN4 two-label split; type stays COUNTER; gauges labeled but unasserted; framing unasserted, label-set ordering NORMALIZED via sorted-key comparison; two per-side receivers + hard `Close()`). The README is the CORRECT two-receiver/cumulative/labels doc (do NOT carry 0090's delta-SUM prose).

- [ ] **Step 5: REGISTER the fixture in the differential runner** (LOAD-BEARING — without this every `0091` run matches zero subtests = a vacuous green; the C1 guard from the 47.2a PLAN — `reference_differential_run_selector`)

`test/differential/runner_test.go` — add the blank-import immediately after the `0090` one at `:117` (before the `helpers` import at `:118`):
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0090-stats-sink-metrics-service-deltas/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0091-stats-sink-metrics-service-labels/driver"
```

- [ ] **Step 6: Run the fixture (subject + reference)** — Docker must be available (`reference_docker_probe_bridge_network`).

Run: `go test ./test/differential/ -run 'TestDifferential/0091' -count=1 -v`
Expected: PASS — and the output MUST show a real `--- PASS: TestDifferential/0091-...` line, NOT "no tests to run" / `ok ... 0 tests` (a vacuous result means the Step-5 import is missing — `reference_differential_run_selector`). Both sides converge to the keyed `{residual-name, sorted labels}` cumulative `value == K` on the subset; decode-ran is proven by the converge poll. **NEVER run bare `-run 0091`** (matches zero subtests — `reference_differential_run_selector`).

- [ ] **Step 7: Commit**

```bash
git add test/fixtures/0091-stats-sink-metrics-service-labels/ test/differential/runner_test.go
git commit -m "phase 47.2b Task 7: 0091-stats-sink-metrics-service-labels cross-side EXACT {residual-name,labels} cumulative-value fixture (registered in runner_test.go)"
```

---

## Task 8: `0091` deliberate breaks + flake-soak + full-package `-race` (CONTROLLER-run)

These are CONTROLLER actions (not a subagent commit) per `reference_differential_break_protocol_count1` — `-count=1` on EVERY break; `-run 'TestDifferential/0091'` NEVER bare; `git restore` to undo (NO checkout-sha/amend — `feedback_subagent_worktree_detach`). (a)/(b) are the load-bearing breaks — they prove the differential SEES the label split (a property the 47.1 no-labels inlining does NOT satisfy).

- [ ] **Step 1: Break (a) — emit the full inlined name with NO label split** — in `label.go` `apply`, temporarily make the tagged branch a passthrough (`out = append(out, fam); continue` for every family). Run `go test ./test/differential/ -run 'TestDifferential/0091' -count=1`. Expected: FAIL — the keyed `FamilyWithLabels("cluster.upstream_rq_total", {envoy.cluster_name})` lookup MISSES (the family is still named `cluster.c_backend.upstream_rq_total` with empty labels) → `subsetConverged` never reaches K → converge-poll timeout / assertion fails. Restore via `git restore internal/statssink/label.go`.

- [ ] **Step 2: Break (b) — emit the Prometheus UNDERSCORE key** — temporarily change the key projection to `proto.String(l.Key)` (the raw `envoy_cluster_name`, dropping the `"envoy." + TrimPrefix`). Run the same `-count=1` command. Expected: FAIL — the `LabelPair.name` is `envoy_cluster_name` not `envoy.cluster_name` → the keyed lookup's labelKey mismatches → fails. Restore via `git restore`.

- [ ] **Step 3: Break (c) — emit the Prometheus FLATTENED name** — temporarily set the residual to `"envoy_" + strings.ReplaceAll(residual, ".", "_")` (the flattenToProm base). Run the same `-count=1` command. Expected: FAIL — the family name is `envoy_cluster_upstream_rq_total` not the dotted residual → the keyed lookup misses on name → fails. Restore via `git restore`.

- [ ] **Step 4: Break (d) — drop the SN4 response-code-class label on `downstream_rq_2xx`** — temporarily, in `name.go` `ExtractTags`, skip the SN4 collapse (comment out the `statusClassRE` block). Run the same `-count=1` command. Expected: FAIL — `http.hcm_local.downstream_rq_2xx` stays named `...downstream_rq_2xx` with only the prefix label → the 2xx keyed entry (`name="http.downstream_rq_xx"` + two labels) misses → fails. **ALSO** confirm this break trips `internal/stats` byte-identity: `go test ./internal/stats/ -run 'TestSN4|TestFlatten' -count=1` FAILS (the prom side also depends on SN4) — a cross-check that SN4 is load-bearing on both projections. Restore via `git restore internal/stats/name.go`.

- [ ] **Step 5: Confirm restored green** — `go test ./test/differential/ -run 'TestDifferential/0091' -count=1`. Expected: PASS.

- [ ] **Step 6: Flake-soak** — `for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0091' -count=1 || break; done`. Expected: 20/20 PASS. (An UNRELATED fixture flaking under suite load is a different gate — this is the isolated 0091 soak.)

- [ ] **Step 7: FULL-package `-race`** — `go test -race ./internal/statssink/ -count=1` AND `go test -race ./internal/stats/ -count=1` (the label mapper + delta map + writer goroutine + ticker are background mutators — `reference_full_suite_race_after_background_mutator`; the FULL packages, not a `-run` subset; `internal/stats` because the shared `ExtractTags` is now on the sink's flush path too). Expected: PASS, no data races.

---

## Task 9: Full differential + six-gate + ADR-0264 body + contract/STATE/ROADMAP (ROW 47 FLIPS `done`)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0264 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`.

- [ ] **Step 1: ADR-0264 body** — append §Decision + §Consequences to the §Context drafted at SPEC §13 (ADR-0044). §Decision: lift the `bootstrap.go:454` `emit_tags_as_labels` reject (reference-parity-accept); add `StatsSinkConfig.EmitTagsAsLabels` (scalar bool); extract the shared `stats.ExtractTags` SN-rule core (D-MS-LABEL-REUSE option (a) — `flattenToProm` becomes a thin Prometheus projection, byte-identical); a sink-owned `labelMapper` applied in `Submit` after `delta` (delta-then-labels; dotted residual name + dotted `envoy.` keys + sorted `LabelPair`s; BOTH types; no shared-slice mutation); the `FamilyWithLabels` receiver surface; the `0091` cross-side EXACT differential. §Consequences: +0 stat surface; BOTH `MetricsServiceConfig` knobs now accepted; the histogram_emit_mode/stats_flush_on_admin/sibling-TypeURL/non-V3/google_grpc/empty-cluster rejects STAY strict; **row 47 FLIPS `done`** (the FINAL sub-leg; ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

- [ ] **Step 2: BEHAVIOR_CONTRACT delta** — extend the `### Stats sinks — the metrics_service gRPC sink` section per SPEC §9 (the tag→`LabelPair` split semantics; residual dotted name; dotted `envoy.` keys; SN4 `_Nxx→_xx` + response_code_class; BOTH Counter and Gauge; value unchanged unless `report_counters_as_deltas` also set; untagged → full name + empty labels; absent/false ⇒ 47.1-identical; the `emit_tags_as_labels` reject lifted, every other reject strict; stat surface stays 1200).

- [ ] **Step 3: STATE + ROADMAP** — STATE.md header → `phase 47.2b IMPL done`; recorded NEXT → check the ROADMAP for the next chartered Observability row (statsd / dog_statsd / OTLP metrics / tap filter — confirm whether any is chartered/`pending` vs merely mentioned; if a next row is chartered, NEXT → its BRAINSTORM; if NONE remains, NEXT is empty → the router's TERMINATION SENTINEL applies); counts → fixtures 93 / DECISIONS ADR-0264 (stat 1200 / fuzzers 50 / BackendKind 38 unchanged). **ROADMAP row 47 FLIPS `done`** (ADR-0106 + `reference_roadmap_split_phase_row_done` — 47.2b is the FINAL sub-leg); update the Observability family-progress note (47.1 core DONE; 47.2a deltas DONE; 47.2b tags-as-labels DONE → row 47 done).

- [ ] **Step 4: Re-verify counts**
```bash
ls -d test/fixtures/*/ | wc -l                       # expect 93
grep -rh '^func Fuzz' --include='*.go' . | wc -l     # expect 50
go mod tidy -diff                                     # EMPTY (no new module — client_model already direct)
```

- [ ] **Step 5: The six-gate (CONTROLLER, on the squashed/frozen HEAD)** — run, in order, and confirm each is GREEN before claiming completion (`superpowers:verification-before-completion`):
  1. `gofmt -l .` → empty.
  2. `go vet ./...` → clean.
  3. `golangci-lint run` → clean.
  4. `go build ./...` → clean.
  5. `go test ./... -count=1` → full unit suite green (incl. `internal/stats`, `internal/statssink`, `internal/bootstrap`, `test/helpers/metricsservice`).
  6. The FULL differential (all 93 dirs): `go test ./test/differential/ -count=1` → green (an unrelated `subject ready: EOF` startup flake is isolate-re-run + full-re-run, NOT a regression — `reference_differential_fullsuite_startup_flake`).
  Plus: `go mod tidy -diff` → EMPTY; `go test -race ./internal/statssink/ ./internal/stats/ -count=1` → clean (Task 8 Step 7).

- [ ] **Step 6: Commit the docs bundle**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md
git commit -m "phase 47.2b Task 9: ADR-0264 body + BEHAVIOR_CONTRACT labels delta + STATE/ROADMAP (row 47 FLIPS done; fixtures 93)"
```

- [ ] **Step 7: Stage-close (CONTROLLER)** — squash the task commits, re-run the six-gate on the frozen HEAD, fast-forward onto master, push (`feedback_push_to_origin`), remove the worktree. Then roll `next-prompt.txt` + STATE: if a next Observability row is chartered, to that row's BRAINSTORM; if NONE remains and STATE's NEXT is empty, the router's TERMINATION SENTINEL applies (create the `stop` file).

---

## Notes carried from the SPEC (honor throughout)

- **D-MS-LABEL-REUSE is the shared core** (Task 3): `stats.ExtractTags` is the single SN-rule matcher; the byte-identity guard (`name_test.go` 53 tests unchanged + the full prom differential) is the load-bearing gate. If an arm's residual projection breaks byte-identity, preserve that arm's exact base-building (the fallback) — never edit a `name_test.go` expectation to make it pass.
- **No-shared-slice-mutation is load-bearing** (SPEC §3.2 HARD CONSTRAINT): `labelMapper.apply()` returns a NEW batch (new family/metric, sorted new `LabelPair`s; value pointer shared read-only); `TestLabelMapper_Apply_DoesNotMutateInput` is the guard. The compose order is delta-THEN-labels (delta on the Counter VALUE, labels on the NAME+LABEL — orthogonal).
- **Labels on BOTH types** (AMEND-MSL-LABELS-ON-BOTH-TYPES): the transform touches Counter AND Gauge families (CONTRAST the 47.2a Counter-only deltas); `TestLabelMapper_Apply` asserts a gauge carries labels.
- **The value is CUMULATIVE under labels alone** (AMEND-MSL-VALUE-CUMULATIVE): `0091` asserts last-seen `value == K` (the 0089 model), NOT a delta-SUM — NO post-convergence stability barrier (`reference_delta_sink_differential_stability_barrier` is a 47.2a/0090-only concern). The `awaitFurtherFlushes` barrier from the 0090 clone is DELETED.
- **The 2xx two-label split is included** (D-MSL-SUBSET-2XX): `http.downstream_rq_xx` + `{envoy.http_conn_manager_prefix, envoy.response_code_class=2}` (sorted); envoy-go's SN4 reproduces it. Fallback to the two single-label splits only if a residual mismatch surfaces at IMPL.
- **The receiver surface is additive** (D-MSL-RECEIVER-KEY): `FamilyWithLabels` (composite `name|sorted-labels` key) is added; `Family`/`FamilySum`/`Messages`/`Count` STAY (0089/0090 non-regress — empty-label families are unaffected).
- **Differential discipline:** two per-side receivers + hard `Close()` (`reference_periodic_sink_differential_two_receivers`); `-run 'TestDifferential/0091'` never bare (`reference_differential_run_selector`); `-count=1` on every break (`reference_differential_break_protocol_count1`); the Docker bridge + decode-ran proof (`reference_docker_probe_bridge_network`); the receiver is driver-owned (BackendKind stays 38 — `reference_differential_grpc_receiver_driver_owned`); assert the aggregated `{name,label-set,value}` payload NOT framing, label-set ordering NORMALIZED via sorted-key comparison (`reference_streaming_sink_differential_framing`); the live reference is the wire truth (`reference_wire_format_both_sides_see_same_bytes`); one fixture dir = ONE runner branch (0091 ≠ 0090 ≠ 0089 — `reference_differential_fixture_dispatch_constraint`); the label mapper + delta map + `ExtractTags` on the flush path + writer goroutine + ticker are background mutators ⇒ FULL-package `-race` on BOTH `internal/statssink` and `internal/stats` (`reference_full_suite_race_after_background_mutator`).
- **Row 47 FLIPS `done` at THIS IMPL** (the FINAL sub-leg; ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.
