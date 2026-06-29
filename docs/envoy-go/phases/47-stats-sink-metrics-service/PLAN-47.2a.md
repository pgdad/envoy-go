# Phase 47.2a Implementation Plan — `report_counters_as_deltas=true`: the per-sink last-flush Counter DELTA state

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a bootstrap `stats_sinks[]` metrics_service entry sets `report_counters_as_deltas: true`, envoy-go emits, per COUNTER family, the per-flush DELTA (`current − last_flushed`) instead of the cumulative absolute value the 47.1 sink emits; GAUGES stay absolute. Absent/false is value-identical to 47.1.

**Architecture:** A per-sink `deltaState` (a `map[string]uint64` keyed by full dotted family name) lives in a new `internal/statssink/delta.go`. A delta-mode `MetricsServiceSink` applies the transform in `Submit` BEFORE the channel send, building a NEW batch (rebuilt COUNTER families; GAUGE families shared by pointer) so it never mutates the shared absolute snapshot slice the `Flusher` fans to every sink (`flusher.go:47`–`:49`). The `bootstrap.go:446` deltas strict-reject is LIFTED (reference-parity-accept); a `ReportCountersAsDeltas bool` is added to `StatsSinkConfig` and threaded through `cmd/envoy-go/main.go`. The `0090-stats-sink-metrics-service-deltas` cross-side EXACT differential asserts the running per-name delta-SUM == K via a new additive `FamilySum` surface on the driver-owned `test/helpers/metricsservice` receiver.

**Tech Stack:** Go; `github.com/prometheus/client_model v0.6.1` (`dto`); `google.golang.org/protobuf/proto`; `go-control-plane/envoy v1.32.4` (`MetricsServiceConfig`); the differential runner (`test/differential`); Docker reference `envoyproxy/envoy:contrib-v1.37.2`.

**Charter:** `docs/envoy-go/phases/47-stats-sink-metrics-service/SPEC-47.2a.md` (read §3 the transform design, §8 the `0090` differential, §10 the task sketch, §11 the executed pins, §12 the D-questions, §13 the ADR-0263 §Context draft).

**Execution mode (per the router + memories):** DOCS-ONLY direct-on-master is the PLAN's own landing; the IMPL THIS PLAN DESCRIBES is subagent-driven (`feedback_execution_style` · `superpowers:subagent-driven-development`) in a FRESH worktree off master (`feedback_git_worktrees`). Subagents commit LOCALLY only (`feedback_subagents_no_push`); the CONTROLLER verifies each commit, cleans leak files, re-runs the six-gate + the FULL-package `internal/statssink` `-race`, does the `0090` deliberate breaks ITSELF, squashes, and pushes at stage-close (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`). Per-task: `gofmt -l` + `golangci-lint` on touched packages, not just `go vet` (`feedback_pertask_gofmt_lint`).

**Anchors:** ADR-0263 (§Decision/§Consequences body lands at this IMPL per ADR-0044) · ADR-0080 (strict-reject — LIFT only `:446`, keep the rest) · ADR-0045 (sub-split — FINAL re-check Task 1, no further split) · ADR-0106 + `reference_roadmap_split_phase_row_done` (row 47 STAYS `in-progress`; flips `done` only at 47.2b IMPL) · ADR-0059 (frozen Registry) · ADR-0060 (no histograms) · ADR-0262 (the 47.1 substrate).

**Anticipated counts at IMPL exit:** stat **1200** (non-H2 **1196**, +0) / fixtures **91 → 92** (`0090-stats-sink-metrics-service-deltas`) / fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0262 → ADR-0263**. NO new package, NO new go.mod module, NO new BackendKind.

---

## File map

**Create:**
- `internal/statssink/delta.go` — the `deltaState` (`map[string]uint64`) + `apply(abs) []*dto.MetricFamily` transform.
- `internal/statssink/delta_test.go` — table-driven transform unit tests.
- `test/fixtures/0090-stats-sink-metrics-service-deltas/driver/driver.go` — cloned from 0089, FamilySum poll-to-converge.
- `test/fixtures/0090-stats-sink-metrics-service-deltas/envoy.yaml` — reference bootstrap (clone + `report_counters_as_deltas: true`).
- `test/fixtures/0090-stats-sink-metrics-service-deltas/envoy-go.yaml` — subject bootstrap (clone + `report_counters_as_deltas: true`).
- `test/fixtures/0090-stats-sink-metrics-service-deltas/expectations.yaml` — delta-SUM prose.
- `test/fixtures/0090-stats-sink-metrics-service-deltas/README.md` — correct two-receiver/SUM README.

