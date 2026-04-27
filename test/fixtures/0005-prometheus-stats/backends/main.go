// Command backends is the controlled HTTP/1.1 backend for fixture
// 0005-prometheus-stats. It starts a server on a configurable port; each
// request reads the X-Backend-Status header to determine the response status
// code. If the header is absent or invalid the backend defaults to 200.
//
// Body:
//   - 200: "OK\n"
//   - 502: "bad gateway\n"
//   - other: "<status> <text>\n" (e.g. "404 Not Found\n")
//
// Usage:
//
//	backends --port <port>
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
)

func main() {
	port := flag.Int("port", 0, "TCP port to listen on (0 = OS-chosen)")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	log.Printf("backend listening on :%d", addr.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequest)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	status := 200
	if hdr := r.Header.Get("X-Backend-Status"); hdr != "" {
		if n, err := strconv.Atoi(hdr); err == nil && n >= 100 && n <= 599 {
			status = n
		}
	}

	// Connection: close forces Envoy to retire the upstream connection after each
	// response, ensuring upstream_cx_active gauges return to 0 after a load burst.
	// Without this, Envoy's keepalive upstream pool keeps connections alive
	// indefinitely, making the gauge snapshot non-zero and non-equal to envoy-go's
	// (which closes connections per ADR-0056).
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)

	switch status {
	case 200:
		_, _ = fmt.Fprint(w, "OK\n")
	case 502:
		_, _ = fmt.Fprint(w, "bad gateway\n")
	default:
		_, _ = fmt.Fprintf(w, "%d %s\n", status, http.StatusText(status))
	}
}
