package admin

import (
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/pgdad/envoy-go/internal/stats"
)

// handleStats returns an HTTP handler that serves the FLAT internal-name stat
// text used by the 0055-redis-roundtrip StatsAsserter (phase 32.1 / D-S32.1-5).
//
// Format: one "name: value\n" line per registered metric, sorted alphabetically
// by internal name. This is the canonical envoy-go flat-stats surface (parallel
// to reference Envoy's /stats endpoint) used by the 32.1 cross-side
// StatsAsserter to compare redis.<sp>.* + cluster.<name>.* counters by internal
// name — bypassing the /stats/prometheus path which silently skips unrecognized
// top-level segments (the redis. Prometheus tag-extractor arm is 32.2). The
// /stats/prometheus path remains unchanged and is the primary parity surface for
// all stats that have a flattenToProm arm in internal/stats/name.go.
//
// Phase 32.1 introduction: admin.Start registers this handler at "/stats".
// "/stats" and "/stats/prometheus" are both exact-path patterns (no trailing
// slash), so Go's net/http ServeMux matches each by exact path; registration
// order is immaterial (the longest-prefix/subtree rule applies only to "/"-
// terminated patterns, which neither is).
func handleStats(r *stats.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		writeAdminHeaders(w, "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		writeFlat(w, r)
	}
}

// writeFlat walks r and emits one "name: value\n" line per metric, sorted
// alphabetically by internal name. Write errors are silently ignored (headers
// already sent; same rationale as WriteProm in prom.go).
func writeFlat(w io.Writer, r *stats.Registry) {
	type entry struct {
		name  string
		value string
	}
	var entries []entry
	r.Walk(func(m stats.Metric) {
		entries = append(entries, entry{name: m.Name(), value: m.Format()})
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s: %s\n", e.name, e.value)
	}
}
