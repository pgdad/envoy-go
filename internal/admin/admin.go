package admin

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/listener"
	"github.com/esalaine/envoy-go/internal/stats"
)

// Server is the admin HTTP/1.1 server. Phase 01 introduced /ready; phase 06.1
// added /stats/prometheus; phase 08.1 adds /config_dump, /clusters, /listeners,
// /server_info (the four read-only operator-introspection endpoints from
// SPEC §1). The constructor widens to thread *bootstrap.Bootstrap (for
// /config_dump body + /server_info node + command_line_options.config_path),
// *cluster.Manager (for /clusters cluster snapshot), and *listener.Manager
// (for /listeners listener snapshot) — the third application of the LBP-1
// explicit-threading discipline (after 06.1's *stats.Registry and 07.1's
// *HTTPRegistry / 07.2's *ListenerFilterRegistry); ADR-0085 records the
// generalisation. 08.2 will add POST /drain_listeners and extend /ready +
// /server_info for the DRAINING state.
type Server struct {
	addr      string
	ln        net.Listener
	httpSrv   *http.Server
	ready     atomic.Bool
	registry  *stats.Registry
	liveGauge *stats.Gauge
	liveOnce  sync.Once
	// 08.1 fields (per ADR-0085 + planner-time decision 6)
	bs       *bootstrap.Bootstrap
	cm       *cluster.Manager
	lm       *listener.Manager
	bootTime time.Time
}

// New returns an admin server targeting addr. The server is not running yet;
// call Start. The /ready gate is initially closed (MarkReady flips it). The
// registry parameter is the boot-time Registry threaded by main.go; it MUST
// NOT be Frozen yet (admin allocates the server.live gauge at New time per
// SPEC §5.4 + §12 #3). The bs/cm/lm parameters are the bootstrap +
// cluster manager + listener manager threaded by main.go for the four new
// 08.1 admin endpoints (per ADR-0085); they may be nil in test code that
// does NOT exercise those endpoints. bootTime is set to time.Now() at call.
func New(addr string, registry *stats.Registry, bs *bootstrap.Bootstrap, cm *cluster.Manager, lm *listener.Manager) *Server {
	return &Server{
		addr:      addr,
		registry:  registry,
		liveGauge: registry.NewGauge("server.live"),
		bs:        bs,
		cm:        cm,
		lm:        lm,
		bootTime:  time.Now(),
	}
}

// Start binds and begins serving in a background goroutine. Returns the bound
// address (useful when addr had port 0). Error only if bind fails.
//
// Six routes are registered post-08.1: /ready (phase 01), /stats/prometheus
// (phase 06.1), /config_dump + /clusters + /listeners + /server_info
// (phase 08.1). WriteTimeout is widened from phase 01's 5s to 30s per
// planner-time decision 2 — /config_dump's protojson rendering of large
// bootstraps may approach the budget on slow scrape clients; 30s is generous
// enough for any reasonable fixture without weakening resilience.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/stats/prometheus", handlePrometheus(s.registry))
	mux.HandleFunc("/config_dump", s.handleConfigDump)
	mux.HandleFunc("/clusters", s.handleClusters)
	mux.HandleFunc("/listeners", s.handleListeners)
	mux.HandleFunc("/server_info", s.handleServerInfo)
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// MarkReady flips /ready into the ready state.
func (s *Server) MarkReady() { s.ready.Store(true) }

// Close performs best-effort shutdown. Idempotent. No graceful drain (phase 08).
func (s *Server) Close() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// handleReady implements the /ready contract per BEHAVIOR_CONTRACT.md §Admin
// API. Byte-exact to upstream Envoy v1.37.2 observations captured in
// docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md
// (ADR-0015 for the pre-init contract; ADR-0014 for the Server header).
//
// Framing deviation from upstream: phase 01 emits Content-Length instead of
// transfer-encoding: chunked. The differential harness (Task 14) dechunks
// upstream before byte-comparing the body; the deviation is codified in
// BEHAVIOR_CONTRACT.md §Admin API (Task 10).
//
// The first time this handler returns 200/LIVE it Set(1)s the server.live
// gauge through a sync.Once guard per SPEC §12 #3; subsequent /ready calls
// (and the pre-init 503 path) leave the gauge untouched.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=UTF-8")
	h.Set("Cache-Control", "no-cache, max-age=0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Server", "envoy")

	if !s.ready.Load() {
		body := []byte("PRE_INITIALIZING\n")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
		return
	}
	// LIVE-path: the first time this branch executes, Set(1) the
	// server.live gauge per SPEC §12 #3. Subsequent calls are no-ops.
	s.liveOnce.Do(func() { s.liveGauge.Set(1) })
	body := []byte("LIVE\n")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// handleServerInfo is a placeholder; real impl lands in Task 9
// (internal/admin/serverinfo.go).
func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	body := []byte("server_info: not yet implemented\n")
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}
