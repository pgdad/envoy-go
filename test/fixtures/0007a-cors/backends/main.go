// Command backends is the controlled HTTP/1.1 backend for fixture
// 0007a-cors. It starts a server on a configurable port; each request
// returns 200 OK with body "hello\n" regardless of path or method.
//
// The fixed body lets the cors differential gate compare actual-request
// bodies byte-for-byte across reference Envoy and envoy-go without RR
// distribution divergence (only one backend instance is allocated per
// fixture run; the body is fixed).
//
// `Connection: close` is set so Envoy's keepalive upstream pool retires
// after each response, mirroring the discipline used by fixture-0005 and
// fixture-0006 backends.
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

const fixedBody = "hello\n" // exactly 6 bytes; pinned by SPEC §11.2 driver outline.

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
