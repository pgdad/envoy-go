package zookeeperproxy

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// firstDiff returns a description of the first index where the two slices differ.
func firstDiff(a, b []string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return fmt.Sprintf("index %d: got %q, want %q", i, a[i], b[i])
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf("lengths differ: got %d, want %d", len(a), len(b))
	}
	return "no diff"
}

// TestCounterRoster_MatchesUpstreamMacro validates that rosterSuffixes() returns
// the exact 201 upstream-macro counter suffixes confirmed by the live reference
// image dump (envoyproxy/envoy:v1.37.2 admin /stats probe; probe date 2026-06-02).
// R2: the roster matches the upstream macro / live reference dump exactly.
func TestCounterRoster_MatchesUpstreamMacro(t *testing.T) {
	suffixes := rosterSuffixes()
	if len(suffixes) != 201 {
		t.Fatalf("rosterSuffixes() = %d names, want 201 (AMEND-A2)", len(suffixes))
	}
	// Family arithmetic (AMEND-A2): 4 plain + 28 _rq + 29 _rq_bytes +
	// 28 _decoder_error + 28 _resp + 28 _resp_bytes + 28 _resp_fast + 28 _resp_slow.
	families := map[string]int{}
	for _, s := range suffixes {
		switch {
		case strings.HasSuffix(s, "_rq_bytes"):
			families["_rq_bytes"]++
		case strings.HasSuffix(s, "_rq"):
			families["_rq"]++
		case strings.HasSuffix(s, "_decoder_error") && s != "decoder_error":
			families["_decoder_error"]++
		case strings.HasSuffix(s, "_resp_bytes"):
			families["_resp_bytes"]++
		case strings.HasSuffix(s, "_resp_fast"):
			families["_resp_fast"]++
		case strings.HasSuffix(s, "_resp_slow"):
			families["_resp_slow"]++
		case strings.HasSuffix(s, "_resp"):
			families["_resp"]++
		default:
			families["plain"]++
		}
	}
	want := map[string]int{"plain": 4, "_rq": 28, "_rq_bytes": 29, "_decoder_error": 28,
		"_resp": 28, "_resp_bytes": 28, "_resp_fast": 28, "_resp_slow": 28}
	for fam, n := range want {
		if families[fam] != n {
			t.Errorf("family %s has %d names, want %d", fam, families[fam], n)
		}
	}
	// Digit-suffix regression guard (reference_proto_roster_extraction_digits).
	set := map[string]bool{}
	for _, s := range suffixes {
		set[s] = true
	}
	for _, must := range []string{"create2_rq", "getchildren2_rq", "setwatches2_rq",
		"getallchildrennumber_rq", "connect_readonly_rq", "auth_rq_bytes", "auth_resp",
		"decoder_error", "request_bytes", "response_bytes", "watch_event"} {
		if !set[must] {
			t.Errorf("roster missing %q", must)
		}
	}
	// Asymmetries (live-dump confirmed): NO auth_rq; NO connect_readonly_resp.
	// NOTE: setauth_rq IS present in the live dump (setauth is a distinct opname
	// in the macro; "auth" opname is only for SetAuth's dynamic per-scheme counter
	// path via authSchemeCounter). connect_readonly has no resp-side counters.
	for _, mustNot := range []string{"auth_rq", "connect_readonly_resp"} {
		if set[mustNot] {
			t.Errorf("roster must NOT contain %q (asymmetry confirmed by live dump)", mustNot)
		}
	}
	// The full sorted golden list derived from the live dump of
	// envoyproxy/envoy:v1.37.2 admin /stats (probe date 2026-06-02) and sorted
	// with Go's sort.Strings (byte-lexicographic; '_'=95 < 'c'=99, so create_*
	// sorts before createcontainer_* — differs from POSIX locale shell sort).
	// Probe command: curl -s http://127.0.0.1:19901/stats | grep '^zkprobe\.zookeeper\.' |
	//   cut -d: -f1 | sed 's/^zkprobe\.zookeeper\.//' → 201 lines confirmed.
	golden := []string{
		"addwatch_decoder_error",
		"addwatch_resp",
		"addwatch_resp_bytes",
		"addwatch_resp_fast",
		"addwatch_resp_slow",
		"addwatch_rq",
		"addwatch_rq_bytes",
		"auth_decoder_error",
		"auth_resp",
		"auth_resp_bytes",
		"auth_resp_fast",
		"auth_resp_slow",
		"auth_rq_bytes",
		"check_decoder_error",
		"check_resp",
		"check_resp_bytes",
		"check_resp_fast",
		"check_resp_slow",
		"check_rq",
		"check_rq_bytes",
		"checkwatches_decoder_error",
		"checkwatches_resp",
		"checkwatches_resp_bytes",
		"checkwatches_resp_fast",
		"checkwatches_resp_slow",
		"checkwatches_rq",
		"checkwatches_rq_bytes",
		"close_decoder_error",
		"close_resp",
		"close_resp_bytes",
		"close_resp_fast",
		"close_resp_slow",
		"close_rq",
		"close_rq_bytes",
		"connect_decoder_error",
		"connect_readonly_rq",
		"connect_readonly_rq_bytes",
		"connect_resp",
		"connect_resp_bytes",
		"connect_resp_fast",
		"connect_resp_slow",
		"connect_rq",
		"connect_rq_bytes",
		"create2_decoder_error",
		"create2_resp",
		"create2_resp_bytes",
		"create2_resp_fast",
		"create2_resp_slow",
		"create2_rq",
		"create2_rq_bytes",
		"create_decoder_error",
		"create_resp",
		"create_resp_bytes",
		"create_resp_fast",
		"create_resp_slow",
		"create_rq",
		"create_rq_bytes",
		"createcontainer_decoder_error",
		"createcontainer_resp",
		"createcontainer_resp_bytes",
		"createcontainer_resp_fast",
		"createcontainer_resp_slow",
		"createcontainer_rq",
		"createcontainer_rq_bytes",
		"createttl_decoder_error",
		"createttl_resp",
		"createttl_resp_bytes",
		"createttl_resp_fast",
		"createttl_resp_slow",
		"createttl_rq",
		"createttl_rq_bytes",
		"decoder_error",
		"delete_decoder_error",
		"delete_resp",
		"delete_resp_bytes",
		"delete_resp_fast",
		"delete_resp_slow",
		"delete_rq",
		"delete_rq_bytes",
		"exists_decoder_error",
		"exists_resp",
		"exists_resp_bytes",
		"exists_resp_fast",
		"exists_resp_slow",
		"exists_rq",
		"exists_rq_bytes",
		"getacl_decoder_error",
		"getacl_resp",
		"getacl_resp_bytes",
		"getacl_resp_fast",
		"getacl_resp_slow",
		"getacl_rq",
		"getacl_rq_bytes",
		"getallchildrennumber_decoder_error",
		"getallchildrennumber_resp",
		"getallchildrennumber_resp_bytes",
		"getallchildrennumber_resp_fast",
		"getallchildrennumber_resp_slow",
		"getallchildrennumber_rq",
		"getallchildrennumber_rq_bytes",
		"getchildren2_decoder_error",
		"getchildren2_resp",
		"getchildren2_resp_bytes",
		"getchildren2_resp_fast",
		"getchildren2_resp_slow",
		"getchildren2_rq",
		"getchildren2_rq_bytes",
		"getchildren_decoder_error",
		"getchildren_resp",
		"getchildren_resp_bytes",
		"getchildren_resp_fast",
		"getchildren_resp_slow",
		"getchildren_rq",
		"getchildren_rq_bytes",
		"getdata_decoder_error",
		"getdata_resp",
		"getdata_resp_bytes",
		"getdata_resp_fast",
		"getdata_resp_slow",
		"getdata_rq",
		"getdata_rq_bytes",
		"getephemerals_decoder_error",
		"getephemerals_resp",
		"getephemerals_resp_bytes",
		"getephemerals_resp_fast",
		"getephemerals_resp_slow",
		"getephemerals_rq",
		"getephemerals_rq_bytes",
		"multi_decoder_error",
		"multi_resp",
		"multi_resp_bytes",
		"multi_resp_fast",
		"multi_resp_slow",
		"multi_rq",
		"multi_rq_bytes",
		"ping_decoder_error",
		"ping_resp",
		"ping_resp_bytes",
		"ping_resp_fast",
		"ping_resp_slow",
		"ping_rq",
		"ping_rq_bytes",
		"reconfig_decoder_error",
		"reconfig_resp",
		"reconfig_resp_bytes",
		"reconfig_resp_fast",
		"reconfig_resp_slow",
		"reconfig_rq",
		"reconfig_rq_bytes",
		"removewatches_decoder_error",
		"removewatches_resp",
		"removewatches_resp_bytes",
		"removewatches_resp_fast",
		"removewatches_resp_slow",
		"removewatches_rq",
		"removewatches_rq_bytes",
		"request_bytes",
		"response_bytes",
		"setacl_decoder_error",
		"setacl_resp",
		"setacl_resp_bytes",
		"setacl_resp_fast",
		"setacl_resp_slow",
		"setacl_rq",
		"setacl_rq_bytes",
		"setauth_decoder_error",
		"setauth_resp",
		"setauth_resp_bytes",
		"setauth_resp_fast",
		"setauth_resp_slow",
		"setauth_rq",
		"setauth_rq_bytes",
		"setdata_decoder_error",
		"setdata_resp",
		"setdata_resp_bytes",
		"setdata_resp_fast",
		"setdata_resp_slow",
		"setdata_rq",
		"setdata_rq_bytes",
		"setwatches2_decoder_error",
		"setwatches2_resp",
		"setwatches2_resp_bytes",
		"setwatches2_resp_fast",
		"setwatches2_resp_slow",
		"setwatches2_rq",
		"setwatches2_rq_bytes",
		"setwatches_decoder_error",
		"setwatches_resp",
		"setwatches_resp_bytes",
		"setwatches_resp_fast",
		"setwatches_resp_slow",
		"setwatches_rq",
		"setwatches_rq_bytes",
		"sync_decoder_error",
		"sync_resp",
		"sync_resp_bytes",
		"sync_resp_fast",
		"sync_resp_slow",
		"sync_rq",
		"sync_rq_bytes",
		"watch_event",
	}
	sorted := append([]string(nil), suffixes...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, golden) {
		t.Errorf("roster diverges from the golden list (len got %d / want %d); diff: %v",
			len(sorted), len(golden), firstDiff(sorted, golden))
	}
}

