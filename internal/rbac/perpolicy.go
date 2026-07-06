package rbac

import (
	"sync"

	"github.com/pgdad/envoy-go/internal/stats"
)

// PerPolicyCounters is the engine-side per-policy lazy-allocation cache
// (moved from the phase-16 HTTP filterStats.incPolicy per D-26.3-7). The
// sync.Map LoadOrStore + NewCounterIfAbsent first-emission path is race-safe.
// The four NAMED base counters + the trackPerRuleStats gate stay per-consumer;
// consumer #2 (rbac_network) never constructs this (F2 — no track_per_rule_stats).
type PerPolicyCounters struct {
	m sync.Map // map[string]*stats.Counter keyed by the full counter name
}

// Inc lazy-allocates + increments <base>.policy.<policyName>.<suffix>. The
// inserted ".policy." segment is the empirically-RATIFIED Envoy v1.37.2 shape
// (phase-16 ADR-0145). No-op on nil reg or empty policyName.
func (s *PerPolicyCounters) Inc(reg *stats.Registry, base, policyName, suffix string) {
	if s == nil || reg == nil || policyName == "" {
		return
	}
	key := base + ".policy." + policyName + "." + suffix
	if cached, ok := s.m.Load(key); ok {
		cached.(*stats.Counter).Inc()
		return
	}
	c := reg.NewCounterIfAbsent(key)
	actual, _ := s.m.LoadOrStore(key, c)
	actual.(*stats.Counter).Inc()
}
