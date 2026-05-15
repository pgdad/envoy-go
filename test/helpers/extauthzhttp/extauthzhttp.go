package extauthzhttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Script configures the per-request response behavior of the auth server.
// Use FixedScript, RouteScript, or InspectScript to construct one.
type Script interface {
	handle(w http.ResponseWriter, r *http.Request)
}

// PathMethod is a composite key for per-path, per-method routing in RouteScript.
type PathMethod struct {
	Path   string
	Method string
}

// ScriptEntry holds the scripted response for one route in RouteScript.
type ScriptEntry struct {
	Status  int
	Body    []byte
	Headers map[string]string
}

// fixedScript returns the same status, body, and headers for every request.
type fixedScript struct {
	status  int
	body    []byte
	headers map[string]string
}

// FixedScript builds a Script that returns a fixed status, body, and headers
// for every inbound request regardless of path or method. A nil body produces
// an empty response body; a nil headers map produces no custom response headers.
func FixedScript(status int, body []byte, headers map[string]string) Script {
	return &fixedScript{status: status, body: body, headers: headers}
}

func (s *fixedScript) handle(w http.ResponseWriter, _ *http.Request) {
	writeResponse(w, s.status, s.body, s.headers)
}

// routeScript dispatches per-path+method; falls through to defaults for
// unknown routes.
type routeScript struct {
	routes         map[PathMethod]ScriptEntry
	defaultStatus  int
	defaultBody    []byte
	defaultHeaders map[string]string
}

// RouteScript builds a Script that dispatches to per-path+method entries.
// Requests not matching any route in routes receive defaultStatus/defaultBody/
// defaultHeaders.
func RouteScript(routes map[PathMethod]ScriptEntry, defaultStatus int, defaultBody []byte, defaultHeaders map[string]string) Script {
	return &routeScript{
		routes:         routes,
		defaultStatus:  defaultStatus,
		defaultBody:    defaultBody,
		defaultHeaders: defaultHeaders,
	}
}

func (s *routeScript) handle(w http.ResponseWriter, r *http.Request) {
	key := PathMethod{Path: r.URL.Path, Method: r.Method}
	if entry, ok := s.routes[key]; ok {
		writeResponse(w, entry.Status, entry.Body, entry.Headers)
		return
	}
	writeResponse(w, s.defaultStatus, s.defaultBody, s.defaultHeaders)
}

// inspectScript calls a user-supplied function with method, path, and body
// to determine the response.
type inspectScript struct {
	fn func(method, path string, body []byte) (status int, respBody []byte, headers map[string]string)
}

// InspectScript builds a Script that invokes fn for every request, passing
// the request method, path, and fully-buffered body. fn returns the status,
// response body, and response headers. Used for scenario 5 (with_request_body)
// where the auth service must inspect the buffered request body.
func InspectScript(fn func(method, path string, body []byte) (int, []byte, map[string]string)) Script {
	return &inspectScript{fn: fn}
}

func (s *inspectScript) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	status, respBody, headers := s.fn(r.Method, r.URL.Path, body)
	writeResponse(w, status, respBody, headers)
}

// writeResponse writes status, body, and headers to w. If status <= 0 it
// defaults to 200.
func writeResponse(w http.ResponseWriter, status int, body []byte, headers map[string]string) {
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	if status <= 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// Server is a minimal in-process scriptable HTTP authorization server.
// Goroutine-safe. Created via New; terminated via Stop.
type Server struct {
	Listener net.Listener // exported per PLAN File structure table

	srv  *http.Server
	addr string

	mu     sync.Mutex
	closed bool
}

// New binds a TCP listener on addr and starts an HTTP server that evaluates
// script for every inbound request. addr may be "127.0.0.1:0" to request an
// ephemeral port; the caller reads (*Server).Addr() afterward to retrieve the
// actual bound address.
//
// The supplied ctx is plumbed into the *http.Server.BaseContext so per-connection
// contexts inherit cancellation. Callers MUST invoke Stop() at teardown to
// release the listener (canceling ctx alone does NOT stop the server).
func New(ctx context.Context, addr string, script Script) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("extauthzhttp: listen %q: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		script.handle(w, r)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	s := &Server{
		Listener: ln,
		srv:      srv,
		addr:     ln.Addr().String(),
	}

	go func() {
		// http.Server.Serve returns http.ErrServerClosed once Shutdown
		// completes; any earlier error is discarded — the caller observes
		// lifecycle via Stop()'s return value.
		_ = s.srv.Serve(ln)
	}()

	return s, nil
}

// Addr returns the listener's actual bound address. Load-bearing when the
// caller supplied "127.0.0.1:0" — the runner reads this back to templatize
// the http_service.server_uri port into envoy.yaml + envoy-go.yaml.
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
