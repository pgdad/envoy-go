// Package stats — bool→int64 conversion helper per ADR-0059 §Decision
// AMENDMENT (phase-21). Cross-phase reusable for any future bool-typed
// gauge consumer.
package stats

// BoolToInt returns 1 if b is true, 0 otherwise. Used by callers that
// encode a bool as an int64-valued *Gauge (e.g., adaptive_concurrency's
// min_rtt_calculation_active gauge per phase-21 SPEC §6.6 + AMEND-3 +
// ADR-0059 §Decision AMENDMENT). See gauge.go's doc-extension paragraph
// for the broader encoding convention (ns for time-typed; ×1000 for
// ratio-typed; 0/1 for bool-typed via this helper).
func BoolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
