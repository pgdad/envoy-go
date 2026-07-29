package statssink

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/pgdad/envoy-go/internal/stats"
)

// golden_bytemirror_test.go — phase 79 T7. THE point of this file: before it
// landed, NOTHING in the tree could tell a byte-mirror ExtractTags arm
// (runtime./access_logs./tracing. -> residual == input, no labels) apart from a
// tag-HOISTING arm (middle segment promoted to a label). The whole
// internal/stats + internal/statssink suite was GREEN under a hoisting arm.
//
// These four goldens pin the WIRE OUTPUT of all four ExtractTags consumers in
// the statssink package over one shared 13-entry registry, so a hoisting arm
// goes RED on every one of them.
//
// The roster is 10 byte-mirror names + 3 STACKED CONTROLS, because a positive
// assertion alone cannot catch an over-firing arm:
//   - cluster.backend.upstream_rq_total / .membership_total: the SN1 hoist MUST
//     SURVIVE (a byte-mirror arm that swallowed cluster.* would show up here).
//   - listener_manager.listener_create_success: matches NO rule, so ExtractTags
//     errors and every consumer takes its own fallback. It MUST STAY
//     UNTRANSFORMED (an arm that started matching it would show up here).
//
// Registration order IS emission order (stats.Registry.Walk is
// registration-ordered, registry.go:133), so every assertion below is on the
// ORDERED SLICE. A per-name presence check or a bare length check would be
// blind to a reordering AND to a name rewrite that happened to keep the count.

// goldenName is one roster entry: the full dotted registered name, the residual
// ExtractTags must return, whether it is a counter, and its value.
type goldenName struct {
	full     string
	residual string // == full for every byte-mirror entry; the SN1 hoist for the controls
	counter  bool
	value    int64
}

// goldenRoster is the registry in REGISTRATION order == emission order.
// residual for the listener_manager control is its FULL name: ExtractTags
// returns an error for it, and every consumer's error fallback is the full
// untransformed name (label.go:39, dogstatsd.go:86, graphite.go:70,
// otlp.go:194-197).
var goldenRoster = []goldenName{
	{"cluster.backend.upstream_rq_total", "cluster.upstream_rq_total", true, 7},
	{"cluster.backend.membership_total", "cluster.membership_total", false, 3},
	{"listener_manager.listener_create_success", "listener_manager.listener_create_success", true, 1},
	{"runtime.num_keys", "runtime.num_keys", false, 5},
	{"runtime.num_layers", "runtime.num_layers", false, 2},
	{"access_logs.grpc_access_log.logs_written", "access_logs.grpc_access_log.logs_written", true, 11},
	{"access_logs.grpc_access_log.logs_dropped", "access_logs.grpc_access_log.logs_dropped", true, 0},
	{"access_logs.open_telemetry_access_log.logs_written", "access_logs.open_telemetry_access_log.logs_written", true, 13},
	{"access_logs.open_telemetry_access_log.logs_dropped", "access_logs.open_telemetry_access_log.logs_dropped", true, 0},
	{"tracing.opentelemetry.spans_sent", "tracing.opentelemetry.spans_sent", true, 17},
	{"tracing.opentelemetry.spans_dropped", "tracing.opentelemetry.spans_dropped", true, 0},
	{"tracing.zipkin.spans_sent", "tracing.zipkin.spans_sent", true, 19},
	{"tracing.zipkin.spans_dropped", "tracing.zipkin.spans_dropped", true, 0},
}

// goldenTaggedIdx are the roster indices carrying a SURVIVING extracted tag —
// the two cluster.* stacked controls. Every other index MUST reach the wire
// with zero tags. Keeping this as an explicit index set (not "everything with a
// dot") means a hoisting arm cannot quietly join it.
var goldenTaggedIdx = map[int]bool{0: true, 1: true}

// goldenPrefix is the sink prefix all four goldens compose with.
const goldenPrefix = "envoy-go"

// goldenOTLPVersion is the telemetry.sdk.version resource attribute value. It
// is part of every OTLP byte pin below, so it is a named constant rather than a
// literal at each call site.
const goldenOTLPVersion = "1.0.0"

// goldenRegistry builds the 13-entry registry in goldenRoster order.
func goldenRegistry(t *testing.T) *stats.Registry {
	t.Helper()
	reg := stats.NewRegistry()
	for _, e := range goldenRoster {
		if e.counter {
			reg.NewCounter(e.full).Add(uint64(e.value))
			continue
		}
		reg.NewGauge(e.full).Set(e.value)
	}
	return reg
}

