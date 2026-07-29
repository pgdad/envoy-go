package stats

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

// helpTextRosterEntry is one INTERNAL hierarchical-dotted stat name that the
// helpText map is intended to cover, plus the metric kind it registers under.
//
// ⚠️ Only INTERNAL names appear here. The Prometheus names are DERIVED by
// running the real projection (flattenToProm / WriteProm) over these — never
// hand-typed. A hand-written Prometheus roster produced a false positive on six
// names during phase-79 specification work; deriving forecloses that class.
type helpTextRosterEntry struct {
	internal string
	gauge    bool
}

// helpTextRoster is the roster of internal stat names whose Prometheus
// projections helpText documents. One entry per distinct Prometheus base: the
// _Nxx status-class family (Rule SN4) collapses four internal names onto one
// base, so a single representative stands for the family.
//
// Each entry is anchored to a live registration call site:
//
//	listener.<addr>.*   internal/listener/manager.go registerListenerMetrics
//	                    (<addr> is normalizeAddr's output — ":" and "." both
//	                    become "_", brackets are dropped, so the real shape is
//	                    listener.0_0_0_0_10000.*, NOT listener.0.0.0.0_10000.*)
//	http.<sp>.*         internal/filter/hcm/config.go
//	cluster.<n>.*       internal/cluster/manager.go
//	server.live         internal/admin/admin.go
//	server.accesslog_*  internal/accesslog/stats.go
//	runtime.*           internal/bootstrap/bootstrap.go
//	access_logs.*       internal/accesslog/stats.go
//	tracing.*           internal/tracing/stats.go
var helpTextRoster = []helpTextRosterEntry{
	{internal: "listener.0_0_0_0_10000.downstream_cx_total"},
	{internal: "listener.0_0_0_0_10000.downstream_cx_active", gauge: true},
	{internal: "listener.0_0_0_0_10000.ssl.handshake"},
	{internal: "listener.0_0_0_0_10000.ssl.fail_verify_error"},
	{internal: "listener.0_0_0_0_10000.ssl.fail_verify_no_cert"},
	{internal: "listener.0_0_0_0_10000.ssl.no_certificate"},
	{internal: "http.ingress_http.downstream_rq_total"},
	{internal: "http.ingress_http.downstream_rq_2xx"},
	{internal: "cluster.backend.upstream_rq_total"},
	{internal: "cluster.backend.upstream_rq_2xx"},
	{internal: "cluster.backend.upstream_cx_total"},
	{internal: "cluster.backend.upstream_cx_active", gauge: true},
	{internal: "cluster.backend.membership_total", gauge: true},
	{internal: "server.live", gauge: true},
	{internal: "server.accesslog_dropped"},

	// Phase 79 byte-mirror roots.
	{internal: "runtime.num_keys", gauge: true},
	{internal: "runtime.num_layers", gauge: true},
	{internal: "access_logs.grpc_access_log.logs_written"},
	{internal: "access_logs.grpc_access_log.logs_dropped"},
	{internal: "access_logs.open_telemetry_access_log.logs_written"},
	{internal: "access_logs.open_telemetry_access_log.logs_dropped"},
	{internal: "tracing.opentelemetry.spans_sent"},
	{internal: "tracing.opentelemetry.spans_dropped"},
	{internal: "tracing.zipkin.spans_sent"},
	{internal: "tracing.zipkin.spans_dropped"},
}

// derivedHelpTextBases projects every roster entry through the REAL
// flattenToProm and returns the distinct Prometheus base names.
func derivedHelpTextBases(t *testing.T) map[string]bool {
	t.Helper()
	out := make(map[string]bool, len(helpTextRoster))
	for _, e := range helpTextRoster {
		base, _, err := flattenToProm(e.internal)
		if err != nil {
			t.Errorf("flattenToProm(%q): unexpected error %v (roster entry is not projectable)", e.internal, err)
			continue
		}
		out[base] = true
	}
	return out
}

// TestHelpText_KeySetExact asserts SET EQUALITY between the derived Prometheus
// roster and helpText's key set, reporting `missing` and `extra` SEPARATELY.
//
// ⚠️ It deliberately asserts NO COUNT. A count-only guard passes a build in
// which one roster name and one helpText key are BOTH wrong in matching ways.
//
// ⚠️ This guard is NOT sufficient on its own: it inspects keys, never values.
// An entry whose value is empty (or equal to its key) has a perfectly correct
// key set and still degrades on the wire. TestHelpText_NoSelfEqualHelp is the
// companion that closes that hole by driving the real projection. Both of those
// cells were EXECUTED: with helpText["envoy_cluster_membership_total"] set to ""
// this test stays GREEN while the companion reports
// "# HELP envoy_cluster_membership_total envoy_cluster_membership_total".
//
// ⚠️ KNOWN RESIDUAL, executed: a CONSISTENT rename — a helpText key typo'd AND
// the matching internal roster name typo'd the same way — passes BOTH guards.
// Neither guard knows the true registered names; helpTextRoster is the declared
// source of truth, so keep it anchored to the call sites listed above. A typo in
// the helpText key ALONE is caught by both guards.
func TestHelpText_KeySetExact(t *testing.T) {
	derived := derivedHelpTextBases(t)

	var missing, extra []string
	for base := range derived {
		if _, ok := helpText[base]; !ok {
			missing = append(missing, base)
		}
	}
	for key := range helpText {
		if !derived[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) != 0 {
		t.Errorf("helpText is missing entries for derived Prometheus names; missing: %v", missing)
	}
	if len(extra) != 0 {
		t.Errorf("helpText carries entries no roster name projects to; extra: %v", extra)
	}
}

// TestHelpText_NoSelfEqualHelp drives the REAL projection — WriteProm over a
// Registry holding the roster — parses every "# HELP <name> <help>" line and
// asserts none is SELF-EQUAL (help text byte-equal to the metric name).
//
// Self-equality is prom.go's degradation signature: prom.go:59-61 renders
//
//	help := g.help
//	if help == "" {
//	    help = g.name // fall back to the name as a no-op help when missing
//	}
//
// so a helpText lookup that misses — a deleted entry, a typo'd key, or an entry
// present with an EMPTY value — emits "# HELP envoy_x envoy_x". The key-set
// guard above cannot see the last two cases at all.
//
// NOTE: HELP text is an envoy-go-internal quality choice. The reference emits
// zero # HELP lines, so this guard protects an envoy-go-only surface.
func TestHelpText_NoSelfEqualHelp(t *testing.T) {
	r := NewRegistry()
	for _, e := range helpTextRoster {
		if e.gauge {
			r.NewGauge(e.internal).Set(1)
			continue
		}
		r.NewCounter(e.internal).Inc()
	}

	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}

	var parsed int
	var selfEqual []string
	for _, line := range strings.Split(buf.String(), "\n") {
		rest, ok := strings.CutPrefix(line, "# HELP ")
		if !ok {
			continue
		}
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			t.Errorf("malformed # HELP line (no help text): %q", line)
			continue
		}
		parsed++
		name, help := rest[:sp], rest[sp+1:]
		if help == name {
			selfEqual = append(selfEqual, line)
		}
	}

	if parsed == 0 {
		t.Errorf("parsed zero # HELP lines from WriteProm output (%d bytes) — the guard would be vacuous", buf.Len())
	}
	if len(selfEqual) != 0 {
		sort.Strings(selfEqual)
		t.Errorf("%d rendered HELP line(s) degraded to the metric name (missing or empty helpText entry): %q", len(selfEqual), selfEqual)
	}
}
