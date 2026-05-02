package admin

import "net/http"

// writeAdminHeaders writes the four constant headers every 08.1 admin
// endpoint emits per SPEC §11.6 + ADR-0014: Content-Type (varies per
// endpoint; supplied by the caller), Cache-Control: no-cache, max-age=0,
// X-Content-Type-Options: nosniff, Server: envoy.
//
// Date and Content-Length are NOT set here — net/http auto-adds them at
// write time (per RFC 9110 §6.6.1 for Date; auto-computed Content-Length
// when WriteHeader precedes a single Write of bounded body). This matches
// the SPEC §12 #5 + planner-time decision 5 stance: leave it to net/http.
//
// /ready (phase 01) and /stats/prometheus (phase 06.1) set these headers
// inline rather than via this helper for historical reasons; the four 08.1
// endpoints all route through this helper for consistency.
func writeAdminHeaders(w http.ResponseWriter, contentType string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "no-cache, max-age=0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Server", "envoy")
}
