package rbac

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// findCounter scans the Registry for a counter with the given name.
// Returns nil if not present.
func findCounter(reg *stats.Registry, name string) *stats.Counter {
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	return found
}

func TestPerPolicyCounters_IncLazyAllocatesPolicySegment(t *testing.T) {
	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed")
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed") // idempotent registration, 2 increments
	got := findCounter(reg, "http.hcm.rbac.p.policy.policy_a.allowed")
	if got == nil || got.Load() != 2 {
		t.Fatalf("per-policy counter name/value wrong: %v", got)
	}
}

func TestPerPolicyCounters_NilRegOrEmptyPolicyIsNoOp(t *testing.T) {
	pc := &PerPolicyCounters{}
	pc.Inc(nil, "b", "p", "allowed")                // nil reg → no-op (no panic)
	pc.Inc(stats.NewRegistry(), "b", "", "allowed") // empty policy name → no-op
}