**Modify:**
- `internal/bootstrap/bootstrap.go:272` (add `ReportCountersAsDeltas bool`), `:446`–`:448` (lift the reject), `:455` (set the field on append), `:427`–`:430` (doc comment).
- `internal/bootstrap/bootstrap_test.go:1809`–`:1821` (drop the `report_counters_as_deltas` reject case from `TestStatsSink_Rejects`; KEEP `emit_tags_as_labels` + all others), `:1925`–`:1944` (repurpose/extend `TestStatsSink_AcceptReportCountersFalse` to also assert the `true` parse-accept sets the field).
- `internal/statssink/sink.go:60`–`:93`, `:101`–`:111` (add `delta *deltaState` field; bool param on `NewMetricsServiceSink` + `newSinkWithCapacity`; apply in `Submit`).
- `internal/statssink/sink_test.go:156,198,236,252` (add the `false` arg; add a delta-mode Submit test).
- `internal/statssink/registration_test.go:33` (add the `false` arg).
- `cmd/envoy-go/main.go:199` (pass `cfg.ReportCountersAsDeltas`).
- `test/differential/runner_test.go:116` (add the `0090` driver blank-import after the `0089` one — WITHOUT this the fixture is unregistered and every `0090` run is a vacuous green).
- `test/helpers/metricsservice/metricsservice.go:54`–`:66`,`:113`–`:118`,`:146`–`:163`,`:194`–`:207` (add `sums map[string]float64`, `FamilySum`, update `StreamMetrics` + `Reset`).
- `test/helpers/metricsservice/metricsservice_test.go` (if present — add a FamilySum test; else add the test to the driver/package as appropriate).
- `test/fixtures/0089-stats-sink-metrics-service/README.md` + `expectations.yaml` (reconcile the STALE single-shared-receiver + `Reset()` prose to the as-built two-receiver model — `reference_periodic_sink_differential_two_receivers`).
- `docs/envoy-go/DECISIONS.md` (ADR-0263 §Decision/§Consequences body), `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the deltas contract delta), `docs/envoy-go/STATE.md` + `docs/envoy-go/ROADMAP.md` (row 47 STAYS `in-progress`).

---

## Task 1: Baselines, PROGRESS scaffold, FINAL ADR-0045 re-check (D-MS-SPLIT-2)

**Files:**
- Create: `docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.2a.md` (controller-local; do NOT commit if the phase convention keeps PROGRESS out of git — match the 47.1 IMPL precedent).

- [ ] **Step 1: Capture baselines** (the worktree is fresh off master at the post-SPEC tip)

Run and record:
```bash
ls -d test/fixtures/*/ | wc -l                          # expect 91
grep -rh '^func Fuzz' --include='*.go' . | wc -l        # expect 50
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1   # tail ADR-0262 (next-free 0263)
go build ./... && go test ./internal/statssink/ ./internal/bootstrap/ ./test/helpers/metricsservice/ -count=1
```
Expected: build clean; the three packages green; counts as noted.

- [ ] **Step 2: Record the D-MS-SPLIT-2 disposition in PROGRESS**

The realized scope is ~150 LoC of new logic (bootstrap ~4 lines; `delta.go` ~45; `sink.go` ~6; `main.go` ~1; receiver ~12; `0090` fixture a mechanical 0089 clone). This is a single focused change — NO 47.2a-i/47.2a-ii sub-split (ADR-0045 final gate; SPEC §3.0). Note it; this resolves D-MS-SPLIT-2.

- [ ] **Step 3: Commit the PROGRESS scaffold** (if the phase commits PROGRESS; otherwise skip)

```bash
git add docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.2a.md
git commit -m "phase 47.2a Task 1: baselines + PROGRESS + D-MS-SPLIT-2 final no-split re-check"
```

---

## Task 2: Config — lift the `report_counters_as_deltas:true` reject + add `ReportCountersAsDeltas bool`

**Files:**
- Modify: `internal/bootstrap/bootstrap.go:272` (field), `:446`–`:448` (lift), `:455` (append), `:427`–`:430` (doc).
- Test: `internal/bootstrap/bootstrap_test.go:1809`–`:1821`, `:1925`–`:1944`.

- [ ] **Step 1: Write the failing test** — repurpose `TestStatsSink_AcceptReportCountersFalse` into `TestStatsSink_AcceptReportCountersDeltas` (or add a sibling) asserting BOTH the `false` and the `true` config parse-accept AND that `true` sets the field:

```go
// TestStatsSink_AcceptReportCountersDeltas: report_counters_as_deltas false OR
// true both parse-accept; true records ReportCountersAsDeltas on the config
// (the strict-reject was lifted at 47.2a — reference-parity-accept, ADR-0263).
func TestStatsSink_AcceptReportCountersDeltas(t *testing.T) {
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
      report_counters_as_deltas: ` + tc.val + `
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
			if got := bs.StatsSinkConfigs[0].ReportCountersAsDeltas; got != tc.want {
				t.Errorf("ReportCountersAsDeltas = %v, want %v", got, tc.want)
			}
		})
	}
}
```
Also DELETE the `report_counters_as_deltas` case (lines ~1809–1821) from `TestStatsSink_Rejects` — it is no longer a reject. KEEP the `emit_tags_as_labels`, `histogram_emit_mode`, `transport_api_version_V2`, `google_grpc`, `empty_cluster_name`, `sibling_statsd_sink`, and `stats_flush_on_admin` cases unchanged.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bootstrap/ -run 'TestStatsSink_AcceptReportCountersDeltas' -count=1 -v`
Expected: FAIL — `ReportCountersAsDeltas` is not a field of `StatsSinkConfig` (compile error), or (if you add the field first) the `true` subtest fails because the reject still fires.

- [ ] **Step 3: Add the field**

`internal/bootstrap/bootstrap.go:272`:
```go
type StatsSinkConfig struct {
	ClusterName string // MetricsServiceConfig.grpc_service.envoy_grpc.cluster_name
	// ReportCountersAsDeltas is MetricsServiceConfig.report_counters_as_deltas
	// (ADR-0263): true ⇒ each COUNTER family carries the per-flush delta
	// (current − last_flushed) instead of the cumulative absolute value; GAUGES
	// stay absolute. Absent/false ⇒ the 47.1 cumulative path.
	ReportCountersAsDeltas bool
}
```

- [ ] **Step 4: Lift the reject + set the field**

`internal/bootstrap/bootstrap.go` — DELETE the `:446`–`:448` arm:
```go
	if d := msc.GetReportCountersAsDeltas(); d != nil && d.GetValue() {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service report_counters_as_deltas:true is not supported (47.1 emits cumulative absolute values; deferred to 47.2)", idx)
	}
```
and set the field on the append (`:455`):
```go
	result.StatsSinkConfigs = append(result.StatsSinkConfigs, StatsSinkConfig{
		ClusterName:            eg.GetClusterName(),
		ReportCountersAsDeltas: msc.GetReportCountersAsDeltas().GetValue(),
	})
```
Update THREE doc comments to drop `report_counters_as_deltas` from the strict-reject list (it is now consumed; `emit_tags_as_labels:true` STAYS rejected): the `StatsSinkConfig` TYPE-level comment (`:265`–`:271`, the "The strict-reject arms (report_counters_as_deltas / emit_tags_as_labels / …)" line at `:269`), the `StatsSinkConfigs` field doc (`:322`–`:331`), and the `parseMetricsServiceConfig` doc comment (`:427`–`:430`). `GetReportCountersAsDeltas()` returns `*wrapperspb.BoolValue`; `.GetValue()` on nil yields `false` — the accepted cumulative default, no nil guard needed.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/bootstrap/ -run 'TestStatsSink' -count=1 -v`
Expected: PASS — `TestStatsSink_AcceptReportCountersDeltas` both subtests green; `TestStatsSink_Rejects` green (the `emit_tags_as_labels` case + the rest still reject). Then `gofmt -l internal/bootstrap/ && go vet ./internal/bootstrap/ && golangci-lint run ./internal/bootstrap/...` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 47.2a Task 2: lift the report_counters_as_deltas:true reject + add StatsSinkConfig.ReportCountersAsDeltas"
```

---

## Task 3: The per-sink delta transform (`internal/statssink/delta.go`)

**Files:**
- Create: `internal/statssink/delta.go`, `internal/statssink/delta_test.go`.

- [ ] **Step 1: Write the failing table-driven test** (`delta_test.go`)

Cover: first flush emits absolute (`current − 0`); a subsequent flush emits the per-flush increment and latches; an idle flush emits 0; GAUGE families pass through ABSOLUTE; the input batch is NOT mutated; family order + names preserved. Build input families with the same shape `snapshot()` produces (one Metric per family, `proto.Float64`, `TimestampMs`).

```go
package statssink

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

func counterFam(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   proto.String(name),
		Type:   dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{{Counter: &dto.Counter{Value: proto.Float64(v)}, TimestampMs: proto.Int64(1)}},
	}
}
func gaugeFam(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   proto.String(name),
		Type:   dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: proto.Float64(v)}, TimestampMs: proto.Int64(1)}},
	}
}
func counterVal(f *dto.MetricFamily) float64 { return f.GetMetric()[0].GetCounter().GetValue() }

func TestDeltaState_Apply(t *testing.T) {
	d := newDeltaState()

	// Flush 1: absolute (last empty -> current-0).
	out := d.apply([]*dto.MetricFamily{counterFam("c.rq", 7), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 7 {
		t.Errorf("flush1 counter delta = %v, want 7 (absolute)", got)
	}
	if out[1].GetType() != dto.MetricType_GAUGE || out[1].GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("flush1 gauge not passed through absolute: %v", out[1])
	}

	// Flush 2: increment since flush 1 -> delta=3; latched.
	out = d.apply([]*dto.MetricFamily{counterFam("c.rq", 10), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 3 {
		t.Errorf("flush2 counter delta = %v, want 3", got)
	}

	// Flush 3: idle -> delta=0 (still emitted).
	out = d.apply([]*dto.MetricFamily{counterFam("c.rq", 10), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 0 {
		t.Errorf("flush3 idle delta = %v, want 0", got)
	}
}

func TestDeltaState_Apply_DoesNotMutateInput(t *testing.T) {
	d := newDeltaState()
	in := []*dto.MetricFamily{counterFam("c.rq", 7)}
	_ = d.apply(in)
	_ = d.apply(in) // re-applying the SAME input must see 7 again, not a mutated value
	if got := counterVal(in[0]); got != 7 {
		t.Fatalf("input counter mutated: got %v, want 7", got)
	}
}

func TestDeltaState_Apply_GaugeSharedNotTransformed(t *testing.T) {
	d := newDeltaState()
	g := gaugeFam("c.live", 5)
	out := d.apply([]*dto.MetricFamily{g})
	if out[0] != g { // gauge family shared by pointer (untransformed)
		t.Errorf("gauge family should be shared by pointer, got a copy")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/statssink/ -run 'TestDeltaState' -count=1 -v`
Expected: FAIL — `newDeltaState`/`apply` undefined.

- [ ] **Step 3: Implement `delta.go`**

```go
package statssink

import (
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

// deltaState holds the per-sink last-flushed COUNTER values keyed by full dotted
// family name, for report_counters_as_deltas=true (ADR-0263). A delta-mode sink
// owns one deltaState and applies it in Submit: each COUNTER family's value is
// rewritten to current-last (the per-flush delta) and last is latched to current;
// GAUGE (and any non-COUNTER) families pass through absolute, untransformed. The
// map is touched ONLY from MetricsServiceSink.Submit, which the single Flusher
// goroutine calls serially per flush (the sink contract forbids Submit after
// Close), so no lock is needed.
type deltaState struct {
	last map[string]uint64
}

func newDeltaState() *deltaState { return &deltaState{last: make(map[string]uint64)} }

// apply returns a NEW batch in which every COUNTER family carries the per-flush
// delta (current - last[name]) instead of the absolute value, latching
// last[name]=current. It MUST NOT mutate the input batch: the Flusher fans the
// SAME absolute slice to every sink (flusher.go:47-49), so an in-place rewrite
// would corrupt a sibling absolute sink and be non-idempotent across the fan-out.
// COUNTER families are rebuilt (new MetricFamily/Metric/Counter); GAUGE (and any
// non-COUNTER) families are shared by pointer (not transformed — the proto field
// applies to counters only, D-MS-DELTA-GAUGE). An absent key reads 0, so the
// first flush / a counter's first appearance emits current-0 = the absolute value
// (AMEND-MSD-FIRST-FLUSH-ABSOLUTE — no special first-flush branch). Counters are
// monotone u64 (current >= last), so the delta is non-negative; no underflow
// guard. envoy-go's mapping emits exactly one Metric per family (no labels), so
// keying last by family name is exact. The absolute value originated as
// float64(Counter.Load()) (u64) so uint64(value) round-trips exactly for the
// realistic counter range (< 2^53).
func (d *deltaState) apply(abs []*dto.MetricFamily) []*dto.MetricFamily {
	out := make([]*dto.MetricFamily, 0, len(abs))
	for _, fam := range abs {
		if fam.GetType() != dto.MetricType_COUNTER {
			out = append(out, fam) // GAUGE/other: shared as-is, untransformed
			continue
		}
		name := fam.GetName()
		ms := make([]*dto.Metric, 0, len(fam.GetMetric()))
		for _, m := range fam.GetMetric() {
			cur := uint64(m.GetCounter().GetValue())
			delta := cur - d.last[name]
			d.last[name] = cur
			ms = append(ms, &dto.Metric{
				Counter:     &dto.Counter{Value: proto.Float64(float64(delta))},
				TimestampMs: m.TimestampMs, // read-only share of the *int64
			})
		}
		out = append(out, &dto.MetricFamily{Name: fam.Name, Type: fam.Type, Metric: ms})
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/statssink/ -run 'TestDeltaState' -count=1 -v`
Expected: PASS (all three). Then `gofmt -l internal/statssink/ && go vet ./internal/statssink/ && golangci-lint run ./internal/statssink/...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/statssink/delta.go internal/statssink/delta_test.go
git commit -m "phase 47.2a Task 3: per-sink Counter delta transform (delta.go) — first=absolute, idle=0, gauges absolute, no input mutation"
```

---

## Task 4: Wire the transform into the sink (`internal/statssink/sink.go`)

**Files:**
- Modify: `internal/statssink/sink.go:60`–`:93`, `:101`–`:111`.
- Test: `internal/statssink/sink_test.go:156,198,236,252` + a new delta-mode Submit test.

- [ ] **Step 1: Write the failing test** — add a delta-mode Submit test to `sink_test.go` (use the existing fake `metricsClient` + the existing stream-capture harness; confirm two flushes of an increasing counter arrive at the receiver as deltas, and that a default (absolute) sink is unchanged). Sketch (adapt to the file's existing fake names):

Use the EXISTING test double from `sink_test.go`: `fakeMetricsClient{streams: []*fakeMetricsStream{stream}}` with `stream.messages()` for capture (the same pattern as `TestSink_*` at `sink_test.go:155`,`:197`). Sketch:
```go
func TestSink_DeltaMode_RewritesCountersToDeltas(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), true /*reportCountersAsDeltas*/, 8)
	defer s.Close()

	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 7)})
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 10)})
	// poll stream.messages() until 2 messages captured, then assert:
	//   msgs[0].EnvoyMetrics[0].counter.value == 7 (absolute first flush)
	//   msgs[1].EnvoyMetrics[0].counter.value == 3 (delta)
}
```
(Reuse `counterFam` from `delta_test.go` — same package; reuse `testNode()` from `sink_test.go`. Mirror the existing send→`messages()` poll exactly; do NOT invent a constructor.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/statssink/ -run 'TestSink_DeltaMode' -count=1 -v`
Expected: FAIL — `newSinkWithCapacity` has 3 args, not 4 (compile error).