// assertOrderedLines compares two string slices element by element and reports
// every difference (Errorf, not Fatalf: a single t.Fatalf would make the rest
// of the roster dead code and hide how WIDE a regression is).
func assertOrderedLines(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: line count = %d, want %d\ngot:  %q\nwant: %q", label, len(got), len(want), got, want)
	}
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			t.Errorf("%s: line[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

// statsdValueSuffix is ":<value>|<c|g>" for a roster entry.
func statsdValueSuffix(e goldenName) string {
	typ := "|g"
	if e.counter {
		typ = "|c"
	}
	return ":" + strconv.FormatInt(e.value, 10) + typ
}

// readOneBatch Submits the snapshot and returns the ORDERED lines of the single
// batched datagram. Both UDP sinks are constructed with a datagram cap far above
// the whole batch, so all 13 lines ride ONE datagram: the order is then the
// sink's emission order by construction, not a property of UDP delivery.
func readOneBatch(t *testing.T, read func(n int) []string) []string {
	t.Helper()
	dgrams := read(1)
	if len(dgrams) != 1 {
		t.Fatalf("expected exactly 1 batched datagram, got %d: %q", len(dgrams), dgrams)
	}
	return strings.Split(dgrams[0], "\n")
}

// goldenDatagramCap is comfortably above the whole 13-line batch (~800 B) and
// below the 4096 B test read buffer.
const goldenDatagramCap = 4000

// ---------------------------------------------------------------------------
// GOLDEN 1 — dog_statsd: "envoy-go.<FULL DOTTED NAME>:<v>|<c|g>", NO "|#"
// suffix, except the two stacked controls which keep their hoisted tag.
// ---------------------------------------------------------------------------

func TestGolden_DogStatsd_ByteMirrorWire(t *testing.T) {
	addr, read := udpListener(t)
	reg := goldenRegistry(t)

	s, err := NewDogStatsdSink(addr, goldenPrefix, goldenDatagramCap)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := readOneBatch(t, read)

	want := make([]string, 0, len(goldenRoster))
	for i, e := range goldenRoster {
		line := goldenPrefix + "." + e.residual + statsdValueSuffix(e)
		if goldenTaggedIdx[i] {
			line += "|#envoy.cluster_name:backend"
		}
		want = append(want, line)
	}
	assertOrderedLines(t, "dogstatsd", got, want)

	// Independent of the literal comparison: NO untagged roster entry may carry
	// a tag suffix at all. A hoisting arm's first symptom is a "|#" appearing on
	// a byte-mirror line.
	for i, line := range got {
		if goldenTaggedIdx[i] {
			continue
		}
		if strings.Contains(line, "|#") {
			t.Errorf("dogstatsd: line[%d] %q carries a |# tag suffix; byte-mirror names must emit none", i, line)
		}
	}
}

// ---------------------------------------------------------------------------
// GOLDEN 2 — graphite_statsd: "envoy-go.<FULL DOTTED NAME>:<v>|<c|g>", NO ";"
// anywhere, except the two stacked controls.
// ---------------------------------------------------------------------------

func TestGolden_Graphite_ByteMirrorWire(t *testing.T) {
	addr, read := udpListener(t)
	reg := goldenRegistry(t)

	s, err := NewGraphiteStatsdSink(addr, goldenPrefix, goldenDatagramCap)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := readOneBatch(t, read)

	want := make([]string, 0, len(goldenRoster))
	for i, e := range goldenRoster {
		name := goldenPrefix + "." + e.residual
		if goldenTaggedIdx[i] {
			name += ";envoy.cluster_name=backend"
		}
		want = append(want, name+statsdValueSuffix(e))
	}
	assertOrderedLines(t, "graphite", got, want)

	for i, line := range got {
		if goldenTaggedIdx[i] {
			continue
		}
		if strings.Contains(line, ";") {
			t.Errorf("graphite: line[%d] %q carries a ';' tag segment; byte-mirror names must emit none", i, line)
		}
	}
}

// ---------------------------------------------------------------------------
// GOLDEN 3 — labelMapper (metrics_service emit_tags_as_labels): the FULL dotted
// name and ZERO LabelPairs, except the two stacked controls.
// ---------------------------------------------------------------------------

func TestGolden_LabelMapper_ByteMirrorWire(t *testing.T) {
	reg := goldenRegistry(t)
	out := newLabelMapper().apply(snapshot(reg, 0))

	gotNames := make([]string, 0, len(out))
	for _, fam := range out {
		gotNames = append(gotNames, fam.GetName())
	}
	wantNames := make([]string, 0, len(goldenRoster))
	for _, e := range goldenRoster {
		wantNames = append(wantNames, e.residual)
	}
	assertOrderedLines(t, "labelmapper", gotNames, wantNames)

	if len(out) != len(goldenRoster) {
		t.Fatalf("labelmapper: family count = %d, want %d", len(out), len(goldenRoster))
	}
	for i, fam := range out {
		labels := fam.GetMetric()[0].GetLabel()
		if goldenTaggedIdx[i] {
			if len(labels) != 1 {
				t.Errorf("labelmapper: control[%d] %q labels = %d, want 1", i, fam.GetName(), len(labels))
				continue
			}
			if labels[0].GetName() != "envoy.cluster_name" || labels[0].GetValue() != "backend" {
				t.Errorf("labelmapper: control[%d] label = {%q,%q}, want {envoy.cluster_name,backend}",
					i, labels[0].GetName(), labels[0].GetValue())
			}
			continue
		}
		if len(labels) != 0 {
			t.Errorf("labelmapper: [%d] %q emitted %d labels %+v; byte-mirror names must emit none",
				i, fam.GetName(), len(labels), labels)
		}
	}
}

// ---------------------------------------------------------------------------
// GOLDEN 4 — OTLP metrics, over the FULL (useTagExtractedName,
// emitTagsAsAttributes) cross-product.
//
// The cross-product is MANDATORY, not thoroughness. The (F, F) cell is PROVEN
// INSENSITIVE: with a tag-hoisting arm applied it is byte-identical to the
// byte-mirror tree, because neither knob ever consults the ExtractTags result.
// A golden run only on the DEFAULT knobs is green on the correct tree AND green
// on the wrong one. The (F,T) and (T,*) cells are what make this gate real.
//
// NOTE also: byte size is NOT monotone under a hoisting arm — the (T, F) cell
// SHRINKS, because hoisting removes a segment from the name and (F) drops the
// attribute that would have replaced it. Never assert "bytes grew".
// ---------------------------------------------------------------------------

type otlpCell struct {
	name                 string
	useTagExtractedName  bool
	emitTagsAsAttributes bool
	wantBytes            int
}

// goldenOTLPCells pins the marshaled ExportMetricsServiceRequest size per cell.
// The pin is safe against wall-clock drift: OTLP's *_unix_nano fields are proto
// fixed64, so a nonzero timestamp always occupies the same number of bytes
// whatever its value. TestGolden_OTLP_TimestampNormalizationIsLive proves both
// that those fields are actually present and that the rendered request is
// timestamp-invariant, so this pin cannot be vacuously stable.
//
// The sizes are a function of goldenOTLPVersion: it rides the request as the
// telemetry.sdk.version resource attribute, so every figure here is
// (cell base) + len(goldenOTLPVersion), measured exactly linear over a
// 0/5/15/16-character sweep. Change that constant and all four pins move
// together by the same amount.
var goldenOTLPCells = []otlpCell{
	{"F_F_default_INERT_UNDER_HOIST", false, false, 1134},
	{"F_T_attrs", false, true, 1200},
	{"T_F_residual_name", true, false, 1118},
	{"T_T_both", true, true, 1184},
}

func TestGolden_OTLP_ByteMirrorWire(t *testing.T) {
	reg := goldenRegistry(t)
	batch := snapshot(reg, 0)

	for _, cell := range goldenOTLPCells {
		t.Run(cell.name, func(t *testing.T) {
			fake := newFakeOTLP()
			s := NewOTLPMetricsSink(fake, goldenOTLPVersion, false, cell.useTagExtractedName, cell.emitTagsAsAttributes, goldenPrefix)
			t.Cleanup(func() { _ = s.Close() })

			req := driveOnce(t, s, fake, batch)
			ms := metricsOf(req)

			gotNames := make([]string, 0, len(ms))
			for _, m := range ms {
				gotNames = append(gotNames, m.GetName())
			}
			wantNames := make([]string, 0, len(goldenRoster))
			for _, e := range goldenRoster {
				base := e.full
				if cell.useTagExtractedName {
					base = e.residual
				}
				wantNames = append(wantNames, goldenPrefix+"."+base)
			}
			assertOrderedLines(t, "otlp names", gotNames, wantNames)

			if len(ms) != len(goldenRoster) {
				t.Fatalf("otlp: metric count = %d, want %d", len(ms), len(goldenRoster))
			}
			for i, m := range ms {
				attrs := otlpDataPointAttrs(t, m)
				wantTagged := cell.emitTagsAsAttributes && goldenTaggedIdx[i]
				if !wantTagged {
					if len(attrs) != 0 {
						t.Errorf("otlp: [%d] %q emitted %d attributes, want 0", i, m.GetName(), len(attrs))
					}
					continue
				}
				if len(attrs) != 1 {
					t.Errorf("otlp: control[%d] %q attributes = %d, want 1", i, m.GetName(), len(attrs))
					continue
				}
				k, v := attrs[0].GetKey(), attrs[0].GetValue().GetStringValue()
				if k != "envoy.cluster_name" || v != "backend" {
					t.Errorf("otlp: control[%d] attribute = {%q,%q}, want {envoy.cluster_name,backend}", i, k, v)
				}
			}

			raw, err := proto.Marshal(req)
			if err != nil {
				t.Fatalf("proto.Marshal: %v", err)
			}
			if len(raw) != cell.wantBytes {
				t.Errorf("otlp: marshaled size = %d bytes, want %d", len(raw), cell.wantBytes)
			}
		})
	}
}

// otlpDataPointAttrs returns the first data point's attributes for either a Sum
// (counter) or a Gauge metric. Both roster types must be reachable — the
// emit_tags knob is a name-model knob, not a value-model one, so a gauge that
// silently lost its attributes would otherwise go unchecked.
func otlpDataPointAttrs(t *testing.T, m *metricspb.Metric) []*commonpb.KeyValue {
	t.Helper()
	var dps []*metricspb.NumberDataPoint
	switch {
	case m.GetSum() != nil:
		dps = m.GetSum().GetDataPoints()
	case m.GetGauge() != nil:
		dps = m.GetGauge().GetDataPoints()
	default:
		t.Fatalf("otlp metric %q carries neither Sum nor Gauge data", m.GetName())
	}
	if len(dps) != 1 {
		t.Fatalf("otlp metric %q has %d data points, want 1", m.GetName(), len(dps))
	}
	return dps[0].GetAttributes()
}

// otlpUnixNanoRE matches the OTLP timestamp fields in a prototext rendering.
var otlpUnixNanoRE = regexp.MustCompile(`(start_time_unix_nano|time_unix_nano):\s*[0-9]+`)

// otlpWhitespaceRE collapses prototext's deliberately randomized inter-token
// whitespace so two renderings can be compared literally.
var otlpWhitespaceRE = regexp.MustCompile(`\s+`)

// TestGolden_OTLP_TimestampNormalizationIsLive is the anti-vacuity guard for the
// byte pins above. It renders two flushes taken at different wall-clock instants
// and proves (a) the *_unix_nano fields ARE present — the normalization regex
// MUST fire, or the "timestamps are the only varying part" premise is untested
// and the pin could be stable for the wrong reason — and (b) once normalized the
// two renderings are byte-identical, i.e. nothing else in the request varies
// between flushes.
func TestGolden_OTLP_TimestampNormalizationIsLive(t *testing.T) {
	reg := goldenRegistry(t)
	batch := snapshot(reg, 0)

	fake := newFakeOTLP()
	s := NewOTLPMetricsSink(fake, goldenOTLPVersion, false, false, true, goldenPrefix)
	t.Cleanup(func() { _ = s.Close() })

	first := normalizeOTLP(t, driveOnce(t, s, fake, batch))
	second := normalizeOTLP(t, driveOnce(t, s, fake, batch))

	if first != second {
		t.Errorf("normalized OTLP renderings differ across flushes:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// normalizeOTLP renders req as prototext, replaces every *_unix_nano value, and
// collapses whitespace. It FAILS THE TEST if the timestamp regex never fires:
// a normalization that matches nothing would make every comparison built on it
// silently vacuous.
func normalizeOTLP(t *testing.T, req *colmetricspb.ExportMetricsServiceRequest) string {
	t.Helper()
	text, err := prototext.Marshal(req)
	if err != nil {
		t.Fatalf("prototext.Marshal: %v", err)
	}
	hits := otlpUnixNanoRE.FindAll(text, -1)
	if len(hits) == 0 {
		t.Fatalf("*_unix_nano normalization matched NOTHING in a %d-byte rendering; "+
			"the timestamp-invariance premise behind the byte pins is unproven", len(text))
	}
	normalized := otlpUnixNanoRE.ReplaceAll(text, []byte("${1}: NORMALIZED"))
	return string(otlpWhitespaceRE.ReplaceAll(normalized, []byte(" ")))
}