// TestRosterStats_EagerCreation validates eager creation (D-P5): newRosterStats
// creates exactly 201 counters in the registry, all at value 0.
func TestRosterStats_EagerCreation(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	if len(rs.counters) != 201 {
		t.Fatalf("rosterStats created %d counters, want 201", len(rs.counters))
	}
	// Spot-check internal names + zero values: response-side counters exist at 0.
	for _, suffix := range []string{"getdata_resp", "getdata_resp_fast", "watch_event", "response_bytes"} {
		c := rs.counters[suffix]
		if c == nil {
			t.Fatalf("counter %q not created", suffix)
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suffix, c.Load())
		}
	}
	// Also exercise add to prevent lint dead-code complaints and verify it works.
	rs.add("request_bytes", 42)
	if rs.counters["request_bytes"].Load() != 42 {
		t.Errorf("add(request_bytes, 42) did not stick")
	}
}

// TestRosterStats_IdempotentSharedPrefix validates that two listeners sharing a
// stat_prefix share counters, no panic (the rbac newFilterStats precedent).
func TestRosterStats_IdempotentSharedPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	a := newRosterStats(reg, "zk")
	b := newRosterStats(reg, "zk")
	if a.counters["getdata_rq"] != b.counters["getdata_rq"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

// TestRosterStats_DynamicAuthSchemeCounters validates dynamic per-scheme auth
// counters: a BUILTIN scheme ("digest") gets its own lazily-created counter;
// repeated calls return the same counter; a NON-builtin scheme takes the
// unknown_scheme fallback (upstream getBuiltin parity), and two distinct
// non-builtin schemes collapse onto the SAME unknown_scheme counter instance.
func TestRosterStats_DynamicAuthSchemeCounters(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	c1 := rs.authSchemeCounter("digest")
	c2 := rs.authSchemeCounter("digest")
	if c1 != c2 {
		t.Fatal("repeated authSchemeCounter(digest) must return the same counter")
	}
	c1.Inc()
	if c1.Load() != 1 {
		t.Fatal("auth counter increment lost")
	}
	// The dynamic name shape: <stat_prefix>.zookeeper.auth.<scheme>_rq
	if c1.Name() != "zk.zookeeper.auth.digest_rq" {
		t.Fatalf("builtin digest counter name = %q, want zk.zookeeper.auth.digest_rq", c1.Name())
	}

	// Non-builtin schemes fall back to the single unknown_scheme counter.
	unknown := rs.authSchemeCounter("kerberos")
	if unknown.Name() != "zk.zookeeper.auth.unknown_scheme_rq" {
		t.Fatalf("non-builtin scheme counter name = %q, want zk.zookeeper.auth.unknown_scheme_rq", unknown.Name())
	}
	if other := rs.authSchemeCounter("sasl"); other != unknown {
		t.Fatal("two distinct non-builtin schemes must collapse onto the SAME unknown_scheme counter")
	}
}

// TestRosterStats_UnknownSuffixPanics validates that inc panics on an unknown
// suffix (programming-error guard).
func TestRosterStats_UnknownSuffixPanics(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	defer func() {
		if recover() == nil {
			t.Fatal("inc(unknown) must panic")
		}
	}()
	rs.inc("not_a_counter")
}
