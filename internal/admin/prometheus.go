package admin

import (
	"log"
	"net/http"

	"github.com/esalaine/envoy-go/internal/stats"
)

// handlePrometheus returns an HTTP handler that serves the
// Prometheus text-format exposition by walking the given Registry.
//
// Per SPEC §11.2 + ADR-0059: the response Content-Type is
// "text/plain; version=0.0.4; charset=utf-8" matching the Prometheus
// exposition spec; the body is the alphabetically-sorted-by-Prom-name
// exposition; HELP-text is the static map keyed by Prometheus name;
// errors from stats.WriteProm are logged and otherwise ignored (per
// BRAINSTORM §5.3: too late to retry — headers already sent).
func handlePrometheus(r *stats.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := stats.WriteProm(w, r); err != nil {
			log.Printf("admin: /stats/prometheus: write: %v", err)
		}
	}
}
