// Backend for fixture 0013-http-local-ratelimit. Serves / with body "backend\n" (8 bytes).
// Mirrors test/fixtures/0011-http-fault/backends/backend.go exactly per phase 11 SPEC §7.4.
package main

import (
	"flag"
	"fmt"
	"net/http"
)

func main() {
	port := flag.Int("port", 18013, "TCP port to bind")
	flag.Parse()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		body := "backend\n"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		panic(err)
	}
}
