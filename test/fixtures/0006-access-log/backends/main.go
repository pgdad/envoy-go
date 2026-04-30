// Command backends is the controlled HTTP/1.1 backend for fixture
// 0006-access-log. It starts a server on a configurable port; each request
// returns 200 OK with a fixed 17-byte body `backend:v1/fixed\n` regardless
// of path or backend-index. The body is byte-identical across all three backend
// instances so that BYTES_SENT = 17 holds uniformly for records 2–4 (the three
// routed requests in the fixture-0006 workload — /api/v1/foo, /api/v1/bar,
// /api/v1/baz). This byte-identity is what makes Tier-E BYTES_SENT equality
// robust against RR endpoint-selection divergence between subject and reference
// (per SPEC §7.2).
//
// Body choice: `backend:v1/fixed\n` = 17 bytes.
// Verify: b(1)+a(1)+c(1)+k(1)+e(1)+n(1)+d(1)+:(1)+v(1)+1(1)+/(1)+f(1)+i(1)+x(1)+e(1)+d(1)+\n(1) = 17.
//
// Connection: close is set so reference Envoy's keepalive pool retires after
// each response, keeping upstream_cx_active gauges at 0 after the burst (the
// same discipline fixture-0005's backend uses per ADR-0062 rationale).
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
)

const fixedBody = "backend:v1/fixed\n" // exactly 17 bytes

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
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, fixedBody)
	})
	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
