package ratelimit

// stats_test.go — byte-exact stat-name guards + cluster-scoped naming +
// NewCounterIfAbsent idempotency for the global rate-limit filter's
// 4-counter stat surface per phase-24 SPEC §6.8 + AMEND-1 + AMEND-10 +
// phase-24.1 PLAN Task 4.
//
// Two-layer guard strategy (per stats.go doc-comment):
//  1. The const declarations in stats.go pin byte-exact leaf names at build time.
//  2. The TestStatNames_ByteStable test here pins each const to its expected
//     literal so a future refactor cannot silently rename const+literal together.
//
// Additionally, TestFilterStats_ClusterScopedNames verifies the constructor
// wires each counter under the AMEND-1 cluster-scoped prefix template
// `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<leaf>`, and
// TestFilterStats_NewCounterIfAbsent_Idempotent verifies the AMEND-10
// idempotent registration discipline (two newFilterStats invocations against
// the same registry+cluster return functionally-equivalent counter handles —
// safe across multiple listeners that mount this filter onto the SAME RLS
// cluster).

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Byte-exact stat-name constant guards (SPEC §6.8 + AMEND-1).
// Task 4 OWNS these — do NOT duplicate in later Tasks.
// -----------------------------------------------------------------------------

// TestStatNames_ByteStable pins each of the 4 leaf-name consts to the
// upstream wire name per AMEND-1 (stat_names.h:15-18,30-33 in the upstream
// common ratelimit lib). A rename of either side (const or literal) fails
// this test, which is the second layer of the two-layer guard described in
// the stats.go doc-comment.
func TestStatNames_ByteStable(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{statNameOK, "ok"},
		{statNameError, "error"},
		{statNameOverLimit, "over_limit"},
		{statNameFailureModeAllowed, "failure_mode_allowed"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("stat leaf name = %q; want %q", tc.got, tc.want)
		}
	}
}

// TestStatNames_Count verifies there are exactly 4 leaf-name consts and that
// all four are distinct — the COUNTER-only roster per AMEND-1 (no gauges, no
// histograms; project stat count 110 -> 114). Distinctness catches a real bug:
// two consts accidentally set to the same string.
func TestStatNames_Count(t *testing.T) {
	names := []string{statNameOK, statNameError, statNameOverLimit, statNameFailureModeAllowed}
	const wantCount = 4
	if len(names) != wantCount {
		t.Errorf("stat name count = %d; want exactly %d (COUNTER-only per AMEND-1)", len(names), wantCount)
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate stat name %q -- each const must be distinct", n)
		}
		seen[n] = true
	}
}

// -----------------------------------------------------------------------------
// Cluster-scoped naming tests (AMEND-1 + AMEND-10 cross-namespace surface).
// -----------------------------------------------------------------------------

// TestFilterStats_ClusterScopedNames verifies the full dotted counter names
// per the AMEND-1 prefix template:
//
//	cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<leaf>
//
// Two sub-rows: empty statPrefix (the default — names elide the prefix
// segment) and non-empty statPrefix (names interleave the prefix segment).
func TestFilterStats_ClusterScopedNames(t *testing.T) {
	cases := []struct {
		name       string
		cluster    string
		statPrefix string
		wantOK     string
		wantErr    string
		wantOver   string
		wantFMA    string
	}{
		{
			name:       "EmptyStatPrefix",
			cluster:    "rls",
			statPrefix: "",
			wantOK:     "cluster.rls.ratelimit.ok",
			wantErr:    "cluster.rls.ratelimit.error",
			wantOver:   "cluster.rls.ratelimit.over_limit",
			wantFMA:    "cluster.rls.ratelimit.failure_mode_allowed",
		},
		{
			name:       "NonEmptyStatPrefix",
			cluster:    "rls",
			statPrefix: "foo",
			wantOK:     "cluster.rls.ratelimit.foo.ok",
			wantErr:    "cluster.rls.ratelimit.foo.error",
			wantOver:   "cluster.rls.ratelimit.foo.over_limit",
			wantFMA:    "cluster.rls.ratelimit.foo.failure_mode_allowed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := stats.NewRegistry()
			fs := newFilterStats(reg, tc.cluster, tc.statPrefix)

			if fs == nil {
				t.Fatal("newFilterStats returned nil with non-nil registry")
			}
			if fs.ok == nil || fs.error == nil || fs.overLimit == nil || fs.failureModeAllowed == nil {
				t.Fatalf("counter(s) nil: ok=%v error=%v overLimit=%v failureModeAllowed=%v",
					fs.ok, fs.error, fs.overLimit, fs.failureModeAllowed)
			}

			type nameCase struct {
				got  string
				want string
			}
			for _, nc := range []nameCase{
				{fs.ok.Name(), tc.wantOK},
				{fs.error.Name(), tc.wantErr},
				{fs.overLimit.Name(), tc.wantOver},
				{fs.failureModeAllowed.Name(), tc.wantFMA},
			} {
				if nc.got != nc.want {
					t.Errorf("counter name = %q; want %q", nc.got, nc.want)
				}
			}
		})
	}
}

