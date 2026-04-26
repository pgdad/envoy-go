// Backend for fixture 0004-h2-routing.
//
// Listens on a TLS port advertising NextProtos=["h2"] via http2.ConfigureServer
// (driver-side; D-3.2 governs envoy-go runtime, not test backends — the test
// tree may import http2.Server freely).
//
// Routes:
//   - /health        -> 200 "OK\n"
//   - /api/v1/<tail> -> 200 "backend-<idx>:v1/<tail>"
//   - any other path -> 404 "not found\n"
//
// BACKEND_IDX env var supplies the per-instance numeric idx that flows into the
// /api/v1 response body (used by the driver's distribution + body-equivalence
// assertions).
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http2"
)

func main() {
	port := flag.String("port", "0", "listen port (numeric)")
	cert := flag.String("cert", "", "server cert PEM path")
	key := flag.String("key", "", "server key PEM path")
	flag.Parse()
	idx := os.Getenv("BACKEND_IDX")
	if idx == "" {
		idx = "?"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "OK\n")
	})
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		_, _ = fmt.Fprintf(w, "backend-%s:v1/%s", idx, suffix)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found\n"))
	})

	tlsCert, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		log.Fatalf("load cert: %v", err)
	}
	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{"h2"},
		},
	}
	if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
		log.Fatalf("ConfigureServer: %v", err)
	}
	log.Printf("backend %s listening on %s", idx, srv.Addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServeTLS: %v", err)
	}
}
