// Command backends is the controlled HTTP/1.1 backend for fixture
// 0007b-iteration-probe. It starts a server on a configurable port; each
// request returns 200 OK with body equal to:
//
//   - the request body bytes verbatim, IF the request body is non-empty
//     (the modify-encode-data + buffer-data + local-reply-decode-data modes
//     all POST a body; this branch lets the encode-side mutation modes
//     observe a body of known length without depending on backend state).
//   - the fixed 8-byte body "backend\n" (7 chars + LF) otherwise (GET
//     requests for the no-body modes).
//
// The fixed body is intentionally longer than the encode-side replacement
// "MODIFIED\n" (9 bytes) so the modify-encode-data mode's copy+truncate
// semantics are observable: an 8-byte source slice receives the first 8
// bytes of "MODIFIED\n" → "MODIFIED" (no trailing LF). The driver asserts
// against this exact byte shape per SPEC §7.3 + the envoygotest filter's
// EncodeData implementation (internal/filter/http/envoygotest/filter.go).
//
// `Connection: close` is set so envoy-go's keepalive upstream pool retires
// after each response, mirroring the discipline used by fixture-0005,
// fixture-0006, and fixture-0007a backends.
//
// Usage:
//
//	backends --port <port>
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

const fixedBody = "backend\n" // 8 bytes; pinned by SPEC §7.3 per-mode table.

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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if len(body) > 0 {
			_, _ = w.Write(body)
		} else {
			_, _ = fmt.Fprint(w, fixedBody)
		}
	})
	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
