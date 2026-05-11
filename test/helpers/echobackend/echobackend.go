package echobackend

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// New returns a configured *http.Server that, for any inbound request, writes
// a JSON body containing {"method": "...", "path": "...", "headers": {"k": "v"}}.
// Headers are echoed with lowercased canonical keys per Envoy wire-form
// discipline (ADR-0072 + phase-04 lowercase-header pattern). Multi-valued
// headers are comma-joined per RFC 7230 §3.2.2. Status 200;
// Content-Type: application/json.
//
// The caller binds via srv.Serve(listener); the listener allocation is the
// caller's responsibility (the runner allocates the port + binds upfront, or
// the cmdline wrapper uses Listen below).
func New() *http.Server {
	return &http.Server{
		Handler: http.HandlerFunc(handle),
	}
}

func handle(w http.ResponseWriter, req *http.Request) {
	body := buildEcho(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// echoRecord is the JSON shape echoed in the response body.
type echoRecord struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

func buildEcho(req *http.Request) []byte {
	rec := echoRecord{
		Method:  req.Method,
		Path:    req.URL.Path,
		Headers: map[string]string{},
	}
	for k, vs := range req.Header {
		// Lowercase canonical key per ADR-0072 + phase-04 lowercase-header pattern.
		// Multi-valued headers are comma-joined per RFC 7230 §3.2.2.
		rec.Headers[strings.ToLower(k)] = strings.Join(vs, ", ")
	}
	// Always include Host (req.Host is the Host header per net/http convention;
	// the Go server strips Host from req.Header).
	if req.Host != "" {
		rec.Headers["host"] = req.Host
	}
	bytes, _ := json.Marshal(rec)
	return bytes
}

// Listen returns a TCP listener bound to the requested port. Helper for the
// cmdline wrapper at cmd/echobackend/main.go.
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
}