- [ ] **Step 3: Add the bool param + field + Submit branch**

`sink.go` — add the field:
```go
type MetricsServiceSink struct {
	ch          chan []*dto.MetricFamily
	client      metricsClient
	node        *corev3.Node
	delta       *deltaState // non-nil ⇒ report_counters_as_deltas (ADR-0263); nil ⇒ absolute
	...
}
```
Thread the bool through the constructors:
```go
func NewMetricsServiceSink(client metricsClient, node *corev3.Node, reportCountersAsDeltas bool) *MetricsServiceSink {
	return newSinkWithCapacity(client, node, reportCountersAsDeltas, defaultChannelCapacity)
}

func newSinkWithCapacity(client metricsClient, node *corev3.Node, reportCountersAsDeltas bool, capacity int) *MetricsServiceSink {
	s := &MetricsServiceSink{
		ch:     make(chan []*dto.MetricFamily, capacity),
		client: client,
		node:   node,
		done:   make(chan struct{}),
	}
	if reportCountersAsDeltas {
		s.delta = newDeltaState()
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}
```
Apply in `Submit` BEFORE the channel send (so the latch advances once per flush, on the single Flusher goroutine; the writer's reconnect re-sends the SAME already-delta'd batch — idempotent):
```go
func (s *MetricsServiceSink) Submit(batch []*dto.MetricFamily) {
	if s.delta != nil {
		batch = s.delta.apply(batch) // build the sink's OWN delta batch; never mutate the shared slice
	}
	select {
	case s.ch <- batch:
	default:
		// ... unchanged drop-newest + rate-limited diagnostic ...
	}
}
```
Update the `MetricsServiceSink` doc comment to note the optional delta mode.

