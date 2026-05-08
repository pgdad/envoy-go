// Backend for fixture 0014-http-csrf. Serves / with body "backend\n" (8 bytes).
// Mirrors test/fixtures/0011-http-fault/backends/backend.go + test/fixtures/0013-http-local-ratelimit/backends/backend.go exactly per phase 12 SPEC §7.4.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	port := flag.Int("port", 0, "listen port")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "8")
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, "backend\n")
	})
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("0014-http-csrf backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
