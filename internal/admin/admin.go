package admin

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// Server is the admin HTTP/1.1 server. /ready and /stats/prometheus are
// implemented in phase 06.1; other admin endpoints land in phase 08.
type Server struct {
	addr      string
	ln        net.Listener
	httpSrv   *http.Server
	ready     atomic.Bool
	registry  *stats.Registry
	liveGauge *stats.Gauge
	liveOnce  sync.Once
}

// New returns an admin server targeting addr. The server is not running yet;
// call Start. The /ready gate is initially closed (MarkReady flips it). The
// registry parameter is the boot-time Registry threaded by main.go; it MUST
// NOT be Frozen yet (admin allocates the server.live gauge at New time per
// SPEC §5.4 + §12 #3).
func New(addr string, registry *stats.Registry) *Server {
	return &Server{
		addr:      addr,
		registry:  registry,
		liveGauge: registry.NewGauge("server.live"),
	}
}

// Start binds and begins serving in a background goroutine. Returns the bound
// address (useful when addr had port 0). Error only if bind fails.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/stats/prometheus", handlePrometheus(s.registry))
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
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