// TestFilterStats_NewCounterIfAbsent_Idempotent verifies the AMEND-10
// idempotent registration discipline: two newFilterStats invocations against
// the same registry + clusterName + statPrefix return the SAME underlying
// counter handles. This is load-bearing for the multi-listener case where
// >=2 HCMs each mount a ratelimit filter pointing at the SAME RLS cluster —
// both filters must charge into the SAME 4 counters (else operator-facing
// dashboards would see split readings).
//
// The proof: increment a counter via the first handle; the second handle's
// Load() must reflect the increment (same atomic-Uint64 cell). Pointer
// equality also confirmed (Registry.NewCounterIfAbsent re-returns the same
// *Counter on duplicate-name resolution per registry.go:161-164).
func TestFilterStats_NewCounterIfAbsent_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()
	const cluster = "rls"
	const statPrefix = "" // exercise the empty-prefix default path

	fs1 := newFilterStats(reg, cluster, statPrefix)
	fs2 := newFilterStats(reg, cluster, statPrefix)
	if fs1 == nil || fs2 == nil {
		t.Fatalf("newFilterStats returned nil: fs1=%v fs2=%v", fs1, fs2)
	}

	// Pointer-equality proof: NewCounterIfAbsent re-returns the same *Counter
	// for the same name (registry.go:161-164).
	pairs := []struct {
		name string
		a, b *stats.Counter
	}{
		{"ok", fs1.ok, fs2.ok},
		{"error", fs1.error, fs2.error},
		{"over_limit", fs1.overLimit, fs2.overLimit},
		{"failure_mode_allowed", fs1.failureModeAllowed, fs2.failureModeAllowed},
	}
	for _, p := range pairs {
		if p.a != p.b {
			t.Errorf("counter %q: fs1 ptr %p != fs2 ptr %p (must be same handle)",
				p.name, p.a, p.b)
		}
	}

	// Behavioral proof: Inc via fs1 visible through fs2 (same atomic cell).
	fs1.ok.Inc()
	if got := fs2.ok.Load(); got != 1 {
		t.Errorf("fs2.ok.Load() after fs1.ok.Inc() = %d; want 1 (idempotent registration broken)", got)
	}
	fs2.overLimit.Inc()
	fs2.overLimit.Inc()
	if got := fs1.overLimit.Load(); got != 2 {
		t.Errorf("fs1.overLimit.Load() after 2x fs2.overLimit.Inc() = %d; want 2", got)
	}
}

// TestFilterStats_NilRegistry verifies the ADR-0085 nil-tolerance contract:
// newFilterStats(nil, ...) returns a non-nil *filterStats with nil counter
// fields (so the Task-7 disposition path can safely call s.ok.Inc() under
// a per-counter nil-guard, mirroring the fault filter precedent at
// internal/filter/http/fault/fault.go:234). Test code paths that do not
// allocate a Registry must not panic on construction.
func TestFilterStats_NilRegistry(t *testing.T) {
	fs := newFilterStats(nil, "rls", "")
	if fs == nil {
		t.Fatal("newFilterStats(nil, ...) returned nil; want non-nil with all-nil counter fields")
	}
	if fs.ok != nil || fs.error != nil || fs.overLimit != nil || fs.failureModeAllowed != nil {
		t.Errorf("expected all-nil counter fields with nil registry; got ok=%v error=%v overLimit=%v failureModeAllowed=%v",
			fs.ok, fs.error, fs.overLimit, fs.failureModeAllowed)
	}
}
