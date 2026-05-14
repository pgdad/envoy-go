package jwksbackend

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is a minimal in-process HTTP JWKS server. Goroutine-safe.
//
// Per SPEC §7.4: serves configurable JWK Set JSON at known URIs. The accepted
// shape is path → raw JSON body (the caller pre-encodes the JWK Set so the
// helper has no JWKS-shape coupling to internal/jwks). The handler writes the
// body verbatim with Content-Type: application/json.
type Server struct {
	listener net.Listener
	srv      *http.Server
	addr     string

	mu     sync.Mutex
	closed bool
}

// New binds a TCP listener on addr and starts an HTTP server serving the
// supplied routes map. addr may be "127.0.0.1:0" to request an ephemeral port
// (the caller reads (*Server).Addr() afterward to retrieve the bound address).
//
// routes maps URL path → JWK Set JSON body. GET on a configured path serves
// the corresponding JSON body with Content-Type: application/json; non-GET
// methods return 405; missing paths return 404.
//
// The supplied ctx is plumbed through to the *http.Server.BaseContext so
// per-connection contexts inherit cancellation; callers MUST invoke Stop() at
// teardown to release the listener (canceling ctx alone does NOT stop the
// server — http.Server.Serve is unaware of BaseContext cancellation).
func New(ctx context.Context, addr string, routes map[string]string) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("jwksbackend: listen %q: %w", addr, err)
	}

	mux := http.NewServeMux()
	for path, body := range routes {
		body := body // capture
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	s := &Server{
		listener: listener,
		srv:      srv,
		addr:     listener.Addr().String(),
	}

	go func() {
		// http.Server.Serve returns http.ErrServerClosed once Shutdown
		// completes; any earlier error (e.g., listener close from a panic
		// path) is discarded — the caller observes lifecycle via Stop()'s
		// return value.
		_ = s.srv.Serve(listener)
	}()

	return s, nil
}

// Addr returns the listener's actual bound address. Load-bearing when the
// caller supplied "127.0.0.1:0" to New() — the runner reads this back to
// templatize the cluster c_jwks_backend port into envoy.yaml + envoy-go.yaml.
func (s *Server) Addr() string {
	return s.addr
}

// Stop terminates the server cleanly. Idempotent: subsequent calls are no-ops.
// Returns the first Shutdown error if Shutdown's 5-second deadline fires
// before in-flight requests drain; nil otherwise.
func (s *Server) Stop() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}
