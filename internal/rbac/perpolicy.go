package rbac

import (
	"log"
	"sync"

	"github.com/pgdad/envoy-go/internal/stats"
)

// logSkipInvalidPolicyNameFmt is the aggregated skip diagnostic emitted by the
// phase-81 source-F2 charset backstop in Inc. Byte-pinned by
// perpolicy_test.go; do not change the wording.
const logSkipInvalidPolicyNameFmt = "rbac: per-policy stat skipped: policy name %q cannot form a valid metric name"

// PerPolicyCounters is the engine-side per-policy lazy-allocation cache
// (moved from the phase-16 HTTP filterStats.incPolicy per D-26.3-7). The
// sync.Map LoadOrStore + NewCounterIfAbsent first-emission path is race-safe.
// The four NAMED base counters + the trackPerRuleStats gate stay per-consumer;
// consumer #2 (rbac_network) never constructs this (F2 — no track_per_rule_stats).
type PerPolicyCounters struct {
	m sync.Map // map[string]*stats.Counter keyed by the full counter name

	// logged deduplicates the phase-81 source-F2 skip diagnostic. Keyed by the
	// SAME assembled counter name as m, so an un-nameable policy produces ONE
	// log line per emitting call site (primary base + suffix family, shadow
	// base + suffix family, per-route bases) and NOT one line per request.
	logged sync.Map // map[string]struct{}
}

// Inc lazy-allocates + increments <base>.policy.<policyName>.<suffix>. The
// inserted ".policy." segment is the empirically-RATIFIED Envoy v1.37.2 shape
// (phase-16 ADR-0145). No-op on nil reg or empty policyName.
//
// Phase-81 source F2: also a no-op (skip + one aggregated log line) when the
// ASSEMBLED key would fail the Registry's name charset. This is a REQUEST-TIME
// backstop, not a duplicate of the boot-time F1 guard in the HTTP consumer:
// the MATCHER engine has no boot-time policy-name enumeration at all
// (BuildMatcherEngine only allow-lists the terminal Any TypeURL; Evaluate
// returns action.GetName() read out of the match tree at request time), so a
// matcher-supplied Action.name reaches this method having passed through no
// charset check whatsoever. Without the skip, NewCounterIfAbsent ->
// stats.checkName PANICS on the HCM dispatch goroutine, which carries no
// recover(), and the process dies.
func (s *PerPolicyCounters) Inc(reg *stats.Registry, base, policyName, suffix string) {
	if s == nil || reg == nil || policyName == "" {
		return
	}
	key := base + ".policy." + policyName + "." + suffix
	if cached, ok := s.m.Load(key); ok {
		cached.(*stats.Counter).Inc()
		return
	}
	if !stats.IsValidName(key) {
		// One line per distinct un-nameable key (i.e. per emitting call site),
		// never one per request: a hot deny path would otherwise flood the log
		// at request rate. LoadOrStore is the dedupe barrier and is race-safe.
		if _, dup := s.logged.LoadOrStore(key, struct{}{}); !dup {
			log.Printf(logSkipInvalidPolicyNameFmt, policyName)
		}
		return
	}
	c := reg.NewCounterIfAbsent(key)
	actual, _ := s.m.LoadOrStore(key, c)
	actual.(*stats.Counter).Inc()
}
