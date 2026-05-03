package admin

import (
	"net/http"
)

// handleDrainListeners implements the POST /drain_listeners contract per
// SPEC §6.3 + §11.1 (POST) + §11.4 (method discrimination):
//
//   - POST: triggers s.dm.Drain() (sync.Once-guarded internally; idempotent
//     across multiple POSTs); emits 200 OK with body "OK\n" + the standard
//     six-header set per §11.6 via writeAdminHeaders. Fire-and-forget — does
//     NOT block on <-s.dm.Done().
//   - GET / PUT / DELETE / HEAD / others: emits 405 Method Not Allowed with
//     body "Method <METHOD> not allowed, POST required.\n" per §11.4
//     empirical pin verbatim. The 405 is a hard rejection — DOES NOT trigger
//     drain.
//
// The ?graceful=true query-param is silently accepted per ADR-0041's silent-
// ignore precedent (envoy-go's drain is always graceful by construction).
//
// Per SPEC §6.3, the endpoint does NOT trigger process exit — the operator-
// driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT (per
// §5.3 lifecycle).
//
// Per planner-time-resolved nil-dm policy: a nil s.dm yields 500 Internal
// Server Error (defensive — the operator gets a clear signal that the drain
// machinery is not wired). Production builds always thread a non-nil dm.
//
// ADR-0093 records the design (partially amends ADR-0090's no-method-
// discrimination posture).
func (s *Server) handleDrainListeners(w http.ResponseWriter, r *http.Request) {
	writeAdminHeaders(w, "text/plain; charset=UTF-8")

	// Method discrimination (per §11.4). Non-POST returns 405 with the
	// templated body. The 405 is a hard rejection — no drain side effect.
	if r.Method != http.MethodPost {
		body := []byte("Method " + r.Method + " not allowed, POST required.\n")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write(body)
		return
	}

	// Defensive 500 on nil dm (per PLAN planner-time-resolved nil-dm policy).
	if s.dm == nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("drain manager not configured\n"))
		return
	}

	// POST path: trigger drain (sync.Once-guarded inside drain.Manager;
	// idempotent across multiple POSTs); emit 200 OK with body "OK\n" per
	// §11.1 empirical pin verbatim.
	s.dm.Drain()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}