- [ ] **Step 4: Fix the other call sites** (compile)

Add the `false` arg (absolute — preserves the 47.1 path verbatim):
- `internal/statssink/registration_test.go:33`: `NewMetricsServiceSink(client, testNode(), false)`
- `internal/statssink/sink_test.go:156,198,252`: `NewMetricsServiceSink(client, testNode(), false)`
- `internal/statssink/sink_test.go:236`: `newSinkWithCapacity(client, testNode(), false, 1)`

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/statssink/ -count=1` then `go test ./internal/statssink/ -run 'TestSink' -count=1 -v`
Expected: PASS — the delta-mode test green; all existing absolute tests green (value-stability: `false` ⇒ `s.delta == nil` ⇒ Submit byte-identical to 47.1). Then `gofmt -l && go vet && golangci-lint run` on the package, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/statssink/sink.go internal/statssink/sink_test.go internal/statssink/registration_test.go
git commit -m "phase 47.2a Task 4: thread report_counters_as_deltas into MetricsServiceSink (apply delta transform in Submit; false ⇒ 47.1-identical)"
```

---

## Task 5: Boot wiring (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go:199`.

- [ ] **Step 1: Thread the bool**

`cmd/envoy-go/main.go:199`:
```go
			statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, node, cfg.ReportCountersAsDeltas))
```
(`cfg` is the loop var over `bs.StatsSinkConfigs` at `:194`.) No other change — the Flusher/Freeze ordering, the Dialer hoist, and the LIFO-drain Close are untouched (the per-sink delta map is in-sink state; the writer goroutine + ticker remain the only background mutators).

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 47.2a Task 5: thread cfg.ReportCountersAsDeltas into the main sink wiring"
```

---

## Task 6: The receiver delta-SUM surface (`test/helpers/metricsservice`)

**Files:**
- Modify: `test/helpers/metricsservice/metricsservice.go:54`–`:66`, `:113`–`:118`, `:146`–`:163`, `:194`–`:207`.
- Test: `test/helpers/metricsservice/metricsservice_test.go` (create if absent).

- [ ] **Step 1: Write the failing test** — assert `FamilySum` accumulates across messages while `Family` stays last-seen (the 0089 non-regress guard). Drive the in-process server via a real gRPC client OR (simpler) call an exported test seam. If no test client harness exists, exercise `StreamMetrics` through a `bufconn`/loopback dial mirroring how `sink_test.go`/the differential connect; otherwise add a focused unit test that sends two `StreamMetricsMessage`s carrying the same counter name with values 3 then 4:

```go
// after streaming two messages with c.rq = 3 then c.rq = 4:
if sum, ok := srv.FamilySum("c.rq"); !ok || sum != 7 {
	t.Fatalf("FamilySum(c.rq) = %v,%v want 7,true", sum, ok)
}
if v, _, ok := srv.Family("c.rq"); !ok || v != 4 {
	t.Fatalf("Family(c.rq) last-seen = %v,%v want 4,true (0089 non-regress)", v, ok)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/helpers/metricsservice/ -count=1 -v`
Expected: FAIL — `FamilySum` undefined.

- [ ] **Step 3: Implement the additive surface**

Add the parallel map to `Server` (`:54`–`:66`):
```go
	mu   sync.RWMutex
	fams map[string]familyValue
	sums map[string]float64
	node *corev3.Node
```
Init in `newServer` (`:113`–`:118`): `sums: make(map[string]float64),`.
Accumulate in `StreamMetrics` (`:146`–`:163`, right after the `s.fams[...] = ...` store):
```go
			s.fams[f.GetName()] = familyValue{value: value, typ: f.GetType()}
			s.sums[f.GetName()] += value
```
Add the accessor (next to `Family`):
```go
// FamilySum returns the running SUM of every received value for the family named
// `name` across all messages, and ok=false if none was received. For a delta sink
// (report_counters_as_deltas=true) the per-flush counter deltas sum to the
// cumulative total (== K after K requests) — the 0090 convergence invariant
// (AMEND-MSD-SUM-IS-THE-INVARIANT). Additive to Family() (last-seen), which 0089
// keeps.
func (s *Server) FamilySum(name string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok = s.sums[name]
	return
}
```
Clear in `Reset` (`:194`–`:207`): add `s.sums = make(map[string]float64)`. Update the `Server` doc block to mention `FamilySum`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/helpers/metricsservice/ -count=1 -v`
Expected: PASS. Then `gofmt -l && go vet && golangci-lint run` on the package, clean. Re-run `go test ./internal/statssink/ ./internal/bootstrap/ -count=1` to confirm no cross-package regression.

- [ ] **Step 5: Commit**

```bash
git add test/helpers/metricsservice/
git commit -m "phase 47.2a Task 6: add the per-name delta-SUM surface (FamilySum) to the metricsservice receiver (additive; 0089 non-regress)"
```

---

## Task 7: The `0090-stats-sink-metrics-service-deltas` cross-side EXACT fixture

**Files:**
- Create: `test/fixtures/0090-stats-sink-metrics-service-deltas/{driver/driver.go,envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`.
- Modify (reconcile): `test/fixtures/0089-stats-sink-metrics-service/{README.md,expectations.yaml}`.

- [ ] **Step 1: Clone the AUTHORITATIVE 0089 driver** (NOT the stale README — `reference_periodic_sink_differential_two_receivers`)

```bash
mkdir -p test/fixtures/0090-stats-sink-metrics-service-deltas/driver
cp test/fixtures/0089-stats-sink-metrics-service/driver/driver.go test/fixtures/0090-stats-sink-metrics-service-deltas/driver/driver.go
cp test/fixtures/0089-stats-sink-metrics-service/envoy.yaml      test/fixtures/0090-stats-sink-metrics-service-deltas/envoy.yaml
cp test/fixtures/0089-stats-sink-metrics-service/envoy-go.yaml   test/fixtures/0090-stats-sink-metrics-service-deltas/envoy-go.yaml
```

- [ ] **Step 2: Adapt the driver** — the changes from 0089:
  - `fixtureName = "0090-stats-sink-metrics-service-deltas"`; `refListenerPort = 10090` (refAdminPort 9901 is fine — fixtures run in isolated containers); `wantNodeID = "envoy-go-subject-0090"` (cluster unchanged is fine, but keep it distinct or identical — keep `envoy-go-differential`).
  - `familyReading` now carries the SUM: `type familyReading struct { sum float64; typ dto.MetricType; ok bool }`.
  - `driveSide` snapshot loop reads the SUM + the type:
    ```go
    for _, name := range subsetNames {
        sum, ok := srv.FamilySum(name)
        _, typ, _ := srv.Family(name) // type is last-seen COUNTER under deltas
        snap.fams[name] = familyReading{sum: sum, typ: typ, ok: ok}
    }
    ```
  - `subsetConverged` polls the SUM == K:
    ```go
    func subsetConverged(srv *metricsservice.Server) bool {
        for _, name := range subsetNames {
            v, ok := srv.FamilySum(name)
            if !ok || v != float64(numReq) {
                return false
            }
        }
        return true
    }
    ```
    and `describeSubset` reads `FamilySum`.
  - `assertSide` asserts `fr.sum == float64(numReq)` (was `fr.value`) and `fr.typ == COUNTER`; update the field name + the `FIXTURE_0089_DUMP` env to `FIXTURE_0090_DUMP` + the dump labels.
  - The package doc comment: rewrite to the delta-SUM model (each COUNTER family carries the per-flush delta; the SUM across flushes == K; `upstream_cx_total` excluded — sums to <K under connection reuse, AMEND-MSD-CX-NOT-K).
  - `subsetNames` is UNCHANGED (the same three counters; SPEC §8.1 / D-MS-DELTA-SUBJECT).

- [ ] **Step 3: Adapt the YAMLs** — in BOTH `envoy.yaml` and `envoy-go.yaml`, add `report_counters_as_deltas: true` into the existing metrics_service `typed_config` block. BOTH yamls already carry `transport_api_version: V3` (`0089/envoy.yaml:50`, `0089/envoy-go.yaml:44`); the new field is order-independent in the map — add it alongside, e.g.:
```yaml
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
      transport_api_version: V3            # already present in both 0089 yamls — keep
      report_counters_as_deltas: true      # NEW for 0090 (both sides)
      grpc_service:
        envoy_grpc:
          cluster_name: c_metrics
```
Update the YAML header comments to note the deltas knob. Keep everything else (clusters, listener, node, `stats_flush_interval: 0.5s`) verbatim.

- [ ] **Step 4: Write `expectations.yaml` + `README.md`** — describe the delta-SUM model (the SUM of each subset counter's per-flush deltas across flushes == K; a single flush is partial; type stays COUNTER; gauges absolute and unasserted; framing unasserted; two per-side receivers + hard `Close()`). The README is the CORRECT two-receiver/SUM doc (do NOT copy 0089's stale Reset prose).

- [ ] **Step 5: REGISTER the fixture in the differential runner** (LOAD-BEARING — without this every `0090` run matches zero subtests = a vacuous green; the headline gate would be structurally blind)

`test/differential/runner_test.go` — add the blank-import immediately after the `0089` one at `:116`:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0089-stats-sink-metrics-service/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0090-stats-sink-metrics-service-deltas/driver"
```

- [ ] **Step 6: Run the fixture (subject + reference)** — Docker must be available (`reference_docker_probe_bridge_network`).

Run: `go test ./test/differential/ -run 'TestDifferential/0090' -count=1 -v`
Expected: PASS — and the output MUST show a real `--- PASS: TestDifferential/0090-...` line, NOT "no tests to run" (a "no tests to run" / `ok ... 0 tests` means the Step-5 import is missing and the result is vacuous — `reference_differential_run_selector`). Both sides converge to delta-SUM == K on the subset; decode-ran is proven by the converge poll. (NEVER run bare `-run 0090`.)

- [ ] **Step 7: Reconcile the 0089 docs** — update `test/fixtures/0089-stats-sink-metrics-service/README.md` + `expectations.yaml` to the as-built TWO-receiver model (drop the stale "single shared receiver + `Reset()` at the start of each side's drive" prose; the driver code is the truth). Re-run `go test ./test/differential/ -run 'TestDifferential/0089' -count=1` — still PASS (docs-only change).

- [ ] **Step 8: Commit**

```bash
git add test/fixtures/0090-stats-sink-metrics-service-deltas/ test/differential/runner_test.go test/fixtures/0089-stats-sink-metrics-service/README.md test/fixtures/0089-stats-sink-metrics-service/expectations.yaml
git commit -m "phase 47.2a Task 7: 0090-stats-sink-metrics-service-deltas cross-side EXACT delta-SUM fixture (registered in runner_test.go) + reconcile 0089 two-receiver docs"
```

---

## Task 8: `0090` deliberate breaks + flake-soak + full-package `-race` (CONTROLLER-run)

These are CONTROLLER actions (not a subagent commit) per `reference_differential_break_protocol_count1` — `-count=1` on EVERY break; `-run 'TestDifferential/0090'` NEVER bare.

- [ ] **Step 1: Break (a) — emit absolute instead of delta** — in `delta.go` `apply`, temporarily change `delta := cur - d.last[name]` to `delta := cur` (skip the subtraction). Run `go test ./test/differential/ -run 'TestDifferential/0090' -count=1`. Expected: FAIL — the running SUM explodes (each flush re-adds the full cumulative). Restore via `git restore internal/statssink/delta.go` (NO checkout-sha/amend — `feedback_subagent_worktree_detach`).

- [ ] **Step 2: Break (b) — skip the latch** — temporarily delete `d.last[name] = cur`. Run the same `-count=1` command. Expected: FAIL — every flush computes `cur − 0` (absolute) → the SUM explodes. Restore via `git restore`.

- [ ] **Step 3: Confirm restored green** — `go test ./test/differential/ -run 'TestDifferential/0090' -count=1`. Expected: PASS. (Break (c) — applying deltas to gauges — is UNIT-test-only since gauges are unasserted in 0090; it is already covered by `TestDeltaState_Apply`'s gauge-passthrough assertion. Note in PROGRESS, do not run as a differential break.)

- [ ] **Step 4: Flake-soak** — `for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0090' -count=1 || break; done`. Expected: 20/20 PASS. (If an UNRELATED fixture flakes under suite load that is a different gate — this is the isolated 0090 soak.)

- [ ] **Step 5: FULL-package `-race`** — `go test -race ./internal/statssink/ -count=1` (the per-sink delta map + the writer goroutine + the ticker are background mutators — `reference_full_suite_race_after_background_mutator`; the FULL package, not a `-run` subset). Expected: PASS, no data races.

---

## Task 9: Full differential + six-gate + ADR-0263 body + contract/STATE/ROADMAP

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0263 §Decision/§Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`.

- [ ] **Step 1: ADR-0263 body** — append §Decision + §Consequences to the §Context drafted at SPEC §13 (ADR-0044). §Decision: lift the `bootstrap.go:446` deltas reject (reference-parity-accept); add `StatsSinkConfig.ReportCountersAsDeltas`; a per-sink `deltaState` (`map[string]uint64`) applied in `Submit` building the sink's own COUNTER families (Counter-only; gauges absolute; no shared-slice mutation); the `FamilySum` receiver surface; the `0090` delta-SUM differential. §Consequences: +0 stat surface; `emit_tags_as_labels` (47.2b/ADR-0264) stays deferred/strict; row 47 STAYS `in-progress`.

- [ ] **Step 2: BEHAVIOR_CONTRACT delta** — extend the `### Stats sinks — the metrics_service gRPC sink` section per SPEC §9 (deltas semantics; first=absolute; idle=0; type stays COUNTER; non-negative; gauges absolute; absent/false ⇒ value-identical; the deltas reject lifted, every other reject strict; stat surface stays 1200).

- [ ] **Step 3: STATE + ROADMAP** — STATE.md header → `phase 47.2a IMPL done`; recorded NEXT → the 47.2b SPEC; counts → fixtures 92 / DECISIONS ADR-0263 (stat 1200 / fuzzers 50 / BackendKind 38 unchanged). ROADMAP row 47 STAYS `in-progress` (ADR-0106 + `reference_roadmap_split_phase_row_done` — 47.2b flips it `done`); update the Observability family-progress note.

- [ ] **Step 4: Re-verify counts**
```bash
ls -d test/fixtures/*/ | wc -l                       # expect 92
grep -rh '^func Fuzz' --include='*.go' . | wc -l     # expect 50
```

- [ ] **Step 5: The six-gate (CONTROLLER, on the squashed/frozen HEAD)** — run, in order, and confirm each is GREEN before claiming completion (`superpowers:verification-before-completion`):
  1. `gofmt -l .` → empty.
  2. `go vet ./...` → clean.
  3. `golangci-lint run` → clean.
  4. `go build ./...` → clean.
  5. `go test ./... -count=1` → full unit suite green (incl. `internal/statssink`, `internal/bootstrap`, `test/helpers/metricsservice`).
  6. The FULL differential (all 92 dirs): `go test ./test/differential/ -count=1` → green (an unrelated `subject ready: EOF` startup flake is isolate-re-run, not a regression — `reference_differential_fullsuite_startup_flake`).
  Plus: `go mod tidy -diff` → EMPTY (no new module — `client_model v0.6.1` already direct); `go test -race ./internal/statssink/ -count=1` → clean (Task 8 Step 5).

- [ ] **Step 6: Commit the docs bundle**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md
git commit -m "phase 47.2a Task 9: ADR-0263 body + BEHAVIOR_CONTRACT deltas delta + STATE/ROADMAP (row 47 stays in-progress; fixtures 92)"
```

- [ ] **Step 7: Stage-close (CONTROLLER)** — squash the task commits, re-run the six-gate on the frozen HEAD, fast-forward onto master, push (`feedback_push_to_origin`), remove the worktree. Then roll `next-prompt.txt` + STATE to the **47.2b SPEC**.

---

## Notes carried from the SPEC (honor throughout)

- **No-shared-slice-mutation is load-bearing** (SPEC §3.2 HARD CONSTRAINT): `apply()` returns a NEW batch; `TestDeltaState_Apply_DoesNotMutateInput` is the guard. Even with one sink today, the per-sink framing + no-in-place rule is the correct boundary.
- **First-flush needs no special branch** (D-MSD-FIRST-FLUSH-IMPL): an absent map key reads 0 ⇒ `current − 0` = absolute.
- **GAUGES stay absolute** (D-MS-DELTA-GAUGE): the `*stats.Gauge`/non-COUNTER arm passes through untransformed (shared by pointer).
- **The delta-SUM is the invariant** (AMEND-MSD-SUM-IS-THE-INVARIANT): a single flush is partial; assert the SUM, never a single flush value; `Family()` last-seen would read ≈0 after idle convergence — that is why `0090` uses `FamilySum`.
- **Subset unchanged** (D-MS-DELTA-SUBJECT): the same three counters as 0089; `_cx_` excluded (sums to <K under connection reuse).
- **Differential discipline:** two per-side receivers + hard `Close()`; `-run 'TestDifferential/0090'` never bare; `-count=1` on every break; Docker bridge + decode-ran proof; receiver is driver-owned (BackendKind stays 38); assert aggregated payload not framing; one fixture dir = one runner branch (0090 ≠ 0089).
