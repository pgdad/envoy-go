// Package main is the slow-streaming Go HTTP backend for fixture
// 0010-graceful-drain. /slow streams 5KB at 1KB/s (5s total in-flight
// window); / is a fast sanity baseline. Per SPEC §7.5.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"time"
)

func main() {
	port := flag.Int("port", 18001, "TCP port to listen on")
	flag.Parse()

	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		// Set Content-Length explicitly so envoy-go's upstream ReadAll can
		// pass a proper Content-Length to the downstream client. Without this,
		// the Go HTTP server uses chunked encoding; envoy-go dechunks internally
		// but writes the raw body without proper framing to the downstream,
		// causing the downstream HTTP client to hang waiting for connection close.
		w.Header().Set("Content-Length", "5120")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 5; i++ {
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, 1024))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1 * time.Second)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend1\n"))
	})
	_ = http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
