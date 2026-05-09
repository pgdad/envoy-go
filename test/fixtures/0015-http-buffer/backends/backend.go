// Backend for fixture 0015-http-buffer. Echoes inbound request method + path + headers as JSON.
// Mirrors the python BaseHTTPRequestHandler echo at SPEC §11.5 empirical pin verbatim, in Go.
// Load-bearing for fixture scenario 6's Content-Length: 10240 assertion at the backend boundary
// per the maybeAddContentLength mirror per phase 13 SPEC §11.8-CL.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	port := flag.Int("port", 0, "listen port")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Read body to consume it (drains chunked + CL bodies; no echo of body bytes — only headers matter for fixture).
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			if n == 0 || err != nil {
				break
			}
		}
		// Lowercase canonical header keys per Envoy wire-form discipline.
		hdrs := make(map[string]string, len(r.Header))
		for k, vs := range r.Header {
			if len(vs) > 0 {
				hdrs[strings.ToLower(k)] = vs[0]
			}
		}
		resp := map[string]any{
			"method":  r.Method,
			"path":    r.URL.Path,
			"headers": hdrs,
		}
		body, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("0015-http-buffer backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
